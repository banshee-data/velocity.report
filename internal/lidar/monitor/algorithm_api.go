package monitor

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// AlgorithmAPI provides HTTP handlers for tracking algorithm selection and configuration.
type AlgorithmAPI struct {
	db        *sql.DB
	sensorID  string
	pipeline  *lidar.DualExtractionPipeline
	vcTracker *lidar.VelocityCoherentTracker
	mu        sync.RWMutex
}

// NewAlgorithmAPI creates a new AlgorithmAPI instance.
func NewAlgorithmAPI(db *sql.DB, sensorID string) *AlgorithmAPI {
	return &AlgorithmAPI{
		db:       db,
		sensorID: sensorID,
	}
}

// SetPipeline sets the dual extraction pipeline for algorithm switching.
func (api *AlgorithmAPI) SetPipeline(pipeline *lidar.DualExtractionPipeline) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.pipeline = pipeline
}

// SetVCTracker sets the velocity-coherent tracker for direct access.
func (api *AlgorithmAPI) SetVCTracker(tracker *lidar.VelocityCoherentTracker) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.vcTracker = tracker
}

// RegisterRoutes registers algorithm API routes on the provided mux.
func (api *AlgorithmAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/lidar/tracking/algorithm", api.handleAlgorithmConfig)
	mux.HandleFunc("/api/lidar/tracking/stats", api.handleTrackingStats)
	mux.HandleFunc("/api/lidar/tracking/vc/tracks", api.handleVCTracks)
	mux.HandleFunc("/api/lidar/tracking/vc/config", api.handleVCConfig)
}

// AlgorithmConfigRequest represents the request body for algorithm configuration.
type AlgorithmConfigRequest struct {
	Active           string        `json:"active"` // "background_subtraction", "velocity_coherent", "dual"
	VelocityCoherent *VCConfig     `json:"velocity_coherent,omitempty"`
	DBSCAN           *DBSCANConfig `json:"dbscan,omitempty"`
}

// VCConfig represents velocity-coherent configuration options.
type VCConfig struct {
	MinPts              int     `json:"min_pts,omitempty"`
	PositionEps         float64 `json:"position_eps,omitempty"`
	VelocityEps         float64 `json:"velocity_eps,omitempty"`
	MaxPredictionFrames int     `json:"max_prediction_frames,omitempty"`
	MaxMisses           int     `json:"max_misses,omitempty"`
	HitsToConfirm       int     `json:"hits_to_confirm,omitempty"`
}

// DBSCANConfig represents DBSCAN configuration options.
type DBSCANConfig struct {
	Eps    float64 `json:"eps,omitempty"`
	MinPts int     `json:"min_pts,omitempty"`
}

// AlgorithmConfigResponse represents the response for algorithm configuration.
type AlgorithmConfigResponse struct {
	Active                       string  `json:"active"`
	BackgroundSubtractionEnabled bool    `json:"background_subtraction_enabled"`
	VelocityCoherentEnabled      bool    `json:"velocity_coherent_enabled"`
	DBSCANEps                    float64 `json:"dbscan_eps"`
	DBSCANMinPts                 int     `json:"dbscan_min_pts"`
	VCMinPts                     int     `json:"vc_min_pts"`
	VCPositionEps                float64 `json:"vc_position_eps"`
	VCVelocityEps                float64 `json:"vc_velocity_eps"`
	VCMaxMisses                  int     `json:"vc_max_misses"`
	VCHitsToConfirm              int     `json:"vc_hits_to_confirm"`
	VCMaxPredictionFrames        int     `json:"vc_max_prediction_frames"`
}

// handleAlgorithmConfig handles GET/POST for /api/lidar/tracking/algorithm
//
// GET: Returns current algorithm configuration
// POST: Switches algorithm and/or updates parameters (hot-reload)
//
// Example GET response:
//
//	{
//	  "active": "background_subtraction",
//	  "background_subtraction_enabled": true,
//	  "velocity_coherent_enabled": false,
//	  "dbscan_eps": 0.6,
//	  "dbscan_min_pts": 12,
//	  "vc_min_pts": 3,
//	  "vc_position_eps": 0.6,
//	  "vc_velocity_eps": 1.0,
//	  "vc_max_misses": 3,
//	  "vc_hits_to_confirm": 3,
//	  "vc_max_prediction_frames": 30
//	}
//
// Example POST request:
//
//	{
//	  "active": "velocity_coherent",
//	  "velocity_coherent": {
//	    "min_pts": 5,
//	    "velocity_eps": 1.5
//	  }
//	}
func (api *AlgorithmAPI) handleAlgorithmConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	pipeline := api.pipeline
	api.mu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		api.handleGetAlgorithmConfig(w, pipeline)
	case http.MethodPost:
		api.handleSetAlgorithmConfig(w, r, pipeline)
	default:
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET or POST")
	}
}

