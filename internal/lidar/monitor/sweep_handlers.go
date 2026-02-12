package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// SweepRunner defines the interface for parameter sweep operations.
// This interface allows the monitor package to work with sweep runners
// without importing the sweep package, avoiding import cycles.
type SweepRunner interface {
	Start(ctx context.Context, req interface{}) error
	GetState() interface{}
	Stop()
}

// AutoTuneRunner defines the interface for auto-tune operations.
type AutoTuneRunner interface {
	Start(ctx context.Context, req interface{}) error
	GetState() interface{}
	Stop()
}

// RLHFRunner defines the interface for RLHF sweep operations.
type RLHFRunner interface {
	Start(ctx context.Context, req interface{}) error
	GetState() interface{}
	Stop()
	ContinueFromLabels(nextDurationMins int, addRound bool) error
	// WaitForChange blocks until the RLHF status differs from lastStatus
	// or the context is cancelled. Returns the new state.
	WaitForChange(ctx context.Context, lastStatus string) interface{}
}

// handleSweepStart starts a parameter sweep
func (ws *WebServer) handleSweepStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.sweepRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "sweep runner not configured")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if err := ws.sweepRunner.Start(context.Background(), req); err != nil {
		// Distinguish "already running" (409) from validation errors (400).
		// We check the message because the sweep package sentinel can't be
		// imported here without creating an import cycle.
		if strings.Contains(err.Error(), "already in progress") {
			ws.writeJSONError(w, http.StatusConflict, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// handleSweepStatus returns the current sweep state
func (ws *WebServer) handleSweepStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.sweepRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "sweep runner not configured")
		return
	}

	state := ws.sweepRunner.GetState()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// handleSweepStop cancels a running sweep
func (ws *WebServer) handleSweepStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.sweepRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "sweep runner not configured")
		return
	}

	ws.sweepRunner.Stop()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// handleAutoTune handles both starting (POST) and getting status (GET) for auto-tune
