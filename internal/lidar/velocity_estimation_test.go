package lidar

import (
	"testing"
	"time"
)

func TestDefaultVelocityEstimationConfig(t *testing.T) {
	config := DefaultVelocityEstimationConfig()

	if config.SearchRadius <= 0 {
		t.Errorf("SearchRadius should be positive, got %v", config.SearchRadius)
	}
	if config.MaxVelocityMps <= 0 {
		t.Errorf("MaxVelocityMps should be positive, got %v", config.MaxVelocityMps)
	}
	if config.MinConfidence < 0 || config.MinConfidence > 1 {
		t.Errorf("MinConfidence should be [0, 1], got %v", config.MinConfidence)
	}
}

func TestNewFrameHistory(t *testing.T) {
	t.Run("default capacity", func(t *testing.T) {
		h := NewFrameHistory(0)
		if h.capacity != 5 {
			t.Errorf("expected default capacity 5, got %d", h.capacity)
		}
	})

	t.Run("custom capacity", func(t *testing.T) {
		h := NewFrameHistory(10)
		if h.capacity != 10 {
			t.Errorf("expected capacity 10, got %d", h.capacity)
		}
	})
}

func TestFrameHistory_AddAndPrevious(t *testing.T) {
	h := NewFrameHistory(3)

	// Add first frame
	now := time.Now()
	frame1 := PointVelocityFrame{
		Points:    []PointVelocity{{X: 1, Y: 1, Z: 1}},
		Timestamp: now,
	}
	h.Add(frame1)

	// Most recent should be frame1
	prev := h.Previous(0)
	if prev == nil {
		t.Fatal("Previous(0) returned nil")
	}
	if len(prev.Points) != 1 || prev.Points[0].X != 1 {
		t.Errorf("Previous(0) returned wrong frame")
	}

	// Previous(1) should be nil (not populated yet)
	if h.Previous(1) != nil && !h.Previous(1).Timestamp.IsZero() {
		t.Errorf("Previous(1) should be nil or zero timestamp")
	}

	// Add second frame
	frame2 := PointVelocityFrame{
		Points:    []PointVelocity{{X: 2, Y: 2, Z: 2}},
		Timestamp: now.Add(100 * time.Millisecond),
	}
	h.Add(frame2)

	// Most recent should now be frame2
	prev = h.Previous(0)
	if prev == nil || prev.Points[0].X != 2 {
		t.Errorf("Previous(0) should be frame2")
	}

	// Previous(1) should now be frame1
	prev = h.Previous(1)
	if prev == nil || prev.Points[0].X != 1 {
		t.Errorf("Previous(1) should be frame1")
	}
}

func TestNewVelocityEstimator(t *testing.T) {
	config := DefaultVelocityEstimationConfig()
	ve := NewVelocityEstimator(config, 5)

	if ve == nil {
		t.Fatal("NewVelocityEstimator returned nil")
	}
	if ve.History == nil {
		t.Error("VelocityEstimator.History is nil")
	}
	if ve.Config.SearchRadius != config.SearchRadius {
		t.Error("Config not properly set")
	}
}

func TestVelocityEstimator_EstimateVelocities_EmptyInput(t *testing.T) {
	ve := NewVelocityEstimator(DefaultVelocityEstimationConfig(), 5)

	result := ve.EstimateVelocities(nil, time.Now(), "sensor-1")
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}

	result = ve.EstimateVelocities([]WorldPoint{}, time.Now(), "sensor-1")
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestVelocityEstimator_EstimateVelocities_FirstFrame(t *testing.T) {
	ve := NewVelocityEstimator(DefaultVelocityEstimationConfig(), 5)

	points := []WorldPoint{
		{X: 0, Y: 0, Z: 0, Intensity: 100, Timestamp: time.Now()},
		{X: 1, Y: 1, Z: 0, Intensity: 100, Timestamp: time.Now()},
	}

	result := ve.EstimateVelocities(points, time.Now(), "sensor-1")

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// First frame should have zero velocity (no previous frame)
	for i, pv := range result {
		if pv.VX != 0 || pv.VY != 0 || pv.VZ != 0 {
			t.Errorf("point %d should have zero velocity for first frame", i)
		}
		if pv.VelocityConfidence != 0 {
			t.Errorf("point %d should have zero confidence for first frame", i)
		}
		if pv.CorrespondingPointIdx != -1 {
			t.Errorf("point %d should have no correspondence for first frame", i)
		}
	}
}

