package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/scene"
)

// sceneIDPattern constrains a scene identifier to what is safe in a URL and a
// filesystem path, because the identifier also names the published asset
// directory.
var sceneIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// sceneRequest is the editable shape of a scene. Capture times are accepted as
// RFC 3339 so an editor can send what a date field produced, and are stored as
// the Unix nanoseconds the recording uses.
type sceneRequest struct {
	SceneID     string  `json:"scene_id"`
	SiteID      *int    `json:"site_id"`
	Title       string  `json:"title"`
	Description *string `json:"description"`

	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`

	CapturedStart *string `json:"captured_start"`
	CapturedEnd   *string `json:"captured_end"`

	SourceCapture     *string `json:"source_capture"`
	SourceVRLOGSHA256 *string `json:"source_vrlog_sha256"`
	FrameCount        *int    `json:"frame_count"`
	FrameStride       *int    `json:"frame_stride"`

	AssetPath *string `json:"asset_path"`
	Published bool    `json:"published"`

	// Vantages is the named-viewpoint list. Sent as structured JSON rather than
	// a string so an editor cannot store something the viewer will choke on.
	Vantages []scene.Vantage `json:"vantages"`
}

// handleScenes routes scene requests. URL forms are /api/scenes and
// /api/scenes/<scene-id>.
func (s *Server) handleScenes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/scenes"), "/")

	if id == "" {
		switch r.Method {
		case http.MethodGet:
			s.listScenes(w, r)
		case http.MethodPost:
			s.createScene(w, r)
		default:
			s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getScene(w, r, id)
	case http.MethodPut:
		s.updateScene(w, r, id)
	case http.MethodDelete:
		s.deleteScene(w, r, id)
	default:
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) listScenes(w http.ResponseWriter, r *http.Request) {
	scenes, err := s.db.GetAllScenes(r.Context())
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve scenes: %v", err))
		return
	}
	if err := json.NewEncoder(w).Encode(scenes); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode scenes")
	}
}

func (s *Server) getScene(w http.ResponseWriter, r *http.Request, id string) {
	scene, err := s.db.GetScene(r.Context(), id)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve scene: %v", err))
		return
	}
	if scene == nil {
		s.writeJSONError(w, http.StatusNotFound, "Scene not found")
		return
	}
	if err := json.NewEncoder(w).Encode(scene); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode scene")
	}
}

func (s *Server) createScene(w http.ResponseWriter, r *http.Request) {
	var req sceneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	scene, problem := sceneFromRequest(req, req.SceneID)
	if problem != "" {
		s.writeJSONError(w, http.StatusBadRequest, problem)
		return
	}

	existing, err := s.db.GetScene(r.Context(), scene.SceneID)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to check scene: %v", err))
		return
	}
	if existing != nil {
		s.writeJSONError(w, http.StatusConflict, fmt.Sprintf("A scene named %q already exists", scene.SceneID))
		return
	}

	if err := s.db.CreateScene(r.Context(), scene); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create scene: %v", err))
		return
	}

	created, err := s.db.GetScene(r.Context(), scene.SceneID)
	if err != nil || created == nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Scene was created but could not be read back")
		return
	}
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(created); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode scene")
	}
}

func (s *Server) updateScene(w http.ResponseWriter, r *http.Request, id string) {
	var req sceneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	// The path names the scene; a body that disagrees is a mistake worth
	// reporting rather than silently resolving one way or the other.
	if req.SceneID != "" && req.SceneID != id {
		s.writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("Body names scene %q but the URL names %q", req.SceneID, id))
		return
	}

	scene, problem := sceneFromRequest(req, id)
	if problem != "" {
		s.writeJSONError(w, http.StatusBadRequest, problem)
		return
	}

	found, err := s.db.UpdateScene(r.Context(), scene)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update scene: %v", err))
		return
	}
	if !found {
		s.writeJSONError(w, http.StatusNotFound, "Scene not found")
		return
	}

	updated, err := s.db.GetScene(r.Context(), id)
	if err != nil || updated == nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Scene was updated but could not be read back")
		return
	}
	if err := json.NewEncoder(w).Encode(updated); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode scene")
	}
}

func (s *Server) deleteScene(w http.ResponseWriter, r *http.Request, id string) {
	found, err := s.db.DeleteScene(r.Context(), id)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete scene: %v", err))
		return
	}
	if !found {
		s.writeJSONError(w, http.StatusNotFound, "Scene not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sceneFromRequest validates a request and converts it to a storable Scene.
// It returns a human-readable problem rather than an error value, because
// every failure here is something the person editing the form can fix.
func sceneFromRequest(req sceneRequest, id string) (*db.Scene, string) {
	if !sceneIDPattern.MatchString(id) {
		return nil, "Scene ID must be lowercase letters, digits, dashes or underscores, starting with a letter or digit"
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, "Title is required"
	}

	// Latitude and longitude are a pair: half a position places nothing.
	if (req.Latitude == nil) != (req.Longitude == nil) {
		return nil, "Latitude and longitude must be given together, or both left empty to inherit the site's position"
	}
	if req.Latitude != nil && (*req.Latitude < -90 || *req.Latitude > 90) {
		return nil, "Latitude must be between -90 and 90"
	}
	if req.Longitude != nil && (*req.Longitude < -180 || *req.Longitude > 180) {
		return nil, "Longitude must be between -180 and 180"
	}

	startNs, problem := parseSceneTime(req.CapturedStart, "start")
	if problem != "" {
		return nil, problem
	}
	endNs, problem := parseSceneTime(req.CapturedEnd, "end")
	if problem != "" {
		return nil, problem
	}
	if startNs != nil && endNs != nil && *endNs < *startNs {
		return nil, "Capture end must not be before capture start"
	}

	var duration *float64
	if startNs != nil && endNs != nil {
		d := float64(*endNs-*startNs) / 1e9
		duration = &d
	}

	// Vantages are validated here rather than at render time, because a broken
	// viewpoint list makes a published scene unusable and the person who can
	// fix it is the one filling in this form.
	var vantagesJSON *string
	if len(req.Vantages) > 0 {
		if problem := scene.ValidateVantages(req.Vantages); problem != "" {
			return nil, problem
		}
		encoded, err := json.Marshal(req.Vantages)
		if err != nil {
			return nil, "Vantages could not be encoded"
		}
		str := string(encoded)
		vantagesJSON = &str
	}

	return &db.Scene{
		SceneID:           id,
		SiteID:            req.SiteID,
		Title:             strings.TrimSpace(req.Title),
		Description:       req.Description,
		Latitude:          req.Latitude,
		Longitude:         req.Longitude,
		CapturedStartNs:   startNs,
		CapturedEndNs:     endNs,
		DurationSecs:      duration,
		SourceCapture:     req.SourceCapture,
		SourceVRLOGSHA256: req.SourceVRLOGSHA256,
		FrameCount:        req.FrameCount,
		FrameStride:       req.FrameStride,
		AssetPath:         req.AssetPath,
		Published:         req.Published,
		VantagesJSON:      vantagesJSON,
	}, ""
}

// parseSceneTime accepts RFC 3339, with or without seconds, and the
// "YYYY-MM-DDTHH:MM" form an HTML datetime-local field produces.
func parseSceneTime(value *string, which string) (*int64, string) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, ""
	}
	raw := strings.TrimSpace(*value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			ns := t.UTC().UnixNano()
			return &ns, ""
		}
	}
	return nil, fmt.Sprintf("Capture %s is not a date and time we recognise: %q", which, raw)
}
