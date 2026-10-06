package l8behaviour

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// referencesFrom treats trajectories as the truth: each sample's body centre,
// heading and believed extent become an independent reference. For the
// analytic scenarios the beliefs are exact, so this is the ground truth.
func referencesFrom(trs []Trajectory) []ReferenceBody {
	var refs []ReferenceBody
	for _, tr := range trs {
		for _, s := range tr.Samples {
			x, y, _ := bodyLocation(s)
			refs = append(refs, ReferenceBody{
				TrackID: tr.Passage.TrackID, CaptureUnixNanos: s.CaptureUnixNanos, MotionClass: tr.Passage.MotionClass,
				CentreX: x, CentreY: y, HeadingRad: s.Heading.Rad, LengthM: s.Length.Metres, WidthM: s.Width.Metres,
			})
		}
	}
	return refs
}

// heldOutPlan is pinned to a set's id and content. Its unmatched, unscorable
// and encounter bounds admit everything; the tests of those bounds tighten
// them.
func heldOutPlan(set HeldOutSet) ScoringPlan {
	return ScoringPlan{
		ReferenceSetID: set.ReferenceSetID, ReferenceSetDigest: set.Digest(), NominalCoverage: 0.9,
		RangeEdgesM: []float64{30, 60}, AspectEdgesRad: []float64{math.Pi / 4, 3 * math.Pi / 4},
		Bounds: AcceptanceBounds{
			MaxEndpointP95AbsErrorM: 0.35, MaxGapP95AbsErrorM: 0.45,
			MinCoverage: 0.8, MaxCoverage: 0.97, MaxSuppressionRate: 0.2, MinCasesPerStratum: 5,
			MinEncountersPerStratum: 1, MaxUnmatchedRate: 1, MaxUnscorableRate: 1,
		},
	}
}

func score(t *testing.T, plan ScoringPlan, set HeldOutSet, estimates []EstimatedPair) HeldOutReport {
	t.Helper()
	r, err := ScoreHeldOut(plan.Hash(), plan, set, estimates)
	if err != nil {
		t.Fatalf("score: %v", err)
	}
	return r
}

func pairsOf(t *testing.T, trs []Trajectory, params FollowingAnalysisParams) []EstimatedPair {
	t.Helper()
	a, err := AnalyseFollowing(trs, params)
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := EstimatedPairsFromAnalysis(a, trs)
	if err != nil {
		t.Fatal(err)
	}
	return pairs
}

func stratumOf(t *testing.T, r HeldOutReport, s Stratum) StratumScore {
	t.Helper()
	for _, x := range r.Strata {
		if x.Stratum == s {
			return x
		}
	}
	t.Fatalf("no stratum %+v in %+v", s, r.Strata)
	return StratumScore{}
}

const (
	rearAspect  = "[0,0.7854)" // the face turned to a sensor behind the traffic
	frontAspect = "[2.356,pi]" // the face turned away from it
)

