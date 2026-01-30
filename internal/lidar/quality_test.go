package lidar

import (
	"encoding/json"
	"testing"
)

func TestComputeRunStatistics_Empty(t *testing.T) {
	stats := ComputeRunStatistics([]*TrackedObject{})
	
	if stats == nil {
		t.Fatal("ComputeRunStatistics returned nil for empty input")
	}
	
	// Verify zero values for empty input
	if stats.AvgTrackLength != 0 {
		t.Errorf("AvgTrackLength = %f, want 0", stats.AvgTrackLength)
	}
}

func TestComputeRunStatistics_SingleTrack(t *testing.T) {
	tracks := []*TrackedObject{
		{
			TrackLengthMeters: 10.0,
			TrackDurationSecs: 5.0,
			OcclusionCount:    2,
			NoisePointRatio:   0.1,
			SpatialCoverage:   0.8,
			ObservationCount:  50,
			ObjectClass:       "vehicle",
			ObjectConfidence:  0.9,
			State:             TrackConfirmed,
		},
	}
	
	stats := ComputeRunStatistics(tracks)
	
	if stats.AvgTrackLength != 10.0 {
		t.Errorf("AvgTrackLength = %f, want 10.0", stats.AvgTrackLength)
	}
	if stats.MedianTrackLength != 10.0 {
		t.Errorf("MedianTrackLength = %f, want 10.0", stats.MedianTrackLength)
	}
	if stats.AvgTrackDuration != 5.0 {
		t.Errorf("AvgTrackDuration = %f, want 5.0", stats.AvgTrackDuration)
	}
	if stats.AvgOcclusionCount != 2.0 {
		t.Errorf("AvgOcclusionCount = %f, want 2.0", stats.AvgOcclusionCount)
	}
	if stats.AvgNoiseRatio != 0.1 {
		t.Errorf("AvgNoiseRatio = %f, want 0.1", stats.AvgNoiseRatio)
	}
	if stats.AvgSpatialCoverage != 0.8 {
		t.Errorf("AvgSpatialCoverage = %f, want 0.8", stats.AvgSpatialCoverage)
	}
	if stats.AvgObservationsPerTrack != 50 {
		t.Errorf("AvgObservationsPerTrack = %d, want 50", stats.AvgObservationsPerTrack)
	}
	if stats.ClassCounts["vehicle"] != 1 {
		t.Errorf("ClassCounts[vehicle] = %d, want 1", stats.ClassCounts["vehicle"])
	}
	if stats.ConfirmedRatio != 1.0 {
		t.Errorf("ConfirmedRatio = %f, want 1.0", stats.ConfirmedRatio)
	}
}

func TestComputeRunStatistics_MultipleTracks(t *testing.T) {
	tracks := []*TrackedObject{
		{
			TrackLengthMeters: 10.0,
			TrackDurationSecs: 2.0,
			OcclusionCount:    1,
			NoisePointRatio:   0.1,
			SpatialCoverage:   0.7,
			ObservationCount:  20,
			ObjectClass:       "vehicle",
			ObjectConfidence:  0.9,
			State:             TrackConfirmed,
		},
		{
			TrackLengthMeters: 20.0,
			TrackDurationSecs: 4.0,
			OcclusionCount:    3,
			NoisePointRatio:   0.2,
			SpatialCoverage:   0.9,
			ObservationCount:  40,
			ObjectClass:       "pedestrian",
			ObjectConfidence:  0.8,
			State:             TrackTentative,
		},
		{
			TrackLengthMeters: 15.0,
			TrackDurationSecs: 3.0,
			OcclusionCount:    2,
			NoisePointRatio:   0.15,
			SpatialCoverage:   0.8,
			ObservationCount:  30,
			ObjectClass:       "", // Should be classified as "other"
			ObjectConfidence:  0.5,
			State:             TrackConfirmed,
		},
	}
	
	stats := ComputeRunStatistics(tracks)
	
	// Check averages
	expectedAvgLength := float32((10.0 + 20.0 + 15.0) / 3.0)
	if stats.AvgTrackLength != expectedAvgLength {
		t.Errorf("AvgTrackLength = %f, want %f", stats.AvgTrackLength, expectedAvgLength)
	}
	
	// Median should be 15.0 (middle value when sorted: 10, 15, 20)
	if stats.MedianTrackLength != 15.0 {
		t.Errorf("MedianTrackLength = %f, want 15.0", stats.MedianTrackLength)
	}
	
	// Check class counts
	if stats.ClassCounts["vehicle"] != 1 {
		t.Errorf("ClassCounts[vehicle] = %d, want 1", stats.ClassCounts["vehicle"])
	}
	if stats.ClassCounts["pedestrian"] != 1 {
		t.Errorf("ClassCounts[pedestrian] = %d, want 1", stats.ClassCounts["pedestrian"])
	}
	if stats.ClassCounts["other"] != 1 {
		t.Errorf("ClassCounts[other] = %d, want 1", stats.ClassCounts["other"])
	}
	
	// Check lifecycle ratios
	if stats.TentativeRatio != 1.0/3.0 {
		t.Errorf("TentativeRatio = %f, want %f", stats.TentativeRatio, 1.0/3.0)
	}
	if stats.ConfirmedRatio != 2.0/3.0 {
		t.Errorf("ConfirmedRatio = %f, want %f", stats.ConfirmedRatio, 2.0/3.0)
	}
	
	// Check unknown ratio
	if stats.UnknownRatio != 1.0/3.0 {
		t.Errorf("UnknownRatio = %f, want %f", stats.UnknownRatio, 1.0/3.0)
	}
}

