package l2frames

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFrameAzimuthCoverage_NilFrame(t *testing.T) {
	if cov := frameAzimuthCoverage(nil); cov != 0 {
		t.Fatalf("expected 0, got %f", cov)
	}
}

func TestFrameAzimuthCoverage_Normal(t *testing.T) {
	f := &LiDARFrame{MinAzimuth: 10, MaxAzimuth: 350}
	cov := frameAzimuthCoverage(f)
	if cov != 340 {
		t.Fatalf("expected 340, got %f", cov)
	}
}

func TestFrameAzimuthCoverage_WrapAround(t *testing.T) {
	f := &LiDARFrame{MinAzimuth: 350, MaxAzimuth: 10}
	cov := frameAzimuthCoverage(f)
	if cov != 20 {
		t.Fatalf("expected 20, got %f", cov)
	}
}

func TestSetMotorSpeed_Zero(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-motor"})
	fb.EnableTimeBased(true)
	fb.SetMotorSpeed(0)
	// After setting 0, time-based should be disabled
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if fb.enableTimeBased {
		t.Fatal("expected enableTimeBased to be false after SetMotorSpeed(0)")
	}
	if fb.expectedFrameDuration != 0 {
		t.Fatal("expected 0 expectedFrameDuration")
	}
}

func TestSetMotorSpeed_NonZero(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-motor"})
	fb.SetMotorSpeed(600) // 600 RPM → 100ms per frame
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if !fb.enableTimeBased {
		t.Fatal("expected enableTimeBased to be true")
	}
	expected := time.Duration(60000/600) * time.Millisecond
	if fb.expectedFrameDuration != expected {
		t.Fatalf("expected %v, got %v", expected, fb.expectedFrameDuration)
	}
}

func TestReset(t *testing.T) {
	var receivedCount int
	var mu sync.Mutex
	cb := func(f *LiDARFrame) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
	}
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "reset-test", FrameCallback: cb})

	// Add some points to create state
	nowNanos := time.Now().UnixNano()
	points := make([]PointPolar, 100)
	for i := range points {
		points[i] = PointPolar{
			Distance:  10.0,
			Azimuth:   float64(i),
			Elevation: 0,
			Intensity: 100,
			Timestamp: nowNanos,
			Channel:   1,
		}
	}
	fb.AddPointsPolar(points)

	// Reset should clear everything
	fb.Reset()

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if fb.currentFrame != nil {
		t.Fatal("expected currentFrame to be nil after Reset")
	}
	if len(fb.frameBuffer) != 0 {
		t.Fatal("expected empty frameBuffer after Reset")
	}
	if fb.lastSequence != 0 {
		t.Fatal("expected lastSequence=0 after Reset")
	}
}

func TestCheckSequenceGaps_Initial(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "seq-test"})
	fb.mu.Lock()

	// First sequence - should initialise
	fb.checkSequenceGaps(100)
	if fb.lastSequence != 100 {
		t.Fatalf("expected lastSequence=100, got %d", fb.lastSequence)
	}
	if len(fb.sequenceGaps) != 0 {
		t.Fatal("expected no gaps on first sequence")
	}

	fb.mu.Unlock()
}

func TestCheckSequenceGaps_WithGap(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "seq-gap-test"})
	fb.mu.Lock()

	fb.checkSequenceGaps(100)
	fb.checkSequenceGaps(105) // gap of 101-104

	if len(fb.sequenceGaps) != 4 {
		t.Fatalf("expected 4 gaps, got %d", len(fb.sequenceGaps))
	}
	for seq := uint32(101); seq <= 104; seq++ {
		if !fb.sequenceGaps[seq] {
			t.Fatalf("expected gap at sequence %d", seq)
		}
	}
	if fb.lastSequence != 105 {
		t.Fatalf("expected lastSequence=105, got %d", fb.lastSequence)
	}

	fb.mu.Unlock()
}

func TestCheckSequenceGaps_Consecutive(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "seq-consec-test"})
	fb.mu.Lock()

	fb.checkSequenceGaps(1)
	fb.checkSequenceGaps(2)
	fb.checkSequenceGaps(3)

	if len(fb.sequenceGaps) != 0 {
		t.Fatal("expected no gaps for consecutive sequences")
	}

	fb.mu.Unlock()
}

func TestRegisterFrameBuilder_InvalidInputs(t *testing.T) {
	// Empty sensorID should be a no-op
	RegisterFrameBuilder("", NewFrameBuilder(FrameBuilderConfig{SensorID: "x"}))
	// nil fb should be a no-op
	RegisterFrameBuilder("test", nil)
	// Valid registration
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "reg-test"})
	RegisterFrameBuilder("reg-test", fb)
	got := GetFrameBuilder("reg-test")
	if got != fb {
		t.Fatal("expected registered FrameBuilder to be retrievable")
	}
}

