package l8behaviour

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const nanosPerSecond = int64(1_000_000_000)

// versionGroup is every interaction of the frozen scenarios at one version.
func versionGroup(t *testing.T, v InteractionVersion) []FollowingInteraction {
	t.Helper()
	var out []FollowingInteraction
	for _, fi := range surfaceInteractions(t) {
		if fi.Event.Version.InteractionVersion() == v {
			out = append(out, fi)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no interactions at %+v", v)
	}
	return out
}

func scenarioInteractions(t *testing.T, sc EncounterScenario) []FollowingInteraction {
	t.Helper()
	_, fis := interactionsOf(t, sc)
	return fis
}

func aggregate(t *testing.T, fis []FollowingInteraction) FollowingDistribution {
	t.Helper()
	d, err := AggregateFollowing(fis)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// The steady approach closes from exactly 2.0 s by 0.025 s a frame over 49
// valid 0.1 s frames and a last instant that stands for nothing, so its
// bins follow by hand (ScenarioSteadyApproach): frame 0 alone in
// [2.0, 2.25), ten frames in each of the next four bins down, and frames 41
// to 49 in [0.75, 1.0), the last of them standing for no time.
func TestAggregateFollowingBinsTheSteadyApproachByHand(t *testing.T) {
	d := aggregate(t, scenarioInteractions(t, ScenarioSteadyApproach()))
	if d.Schema != DistributionSchema || d.Events != 1 || d.ExposureEvents != 1 {
		t.Fatalf("schema %q, %d events, %d with exposure", d.Schema, d.Events, d.ExposureEvents)
	}
	want := map[float64]struct {
		instants int
		nanos    int64
	}{
		0.75: {9, 800_000_000}, 1.0: {10, nanosPerSecond}, 1.25: {10, nanosPerSecond},
		1.5: {10, nanosPerSecond}, 1.75: {10, nanosPerSecond}, 2.0: {1, 100_000_000},
	}
	h := d.Histograms[MetricFollowingNetTimeGap]
	if h.Unit != "s" || len(h.Bins) != 13 {
		t.Fatalf("net time gap histogram in %q with %d bins", h.Unit, len(h.Bins))
	}
	for _, b := range h.Bins {
		w := want[b.Lower]
		if b.Instants != w.instants || b.Nanos != w.nanos {
			t.Errorf("bin [%g, ...): %d instants, %d ns; want %d, %d", b.Lower, b.Instants, b.Nanos, w.instants, w.nanos)
		}
	}
	if last := h.Bins[len(h.Bins)-1]; last.Lower != 3 || last.Upper != nil {
		t.Fatalf("last bin %+v is not open from 3 s", last)
	}
	// Bands: below 2.0 s is frames 1-48, below 1.5 s frames 21-48, below
	// 1.0 s frames 41-48; the boundary frames are not below their band.
	for i, w := range []struct {
		instants int
		nanos    int64
	}{{49, 4_800_000_000}, {29, 2_800_000_000}, {9, 800_000_000}} {
		b := d.Bands[i]
		if b.Instants != w.instants || b.Nanos != w.nanos || b.Events != 1 {
			t.Errorf("band %s: %d instants, %d ns, %d events; want %d, %d, 1", b.Name, b.Instants, b.Nanos, b.Events,
				w.instants, w.nanos)
		}
		if b.Rate.Suppressed || *b.Rate.Value != float64(w.nanos)/4.9e9 || *b.Rate.OpportunitySeconds != 4.9 ||
			b.Rate.Uncertainty.Kind != UncertaintyNone {
			t.Errorf("band %s rate %+v", b.Name, b.Rate)
		}
	}
	if len(d.Excluded) != 0 || d.AccountedNanos != 4_900_000_000 {
		t.Fatalf("excluded %v of %d ns accounted", d.Excluded, d.AccountedNanos)
	}
}

// Pooling one version: the bins and the excluded time add up to the pooled
// accounting, every band's time is the time left of its rule, and the
// predicted-only time is all excluded.
func TestAggregateFollowingPoolsOneVersion(t *testing.T) {
	fis := scenarioInteractions(t, ScenarioSteadyApproach())
	group := versionGroup(t, fis[0].Event.Version.InteractionVersion())
	d := aggregate(t, group)
	if d.Events != len(group) || d.Events < 5 || d.ExposureEvents != d.Events {
		t.Fatalf("%d events, %d with exposure, of %d", d.Events, d.ExposureEvents, len(group))
	}
	var want InteractionAccounting
	want.BandNanos = map[MetricID]int64{}
	for _, fi := range group {
		addAccounting(&want, fi.Event.Accounting)
	}
	if !reflect.DeepEqual(d.Accounting, want) {
		t.Fatalf("accounting %+v, want %+v", d.Accounting, want)
	}
	if d.Accounting.PredictedOnlyNanos == 0 || len(d.Accounting.Suppressions) < 3 {
		t.Fatalf("the group should include predicted-only and several suppressed intervals: %+v", d.Accounting)
	}
	var excluded, predicted int64
	for r, x := range d.Excluded {
		// Every encounter's exposure is supported, so excluded time is
		// exactly the suppressed time, reason by reason.
		if s := d.Accounting.Suppressions[r]; s.Instants != x.Instants || s.Nanos != x.Nanos {
			t.Errorf("excluded %s %+v, suppressed %+v", r, x, s)
		}
		if x.PredictedOnlyNanos > x.Nanos {
			t.Errorf("excluded %s: predicted-only %d ns of %d", r, x.PredictedOnlyNanos, x.Nanos)
		}
		excluded += x.Nanos
		predicted += x.PredictedOnlyNanos
	}
	if predicted != d.Accounting.PredictedOnlyNanos {
		t.Fatalf("excluded predicted-only %d ns, accounting %d ns", predicted, d.Accounting.PredictedOnlyNanos)
	}
	for id, h := range d.Histograms {
		if h.Nanos() != d.Accounting.ValidNanos || h.Nanos()+excluded != d.AccountedNanos {
			t.Fatalf("%s: %d ns binned + %d ns excluded, %d ns valid of %d ns accounted", id, h.Nanos(), excluded,
				d.Accounting.ValidNanos, d.AccountedNanos)
		}
		var inside, overlap int64
		for _, b := range h.Bins {
			if b.SigmaInsideNanos > b.Nanos || b.Nanos > b.SigmaOverlapNanos {
				t.Errorf("%s bin %g: inside %d, time %d, overlap %d", id, b.Lower, b.SigmaInsideNanos, b.Nanos, b.SigmaOverlapNanos)
			}
			inside += b.SigmaInsideNanos
			overlap += b.SigmaOverlapNanos
		}
		if inside == h.Nanos() || overlap == h.Nanos() {
			t.Errorf("%s: one-sigma reach %d..%d ns does not bracket %d ns", id, inside, overlap, h.Nanos())
		}
	}
	thw := d.Histograms[MetricFollowingNetTimeGap]
	for _, band := range d.Bands {
		var left int64
		for _, b := range thw.Bins {
			if b.Upper != nil && *b.Upper <= band.Threshold {
				left += b.Nanos
			}
		}
		if left != band.Nanos || band.Nanos != d.Accounting.BandNanos[band.Name] {
			t.Errorf("band %s: %d ns left of the rule, %d ns pooled, %d ns accounted", band.Name, left, band.Nanos,
				d.Accounting.BandNanos[band.Name])
		}
	}
}

// An encounter whose band exposure is suppressed contributes its valid time
// beside the distribution under that reason, never as an empty bar, and a
// group with no supported exposure suppresses every pooled rate.
func TestAggregateFollowingShowsUnsupportedExposureBeside(t *testing.T) {
	d := aggregate(t, scenarioInteractions(t, ScenarioAmbiguousThenResolved()))
	if d.Events != 2 || d.ExposureEvents != 0 {
		t.Fatalf("%d events, %d with exposure", d.Events, d.ExposureEvents)
	}
	for id, h := range d.Histograms {
		if h.Nanos() != 0 {
			t.Fatalf("%s binned %d ns with no supported exposure", id, h.Nanos())
		}
	}
	insufficient := d.Excluded[ReasonInsufficientObservation]
	if insufficient.Nanos != d.Accounting.ValidNanos || insufficient.Nanos != 900_000_000 || insufficient.PredictedOnlyNanos != 0 {
		t.Fatalf("valid time of the unsupported encounter: %+v", insufficient)
	}
	if got := d.Excluded[ReasonAmbiguousLeader].Nanos; got != 4*nanosPerSecond {
		t.Fatalf("ambiguous_leader %d ns", got)
	}
	if d.AccountedNanos != 4_900_000_000 {
		t.Fatalf("accounted %d ns", d.AccountedNanos)
	}
	for _, b := range d.Bands {
		if !b.Rate.Suppressed || b.Rate.Reason != ReasonInsufficientObservation || b.Rate.Value != nil || b.Nanos != 0 {
			t.Errorf("band %s: %+v", b.Name, b)
		}
	}
	if got := d.ExcludedReasons(); !reflect.DeepEqual(got, []SuppressionReason{ReasonInsufficientObservation, ReasonAmbiguousLeader}) {
		t.Fatalf("excluded reasons %v, want precedence order", got)
	}
}

// A non-final encounter is read from its review-only block: the same bins
// as its final twin, at its own version, and a status that says provisional.
func TestAggregateFollowingReadsTheProvisionalBlockBelowFinal(t *testing.T) {
	final := aggregate(t, scenarioInteractions(t, ScenarioSteadyApproach()))
	fixed := aggregate(t, scenarioInteractions(t, provisionalScenario()))
	if fixed.Version.EstimateStage != StageFixedLag || fixed.ExposureEvents != 1 {
		t.Fatalf("stage %s, %d with exposure", fixed.Version.EstimateStage, fixed.ExposureEvents)
	}
	if !reflect.DeepEqual(fixed.Histograms, final.Histograms) || !reflect.DeepEqual(fixed.Bands, final.Bands) {
		t.Fatal("the provisional block should bin exactly as the final encounter does")
	}
	if s := StatusOf(fixed.Version); s != StatusSyntheticOracle || !strings.HasPrefix(s.Label(), "PROVISIONAL") {
		t.Fatalf("fixture estimator status %s, label %q", s, s.Label())
	}
	v := fixed.Version
	v.EstimatorID = "cv_kf_v1"
	if s := StatusOf(v); s != StatusProvisional || s.Label() != "PROVISIONAL" {
		t.Fatalf("estimator status %s, label %q", s, s.Label())
	}
}

func TestAggregateFollowingRefusesWhatItCannotPool(t *testing.T) {
	final := scenarioInteractions(t, ScenarioSteadyApproach())
	fixed := scenarioInteractions(t, provisionalScenario())
	if _, err := AggregateFollowing(nil); !errors.Is(err, ErrNothingToAggregate) {
		t.Fatalf("empty input: %v", err)
	}
	if _, err := AggregateFollowing(append(final, fixed...)); err == nil || !strings.Contains(err.Error(), "pools one version") {
		t.Fatalf("two versions: %v", err)
	}
	if _, err := AggregateFollowing(append(final, final...)); err == nil || !strings.Contains(err.Error(), "appears twice") {
		t.Fatalf("repeated event: %v", err)
	}
	tampered := final[0]
	tampered.Event.Accounting.ValidNanos++
	if _, err := AggregateFollowing([]FollowingInteraction{tampered}); err == nil {
		t.Fatal("an interaction whose accounting disagrees with its instants was pooled")
	}
}

func TestHistogramEdgesAndSigmaReach(t *testing.T) {
	if err := checkBandEdges(); err != nil {
		t.Fatal(err)
	}
	h := distributionHistograms[0]
	for v, want := range map[float64]int{0: 0, 0.2499: 0, 0.25: 1, 1.0: 4, 1.4999: 5, 1.5: 6, 2.0: 8, 2.9999: 11, 3: 12, 99: 12} {
		if got := h.index(v); got != want {
			t.Errorf("index(%g) = %d, want %d", v, got, want)
		}
	}
	for _, c := range []struct {
		value, sigma          float64
		bin                   int
		inside                bool
		overlapFirst, overlap int
	}{
		{1.6, 0.05, 6, true, 6, 1},      // wholly inside [1.5, 1.75)
		{1.52, 0.05, 6, false, 5, 2},    // reaches down into [1.25, 1.5)
		{1.6, 0, 6, true, 6, 1},         // no spread
		{0.05, 0.1, 0, false, 0, 1},     // reaches below the first edge only
		{2.9, 0.5, 11, false, 9, 4},     // from [2.25, 2.5) into the open bin
		{3.5, 0.1, 12, true, 12, 1},     // wholly inside the open bin
		{1.25, 0.2, 5, false, 4, 2},     // on an edge, reaching down
		{0.74, 0.02, 2, false, 2, 2},    // just below an edge, reaching up
		{0.125, 0.125, 0, false, 0, 2},  // closed [0, 0.25] touches bin 1's edge
		{0.125, 0.1249, 0, true, 0, 1},  // short of both edges
		{0.125, 0.1251, 0, false, 0, 2}, // past both
	} {
		hist := h.empty()
		if err := h.add(&hist, SeriesValue{Value: c.value, Sigma: c.sigma}, 7); err != nil {
			t.Fatal(err)
		}
		var overlapping []int
		for k, b := range hist.Bins {
			if b.SigmaOverlapNanos == 7 {
				overlapping = append(overlapping, k)
			}
			if (k == c.bin) != (b.Nanos == 7) || (k == c.bin && c.inside) != (b.SigmaInsideNanos == 7) {
				t.Errorf("%g +/- %g: bin %d holds %+v", c.value, c.sigma, k, b)
			}
		}
		if len(overlapping) != c.overlap || overlapping[0] != c.overlapFirst {
			t.Errorf("%g +/- %g overlaps bins %v, want %d from %d", c.value, c.sigma, overlapping, c.overlap, c.overlapFirst)
		}
	}
	hist := h.empty()
	if err := h.add(&hist, SeriesValue{Value: -0.1}, 1); err == nil {
		t.Fatal("a negative value was binned")
	}
}

// A summary is the stored event without the provenance each measurement
// repeats: same values, same suppressions, and review-only values only
// below final.
func TestEncounterSummaryKeepsStoredValues(t *testing.T) {
	for _, fi := range append(scenarioInteractions(t, ScenarioOcclusion()), scenarioInteractions(t, provisionalScenario())...) {
		ev := fi.Event
		s := ev.Summary()
		if s.EventID != ev.EventID || s.PrimaryTrackID != ev.PrimaryTrackID || s.SecondaryTrackID != ev.SecondaryTrackID ||
			s.GeometryID != ev.Version.GeometryID || !reflect.DeepEqual(s.Accounting, ev.Accounting) {
			t.Fatalf("summary %+v of %s", s, ev.EventID)
		}
		if (len(s.Provisional) > 0) != (ev.Version.EstimateStage != StageFinal) {
			t.Fatalf("%s at %s: %d provisional values", ev.EventID, ev.Version.EstimateStage, len(s.Provisional))
		}
		for id, m := range ev.Measurements {
			got := s.Measurements[id]
			if err := got.Validate(); err != nil {
				t.Fatal(err)
			}
			if got.Suppressed != m.Suppressed || got.Reason != m.Reason || !reflect.DeepEqual(got.Uncertainty, m.Uncertainty) ||
				!reflect.DeepEqual(got.Value, m.Value) {
				t.Fatalf("%s %s: %+v, stored %+v", ev.EventID, id, got, m)
			}
		}
		for _, id := range []MetricID{MetricFollowingNetTimeGapMin, MetricFollowingNetTimeGapP50} {
			block := s.Measurements
			if ev.Version.EstimateStage != StageFinal {
				block = s.Provisional
			}
			if u := block[id].Uncertainty; u == nil || u.Kind != UncertaintyInterval || u.Method != MethodMonteCarlo || u.Samples == 0 {
				t.Fatalf("%s %s: the stored Monte Carlo interval did not survive: %+v", ev.EventID, id, u)
			}
		}
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"provenance"`) {
			t.Fatalf("summary repeats provenance: %s", raw)
		}
	}
}
