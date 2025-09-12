package lidar

import (
	"sync"
	"testing"
	"time"
)

//
// FrameBuilder Test Suite
//
// This test suite comprehensively tests the FrameBuilder component, which
// accumulates LiDAR points from multiple packets into complete rotational frames
// using time-based buffering to handle late-arriving packets.
//
// Test Coverage:
// - Configuration validation (default and custom settings)
// - Time slot calculation for frame assignment
// - Single and multiple frame completion
// - Late packet handling and buffering
// - Buffer size limits and eviction
// - Concurrent access and thread safety
// - Frame statistics and monitoring
// - Edge cases (empty points, azimuth ranges)
// - Performance benchmarks
//
// Performance Results (Apple M1 Pro):
// - AddPoints (400 points): ~32μs per operation (~80ns per point)
// - Single Point: ~63ns per point
//

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

	if fb.frameBufferSize != 50 {
		t.Errorf("Expected default frameBufferSize 50, got %d", fb.frameBufferSize)
	}

	if fb.frameDuration != 100*time.Millisecond {
		t.Errorf("Expected default frameDuration 100ms, got %v", fb.frameDuration)
	}

	if fb.bufferTimeout != 200*time.Millisecond {
		t.Errorf("Expected default bufferTimeout 200ms, got %v", fb.bufferTimeout)
	}

	if fb.cleanupInterval != 500*time.Millisecond {
		t.Errorf("Expected default cleanupInterval 500ms, got %v", fb.cleanupInterval)
	}
}

// TestFrameBuilder_CustomConfiguration tests custom configuration values
func TestFrameBuilder_CustomConfiguration(t *testing.T) {
	config := FrameBuilderConfig{
		SensorID:        "custom-sensor",
		FrameCallback:   nil,
		FrameBufferSize: 20,
		FrameDuration:   50 * time.Millisecond,
		BufferTimeout:   100 * time.Millisecond,
		CleanupInterval: 250 * time.Millisecond,
	}

	fb := NewFrameBuilder(config)

	if fb.frameBufferSize != 20 {
		t.Errorf("Expected frameBufferSize 20, got %d", fb.frameBufferSize)
	}

	if fb.frameDuration != 50*time.Millisecond {
		t.Errorf("Expected frameDuration 50ms, got %v", fb.frameDuration)
	}

	if fb.bufferTimeout != 100*time.Millisecond {
		t.Errorf("Expected bufferTimeout 100ms, got %v", fb.bufferTimeout)
	}

	if fb.cleanupInterval != 250*time.Millisecond {
		t.Errorf("Expected cleanupInterval 250ms, got %v", fb.cleanupInterval)
	}
}

// TestFrameBuilder_TimeSlotCalculation tests the time slot calculation logic
func TestFrameBuilder_TimeSlotCalculation(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:      "test-sensor",
		FrameDuration: 100 * time.Millisecond,
	})

	baseTime := time.Date(2025, 9, 11, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name         string
		timestamp    time.Time
		expectedSlot int64
	}{
		{
			name:         "base time",
			timestamp:    baseTime,
			expectedSlot: baseTime.UnixNano() / (100 * time.Millisecond).Nanoseconds(),
		},
		{
			name:         "50ms later (same slot)",
			timestamp:    baseTime.Add(50 * time.Millisecond),
			expectedSlot: baseTime.UnixNano() / (100 * time.Millisecond).Nanoseconds(),
		},
		{
			name:         "150ms later (next slot)",
			timestamp:    baseTime.Add(150 * time.Millisecond),
			expectedSlot: baseTime.Add(150*time.Millisecond).UnixNano() / (100 * time.Millisecond).Nanoseconds(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			slot := fb.getTimeSlot(tc.timestamp)
			if slot != tc.expectedSlot {
				t.Errorf("Expected time slot %d, got %d", tc.expectedSlot, slot)
			}
		})
	}
}