// TestHeldOutKnownPerturbations scores the steady approach with two known
// defects: the leader's positions 0.3 m too far along the road, and the
// follower's length believed 4.5 m against a true 4.0 m. Every leader rear is
// then 0.3 m long, every follower front 0.25 m long, and every gap 0.05 m
// long. At 90 % nominal coverage (z = 1.645) the endpoint sigma of 0.177 m
// covers 0.29 m, so the follower fronts are covered and the leader rears are
// not; the gap sigma of 0.25 m covers 0.41 m.
func TestHeldOutKnownPerturbations(t *testing.T) {
	truth := ScenarioSteadyApproach()
	estimate := ScenarioSteadyApproach()
	for i := range estimate.Trajectories[1].Samples {
		estimate.Trajectories[1].Samples[i].X += 0.3
	}
	for i := range estimate.Trajectories[0].Samples {
		estimate.Trajectories[0].Samples[i].Length.Metres = 4.5
	}
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(truth.Trajectories)}
	plan := heldOutPlan(set)
	r := score(t, plan, set, pairsOf(t, estimate.Trajectories, estimate.Params))
	if r.MethodID != HeldOutScoringMethodID || r.PlanHash != plan.Hash() || r.UnmatchedReferences != 0 ||
		r.UnscorableReferences != 0 || math.Abs(r.Z-1.6448536269514722) > 1e-12 {
		t.Fatalf("report header %+v", r)
	}
	// Leader rears: x = 44.25 + 0.75 k, so 21 frames under 60 m.
	// Follower fronts and gaps: x = 20 + k, so 10, 30 and 10 frames.
	for _, c := range []struct {
		kind, rng, aspect string
		cases             int
		bias, coverage    float64
		verdict           string
	}{
		{CaseKindEndpoint, "[30,60)", rearAspect, 21, 0.3, 0, VerdictFail},
		{CaseKindEndpoint, "[60,inf]", rearAspect, 29, 0.3, 0, VerdictFail},
		{CaseKindEndpoint, "[0,30)", frontAspect, 10, 0.25, 1, VerdictFail},
		{CaseKindEndpoint, "[30,60)", frontAspect, 30, 0.25, 1, VerdictFail},
		{CaseKindEndpoint, "[60,inf]", frontAspect, 10, 0.25, 1, VerdictFail},
		{CaseKindGap, "[0,30)", frontAspect, 10, 0.05, 1, VerdictFail},
		{CaseKindGap, "[30,60)", frontAspect, 30, 0.05, 1, VerdictFail},
		{CaseKindGap, "[60,inf]", frontAspect, 10, 0.05, 1, VerdictFail},
	} {
		s := stratumOf(t, r, Stratum{Kind: c.kind, Class: MotionRigidVehicle, Range: c.rng, Aspect: c.aspect, Support: SupportObserved})
		if s.Cases != c.cases || math.Abs(s.BiasM-c.bias) > 1e-9 || math.Abs(s.RMSErrorM-c.bias) > 1e-9 ||
			math.Abs(s.P95AbsErrorM-c.bias) > 1e-9 || s.Coverage != c.coverage || s.SuppressionRate != 0 || s.Verdict != c.verdict {
			t.Errorf("%s %s %s: %+v", c.kind, c.rng, c.aspect, s)
		}
	}
	if len(r.Strata) != 8 || r.Verdict != VerdictFail {
		t.Fatalf("%d strata, verdict %s", len(r.Strata), r.Verdict)
	}
	// Why each fails: the rears are overconfident; the fronts and gaps cover
	// every case, which is an interval too wide to inform at 90 % nominal.
	if rear := r.Strata[1]; !reflect.DeepEqual(rear.Failed, []string{"coverage_low"}) {
		t.Errorf("rear failed %v", rear.Failed)
	}
	if front := r.Strata[2]; !reflect.DeepEqual(front.Failed, []string{"coverage_high"}) {
		t.Errorf("front failed %v", front.Failed)
	}
}

// TestHeldOutCoverageMatchesNominalUnderStatedNoise: when the estimates' true
// errors are drawn from exactly the sigmas they state, empirical coverage
// converges on the nominal coverage, and bias on zero.
func TestHeldOutCoverageMatchesNominalUnderStatedNoise(t *testing.T) {
	rng := rand.New(rand.NewSource(8311))
	path := fixturePathX()
	const cases = 400
	var refs []ReferenceBody
	var pairs []EstimatedPair
	for k := int64(0); k < cases; k++ {
		at := fixtureAt(k)
		lTrue, fTrue := fixtureCar(at, 30, 0, 10), fixtureCar(at, 20, 0, 10).followerBody()
		l, f := lTrue, fTrue
		// Position sigma 0.125 m and length sigma 0.25 m are what the
		// fixture bodies state.
		l.x += 0.125 * rng.NormFloat64()
		f.x += 0.125 * rng.NormFloat64()
		l.length.Metres += 0.25 * rng.NormFloat64()
		f.length.Metres += 0.25 * rng.NormFloat64()
		lt, ft := fixtureTrajectory("trk_l", l), fixtureTrajectory("trk_f", f)
		refs = append(refs, referencesFrom([]Trajectory{fixtureTrajectory("trk_l", lTrue), fixtureTrajectory("trk_f", fTrue)})...)
		lp, _ := PartyAt(lt, at)
		fp, _ := PartyAt(ft, at)
		pt, err := EvaluateFollowing(path, lp, fp, FixtureParams())
		if err != nil {
			t.Fatal(err)
		}
		pairs = append(pairs, EstimatedPair{Path: path, Point: pt, LeaderSupport: SupportObserved, FollowerSupport: SupportObserved})
	}
	set := HeldOutSet{ReferenceSetID: "fixture/noise_v1", References: refs}
	plan := heldOutPlan(set)
	// An honest estimate's 95th percentile error is 1.96 sigma: 0.35 m for
	// an endpoint and 0.49 m for the gap, so the bounds sit above them.
	plan.Bounds.MaxEndpointP95AbsErrorM, plan.Bounds.MaxGapP95AbsErrorM = 0.45, 0.6
	r := score(t, plan, set, pairs)
	if len(r.Strata) != 3 {
		t.Fatalf("strata %+v", r.Strata)
	}
	// Binomial standard error of 0.9 over 400 cases is 0.015.
	for _, s := range r.Strata {
		if s.Cases != cases || math.Abs(s.Coverage-0.9) > 0.05 || math.Abs(s.BiasM) > 0.05 || s.Verdict != VerdictPass {
			t.Errorf("%s %s: %+v", s.Kind, s.Aspect, s)
		}
	}
	if r.Verdict != VerdictPass {
		t.Fatalf("verdict %s", r.Verdict)
	}
}

