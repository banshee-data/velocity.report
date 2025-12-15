package lidar

import (
	"testing"
)

// TestComputeRunStatistics tests aggregate statistics calculation
func TestComputeRunStatistics(t *testing.T) {
	t.Run("empty tracks", func(t *testing.T) {
		stats := ComputeRunStatistics([]*TrackedObject{})
		if stats == nil {
			t.Fatal("expected non-nil stats")
		}
		if stats.AvgTrackLength != 0 {
			t.Errorf("expected AvgTrackLength=0, got %f", stats.AvgTrackLength)
		}
	})

	t.Run("single track", func(t *testing.T) {
		track := &TrackedObject{
			TrackID:           "track-1",
			TrackLengthMeters: 50.0,
			TrackDurationSecs: 5.0,
			OcclusionCount:    2,
			NoisePointRatio:   0.1,
			SpatialCoverage:   0.8,
			ObservationCount:  50,
			ObjectClass:       "vehicle",
			ObjectConfidence:  0.9,
			State:             TrackConfirmed,
		}

		stats := ComputeRunStatistics([]*TrackedObject{track})
		if stats.AvgTrackLength != 50.0 {
			t.Errorf("expected AvgTrackLength=50.0, got %f", stats.AvgTrackLength)
		}
		if stats.MedianTrackLength != 50.0 {
			t.Errorf("expected MedianTrackLength=50.0, got %f", stats.MedianTrackLength)
		}
		if stats.AvgOcclusionCount != 2.0 {
			t.Errorf("expected AvgOcclusionCount=2.0, got %f", stats.AvgOcclusionCount)
		}
		if stats.ClassCounts["vehicle"] != 1 {
			t.Errorf("expected vehicle count=1, got %d", stats.ClassCounts["vehicle"])
		}
		if stats.ConfirmedRatio != 1.0 {
			t.Errorf("expected ConfirmedRatio=1.0, got %f", stats.ConfirmedRatio)
		}
	})

	t.Run("multiple tracks with varied metrics", func(t *testing.T) {
		tracks := []*TrackedObject{
			{
				TrackID:           "track-1",
				TrackLengthMeters: 30.0,
				TrackDurationSecs: 3.0,
				OcclusionCount:    1,
				NoisePointRatio:   0.05,
				SpatialCoverage:   0.9,
				ObservationCount:  30,
				ObjectClass:       "vehicle",
				ObjectConfidence:  0.95,
				State:             TrackConfirmed,
			},
			{
				TrackID:           "track-2",
				TrackLengthMeters: 50.0,
				TrackDurationSecs: 5.0,
				OcclusionCount:    3,
				NoisePointRatio:   0.15,
				SpatialCoverage:   0.7,
				ObservationCount:  50,
				ObjectClass:       "vehicle",
				ObjectConfidence:  0.85,
				State:             TrackConfirmed,
			},
			{
				TrackID:           "track-3",
				TrackLengthMeters: 10.0,
				TrackDurationSecs: 1.0,
				OcclusionCount:    0,
				NoisePointRatio:   0.2,
				SpatialCoverage:   0.6,
				ObservationCount:  10,
				ObjectClass:       "pedestrian",
				ObjectConfidence:  0.8,
				State:             TrackTentative,
			},
		}

		stats := ComputeRunStatistics(tracks)
		
		// Average track length: (30 + 50 + 10) / 3 = 30
		expectedAvg := float32(30.0)
		if stats.AvgTrackLength != expectedAvg {
			t.Errorf("expected AvgTrackLength=%.1f, got %.1f", expectedAvg, stats.AvgTrackLength)
		}
		
		// Median should be middle value: 30
		if stats.MedianTrackLength != 30.0 {
			t.Errorf("expected MedianTrackLength=30.0, got %.1f", stats.MedianTrackLength)
		}
		
		// Average occlusions: (1 + 3 + 0) / 3 = 1.33
		if stats.AvgOcclusionCount < 1.3 || stats.AvgOcclusionCount > 1.4 {
			t.Errorf("expected AvgOcclusionCount~=1.33, got %.2f", stats.AvgOcclusionCount)
		}
		
		// Class distribution
		if stats.ClassCounts["vehicle"] != 2 {
			t.Errorf("expected vehicle count=2, got %d", stats.ClassCounts["vehicle"])
		}
		if stats.ClassCounts["pedestrian"] != 1 {
			t.Errorf("expected pedestrian count=1, got %d", stats.ClassCounts["pedestrian"])
		}
		
		// Lifecycle ratios
		if stats.TentativeRatio != float32(1.0/3.0) {
			t.Errorf("expected TentativeRatio=0.33, got %.2f", stats.TentativeRatio)
		}
		if stats.ConfirmedRatio != float32(2.0/3.0) {
			t.Errorf("expected ConfirmedRatio=0.67, got %.2f", stats.ConfirmedRatio)
		}
	})
}

