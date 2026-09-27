package server

import (
	"fmt"
	"net/http"

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// handleSceneClip queues an offline replay and annotation export for a case
// made from a selected segment. A freehand case has no selection provenance.
func (ws *Server) handleSceneClip(w http.ResponseWriter, r *http.Request, caseID string) {
	if ws.annotationPacksDir == "" {
		ws.writeJSONError(w, 501, "annotation packs directory is not configured")
		return
	}
	var segmentID string
	if err := ws.db.QueryRow(`SELECT segment_id FROM lidar_segment_selections WHERE replay_case_id=?`, caseID).Scan(&segmentID); err != nil {
		ws.writeJSONError(w, 404, "case has no selected annotation segment")
		return
	}
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, 503, err.Error())
		return
	}
	job, err := store.EnqueueJob("vrlog_record", segmentID, "", fmt.Sprintf("clip for %s", caseID))
	if err != nil {
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	if _, err = ws.db.Exec(`INSERT OR IGNORE INTO lidar_segment_clip_jobs(job_id,segment_id,replay_case_id) VALUES(?,?,?)`, job.JobID, segmentID, caseID); err != nil {
		_ = store.FinishJob(job.JobID, sqlite.JobFailed, err.Error())
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	ws.writeJSON(w, 202, map[string]any{"job": job, "segment_id": segmentID})
}
