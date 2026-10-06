package l8behaviour

import (
	"encoding/json"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// z90 is the standard normal 95th percentile: the half-width, in sigmas, of a
// 90 % central interval.
var z90 = math.Sqrt2 * math.Erfinv(0.9)

// mcTolerance bounds the Monte Carlo quantile error for the scenarios' 2000
// draws: the 5 % quantile's standard error is about 0.047 sigma, so a fifth of
// a sigma is over four standard errors.
const mcTolerance = 0.2

func analyse(t *testing.T, sc EncounterScenario) FollowingAnalysis {
	t.Helper()
	a, err := AnalyseFollowing(sc.Trajectories, sc.Params)
	if err != nil {
		t.Fatalf("%s: analyse: %v", sc.Name, err)
	}
	return a
}

func sortedIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

func TestEncounterScenarios(t *testing.T) {
	for _, sc := range EncounterScenarios() {
		t.Run(sc.Name, func(t *testing.T) {
			a := analyse(t, sc)
			if a.MethodID != FollowingEncounterMethodID || a.ParamsHash != sc.Params.Hash() {
				t.Fatalf("method %s params %s", a.MethodID, a.ParamsHash)
			}
			checkPaths(t, a, sc)
			if len(a.Encounters) != len(sc.Encounters) {
				var got []string
				for _, e := range a.Encounters {
					got = append(got, e.LeaderTrackID+"->"+e.FollowerTrackID)
				}
				t.Fatalf("encounters %v, want %d", got, len(sc.Encounters))
			}
			for i, want := range sc.Encounters {
				checkEncounter(t, a, a.Encounters[i], want, sc.Params)
			}
			for _, want := range sc.Decisions {
				checkDecision(t, a, want)
			}
		})
	}
}

func checkPaths(t *testing.T, a FollowingAnalysis, sc EncounterScenario) {
	t.Helper()
	if len(a.Paths) != len(sc.Paths) || len(a.Timelines) != len(sc.Paths) {
		t.Fatalf("%d paths and %d timelines, want %d", len(a.Paths), len(a.Timelines), len(sc.Paths))
	}
	for i, want := range sc.Paths {
		got := a.Paths[i]
		if got.FollowerTrackID != want.FollowerTrackID || got.MethodID != LocalPathMethodID {
			t.Fatalf("path %d is %s (%s), want %s", i, got.FollowerTrackID, got.MethodID, want.FollowerTrackID)
		}
		if !reflect.DeepEqual(got.MemberTrackIDs, want.MemberTrackIDs) {
			t.Errorf("%s members %v, want %v", want.FollowerTrackID, got.MemberTrackIDs, want.MemberTrackIDs)
		}
		if !reflect.DeepEqual(got.Conditions, want.Conditions) {
			t.Errorf("%s conditions %v, want %v", want.FollowerTrackID, got.Conditions, want.Conditions)
		}
		if (got.Path == nil) != (len(want.Conditions) > 0) {
			t.Fatalf("%s: path %v with conditions %v", want.FollowerTrackID, got.Path != nil, got.Conditions)
		}
		tl := a.Timelines[i]
		ft, _ := trajectoryOf(sc, want.FollowerTrackID)
		if tl.FollowerTrackID != want.FollowerTrackID || len(tl.Decisions) != len(ft.Samples) {
			t.Fatalf("%s timeline has %d decisions for %d samples", tl.FollowerTrackID, len(tl.Decisions), len(ft.Samples))
		}
		if got.Path != nil {
			if err := got.Path.Validate(); err != nil {
				t.Fatalf("%s: %v", want.FollowerTrackID, err)
			}
			if tl.GeometryID != got.Path.ID || got.Path.Stage() != StageFinal {
				t.Errorf("%s: geometry %s stage %s", want.FollowerTrackID, tl.GeometryID, got.Path.Stage())
			}
			continue
		}
		// A refused path orders nothing: every instant is no_common_path
		// with the refusal's first condition, and belongs to no encounter.
		for _, d := range tl.Decisions {
			if d.Reason != ReasonNoCommonPath || d.Condition != want.Conditions[0] || d.LeaderTrackID != "" || d.Candidates != nil {
				t.Fatalf("%s refused path decision %+v", want.FollowerTrackID, d)
			}
		}
		for _, e := range a.Encounters {
			if e.FollowerTrackID == want.FollowerTrackID {
				t.Fatalf("%s has a refused path and an encounter with %s", e.FollowerTrackID, e.LeaderTrackID)
			}
		}
	}
}

func trajectoryOf(sc EncounterScenario, id string) (Trajectory, bool) {
	for _, tr := range sc.Trajectories {
		if tr.Passage.TrackID == id {
			return tr, true
		}
	}
	return Trajectory{}, false
}

func checkDecision(t *testing.T, a FollowingAnalysis, want ExpectedPairing) {
	t.Helper()
	tl, ok := a.Timeline(want.FollowerTrackID)
	if !ok {
		t.Fatalf("no timeline for %s", want.FollowerTrackID)
	}
	for _, d := range tl.Decisions {
		if d.CaptureUnixNanos != want.CaptureUnixNanos {
			continue
		}
		var ids []string
		for _, c := range d.Candidates {
			ids = append(ids, c.TrackID)
			if !c.Disposition.Valid() {
				t.Errorf("%s candidate %s has no disposition", want.FollowerTrackID, c.TrackID)
			}
		}
		if d.LeaderTrackID != want.LeaderTrackID || d.Reason != want.Reason || d.Condition != want.Condition ||
			!reflect.DeepEqual(ids, sortedIDs(want.CandidateTrackIDs)) {
			t.Errorf("%s at %d: leader %q reason %s condition %s candidates %v; want %q %s %s %v",
				want.FollowerTrackID, want.CaptureUnixNanos, d.LeaderTrackID, d.Reason, d.Condition, ids,
				want.LeaderTrackID, want.Reason, want.Condition, sortedIDs(want.CandidateTrackIDs))
		}
		return
	}
	t.Fatalf("%s has no decision at %d", want.FollowerTrackID, want.CaptureUnixNanos)
}

func checkEncounter(t *testing.T, a FollowingAnalysis, e Encounter, want ExpectedEncounter, params FollowingAnalysisParams) {
	t.Helper()
	what := want.LeaderTrackID + "->" + want.FollowerTrackID
	if e.LeaderTrackID != want.LeaderTrackID || e.FollowerTrackID != want.FollowerTrackID {
		t.Fatalf("encounter %s->%s, want %s", e.LeaderTrackID, e.FollowerTrackID, what)
	}
	path, _ := a.Path(want.FollowerTrackID)
	if e.GeometryID != path.Path.ID || e.Stage != StageFinal || e.Provisional != nil ||
		e.MethodID != FollowingEncounterMethodID+"/"+params.Hash() {
		t.Errorf("%s: geometry %s stage %s method %s provisional %v", what, e.GeometryID, e.Stage, e.MethodID, e.Provisional != nil)
	}
	if len(e.Instants) != want.Instants {
		t.Fatalf("%s: %d instants, want %d", what, len(e.Instants), want.Instants)
	}
	acc := e.Accounting
	if acc.ValidNanos != want.ValidNanos || !reflect.DeepEqual(acc.BandNanos, want.BandNanos) ||
		acc.UnobservedNanos != want.UnobservedNanos || acc.RecordGapNanos != 0 {
		t.Errorf("%s: valid %d bands %v unobserved %d gaps %d; want %d %v %d 0",
			what, acc.ValidNanos, acc.BandNanos, acc.UnobservedNanos, acc.RecordGapNanos,
			want.ValidNanos, want.BandNanos, want.UnobservedNanos)
	}
	if !reflect.DeepEqual(acc.Suppressions, want.Suppressions) {
		t.Errorf("%s: suppressions %+v, want %+v", what, acc.Suppressions, want.Suppressions)
	}
	// Every interval is accounted exactly once: valid or one reason.
	var total, accounted int64
	for i, inst := range e.Instants {
		if i > 0 && inst.CaptureUnixNanos <= e.Instants[i-1].CaptureUnixNanos {
			t.Fatalf("%s: instants out of order", what)
		}
		total += inst.IntervalNanos
		if inst.Valid == (inst.Reason != ReasonUnspecified) {
			t.Fatalf("%s at %d: valid %v with reason %s", what, inst.CaptureUnixNanos, inst.Valid, inst.Reason)
		}
	}
	accounted = acc.ValidNanos
	for _, s := range acc.Suppressions {
		accounted += s.Nanos
	}
	if accounted != total {
		t.Errorf("%s: accounted %d of %d ns", what, accounted, total)
	}
	if len(e.SpatialGapSeries) != want.SpatialGapSize || len(e.NetTimeGapSeries) != want.NetTimeGapSize ||
		len(e.PredictedGapSeries) != want.PredictedGaps {
		t.Errorf("%s: series %d/%d/%d, want %d/%d/%d", what, len(e.SpatialGapSeries), len(e.NetTimeGapSeries),
			len(e.PredictedGapSeries), want.SpatialGapSize, want.NetTimeGapSize, want.PredictedGaps)
	}
	for _, p := range e.SpatialGapSeries {
		if math.Abs(p.Sigma-want.GapSigma) > 1e-12 {
			t.Errorf("%s: gap sigma %.17g at %d, want %v", what, p.Sigma, p.CaptureUnixNanos, want.GapSigma)
		}
	}

	metrics := EncounterMetrics()
	if len(e.Measurements) != len(metrics) {
		t.Fatalf("%s: %d measurements", what, len(e.Measurements))
	}
	for i, m := range e.Measurements {
		if m.Name != metrics[i] {
			t.Fatalf("%s: measurement %d is %s, want %s", what, i, m.Name, metrics[i])
		}
		if err := m.Validate(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		v := m.Provenance.Version
		in := m.Provenance.Input
		if v.EstimateStage != StageFinal || v.GeometryID != e.GeometryID || v.MethodID != e.MethodID ||
			!reflect.DeepEqual(in.ContributingTrackIDs, []string{e.LeaderTrackID, e.FollowerTrackID}) ||
			in.FirstUnixNanos != e.FirstUnixNanos || in.LastUnixNanos != e.LastUnixNanos || !in.PlanarFallback {
			t.Errorf("%s %s provenance %+v", what, m.Name, m.Provenance)
		}
	}
	validSeconds := float64(want.ValidNanos) / 1e9
	if m, _ := e.Measurement(MetricFollowingValidTime); m.Suppressed || *m.Value != validSeconds ||
		m.Uncertainty.Kind != UncertaintyNone {
		t.Errorf("%s: valid time %+v, want %v s with no uncertainty", what, m, validSeconds)
	}

	stat := func(id MetricID, want *float64, sigma float64) {
		m, _ := e.Measurement(id)
		if want == nil {
			if !m.Suppressed || m.Reason != ReasonInsufficientObservation {
				t.Errorf("%s %s: want insufficient_observation, got %+v", what, id, m)
			}
			return
		}
		if m.Suppressed || math.Abs(*m.Value-*want) > 1e-12 {
			t.Errorf("%s %s = %v, want %v", what, id, m.Value, *want)
			return
		}
		u := m.Uncertainty
		if u.Kind != UncertaintyInterval || u.Method != MethodMonteCarlo || u.Samples != params.Exposure.MonteCarloSamples ||
			*u.Coverage != params.Exposure.IntervalCoverage {
			t.Errorf("%s %s uncertainty %+v", what, id, u)
		}
		if sigma > 0 {
			// Fully common-mode with a constant sigma: every draw shifts the
			// whole series together, so the interval is value -/+ z sigma.
			if math.Abs(*u.Lower-(*want-z90*sigma)) > mcTolerance*sigma ||
				math.Abs(*u.Upper-(*want+z90*sigma)) > mcTolerance*sigma {
				t.Errorf("%s %s interval [%v, %v], want %v -/+ %v", what, id, *u.Lower, *u.Upper, *want, z90*sigma)
			}
		}
	}
	stat(MetricFollowingSpatialGapMin, want.SpatialGapMin, want.GapSigma)
	stat(MetricFollowingSpatialGapP50, want.SpatialGapP50, want.GapSigma)
	thwSigma := 0.0
	if s := e.NetTimeGapSeries; len(s) > 0 && want.NetTimeGapMin != nil && *want.NetTimeGapMin == *want.NetTimeGapP50 {
		thwSigma = s[0].Sigma
	}
	stat(MetricFollowingNetTimeGapMin, want.NetTimeGapMin, thwSigma)
	stat(MetricFollowingNetTimeGapP50, want.NetTimeGapP50, thwSigma)

	for b, band := range FollowingBands() {
		for _, id := range []MetricID{band.Duration, band.Rate} {
			m, _ := e.Measurement(id)
			if validSeconds < params.Exposure.MinOpportunitySeconds {
				if !m.Suppressed || m.Reason != ReasonInsufficientObservation {
					t.Errorf("%s %s: below minimum opportunity, want insufficient_observation, got %+v", what, id, m)
				}
				continue
			}
			want := float64(want.BandNanos[b]) / 1e9
			if id == band.Rate {
				want = float64(e.Accounting.BandNanos[b]) / float64(e.Accounting.ValidNanos)
			}
			if m.Suppressed || *m.Value != want || m.OpportunitySeconds == nil || *m.OpportunitySeconds != validSeconds {
				t.Errorf("%s %s = %+v, want %v over %v s", what, id, m, want, validSeconds)
				continue
			}
			if u := m.Uncertainty; u.Kind != UncertaintyInterval || !(*u.Lower <= *u.Upper) {
				t.Errorf("%s %s uncertainty %+v", what, id, u)
			}
		}
	}
}

// TestSteadyApproachBandIntervals: with a fully common-mode error the band
// durations move together, so each band's interval brackets the point value
// and is wider for the band the series crosses most slowly.
func TestSteadyApproachBandIntervals(t *testing.T) {
	sc := ScenarioSteadyApproach()
	e := analyse(t, sc).Encounters[0]
	for _, band := range FollowingBands() {
		m, _ := e.Measurement(band.Duration)
		u := m.Uncertainty
		if !(*u.Lower <= *m.Value && *m.Value <= *u.Upper) || !(*u.Upper > *u.Lower) {
			t.Errorf("%s = %v in [%v, %v]", band.Duration, *m.Value, *u.Lower, *u.Upper)
		}
	}
}

// TestEncounterBenchmarksFollowTheRegistry: a band duration carries its
// registered no_established_threshold benchmark with the band as threshold
// and nothing else; the local_distribution metrics (rates, minima, medians)
// carry none, because their stratification belongs to whoever holds the
// comparison population, and valid time declares none at all.
func TestEncounterBenchmarksFollowTheRegistry(t *testing.T) {
	e := analyse(t, ScenarioSteadyApproach()).Encounters[0]
	bands := map[MetricID]float64{}
	for _, b := range FollowingBands() {
		bands[b.Duration] = b.Seconds
	}
	for _, m := range e.Measurements {
		def, _ := LookupMetric(m.Name)
		if seconds, ok := bands[m.Name]; ok {
			if m.Benchmark == nil || m.Benchmark.Kind != BenchmarkNoEstablishedThreshold ||
				*m.Benchmark.Threshold != seconds || m.Benchmark.Stratification != "" || m.Benchmark.Citation != "" {
				t.Errorf("%s benchmark %+v", m.Name, m.Benchmark)
			}
			continue
		}
		if m.Benchmark != nil {
			t.Errorf("%s (registered %s) carries benchmark %+v", m.Name, def.Benchmark, m.Benchmark)
		}
	}
}

// TestProvisionalEncounterIsReviewLabelled: the steady approach over
// fixed-lag estimates has the same arithmetic, but every production
// measurement is suppressed with estimate_not_final, the path and encounter
// say fixed_lag, and the values appear only in the Provisional block.
func TestProvisionalEncounterIsReviewLabelled(t *testing.T) {
	final := ScenarioSteadyApproach()
	provisional := ScenarioSteadyApproach()
	for i := range provisional.Trajectories {
		for j := range provisional.Trajectories[i].Samples {
			provisional.Trajectories[i].Samples[j].Stage = StageFixedLag
		}
	}
	fe := analyse(t, final).Encounters[0]
	a := analyse(t, provisional)
	pe := a.Encounters[0]
	if p, _ := a.Path(pe.FollowerTrackID); p.Path.Stage() != StageFixedLag || pe.Stage != StageFixedLag {
		t.Fatalf("path stage %s encounter stage %s", p.Path.Stage(), pe.Stage)
	}
	if !reflect.DeepEqual(pe.Accounting, fe.Accounting) || !reflect.DeepEqual(pe.NetTimeGapSeries, fe.NetTimeGapSeries) {
		t.Fatal("provisional arithmetic differs from final")
	}
	if len(pe.Provisional) != len(EncounterMetrics()) {
		t.Fatalf("%d provisional measurements", len(pe.Provisional))
	}
	for i, m := range pe.Measurements {
		if !m.Suppressed || m.Reason != ReasonEstimateNotFinal || m.Provenance.Version.EstimateStage != StageFixedLag {
			t.Errorf("%s: production %+v, want estimate_not_final", m.Name, m)
		}
		p := pe.Provisional[i]
		f := fe.Measurements[i]
		if p.Suppressed || p.Name != m.Name || *p.Value != *f.Value || p.Provenance.Version.EstimateStage != StageFixedLag {
			t.Errorf("%s: provisional %+v, final value %v", m.Name, p, *f.Value)
		}
	}
	// The pointwise points agree: supported opportunity, suppressed values.
	for _, inst := range pe.Instants {
		if !inst.Valid || inst.Point.NetTimeGap.Reason != ReasonEstimateNotFinal || inst.Point.TimeGap.ValueS == nil {
			t.Fatalf("provisional instant %+v", inst)
		}
	}
}

// TestAnalyseFollowingIsDeterministic: the same capture, supplied in any
// order, gives an identical analysis, down to the encoded bytes.
func TestAnalyseFollowingIsDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(20260925))
	for _, sc := range EncounterScenarios() {
		first := analyse(t, sc)
		shuffled := sc
		shuffled.Trajectories = append([]Trajectory(nil), sc.Trajectories...)
		rng.Shuffle(len(shuffled.Trajectories), func(i, j int) {
			shuffled.Trajectories[i], shuffled.Trajectories[j] = shuffled.Trajectories[j], shuffled.Trajectories[i]
		})
		second := analyse(t, shuffled)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("%s: analyses differ between runs", sc.Name)
		}
		a, err := json.Marshal(first)
		if err != nil {
			t.Fatalf("%s: marshal: %v", sc.Name, err)
		}
		b, _ := json.Marshal(second)
		if string(a) != string(b) {
			t.Fatalf("%s: encodings differ", sc.Name)
		}
	}
}

