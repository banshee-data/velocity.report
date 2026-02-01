package monitor

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

func TestPrepareHeatmapFromBuckets(t *testing.T) {
	tests := []struct {
		name       string
		buckets    []lidar.CoarseBucket
		sensorID   string
		wantPoints int
		wantMaxVal float64
	}{
		{
			name:       "empty buckets",
			buckets:    []lidar.CoarseBucket{},
			sensorID:   "sensor-001",
			wantPoints: 0,
			wantMaxVal: 1.0,
		},
		{
			name: "single bucket",
			buckets: []lidar.CoarseBucket{
				{
					Ring:            0,
					AzimuthDegStart: 0,
					AzimuthDegEnd:   3,
					FilledCells:     10,
					MeanTimesSeen:   5.0,
					MeanRangeMeters: 10.0,
				},
			},
			sensorID:   "sensor-001",
			wantPoints: 1,
			wantMaxVal: 5.0,
		},
		{
			name: "bucket with zero filled cells skipped",
			buckets: []lidar.CoarseBucket{
				{FilledCells: 0, MeanTimesSeen: 5.0},
				{FilledCells: 5, MeanTimesSeen: 10.0, MeanRangeMeters: 5.0},
			},
			sensorID:   "sensor-001",
			wantPoints: 1,
			wantMaxVal: 10.0,
		},
		{
			name: "uses settled cells when mean times seen is zero",
			buckets: []lidar.CoarseBucket{
				{FilledCells: 5, MeanTimesSeen: 0, SettledCells: 3, MeanRangeMeters: 5.0},
			},
			sensorID:   "sensor-001",
			wantPoints: 1,
			wantMaxVal: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrepareHeatmapFromBuckets(tt.buckets, tt.sensorID)
			if result == nil {
				t.Fatal("PrepareHeatmapFromBuckets returned nil")
			}
			if len(result.Points) != tt.wantPoints {
				t.Errorf("got %d points, want %d", len(result.Points), tt.wantPoints)
			}
			if result.MaxValue != tt.wantMaxVal {
				t.Errorf("got max value %v, want %v", result.MaxValue, tt.wantMaxVal)
			}
			if result.SensorID != tt.sensorID {
				t.Errorf("got sensor ID %q, want %q", result.SensorID, tt.sensorID)
			}
		})
	}
}

func TestPrepareHeatmapFromBuckets_PolarToCartesian(t *testing.T) {
	// Test that polar coordinates are correctly converted to cartesian
	buckets := []lidar.CoarseBucket{
		{
			AzimuthDegStart: 0,
			AzimuthDegEnd:   6,
			FilledCells:     1,
			MeanTimesSeen:   1.0,
			MeanRangeMeters: 10.0,
		},
	}

	result := PrepareHeatmapFromBuckets(buckets, "sensor-001")
	if len(result.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result.Points))
	}

	// At azimuth 3 degrees (midpoint), range 10m:
	// x = 10 * cos(3°) ≈ 9.986
	// y = 10 * sin(3°) ≈ 0.523
	p := result.Points[0]
	expectedX := 10.0 * math.Cos(3*math.Pi/180)
	expectedY := 10.0 * math.Sin(3*math.Pi/180)

	if math.Abs(p.X-expectedX) > 0.001 {
		t.Errorf("X coordinate: got %v, want %v", p.X, expectedX)
	}
	if math.Abs(p.Y-expectedY) > 0.001 {
		t.Errorf("Y coordinate: got %v, want %v", p.Y, expectedY)
	}
}

func TestPrepareForegroundChartData(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *lidar.ForegroundSnapshot
		sensorID string
		wantFg   int
		wantBg   int
	}{
		{
			name: "mixed foreground and background",
			snapshot: &lidar.ForegroundSnapshot{
				ForegroundPoints: []lidar.ProjectedPoint{
					{X: 1.0, Y: 2.0},
					{X: 3.0, Y: 4.0},
				},
				BackgroundPoints: []lidar.ProjectedPoint{
					{X: 5.0, Y: 6.0},
				},
				ForegroundCount: 2,
				BackgroundCount: 1,
				TotalPoints:     3,
				Timestamp:       time.Now(),
			},
			sensorID: "sensor-001",
			wantFg:   2,
			wantBg:   1,
		},
		{
			name: "empty snapshot",
			snapshot: &lidar.ForegroundSnapshot{
				ForegroundPoints: []lidar.ProjectedPoint{},
				BackgroundPoints: []lidar.ProjectedPoint{},
				Timestamp:        time.Now(),
			},
			sensorID: "sensor-001",
			wantFg:   0,
			wantBg:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrepareForegroundChartData(tt.snapshot, tt.sensorID)
			if result == nil {
				t.Fatal("PrepareForegroundChartData returned nil")
			}
			if len(result.ForegroundPoints) != tt.wantFg {
				t.Errorf("got %d foreground points, want %d", len(result.ForegroundPoints), tt.wantFg)
			}
			if len(result.BackgroundPoints) != tt.wantBg {
				t.Errorf("got %d background points, want %d", len(result.BackgroundPoints), tt.wantBg)
			}
			if result.SensorID != tt.sensorID {
				t.Errorf("got sensor ID %q, want %q", result.SensorID, tt.sensorID)
			}
		})
	}
}