func (ws *WebServer) handleAutoTune(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		ws.handleAutoTuneStart(w, r)
	} else if r.Method == http.MethodGet {
		ws.handleAutoTuneStatus(w, r)
	} else {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAutoTuneStart starts an auto-tuning run
func (ws *WebServer) handleAutoTuneStart(w http.ResponseWriter, r *http.Request) {
	if ws.autoTuneRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "auto-tune runner not configured")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if err := ws.autoTuneRunner.Start(context.Background(), req); err != nil {
		// Distinguish "already running" (409) from validation errors (400)
		if strings.Contains(err.Error(), "already in progress") {
			ws.writeJSONError(w, http.StatusConflict, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// handleAutoTuneStatus returns the current auto-tuning state
func (ws *WebServer) handleAutoTuneStatus(w http.ResponseWriter, r *http.Request) {
	if ws.autoTuneRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "auto-tune runner not configured")
		return
	}

	state := ws.autoTuneRunner.GetState()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// handleAutoTuneStop cancels a running auto-tune
func (ws *WebServer) handleAutoTuneStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.autoTuneRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "auto-tune runner not configured")
		return
	}

	ws.autoTuneRunner.Stop()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// handleRLHF handles both starting (POST) and getting status (GET) for RLHF sweep.
func (ws *WebServer) handleRLHF(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		ws.handleRLHFStart(w, r)
	} else if r.Method == http.MethodGet {
		ws.handleRLHFStatus(w, r)
	} else {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRLHFStart starts an RLHF sweep.
func (ws *WebServer) handleRLHFStart(w http.ResponseWriter, r *http.Request) {
	if ws.rlhfRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "RLHF runner not configured")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	// Use context.Background so the RLHF goroutine outlives the HTTP request.
	if err := ws.rlhfRunner.Start(context.Background(), req); err != nil {
		if strings.Contains(err.Error(), "already in progress") {
			ws.writeJSONError(w, http.StatusConflict, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// handleRLHFStatus returns the current RLHF state.
// With ?wait_for_change=<status>, the handler blocks until the status differs
// from the supplied value — replacing 5-second polling with long-polling.
func (ws *WebServer) handleRLHFStatus(w http.ResponseWriter, r *http.Request) {
	if ws.rlhfRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "RLHF runner not configured")
		return
	}

	var state interface{}
	if lastStatus := r.URL.Query().Get("wait_for_change"); lastStatus != "" {
		state = ws.rlhfRunner.WaitForChange(r.Context(), lastStatus)
		if r.Context().Err() != nil {
			return // client disconnected
		}
	} else {
		state = ws.rlhfRunner.GetState()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// handleRLHFContinue signals the RLHF tuner to proceed from labels to sweep.
func (ws *WebServer) handleRLHFContinue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.rlhfRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "RLHF runner not configured")
		return
	}

	var body struct {
		NextSweepDurationMins int  `json:"next_sweep_duration_mins"`
		AddRound              bool `json:"add_round"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Allow empty body (io.EOF), but reject malformed JSON.
		if !errors.Is(err, io.EOF) {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		body.NextSweepDurationMins = 0
		body.AddRound = false
	}

	if err := ws.rlhfRunner.ContinueFromLabels(body.NextSweepDurationMins, body.AddRound); err != nil {
		if strings.Contains(err.Error(), "threshold") {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		} else if strings.Contains(err.Error(), "not in awaiting_labels") {
			ws.writeJSONError(w, http.StatusConflict, err.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "continued"})
}

// handleRLHFStop cancels a running RLHF sweep.
func (ws *WebServer) handleRLHFStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.rlhfRunner == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "RLHF runner not configured")
		return
	}

	ws.rlhfRunner.Stop()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// handleListSweeps returns a list of recent sweep records for the current sensor.
func (ws *WebServer) handleListSweeps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.sweepStore == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "sweep store not configured")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	sweeps, err := ws.sweepStore.ListSweeps(sensorID, limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to list sweeps: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sweeps)
}

// handleGetSweep returns a single sweep record with full results.
func (ws *WebServer) handleGetSweep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.sweepStore == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "sweep store not configured")
		return
	}

	// Extract sweep_id from path: /api/lidar/sweeps/{sweep_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/lidar/sweeps/")
	sweepID := strings.TrimRight(path, "/")
	if sweepID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing sweep_id in path")
		return
	}

	sweep, err := ws.sweepStore.GetSweep(sweepID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to get sweep: "+err.Error())
		return
	}
	if sweep == nil {
		ws.writeJSONError(w, http.StatusNotFound, "sweep not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sweep)
}

// handleSweepCharts saves chart configuration for a sweep.
func (ws *WebServer) handleSweepCharts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.sweepStore == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "sweep store not configured")
		return
	}

	var req struct {
		SweepID string          `json:"sweep_id"`
		Charts  json.RawMessage `json:"charts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if req.SweepID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "sweep_id is required")
		return
	}

	// Validate that charts is a valid JSON array or object, not primitives or double-encoded strings
	if len(req.Charts) > 0 {
		var test interface{}
		if err := json.Unmarshal(req.Charts, &test); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "charts must be valid JSON: "+err.Error())
			return
		}
		// Only allow JSON objects or arrays; reject primitives and double-encoded strings
		switch test.(type) {
		case map[string]interface{}, []interface{}:
			// valid chart structure
		default:
			ws.writeJSONError(w, http.StatusBadRequest, "charts must be a JSON array or object")
			return
		}
	}

	if err := ws.sweepStore.UpdateSweepCharts(req.SweepID, req.Charts); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to save charts: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}

// handleSweepExplain returns a score explanation for a sweep.
func (ws *WebServer) handleSweepExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.sweepStore == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "sweep store not configured")
		return
	}

	// Extract sweep_id from path: /api/lidar/sweep/explain/{sweep_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/lidar/sweep/explain/")
	sweepID := strings.TrimRight(path, "/")
	if sweepID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing sweep_id in path")
		return
	}

	record, err := ws.sweepStore.GetSweep(sweepID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to get sweep: "+err.Error())
		return
	}
	if record == nil {
		ws.writeJSONError(w, http.StatusNotFound, "sweep not found")
		return
	}

	// Build explanation from stored components
	var response struct {
		SweepID                   string          `json:"sweep_id"`
		ObjectiveName             string          `json:"objective_name,omitempty"`
		ObjectiveVersion          string          `json:"objective_version,omitempty"`
		ScoreComponents           json.RawMessage `json:"score_components,omitempty"`
		RecommendationExplanation json.RawMessage `json:"recommendation_explanation,omitempty"`
		LabelProvenanceSummary    json.RawMessage `json:"label_provenance_summary,omitempty"`
	}
	response.SweepID = record.SweepID
	response.ObjectiveName = record.ObjectiveName
	response.ObjectiveVersion = record.ObjectiveVersion
	response.ScoreComponents = record.ScoreComponents
	response.RecommendationExplanation = record.RecommendationExplanation
	response.LabelProvenanceSummary = record.LabelProvenanceSummary

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
