package lidar

import (
	"testing"
	"time"
)

func TestNewVelocityCoherentTracker(t *testing.T) {
	config := DefaultVelocityCoherentTrackerConfig()
	tracker := NewVelocityCoherentTracker(config)

	if tracker == nil {
		t.Fatal("NewVelocityCoherentTracker returned nil")
	}

	if tracker.velocityEstimator == nil {
		t.Error("Velocity estimator not initialized")
	}

	if tracker.longTailManager == nil {
		t.Error("Long tail manager not initialized")
	}

	if tracker.tracks == nil {
		t.Error("Tracks map not initialized")
	}

	if tracker.nextTrackID != 1 {
		t.Errorf("Expected nextTrackID=1, got %d", tracker.nextTrackID)
	}
}

func TestVelocityCoherentTrackerUpdate_EmptyPoints(t *testing.T) {
	tracker := NewVelocityCoherentTracker(DefaultVelocityCoherentTrackerConfig())

	// Process empty frame - should not panic
	tracker.Update(nil, time.Now(), "test-sensor")

	activeTracks := tracker.GetActiveTracks()
	if len(activeTracks) != 0 {
		t.Errorf("Expected 0 active tracks, got %d", len(activeTracks))
	}
}

func TestVelocityCoherentTrackerUpdate_SingleCluster(t *testing.T) {
	tracker := NewVelocityCoherentTracker(DefaultVelocityCoherentTrackerConfig())

	// Create a cluster of points
	now := time.Now()
	points := []WorldPoint{
		{X: 5.0, Y: 0.0, Z: 0.5},
		{X: 5.1, Y: 0.1, Z: 0.5},
		{X: 5.2, Y: -0.1, Z: 0.5},
		{X: 4.9, Y: 0.05, Z: 0.6},
		{X: 5.05, Y: -0.05, Z: 0.5},
	}

	// First update - may or may not create tracks depending on velocity estimation
	tracker.Update(points, now, "test-sensor")

	// Second update with slightly moved points (simulating movement)
	now = now.Add(100 * time.Millisecond)
	movedPoints := make([]WorldPoint, len(points))
	for i, p := range points {
		movedPoints[i] = WorldPoint{
			X: p.X + 0.5, // Moving at 5 m/s in X direction
			Y: p.Y,
			Z: p.Z,
		}
	}
	tracker.Update(movedPoints, now, "test-sensor")

	// After two frames with movement, we might have tracks
	// The exact behavior depends on velocity estimation configuration
	activeTracks := tracker.GetActiveTracks()
	// Just verify no panic occurred
	t.Logf("Active tracks after 2 updates: %d", len(activeTracks))
}

func TestVelocityCoherentTrackerUpdate_MultipleFrames(t *testing.T) {
	config := DefaultVelocityCoherentTrackerConfig()
	config.HitsToConfirm = 3
	tracker := NewVelocityCoherentTracker(config)

	baseTime := time.Now()

	// Simulate a moving vehicle
	for i := 0; i < 10; i++ {
		timestamp := baseTime.Add(time.Duration(i*100) * time.Millisecond)
		offset := float64(i) * 0.5 // 5 m/s movement

		points := []WorldPoint{
			{X: 5.0 + offset, Y: 0.0, Z: 0.5},
			{X: 5.1 + offset, Y: 0.1, Z: 0.5},
			{X: 5.2 + offset, Y: -0.1, Z: 0.5},
			{X: 4.9 + offset, Y: 0.05, Z: 0.6},
			{X: 5.05 + offset, Y: -0.05, Z: 0.5},
		}

		tracker.Update(points, timestamp, "test-sensor")
	}

	// Verify tracker state
	activeTracks := tracker.GetActiveTracks()
	confirmedTracks := tracker.GetConfirmedTracks()

	t.Logf("Active tracks: %d, Confirmed tracks: %d", len(activeTracks), len(confirmedTracks))
}

func TestVelocityCoherentTrackerGetConfirmedTracks(t *testing.T) {
	tracker := NewVelocityCoherentTracker(DefaultVelocityCoherentTrackerConfig())

	// Initially no confirmed tracks
	confirmed := tracker.GetConfirmedTracks()
	if len(confirmed) != 0 {
		t.Errorf("Expected 0 confirmed tracks initially, got %d", len(confirmed))
	}
}

func TestVelocityCoherentTrackerGetCompletedTracks(t *testing.T) {
	tracker := NewVelocityCoherentTracker(DefaultVelocityCoherentTrackerConfig())

	// Initially no completed tracks
	completed := tracker.GetCompletedTracks()
	if len(completed) != 0 {
		t.Errorf("Expected 0 completed tracks initially, got %d", len(completed))
	}
}

func TestVelocityCoherentTrackerConcurrentAccess(t *testing.T) {
	tracker := NewVelocityCoherentTracker(DefaultVelocityCoherentTrackerConfig())

	// Start concurrent readers
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = tracker.GetActiveTracks()
				_ = tracker.GetConfirmedTracks()
				_ = tracker.GetCompletedTracks()
			}
			done <- true
		}()
	}

	// Concurrent writer
	go func() {
		for j := 0; j < 100; j++ {
			timestamp := time.Now()
			points := []WorldPoint{
				{X: float64(j), Y: 0.0, Z: 0.5},
			}
			tracker.Update(points, timestamp, "test-sensor")
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 6; i++ {
		<-done
	}
}

func TestVelocityCoherentTrackerReset(t *testing.T) {
	tracker := NewVelocityCoherentTracker(DefaultVelocityCoherentTrackerConfig())

	// Add some updates
	for i := 0; i < 5; i++ {
		timestamp := time.Now()
		points := []WorldPoint{
			{X: float64(i), Y: 0.0, Z: 0.5},
			{X: float64(i) + 0.1, Y: 0.1, Z: 0.5},
		}
		tracker.Update(points, timestamp, "test-sensor")
	}

	// Reset
	tracker.Reset()

	// Verify reset state
	if len(tracker.GetActiveTracks()) != 0 {
		t.Error("Expected no active tracks after reset")
	}
	if len(tracker.GetConfirmedTracks()) != 0 {
		t.Error("Expected no confirmed tracks after reset")
	}
	if len(tracker.GetCompletedTracks()) != 0 {
		t.Error("Expected no completed tracks after reset")
	}
}

func TestDefaultVelocityCoherentTrackerConfig(t *testing.T) {
	config := DefaultVelocityCoherentTrackerConfig()

	if config.MaxTracks <= 0 {
		t.Errorf("MaxTracks should be positive, got %d", config.MaxTracks)
	}

	if config.MaxMisses <= 0 {
		t.Errorf("MaxMisses should be positive, got %d", config.MaxMisses)
	}

	if config.HitsToConfirm <= 0 {
		t.Errorf("HitsToConfirm should be positive, got %d", config.HitsToConfirm)
	}
}
