package l2frames

import (
	"sync"
	"testing"
	"time"
)

// --- UnregisterFrameBuilder ---

func TestUnregisterFrameBuilder(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "unreg-test"})
	defer fb.Close()

	RegisterFrameBuilder("unreg-test", fb)
	if got := GetFrameBuilder("unreg-test"); got == nil {
		t.Fatal("expected registered builder")
	}

	UnregisterFrameBuilder("unreg-test")
	if got := GetFrameBuilder("unreg-test"); got != nil {
		t.Fatal("expected nil after unregister")
	}
}

// --- Reset with populated buffers ---

func TestReset_ClearsBuffersAndGaps(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "reset-full"})
	defer fb.Close()

	fb.mu.Lock()
	// Populate frame buffer
	fb.frameBuffer["f1"] = &LiDARFrame{FrameID: "f1"}
	fb.frameBuffer["f2"] = &LiDARFrame{FrameID: "f2"}
	// Populate sequence gaps
	fb.sequenceGaps[100] = true
	fb.sequenceGaps[200] = true
	// Populate pending packets
	fb.pendingPackets[300] = []Point{{X: 1}}
	fb.mu.Unlock()

	fb.Reset()

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.frameBuffer) != 0 {
		t.Fatalf("expected empty frame buffer, got %d", len(fb.frameBuffer))
	}
	if len(fb.sequenceGaps) != 0 {
		t.Fatalf("expected empty sequence gaps, got %d", len(fb.sequenceGaps))
	}
	if len(fb.pendingPackets) != 0 {
		t.Fatalf("expected empty pending packets, got %d", len(fb.pendingPackets))
	}
}

// --- AddPointsPolar empty slice guard ---

func TestAddPointsPolar_Empty(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "empty-polar"})
	defer fb.Close()
	// Should return immediately without locking or panicking
	fb.AddPointsPolar(nil)
	fb.AddPointsPolar([]PointPolar{})
}

// --- addPointsDualInternal empty guard ---

func TestAddPointsDualInternal_Empty(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "dual-empty"})
	defer fb.Close()
	fb.mu.Lock()
	fb.addPointsDualInternal(nil, nil)
	fb.mu.Unlock()
}

// --- addPointsDualInternal mismatched lengths ---

func TestAddPointsDualInternal_Mismatch(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "dual-mismatch"})
	defer fb.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for mismatched slice lengths")
		}
		// Unlock the mutex that was held when panic occurred
		fb.mu.Unlock()
	}()

	fb.mu.Lock()
	fb.addPointsDualInternal(
		[]Point{{X: 1}},
		[]PointPolar{{Distance: 1}, {Distance: 2}},
	)
	// If we reach here, no panic occurred — the deferred recover will fail
	fb.mu.Unlock()
}

// --- addPointToCurrentFrame nil guard ---

func TestAddPointToCurrentFrame_NilCurrentFrame(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "nil-frame"})
	defer fb.Close()
	fb.mu.Lock()
	fb.currentFrame = nil
	fb.addPointToCurrentFrame(Point{X: 1})
	fb.mu.Unlock()
}

// --- addPointToCurrentFrame timestamp before range ---

func TestAddPointToCurrentFrame_TimestampBeforeRange(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "ts-before"})
	defer fb.Close()

	now := time.Now()
	earlier := now.Add(-5 * time.Second)

	fb.mu.Lock()
	fb.currentFrame = &LiDARFrame{
		FrameID:         "test",
		StartTimestamp:  now,
		EndTimestamp:    now,
		Points:          []Point{},
		ReceivedPackets: map[uint32]bool{},
		ExpectedPackets: map[uint32]bool{},
	}
	fb.addPointToCurrentFrame(Point{
		Timestamp: earlier,
	})
	// StartTimestamp should be updated to the earlier time
	if !fb.currentFrame.StartTimestamp.Equal(earlier) {
		t.Fatalf("expected StartTimestamp=%v, got %v", earlier, fb.currentFrame.StartTimestamp)
	}
	fb.mu.Unlock()
}

// --- finalizeCurrentFrame in blocking mode ---