// TestHeldOutSuppressionUnmatchedAndUnscorable scores the occlusion scenario
// against exact references. Coasted instants still project bodies, so their
// endpoints are scored under their own support, while their gaps are
// suppressed and counted by reason. A reference no estimate matched, and one
// whose footprint leaves the path, are counted rather than scored.
func TestHeldOutSuppressionUnmatchedAndUnscorable(t *testing.T) {
	sc := ScenarioOcclusion()
	refs := referencesFrom(sc.Trajectories)
	refs = append(refs, ReferenceBody{
		TrackID: "trk_never_estimated", CaptureUnixNanos: fixtureAt(3), MotionClass: MotionRigidVehicle,
		CentreX: 60, LengthM: 4, WidthM: 2,
	})
	for i := range refs {
		if refs[i].TrackID == "trk_s_occlusion_follower" && refs[i].CaptureUnixNanos == fixtureAt(0) {
			refs[i].CentreX = 10 // before the path starts
		}
	}
	set := HeldOutSet{ReferenceSetID: "fixture/occlusion_v1", References: refs}
	plan := heldOutPlan(set)
	r := score(t, plan, set, pairsOf(t, sc.Trajectories, sc.Params))
	if r.UnmatchedReferences != 1 || r.UnscorableReferences != 1 {
		t.Fatalf("unmatched %d unscorable %d", r.UnmatchedReferences, r.UnscorableReferences)
	}
	coastedGap := stratumOf(t, r, Stratum{Kind: CaseKindGap, Class: MotionRigidVehicle, Range: "[30,60)", Aspect: frontAspect, Support: SupportCoasted})
	if coastedGap.Cases != 0 || coastedGap.SuppressionRate != 1 || coastedGap.Verdict != VerdictInsufficientCases ||
		!reflect.DeepEqual(coastedGap.Suppressed, []ReasonCount{{ReasonModelDegraded, 2}, {ReasonNotObserved, 3}}) {
		t.Fatalf("coasted gap stratum %+v", coastedGap)
	}
	coastedFront := stratumOf(t, r, Stratum{Kind: CaseKindEndpoint, Class: MotionRigidVehicle, Range: "[30,60)", Aspect: frontAspect, Support: SupportCoasted})
	if coastedFront.Cases != 5 || coastedFront.BiasM != 0 || coastedFront.Coverage != 1 {
		t.Fatalf("coasted front stratum %+v", coastedFront)
	}
	if r.Verdict != VerdictFail {
		t.Fatalf("verdict %s: exact estimates with honest sigmas over-cover", r.Verdict)
	}
}

// TestHeldOutScoresTheLocalTangentOnACurve: on the 150 m curve the reference
// endpoints come from corner projection onto the fitted path, independently
// of the closed form's local tangent, so the harness measures that
// approximation. It is real and signed: a body's inner corners project
// further along a curve than its tangent-line extent, by about half-length x
// half-width / radius (1.2 cm at the follower's front, 1.5 cm at the
// leader's rear), so the closed form overstates the gap by a couple of
// centimetres here, well inside its stated sigma.
func TestHeldOutScoresTheLocalTangentOnACurve(t *testing.T) {
	const r = 150.0
	follower := frames(30, func(k, t int64) bodySpec { return arcCar(t, r, 20+float64(k), 10).followerBody() })
	leader := frames(30, func(k, t int64) bodySpec { return arcCar(t, r, 40+float64(k), 10) })
	trs := []Trajectory{fixtureTrajectory("trk_curve_follower", follower...), fixtureTrajectory("trk_curve_leader", leader...)}
	set := HeldOutSet{ReferenceSetID: "fixture/curve_v1", References: referencesFrom(trs)}
	plan := heldOutPlan(set)
	plan.Bounds.MaxCoverage = 1
	rep := score(t, plan, set, pairsOf(t, trs, EncounterScenarioParams()))
	cases := 0
	for _, s := range rep.Strata {
		cases += s.Cases
		if s.P95AbsErrorM > 0.03 || s.Verdict != VerdictPass || (s.Kind == CaseKindGap && !(s.BiasM > 0)) {
			t.Errorf("%s %s %s: %+v", s.Kind, s.Range, s.Aspect, s)
		}
	}
	if cases != 90 || rep.UnscorableReferences != 0 || rep.Verdict != VerdictPass {
		t.Fatalf("%d cases, %d unscorable, verdict %s", cases, rep.UnscorableReferences, rep.Verdict)
	}
}