func TestFinalizeFrame_NilFrame(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "fin-nil"})
	// Should not panic
	fb.finalizeFrame(nil, "test")
}

func TestFinalizeFrame_IncompleteFrame(t *testing.T) {
	done := make(chan struct{})
	var received *LiDARFrame
	var mu sync.Mutex
	cb := func(f *LiDARFrame) {
		mu.Lock()
		received = f
		mu.Unlock()
		close(done)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "fin-incomplete", FrameCallback: cb})

	// Create a frame with few points and low azimuth coverage (incomplete)
	frame := &LiDARFrame{
		FrameID:        "test-incomplete",
		SensorID:       "fin-incomplete",
		StartTimestamp: time.Now(),
		EndTimestamp:   time.Now().Add(100 * time.Millisecond),
		Points:         make([]Point, 5),
		MinAzimuth:     0,
		MaxAzimuth:     100,
		PointCount:     5,
	}

	fb.finalizeFrame(frame, "test-incomplete-reason")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback")
	}

	if frame.SpinComplete {
		t.Fatal("expected SpinComplete=false for incomplete frame")
	}
	mu.Lock()
	if received == nil {
		t.Fatal("expected callback to be invoked")
	}
	mu.Unlock()
}

func TestFinalizeFrame_CompleteFrame(t *testing.T) {
	done := make(chan struct{})
	var received *LiDARFrame
	var mu sync.Mutex
	cb := func(f *LiDARFrame) {
		mu.Lock()
		received = f
		mu.Unlock()
		close(done)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "fin-complete", FrameCallback: cb})

	// Create a frame with full coverage and enough points
	points := make([]Point, 15000)
	frame := &LiDARFrame{
		FrameID:        "test-complete",
		SensorID:       "fin-complete",
		StartTimestamp: time.Now(),
		EndTimestamp:   time.Now().Add(100 * time.Millisecond),
		Points:         points,
		MinAzimuth:     0,
		MaxAzimuth:     355,
		PointCount:     15000,
	}

	fb.finalizeFrame(frame, "test-complete-reason")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback")
	}

	if !frame.SpinComplete {
		t.Fatal("expected SpinComplete=true for complete frame")
	}
	mu.Lock()
	if received == nil {
		t.Fatal("expected callback to be invoked")
	}
	mu.Unlock()
}

func TestFinalizeFrame_ExportNextSkipsIncomplete(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "export-skip"})
	fb.exportNextFrameASC = true

	// Incomplete frame - should skip export but keep the flag
	frame := &LiDARFrame{
		FrameID:        "test-skip",
		SensorID:       "export-skip",
		StartTimestamp: time.Now(),
		EndTimestamp:   time.Now().Add(100 * time.Millisecond),
		Points:         make([]Point, 5),
		MinAzimuth:     0,
		MaxAzimuth:     50,
		PointCount:     5,
	}

	fb.finalizeFrame(frame, "test")

	// Flag should still be set (not cleared) since frame was incomplete
	if !fb.exportNextFrameASC {
		t.Fatal("expected exportNextFrameASC to remain true for incomplete frame")
	}
}

func TestFinalizeFrame_ExportBatchSkipsIncomplete(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "batch-skip"})
	fb.exportBatchCount = 3
	fb.exportBatchExported = 0

	// Incomplete frame - should skip batch export
	frame := &LiDARFrame{
		FrameID:        "test-batch-skip",
		SensorID:       "batch-skip",
		StartTimestamp: time.Now(),
		EndTimestamp:   time.Now().Add(100 * time.Millisecond),
		Points:         make([]Point, 5),
		MinAzimuth:     0,
		MaxAzimuth:     50,
		PointCount:     5,
	}

	fb.finalizeFrame(frame, "test")

	// Batch counter should NOT have incremented for incomplete frame
	if fb.exportBatchExported != 0 {
		t.Fatalf("expected exportBatchExported=0 for incomplete, got %d", fb.exportBatchExported)
	}
}

func TestExportFrameToASCInternal_NilFrame(t *testing.T) {
	err := exportFrameToASCInternal(nil)
	if err == nil {
		t.Fatal("expected error for nil frame")
	}
}

func TestExportFrameToASCInternal_EmptyFrame(t *testing.T) {
	frame := &LiDARFrame{Points: []Point{}}
	err := exportFrameToASCInternal(frame)
	if err == nil {
		t.Fatal("expected error for empty frame")
	}
}

