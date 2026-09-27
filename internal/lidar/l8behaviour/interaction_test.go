package l8behaviour

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const testSourceID = "source/v1/fixture"

// interactionsOf analyses a scenario and maps every encounter to its stored
// form.
func interactionsOf(t *testing.T, sc EncounterScenario) (FollowingAnalysis, []FollowingInteraction) {
	t.Helper()
	a := analyse(t, sc)
	fis, err := FollowingInteractions(testSourceID, a)
	if err != nil {
		t.Fatalf("%s: %v", sc.Name, err)
	}
	if len(fis) != len(a.Encounters) {
		t.Fatalf("%s: %d interactions for %d encounters", sc.Name, len(fis), len(a.Encounters))
	}
	return a, fis
}

// provisionalScenario is the steady approach over fixed-lag estimates: the
// same values, none of them publishable.
func provisionalScenario() EncounterScenario {
	sc := ScenarioSteadyApproach()
	for i := range sc.Trajectories {
		for j := range sc.Trajectories[i].Samples {
			sc.Trajectories[i].Samples[j].Stage = StageFixedLag
		}
	}
	sc.Name = "steady_approach_fixed_lag"
	return sc
}

// roundTrip encodes and decodes every record separately, as the store does.
func roundTrip(t *testing.T, fi FollowingInteraction) FollowingInteraction {
	t.Helper()
	var out FollowingInteraction
	decode := func(v any, dst any) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
	}
	decode(fi.Event, &out.Event)
	for _, in := range fi.Instants {
		var got InteractionInstant
		decode(in, &got)
		out.Instants = append(out.Instants, got)
	}
	for _, w := range fi.Windows {
		var got ExposureWindow
		decode(w, &got)
		out.Windows = append(out.Windows, got)
	}
	return out
}

// Every field of an encounter that is persisted survives the mapping and a
// JSON round trip, and reads back in the encounter's own shape.
func TestFollowingInteractionRoundTrip(t *testing.T) {
	scenarios := append(EncounterScenarios(), provisionalScenario())
	for _, sc := range scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			a, fis := interactionsOf(t, sc)
			for i, fi := range fis {
				e := a.Encounters[i]
				got := roundTrip(t, fi)
				if !reflect.DeepEqual(got, fi) {
					t.Fatalf("%s -> %s: records changed in a JSON round trip", e.LeaderTrackID, e.FollowerTrackID)
				}
				if err := got.Validate(); err != nil {
					t.Fatalf("decoded records do not validate: %v", err)
				}
				checkAgainstEncounter(t, got, e)
			}
		})
	}
}

