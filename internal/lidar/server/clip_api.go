package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/adapters"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/google/uuid"
)

var (
	startPCAPInternalForClip = func(ws *Server, pcapPath string, config ReplayConfig) error {
		return ws.StartPCAPInternal(pcapPath, config)
	}
	lastRunIDForClip = func(ws *Server) string {
		return ws.LastRunID()
	}
)

// REST API for clip management
// These handlers manage LiDAR clips (PCAP + sensor + params).
//
// Routes:
// - GET /api/lidar/clips — list clips (optional sensor_id filter)
// - POST /api/lidar/clips — create clip
// - GET /api/lidar/clips/{clip_id} — get clip details
// - PUT /api/lidar/clips/{clip_id} — update clip
// - DELETE /api/lidar/clips/{clip_id} — delete clip
// - POST /api/lidar/clips/{clip_id}/replay — clip

// handleClips handles /api/lidar/clips (list and create).
func (ws *Server) handleClips(w http.ResponseWriter, r *http.Request) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		ws.handleListClips(w, r)
	case http.MethodPost:
		ws.handleCreateClip(w, r)
	default:
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleClipByID handles /api/lidar/clips/{clip_id}/* routes.
func (ws *Server) handleClipByID(w http.ResponseWriter, r *http.Request) {
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	clipID, action := parseClipPath(r.URL.Path)
	if clipID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing clip_id in path")
		return
	}

	switch action {
	case "":
		// /api/lidar/clips/{clip_id}
		switch r.Method {
		case http.MethodGet:
			ws.handleGetClip(w, r, clipID)
		case http.MethodPut:
			ws.handleUpdateClip(w, r, clipID)
		case http.MethodDelete:
			ws.handleDeleteClip(w, r, clipID)
		default:
			ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "replay":
		// /api/lidar/clips/{clip_id}/replay
		if r.Method == http.MethodPost {
			ws.handleReplayClip(w, r, clipID)
		} else {
			ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "clip":
		if r.Method == http.MethodPost {
			ws.handleClipPack(w, r, clipID)
		} else {
			ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "location":
		// /api/lidar/clips/{clip_id}/location
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete:
			ws.handleSetCaseLocation(w, r, clipID)
		default:
			ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "evaluations":
		// /api/lidar/clips/{clip_id}/evaluations
		switch r.Method {
		case http.MethodGet:
			ws.handleListClipEvaluations(w, r, clipID)
		case http.MethodPost:
			ws.handleCreateClipEvaluation(w, r, clipID)
		default:
			ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		ws.writeJSONError(w, http.StatusNotFound, "endpoint not found")
	}
}

// parseClipPath extracts clip_id and action from /api/lidar/clips/{clip_id}/{action}
func parseClipPath(path string) (clipID string, action string) {
	trimmed := strings.TrimPrefix(path, "/api/lidar/clips/")
	if trimmed == path {
		// No prefix match
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	clipID = parts[0]
	if len(parts) > 1 {
		action = parts[1]
	}
	return
}

// handleListClips lists all clips, optionally filtered by sensor_id.
func (ws *Server) handleListClips(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")

	store := sqlite.NewClipStore(ws.db)
	clips, err := store.ListClips(sensorID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list clips: %v", err))
		return
	}

	// Ensure we return an empty array instead of null when no clips
	if clips == nil {
		clips = []*sqlite.Clip{}
	}

	ws.writeJSON(w, http.StatusOK, map[string]interface{}{
		"clips": clips,
		"count": len(clips),
	})
}

// CreateClipRequest is the request body for creating a clip.
type CreateClipRequest struct {
	SensorID string `json:"sensor_id"`
	// PCAPFiles is the ordered capture sequence. Offsets below are measured
	// from the start of the whole sequence rather than any one file.
	PCAPFiles        []string `json:"pcap_files,omitempty"`
	PCAPStartSecs    *float64 `json:"pcap_start_secs,omitempty"`
	PCAPDurationSecs *float64 `json:"pcap_duration_secs,omitempty"`
	Description      string   `json:"description,omitempty"`
	// SessionID and SourcePeriodID record where the case came from when it was
	// cut from an indexed session. Both advisory.
	SessionID      string `json:"session_id,omitempty"`
	SourcePeriodID string `json:"source_period_id,omitempty"`
}

// handleCreateClip creates a new clip.
func (ws *Server) handleCreateClip(w http.ResponseWriter, r *http.Request) {
	var req CreateClipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Validate required fields
	if req.SensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "sensor_id is required")
		return
	}
	files := req.PCAPFiles
	if len(files) == 0 {
		ws.writeJSONError(w, http.StatusBadRequest, "pcap_files is required")
		return
	}

	// A case whose captures do not abut is not a case: replaying it would
	// present two unrelated stretches to the pipeline as one recording.
	// Authoring time is the last point at which the operator can still do
	// something about it, so the check happens here rather than at replay.
	if len(files) > 1 {
		if _, err := ws.validateClipFiles(files); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	clip := &sqlite.Clip{
		SensorID: req.SensorID, Files: []sqlite.ClipCapture{{PCAPFile: files[0]}}, PCAPStartSecs: req.PCAPStartSecs,
		PCAPDurationSecs: req.PCAPDurationSecs,
		Description:      req.Description,
	}

	store := sqlite.NewClipStore(ws.db)
	if err := store.InsertClip(clip); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create clip: %v", err))
		return
	}

	caseFiles := make([]sqlite.ClipCapture, 0, len(files))
	for i, f := range files {
		caseFiles = append(caseFiles, sqlite.ClipCapture{Ordinal: i, PCAPFile: f})
	}
	if err := store.SetClipFiles(clip.ClipID, caseFiles); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to record the case's captures: %v", err))
		return
	}
	if req.SessionID != "" || req.SourcePeriodID != "" {
		if err := store.SetClipSession(clip.ClipID, req.SessionID, req.SourcePeriodID); err != nil {
			opsf("Warning: could not link clip %s to its session: %v", clip.ClipID, err)
		}
	}

	clip.Files = caseFiles
	clip.FileCount = len(caseFiles)
	clip.SessionID = req.SessionID
	clip.SourcePeriodID = req.SourcePeriodID
	ws.writeJSON(w, http.StatusCreated, clip)
}

// handleGetClip retrieves a clip by ID.
func (ws *Server) handleGetClip(w http.ResponseWriter, r *http.Request, clipID string) {
	store := sqlite.NewClipStore(ws.db)
	clip, err := store.GetClip(clipID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get clip: %v", err))
		}
		return
	}

	// A case is its file list; reading one without it would show only the
	// first capture and quietly misrepresent a multi-file case as single-file.
	if files, err := store.ClipFiles(clipID); err != nil {
		opsf("Warning: could not load captures for clip %s: %v", clipID, err)
	} else {
		clip.Files = files
	}
	if loc, err := store.CaseLocationOf(clipID); err != nil {
		opsf("Warning: could not load the location of clip %s: %v", clipID, err)
	} else {
		clip.Location = loc
	}

	ws.writeJSON(w, http.StatusOK, clip)
}

// UpdateClipRequest is the request body for updating a clip.
type UpdateClipRequest struct {
	Description       *string          `json:"description,omitempty"`
	ReferenceRunID    *string          `json:"reference_run_id,omitempty"`
	OptimalParamsJSON *json.RawMessage `json:"optimal_params_json,omitempty"`
	PCAPStartSecs     *float64         `json:"pcap_start_secs,omitempty"`
	PCAPDurationSecs  *float64         `json:"pcap_duration_secs,omitempty"`
	// PCAPFiles replaces the ordered capture association. Its order is the
	// replay order and is deliberately editable: field capture rolls files
	// every five minutes, while one useful case often spans several rolls.
	PCAPFiles []string `json:"pcap_files,omitempty"`
}

// handleUpdateClip updates a clip's fields.
func (ws *Server) handleUpdateClip(w http.ResponseWriter, r *http.Request, clipID string) {
	var req UpdateClipRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("failed to read body: %v", err))
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	store := sqlite.NewClipStore(ws.db)

	// Get existing clip
	clip, err := store.GetClip(clipID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get clip: %v", err))
		}
		return
	}

	// Update fields if provided
	if req.Description != nil {
		clip.Description = *req.Description
	}
	if req.ReferenceRunID != nil {
		clip.ReferenceRunID = *req.ReferenceRunID
	}
	if req.OptimalParamsJSON != nil {
		clip.OptimalParamsJSON = *req.OptimalParamsJSON
	}
	if req.PCAPStartSecs != nil {
		clip.PCAPStartSecs = req.PCAPStartSecs
	}
	if req.PCAPDurationSecs != nil {
		clip.PCAPDurationSecs = req.PCAPDurationSecs
	}
	if req.PCAPFiles != nil {
		if len(req.PCAPFiles) == 0 {
			ws.writeJSONError(w, http.StatusBadRequest, "pcap_files must contain at least one capture")
			return
		}
		if len(req.PCAPFiles) > 1 {
			if _, err := ws.validateClipFiles(req.PCAPFiles); err != nil {
				ws.writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
	}

	if err := store.UpdateClip(clip); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update clip: %v", err))
		return
	}
	if req.PCAPFiles != nil {
		files := make([]sqlite.ClipCapture, 0, len(req.PCAPFiles))
		for i, path := range req.PCAPFiles {
			files = append(files, sqlite.ClipCapture{Ordinal: i, PCAPFile: path})
		}
		if err := store.SetClipFiles(clipID, files); err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError,
				fmt.Sprintf("failed to update the case's captures: %v", err))
			return
		}
		clip.Files = files
		clip.FileCount = len(files)
	}

	ws.writeJSON(w, http.StatusOK, clip)
}

// handleDeleteClip deletes a clip by ID.
func (ws *Server) handleDeleteClip(w http.ResponseWriter, r *http.Request, clipID string) {
	store := sqlite.NewClipStore(ws.db)
	if err := store.DeleteClip(clipID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete clip: %v", err))
		}
		return
	}

	ws.writeJSON(w, http.StatusOK, map[string]string{
		"message": "clip deleted",
	})
}

// handleReplayClip handles PCAP replay for a clip.
// Replays the clip's PCAP file, creates an analysis run, and returns the run_id.
func (ws *Server) handleReplayClip(w http.ResponseWriter, r *http.Request, clipID string) {
	store := sqlite.NewClipStore(ws.db)
	clip, err := store.GetClip(clipID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, "clip not found")
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load clip: %v", err))
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

	// Determine which params to use as launch intent lineage.
	var paramsJSON json.RawMessage
	if req.ParamsJSON != nil {
		paramsJSON = req.ParamsJSON
	} else if len(clip.RecommendedParams) > 0 {
		paramsJSON = clip.RecommendedParams
	} else if len(clip.OptimalParamsJSON) > 0 {
		paramsJSON = clip.OptimalParamsJSON
	}
	preferredRunID := fmt.Sprintf("replay-%s-%s", clipID, uuid.New().String()[:8])

	// Start PCAP replay
	// Note: The actual run completion and track insertion will be handled by RunManager
	// when the frame builder processes the PCAP. This is a trigger to start the replay.
	var startSecs, durationSecs float64
	if clip.PCAPStartSecs != nil {
		startSecs = *clip.PCAPStartSecs
	}
	if clip.PCAPDurationSecs != nil {
		durationSecs = *clip.PCAPDurationSecs
	}

	// A case names an ordered set of captures, which for a single-file case is
	// a list of one and behaves exactly as before.
	casePaths, err := store.ClipPaths(clipID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to load the case's captures: %v", err))
		return
	}
	if len(casePaths) == 0 {
		ws.writeJSONError(w, http.StatusBadRequest, "the clip names no captures")
		return
	}

	config := ReplayConfig{
		StartSeconds:        startSecs,
		DurationSeconds:     durationSecs,
		AnalysisMode:        true, // Preserve state after completion
		SensorID:            clip.SensorID,
		PreferredRunID:      preferredRunID,
		ClipID:              clipID,
		RequestedParamsJSON: paramsJSON,
	}
	if len(casePaths) > 1 {
		// Multi-file replay is planned before it starts, so a broken join is
		// reported here rather than discovered mid-replay. It also requires
		// analysis speed mode, which a case replay already uses.
		config.ReplayFiles = casePaths
		config.SpeedMode = "analysis"
	}

	// Reset tracker to ensure deterministic track IDs starting from track_1
	if ws.tracker != nil {
		ws.tracker.Reset()
	}
	// Reset background grid for clean analysis
	if err := ws.resetBackgroundGrid(); err != nil {
		opsf("Warning: failed to reset background grid before replay: %v", err)
	}

	if err := startPCAPInternalForClip(ws, casePaths[0], config); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start PCAP replay: %v", err))
		return
	}
	runID := lastRunIDForClip(ws)
	if runID == "" {
		runID = preferredRunID
	}

	ws.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"run_id":  runID,
		"clip_id": clipID,
		"status":  "running",
		"message": "PCAP replay initiated; analysis run created",
	})
}

// handleListClipEvaluations lists all persisted ground truth evaluation scores for a clip.
// GET /api/lidar/clips/{clip_id}/evaluations
func (ws *Server) handleListClipEvaluations(w http.ResponseWriter, r *http.Request, clipID string) {
	evalStore := sqlite.NewEvaluationStore(ws.db)
	evals, err := evalStore.ListByClip(clipID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list evaluations: %v", err))
		return
	}
	if evals == nil {
		evals = []*sqlite.Evaluation{}
	}
	ws.writeJSON(w, http.StatusOK, map[string]interface{}{
		"clip_id":     clipID,
		"evaluations": evals,
	})
}

// handleCreateClipEvaluation evaluates a candidate run against the clip's reference run
// and persists the result.
// POST /api/lidar/clips/{clip_id}/evaluations
// Request body: {"candidate_run_id": "..."}
func (ws *Server) handleCreateClipEvaluation(w http.ResponseWriter, r *http.Request, clipID string) {
	clipStore := sqlite.NewClipStore(ws.db)
	clip, err := clipStore.GetClip(clipID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ws.writeJSONError(w, http.StatusNotFound, "clip not found")
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load clip: %v", err))
		}
		return
	}

	if clip.ReferenceRunID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "clip has no reference run; set reference_run_id first")
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
	runStore := sqlite.NewRunStore(ws.db)
	evaluator := adapters.NewGroundTruthEvaluator(runStore, adapters.DefaultGroundTruthWeights())
	score, err := evaluator.Evaluate(clip.ReferenceRunID, req.CandidateRunID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("evaluation failed: %v", err))
		return
	}

	eval := &sqlite.Evaluation{
		ClipID:              clipID,
		ReferenceRunID:      clip.ReferenceRunID,
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

	evalStore := sqlite.NewEvaluationStore(ws.db)
	if err := evalStore.Insert(eval); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to persist evaluation: %v", err))
		return
	}

	ws.writeJSON(w, http.StatusCreated, eval)
}
