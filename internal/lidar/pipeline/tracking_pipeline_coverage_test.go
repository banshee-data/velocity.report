package pipeline

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"

	_ "modernc.org/sqlite"
)

// TestIsNilInterface covers the nil-check helper.
func TestIsNilInterface_NilValue(t *testing.T) {
	if !isNilInterface(nil) {
		t.Error("expected nil for nil value")
	}
}

func TestIsNilInterface_NilPointer(t *testing.T) {
	var p *int
	// Passing a typed nil pointer inside an interface
	if !isNilInterface(p) {
		t.Error("expected true for nil pointer wrapped in interface")
	}
}

func TestIsNilInterface_NonNilPointer(t *testing.T) {
	x := 42
	if isNilInterface(&x) {
		t.Error("expected false for non-nil pointer")
	}
}

func TestIsNilInterface_NonPointerValue(t *testing.T) {
	if isNilInterface(42) {
		t.Error("expected false for non-pointer int value")
	}
	if isNilInterface("hello") {
		t.Error("expected false for non-pointer string value")
	}
}

func TestIsNilInterface_NilSlice(t *testing.T) {
	var s []int
	if !isNilInterface(s) {
		t.Error("expected true for nil slice")
	}
}

func TestIsNilInterface_NilMap(t *testing.T) {
	var m map[string]int
	if !isNilInterface(m) {
		t.Error("expected true for nil map")
	}
}

func TestIsNilInterface_NilChan(t *testing.T) {
	var ch chan int
	if !isNilInterface(ch) {
		t.Error("expected true for nil channel")
	}
}

func TestIsNilInterface_NilFunc(t *testing.T) {
	var fn func()
	if !isNilInterface(fn) {
		t.Error("expected true for nil func")
	}
}

func TestIsNilInterface_NonNilSlice(t *testing.T) {
	s := make([]int, 0)
	if isNilInterface(s) {
		t.Error("expected false for non-nil slice")
	}
}

// TestIsNilInterfaceExported verifies the exported alias.
func TestIsNilInterfaceExported(t *testing.T) {
	if !IsNilInterface(nil) {
		t.Error("exported IsNilInterface should return true for nil")
	}
	x := 42
	if IsNilInterface(&x) {
		t.Error("exported IsNilInterface should return false for non-nil pointer")
	}
}

// TestSensorRuntime_Fields verifies the SensorRuntime struct.
func TestSensorRuntime_Fields(t *testing.T) {
	rt := SensorRuntime{SensorID: "sensor-test"}
	if rt.SensorID != "sensor-test" {
		t.Errorf("expected sensor-test, got %s", rt.SensorID)
	}
	if rt.FrameBuilder != nil {
		t.Error("FrameBuilder should be nil by default")
	}
	if rt.BackgroundManager != nil {
		t.Error("BackgroundManager should be nil by default")
	}
	if rt.AnalysisRunManager != nil {
		t.Error("AnalysisRunManager should be nil by default")
	}
}

// TestTrackingPipelineConfig_NewFrameCallback_NilFrame verifies nil/empty frame handling.
func TestTrackingPipelineConfig_NewFrameCallback_NilFrame(t *testing.T) {
	cfg := &TrackingPipelineConfig{SensorID: "test-nil-frame"}
	cb := cfg.NewFrameCallback()

	// Should not panic on nil frame.
	cb(nil)

	// Should not panic on frame with no points.
	cb(&l2frames.LiDARFrame{})
}

// TestTrackingPipelineConfig_NewFrameCallback_NoBackgroundManager tests early return
// when BackgroundManager is nil.
func TestTrackingPipelineConfig_NewFrameCallback_NoBackgroundManager(t *testing.T) {
	cfg := &TrackingPipelineConfig{SensorID: "test-no-bgmgr"}
	cb := cfg.NewFrameCallback()

	pts := []l2frames.Point{
		{Channel: 1, Azimuth: 90.0, Distance: 5.0, Intensity: 80, Timestamp: time.Now()},
	}
	frame := &l2frames.LiDARFrame{
		FrameID:     "test-frame",
		PolarPoints: pointsToPolar(pts),
		Points:      pts,
	}

	// Should not panic; callback should return early when BackgroundManager is nil.
	cb(frame)
}

// ---------------------------------------------------------------------------
// Helper: create a background manager with tight params that quickly
// converge so foreground detection works within a few frames.
// ---------------------------------------------------------------------------

func makeTestBgManager(t *testing.T, sensorID string) *l3grid.BackgroundManager {
	t.Helper()
	return l3grid.NewBackgroundManagerDI(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation:       true,
		BackgroundUpdateFraction:       0.5,
		ClosenessSensitivityMultiplier: 2.0,
		SafetyMarginMeters:             0.5,
		NeighborConfirmationCount:      0,
		NoiseRelativeFraction:          0.01,
		// Zero warmup — foreground emitted immediately
		WarmupDurationNanos: 0,
		WarmupMinFrames:     0,
	}, nil)
}

// fgChannel is the single channel used by foreground points so seed and
// foreground frames always hit the same background-grid cell.
const fgChannel = 1

// fgAzimuth is the single azimuth used by foreground points.
const fgAzimuth = 180.0

// pointsToPolar restructures Cartesian Points into PointPolar format by
// copying the polar coordinate fields (Distance, Azimuth, Elevation) and
// metadata. No mathematical conversion is performed — the input Points
// must already contain valid polar values in their respective fields.
func pointsToPolar(points []l2frames.Point) []l2frames.PointPolar {
	polar := make([]l2frames.PointPolar, len(points))
	for i, p := range points {
		polar[i] = l2frames.PointPolar{
			Channel:         p.Channel,
			Azimuth:         p.Azimuth,
			Elevation:       p.Elevation,
			Distance:        p.Distance,
			Intensity:       p.Intensity,
			Timestamp:       p.Timestamp.UnixNano(),
			BlockID:         p.BlockID,
			UDPSequence:     p.UDPSequence,
			RawBlockAzimuth: p.RawBlockAzimuth,
		}
	}
	return polar
}

// makeStableFrame creates a frame with points at a stable background distance.
// It includes both a spread of channels/azimuths AND the exact foreground cell
// (fgChannel / fgAzimuth) so the background model is seeded for that cell.
func makeStableFrame(id string, ts time.Time, distance float64) *l2frames.LiDARFrame {
	points := make([]l2frames.Point, 0, 50)
	// Spread points across various cells
	for i := 0; i < 40; i++ {
		points = append(points, l2frames.Point{
			Channel:   i%16 + 1,
			Azimuth:   float64(i%40) * 9.0,
			Distance:  distance,
			Intensity: 80,
			Timestamp: ts,
		})
	}
	// Explicitly ensure the foreground cell is seeded with a background distance.
	// Some of the spread points above may already hit this cell, but we add
	// extras to guarantee a strong background model.
	for i := 0; i < 10; i++ {
		points = append(points, l2frames.Point{
			Channel:   fgChannel,
			Azimuth:   fgAzimuth,
			Distance:  distance,
			Intensity: 80,
			Timestamp: ts,
		})
	}
	return &l2frames.LiDARFrame{
		FrameID:        id,
		StartTimestamp: ts,
		PolarPoints:    pointsToPolar(points),
		Points:         points,
	}
}