func TestRunStatistics_JSON(t *testing.T) {
	stats := &RunStatistics{
		AvgTrackLength:    10.5,
		MedianTrackLength: 10.0,
		ClassCounts:       map[string]int{"vehicle": 5, "pedestrian": 3},
	}
	
	// Test ToJSON
	jsonStr, err := stats.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	
	if jsonStr == "" {
		t.Error("ToJSON returned empty string")
	}
	
	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Errorf("ToJSON produced invalid JSON: %v", err)
	}
	
	// Test ParseRunStatistics
	parsed2, err := ParseRunStatistics(jsonStr)
	if err != nil {
		t.Fatalf("ParseRunStatistics failed: %v", err)
	}
	
	if parsed2.AvgTrackLength != stats.AvgTrackLength {
		t.Errorf("Parsed AvgTrackLength = %f, want %f", parsed2.AvgTrackLength, stats.AvgTrackLength)
	}
	if parsed2.ClassCounts["vehicle"] != 5 {
		t.Errorf("Parsed ClassCounts[vehicle] = %d, want 5", parsed2.ClassCounts["vehicle"])
	}
}

func TestParseRunStatistics_Invalid(t *testing.T) {
	_, err := ParseRunStatistics("invalid json")
	if err == nil {
		t.Error("ParseRunStatistics should fail on invalid JSON")
	}
}

func TestComputeTrackQualityMetrics(t *testing.T) {
	track := &TrackedObject{
		TrackID:            "test-track-001",
		TrackLengthMeters:  25.0,
		TrackDurationSecs:  5.0,
		OcclusionCount:     2,
		MaxOcclusionFrames: 5,
		SpatialCoverage:    0.8,
		NoisePointRatio:    0.1,
	}
	
	metrics := ComputeTrackQualityMetrics(track)
	
	if metrics.TrackID != track.TrackID {
		t.Errorf("TrackID = %s, want %s", metrics.TrackID, track.TrackID)
	}
	if metrics.TrackLengthMeters != 25.0 {
		t.Errorf("TrackLengthMeters = %f, want 25.0", metrics.TrackLengthMeters)
	}
	if metrics.SpatialCoverage != 0.8 {
		t.Errorf("SpatialCoverage = %f, want 0.8", metrics.SpatialCoverage)
	}
	
	// Verify quality score is computed and in range [0, 1]
	if metrics.QualityScore < 0 || metrics.QualityScore > 1 {
		t.Errorf("QualityScore = %f, want value in [0, 1]", metrics.QualityScore)
	}
	
	// High quality track should have high score
	if metrics.QualityScore < 0.5 {
		t.Errorf("Expected high quality score for good track, got %f", metrics.QualityScore)
	}
}

