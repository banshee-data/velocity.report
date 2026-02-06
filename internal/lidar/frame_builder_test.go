package lidar

import (
	"sync"
	"testing"
	"time"
)

// toPolar converts a slice of cartesian Points to PointPolar for polar-first API tests.
func toPolar(points []Point) []PointPolar {
	out := make([]PointPolar, 0, len(points))
	for _, p := range points {
		out = append(out, PointPolar{
			Channel:     p.Channel,
			Azimuth:     p.Azimuth,
			Elevation:   p.Elevation,
			Distance:    p.Distance,
			Intensity:   p.Intensity,
			Timestamp:   p.Timestamp.UnixNano(),
			BlockID:     p.BlockID,
			UDPSequence: p.UDPSequence,
		})
	}
	return out
}

// TestFrameBuilder_BasicConfiguration tests the basic configuration and defaults
func TestFrameBuilder_BasicConfiguration(t *testing.T) {
	sensorID := "test-sensor-001"
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	// Test with default configuration
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:      sensorID,
		FrameCallback: callback,
	})

	if fb.sensorID != sensorID {
		t.Errorf("Expected sensorID %s, got %s", sensorID, fb.sensorID)
	}

	if fb.frameBufferSize != 10 {
		t.Errorf("Expected default frameBufferSize 10, got %d", fb.frameBufferSize)
	}

	if fb.azimuthTolerance != 10.0 {
		t.Errorf("Expected default azimuthTolerance 10.0, got %f", fb.azimuthTolerance)
	}

	if fb.minFramePoints != 1000 {
		t.Errorf("Expected default minFramePoints 1000, got %d", fb.minFramePoints)
	}

	if fb.bufferTimeout != 1000*time.Millisecond {
		t.Errorf("Expected default bufferTimeout 1000ms, got %v", fb.bufferTimeout)
	}

	if fb.cleanupInterval != 250*time.Millisecond {
		t.Errorf("Expected default cleanupInterval 250ms, got %v", fb.cleanupInterval)
	}
}

// TestFrameBuilder_AzimuthFrameDetection tests the azimuth-based frame detection logic
func TestFrameBuilder_AzimuthFrameDetection(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:         "test-sensor",
		FrameCallback:    callback,
		AzimuthTolerance: 10.0,
		MinFramePoints:   1000, // Reasonable threshold
		BufferTimeout:    50 * time.Millisecond,
		CleanupInterval:  25 * time.Millisecond,
	})

	baseTime := time.Now()

	// Create a realistic full rotation of points to trigger frame completion
	// Need >50k points and >340° coverage for frame detection
	points := make([]Point, 0, 60000)

	// Create points across full 360° rotation with enough density
	for i := 0; i < 60000; i++ {
		azimuth := float64(i) * 360.0 / 60000.0 // Evenly distributed across 360°
		timestamp := baseTime.Add(time.Duration(i) * time.Microsecond)
		udpSeq := uint32(i + 100)

		points = append(points, Point{
			Azimuth:     azimuth,
			Timestamp:   timestamp,
			UDPSequence: udpSeq,
		})
	}

	// Add the wrap-around point that should trigger frame completion
	points = append(points, Point{
		Azimuth:     5.0, // Low azimuth after high values
		Timestamp:   baseTime.Add(time.Duration(60000) * time.Microsecond),
		UDPSequence: 60100,
	})

	// Add points to frame builder (polar-first)
	fb.AddPointsPolar(toPolar(points))

	// Wait for frame to be finalized
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	// Should have at least 1 frame due to azimuth wrap
	if frameCount < 1 {
		t.Errorf("Expected at least 1 frame, got %d", frameCount)
	}

	if frameCount > 0 {
		frame := receivedFrames[0]
		if frame.SensorID != "test-sensor" {
			t.Errorf("Expected sensorID 'test-sensor', got '%s'", frame.SensorID)
		}
		if frame.PointCount < MinFramePointsForCompletion {
			t.Errorf("Expected at least %d points in frame, got %d", MinFramePointsForCompletion, frame.PointCount)
		}
		if frame.MaxAzimuth-frame.MinAzimuth < MinAzimuthCoverage {
			t.Errorf("Expected azimuth coverage >%f°, got %f", MinAzimuthCoverage, frame.MaxAzimuth-frame.MinAzimuth)
		}
	}
}

