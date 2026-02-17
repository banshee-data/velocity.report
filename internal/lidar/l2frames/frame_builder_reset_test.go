package l2frames

import (
	"testing"
)

// TestFrameBuilder_Reset verifies that Reset() correctly clears all frame state
// preventing data mixing when switching sources/timestamps.
func TestFrameBuilder_Reset(t *testing.T) {
	// Setup
	frameCallbackCalled := false
	fbConfig := FrameBuilderConfig{
		SensorID: "test-sensor",
		FrameCallback: func(f *LiDARFrame) {
			frameCallbackCalled = true
		},
		MinFramePoints: 10,
	}
	fb := NewFrameBuilder(fbConfig)

	// Add some data (incomplete frame)
	points := []PointPolar{
		{Channel: 1, Azimuth: 0.0, Distance: 10.0, Timestamp: 1000},
		{Channel: 1, Azimuth: 10.0, Distance: 10.0, Timestamp: 1001},
	}
	fb.AddPointsPolar(points)

	// Verify buffer has points
	fb.mu.Lock()
	if fb.currentFrame == nil || len(fb.currentFrame.Points) == 0 {
		t.Errorf("Expected points in currentFrame")
	}
	fb.mu.Unlock()

	// Perform Reset
	fb.Reset()

	// Verify buffer is empty
	fb.mu.Lock()
	if len(fb.frameBuffer) != 0 {
		t.Errorf("Reset() failed to clear frameBuffer, length: %d", len(fb.frameBuffer))
	}
	if fb.currentFrame != nil {
		t.Errorf("Reset() failed to nil currentFrame")
	}
	// Verify critical state variables are reset
	if fb.lastSequence != 0 {
		t.Errorf("Reset() failed to clear lastSequence")
	}
	if fb.lastAzimuth != 0 {
		t.Errorf("Reset() failed to clear lastAzimuth")
	}
	if len(fb.sequenceGaps) != 0 {
		t.Errorf("Reset() failed to clear sequenceGaps")
	}
	fb.mu.Unlock()

	// Verify callback NOT called during Reset (should arguably just drop data)
	if frameCallbackCalled {
		t.Errorf("Reset() shouldn't trigger callback for incomplete frame")
	}

	// Add new point (simulating new stream start)
	// If reset failed, this might be appended to old data or cause gap detection issues
	newPoints := []PointPolar{
		{Channel: 1, Azimuth: 0.0, Distance: 5.0, Timestamp: 5000},
	}
	fb.AddPointsPolar(newPoints)

	fb.mu.Lock()
	if fb.currentFrame == nil {
		t.Fatalf("Failed to start new frame after reset")
	}
	if len(fb.currentFrame.Points) != 1 {
		t.Errorf("New frame should have exactly 1 point, got %d", len(fb.currentFrame.Points))
	}
	fb.mu.Unlock()
}