func TestComputeTrackQualityMetrics_PoorQuality(t *testing.T) {
	track := &TrackedObject{
		TrackID:            "poor-track",
		TrackLengthMeters:  2.0,  // Short track
		TrackDurationSecs:  0.5,
		OcclusionCount:     20,   // Many occlusions
		MaxOcclusionFrames: 10,
		SpatialCoverage:    0.2,  // Low coverage
		NoisePointRatio:    0.9,  // High noise
	}
	
	metrics := ComputeTrackQualityMetrics(track)
	
	// Poor quality track should have low score
	if metrics.QualityScore > 0.3 {
		t.Errorf("Expected low quality score for poor track, got %f", metrics.QualityScore)
	}
}

func TestComputeNoiseCoverageMetrics(t *testing.T) {
	tracks := []*TrackedObject{
		{
			NoisePointRatio:  0.4,  // High noise
			ObjectClass:      "other",
			ObjectConfidence: 0.5,  // Low confidence
		},
		{
			NoisePointRatio:  0.2,  // Low noise
			ObjectClass:      "vehicle",
			ObjectConfidence: 0.9,
		},
		{
			NoisePointRatio:  0.15, // Low noise
			ObjectClass:      "",   // Unknown
			ObjectConfidence: 0.4,  // Low confidence
		},
	}
	
	metrics := ComputeNoiseCoverageMetrics(tracks)
	
	if metrics.TotalTracks != 3 {
		t.Errorf("TotalTracks = %d, want 3", metrics.TotalTracks)
	}
	if metrics.TracksWithHighNoise != 1 {
		t.Errorf("TracksWithHighNoise = %d, want 1", metrics.TracksWithHighNoise)
	}
	if metrics.TracksUnknownClass != 2 {
		t.Errorf("TracksUnknownClass = %d, want 2", metrics.TracksUnknownClass)
	}
	if metrics.TracksLowConfidence != 2 {
		t.Errorf("TracksLowConfidence = %d, want 2", metrics.TracksLowConfidence)
	}
	
	// Check histogram bins
	if len(metrics.NoiseRatioHistogram) != 10 {
		t.Errorf("NoiseRatioHistogram length = %d, want 10", len(metrics.NoiseRatioHistogram))
	}
}

func TestDefaultTrackTrainingFilter(t *testing.T) {
	filter := DefaultTrackTrainingFilter()
	
	if filter == nil {
		t.Fatal("DefaultTrackTrainingFilter returned nil")
	}
	
	if filter.MinQualityScore <= 0 {
		t.Error("MinQualityScore should be positive")
	}
	if filter.MinDuration <= 0 {
		t.Error("MinDuration should be positive")
	}
	if filter.MinLength <= 0 {
		t.Error("MinLength should be positive")
	}
	if len(filter.AllowedStates) == 0 {
		t.Error("AllowedStates should not be empty")
	}
}

func TestFilterTracksForTraining(t *testing.T) {
	tracks := []*TrackedObject{
		{
			// High quality track
			TrackID:            "good-track",
			TrackLengthMeters:  50.0,
			TrackDurationSecs:  5.0,
			OcclusionCount:     1,
			NoisePointRatio:    0.05,
			SpatialCoverage:    0.9,
			ObservationCount:   50,
			ObjectClass:        "vehicle",
			ObjectConfidence:   0.95,
			State:              TrackConfirmed,
		},
		{
			// Poor quality track (too short)
			TrackID:            "short-track",
			TrackLengthMeters:  2.0,
			TrackDurationSecs:  0.5,
			OcclusionCount:     0,
			NoisePointRatio:    0.1,
			SpatialCoverage:    0.8,
			ObservationCount:   5,
			ObjectClass:        "vehicle",
			ObjectConfidence:   0.9,
			State:              TrackConfirmed,
		},
		{
			// Tentative track (wrong state)
			TrackID:            "tentative-track",
			TrackLengthMeters:  10.0,
			TrackDurationSecs:  2.0,
			OcclusionCount:     1,
			NoisePointRatio:    0.1,
			SpatialCoverage:    0.8,
			ObservationCount:   20,
			ObjectClass:        "vehicle",
			ObjectConfidence:   0.8,
			State:              TrackTentative,
		},
	}
	
	filter := DefaultTrackTrainingFilter()
	filtered := FilterTracksForTraining(tracks, filter)
	
	// Only the first track should pass
	if len(filtered) != 1 {
		t.Errorf("Filtered tracks count = %d, want 1", len(filtered))
	}
	
	if len(filtered) > 0 && filtered[0].TrackID != "good-track" {
		t.Errorf("Filtered track ID = %s, want 'good-track'", filtered[0].TrackID)
	}
}