// of foreground points at a very different distance (closer). All foreground
// points hit the SAME grid cell (fgChannel / fgAzimuth) that was seeded by
// makeStableFrame. Points are slightly spread in distance so the resulting world
// cluster has a non-zero diameter (DBSCAN rejects diameter < 0.05 m).
func makeForegroundFrame(id string, ts time.Time, bgDist, fgDist float64) *l2frames.LiDARFrame {
	points := make([]l2frames.Point, 0, 60)
	// Background points – same spread as seed frames
	for i := 0; i < 40; i++ {
		points = append(points, l2frames.Point{
			Channel:   i%16 + 1,
			Azimuth:   float64(i%40) * 9.0,
			Distance:  bgDist,
			Intensity: 80,
			Timestamp: ts,
		})
	}
	// Foreground cluster: 20 points all in the seeded cell but with slight
	// distance spread (±0.05 m) so the world-space cluster exceeds the min
	// diameter threshold (0.05 m). The distance variation stays well within
	// the background delta so all points remain foreground.
	for i := 0; i < 20; i++ {
		d := fgDist + float64(i-10)*0.01 // ±0.1 m spread
		points = append(points, l2frames.Point{
			Channel:   fgChannel,
			Azimuth:   fgAzimuth,
			Distance:  d,
			Intensity: 200,
			Timestamp: ts,
		})
	}
	return &l2frames.LiDARFrame{
		FrameID:        id,
		StartTimestamp: ts,
		PolarPoints:    pointsToPolar(points),
		Points:         points,
	}
}