// TestHeldOutStratifiesByClass: a reference that says the leader is a
// two-wheeler puts its endpoints in their own stratum; the class comes from
// the reference, not the estimate.
func TestHeldOutStratifiesByClass(t *testing.T) {
	sc := ScenarioLaneAdjacentDistractor()
	refs := referencesFrom(sc.Trajectories)
	for i := range refs {
		if refs[i].TrackID == "trk_s_lane_leader" {
			refs[i].MotionClass = MotionTwoWheeler
		}
	}
	set := HeldOutSet{ReferenceSetID: "fixture/lane_v1", References: refs}
	r := score(t, heldOutPlan(set), set, pairsOf(t, sc.Trajectories, sc.Params))
	classes := map[MotionClass]int{}
	for _, s := range r.Strata {
		classes[s.Class] += s.Cases
	}
	if classes[MotionTwoWheeler] != 30 || classes[MotionRigidVehicle] != 60 {
		t.Fatalf("cases by class %v", classes)
	}
}

func TestHeldOutVerdicts(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)
	plan.Bounds.MaxCoverage = 1
	if r := score(t, plan, set, pairs); r.Verdict != VerdictPass {
		t.Fatalf("exact estimates under a coverage ceiling of 1: %s %+v", r.Verdict, r.Strata)
	}
	plan.Bounds.MinCasesPerStratum = 31
	if r := score(t, plan, set, pairs); r.Verdict != VerdictInsufficientCases {
		t.Fatalf("every stratum under 31 cases: %s", r.Verdict)
	}
	if r := score(t, plan, set, nil); r.Verdict != VerdictInsufficientCases || r.UnmatchedReferences != 100 {
		t.Fatalf("nothing scored: %+v", r)
	}
}

// TestHeldOutPinsThePlan: the plan is hashed and pinned before scoring, and a
// plan for another reference set is refused.
func TestHeldOutPinsThePlan(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)
	pinned := plan.Hash()
	loosened := plan
	loosened.Bounds.MaxGapP95AbsErrorM = 1
	if _, err := ScoreHeldOut(pinned, loosened, set, pairs); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("a bound loosened after pinning: err %v", err)
	}
	other := plan
	other.ReferenceSetID = "fixture/other"
	if _, err := ScoreHeldOut(other.Hash(), other, set, pairs); err == nil || !strings.Contains(err.Error(), "reference set") {
		t.Fatalf("a plan for another set: err %v", err)
	}
	for name, fn := range map[string]func(*ScoringPlan){
		"coverage":  func(p *ScoringPlan) { p.NominalCoverage = 0.95 },
		"range":     func(p *ScoringPlan) { p.RangeEdgesM = []float64{25, 60} },
		"aspect":    func(p *ScoringPlan) { p.AspectEdgesRad = []float64{1} },
		"min cases": func(p *ScoringPlan) { p.Bounds.MinCasesPerStratum = 6 },
	} {
		p := plan
		fn(&p)
		if p.Hash() == pinned {
			t.Errorf("%s: the hash did not move", name)
		}
	}
	a := score(t, plan, set, pairs)
	b := score(t, plan, set, pairs)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same scoring twice differs")
	}
}

