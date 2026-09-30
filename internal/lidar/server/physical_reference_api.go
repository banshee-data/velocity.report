package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/security"
)

// Physical references over HTTP: the one writer the macOS annotation client
// uses, so evidence validation, the origin ledger, review rules and the
// revision protocol live in Go alone.
//
// A pack is named by its handle, its directory relative to the annotation
// packs directory ("<pack>" or "<pack>/pack"), together with the pack digest
// the client opened. The handle is resolved beneath the configured directory
// and the digest must match what is there, so a request cannot name an
// arbitrary path and cannot write to a different pack that shares a name.
//
//	GET  /api/annotations/physical?pack=<handle>&pack_digest=<d>[&revision=<n>]
//	POST /api/annotations/physical/validate
//	POST /api/annotations/physical/save
//	POST /api/annotations/physical/review
//	GET  /api/annotations/physical/history?pack=<handle>&pack_digest=<d>
//	POST /api/annotations/physical/restore

// maxPhysicalRequestBytes bounds a request body. A pilot pack's references
// are kilobytes; the sidecar cap is a ceiling for the stored document, not a
// sensible request size.
const maxPhysicalRequestBytes = 8 << 20

// physicalError codes let a client explain a failure without parsing prose.
const (
	physicalCodeBadRequest        = "bad_request"
	physicalCodeNotConfigured     = "not_configured"
	physicalCodePackNotFound      = "pack_not_found"
	physicalCodePackMismatch      = "pack_mismatch"
	physicalCodeConflict          = "conflict"
	physicalCodeMembershipChanged = "membership_changed"
	physicalCodeBusy              = "busy"
	physicalCodeInvalid           = "invalid"
	physicalCodeNotFound          = "not_found"
	physicalCodeInternal          = "internal"
)

type physicalErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func (ws *Server) writePhysicalError(w http.ResponseWriter, status int, code, msg string) {
	ws.writeJSON(w, status, physicalErrorBody{Error: msg, Code: code})
}

// physicalPackState is what a client needs to edit against a pack: the
// document, its revision and exact-byte token, and the membership it was
// checked against.
type physicalPackState struct {
	Pack             string                   `json:"pack"`
	PackDir          string                   `json:"pack_dir"`
	DatasetID        string                   `json:"dataset_id"`
	PackDigest       string                   `json:"pack_digest"`
	Exists           bool                     `json:"exists"`
	Revision         int                      `json:"revision"`
	Digest           string                   `json:"digest"`
	ContentDigest    string                   `json:"content_digest"`
	Head             bool                     `json:"head"`
	MembershipDigest string                   `json:"membership_digest"`
	MembershipRev    int                      `json:"membership_revision"`
	Stale            []annotation.LinkProblem `json:"stale"`
	// ReviewDrift lists reviewed records whose membership changed, in the
	// frames they rest on, after they were reviewed.
	ReviewDrift []annotation.LinkProblem         `json:"review_drift"`
	Document    *annotation.PhysicalReferenceSet `json:"document"`
}

type physicalEditRequest struct {
	Pack             string                      `json:"pack"`
	PackDigest       string                      `json:"pack_digest"`
	BaseRevision     int                         `json:"base_revision"`
	BaseDigest       string                      `json:"base_digest"`
	MembershipDigest string                      `json:"membership_digest"`
	Author           string                      `json:"author"`
	Session          string                      `json:"session"`
	Objects          []annotation.PhysicalObject `json:"objects"`
}

type physicalEditResponse struct {
	Valid         bool                     `json:"valid"`
	Invalid       string                   `json:"invalid,omitempty"`
	LinkProblems  []annotation.LinkProblem `json:"link_problems"`
	ResetReviews  []string                 `json:"reset_reviews"`
	RenamedBodies map[string]string        `json:"renamed_bodies"`
	State         *physicalPackState       `json:"state,omitempty"`
}

