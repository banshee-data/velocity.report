package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// TrackAPI provides HTTP handlers for track-related endpoints.
// It supports both in-memory tracker queries and database persistence.
type TrackAPI struct {
	db       *sqlite.SQLDB
	sensorID string
	tracker  *l5tracks.Tracker // Optional: in-memory tracker for real-time queries
}

// NewTrackAPI creates a new TrackAPI instance.
func NewTrackAPI(db *sqlite.SQLDB, sensorID string) *TrackAPI {
	return &TrackAPI{
		db:       db,
		sensorID: sensorID,
	}
}

// SetTracker sets the in-memory tracker for real-time queries.
func (api *TrackAPI) SetTracker(tracker *l5tracks.Tracker) {
	api.tracker = tracker
}

// handleClearTracks deletes all tracks, observations, and clusters for a sensor.
// Method: POST (or GET for convenience). Query param: sensor_id (required).
func (api *TrackAPI) handleClearTracks(w http.ResponseWriter, r *http.Request) {
	if api.db == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use POST")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = api.sensorID
	}
	if sensorID == "" {
		api.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	if err := sqlite.ClearTracks(api.db, sensorID); err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to clear tracks: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"sensor_id": sensorID,
	})
}

// handleClearRuns deletes all analysis runs and their associated run tracks for a sensor.
// Method: POST (or GET for convenience). Query param: sensor_id (required).
func (api *TrackAPI) handleClearRuns(w http.ResponseWriter, r *http.Request) {
	if api.db == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use POST")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = api.sensorID
	}
	if sensorID == "" {
		api.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	if err := sqlite.ClearRuns(api.db, sensorID); err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to clear runs: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"sensor_id": sensorID,
	})
}