func checkAgainstEncounter(t *testing.T, fi FollowingInteraction, e Encounter) {
	t.Helper()
	ev := fi.Event
	if ev.Type != InteractionFollowing || ev.PrimaryTrackID != e.FollowerTrackID || ev.SecondaryTrackID != e.LeaderTrackID ||
		ev.StartUnixNanos != e.FirstUnixNanos || ev.EndUnixNanos != e.LastUnixNanos || ev.SourceID != testSourceID {
		t.Fatalf("identity %+v", ev)
	}
	if ev.Version.EstimateStage != e.Stage || ev.Version.MethodID != e.MethodID || ev.Version.GeometryID != e.GeometryID ||
		ev.WorstSupport != e.WorstSupport {
		t.Fatalf("version %+v worst %s, encounter stage %s method %s geometry %s worst %s",
			ev.Version, ev.WorstSupport, e.Stage, e.MethodID, e.GeometryID, e.WorstSupport)
	}
	for name, pair := range map[string][2]any{
		"measurements":         {ev.OrderedMeasurements(), e.Measurements},
		"provisional":          {ev.OrderedProvisional(), e.Provisional},
		"accounting":           {ev.EncounterAccounting(), e.Accounting},
		"spatial gap series":   {fi.SpatialGapSeries(), e.SpatialGapSeries},
		"net time gap series":  {fi.NetTimeGapSeries(), e.NetTimeGapSeries},
		"predicted gap series": {fi.PredictedGapSeries(), e.PredictedGapSeries},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s: stored %+v, encounter %+v", name, pair[0], pair[1])
		}
	}
	if len(fi.Instants) != len(e.Instants) {
		t.Fatalf("%d instants stored, %d in the encounter", len(fi.Instants), len(e.Instants))
	}
	for i, in := range fi.Instants {
		want := e.Instants[i]
		if in.CaptureUnixNanos != want.CaptureUnixNanos || in.IntervalNanos != want.IntervalNanos ||
			in.RecordGap != want.RecordGap || in.Role != want.Role || in.Valid != want.Valid ||
			in.Reason != want.Reason || in.Condition != want.Condition ||
			in.FollowerSupport != want.FollowerSupport || in.LeaderSupport != want.LeaderSupport ||
			(in.Basis == BasisPredictedOnly) != want.Unobserved {
			t.Fatalf("instant %d: stored %+v, encounter %+v", i, in, want)
		}
		pt := want.Point
		if pt == nil {
			if in.Reasons != nil || in.LeaderTrailing != nil || in.FollowerLeading != nil || in.CoastAgeNanos != 0 {
				t.Fatalf("instant %d has evidence the encounter did not evaluate: %+v", i, in)
			}
			continue
		}
		if !reflect.DeepEqual(in.Reasons, pt.Reasons) {
			t.Errorf("instant %d reasons %v, point %v", i, in.Reasons, pt.Reasons)
		}
		if (pt.Leader == nil) != (in.LeaderTrailing == nil) || (pt.Leader != nil && *in.LeaderTrailing != pt.Leader.Trailing) ||
			(pt.Follower == nil) != (in.FollowerLeading == nil) || (pt.Follower != nil && *in.FollowerLeading != pt.Follower.Leading) {
			t.Errorf("instant %d endpoints differ from the point's", i)
		}
		for _, ep := range []*Endpoint{in.LeaderTrailing, in.FollowerLeading} {
			if ep != nil && (!ep.LengthProvenance.Valid() || !ep.WidthProvenance.Valid()) {
				t.Errorf("instant %d endpoint of %s lost its extent provenance", i, ep.TrackID)
			}
		}
	}
}

// A suppressed value is absent from what is stored, never a zero: checked on
// the raw JSON, which is what a reader of the database sees.
func TestSuppressedValuesStayAbsent(t *testing.T) {
	suppressed := 0
	for _, sc := range []EncounterScenario{ScenarioOcclusion(), ScenarioStandstillQueue(), ScenarioAmbiguousThenResolved(), provisionalScenario()} {
		_, fis := interactionsOf(t, sc)
		for _, fi := range fis {
			raw, _ := json.Marshal(fi.Event)
			var ev struct {
				Measurements map[string]map[string]json.RawMessage `json:"measurements"`
				Provisional  map[string]map[string]json.RawMessage `json:"provisional"`
			}
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatal(err)
			}
			for _, block := range []map[string]map[string]json.RawMessage{ev.Measurements, ev.Provisional} {
				for id, m := range block {
					if string(m["suppressed"]) != "true" {
						continue
					}
					suppressed++
					for _, k := range []string{"value", "uncertainty", "percentile", "opportunity_seconds"} {
						if _, ok := m[k]; ok {
							t.Errorf("%s: suppressed %s stores %s", sc.Name, id, k)
						}
					}
					if _, ok := m["reason"]; !ok {
						t.Errorf("%s: suppressed %s stores no reason", sc.Name, id)
					}
				}
			}
			for _, in := range fi.Instants {
				raw, _ := json.Marshal(in)
				if !in.Valid && strings.Contains(string(raw), `"`+string(MetricFollowingNetTimeGap)+`"`) {
					t.Errorf("%s: invalid instant %d stores a net time gap", sc.Name, in.CaptureUnixNanos)
				}
				if len(in.Values) == 0 && strings.Contains(string(raw), `"values"`) {
					t.Errorf("%s: instant %d stores an empty value block", sc.Name, in.CaptureUnixNanos)
				}
			}
		}
	}
	if suppressed == 0 {
		t.Fatal("no suppressed measurement was checked")
	}
	// Standstill: the gap stays, the time gap below the speed floor is absent.
	_, fis := interactionsOf(t, ScenarioStandstillQueue())
	var gapOnly int
	for _, in := range fis[0].Instants {
		_, gap := in.Values[MetricFollowingSpatialGap]
		_, thw := in.Values[MetricFollowingNetTimeGap]
		if gap && !thw && in.Reason == ReasonBelowSpeedFloor {
			gapOnly++
		}
	}
	if gapOnly == 0 {
		t.Fatal("no standstill instant kept its gap without a time gap")
	}
}

