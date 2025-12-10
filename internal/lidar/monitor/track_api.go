package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// TrackAPI provides HTTP handlers for track-related endpoints.
// It supports both in-memory tracker queries and database persistence.
type TrackAPI struct {
	db       *sql.DB
	sensorID string
	tracker  *lidar.Tracker // Optional: in-memory tracker for real-time queries
}

// NewTrackAPI creates a new TrackAPI instance.
func NewTrackAPI(db *sql.DB, sensorID string) *TrackAPI {
	return &TrackAPI{
		db:       db,
		sensorID: sensorID,
	}
}

// SetTracker sets the in-memory tracker for real-time queries.
func (api *TrackAPI) SetTracker(tracker *lidar.Tracker) {
	api.tracker = tracker
}

// RegisterRoutes registers track API routes on the provided mux.
func (api *TrackAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/lidar/tracks", api.handleListTracks)
	mux.HandleFunc("/api/lidar/tracks/history", api.handleListTracks) // Support both legacy '/history' and canonical '/tracks' endpoints for backward compatibility; '/history' may be deprecated in a future release.
	mux.HandleFunc("/api/lidar/tracks/active", api.handleActiveTracks)
	mux.HandleFunc("/api/lidar/tracks/", api.handleTrackByID)
	mux.HandleFunc("/api/lidar/tracks/summary", api.handleTrackSummary)
	mux.HandleFunc("/api/lidar/clusters", api.handleListClusters)
}

// TrackResponse represents a track in JSON API responses.
type TrackResponse struct {
	TrackID             string               `json:"track_id"`
	SensorID            string               `json:"sensor_id"`
	State               string               `json:"state"`
	Position            Position             `json:"position"`
	Velocity            Velocity             `json:"velocity"`
	SpeedMps            float32              `json:"speed_mps"`
	HeadingRad          float32              `json:"heading_rad"`
	ObjectClass         string               `json:"object_class,omitempty"`
	ObjectConfidence    float32              `json:"object_confidence,omitempty"`
	ClassificationModel string               `json:"classification_model,omitempty"`
	ObservationCount    int                  `json:"observation_count"`
	AgeSeconds          float64              `json:"age_seconds"`
	AvgSpeedMps         float32              `json:"avg_speed_mps"`
	PeakSpeedMps        float32              `json:"peak_speed_mps"`
	BoundingBox         BBox                 `json:"bounding_box"`
	FirstSeen           string               `json:"first_seen"`
	LastSeen            string               `json:"last_seen"`
	History             []TrackPointResponse `json:"history,omitempty"`
}

// TrackPointResponse represents a point in a track's history.
type TrackPointResponse struct {
	X         float32 `json:"x"`
	Y         float32 `json:"y"`
	Timestamp string  `json:"timestamp"`
}

// Position represents a 3D position in world coordinates.
type Position struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

// Velocity represents 2D velocity in world coordinates.
type Velocity struct {
	VX float32 `json:"vx"`
	VY float32 `json:"vy"`
}

// BBox represents average bounding box dimensions.
type BBox struct {
	LengthAvg float32 `json:"length_avg"`
	WidthAvg  float32 `json:"width_avg"`
	HeightAvg float32 `json:"height_avg"`
}

// ClusterResponse represents a cluster in JSON API responses.
type ClusterResponse struct {
	ClusterID   int64    `json:"cluster_id"`
	SensorID    string   `json:"sensor_id"`
	Timestamp   string   `json:"timestamp"`
	Centroid    Position `json:"centroid"`
	BoundingBox struct {
		Length float32 `json:"length"`
		Width  float32 `json:"width"`
		Height float32 `json:"height"`
	} `json:"bounding_box"`
	PointsCount   int     `json:"points_count"`
	HeightP95     float32 `json:"height_p95"`
	IntensityMean float32 `json:"intensity_mean"`
}

// TracksListResponse is the JSON response for listing tracks.
type TracksListResponse struct {
	Tracks    []TrackResponse `json:"tracks"`
	Count     int             `json:"count"`
	Timestamp string          `json:"timestamp"`
}

// ClustersListResponse is the JSON response for listing clusters.
type ClustersListResponse struct {
	Clusters  []ClusterResponse `json:"clusters"`
	Count     int               `json:"count"`
	Timestamp string            `json:"timestamp"`
}

