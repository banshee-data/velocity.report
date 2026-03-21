package l8analytics

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestComputeTrackSummary_Empty(t *testing.T) {
	result := ComputeTrackSummary(nil)
	if result.Overall.TotalTracks != 0 {
		t.Errorf("TotalTracks = %d, want 0", result.Overall.TotalTracks)
	}
}

func TestComputeTrackSummary_MultipleTracks(t *testing.T) {
	tracks := []*l5tracks.TrackedObject{
		{
			ObjectClass:    "car",
			AvgSpeedMps:    10.0,
			MaxSpeedMps:    15.0,
			State:          l5tracks.TrackConfirmed,
			FirstUnixNanos: 1000000000,
			LastUnixNanos:  6000000000, // 5 seconds
		},
		{
			ObjectClass:    "car",
			AvgSpeedMps:    20.0,
			MaxSpeedMps:    25.0,
			State:          l5tracks.TrackConfirmed,
			FirstUnixNanos: 2000000000,
			LastUnixNanos:  5000000000, // 3 seconds
		},
		{
			ObjectClass:    "pedestrian",
			AvgSpeedMps:    1.5,
			MaxSpeedMps:    2.0,
			State:          l5tracks.TrackTentative,
			FirstUnixNanos: 1000000000,
			LastUnixNanos:  4000000000, // 3 seconds
		},
	}

	result := ComputeTrackSummary(tracks)

	if result.Overall.TotalTracks != 3 {
		t.Errorf("TotalTracks = %d, want 3", result.Overall.TotalTracks)
	}
	if result.Overall.ConfirmedCount != 2 {
		t.Errorf("ConfirmedCount = %d, want 2", result.Overall.ConfirmedCount)
	}
	if result.Overall.TentativeCount != 1 {
		t.Errorf("TentativeCount = %d, want 1", result.Overall.TentativeCount)
	}

	carSummary, ok := result.ByClass["car"]
	if !ok {
		t.Fatal("missing class 'car' in summary")
	}
	if carSummary.Count != 2 {
		t.Errorf("car count = %d, want 2", carSummary.Count)
	}
	if carSummary.MaxSpeedMps != 25.0 {
		t.Errorf("car max speed = %f, want 25.0", carSummary.MaxSpeedMps)
	}
	wantAvg := float32(15.0) // (10+20)/2
	if carSummary.AvgSpeedMps != wantAvg {
		t.Errorf("car avg speed = %f, want %f", carSummary.AvgSpeedMps, wantAvg)
	}

	pedSummary, ok := result.ByClass["pedestrian"]
	if !ok {
		t.Fatal("missing class 'pedestrian' in summary")
	}
	if pedSummary.Count != 1 {
		t.Errorf("pedestrian count = %d, want 1", pedSummary.Count)
	}
}

func TestComputeTrackSummary_UnclassifiedTracks(t *testing.T) {
	tracks := []*l5tracks.TrackedObject{
		{ObjectClass: "", AvgSpeedMps: 5.0, State: l5tracks.TrackConfirmed},
	}

	result := ComputeTrackSummary(tracks)

	if _, ok := result.ByClass["unclassified"]; !ok {
		t.Error("expected 'unclassified' class for empty ObjectClass")
	}
}

func TestComputeLabellingProgress(t *testing.T) {
	tests := []struct {
		total, labelled int
		want            float64
	}{
		{0, 0, 0.0},
		{100, 50, 50.0},
		{100, 100, 100.0},
		{10, 3, 30.0},
	}

	for _, tt := range tests {
		got := ComputeLabellingProgress(tt.total, tt.labelled)
		if got != tt.want {
			t.Errorf("ComputeLabellingProgress(%d, %d) = %f, want %f",
				tt.total, tt.labelled, got, tt.want)
		}
	}
}