func TestPrepareForegroundChartData_MaxAbs(t *testing.T) {
	snapshot := &lidar.ForegroundSnapshot{
		ForegroundPoints: []lidar.ProjectedPoint{
			{X: 10.0, Y: 5.0},
			{X: -15.0, Y: 3.0},
		},
		BackgroundPoints: []lidar.ProjectedPoint{
			{X: 2.0, Y: -20.0},
		},
		ForegroundCount: 2,
		BackgroundCount: 1,
		TotalPoints:     3,
		Timestamp:       time.Now(),
	}

	result := PrepareForegroundChartData(snapshot, "sensor-001")

	// Max abs should be 20 (from Y=-20) * 1.05 = 21
	expectedMaxAbs := 20.0 * 1.05
	if math.Abs(result.MaxAbs-expectedMaxAbs) > 0.001 {
		t.Errorf("got MaxAbs %v, want %v", result.MaxAbs, expectedMaxAbs)
	}
}

func TestPrepareForegroundChartData_ForegroundPercent(t *testing.T) {
	snapshot := &lidar.ForegroundSnapshot{
		ForegroundPoints: []lidar.ProjectedPoint{{X: 1, Y: 1}},
		BackgroundPoints: []lidar.ProjectedPoint{{X: 2, Y: 2}, {X: 3, Y: 3}},
		ForegroundCount:  1,
		BackgroundCount:  2,
		TotalPoints:      3,
		Timestamp:        time.Now(),
	}

	result := PrepareForegroundChartData(snapshot, "sensor-001")

	expectedPercent := (1.0 / 3.0) * 100
	if math.Abs(result.ForegroundPercent-expectedPercent) > 0.001 {
		t.Errorf("got ForegroundPercent %v, want %v", result.ForegroundPercent, expectedPercent)
	}
}

func TestPrepareRecentClustersData(t *testing.T) {
	tests := []struct {
		name       string
		clusters   []*lidar.WorldCluster
		sensorID   string
		wantNum    int
		wantMaxPts int
	}{
		{
			name:       "empty clusters",
			clusters:   []*lidar.WorldCluster{},
			sensorID:   "sensor-001",
			wantNum:    0,
			wantMaxPts: 1,
		},
		{
			name: "multiple clusters",
			clusters: []*lidar.WorldCluster{
				{CentroidX: 1.0, CentroidY: 2.0, PointsCount: 5},
				{CentroidX: 3.0, CentroidY: 4.0, PointsCount: 10},
				{CentroidX: 5.0, CentroidY: 6.0, PointsCount: 3},
			},
			sensorID:   "sensor-001",
			wantNum:    3,
			wantMaxPts: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrepareRecentClustersData(tt.clusters, tt.sensorID)
			if result == nil {
				t.Fatal("PrepareRecentClustersData returned nil")
			}
			if result.NumClusters != tt.wantNum {
				t.Errorf("got %d clusters, want %d", result.NumClusters, tt.wantNum)
			}
			if result.MaxPoints != tt.wantMaxPts {
				t.Errorf("got max points %d, want %d", result.MaxPoints, tt.wantMaxPts)
			}
			if result.SensorID != tt.sensorID {
				t.Errorf("got sensor ID %q, want %q", result.SensorID, tt.sensorID)
			}
		})
	}
}

func TestPrepareRecentClustersData_MaxAbs(t *testing.T) {
	clusters := []*lidar.WorldCluster{
		{CentroidX: 10.0, CentroidY: 5.0, PointsCount: 1},
		{CentroidX: -25.0, CentroidY: 3.0, PointsCount: 1},
	}

	result := PrepareRecentClustersData(clusters, "sensor-001")

	// Max abs should be 25 * 1.05 = 26.25
	expectedMaxAbs := 25.0 * 1.05
	if math.Abs(result.MaxAbs-expectedMaxAbs) > 0.001 {
		t.Errorf("got MaxAbs %v, want %v", result.MaxAbs, expectedMaxAbs)
	}
}

func TestWebServer_WriteJSON(t *testing.T) {
	ws := &WebServer{}

	data := map[string]interface{}{
		"key": "value",
		"num": 42,
	}

	rec := httptest.NewRecorder()
	ws.writeJSON(rec, http.StatusOK, data)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("got Content-Type %q, want %q", contentType, "application/json")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("got key=%v, want 'value'", result["key"])
	}
}

