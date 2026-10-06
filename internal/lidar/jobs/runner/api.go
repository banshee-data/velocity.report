package runner

import (
	"archive/tar"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
	"github.com/banshee-data/velocity.report/internal/security"
)

// API is the worker's HTTP surface: submit a job, watch it, fetch its
// bundle. Everything but /health needs the bearer token. It is LAN only by
// policy; the token is what stops a stray process on the LAN queueing a
// twenty-minute replay.
type API struct {
	Runner *Runner
	Token  string
}

// Handler returns the mux.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "worker": a.Runner.Profile.WorkerID})
	})
	mux.HandleFunc("GET /api/worker/status", a.auth(a.status))
	mux.HandleFunc("GET /api/worker/kinds", a.auth(a.kinds))
	mux.HandleFunc("GET /api/worker/jobs", a.auth(a.list))
	mux.HandleFunc("POST /api/worker/jobs", a.auth(a.submit))
	mux.HandleFunc("POST /api/worker/campaigns", a.auth(a.submitCampaign))
	mux.HandleFunc("GET /api/worker/jobs/{id}", a.auth(a.get))
	mux.HandleFunc("GET /api/worker/jobs/{id}/log", a.auth(a.log))
	mux.HandleFunc("POST /api/worker/jobs/{id}/cancel", a.auth(a.cancel))
	mux.HandleFunc("GET /api/worker/jobs/{id}/bundle", a.auth(a.bundle))
	mux.HandleFunc("GET /api/worker/jobs/{id}/bundle.tar", a.auth(a.bundleTar))
	mux.HandleFunc("GET /api/worker/jobs/{id}/files/{path...}", a.auth(a.file))
	return mux
}

