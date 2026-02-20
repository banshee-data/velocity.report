package pipeline

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
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
		Points: []l2frames.LiDARPoint{
			{Channel: 1, Azimuth: 90.0, Distance: 5.0, Intensity: 80, Timestamp: time.Now()},
		},
	}

	// Should not panic; exits after polar conversion when BackgroundManager is nil.
	cb(frame)
}

// TestTrackingPipelineConfig_NewFrameCallback_WithBackgroundManager tests the pipeline
// up to the foreground extraction stage (no tracker).
func TestTrackingPipelineConfig_NewFrameCallback_WithBackgroundManager(t *testing.T) {
	sensorID := "coverage-test-" + t.Name()
	bgMgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation: true,
	}, nil)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		RemoveGround:      true,
	}
	cb := cfg.NewFrameCallback()

	// Feed a minimal frame — background manager exists but no tracker.
	now := time.Now()
	frame := &l2frames.LiDARFrame{
		FrameID:        "frame-1",
		StartTimestamp: now,
		Points: []l2frames.LiDARPoint{
			{Channel: 1, Azimuth: 90.0, Distance: 5.0, Intensity: 80, Timestamp: now},
			{Channel: 2, Azimuth: 91.0, Distance: 5.1, Intensity: 78, Timestamp: now},
		},
	}

	// Should not panic; processes through foreground extraction.
	cb(frame)
}

// TestTrackingPipelineConfig_NewFrameCallback_FullPipeline tests the entire pipeline.
func TestTrackingPipelineConfig_NewFrameCallback_FullPipeline(t *testing.T) {
	sensorID := "coverage-full-" + t.Name()
	bgMgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation: true,
	}, nil)

	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	featureExportCalled := false
	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		DebugMode:         true,
		RemoveGround:      true,
		HeightBandFloor:   -5.0,
		HeightBandCeiling: 5.0,
		MaxFrameRate:      0, // no throttling
		VoxelLeafSize:     0.08,
		FeatureExportFunc: func(trackID string, features l6objects.TrackFeatures, class string, confidence float32) {
			featureExportCalled = true
		},
	}
	cb := cfg.NewFrameCallback()

	// Feed several frames to seed the background and generate foreground.
	now := time.Now()
	for i := 0; i < 5; i++ {
		points := make([]l2frames.LiDARPoint, 50)
		for j := range points {
			points[j] = l2frames.LiDARPoint{
				Channel:   uint8(j%16 + 1),
				Azimuth:   float64(j) * 7.2,
				Distance:  10.0 + float64(j)*0.1,
				Intensity: 80,
				Timestamp: now.Add(time.Duration(i)*100*time.Millisecond + time.Duration(j)*time.Microsecond),
			}
		}

		frame := &l2frames.LiDARFrame{
			FrameID:        "frame-" + string(rune('A'+i)),
			StartTimestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			Points:         points,
		}
		cb(frame)
	}

	_, _, confirmed, _ := tracker.GetTrackCount()
	t.Logf("Background seeded, confirmed tracks: %d", confirmed)
}

// TestTrackingPipelineConfig_NewFrameCallback_Throttling tests frame rate throttling.
func TestTrackingPipelineConfig_NewFrameCallback_Throttling(t *testing.T) {
	sensorID := "coverage-throttle-" + t.Name()
	bgMgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation: true,
	}, nil)

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
	for i := 0; i < 10; i++ {
		frame := &l2frames.LiDARFrame{
			FrameID:        "throttle-frame",
			StartTimestamp: now.Add(time.Duration(i) * 10 * time.Millisecond),
			Points: []l2frames.LiDARPoint{
				{Channel: 1, Azimuth: 90.0, Distance: 5.0, Intensity: 80, Timestamp: now},
			},
		}
		cb(frame)
	}
}

// TestTrackingPipelineConfig_NewFrameCallback_DebugMode tests debug logging paths.
func TestTrackingPipelineConfig_NewFrameCallback_DebugMode(t *testing.T) {
	sensorID := "coverage-debug-" + t.Name()
	bgMgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation: true,
	}, nil)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		DebugMode:         true,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	frame := &l2frames.LiDARFrame{
		FrameID:        "debug-frame",
		StartTimestamp: now,
		Points: []l2frames.LiDARPoint{
			{Channel: 1, Azimuth: 90.0, Distance: 5.0, Intensity: 80, Timestamp: now},
		},
	}
	cb(frame)
}

// TestTrackingPipelineConfig_NewFrameCallback_NoGroundRemoval verifies RemoveGround=false path.
func TestTrackingPipelineConfig_NewFrameCallback_NoGroundRemoval(t *testing.T) {
	sensorID := "coverage-no-ground-" + t.Name()
	bgMgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation: true,
	}, nil)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		RemoveGround:      false,
		DebugMode:         true,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	frame := &l2frames.LiDARFrame{
		FrameID:        "no-ground-frame",
		StartTimestamp: now,
		Points: []l2frames.LiDARPoint{
			{Channel: 1, Azimuth: 90.0, Distance: 5.0, Intensity: 80, Timestamp: now},
		},
	}
	cb(frame)
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
	bgMgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation: true,
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
	// Feed enough frames to generate foreground points
	for i := 0; i < 5; i++ {
		points := make([]l2frames.LiDARPoint, 20)
		for j := range points {
			points[j] = l2frames.LiDARPoint{
				Channel:   uint8(j%16 + 1),
				Azimuth:   float64(j) * 18.0,
				Distance:  float64(5 + i),
				Intensity: 80,
				Timestamp: now.Add(time.Duration(i*100) * time.Millisecond),
			}
		}

		frame := &l2frames.LiDARFrame{
			FrameID:        "fwd-frame",
			StartTimestamp: now.Add(time.Duration(i*100) * time.Millisecond),
			Points:         points,
		}
		cb(frame)
	}
	t.Logf("Foreground forwarder calls: %d, points: %d", fwd.calls, len(fwd.points))
}

// TestTrackingPipelineConfig_WithNilFgForwarder tests nil forwarder interface path.
func TestTrackingPipelineConfig_WithNilFgForwarder(t *testing.T) {
	sensorID := "coverage-nil-fwd-" + t.Name()
	bgMgr := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation: true,
	}, nil)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		FgForwarder:       nil,
		DebugMode:         true,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	frame := &l2frames.LiDARFrame{
		FrameID:        "nil-fwd",
		StartTimestamp: now,
		Points: []l2frames.LiDARPoint{
			{Channel: 1, Azimuth: 90.0, Distance: 5.0, Intensity: 80, Timestamp: now},
		},
	}
	cb(frame)
}