// TestFrameBuilder_NoWrapWithoutCriteria tests that frames aren't created without meeting criteria
func TestFrameBuilder_NoWrapWithoutCriteria(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:         "test-sensor",
		FrameCallback:    callback,
		AzimuthTolerance: 10.0,
		MinFramePoints:   1000,
		BufferTimeout:    50 * time.Millisecond,
		CleanupInterval:  25 * time.Millisecond,
	})

	baseTime := time.Now()

	// Create points that wrap but don't meet the criteria (too few points)
	points := []Point{
		{Azimuth: 350.0, Timestamp: baseTime, UDPSequence: 1},
		{Azimuth: 355.0, Timestamp: baseTime.Add(10 * time.Millisecond), UDPSequence: 2},
		{Azimuth: 359.0, Timestamp: baseTime.Add(20 * time.Millisecond), UDPSequence: 3},
		{Azimuth: 5.0, Timestamp: baseTime.Add(30 * time.Millisecond), UDPSequence: 4}, // Wrap but insufficient data
	}

	// Add points to frame builder (polar-first)
	fb.AddPointsPolar(toPolar(points))

	// Wait for potential frame finalization
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	// Should not create frame without meeting all criteria
	if frameCount > 0 {
		t.Errorf("Expected no frames without meeting criteria, got %d", frameCount)
	}
}

// TestFrameBuilder_UDPSequenceTracking tests that UDP sequences are properly tracked
func TestFrameBuilder_UDPSequenceTracking(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:         "test-sensor",
		FrameCallback:    callback,
		AzimuthTolerance: 10.0,
		MinFramePoints:   1000,
		BufferTimeout:    50 * time.Millisecond,
		CleanupInterval:  25 * time.Millisecond,
	})

	// Create a realistic rotation with tracked UDP sequences
	baseTime := time.Now()
	points := make([]Point, 0, 55000)

	for i := 0; i < 55000; i++ {
		azimuth := float64(i) * 360.0 / 55000.0
		timestamp := baseTime.Add(time.Duration(i) * time.Microsecond)
		udpSeq := uint32(i + 200) // Start from 200

		points = append(points, Point{
			Azimuth:     azimuth,
			Timestamp:   timestamp,
			UDPSequence: udpSeq,
		})
	}

	// Add wrap point
	points = append(points, Point{
		Azimuth:     5.0,
		Timestamp:   baseTime.Add(time.Duration(55000) * time.Microsecond),
		UDPSequence: 55200,
	})

	// Add points to frame builder
	fb.AddPointsPolar(toPolar(points))

	// Wait for frame finalization
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount < 1 {
		t.Errorf("Expected at least 1 frame, got %d", frameCount)
		return
	}

	frame := receivedFrames[0]

	// Check that UDP sequences are tracked in the frame
	if len(frame.ReceivedPackets) == 0 {
		t.Error("Expected frame to track received UDP packets")
	}

	// Verify some specific sequences are present
	expectedSequences := []uint32{200, 210, 220, 230}
	for _, seq := range expectedSequences {
		if !frame.ReceivedPackets[seq] {
			t.Errorf("Expected UDP sequence %d to be tracked in frame", seq)
		}
	}
}

