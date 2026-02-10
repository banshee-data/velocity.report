package monitor

import (
	"context"
	"encoding/json"
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

	// Validate that charts is a valid JSON array or object, not a double-encoded string
	if len(req.Charts) > 0 {
		var test interface{}
		if err := json.Unmarshal(req.Charts, &test); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "charts must be valid JSON: "+err.Error())
			return
		}
		// If it's a string, it might be double-encoded - reject it
		if _, isString := test.(string); isString {
			ws.writeJSONError(w, http.StatusBadRequest, "charts must be a JSON array or object, not a JSON string")
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
