package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/followingreview"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/google/uuid"
)

// handleFollowingReview exposes immutable preparation artefacts by registered
// run identity. No client can supply a path, replace evidence or overwrite audit.
func (ws *Server) handleFollowingReview(w http.ResponseWriter, r *http.Request, runID, action string) {
	w.Header().Set("Cache-Control", "no-store")
	if action != "" && action != "evidence" && action != "geometry" && action != "audit" && action != "analysis" {
		ws.writeJSONError(w, 404, "unknown following review endpoint")
		return
	}
	if (action == "analysis" && r.Method != http.MethodPost) || (r.Method != http.MethodGet && (r.Method != http.MethodPost || (action != "audit" && action != "analysis"))) {
		ws.writeJSONError(w, 405, "review accepts GET, POST audit, or POST analysis")
		return
	}
	if action == "analysis" {
		if !ws.followingAnalysisMu.TryLock() {
			ws.writeJSONError(w, 409, "another following analysis is running; retry after it completes")
			return
		}
		defer ws.followingAnalysisMu.Unlock()
	}
	if ws.db == nil {
		ws.writeJSONError(w, 503, "review database is unavailable")
		return
	}
	run, err := sqlite.NewRunStore(ws.db).GetRun(runID)
	if errors.Is(err, sqlite.ErrNotFound) {
		ws.writeJSONError(w, 404, "recording run not found")
		return
	}
	if err != nil {
		ws.writeJSONError(w, 500, "cannot read registered recording")
		return
	}
	session, err := ws.followingReviewCache.Open(run)
	if err != nil {
		status := http.StatusConflict
		if os.IsNotExist(err) {
			status = http.StatusServiceUnavailable
		}
		ws.writeJSONError(w, status, "paired review evidence unavailable or mismatched: "+err.Error())
		return
	}
	w.Header().Set("X-Canonical-Evidence-SHA256", session.Metadata.EvidenceSHA256)
	if action == "analysis" {
		analysis, err := session.Analyse(r.Context(), nil)
		if err != nil {
			ws.writeJSONError(w, 409, "following analysis refused: "+err.Error())
			return
		}
		ws.writeJSON(w, 200, analysis)
		return
	}
	if action == "" {
		ws.writeJSON(w, 200, session.Metadata)
		return
	}
	if action == "audit" {
		ws.handleFollowingAudit(w, r, session)
		return
	}
	at, err := strconv.ParseInt(r.URL.Query().Get("at_ns"), 10, 64)
	if err != nil {
		ws.writeJSONError(w, 400, "at_ns must be an exact Int64 capture timestamp")
		return
	}
	if action == "geometry" {
		g, err := session.Geometry(at, r.URL.Query().Get("follower"))
		if err != nil {
			ws.writeJSONError(w, 409, err.Error())
			return
		}
		ws.writeJSON(w, 200, g)
		return
	}
	b, _, err := session.Evidence(at)
	if err != nil {
		ws.writeJSONError(w, 409, err.Error())
		return
	}
	// These are the receipt-verified source bytes, not a JSON re-encoding.
	w.Header().Set("X-Evidence-SHA256", followingreview.Digest(b))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (ws *Server) handleFollowingAudit(w http.ResponseWriter, r *http.Request, s *followingreview.Session) {
	store := sqlite.NewFollowingAuditStore(ws.db)
	if r.Method == http.MethodGet {
		records, err := store.ListFollowingAudit(s.Metadata.RunID, s.Metadata.EvidenceSHA256)
		if err != nil {
			ws.writeJSONError(w, 500, "cannot read review audit")
			return
		}
		ws.writeJSON(w, 200, followingreview.Audit{Schema: "following_audit_v1", Binding: s.Metadata.Binding, Records: records})
		return
	}
	var req struct {
		Binding followingreview.Binding `json:"binding"`
		Record  followingreview.Record  `json:"record"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		ws.writeJSONError(w, 400, "invalid bounded audit record: "+err.Error())
		return
	}
	if err := d.Decode(new(any)); err != io.EOF {
		ws.writeJSONError(w, 400, "audit request contains trailing JSON")
		return
	}
	if req.Binding != s.Metadata.Binding {
		ws.writeJSONError(w, 409, "audit binding differs from current canonical evidence")
		return
	}
	_, rows, err := s.Evidence(req.Record.StartNanos)
	if err != nil {
		ws.writeJSONError(w, 409, err.Error())
		return
	}
	if err = followingreview.ValidateRecord(req.Record, rows, s.Metadata.RunID, s.Metadata.Start, s.Metadata.End); err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	req.Record.ID = uuid.MustParse(req.Record.ID).String()
	// Swift's Playback requires a JSON array. Missing viewing context remains
	// unknown, but must never poison the immutable audit with a null array.
	if req.Record.Playback.FrameSourceIDs == nil {
		req.Record.Playback.FrameSourceIDs = []string{}
	}
	binding, _ := json.Marshal(req.Binding)
	record, _ := json.Marshal(req.Record)
	created, err := store.AppendFollowingAudit(s.Metadata.RunID, s.Metadata.EvidenceSHA256, req.Record.ID, binding, record)
	if errors.Is(err, sqlite.ErrFollowingAuditConflict) {
		ws.writeJSONError(w, 409, "audit identity already has different immutable content")
		return
	}
	if errors.Is(err, sqlite.ErrFollowingAuditLimit) {
		ws.writeJSONError(w, 413, "review audit has reached its bounded record or byte limit")
		return
	}
	if err != nil {
		ws.writeJSONError(w, 500, "cannot persist review audit")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	ws.writeJSON(w, status, map[string]any{"created": created, "record": req.Record})
}
