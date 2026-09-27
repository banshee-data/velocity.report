package server

import (
	"errors"
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
	store := sqlite.NewSegmentStore(ws.db)
	selection, err := store.SelectionForCase(caseID)
	if err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			ws.writeJSONError(w, 404, "case has no selected annotation segment")
		} else {
			ws.writeJSONError(w, 500, err.Error())
		}
		return
	}
	// The job and its link to the segment are written together, so the worker
	// never claims a clip that does not yet say what it cuts. A second request
	// while one is queued or running returns that one.
	job, err := store.EnqueueClip(selection.SegmentID, fmt.Sprintf("clip for %s", caseID))
	if err != nil {
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	ws.writeJSON(w, 202, map[string]any{"job": job, "segment_id": selection.SegmentID})
}