// TestCoastedTimeNeverEntersExposure checks the occlusion scenario's hidden
// frames instant by instant: review-only predicted gaps with growing coast
// age and sigma, and nothing in valid time, the series or the bands.
func TestCoastedTimeNeverEntersExposure(t *testing.T) {
	sc := ScenarioOcclusion()
	e := analyse(t, sc).Encounters[0]
	lastSigma := 0.0
	for i, p := range e.PredictedGapSeries {
		k := int64(15 + i)
		if p.CaptureUnixNanos != fixtureAt(k) || p.CoastAgeNanos != (k-14)*FixtureFramePeriodNanos ||
			p.ValueM != 15.75 || math.Abs(p.SigmaM-occlusionSigmas[i]) > 1e-15 || !(p.SigmaM > lastSigma) {
			t.Errorf("predicted %d: %+v", i, p)
		}
		lastSigma = p.SigmaM
	}
	for _, inst := range e.Instants {
		hidden := inst.CaptureUnixNanos >= fixtureAt(15) && inst.CaptureUnixNanos <= fixtureAt(19)
		if hidden != inst.Unobserved || (hidden && inst.Valid) {
			t.Errorf("instant %d: unobserved %v valid %v", inst.CaptureUnixNanos, inst.Unobserved, inst.Valid)
		}
	}
	for _, s := range append(append([]SeriesPoint(nil), e.SpatialGapSeries...), e.NetTimeGapSeries...) {
		if s.CaptureUnixNanos >= fixtureAt(15) && s.CaptureUnixNanos <= fixtureAt(19) {
			t.Fatalf("coasted instant %d entered a supported series", s.CaptureUnixNanos)
		}
	}
	// The path bridged the unobserved stretch rather than breaking there.
	p, _ := analyse(t, sc).Path(e.FollowerTrackID)
	var bridged []float64
	for _, k := range p.Path.Knots {
		if k.Bridged {
			bridged = append(bridged, k.X)
		}
	}
	if !reflect.DeepEqual(bridged, []float64{37, 39}) {
		t.Fatalf("bridged knots at %v, want x = 37 and 39", bridged)
	}
}