// Predicted-only time is stored, labelled and never observed opportunity:
// in the instants, in the windows, and in the one helper a denominator uses.
func TestPredictedOnlyIsNeverObservedOpportunity(t *testing.T) {
	_, fis := interactionsOf(t, ScenarioOcclusion())
	fi := fis[0]
	acc := fi.Event.Accounting
	if acc.PredictedOnlyNanos != nanosOf(5) || acc.ValidNanos != nanosOf(34) {
		t.Fatalf("accounting %+v", acc)
	}
	var observed, predicted int64
	for _, w := range fi.Windows {
		switch w.Basis {
		case BasisObserved:
			observed += w.DurationNanos
		case BasisPredictedOnly:
			predicted += w.DurationNanos
			if w.BandNanos != nil {
				t.Errorf("predicted window %s carries band time", w.WindowID)
			}
		}
	}
	// Frames 0-14 and 20-38 are valid, 15-19 predicted: three windows, the
	// predicted one between the two observed ones.
	if len(fi.Windows) != 3 || fi.Windows[1].Basis != BasisPredictedOnly ||
		fi.Windows[1].StartUnixNanos != fixtureAt(15) || fi.Windows[1].EndUnixNanos != fixtureAt(20) {
		t.Fatalf("windows %+v", fi.Windows)
	}
	if observed != acc.ValidNanos || predicted != acc.PredictedOnlyNanos || OpportunityNanos(fi.Windows) != acc.ValidNanos {
		t.Fatalf("observed %d predicted %d opportunity %d, accounting %+v", observed, predicted, OpportunityNanos(fi.Windows), acc)
	}
	var predictedInstants int
	for _, in := range fi.Instants {
		if in.Basis != BasisPredictedOnly {
			if _, ok := in.Values[MetricFollowingPredictedGap]; ok {
				t.Errorf("observed instant %d carries the predicted gap", in.CaptureUnixNanos)
			}
			continue
		}
		predictedInstants++
		if in.Valid || in.CoastAgeNanos <= 0 || len(in.Values) != 1 {
			t.Errorf("predicted instant %+v", in)
		}
		if in.FollowerLeading.Support != SupportCoasted {
			t.Errorf("predicted instant %d endpoint says %s", in.CaptureUnixNanos, in.FollowerLeading.Support)
		}
	}
	if predictedInstants != 5 {
		t.Fatalf("%d predicted instants, want 5", predictedInstants)
	}
	// Band time lives in observed windows and sums to the accounting.
	for _, band := range FollowingBands() {
		var sum int64
		for _, w := range fi.Windows {
			sum += w.BandNanos[band.Duration]
		}
		if sum != acc.BandNanos[band.Duration] {
			t.Errorf("%s: windows sum %d, accounting %d", band.Duration, sum, acc.BandNanos[band.Duration])
		}
	}
}

