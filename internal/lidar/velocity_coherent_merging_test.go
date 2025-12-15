package lidar

import (
	"testing"
)

// =============================================================================
// Phase 5: Track Fragment Merging Tests
// =============================================================================

func TestDefaultMergeConfig(t *testing.T) {
	config := DefaultMergeConfig()

	if config.MaxTimeGapSeconds <= 0 {
		t.Error("MaxTimeGapSeconds should be positive")
	}
	if config.MaxPositionErrorMeters <= 0 {
		t.Error("MaxPositionErrorMeters should be positive")
	}
	if config.MaxVelocityDifferenceMs <= 0 {
		t.Error("MaxVelocityDifferenceMs should be positive")
	}
	if config.MinAlignmentScore <= 0 || config.MinAlignmentScore > 1 {
		t.Error("MinAlignmentScore should be in (0, 1]")
	}
}

func TestDefaultSensorBoundary(t *testing.T) {
	boundary := DefaultSensorBoundary()

	if boundary.MaxX <= boundary.MinX {
		t.Error("MaxX should be greater than MinX")
	}
	if boundary.MaxY <= boundary.MinY {
		t.Error("MaxY should be greater than MinY")
	}
	if boundary.Margin <= 0 {
		t.Error("Margin should be positive")
	}
}

func TestSensorBoundary_IsNearBoundary(t *testing.T) {
	boundary := SensorBoundary{
		MinX:   -50.0,
		MaxX:   50.0,
		MinY:   -50.0,
		MaxY:   100.0,
		Margin: 2.0,
	}

	tests := []struct {
		x, y     float32
		expected bool
	}{
		{0, 0, false},    // Center
		{-49, 0, true},   // Near left edge
		{49, 0, true},    // Near right edge
		{0, -49, true},   // Near bottom edge
		{0, 99, true},    // Near top edge
		{-40, 80, false}, // Interior
	}

	for _, tt := range tests {
		result := boundary.IsNearBoundary(tt.x, tt.y)
		if result != tt.expected {
			t.Errorf("IsNearBoundary(%v, %v) = %v, want %v",
				tt.x, tt.y, result, tt.expected)
		}
	}
}

func TestNewFragmentMerger(t *testing.T) {
	merger := NewFragmentMerger(DefaultMergeConfig(), DefaultSensorBoundary())

	if merger == nil {
		t.Fatal("NewFragmentMerger returned nil")
	}
}

func TestFragmentMerger_DetectFragments(t *testing.T) {
	merger := NewFragmentMerger(DefaultMergeConfig(), DefaultSensorBoundary())

	tracks := []*VelocityCoherentTrack{
		{
			TrackID:        "track1",
			FirstUnixNanos: 1000000000,
			LastUnixNanos:  2000000000,
			History: []TrackPoint{
				{X: 0, Y: 0, Timestamp: 1000000000},
				{X: 1, Y: 0, Timestamp: 1100000000},
				{X: 2, Y: 0, Timestamp: 1200000000},
			},
		},
	}

	fragments := merger.DetectFragments(tracks)

	if len(fragments) != 1 {
		t.Fatalf("Expected 1 fragment, got %d", len(fragments))
	}

	frag := fragments[0]
	if frag.Track.TrackID != "track1" {
		t.Errorf("TrackID = %s, want track1", frag.Track.TrackID)
	}
	if frag.EntryPoint[0] != 0 {
		t.Errorf("EntryPoint[0] = %v, want 0", frag.EntryPoint[0])
	}
	if frag.ExitPoint[0] != 2 {
		t.Errorf("ExitPoint[0] = %v, want 2", frag.ExitPoint[0])
	}
}

func TestFragmentMerger_DetectFragments_BoundaryFlags(t *testing.T) {
	merger := NewFragmentMerger(DefaultMergeConfig(), DefaultSensorBoundary())

	tracks := []*VelocityCoherentTrack{
		{
			TrackID:        "track1",
			FirstUnixNanos: 1000000000,
			LastUnixNanos:  2000000000,
			History: []TrackPoint{
				{X: -49, Y: 0, Timestamp: 1000000000}, // Near left boundary
				{X: 0, Y: 0, Timestamp: 1500000000},
				{X: 49, Y: 0, Timestamp: 2000000000}, // Near right boundary
			},
		},
	}

	fragments := merger.DetectFragments(tracks)

	if len(fragments) != 1 {
		t.Fatalf("Expected 1 fragment, got %d", len(fragments))
	}

	frag := fragments[0]
	if !frag.HasNaturalEntry {
		t.Error("Expected HasNaturalEntry to be true")
	}
	if !frag.HasNaturalExit {
		t.Error("Expected HasNaturalExit to be true")
	}
}