func TestExportFrameToASCInternal_WithZeroZ(t *testing.T) {
	oldDir := defaultExportDir
	defaultExportDir = t.TempDir()
	defer func() { defaultExportDir = oldDir }()

	frame := &LiDARFrame{
		FrameID:  "z-zero-test",
		SensorID: "test",
		Points: []Point{
			{X: 1, Y: 2, Z: 0, Distance: 10, Azimuth: 45, Elevation: 10, Intensity: 100},
			{X: 3, Y: 4, Z: 0, Distance: 20, Azimuth: 90, Elevation: 5, Intensity: 200},
		},
		PointCount: 2,
	}
	err := exportFrameToASCInternal(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportFrameToASCInternal_WithNonZeroZ(t *testing.T) {
	oldDir := defaultExportDir
	defaultExportDir = t.TempDir()
	defer func() { defaultExportDir = oldDir }()

	frame := &LiDARFrame{
		FrameID:  "z-nonzero-test",
		SensorID: "test",
		Points: []Point{
			{X: 1, Y: 2, Z: 3, Distance: 10, Azimuth: 45, Elevation: 10, Intensity: 100},
			{X: 4, Y: 5, Z: 6, Distance: 20, Azimuth: 90, Elevation: 5, Intensity: 200},
		},
		PointCount: 2,
	}
	err := exportFrameToASCInternal(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFinalizeFrame_WithExportNext_Complete(t *testing.T) {
	oldDir := defaultExportDir
	defaultExportDir = t.TempDir()
	defer func() { defaultExportDir = oldDir }()

	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "export-complete"})
	fb.exportNextFrameASC = true

	// Create a complete frame (enough coverage and points)
	points := make([]Point, 15000)
	for i := range points {
		points[i] = Point{X: float64(i), Y: float64(i), Z: float64(i), Intensity: 100, Distance: 10, Azimuth: float64(i) * 0.024, Elevation: 5}
	}
	frame := &LiDARFrame{
		FrameID:        "export-next",
		SensorID:       "export-complete",
		StartTimestamp: time.Now(),
		EndTimestamp:   time.Now().Add(100 * time.Millisecond),
		Points:         points,
		MinAzimuth:     0,
		MaxAzimuth:     355,
		PointCount:     15000,
	}

	fb.finalizeFrame(frame, "test-export")
	// Export flag should be cleared after successful export
}

func TestFinalizeFrame_WithBatchExport_Complete(t *testing.T) {
	oldDir := defaultExportDir
	defaultExportDir = t.TempDir()
	defer func() { defaultExportDir = oldDir }()

	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "batch-complete"})
	fb.exportBatchCount = 2
	fb.exportBatchExported = 0

	points := make([]Point, 15000)
	for i := range points {
		points[i] = Point{X: float64(i), Y: float64(i), Z: float64(i), Intensity: 100}
	}
	frame := &LiDARFrame{
		FrameID:        "batch-1",
		SensorID:       "batch-complete",
		StartTimestamp: time.Now(),
		EndTimestamp:   time.Now().Add(100 * time.Millisecond),
		Points:         points,
		MinAzimuth:     0,
		MaxAzimuth:     355,
		PointCount:     15000,
	}

	fb.finalizeFrame(frame, "test-batch-1")

	if fb.exportBatchExported != 1 {
		t.Fatalf("expected exportBatchExported=1, got %d", fb.exportBatchExported)
	}
}

func TestNewFrameBuilderWithDebugLoggingAndInterval_Coverage(t *testing.T) {
	fb := NewFrameBuilderWithDebugLoggingAndInterval("test-dbg", true, 500*time.Millisecond)
	if fb == nil {
		t.Fatal("expected non-nil FrameBuilder")
	}
	if fb.frameCallback == nil {
		t.Fatal("expected non-nil frameCallback when debug=true")
	}
}

