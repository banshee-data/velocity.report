package l8behaviour

import (
	"math"
	"strings"
	"testing"
)

// clipScenarios are the fixture scenarios with stored encounters, and a
// non-final one, whose production block is suppressed by stage.
func clipScenarios() []EncounterScenario {
	return append(EncounterScenarios(), provisionalScenario())
}

// The recomputation is exact: over all of an encounter's stored instants it
// reproduces the stored interval, accounting, worst support, provenance and
// every measurement, for every fixture encounter. This is the check
// ClipFollowingInteraction runs before it clips, and what makes a clipped
// encounter's values the encounter method's own rather than an
// approximation of them.
func TestRecomputeReproducesEveryStoredEncounter(t *testing.T) {
	n := 0
	for _, sc := range clipScenarios() {
		_, fis := interactionsOf(t, sc)
		for _, fi := range fis {
			stored := roundTrip(t, fi)
			got, err := recomputeEncounter(stored.Event, stored.Instants, sc.Params, sc.Params.Hash())
			if err != nil {
				t.Fatalf("%s %s: %v", sc.Name, fi.Event.EventID, err)
			}
			if diff := sameRecord(got, stored.Event); diff != "" {
				t.Errorf("%s %s: recomputed %s differs", sc.Name, fi.Event.EventID, diff)
			}
			n++
		}
	}
	if n == 0 {
		t.Fatal("no fixture encounters")
	}
}

// longestInteraction is the steady approach's encounter, 50 instants.
func longestInteraction(t *testing.T) (FollowingInteraction, FollowingAnalysisParams) {
	t.Helper()
	sc := ScenarioSteadyApproach()
	_, fis := interactionsOf(t, sc)
	best := fis[0]
	for _, fi := range fis {
		if len(fi.Instants) > len(best.Instants) {
			best = fi
		}
	}
	if len(best.Instants) < 10 {
		t.Fatalf("longest encounter has %d instants", len(best.Instants))
	}
	return roundTrip(t, best), sc.Params
}

func TestClipWholeEncounterIsContained(t *testing.T) {
	fi, params := longestInteraction(t)
	got, outcome, _, err := ClipFollowingInteraction(fi, fi.Event.StartUnixNanos, fi.Event.EndUnixNanos, params)
	if err != nil || outcome != ClipContained {
		t.Fatalf("outcome %v, %v; want contained", outcome, err)
	}
	if got.Event.EventID != fi.Event.EventID || len(got.Instants) != len(fi.Instants) {
		t.Fatal("a contained encounter was changed")
	}
}