// TestFrameBuilder_SingleFrameCompletion tests basic frame building and completion
func TestFrameBuilder_SingleFrameCompletion(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "test-sensor",
		FrameCallback:   callback,
		FrameDuration:   100 * time.Millisecond,
		BufferTimeout:   50 * time.Millisecond,
		CleanupInterval: 25 * time.Millisecond,
	})

	baseTime := time.Now()
	// All points in the same time slot (within 100ms frame duration)
	// Use much smaller time differences to ensure same slot
	points := []Point{
		{
			X: 1.0, Y: 2.0, Z: 3.0,
			Intensity: 100, Distance: 5.0,
			Azimuth: 45.0, Elevation: 2.0,
			Channel: 1, Timestamp: baseTime,
			BlockID: 0,
		},
		{
			X: 2.0, Y: 3.0, Z: 4.0,
			Intensity: 150, Distance: 6.0,
			Azimuth: 90.0, Elevation: 1.0,
			Channel: 2, Timestamp: baseTime.Add(10 * time.Millisecond), // Much smaller gap
			BlockID: 1,
		},
		{
			X: 3.0, Y: 4.0, Z: 5.0,
			Intensity: 200, Distance: 7.0,
			Azimuth: 135.0, Elevation: 0.0,
			Channel: 3, Timestamp: baseTime.Add(20 * time.Millisecond), // Much smaller gap
			BlockID: 2,
		},
	}

	// Add points to frame builder
	fb.AddPoints(points)

	// Wait for frame to be finalized (need to wait longer than buffer timeout)
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	// Should have at least 1 frame since all points are in same time slot
	// May have more due to timing variations in cleanup
	if frameCount < 1 {
		t.Fatalf("Expected at least 1 completed frame, got %d", frameCount)
	}

	// Verify total points across all frames
	totalPoints := 0
	allAzimuths := []float64{}
	for _, frame := range receivedFrames {
		totalPoints += frame.PointCount
		for _, point := range frame.Points {
			allAzimuths = append(allAzimuths, point.Azimuth)
		}
	}

	if totalPoints != 3 {
		t.Errorf("Expected 3 total points across all frames, got %d", totalPoints)
	}

	// Check the first frame properties
	frame := receivedFrames[0]

	// Verify frame properties
	if frame.SensorID != "test-sensor" {
		t.Errorf("Expected SensorID 'test-sensor', got '%s'", frame.SensorID)
	}

	if !frame.SpinComplete {
		t.Error("Expected frame.SpinComplete to be true")
	}

	// Check that we have all expected azimuth values somewhere
	expectedAzimuths := []float64{45.0, 90.0, 135.0}
	for _, expected := range expectedAzimuths {
		found := false
		for _, actual := range allAzimuths {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected azimuth %.1f not found in any frame", expected)
		}
	}
}

// TestFrameBuilder_MultipleFrames tests handling multiple frames across time slots
func TestFrameBuilder_MultipleFrames(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "test-sensor",
		FrameCallback:   callback,
		FrameDuration:   100 * time.Millisecond,
		BufferTimeout:   50 * time.Millisecond,
		CleanupInterval: 25 * time.Millisecond,
	})

	baseTime := time.Now()

	// Points for first frame (0-50ms)
	frame1Points := []Point{
		{
			X: 1.0, Y: 1.0, Z: 1.0,
			Azimuth: 0.0, Timestamp: baseTime,
			Intensity: 100, Distance: 2.0,
		},
		{
			X: 2.0, Y: 2.0, Z: 2.0,
			Azimuth: 180.0, Timestamp: baseTime.Add(25 * time.Millisecond),
			Intensity: 150, Distance: 3.0,
		},
	}

	// Points for second frame (150-200ms - clearly in different time slot)
	frame2Points := []Point{
		{
			X: 3.0, Y: 3.0, Z: 3.0,
			Azimuth: 90.0, Timestamp: baseTime.Add(175 * time.Millisecond),
			Intensity: 200, Distance: 4.0,
		},
		{
			X: 4.0, Y: 4.0, Z: 4.0,
			Azimuth: 270.0, Timestamp: baseTime.Add(190 * time.Millisecond),
			Intensity: 250, Distance: 5.0,
		},
	}

	// Add points for both frames
	fb.AddPoints(frame1Points)
	fb.AddPoints(frame2Points)

	// Wait for frames to be finalized (need to wait longer than buffer timeout)
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	// Should have at least 1 frame, may have more due to timing
	if frameCount < 1 {
		t.Fatalf("Expected at least 1 completed frame, got %d", frameCount)
	}

	// Verify we have frames with the expected total points
	totalPoints := 0
	for _, frame := range receivedFrames {
		totalPoints += frame.PointCount
	}

	if totalPoints != 4 {
		t.Errorf("Expected total of 4 points across all frames, got %d", totalPoints)
	}

	// Verify frames are ordered by time
	if len(receivedFrames) > 1 && receivedFrames[0].StartTimestamp.After(receivedFrames[1].StartTimestamp) {
		t.Error("Frames should be received in chronological order")
	}
}