// A record gap stands for nothing and cuts the window it falls in; the
// windows still sum to valid time, and the worst support names the missing
// rows.
func TestRecordGapCutsWindows(t *testing.T) {
	sc := ScenarioSteadyApproach()
	f := &sc.Trajectories[0]
	f.Samples = append(append([]TrajectorySample(nil), f.Samples[:10]...), f.Samples[13:]...)
	_, fis := interactionsOf(t, sc)
	fi := fis[0]
	if len(fi.Windows) != 2 || fi.Windows[0].EndUnixNanos != fixtureAt(9) || fi.Windows[1].StartUnixNanos != fixtureAt(13) {
		t.Fatalf("windows %+v", fi.Windows)
	}
	if OpportunityNanos(fi.Windows) != fi.Event.Accounting.ValidNanos || fi.Event.Accounting.RecordGapNanos != nanosOf(4) {
		t.Fatalf("opportunity %d, accounting %+v", OpportunityNanos(fi.Windows), fi.Event.Accounting)
	}
	if fi.Event.WorstSupport != SupportMissedUnknown {
		t.Fatalf("worst support %s, want missed_unknown", fi.Event.WorstSupport)
	}
}

// A non-final encounter stores its stage, suppresses every production
// measurement and keeps its values only in the provisional block.
func TestProvisionalStageSurvives(t *testing.T) {
	_, fis := interactionsOf(t, provisionalScenario())
	fi := roundTrip(t, fis[0])
	ev := fi.Event
	if ev.Version.EstimateStage != StageFixedLag || ev.Version.InteractionVersion().EstimateStage != StageFixedLag {
		t.Fatalf("stage %s", ev.Version.EstimateStage)
	}
	for id, m := range ev.Measurements {
		if !m.Suppressed || m.Reason != ReasonEstimateNotFinal {
			t.Errorf("production %s: %+v", id, m)
		}
		p := ev.Provisional[id]
		if p.Suppressed || p.Value == nil {
			t.Errorf("provisional %s: %+v", id, p)
		}
	}
	// The instants keep the review values, labelled by the event's stage.
	if len(fi.NetTimeGapSeries()) != 50 {
		t.Fatalf("%d provisional net time gaps", len(fi.NetTimeGapSeries()))
	}

	dropped := fi
	dropped.Event.Provisional = nil
	if err := dropped.Validate(); err == nil || !strings.Contains(err.Error(), "provisional") {
		t.Fatalf("a non-final event without its provisional block validated: %v", err)
	}
	_, final := interactionsOf(t, ScenarioSteadyApproach())
	withBlock := final[0]
	withBlock.Event.Provisional = ev.Provisional
	if err := withBlock.Validate(); err == nil {
		t.Fatal("a final event with a provisional block validated")
	}
}

// Identity is a digest of source, type, pair and every version axis, so a
// regeneration under any new version is a new event; the same inputs give
// the same id.
func TestInteractionIdentityIsVersioned(t *testing.T) {
	_, fis := interactionsOf(t, ScenarioSteadyApproach())
	ev := fis[0].Event
	id := func(source string, v VersionProvenance) string {
		return InteractionEventID(source, ev.Type, ev.PrimaryTrackID, ev.SecondaryTrackID, v)
	}
	if id(ev.SourceID, ev.Version) != ev.EventID || !strings.HasPrefix(ev.EventID, "interaction/v1/") {
		t.Fatalf("id %s", ev.EventID)
	}
	seen := map[string]string{ev.EventID: "original"}
	for name, fn := range map[string]func(*VersionProvenance){
		"stage":      func(v *VersionProvenance) { v.EstimateStage = StageFixedLag },
		"estimator":  func(v *VersionProvenance) { v.EstimatorID += "_next" },
		"obs model":  func(v *VersionProvenance) { v.ObsModelID += "_next" },
		"method":     func(v *VersionProvenance) { v.MethodID += "_next" },
		"geometry":   func(v *VersionProvenance) { v.GeometryID += "_next" },
		"param hash": func(v *VersionProvenance) { v.ParamHash += "_next" },
	} {
		v := ev.Version
		fn(&v)
		got := id(ev.SourceID, v)
		if prev, dup := seen[got]; dup {
			t.Errorf("%s: id collides with %s", name, prev)
		}
		seen[got] = name
	}
	if id(ev.SourceID+"x", ev.Version) == ev.EventID {
		t.Error("source does not move the id")
	}
	if InteractionEventID(ev.SourceID, ev.Type, ev.SecondaryTrackID, ev.PrimaryTrackID, ev.Version) == ev.EventID {
		t.Error("swapping roles does not move the id")
	}
	// Length-prefixed parts: moving a boundary is a different identity.
	if identityDigest("ab", "c") == identityDigest("a", "bc") {
		t.Error("identity parts are ambiguous")
	}
	if len(identityDigest("x")) != 32 {
		t.Error("digest length")
	}
}