type physicalReviewRequest struct {
	Pack             string `json:"pack"`
	PackDigest       string `json:"pack_digest"`
	BaseRevision     int    `json:"base_revision"`
	BaseDigest       string `json:"base_digest"`
	MembershipDigest string `json:"membership_digest"`
	Kind             string `json:"kind"`
	ObjectID         string `json:"object_id"`
	RecordID         string `json:"record_id"`
	Reviewer         string `json:"reviewer"`
	Session          string `json:"session"`
}

// resolvePhysicalPack opens the pack a handle names, beneath the configured
// directory, and checks it is the pack the client opened.
func (ws *Server) resolvePhysicalPack(w http.ResponseWriter, handle, digest string) (*annotation.Pack, string, bool) {
	if ws.annotationPacksDir == "" {
		ws.writePhysicalError(w, http.StatusNotImplemented, physicalCodeNotConfigured,
			"annotation packs directory is not configured: start the server with --lidar-annotation-dir")
		return nil, "", false
	}
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
		return nil, "", false
	}
	if digest == "" {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "pack_digest is required")
		return nil, "", false
	}
	dir, err := security.ResolvePathWithinDirectory(filepath.Join(ws.annotationPacksDir, filepath.FromSlash(clean)), ws.annotationPacksDir)
	if err != nil {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, fmt.Sprintf("pack handle %q: %v", handle, err))
		return nil, "", false
	}
	pack, err := annotation.OpenPack(dir)
	if err != nil {
		ws.writePhysicalError(w, http.StatusNotFound, physicalCodePackNotFound,
			fmt.Sprintf("no readable pack %q under the annotation packs directory: %v", handle, err))
		return nil, "", false
	}
	if pack.Manifest.PackDigest != digest {
		ws.writePhysicalError(w, http.StatusConflict, physicalCodePackMismatch,
			fmt.Sprintf("pack %q is %s, the client opened %s: this is not the same pack", handle, pack.Manifest.PackDigest, digest))
		return nil, "", false
	}
	return pack, clean, true
}

// physicalStateOf describes a document to a client: the document, its
// revision and token, whether it is the head, and the membership it was
// checked against.
func physicalStateOf(pack *annotation.Pack, handle string, doc, head *annotation.PhysicalReferenceSet, sidecar *annotation.Sidecar) (*physicalPackState, error) {
	content, err := doc.ContentDigest()
	if err != nil {
		return nil, err
	}
	stale := doc.Stale()
	if stale == nil {
		stale = []annotation.LinkProblem{}
	}
	drift := doc.ReviewDrift(pack, sidecar)
	if drift == nil {
		drift = []annotation.LinkProblem{}
	}
	return &physicalPackState{
		Pack: handle, PackDir: pack.Dir, DatasetID: pack.Manifest.DatasetID, PackDigest: pack.Manifest.PackDigest,
		Exists: doc.Digest() != "", Revision: doc.Revision, Digest: doc.Digest(), ContentDigest: content,
		Head:             doc.Revision == head.Revision && doc.Digest() == head.Digest(),
		MembershipDigest: sidecar.Digest(), MembershipRev: sidecar.Revision,
		Stale: stale, ReviewDrift: drift, Document: doc,
	}, nil
}

// loadPhysicalState reads the current document, or a retained revision, and
// the membership sidecar.
func loadPhysicalState(pack *annotation.Pack, handle string, revision int) (*physicalPackState, error) {
	head, err := annotation.LoadPhysicalReferences(pack)
	if err != nil && revision == 0 {
		return nil, err
	}
	doc := head
	if revision > 0 {
		if doc, err = annotation.LoadPhysicalReferenceRevision(pack, revision); err != nil {
			return nil, err
		}
		if head == nil {
			// A damaged head does not hide retained history, but nothing
			// read from it is the head.
			head = &annotation.PhysicalReferenceSet{Revision: -1}
		}
	}
	sidecar, err := annotation.LoadSidecar(pack)
	if err != nil {
		return nil, err
	}
	return physicalStateOf(pack, handle, doc, head, sidecar)
}