func TestVelocityEstimator_EstimateVelocities_StationaryPoints(t *testing.T) {
	ve := NewVelocityEstimator(DefaultVelocityEstimationConfig(), 5)

	now := time.Now()

	// First frame - establish baseline
	points1 := []WorldPoint{
		{X: 0, Y: 0, Z: 0, Intensity: 100, Timestamp: now},
		{X: 5, Y: 5, Z: 0, Intensity: 100, Timestamp: now},
	}
	ve.EstimateVelocities(points1, now, "sensor-1")

	// Second frame - same positions (stationary)
	later := now.Add(100 * time.Millisecond)
	points2 := []WorldPoint{
		{X: 0, Y: 0, Z: 0, Intensity: 100, Timestamp: later},
		{X: 5, Y: 5, Z: 0, Intensity: 100, Timestamp: later},
	}
	result := ve.EstimateVelocities(points2, later, "sensor-1")

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Stationary points should have near-zero velocity
	for i, pv := range result {
		velMag := pv.VX*pv.VX + pv.VY*pv.VY + pv.VZ*pv.VZ
		if velMag > 0.1 { // Allow small numerical error
			t.Errorf("point %d should have near-zero velocity, got (%.2f, %.2f, %.2f)",
				i, pv.VX, pv.VY, pv.VZ)
		}
	}
}

func TestVelocityEstimator_EstimateVelocities_MovingPoint(t *testing.T) {
	ve := NewVelocityEstimator(DefaultVelocityEstimationConfig(), 5)

	now := time.Now()

	// First frame
	points1 := []WorldPoint{
		{X: 0, Y: 0, Z: 0, Intensity: 100, Timestamp: now},
	}
	ve.EstimateVelocities(points1, now, "sensor-1")

	// Second frame - point moved 1m in 0.1s = 10 m/s
	later := now.Add(100 * time.Millisecond)
	points2 := []WorldPoint{
		{X: 1, Y: 0, Z: 0, Intensity: 100, Timestamp: later},
	}
	result := ve.EstimateVelocities(points2, later, "sensor-1")

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	pv := result[0]

	// Should detect the correspondence and compute velocity
	if pv.CorrespondingPointIdx < 0 {
		t.Error("expected correspondence to be found")
	}

	// Velocity should be approximately 10 m/s in X direction
	expectedVX := 10.0 // 1m / 0.1s
	tolerance := 0.5

	if pv.VX < expectedVX-tolerance || pv.VX > expectedVX+tolerance {
		t.Errorf("expected VX ~%.1f, got %.2f", expectedVX, pv.VX)
	}
	if pv.VY > tolerance || pv.VY < -tolerance {
		t.Errorf("expected VY ~0, got %.2f", pv.VY)
	}
}

func TestVelocityEstimator_RejectsImplausibleVelocity(t *testing.T) {
	config := DefaultVelocityEstimationConfig()
	config.MaxVelocityMps = 30.0 // Set a low max
	ve := NewVelocityEstimator(config, 5)

	now := time.Now()

	// First frame
	points1 := []WorldPoint{
		{X: 0, Y: 0, Z: 0, Intensity: 100, Timestamp: now},
	}
	ve.EstimateVelocities(points1, now, "sensor-1")

	// Second frame - point moved 10m in 0.1s = 100 m/s (exceeds max)
	later := now.Add(100 * time.Millisecond)
	points2 := []WorldPoint{
		{X: 10, Y: 0, Z: 0, Intensity: 100, Timestamp: later},
	}
	result := ve.EstimateVelocities(points2, later, "sensor-1")

	pv := result[0]

	// Should reject the implausible velocity match
	if pv.CorrespondingPointIdx != -1 {
		t.Error("expected implausible velocity to be rejected")
	}
	if pv.VelocityConfidence > 0 {
		t.Errorf("expected zero confidence for rejected velocity, got %.2f", pv.VelocityConfidence)
	}
}

func TestVelocityEstimator_GetSetConfig(t *testing.T) {
	ve := NewVelocityEstimator(DefaultVelocityEstimationConfig(), 5)

	original := ve.GetConfig()

	newConfig := VelocityEstimationConfig{
		SearchRadius:              3.0,
		MaxVelocityMps:            100.0,
		VelocityVarianceThreshold: 5.0,
		MinConfidence:             0.5,
		NeighborRadius:            2.0,
		MinNeighborsForContext:    5,
	}

	ve.SetConfig(newConfig)

	updated := ve.GetConfig()
	if updated.SearchRadius != 3.0 {
		t.Errorf("SearchRadius not updated: got %.1f, want 3.0", updated.SearchRadius)
	}
	if updated.MaxVelocityMps != 100.0 {
		t.Errorf("MaxVelocityMps not updated: got %.1f, want 100.0", updated.MaxVelocityMps)
	}

	// Restore original
	ve.SetConfig(original)
}