// TestFrameBuilder_MinimumPointValidation tests the minimum point requirement
func TestFrameBuilder_MinimumPointValidation(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	// Test with higher minimum point threshold
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:         "test-sensor",
		FrameCallback:    callback,
		AzimuthTolerance: 10.0,
		MinFramePoints:   60000, // Higher than our test data
		BufferTimeout:    50 * time.Millisecond,
		CleanupInterval:  25 * time.Millisecond,
	})

	baseTime := time.Now()
	points := make([]Point, 0, 55000) // Less than minFramePoints

	for i := 0; i < 55000; i++ {
		azimuth := float64(i) * 360.0 / 55000.0
		timestamp := baseTime.Add(time.Duration(i) * time.Microsecond)
		udpSeq := uint32(i + 300)

		points = append(points, Point{
			Azimuth:     azimuth,
			Timestamp:   timestamp,
			UDPSequence: udpSeq,
		})
	}

	// Add wrap point
	points = append(points, Point{
		Azimuth:     5.0,
		Timestamp:   baseTime.Add(time.Duration(55000) * time.Microsecond),
		UDPSequence: 55300,
	})

	// Add points to frame builder
	fb.AddPointsPolar(toPolar(points))

	// Wait for frame finalization attempt
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	// Should not create frame below minimum point threshold
	if frameCount > 0 {
		t.Errorf("Expected no frames below minimum point threshold (60000), got %d frames", frameCount)
	}
}

// TestFrameBuilder_HybridDetection tests the hybrid time+azimuth frame detection
func TestFrameBuilder_HybridDetection(t *testing.T) {
	sensorID := "test-sensor-hybrid"
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
		t.Logf("Frame completed: %s, Points: %d, Azimuth: %.1f°-%.1f°, Duration: %v",
			frame.FrameID, frame.PointCount, frame.MinAzimuth, frame.MaxAzimuth,
			frame.EndTimestamp.Sub(frame.StartTimestamp))
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        sensorID,
		FrameCallback:   callback,
		MinFramePoints:  12000,                  // Higher than MinFramePointsForCompletion (10000)
		BufferTimeout:   100 * time.Millisecond, // Shorter timeout for testing
		CleanupInterval: 50 * time.Millisecond,  // Shorter cleanup interval for testing
	})

	// Test 1: Enable time-based detection with 100ms frame duration (600 RPM)
	fb.SetMotorSpeed(600) // Should set 100ms frame duration

	baseTime := time.Now()

	// Use same successful pattern as TraditionalAzimuthOnly but with time-based detection
	points := make([]Point, 0)
	for i := 0; i < 60000; i++ {
		azimuth := float64(i) * 356.0 / 60000.0 // 0° to 356° evenly distributed (like working test)
		point := Point{
			Azimuth:     azimuth,
			Timestamp:   baseTime.Add(time.Duration(i) * time.Microsecond), // Same timing as working test
			UDPSequence: uint32(i + 100),
		}
		points = append(points, point)
	}

	fb.AddPointsPolar(toPolar(points))

	// Wait a moment to ensure points are processed
	time.Sleep(10 * time.Millisecond)

	// Now add a wrap point that should trigger time+azimuth completion
	finalPoint := Point{
		Azimuth:     5.0,                                  // Azimuth wrap (like working test)
		Timestamp:   baseTime.Add(120 * time.Millisecond), // Exceeds time threshold (110ms)
		UDPSequence: 60100,
	}

	fb.AddPointsPolar(toPolar([]Point{finalPoint}))

	// Frame should be detected and buffered, wait for buffer timeout
	time.Sleep(200 * time.Millisecond)

	// Should have triggered frame completion due to time + azimuth coverage
	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount == 0 {
		t.Logf("DEBUG: No frames completed. Check conditions:")
		t.Logf("  - Motor speed: 600 RPM (100ms expected frame duration)")
		t.Logf("  - Point count: 60000 (> 10000 required)")
		t.Logf("  - Azimuth coverage: 356° (> 270° required)")
		t.Logf("  - Time span: 120ms (> 110ms threshold)")
		t.Errorf("Expected frame completion due to time threshold + azimuth coverage, got %d frames", frameCount)
	} else {
		frame := receivedFrames[0]
		duration := frame.EndTimestamp.Sub(frame.StartTimestamp)
		t.Logf("Frame completed with %d points, %.1f° coverage, %v duration",
			frame.PointCount, frame.MaxAzimuth-frame.MinAzimuth, duration)

		// In time-based mode, frame can complete via azimuth wrap OR time threshold
		// This test triggers azimuth wrap first (356° -> 5°), which is valid behavior
		if duration < 50*time.Millisecond {
			t.Errorf("Expected frame duration >= 50ms (half expected duration), got %v", duration)
		}
		coverage := frame.MaxAzimuth - frame.MinAzimuth
		if coverage < 350.0 { // Expect nearly full rotation coverage
			t.Errorf("Expected azimuth coverage >= 350°, got %.1f°", coverage)
		}
	}
}