// Clipping keeps the instants inside the window and rebuilds everything else
// from them: the interval, the accounting and windows, and the measurements,
// whose point values are the clipped series' own.
func TestClipRebuildsTheEncounterFromTheInstantsInside(t *testing.T) {
	fi, params := longestInteraction(t)
	// The steady approach closes all the way, so its minimum net time gap is
	// at its end; a window over its first half has a larger one.
	mid := fi.Instants[len(fi.Instants)/2].CaptureUnixNanos
	start, end := fi.Event.StartUnixNanos-1_000_000_000, mid

	got, outcome, detail, err := ClipFollowingInteraction(fi, start, end, params)
	if err != nil || outcome != ClipClipped {
		t.Fatalf("outcome %v %q, %v; want clipped", outcome, detail, err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("clipped encounter does not validate: %v", err)
	}
	if got.Event.EventID != fi.Event.EventID {
		t.Error("clipping changed the event id: it is the same encounter at the same version")
	}
	var kept []InteractionInstant
	for _, in := range fi.Instants {
		if in.CaptureUnixNanos <= end {
			kept = append(kept, in)
		}
	}
	if len(got.Instants) != len(kept) || got.Event.StartUnixNanos != fi.Event.StartUnixNanos ||
		got.Event.EndUnixNanos != mid {
		t.Fatalf("clipped to %d instants %d..%d, want %d to %d", len(got.Instants),
			got.Event.StartUnixNanos, got.Event.EndUnixNanos, len(kept), mid)
	}
	if got.Event.Accounting.ValidNanos >= fi.Event.Accounting.ValidNanos {
		t.Errorf("clipped valid time %d is not less than the whole %d", got.Event.Accounting.ValidNanos, fi.Event.Accounting.ValidNanos)
	}

	// The minimum net time gap is the clipped series' minimum, not the
	// untrimmed encounter's.
	minThw := math.Inf(1)
	for _, in := range kept {
		if v, ok := in.Values[MetricFollowingNetTimeGap]; ok && in.Valid {
			minThw = math.Min(minThw, v.Value)
		}
	}
	m := got.Event.Measurements[MetricFollowingNetTimeGapMin]
	if m.Suppressed || m.Value == nil || *m.Value != minThw {
		t.Fatalf("clipped minimum net time gap = %+v, want %v", m, minThw)
	}
	whole := fi.Event.Measurements[MetricFollowingNetTimeGapMin]
	if *whole.Value == minThw {
		t.Fatal("test window does not change the minimum; choose another")
	}

	// Clipping again to the same window changes nothing.
	again, outcome, _, err := ClipFollowingInteraction(got, start, end, params)
	if err != nil || outcome != ClipContained || again.Event.EventID != got.Event.EventID {
		t.Fatalf("second clip: %v, %v", outcome, err)
	}

	// A distribution pools the clipped encounter like any other.
	if _, err := AggregateFollowing([]FollowingInteraction{got}); err != nil {
		t.Fatalf("aggregate clipped: %v", err)
	}
}

// A non-final encounter clips the same way; its production block stays
// suppressed by stage and its provisional block is recomputed.
func TestClipNonFinalEncounter(t *testing.T) {
	sc := provisionalScenario()
	_, fis := interactionsOf(t, sc)
	fi := roundTrip(t, fis[0])
	start := fi.Instants[len(fi.Instants)/2].CaptureUnixNanos
	got, outcome, detail, err := ClipFollowingInteraction(fi, start, fi.Event.EndUnixNanos, sc.Params)
	if err != nil || outcome != ClipClipped {
		t.Fatalf("outcome %v %q, %v", outcome, detail, err)
	}
	for id, m := range got.Event.Measurements {
		if !m.Suppressed || m.Reason != ReasonEstimateNotFinal {
			t.Errorf("%s: %+v, want suppressed as not final", id, m)
		}
	}
	if len(got.Event.Provisional) == 0 {
		t.Fatal("clipped non-final encounter has no provisional block")
	}
}

func TestClipOutsideTheEncounterIsEmpty(t *testing.T) {
	fi, params := longestInteraction(t)
	a, b := fi.Instants[0].CaptureUnixNanos, fi.Instants[1].CaptureUnixNanos
	for _, w := range [][2]int64{
		{a + 1, b - 1}, // between two instants
		{fi.Event.EndUnixNanos + 1, fi.Event.EndUnixNanos + 1_000_000_000},
	} {
		if _, outcome, _, err := ClipFollowingInteraction(fi, w[0], w[1], params); err != nil || outcome != ClipEmpty {
			t.Errorf("window %v: outcome %v, %v; want empty", w, outcome, err)
		}
	}
}

// An encounter analysed under other parameters, or whose stored values the
// recomputation does not reproduce, is never clipped.
func TestClipRefusesWhatItCannotRecompute(t *testing.T) {
	fi, params := longestInteraction(t)
	start := fi.Instants[len(fi.Instants)/2].CaptureUnixNanos

	other := params
	other.Exposure.MonteCarloSamples++
	if _, outcome, detail, err := ClipFollowingInteraction(fi, start, fi.Event.EndUnixNanos, other); err != nil ||
		outcome != ClipUnrecomputable || !strings.Contains(detail, "method") {
		t.Errorf("other parameters: outcome %v %q, %v; want unrecomputable naming the method", outcome, detail, err)
	}

	tampered := roundTrip(t, fi)
	m := tampered.Event.Measurements[MetricFollowingSpatialGapMin]
	if m.Value == nil {
		t.Fatal("fixture has no spatial gap minimum")
	}
	v := *m.Value + 0.5
	m.Value = &v
	tampered.Event.Measurements[MetricFollowingSpatialGapMin] = m
	if _, outcome, detail, err := ClipFollowingInteraction(tampered, start, fi.Event.EndUnixNanos, params); err != nil ||
		outcome != ClipUnrecomputable || !strings.Contains(detail, "measurements") {
		t.Errorf("tampered value: outcome %v %q, %v; want unrecomputable naming measurements", outcome, detail, err)
	}
}

func TestClipRefusesAnUnorderedWindow(t *testing.T) {
	fi, params := longestInteraction(t)
	for _, w := range [][2]int64{{0, 10}, {20, 10}} {
		if _, _, _, err := ClipFollowingInteraction(fi, w[0], w[1], params); err == nil {
			t.Errorf("window %v accepted", w)
		}
	}
}
