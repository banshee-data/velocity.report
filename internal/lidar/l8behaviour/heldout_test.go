package l8behaviour

import (
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

func heldOutPlan(setID string) ScoringPlan {
	return ScoringPlan{
		ReferenceSetID: setID, NominalCoverage: 0.9,
		RangeEdgesM: []float64{30, 60}, AspectEdgesRad: []float64{math.Pi / 4, 3 * math.Pi / 4},
		Bounds: AcceptanceBounds{
			MaxEndpointP95AbsErrorM: 0.35, MaxGapP95AbsErrorM: 0.45,
			MinCoverage: 0.8, MaxCoverage: 0.97, MaxSuppressionRate: 0.2, MinCasesPerStratum: 5,
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
	plan := heldOutPlan(set.ReferenceSetID)
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
	plan := heldOutPlan(set.ReferenceSetID)
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
	plan := heldOutPlan(set.ReferenceSetID)
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
	plan := heldOutPlan(set.ReferenceSetID)
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
	r := score(t, heldOutPlan(set.ReferenceSetID), set, pairsOf(t, sc.Trajectories, sc.Params))
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
	plan := heldOutPlan(set.ReferenceSetID)
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
	plan := heldOutPlan(set.ReferenceSetID)
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
	plan := heldOutPlan(set.ReferenceSetID)

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
