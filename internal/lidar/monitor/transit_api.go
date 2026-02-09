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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	sensorID := query.Get("sensor_id")

	// Parse time range
	var startUnix, endUnix float64
	if s := query.Get("start"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			startUnix = v
		}
	}
	if e := query.Get("end"); e != "" {
		if v, err := strconv.ParseFloat(e, 64); err == nil {
			endUnix = v
		}
	}

	// Parse speed filters
	var minSpeed, maxSpeed float32
	if ms := query.Get("min_speed"); ms != "" {
		if v, err := strconv.ParseFloat(ms, 32); err == nil {
			minSpeed = float32(v)
		}
	}
	if ms := query.Get("max_speed"); ms != "" {
		if v, err := strconv.ParseFloat(ms, 32); err == nil {
			maxSpeed = float32(v)
		}
	}

	// Parse limit
	limit := 100 // Default limit
	if l := query.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	// Query database
	if ws.db == nil {
		http.Error(w, "Database not configured", http.StatusInternalServerError)
		return
	}

	store := lidar.NewTransitStore(ws.db.DB)
	transits, err := store.ListTransits(sensorID, startUnix, endUnix, minSpeed, maxSpeed, limit)
	if err != nil {
		log.Printf("Error listing transits: %v", err)
		http.Error(w, "Failed to list transits", http.StatusInternalServerError)
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	sensorID := query.Get("sensor_id")

	// Parse time range
	var startUnix, endUnix float64
	if s := query.Get("start"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			startUnix = v
		}
	}
	if e := query.Get("end"); e != "" {
		if v, err := strconv.ParseFloat(e, 64); err == nil {
			endUnix = v
		}
	}

	// Query database
	if ws.db == nil {
		http.Error(w, "Database not configured", http.StatusInternalServerError)
		return
	}

	store := lidar.NewTransitStore(ws.db.DB)
	summary, err := store.GetTransitSummary(sensorID, startUnix, endUnix)
	if err != nil {
		log.Printf("Error getting transit summary: %v", err)
		http.Error(w, "Failed to get transit summary", http.StatusInternalServerError)
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		log.Printf("Error encoding transit summary response: %v", err)
	}
}