func TestHeldOutRejectsMalformedInputs(t *testing.T) {
	sc := ScenarioSteadyApproach()
	refs := referencesFrom(sc.Trajectories)
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: refs}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)

	for name, fn := range map[string]func(*ScoringPlan){
		"no set":            func(p *ScoringPlan) { p.ReferenceSetID = "" },
		"coverage of one":   func(p *ScoringPlan) { p.NominalCoverage = 1 },
		"descending ranges": func(p *ScoringPlan) { p.RangeEdgesM = []float64{60, 30} },
		"aspect past pi":    func(p *ScoringPlan) { p.AspectEdgesRad = []float64{4} },
		"no endpoint bound": func(p *ScoringPlan) { p.Bounds.MaxEndpointP95AbsErrorM = 0 },
		"inverted coverage": func(p *ScoringPlan) { p.Bounds.MinCoverage, p.Bounds.MaxCoverage = 0.9, 0.8 },
		"no cases":          func(p *ScoringPlan) { p.Bounds.MinCasesPerStratum = 0 },
		"suppression over1": func(p *ScoringPlan) { p.Bounds.MaxSuppressionRate = 2 },
	} {
		p := plan
		fn(&p)
		if _, err := ScoreHeldOut(p.Hash(), p, set, pairs); err == nil {
			t.Errorf("plan %s: want an error", name)
		}
	}

	dup := set
	dup.References = append(append([]ReferenceBody(nil), refs...), refs[0])
	flat := set
	flat.References = append([]ReferenceBody(nil), refs...)
	flat.References[0].WidthM = 0
	noClass := set
	noClass.References = append([]ReferenceBody(nil), refs...)
	noClass.References[0].MotionClass = MotionClassUnspecified
	nanSensor := set
	nanSensor.SensorX = math.NaN()
	for name, s := range map[string]HeldOutSet{"duplicate": dup, "no width": flat, "no class": noClass, "NaN sensor": nanSensor} {
		if _, err := ScoreHeldOut(plan.Hash(), plan, s, pairs); err == nil {
			t.Errorf("set %s: want an error", name)
		}
	}

	noPath := append([]EstimatedPair(nil), pairs...)
	noPath[0].Path = nil
	wrongPath := append([]EstimatedPair(nil), pairs...)
	wrongPath[0].Path = fixturePathX()
	noSupport := append([]EstimatedPair(nil), pairs...)
	noSupport[0].LeaderSupport = SupportUnspecified
	for name, est := range map[string][]EstimatedPair{"no path": noPath, "wrong path": wrongPath, "no support": noSupport} {
		if _, err := ScoreHeldOut(plan.Hash(), plan, set, est); err == nil {
			t.Errorf("estimates %s: want an error", name)
		}
	}

	a, _ := AnalyseFollowing(sc.Trajectories, sc.Params)
	if _, err := EstimatedPairsFromAnalysis(a, sc.Trajectories[:1]); err == nil {
		t.Error("trajectories that do not match the analysis: want an error")
	}
}

// The plan pins the reference set's content as well as its name: a set with
// the same id and one annotation moved is refused, while listing the same
// references in another order is the same set.
func TestHeldOutPinsTheReferenceContent(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	plan := heldOutPlan(set)
	pairs := pairsOf(t, sc.Trajectories, sc.Params)

	reordered := set
	reordered.References = append([]ReferenceBody(nil), set.References...)
	for i, j := 0, len(reordered.References)-1; i < j; i, j = i+1, j-1 {
		reordered.References[i], reordered.References[j] = reordered.References[j], reordered.References[i]
	}
	if reordered.Digest() != set.Digest() {
		t.Fatal("the digest depends on the order references are listed in")
	}
	if r := score(t, plan, reordered, pairs); r.ReferenceSetDigest != set.Digest() || r.MethodID != "following_heldout_scoring_v2" {
		t.Fatalf("report header %+v", r)
	}

	edited := set
	edited.References = append([]ReferenceBody(nil), set.References...)
	edited.References[3].CentreX += 0.1
	if _, err := ScoreHeldOut(plan.Hash(), plan, edited, pairs); err == nil || !strings.Contains(err.Error(), "content digest") {
		t.Fatalf("an edited set under the same id: %v", err)
	}
	if _, err := ScoreHeldOut(plan.Hash(), plan, HeldOutSet{ReferenceSetID: set.ReferenceSetID}, pairs); err == nil {
		t.Fatal("an empty set was scored")
	}
}

// References no estimate reached count against the report: good scores on
// the references that were reached say nothing about the rest.
func TestHeldOutUnmatchedReferencesFailTheReport(t *testing.T) {
	sc := ScenarioSteadyApproach()
	refs := referencesFrom(sc.Trajectories)
	// A body the estimator never tracked: 50 references for a track with no
	// estimate, a third of the set.
	for _, r := range refs {
		if r.TrackID == refs[0].TrackID {
			r.TrackID = "trk_s_missed_entirely"
			refs = append(refs, r)
		}
	}
	set := HeldOutSet{ReferenceSetID: "fixture/steady_missed_v1", References: refs}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)
	plan.Bounds.MaxCoverage = 1 // the exact estimates' strata pass

	if r := score(t, plan, set, pairs); r.Verdict != VerdictPass || r.UnmatchedReferences != 50 ||
		math.Abs(r.UnmatchedRate-50.0/150) > 1e-12 {
		t.Fatalf("unbounded: %s, %d unmatched at %v", r.Verdict, r.UnmatchedReferences, r.UnmatchedRate)
	}
	plan.Bounds.MaxUnmatchedRate = 0.1
	r := score(t, plan, set, pairs)
	if r.Verdict != VerdictFail || !reflect.DeepEqual(r.Failed, []string{"unmatched_rate"}) {
		t.Fatalf("a third unmatched against a 10%% bound: %s %v", r.Verdict, r.Failed)
	}
	for _, s := range r.Strata {
		if s.Verdict != VerdictPass {
			t.Fatalf("stratum %+v: the failure is the report's, not a stratum's", s.Stratum)
		}
	}
}