// TestFilterTracksForTraining tests track filtering for ML training
func TestFilterTracksForTraining(t *testing.T) {
	t.Run("filter by quality threshold", func(t *testing.T) {
		tracks := []*TrackedObject{
			{TrackID: "track-1", State: TrackConfirmed, TrackLengthMeters: 60, TrackDurationSecs: 6.0, OcclusionCount: 1, MaxOcclusionFrames: 5, SpatialCoverage: 0.9, ObservationCount: 60, NoisePointRatio: 0.05},
			{TrackID: "track-2", State: TrackConfirmed, TrackLengthMeters: 2, TrackDurationSecs: 0.5, OcclusionCount: 10, MaxOcclusionFrames: 50, SpatialCoverage: 0.2, ObservationCount: 5, NoisePointRatio: 0.4}, // Low quality
			{TrackID: "track-3", State: TrackTentative, TrackLengthMeters: 50, TrackDurationSecs: 5.0, OcclusionCount: 0, MaxOcclusionFrames: 0, SpatialCoverage: 0.95, ObservationCount: 50, NoisePointRatio: 0.05}, // Tentative
		}

		filter := DefaultTrackTrainingFilter()
		filtered := FilterTracksForTraining(tracks, filter)
		
		// Should keep only track-1 (high quality, confirmed, meets all thresholds)
		if len(filtered) != 1 {
			t.Errorf("expected 1 filtered track, got %d", len(filtered))
		}
		if len(filtered) > 0 && filtered[0].TrackID != "track-1" {
			t.Errorf("expected track-1, got %s", filtered[0].TrackID)
		}
	})

	t.Run("custom filter thresholds", func(t *testing.T) {
		tracks := []*TrackedObject{
			{TrackID: "track-1", State: TrackConfirmed, TrackLengthMeters: 8, TrackDurationSecs: 1.5, OcclusionCount: 1, MaxOcclusionFrames: 3, SpatialCoverage: 0.7, ObservationCount: 15},
			{TrackID: "track-2", State: TrackConfirmed, TrackLengthMeters: 3, TrackDurationSecs: 0.8, OcclusionCount: 0, MaxOcclusionFrames: 0, SpatialCoverage: 0.65, ObservationCount: 8},
		}

		// Relaxed filter
		filter := &TrackTrainingFilter{
			MinQualityScore:   0.3,
			MinDuration:       0.5,
			MinLength:         2.0,
			MaxOcclusionRatio: 0.5,
			MinObservations:   5,
			RequireClass:      false,
			AllowedStates:     []TrackState{TrackConfirmed},
		}
		
		filtered := FilterTracksForTraining(tracks, filter)
		
		// Both should pass with relaxed thresholds
		if len(filtered) != 2 {
			t.Errorf("expected 2 filtered tracks, got %d", len(filtered))
		}
	})
}