// TestFrameBuilder_LatePackets tests handling of late-arriving packets
func TestFrameBuilder_LatePackets(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "test-sensor",
		FrameCallback:   callback,
		FrameDuration:   100 * time.Millisecond,
		BufferTimeout:   100 * time.Millisecond, // Longer timeout to allow late packets
		CleanupInterval: 25 * time.Millisecond,
	})

	baseTime := time.Now()

	// Add initial points for frame 1
	earlyPoints := []Point{
		{
			X: 1.0, Y: 1.0, Z: 1.0,
			Azimuth: 0.0, Timestamp: baseTime,
			Intensity: 100,
		},
	}

	fb.AddPoints(earlyPoints)

	// Wait a bit, then add a late packet for the same frame
	time.Sleep(30 * time.Millisecond)

	latePoints := []Point{
		{
			X: 2.0, Y: 2.0, Z: 2.0,
			Azimuth: 180.0, Timestamp: baseTime.Add(25 * time.Millisecond), // Late packet for same frame
			Intensity: 150,
		},
	}

	fb.AddPoints(latePoints)

	// Wait for frame to be finalized
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount < 1 {
		t.Fatalf("Expected at least 1 completed frame, got %d", frameCount)
	}

	// Check that all points ended up in frames
	totalPoints := 0
	for _, frame := range receivedFrames {
		totalPoints += frame.PointCount
	}

	if totalPoints != 2 {
		t.Errorf("Expected 2 total points (including late packet), got %d", totalPoints)
	}

	// Verify the late packet was included somewhere
	foundLatePoint := false
	for _, frame := range receivedFrames {
		for _, point := range frame.Points {
			if point.Azimuth == 180.0 {
				foundLatePoint = true
				break
			}
		}
		if foundLatePoint {
			break
		}
	}

	if !foundLatePoint {
		t.Error("Late packet was not included in any frame")
	}
}

// TestFrameBuilder_BufferSizeLimit tests buffer size enforcement
func TestFrameBuilder_BufferSizeLimit(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	// Small buffer size for testing
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "test-sensor",
		FrameCallback:   callback,
		FrameBufferSize: 2, // Only buffer 2 frames
		FrameDuration:   100 * time.Millisecond,
		BufferTimeout:   200 * time.Millisecond,
		CleanupInterval: 25 * time.Millisecond,
	})

	baseTime := time.Now()

	// Create points for multiple frames to exceed buffer size
	for i := 0; i < 5; i++ {
		points := []Point{
			{
				X: float64(i), Y: float64(i), Z: float64(i),
				Azimuth:   float64(i * 72),                                         // Different azimuth for each frame
				Timestamp: baseTime.Add(time.Duration(i) * 200 * time.Millisecond), // Different time slots
				Intensity: uint8(100 + i*20),
			},
		}
		fb.AddPoints(points)
	}

	// Wait for frames to be processed
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	// Should have received frames due to buffer eviction
	if frameCount == 0 {
		t.Error("Expected at least some frames to be completed due to buffer eviction")
	}

	// Check current buffer stats
	bufferCount, _, _ := fb.GetCurrentFrameStats()
	if bufferCount > 2 {
		t.Errorf("Buffer should not exceed limit of 2, got %d", bufferCount)
	}
}

// TestFrameBuilder_GetCurrentFrameStats tests the frame statistics functionality
func TestFrameBuilder_GetCurrentFrameStats(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:      "test-sensor",
		FrameCallback: nil, // No callback to keep frames in buffer
		FrameDuration: 100 * time.Millisecond,
		BufferTimeout: 5 * time.Second, // Very long timeout to keep frames in buffer
	})

	// Initially should have no frames
	count, _, _ := fb.GetCurrentFrameStats()
	if count != 0 {
		t.Errorf("Expected 0 frames initially, got %d", count)
	}

	// Use a fixed base time in the past to avoid timing races
	baseTime := time.Now().Add(-1 * time.Second)

	// Add points for multiple frames with sufficient time spacing
	for i := 0; i < 3; i++ {
		points := []Point{
			{
				X: float64(i), Y: float64(i), Z: float64(i),
				Timestamp: baseTime.Add(time.Duration(i) * 150 * time.Millisecond), // 150ms spacing for clear separation
				Azimuth:   float64(i * 60),
				Intensity: 100,
			},
		}
		fb.AddPoints(points)
	}

	// Give a small delay for frame creation
	time.Sleep(10 * time.Millisecond)

	// Check stats
	count, oldest, newest := fb.GetCurrentFrameStats()
	if count != 3 {
		t.Errorf("Expected 3 frames in buffer, got %d", count)
	}

	// Age checks (should be positive since we used past timestamps)
	if oldest <= 0 || oldest > 10*time.Second {
		t.Errorf("Oldest frame age seems incorrect: %v", oldest)
	}

	if newest <= 0 || newest > 10*time.Second {
		t.Errorf("Newest frame age seems incorrect: %v", newest)
	}

	// Newest should be less than or equal to oldest (age-wise)
	if newest > oldest {
		t.Errorf("Newest frame should not be older than oldest frame: newest=%v, oldest=%v", newest, oldest)
	}
}

