package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/lanefieldrun"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/security"
)

// resolveRunVRLOG holds a run's stored recording path to the VRLOG directory,
// as replay and the scene export do, and requires the recording's header.
func (ws *Server) resolveRunVRLOG(stored string) (string, error) {
	path, err := security.ResolvePathWithinDirectory(stored, ws.vrlogSafeDir)
	if err != nil {
		return "", fmt.Errorf("the recording is outside the VRLOG directory")
	}
	if _, err := os.Stat(filepath.Join(path, "header.json")); err != nil {
		return "", fmt.Errorf("the recording is missing")
	}
	return path, nil
}

// handleRunLaneField serves GET /api/lidar/runs/{run_id}/lane-field?part=…:
// one part of the run's lane field (docs/plans/lidar-lane-proposals-from-tracks-plan.md
// §1), from its stored observations or else its recording, in the JSON `gerardus upload-lane-field` reads, byte for byte what
// `velocity lidar lane-field` writes. k is always five and cannot be asked
// for: no path serves a thinner field than Gerardus receives (ADR-0011).
func (ws *Server) handleRunLaneField(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "this endpoint only accepts GET and HEAD requests")
		return
	}
	q := r.URL.Query()
	if q.Has("k") {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("k is fixed at %d and cannot be changed", l8analytics.LaneFieldK))
		return
	}
	part, err := l8analytics.ParseLaneFieldPart(q.Get("part"))
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	field, _, err := lanefieldrun.Build(sqlite.NewRunStore(ws.db), runID, part, ws.resolveRunVRLOG)
	if errors.Is(err, sqlite.ErrNotFound) {
		ws.writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if errors.Is(err, lanefieldrun.ErrNoRecording) {
		ws.writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not build the lane field: %v", err))
		return
	}
	body, err := field.JSON()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not encode the lane field: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		return
	}
	_, _ = w.Write(body)
}