func TestExportFrameToASCInternal_WriteDirNotExist(t *testing.T) {
	oldDir := defaultExportDir
	defaultExportDir = "/nonexistent/path/that/should/not/exist"
	defer func() { defaultExportDir = oldDir }()

	frame := &LiDARFrame{
		FrameID:  "fail-test",
		SensorID: "test",
		Points: []Point{
			{X: 1, Y: 2, Z: 3, Distance: 10, Azimuth: 45, Elevation: 10, Intensity: 100},
		},
		PointCount: 1,
	}
	err := exportFrameToASCInternal(frame)
	if err == nil {
		t.Fatal("expected error when export dir does not exist")
	}
	if !strings.Contains(err.Error(), "failed to export ASC") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// --- NewFrameBuilderDI tests ---

func TestNewFrameBuilderDI_DefaultConfig(t *testing.T) {
	var mu sync.Mutex
	var frames []*LiDARFrame

	fb := NewFrameBuilderDI(FrameBuilderConfig{
		SensorID: "test-di",
		FrameCallback: func(f *LiDARFrame) {
			mu.Lock()
			defer mu.Unlock()
			frames = append(frames, f)
		},
	})
	if fb == nil {
		t.Fatal("NewFrameBuilderDI returned nil")
	}
	defer fb.Close()

	// Verify defaults are applied
	if fb.sensorID != "test-di" {
		t.Errorf("sensorID = %q, want %q", fb.sensorID, "test-di")
	}
	if fb.frameBufferSize != 10 {
		t.Errorf("frameBufferSize = %d, want 10", fb.frameBufferSize)
	}
	if fb.azimuthTolerance != 10.0 {
		t.Errorf("azimuthTolerance = %f, want 10.0", fb.azimuthTolerance)
	}
	if fb.minFramePoints != 1000 {
		t.Errorf("minFramePoints = %d, want 1000", fb.minFramePoints)
	}
	if fb.maxBackfillDelay != 100*time.Millisecond {
		t.Errorf("maxBackfillDelay = %v, want 100ms", fb.maxBackfillDelay)
	}
	if fb.bufferTimeout != 1000*time.Millisecond {
		t.Errorf("bufferTimeout = %v, want 1000ms", fb.bufferTimeout)
	}
	if fb.cleanupInterval != 250*time.Millisecond {
		t.Errorf("cleanupInterval = %v, want 250ms", fb.cleanupInterval)
	}

	// Verify NOT registered in global registry
	got := GetFrameBuilder("test-di")
	if got != nil {
		t.Error("expected NewFrameBuilderDI NOT to register in global registry")
	}
}

func TestNewFrameBuilderDI_CustomConfig(t *testing.T) {
	fb := NewFrameBuilderDI(FrameBuilderConfig{
		SensorID:              "test-di-custom",
		FrameBufferSize:       20,
		AzimuthTolerance:      5.0,
		MinFramePoints:        500,
		MaxBackfillDelay:      50 * time.Millisecond,
		BufferTimeout:         2 * time.Second,
		CleanupInterval:       500 * time.Millisecond,
		ExpectedFrameDuration: 100 * time.Millisecond,
		EnableTimeBased:       true,
	})
	if fb == nil {
		t.Fatal("NewFrameBuilderDI returned nil")
	}
	defer fb.Close()

	if fb.frameBufferSize != 20 {
		t.Errorf("frameBufferSize = %d, want 20", fb.frameBufferSize)
	}
	if fb.azimuthTolerance != 5.0 {
		t.Errorf("azimuthTolerance = %f, want 5.0", fb.azimuthTolerance)
	}
	if fb.enableTimeBased != true {
		t.Error("enableTimeBased should be true")
	}
}

func TestNewFrameBuilderDI_NilCallback(t *testing.T) {
	// Without a callback, frameCh should be nil
	fb := NewFrameBuilderDI(FrameBuilderConfig{
		SensorID: "test-di-nil-cb",
	})
	if fb == nil {
		t.Fatal("NewFrameBuilderDI returned nil")
	}

	if fb.frameCh != nil {
		t.Error("frameCh should be nil when no callback is provided")
	}

	// Close should be safe even with nil channel
	fb.Close()
}

func TestClose_WithCallback(t *testing.T) {
	var mu sync.Mutex
	var processed int

	fb := NewFrameBuilderDI(FrameBuilderConfig{
		SensorID: "test-close",
		FrameCallback: func(f *LiDARFrame) {
			mu.Lock()
			defer mu.Unlock()
			processed++
		},
	})

	// Send a frame through the channel
	fb.frameCh <- &LiDARFrame{FrameID: "test"}

	// Close should wait for the frame to be processed
	fb.Close()

	mu.Lock()
	defer mu.Unlock()
	if processed != 1 {
		t.Errorf("expected 1 processed frame, got %d", processed)
	}
}

func TestClose_NilChannel(t *testing.T) {
	fb := NewFrameBuilderDI(FrameBuilderConfig{
		SensorID: "test-close-nil",
	})
	// Should not panic
	fb.Close()
}

func TestDroppedFrames_ZeroInitially(t *testing.T) {
	fb := NewFrameBuilderDI(FrameBuilderConfig{
		SensorID: "test-dropped-init",
	})
	defer fb.Close()
	if fb.DroppedFrames() != 0 {
		t.Errorf("expected 0 dropped frames initially; got %d", fb.DroppedFrames())
	}
}