// TestFrameBuilder_TimeBasedWithInsufficientCoverage tests that time-based detection requires azimuth validation
func TestFrameBuilder_TimeBasedWithInsufficientCoverage(t *testing.T) {
	sensorID := "test-sensor-coverage"
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       sensorID,
		FrameCallback:  callback,
		MinFramePoints: 10,
	})

	// Enable time-based detection
	fb.SetMotorSpeed(600) // 100ms frame duration

	baseTime := time.Now()

	// Add points with insufficient azimuth coverage (only 50°) but exceeding time
	points := make([]Point, 0)
	for i := 0; i < 50; i++ {
		point := Point{
			Azimuth:     float64(i),                                            // Only 0° to 49° (insufficient coverage)
			Timestamp:   baseTime.Add(time.Duration(i) * 2 * time.Millisecond), // 100ms total
			UDPSequence: uint32(i),
			X:           1.0, Y: 1.0, Z: 1.0, Intensity: 100,
		}
		points = append(points, point)
	}

	// Add final point exceeding time threshold
	finalPoint := Point{
		Azimuth:     50.0,
		Timestamp:   baseTime.Add(120 * time.Millisecond), // Exceeds time threshold
		UDPSequence: 50,
		X:           1.0, Y: 1.0, Z: 1.0, Intensity: 100,
	}
	points = append(points, finalPoint)

	fb.AddPointsPolar(toPolar(points))

	// Should NOT trigger frame completion due to insufficient azimuth coverage
	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount != 0 {
		t.Errorf("Expected no frame completion due to insufficient azimuth coverage, got %d frames", frameCount)
	}
}

// TestFrameBuilder_AzimuthWrapWithTimeBased tests azimuth wrap detection in time-based mode
func TestFrameBuilder_AzimuthWrapWithTimeBased(t *testing.T) {
	sensorID := "test-sensor-azwrap"
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        sensorID,
		FrameCallback:   callback,
		MinFramePoints:  12000,                  // Higher than MinFramePointsForCompletion (10000)
		BufferTimeout:   100 * time.Millisecond, // Same as HybridDetection
		CleanupInterval: 50 * time.Millisecond,  // Same as HybridDetection
	})

	// Enable time-based detection
	fb.SetMotorSpeed(600) // 100ms frame duration

	baseTime := time.Now()

	// Use same successful pattern as HybridDetection
	points := make([]Point, 0)
	for i := 0; i < 60000; i++ {
		azimuth := float64(i) * 356.0 / 60000.0 // 0° to 356° evenly distributed (like working test)
		point := Point{
			Azimuth:     azimuth,
			Timestamp:   baseTime.Add(time.Duration(i) * time.Microsecond),
			UDPSequence: uint32(i + 100),
		}
		points = append(points, point)
	}

	fb.AddPointsPolar(toPolar(points))

	// Wait a moment to ensure points are processed
	time.Sleep(10 * time.Millisecond)

	// Add azimuth wrap point (exactly like HybridDetection pattern)
	wrapPoint := Point{
		Azimuth:     15.0,                                // Azimuth wrap from 356° to 15°
		Timestamp:   baseTime.Add(60 * time.Millisecond), // > 50ms (half duration)
		UDPSequence: 60100,
	}

	fb.AddPointsPolar(toPolar([]Point{wrapPoint}))

	// Wait for frame processing (buffer timeout)
	time.Sleep(200 * time.Millisecond)

	// Should trigger frame completion due to azimuth wrap + minimum time
	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount == 0 {
		t.Logf("DEBUG: Expected azimuth wrap completion (350° -> 15°) after 60ms")
		t.Errorf("Expected frame completion due to azimuth wrap + minimum time, got %d frames", frameCount)
	} else {
		frame := receivedFrames[0]
		t.Logf("Frame completed with %d points, %.1f° coverage, %v duration",
			frame.PointCount, frame.MaxAzimuth-frame.MinAzimuth,
			frame.EndTimestamp.Sub(frame.StartTimestamp))
	}
}