func TestFinalizeCurrentFrame_BlockingMode(t *testing.T) {
	var received []*LiDARFrame
	var mu sync.Mutex
	cb := func(f *LiDARFrame) {
		mu.Lock()
		received = append(received, f)
		mu.Unlock()
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "block-finalize",
		FrameCallback:  cb,
		MinFramePoints: 1, // low threshold so frame isn't discarded
	})
	defer fb.Close()

	fb.SetBlockOnFrameChannel(true)

	fb.mu.Lock()
	fb.currentFrame = &LiDARFrame{
		FrameID:         "blocking-1",
		SensorID:        "block-finalize",
		PointCount:      100,
		StartTimestamp:  time.Now().Add(-1 * time.Second),
		EndTimestamp:    time.Now(),
		ReceivedPackets: map[uint32]bool{1: true, 2: true},
		ExpectedPackets: map[uint32]bool{},
		Points:          make([]Point, 100),
	}
	fb.finalizeCurrentFrame()
	fb.mu.Unlock()

	// Wait briefly for callback
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(received) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(received))
	}
	mu.Unlock()
}

// --- finalizeCurrentFrame triggers buffer eviction ---

func TestFinalizeCurrentFrame_BufferEviction(t *testing.T) {
	var received []*LiDARFrame
	var mu sync.Mutex
	cb := func(f *LiDARFrame) {
		mu.Lock()
		received = append(received, f)
		mu.Unlock()
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "evict-test",
		FrameCallback:   cb,
		MinFramePoints:  1,
		FrameBufferSize: 1, // very small buffer
	})
	defer fb.Close()

	fb.mu.Lock()
	// Pre-fill the buffer so it's at capacity
	fb.frameBuffer["existing"] = &LiDARFrame{
		FrameID:        "existing",
		StartTimestamp: time.Now().Add(-10 * time.Second),
		PointCount:     50,
	}

	fb.currentFrame = &LiDARFrame{
		FrameID:         "new-frame",
		SensorID:        "evict-test",
		PointCount:      100,
		StartTimestamp:  time.Now().Add(-1 * time.Second),
		EndTimestamp:    time.Now(),
		ReceivedPackets: map[uint32]bool{1: true},
		ExpectedPackets: map[uint32]bool{},
		Points:          make([]Point, 100),
	}
	fb.finalizeCurrentFrame()
	fb.mu.Unlock()

	// Give callback time to fire
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// The evicted frame should trigger a callback
	if len(received) < 1 {
		t.Fatalf("expected at least 1 callback from eviction, got %d", len(received))
	}
}

// --- calculateFrameCompleteness empty packets ---

func TestCalculateFrameCompleteness_Empty(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "comp-empty"})
	defer fb.Close()

	frame := &LiDARFrame{
		ReceivedPackets: map[uint32]bool{},
		ExpectedPackets: map[uint32]bool{},
	}
	fb.calculateFrameCompleteness(frame)
	// Should return early, no changes
	if frame.PacketGaps != 0 {
		t.Fatalf("expected 0 gaps, got %d", frame.PacketGaps)
	}
}

// --- calculateFrameCompleteness missing packets ---

func TestCalculateFrameCompleteness_WithGaps(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "comp-gaps"})
	defer fb.Close()

	frame := &LiDARFrame{
		ReceivedPackets: map[uint32]bool{1: true, 3: true, 5: true},
		ExpectedPackets: map[uint32]bool{},
	}
	fb.calculateFrameCompleteness(frame)
	// Sequences 2 and 4 should be missing
	if frame.PacketGaps != 2 {
		t.Fatalf("expected 2 gaps, got %d", frame.PacketGaps)
	}
	if len(frame.MissingPackets) != 2 {
		t.Fatalf("expected 2 missing packets, got %d", len(frame.MissingPackets))
	}
}

// --- calculateFrameCompleteness negative coverage wrap ---

func TestCalculateFrameCompleteness_NegativeCoverage(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "comp-wrap"})
	defer fb.Close()

	frame := &LiDARFrame{
		MinAzimuth:      350.0,
		MaxAzimuth:      10.0,
		ReceivedPackets: map[uint32]bool{1: true},
		ExpectedPackets: map[uint32]bool{},
	}
	fb.calculateFrameCompleteness(frame)
	// AzimuthCoverage would be 10-350 = -340, then +360 = 20
	if frame.AzimuthCoverage != 20.0 {
		t.Fatalf("expected 20.0 coverage, got %f", frame.AzimuthCoverage)
	}
}

// --- cleanupFrames with zero-timestamp frames ---

