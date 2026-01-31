package lidar

import (
	"testing"
	"time"
)

// TestGetAllTracks tests retrieving all tracks including deleted ones.
func TestGetAllTracks(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 2
	config.MaxMisses = 1
	tracker := NewTracker(config)

	now := time.Now()

	// Create multiple tracks with different states
	clusters := []WorldCluster{
		{CentroidX: 5.0, CentroidY: 10.0, SensorID: "test"},
		{CentroidX: 15.0, CentroidY: 20.0, SensorID: "test"},
		{CentroidX: 25.0, CentroidY: 30.0, SensorID: "test"},
	}

	// Frame 1: Create tentative tracks
	tracker.Update(clusters, now)

	// Frame 2: Confirm tracks
	now = now.Add(100 * time.Millisecond)
	for i := range clusters {
		clusters[i].CentroidX += 0.1
	}
	tracker.Update(clusters, now)

	// GetAllTracks should return all tracks
	allTracks := tracker.GetAllTracks()

	if len(allTracks) != 3 {
		t.Errorf("Expected 3 tracks from GetAllTracks, got %d", len(allTracks))
	}
}

// TestGetAllTracks_IncludesDeleted tests that deleted tracks are included.
func TestGetAllTracks_IncludesDeleted(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 2
	config.MaxMisses = 1
	config.DeletedTrackGracePeriod = 0 // Keep deleted tracks
	tracker := NewTracker(config)

	now := time.Now()

	// Create a track
	cluster := WorldCluster{CentroidX: 5.0, CentroidY: 10.0, SensorID: "test"}
	tracker.Update([]WorldCluster{cluster}, now)

	// Confirm track
	now = now.Add(100 * time.Millisecond)
	cluster.CentroidX = 5.1
	tracker.Update([]WorldCluster{cluster}, now)

	// Miss the track to delete it
	now = now.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, now)

	now = now.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, now)

	// GetAllTracks should still include the deleted track
	allTracks := tracker.GetAllTracks()

	if len(allTracks) == 0 {
		t.Error("Expected at least 1 track from GetAllTracks (including deleted)")
	}

	// Find the deleted track
	hasDeleted := false
	for _, tr := range allTracks {
		if tr.State == TrackDeleted {
			hasDeleted = true
			break
		}
	}

	if !hasDeleted {
		t.Error("Expected to find a deleted track in GetAllTracks")
	}
}

// TestSpeedHistory tests the SpeedHistory method.
func TestSpeedHistory(t *testing.T) {
	track := &TrackedObject{
		TrackID:      "track-speed-history",
		speedHistory: []float32{5.0, 6.0, 7.0, 8.0, 9.0},
	}

	history := track.SpeedHistory()

	if len(history) != 5 {
		t.Errorf("Expected 5 speed values, got %d", len(history))
	}

	// Verify values are copied correctly
	for i, expected := range []float32{5.0, 6.0, 7.0, 8.0, 9.0} {
		if history[i] != expected {
			t.Errorf("Speed[%d] = %f, want %f", i, history[i], expected)
		}
	}

	// Verify it's a copy (modifying returned slice doesn't affect original)
	history[0] = 99.0
	if track.speedHistory[0] == 99.0 {
		t.Error("SpeedHistory should return a copy, not the original slice")
	}
}

// TestSpeedHistory_Nil tests SpeedHistory with nil history.
func TestSpeedHistory_Nil(t *testing.T) {
	track := &TrackedObject{
		TrackID:      "track-nil-history",
		speedHistory: nil,
	}

	history := track.SpeedHistory()

	if history != nil {
		t.Errorf("Expected nil for nil speedHistory, got %v", history)
	}
}

// TestComputeQualityMetrics tests the quality metrics computation.
func TestComputeQualityMetrics(t *testing.T) {
	track := &TrackedObject{
		TrackID:          "track-quality",
		FirstUnixNanos:   1000000000, // 1 second
		LastUnixNanos:    6000000000, // 6 seconds (5 second duration)
		ObservationCount: 50,
		History: []TrackPoint{
			{X: 0.0, Y: 0.0, Timestamp: 1000000000},
			{X: 1.0, Y: 0.0, Timestamp: 1100000000},
			{X: 2.0, Y: 0.0, Timestamp: 1200000000},
			{X: 3.0, Y: 0.0, Timestamp: 1300000000},
			{X: 4.0, Y: 0.0, Timestamp: 1400000000},
		},
	}

	track.ComputeQualityMetrics()

	// Track length should be 4 metres (4 steps of 1 metre each)
	if track.TrackLengthMeters < 3.9 || track.TrackLengthMeters > 4.1 {
		t.Errorf("TrackLengthMeters = %f, want ~4.0", track.TrackLengthMeters)
	}

	// Duration should be 5 seconds
	if track.TrackDurationSecs < 4.9 || track.TrackDurationSecs > 5.1 {
		t.Errorf("TrackDurationSecs = %f, want ~5.0", track.TrackDurationSecs)
	}
}

