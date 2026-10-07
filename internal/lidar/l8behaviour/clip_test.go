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
	got, outcome, _, err := ClipFollowingInteraction(fi, fi.Event.StartUnixNanos, fi.Event.EndUnixNanos+1, params)
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
		if in.CaptureUnixNanos < end { // the window is half-open
			kept = append(kept, in)
		}
	}
	last := kept[len(kept)-1].CaptureUnixNanos
	if len(got.Instants) != len(kept) || got.Event.StartUnixNanos != fi.Event.StartUnixNanos ||
		got.Event.EndUnixNanos != last || last >= mid {
		t.Fatalf("clipped to %d instants %d..%d, want %d to %d, before %d", len(got.Instants),
			got.Event.StartUnixNanos, got.Event.EndUnixNanos, len(kept), last, mid)
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
	for _, w := range [][2]int64{{0, 10}, {20, 10}, {10, 10}} {
		if _, _, _, err := ClipFollowingInteraction(fi, w[0], w[1], params); err == nil {
			t.Errorf("window %v accepted", w)
		}
	}
}

// unobservedLeaderScenario is the steady approach with every fifth leader
// and follower sample unobserved (missed_unknown): rows whose points carry
// not_observed, which buildEncounter still counts as evaluated frames.
func unobservedLeaderScenario() EncounterScenario {
	sc := ScenarioSteadyApproach()
	for i := range sc.Trajectories {
		for j := range sc.Trajectories[i].Samples {
			if j%5 == 2 {
				s := &sc.Trajectories[i].Samples[j]
				s.Support = SupportMissedUnknown
				s.Faces.FrontObserved, s.Faces.RearObserved = false, false // no face is seen unobserved
			}
		}
	}
	sc.Name = "steady_approach_unobserved_rows"
	return sc
}

// A leader sample whose support is missed_unknown is still a sample: the pair
// was evaluated there, and its frames count. The recomputation must count
// them as buildEncounter does, or every such encounter fails the check.
func TestRecomputeCountsUnobservedSamplesAsEvaluated(t *testing.T) {
	sc := unobservedLeaderScenario()
	_, fis := interactionsOf(t, sc)
	notObserved := 0
	for _, fi := range fis {
		stored := roundTrip(t, fi)
		for _, in := range stored.Instants {
			if in.Role == DispositionLeader && in.LeaderSupport == SupportMissedUnknown && len(in.Reasons) > 0 {
				notObserved++
			}
		}
		got, err := recomputeEncounter(stored.Event, stored.Instants, sc.Params, sc.Params.Hash())
		if err != nil {
			t.Fatal(err)
		}
		if diff := sameRecord(got, stored.Event); diff != "" {
			t.Errorf("%s: recomputed %s differs", fi.Event.EventID, diff)
		}
	}
	if notObserved == 0 {
		t.Fatal("the scenario has no evaluated instant with a missed_unknown leader sample")
	}
}

// Adjacent half-open windows partition an encounter's instants, so their
// clipped valid times add up to the whole encounter's exactly.
func TestClipAdjacentWindowsPartitionValidTime(t *testing.T) {
	fi, params := longestInteraction(t)
	start, end := fi.Event.StartUnixNanos, fi.Event.EndUnixNanos+1
	cut := fi.Instants[len(fi.Instants)/3].CaptureUnixNanos
	var valid int64
	var instants int
	for _, w := range [][2]int64{{start, cut}, {cut, end}} {
		got, outcome, detail, err := ClipFollowingInteraction(fi, w[0], w[1], params)
		if err != nil || outcome != ClipClipped {
			t.Fatalf("window %v: %v %q, %v", w, outcome, detail, err)
		}
		valid += got.Event.Accounting.ValidNanos
		instants += len(got.Instants)
	}
	if valid != fi.Event.Accounting.ValidNanos || instants != len(fi.Instants) {
		t.Fatalf("windows hold %d ns over %d instants, the encounter %d ns over %d",
			valid, instants, fi.Event.Accounting.ValidNanos, len(fi.Instants))
	}
}

// Interval bounds may differ in their last bits between machines, which fuse
// multiply-adds differently; nothing else may differ at all.
func TestSameRecordToleratesOnlyIntervalBoundRounding(t *testing.T) {
	fi, _ := longestInteraction(t)
	perturb := func(field string, by float64) InteractionEvent {
		ev := roundTrip(t, fi).Event
		m := ev.Measurements[MetricFollowingSpatialGapMin]
		switch field {
		case "lower":
			v := *m.Uncertainty.Lower * (1 + by)
			m.Uncertainty.Lower = &v
		case "value":
			v := *m.Value * (1 + by)
			m.Value = &v
		}
		ev.Measurements[MetricFollowingSpatialGapMin] = m
		return ev
	}
	stored := roundTrip(t, fi).Event
	if m := stored.Measurements[MetricFollowingSpatialGapMin]; m.Uncertainty == nil || m.Uncertainty.Lower == nil || m.Value == nil {
		t.Fatal("fixture minimum has no interval")
	}
	if diff := sameRecord(perturb("lower", 1e-15), stored); diff != "" {
		t.Errorf("a last-bit difference in a bound was refused: %s", diff)
	}
	if diff := sameRecord(perturb("lower", 1e-6), stored); diff == "" {
		t.Error("a real difference in a bound was accepted")
	}
	if diff := sameRecord(perturb("value", 1e-15), stored); diff == "" {
		t.Error("a difference in a point value was accepted")
	}
}