// TestRecordGapStandsForNothing: rows missing from the follower's record are
// a record gap, not time attributed to the instant before them.
func TestRecordGapStandsForNothing(t *testing.T) {
	sc := ScenarioSteadyApproach()
	f := &sc.Trajectories[0]
	f.Samples = append(append([]TrajectorySample(nil), f.Samples[:10]...), f.Samples[13:]...)
	e := analyse(t, sc).Encounters[0]
	acc := e.Accounting
	// Frame 9 now stands for the four frames to frame 13, over the 150 ms
	// bound; the frames it would have covered were all below 2.0 s.
	if acc.RecordGapNanos != nanosOf(4) || acc.ValidNanos != nanosOf(45) ||
		!reflect.DeepEqual(acc.BandNanos, []int64{nanosOf(44), nanosOf(28), nanosOf(8)}) {
		t.Fatalf("accounting %+v", acc)
	}
	if !e.Instants[9].RecordGap || e.Instants[9].IntervalNanos != nanosOf(4) || !e.Instants[9].Valid {
		t.Fatalf("instant 9 %+v", e.Instants[9])
	}
}

// TestMissingLeaderRowIsNotObserved: a leader present on both sides of an
// instant without a row there is still the leader, placed by interpolation,
// and the instant is not_observed rather than evaluated or re-paired.
func TestMissingLeaderRowIsNotObserved(t *testing.T) {
	sc := ScenarioSteadyApproach()
	l := &sc.Trajectories[1]
	l.Samples = append(append([]TrajectorySample(nil), l.Samples[:30]...), l.Samples[31:]...)
	a := analyse(t, sc)
	e := a.Encounters[0]
	inst := e.Instants[30]
	if inst.Reason != ReasonNotObserved || !inst.Unobserved || inst.Point != nil || inst.Role != DispositionLeader {
		t.Fatalf("instant 30 %+v", inst)
	}
	tl, _ := a.Timeline(e.FollowerTrackID)
	if c, _ := tl.Decisions[30].Candidate(e.LeaderTrackID); c.Synchronised || c.Disposition != DispositionLeader {
		t.Fatalf("candidate %+v", c)
	}
	acc := e.Accounting
	if acc.ValidNanos != nanosOf(48) || acc.UnobservedNanos != nanosOf(1) ||
		!reflect.DeepEqual(acc.Suppressions, []ReasonTally{{Reason: ReasonNotObserved, Instants: 1, Nanos: nanosOf(1)}}) ||
		!reflect.DeepEqual(acc.BandNanos, []int64{nanosOf(47), nanosOf(27), nanosOf(8)}) {
		t.Fatalf("accounting %+v", acc)
	}
}