func TestFilterTracksForTraining_RequireClass(t *testing.T) {
	tracks := []*TrackedObject{
		{
			TrackLengthMeters: 10.0,
			TrackDurationSecs: 3.0,
			OcclusionCount:    1,
			NoisePointRatio:   0.1,
			SpatialCoverage:   0.8,
			ObservationCount:  30,
			ObjectClass:       "", // No class
			ObjectConfidence:  0.9,
			State:             TrackConfirmed,
		},
		{
			TrackLengthMeters: 10.0,
			TrackDurationSecs: 3.0,
			OcclusionCount:    1,
			NoisePointRatio:   0.1,
			SpatialCoverage:   0.8,
			ObservationCount:  30,
			ObjectClass:       "vehicle", // Has class
			ObjectConfidence:  0.9,
			State:             TrackConfirmed,
		},
	}
	
	filter := DefaultTrackTrainingFilter()
	filter.RequireClass = true
	
	filtered := FilterTracksForTraining(tracks, filter)
	
	// Only the second track should pass (has class)
	if len(filtered) != 1 {
		t.Errorf("Filtered tracks count = %d, want 1", len(filtered))
	}
	
	if len(filtered) > 0 && filtered[0].ObjectClass != "vehicle" {
		t.Errorf("Filtered track class = %s, want 'vehicle'", filtered[0].ObjectClass)
	}
}

func TestSummarizeTrainingDataset_Empty(t *testing.T) {
	summary := SummarizeTrainingDataset([]*TrackedObject{})
	
	if summary == nil {
		t.Fatal("SummarizeTrainingDataset returned nil")
	}
	
	if summary.TotalTracks != 0 {
		t.Errorf("TotalTracks = %d, want 0", summary.TotalTracks)
	}
	
	if summary.ClassDistribution == nil {
		t.Error("ClassDistribution should not be nil")
	}
}

func TestSummarizeTrainingDataset(t *testing.T) {
	tracks := []*TrackedObject{
		{
			TrackLengthMeters: 10.0,
			TrackDurationSecs: 2.0,
			ObservationCount:  20,
			ObjectClass:       "vehicle",
			NoisePointRatio:   0.1,
			SpatialCoverage:   0.8,
			OcclusionCount:    1,
			MaxOcclusionFrames: 3,
		},
		{
			TrackLengthMeters: 15.0,
			TrackDurationSecs: 3.0,
			ObservationCount:  30,
			ObjectClass:       "vehicle",
			NoisePointRatio:   0.05,
			SpatialCoverage:   0.9,
			OcclusionCount:    0,
			MaxOcclusionFrames: 0,
		},
		{
			TrackLengthMeters: 8.0,
			TrackDurationSecs: 1.5,
			ObservationCount:  15,
			ObjectClass:       "", // Unlabeled
			NoisePointRatio:   0.15,
			SpatialCoverage:   0.7,
			OcclusionCount:    2,
			MaxOcclusionFrames: 5,
		},
	}
	
	summary := SummarizeTrainingDataset(tracks)
	
	if summary.TotalTracks != 3 {
		t.Errorf("TotalTracks = %d, want 3", summary.TotalTracks)
	}
	
	if summary.TotalFrames != 65 {
		t.Errorf("TotalFrames = %d, want 65", summary.TotalFrames)
	}
	
	if summary.ClassDistribution["vehicle"] != 2 {
		t.Errorf("ClassDistribution[vehicle] = %d, want 2", summary.ClassDistribution["vehicle"])
	}
	
	if summary.ClassDistribution["unlabeled"] != 1 {
		t.Errorf("ClassDistribution[unlabeled] = %d, want 1", summary.ClassDistribution["unlabeled"])
	}
	
	expectedAvgLength := float32((10.0 + 15.0 + 8.0) / 3.0)
	if summary.AvgLength != expectedAvgLength {
		t.Errorf("AvgLength = %f, want %f", summary.AvgLength, expectedAvgLength)
	}
	
	// Check quality score is computed
	if summary.AvgQualityScore <= 0 {
		t.Error("AvgQualityScore should be positive")
	}
}
