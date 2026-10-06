package l5tracks

import (
	"reflect"
	"testing"
)

// QualityMetrics computes what ComputeQualityMetrics stores, and leaves the
// track as it was.
func TestQualityMetricsMatchesComputeQualityMetrics(t *testing.T) {
	const second = int64(1_000_000_000)
	track := &TrackedObject{TrackID: "trk-q", TrackMeasurement: TrackMeasurement{
		StartUnixNanos: 10 * second, EndUnixNanos: 14 * second, ObservationCount: 30,
	}}
	// Three metres east, a 0.5 s gap (an occlusion of five frames), four north.
	track.History = []TrackPoint{
		{X: 0, Y: 0, Timestamp: 10 * second},
		{X: 3, Y: 0, Timestamp: 10*second + second/10},
		{X: 3, Y: 4, Timestamp: 10*second + 6*second/10},
	}
	before := *track
	q := track.QualityMetrics()
	if !reflect.DeepEqual(*track, before) {
		t.Fatal("QualityMetrics changed the track")
	}
	want := TrackQuality{LengthMeters: 7, DurationSecs: 4, OcclusionCount: 1, MaxOcclusionFrames: 5, SpatialCoverage: 0.75}
	if q != want {
		t.Fatalf("QualityMetrics = %+v, want %+v", q, want)
	}
	track.ComputeQualityMetrics()
	if track.TrackLengthMeters != q.LengthMeters || track.TrackDurationSecs != q.DurationSecs ||
		track.OcclusionCount != q.OcclusionCount || track.MaxOcclusionFrames != q.MaxOcclusionFrames ||
		track.SpatialCoverage != q.SpatialCoverage {
		t.Fatalf("ComputeQualityMetrics stored %+v", track)
	}
}

// A track with no elapsed time keeps the duration and coverage it held, as
// before QualityMetrics existed.
func TestComputeQualityMetricsKeepsUndefinedDurationAndCoverage(t *testing.T) {
	track := &TrackedObject{TrackID: "trk-single", TrackMeasurement: TrackMeasurement{
		StartUnixNanos: 5, EndUnixNanos: 5, ObservationCount: 1,
	}}
	track.TrackDurationSecs, track.SpatialCoverage = 2, 0.4
	track.ComputeQualityMetrics()
	if track.TrackDurationSecs != 2 || track.SpatialCoverage != 0.05 {
		t.Fatalf("duration %v coverage %v, want 2 and 1/(2*10)", track.TrackDurationSecs, track.SpatialCoverage)
	}
	if q := track.QualityMetrics(); q.DurationSecs != 0 || q.SpatialCoverage != 0 {
		t.Fatalf("QualityMetrics on a single instant: %+v, want no duration or coverage", q)
	}
}