// TestComputeQualityMetrics_WithOcclusions tests occlusion detection.
func TestComputeQualityMetrics_WithOcclusions(t *testing.T) {
	// Create history with gaps (>200ms = occlusion)
	track := &TrackedObject{
		TrackID:          "track-occlusion",
		FirstUnixNanos:   1000000000,
		LastUnixNanos:    5000000000,
		ObservationCount: 10,
		History: []TrackPoint{
			{X: 0.0, Y: 0.0, Timestamp: 1000000000},
			{X: 1.0, Y: 0.0, Timestamp: 1100000000}, // Normal gap (100ms)
			{X: 2.0, Y: 0.0, Timestamp: 1500000000}, // 400ms gap = occlusion
			{X: 3.0, Y: 0.0, Timestamp: 1600000000}, // Normal gap
			{X: 4.0, Y: 0.0, Timestamp: 2100000000}, // 500ms gap = occlusion
		},
	}

	track.ComputeQualityMetrics()

	if track.OcclusionCount != 2 {
		t.Errorf("OcclusionCount = %d, want 2", track.OcclusionCount)
	}

	// Max occlusion should be ~5 frames (500ms / 100ms)
	if track.MaxOcclusionFrames < 4 {
		t.Errorf("MaxOcclusionFrames = %d, want >= 4", track.MaxOcclusionFrames)
	}
}

// TestComputeQualityMetrics_SinglePoint tests with single point history.
func TestComputeQualityMetrics_SinglePoint(t *testing.T) {
	track := &TrackedObject{
		TrackID:          "track-single",
		FirstUnixNanos:   1000000000,
		LastUnixNanos:    1000000000,
		ObservationCount: 1,
		History: []TrackPoint{
			{X: 0.0, Y: 0.0, Timestamp: 1000000000},
		},
	}

	track.ComputeQualityMetrics()

	if track.TrackLengthMeters != 0 {
		t.Errorf("TrackLengthMeters should be 0 for single point, got %f", track.TrackLengthMeters)
	}

	if track.OcclusionCount != 0 {
		t.Errorf("OcclusionCount should be 0 for single point, got %d", track.OcclusionCount)
	}
}

// TestComputeQualityMetrics_EmptyHistory tests with empty history.
func TestComputeQualityMetrics_EmptyHistory(t *testing.T) {
	track := &TrackedObject{
		TrackID:          "track-empty",
		FirstUnixNanos:   1000000000,
		LastUnixNanos:    2000000000,
		ObservationCount: 0,
		History:          []TrackPoint{},
	}

	// Should not panic
	track.ComputeQualityMetrics()

	if track.TrackLengthMeters != 0 {
		t.Errorf("TrackLengthMeters should be 0 for empty history, got %f", track.TrackLengthMeters)
	}
}

// TestComputeQualityMetrics_SpatialCoverage tests spatial coverage calculation.
func TestComputeQualityMetrics_SpatialCoverage(t *testing.T) {
	// 5 second duration at 10Hz = 50 theoretical observations
	track := &TrackedObject{
		TrackID:          "track-coverage",
		FirstUnixNanos:   1000000000,
		LastUnixNanos:    6000000000, // 5 seconds
		ObservationCount: 45,         // ~90% coverage
		History:          []TrackPoint{},
	}

	track.ComputeQualityMetrics()

	// Spatial coverage should be ~0.9
	if track.SpatialCoverage < 0.85 || track.SpatialCoverage > 0.95 {
		t.Errorf("SpatialCoverage = %f, want ~0.9", track.SpatialCoverage)
	}
}