func TestCleanupFrames_ZeroTimestamp(t *testing.T) {
	var received []*LiDARFrame
	var mu sync.Mutex
	cb := func(f *LiDARFrame) {
		mu.Lock()
		received = append(received, f)
		mu.Unlock()
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "cleanup-zero",
		FrameCallback:  cb,
		MinFramePoints: 0,
	})
	defer fb.Close()

	// Add a frame with all zero timestamps — should be cleaned up immediately.
	// cleanupFrames takes its own lock, so we only hold the lock for setup.
	fb.mu.Lock()
	fb.frameBuffer["zero-ts"] = &LiDARFrame{
		FrameID:         "zero-ts",
		SensorID:        "cleanup-zero",
		PointCount:      10,
		ReceivedPackets: map[uint32]bool{1: true},
		ExpectedPackets: map[uint32]bool{},
		Points:          make([]Point, 10),
	}
	fb.mu.Unlock()

	// Call cleanupFrames directly (it takes its own lock)
	fb.cleanupFrames()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) < 1 {
		t.Fatalf("expected callback for zero-timestamp frame, got %d", len(received))
	}
}

// --- cleanupFrames with EndTimestamp fallback for currentFrame ---

func TestCleanupFrames_CurrentFrameEndTimestampFallback(t *testing.T) {
	var received []*LiDARFrame
	var mu sync.Mutex
	cb := func(f *LiDARFrame) {
		mu.Lock()
		received = append(received, f)
		mu.Unlock()
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "cleanup-endts",
		FrameCallback:  cb,
		MinFramePoints: 1,
		BufferTimeout:  1 * time.Millisecond,
	})
	defer fb.Close()

	// Use blocking mode so finalizeCurrentFrame calls finalizeFrame directly
	fb.SetBlockOnFrameChannel(true)

	old := time.Now().Add(-10 * time.Second)

	// Set current frame with EndWallTime zero but EndTimestamp set and old.
	// cleanupFrames takes its own lock, so we only hold the lock for setup.
	fb.mu.Lock()
	fb.currentFrame = &LiDARFrame{
		FrameID:         "endts-fallback",
		SensorID:        "cleanup-endts",
		PointCount:      100,
		EndTimestamp:    old,
		ReceivedPackets: map[uint32]bool{1: true},
		ExpectedPackets: map[uint32]bool{},
		Points:          make([]Point, 100),
	}
	fb.mu.Unlock()

	fb.cleanupFrames()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) < 1 {
		t.Fatalf("expected callback for current frame finalised via EndTimestamp fallback, got %d", len(received))
	}
}

// --- finalizeFrame export error paths ---

// completeFrame returns a frame with enough coverage and points to be considered
// spin-complete by finalizeFrame.
func completeFrame(id, sensorID string) *LiDARFrame {
	return &LiDARFrame{
		FrameID:        id,
		SensorID:       sensorID,
		PointCount:     MinFramePointsForCompletion + 1,
		MinAzimuth:     0,
		MaxAzimuth:     350,
		StartTimestamp: time.Now().Add(-1 * time.Second),
		EndTimestamp:   time.Now(),
		Points:         make([]Point, MinFramePointsForCompletion+1),
	}
}

func TestFinalizeFrame_ExportNextError(t *testing.T) {
	// Point defaultExportDir at an invalid path to force export failure
	oldDir := defaultExportDir
	defaultExportDir = "/nonexistent/path/for/export"
	defer func() { defaultExportDir = oldDir }()

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "export-err",
		MinFramePoints: 1,
	})
	defer fb.Close()

	fb.mu.Lock()
	fb.exportNextFrameASC = true
	fb.finalizeFrame(completeFrame("export-err-frame", "export-err"), "test")
	// After a failed export, exportNextFrameASC should remain true (not cleared)
	if !fb.exportNextFrameASC {
		t.Fatal("expected exportNextFrameASC to remain true after export error")
	}
	fb.mu.Unlock()
}

func TestFinalizeFrame_BatchExportError(t *testing.T) {
	oldDir := defaultExportDir
	defaultExportDir = "/nonexistent/path/for/export"
	defer func() { defaultExportDir = oldDir }()

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "batch-err",
		MinFramePoints: 1,
	})
	defer fb.Close()

	fb.mu.Lock()
	fb.exportBatchCount = 2
	fb.exportBatchExported = 0
	fb.finalizeFrame(completeFrame("batch-err-frame", "batch-err"), "test")
	// Even on error, exportBatchExported should increment
	if fb.exportBatchExported != 1 {
		t.Fatalf("expected exportBatchExported=1, got %d", fb.exportBatchExported)
	}
	fb.mu.Unlock()
}

