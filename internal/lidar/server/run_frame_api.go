package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/security"
)

// handleRunFrame maps an instant in a run to its recording's frame index,
// the number a velocityreport://run/<run_id>?frame=<n> link names: the
// 0-based index of a record in the run's VRLOG, background snapshots
// included, as the macOS visualiser counts frames. The web player's own
// scene export leaves the snapshots out, so its frame numbers differ; this is
// how the web names the frame the visualiser will show. Read-only.
//
// GET /api/lidar/runs/{run_id}/vrlog-frame?at_ns=<unix nanoseconds>
func (ws *Server) handleRunFrame(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "this endpoint only accepts GET requests")
		return
	}
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "database is not configured: check server startup includes --db-path")
		return
	}
	atNs, err := strconv.ParseInt(r.URL.Query().Get("at_ns"), 10, 64)
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "at_ns must be an instant in Unix nanoseconds")
		return
	}

	run, err := sqlite.NewRunStore(ws.db).GetRun(runID)
	if errors.Is(err, sqlite.ErrNotFound) {
		ws.writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not retrieve run: %v", err))
		return
	}
	if run.VRLogPath == "" {
		ws.writeJSONError(w, http.StatusNotFound, "this run has no recording")
		return
	}
	// Read from the run record, so held to the same read boundary as replay.
	vrlogPath, err := security.ResolvePathWithinDirectory(run.VRLogPath, ws.vrlogSafeDir)
	if err != nil {
		ws.writeJSONError(w, http.StatusNotFound, "this run's recording is outside the VRLOG directory")
		return
	}
	if _, err := os.Stat(filepath.Join(vrlogPath, "header.json")); err != nil {
		ws.writeJSONError(w, http.StatusNotFound, "this run's recording is missing")
		return
	}

	replayer, err := recorder.NewReplayer(vrlogPath)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not open the run's recording: %v", err))
		return
	}
	defer replayer.Close()
	total := replayer.TotalFrames()
	if total == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "this run's recording holds no frames")
		return
	}
	frame, ok := replayer.FrameAt(atNs)
	if !ok {
		ws.writeJSONError(w, http.StatusNotFound, "this run's recording holds no frame in its own time span")
		return
	}

	ws.writeJSON(w, http.StatusOK, map[string]interface{}{
		"run_id":       runID,
		"at_ns":        atNs,
		"frame":        frame,
		"total_frames": total,
	})
}
