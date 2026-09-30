package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/security"
	"github.com/banshee-data/velocity.report/internal/version"
)

// Freezing a split from the annotation window.
//
// The CLI freezes a draft whose pack paths are wherever the operator put
// them. Over HTTP the packs are the server's own, so a draft names them by
// handle beneath the annotation packs directory, as the physical-reference
// API does, and the frozen split is written beneath the same directory in
// splits/, write-once. Every rule FreezeSplit applies holds here; the preview
// is the same computation without the write, so what the window shows is
// what the freeze would do.
//
//	POST /api/annotations/split/preview   {draft}
//	POST /api/annotations/split/freeze    {draft, author, output, supersedes?, note?, guard_seconds?}
//	GET  /api/annotations/splits

const splitsDirName = "splits"

// splitNamePattern is a file name a split may be written under: one path
// element, letters, digits, dots, dashes and underscores, ending in .json.
var splitNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.json$`)

type splitFreezeRequest struct {
	Draft        annotation.SplitDraft `json:"draft"`
	Author       string                `json:"author"`
	Output       string                `json:"output"`
	Supersedes   string                `json:"supersedes,omitempty"`
	GuardSeconds *float64              `json:"guard_seconds,omitempty"`
	ForConfig    string                `json:"for_config_hash,omitempty"`
	ForParams    string                `json:"for_params_hash,omitempty"`
}

type splitPreviewRequest struct {
	Draft        annotation.SplitDraft `json:"draft"`
	Supersedes   string                `json:"supersedes,omitempty"`
	GuardSeconds *float64              `json:"guard_seconds,omitempty"`
}

type splitListing struct {
	Name        string `json:"name"`
	Revision    int    `json:"revision"`
	SplitDigest string `json:"split_digest"`
	FrozenUTC   string `json:"frozen_utc"`
	Author      string `json:"author"`
	Packs       int    `json:"packs"`
	Error       string `json:"error,omitempty"`
}

// resolveDraftPacks holds every pack path in a draft to a handle beneath the
// packs directory, and rewrites it absolute so FreezeSplit reads the right
// pack whatever base directory it is given.
func (ws *Server) resolveDraftPacks(w http.ResponseWriter, draft *annotation.SplitDraft) bool {
	if ws.annotationPacksDir == "" {
		ws.writePhysicalError(w, http.StatusNotImplemented, physicalCodeNotConfigured,
			"annotation packs directory is not configured: start the server with --lidar-annotation-dir")
		return false
	}
	if len(draft.Packs) == 0 && len(draft.Cases) == 0 {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "the draft names no packs and no cases")
		return false
	}
	for i := range draft.Packs {
		dp := &draft.Packs[i]
		dir, ok := ws.packHandleDir(w, dp.Dir)
		if !ok {
			return false
		}
		dp.Dir = dir
		if dp.SplitManifest != "" {
			// A manifest file beside the pack, named relative to the packs
			// directory, and nowhere else.
			path, err := security.ResolvePathWithinDirectory(
				filepath.Join(ws.annotationPacksDir, filepath.FromSlash(dp.SplitManifest)), ws.annotationPacksDir)
			if err != nil || filepath.IsAbs(dp.SplitManifest) || strings.Contains(dp.SplitManifest, "..") {
				ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest,
					fmt.Sprintf("draft pack %d: split_manifest %q is not beneath the annotation packs directory", i, dp.SplitManifest))
				return false
			}
			dp.SplitManifest = path
		}
	}
	return true
}

// packHandleDir is a pack handle's directory beneath the packs directory,
// by the same rule as the physical-reference API, without opening it.
func (ws *Server) packHandleDir(w http.ResponseWriter, handle string) (string, bool) {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(handle)))
	parts := strings.Split(clean, "/")
	valid := handle != "" && !filepath.IsAbs(handle) && clean == handle && len(parts) <= 2
	for _, part := range parts {
		valid = valid && part != "" && part != "." && part != ".."
	}
	if len(parts) == 2 {
		valid = valid && parts[1] == "pack"
	}
	if !valid {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest,
			fmt.Sprintf("pack handle %q is not a pack directory name under the annotation packs directory", handle))
		return "", false
	}
	dir, err := security.ResolvePathWithinDirectory(filepath.Join(ws.annotationPacksDir, filepath.FromSlash(clean)), ws.annotationPacksDir)
	if err != nil {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, fmt.Sprintf("pack handle %q: %v", handle, err))
		return "", false
	}
	return dir, true
}

// splitPath is the file a split name reads from or writes to.
func (ws *Server) splitPath(w http.ResponseWriter, name string) (string, bool) {
	if !splitNamePattern.MatchString(name) {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest,
			fmt.Sprintf("split name %q: use letters, digits, dots, dashes and underscores, ending in .json", name))
		return "", false
	}
	dir := filepath.Join(ws.annotationPacksDir, splitsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		ws.writePhysicalError(w, http.StatusInternalServerError, physicalCodeInternal, fmt.Sprintf("create splits directory: %v", err))
		return "", false
	}
	path, err := security.ResolvePathWithinDirectory(filepath.Join(dir, name), dir)
	if err != nil {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, fmt.Sprintf("split name %q: %v", name, err))
		return "", false
	}
	return path, true
}

func (ws *Server) freezeOptions(w http.ResponseWriter, draft *annotation.SplitDraft, supersedes string, guard *float64,
	author, forConfig, forParams string) (annotation.FreezeOptions, bool) {
	opts := annotation.FreezeOptions{
		Draft: draft, BaseDir: ws.annotationPacksDir, Author: author, Now: time.Now(),
		BuildVersion: version.Version, BuildGitSHA: version.GitSHA,
		ForConfigHash: forConfig, ForParamsHash: forParams, GuardSeconds: annotation.DefaultSplitGuardSeconds,
	}
	if guard != nil {
		opts.GuardSeconds = *guard
	}
	if supersedes != "" {
		path, ok := ws.splitPath(w, supersedes)
		if !ok {
			return opts, false
		}
		prev, err := annotation.LoadFrozenSplit(path)
		if err != nil {
			ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, fmt.Sprintf("supersedes %q: %v", supersedes, err))
			return opts, false
		}
		opts.Supersedes = prev
	}
	return opts, true
}

// handleSplitPreview says what freezing the draft would pin and what stops
// it, without writing.
func (ws *Server) handleSplitPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts POST requests")
		return
	}
	var req splitPreviewRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	if !ws.resolveDraftPacks(w, &req.Draft) {
		return
	}
	// Preview needs no author; the freeze does. A placeholder keeps the
	// options valid without the preview ever claiming one.
	opts, ok := ws.freezeOptions(w, &req.Draft, req.Supersedes, req.GuardSeconds, "preview", "", "")
	if !ok {
		return
	}
	preview, err := annotation.PreviewFreeze(opts)
	if err != nil {
		ws.writePhysicalError(w, http.StatusUnprocessableEntity, physicalCodeInvalid, err.Error())
		return
	}
	ws.writeJSON(w, http.StatusOK, preview)
}

// handleSplitFreeze freezes the draft and writes the split, write-once,
// beneath the packs directory.
func (ws *Server) handleSplitFreeze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts POST requests")
		return
	}
	var req splitFreezeRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Author) == "" {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "author is required: a frozen split says who answered for it")
		return
	}
	if !ws.resolveDraftPacks(w, &req.Draft) {
		return
	}
	output, ok := ws.splitPath(w, req.Output)
	if !ok {
		return
	}
	if _, err := os.Stat(output); err == nil {
		ws.writePhysicalError(w, http.StatusConflict, physicalCodeConflict,
			fmt.Sprintf("split %q already exists: a frozen split is written once; freeze a new revision under another name", req.Output))
		return
	}
	opts, ok := ws.freezeOptions(w, &req.Draft, req.Supersedes, req.GuardSeconds, req.Author, req.ForConfig, req.ForParams)
	if !ok {
		return
	}
	frozen, err := annotation.FreezeSplit(opts)
	if err != nil {
		ws.writePhysicalError(w, http.StatusUnprocessableEntity, physicalCodeInvalid, err.Error())
		return
	}
	if err := annotation.WriteFrozenSplit(output, frozen); err != nil {
		ws.writePhysicalError(w, http.StatusInternalServerError, physicalCodeInternal, fmt.Sprintf("write split: %v", err))
		return
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{
		"name": req.Output, "path": output, "split_digest": frozen.SplitDigest, "revision": frozen.Revision,
		"split": frozen,
	})
}

// handleSplits lists the frozen splits beneath the packs directory.
func (ws *Server) handleSplits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts GET requests")
		return
	}
	if ws.annotationPacksDir == "" {
		ws.writePhysicalError(w, http.StatusNotImplemented, physicalCodeNotConfigured,
			"annotation packs directory is not configured: start the server with --lidar-annotation-dir")
		return
	}
	entries, err := os.ReadDir(filepath.Join(ws.annotationPacksDir, splitsDirName))
	if err != nil && !os.IsNotExist(err) {
		ws.writePhysicalError(w, http.StatusInternalServerError, physicalCodeInternal, err.Error())
		return
	}
	listed := []splitListing{}
	for _, entry := range entries {
		if entry.IsDir() || !splitNamePattern.MatchString(entry.Name()) {
			continue
		}
		item := splitListing{Name: entry.Name()}
		f, err := annotation.LoadFrozenSplit(filepath.Join(ws.annotationPacksDir, splitsDirName, entry.Name()))
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Revision, item.SplitDigest, item.FrozenUTC, item.Author, item.Packs =
				f.Revision, f.SplitDigest, f.Frozen.FrozenUTC, f.Frozen.Author, len(f.Packs)
		}
		listed = append(listed, item)
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{"splits": listed, "count": len(listed)})
}
