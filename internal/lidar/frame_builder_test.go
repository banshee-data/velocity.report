package lidar

import (
	"sync"
	"testing"
	"time"
)

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

	// Add points to frame builder
	fb.AddPoints(points)

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

	// Add points to frame builder
	fb.AddPoints(points)

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
	fb.AddPoints(points)

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
	fb.AddPoints(points)

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