// A class the following rule does not support suppresses every stored
// measurement, and a clipped encounter keeps that decision: the class is
// not stored, so it is read back from the stored measurements.
func TestClipKeepsAnUnsupportedClassSuppressed(t *testing.T) {
	sc := ScenarioSteadyApproach()
	for i := range sc.Trajectories {
		sc.Trajectories[i].Passage.MotionClass = MotionPedestrian
	}
	_, fis := interactionsOf(t, sc)
	if len(fis) == 0 {
		t.Fatal("no encounter between pedestrians")
	}
	fi := roundTrip(t, fis[0])
	if storedClassReason(fi.Event) != ReasonClassNotSupported {
		t.Fatalf("stored measurements %+v are not suppressed for class", fi.Event.Measurements)
	}
	start := fi.Instants[len(fi.Instants)/2].CaptureUnixNanos
	clipped, outcome, detail, err := ClipFollowingInteraction(fi, start, fi.Event.EndUnixNanos, sc.Params)
	if err != nil || outcome != ClipClipped {
		t.Fatalf("outcome %v %q, %v; want clipped", outcome, detail, err)
	}
	for id, m := range clipped.Event.Measurements {
		if !m.Suppressed || m.Reason != ReasonClassNotSupported {
			t.Errorf("clipped %s is %+v; want it suppressed for class", id, m)
		}
	}
}

// An invalid encounter is an error, not an outcome.
func TestClipRefusesAnInvalidEncounter(t *testing.T) {
	fi, params := longestInteraction(t)
	broken := roundTrip(t, fi)
	broken.Event.EventID = ""
	if _, _, _, err := ClipFollowingInteraction(broken, fi.Event.StartUnixNanos+1, fi.Event.EndUnixNanos, params); err == nil {
		t.Error("an encounter with no event id: want an error")
	}
}

// A record gap marked on a stored instant that the analysis did not mark,
// with the stored accounting and windows made to agree with it so the record
// still validates, changes what the whole encounter recomputes to: its
// worst support and measurements no longer match, so it is not clipped.
func TestClipRefusesARecordWithAnAlteredGap(t *testing.T) {
	fi, params := longestInteraction(t)
	altered := roundTrip(t, fi)
	mid := len(altered.Instants) / 2
	altered.Instants[mid].RecordGap = true
	altered.Event.Accounting = instantAccounting(altered.Instants)
	altered.Windows = deriveWindows(altered.Event, altered.Instants)
	if err := altered.Validate(); err != nil {
		t.Fatalf("altered record does not validate: %v", err)
	}
	start := altered.Instants[mid].CaptureUnixNanos
	_, outcome, detail, err := ClipFollowingInteraction(altered, start, altered.Event.EndUnixNanos, params)
	if err != nil || outcome != ClipUnrecomputable || !strings.Contains(detail, "does not reproduce") {
		t.Errorf("outcome %v %q, %v; want unrecomputable", outcome, detail, err)
	}
}

// sameEncoded compares structure exactly: keys, lengths, element order and
// types must match, and only numbers under "lower" or "upper" may differ by
// the tolerance. A value that cannot be encoded matches nothing.
func TestSameEncodedComparesStructure(t *testing.T) {
	for _, c := range []struct {
		name string
		a, b any
		want bool
	}{
		{"identical", map[string]any{"x": 1.0}, map[string]any{"x": 1.0}, true},
		{"other key", map[string]any{"x": 1.0}, map[string]any{"y": 1.0}, false},
		{"extra key", map[string]any{"x": 1.0}, map[string]any{"x": 1.0, "y": 2.0}, false},
		{"map against list", map[string]any{"x": 1.0}, []any{1.0}, false},
		{"list length", []any{1.0}, []any{1.0, 2.0}, false},
		{"list against map", []any{1.0}, map[string]any{"x": 1.0}, false},
		{"list element", []any{1.0, 2.0}, []any{1.0, 3.0}, false},
		{"number against text", map[string]any{"lower": 1.0}, map[string]any{"lower": "1"}, false},
		{"bound within tolerance", map[string]any{"lower": 1.0}, map[string]any{"lower": 1.0 + 1e-15}, true},
		{"bound past tolerance", map[string]any{"upper": 1.0}, map[string]any{"upper": 1.0 + 1e-9}, false},
		{"other number in last bit", map[string]any{"value": 1.0}, map[string]any{"value": 1.0 + 1e-15}, false},
		{"bounds in a list", map[string]any{"lower": []any{1.0}}, map[string]any{"lower": []any{1.0 + 1e-15}}, true},
		{"unencodable", math.NaN(), math.NaN(), false},
	} {
		if got := sameEncoded(c.a, c.b); got != c.want {
			t.Errorf("%s: sameEncoded = %v, want %v", c.name, got, c.want)
		}
	}
}

// A stored encounter whose interval does not span its instants exactly is
// invalid, and so is one whose windows are not the ones its instants imply.
func TestValidateRefusesAnIntervalOrWindowsThatDisagreeWithTheInstants(t *testing.T) {
	fi, _ := longestInteraction(t)
	trimmed := roundTrip(t, fi)
	trimmed.Instants = trimmed.Instants[1:] // the event still starts at the dropped instant
	trimmed.Event.Accounting = instantAccounting(trimmed.Instants)
	if err := trimmed.Validate(); err == nil || !strings.Contains(err.Error(), "instants span") {
		t.Errorf("interval wider than its instants: %v; want it refused", err)
	}
	if len(fi.Windows) == 0 {
		t.Fatal("fixture encounter has no windows")
	}
	fewer := roundTrip(t, fi)
	fewer.Windows = fewer.Windows[:len(fewer.Windows)-1]
	if err := fewer.Validate(); err == nil || !strings.Contains(err.Error(), "windows stored") {
		t.Errorf("a window missing: %v; want it refused", err)
	}
}