// TestFrameBuilder_TraditionalAzimuthOnly tests traditional azimuth-only detection
func TestFrameBuilder_TraditionalAzimuthOnly(t *testing.T) {
	sensorID := "test-sensor-traditional"
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        sensorID,
		FrameCallback:   callback,
		MinFramePoints:  12000,                  // Higher than MinFramePointsForCompletion (10000)
		BufferTimeout:   100 * time.Millisecond, // Shorter timeout for testing
		CleanupInterval: 50 * time.Millisecond,  // Shorter cleanup interval for testing
	})

	// Do NOT enable time-based detection (traditional mode)
	// fb.SetMotorSpeed() not called, so enableTimeBased remains false

	baseTime := time.Now()

	// Build a complete rotation with sufficient points and coverage
	points := make([]Point, 0)

	// Add points from 0° to 355° with sufficient coverage > MinAzimuthCoverage (340°)
	// and sufficient points > MinFramePointsForCompletion (10000) - using 60000 like working test
	for i := 0; i < 60000; i++ { // 60000 points like working test
		azimuth := float64(i) * 356.0 / 60000.0 // 0° to 356° evenly distributed
		point := Point{
			Azimuth:     azimuth,
			Timestamp:   baseTime.Add(time.Duration(i) * time.Microsecond),
			UDPSequence: uint32(i + 100),
		}
		points = append(points, point)
	}

	fb.AddPointsPolar(toPolar(points))

	// Verify no frame completion yet (no azimuth wrap)
	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount != 0 {
		t.Errorf("Expected no frame completion before azimuth wrap, got %d frames", frameCount)
		return
	}

	// Add azimuth wrap point (356° → 5°) - should trigger completion
	wrapPoints := []Point{
		{
			Azimuth: 5.0, Timestamp: baseTime.Add(500 * time.Millisecond),
			UDPSequence: 60100,
		},
	}

	fb.AddPointsPolar(toPolar(wrapPoints))

	// Frame should be detected and buffered, wait for buffer timeout (100ms) + cleanup cycles (50ms each)
	time.Sleep(250 * time.Millisecond) // BufferTimeout (100ms) + multiple CleanupInterval cycles (50ms each)

	// Should trigger frame completion due to azimuth wrap + sufficient coverage + points
	mu.Lock()
	frameCount = len(receivedFrames)
	mu.Unlock()

	if frameCount == 0 {
		t.Logf("DEBUG: Frame builder state may not have triggered completion")
		t.Logf("DEBUG: Expected points > 10000, azimuth coverage > 340°, azimuth wrap 351° → 5°")
		t.Errorf("Expected frame completion due to azimuth wrap in traditional mode, got %d frames", frameCount)
	} else {
		frame := receivedFrames[0]
		t.Logf("DEBUG: Completed frame with %d points, %.1f° coverage", frame.PointCount, frame.MaxAzimuth-frame.MinAzimuth)
		if frame.PointCount < 10000 { // MinFramePointsForCompletion
			t.Errorf("Expected >= %d points, got %d", 10000, frame.PointCount)
		}
		coverage := frame.MaxAzimuth - frame.MinAzimuth
		if coverage < 340.0 { // MinAzimuthCoverage
			t.Errorf("Expected azimuth coverage >= %.1f°, got %.1f°", 340.0, coverage)
		}
	}
}