// TestEncounterWorstSupport: each instant records both parties' support, and
// the encounter the worst of them, by evidence rather than declaration order.
func TestEncounterWorstSupport(t *testing.T) {
	steady := analyse(t, ScenarioSteadyApproach()).Encounters[0]
	if steady.WorstSupport != SupportObserved {
		t.Fatalf("steady approach worst support %s", steady.WorstSupport)
	}
	for _, inst := range steady.Instants {
		if inst.FollowerSupport != SupportObserved || inst.LeaderSupport != SupportObserved {
			t.Fatalf("instant %+v", inst)
		}
	}
	occluded := analyse(t, ScenarioOcclusion()).Encounters[0]
	if occluded.WorstSupport != SupportCoasted {
		t.Fatalf("occlusion worst support %s", occluded.WorstSupport)
	}
	for _, inst := range occluded.Instants {
		hidden := inst.CaptureUnixNanos >= fixtureAt(15) && inst.CaptureUnixNanos <= fixtureAt(19)
		if (inst.FollowerSupport == SupportCoasted) != hidden || inst.LeaderSupport != SupportObserved {
			t.Errorf("instant %d supports %s/%s", inst.CaptureUnixNanos, inst.FollowerSupport, inst.LeaderSupport)
		}
	}
	// A leader present without a row is an unexplained miss.
	sc := ScenarioSteadyApproach()
	l := &sc.Trajectories[1]
	l.Samples = append(append([]TrajectorySample(nil), l.Samples[:30]...), l.Samples[31:]...)
	missing := analyse(t, sc).Encounters[0]
	if missing.WorstSupport != SupportMissedUnknown || missing.Instants[30].LeaderSupport != SupportMissedUnknown {
		t.Fatalf("missing row: worst %s, instant 30 leader %s", missing.WorstSupport, missing.Instants[30].LeaderSupport)
	}
}