// TrackSummaryResponse is the JSON response for track summary statistics.
type TrackSummaryResponse struct {
	SensorID  string                  `json:"sensor_id"`
	StartTime string                  `json:"start_time,omitempty"`
	EndTime   string                  `json:"end_time,omitempty"`
	ByClass   map[string]ClassSummary `json:"by_class"`
	ByState   map[string]int          `json:"by_state"`
	Overall   OverallSummary          `json:"overall"`
	Timestamp string                  `json:"timestamp"`
}

// ClassSummary contains summary statistics for a single object class.
type ClassSummary struct {
	Count        int     `json:"count"`
	AvgSpeedMps  float32 `json:"avg_speed_mps"`
	PeakSpeedMps float32 `json:"peak_speed_mps"`
	AvgDuration  float64 `json:"avg_duration_seconds"`
}

// OverallSummary contains overall summary statistics across all tracks.
type OverallSummary struct {
	TotalTracks    int     `json:"total_tracks"`
	ConfirmedCount int     `json:"confirmed_count"`
	TentativeCount int     `json:"tentative_count"`
	DeletedCount   int     `json:"deleted_count"`
	AvgSpeedMps    float32 `json:"avg_speed_mps"`
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

	var tracks []*lidar.TrackedObject
	var err error

	if startParam != "" || endParam != "" {
		tracks, err = lidar.GetTracksInRange(api.db, sensorID, state, startNanos, endNanos, limit)
	} else {
		tracks, err = lidar.GetActiveTracks(api.db, sensorID, state)
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

	var tracks []*lidar.TrackedObject

	// Prefer in-memory tracker for real-time data
	if api.tracker != nil {
		switch state {
		case "confirmed":
			tracks = api.tracker.GetConfirmedTracks()
		case "tentative":
			allActive := api.tracker.GetActiveTracks()
			for _, t := range allActive {
				if t.State == lidar.TrackTentative {
					tracks = append(tracks, t)
				}
			}
		default:
			tracks = api.tracker.GetActiveTracks()
		}
	} else if api.db != nil {
		// Fall back to database
		dbTracks, err := lidar.GetActiveTracks(api.db, sensorID, state)
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
	var track *lidar.TrackedObject

	// Try in-memory tracker first
	if api.tracker != nil {
		track = api.tracker.GetTrack(trackID)
	}

	// Fall back to database if not found in memory
	if track == nil && api.db != nil {
		// For now, we search active tracks - could add a direct GetTrack query
		tracks, err := lidar.GetActiveTracks(api.db, api.sensorID, "")
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
	var track *lidar.TrackedObject
	if api.tracker != nil {
		track = api.tracker.GetTrack(trackID)
	}

	if track == nil {
		// Try to find in database
		tracks, err := lidar.GetActiveTracks(api.db, api.sensorID, "")
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
	if err := lidar.UpdateTrack(api.db, track, "site"); err != nil {
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

	observations, err := lidar.GetTrackObservations(api.db, trackID, limit)
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
		response.Observations = append(response.Observations, ObsResponse{
			Timestamp: time.Unix(0, obs.TSUnixNanos).UTC().Format(time.RFC3339Nano),
			Position: Position{
				X: obs.X,
				Y: obs.Y,
				Z: obs.Z,
			},
			Velocity: Velocity{
				VX: obs.VelocityX,
				VY: obs.VelocityY,
			},
			SpeedMps:   obs.SpeedMps,
			HeadingRad: obs.HeadingRad,
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

	var tracks []*lidar.TrackedObject

	// Get tracks from in-memory tracker or database
	if api.tracker != nil {
		tracks = api.tracker.GetActiveTracks()
		// Also include deleted tracks for summary
		for _, t := range api.tracker.Tracks {
			if t.State == lidar.TrackDeleted {
				tracks = append(tracks, t)
			}
		}
	} else if api.db != nil {
		// Get all tracks including deleted for summary
		allTracks, err := lidar.GetActiveTracks(api.db, sensorID, "")
		if err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get tracks: %v", err))
			return
		}
		tracks = allTracks
	} else {
		api.writeJSONError(w, http.StatusServiceUnavailable, "no tracker or database configured")
		return
	}

	// Compute summary statistics
	byClass := make(map[string]*classSummaryAccum)
	byState := make(map[string]int)
	var totalSpeed float32
	var speedCount int

	for _, track := range tracks {
		// By state
		byState[string(track.State)]++

		// By class
		class := track.ObjectClass
		if class == "" {
			class = "unclassified"
		}
		if _, ok := byClass[class]; !ok {
			byClass[class] = &classSummaryAccum{}
		}
		accum := byClass[class]
		accum.count++
		accum.totalSpeed += track.AvgSpeedMps
		if track.PeakSpeedMps > accum.peakSpeed {
			accum.peakSpeed = track.PeakSpeedMps
		}
		if track.LastUnixNanos > 0 && track.FirstUnixNanos > 0 {
			duration := float64(track.LastUnixNanos-track.FirstUnixNanos) / 1e9
			accum.totalDuration += duration
		}

		// Overall
		totalSpeed += track.AvgSpeedMps
		speedCount++
	}

	// Build response
	response := TrackSummaryResponse{
		SensorID:  sensorID,
		ByClass:   make(map[string]ClassSummary),
		ByState:   byState,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	for class, accum := range byClass {
		var avgSpeed float32
		var avgDuration float64
		if accum.count > 0 {
			avgSpeed = accum.totalSpeed / float32(accum.count)
			avgDuration = accum.totalDuration / float64(accum.count)
		}
		response.ByClass[class] = ClassSummary{
			Count:        accum.count,
			AvgSpeedMps:  avgSpeed,
			PeakSpeedMps: accum.peakSpeed,
			AvgDuration:  avgDuration,
		}
	}

	var overallAvgSpeed float32
	if speedCount > 0 {
		overallAvgSpeed = totalSpeed / float32(speedCount)
	}

	response.Overall = OverallSummary{
		TotalTracks:    len(tracks),
		ConfirmedCount: byState["confirmed"],
		TentativeCount: byState["tentative"],
		DeletedCount:   byState["deleted"],
		AvgSpeedMps:    overallAvgSpeed,
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

	clusters, err := lidar.GetRecentClusters(api.db, sensorID, startNanos, endNanos, limit)
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

// Helper methods

func (api *TrackAPI) writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (api *TrackAPI) trackToResponse(track *lidar.TrackedObject) TrackResponse {
	first := track.FirstUnixNanos
	last := track.LastUnixNanos
	if last == 0 {
		last = track.FirstUnixNanos
	}
	if last < first {
		last = first
	}

	var spanSeconds float64
	if first > 0 && last > 0 {
		spanSeconds = float64(last-first) / 1e9
	}

	speed := float32(math.Sqrt(float64(track.VX*track.VX + track.VY*track.VY)))
	heading := float32(math.Atan2(float64(track.VY), float64(track.VX)))

	history := make([]TrackPointResponse, 0, len(track.History))
	for _, p := range track.History {
		history = append(history, TrackPointResponse{
			X:         p.X,
			Y:         p.Y,
			Timestamp: time.Unix(0, p.Timestamp).UTC().Format(time.RFC3339Nano),
		})
	}

	return TrackResponse{
		TrackID:  track.TrackID,
		SensorID: track.SensorID,
		State:    string(track.State),
		Position: Position{
			X: track.X,
			Y: track.Y,
			// Z is 0 because TrackedObject uses a 2D Kalman filter tracking (x, y, vx, vy).
			// Height information is captured in bounding_box and height_p95 from cluster features.
			Z: 0,
		},
		Velocity: Velocity{
			VX: track.VX,
			VY: track.VY,
		},
		SpeedMps:            speed,
		HeadingRad:          heading,
		ObjectClass:         track.ObjectClass,
		ObjectConfidence:    track.ObjectConfidence,
		ClassificationModel: track.ClassificationModel,
		ObservationCount:    track.ObservationCount,
		AgeSeconds:          spanSeconds,
		AvgSpeedMps:         track.AvgSpeedMps,
		PeakSpeedMps:        track.PeakSpeedMps,
		BoundingBox: BBox{
			LengthAvg: track.BoundingBoxLengthAvg,
			WidthAvg:  track.BoundingBoxWidthAvg,
			HeightAvg: track.BoundingBoxHeightAvg,
		},
		FirstSeen: time.Unix(0, first).UTC().Format(time.RFC3339Nano),
		LastSeen:  time.Unix(0, last).UTC().Format(time.RFC3339Nano),
		History:   history,
	}
}

// classSummaryAccum is an accumulator for computing class summary statistics.
type classSummaryAccum struct {
	count         int
	totalSpeed    float32
	peakSpeed     float32
	totalDuration float64
}
