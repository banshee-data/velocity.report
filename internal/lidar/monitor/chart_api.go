// Package monitor provides JSON API endpoints for LiDAR chart data.
// These endpoints return structured data that can be consumed by any frontend,
// decoupling data preparation from eCharts rendering.
package monitor

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// NOTE: Chart API route registration has been consolidated into
// WebServer.RegisterRoutes in webserver.go. The handler methods below
// remain on WebServer for code organisation.

// handleChartPolarJSON returns polar chart data as JSON.
// Query params:
//   - sensor_id (optional; defaults to configured sensor)
//   - max_points (optional; default 8000)
func (ws *WebServer) handleChartPolarJSON(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := lidar.GetBackgroundManager(sensorID)
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

	data := PreparePolarChartData(cells, sensorID, maxPoints)
	ws.writeJSON(w, http.StatusOK, data)
}

// handleChartHeatmapJSON returns heatmap chart data as JSON.
// Query params:
//   - sensor_id (optional; defaults to configured sensor)
//   - azimuth_bucket_deg (optional; default 3.0)
//   - settled_threshold (optional; default 5)
func (ws *WebServer) handleChartHeatmapJSON(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := lidar.GetBackgroundManager(sensorID)
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

	data := PrepareHeatmapFromBuckets(heatmap.Buckets, sensorID)
	ws.writeJSON(w, http.StatusOK, data)
}

// HeatmapBucketData represents prepared heatmap data from grid buckets.
type HeatmapBucketData struct {
	Points     []ScatterPoint `json:"points"`
	MaxValue   float64        `json:"max_value"`
	MaxAbs     float64        `json:"max_abs"`
	SensorID   string         `json:"sensor_id"`
	NumBuckets int            `json:"num_buckets"`
}

// PrepareHeatmapFromBuckets transforms grid heatmap buckets into chart-ready data.
func PrepareHeatmapFromBuckets(buckets []lidar.CoarseBucket, sensorID string) *HeatmapBucketData {
	points := make([]ScatterPoint, 0, len(buckets))
	maxVal := 0.0
	maxAbs := 0.0

	// First pass: find max value
	for _, b := range buckets {
		if b.FilledCells == 0 {
			continue
		}
		val := b.MeanTimesSeen
		if val == 0 {
			val = float64(b.SettledCells)
		}
		if val > maxVal {
			maxVal = val
		}
	}
	if maxVal == 0 {
		maxVal = 1.0
	}

	// Second pass: convert to points
	for _, b := range buckets {
		if b.FilledCells == 0 {
			continue
		}
		azMid := (b.AzimuthDegStart + b.AzimuthDegEnd) / 2.0
		rRange := b.MeanRangeMeters
		if rRange == 0 {
			rRange = (b.MinRangeMeters + b.MaxRangeMeters) / 2.0
		}

		// Convert polar to cartesian
		theta := azMid * math.Pi / 180.0
		x := rRange * math.Cos(theta)
		y := rRange * math.Sin(theta)

		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}

		points = append(points, ScatterPoint{X: x, Y: y, Value: b.MeanTimesSeen})
	}

	if maxAbs > 0 {
		maxAbs *= 1.05
	} else {
		maxAbs = 1.0
	}

	return &HeatmapBucketData{
		Points:     points,
		MaxValue:   maxVal,
		MaxAbs:     maxAbs,
		SensorID:   sensorID,
		NumBuckets: len(points),
	}
}

// ForegroundChartData holds foreground/background point data for charting.
type ForegroundChartData struct {
	ForegroundPoints  []ScatterPoint `json:"foreground_points"`
	BackgroundPoints  []ScatterPoint `json:"background_points"`
	MaxAbs            float64        `json:"max_abs"`
	ForegroundCount   int            `json:"foreground_count"`
	BackgroundCount   int            `json:"background_count"`
	TotalPoints       int            `json:"total_points"`
	ForegroundPercent float64        `json:"foreground_percent"`
	Timestamp         string         `json:"timestamp"`
	SensorID          string         `json:"sensor_id"`
}

// handleChartForegroundJSON returns foreground frame data as JSON.
func (ws *WebServer) handleChartForegroundJSON(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	snapshot := lidar.GetForegroundSnapshot(sensorID)
	if snapshot == nil || (len(snapshot.ForegroundPoints) == 0 && len(snapshot.BackgroundPoints) == 0) {
		ws.writeJSONError(w, http.StatusNotFound, "no foreground snapshot available")
		return
	}

	data := PrepareForegroundChartData(snapshot, sensorID)
	ws.writeJSON(w, http.StatusOK, data)
}

