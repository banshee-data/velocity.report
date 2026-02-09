package monitor

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Phase 1.6: REST API endpoints for lidar_run_tracks labelling
// Phase 1.7: REST API for analysis run management
//
// These handlers are methods on WebServer since it already has access to
// the AnalysisRunManager and db. Routes are registered in RegisterRoutes().

// handleRunTrackAPI is the main dispatcher for /api/lidar/runs/* endpoints.
// It parses the URL path and dispatches to appropriate sub-handlers.
func (ws *WebServer) handleRunTrackAPI(w http.ResponseWriter, r *http.Request) {
	runID, subPath := parseRunPath(r.URL.Path)

	// Handle /api/lidar/runs (list runs)
	if runID == "" {
		ws.handleListRuns(w, r)
		return
	}

	// Handle /api/lidar/runs/{run_id} (get run details)
	if subPath == "" {
		ws.handleGetRun(w, r, runID)
		return
	}

	// Handle /api/lidar/runs/{run_id}/tracks (list run tracks)
	if subPath == "tracks" {
		ws.handleListRunTracks(w, r, runID)
		return
	}

	// Handle /api/lidar/runs/{run_id}/labelling-progress
	if subPath == "labelling-progress" {
		ws.handleLabellingProgress(w, r, runID)
		return
	}

	// Handle /api/lidar/runs/{run_id}/reprocess
	if subPath == "reprocess" {
		ws.handleReprocessRun(w, r, runID)
		return
	}

	// Handle /api/lidar/runs/{run_id}/evaluate (Phase 4.5)
	if subPath == "evaluate" {
		ws.handleEvaluateRun(w, r, runID)
		return
	}

	// Handle /api/lidar/runs/{run_id}/tracks/{track_id}/*
	if strings.HasPrefix(subPath, "tracks/") {
		trackPath := strings.TrimPrefix(subPath, "tracks/")
		trackID, action := parseTrackPath(trackPath)
		if trackID == "" {
			ws.writeJSONError(w, http.StatusBadRequest, "missing track_id in path")
			return
		}

		switch action {
		case "label":
			ws.handleUpdateTrackLabel(w, r, runID, trackID)
		case "flags":
			ws.handleUpdateTrackFlags(w, r, runID, trackID)
		default:
			ws.writeJSONError(w, http.StatusNotFound, "unknown track action")
		}
		return
	}

	ws.writeJSONError(w, http.StatusNotFound, "endpoint not found")
}