// Validate refuses a record set that disagrees with itself; each mutation is
// one way a stored summary could drift from the evidence under it.
func TestFollowingInteractionRejectsTampering(t *testing.T) {
	_, fis := interactionsOf(t, ScenarioOcclusion())
	base := fis[0]
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := func() FollowingInteraction { return roundTrip(t, base) }
	firstPredicted := -1
	for i, in := range base.Instants {
		if in.Basis == BasisPredictedOnly {
			firstPredicted = i
			break
		}
	}
	for name, mutate := range map[string]func(*FollowingInteraction){
		"schema":            func(f *FollowingInteraction) { f.Event.Schema = "interaction_record_v0" },
		"event id":          func(f *FollowingInteraction) { f.Event.EventID += "x" },
		"source":            func(f *FollowingInteraction) { f.Event.SourceID = "" },
		"same track twice":  func(f *FollowingInteraction) { f.Event.SecondaryTrackID = f.Event.PrimaryTrackID },
		"worst support":     func(f *FollowingInteraction) { f.Event.WorstSupport = SupportUnspecified },
		"valid time":        func(f *FollowingInteraction) { f.Event.Accounting.ValidNanos += 1 },
		"band over valid":   func(f *FollowingInteraction) { f.Event.Accounting.BandNanos[MetricFollowingTimeBelow2000ms] = 1 << 40 },
		"predicted time":    func(f *FollowingInteraction) { f.Event.Accounting.PredictedOnlyNanos = 0 },
		"suppression count": func(f *FollowingInteraction) { delete(f.Event.Accounting.Suppressions, ReasonNotObserved) },
		"instant dropped":   func(f *FollowingInteraction) { f.Instants = f.Instants[1:] },
		"window dropped":    func(f *FollowingInteraction) { f.Windows = f.Windows[1:] },
		"window basis flipped": func(f *FollowingInteraction) {
			f.Windows[1].Basis = BasisObserved
		},
		"measurement missing": func(f *FollowingInteraction) {
			delete(f.Event.Measurements, MetricFollowingValidTime)
		},
		"measurement rekeyed": func(f *FollowingInteraction) {
			m := f.Event.Measurements[MetricFollowingValidTime]
			m.Name = MetricFollowingSpatialGapMin
			f.Event.Measurements[MetricFollowingValidTime] = m
		},
		"measurement provenance": func(f *FollowingInteraction) {
			m := f.Event.Measurements[MetricFollowingValidTime]
			m.Provenance.Input.ObservedFrames++
			f.Event.Measurements[MetricFollowingValidTime] = m
		},
		"predicted instant made valid": func(f *FollowingInteraction) {
			f.Instants[firstPredicted].Valid, f.Instants[firstPredicted].Reason = true, ReasonUnspecified
		},
		"predicted gap on an observed instant": func(f *FollowingInteraction) {
			f.Instants[0].Values[MetricFollowingPredictedGap] = SeriesValue{Value: 15.75, Sigma: 0.3}
		},
		"observed gap on a predicted instant": func(f *FollowingInteraction) {
			f.Instants[firstPredicted].Values[MetricFollowingSpatialGap] = SeriesValue{Value: 15.75, Sigma: 0.3}
		},
		"basis disagrees with support": func(f *FollowingInteraction) {
			f.Instants[firstPredicted].FollowerSupport = SupportObserved
		},
		"reasons out of order": func(f *FollowingInteraction) {
			in := &f.Instants[firstPredicted]
			in.Reasons = append([]SuppressionReason{ReasonEstimateNotFinal}, in.Reasons...)
		},
		"endpoint of another track": func(f *FollowingInteraction) {
			f.Instants[0].LeaderTrailing.TrackID = f.Event.PrimaryTrackID
		},
		"endpoint provenance contradicts source": func(f *FollowingInteraction) {
			f.Instants[0].LeaderTrailing.LengthProvenance = ProvenanceClassPrior
		},
		"coast age on an observed instant": func(f *FollowingInteraction) { f.Instants[0].CoastAgeNanos = 1 },
		"instant of another event":         func(f *FollowingInteraction) { f.Instants[0].EventID = "interaction/v1/other" },
		"instants out of order": func(f *FollowingInteraction) {
			f.Instants[0], f.Instants[1] = f.Instants[1], f.Instants[0]
		},
	} {
		f := clone()
		mutate(&f)
		if err := f.Validate(); err == nil {
			t.Errorf("%s: tampered record set validated", name)
		}
	}
}