// TestComputeQualityMetrics_SpatialCoverageClamped tests coverage is clamped to 1.0.
func TestComputeQualityMetrics_SpatialCoverageClamped(t *testing.T) {
	// More observations than theoretical max (can happen with frame rate variations)
	track := &TrackedObject{
		TrackID:          "track-over-coverage",
		FirstUnixNanos:   1000000000,
		LastUnixNanos:    2000000000, // 1 second = 10 theoretical at 10Hz
		ObservationCount: 15,         // 150% theoretical
		History:          []TrackPoint{},
	}

	track.ComputeQualityMetrics()

	// Should be clamped to 1.0
	if track.SpatialCoverage > 1.0 {
		t.Errorf("SpatialCoverage should be clamped to 1.0, got %f", track.SpatialCoverage)
	}
}

// TestSpeed tests the Speed method.
func TestSpeed(t *testing.T) {
	tests := []struct {
		name     string
		vx       float32
		vy       float32
		expected float32
	}{
		{"zero velocity", 0, 0, 0},
		{"x only", 3, 0, 3},
		{"y only", 0, 4, 4},
		{"3-4-5 triangle", 3, 4, 5},
		{"negative values", -3, -4, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := &TrackedObject{VX: tt.vx, VY: tt.vy}
			speed := track.Speed()

			if speed < tt.expected-0.001 || speed > tt.expected+0.001 {
				t.Errorf("Speed() = %f, want %f", speed, tt.expected)
			}
		})
	}
}

// TestHeading tests the Heading method.
func TestHeading(t *testing.T) {
	tests := []struct {
		name     string
		vx       float32
		vy       float32
		expected float32 // approximate, in radians
	}{
		{"east", 1, 0, 0},
		{"north", 0, 1, 1.5708}, // π/2
		{"west", -1, 0, 3.1416}, // π
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := &TrackedObject{VX: tt.vx, VY: tt.vy}
			heading := track.Heading()

			// Allow tolerance for floating point
			diff := heading - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("Heading() = %f, want ~%f", heading, tt.expected)
			}
		})
	}
}

// TestTracker_GetTrackCount_MultipleStates tests counting tracks in different states.
func TestTracker_GetTrackCount_MultipleStates(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 2
	config.MaxMisses = 1
	tracker := NewTracker(config)

	now := time.Now()

	// Create 3 tracks
	clusters := []WorldCluster{
		{CentroidX: 5.0, CentroidY: 10.0, SensorID: "test"},
		{CentroidX: 15.0, CentroidY: 20.0, SensorID: "test"},
		{CentroidX: 25.0, CentroidY: 30.0, SensorID: "test"},
	}

	// Frame 1: All tentative
	tracker.Update(clusters, now)
	total, tentative, confirmed, deleted := tracker.GetTrackCount()

	if total != 3 {
		t.Errorf("Total = %d, want 3", total)
	}
	if tentative != 3 {
		t.Errorf("Tentative = %d, want 3", tentative)
	}

	// Frame 2: Confirm 2 tracks (only update first 2 clusters)
	now = now.Add(100 * time.Millisecond)
	clusters[0].CentroidX += 0.1
	clusters[1].CentroidX += 0.1
	tracker.Update(clusters[:2], now)

	total, tentative, confirmed, deleted = tracker.GetTrackCount()

	if confirmed != 2 {
		t.Errorf("Confirmed = %d, want 2", confirmed)
	}

	// Third track should have missed once
	// Frame 3: Delete third track
	now = now.Add(100 * time.Millisecond)
	tracker.Update(clusters[:2], now)

	total, tentative, confirmed, deleted = tracker.GetTrackCount()

	if deleted < 1 {
		t.Errorf("Deleted = %d, want >= 1", deleted)
	}
}

// TestTrackedObject_DiagonalMovement tests speed with diagonal movement.
func TestTrackedObject_DiagonalMovement(t *testing.T) {
	track := &TrackedObject{
		TrackID:          "track-diagonal",
		FirstUnixNanos:   1000000000,
		LastUnixNanos:    2000000000,
		ObservationCount: 5,
		History: []TrackPoint{
			{X: 0.0, Y: 0.0, Timestamp: 1000000000},
			{X: 1.0, Y: 1.0, Timestamp: 1100000000},
			{X: 2.0, Y: 2.0, Timestamp: 1200000000},
		},
	}

	track.ComputeQualityMetrics()

	// Each diagonal step is sqrt(2) ≈ 1.414 metres
	// 2 steps = ~2.83 metres
	expectedLength := float32(2.828)
	if track.TrackLengthMeters < expectedLength-0.1 || track.TrackLengthMeters > expectedLength+0.1 {
		t.Errorf("TrackLengthMeters = %f, want ~%f", track.TrackLengthMeters, expectedLength)
	}
}
