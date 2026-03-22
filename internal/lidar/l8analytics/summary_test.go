package l8analytics

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestComputeRunStatistics_Empty(t *testing.T) {
	stats := ComputeRunStatistics([]*l5tracks.TrackedObject{})

	if stats == nil {
		t.Fatal("ComputeRunStatistics returned nil for empty input")
	}

	if stats.AvgTrackLength != 0 {
		t.Errorf("AvgTrackLength = %f, want 0", stats.AvgTrackLength)
	}
}

func TestComputeRunStatistics_SingleTrack(t *testing.T) {
	tracks := []*l5tracks.TrackedObject{
		{
			TrackLengthMeters: 10.0,
			TrackDurationSecs: 5.0,
			OcclusionCount:    2,
			NoisePointRatio:   0.1,
			SpatialCoverage:   0.8,
			ObservationCount:  50,
			ObjectClass:       "vehicle",
			ObjectConfidence:  0.9,
			TrackState:        l5tracks.TrackConfirmed,
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
	if stats.ClassCounts["vehicle"] != 1 {
		t.Errorf("ClassCounts[vehicle] = %d, want 1", stats.ClassCounts["vehicle"])
	}
	if stats.ConfirmedRatio != 1.0 {
		t.Errorf("ConfirmedRatio = %f, want 1.0", stats.ConfirmedRatio)
	}
}

func TestComputeRunStatistics_MultipleTracks(t *testing.T) {
	tracks := []*l5tracks.TrackedObject{
		{
			TrackLengthMeters: 10.0,
			TrackDurationSecs: 2.0,
			OcclusionCount:    1,
			NoisePointRatio:   0.1,
			SpatialCoverage:   0.7,
			ObservationCount:  20,
			ObjectClass:       "vehicle",
			ObjectConfidence:  0.9,
			TrackState:        l5tracks.TrackConfirmed,
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
			TrackState:        l5tracks.TrackTentative,
		},
		{
			TrackLengthMeters: 15.0,
			TrackDurationSecs: 3.0,
			OcclusionCount:    2,
			NoisePointRatio:   0.15,
			SpatialCoverage:   0.8,
			ObservationCount:  30,
			ObjectClass:       "",
			ObjectConfidence:  0.5,
			TrackState:        l5tracks.TrackConfirmed,
		},
	}

	stats := ComputeRunStatistics(tracks)

	expectedAvgLength := float32((10.0 + 20.0 + 15.0) / 3.0)
	if stats.AvgTrackLength != expectedAvgLength {
		t.Errorf("AvgTrackLength = %f, want %f", stats.AvgTrackLength, expectedAvgLength)
	}

	if stats.MedianTrackLength != 15.0 {
		t.Errorf("MedianTrackLength = %f, want 15.0", stats.MedianTrackLength)
	}

	if stats.ClassCounts["vehicle"] != 1 {
		t.Errorf("ClassCounts[vehicle] = %d, want 1", stats.ClassCounts["vehicle"])
	}
	if stats.ClassCounts["pedestrian"] != 1 {
		t.Errorf("ClassCounts[pedestrian] = %d, want 1", stats.ClassCounts["pedestrian"])
	}
	if stats.ClassCounts["dynamic"] != 1 {
		t.Errorf("ClassCounts[dynamic] = %d, want 1", stats.ClassCounts["dynamic"])
	}

	if stats.TentativeRatio != 1.0/3.0 {
		t.Errorf("TentativeRatio = %f, want %f", stats.TentativeRatio, 1.0/3.0)
	}
	if stats.ConfirmedRatio != 2.0/3.0 {
		t.Errorf("ConfirmedRatio = %f, want %f", stats.ConfirmedRatio, 2.0/3.0)
	}
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

	jsonStr, err := stats.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	if jsonStr == "" {
		t.Error("ToJSON returned empty string")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Errorf("ToJSON produced invalid JSON: %v", err)
	}

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

func TestRunStatistics_ToJSON_Error(t *testing.T) {
	stats := &RunStatistics{
		AvgTrackLength: float32(math.NaN()),
	}

	_, err := stats.ToJSON()
	if err == nil {
		t.Error("ToJSON should fail with NaN value")
	}
}
