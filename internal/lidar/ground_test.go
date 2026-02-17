package lidar

import (
	"testing"
	"time"
)

func TestDefaultHeightBandFilter(t *testing.T) {
	filter := DefaultHeightBandFilter()

	if filter.FloorHeightM != -2.8 {
		t.Errorf("Expected FloorHeightM=-2.8, got %f", filter.FloorHeightM)
	}
	if filter.CeilingHeightM != 1.5 {
		t.Errorf("Expected CeilingHeightM=1.5, got %f", filter.CeilingHeightM)
	}
}

func TestHeightBandFilter_NilInput(t *testing.T) {
	filter := DefaultHeightBandFilter()
	result := filter.FilterVertical(nil)

	if result != nil {
		t.Errorf("Expected nil output for nil input, got %v", result)
	}
}

func TestHeightBandFilter_EmptySlice(t *testing.T) {
	filter := DefaultHeightBandFilter()
	empty := []WorldPoint{}
	result := filter.FilterVertical(empty)

	if result != nil {
		t.Errorf("Expected nil output for empty slice, got %v", result)
	}
}

func TestHeightBandFilter_AllWithinBand(t *testing.T) {
	filter := NewHeightBandFilter(0.2, 3.0)

	input := []WorldPoint{
		{X: 10.0, Y: 20.0, Z: 0.5, Intensity: 100, Timestamp: time.Now()},
		{X: 11.0, Y: 21.0, Z: 1.2, Intensity: 150, Timestamp: time.Now()},
		{X: 12.0, Y: 22.0, Z: 2.8, Intensity: 200, Timestamp: time.Now()},
	}

	output := filter.FilterVertical(input)

	if len(output) != 3 {
		t.Errorf("Expected all 3 points retained, got %d", len(output))
	}

	// Verify stats
	proc, kept, below, above := filter.Stats()
	if proc != 3 || kept != 3 || below != 0 || above != 0 {
		t.Errorf("Stats: processed=%d kept=%d below=%d above=%d", proc, kept, below, above)
	}
}

func TestHeightBandFilter_MixedElevations(t *testing.T) {
	filter := NewHeightBandFilter(0.2, 3.0)

	ts := time.Now()
	input := []WorldPoint{
		{X: 1.0, Y: 2.0, Z: 0.05, Intensity: 100, Timestamp: ts, SensorID: "S1"}, // below floor
		{X: 2.0, Y: 3.0, Z: 0.15, Intensity: 110, Timestamp: ts, SensorID: "S1"}, // below floor
		{X: 3.0, Y: 4.0, Z: 0.25, Intensity: 120, Timestamp: ts, SensorID: "S1"}, // in band
		{X: 4.0, Y: 5.0, Z: 1.50, Intensity: 130, Timestamp: ts, SensorID: "S1"}, // in band
		{X: 5.0, Y: 6.0, Z: 2.90, Intensity: 140, Timestamp: ts, SensorID: "S1"}, // in band
		{X: 6.0, Y: 7.0, Z: 3.50, Intensity: 150, Timestamp: ts, SensorID: "S1"}, // above ceiling
		{X: 7.0, Y: 8.0, Z: 5.00, Intensity: 160, Timestamp: ts, SensorID: "S1"}, // above ceiling
	}

	output := filter.FilterVertical(input)

	if len(output) != 3 {
		t.Errorf("Expected 3 points in band [0.2, 3.0], got %d", len(output))
	}

	// Verify correct points retained
	expectedZ := []float64{0.25, 1.50, 2.90}
	for i, pt := range output {
		if pt.Z != expectedZ[i] {
			t.Errorf("Point %d: expected Z=%f, got Z=%f", i, expectedZ[i], pt.Z)
		}
	}

	// Check statistics
	proc, kept, below, above := filter.Stats()
	if proc != 7 {
		t.Errorf("Expected 7 processed, got %d", proc)
	}
	if kept != 3 {
		t.Errorf("Expected 3 kept, got %d", kept)
	}
	if below != 2 {
		t.Errorf("Expected 2 below floor, got %d", below)
	}
	if above != 2 {
		t.Errorf("Expected 2 above ceiling, got %d", above)
	}
}