// TestSummarizeTrainingDataset tests dataset summary generation
func TestSummarizeTrainingDataset(t *testing.T) {
	tracks := []*TrackedObject{
		{TrackID: "track-1", ObjectClass: "vehicle", TrackLengthMeters: 50, TrackDurationSecs: 5.0, SpatialCoverage: 0.9},
		{TrackID: "track-2", ObjectClass: "vehicle", TrackLengthMeters: 40, TrackDurationSecs: 4.0, SpatialCoverage: 0.85},
		{TrackID: "track-3", ObjectClass: "pedestrian", TrackLengthMeters: 15, TrackDurationSecs: 3.0, SpatialCoverage: 0.8},
	}

	summary := SummarizeTrainingDataset(tracks)
	
	if summary.TotalTracks != 3 {
		t.Errorf("expected TotalTracks=3, got %d", summary.TotalTracks)
	}
	if summary.ClassDistribution["vehicle"] != 2 {
		t.Errorf("expected vehicle count=2, got %d", summary.ClassDistribution["vehicle"])
	}
	if summary.ClassDistribution["pedestrian"] != 1 {
		t.Errorf("expected pedestrian count=1, got %d", summary.ClassDistribution["pedestrian"])
	}
	
	// Average quality should be calculated
	if summary.AvgQualityScore < 0 || summary.AvgQualityScore > 1 {
		t.Errorf("expected AvgQualityScore in [0,1], got %.2f", summary.AvgQualityScore)
	}
}

// TestComputeTrackQualityMetrics tests the quality scoring function
func TestComputeTrackQualityMetrics(t *testing.T) {
	t.Run("high quality track", func(t *testing.T) {
		track := &TrackedObject{
			TrackLengthMeters:  80.0,
			TrackDurationSecs:  8.0,
			OcclusionCount:     1,
			MaxOcclusionFrames: 3,
			SpatialCoverage:    0.95,
		}
		
		metrics := ComputeTrackQualityMetrics(track)
		
		if metrics.QualityScore < 0.7 {
			t.Errorf("expected high quality score (>0.7), got %.2f", metrics.QualityScore)
		}
		// Quality score should be in valid range
		if metrics.QualityScore < 0 || metrics.QualityScore > 1 {
			t.Errorf("expected QualityScore in [0,1], got %.2f", metrics.QualityScore)
		}
	})

	t.Run("low quality track", func(t *testing.T) {
		track := &TrackedObject{
			TrackLengthMeters:  2.0,
			TrackDurationSecs:  0.3,
			OcclusionCount:     15,
			MaxOcclusionFrames: 50,
			SpatialCoverage:    0.2,
		}
		
		metrics := ComputeTrackQualityMetrics(track)
		
		if metrics.QualityScore > 0.4 {
			t.Errorf("expected low quality score (<0.4), got %.2f", metrics.QualityScore)
		}
	})
}

// TestNoiseCoverageMetrics tests noise coverage analysis
func TestNoiseCoverageMetrics(t *testing.T) {
	tracks := []*TrackedObject{
		{TrackID: "track-1", NoisePointRatio: 0.35, ObjectClass: "vehicle", ObjectConfidence: 0.95},
		{TrackID: "track-2", NoisePointRatio: 0.15, ObjectClass: "other", ObjectConfidence: 0.3},
		{TrackID: "track-3", NoisePointRatio: 0.05, ObjectClass: "pedestrian", ObjectConfidence: 0.85},
	}

	metrics := ComputeNoiseCoverageMetrics(tracks)
	
	if metrics.TracksWithHighNoise != 1 {
		t.Errorf("expected 1 high-noise track, got %d", metrics.TracksWithHighNoise)
	}
	if metrics.TracksUnknownClass != 1 {
		t.Errorf("expected 1 unknown-class track, got %d", metrics.TracksUnknownClass)
	}
	if metrics.TracksLowConfidence != 1 {
		t.Errorf("expected 1 low-confidence track, got %d", metrics.TracksLowConfidence)
	}
}
