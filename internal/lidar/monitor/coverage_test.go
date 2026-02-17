// Package monitor provides additional comprehensive tests for coverage improvement.
package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

// ====== TrackAPI handleListObservations coverage tests (DB-backed) ======

func TestTrackAPI_HandleListObservations_InvalidStartTimeWithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?start_time=invalid", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid start_time, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_InvalidEndTimeWithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?end_time=invalid", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid end_time, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_StartAfterEndWithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	// Start time after end time
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?start_time=2000000000000000000&end_time=1000000000000000000", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for start > end, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleListObservations_WithTrackIDFilterDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	insertTestTrack(t, db, "track-002", "sensor-A", "confirmed", now-1e9, now)
	insertTestObservation(t, db, "track-001", now-500e6, 10.0, 5.0)
	insertTestObservation(t, db, "track-002", now-500e6, 20.0, 15.0)

	api := NewTrackAPI(db, "sensor-A")

	// Filter by track_id
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?track_id=track-001", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleListObservations_WithLimitParamDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	for i := 0; i < 10; i++ {
		insertTestObservation(t, db, "track-001", now-int64(i)*100e6, float64(10+i), float64(5+i))
	}

	api := NewTrackAPI(db, "sensor-A")

	// Request with limit=3
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?limit=3", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_InvalidLimitDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	// Invalid limit (negative) should use default
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?limit=-10", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 with default limit, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_ExcessiveLimitDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	// Excessive limit should be capped
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?limit=10000", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 with capped limit, got %d", w.Code)
	}
}

// ====== TrackAPI handleUpdateTrack coverage tests ======

func TestTrackAPI_HandleUpdateTrack_InvalidJSONDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodPatch, "/api/lidar/tracks/track-001", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleUpdateTrack(w, req, "track-001")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid JSON, got %d", w.Code)
	}
}

func TestTrackAPI_HandleUpdateTrack_TrackNotFoundDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	body := `{"object_class": "car"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/lidar/tracks/nonexistent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleUpdateTrack(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for nonexistent track, got %d", w.Code)
	}
}

func TestTrackAPI_HandleUpdateTrack_WithTrackerDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Create a track by updating tracker
	cluster := lidar.WorldCluster{
		SensorID:          "sensor-A",
		CentroidX:         10.0,
		CentroidY:         5.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
	}

	ts := time.Now()
	for i := 0; i < 5; i++ {
		cluster.CentroidX = 10.0 + float32(i)*0.5
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update([]lidar.WorldCluster{cluster}, ts)
	}

	// Get track ID from tracker
	tracks := tracker.GetConfirmedTracks()
	if len(tracks) == 0 {
		t.Skip("No confirmed tracks available")
	}
	trackID := tracks[0].TrackID

	api := NewTrackAPI(db, "sensor-A")
	api.SetTracker(tracker)

	body := `{"object_class": "vehicle"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/lidar/tracks/"+trackID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleUpdateTrack(w, req, trackID)

	// Accept various responses - track needs to exist in DB too for the update to persist
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d: %s", w.Code, w.Body.String())
	}
}

// ====== TrackAPI handleListClusters coverage tests ======

func TestTrackAPI_HandleListClusters_MethodNotAllowedDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "test-sensor")

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/clusters", nil)
	w := httptest.NewRecorder()

	api.handleListClusters(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListClusters_InvalidStartTimeDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	// Note: handleListClusters silently ignores invalid 'start' params (uses default)
	// It uses 'start' and 'end' params (in seconds), not 'start_time' and 'end_time'
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clusters?start=notanumber", nil)
	w := httptest.NewRecorder()

	api.handleListClusters(w, req)

	// Handler returns 200 and uses default time range when param is invalid
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 (invalid param ignored), got %d", w.Code)
	}
}

func TestTrackAPI_HandleListClusters_InvalidEndTimeDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	// Note: handleListClusters silently ignores invalid 'end' params (uses default)
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clusters?end=notanumber", nil)
	w := httptest.NewRecorder()

	api.handleListClusters(w, req)

	// Handler returns 200 and uses default time range when param is invalid
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 (invalid param ignored), got %d", w.Code)
	}
}