// handleGetAlgorithmConfig returns the current algorithm configuration.
func (api *AlgorithmAPI) handleGetAlgorithmConfig(w http.ResponseWriter, pipeline *lidar.DualExtractionPipeline) {
	var response AlgorithmConfigResponse

	if pipeline != nil {
		config := pipeline.GetConfig()
		response = AlgorithmConfigResponse{
			Active:                       string(config.ActiveAlgorithm),
			BackgroundSubtractionEnabled: config.BackgroundSubtractionEnabled,
			VelocityCoherentEnabled:      config.VelocityCoherentEnabled,
			DBSCANEps:                    config.DBSCANParams.Eps,
			DBSCANMinPts:                 config.DBSCANParams.MinPts,
			VCMinPts:                     config.VCConfig.Clustering.MinPts,
			VCPositionEps:                config.VCConfig.Clustering.PositionEps,
			VCVelocityEps:                config.VCConfig.Clustering.VelocityEps,
			VCMaxMisses:                  config.VCConfig.MaxMisses,
			VCHitsToConfirm:              config.VCConfig.HitsToConfirm,
			VCMaxPredictionFrames:        config.VCConfig.PostTail.MaxPredictionFrames,
		}
	} else {
		// Return defaults if pipeline not configured
		defaultConfig := lidar.DefaultDualPipelineConfig()
		response = AlgorithmConfigResponse{
			Active:                       string(defaultConfig.ActiveAlgorithm),
			BackgroundSubtractionEnabled: defaultConfig.BackgroundSubtractionEnabled,
			VelocityCoherentEnabled:      defaultConfig.VelocityCoherentEnabled,
			DBSCANEps:                    defaultConfig.DBSCANParams.Eps,
			DBSCANMinPts:                 defaultConfig.DBSCANParams.MinPts,
			VCMinPts:                     defaultConfig.VCConfig.Clustering.MinPts,
			VCPositionEps:                defaultConfig.VCConfig.Clustering.PositionEps,
			VCVelocityEps:                defaultConfig.VCConfig.Clustering.VelocityEps,
			VCMaxMisses:                  defaultConfig.VCConfig.MaxMisses,
			VCHitsToConfirm:              defaultConfig.VCConfig.HitsToConfirm,
			VCMaxPredictionFrames:        defaultConfig.VCConfig.PostTail.MaxPredictionFrames,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSetAlgorithmConfig updates the algorithm configuration (hot-reload).
func (api *AlgorithmAPI) handleSetAlgorithmConfig(w http.ResponseWriter, r *http.Request, pipeline *lidar.DualExtractionPipeline) {
	if pipeline == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "pipeline not configured")
		return
	}

	var req AlgorithmConfigRequest

	// Support both JSON and form-encoded data
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/x-www-form-urlencoded" || r.Header.Get("Content-Type") == "" {
		// Parse form data
		if err := r.ParseForm(); err != nil {
			api.writeJSONError(w, http.StatusBadRequest, "invalid form data: "+err.Error())
			return
		}
		req.Active = r.FormValue("active")
	} else {
		// Parse JSON data
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}

	// Validate algorithm selection
	alg := lidar.TrackingAlgorithm(req.Active)
	if req.Active != "" && !alg.IsValid() {
		api.writeJSONError(w, http.StatusBadRequest, "invalid algorithm; use 'background_subtraction', 'velocity_coherent', or 'dual'")
		return
	}

	// Switch algorithm if specified
	if req.Active != "" {
		pipeline.SetActiveAlgorithm(alg)

		// Log the change
		if api.db != nil {
			configJSON, _ := json.Marshal(req)
			lidar.LogAlgorithmConfig(api.db, alg, string(configJSON), "api")
		}
	}

	// Update VC configuration if specified
	if req.VelocityCoherent != nil {
		vcConfig := pipeline.GetConfig().VCConfig

		if req.VelocityCoherent.MinPts > 0 {
			vcConfig.Clustering.MinPts = req.VelocityCoherent.MinPts
		}
		if req.VelocityCoherent.PositionEps > 0 {
			vcConfig.Clustering.PositionEps = req.VelocityCoherent.PositionEps
		}
		if req.VelocityCoherent.VelocityEps > 0 {
			vcConfig.Clustering.VelocityEps = req.VelocityCoherent.VelocityEps
		}
		if req.VelocityCoherent.MaxPredictionFrames > 0 {
			vcConfig.PostTail.MaxPredictionFrames = req.VelocityCoherent.MaxPredictionFrames
		}
		if req.VelocityCoherent.MaxMisses > 0 {
			vcConfig.MaxMisses = req.VelocityCoherent.MaxMisses
		}
		if req.VelocityCoherent.HitsToConfirm > 0 {
			vcConfig.HitsToConfirm = req.VelocityCoherent.HitsToConfirm
		}

		pipeline.UpdateVCConfig(vcConfig)
	}

	// Update DBSCAN configuration if specified
	if req.DBSCAN != nil {
		dbscanParams := pipeline.GetConfig().DBSCANParams

		if req.DBSCAN.Eps > 0 {
			dbscanParams.Eps = req.DBSCAN.Eps
		}
		if req.DBSCAN.MinPts > 0 {
			dbscanParams.MinPts = req.DBSCAN.MinPts
		}

		pipeline.UpdateDBSCANParams(dbscanParams)
	}

	// If form submission, redirect back to status page
	if contentType == "application/x-www-form-urlencoded" || r.Header.Get("Content-Type") == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Return updated configuration (for JSON API clients)
	api.handleGetAlgorithmConfig(w, pipeline)
}