// A stratum's evidence is counted in encounters as well as frames: thousands
// of correlated frames from one pair are one encounter.
func TestHeldOutCountsEncountersPerStratum(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)
	plan.Bounds.MaxCoverage = 1
	r := score(t, plan, set, pairs)
	for _, s := range r.Strata {
		if s.Cases > 0 && s.Encounters != 1 {
			t.Fatalf("stratum %+v: %d encounters from the scenario's one pair", s.Stratum, s.Encounters)
		}
	}
	plan.Bounds.MinEncountersPerStratum = 2
	if r := score(t, plan, set, pairs); r.Verdict != VerdictInsufficientCases {
		t.Fatalf("one pair against a two-encounter minimum: %s", r.Verdict)
	}
}

func TestHeldOutPlanRequiresTheV2Bounds(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	for name, mutate := range map[string]func(*ScoringPlan){
		"no digest":           func(p *ScoringPlan) { p.ReferenceSetDigest = "" },
		"no encounters":       func(p *ScoringPlan) { p.Bounds.MinEncountersPerStratum = 0 },
		"unmatched above one": func(p *ScoringPlan) { p.Bounds.MaxUnmatchedRate = 1.5 },
		"unscorable below 0":  func(p *ScoringPlan) { p.Bounds.MaxUnscorableRate = -0.1 },
	} {
		p := heldOutPlan(set)
		mutate(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: plan accepted", name)
		}
	}
}

// Estimates are validated: a non-finite endpoint or gap would sort first and
// shift the 95th percentile, and the same pair at the same instant twice
// would be counted twice.
func TestHeldOutRejectsNonFiniteAndDuplicateEstimates(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)

	nanArc := append([]EstimatedPair(nil), pairs...)
	leader := *nanArc[0].Point.Leader
	leader.Trailing.ArcM = math.NaN()
	nanArc[0].Point.Leader = &leader
	nanGap := append([]EstimatedPair(nil), pairs...)
	gap := *nanGap[0].Point.Gap
	gap.SigmaM = math.Inf(1)
	nanGap[0].Point.Gap = &gap
	twice := append(append([]EstimatedPair(nil), pairs...), pairs[0])
	for name, est := range map[string][]EstimatedPair{"NaN endpoint": nanArc, "infinite gap sigma": nanGap, "duplicate": twice} {
		if _, err := ScoreHeldOut(plan.Hash(), plan, set, est); err == nil {
			t.Errorf("estimates %s: want an error", name)
		}
	}
}

// A malformed reference is refused for what is wrong with it, before its
// digest is compared: a NaN cannot be encoded, so its digest is empty.
func TestHeldOutValidatesReferencesBeforeTheDigest(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)

	bad := set
	bad.References = append([]ReferenceBody(nil), set.References...)
	bad.References[3].CentreX = math.NaN()
	if bad.Digest() != "" {
		t.Error("a set holding a NaN has a digest")
	}
	_, err := ScoreHeldOut(plan.Hash(), plan, bad, pairs)
	if err == nil || !strings.Contains(err.Error(), "pose must be finite") {
		t.Fatalf("NaN reference: %v; want it refused as a non-finite pose", err)
	}
}

// The plan hash covers the method id, so a plan pinned for one version of
// the method is not accepted by another.
func TestHeldOutPlanHashCoversTheMethod(t *testing.T) {
	sc := ScenarioSteadyApproach()
	plan := heldOutPlan(HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)})
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if planOnly := hex.EncodeToString(sum[:])[:16]; plan.Hash() == planOnly {
		t.Fatal("plan hash is the plan alone, without the method id")
	}
}