func TestHeightBandFilter_BoundaryValues(t *testing.T) {
	filter := NewHeightBandFilter(0.2, 3.0)

	input := []WorldPoint{
		{X: 1.0, Y: 1.0, Z: 0.2, Intensity: 100, Timestamp: time.Now()},   // exactly at floor (should keep)
		{X: 2.0, Y: 2.0, Z: 3.0, Intensity: 110, Timestamp: time.Now()},   // exactly at ceiling (should keep)
		{X: 3.0, Y: 3.0, Z: 0.199, Intensity: 120, Timestamp: time.Now()}, // just below floor
		{X: 4.0, Y: 4.0, Z: 3.001, Intensity: 130, Timestamp: time.Now()}, // just above ceiling
	}

	output := filter.FilterVertical(input)

	// Boundaries are inclusive: >= floor and <= ceiling
	if len(output) != 2 {
		t.Errorf("Expected 2 points at boundaries, got %d", len(output))
	}

	if output[0].Z != 0.2 {
		t.Errorf("Expected Z=0.2 (floor), got %f", output[0].Z)
	}
	if output[1].Z != 3.0 {
		t.Errorf("Expected Z=3.0 (ceiling), got %f", output[1].Z)
	}
}

func TestHeightBandFilter_PreservesAttributes(t *testing.T) {
	filter := DefaultHeightBandFilter()

	ts := time.Now()
	input := []WorldPoint{
		{X: 15.5, Y: 25.3, Z: 1.2, Intensity: 185, Timestamp: ts, SensorID: "pandar-40p"},
	}

	output := filter.FilterVertical(input)

	if len(output) != 1 {
		t.Fatalf("Expected 1 point, got %d", len(output))
	}

	pt := output[0]
	if pt.X != 15.5 || pt.Y != 25.3 || pt.Z != 1.2 {
		t.Errorf("Coordinates changed: got (%.1f, %.1f, %.1f)", pt.X, pt.Y, pt.Z)
	}
	if pt.Intensity != 185 {
		t.Errorf("Intensity changed: expected 185, got %d", pt.Intensity)
	}
	if pt.SensorID != "pandar-40p" {
		t.Errorf("SensorID changed: expected 'pandar-40p', got '%s'", pt.SensorID)
	}
	if !pt.Timestamp.Equal(ts) {
		t.Errorf("Timestamp changed")
	}
}

func TestHeightBandFilter_StatsReset(t *testing.T) {
	filter := DefaultHeightBandFilter()

	// Run filter once
	input := []WorldPoint{
		{Z: 0.1}, {Z: 0.5}, {Z: 3.5},
	}
	filter.FilterVertical(input)

	proc1, _, _, _ := filter.Stats()
	if proc1 != 3 {
		t.Fatalf("Expected 3 processed before reset, got %d", proc1)
	}

	// Reset and verify
	filter.ResetStats()
	proc2, kept2, below2, above2 := filter.Stats()
	if proc2 != 0 || kept2 != 0 || below2 != 0 || above2 != 0 {
		t.Errorf("After reset: processed=%d kept=%d below=%d above=%d", proc2, kept2, below2, above2)
	}
}

func TestHeightBandFilter_StatisticsAccumulation(t *testing.T) {
	filter := NewHeightBandFilter(1.0, 2.0)

	// First batch
	batch1 := []WorldPoint{
		{Z: 0.5}, {Z: 1.5}, {Z: 2.5},
	}
	filter.FilterVertical(batch1)

	// Second batch
	batch2 := []WorldPoint{
		{Z: 0.8}, {Z: 1.2}, {Z: 1.8}, {Z: 2.2},
	}
	filter.FilterVertical(batch2)

	// Check cumulative statistics
	// Filter range [1.0, 2.0] (inclusive boundaries)
	// batch1: 0.5 (below), 1.5 (kept), 2.5 (above)
	// batch2: 0.8 (below), 1.2 (kept), 1.8 (kept), 2.2 (above)
	// Total: 7 processed, 3 kept, 2 below, 2 above
	proc, kept, below, above := filter.Stats()
	if proc != 7 {
		t.Errorf("Expected 7 total processed, got %d", proc)
	}
	if kept != 3 {
		t.Errorf("Expected 3 total kept (1.5, 1.2, 1.8), got %d", kept)
	}
	if below != 2 {
		t.Errorf("Expected 2 below floor, got %d", below)
	}
	if above != 2 {
		t.Errorf("Expected 2 above ceiling, got %d", above)
	}
}