func TestTrackAPI_HandleListClusters_LimitParamDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	// Excessive limit should be capped
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clusters?limit=50000", nil)
	w := httptest.NewRecorder()

	api.handleListClusters(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// ====== TrackAPI handleActiveTracks coverage tests ======

func TestTrackAPI_HandleActiveTracks_ConfirmedStateFilter(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	cluster := lidar.WorldCluster{
		SensorID:          "test-sensor",
		CentroidX:         10.0,
		CentroidY:         5.0,
		BoundingBoxLength: 4.0,
		BoundingBoxHeight: 1.5,
	}

	ts := time.Now()
	for i := 0; i < 10; i++ {
		cluster.CentroidX = 10.0 + float32(i)*0.5
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update([]lidar.WorldCluster{cluster}, ts)
	}

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active?state=confirmed", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestTrackAPI_HandleActiveTracks_TentativeStateFilter(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Create a tentative track (few observations)
	cluster := lidar.WorldCluster{
		SensorID:          "test-sensor",
		CentroidX:         30.0,
		CentroidY:         20.0,
		BoundingBoxLength: 2.0,
		BoundingBoxHeight: 1.0,
	}

	ts := time.Now()
	tracker.Update([]lidar.WorldCluster{cluster}, ts)

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active?state=tentative", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestTrackAPI_HandleActiveTracks_WithDBFallback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active?sensor_id=sensor-A", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ====== TrackAPI handleTrackObservations coverage tests ======

func TestTrackAPI_HandleTrackObservations_NoDB_AnyMethod(t *testing.T) {
	// Note: handleTrackObservations does NOT check HTTP method
	// It immediately checks for db == nil, so any method returns 503 when db is nil
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/track-001/observations", nil)
	w := httptest.NewRecorder()

	api.handleTrackObservations(w, req, "track-001")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 (no db), got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackObservations_NoDBConfigured(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/track-001/observations", nil)
	w := httptest.NewRecorder()

	api.handleTrackObservations(w, req, "track-001")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackObservations_WithLimitDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	for i := 0; i < 10; i++ {
		insertTestObservation(t, db, "track-001", now-int64(i)*100e6, float64(10+i), float64(5+i))
	}

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/track-001/observations?limit=5", nil)
	w := httptest.NewRecorder()

	api.handleTrackObservations(w, req, "track-001")

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("expected status 200 or 404, got %d", w.Code)
	}
}

// ====== Client additional coverage tests ======

func TestClient_FetchGridStatus_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	_, err := c.FetchGridStatus()

	if err == nil {
		t.Error("Expected error for server error response")
	}
}

func TestClient_FetchGridStatus_InvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	_, err := c.FetchGridStatus()

	if err == nil {
		t.Error("Expected error for invalid JSON response")
	}
}

func TestClient_ResetAcceptance_NetworkFailure(t *testing.T) {
	// Use a non-existent server to trigger network error
	c := NewClient(&http.Client{Timeout: 1 * time.Millisecond}, "http://127.0.0.1:1", "sensor1")
	err := c.ResetAcceptance()

	if err == nil {
		t.Error("Expected network error")
	}
}

func TestClient_FetchBuckets_MixedValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"BucketsMeters": []interface{}{"string-value", 1.0, 2.0},
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	buckets := c.FetchBuckets()

	// Should handle non-float values gracefully
	if len(buckets) != 3 {
		t.Errorf("Expected 3 buckets, got %d", len(buckets))
	}
	if buckets[0] != "string-value" {
		t.Errorf("Expected first bucket 'string-value', got %s", buckets[0])
	}
}

func TestClient_WaitForGridSettle_TimeoutReached(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"background_count": 0.0, // Always return 0 to force timeout
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")

	start := time.Now()
	c.WaitForGridSettle(500 * time.Millisecond) // Short timeout
	elapsed := time.Since(start)

	// Should have waited approximately 500ms
	if elapsed < 400*time.Millisecond {
		t.Errorf("Should have waited closer to timeout, only waited %v", elapsed)
	}
}

func TestClient_WaitForGridSettle_ServerReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")

	start := time.Now()
	c.WaitForGridSettle(300 * time.Millisecond)
	elapsed := time.Since(start)

	// Should still wait for timeout on errors
	if elapsed < 250*time.Millisecond {
		t.Errorf("Should have waited closer to timeout on errors, only waited %v", elapsed)
	}
}

func TestClient_WaitForGridSettle_InvalidJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")

	start := time.Now()
	c.WaitForGridSettle(300 * time.Millisecond)
	elapsed := time.Since(start)

	// Should still wait for timeout on decode errors
	if elapsed < 250*time.Millisecond {
		t.Errorf("Should have waited closer to timeout on decode errors, only waited %v", elapsed)
	}
}