// A stratum with too few cases is insufficient, not failed, but still names
// the bounds its point estimates break.
func TestHeldOutInsufficientStrataStillNameBrokenBounds(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)
	plan.Bounds.MinCasesPerStratum = 31                       // every stratum has fewer
	plan.Bounds.MinCoverage, plan.Bounds.MaxCoverage = 0, 0.5 // exact estimates are always covered
	r := score(t, plan, set, pairs)
	if r.Verdict != VerdictInsufficientCases {
		t.Fatalf("verdict %s, want insufficient", r.Verdict)
	}
	for _, s := range r.Strata {
		if s.Verdict != VerdictInsufficientCases || !reflect.DeepEqual(s.Failed, []string{"coverage_high"}) {
			t.Errorf("stratum %+v: verdict %s, failed %v; want insufficient naming coverage_high", s.Stratum, s.Verdict, s.Failed)
		}
	}
}

// An instant whose leader choice was suppressed is scored as a suppression
// in its stratum, once per follower instant however many leaders competed,
// and its references count as matched. Dropped instead, they would count as
// unmatched in the pooled share.
func TestHeldOutScoresUnevaluatedInstantsAsSuppressions(t *testing.T) {
	sc := ScenarioAmbiguousThenResolved()
	set := HeldOutSet{ReferenceSetID: "fixture/ambiguous_v1", References: referencesFrom(sc.Trajectories)}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)

	followerInstants := map[[2]string]bool{}
	var evaluated []EstimatedPair
	for _, p := range pairs {
		if p.NotEvaluated == ReasonAmbiguousLeader {
			followerInstants[[2]string{p.Point.FollowerTrackID, fmt.Sprint(p.Point.CaptureUnixNanos)}] = true
		} else {
			evaluated = append(evaluated, p)
		}
	}
	if len(followerInstants) == 0 {
		t.Fatal("the scenario has no ambiguous instant")
	}
	r := score(t, plan, set, pairs)
	gapSuppressed := 0
	for _, s := range r.Strata {
		if s.Kind != CaseKindGap {
			continue
		}
		for _, rc := range s.Suppressed {
			if rc.Reason == ReasonAmbiguousLeader {
				gapSuppressed += rc.Count
			}
		}
	}
	if gapSuppressed != len(followerInstants) {
		t.Errorf("%d ambiguous gap suppressions, want one per follower instant (%d)", gapSuppressed, len(followerInstants))
	}
	dropped := score(t, plan, set, evaluated)
	if dropped.UnmatchedReferences <= r.UnmatchedReferences {
		t.Errorf("dropping the unevaluated instants left %d unmatched, scoring them %d; want more when dropped",
			dropped.UnmatchedReferences, r.UnmatchedReferences)
	}
}

// Declaring hard instants unevaluated is no cheaper than suppressing them:
// the stratum's suppression rate carries them, and the pooled unmatched
// share does not move.
func TestHeldOutUnevaluatedInstantsCannotEscapeTheirStratum(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	plan := heldOutPlan(set)
	plan.Bounds.MaxCoverage = 1

	declared := append([]EstimatedPair(nil), pairs...)
	for i := 0; i < len(declared)/2; i++ {
		pt := declared[i].Point
		declared[i].Point = FollowingPoint{LeaderTrackID: pt.LeaderTrackID, FollowerTrackID: pt.FollowerTrackID, CaptureUnixNanos: pt.CaptureUnixNanos}
		declared[i].NotEvaluated = ReasonAmbiguousLeader
	}
	r := score(t, plan, set, declared)
	if r.UnmatchedReferences != 0 {
		t.Errorf("%d references unmatched; unevaluated instants still match theirs", r.UnmatchedReferences)
	}
	if r.Verdict != VerdictFail {
		t.Fatalf("half the instants declared unevaluated: verdict %s, want fail on suppression", r.Verdict)
	}
	failed := false
	for _, s := range r.Strata {
		for _, f := range s.Failed {
			failed = failed || f == "suppression_rate"
		}
	}
	if !failed {
		t.Errorf("no stratum failed its suppression bound: %+v", r.Strata)
	}

	// An unevaluated instant carrying an evaluation is refused.
	mixed := append([]EstimatedPair(nil), pairs...)
	mixed[0].NotEvaluated = ReasonAmbiguousLeader
	if _, err := ScoreHeldOut(plan.Hash(), plan, set, mixed); err == nil {
		t.Error("an unevaluated instant with an evaluation: want an error")
	}
}