func TestExposureWindowValidate(t *testing.T) {
	_, fis := interactionsOf(t, ScenarioOcclusion())
	good := fis[0].Windows[0]
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ExposureWindow){
		"id":               func(w *ExposureWindow) { w.WindowID = "exposure/v1/x" },
		"no event":         func(w *ExposureWindow) { w.EventID = "" },
		"kind":             func(w *ExposureWindow) { w.Kind = ExposureKindUnspecified },
		"duration":         func(w *ExposureWindow) { w.DurationNanos++ },
		"one track":        func(w *ExposureWindow) { w.CounterpartTrackID = w.TrackID },
		"band over window": func(w *ExposureWindow) { w.BandNanos[MetricFollowingTimeBelow2000ms] = w.DurationNanos + 1 },
		"band missing":     func(w *ExposureWindow) { delete(w.BandNanos, MetricFollowingTimeBelow1000ms) },
		"version":          func(w *ExposureWindow) { w.Version.ParamHash = "" },
		"predicted with bands": func(w *ExposureWindow) {
			w.Basis = BasisPredictedOnly
			w.WindowID = ExposureWindowID(w.EventID, w.Kind, w.Basis, w.StartUnixNanos)
		},
	} {
		var w ExposureWindow
		raw, _ := json.Marshal(good)
		_ = json.Unmarshal(raw, &w)
		mutate(&w)
		if err := w.Validate(); err == nil {
			t.Errorf("%s: window validated", name)
		}
	}
	if OpportunityNanos(nil) != 0 {
		t.Fatal("no windows is no opportunity")
	}
}

func TestNewFollowingInteractionRejectsCallerErrors(t *testing.T) {
	a, _ := interactionsOf(t, ScenarioSteadyApproach())
	e := a.Encounters[0]
	if _, err := NewFollowingInteraction("", e); err == nil {
		t.Error("an interaction without a source was built")
	}
	empty := e
	empty.Measurements = nil
	if _, err := NewFollowingInteraction(testSourceID, empty); err == nil {
		t.Error("an encounter without measurements was mapped")
	}
	restaged := e
	restaged.Stage = StageFixedLag
	if _, err := NewFollowingInteraction(testSourceID, restaged); err == nil {
		t.Error("an encounter whose stage disagrees with its provenance was mapped")
	}
	dup := e
	dup.Measurements = append(append([]Measurement(nil), e.Measurements...), e.Measurements[0])
	if _, err := NewFollowingInteraction(testSourceID, dup); err == nil {
		t.Error("an encounter with a duplicated measurement was mapped")
	}
	unobserved := e
	unobserved.Instants = append([]EncounterInstant(nil), e.Instants...)
	unobserved.Instants[3].Unobserved = true
	if _, err := NewFollowingInteraction(testSourceID, unobserved); err == nil {
		t.Error("an instant whose unobserved flag contradicts its supports was mapped")
	}
}

func TestInteractionVersionValidate(t *testing.T) {
	_, fis := interactionsOf(t, ScenarioSteadyApproach())
	v := fis[0].Event.Version.InteractionVersion()
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	v.MethodID = ""
	if err := v.Validate(); err == nil {
		t.Fatal("a version without its method validated")
	}
}