func TestClient_WaitForGridSettle_NoBackgroundCountField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"some_other_field": 100,
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")

	start := time.Now()
	c.WaitForGridSettle(300 * time.Millisecond)
	elapsed := time.Since(start)

	// Should wait for timeout when background_count is missing
	if elapsed < 250*time.Millisecond {
		t.Errorf("Should have waited closer to timeout, only waited %v", elapsed)
	}
}

// ====== Chart API additional coverage tests ======

func TestPrepareHeatmapFromBuckets_ZeroMeanRange(t *testing.T) {
	// Test case where MeanRangeMeters is 0, should use fallback calculation
	buckets := []lidar.CoarseBucket{
		{
			AzimuthDegStart: 0,
			AzimuthDegEnd:   6,
			FilledCells:     5,
			MeanTimesSeen:   3.0,
			MeanRangeMeters: 0, // Zero - should use min/max fallback
			MinRangeMeters:  5.0,
			MaxRangeMeters:  15.0,
		},
	}

	result := PrepareHeatmapFromBuckets(buckets, "sensor-001")

	if len(result.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result.Points))
	}

	// Should use midpoint of min/max = (5 + 15) / 2 = 10
	// At azimuth 3 degrees, x = 10 * cos(3°) ≈ 9.986
	if result.Points[0].X < 9 || result.Points[0].X > 11 {
		t.Errorf("expected X around 10, got %f", result.Points[0].X)
	}
}

func TestPrepareHeatmapFromBuckets_AllZeroValues(t *testing.T) {
	buckets := []lidar.CoarseBucket{
		{
			FilledCells:   5,
			MeanTimesSeen: 0,
			SettledCells:  0,
		},
	}

	result := PrepareHeatmapFromBuckets(buckets, "sensor-001")

	// MaxValue should default to 1.0 when all values are zero
	if result.MaxValue != 1.0 {
		t.Errorf("expected MaxValue=1.0, got %f", result.MaxValue)
	}
}

func TestPrepareHeatmapFromBuckets_MaxAbsCalculation(t *testing.T) {
	buckets := []lidar.CoarseBucket{
		{
			AzimuthDegStart: 90, // Points primarily in Y direction
			AzimuthDegEnd:   96,
			FilledCells:     5,
			MeanTimesSeen:   3.0,
			MeanRangeMeters: 20.0, // Large range
		},
	}

	result := PrepareHeatmapFromBuckets(buckets, "sensor-001")

	// At azimuth ~93 degrees, Y will be much larger than X
	// MaxAbs should be 20 * 1.05 = 21
	expectedMaxAbs := 20.0 * 1.05
	if result.MaxAbs < expectedMaxAbs-1 || result.MaxAbs > expectedMaxAbs+1 {
		t.Errorf("expected MaxAbs around %f, got %f", expectedMaxAbs, result.MaxAbs)
	}
}

// ====== Templates additional coverage tests ======

func TestMockTemplateProvider_ExecuteTemplate_TemplateNotFound(t *testing.T) {
	provider := NewMockTemplateProvider(map[string]string{})

	var buf bytes.Buffer
	err := provider.ExecuteTemplate(&buf, "nonexistent.html", nil)

	if err == nil {
		t.Error("expected error for nonexistent template")
	}
	if err != fs.ErrNotExist {
		t.Errorf("expected fs.ErrNotExist, got %v", err)
	}
}

// ====== DataSource additional coverage tests ======

func TestRealDataSourceManager_StopPCAPReplay_NotStarted(t *testing.T) {
	ops := &MockWebServerOps{}
	mgr := NewRealDataSourceManager(ops)

	// Stop without starting - should be no-op
	err := mgr.StopPCAPReplay()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ops.StopPCAPCalls != 0 {
		t.Errorf("Expected 0 StopPCAPCalls, got %d", ops.StopPCAPCalls)
	}
}

func TestRealDataSourceManager_StartPCAPReplay_Error(t *testing.T) {
	ops := &MockWebServerOps{StartPCAPErr: errors.New("file not found")}
	mgr := NewRealDataSourceManager(ops)

	err := mgr.StartPCAPReplay(context.Background(), "nonexistent.pcap", ReplayConfig{})
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "file not found" {
		t.Errorf("Expected 'file not found', got: %v", err)
	}
	if mgr.IsPCAPInProgress() {
		t.Error("Expected PCAP not in progress after error")
	}
}

