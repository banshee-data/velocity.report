package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/banshee-data/velocity.report/internal/lidar/geoindex"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// SetCaseLocationRequest labels a replay case with where it was captured.
//
// Only the position is accepted. The S2 tokens are derived from it server-side
// with Parent, never taken from the caller: a family assembled by a client
// could disagree with itself, and the guide treats that as a hard provenance
// error rather than something to reconcile.
type SetCaseLocationRequest struct {
	Lat float64 `json:"origin_lat"`
	Lon float64 `json:"origin_lon"`
	// Source records how the position was established: surveyed, operator, or
	// fix. Empty means operator.
	Source string `json:"geographic_source,omitempty"`
}

// handleSetCaseLocation records a replay case's capture position.
//
// POST /api/lidar/scenes/{replay_case_id}/location
// DELETE the same path clears it.
func (ws *Server) handleSetCaseLocation(w http.ResponseWriter, r *http.Request, replayCaseID string) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database configured")
		return
	}
	store := sqlite.NewReplayCaseStore(ws.db)

	if r.Method == http.MethodDelete {
		if err := store.ClearCaseLocation(replayCaseID); err != nil {
			if errors.Is(err, sqlite.ErrNotFound) {
				ws.writeJSONError(w, http.StatusNotFound, "replay case not found")
				return
			}
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ws.writeJSON(w, http.StatusOK, map[string]any{
			"replay_case_id":    replayCaseID,
			"geographic_status": sqlite.GeoUnavailable,
		})
		return
	}

	var req SetCaseLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Source != "" && req.Source != sqlite.GeoSourceSurveyed &&
		req.Source != sqlite.GeoSourceOperator && req.Source != sqlite.GeoSourceFix {
		ws.writeJSONError(w, http.StatusBadRequest,
			"geographic_source must be surveyed, operator, or fix")
		return
	}

	loc, err := store.SetCaseLocation(replayCaseID, req.Lat, req.Lon, req.Source)
	if err != nil {
		switch {
		case errors.Is(err, geoindex.ErrNoPosition):
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, sqlite.ErrNotFound):
			ws.writeJSONError(w, http.StatusNotFound, "replay case not found")
		default:
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{
		"replay_case_id": replayCaseID,
		"location":       loc,
	})
}

// handleSceneMap returns every located site and the cases captured there.
//
// GET /api/lidar/scene-map
//
// A site is an L10 cell, so many visits to one junction collapse into one
// entry. Each carries the cell's centre and bounds, so a map can draw the site
// rather than only pin a position inside it.
func (ws *Server) handleSceneMap(w http.ResponseWriter, r *http.Request) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database configured")
		return
	}
	sites, err := sqlite.NewReplayCaseStore(ws.db).SceneSites()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var cases int
	for _, s := range sites {
		cases += s.CaseCount
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{
		"sites":         sites,
		"site_count":    len(sites),
		"case_count":    cases,
		"coarse_level":  geoindex.LevelCoarse,
		"fine_level":    geoindex.LevelFine,
		"precise_level": geoindex.LevelPrecise,
	})
}