func (ws *Server) handlePhysicalReferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts GET requests")
		return
	}
	q := r.URL.Query()
	revision := 0
	if s := q.Get("revision"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "revision must be a positive integer")
			return
		}
		revision = n
	}
	pack, handle, ok := ws.resolvePhysicalPack(w, q.Get("pack"), q.Get("pack_digest"))
	if !ok {
		return
	}
	state, err := loadPhysicalState(pack, handle, revision)
	if err != nil {
		ws.writePhysicalFailure(w, err)
		return
	}
	ws.writeJSON(w, http.StatusOK, state)
}

func (ws *Server) handlePhysicalEdit(commit bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts POST requests")
			return
		}
		var req physicalEditRequest
		if !ws.decodePhysicalRequest(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.Author) == "" {
			ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "author is required")
			return
		}
		if req.Objects == nil {
			ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "objects is required: send [] to remove every object")
			return
		}
		pack, handle, ok := ws.resolvePhysicalPack(w, req.Pack, req.PackDigest)
		if !ok {
			return
		}
		edit := annotation.PhysicalEdit{
			BaseRevision: req.BaseRevision, BaseDigest: req.BaseDigest, MembershipDigest: req.MembershipDigest,
			Objects: req.Objects,
			Change:  annotation.Provenance{Author: req.Author, Session: req.Session, Operation: "physical_edit"},
		}
		// Validation first, even for a save, so a refusal carries the same
		// structured diagnostics a validate call returns.
		out, err := annotation.ValidatePhysicalEdit(pack, edit)
		if err != nil {
			ws.writePhysicalFailure(w, err)
			return
		}
		resp := physicalEditResponse{
			Valid: out.Valid(), Invalid: out.Invalid, LinkProblems: out.LinkProblems,
			ResetReviews: out.ResetReviews, RenamedBodies: out.RenamedBodies,
		}
		if !commit || !out.Valid() {
			status := http.StatusOK
			if commit {
				status = http.StatusUnprocessableEntity
			}
			ws.writeJSON(w, status, resp)
			return
		}
		saved, err := annotation.SavePhysicalEdit(pack, edit)
		if err != nil {
			ws.writePhysicalFailure(w, err)
			return
		}
		resp.ResetReviews, resp.RenamedBodies = saved.ResetReviews, saved.RenamedBodies
		ws.writeSavedState(w, pack, handle, saved, func(state *physicalPackState) any {
			resp.State = state
			return resp
		})
	}
}

func (ws *Server) handlePhysicalReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts POST requests")
		return
	}
	var req physicalReviewRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Reviewer) == "" {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "reviewer is required")
		return
	}
	pack, handle, ok := ws.resolvePhysicalPack(w, req.Pack, req.PackDigest)
	if !ok {
		return
	}
	saved, err := annotation.ReviewPhysicalRecord(pack, annotation.PhysicalReviewRequest{
		BaseRevision: req.BaseRevision, BaseDigest: req.BaseDigest, MembershipDigest: req.MembershipDigest,
		Kind: annotation.PhysicalRecordKind(req.Kind), ObjectID: req.ObjectID, RecordID: req.RecordID,
		Reviewer: annotation.Provenance{Author: req.Reviewer, Session: req.Session},
	})
	if err != nil {
		ws.writePhysicalFailure(w, err)
		return
	}
	ws.writeSavedState(w, pack, handle, saved, func(state *physicalPackState) any { return state })
}

// writeSavedState answers a committed write with the document the store
// returned, so the client never has to re-read what it just saved.
func (ws *Server) writeSavedState(w http.ResponseWriter, pack *annotation.Pack, handle string,
	saved *annotation.PhysicalEditOutcome, wrap func(*physicalPackState) any) {
	state, err := physicalStateOf(pack, handle, saved.Document, saved.Document, saved.Membership)
	if err != nil {
		ws.writePhysicalFailure(w, err)
		return
	}
	ws.writeJSON(w, http.StatusOK, wrap(state))
}