func (a *API) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.Token == "" {
			writeError(w, http.StatusServiceUnavailable, "this worker has no token configured; refusing every request")
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(a.Token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="velocity worker"`)
			writeError(w, http.StatusUnauthorized, "a bearer token is required")
			return
		}
		next(w, r)
	}
}

// Status is what the worker says about itself.
type Status struct {
	Worker           jobs.WorkerProfile `json:"worker"`
	Now              string             `json:"now_utc"`
	Current          string             `json:"current_attempt,omitempty"`
	Queued           int                `json:"queued"`
	CaptureRoot      string             `json:"capture_root"`
	VerifiedCaptures []jobs.Digest      `json:"verified_captures"`
	Kinds            []string           `json:"kinds"`
	FreeBytes        uint64             `json:"free_bytes"`
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	all, err := a.Runner.Store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	queued := 0
	for _, rec := range all {
		if rec.Attempt.State == jobs.StateQueued && len(rec.Children) == 0 {
			queued++
		}
	}
	var kinds []string
	for _, k := range jobs.KindNames() {
		if a.Runner.Available(k) == nil {
			kinds = append(kinds, k)
		}
	}
	writeJSONResponse(w, http.StatusOK, Status{
		Worker: a.Runner.Profile, Now: a.Runner.now().UTC().Format(time.RFC3339), Current: a.Runner.Current(),
		Queued: queued, CaptureRoot: a.Runner.Captures.Root(), VerifiedCaptures: a.Runner.Captures.Verified(),
		Kinds: kinds, FreeBytes: freeBytes(a.Runner.Store.Root()),
	})
}

func (a *API) kinds(w http.ResponseWriter, r *http.Request) {
	type kind struct {
		Name      string `json:"name"`
		Tool      string `json:"tool"`
		OnMain    bool   `json:"on_main"`
		Available bool   `json:"available"`
		Reason    string `json:"reason,omitempty"`
	}
	var out []kind
	for _, name := range jobs.KindNames() {
		k := jobs.Kinds[name]
		entry := kind{Name: name, Tool: k.Tool, OnMain: k.OnMain, Available: true}
		if err := a.Runner.Available(name); err != nil {
			entry.Available, entry.Reason = false, err.Error()
		}
		out = append(out, entry)
	}
	writeJSONResponse(w, http.StatusOK, out)
}

// Listing is one attempt in a list: enough to choose one, not its whole job.
type Listing struct {
	AttemptID         string        `json:"attempt_id"`
	Kind              string        `json:"kind"`
	State             jobs.State    `json:"state"`
	Label             string        `json:"label,omitempty"`
	Parent            string        `json:"parent,omitempty"`
	Children          int           `json:"children,omitempty"`
	Note              string        `json:"note,omitempty"`
	RunIdentityDigest jobs.Digest   `json:"run_identity_digest"`
	CreatedAt         time.Time     `json:"created_at"`
	StartedAt         *time.Time    `json:"started_at,omitempty"`
	FinishedAt        *time.Time    `json:"finished_at,omitempty"`
	Progress          jobs.Progress `json:"progress"`
	Failure           *jobs.Failure `json:"failure,omitempty"`
	BundleDigest      jobs.Digest   `json:"bundle_digest,omitempty"`
}

func listing(rec Record) Listing {
	return Listing{
		AttemptID: rec.Attempt.AttemptID, Kind: rec.Job.Kind, State: rec.Attempt.State, Label: rec.Label,
		Parent: rec.Parent, Children: len(rec.Children), Note: rec.Job.Note,
		RunIdentityDigest: rec.RunIdentityDigest, CreatedAt: rec.Attempt.CreatedAt,
		StartedAt: rec.Attempt.StartedAt, FinishedAt: rec.Attempt.FinishedAt, Progress: rec.Attempt.Progress,
		Failure: rec.Attempt.Failure, BundleDigest: rec.Attempt.BundleDigest,
	}
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	all, err := a.Runner.Store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	state := r.URL.Query().Get("state")
	out := make([]Listing, 0, len(all))
	// Newest first: what an operator wants to see is what just happened.
	for i := len(all) - 1; i >= 0; i-- {
		if state != "" && string(all[i].Attempt.State) != state {
			continue
		}
		out = append(out, listing(all[i]))
	}
	writeJSONResponse(w, http.StatusOK, out)
}

func (a *API) submit(w http.ResponseWriter, r *http.Request) {
	var req jobs.JobRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.Runner.Available(req.Kind); err != nil {
		writeError(w, http.StatusNotImplemented, err.Error())
		return
	}
	rec, err := a.Runner.Store.Submit(req, a.Runner.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONResponse(w, http.StatusCreated, listing(rec))
}

func (a *API) submitCampaign(w http.ResponseWriter, r *http.Request) {
	var c Campaign
	if err := decodeBody(r, &c); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, _, err := c.Expand(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.Runner.Available(c.Kind); err != nil {
		writeError(w, http.StatusNotImplemented, err.Error())
		return
	}
	rec, err := a.Runner.Store.SubmitCampaign(c, a.Runner.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONResponse(w, http.StatusCreated, listing(rec))
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.lookup(w, r)
	if !ok {
		return
	}
	writeJSONResponse(w, http.StatusOK, rec)
}

func (a *API) log(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.lookup(w, r)
	if !ok {
		return
	}
	n := 200
	if q := r.URL.Query().Get("lines"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			n = v
		}
	}
	lines, err := a.Runner.Store.LogTail(rec.Attempt.AttemptID, n)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"attempt_id": rec.Attempt.AttemptID, "lines": lines})
}

func (a *API) cancel(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.lookup(w, r)
	if !ok {
		return
	}
	id := rec.Attempt.AttemptID
	if a.Runner.Cancel(id) {
		writeJSONResponse(w, http.StatusAccepted, map[string]any{"attempt_id": id, "cancelling": true})
		return
	}
	done, err := a.Runner.Store.Transition(id, jobs.StateCancelled, jobs.ActorOperator, a.Runner.now())
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	// A cancelled campaign cancels what has not started. What is running
	// is cancelled through the runner; what is done stays done.
	for _, child := range done.Children {
		if a.Runner.Cancel(child) {
			continue
		}
		_, _ = a.Runner.Store.Transition(child, jobs.StateCancelled, jobs.ActorOperator, a.Runner.now())
	}
	writeJSONResponse(w, http.StatusOK, listing(done))
}

func (a *API) bundle(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.lookup(w, r)
	if !ok {
		return
	}
	m, err := a.Runner.Store.ReadBundle(rec.Attempt.AttemptID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "this attempt has written no bundle yet")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, m)
}

// file serves one file of a bundle. The path is the manifest's relative
// path, and it is checked twice: as a clean relative path, and as resolving
// inside this attempt's bundle directory.
func (a *API) file(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.lookup(w, r)
	if !ok {
		return
	}
	rel := r.PathValue("path")
	if rel == "" || path.Clean("/"+rel) != "/"+rel {
		writeError(w, http.StatusBadRequest, "path is not a clean relative path")
		return
	}
	dir := a.Runner.Store.BundlePath(rec.Attempt.AttemptID)
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := security.ValidatePathWithinDirectory(full, dir); err != nil {
		writeError(w, http.StatusBadRequest, "path is outside the bundle")
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "no such file in the bundle")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	http.ServeFile(w, r, full)
}

// bundleTar streams the whole bundle as one tar, for a hub or an operator
// on another host to fetch in one request.
func (a *API) bundleTar(w http.ResponseWriter, r *http.Request) {
	rec, ok := a.lookup(w, r)
	if !ok {
		return
	}
	dir := a.Runner.Store.BundlePath(rec.Attempt.AttemptID)
	if _, err := os.Stat(dir); err != nil {
		writeError(w, http.StatusNotFound, "this attempt has written no bundle yet")
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar"`, rec.Attempt.AttemptID))
	tw := tar.NewWriter(w)
	defer tw.Close()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: info.Size(), ModTime: info.ModTime()}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	})
}

func (a *API) lookup(w http.ResponseWriter, r *http.Request) (Record, bool) {
	rec, err := a.Runner.Store.Get(r.PathValue("id"))
	if err != nil {
		if IsNotFound(err) {
			writeError(w, http.StatusNotFound, "no such attempt")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return Record{}, false
	}
	return rec, true
}

func decodeBody(r *http.Request, into any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("request body: %w", err)
	}
	// Decode reads exactly one JSON value and says nothing about what
	// follows it, so a body with a second value appended would otherwise
	// be silently ignored rather than rejected.
	if dec.More() {
		return fmt.Errorf("request body: unexpected data after the JSON value")
	}
	return nil
}

func writeJSONResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSONResponse(w, status, map[string]string{"error": msg})
}