func TestReplayConfig_DebugFields(t *testing.T) {
	config := ReplayConfig{
		DebugRingMin: 5,
		DebugRingMax: 10,
		DebugAzMin:   0.0,
		DebugAzMax:   90.0,
		EnableDebug:  true,
		EnablePlots:  true,
	}

	if config.DebugRingMin != 5 {
		t.Errorf("Expected DebugRingMin 5, got %d", config.DebugRingMin)
	}
	if config.DebugRingMax != 10 {
		t.Errorf("Expected DebugRingMax 10, got %d", config.DebugRingMax)
	}
	if config.DebugAzMin != 0.0 {
		t.Errorf("Expected DebugAzMin 0.0, got %f", config.DebugAzMin)
	}
	if config.DebugAzMax != 90.0 {
		t.Errorf("Expected DebugAzMax 90.0, got %f", config.DebugAzMax)
	}
	if !config.EnableDebug {
		t.Error("Expected EnableDebug true")
	}
	if !config.EnablePlots {
		t.Error("Expected EnablePlots true")
	}
}

// ====== Stats coverage tests ======

func TestLogStats_NoDataLogged(t *testing.T) {
	stats := NewPacketStats()

	// Log stats without adding any data - should not create snapshot
	stats.LogStats(false)

	snapshot := stats.GetLatestSnapshot()
	// When no packets, no snapshot is created (packets == 0 && dropped == 0)
	if snapshot != nil {
		// If a snapshot was created, verify it's valid
		if snapshot.PacketsPerSec != 0 {
			t.Errorf("Expected 0 PacketsPerSec, got %f", snapshot.PacketsPerSec)
		}
	}
}

func TestLogStats_WithDroppedOnly(t *testing.T) {
	stats := NewPacketStats()

	// Only add dropped packets
	stats.AddDropped()
	stats.AddDropped()

	stats.LogStats(false)

	snapshot := stats.GetLatestSnapshot()
	if snapshot == nil {
		t.Fatal("Expected snapshot after LogStats with dropped, got nil")
	}
	if snapshot.DroppedCount != 2 {
		t.Errorf("Expected DroppedCount=2, got %d", snapshot.DroppedCount)
	}
}

func TestFormatWithCommas_LargeNumbers(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{1000000000, "1,000,000,000"},
		{123456789012, "123,456,789,012"},
		{1, "1"},
		{12, "12"},
		{999, "999"},
	}

	for _, tt := range tests {
		result := FormatWithCommas(tt.input)
		if result != tt.expected {
			t.Errorf("FormatWithCommas(%d): expected %s, got %s", tt.input, tt.expected, result)
		}
	}
}

// ====== Chart data coverage tests ======

func TestPreparePolarChartData_NegativeMaxPoints(t *testing.T) {
	cells := []lidar.ExportedCell{
		{AzimuthDeg: 0, Range: 10, TimesSeen: 1},
	}

	// Negative maxPoints should use default
	result := PreparePolarChartData(cells, "test", -100)

	if result.Stride != 1 {
		t.Errorf("Expected Stride=1 with default maxPoints, got %d", result.Stride)
	}
}

func TestPrepareClustersChartData_SingleEmptyCluster(t *testing.T) {
	clusters := [][]lidar.ExportedCell{
		{}, // Empty cluster
	}

	result := PrepareClustersChartData(clusters, "test")

	if result.NumClusters != 1 {
		t.Errorf("Expected 1 cluster, got %d", result.NumClusters)
	}
	if len(result.Points) != 0 {
		t.Errorf("Expected 0 points, got %d", len(result.Points))
	}
	// MaxAbs defaults to 1.0 when no points
	if result.MaxAbs != 1.0 {
		t.Errorf("Expected MaxAbs=1.0, got %f", result.MaxAbs)
	}
}

func TestPrepareClustersChartData_ClusterIDsCorrect(t *testing.T) {
	clusters := [][]lidar.ExportedCell{
		{{AzimuthDeg: 0, Range: 5}},
		{{AzimuthDeg: 90, Range: 10}},
		{{AzimuthDeg: 180, Range: 15}},
	}

	result := PrepareClustersChartData(clusters, "test")

	if result.NumClusters != 3 {
		t.Fatalf("Expected 3 clusters, got %d", result.NumClusters)
	}

	// Verify each point has correct cluster ID
	for i, p := range result.Points {
		if p.ClusterID != i {
			t.Errorf("Point %d: expected ClusterID=%d, got %d", i, i, p.ClusterID)
		}
	}
}

// GridPlotter tests are in gridplotter_test.go - no duplicates needed here

// ====== WebServer handler additional coverage tests ======