// handleTrackingStats returns statistics for both algorithms.
func (api *AlgorithmAPI) handleTrackingStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	api.mu.RLock()
	pipeline := api.pipeline
	api.mu.RUnlock()

	if pipeline == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "pipeline not configured")
		return
	}

	stats := pipeline.GetStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleVCTracks returns active velocity-coherent tracks.
func (api *AlgorithmAPI) handleVCTracks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	api.mu.RLock()
	vcTracker := api.vcTracker
	pipeline := api.pipeline
	api.mu.RUnlock()

	// Try to get tracker from pipeline if not directly set
	if vcTracker == nil && pipeline != nil {
		vcTracker = pipeline.GetVCTracker()
	}

	if vcTracker == nil {
		api.writeJSONError(w, http.StatusServiceUnavailable, "velocity-coherent tracker not configured")
		return
	}

	tracks := vcTracker.GetActiveTracks()

	// Convert to response format
	type VCTrackResponse struct {
		TrackID             string  `json:"track_id"`
		SensorID            string  `json:"sensor_id"`
		State               string  `json:"state"`
		X                   float32 `json:"x"`
		Y                   float32 `json:"y"`
		VX                  float32 `json:"vx"`
		VY                  float32 `json:"vy"`
		SpeedMps            float32 `json:"speed_mps"`
		VelocityConfidence  float32 `json:"velocity_confidence"`
		VelocityConsistency float32 `json:"velocity_consistency"`
		ObservationCount    int     `json:"observation_count"`
		Hits                int     `json:"hits"`
		Misses              int     `json:"misses"`
		MinPointsObserved   int     `json:"min_points_observed"`
		SparseFrameCount    int     `json:"sparse_frame_count"`
		ObjectClass         string  `json:"object_class,omitempty"`
		ObjectConfidence    float32 `json:"object_confidence,omitempty"`
	}

	response := make([]VCTrackResponse, 0, len(tracks))
	for _, t := range tracks {
		response = append(response, VCTrackResponse{
			TrackID:             t.TrackID,
			SensorID:            t.SensorID,
			State:               string(t.State),
			X:                   t.X,
			Y:                   t.Y,
			VX:                  t.VX,
			VY:                  t.VY,
			SpeedMps:            t.AvgSpeedMps,
			VelocityConfidence:  t.VelocityConfidence,
			VelocityConsistency: t.VelocityConsistency,
			ObservationCount:    t.ObservationCount,
			Hits:                t.Hits,
			Misses:              t.Misses,
			MinPointsObserved:   t.MinPointsObserved,
			SparseFrameCount:    t.SparseFrameCount,
			ObjectClass:         t.ObjectClass,
			ObjectConfidence:    t.ObjectConfidence,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tracks":      response,
		"track_count": len(response),
	})
}

// handleVCConfig returns/updates just the velocity-coherent configuration.
func (api *AlgorithmAPI) handleVCConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	pipeline := api.pipeline
	vcTracker := api.vcTracker
	api.mu.RUnlock()

	// Try to get tracker from pipeline if not directly set
	if vcTracker == nil && pipeline != nil {
		vcTracker = pipeline.GetVCTracker()
	}

	switch r.Method {
	case http.MethodGet:
		if vcTracker == nil {
			api.writeJSONError(w, http.StatusServiceUnavailable, "velocity-coherent tracker not configured")
			return
		}

		config := vcTracker.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"min_pts":               config.Clustering.MinPts,
			"position_eps":          config.Clustering.PositionEps,
			"velocity_eps":          config.Clustering.VelocityEps,
			"max_prediction_frames": config.PostTail.MaxPredictionFrames,
			"max_misses":            config.MaxMisses,
			"hits_to_confirm":       config.HitsToConfirm,
			"max_tracks":            config.MaxTracks,
		})

	case http.MethodPost:
		if vcTracker == nil {
			api.writeJSONError(w, http.StatusServiceUnavailable, "velocity-coherent tracker not configured")
			return
		}

		var req VCConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}

		config := vcTracker.GetConfig()

		if req.MinPts > 0 {
			config.Clustering.MinPts = req.MinPts
		}
		if req.PositionEps > 0 {
			config.Clustering.PositionEps = req.PositionEps
		}
		if req.VelocityEps > 0 {
			config.Clustering.VelocityEps = req.VelocityEps
		}
		if req.MaxPredictionFrames > 0 {
			config.PostTail.MaxPredictionFrames = req.MaxPredictionFrames
		}
		if req.MaxMisses > 0 {
			config.MaxMisses = req.MaxMisses
		}
		if req.HitsToConfirm > 0 {
			config.HitsToConfirm = req.HitsToConfirm
		}

		vcTracker.UpdateConfig(config)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET or POST")
	}
}

// writeJSONError writes a JSON error response.
func (api *AlgorithmAPI) writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
