package monitor

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// handleListTransits handles GET /api/lidar/transits
// Query parameters: sensor_id, start, end, min_speed, max_speed, limit
func (ws *WebServer) handleListTransits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	query := r.URL.Query()
	sensorID := query.Get("sensor_id")

	// Parse time range
	var startUnix, endUnix float64
	if s := query.Get("start"); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid 'start' query parameter: must be a Unix timestamp in seconds")
			return
		}
		startUnix = v
	}
	if e := query.Get("end"); e != "" {
		v, err := strconv.ParseFloat(e, 64)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid 'end' query parameter: must be a Unix timestamp in seconds")
			return
		}
		endUnix = v
	}

	// Parse speed filters
	var minSpeed, maxSpeed float32
	if ms := query.Get("min_speed"); ms != "" {
		v, err := strconv.ParseFloat(ms, 32)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid 'min_speed' query parameter: must be a number")
			return
		}
		minSpeed = float32(v)
	}
	if ms := query.Get("max_speed"); ms != "" {
		v, err := strconv.ParseFloat(ms, 32)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid 'max_speed' query parameter: must be a number")
			return
		}
		maxSpeed = float32(v)
	}

	// Parse limit
	limit := 100 // Default limit
	if l := query.Get("limit"); l != "" {
		v, err := strconv.Atoi(l)
		if err != nil || v <= 0 {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid 'limit' query parameter: must be a positive integer")
			return
		}
		limit = v
	}

	store := lidar.NewTransitStore(ws.db.DB)
	transits, err := store.ListTransits(sensorID, startUnix, endUnix, minSpeed, maxSpeed, limit)
	if err != nil {
		log.Printf("Error listing transits: %v", err)
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to list transits")
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(transits); err != nil {
		log.Printf("Error encoding transits response: %v", err)
	}
}

// handleTransitSummary handles GET /api/lidar/transits/summary
// Query parameters: sensor_id, start, end
func (ws *WebServer) handleTransitSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	query := r.URL.Query()
	sensorID := query.Get("sensor_id")

	// Parse time range
	var startUnix, endUnix float64
	if s := query.Get("start"); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid 'start' query parameter: must be a Unix timestamp in seconds")
			return
		}
		startUnix = v
	}
	if e := query.Get("end"); e != "" {
		v, err := strconv.ParseFloat(e, 64)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid 'end' query parameter: must be a Unix timestamp in seconds")
			return
		}
		endUnix = v
	}

	store := lidar.NewTransitStore(ws.db.DB)
	summary, err := store.GetTransitSummary(sensorID, startUnix, endUnix)
	if err != nil {
		log.Printf("Error getting transit summary: %v", err)
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to get transit summary")
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		log.Printf("Error encoding transit summary response: %v", err)
	}
}