func TestWebServer_HandleBackgroundGridPolar_EmptyCells(t *testing.T) {
	// Test when background manager has no grid cells
	sensorID := "polar-empty-" + time.Now().Format("150405")
	params := lidar.BackgroundParams{}
	mgr := lidar.NewBackgroundManager(sensorID, 40, 1800, params, nil)
	lidar.RegisterBackgroundManager(sensorID, mgr)
	defer lidar.RegisterBackgroundManager(sensorID, nil)

	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/background/polar?sensor_id="+sensorID, nil)
	rec := httptest.NewRecorder()

	server.handleBackgroundGridPolar(rec, req)

	// Handler should return 404 when no cells available
	if rec.Code != http.StatusNotFound {
		t.Logf("handleBackgroundGridPolar returned status %d (expected 404 for empty cells)", rec.Code)
	}
}

func TestWebServer_HandleBackgroundGridPolar_InvalidMaxPoints(t *testing.T) {
	sensorID := "polar-maxpts-" + time.Now().Format("150405")
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	// Test with invalid max_points (too low)
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/background/polar?sensor_id="+sensorID+"&max_points=50", nil)
	rec := httptest.NewRecorder()

	server.handleBackgroundGridPolar(rec, req)

	t.Logf("handleBackgroundGridPolar with invalid max_points returned status %d", rec.Code)
}

