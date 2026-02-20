package monitor

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

// newTestWebServerForDirectBackend creates a minimal WebServer for direct backend testing.
func newTestWebServerForDirectBackend(t *testing.T, tracker *l5tracks.Tracker) *WebServer {
	t.Helper()
	return &WebServer{
		sensorID: "test-sensor",
		tracker:  tracker,
	}
}

func TestNewDirectBackend(t *testing.T) {
	ws := newTestWebServerForDirectBackend(t, nil)
	db := NewDirectBackend("sensor-1", ws)

	if db == nil {
		t.Fatal("expected non-nil DirectBackend")
	}
	if db.sensorID != "sensor-1" {
		t.Errorf("expected sensorID 'sensor-1', got %q", db.sensorID)
	}
}

func TestDirectBackend_SensorID(t *testing.T) {
	ws := newTestWebServerForDirectBackend(t, nil)
	db := NewDirectBackend("my-sensor", ws)

	if db.SensorID() != "my-sensor" {
		t.Errorf("SensorID() = %q, want %q", db.SensorID(), "my-sensor")
	}
}

func TestDirectBackend_ImplementsSweepBackend(t *testing.T) {
	// Compile-time check is in direct_backend.go, but verify at runtime too.
	ws := newTestWebServerForDirectBackend(t, nil)
	var _ sweep.SweepBackend = NewDirectBackend("sensor", ws)
}

func TestDirectBackend_FetchBuckets_NoManager(t *testing.T) {
	sensorID := "direct-test-no-mgr-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	buckets := db.FetchBuckets()
	// Should return default buckets when no BackgroundManager is registered.
	if len(buckets) == 0 {
		t.Error("expected default buckets when no manager is registered")
	}
}

func TestDirectBackend_FetchBuckets_WithManager(t *testing.T) {
	sensorID := "direct-test-buckets-" + t.Name()
	bgMgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	if bgMgr == nil {
		t.Fatal("failed to create BackgroundManager")
	}

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	buckets := db.FetchBuckets()
	if len(buckets) == 0 {
		t.Error("expected non-empty buckets from manager")
	}
}

func TestDirectBackend_FetchAcceptanceMetrics_NoManager(t *testing.T) {
	sensorID := "direct-test-no-accept-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	_, err := db.FetchAcceptanceMetrics()
	if err == nil {
		t.Error("expected error when no manager is registered")
	}
}

func TestDirectBackend_FetchAcceptanceMetrics_WithManager(t *testing.T) {
	sensorID := "direct-test-accept-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	result, err := db.FetchAcceptanceMetrics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify expected keys
	for _, key := range []string{"BucketsMeters", "AcceptCounts", "RejectCounts", "Totals", "AcceptanceRates"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing key %q in acceptance metrics", key)
		}
	}
}

func TestDirectBackend_ResetAcceptance_NoManager(t *testing.T) {
	sensorID := "direct-test-no-reset-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.ResetAcceptance()
	if err == nil {
		t.Error("expected error when no manager is registered")
	}
}

func TestDirectBackend_ResetAcceptance_WithManager(t *testing.T) {
	sensorID := "direct-test-reset-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.ResetAcceptance()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDirectBackend_FetchGridStatus_NoManager(t *testing.T) {
	sensorID := "direct-test-no-grid-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	_, err := db.FetchGridStatus()
	if err == nil {
		t.Error("expected error when no manager is registered")
	}
}

func TestDirectBackend_FetchGridStatus_WithManager(t *testing.T) {
	sensorID := "direct-test-grid-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	status, err := db.FetchGridStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil grid status")
	}
}

func TestDirectBackend_ResetGrid_NoManager(t *testing.T) {
	sensorID := "direct-test-no-gridreset-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.ResetGrid()
	if err == nil {
		t.Error("expected error when no manager is registered")
	}
}

func TestDirectBackend_ResetGrid_WithManager(t *testing.T) {
	sensorID := "direct-test-gridreset-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)
	ws := &WebServer{sensorID: sensorID, tracker: tracker}
	db := NewDirectBackend(sensorID, ws)

	err := db.ResetGrid()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDirectBackend_WaitForGridSettle_ZeroTimeout(t *testing.T) {
	sensorID := "direct-test-settle-zero-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	// Should return immediately with zero timeout.
	start := time.Now()
	db.WaitForGridSettle(0)
	if time.Since(start) > 100*time.Millisecond {
		t.Error("WaitForGridSettle with 0 timeout should return immediately")
	}
}

func TestDirectBackend_WaitForGridSettle_NegativeTimeout(t *testing.T) {
	sensorID := "direct-test-settle-neg-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	// Should return immediately with negative timeout.
	start := time.Now()
	db.WaitForGridSettle(-1 * time.Second)
	if time.Since(start) > 100*time.Millisecond {
		t.Error("WaitForGridSettle with negative timeout should return immediately")
	}
}

func TestDirectBackend_WaitForGridSettle_Timeout(t *testing.T) {
	sensorID := "direct-test-settle-timeout-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	// Should time out since background_count is 0 and won't change.
	start := time.Now()
	db.WaitForGridSettle(300 * time.Millisecond)
	elapsed := time.Since(start)
	if elapsed < 250*time.Millisecond {
		t.Errorf("WaitForGridSettle should have waited ~300ms, got %v", elapsed)
	}
}

func TestDirectBackend_FetchTrackingMetrics_NoTracker(t *testing.T) {
	sensorID := "direct-test-no-tracker-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	_, err := db.FetchTrackingMetrics()
	if err == nil {
		t.Error("expected error when tracker is nil")
	}
}

func TestDirectBackend_FetchTrackingMetrics_WithTracker(t *testing.T) {
	sensorID := "direct-test-tracker-" + t.Name()
	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)
	ws := &WebServer{sensorID: sensorID, tracker: tracker}
	db := NewDirectBackend(sensorID, ws)

	result, err := db.FetchTrackingMetrics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil metrics result")
	}
}