// TestFrameBuilder_EvictOldestBufferedFrame tests the buffer eviction logic.
func TestFrameBuilder_EvictOldestBufferedFrame(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex
	evictDone := make(chan struct{})

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
		// Signal that eviction callback was received
		select {
		case evictDone <- struct{}{}:
		default:
		}
	}

	// Create frame builder with small buffer to trigger eviction
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "test-sensor",
		FrameCallback:   callback,
		FrameBufferSize: 2, // Small buffer to trigger eviction
		MinFramePoints:  10,
	})

	baseTime := time.Now()

	// Manually add frames to the buffer to test eviction
	fb.mu.Lock()

	// Add first frame (oldest)
	fb.frameBuffer["frame-1"] = &LiDARFrame{
		FrameID:        "frame-1",
		SensorID:       "test-sensor",
		PointCount:     100,
		StartTimestamp: baseTime,
		EndTimestamp:   baseTime.Add(10 * time.Millisecond),
	}

	// Add second frame
	fb.frameBuffer["frame-2"] = &LiDARFrame{
		FrameID:        "frame-2",
		SensorID:       "test-sensor",
		PointCount:     100,
		StartTimestamp: baseTime.Add(100 * time.Millisecond),
		EndTimestamp:   baseTime.Add(110 * time.Millisecond),
	}

	// Add third frame - this should not trigger eviction yet as we do it manually
	fb.frameBuffer["frame-3"] = &LiDARFrame{
		FrameID:        "frame-3",
		SensorID:       "test-sensor",
		PointCount:     100,
		StartTimestamp: baseTime.Add(200 * time.Millisecond),
		EndTimestamp:   baseTime.Add(210 * time.Millisecond),
	}

	fb.mu.Unlock()

	// Buffer now has 3 frames, which exceeds buffer size of 2
	// Call evictOldestBufferedFrame
	fb.mu.Lock()
	fb.evictOldestBufferedFrame()
	fb.mu.Unlock()

	// Wait for async callback (with timeout)
	select {
	case <-evictDone:
		// Callback received
	case <-time.After(500 * time.Millisecond):
		// Timeout - callback may not have been called
	}

	// Check that oldest frame was evicted and callback was called
	mu.Lock()
	evictedCount := len(receivedFrames)
	mu.Unlock()

	if evictedCount != 1 {
		t.Errorf("Expected 1 evicted frame via callback, got %d", evictedCount)
	}

	fb.mu.Lock()
	remaining := len(fb.frameBuffer)
	fb.mu.Unlock()

	if remaining != 2 {
		t.Errorf("Expected 2 frames remaining in buffer, got %d", remaining)
	}

	// Verify the oldest frame (frame-1) was evicted
	fb.mu.Lock()
	_, hasFrame1 := fb.frameBuffer["frame-1"]
	_, hasFrame2 := fb.frameBuffer["frame-2"]
	_, hasFrame3 := fb.frameBuffer["frame-3"]
	fb.mu.Unlock()

	if hasFrame1 {
		t.Error("Expected frame-1 (oldest) to be evicted")
	}
	if !hasFrame2 {
		t.Error("Expected frame-2 to remain in buffer")
	}
	if !hasFrame3 {
		t.Error("Expected frame-3 to remain in buffer")
	}
}