// TestUnsupportedClassSuppressesTheEncounter: a cyclist leader is a pair the
// following metrics are not defined for; the encounter still exists, with its
// suppression history, but every measurement is class_not_supported.
func TestUnsupportedClassSuppressesTheEncounter(t *testing.T) {
	sc := ScenarioSteadyApproach()
	sc.Trajectories[1].Passage.MotionClass = MotionTwoWheeler
	e := analyse(t, sc).Encounters[0]
	for _, m := range e.Measurements {
		if !m.Suppressed || m.Reason != ReasonClassNotSupported {
			t.Errorf("%s: %+v", m.Name, m)
		}
	}
	if e.Accounting.ValidNanos != 0 || e.Accounting.Suppression(ReasonClassNotSupported).Instants != 50 {
		t.Fatalf("accounting %+v", e.Accounting)
	}
}

// TestCommonModeFractionShapesTheMinimum: with independent errors the
// minimum of a long, constant series is biased low, as Section 9.3 warns, and
// its interval sits below the fully common-mode one.
func TestCommonModeFractionShapesTheMinimum(t *testing.T) {
	sc := ScenarioStandstillQueue()
	common := analyse(t, sc).Encounters[0]
	sc.Params.Exposure.CommonModeFraction = 0
	independent := analyse(t, sc).Encounters[0]
	c, _ := common.Measurement(MetricFollowingSpatialGapMin)
	i, _ := independent.Measurement(MetricFollowingSpatialGapMin)
	if *c.Value != *i.Value {
		t.Fatal("the point value does not depend on the error model")
	}
	cMid := (*c.Uncertainty.Lower + *c.Uncertainty.Upper) / 2
	iMid := (*i.Uncertainty.Lower + *i.Uncertainty.Upper) / 2
	if !(iMid < cMid-alignedGapSigma) || !(*i.Uncertainty.Upper < *c.Value) {
		t.Fatalf("independent [%v, %v] against common [%v, %v]", *i.Uncertainty.Lower, *i.Uncertainty.Upper,
			*c.Uncertainty.Lower, *c.Uncertainty.Upper)
	}
	// The median of many independent draws concentrates on the value.
	cp, _ := common.Measurement(MetricFollowingSpatialGapP50)
	ip, _ := independent.Measurement(MetricFollowingSpatialGapP50)
	if !(*ip.Uncertainty.Upper-*ip.Uncertainty.Lower < *cp.Uncertainty.Upper-*cp.Uncertainty.Lower) {
		t.Fatal("independent median interval is not narrower than the common-mode one")
	}
}