func TestDirectBackend_SetTuningParams_NoManager(t *testing.T) {
	sensorID := "direct-test-no-tuning-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.SetTuningParams(map[string]interface{}{"noise_relative": 0.1})
	if err == nil {
		t.Error("expected error when no manager is registered")
	}
}

func TestDirectBackend_SetTuningParams_WithTracker(t *testing.T) {
	sensorID := "direct-test-tuning-tracker-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)
	ws := &WebServer{sensorID: sensorID, tracker: tracker}
	db := NewDirectBackend(sensorID, ws)

	params := map[string]interface{}{
		"noise_relative":              0.15,
		"enable_diagnostics":          true,
		"closeness_multiplier":        2.0,
		"seed_from_first":             true,
		"gating_distance_squared":     5.0,
		"process_noise_pos":           0.5,
		"process_noise_vel":           1.0,
		"measurement_noise":           0.3,
		"occlusion_cov_inflation":     2.0,
		"hits_to_confirm":             4,
		"max_misses":                  8,
		"max_misses_confirmed":        12,
		"background_update_fraction":  0.02,
		"safety_margin_meters":        0.5,
		"post_settle_update_fraction": 0.01,
	}

	err := db.SetTuningParams(params)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify tracker config was updated
	if tracker.Config.GatingDistanceSquared != 5.0 {
		t.Errorf("expected GatingDistanceSquared 5.0, got %f", tracker.Config.GatingDistanceSquared)
	}
	if tracker.Config.HitsToConfirm != 4 {
		t.Errorf("expected HitsToConfirm 4, got %d", tracker.Config.HitsToConfirm)
	}
	if tracker.Config.MaxMisses != 8 {
		t.Errorf("expected MaxMisses 8, got %d", tracker.Config.MaxMisses)
	}
}

func TestDirectBackend_SetTuningParams_EmptyParams(t *testing.T) {
	sensorID := "direct-test-tuning-empty-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.SetTuningParams(map[string]interface{}{})
	if err != nil {
		t.Errorf("unexpected error for empty params: %v", err)
	}
}

func TestDirectBackend_SetTuningParams_WarmupParams(t *testing.T) {
	sensorID := "direct-test-warmup-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.SetTuningParams(map[string]interface{}{
		"warmup_duration_nanos": int64(5e9),
		"warmup_min_frames":     10,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDirectBackend_SetTuningParams_ForegroundClusterParams(t *testing.T) {
	sensorID := "direct-test-fg-cluster-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.SetTuningParams(map[string]interface{}{
		"foreground_min_cluster_points": 5,
		"foreground_dbscan_eps":         0.8,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDirectBackend_SetTuningParams_NeighborConfirmation(t *testing.T) {
	sensorID := "direct-test-neighbor-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.SetTuningParams(map[string]interface{}{
		"neighbor_confirmation_count": 3,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDirectBackend_GetLastAnalysisRunID(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}
	db := NewDirectBackend("test-sensor", ws)

	runID := db.GetLastAnalysisRunID()
	if runID != "" {
		t.Errorf("expected empty run ID, got %q", runID)
	}
}

func TestDirectBackend_GetLastAnalysisRunID_WithValue(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}
	ws.pcapLastRunID = "run-abc-123"
	db := NewDirectBackend("test-sensor", ws)

	runID := db.GetLastAnalysisRunID()
	if runID != "run-abc-123" {
		t.Errorf("expected 'run-abc-123', got %q", runID)
	}
}

func TestDirectBackend_WaitForPCAPComplete_NoReplay(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}
	db := NewDirectBackend("test-sensor", ws)

	// PCAPDone returns nil when no replay is in progress, so should return immediately.
	err := db.WaitForPCAPComplete(1 * time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDirectBackend_WaitForPCAPComplete_DefaultTimeout(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}
	db := NewDirectBackend("test-sensor", ws)

	// With no replay, 0 timeout should use default of 120s, but return immediately
	// since PCAPDone() returns nil.
	err := db.WaitForPCAPComplete(0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDirectBackend_WaitForPCAPComplete_ClosedChannel(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}
	// Simulate completed PCAP by pre-closing the channel.
	ch := make(chan struct{})
	close(ch)
	ws.pcapDone = ch
	db := NewDirectBackend("test-sensor", ws)

	err := db.WaitForPCAPComplete(1 * time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestToInterfaceSlice tests the generic helper.
func TestToInterfaceSlice(t *testing.T) {
	result := toInterfaceSlice([]float64{1.0, 2.0, 3.0})
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	if result[0] != 1.0 {
		t.Errorf("expected 1.0, got %v", result[0])
	}
}

func TestToInterfaceSlice_Empty(t *testing.T) {
	result := toInterfaceSlice([]int{})
	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}
}

func TestToInterfaceSliceInt64(t *testing.T) {
	result := toInterfaceSliceInt64([]int64{10, 20, 30})
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	if result[1] != int64(20) {
		t.Errorf("expected 20, got %v", result[1])
	}
}

func TestToInterfaceSliceInt64_Empty(t *testing.T) {
	result := toInterfaceSliceInt64([]int64{})
	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}
}

func TestToInterfaceSliceFloat64(t *testing.T) {
	result := toInterfaceSliceFloat64([]float64{0.5, 0.75, 1.0})
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	if result[2] != 1.0 {
		t.Errorf("expected 1.0, got %v", result[2])
	}
}

func TestToInterfaceSliceFloat64_Empty(t *testing.T) {
	result := toInterfaceSliceFloat64([]float64{})
	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}
}