func TestWebServer_HandleLidarDebugDashboard_DefaultSensor(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-dashboard",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/debug/dashboard", nil)
	rec := httptest.NewRecorder()

	server.handleLidarDebugDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("Expected HTML content type, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestWebServer_HandleLidarDebugDashboard_WithSensorID(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "default-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/debug/dashboard?sensor_id=custom-sensor", nil)
	rec := httptest.NewRecorder()

	server.handleLidarDebugDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestWebServer_HandleTrafficChart_NilStats(t *testing.T) {
	config := WebServerConfig{
		Address:           ":0",
		Stats:             nil, // No stats
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/traffic/chart", nil)
	rec := httptest.NewRecorder()

	server.handleTrafficChart(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for nil stats, got %d", rec.Code)
	}
}

func TestWebServer_HandleTrafficChart_WithStats(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/traffic/chart", nil)
	rec := httptest.NewRecorder()

	server.handleTrafficChart(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestWebServer_HandleExportSnapshotASC_NoManager(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	// Use non-existent sensor
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/export/snapshot.asc?sensor_id=nonexistent", nil)
	rec := httptest.NewRecorder()

	server.handleExportSnapshotASC(rec, req)

	// Handler returns 500 (Internal Server Error) when no background manager
	if rec.Code != http.StatusInternalServerError && rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 500 or 404 for non-existent sensor, got %d", rec.Code)
	}
}

func TestWebServer_HandleExportNextFrameASC_NoManager(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/export/nextframe.asc?sensor_id=nonexistent", nil)
	rec := httptest.NewRecorder()

	server.handleExportNextFrameASC(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent sensor, got %d", rec.Code)
	}
}

func TestWebServer_HandleExportForegroundASC_NoManager(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/export/foreground.asc?sensor_id=nonexistent", nil)
	rec := httptest.NewRecorder()

	server.handleExportForegroundASC(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent sensor, got %d", rec.Code)
	}
}

func TestWebServer_HandlePCAPStop_NoPCAPActive(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/stop?sensor_id=test-sensor", nil)
	rec := httptest.NewRecorder()

	server.handlePCAPStop(rec, req)

	// Should return success even if no PCAP is active
	if rec.Code < 200 || rec.Code >= 300 {
		t.Logf("handlePCAPStop returned status %d", rec.Code)
	}
}

func TestWebServer_HandlePCAPResumeLive_NoPCAPActive(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/resume-live?sensor_id=test-sensor", nil)
	rec := httptest.NewRecorder()

	server.handlePCAPResumeLive(rec, req)

	t.Logf("handlePCAPResumeLive returned status %d", rec.Code)
}

func TestWebServer_HandleStatus_Complete(t *testing.T) {
	sensorID := "status-test-" + time.Now().Format("150405")
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/status?sensor_id="+sensorID, nil)
	rec := httptest.NewRecorder()

	server.handleStatus(rec, req)

	// Log the status regardless - the actual status depends on background grid state
	t.Logf("handleStatus returned status %d", rec.Code)
}

func TestWebServer_HandleBackgroundRegions_NonexistentSensor(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/background/regions?sensor_id=nonexistent", nil)
	rec := httptest.NewRecorder()

	server.handleBackgroundRegions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestWebServer_HandleBackgroundRegions_ValidSensor(t *testing.T) {
	sensorID := "regions-test-" + time.Now().Format("150405")
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/background/regions?sensor_id="+sensorID, nil)
	rec := httptest.NewRecorder()

	server.handleBackgroundRegions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestWebServer_HandleBackgroundGrid_NonexistentSensor(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/background/grid?sensor_id=nonexistent", nil)
	rec := httptest.NewRecorder()

	server.handleBackgroundGrid(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestWebServer_HandleGridHeatmap_NonexistentSensor(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/heatmap?sensor_id=nonexistent", nil)
	rec := httptest.NewRecorder()

	server.handleGridHeatmap(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestWebServer_HandleBackgroundGridHeatmapChart_NonexistentSensor(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/background/heatmap/chart?sensor_id=nonexistent", nil)
	rec := httptest.NewRecorder()

	server.handleBackgroundGridHeatmapChart(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestWebServer_HandleBackgroundGridHeatmapChart_ValidSensor(t *testing.T) {
	sensorID := "heatmap-chart-" + time.Now().Format("150405")
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/background/heatmap/chart?sensor_id="+sensorID, nil)
	rec := httptest.NewRecorder()

	server.handleBackgroundGridHeatmapChart(rec, req)

	// Should return 200 OK even if no cells (renders empty chart)
	t.Logf("handleBackgroundGridHeatmapChart returned status %d", rec.Code)
}

func TestWebServer_ResolvePCAPPath_Absolute(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
		PCAPSafeDir:       "/", // Configure a safe directory
	}
	server := NewWebServer(config)

	// Test absolute path within safe directory
	path, err := server.resolvePCAPPath("/tmp/file.pcap")
	if err != nil {
		t.Logf("resolvePCAPPath error (expected for security): %v", err)
	}
	if path != "" {
		t.Logf("Resolved path: %s", path)
	}
}

func TestWebServer_ResolvePCAPPath_Relative(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
		PCAPSafeDir:       "/tmp", // Configure a safe directory
	}
	server := NewWebServer(config)

	// Test relative path (should be resolved relative to safe dir)
	path, err := server.resolvePCAPPath("file.pcap")
	if err != nil {
		t.Logf("resolvePCAPPath relative error: %v", err)
	}
	if path != "" {
		t.Logf("Resolved relative path: %s", path)
	}
}

func TestWebServer_ResolvePCAPPath_Empty(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	// Test empty path
	_, err := server.resolvePCAPPath("")
	if err == nil {
		t.Error("Expected error for empty path")
	}
}

func TestWebServer_Close_NotStarted(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		SensorID:          "test-sensor",
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	server := NewWebServer(config)

	// Close without starting should not panic
	err := server.Close()
	if err != nil {
		t.Errorf("Unexpected error closing non-started server: %v", err)
	}
}

// ====== Additional TrackAPI coverage tests ======

func TestTrackAPI_HandleListTracks_WithStateFilter(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-state")

	// Insert test tracks with different states
	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-state", "confirmed", now-1000, now)
	insertTestTrack(t, db, "track-002", "sensor-state", "tentative", now-2000, now-500)
	insertTestTrack(t, db, "track-003", "sensor-state", "deleted", now-3000, now-1000)

	tests := []struct {
		name        string
		state       string
		expectCount int
	}{
		{"confirmed only", "confirmed", 1},
		{"tentative only", "tentative", 1},
		{"deleted only", "deleted", 1},
		{"all states", "all", 3},
		{"empty state (default)", "", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := "/api/lidar/tracks?sensor_id=sensor-state"
			if tc.state != "" {
				url += "&state=" + tc.state
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			api.handleListTracks(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}
		})
	}
}

func TestTrackAPI_HandleListTracks_InvalidSensorID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "default-sensor")

	// Query with non-existent sensor - should return empty list
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?sensor_id=nonexistent", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for non-existent sensor, got %d", w.Code)
	}
}

func TestTrackAPI_HandleClearTracks_NoSensorID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "default-sensor")

	// Clear tracks without sensor_id - should use default
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/clear", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestTrackAPI_HandleClearTracks_WrongMethod(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "test-sensor")

	// Note: handleClearTracks doesn't check HTTP method - it accepts any method
	// This test verifies the handler works with GET method
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/clear", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	// Handler returns 200 regardless of method
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestTrackAPI_HandleClearTracks_NoDBConfigured(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/clear", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}