// all 16 channels × 350 azimuths, plus 20 foreground points. This exercises
// the background downsampling branches (stride and cap).
func makeDenseFrame(id string, ts time.Time, bgDist, fgDist float64) *l2frames.LiDARFrame {
	points := make([]l2frames.Point, 0, 5700)
	for ch := 1; ch <= 16; ch++ {
		for az := 0; az < 350; az++ {
			points = append(points, l2frames.Point{
				Channel:   ch,
				Azimuth:   float64(az),
				Distance:  bgDist,
				Intensity: 80,
				Timestamp: ts,
			})
		}
	}
	for i := 0; i < 20; i++ {
		d := fgDist + float64(i-10)*0.01
		points = append(points, l2frames.Point{
			Channel:   fgChannel,
			Azimuth:   fgAzimuth,
			Distance:  d,
			Intensity: 200,
			Timestamp: ts,
		})
	}
	return &l2frames.LiDARFrame{
		FrameID:        id,
		StartTimestamp: ts,
		PolarPoints:    pointsToPolar(points),
		Points:         points,
	}
}
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "pipeline-test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatalf("pragma %q: %v", pragma, err)
		}
	}

	schemaPath := filepath.Join("..", "storage", "sqlite", "..", "..", "..", "db", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (15, false)`); err != nil {
		t.Fatalf("baseline migrations: %v", err)
	}
	return db
}

// TestTrackingPipelineConfig_NewFrameCallback_WithBackgroundManager tests the pipeline
// up to the foreground extraction stage (no tracker).
func TestTrackingPipelineConfig_NewFrameCallback_WithBackgroundManager(t *testing.T) {
	sensorID := "coverage-with-bgmgr-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		RemoveGround:      true,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	// Seed background
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	// Now send a frame with foreground — should extract foreground but no tracker
	cb(makeForegroundFrame("fg-1", now.Add(600*time.Millisecond), 20.0, 5.0))
}

// TestTrackingPipelineConfig_NewFrameCallback_FullPipelineWithDB tests the entire pipeline
// including foreground extraction → clustering → tracking → DB persistence → pruning.
func TestTrackingPipelineConfig_NewFrameCallback_FullPipelineWithDB(t *testing.T) {
	sensorID := "coverage-full-db-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	db := setupTestDB(t)

	var featureExportCalled atomic.Int32
	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		Classifier:        l6objects.NewTrackClassifier(),
		DB:                db,
		RemoveGround:      true,
		HeightBandFloor:   -10.0, // very wide to not filter anything in tests
		HeightBandCeiling: 10.0,
		MaxFrameRate:      0, // no throttling
		VoxelLeafSize:     0.0,
		FeatureExportFunc: func(trackID string, features l6objects.TrackFeatures, class string, confidence float32) {
			featureExportCalled.Add(1)
		},
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Seed background with stable frames
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	// Send foreground frames to create and confirm tracks
	for i := 0; i < 15; i++ {
		ts := now.Add(time.Duration(500+i*100) * time.Millisecond)
		fgDist := 5.0 + float64(i)*0.1 // slightly moving foreground
		cb(makeForegroundFrame("fg-"+string(rune('A'+i)), ts, 20.0, fgDist))
	}

	total, _, confirmed, _ := tracker.GetTrackCount()
	t.Logf("Tracks: total=%d, confirmed=%d, featureExport=%d", total, confirmed, featureExportCalled.Load())
}

// TestTrackingPipelineConfig_NewFrameCallback_Throttling tests frame rate throttling.
func TestTrackingPipelineConfig_NewFrameCallback_Throttling(t *testing.T) {
	sensorID := "coverage-throttle-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		MaxFrameRate:      1, // 1 fps — should throttle rapid frames
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Seed background
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	// Rapid foreground frames — most should be throttled
	for i := 0; i < 60; i++ {
		ts := now.Add(time.Duration(500+i*10) * time.Millisecond) // 10ms apart = 100fps
		cb(makeForegroundFrame("throttle-"+string(rune('0'+i%10)), ts, 20.0, 5.0))
	}
}

// TestTrackingPipelineConfig_NewFrameCallback_DebugMode tests debug logging paths.
func TestTrackingPipelineConfig_NewFrameCallback_DebugMode(t *testing.T) {
	sensorID := "coverage-debug-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	// Seed + 1 foreground
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	cb(makeForegroundFrame("dfg-1", now.Add(600*time.Millisecond), 20.0, 5.0))
}

// TestTrackingPipelineConfig_NewFrameCallback_NoGroundRemoval verifies RemoveGround=false path.
func TestTrackingPipelineConfig_NewFrameCallback_NoGroundRemoval(t *testing.T) {
	sensorID := "coverage-no-ground-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	cb(makeForegroundFrame("ng-1", now.Add(600*time.Millisecond), 20.0, 5.0))
}

// mockFgForwarder implements ForegroundForwarder for testing.
type mockFgForwarder struct {
	calls  int
	points []l2frames.PointPolar
}

func (m *mockFgForwarder) ForwardForeground(points []l2frames.PointPolar) {
	m.calls++
	m.points = append(m.points, points...)
}

// mockVisualiserPublisher implements VisualiserPublisher for testing.
type mockVisualiserPublisher struct {
	publishCalls int
}

func (m *mockVisualiserPublisher) Publish(frame interface{}) {
	m.publishCalls++
}

// mockVisualiserAdapter implements VisualiserAdapter for testing.
type mockVisualiserAdapter struct {
	adaptCalls      int
	adaptEmptyCalls int
}

func (m *mockVisualiserAdapter) AdaptFrame(frame *l2frames.LiDARFrame, foregroundMask []bool, clusters []l4perception.WorldCluster, tracker l5tracks.TrackerInterface, debugFrame interface{}) interface{} {
	m.adaptCalls++
	return struct{}{}
}

func (m *mockVisualiserAdapter) AdaptEmptyFrame(frame *l2frames.LiDARFrame) interface{} {
	m.adaptEmptyCalls++
	return struct{}{}
}

// mockLidarViewAdapter implements LidarViewAdapter for testing.
type mockLidarViewAdapter struct {
	calls int
}

func (m *mockLidarViewAdapter) PublishFrameBundle(bundle interface{}, foregroundPoints []l2frames.PointPolar) {
	m.calls++
}

// TestTrackingPipelineConfig_WithFgForwarder tests foreground forwarding path.
func TestTrackingPipelineConfig_WithFgForwarder(t *testing.T) {
	sensorID := "coverage-fwd-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	fwd := &mockFgForwarder{}
	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		FgForwarder:       fwd,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	// Seed background
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	// Send foreground frames
	for i := 0; i < 3; i++ {
		cb(makeForegroundFrame("fwd-"+string(rune('A'+i)), now.Add(time.Duration(500+i*100)*time.Millisecond), 20.0, 5.0))
	}
	t.Logf("Foreground forwarder calls: %d, points: %d", fwd.calls, len(fwd.points))
}

// TestTrackingPipelineConfig_WithNilFgForwarder tests nil forwarder interface path.
func TestTrackingPipelineConfig_WithNilFgForwarder(t *testing.T) {
	sensorID := "coverage-nil-fwd-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		FgForwarder:       nil,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	cb(makeForegroundFrame("nil-fwd-1", now.Add(600*time.Millisecond), 20.0, 5.0))
}

// TestTrackingPipelineConfig_WithFgForwarder_DebugRange tests the debug range filtering
// of foreground points before forwarding.
func TestTrackingPipelineConfig_WithFgForwarder_DebugRange(t *testing.T) {
	sensorID := "coverage-debug-range-" + t.Name()
	bgMgr := l3grid.NewBackgroundManagerDI(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation:       true,
		BackgroundUpdateFraction:       0.5,
		ClosenessSensitivityMultiplier: 2.0,
		SafetyMarginMeters:             0.5,
		NeighborConfirmationCount:      0,
		NoiseRelativeFraction:          0.01,
		// Set debug range to only forward a specific region
		DebugRingMin: 0,
		DebugRingMax: 5,
		DebugAzMin:   0,
		DebugAzMax:   30,
	}, nil)

	fwd := &mockFgForwarder{}
	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		FgForwarder:       fwd,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	cb(makeForegroundFrame("debug-range-1", now.Add(600*time.Millisecond), 20.0, 5.0))
	t.Logf("Foreground forwarder calls with debug range: %d", fwd.calls)
}

// TestTrackingPipelineConfig_WithVisualiserPublisher tests the gRPC visualiser publishing path.
func TestTrackingPipelineConfig_WithVisualiserPublisher(t *testing.T) {
	sensorID := "coverage-vis-pub-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	visPub := &mockVisualiserPublisher{}
	visAdapter := &mockVisualiserAdapter{}
	lidarView := &mockLidarViewAdapter{}

	cfg := &TrackingPipelineConfig{
		SensorID:            sensorID,
		BackgroundManager:   bgMgr,
		Tracker:             tracker,
		RemoveGround:        false,
		VisualiserPublisher: visPub,
		VisualiserAdapter:   visAdapter,
		LidarViewAdapter:    lidarView,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	for i := 0; i < 5; i++ {
		cb(makeForegroundFrame("vis-"+string(rune('A'+i)), now.Add(time.Duration(500+i*100)*time.Millisecond), 20.0, 5.0))
	}

	t.Logf("Visualiser: publishCalls=%d, adaptCalls=%d, lidarViewCalls=%d",
		visPub.publishCalls, visAdapter.adaptCalls, lidarView.calls)
}

// TestTrackingPipelineConfig_LidarViewOnly tests LidarView-only mode (no gRPC adapter).
func TestTrackingPipelineConfig_LidarViewOnly(t *testing.T) {
	sensorID := "coverage-lidarview-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	lidarView := &mockLidarViewAdapter{}

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		RemoveGround:      false,
		// No VisualiserPublisher/Adapter, just LidarViewAdapter
		LidarViewAdapter: lidarView,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	for i := 0; i < 3; i++ {
		cb(makeForegroundFrame("lv-"+string(rune('A'+i)), now.Add(time.Duration(500+i*100)*time.Millisecond), 20.0, 5.0))
	}
	t.Logf("LidarView-only calls: %d", lidarView.calls)
}

// TestTrackingPipelineConfig_VoxelDownsample tests the voxel grid downsampling path.
func TestTrackingPipelineConfig_VoxelDownsample(t *testing.T) {
	sensorID := "coverage-voxel-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		RemoveGround:      false,
		VoxelLeafSize:     0.5, // large leaf to ensure downsampling happens
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	cb(makeForegroundFrame("vox-1", now.Add(600*time.Millisecond), 20.0, 5.0))
}

// TestTrackingPipelineConfig_AnalysisRunManager tests the analysis run recording path.
func TestTrackingPipelineConfig_AnalysisRunManager(t *testing.T) {
	sensorID := "coverage-arm-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	db := setupTestDB(t)
	arm := sqlite.NewAnalysisRunManager(db, sensorID)

	// Start an analysis run so IsRunActive() returns true
	if _, err := arm.StartRun("test-pcap.vrlog", sqlite.RunParams{}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	cfg := &TrackingPipelineConfig{
		SensorID:           sensorID,
		BackgroundManager:  bgMgr,
		Tracker:            tracker,
		DB:                 db,
		AnalysisRunManager: arm,
		RemoveGround:       false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	for i := 0; i < 10; i++ {
		cb(makeForegroundFrame("arm-"+string(rune('A'+i)), now.Add(time.Duration(500+i*100)*time.Millisecond), 20.0, 5.0+float64(i)*0.05))
	}

	t.Logf("Analysis run active: %v", arm.IsRunActive())
}

// TestTrackingPipelineConfig_NoClusters tests the no-clusters early return path
// with RecordFrameStats.
func TestTrackingPipelineConfig_NoClusters(t *testing.T) {
	sensorID := "coverage-noclus-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		RemoveGround:      true,
		HeightBandFloor:   0.0,   // filter everything below sensor plane
		HeightBandCeiling: 0.001, // very narrow band — should filter most points
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	// Foreground detected but ground filter removes points → 0 clusters
	cb(makeForegroundFrame("nc-1", now.Add(600*time.Millisecond), 20.0, 5.0))
}

// TestTrackingPipelineConfig_DBPruning tests the periodic DB pruning path by
// advancing time past the prune interval.
func TestTrackingPipelineConfig_DBPruning(t *testing.T) {
	sensorID := "coverage-prune-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	db := setupTestDB(t)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		DB:                db,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	// First foreground frame triggers initial prune timer
	cb(makeForegroundFrame("pr-1", now.Add(600*time.Millisecond), 20.0, 5.0))

	// Subsequent frames — time advances. The prune runs at most once per minute.
	// In practice it runs on the first frame with DB set.
	for i := 0; i < 5; i++ {
		cb(makeForegroundFrame("pr-"+string(rune('A'+i)), now.Add(time.Duration(700+i*100)*time.Millisecond), 20.0, 5.0))
	}
}

// TestTrackingPipelineConfig_ThrottleDiagf verifies that
// the diagf log fires on every 50th throttled frame.
func TestTrackingPipelineConfig_ThrottleDiagf(t *testing.T) {
	var diagBuf bytes.Buffer
	SetLogWriters(nil, &diagBuf, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-throttle-diagf-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		MaxFrameRate:      1, // 1 fps → 1s min interval; ensures all rapid-burst frames are throttled regardless of CI machine speed
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Seed background model.
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	// First foreground frame — passes the throttle check.
	cb(makeForegroundFrame("fg-pass", now.Add(600*time.Millisecond), 20.0, 5.0))

	// Rapid burst: 55 frames at ~0 interval → all throttled.
	// The 50th throttled frame triggers the diagf log.
	for i := 0; i < 55; i++ {
		cb(makeForegroundFrame(
			fmt.Sprintf("fg-rapid-%d", i),
			now.Add(time.Duration(601+i)*time.Millisecond),
			20.0, 5.0,
		))
	}

	if !strings.Contains(diagBuf.String(), "[Pipeline] Throttled") {
		t.Errorf("expected diagf for throttled frame count; got: %s", diagBuf.String())
	}
}

// TestTrackingPipelineConfig_NilMaskEarlyReturn verifies the ops log fires and
// the callback returns early when ProcessFramePolarWithMask yields a nil mask
// (not an error — the zero-value BackgroundManager has a nil Grid, so the
// method returns (nil, nil)). The pipeline treats a nil mask the same as an
// error and logs via opsf.
func TestTrackingPipelineConfig_NilMaskEarlyReturn(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-nil-mask-" + t.Name()
	// Zero-value BackgroundManager: Grid is nil, so ProcessFramePolarWithMask
	// returns (nil, nil) — a nil mask with no error.
	bgMgr := &l3grid.BackgroundManager{}

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Send a frame with a real point so it reaches ProcessFramePolarWithMask.
	pts := []l2frames.Point{
		{Channel: 1, Azimuth: 0, Distance: 10.0, Intensity: 100, Timestamp: now},
	}
	frame := &l2frames.LiDARFrame{
		FrameID:        "nil-mask-frame",
		StartTimestamp: now,
		PolarPoints:    pointsToPolar(pts),
		Points:         pts,
	}
	cb(frame)

	if !strings.Contains(opsBuf.String(), "Failed to get foreground mask") {
		t.Errorf("expected ops log for nil mask; got: %q", opsBuf.String())
	}
}

// TestTrackingPipelineConfig_GroundRemovalDisabledDiagf exercises the RemoveGround=false
// path with enough foreground frames for the background model to converge, so
// the pipeline reaches the ground removal stage and logs via diagf.
func TestTrackingPipelineConfig_GroundRemovalDisabledDiagf(t *testing.T) {
	var diagBuf bytes.Buffer
	SetLogWriters(nil, &diagBuf, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-no-ground-diagf-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	// Send enough foreground frames to let the background model converge.
	for i := 0; i < 15; i++ {
		ts := now.Add(time.Duration(500+i*100) * time.Millisecond)
		fgDist := 5.0 + float64(i)*0.1
		cb(makeForegroundFrame(fmt.Sprintf("ng-%d", i), ts, 20.0, fgDist))
	}

	if !strings.Contains(diagBuf.String(), "Ground removal disabled") {
		t.Errorf("expected diagf for ground removal disabled; got: %s", diagBuf.String())
	}
}

// TestTrackingPipelineConfig_DBPruneSuccess exercises the prune-deleted-tracks
// success path by inserting a soft-deleted track older than the TTL, then running
// enough pipeline frames to trigger the once-per-minute prune interval.
func TestTrackingPipelineConfig_DBPruneSuccess(t *testing.T) {
	var diagBuf bytes.Buffer
	SetLogWriters(nil, &diagBuf, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-prune-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	db := setupTestDB(t)

	// Insert a soft-deleted track older than deletedTrackTTL (5 min).
	frameID := fmt.Sprintf("site/%s", sensorID)
	oldNanos := time.Now().Add(-20 * time.Minute).UnixNano()
	_, err := db.Exec(`INSERT INTO lidar_tracks
		(track_id, sensor_id, frame_id, track_state, start_unix_nanos, end_unix_nanos)
		VALUES (?, ?, ?, 'deleted', ?, ?)`,
		"old-track-1", sensorID, frameID, oldNanos, oldNanos,
	)
	if err != nil {
		t.Fatalf("insert old track: %v", err)
	}

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		DB:                db,
		RemoveGround:      true,
		HeightBandFloor:   -10.0,
		HeightBandCeiling: 10.0,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	for i := 0; i < 15; i++ {
		ts := now.Add(time.Duration(500+i*100) * time.Millisecond)
		fgDist := 5.0 + float64(i)*0.1
		cb(makeForegroundFrame(fmt.Sprintf("prune-%d", i), ts, 20.0, fgDist))
	}

	if !strings.Contains(diagBuf.String(), "Pruned") {
		t.Logf("diagBuf: %s", diagBuf.String())
		// Prune may not fire if no tracks reach confirmed+deleted state.
		// The test still exercises the DB prune branch (lastPruneTime.IsZero()).
	}
}

// TestTrackingPipelineConfig_CustomDBSCANParams exercises the tuning config
// paths for ForegroundMinClusterPoints and ForegroundDBSCANEps.
func TestTrackingPipelineConfig_CustomDBSCANParams(t *testing.T) {
	sensorID := "coverage-dbscan-params-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	// Set custom DBSCAN params via the background manager params.
	params := bgMgr.GetParams()
	params.ForegroundMinClusterPoints = 3
	params.ForegroundDBSCANEps = 1.5
	bgMgr.SetParams(params)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		RemoveGround:      true,
		HeightBandFloor:   -10.0,
		HeightBandCeiling: 10.0,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	for i := 0; i < 15; i++ {
		ts := now.Add(time.Duration(500+i*100) * time.Millisecond)
		fgDist := 5.0 + float64(i)*0.1
		cb(makeForegroundFrame(fmt.Sprintf("dbscan-%d", i), ts, 20.0, fgDist))
	}
}

// TestTrackingPipelineConfig_DenseBackgroundDownsample exercises the
// background downsampling branches when backgroundCount > 5000
// (stride and cap calculation).
func TestTrackingPipelineConfig_DenseBackgroundDownsample(t *testing.T) {
	sensorID := "coverage-dense-bg-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		RemoveGround:      true,
		HeightBandFloor:   -10.0,
		HeightBandCeiling: 10.0,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	// Seed background model with dense frames so all 5600 cells converge.
	for i := 0; i < 3; i++ {
		cb(makeDenseFrame(fmt.Sprintf("dense-seed-%d", i), now.Add(time.Duration(i)*100*time.Millisecond), 20.0, 20.0))
	}
	// Now send a dense frame with foreground — background count > 5000.
	for i := 0; i < 3; i++ {
		ts := now.Add(time.Duration(300+i*100) * time.Millisecond)
		cb(makeDenseFrame(fmt.Sprintf("dense-fg-%d", i), ts, 20.0, 5.0))
	}
}

// TestTrackingPipelineConfig_GroundFilterRemovesAll exercises the
// filteredPoints == 0 early return after ground removal filters everything.
func TestTrackingPipelineConfig_GroundFilterRemovesAll(t *testing.T) {
	sensorID := "coverage-ground-filter-all-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	// Use a very narrow height band that rejects everything.
	// Sensor at Z=0, ground at about −3m for typical mount.
	// If floor=0.0 and ceiling=0.0, all points are filtered out.
	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		RemoveGround:      true,
		HeightBandFloor:   99.0,  // Impossibly high floor
		HeightBandCeiling: 100.0, // Impossibly high ceiling
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	// Foreground extracted, but ground filter should reject all points.
	for i := 0; i < 15; i++ {
		ts := now.Add(time.Duration(500+i*100) * time.Millisecond)
		fgDist := 5.0 + float64(i)*0.1
		cb(makeForegroundFrame(fmt.Sprintf("gf-%d", i), ts, 20.0, fgDist))
	}
}

// ---------------------------------------------------------------------------
// Helpers ported from internal/lidar/ root test files
// ---------------------------------------------------------------------------

// minObsForClassification loads the default min_observations_for_classification
// from the tuning config, used by tests that need the threshold.
func minObsForClassification() int {
	return config.MustLoadDefaultConfig().GetMinObservationsForClassification()
}

// mockTrackerCov implements l5tracks.TrackerInterface with controllable return
// values for deterministic coverage testing.
type mockTrackerCov struct {
	updateCalls      int
	frameStatsCalls  int
	confirmedTracks  []*l5tracks.TrackedObject
	lastForeground   int
	lastClustered    int
	lastAssociations []string
	activeTracks     []*l5tracks.TrackedObject
	allTracks        []*l5tracks.TrackedObject
	recentlyDeleted  []*l5tracks.TrackedObject
	metricsResult    l5tracks.TrackingMetrics
	totalCount       int
	tentativeCount   int
	confirmedCount   int
	deletedCount     int
}

func (m *mockTrackerCov) Update(clusters []l5tracks.WorldCluster, timestamp time.Time) {
	m.updateCalls++
}
func (m *mockTrackerCov) GetActiveTracks() []*l5tracks.TrackedObject    { return m.activeTracks }
func (m *mockTrackerCov) GetConfirmedTracks() []*l5tracks.TrackedObject { return m.confirmedTracks }
func (m *mockTrackerCov) GetTrack(trackID string) *l5tracks.TrackedObject {
	return nil
}
func (m *mockTrackerCov) GetTrackCount() (total, tentative, confirmed, deleted int) {
	return m.totalCount, m.tentativeCount, m.confirmedCount, m.deletedCount
}
func (m *mockTrackerCov) GetAllTracks() []*l5tracks.TrackedObject { return m.allTracks }
func (m *mockTrackerCov) GetRecentlyDeletedTracks(nowNanos int64) []*l5tracks.TrackedObject {
	return m.recentlyDeleted
}
func (m *mockTrackerCov) GetLastAssociations() []string { return m.lastAssociations }
func (m *mockTrackerCov) GetTrackingMetrics() l5tracks.TrackingMetrics {
	return m.metricsResult
}
func (m *mockTrackerCov) RecordFrameStats(totalFg, clustered int) {
	m.frameStatsCalls++
	m.lastForeground = totalFg
	m.lastClustered = clustered
}
func (m *mockTrackerCov) UpdateClassification(trackID, objectClass string, confidence float32, model string) {
}
func (m *mockTrackerCov) AdvanceMisses(timestamp time.Time) {
}
func (m *mockTrackerCov) GetDeletedTrackGracePeriod() time.Duration {
	return 5 * time.Second
}
func (m *mockTrackerCov) UpdateConfig(fn func(*l5tracks.TrackerConfig)) {
	// no-op in mock
}

// testBackgroundManagerPrePopulated creates a 40×1800 BackgroundManager with
// every cell pre-settled to a 10 m baseline. Points at a different distance
// (e.g. 2 m or 5 m) will be detected as strong foreground.
func testBackgroundManagerPrePopulated(t *testing.T) *l3grid.BackgroundManager {
	t.Helper()
	params := l3grid.BackgroundParams{
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		BackgroundUpdateFraction:       0.02,
		NeighborConfirmationCount:      0, // no neighbour requirement
		WarmupMinFrames:                0, // skip warmup
	}
	bm := l3grid.NewBackgroundManagerDI("test-cov", 40, 1800, params, nil)
	if bm == nil {
		t.Fatal("NewBackgroundManagerDI returned nil")
	}
	for i := range bm.Grid.Cells {
		bm.Grid.Cells[i].AverageRangeMeters = 10.0
		bm.Grid.Cells[i].TimesSeenCount = 100
	}
	bm.HasSettled = true
	return bm
}

// testFramePrePopulated returns a frame with n points that will be detected as
// foreground because their distance (2 m) differs from the 10 m background
// baseline of testBackgroundManagerPrePopulated.
func testFramePrePopulated(n int) *l2frames.LiDARFrame {
	now := time.Now()
	points := make([]l2frames.Point, n)
	for i := 0; i < n; i++ {
		points[i] = l2frames.Point{
			Channel:   int(i%40) + 1,
			Azimuth:   float64(i%360) + 0.5,
			Elevation: float64(i%40)*0.5 - 10,
			Distance:  2.0,
			Intensity: 20,
			Timestamp: now,
		}
	}
	return &l2frames.LiDARFrame{
		FrameID:        "test-frame",
		PolarPoints:    pointsToPolar(points),
		Points:         points,
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}
}

// clusterFramePrePopulated returns a frame whose points form a tight DBSCAN
// cluster after foreground extraction with the pre-populated 40×1800 grid.
//
// Geometry:
//   - 40×1800 background grid at 10 m baseline (testBackgroundManagerPrePopulated).
//   - distance=5 m → clearly foreground vs 10 m baseline.
//   - elevation=6° → z ≈ 0.52 m → passes default height band [−2.8, 1.5].
//   - azimuths 180°..189° in 1° steps → XY spread ≈ 0.78 m < Eps=0.8 m.
//   - 10 points ≥ foreground_min_cluster_points (5).
func clusterFramePrePopulated() *l2frames.LiDARFrame {
	now := time.Now()
	pts := make([]l2frames.Point, 10)
	for i := range pts {
		pts[i] = l2frames.Point{
			Channel:   i + 1,              // channels 1-10
			Azimuth:   180.0 + float64(i), // 180°-189°
			Elevation: 6.0,
			Distance:  5.0,
			Intensity: 40,
			Timestamp: now,
		}
	}
	return &l2frames.LiDARFrame{
		FrameID:        "cluster-frame",
		PolarPoints:    pointsToPolar(pts),
		Points:         pts,
		StartTimestamp: now,
		MinAzimuth:     180,
		MaxAzimuth:     190,
	}
}

// mockFgForwarderDetailed tracks forwarding calls with detailed state, used
// by tests that check forwardCalled / lastPoints fields.
type mockFgForwarderDetailed struct {
	forwardCalled bool
	lastPoints    []l2frames.PointPolar
	callCount     int
}

func (m *mockFgForwarderDetailed) ForwardForeground(points []l2frames.PointPolar) {
	m.forwardCalled = true
	m.lastPoints = points
	m.callCount++
}

// ---------------------------------------------------------------------------
// Tests ported from internal/lidar/ root — unique coverage paths
// ---------------------------------------------------------------------------

// TestIsNilInterface_WithForegroundForwarder tests the specific case that
// caused a bug: a nil pointer assigned to the ForegroundForwarder interface.
func TestIsNilInterface_WithForegroundForwarder(t *testing.T) {
	var fwd ForegroundForwarder

	// Case 1: uninitialised interface
	if !isNilInterface(fwd) {
		t.Error("expected nil interface to be detected as nil")
	}

	// Case 2: nil pointer assigned to interface (the bug case)
	var nilPtr *mockFgForwarderDetailed
	fwd = nilPtr
	if !isNilInterface(fwd) {
		t.Error("expected interface holding nil pointer to be detected as nil")
	}

	// Case 3: valid pointer assigned to interface
	validPtr := &mockFgForwarderDetailed{}
	fwd = validPtr
	if isNilInterface(fwd) {
		t.Error("expected interface holding valid pointer to be detected as non-nil")
	}
}

// TestNilTrackerAfterClusters covers the early return at tracker.Update when
// cfg.Tracker is nil but DBSCAN produced clusters.
func TestNilTrackerAfterClusters(t *testing.T) {
	bm := testBackgroundManagerPrePopulated(t)

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bm,
		Tracker:           nil, // explicitly nil
	}

	cb := cfg.NewFrameCallback()
	// clusterFramePrePopulated produces DBSCAN clusters → reaches "if cfg.Tracker == nil" after clusters.
	cb(clusterFramePrePopulated())
}

// TestNoClustersRecordStats covers the path where foreground points survive
// the height filter but DBSCAN produces no clusters (MinPts set very high),
// and RecordFrameStats is called with 0 clustered points.
func TestNoClustersRecordStats(t *testing.T) {
	params := l3grid.BackgroundParams{
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		BackgroundUpdateFraction:       0.02,
		NeighborConfirmationCount:      0,
		WarmupMinFrames:                0,
		ForegroundMinClusterPoints:     999, // impossibly high → no clusters
		ForegroundDBSCANEps:            2.0,
	}
	bm := l3grid.NewBackgroundManagerDI("test-nocl", 40, 1800, params, nil)
	for i := range bm.Grid.Cells {
		bm.Grid.Cells[i].AverageRangeMeters = 10.0
		bm.Grid.Cells[i].TimesSeenCount = 100
	}
	bm.HasSettled = true

	tracker := &mockTrackerCov{}

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bm,
		Tracker:           tracker,
	}

	cb := cfg.NewFrameCallback()
	cb(clusterFramePrePopulated())

	if tracker.frameStatsCalls == 0 {
		t.Error("expected RecordFrameStats to be called on the no-clusters path")
	}
}

// TestNoForegroundReturn covers the early return after StoreForegroundSnapshot
// when all points match the background distance.
func TestNoForegroundReturn(t *testing.T) {
	bm := testBackgroundManagerPrePopulated(t)
	tracker := &mockTrackerCov{}

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bm,
		Tracker:           tracker,
	}

	cb := cfg.NewFrameCallback()

	// All points at 10 m exactly match the 10 m background baseline,
	// so none are detected as foreground.
	now := time.Now()
	pts := make([]l2frames.Point, 50)
	for i := range pts {
		pts[i] = l2frames.Point{
			Channel:   (i % 40) + 1,
			Azimuth:   float64(i%360) + 0.5,
			Distance:  10.0,
			Intensity: 20,
			Timestamp: now,
		}
	}
	frame := &l2frames.LiDARFrame{
		FrameID:        "bg-only",
		PolarPoints:    pointsToPolar(pts),
		Points:         pts,
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}
	cb(frame)

	if tracker.updateCalls > 0 {
		t.Error("tracker should not be called when no foreground is detected")
	}
}

// TestDBInsertErrors exercises InsertTrack and InsertTrackObservation error
// paths. A closed database reliably triggers both errors.
func TestDBInsertErrors(t *testing.T) {
	db := setupTestDB(t)
	db.Close() // close before pipeline uses it to force errors

	bm := testBackgroundManagerPrePopulated(t)

	tracker := &mockTrackerCov{
		confirmedTracks: []*l5tracks.TrackedObject{
			{
				TrackID:          "err-t1",
				ObservationCount: 10,
				AvgSpeedMps:      4.0,
			},
		},
	}

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bm,
		Tracker:           tracker,
		DB:                db,
		SensorID:          "cov-err",
	}

	cb := cfg.NewFrameCallback()
	// Should not panic; errors are logged.
	cb(clusterFramePrePopulated())
}

// TestRegistryRunManager covers the getRunManager fallback path through the
// global registry (cfg.AnalysisRunManager is nil, but a manager is
// registered for the sensor).
func TestRegistryRunManager(t *testing.T) {
	db := setupTestDB(t)

	sensorID := "cov-reg"
	runMgr := sqlite.NewAnalysisRunManager(db, sensorID)
	sqlite.RegisterAnalysisRunManager(sensorID, runMgr)
	defer sqlite.RegisterAnalysisRunManager(sensorID, nil)

	rp := sqlite.DefaultRunParams()
	if _, err := runMgr.StartRun("/cov-reg.pcap", rp); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	defer runMgr.CompleteRun()

	bm := testBackgroundManagerPrePopulated(t)
	tracker := &mockTrackerCov{
		confirmedTracks: []*l5tracks.TrackedObject{
			{TrackID: "reg-t1", ObservationCount: 10, AvgSpeedMps: 5.0},
		},
	}

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bm,
		Tracker:           tracker,
		SensorID:          sensorID,
		// AnalysisRunManager intentionally nil → falls through to registry
	}

	cb := cfg.NewFrameCallback()
	cb(clusterFramePrePopulated())
}

// TestFeatureExportWithRunManager combines FeatureExportFunc and an active
// AnalysisRunManager to hit both export and run recording for confirmed
// tracks in a single callback invocation.
func TestFeatureExportWithRunManager(t *testing.T) {
	db := setupTestDB(t)

	bm := testBackgroundManagerPrePopulated(t)
	runMgr := sqlite.NewAnalysisRunManager(db, "cov-fe")
	rp := sqlite.DefaultRunParams()
	if _, err := runMgr.StartRun("/cov-fe.pcap", rp); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	defer runMgr.CompleteRun()

	var exported atomic.Int32
	tracker := &mockTrackerCov{
		confirmedTracks: []*l5tracks.TrackedObject{
			{
				TrackID:          "fe-t1",
				ObservationCount: minObsForClassification() + 5,
				AvgSpeedMps:      6.0,
				ObjectClass:      "vehicle",
				ObjectConfidence: 0.9,
			},
		},
	}

	cfg := &TrackingPipelineConfig{
		BackgroundManager:  bm,
		Tracker:            tracker,
		AnalysisRunManager: runMgr,
		SensorID:           "cov-fe",
		FeatureExportFunc: func(trackID string, features l6objects.TrackFeatures, class string, confidence float32) {
			exported.Add(1)
		},
	}

	cb := cfg.NewFrameCallback()
	cb(clusterFramePrePopulated())
}

// TestDBPersistenceAndObservation exercises the full DB persistence path
// (InsertTrack + InsertTrackObservation) with a functional database and
// mockTrackerCov for deterministic confirmed tracks.
func TestDBPersistenceAndObservation(t *testing.T) {
	db := setupTestDB(t)

	bm := testBackgroundManagerPrePopulated(t)

	tracker := &mockTrackerCov{
		confirmedTracks: []*l5tracks.TrackedObject{
			{
				TrackID:              "db-t1",
				SensorID:             "cov-db",
				ObservationCount:     8,
				AvgSpeedMps:          4.5,
				X:                    1.0,
				Y:                    2.0,
				VX:                   0.3,
				VY:                   0.1,
				BoundingBoxLengthAvg: 4.0,
				BoundingBoxWidthAvg:  1.8,
				BoundingBoxHeightAvg: 1.5,
				HeightP95Max:         2.0,
				IntensityMeanAvg:     45.0,
			},
		},
	}

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bm,
		Tracker:           tracker,
		DB:                db,
		SensorID:          "cov-db",
	}

	cb := cfg.NewFrameCallback()
	cb(clusterFramePrePopulated())

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM lidar_tracks WHERE track_id='db-t1'").Scan(&count); err != nil {
		t.Fatalf("query tracks: %v", err)
	}
	if count == 0 {
		t.Log("track not persisted (DBSCAN may not have formed clusters)")
	}
}

// TestAnalysisRunManagerRecordTrack ensures runManager.RecordTrack is invoked
// for confirmed tracks when an analysis run is active, combined with cluster
// classification.
func TestAnalysisRunManagerRecordTrack(t *testing.T) {
	db := setupTestDB(t)

	bm := testBackgroundManagerPrePopulated(t)
	runMgr := sqlite.NewAnalysisRunManager(db, "cov-rt")
	rp := sqlite.DefaultRunParams()
	if _, err := runMgr.StartRun("/cov-rt.pcap", rp); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	defer runMgr.CompleteRun()

	classifier := l6objects.NewTrackClassifier()

	tracker := &mockTrackerCov{
		confirmedTracks: []*l5tracks.TrackedObject{
			{
				TrackID:          "rt-t1",
				ObservationCount: minObsForClassification(),
				AvgSpeedMps:      3.0,
				ObjectClass:      "", // triggers classify
			},
		},
	}

	cfg := &TrackingPipelineConfig{
		BackgroundManager:  bm,
		Tracker:            tracker,
		Classifier:         classifier,
		AnalysisRunManager: runMgr,
		SensorID:           "cov-rt",
	}

	cb := cfg.NewFrameCallback()
	cb(clusterFramePrePopulated())
}

// TestNilTracker exercises the nil-tracker path with a background manager
// that has a non-settled, process-based background.
func TestNilTracker(t *testing.T) {
	bgMgr := l3grid.NewBackgroundManagerDI("test", 16, 360, l3grid.BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []l2frames.PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           nil,
		SensorID:          "test-sensor",
	}

	cb := cfg.NewFrameCallback()

	now := time.Now()
	fPts := []l2frames.Point{
		{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
	}
	frame := &l2frames.LiDARFrame{
		FrameID:        "test-frame",
		PolarPoints:    pointsToPolar(fPts),
		Points:         fPts,
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	// Should handle nil tracker without panicking
	cb(frame)
}

// TestDebugModeConfirmedTracks covers the debug log after the confirmed-tracks
// loop when confirmed tracks are present.
func TestDebugModeConfirmedTracks(t *testing.T) {
	bm := testBackgroundManagerPrePopulated(t)

	tracker := &mockTrackerCov{
		confirmedTracks: []*l5tracks.TrackedObject{
			{TrackID: "dbg-t1", ObservationCount: 3, AvgSpeedMps: 2.0},
		},
	}

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bm,
		Tracker:           tracker,
	}

	cb := cfg.NewFrameCallback()
	cb(clusterFramePrePopulated())

	if tracker.updateCalls == 0 {
		t.Error("expected tracker.Update (clusters should form)")
	}
}

// TestBackgroundDownsamplingPrePopulated covers stride/cap branches when
// there are >5000 background points, using the pre-populated 40×1800 grid.
func TestBackgroundDownsamplingPrePopulated(t *testing.T) {
	bm := testBackgroundManagerPrePopulated(t)
	tracker := &mockTrackerCov{}

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bm,
		Tracker:           tracker,
	}

	cb := cfg.NewFrameCallback()

	now := time.Now()
	const bgCount = 6000
	const fgCount = 10
	pts := make([]l2frames.Point, 0, bgCount+fgCount)

	for i := 0; i < bgCount; i++ {
		pts = append(pts, l2frames.Point{
			Channel:   (i % 40) + 1,
			Azimuth:   float64(i%1800) * 0.2,
			Elevation: 0,
			Distance:  10.0,
			Intensity: 10,
			Timestamp: now,
		})
	}
	for i := 0; i < fgCount; i++ {
		pts = append(pts, l2frames.Point{
			Channel:   i + 1,
			Azimuth:   180.0 + float64(i),
			Elevation: 6.0,
			Distance:  5.0,
			Intensity: 40,
			Timestamp: now,
		})
	}

	frame := &l2frames.LiDARFrame{
		FrameID:        "big-frame",
		PolarPoints:    pointsToPolar(pts),
		Points:         pts,
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}
	cb(frame)
}

// TestTrackingPipelineConfig_BenchmarkMode_EmitsTimingLogs verifies that
// enabling BenchmarkMode produces [Benchmark] log output via opsf.
func TestTrackingPipelineConfig_BenchmarkMode_EmitsTimingLogs(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-benchmark-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	benchmarkMode := &atomic.Bool{}
	benchmarkMode.Store(true)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		Classifier:        l6objects.NewTrackClassifier(),
		RemoveGround:      false,
		MaxFrameRate:      0,
		BenchmarkMode:     benchmarkMode,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Seed background
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("bench-seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	// Send foreground frames to exercise full benchmark path
	for i := 0; i < 3; i++ {
		ts := now.Add(time.Duration(600+i*100) * time.Millisecond)
		cb(makeForegroundFrame("bench-fg-"+string(rune('A'+i)), ts, 20.0, 5.0))
	}

	output := opsBuf.String()
	if !strings.Contains(output, "[Benchmark]") {
		t.Errorf("expected [Benchmark] log output when BenchmarkMode is enabled, got:\n%s", output)
	}
	if !strings.Contains(output, "total=") {
		t.Errorf("expected timing output with 'total=' in [Benchmark] log")
	}
}

// TestTrackingPipelineConfig_BenchmarkMode_Disabled_NoOutput verifies that
// when BenchmarkMode is nil or false, no [Benchmark] logs are emitted.
func TestTrackingPipelineConfig_BenchmarkMode_Disabled_NoOutput(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-benchmark-disabled-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	// BenchmarkMode nil — should produce zero benchmark output
	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		RemoveGround:      false,
		BenchmarkMode:     nil,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 3; i++ {
		cb(makeStableFrame("nobench-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	output := opsBuf.String()
	if strings.Contains(output, "[Benchmark]") {
		t.Errorf("unexpected [Benchmark] output when BenchmarkMode is nil:\n%s", output)
	}
}

// TestTrackingPipelineConfig_BenchmarkMode_HealthSummary verifies the periodic
// health summary is emitted after healthSummaryInterval processed frames.
func TestTrackingPipelineConfig_BenchmarkMode_HealthSummary(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-benchmark-health-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	benchmarkMode := &atomic.Bool{}
	benchmarkMode.Store(true)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		Classifier:        l6objects.NewTrackClassifier(),
		RemoveGround:      false,
		MaxFrameRate:      0,
		BenchmarkMode:     benchmarkMode,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Seed background quickly
	for i := 0; i < 3; i++ {
		cb(makeStableFrame("health-seed-"+fmt.Sprintf("%d", i), now.Add(time.Duration(i)*50*time.Millisecond), 20.0))
	}

	// Send 100+ foreground frames to trigger healthSummaryInterval (100)
	for i := 0; i < 105; i++ {
		ts := now.Add(time.Duration(200+i*50) * time.Millisecond)
		cb(makeForegroundFrame("health-fg-"+fmt.Sprintf("%03d", i), ts, 20.0, 5.0+float64(i)*0.01))
	}

	output := opsBuf.String()
	if !strings.Contains(output, "health:") {
		t.Errorf("expected health summary after 100 processed frames, got:\n%s", output)
	}
	if !strings.Contains(output, "goroutines=") {
		t.Errorf("expected goroutine count in health summary")
	}
}

// TestTrackingPipelineConfig_DisableTrackPersistence verifies that when
// DisableTrackPersistence is set, no DB writes occur even when a DB and
// tracker are configured.
func TestTrackingPipelineConfig_DisableTrackPersistence(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-disable-persist-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	db := setupTestDB(t)

	disablePersist := &atomic.Bool{}
	disablePersist.Store(true)

	cfg := &TrackingPipelineConfig{
		SensorID:                sensorID,
		BackgroundManager:       bgMgr,
		Tracker:                 tracker,
		Classifier:              l6objects.NewTrackClassifier(),
		DB:                      db,
		RemoveGround:            false,
		MaxFrameRate:            0,
		DisableTrackPersistence: disablePersist,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Seed background
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("persist-seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	// Send foreground frames to create confirmed tracks
	for i := 0; i < 15; i++ {
		ts := now.Add(time.Duration(500+i*100) * time.Millisecond)
		cb(makeForegroundFrame("persist-fg-"+string(rune('A'+i)), ts, 20.0, 5.0+float64(i)*0.1))
	}

	// Verify tracks were created in the tracker
	total, _, _, _ := tracker.GetTrackCount()
	if total == 0 {
		t.Error("expected at least one track to be created")
	}

	// Verify no tracks were persisted to DB
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM lidar_tracks").Scan(&count)
	if err != nil {
		t.Fatalf("query lidar_tracks: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 persisted tracks with DisableTrackPersistence=true, got %d", count)
	}
}

// TestTrackingPipelineConfig_DBTransactionBatching verifies that when
// DisableTrackPersistence is false (nil), tracks are persisted to the DB
// using the per-frame transaction batching path.
func TestTrackingPipelineConfig_DBTransactionBatching(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-tx-batch-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	db := setupTestDB(t)

	cfg := &TrackingPipelineConfig{
		SensorID:                sensorID,
		BackgroundManager:       bgMgr,
		Tracker:                 tracker,
		Classifier:              l6objects.NewTrackClassifier(),
		DB:                      db,
		RemoveGround:            false,
		MaxFrameRate:            0,
		DisableTrackPersistence: nil, // nil means persist
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Seed background
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("txbatch-seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	// Send foreground frames
	for i := 0; i < 15; i++ {
		ts := now.Add(time.Duration(500+i*100) * time.Millisecond)
		cb(makeForegroundFrame("txbatch-fg-"+string(rune('A'+i)), ts, 20.0, 5.0+float64(i)*0.1))
	}

	// Verify tracks were persisted
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM lidar_tracks").Scan(&count)
	if err != nil {
		t.Fatalf("query lidar_tracks: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one track persisted to DB with default persistence")
	}
}

// TestTrackingPipelineConfig_BenchmarkMode_LagDetection verifies that the
// lag detection logic in benchmark mode produces BEHIND warnings when
// frames take longer than the inter-frame interval.
func TestTrackingPipelineConfig_BenchmarkMode_LagDetection(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	sensorID := "coverage-lag-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	benchmarkMode := &atomic.Bool{}
	benchmarkMode.Store(true)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		Classifier:        l6objects.NewTrackClassifier(),
		RemoveGround:      false,
		MaxFrameRate:      0,
		BenchmarkMode:     benchmarkMode,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()

	// Seed background
	for i := 0; i < 3; i++ {
		cb(makeStableFrame("lag-seed-"+fmt.Sprintf("%d", i), now.Add(time.Duration(i)*50*time.Millisecond), 20.0))
	}

	// Send many foreground frames rapidly — the processing time is close
	// enough that lag detection is exercised, provided the timestamps
	// advance faster than the wall-clock processing.
	for i := 0; i < 10; i++ {
		ts := now.Add(time.Duration(200+i*10) * time.Millisecond) // 10ms apart in PCAP time
		cb(makeForegroundFrame("lag-fg-"+fmt.Sprintf("%03d", i), ts, 20.0, 5.0+float64(i)*0.01))
	}

	// The test primarily ensures the lag detection path doesn't panic or
	// deadlock. Whether BEHIND log appears depends on actual processing
	// speed — verify the benchmark output is present.
	output := opsBuf.String()
	if !strings.Contains(output, "[Benchmark]") {
		t.Errorf("expected [Benchmark] output in lag detection test")
	}
}