// TestFrameBuilder_EvictOldestBufferedFrame_EmptyBuffer tests eviction with empty buffer.
func TestFrameBuilder_EvictOldestBufferedFrame_EmptyBuffer(t *testing.T) {
	var callbackCalled bool
	callback := func(frame *LiDARFrame) {
		callbackCalled = true
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "test-sensor",
		FrameCallback:   callback,
		FrameBufferSize: 2,
	})

	// Evict from empty buffer - should not panic
	fb.mu.Lock()
	fb.evictOldestBufferedFrame()
	fb.mu.Unlock()

	if callbackCalled {
		t.Error("Callback should not be called when buffer is empty")
	}
}

// TestFrameBuilder_FinalizeFrame tests the finalizeFrame function directly.
func TestFrameBuilder_FinalizeFrame(t *testing.T) {
	t.Run("nil frame returns early", func(t *testing.T) {
		fb := NewFrameBuilder(FrameBuilderConfig{
			SensorID: "test-sensor",
		})
		// Should not panic
		fb.finalizeFrame(nil, "test")
	})

	t.Run("sets SpinComplete for full coverage frame", func(t *testing.T) {
		var received *LiDARFrame
		done := make(chan struct{})
		fb := NewFrameBuilder(FrameBuilderConfig{
			SensorID: "test-sensor",
			FrameCallback: func(frame *LiDARFrame) {
				received = frame
				close(done)
			},
		})

		frame := &LiDARFrame{
			FrameID:    "test-frame",
			SensorID:   "test-sensor",
			PointCount: 20000, // > MinFramePointsForCompletion
			MinAzimuth: 0.0,
			MaxAzimuth: 359.0, // ~359 degrees coverage
		}

		fb.finalizeFrame(frame, "test")

		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Callback not called")
		}

		if !received.SpinComplete {
			t.Error("Expected SpinComplete to be true for full coverage frame")
		}
	})

	t.Run("sets SpinComplete false for incomplete frame", func(t *testing.T) {
		var received *LiDARFrame
		done := make(chan struct{})
		fb := NewFrameBuilder(FrameBuilderConfig{
			SensorID: "test-sensor",
			FrameCallback: func(frame *LiDARFrame) {
				received = frame
				close(done)
			},
		})

		frame := &LiDARFrame{
			FrameID:    "test-frame",
			SensorID:   "test-sensor",
			PointCount: 100, // < MinFramePointsForCompletion
			MinAzimuth: 0.0,
			MaxAzimuth: 90.0, // Only 90 degrees coverage
		}

		fb.finalizeFrame(frame, "test")

		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Callback not called")
		}

		if received.SpinComplete {
			t.Error("Expected SpinComplete to be false for incomplete frame")
		}
	})

	t.Run("with debug mode enabled", func(t *testing.T) {
		done := make(chan struct{})
		fb := NewFrameBuilder(FrameBuilderConfig{
			SensorID: "test-sensor",
			FrameCallback: func(frame *LiDARFrame) {
				close(done)
			},
		})
		// Enable debug mode directly on the FrameBuilder
		fb.debug = true

		frame := &LiDARFrame{
			FrameID:        "test-frame",
			SensorID:       "test-sensor",
			PointCount:     100,
			PacketGaps:     2, // Has gaps - should trigger debug log
			MinAzimuth:     0.0,
			MaxAzimuth:     90.0,
			StartTimestamp: time.Now(),
			EndTimestamp:   time.Now().Add(100 * time.Millisecond),
		}

		fb.finalizeFrame(frame, "debug-test")

		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Callback not called")
		}
	})

	t.Run("no callback when nil", func(t *testing.T) {
		fb := NewFrameBuilder(FrameBuilderConfig{
			SensorID:      "test-sensor",
			FrameCallback: nil,
		})

		frame := &LiDARFrame{
			FrameID:    "test-frame",
			SensorID:   "test-sensor",
			PointCount: 100,
		}

		// Should not panic when callback is nil
		fb.finalizeFrame(frame, "test")
	})
}
