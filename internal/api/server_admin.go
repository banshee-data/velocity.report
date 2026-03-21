package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/units"
)

func (s *Server) showConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	config := map[string]interface{}{
		"units":    s.units,
		"timezone": s.timezone,
	}

	if err := json.NewEncoder(w).Encode(config); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to write config")
		return
	}
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check for units override in query parameter
	displayUnits := s.units // default to CLI-set units
	if u := r.URL.Query().Get("units"); u != "" {
		if units.IsValid(u) {
			displayUnits = u
		} else {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'units' parameter. Must be one of: %s", units.GetValidUnitsString()))
			return
		}
	}

	// Check for timezone override in query parameter
	displayTimezone := s.timezone // default to CLI-set timezone
	if tz := r.URL.Query().Get("timezone"); tz != "" {
		if units.IsTimezoneValid(tz) {
			displayTimezone = tz
		} else {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'timezone' parameter. Must be one of: %s", units.GetValidTimezonesString()))
			return
		}
	}

	// TODO: Add timezone conversion for timestamps once database schema includes timestamps
	_ = displayTimezone // Silence unused variable warning for now

	events, err := s.db.Events()
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve events: %v", err))
		return
	}

	// without the EventAPI struct and EventToAPI function the response
	// would be a list of events with their raw fields (Float64, Valid).
	// we control the output format with the EventAPI struct.
	apiEvents := make([]db.EventAPI, len(events))
	for i, e := range events {
		apiEvents[i] = convertEventAPISpeed(db.EventToAPI(e), displayUnits)
	}

	if err := json.NewEncoder(w).Encode(apiEvents); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to write events")
		return
	}
}

// handleDatabaseStats returns database table sizes and disk usage statistics.
// GET: returns { total_size_mb: float, tables: [{name, row_count, size_mb}, ...] }
func (s *Server) handleDatabaseStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if s.db == nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	stats, err := s.db.GetDatabaseStats()
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get database stats: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}
}

// handleTransitWorker provides API endpoints for controlling the transit worker.
// GET: returns current state, including current and last run details.
// POST: with { enabled: bool } to update state, optionally { trigger: true } for manual run
// and { trigger_full_history: true } for a full-history run.
func (s *Server) handleTransitWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if transit controller is available
	if s.transitController == nil {
		s.writeJSONError(w, http.StatusServiceUnavailable, "Transit worker not available")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Return current status including last run time and error
		status := s.transitController.GetStatus()
		if err := json.NewEncoder(w).Encode(status); err != nil {
			log.Printf("failed to encode transit worker status: %v", err)
		}

	case http.MethodPost:
		// Update state or trigger manual run
		var req struct {
			Enabled            *bool `json:"enabled"`
			Trigger            bool  `json:"trigger"`
			TriggerFullHistory bool  `json:"trigger_full_history"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Update enabled state if provided
		if req.Enabled != nil {
			s.transitController.SetEnabled(*req.Enabled)
			status := "disabled"
			if *req.Enabled {
				status = "enabled"
			}
			log.Printf("Transit worker %s via API", status)
		}

		// Trigger manual run if requested
		if req.Trigger {
			s.transitController.TriggerManualRun()
			log.Printf("Transit worker manual run triggered via API")
		}

		// Trigger full-history run if requested
		if req.TriggerFullHistory {
			s.transitController.TriggerFullHistoryRun()
			log.Printf("Transit worker full-history run triggered via API")
		}

		// Return updated status
		status := s.transitController.GetStatus()
		if err := json.NewEncoder(w).Encode(status); err != nil {
			log.Printf("failed to encode transit worker response: %v", err)
		}

	default:
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
