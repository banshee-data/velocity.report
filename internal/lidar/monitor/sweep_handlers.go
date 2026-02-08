package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SweepRunner defines the interface for parameter sweep operations.
// This interface allows the monitor package to work with sweep runners
// without importing the sweep package, avoiding import cycles.
type SweepRunner interface {
	Start(ctx context.Context, req interface{}) error
	GetState() interface{}
	Stop()
}

// ErrSweepAlreadyRunning is returned when a sweep is already in progress.
var ErrSweepAlreadyRunning = fmt.Errorf("sweep already in progress")

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
		// Distinguish "already running" (409) from validation errors (400)
		if err.Error() == ErrSweepAlreadyRunning.Error() {
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
