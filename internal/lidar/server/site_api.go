package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// SetSiteCanonicalPoseRequest sets a site's fixed position — a surveyed or
// hand-entered point such as the midpoint of the intersection — distinct from
// any one case's sensor pose. Label is optional; an empty one leaves whatever
// label the site already has.
type SetSiteCanonicalPoseRequest struct {
	Lat    float64 `json:"canonical_lat"`
	Lon    float64 `json:"canonical_lon"`
	Source string  `json:"canonical_source"`
	Label  string  `json:"label,omitempty"`
}

// handleSiteByToken handles /api/lidar/sites/{l16_token}.
//
// PUT sets the site's canonical pose. A site only exists once a case has been
// located there (see handleSetCaseLocation), so this 404s for a token nothing
// has visited yet rather than creating a site out of nowhere.
func (ws *Server) handleSiteByToken(w http.ResponseWriter, r *http.Request) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database configured")
		return
	}

	token := strings.TrimPrefix(r.URL.Path, "/api/lidar/sites/")
	if token == "" || token == r.URL.Path {
		ws.writeJSONError(w, http.StatusBadRequest, "missing l16_token in path")
		return
	}

	switch r.Method {
	case http.MethodPut:
		ws.handleSetSiteCanonicalPose(w, r, token)
	default:
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (ws *Server) handleSetSiteCanonicalPose(w http.ResponseWriter, r *http.Request, l16Token string) {
	var req SetSiteCanonicalPoseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Source != sqlite.SiteSourceSurveyed && req.Source != sqlite.SiteSourceOperator {
		ws.writeJSONError(w, http.StatusBadRequest, "canonical_source must be surveyed or operator")
		return
	}

	store := sqlite.NewSiteStore(ws.db)
	site, err := store.SetCanonicalPose(l16Token, req.Lat, req.Lon, req.Source, req.Label)
	if err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			ws.writeJSONError(w, http.StatusNotFound,
				"no site at this token yet — locate a case there first")
			return
		}
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ws.writeJSON(w, http.StatusOK, site)
}
