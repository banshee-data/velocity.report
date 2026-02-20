package pipeline

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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

	frame := &l2frames.LiDARFrame{
		FrameID: "test-frame",
		Points: []l2frames.Point{
			{Channel: 1, Azimuth: 90.0, Distance: 5.0, Intensity: 80, Timestamp: time.Now()},
		},
	}

	// Should not panic; exits after polar conversion when BackgroundManager is nil.
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
		Points:         points,
	}
}

// makeForegroundFrame creates a frame with stable background + a tight cluster
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
		Points:         points,
	}
}

// setupTestDB creates a temporary SQLite database for pipeline testing.
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
		DebugMode:         true,
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
		DebugMode:         true,
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
		DebugMode:         true,
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
	points []l4perception.PointPolar
}

func (m *mockFgForwarder) ForwardForeground(points []l4perception.PointPolar) {
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
	adaptCalls int
}

func (m *mockVisualiserAdapter) AdaptFrame(frame *l2frames.LiDARFrame, foregroundMask []bool, clusters []l4perception.WorldCluster, tracker l5tracks.TrackerInterface, debugFrame interface{}) interface{} {
	m.adaptCalls++
	return struct{}{}
}

// mockLidarViewAdapter implements LidarViewAdapter for testing.
type mockLidarViewAdapter struct {
	calls int
}

func (m *mockLidarViewAdapter) PublishFrameBundle(bundle interface{}, foregroundPoints []l4perception.PointPolar) {
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
		DebugMode:         true, // hits the "FgForwarder is nil" debug log
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
		DebugMode:           true,
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
		DebugMode:          true,
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
		DebugMode:         true,
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