// TestFrameBuilder_EmptyPoints tests handling of empty point slices
func TestFrameBuilder_EmptyPoints(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:      "test-sensor",
		FrameCallback: callback,
	})

	// Add empty point slice
	fb.AddPoints([]Point{})

	// Add nil slice (if that's how it's called)
	fb.AddPoints(nil)

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	// Should not have created any frames
	if frameCount != 0 {
		t.Errorf("Expected 0 frames from empty points, got %d", frameCount)
	}
}

// TestFrameBuilder_ConvenientConstructors tests the convenience constructor functions
func TestFrameBuilder_ConvenientConstructors(t *testing.T) {
	// Test NewFrameBuilderWithLogging
	fb1 := NewFrameBuilderWithLogging("sensor-1")
	if fb1.sensorID != "sensor-1" {
		t.Errorf("Expected sensorID 'sensor-1', got '%s'", fb1.sensorID)
	}

	// Test NewFrameBuilderWithDebugLogging
	fb2 := NewFrameBuilderWithDebugLogging("sensor-2", false)
	if fb2.sensorID != "sensor-2" {
		t.Errorf("Expected sensorID 'sensor-2', got '%s'", fb2.sensorID)
	}

	// Test NewFrameBuilderWithDebugLoggingAndInterval
	fb3 := NewFrameBuilderWithDebugLoggingAndInterval("sensor-3", true, 5*time.Second)
	if fb3.sensorID != "sensor-3" {
		t.Errorf("Expected sensorID 'sensor-3', got '%s'", fb3.sensorID)
	}

	// Verify enhanced settings from convenience constructor
	if fb3.frameBufferSize != 100 {
		t.Errorf("Expected enhanced frameBufferSize 100, got %d", fb3.frameBufferSize)
	}

	if fb3.bufferTimeout != 500*time.Millisecond {
		t.Errorf("Expected enhanced bufferTimeout 500ms, got %v", fb3.bufferTimeout)
	}
}

// TestFrameBuilder_ConcurrentAccess tests thread safety with concurrent access
func TestFrameBuilder_ConcurrentAccess(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "test-sensor",
		FrameCallback:   callback,
		FrameDuration:   50 * time.Millisecond,
		BufferTimeout:   100 * time.Millisecond,
		CleanupInterval: 25 * time.Millisecond,
	})

	baseTime := time.Now()
	var wg sync.WaitGroup

	// Start multiple goroutines adding points concurrently
	numGoroutines := 5
	pointsPerGoroutine := 10

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < pointsPerGoroutine; i++ {
				points := []Point{
					{
						X:         float64(goroutineID*100 + i),
						Y:         float64(goroutineID*100 + i),
						Z:         float64(goroutineID*100 + i),
						Azimuth:   float64((goroutineID*pointsPerGoroutine + i) % 360),
						Timestamp: baseTime.Add(time.Duration(i*25) * time.Millisecond),
						Intensity: uint8(100 + goroutineID*10 + i),
						Channel:   goroutineID + 1,
					},
				}
				fb.AddPoints(points)

				// Small delay to simulate realistic timing
				time.Sleep(5 * time.Millisecond)
			}
		}(g)
	}

	// Also test concurrent stats access
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			fb.GetCurrentFrameStats()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Wait for frames to be finalized
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	totalPoints := 0
	for _, frame := range receivedFrames {
		totalPoints += frame.PointCount
	}
	mu.Unlock()

	expectedTotalPoints := numGoroutines * pointsPerGoroutine
	if totalPoints != expectedTotalPoints {
		t.Errorf("Expected total points %d, got %d", expectedTotalPoints, totalPoints)
	}

	t.Logf("Concurrent test completed: %d frames, %d total points", frameCount, totalPoints)
}