func TestFragmentMerger_FindMergeCandidates(t *testing.T) {
	merger := NewFragmentMerger(DefaultMergeConfig(), DefaultSensorBoundary())

	// Create two fragments that should merge
	// Fragment 1: ends at (10, 0) moving right at 10 m/s
	// Fragment 2: starts at (15, 0) moving right at 10 m/s, 0.5 seconds later
	fragment1 := TrackFragment{
		Track:          &VelocityCoherentTrack{TrackID: "track1"},
		ExitPoint:      [2]float32{10, 0},
		ExitVelocity:   [2]float32{10, 0}, // 10 m/s right
		StartNanos:     1000000000,
		EndNanos:       2000000000,
		HasNaturalExit: false, // Disappeared mid-field
	}

	fragment2 := TrackFragment{
		Track:           &VelocityCoherentTrack{TrackID: "track2"},
		EntryPoint:      [2]float32{15, 0}, // 5m to the right
		EntryVelocity:   [2]float32{10, 0},
		StartNanos:      2500000000, // 0.5 seconds later
		EndNanos:        3500000000,
		HasNaturalEntry: false, // Appeared mid-field
	}

	fragments := []TrackFragment{fragment1, fragment2}
	candidates := merger.FindMergeCandidates(fragments)

	if len(candidates) != 1 {
		t.Fatalf("Expected 1 merge candidate, got %d", len(candidates))
	}

	candidate := candidates[0]
	if candidate.Earlier.Track.TrackID != "track1" {
		t.Errorf("Earlier track = %s, want track1", candidate.Earlier.Track.TrackID)
	}
	if candidate.Later.Track.TrackID != "track2" {
		t.Errorf("Later track = %s, want track2", candidate.Later.Track.TrackID)
	}
	if candidate.OverallScore < 0.7 {
		t.Errorf("OverallScore = %v, want >= 0.7", candidate.OverallScore)
	}
}

func TestFragmentMerger_NoMerge_NaturalExit(t *testing.T) {
	merger := NewFragmentMerger(DefaultMergeConfig(), DefaultSensorBoundary())

	// Fragment 1 has natural exit (went to boundary) - should not merge
	fragment1 := TrackFragment{
		Track:          &VelocityCoherentTrack{TrackID: "track1"},
		ExitPoint:      [2]float32{49, 0}, // Near boundary
		ExitVelocity:   [2]float32{10, 0},
		StartNanos:     1000000000,
		EndNanos:       2000000000,
		HasNaturalExit: true,
	}

	fragment2 := TrackFragment{
		Track:           &VelocityCoherentTrack{TrackID: "track2"},
		EntryPoint:      [2]float32{10, 0},
		EntryVelocity:   [2]float32{10, 0},
		StartNanos:      2500000000,
		EndNanos:        3500000000,
		HasNaturalEntry: false,
	}

	fragments := []TrackFragment{fragment1, fragment2}
	candidates := merger.FindMergeCandidates(fragments)

	if len(candidates) != 0 {
		t.Errorf("Expected 0 candidates (natural exit), got %d", len(candidates))
	}
}

func TestFragmentMerger_NoMerge_TimeGapTooLarge(t *testing.T) {
	config := DefaultMergeConfig()
	config.MaxTimeGapSeconds = 1.0 // Only allow 1 second gap

	merger := NewFragmentMerger(config, DefaultSensorBoundary())

	fragment1 := TrackFragment{
		Track:          &VelocityCoherentTrack{TrackID: "track1"},
		ExitPoint:      [2]float32{10, 0},
		ExitVelocity:   [2]float32{10, 0},
		StartNanos:     1000000000,
		EndNanos:       2000000000,
		HasNaturalExit: false,
	}

	fragment2 := TrackFragment{
		Track:           &VelocityCoherentTrack{TrackID: "track2"},
		EntryPoint:      [2]float32{15, 0},
		EntryVelocity:   [2]float32{10, 0},
		StartNanos:      5000000000, // 3 seconds later (too long)
		EndNanos:        6000000000,
		HasNaturalEntry: false,
	}

	fragments := []TrackFragment{fragment1, fragment2}
	candidates := merger.FindMergeCandidates(fragments)

	if len(candidates) != 0 {
		t.Errorf("Expected 0 candidates (time gap too large), got %d", len(candidates))
	}
}

