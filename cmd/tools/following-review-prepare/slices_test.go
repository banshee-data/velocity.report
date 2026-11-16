package main

import (
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

func TestReviewSlicesPartitionAnchorsAndRetainOriginalIntervals(t *testing.T) {
	const start = int64(1_788_380_529_986_405_330)
	rows := []fieldrun.EvidenceRow{
		{FollowerTrackID: "a", CaptureUnixNanos: start, IntervalNanos: 100},
		{FollowerTrackID: "b", CaptureUnixNanos: start + 59_999_999_999, IntervalNanos: 100_000_000},
		{FollowerTrackID: "a", CaptureUnixNanos: start + 60_000_000_000},
	}
	d := fieldrun.EvidenceDocument{Rows: rows, Analysis: l8behaviour.FollowingAnalysis{Timelines: []l8behaviour.FollowerTimeline{{FollowerTrackID: "a", GeometryID: "path-a"}, {FollowerTrackID: "b", GeometryID: "path-b"}}}}
	s := makeSlices(d, recorder.LogHeader{StartNs: start, EndNs: start + 60_000_000_000, BuildGitSHA: "producer"}, "canonical-sha", "header-sha", "calibration")
	if len(s) != 2 || len(s[0].Rows) != 2 || len(s[1].Rows) != 1 {
		t.Fatalf("incorrect partition: %+v", s)
	}
	if !reflect.DeepEqual(append(s[0].Rows, s[1].Rows...), rows) {
		t.Fatal("changed an authoritative row")
	}
	if s[0].ReviewSlice.SampleEndExclusiveNanos != s[1].ReviewSlice.SampleStartNanos || s[1].ReviewSlice.SampleEndExclusiveNanos != start+60_000_000_001 {
		t.Fatal("lost the terminal sample or exact nanoseconds")
	}
	if s[0].Rows[1].IntervalNanos != 100_000_000 {
		t.Fatal("clipped a straddling interval")
	}
	if len(s[1].Analysis.Timelines) != 1 || s[1].Analysis.Timelines[0].GeometryID != "path-a" {
		t.Fatal("substituted a path")
	}
	if s[0].ReviewSlice.CanonicalEvidenceSHA256 != "canonical-sha" || s[0].ReviewSlice.RecordingHeaderSHA256 != "header-sha" || s[0].ReviewSlice.RecordingBuildGitSHA != "producer" {
		t.Fatal("lost provenance")
	}
}
