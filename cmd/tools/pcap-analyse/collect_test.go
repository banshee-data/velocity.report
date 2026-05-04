//go:build pcap
// +build pcap

package main

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
)

// makeFrameBuilder returns a minimal analysisFrameBuilder suitable for unit-testing
// collectTrackResults. Only tracker and classifier are populated; no background
// model, database, or PCAP I/O is involved.
func makeFrameBuilder(tracks map[string]*l5tracks.TrackedObject) *analysisFrameBuilder {
	tracker := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	tracker.Tracks = tracks
	return &analysisFrameBuilder{
		tracker:    tracker,
		classifier: l6objects.NewTrackClassifier(),
	}
}

// newResult returns a zero-valued AnalysisResult ready for collectTrackResults.
func newResult() *AnalysisResult {
	return &AnalysisResult{TracksByClass: make(map[string]int)}
}

func TestCollectTrackResults_ConfirmedCount(t *testing.T) {
	tracks := map[string]*l5tracks.TrackedObject{
		"t1": {
			TrackID: "t1",
			TrackMeasurement: l5tracks.TrackMeasurement{
				TrackState:     l5tracks.TrackConfirmed,
				ObjectClass:    "vehicle",
				StartUnixNanos: 1_000_000_000,
				EndUnixNanos:   2_000_000_000,
			},
		},
		"t2": {
			TrackID: "t2",
			TrackMeasurement: l5tracks.TrackMeasurement{
				TrackState:     l5tracks.TrackTentative,
				ObjectClass:    "vehicle",
				StartUnixNanos: 1_000_000_000,
				EndUnixNanos:   2_000_000_000,
			},
		},
	}

	fb := makeFrameBuilder(tracks)
	result := newResult()
	collectTrackResults(fb, result)

	if result.TotalTracks != 2 {
		t.Errorf("TotalTracks: want 2, got %d", result.TotalTracks)
	}
	if result.ConfirmedTracks != 1 {
		t.Errorf("ConfirmedTracks: want 1, got %d", result.ConfirmedTracks)
	}
}

func TestCollectTrackResults_TimestampFields(t *testing.T) {
	const startNanos int64 = 1_700_000_000_000_000_000 // 2023-11-14 22:13:20 UTC
	const endNanos int64 = 1_700_000_010_000_000_000   // +10 seconds

	tracks := map[string]*l5tracks.TrackedObject{
		"t1": {
			TrackID: "t1",
			TrackMeasurement: l5tracks.TrackMeasurement{
				TrackState:     l5tracks.TrackConfirmed,
				ObjectClass:    "vehicle",
				StartUnixNanos: startNanos,
				EndUnixNanos:   endNanos,
			},
		},
	}

	fb := makeFrameBuilder(tracks)
	result := newResult()
	collectTrackResults(fb, result)

	if len(result.Tracks) != 1 {
		t.Fatalf("expected 1 track export, got %d", len(result.Tracks))
	}
	export := result.Tracks[0]

	wantStart := time.Unix(0, startNanos).Format(time.RFC3339)
	wantEnd := time.Unix(0, endNanos).Format(time.RFC3339)
	const wantDuration = 10.0

	if export.StartTime != wantStart {
		t.Errorf("StartTime: want %q, got %q", wantStart, export.StartTime)
	}
	if export.EndTime != wantEnd {
		t.Errorf("EndTime: want %q, got %q", wantEnd, export.EndTime)
	}
	if export.DurationSecs != wantDuration {
		t.Errorf("DurationSecs: want %v, got %v", wantDuration, export.DurationSecs)
	}
}

func TestCollectTrackResults_ZeroTimestamps(t *testing.T) {
	// A track with no timestamps (zero nanos) should still export without panic.
	tracks := map[string]*l5tracks.TrackedObject{
		"t1": {
			TrackID: "t1",
			TrackMeasurement: l5tracks.TrackMeasurement{
				TrackState:  l5tracks.TrackConfirmed,
				ObjectClass: "other",
			},
		},
	}

	fb := makeFrameBuilder(tracks)
	result := newResult()
	collectTrackResults(fb, result)

	if len(result.Tracks) != 1 {
		t.Fatalf("expected 1 track export, got %d", len(result.Tracks))
	}
	export := result.Tracks[0]

	if export.DurationSecs != 0 {
		t.Errorf("DurationSecs: want 0 for zero timestamps, got %v", export.DurationSecs)
	}
}

func TestCollectTrackResults_ClassFallback(t *testing.T) {
	// A track with no ObjectClass should be labelled "other" in the export.
	tracks := map[string]*l5tracks.TrackedObject{
		"t1": {
			TrackID: "t1",
			TrackMeasurement: l5tracks.TrackMeasurement{
				TrackState:       l5tracks.TrackTentative,
				ObservationCount: 3, // below classifier threshold
			},
		},
	}

	fb := makeFrameBuilder(tracks)
	result := newResult()
	collectTrackResults(fb, result)

	if result.Tracks[0].Class != "other" {
		t.Errorf("Class: want %q, got %q", "other", result.Tracks[0].Class)
	}
	if result.TracksByClass["other"] != 1 {
		t.Errorf("TracksByClass[other]: want 1, got %d", result.TracksByClass["other"])
	}
}
