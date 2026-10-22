package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/banshee-data/velocity.report/internal/lidar/geoindex"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// SetClipLocationRequest labels a clip with where it was captured.
//
// Only the position is accepted. The S2 tokens are derived from it server-side
// with Parent, never taken from the caller: a family assembled by a client
// could disagree with itself, and the guide treats that as a hard provenance
// error rather than something to reconcile.
type SetClipLocationRequest struct {
	Lat float64 `json:"origin_lat"`
	Lon float64 `json:"origin_lon"`
	// Source records how the position was established: surveyed, operator, or
	// fix. Empty means operator.
	Source string `json:"geographic_source,omitempty"`
}

// handleSetClipLocation records a clip's capture position.
//
// POST /api/lidar/clips/{clip_id}/location
// DELETE the same path clears it.
func (ws *Server) handleSetClipLocation(w http.ResponseWriter, r *http.Request, clipID string) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database configured")
		return
	}
	store := sqlite.NewClipStore(ws.db)

	if r.Method == http.MethodDelete {
		if err := store.ClearClipLocation(clipID); err != nil {
			if errors.Is(err, sqlite.ErrNotFound) {
				ws.writeJSONError(w, http.StatusNotFound, "clip not found")
				return
			}
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ws.writeJSON(w, http.StatusOK, map[string]any{
			"clip_id":           clipID,
			"geographic_status": sqlite.GeoUnavailable,
		})
		return
	}

	var req SetClipLocationRequest
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

	loc, err := store.SetClipLocation(clipID, req.Lat, req.Lon, req.Source)
	if err != nil {
		switch {
		case errors.Is(err, geoindex.ErrNoPosition):
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, sqlite.ErrNotFound):
			ws.writeJSONError(w, http.StatusNotFound, "clip not found")
		default:
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{
		"clip_id":  clipID,
		"location": loc,
	})
}

// handleClipLocations returns every area captures have been taken in, each holding
// the sites within it and the clips captured at each one.
//
// GET /api/lidar/clip-locations
//
// An area is an L10 cell, district-scale, so it typically holds several
// distinct sites (L16 cells — a junction and its approaches). Each area
// carries its cell's centre and bounds, so a map can draw it rather than only
// pin a position inside it.
func (ws *Server) handleClipLocations(w http.ResponseWriter, r *http.Request) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database configured")
		return
	}
	districts, err := sqlite.NewClipStore(ws.db).Districts()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var clips, sites int
	for _, a := range districts {
		clips += a.ClipCount
		sites += len(a.Sites)
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{
		// Registered aliases for the previous DMG; current clients use districts and clips.
		"areas": districts, "area_count": len(districts), "case_count": clips,
		"districts":      districts,
		"district_count": len(districts),
		"site_count":     sites,
		"clip_count":     clips,
		"coarse_level":   geoindex.LevelCoarse,
		"fine_level":     geoindex.LevelFine,
		"precise_level":  geoindex.LevelPrecise,
	})
}