// handleListTracks handles GET /api/lidar/tracks
// Query params:
//   - sensor_id (optional, defaults to configured sensor)
//   - state (optional): filter by track state (tentative, confirmed, deleted, all)
//   - start (optional): start timestamp (unix seconds)
//   - end (optional): end timestamp (unix seconds)
//   - limit (optional): max results (default 100)
func (api *TrackAPI) handleListTracks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = api.sensorID
	}

	state := r.URL.Query().Get("state")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Optional time window (nanoseconds since epoch)
	startParam := r.URL.Query().Get("start_time")
	endParam := r.URL.Query().Get("end_time")
	startNanos := int64(0)
	endNanos := time.Now().UnixNano()
	if startParam != "" {
		parsed, err := strconv.ParseInt(startParam, 10, 64)
		if err != nil {
			api.writeJSONError(w, http.StatusBadRequest, "invalid start_time")
			return
		}
		startNanos = parsed
	}
	if endParam != "" {
		parsed, err := strconv.ParseInt(endParam, 10, 64)
		if err != nil {
			api.writeJSONError(w, http.StatusBadRequest, "invalid end_time")
			return
		}
		endNanos = parsed
	}
	if endNanos > 0 && startNanos > endNanos {
		api.writeJSONError(w, http.StatusBadRequest, "start_time must be <= end_time")
		return
	}

	if api.db == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	var tracks []*l5tracks.TrackedObject
	var err error

	if startParam != "" || endParam != "" {
		tracks, err = sqlite.GetTracksInRange(api.db, sensorID, state, startNanos, endNanos, limit)
	} else {
		tracks, err = sqlite.GetActiveTracks(api.db, sensorID, state)
		if err == nil && len(tracks) > limit {
			tracks = tracks[:limit]
		}
	}
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get tracks: %v", err))
		return
	}

	response := TracksListResponse{
		Tracks:    make([]TrackResponse, 0, len(tracks)),
		Count:     len(tracks),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	for _, track := range tracks {
		response.Tracks = append(response.Tracks, api.trackToResponse(track))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleListObservations handles GET /api/lidar/observations
// Returns raw track observations for a sensor within a time window (for overlay/debug).
// Query params:
//   - sensor_id (optional, defaults to configured sensor)
//   - track_id (optional): limit to a single track
//   - start_time, end_time (unix nanoseconds; defaults to last 5 minutes)
//   - limit (optional, default 1000, max 5000)
func (api *TrackAPI) handleListObservations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if api.db == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = api.sensorID
	}

	trackID := r.URL.Query().Get("track_id")

	limit := 1000
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			if parsed > 0 && parsed <= 5000 {
				limit = parsed
			}
		}
	}

	endNanos := time.Now().UnixNano()
	startNanos := endNanos - int64(5*time.Minute)

	if s := r.URL.Query().Get("start_time"); s != "" {
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			api.writeJSONError(w, http.StatusBadRequest, "invalid start_time")
			return
		}
		startNanos = parsed
	}

	if e := r.URL.Query().Get("end_time"); e != "" {
		parsed, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			api.writeJSONError(w, http.StatusBadRequest, "invalid end_time")
			return
		}
		endNanos = parsed
	}

	if endNanos > 0 && startNanos > endNanos {
		api.writeJSONError(w, http.StatusBadRequest, "start_time must be <= end_time")
		return
	}

	observations, err := sqlite.GetTrackObservationsInRange(api.db, sensorID, startNanos, endNanos, limit, trackID)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get observations: %v", err))
		return
	}

	response := ObservationsListResponse{
		Observations: make([]ObservationResponse, 0, len(observations)),
		Count:        len(observations),
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	for _, obs := range observations {
		posX, posY := toDisplayFrame(obs.X, obs.Y)
		velX, velY := toDisplayFrame(obs.VelocityX, obs.VelocityY)
		response.Observations = append(response.Observations, ObservationResponse{
			TrackID:   obs.TrackID,
			Timestamp: time.Unix(0, obs.TSUnixNanos).UTC().Format(time.RFC3339Nano),
			Position: Position{
				X: posX,
				Y: posY,
				Z: obs.Z,
			},
			Velocity: Velocity{
				VX: velX,
				VY: velY,
			},
			SpeedMps:   float32(math.Sqrt(float64(velX*velX + velY*velY))),
			HeadingRad: headingFromVelocity(velX, velY),
			BoundingBox: struct {
				Length float32 `json:"length"`
				Width  float32 `json:"width"`
				Height float32 `json:"height"`
			}{
				Length: obs.BoundingBoxLength,
				Width:  obs.BoundingBoxWidth,
				Height: obs.BoundingBoxHeight,
			},
			HeightP95:     obs.HeightP95,
			IntensityMean: obs.IntensityMean,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleActiveTracks handles GET /api/lidar/tracks/active
// Returns currently active tracks from in-memory tracker (if available) or database.
// Query params:
//   - sensor_id (optional)
//   - state (optional): confirmed, tentative, all (default: all non-deleted)
func (api *TrackAPI) handleActiveTracks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = api.sensorID
	}

	state := r.URL.Query().Get("state")

	var tracks []*l5tracks.TrackedObject

	// Prefer in-memory tracker for real-time data
	if api.tracker != nil {
		switch state {
		case "confirmed":
			tracks = api.tracker.GetConfirmedTracks()
		case "tentative":
			allActive := api.tracker.GetActiveTracks()
			for _, t := range allActive {
				if t.State == l5tracks.TrackTentative {
					tracks = append(tracks, t)
				}
			}
		default:
			tracks = api.tracker.GetActiveTracks()
		}
	} else if api.db != nil {
		// Fall back to database
		dbTracks, err := sqlite.GetActiveTracks(api.db, sensorID, state)
		if err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get tracks: %v", err))
			return
		}
		tracks = dbTracks
	} else {
		api.writeJSONError(w, http.StatusServiceUnavailable, "no tracker or database configured")
		return
	}

	response := TracksListResponse{
		Tracks:    make([]TrackResponse, 0, len(tracks)),
		Count:     len(tracks),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	for _, track := range tracks {
		response.Tracks = append(response.Tracks, api.trackToResponse(track))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTrackByID handles:
//   - GET /api/lidar/tracks/{track_id} - get track details
//   - PUT /api/lidar/tracks/{track_id} - update track metadata
//   - GET /api/lidar/tracks/{track_id}/observations - get track observations
func (api *TrackAPI) handleTrackByID(w http.ResponseWriter, r *http.Request) {
	// Parse track ID from path
	path := r.URL.Path
	// Path format: /api/lidar/tracks/{track_id} or /api/lidar/tracks/{track_id}/observations
	prefix := "/api/lidar/tracks/"
	if len(path) <= len(prefix) {
		api.writeJSONError(w, http.StatusBadRequest, "missing track_id in path")
		return
	}

	remainder := path[len(prefix):]
	trackID := remainder
	subPath := ""

	// Check for sub-paths like /observations
	if idx := strings.Index(remainder, "/"); idx != -1 {
		trackID = remainder[:idx]
		subPath = remainder[idx+1:]
	}

	if trackID == "" {
		api.writeJSONError(w, http.StatusBadRequest, "missing track_id")
		return
	}

	switch {
	case subPath == "observations" && r.Method == http.MethodGet:
		api.handleTrackObservations(w, r, trackID)
	case subPath == "" && r.Method == http.MethodGet:
		api.handleGetTrack(w, r, trackID)
	case subPath == "" && r.Method == http.MethodPut:
		api.handleUpdateTrack(w, r, trackID)
	default:
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleGetTrack returns details for a specific track.
func (api *TrackAPI) handleGetTrack(w http.ResponseWriter, r *http.Request, trackID string) {
	var track *l5tracks.TrackedObject

	// Try in-memory tracker first
	if api.tracker != nil {
		track = api.tracker.GetTrack(trackID)
	}

	// Fall back to database if not found in memory
	if track == nil && api.db != nil {
		// For now, we search active tracks - could add a direct GetTrack query
		tracks, err := sqlite.GetActiveTracks(api.db, api.sensorID, "")
		if err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to query tracks: %v", err))
			return
		}
		for _, t := range tracks {
			if t.TrackID == trackID {
				track = t
				break
			}
		}
	}

	if track == nil {
		api.writeJSONError(w, http.StatusNotFound, "track not found")
		return
	}

	response := api.trackToResponse(track)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleUpdateTrack updates metadata for a specific track.
// Supports updating: object_class, object_confidence, classification_model
func (api *TrackAPI) handleUpdateTrack(w http.ResponseWriter, r *http.Request, trackID string) {
	if api.db == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	var req struct {
		ObjectClass         *string  `json:"object_class"`
		ObjectConfidence    *float32 `json:"object_confidence"`
		ClassificationModel *string  `json:"classification_model"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Find the track first (in-memory or database)
	var track *l5tracks.TrackedObject
	if api.tracker != nil {
		track = api.tracker.GetTrack(trackID)
	}

	if track == nil {
		// Try to find in database
		tracks, err := sqlite.GetActiveTracks(api.db, api.sensorID, "")
		if err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to query tracks: %v", err))
			return
		}
		for _, t := range tracks {
			if t.TrackID == trackID {
				track = t
				break
			}
		}
	}

	if track == nil {
		api.writeJSONError(w, http.StatusNotFound, "track not found")
		return
	}

	// Apply updates
	if req.ObjectClass != nil {
		track.ObjectClass = *req.ObjectClass
	}
	if req.ObjectConfidence != nil {
		track.ObjectConfidence = *req.ObjectConfidence
	}
	if req.ClassificationModel != nil {
		track.ClassificationModel = *req.ClassificationModel
	}

	// Persist to database
	if err := sqlite.UpdateTrack(api.db, track); err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update track: %v", err))
		return
	}

	response := api.trackToResponse(track)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTrackObservations returns the observation history for a track.
func (api *TrackAPI) handleTrackObservations(w http.ResponseWriter, r *http.Request, trackID string) {
	if api.db == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	observations, err := sqlite.GetTrackObservations(api.db, trackID, limit)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get observations: %v", err))
		return
	}

	type ObsResponse struct {
		Timestamp   string   `json:"timestamp"`
		Position    Position `json:"position"`
		Velocity    Velocity `json:"velocity"`
		SpeedMps    float32  `json:"speed_mps"`
		HeadingRad  float32  `json:"heading_rad"`
		BoundingBox struct {
			Length float32 `json:"length"`
			Width  float32 `json:"width"`
			Height float32 `json:"height"`
		} `json:"bounding_box"`
		HeightP95     float32 `json:"height_p95"`
		IntensityMean float32 `json:"intensity_mean"`
	}

	response := struct {
		TrackID      string        `json:"track_id"`
		Observations []ObsResponse `json:"observations"`
		Count        int           `json:"count"`
		Timestamp    string        `json:"timestamp"`
	}{
		TrackID:      trackID,
		Observations: make([]ObsResponse, 0, len(observations)),
		Count:        len(observations),
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	for _, obs := range observations {
		posX, posY := toDisplayFrame(obs.X, obs.Y)
		velX, velY := toDisplayFrame(obs.VelocityX, obs.VelocityY)
		speed := float32(math.Sqrt(float64(velX*velX + velY*velY)))
		heading := headingFromVelocity(velX, velY)

		response.Observations = append(response.Observations, ObsResponse{
			Timestamp: time.Unix(0, obs.TSUnixNanos).UTC().Format(time.RFC3339Nano),
			Position: Position{
				X: posX,
				Y: posY,
				Z: obs.Z,
			},
			Velocity: Velocity{
				VX: velX,
				VY: velY,
			},
			SpeedMps:   speed,
			HeadingRad: heading,
			BoundingBox: struct {
				Length float32 `json:"length"`
				Width  float32 `json:"width"`
				Height float32 `json:"height"`
			}{
				Length: obs.BoundingBoxLength,
				Width:  obs.BoundingBoxWidth,
				Height: obs.BoundingBoxHeight,
			},
			HeightP95:     obs.HeightP95,
			IntensityMean: obs.IntensityMean,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTrackSummary handles GET /api/lidar/tracks/summary
// Returns aggregated statistics by object class and state.
// Query params:
//   - sensor_id (optional)
//   - start (optional): start timestamp (unix seconds)
//   - end (optional): end timestamp (unix seconds)
//   - group_by (optional): "object_class" (default)
func (api *TrackAPI) handleTrackSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = api.sensorID
	}

	var tracks []*l5tracks.TrackedObject

	// Get tracks from in-memory tracker or database
	if api.tracker != nil {
		tracks = api.tracker.GetActiveTracks()
		// Also include deleted tracks for summary
		for _, t := range api.tracker.Tracks {
			if t.State == l5tracks.TrackDeleted {
				tracks = append(tracks, t)
			}
		}
	} else if api.db != nil {
		// Get all tracks including deleted for summary
		allTracks, err := sqlite.GetActiveTracks(api.db, sensorID, "")
		if err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get tracks: %v", err))
			return
		}
		tracks = allTracks
	} else {
		api.writeJSONError(w, http.StatusServiceUnavailable, "no tracker or database configured")
		return
	}

	summary := l8analytics.ComputeTrackSummary(tracks)

	response := TrackSummaryResponse{
		SensorID:  sensorID,
		ByClass:   summary.ByClass,
		ByState:   summary.ByState,
		Overall:   summary.Overall,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleListClusters handles GET /api/lidar/clusters
// Query params:
//   - sensor_id (optional)
//   - start (optional): start timestamp (unix seconds)
//   - end (optional): end timestamp (unix seconds)
//   - limit (optional): max results (default 100)
func (api *TrackAPI) handleListClusters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if api.db == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = api.sensorID
	}

	// Parse time range
	var startNanos, endNanos int64
	if s := r.URL.Query().Get("start"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			startNanos = parsed * 1e9 // Convert seconds to nanoseconds
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if parsed, err := strconv.ParseInt(e, 10, 64); err == nil {
			endNanos = parsed * 1e9
		}
	}

	// Default to last hour if no range specified
	if endNanos == 0 {
		endNanos = time.Now().UnixNano()
	}
	if startNanos == 0 {
		startNanos = endNanos - int64(time.Hour)
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	clusters, err := sqlite.GetRecentClusters(api.db, sensorID, startNanos, endNanos, limit)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get clusters: %v", err))
		return
	}

	response := ClustersListResponse{
		Clusters:  make([]ClusterResponse, 0, len(clusters)),
		Count:     len(clusters),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	for _, cluster := range clusters {
		cr := ClusterResponse{
			ClusterID: cluster.ClusterID,
			SensorID:  cluster.SensorID,
			Timestamp: time.Unix(0, cluster.TSUnixNanos).UTC().Format(time.RFC3339Nano),
			Centroid: Position{
				X: cluster.CentroidX,
				Y: cluster.CentroidY,
				Z: cluster.CentroidZ,
			},
			PointsCount:   cluster.PointsCount,
			HeightP95:     cluster.HeightP95,
			IntensityMean: cluster.IntensityMean,
		}
		cr.BoundingBox.Length = cluster.BoundingBoxLength
		cr.BoundingBox.Width = cluster.BoundingBoxWidth
		cr.BoundingBox.Height = cluster.BoundingBoxHeight
		response.Clusters = append(response.Clusters, cr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTrackingMetrics returns aggregate velocity-trail alignment metrics
// across all active tracks. Used by the sweep tool to evaluate tracking
// parameter configurations.
//
// GET /api/lidar/tracks/metrics
// Optional query parameter: include_per_track=true to include per-track breakdown
func (api *TrackAPI) handleTrackingMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	if api.tracker == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "in-memory tracker not available")
		return
	}

	metrics := api.tracker.GetTrackingMetrics()

	// Omit per-track breakdown unless explicitly requested
	includePerTrack := r.URL.Query().Get("include_per_track") == "true"
	if !includePerTrack {
		metrics.PerTrack = nil
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