func TestFragmentMerger_MergeFragments(t *testing.T) {
	merger := NewFragmentMerger(DefaultMergeConfig(), DefaultSensorBoundary())

	earlier := &TrackFragment{
		Track: &VelocityCoherentTrack{
			TrackID:            "track1",
			SensorID:           "sensor1",
			State:              TrackConfirmVC,
			FirstUnixNanos:     1000000000,
			LastUnixNanos:      2000000000,
			X:                  10,
			Y:                  0,
			VX:                 10,
			VY:                 0,
			Hits:               5,
			ObservationCount:   10,
			VelocityConfidence: 0.8,
			History: []TrackPoint{
				{X: 0, Y: 0, Timestamp: 1000000000},
				{X: 5, Y: 0, Timestamp: 1500000000},
				{X: 10, Y: 0, Timestamp: 2000000000},
			},
		},
		ExitPoint:    [2]float32{10, 0},
		ExitVelocity: [2]float32{10, 0},
		EndNanos:     2000000000,
	}

	later := &TrackFragment{
		Track: &VelocityCoherentTrack{
			TrackID:            "track2",
			SensorID:           "sensor1",
			State:              TrackConfirmVC,
			FirstUnixNanos:     2500000000,
			LastUnixNanos:      3500000000,
			X:                  25,
			Y:                  0,
			VX:                 10,
			VY:                 0,
			Hits:               4,
			ObservationCount:   8,
			VelocityConfidence: 0.9,
			History: []TrackPoint{
				{X: 15, Y: 0, Timestamp: 2500000000},
				{X: 20, Y: 0, Timestamp: 3000000000},
				{X: 25, Y: 0, Timestamp: 3500000000},
			},
		},
		EntryPoint: [2]float32{15, 0},
	}

	merged := merger.MergeFragments(earlier, later, 0.5)

	if merged == nil {
		t.Fatal("MergeFragments returned nil")
	}

	// Check merged track uses earlier ID
	if merged.TrackID != "track1" {
		t.Errorf("TrackID = %s, want track1", merged.TrackID)
	}

	// Check lifecycle spans both fragments
	if merged.FirstUnixNanos != 1000000000 {
		t.Errorf("FirstUnixNanos = %d, want 1000000000", merged.FirstUnixNanos)
	}
	if merged.LastUnixNanos != 3500000000 {
		t.Errorf("LastUnixNanos = %d, want 3500000000", merged.LastUnixNanos)
	}

	// Check position/velocity from later track
	if merged.X != 25 || merged.Y != 0 {
		t.Errorf("Position = (%v, %v), want (25, 0)", merged.X, merged.Y)
	}

	// Check aggregated counts
	if merged.Hits != 9 { // 5 + 4
		t.Errorf("Hits = %d, want 9", merged.Hits)
	}
	if merged.ObservationCount != 18 { // 10 + 8
		t.Errorf("ObservationCount = %d, want 18", merged.ObservationCount)
	}

	// Check history is merged
	if len(merged.History) < 6 { // 3 + 3 (or more with interpolation)
		t.Errorf("History length = %d, want >= 6", len(merged.History))
	}
}

func TestInterpolateGapPoints(t *testing.T) {
	exitPoint := [2]float32{10, 0}
	exitVelocity := [2]float32{10, 0} // 10 m/s
	entryPoint := [2]float32{15, 0}
	startNanos := int64(1000000000)
	endNanos := int64(1500000000) // 500ms gap

	points := interpolateGapPoints(exitPoint, exitVelocity, entryPoint, startNanos, endNanos)

	// 500ms gap with 100ms step = 5 points
	expectedPoints := 5
	if len(points) != expectedPoints {
		t.Errorf("Got %d interpolated points, want %d", len(points), expectedPoints)
	}

	// Check timestamps are in order
	for i := 1; i < len(points); i++ {
		if points[i].Timestamp <= points[i-1].Timestamp {
			t.Error("Interpolated points should have increasing timestamps")
		}
	}
}