// parseRunPath extracts run_id and remaining path segments from /api/lidar/runs/{run_id}/...
func parseRunPath(path string) (runID string, subPath string) {
	trimmed := strings.TrimPrefix(path, "/api/lidar/runs/")
	if trimmed == path {
		// No prefix match, return empty
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	runID = parts[0]
	if len(parts) > 1 {
		subPath = parts[1]
	}
	return
}

// parseTrackPath extracts track_id and action from tracks/{track_id}/{action}
func parseTrackPath(path string) (trackID string, action string) {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	trackID = parts[0]
	if len(parts) > 1 {
		action = parts[1]
	}
	return
}

// Phase 1.6 Handlers

// handleUpdateTrackLabel updates the user label and quality label for a track.
// PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label
// Request body: {"user_label": "good_vehicle", "quality_label": "perfect", "label_confidence": 0.95, "labeler_id": "user1"}
func (ws *WebServer) handleUpdateTrackLabel(w http.ResponseWriter, r *http.Request, runID, trackID string) {
	if r.Method != http.MethodPut {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use PUT")
		return
	}

	var req struct {
		UserLabel       string  `json:"user_label"`
		QualityLabel    string  `json:"quality_label"`
		LabelConfidence float32 `json:"label_confidence"`
		LabelerID       string  `json:"labeler_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Validate user_label (allow empty to clear)
	if req.UserLabel != "" && !api.ValidateUserLabel(req.UserLabel) {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid user_label: %s", req.UserLabel))
		return
	}

	// Validate quality_label (allow empty to clear)
	if req.QualityLabel != "" && !api.ValidateQualityLabel(req.QualityLabel) {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid quality_label: %s", req.QualityLabel))
		return
	}

	// Update the track label
	store := lidar.NewAnalysisRunStore(ws.db.DB)
	err := store.UpdateTrackLabel(runID, trackID, req.UserLabel, req.QualityLabel, req.LabelConfidence, req.LabelerID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update track label: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "ok",
		"run_id":           runID,
		"track_id":         trackID,
		"user_label":       req.UserLabel,
		"quality_label":    req.QualityLabel,
		"label_confidence": req.LabelConfidence,
		"labeler_id":       req.LabelerID,
	})
}

// handleUpdateTrackFlags updates the split/merge flags for a track.
// PUT /api/lidar/runs/{run_id}/tracks/{track_id}/flags
// Request body: {"linked_track_ids": ["track-002", "track-003"], "user_label": "split"}
func (ws *WebServer) handleUpdateTrackFlags(w http.ResponseWriter, r *http.Request, runID, trackID string) {
	if r.Method != http.MethodPut {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use PUT")
		return
	}

	var req struct {
		LinkedTrackIDs []string `json:"linked_track_ids"`
		UserLabel      string   `json:"user_label,omitempty"` // Optional: "split" or "merge"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Determine split/merge flags from user_label
	isSplit := false
	isMerge := false
	if req.UserLabel == "split" {
		isSplit = true
	} else if req.UserLabel == "merge" {
		isMerge = true
	}

	// Update the track quality flags
	store := lidar.NewAnalysisRunStore(ws.db.DB)
	err := store.UpdateTrackQualityFlags(runID, trackID, isSplit, isMerge, req.LinkedTrackIDs)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update track flags: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "ok",
		"run_id":           runID,
		"track_id":         trackID,
		"is_split":         isSplit,
		"is_merge":         isMerge,
		"linked_track_ids": req.LinkedTrackIDs,
	})
}

// handleListRunTracks lists all tracks for an analysis run.
// GET /api/lidar/runs/{run_id}/tracks
func (ws *WebServer) handleListRunTracks(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	store := lidar.NewAnalysisRunStore(ws.db.DB)
	tracks, err := store.GetRunTracks(runID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get run tracks: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"run_id": runID,
		"tracks": tracks,
		"count":  len(tracks),
	})
}

// handleLabellingProgress returns labelling statistics for a run.
// GET /api/lidar/runs/{run_id}/labelling-progress
func (ws *WebServer) handleLabellingProgress(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	store := lidar.NewAnalysisRunStore(ws.db.DB)
	total, labelled, byClass, err := store.GetLabelingProgress(runID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get labelling progress: %v", err))
		return
	}

	// Calculate progress percentage
	progressPct := 0.0
	if total > 0 {
		progressPct = float64(labelled) / float64(total) * 100.0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"run_id":       runID,
		"total":        total,
		"labelled":     labelled,
		"by_class":     byClass,
		"progress_pct": progressPct,
	})
}

// Phase 1.7 Handlers

// handleListRuns lists analysis runs with optional filters.
// GET /api/lidar/runs?limit=50&sensor_id=sensor1&status=completed
func (ws *WebServer) handleListRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	limitStr := query.Get("limit")
	sensorID := query.Get("sensor_id")
	status := query.Get("status")

	// Default limit
	limit := 50
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Fetch runs from database
	store := lidar.NewAnalysisRunStore(ws.db.DB)
	runs, err := store.ListRuns(limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list runs: %v", err))
		return
	}

	// Apply filters (sensor_id, status)
	var filteredRuns []*lidar.AnalysisRun
	for _, run := range runs {
		if sensorID != "" && run.SensorID != sensorID {
			continue
		}
		if status != "" && run.Status != status {
			continue
		}
		filteredRuns = append(filteredRuns, run)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"runs":  filteredRuns,
		"count": len(filteredRuns),
	})
}

// handleGetRun returns details for a specific analysis run.
// GET /api/lidar/runs/{run_id}
func (ws *WebServer) handleGetRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	store := lidar.NewAnalysisRunStore(ws.db.DB)
	run, err := store.GetRun(runID)
	if errors.Is(err, sql.ErrNoRows) {
		ws.writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get run: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
}

// handleReprocessRun re-runs analysis on a PCAP file (placeholder).
// POST /api/lidar/runs/{run_id}/reprocess
func (ws *WebServer) handleReprocessRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use POST")
		return
	}

	// Phase 2 implementation: connect to PCAP replay
	// For now, return 501 Not Implemented
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "not_implemented",
		"message": "Reprocessing not yet implemented. This will be connected to PCAP replay in Phase 2.",
		"run_id":  runID,
	})
}

// Phase 4.5: Ground Truth Evaluation Endpoint

// handleEvaluateRun compares a candidate run against a reference run and returns ground truth scores.
// POST /api/lidar/runs/{run_id}/evaluate
// Request body: {"reference_run_id": "..."} or auto-detect from scene
func (ws *WebServer) handleEvaluateRun(w http.ResponseWriter, r *http.Request, candidateRunID string) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use POST")
		return
	}

	var req struct {
		ReferenceRunID string `json:"reference_run_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	referenceRunID := req.ReferenceRunID

	// If no reference run specified, try to auto-detect from scene
	if referenceRunID == "" {
		// Get the candidate run to find its source path / scene
		store := lidar.NewAnalysisRunStore(ws.db.DB)
		candidateRun, err := store.GetRun(candidateRunID)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get candidate run: %v", err))
			return
		}

		// Try to find a scene for this sensor that has a reference run
		// Match by sensor ID and optionally source path
		sceneStore := lidar.NewSceneStore(ws.db.DB)
		scenes, err := sceneStore.ListScenes(candidateRun.SensorID)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list scenes: %v", err))
			return
		}

		// Find scene matching sensor and source path (if available)
		// First pass: try to match both sensor and source path
		if candidateRun.SourcePath != "" {
			for _, scene := range scenes {
				if scene.ReferenceRunID != "" && scene.PCAPFile == candidateRun.SourcePath {
					referenceRunID = scene.ReferenceRunID
					break
				}
			}
		}

		// Second pass: if no exact match, fall back to first scene with reference for this sensor
		// This is a reasonable heuristic when source path matching isn't possible
		if referenceRunID == "" {
			for _, scene := range scenes {
				if scene.ReferenceRunID != "" {
					referenceRunID = scene.ReferenceRunID
					break
				}
			}
		}

		if referenceRunID == "" {
			ws.writeJSONError(w, http.StatusBadRequest, "no reference_run_id provided and no scene with reference run found")
			return
		}
	}

	// Perform ground truth evaluation
	runStore := lidar.NewAnalysisRunStore(ws.db.DB)
	evaluator := lidar.NewGroundTruthEvaluator(runStore, lidar.DefaultGroundTruthWeights())

	score, err := evaluator.Evaluate(referenceRunID, candidateRunID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("evaluation failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"reference_run_id": referenceRunID,
		"candidate_run_id": candidateRunID,
		"score":            score,
	}); err != nil {
		// Log but don't write error response - headers already sent
		fmt.Fprintf(w, `{"error": "failed to encode response"}`)
	}
}
