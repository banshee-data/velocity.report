package monitor

import (
	"context"
	"encoding/json"
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

// handleSweepStart starts a parameter sweep
func (ws *WebServer) handleSweepStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if ws.sweepRunner == nil {
		http.Error(w, "Sweep runner not configured", http.StatusServiceUnavailable)
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := ws.sweepRunner.Start(r.Context(), req); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// handleSweepStatus returns the current sweep state
func (ws *WebServer) handleSweepStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if ws.sweepRunner == nil {
		http.Error(w, "Sweep runner not configured", http.StatusServiceUnavailable)
		return
	}

	state := ws.sweepRunner.GetState()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// handleSweepStop cancels a running sweep
func (ws *WebServer) handleSweepStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if ws.sweepRunner == nil {
		http.Error(w, "Sweep runner not configured", http.StatusServiceUnavailable)
		return
	}

	ws.sweepRunner.Stop()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}