func TestHandleChartTrafficJSON_NoStats(t *testing.T) {
	ws := &WebServer{stats: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/traffic", nil)
	rec := httptest.NewRecorder()

	ws.handleChartTrafficJSON(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleChartPolarJSON_NoBackgroundManager(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/polar", nil)
	rec := httptest.NewRecorder()

	ws.handleChartPolarJSON(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleChartHeatmapJSON_NoBackgroundManager(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/heatmap", nil)
	rec := httptest.NewRecorder()

	ws.handleChartHeatmapJSON(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleChartForegroundJSON_NoSnapshot(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/foreground", nil)
	rec := httptest.NewRecorder()

	ws.handleChartForegroundJSON(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleChartClustersJSON_NoTrackAPI(t *testing.T) {
	ws := &WebServer{trackAPI: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/clusters", nil)
	rec := httptest.NewRecorder()

	ws.handleChartClustersJSON(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
func TestHandleChartPolarJSON_WithBackgroundManager(t *testing.T) {
	sensorID := "test-polar-" + time.Now().Format("150405")
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	ws := &WebServer{sensorID: sensorID}

	// Verify background manager is registered
	bm := lidar.GetBackgroundManager(sensorID)
	if bm == nil {
		t.Error("Expected non-nil background manager")
		return
	}
	if bm.Grid == nil {
		t.Error("Expected non-nil grid in background manager")
		return
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/polar", nil)
	rec := httptest.NewRecorder()

	ws.handleChartPolarJSON(rec, req)

	// Accept 200 OK if cells exist, or 404 if grid is empty
	if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 200 or 404, got %d", rec.Code)
	}
}

func TestHandleChartPolarJSON_CustomMaxPoints(t *testing.T) {
	sensorID := "test-polar-maxpts-" + time.Now().Format("150405")
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	ws := &WebServer{sensorID: sensorID}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/polar?max_points=500", nil)
	rec := httptest.NewRecorder()

	ws.handleChartPolarJSON(rec, req)

	// Accept 200 OK if cells exist, or 404 if grid is empty
	if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 200 or 404, got %d", rec.Code)
	}
}

func TestHandleChartHeatmapJSON_WithParams(t *testing.T) {
	sensorID := "test-heatmap-params-" + time.Now().Format("150405")
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	ws := &WebServer{sensorID: sensorID}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/heatmap?azimuth_bucket_deg=6&settled_threshold=10", nil)
	rec := httptest.NewRecorder()

	ws.handleChartHeatmapJSON(rec, req)

	// Should accept custom parameters
	if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want OK or NotFound", rec.Code)
	}
}

func TestHandleChartHeatmapJSON_WithSensorID(t *testing.T) {
	sensorID := "test-heatmap-sensor-" + time.Now().Format("150405")
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	ws := &WebServer{sensorID: "other-sensor"}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/heatmap?sensor_id="+sensorID, nil)
	rec := httptest.NewRecorder()

	ws.handleChartHeatmapJSON(rec, req)

	// Should use sensor_id from query param
	if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want OK or NotFound", rec.Code)
	}
}

func TestHandleChartTrafficJSON_WithStats(t *testing.T) {
	stats := NewPacketStats()
	stats.AddPacket(1000)
	stats.AddPoints(100)
	stats.LogStats(true) // Create a snapshot

	ws := &WebServer{stats: stats}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/traffic", nil)
	rec := httptest.NewRecorder()

	ws.handleChartTrafficJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Should have expected fields
	if _, ok := result["packets_per_sec"]; !ok {
		t.Error("expected 'packets_per_sec' field in response")
	}
}

func TestHandleChartTrafficJSON_NoSnapshot(t *testing.T) {
	stats := NewPacketStats()
	// Don't call LogStats, so no snapshot exists

	ws := &WebServer{stats: stats}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/traffic", nil)
	rec := httptest.NewRecorder()

	ws.handleChartTrafficJSON(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestWriteJSONError(t *testing.T) {
	ws := &WebServer{}

	rec := httptest.NewRecorder()
	ws.writeJSONError(rec, http.StatusBadRequest, "test error message")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if msg, ok := result["error"].(string); !ok || msg != "test error message" {
		t.Errorf("got error=%v, want 'test error message'", result["error"])
	}
}

func TestHandleChartClustersJSON_WithQueryParams(t *testing.T) {
	ws := &WebServer{
		sensorID: "test-sensor",
		trackAPI: nil, // Will return service unavailable
	}

	// Test with various query parameters
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/chart/clusters?start=1000&end=2000&limit=50", nil)
	rec := httptest.NewRecorder()

	ws.handleChartClustersJSON(rec, req)

	// Should fail because trackAPI is nil, but should parse params first
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