func TestFinalizeFrame_BatchExportCompletion(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "batch-done",
		MinFramePoints: 1,
	})
	defer fb.Close()

	fb.mu.Lock()
	fb.exportBatchCount = 1
	fb.exportBatchExported = 0
	fb.finalizeFrame(completeFrame("batch-done-frame", "batch-done"), "test")
	// After completing the batch (export succeeds to tmpdir), counters should be reset
	if fb.exportBatchCount != 0 {
		t.Fatalf("expected exportBatchCount=0, got %d", fb.exportBatchCount)
	}
	if fb.exportBatchExported != 0 {
		t.Fatalf("expected exportBatchExported=0, got %d", fb.exportBatchExported)
	}
	fb.mu.Unlock()
}

// --- NewFrameBuilderWithDebugLoggingAndInterval export paths ---

func TestNewFrameBuilderWithDebugLoggingAndInterval_ExportError(t *testing.T) {
	// Point export dir at an invalid path to force export failure
	oldDir := defaultExportDir
	defaultExportDir = "/nonexistent/path/for/export"
	defer func() { defaultExportDir = oldDir }()

	// Build with debug=true and a very short interval so the export branch fires
	fb := NewFrameBuilderWithDebugLoggingAndInterval("debug-interval", true, 0)
	defer fb.Close()

	// The callback should be set; invoke it with a frame that will fail export
	fb.mu.Lock()
	cb := fb.frameCallback
	fb.mu.Unlock()

	if cb == nil {
		t.Fatal("expected non-nil callback")
	}

	// Call the callback directly — exportFrameToASC will fail but should not panic
	frame := &LiDARFrame{
		FrameID:        "debug-test",
		SensorID:       "debug-interval",
		PointCount:     10,
		StartTimestamp: time.Now().Add(-1 * time.Second),
		EndTimestamp:   time.Now(),
		Points:         make([]Point, 10),
	}
	cb(frame) // should not panic
}

func TestNewFrameBuilderWithDebugLoggingAndInterval_SkipInterval(t *testing.T) {
	// Use a long interval so the second call skips export
	fb := NewFrameBuilderWithDebugLoggingAndInterval("debug-skip", true, 1*time.Hour)
	defer fb.Close()

	fb.mu.Lock()
	cb := fb.frameCallback
	fb.mu.Unlock()

	if cb == nil {
		t.Fatal("expected non-nil callback")
	}

	frame := &LiDARFrame{
		FrameID:        "skip-1",
		SensorID:       "debug-skip",
		PointCount:     10,
		StartTimestamp: time.Now(),
		EndTimestamp:   time.Now(),
		Points:         make([]Point, 10),
	}
	// First call triggers export (sets lastExportTime)
	cb(frame)
	// Second call should skip export because interval hasn't elapsed
	frame.FrameID = "skip-2"
	cb(frame) // exercises the else branch
}

// --- Close prevents cleanupFrames rescheduling ---

func TestClose_PreventsCleanupReschedule(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "close-reschedule",
		CleanupInterval: 50 * time.Millisecond,
	})

	// Let the timer fire once
	time.Sleep(100 * time.Millisecond)

	fb.Close()

	// After Close(), no new timer should be scheduled
	fb.mu.Lock()
	if !fb.closed {
		t.Fatal("expected closed=true after Close()")
	}
	fb.mu.Unlock()
}

// --- GetCurrentFrameStats with multiple buffered frames ---

func TestGetCurrentFrameStats_MultipleFrames(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "stats-multi"})
	defer fb.Close()

	now := time.Now()
	older := now.Add(-10 * time.Second)
	newer := now.Add(-1 * time.Second)

	fb.mu.Lock()
	fb.frameBuffer["old"] = &LiDARFrame{
		FrameID:        "old",
		StartTimestamp: older,
	}
	fb.frameBuffer["new"] = &LiDARFrame{
		FrameID:        "new",
		StartTimestamp: newer,
	}
	fb.mu.Unlock()

	count, oldestAge, newestAge := fb.GetCurrentFrameStats()
	if count != 2 {
		t.Fatalf("expected 2 frames, got %d", count)
	}
	// The oldest frame should be ~10s old, newest ~1s old
	if oldestAge < 9*time.Second {
		t.Fatalf("expected oldest age >= 9s, got %v", oldestAge)
	}
	if newestAge > 3*time.Second {
		t.Fatalf("expected newest age <= 3s, got %v", newestAge)
	}
}