// PrepareForegroundChartData transforms a foreground snapshot into chart-ready data.
func PrepareForegroundChartData(snapshot *lidar.ForegroundSnapshot, sensorID string) *ForegroundChartData {
	fgPts := make([]ScatterPoint, 0, len(snapshot.ForegroundPoints))
	bgPts := make([]ScatterPoint, 0, len(snapshot.BackgroundPoints))
	maxAbs := 0.0

	for _, p := range snapshot.BackgroundPoints {
		if math.Abs(p.X) > maxAbs {
			maxAbs = math.Abs(p.X)
		}
		if math.Abs(p.Y) > maxAbs {
			maxAbs = math.Abs(p.Y)
		}
		bgPts = append(bgPts, ScatterPoint{X: p.X, Y: p.Y, Value: 0})
	}

	for _, p := range snapshot.ForegroundPoints {
		if math.Abs(p.X) > maxAbs {
			maxAbs = math.Abs(p.X)
		}
		if math.Abs(p.Y) > maxAbs {
			maxAbs = math.Abs(p.Y)
		}
		fgPts = append(fgPts, ScatterPoint{X: p.X, Y: p.Y, Value: 1})
	}

	if maxAbs > 0 {
		maxAbs *= 1.05
	} else {
		maxAbs = 1.0
	}

	fgPercent := 0.0
	if snapshot.TotalPoints > 0 {
		fgPercent = float64(snapshot.ForegroundCount) / float64(snapshot.TotalPoints) * 100
	}

	return &ForegroundChartData{
		ForegroundPoints:  fgPts,
		BackgroundPoints:  bgPts,
		MaxAbs:            maxAbs,
		ForegroundCount:   snapshot.ForegroundCount,
		BackgroundCount:   snapshot.BackgroundCount,
		TotalPoints:       snapshot.TotalPoints,
		ForegroundPercent: fgPercent,
		Timestamp:         snapshot.Timestamp.UTC().Format(time.RFC3339),
		SensorID:          sensorID,
	}
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

	clusters, err := lidar.GetRecentClusters(ws.trackAPI.db, sensorID, startNanos, endNanos, limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get clusters: %v", err))
		return
	}

	data := PrepareRecentClustersData(clusters, sensorID)
	ws.writeJSON(w, http.StatusOK, data)
}

// RecentClustersData holds cluster centroid data for charting.
type RecentClustersData struct {
	Clusters    []ClusterCentroid `json:"clusters"`
	MaxAbs      float64           `json:"max_abs"`
	MaxPoints   int               `json:"max_points"`
	NumClusters int               `json:"num_clusters"`
	SensorID    string            `json:"sensor_id"`
}

// ClusterCentroid represents a cluster's centroid position and size.
type ClusterCentroid struct {
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	PointsCount int     `json:"points_count"`
}

// PrepareRecentClustersData transforms cluster records into chart-ready data.
func PrepareRecentClustersData(clusters []*lidar.WorldCluster, sensorID string) *RecentClustersData {
	pts := make([]ClusterCentroid, 0, len(clusters))
	maxPts := 1
	maxAbs := 0.0

	for _, c := range clusters {
		if c.PointsCount > maxPts {
			maxPts = c.PointsCount
		}
	}

	for _, c := range clusters {
		x := float64(c.CentroidX)
		y := float64(c.CentroidY)
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		pts = append(pts, ClusterCentroid{X: x, Y: y, PointsCount: c.PointsCount})
	}

	if maxAbs > 0 {
		maxAbs *= 1.05
	} else {
		maxAbs = 1.0
	}

	return &RecentClustersData{
		Clusters:    pts,
		MaxAbs:      maxAbs,
		MaxPoints:   maxPts,
		NumClusters: len(clusters),
		SensorID:    sensorID,
	}
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

	data := PrepareTrafficMetrics(snap)
	ws.writeJSON(w, http.StatusOK, data)
}

// writeJSON writes a JSON response with the given status code.
func (ws *WebServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Log encoding error but response is already started
		fmt.Printf("JSON encoding error: %v\n", err)
	}
}