// A plan that cannot be encoded (a non-finite number) has no hash, so no
// pinned hash can match it.
func TestHeldOutPlanHashIsEmptyWhenUnencodable(t *testing.T) {
	sc := ScenarioSteadyApproach()
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(sc.Trajectories)}
	plan := heldOutPlan(set)
	plan.NominalCoverage = math.NaN()
	if h := plan.Hash(); h != "" {
		t.Fatalf("hash %q of a plan holding a NaN, want empty", h)
	}
	if _, err := ScoreHeldOut("", plan, set, pairsOf(t, sc.Trajectories, sc.Params)); err == nil {
		t.Error("an unencodable plan pinned by an empty hash: want an error")
	}
}

// The share of matched references whose footprint left the path fails the
// report past its bound, for a leader's trailing extreme as for a
// follower's leading one.
func TestHeldOutUnscorableBoundFailsTheReport(t *testing.T) {
	sc := ScenarioOcclusion()
	refs := referencesFrom(sc.Trajectories)
	for i := range refs {
		if refs[i].CaptureUnixNanos == fixtureAt(0) &&
			(refs[i].TrackID == "trk_s_occlusion_follower" || refs[i].TrackID == "trk_s_occlusion_leader") {
			refs[i].CentreX = 10 // before the path starts
		}
	}
	set := HeldOutSet{ReferenceSetID: "fixture/occlusion_v1", References: refs}
	plan := heldOutPlan(set)
	pairs := pairsOf(t, sc.Trajectories, sc.Params)
	if r := score(t, plan, set, pairs); r.UnscorableReferences != 2 || containsString(r.Failed, "unscorable_rate") {
		t.Fatalf("unscorable %d, failed %v; want 2 within an admitting bound", r.UnscorableReferences, r.Failed)
	}
	plan.Bounds.MaxUnscorableRate = 0
	r := score(t, plan, set, pairs)
	if r.Verdict != VerdictFail || !containsString(r.Failed, "unscorable_rate") {
		t.Fatalf("verdict %s, failed %v; want the unscorable bound to fail the report", r.Verdict, r.Failed)
	}
}

// A 95th percentile past the bound for its kind fails the stratum.
func TestHeldOutP95BoundFailsAStratum(t *testing.T) {
	truth := ScenarioSteadyApproach()
	estimate := ScenarioSteadyApproach()
	for i := range estimate.Trajectories[1].Samples {
		estimate.Trajectories[1].Samples[i].X += 0.3 // every leader rear 0.3 m long
	}
	set := HeldOutSet{ReferenceSetID: "fixture/steady_v1", References: referencesFrom(truth.Trajectories)}
	plan := heldOutPlan(set)
	plan.Bounds.MaxEndpointP95AbsErrorM = 0.2
	r := score(t, plan, set, pairsOf(t, estimate.Trajectories, estimate.Params))
	rear := stratumOf(t, r, Stratum{Kind: CaseKindEndpoint, Class: MotionRigidVehicle, Range: "[30,60)", Aspect: rearAspect, Support: SupportObserved})
	if rear.Verdict != VerdictFail || !containsString(rear.Failed, "p95_abs_error") {
		t.Fatalf("rear stratum %+v; want it failed on p95_abs_error", rear)
	}
}

// Collecting estimates refuses an encounter whose path is missing, and an
// unevaluated instant that recorded no reason is taken as not observed.
func TestEstimatedPairsFromAnalysisEdges(t *testing.T) {
	sc := ScenarioSteadyApproach()
	a, err := AnalyseFollowing(sc.Trajectories, sc.Params)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Encounters) == 0 || len(a.Encounters[0].Instants) == 0 {
		t.Fatal("the scenario has no encounter")
	}
	noPaths := a
	noPaths.Paths = nil
	if _, err := EstimatedPairsFromAnalysis(noPaths, sc.Trajectories); err == nil {
		t.Error("an encounter with no path: want an error")
	}

	unreasoned := a
	unreasoned.Encounters = append([]Encounter(nil), a.Encounters...)
	unreasoned.Encounters[0].Instants = append([]EncounterInstant(nil), a.Encounters[0].Instants...)
	unreasoned.Encounters[0].Instants[0].Point = nil
	unreasoned.Encounters[0].Instants[0].Reason = ReasonUnspecified
	pairs, err := EstimatedPairsFromAnalysis(unreasoned, sc.Trajectories)
	if err != nil {
		t.Fatal(err)
	}
	if pairs[0].NotEvaluated != ReasonNotObserved {
		t.Errorf("unevaluated instant with no reason marked %v, want %v", pairs[0].NotEvaluated, ReasonNotObserved)
	}
}

func containsString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
