package sweep

import (
	"math"
	"testing"
)

func TestHeadingObjectiveMissingEvidenceAndBand(t *testing.T) {
	w := ObjectiveWeights{CourseAlignment: -.01, ActiveTrackBand: &TrackCountBand{Min: 2, Max: 4, Penalty: .5}}
	x := 20.0
	r := ComboResult{CourseAlignmentP50Mean: &x, CourseAlignmentSnapshots: 1, TrackMetricsSnapshots: 1, ActiveTracksMean: 3}
	if s := ScoreResult(r, w); math.Abs(s+.2) > 1e-9 {
		t.Fatal(s)
	}
	for _, count := range []float64{1, 5} {
		r.ActiveTracksMean = count
		if s := ScoreResult(r, w); math.Abs(s+.7) > 1e-9 {
			t.Fatal(s)
		}
	}
	r.CourseAlignmentP50Mean = nil
	if ScoreResult(r, w) != -math.MaxFloat64 {
		t.Fatal("missing course won")
	}
	r.CourseAlignmentP50Mean = &x
	r.TrackMetricsSnapshots = 0
	if ScoreResult(r, w) != -math.MaxFloat64 {
		t.Fatal("missing count won")
	}
	r.TrackMetricsSnapshots = 1
	for _, bad := range []float64{math.NaN(), math.Inf(1), -1, 91} {
		r.CourseAlignmentP50Mean = &bad
		if ScoreResult(r, w) != -math.MaxFloat64 {
			t.Fatal("invalid course accepted")
		}
	}
	r.CourseAlignmentP50Mean = &x
	for _, bad := range []float64{1, math.NaN(), math.Inf(-1)} {
		w.CourseAlignment = bad
		if ScoreResult(r, w) != -math.MaxFloat64 {
			t.Fatal("invalid course weight")
		}
	}
}

func TestFiniteMetricNumber(t *testing.T) {
	for _, v := range []interface{}{nil, "0", math.NaN(), math.Inf(1), true} {
		if _, ok := finiteMetricNumber(v); ok {
			t.Fatalf("malformed value accepted: %v", v)
		}
	}
	for _, v := range []interface{}{float64(0), float32(0), int(0), int64(0)} {
		if n, ok := finiteMetricNumber(v); !ok || n != 0 {
			t.Fatalf("real zero rejected: %v", v)
		}
	}
}

func TestTrackCountBandValidityAndDefaultReward(t *testing.T) {
	for _, b := range []TrackCountBand{{Min: -1, Max: 2, Penalty: 1}, {Min: 3, Max: 2, Penalty: 1}, {Min: 0, Max: 2, Penalty: 0}, {Min: 0, Max: math.Inf(1), Penalty: 1}, {Min: 0, Max: 2, Penalty: math.NaN()}} {
		if b.valid() {
			t.Fatal("invalid band")
		}
	}
	w := DefaultObjectiveWeights()
	if ScoreResult(ComboResult{ActiveTracksMean: 100}, w) != ScoreResult(ComboResult{ActiveTracksMean: 1}, w) {
		t.Fatal("default rewards duplicates")
	}
}

func TestHeadingAggregationExcludesMissingSnapshots(t *testing.T) {
	r := &Runner{}
	got := r.computeComboResult([]SampleResult{
		{TrackMetricsAvailable: true, ActiveTracks: 4, CourseAlignmentSamples: 10, CourseAlignmentP50Deg: 20},
		{},
		{TrackMetricsAvailable: true, ActiveTracks: 6, CourseAlignmentSamples: 20, CourseAlignmentP50Deg: 40},
		{CourseAlignmentSamples: 1, CourseAlignmentP50Deg: math.NaN()},
	}, nil)
	if got.CourseAlignmentP50Mean == nil || *got.CourseAlignmentP50Mean != 30 || got.CourseAlignmentSnapshots != 2 || got.ActiveTracksMean != 5 || got.TrackMetricsSnapshots != 2 {
		t.Fatalf("bad aggregation: %+v", got)
	}
	if got := r.computeComboResult([]SampleResult{{}}, nil); got.CourseAlignmentP50Mean != nil {
		t.Fatal("missing became zero")
	}
}