type physicalRestoreRequest struct {
	Pack             string `json:"pack"`
	PackDigest       string `json:"pack_digest"`
	BaseRevision     int    `json:"base_revision"`
	BaseDigest       string `json:"base_digest"`
	MembershipDigest string `json:"membership_digest"`
	Revision         int    `json:"revision"`
	Author           string `json:"author"`
	Session          string `json:"session"`
}

func (ws *Server) handlePhysicalHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts GET requests")
		return
	}
	pack, handle, ok := ws.resolvePhysicalPack(w, r.URL.Query().Get("pack"), r.URL.Query().Get("pack_digest"))
	if !ok {
		return
	}
	history, err := annotation.PhysicalReferenceHistory(pack)
	if err != nil {
		ws.writePhysicalFailure(w, err)
		return
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{"pack": handle, "revisions": history})
}

// handlePhysicalRestore makes a retained revision current as a new revision.
func (ws *Server) handlePhysicalRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts POST requests")
		return
	}
	var req physicalRestoreRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Author) == "" || req.Revision < 1 {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "author and a positive revision are required")
		return
	}
	pack, handle, ok := ws.resolvePhysicalPack(w, req.Pack, req.PackDigest)
	if !ok {
		return
	}
	saved, err := annotation.RestorePhysicalRevision(pack, annotation.PhysicalRestoreRequest{
		BaseRevision: req.BaseRevision, BaseDigest: req.BaseDigest, MembershipDigest: req.MembershipDigest,
		Revision: req.Revision, Author: req.Author, Session: req.Session,
	})
	if err != nil {
		ws.writePhysicalFailure(w, err)
		return
	}
	ws.writeSavedState(w, pack, handle, saved, func(state *physicalPackState) any { return state })
}

// decodePhysicalRequest is a bounded, strict decode: unknown fields and
// trailing data are refused rather than dropped.
func (ws *Server) decodePhysicalRequest(w http.ResponseWriter, r *http.Request, into any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPhysicalRequestBytes+1))
	if err != nil {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, fmt.Sprintf("read request: %v", err))
		return false
	}
	if len(body) > maxPhysicalRequestBytes {
		ws.writePhysicalError(w, http.StatusRequestEntityTooLarge, physicalCodeBadRequest,
			fmt.Sprintf("request exceeds %d bytes", maxPhysicalRequestBytes))
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, fmt.Sprintf("request is not valid: %v", err))
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "request has trailing data after the JSON object")
		return false
	}
	return true
}

// writePhysicalFailure maps a store error to a status and code.
func (ws *Server) writePhysicalFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, annotation.ErrSidecarConflict):
		ws.writePhysicalError(w, http.StatusConflict, physicalCodeConflict,
			"the physical references changed since they were loaded: reload and reconcile before saving")
	case errors.Is(err, annotation.ErrMembershipChanged):
		ws.writePhysicalError(w, http.StatusConflict, physicalCodeMembershipChanged, err.Error())
	case errors.Is(err, annotation.ErrSidecarBusy):
		ws.writePhysicalError(w, http.StatusLocked, physicalCodeBusy, "another writer holds the annotation lock: try again shortly")
	case errors.Is(err, annotation.ErrPhysicalRecordNotFound):
		ws.writePhysicalError(w, http.StatusNotFound, physicalCodeNotFound, err.Error())
	case errors.Is(err, annotation.ErrPhysicalInvalid):
		ws.writePhysicalError(w, http.StatusUnprocessableEntity, physicalCodeInvalid, err.Error())
	default:
		ws.writePhysicalError(w, http.StatusInternalServerError, physicalCodeInternal, err.Error())
	}
}
