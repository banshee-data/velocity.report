// Package monitor provides thin HTTP handler shells for LiDAR chart endpoints.
// Data preparation and types live in l9endpoints; these handlers only parse
// requests, obtain data sources, delegate to l9endpoints, and write responses.
package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// handleChartPolarJSON returns polar chart data as JSON.
func (ws *WebServer) handleChartPolarJSON(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := l3grid.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	maxPoints := 8000
	if mp := r.URL.Query().Get("max_points"); mp != "" {
		if v, err := strconv.Atoi(mp); err == nil && v > 100 && v <= 50000 {
			maxPoints = v
		}
	}

	cells := bm.GetGridCells()
	if len(cells) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no background cells available")
		return
	}

	data := l9endpoints.PreparePolarChartData(cells, sensorID, maxPoints)
	ws.writeJSON(w, http.StatusOK, data)
}

// handleChartHeatmapJSON returns heatmap chart data as JSON.
func (ws *WebServer) handleChartHeatmapJSON(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := l3grid.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	azBucketDeg := 3.0
	if v := r.URL.Query().Get("azimuth_bucket_deg"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 1.0 {
			azBucketDeg = parsed
		}
	}
	settledThreshold := uint32(5)
	if v := r.URL.Query().Get("settled_threshold"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
			settledThreshold = uint32(parsed)
		}
	}

	heatmap := bm.GetGridHeatmap(azBucketDeg, settledThreshold)
	if heatmap == nil || len(heatmap.Buckets) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no heatmap buckets available")
		return
	}

	data := l9endpoints.PrepareHeatmapFromBuckets(heatmap.Buckets, sensorID)
	ws.writeJSON(w, http.StatusOK, data)
}

// handleChartForegroundJSON returns foreground frame data as JSON.
func (ws *WebServer) handleChartForegroundJSON(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	snapshot := l3grid.GetForegroundSnapshot(sensorID)
	if snapshot == nil || (len(snapshot.ForegroundPoints) == 0 && len(snapshot.BackgroundPoints) == 0) {
		ws.writeJSONError(w, http.StatusNotFound, "no foreground snapshot available")
		return
	}

	data := l9endpoints.PrepareForegroundChartData(snapshot, sensorID)
	ws.writeJSON(w, http.StatusOK, data)
}

// handleChartClustersJSON returns cluster chart data as JSON.
func (ws *WebServer) handleChartClustersJSON(w http.ResponseWriter, r *http.Request) {
	if ws.trackAPI == nil || ws.trackAPI.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "track DB not configured")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	var startNanos, endNanos int64
	if s := r.URL.Query().Get("start"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			startNanos = parsed * 1e9
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if parsed, err := strconv.ParseInt(e, 10, 64); err == nil {
			endNanos = parsed * 1e9
		}
	}
	if endNanos == 0 {
		endNanos = time.Now().UnixNano()
	}
	if startNanos == 0 {
		startNanos = endNanos - int64(time.Hour)
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	clusters, err := sqlite.GetRecentClusters(ws.trackAPI.db, sensorID, startNanos, endNanos, limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get clusters: %v", err))
		return
	}

	data := l9endpoints.PrepareRecentClustersData(clusters, sensorID)
	ws.writeJSON(w, http.StatusOK, data)
}

// handleChartTrafficJSON returns traffic metrics as JSON.
func (ws *WebServer) handleChartTrafficJSON(w http.ResponseWriter, r *http.Request) {
	if ws.stats == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no packet stats available")
		return
	}

	snap := ws.stats.GetLatestSnapshot()
	if snap == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no traffic stats available")
		return
	}

	data := l9endpoints.PrepareTrafficMetrics(snap)
	ws.writeJSON(w, http.StatusOK, data)
}

// writeJSON writes a JSON response with the given status code.
func (ws *WebServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		fmt.Printf("JSON encoding error: %v\n", err)
	}
}
