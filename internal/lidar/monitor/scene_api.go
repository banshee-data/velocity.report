package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/google/uuid"
)

// Phase 2.3: REST API for scene management
// These handlers manage LiDAR evaluation scenes (PCAP + sensor + params).
//
// Routes:
// - GET /api/lidar/scenes — list scenes (optional sensor_id filter)
// - POST /api/lidar/scenes — create scene
// - GET /api/lidar/scenes/{scene_id} — get scene details
// - PUT /api/lidar/scenes/{scene_id} — update scene
// - DELETE /api/lidar/scenes/{scene_id} — delete scene
// - POST /api/lidar/scenes/{scene_id}/replay — replay scene (placeholder for Phase 2.4/5)

// handleScenes handles /api/lidar/scenes (list and create).
func (ws *WebServer) handleScenes(w http.ResponseWriter, r *http.Request) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		ws.handleListScenes(w, r)
	case http.MethodPost:
		ws.handleCreateScene(w, r)
	default:
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleSceneByID handles /api/lidar/scenes/{scene_id}/* routes.
func (ws *WebServer) handleSceneByID(w http.ResponseWriter, r *http.Request) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	sceneID, action := parseScenePath(r.URL.Path)
	if sceneID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing scene_id in path")
		return
	}

	switch action {
	case "":
		// /api/lidar/scenes/{scene_id}
		switch r.Method {
		case http.MethodGet:
			ws.handleGetScene(w, r, sceneID)
		case http.MethodPut:
			ws.handleUpdateScene(w, r, sceneID)
		case http.MethodDelete:
			ws.handleDeleteScene(w, r, sceneID)
		default:
			ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "replay":
		// /api/lidar/scenes/{scene_id}/replay
		if r.Method == http.MethodPost {
			ws.handleReplayScene(w, r, sceneID)
		} else {
			ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "evaluations":
		// /api/lidar/scenes/{scene_id}/evaluations
		switch r.Method {
		case http.MethodGet:
			ws.handleListSceneEvaluations(w, r, sceneID)
		case http.MethodPost:
			ws.handleCreateSceneEvaluation(w, r, sceneID)
		default:
			ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		ws.writeJSONError(w, http.StatusNotFound, "endpoint not found")
	}
}

// parseScenePath extracts scene_id and action from /api/lidar/scenes/{scene_id}/{action}
func parseScenePath(path string) (sceneID string, action string) {
	trimmed := strings.TrimPrefix(path, "/api/lidar/scenes/")
	if trimmed == path {
		// No prefix match
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	sceneID = parts[0]
	if len(parts) > 1 {
		action = parts[1]
	}
	return
}

// handleListScenes lists all scenes, optionally filtered by sensor_id.
func (ws *WebServer) handleListScenes(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")

	store := lidar.NewSceneStore(ws.db.DB)
	scenes, err := store.ListScenes(sensorID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list scenes: %v", err))
		return
	}

	// Ensure we return an empty array instead of null when no scenes
	if scenes == nil {
		scenes = []*lidar.Scene{}
	}

	ws.writeJSON(w, http.StatusOK, map[string]interface{}{
		"scenes": scenes,
		"count":  len(scenes),
	})
}

// CreateSceneRequest is the request body for creating a scene.
type CreateSceneRequest struct {
	SensorID         string   `json:"sensor_id"`
	PCAPFile         string   `json:"pcap_file"`
	PCAPStartSecs    *float64 `json:"pcap_start_secs,omitempty"`
	PCAPDurationSecs *float64 `json:"pcap_duration_secs,omitempty"`
	Description      string   `json:"description,omitempty"`
}

// handleCreateScene creates a new scene.
func (ws *WebServer) handleCreateScene(w http.ResponseWriter, r *http.Request) {
	var req CreateSceneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Validate required fields
	if req.SensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "sensor_id is required")
		return
	}
	if req.PCAPFile == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "pcap_file is required")
		return
	}

	scene := &lidar.Scene{
		SensorID:         req.SensorID,
		PCAPFile:         req.PCAPFile,
		PCAPStartSecs:    req.PCAPStartSecs,
		PCAPDurationSecs: req.PCAPDurationSecs,
		Description:      req.Description,
	}

	store := lidar.NewSceneStore(ws.db.DB)
	if err := store.InsertScene(scene); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create scene: %v", err))
		return
	}

	ws.writeJSON(w, http.StatusCreated, scene)
}

// handleGetScene retrieves a scene by ID.
func (ws *WebServer) handleGetScene(w http.ResponseWriter, r *http.Request, sceneID string) {
	store := lidar.NewSceneStore(ws.db.DB)
	scene, err := store.GetScene(sceneID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get scene: %v", err))
		}
		return
	}

	ws.writeJSON(w, http.StatusOK, scene)
}

// UpdateSceneRequest is the request body for updating a scene.
type UpdateSceneRequest struct {
	Description       *string          `json:"description,omitempty"`
	ReferenceRunID    *string          `json:"reference_run_id,omitempty"`
	OptimalParamsJSON *json.RawMessage `json:"optimal_params_json,omitempty"`
	PCAPStartSecs     *float64         `json:"pcap_start_secs,omitempty"`
	PCAPDurationSecs  *float64         `json:"pcap_duration_secs,omitempty"`
}

// handleUpdateScene updates a scene's fields.
func (ws *WebServer) handleUpdateScene(w http.ResponseWriter, r *http.Request, sceneID string) {
	var req UpdateSceneRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("failed to read body: %v", err))
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	store := lidar.NewSceneStore(ws.db.DB)

	// Get existing scene
	scene, err := store.GetScene(sceneID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get scene: %v", err))
		}
		return
	}

	// Update fields if provided
	if req.Description != nil {
		scene.Description = *req.Description
	}
	if req.ReferenceRunID != nil {
		scene.ReferenceRunID = *req.ReferenceRunID
	}
	if req.OptimalParamsJSON != nil {
		scene.OptimalParamsJSON = *req.OptimalParamsJSON
	}
	if req.PCAPStartSecs != nil {
		scene.PCAPStartSecs = req.PCAPStartSecs
	}
	if req.PCAPDurationSecs != nil {
		scene.PCAPDurationSecs = req.PCAPDurationSecs
	}

	if err := store.UpdateScene(scene); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update scene: %v", err))
		return
	}

	ws.writeJSON(w, http.StatusOK, scene)
}

// handleDeleteScene deletes a scene by ID.
func (ws *WebServer) handleDeleteScene(w http.ResponseWriter, r *http.Request, sceneID string) {
	store := lidar.NewSceneStore(ws.db.DB)
	if err := store.DeleteScene(sceneID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete scene: %v", err))
		}
		return
	}

	ws.writeJSON(w, http.StatusOK, map[string]string{
		"message": "scene deleted",
	})
}

// handleReplayScene handles PCAP replay for a scene.
// Phase 2.4: Replays the scene's PCAP file, creates an analysis run, and returns the run_id.
func (ws *WebServer) handleReplayScene(w http.ResponseWriter, r *http.Request, sceneID string) {
	store := lidar.NewSceneStore(ws.db.DB)
	scene, err := store.GetScene(sceneID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, "scene not found")
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load scene: %v", err))
		}
		return
	}

	// Parse optional params override from request body
	var req struct {
		ParamsJSON json.RawMessage `json:"params_json,omitempty"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			// Ignore EOF errors (empty body is acceptable)
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
			return
		}
	}

	// Determine which params to use
	var paramsJSON json.RawMessage
	if req.ParamsJSON != nil {
		paramsJSON = req.ParamsJSON
	} else if len(scene.OptimalParamsJSON) > 0 {
		paramsJSON = scene.OptimalParamsJSON
	}

	// Create analysis run with UUID for uniqueness
	runStore := lidar.NewAnalysisRunStore(ws.db.DB)
	run := &lidar.AnalysisRun{
		RunID:      fmt.Sprintf("replay-%s-%s", sceneID, uuid.New().String()[:8]),
		SourceType: "pcap",
		SourcePath: scene.PCAPFile,
		SensorID:   scene.SensorID,
		Status:     "running",
		CreatedAt:  time.Now(),
		ParamsJSON: paramsJSON,
	}

	if err := runStore.InsertRun(run); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create analysis run: %v", err))
		return
	}

	// Start PCAP replay
	// Note: The actual run completion and track insertion will be handled by AnalysisRunManager
	// when the frame builder processes the PCAP. This is a trigger to start the replay.
	var startSecs, durationSecs float64
	if scene.PCAPStartSecs != nil {
		startSecs = *scene.PCAPStartSecs
	}
	if scene.PCAPDurationSecs != nil {
		durationSecs = *scene.PCAPDurationSecs
	}

	config := ReplayConfig{
		StartSeconds:    startSecs,
		DurationSeconds: durationSecs,
		AnalysisMode:    true, // Preserve state after completion
	}

	// Reset tracker to ensure deterministic track IDs starting from track_1
	if ws.tracker != nil {
		ws.tracker.Reset()
	}
	// Reset background grid for clean analysis
	if err := ws.resetBackgroundGrid(); err != nil {
		log.Printf("Warning: failed to reset background grid before replay: %v", err)
	}

	if err := ws.StartPCAPInternal(scene.PCAPFile, config); err != nil {
		// Update run status to failed
		if updateErr := runStore.UpdateRunStatus(run.RunID, "failed", fmt.Sprintf("PCAP replay failed: %v", err)); updateErr != nil {
			log.Printf("failed to update run status: %v", updateErr)
		}
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start PCAP replay: %v", err))
		return
	}

	ws.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"run_id":   run.RunID,
		"scene_id": sceneID,
		"status":   "running",
		"message":  "PCAP replay initiated; analysis run created",
	})
}

// handleListSceneEvaluations lists all persisted ground truth evaluation scores for a scene.
// GET /api/lidar/scenes/{scene_id}/evaluations
func (ws *WebServer) handleListSceneEvaluations(w http.ResponseWriter, r *http.Request, sceneID string) {
	evalStore := lidar.NewEvaluationStore(ws.db.DB)
	evals, err := evalStore.ListByScene(sceneID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list evaluations: %v", err))
		return
	}
	if evals == nil {
		evals = []*lidar.Evaluation{}
	}
	ws.writeJSON(w, http.StatusOK, map[string]interface{}{
		"scene_id":    sceneID,
		"evaluations": evals,
	})
}

// handleCreateSceneEvaluation evaluates a candidate run against the scene's reference run
// and persists the result.
// POST /api/lidar/scenes/{scene_id}/evaluations
// Request body: {"candidate_run_id": "..."}
func (ws *WebServer) handleCreateSceneEvaluation(w http.ResponseWriter, r *http.Request, sceneID string) {
	sceneStore := lidar.NewSceneStore(ws.db.DB)
	scene, err := sceneStore.GetScene(sceneID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, "scene not found")
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load scene: %v", err))
		}
		return
	}

	if scene.ReferenceRunID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "scene has no reference run; set reference_run_id first")
		return
	}

	var req struct {
		CandidateRunID string `json:"candidate_run_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	if req.CandidateRunID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "candidate_run_id is required")
		return
	}

	// Run evaluation
	runStore := lidar.NewAnalysisRunStore(ws.db.DB)
	evaluator := lidar.NewGroundTruthEvaluator(runStore, lidar.DefaultGroundTruthWeights())
	score, err := evaluator.Evaluate(scene.ReferenceRunID, req.CandidateRunID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("evaluation failed: %v", err))
		return
	}

	// Get candidate run params for snapshot
	candidateRun, err := runStore.GetRun(req.CandidateRunID)
	if err != nil {
		log.Printf("Warning: failed to get candidate run params: %v", err)
	}

	eval := &lidar.Evaluation{
		SceneID:             sceneID,
		ReferenceRunID:      scene.ReferenceRunID,
		CandidateRunID:      req.CandidateRunID,
		DetectionRate:       score.DetectionRate,
		Fragmentation:       score.Fragmentation,
		FalsePositiveRate:   score.FalsePositiveRate,
		VelocityCoverage:    score.VelocityCoverage,
		QualityPremium:      score.QualityPremium,
		TruncationRate:      score.TruncationRate,
		VelocityNoiseRate:   score.VelocityNoiseRate,
		StoppedRecoveryRate: score.StoppedRecoveryRate,
		CompositeScore:      score.CompositeScore,
		MatchedCount:        score.MatchedCount,
		ReferenceCount:      score.ReferenceCount,
		CandidateCount:      score.CandidateCount,
	}
	if candidateRun != nil && len(candidateRun.ParamsJSON) > 0 {
		eval.ParamsJSON = candidateRun.ParamsJSON
	}

	evalStore := lidar.NewEvaluationStore(ws.db.DB)
	if err := evalStore.Insert(eval); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to persist evaluation: %v", err))
		return
	}

	ws.writeJSON(w, http.StatusCreated, eval)
}
