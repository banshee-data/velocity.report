package monitor

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// --- WaitForGridSettle: positive int64 path (early return) ---

func TestDirectBackend_WaitForGridSettle_Int64Positive(t *testing.T) {
	sensorID := "direct-test-settle-int64-" + t.Name()
	mgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	if mgr == nil {
		t.Fatal("failed to create BackgroundManager")
	}

	// Set a positive background count so the int64 branch triggers an early return.
	mgr.Grid.BackgroundCount = 42

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	start := time.Now()
	db.WaitForGridSettle(5 * time.Second)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("WaitForGridSettle should have returned early (int64 > 0), took %v", elapsed)
	}
}

// --- WaitForGridSettle: no manager registered ---

func TestDirectBackend_WaitForGridSettle_NoManager(t *testing.T) {
	sensorID := "direct-test-settle-nomgr-" + t.Name()
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	// Should loop until timeout with no manager.
	start := time.Now()
	db.WaitForGridSettle(300 * time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 250*time.Millisecond {
		t.Errorf("should have waited ~300ms, got %v", elapsed)
	}
}

// --- FetchBuckets: nil metrics from manager ---

func TestDirectBackend_FetchBuckets_NilMetrics(t *testing.T) {
	sensorID := "direct-test-nilmetrics-" + t.Name()
	// Create a manager; by default, GetAcceptanceMetrics() returns metrics
	// with empty BucketsMeters — which triggers the nil/empty check.
	mgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	if mgr == nil {
		t.Fatal("failed to create BackgroundManager")
	}
	// The default manager has buckets, so reset them to empty to trigger the branch.
	mgr.Grid.AcceptanceBucketsMeters = nil

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	buckets := db.FetchBuckets()
	// Should return default buckets when metrics.BucketsMeters is empty.
	if len(buckets) == 0 {
		t.Error("expected default buckets when metrics are empty")
	}
	// Verify it returns the standard defaults (11 entries)
	defaults := DefaultBuckets()
	if len(buckets) != len(defaults) {
		t.Errorf("expected %d default buckets, got %d", len(defaults), len(buckets))
	}
}

// --- FetchAcceptanceMetrics: nil metrics fallback ---

func TestDirectBackend_FetchAcceptanceMetrics_NilMetrics(t *testing.T) {
	sensorID := "direct-test-nilaccept-" + t.Name()
	mgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	if mgr == nil {
		t.Fatal("failed to create BackgroundManager")
	}
	// Clear acceptance data to force the nil/empty metrics path.
	mgr.Grid.AcceptanceBucketsMeters = nil
	mgr.Grid.AcceptByRangeBuckets = nil
	mgr.Grid.RejectByRangeBuckets = nil

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	result, err := db.FetchAcceptanceMetrics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// With nil BucketsMeters, the metrics object should still be valid with empty slices.
	bm, ok := result["BucketsMeters"].([]interface{})
	if !ok {
		t.Fatal("expected BucketsMeters as []interface{}")
	}
	if len(bm) != 0 {
		t.Errorf("expected 0 buckets, got %d", len(bm))
	}
}

// --- FetchGridStatus: nil grid status ---

func TestDirectBackend_FetchGridStatus_NilStatus(t *testing.T) {
	sensorID := "direct-test-nilgrid-" + t.Name()
	// Create a manager but nil out the grid to trigger nil status.
	mgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	if mgr == nil {
		t.Fatal("failed to create BackgroundManager")
	}
	mgr.Grid = nil // force GridStatus() to return nil

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	_, err := db.FetchGridStatus()
	if err == nil {
		t.Error("expected error when GridStatus returns nil")
	}
}

// --- ResetGrid: with frame builder ---

func TestDirectBackend_ResetGrid_NoTracker(t *testing.T) {
	sensorID := "direct-test-gridreset-notracker-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)

	// No tracker set — verify ws.tracker == nil branch works.
	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.ResetGrid()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- FetchTrackingMetrics: with tracker ---

func TestDirectBackend_FetchTrackingMetrics_Success(t *testing.T) {
	sensorID := "direct-test-trackmetrics-" + t.Name()
	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)
	ws := &WebServer{sensorID: sensorID, tracker: tracker}
	db := NewDirectBackend(sensorID, ws)

	result, err := db.FetchTrackingMetrics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// --- SetTuningParams: exercise more branches ---

func TestDirectBackend_SetTuningParams_NilGrid(t *testing.T) {
	sensorID := "direct-test-tuning-nilgrid-" + t.Name()
	mgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	if mgr == nil {
		t.Fatal("failed to create manager")
	}
	mgr.Grid = nil // force the Grid == nil check

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	err := db.SetTuningParams(map[string]interface{}{"noise_relative": 0.1})
	if err == nil {
		t.Error("expected error when Grid is nil")
	}
}

// --- WaitForGridSettle: concurrent modification ---

func TestDirectBackend_WaitForGridSettle_BecomesPositive(t *testing.T) {
	sensorID := "direct-test-settle-become-" + t.Name()
	params := l3grid.BackgroundParams{
		SeedFromFirstObservation: true,
	}
	mgr := l3grid.NewBackgroundManager(sensorID, 16, 360, params, nil)
	if mgr == nil {
		t.Fatal("failed to create BackgroundManager")
	}

	ws := &WebServer{sensorID: sensorID}
	db := NewDirectBackend(sensorID, ws)

	// Feed points through ProcessFramePolar inside a goroutine.
	// ProcessFramePolar acquires g.mu.Lock() internally, avoiding the
	// data race that occurs when writing BackgroundCount directly.
	// Multiple frames are needed: the first seeds the cells, subsequent
	// frames classify observations as background.
	go func() {
		time.Sleep(100 * time.Millisecond)
		points := make([]l3grid.PointPolar, 0, 360)
		for az := 0; az < 360; az++ {
			points = append(points, l3grid.PointPolar{
				Channel:  1,
				Azimuth:  float64(az),
				Distance: 5.0,
			})
		}
		// Frame 1: seeds cells. Frame 2+: classifies as background.
		for i := 0; i < 3; i++ {
			mgr.ProcessFramePolar(points)
		}
	}()

	start := time.Now()
	db.WaitForGridSettle(2 * time.Second)
	elapsed := time.Since(start)

	if elapsed > 1500*time.Millisecond {
		t.Errorf("should have returned within ~1s, took %v", elapsed)
	}
}

// --- ResetGrid: with registered frame builder ---

func TestDirectBackend_ResetGrid_WithFrameBuilder(t *testing.T) {
	sensorID := "direct-test-gridreset-withfb-" + t.Name()
	_ = l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)

	// Register a frame builder for this sensor so the fb != nil branch is covered.
	fb := l2frames.NewFrameBuilderWithLogging(sensorID)
	defer func() {
		fb.Close()
		l2frames.UnregisterFrameBuilder(sensorID)
	}()
	l2frames.RegisterFrameBuilder(sensorID, fb)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)
	ws := &WebServer{sensorID: sensorID, tracker: tracker}
	db := NewDirectBackend(sensorID, ws)

	err := db.ResetGrid()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