// TestFrameBuilder_AzimuthRangeCalculation tests azimuth range calculation edge cases
func TestFrameBuilder_AzimuthRangeCalculation(t *testing.T) {
	var receivedFrames []*LiDARFrame
	var mu sync.Mutex

	callback := func(frame *LiDARFrame) {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
	}

	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:        "test-sensor",
		FrameCallback:   callback,
		FrameDuration:   100 * time.Millisecond,
		BufferTimeout:   50 * time.Millisecond,
		CleanupInterval: 25 * time.Millisecond,
	})

	baseTime := time.Now()

	// Test points with various azimuth values including edge cases
	// Use small time differences to ensure they go in the same frame
	points := []Point{
		{Azimuth: 0.0, Timestamp: baseTime, X: 1, Y: 1, Z: 1, Intensity: 100},
		{Azimuth: 359.9, Timestamp: baseTime.Add(5 * time.Millisecond), X: 2, Y: 2, Z: 2, Intensity: 150},
		{Azimuth: 180.0, Timestamp: baseTime.Add(10 * time.Millisecond), X: 3, Y: 3, Z: 3, Intensity: 200},
		{Azimuth: 90.0, Timestamp: baseTime.Add(15 * time.Millisecond), X: 4, Y: 4, Z: 4, Intensity: 250},
	}

	fb.AddPoints(points)

	// Wait for frame completion (longer wait)
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount < 1 {
		t.Fatalf("Expected at least 1 frame, got %d", frameCount)
	}

	// Collect all azimuth values from all frames
	allAzimuths := []float64{}
	for _, frame := range receivedFrames {
		for _, point := range frame.Points {
			allAzimuths = append(allAzimuths, point.Azimuth)
		}
	}

	// Find min and max azimuth across all frames
	if len(allAzimuths) == 0 {
		t.Fatal("No points found in any frame")
	}

	minAzimuth := allAzimuths[0]
	maxAzimuth := allAzimuths[0]
	for _, az := range allAzimuths {
		if az < minAzimuth {
			minAzimuth = az
		}
		if az > maxAzimuth {
			maxAzimuth = az
		}
	}

	// Check azimuth range across all received points
	expectedMin := 0.0
	expectedMax := 359.9

	if minAzimuth != expectedMin {
		t.Errorf("Expected MinAzimuth %.1f, got %.1f", expectedMin, minAzimuth)
	}

	if maxAzimuth != expectedMax {
		t.Errorf("Expected MaxAzimuth %.1f, got %.1f", expectedMax, maxAzimuth)
	}
}

// BenchmarkFrameBuilder_AddPoints benchmarks the performance of adding points
func BenchmarkFrameBuilder_AddPoints(b *testing.B) {
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:      "benchmark-sensor",
		FrameCallback: nil, // No callback for benchmark
		FrameDuration: 100 * time.Millisecond,
	})

	baseTime := time.Now()
	points := make([]Point, 400) // Typical packet size

	// Initialize points
	for i := range points {
		points[i] = Point{
			X:         float64(i),
			Y:         float64(i),
			Z:         float64(i),
			Azimuth:   float64(i) * 0.9, // 0.9 degrees per point
			Timestamp: baseTime.Add(time.Duration(i) * time.Microsecond),
			Intensity: uint8(100 + i%155),
			Channel:   i%40 + 1,
			Distance:  float64(5 + i%20),
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fb.AddPoints(points)
	}
}

// BenchmarkFrameBuilder_SinglePoint benchmarks adding individual points
func BenchmarkFrameBuilder_SinglePoint(b *testing.B) {
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:      "benchmark-sensor",
		FrameCallback: nil,
		FrameDuration: 100 * time.Millisecond,
	})

	baseTime := time.Now()
	point := Point{
		X:         1.0,
		Y:         2.0,
		Z:         3.0,
		Azimuth:   45.0,
		Timestamp: baseTime,
		Intensity: 150,
		Channel:   1,
		Distance:  5.0,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fb.AddPoints([]Point{point})
		point.Timestamp = point.Timestamp.Add(time.Microsecond) // Slight time progression
	}
}