func TestAnalyseFollowingRejectsCallerErrors(t *testing.T) {
	sc := ScenarioSteadyApproach()
	dup := append([]Trajectory(nil), sc.Trajectories...)
	dup = append(dup, dup[0])
	mixed := append([]Trajectory(nil), sc.Trajectories...)
	mixed[1].Estimate.ParamHash = "other"
	invalid := append([]Trajectory(nil), sc.Trajectories...)
	invalid[0].Passage.SensorID = ""
	badParams := sc.Params
	badParams.Exposure.MonteCarloSamples = 10
	for _, c := range []struct {
		name   string
		trs    []Trajectory
		params FollowingAnalysisParams
		want   string
	}{
		{"duplicate track", dup, sc.Params, "twice"},
		{"mixed estimators", mixed, sc.Params, "estimator versions"},
		{"invalid trajectory", invalid, sc.Params, "sensor id"},
		{"invalid params", sc.Trajectories, badParams, "Monte Carlo"},
	} {
		if _, err := AnalyseFollowing(c.trs, c.params); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

func TestAnalysisParamsValidateAndHash(t *testing.T) {
	good := EncounterScenarioParams()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, fn := range map[string]func(*FollowingAnalysisParams){
		"no opportunity":      func(p *FollowingAnalysisParams) { p.Exposure.MinOpportunitySeconds = 0 },
		"no interval":         func(p *FollowingAnalysisParams) { p.Exposure.MaxIntervalNanos = 0 },
		"coverage of one":     func(p *FollowingAnalysisParams) { p.Exposure.IntervalCoverage = 1 },
		"too few draws":       func(p *FollowingAnalysisParams) { p.Exposure.MonteCarloSamples = 99 },
		"common mode over 1":  func(p *FollowingAnalysisParams) { p.Exposure.CommonModeFraction = 1.5 },
		"no pairing range":    func(p *FollowingAnalysisParams) { p.Pairing.MaxLeaderRangeM = 0 },
		"no following floor":  func(p *FollowingAnalysisParams) { p.Following.SpeedFloorMps = 0 },
		"no knot spacing":     func(p *FollowingAnalysisParams) { p.Path.KnotSpacingM = 0 },
		"NaN separation":      func(p *FollowingAnalysisParams) { p.Pairing.SeparationSigmas = math.NaN() },
		"extent under 2 knot": func(p *FollowingAnalysisParams) { p.Path.MinExtentM = 3 },
	} {
		p := good
		fn(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
		if p.Hash() == good.Hash() {
			t.Errorf("%s: the hash did not move", name)
		}
	}
	if good.Hash() != EncounterScenarioParams().Hash() || len(good.Hash()) != 16 {
		t.Fatalf("hash %q is not stable", good.Hash())
	}
}

// The predicted-gap review series reads the measurement it is gated on: its
// own value and sigma, and nothing for a suppressed or non-sigma measurement.
func TestSigmaMeasurement(t *testing.T) {
	m, err := NewMeasurement(MetricFollowingPredictedGap, 7.5, SigmaUncertainty(0.4, MethodLinearised), testProvenance())
	if err != nil {
		t.Fatal(err)
	}
	if v, s, ok := sigmaMeasurement(&m); !ok || v != 7.5 || s != 0.4 {
		t.Fatalf("sigmaMeasurement = %v, %v, %v", v, s, ok)
	}
	suppressed, err := NewSuppressedMeasurement(MetricFollowingPredictedGap, ReasonNotObserved, testProvenance())
	if err != nil {
		t.Fatal(err)
	}
	bounds := m
	bounds.Uncertainty = &Uncertainty{Kind: UncertaintyBounds, Lower: ptr(7.0)}
	for name, c := range map[string]*Measurement{"nil": nil, "suppressed": &suppressed, "bounds": &bounds} {
		if _, _, ok := sigmaMeasurement(c); ok {
			t.Errorf("%s: want no value", name)
		}
	}
}
