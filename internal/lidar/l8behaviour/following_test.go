package l8behaviour

import (
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// alignedParties returns the aligned fixture's leader and follower, ready to
// be mutated by a test.
func alignedParties(t *testing.T) (StraightPath, Party, Party) {
	t.Helper()
	f := FixtureAlignedPair()
	l, _ := f.Trajectory("trk_fixture_aligned_leader")
	fo, _ := f.Trajectory("trk_fixture_aligned_follower")
	leader, _ := PartyAt(l, f.Expected[0].CaptureUnixNanos)
	follower, _ := PartyAt(fo, f.Expected[0].CaptureUnixNanos)
	return f.Path, leader, follower
}

func mustEvaluate(t *testing.T, path PathFrame, leader, follower Party) FollowingPoint {
	t.Helper()
	pt, err := EvaluateFollowing(path, leader, follower, FixtureParams())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return pt
}

// TestProjectBodyMatchesCornerBruteForce checks the closed-form extremity
// against the footprint's four corners projected onto the path, over random
// bodies, headings, anchors and paths.
func TestProjectBodyMatchesCornerBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(20260925))
	for i := 0; i < 500; i++ {
		path := StraightPath{
			ID: "random", OriginX: rng.Float64()*20 - 10, OriginY: rng.Float64()*20 - 10,
			HeadingRad: rng.Float64()*2*math.Pi - math.Pi, LengthM: 1000,
		}
		L, W := 1+rng.Float64()*10, 0.5+rng.Float64()*2
		psi := path.HeadingRad + rng.Float64()*2*math.Pi - math.Pi
		o := BodyOffset{LongitudinalM: rng.Float64()*4 - 2, LateralM: rng.Float64()*2 - 1}
		cxArc, cxLat := 50+rng.Float64()*100, rng.Float64()*4-2
		cx, cy := path.PointAt(cxArc, cxLat)
		// Place the anchor so that anchor + R(psi) o is the chosen centre.
		ax := cx - (o.LongitudinalM*math.Cos(psi) - o.LateralM*math.Sin(psi))
		ay := cy - (o.LongitudinalM*math.Sin(psi) + o.LateralM*math.Cos(psi))
		s := fixtureCar(FixtureBaseUnixNanos, ax, ay, 0).sample()
		s.Reference, s.AnchorToCentre = ReferenceNearFaceCentre, o
		s.Heading.Rad = psi
		s.Length.Metres, s.Width.Metres = L, W

		body, reason, err := ProjectBody(path, "trk_random", s)
		if err != nil || reason != ReasonUnspecified {
			t.Fatalf("case %d: reason %s err %v", i, reason, err)
		}
		lead, trail := math.Inf(-1), math.Inf(1)
		for _, a := range []float64{L / 2, -L / 2} {
			for _, b := range []float64{W / 2, -W / 2} {
				x := cx + a*math.Cos(psi) - b*math.Sin(psi)
				y := cy + a*math.Sin(psi) + b*math.Cos(psi)
				loc, _ := path.Locate(x, y)
				lead, trail = math.Max(lead, loc.ArcM), math.Min(trail, loc.ArcM)
			}
		}
		if math.Abs(body.Leading.ArcM-lead) > 1e-9 || math.Abs(body.Trailing.ArcM-trail) > 1e-9 {
			t.Fatalf("case %d: leading/trailing %v/%v, corners give %v/%v", i, body.Leading.ArcM, body.Trailing.ArcM, lead, trail)
		}
		if math.Abs(body.CentreArcM-cxArc) > 1e-9 || math.Abs(body.LateralM-cxLat) > 1e-9 {
			t.Fatalf("case %d: centre %v/%v, want %v/%v", i, body.CentreArcM, body.LateralM, cxArc, cxLat)
		}
	}
}

func TestProjectBodyRefusesWhatItCannotName(t *testing.T) {
	path, _, follower := alignedParties(t)
	base := follower.Sample

	medoid := base
	medoid.Reference, medoid.Estimation = ReferenceClusterMedoid, EstimationInitialising
	unresolved := base
	unresolved.Estimation, unresolved.Faces = EstimationGeometryConverging, FaceVisibility{}
	unresolved.Heading.AmbiguousModeWeight = 0.5
	noWidth := base
	noWidth.Estimation, noWidth.Width = EstimationGeometryConverging, ExtentBelief{}
	offPath := base
	offPath.X = 500

	for _, c := range []struct {
		name   string
		s      TrajectorySample
		reason SuppressionReason
	}{
		{"medoid reference", medoid, ReasonInsufficientObservation},
		{"unresolved front and rear", unresolved, ReasonOrientationUnresolved},
		{"no width belief", noWidth, ReasonExtentNotConverged},
		{"off the path", offPath, ReasonNoCommonPath},
	} {
		body, reason, err := ProjectBody(path, "trk", c.s)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if reason != c.reason || !reflect.DeepEqual(body, BodyOnPath{}) {
			t.Errorf("%s: reason %s, body %+v; want %s and no body", c.name, reason, body, c.reason)
		}
	}

	invalid := base
	invalid.StateModel = "cv_polar_v9"
	for _, c := range []struct {
		name string
		path PathFrame
		id   string
		s    TrajectorySample
	}{
		{"nil path", nil, "trk", base},
		{"no track id", path, "", base},
		{"invalid sample", path, "trk", invalid},
	} {
		if _, _, err := ProjectBody(c.path, c.id, c.s); err == nil {
			t.Errorf("%s: want an error", c.name)
		}
	}
}

func TestProjectBodyFacingAgainstThePathLeadsWithItsRear(t *testing.T) {
	path, _, follower := alignedParties(t)
	s := follower.Sample
	s.Heading.Rad = math.Pi // reversing along the path
	body, reason, err := ProjectBody(path, "trk", s)
	if err != nil || reason != ReasonUnspecified {
		t.Fatalf("reason %s err %v", reason, err)
	}
	// The front face was seen, and it is now the trailing extreme.
	if body.Leading.Source != EndpointTemporallyInferred || body.Trailing.Source != EndpointDirectlyObserved {
		t.Fatalf("sources leading %s trailing %s", body.Leading.Source, body.Trailing.Source)
	}
	if body.AlongSpeedMps != s.VX {
		t.Fatalf("along-path speed %v, want %v", body.AlongSpeedMps, s.VX)
	}
}

func TestEndpointSourceIsDerivedNotDeclared(t *testing.T) {
	prior := ExtentBelief{Metres: 4.5, SigmaMetres: 1.5, Provenance: ProvenanceClassPrior}
	evidence := converged(4.0, 0.25)
	for _, c := range []struct {
		seen          bool
		length, width ExtentBelief
		want          EndpointSource
	}{
		{true, prior, prior, EndpointDirectlyObserved},
		{false, evidence, evidence, EndpointTemporallyInferred},
		{false, prior, evidence, EndpointPriorDominated},
		{false, evidence, prior, EndpointPriorDominated},
	} {
		if got := endpointSource(c.seen, c.length, c.width); got != c.want {
			t.Errorf("seen=%v L=%s W=%s: %s, want %s", c.seen, c.length.Provenance, c.width.Provenance, got, c.want)
		}
	}
}

func endpointPair() (Endpoint, Endpoint) {
	leader := Endpoint{
		TrackID: "L", CaptureUnixNanos: 1, Extremity: ExtremityTrailing, ArcM: 30, SigmaM: 0.3,
		Source: EndpointTemporallyInferred, Support: SupportObserved, ExtentConverged: true,
	}
	follower := Endpoint{
		TrackID: "F", CaptureUnixNanos: 1, Extremity: ExtremityLeading, ArcM: 26, SigmaM: 0.4,
		Source: EndpointDirectlyObserved, Support: SupportObserved, ExtentConverged: true,
	}
	return leader, follower
}

func TestSpatialGapReasonsInPrecedence(t *testing.T) {
	l, f := endpointPair()
	g, err := SpatialGap(l, f)
	if err != nil || g.Reason != ReasonUnspecified || g.ValueM != 4 || g.SigmaM != 0.5 {
		t.Fatalf("gap %+v err %v", g, err)
	}
	if g.Source() != EndpointTemporallyInferred {
		t.Fatalf("pair source %s, want the weaker end", g.Source())
	}

	coasted := f
	coasted.Support, coasted.Source = SupportCoasted, EndpointTemporallyInferred
	unconverged := l
	unconverged.ExtentConverged = false
	overlap := f
	overlap.ArcM = 30
	behind := f
	behind.ArcM = 31
	both := coasted
	both.ArcM, both.ExtentConverged = 31, false

	for _, c := range []struct {
		name     string
		l, f     Endpoint
		reason   SuppressionReason
		wantValM float64
	}{
		{"coasted follower", l, coasted, ReasonNotObserved, 4},
		{"unconverged leader", unconverged, f, ReasonExtentNotConverged, 4},
		{"touching", l, overlap, ReasonNonPositiveGap, 0},
		{"overlapping", l, behind, ReasonNonPositiveGap, -1},
		{"not observed outranks the rest", l, both, ReasonNotObserved, -1},
	} {
		g, err := SpatialGap(c.l, c.f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		// The arithmetic is retained for review even when suppressed.
		if g.Reason != c.reason || g.ValueM != c.wantValM {
			t.Errorf("%s: reason %s value %v, want %s and %v", c.name, g.Reason, g.ValueM, c.reason, c.wantValM)
		}
		m, err := g.Measurement(MetricFollowingSpatialGap, testProvenance())
		if err != nil || !m.Suppressed || m.Value != nil || m.Reason != c.reason {
			t.Errorf("%s: measurement %+v err %v", c.name, m, err)
		}
	}
}

func TestSpatialGapRejectsMalformedEndpoints(t *testing.T) {
	l, f := endpointPair()
	mutate := func(fn func(*Endpoint, *Endpoint)) (Endpoint, Endpoint) {
		a, b := l, f
		fn(&a, &b)
		return a, b
	}
	for name, fn := range map[string]func(*Endpoint, *Endpoint){
		"swapped extremities": func(a, b *Endpoint) { a.Extremity, b.Extremity = b.Extremity, a.Extremity },
		"same track":          func(a, b *Endpoint) { b.TrackID = a.TrackID },
		"unsynchronised":      func(a, b *Endpoint) { b.CaptureUnixNanos++ },
		"no track id":         func(a, b *Endpoint) { a.TrackID = "" },
		"NaN arc":             func(a, b *Endpoint) { a.ArcM = math.NaN() },
		"negative sigma":      func(a, b *Endpoint) { b.SigmaM = -1 },
		"no source":           func(a, b *Endpoint) { b.Source = EndpointSourceUnspecified },
		"no support":          func(a, b *Endpoint) { a.Support = SupportUnspecified },
		"converged prior":     func(a, b *Endpoint) { a.Source = EndpointPriorDominated },
		"observed while coasting": func(a, b *Endpoint) {
			b.Support = SupportCoasted
		},
	} {
		a, b := mutate(fn)
		if _, err := SpatialGap(a, b); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestNetTimeGap(t *testing.T) {
	gap := GapEstimate{ValueM: 6, SigmaM: 0.3}
	thw, err := NetTimeGap(gap, 8, 0.4, 0.5)
	if err != nil || thw.Reason != ReasonUnspecified {
		t.Fatalf("thw %+v err %v", thw, err)
	}
	wantSigma := 0.75 * math.Sqrt(sq(0.3/6)+sq(0.4/8))
	if *thw.ValueS != 0.75 || math.Abs(*thw.SigmaS-wantSigma) > 1e-15 {
		t.Fatalf("thw %v +- %v, want 0.75 +- %v", *thw.ValueS, *thw.SigmaS, wantSigma)
	}
	// At the floor exactly the time gap is defined; below it, it is not.
	if thw, _ := NetTimeGap(gap, 0.5, 0.1, 0.5); thw.ValueS == nil {
		t.Fatal("a follower at the floor has a time gap")
	}
	for _, speed := range []float64{0.49, 0, -3} {
		thw, err := NetTimeGap(gap, speed, 0.1, 0.5)
		if err != nil || thw.Reason != ReasonBelowSpeedFloor || thw.ValueS != nil || thw.SigmaS != nil {
			t.Errorf("speed %v: %+v err %v; want below_speed_floor and no value", speed, thw, err)
		}
	}
	suppressedGap := GapEstimate{ValueM: -1, SigmaM: 0.3, Reason: ReasonNonPositiveGap}
	if thw, err := NetTimeGap(suppressedGap, 8, 0.4, 0.5); err != nil || thw.Reason != ReasonNonPositiveGap || thw.ValueS != nil {
		t.Fatalf("a suppressed gap passes its reason through: %+v err %v", thw, err)
	}
	for name, args := range map[string][4]float64{
		"zero floor":          {6, 8, 0.4, 0},
		"infinite floor":      {6, 8, 0.4, math.Inf(1)},
		"NaN speed":           {6, math.NaN(), 0.4, 0.5},
		"negative sigma":      {6, 8, -0.4, 0.5},
		"NaN gap":             {math.NaN(), 8, 0.4, 0.5},
		"non-positive at all": {-1, 8, 0.4, 0.5},
	} {
		if _, err := NetTimeGap(GapEstimate{ValueM: args[0], SigmaM: 0.3}, args[1], args[2], args[3]); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestObservedSurfaceGap(t *testing.T) {
	leader := ObservedSurface{TrackID: "L", CaptureUnixNanos: 5, Extremity: ExtremityTrailing, ArcM: 20, SigmaM: 0.03, PointCount: 40}
	follower := ObservedSurface{TrackID: "F", CaptureUnixNanos: 5, Extremity: ExtremityLeading, ArcM: 15, SigmaM: 0.04, PointCount: 25}
	g, err := ObservedSurfaceGap(leader, follower)
	if err != nil || g.ValueM != 5 || math.Abs(g.SigmaM-0.05) > 1e-15 || g.Reason != ReasonUnspecified {
		t.Fatalf("gap %+v err %v", g, err)
	}
	if g.Source() != EndpointDirectlyObserved {
		t.Fatalf("observed surfaces are directly observed, got %s", g.Source())
	}
	m, err := g.Measurement(MetricFollowingObservedSurfaceGap, testProvenance())
	if err != nil || m.Suppressed || *m.Value != 5 {
		t.Fatalf("measurement %+v err %v", m, err)
	}
	if def, _ := LookupMetric(MetricFollowingObservedSurfaceGap); def.Visibility.EntersDistribution() {
		t.Fatal("observed_surface_gap must never enter the published distribution")
	}

	overlap := follower
	overlap.ArcM = 21
	if g, _ := ObservedSurfaceGap(leader, overlap); g.Reason != ReasonNonPositiveGap {
		t.Fatalf("overlapping surfaces: reason %s", g.Reason)
	}
	for name, fn := range map[string]func(l, f *ObservedSurface){
		"no returns":         func(l, f *ObservedSurface) { f.PointCount = 0 },
		"wrong extremity":    func(l, f *ObservedSurface) { l.Extremity = ExtremityLeading },
		"no track":           func(l, f *ObservedSurface) { l.TrackID = "" },
		"same track":         func(l, f *ObservedSurface) { f.TrackID = "L" },
		"unsynchronised":     func(l, f *ObservedSurface) { f.CaptureUnixNanos = 6 },
		"infinite arc":       func(l, f *ObservedSurface) { l.ArcM = math.Inf(1) },
		"negative sigma bar": func(l, f *ObservedSurface) { f.SigmaM = -0.1 },
	} {
		l, f := leader, follower
		fn(&l, &f)
		if _, err := ObservedSurfaceGap(l, f); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestEvaluateFollowingPrecedence(t *testing.T) {
	path, leader, follower := alignedParties(t)

	cyclist := leader
	cyclist.Passage.MotionClass = MotionTwoWheeler
	onlineLeader := leader
	onlineLeader.Sample.Stage = StageOnline
	invalidModel := leader
	invalidModel.Sample.Estimation = EstimationModelInvalid
	adjacent := leader
	adjacent.Sample.Y = 3.5
	yawed := follower
	yawed.Sample.Heading.Rad = 0.5
	crawling := follower
	crawling.Sample.VX = 0.25
	crawlingOnline := crawling
	crawlingOnline.Sample.Stage = StageFixedLag

	for _, c := range []struct {
		name             string
		leader, follower Party
		gap, thw         SuppressionReason
		reasons          []SuppressionReason
		opportunity      bool
	}{
		{"supported", leader, follower, 0, 0, nil, true},
		{"class first", cyclist, onlineLeaderFollower(follower), ReasonClassNotSupported, ReasonClassNotSupported,
			[]SuppressionReason{ReasonClassNotSupported, ReasonEstimateNotFinal}, false},
		{"stage last and not an opportunity loss", onlineLeader, follower, ReasonEstimateNotFinal, ReasonEstimateNotFinal,
			[]SuppressionReason{ReasonEstimateNotFinal}, true},
		{"model invalid", invalidModel, follower, ReasonModelDegraded, ReasonModelDegraded,
			[]SuppressionReason{ReasonModelDegraded}, false},
		{"lane adjacent", adjacent, follower, ReasonNoCommonPath, ReasonNoCommonPath,
			[]SuppressionReason{ReasonNoCommonPath}, false},
		{"yawed beyond the corridor heading", leader, yawed, ReasonNoCommonPath, ReasonNoCommonPath,
			[]SuppressionReason{ReasonNoCommonPath}, false},
		{"gap valid, time gap below floor", leader, crawling, 0, ReasonBelowSpeedFloor,
			[]SuppressionReason{ReasonBelowSpeedFloor}, false},
		{"floor outranks stage for the time gap", leader, crawlingOnline, ReasonEstimateNotFinal, ReasonBelowSpeedFloor,
			[]SuppressionReason{ReasonBelowSpeedFloor, ReasonEstimateNotFinal}, false},
	} {
		pt := mustEvaluate(t, path, c.leader, c.follower)
		if pt.SpatialGap.Reason != c.gap || pt.NetTimeGap.Reason != c.thw {
			t.Errorf("%s: gap %s thw %s, want %s and %s", c.name, pt.SpatialGap.Reason, pt.NetTimeGap.Reason, c.gap, c.thw)
		}
		if !reflect.DeepEqual(pt.Reasons, c.reasons) {
			t.Errorf("%s: reasons %v, want %v", c.name, pt.Reasons, c.reasons)
		}
		if pt.SupportedOpportunity != c.opportunity {
			t.Errorf("%s: opportunity %v, want %v", c.name, pt.SupportedOpportunity, c.opportunity)
		}
		if pt.PredictedGap != nil {
			t.Errorf("%s: both parties observed, so no predicted gap", c.name)
		}
	}
}

func onlineLeaderFollower(p Party) Party {
	p.Sample.Stage = StageOnline
	return p
}

func TestEvaluateFollowingProvenance(t *testing.T) {
	path, leader, follower := alignedParties(t)
	leader.Sample.Stage = StageFixedLag
	pt := mustEvaluate(t, path, leader, follower)
	v := pt.SpatialGap.Provenance.Version
	if v.EstimateStage != StageFixedLag || v.MethodID != FollowingMethodID || v.GeometryID != path.ID ||
		v.EstimatorID != FixtureEstimate().EstimatorID || v.ParamHash != FixtureEstimate().ParamHash {
		t.Fatalf("version provenance %+v", v)
	}
	in := pt.SpatialGap.Provenance.Input
	if !reflect.DeepEqual(in.ContributingTrackIDs, []string{leader.Passage.TrackID, follower.Passage.TrackID}) ||
		in.FirstUnixNanos != pt.CaptureUnixNanos || in.LastUnixNanos != pt.CaptureUnixNanos ||
		in.ObservedFrames != 2 || in.CoastedFrames != 0 || !in.PlanarFallback {
		t.Fatalf("input provenance %+v", in)
	}
}

func TestEvaluateFollowingPredictedGap(t *testing.T) {
	path, leader, follower := alignedParties(t)
	coast := func(p Party, estimation EstimationState) Party {
		p.Sample.Support, p.Sample.Faces, p.Sample.Estimation = SupportOccludedInferred, FaceVisibility{}, estimation
		p.Sample.LastObservedUnixNanos = p.Sample.CaptureUnixNanos - 3*FixtureFramePeriodNanos
		return p
	}

	pt := mustEvaluate(t, path, coast(leader, EstimationEstablished), follower)
	if pt.PredictedGap == nil || pt.PredictedGap.Suppressed || *pt.PredictedGap.Value != 5.75 {
		t.Fatalf("occluded leader: predicted gap %+v", pt.PredictedGap)
	}
	if pt.SpatialGap.Reason != ReasonNotObserved || pt.CoastAgeNanos != 3*FixtureFramePeriodNanos {
		t.Fatalf("occluded leader: gap reason %s coast age %d", pt.SpatialGap.Reason, pt.CoastAgeNanos)
	}

	// model_invalid is a tolerated reason but has no pose, even for review.
	pt = mustEvaluate(t, path, coast(leader, EstimationModelInvalid), follower)
	if pt.PredictedGap == nil || !pt.PredictedGap.Suppressed || pt.PredictedGap.Reason != ReasonModelDegraded {
		t.Fatalf("model invalid: predicted gap %+v", pt.PredictedGap)
	}

	// A predicted overlap is a geometry-review item, not a prediction.
	close := coast(leader, EstimationEstablished)
	close.Sample.X = 24
	pt = mustEvaluate(t, path, close, follower)
	if pt.PredictedGap == nil || pt.PredictedGap.Reason != ReasonNonPositiveGap {
		t.Fatalf("predicted overlap: %+v", pt.PredictedGap)
	}

	// With no extent belief there is no body, so nothing to predict, even
	// though an absent extent is otherwise a tolerated reason.
	noExtent := coast(leader, EstimationGeometryConverging)
	noExtent.Sample.Length = ExtentBelief{}
	pt = mustEvaluate(t, path, noExtent, follower)
	if pt.Gap != nil || pt.PredictedGap == nil || pt.PredictedGap.Reason != ReasonExtentNotConverged {
		t.Fatalf("no extent: gap %+v predicted %+v", pt.Gap, pt.PredictedGap)
	}
}

func TestEvaluateFollowingRejectsCallerErrors(t *testing.T) {
	path, leader, follower := alignedParties(t)
	otherEstimator := follower
	otherEstimator.Estimate.ParamHash = "fixture/other"
	unsynchronised := follower
	unsynchronised.Sample.CaptureUnixNanos += FixtureFramePeriodNanos
	unsynchronised.Sample.LastObservedUnixNanos = unsynchronised.Sample.CaptureUnixNanos
	sameTrack := follower
	sameTrack.Passage.TrackID = leader.Passage.TrackID
	noPassage := follower
	noPassage.Passage.SensorID = ""
	noEstimate := leader
	noEstimate.Estimate = EstimateIdentity{}
	noClass := follower
	noClass.Passage.MotionClass = MotionClassUnspecified
	badSample := follower
	badSample.Sample.Support = SupportUnspecified

	for _, c := range []struct {
		name             string
		leader, follower Party
		params           FollowingParams
		want             string
	}{
		{"mixed estimator versions", leader, otherEstimator, FixtureParams(), "mixes estimator versions"},
		{"unsynchronised", leader, unsynchronised, FixtureParams(), "not synchronised"},
		{"same track", leader, sameTrack, FixtureParams(), "same track"},
		{"invalid passage", leader, noPassage, FixtureParams(), "sensor id"},
		{"invalid estimate", noEstimate, follower, FixtureParams(), "estimate identity"},
		{"unspecified class", leader, noClass, FixtureParams(), "motion class"},
		{"invalid sample", leader, badSample, FixtureParams(), "support"},
		{"invalid params", leader, follower, FollowingParams{}, "positive"},
	} {
		_, err := EvaluateFollowing(path, c.leader, c.follower, c.params)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

// brokenPath is a PathFrame implementation with a defect: it locates every
// point, but with the arc and tangent a test chooses.
type brokenPath struct {
	arc     func(x float64) float64
	tangent float64
}

func (p brokenPath) GeometryID() string   { return "broken" }
func (p brokenPath) Stage() EstimateStage { return StageFinal }
func (p brokenPath) Locate(x, _ float64) (PathLocation, bool) {
	return PathLocation{ArcM: p.arc(x), TangentRad: p.tangent}, true
}

// stagedPath is the straight fixture path, but estimated at a chosen stage.
type stagedPath struct {
	StraightPath
	stage EstimateStage
}

func (p stagedPath) Stage() EstimateStage { return p.stage }

// TestEvaluateFollowingPathStage: a gap measured along a path fitted from
// non-final estimates is no more final than they are, even when the pair
// itself is final. It stays supported opportunity for review, as a non-final
// pair does, and an unset path stage is a caller error.
func TestEvaluateFollowingPathStage(t *testing.T) {
	path, leader, follower := alignedParties(t)
	pt := mustEvaluate(t, stagedPath{path, StageFixedLag}, leader, follower)
	if !reflect.DeepEqual(pt.Reasons, []SuppressionReason{ReasonEstimateNotFinal}) ||
		pt.SpatialGap.Reason != ReasonEstimateNotFinal || pt.NetTimeGap.Reason != ReasonEstimateNotFinal {
		t.Fatalf("fixed-lag path: reasons %v gap %s thw %s", pt.Reasons, pt.SpatialGap.Reason, pt.NetTimeGap.Reason)
	}
	if !pt.SupportedOpportunity || pt.SpatialGap.Provenance.Version.EstimateStage != StageFixedLag {
		t.Fatalf("fixed-lag path: opportunity %v stage %s", pt.SupportedOpportunity, pt.SpatialGap.Provenance.Version.EstimateStage)
	}
	if pt := mustEvaluate(t, stagedPath{path, StageFinal}, leader, follower); len(pt.Reasons) != 0 {
		t.Fatalf("final path: reasons %v", pt.Reasons)
	}
	if _, err := EvaluateFollowing(stagedPath{path, StageUnspecified}, leader, follower, FixtureParams()); err == nil ||
		!strings.Contains(err.Error(), "stage") {
		t.Fatalf("unset path stage: err %v", err)
	}
	if _, err := EvaluateFollowing(nil, leader, follower, FixtureParams()); err == nil {
		t.Fatal("nil path: want an error")
	}
}

// TestEvaluateFollowingRetainsTimeGapArithmetic: the time-gap arithmetic is
// kept beside the measurement, so a provisional pair still has a value to
// review, while a gap that carries its own reason yields none.
func TestEvaluateFollowingRetainsTimeGapArithmetic(t *testing.T) {
	path, leader, follower := alignedParties(t)
	pt := mustEvaluate(t, path, leader, follower)
	if pt.TimeGap == nil || pt.TimeGap.ValueS == nil || *pt.TimeGap.ValueS != *pt.NetTimeGap.Value ||
		*pt.TimeGap.SigmaS != *pt.NetTimeGap.Uncertainty.Sigma {
		t.Fatalf("supported: time gap %+v, measurement %+v", pt.TimeGap, pt.NetTimeGap)
	}

	provisional := follower
	provisional.Sample.Stage = StageFixedLag
	pt = mustEvaluate(t, path, leader, provisional)
	if !pt.NetTimeGap.Suppressed || pt.TimeGap == nil || pt.TimeGap.ValueS == nil || *pt.TimeGap.ValueS != 1.15 {
		t.Fatalf("provisional: measurement %+v, time gap %+v", pt.NetTimeGap, pt.TimeGap)
	}

	crawling := follower
	crawling.Sample.VX = 0.25
	pt = mustEvaluate(t, path, leader, crawling)
	if pt.TimeGap == nil || pt.TimeGap.ValueS != nil || pt.TimeGap.Reason != ReasonBelowSpeedFloor {
		t.Fatalf("below the floor: time gap %+v", pt.TimeGap)
	}

	coasted := follower
	coasted.Sample.Support, coasted.Sample.Faces = SupportCoasted, FaceVisibility{}
	coasted.Sample.LastObservedUnixNanos -= FixtureFramePeriodNanos
	pt = mustEvaluate(t, path, leader, coasted)
	if pt.TimeGap == nil || pt.TimeGap.ValueS != nil || pt.TimeGap.Reason != ReasonNotObserved {
		t.Fatalf("coasted: time gap %+v", pt.TimeGap)
	}

	noBody := follower
	noBody.Sample.Estimation, noBody.Sample.Faces = EstimationGeometryConverging, FaceVisibility{}
	noBody.Sample.Heading.AmbiguousModeWeight = 0.5
	if pt = mustEvaluate(t, path, leader, noBody); pt.Gap != nil || pt.TimeGap != nil {
		t.Fatalf("no body: gap %+v time gap %+v", pt.Gap, pt.TimeGap)
	}
}

// TestEvaluateFollowingRejectsABrokenPath: a defective path implementation
// must fail loudly, never surface as a gap or a time gap.
func TestEvaluateFollowingRejectsABrokenPath(t *testing.T) {
	_, leader, follower := alignedParties(t)
	for name, path := range map[string]brokenPath{
		// A NaN tangent makes every endpoint NaN.
		"NaN tangent": {arc: func(x float64) float64 { return x }, tangent: math.NaN()},
		// Finite arcs whose difference overflows: the gap is infinite.
		"overflowing arcs": {arc: func(x float64) float64 {
			if x > 25 {
				return math.MaxFloat64 / 1.5
			}
			return -math.MaxFloat64 / 1.5
		}},
	} {
		if _, err := EvaluateFollowing(path, leader, follower, FixtureParams()); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestProjectBodyWithoutAnyHeadingBelief(t *testing.T) {
	path, _, follower := alignedParties(t)
	s := follower.Sample
	s.Estimation, s.Faces, s.Heading = EstimationGeometryConverging, FaceVisibility{}, HeadingBelief{}
	if err := s.Validate(); err != nil {
		t.Fatalf("a sample with no heading belief is valid: %v", err)
	}
	if _, reason, err := ProjectBody(path, "trk", s); err != nil || reason != ReasonOrientationUnresolved {
		t.Fatalf("reason %s err %v", reason, err)
	}
}

func TestFollowingParamsValidate(t *testing.T) {
	good := FixtureParams()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, fn := range map[string]func(*FollowingParams){
		"no floor":            func(p *FollowingParams) { p.SpeedFloorMps = 0 },
		"NaN corridor":        func(p *FollowingParams) { p.CorridorHalfWidthM = math.NaN() },
		"infinite floor":      func(p *FollowingParams) { p.SpeedFloorMps = math.Inf(1) },
		"quarter-turn window": func(p *FollowingParams) { p.MaxRelativeHeadingRad = math.Pi / 2 },
		"no heading bound":    func(p *FollowingParams) { p.MaxRelativeHeadingRad = 0 },
	} {
		p := good
		fn(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestMeasureOrSuppressRefusesAReasonlessMissingValue(t *testing.T) {
	if _, err := measureOrSuppress(MetricFollowingNetTimeGap, ReasonUnspecified, testProvenance(), nil, &TimeGapEstimate{}); err == nil {
		t.Fatal("a reason-free call with no value must be an error, not a zero")
	}
}

func TestPartyAt(t *testing.T) {
	f := FixtureOcclusion()
	tr, _ := f.Trajectory("trk_fixture_occlusion_leader")
	p, ok := PartyAt(tr, fixtureAt(3))
	if !ok || p.Sample.CaptureUnixNanos != fixtureAt(3) || p.Passage != tr.Passage || p.Estimate != tr.Estimate {
		t.Fatalf("party %+v ok %v", p, ok)
	}
	if _, ok := PartyAt(tr, fixtureAt(3)+1); ok {
		t.Fatal("no sample between frames")
	}
}

func TestStraightPath(t *testing.T) {
	p := StraightPath{ID: "p", OriginX: 1, OriginY: 2, HeadingRad: math.Pi / 2, LengthM: 10}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	loc, ok := p.Locate(0, 5)
	if !ok || math.Abs(loc.ArcM-3) > 1e-12 || math.Abs(loc.LateralM-1) > 1e-12 || loc.TangentRad != math.Pi/2 {
		t.Fatalf("locate %+v ok %v", loc, ok)
	}
	x, y := p.PointAt(3, 1)
	if math.Abs(x-0) > 1e-12 || math.Abs(y-5) > 1e-12 {
		t.Fatalf("point at (3, 1) = (%v, %v)", x, y)
	}
	for _, pt := range [][2]float64{{1, 1.9}, {1, 12.1}} {
		if _, ok := p.Locate(pt[0], pt[1]); ok {
			t.Errorf("%v projects outside the segment", pt)
		}
	}
	for _, bad := range []StraightPath{{LengthM: 1}, {ID: "p"}, {ID: "p", LengthM: 1, HeadingRad: math.NaN()}} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v: want an error", bad)
		}
	}
	if p.GeometryID() != "p" {
		t.Fatal("geometry id")
	}
}

// TestProjectBodyClampsARoundingNegativeVariance pins the clamp on the
// projected position variance. Validate accepts a position block within a
// rounding tolerance of positive semidefinite, and projecting such a block
// onto the wrong diagonal gives a variance a hair below zero; with exact
// extents and heading nothing else in the sum lifts it, so without the clamp
// the endpoint sigma is NaN.
func TestProjectBodyClampsARoundingNegativeVariance(t *testing.T) {
	_, _, follower := alignedParties(t)
	path := StraightPath{ID: "diag", HeadingRad: -math.Pi / 4, LengthM: 200}
	s := follower.Sample
	s.X, s.Y = path.PointAt(50, 0)
	s.VX, s.VY = 8*math.Cos(path.HeadingRad), 8*math.Sin(path.HeadingRad)
	s.Heading.Rad, s.Heading.VarianceRad2 = path.HeadingRad, 0
	s.Length.SigmaMetres, s.Width.SigmaMetres = 0, 0
	const delta = 4e-10
	s.Covariance[0], s.Covariance[5] = 1, 1
	s.Covariance[1], s.Covariance[4] = 1+delta, 1+delta
	if err := s.Validate(); err != nil {
		t.Fatalf("a block within the PSD tolerance is valid: %v", err)
	}
	body, reason, err := ProjectBody(path, "trk", s)
	if err != nil || reason != ReasonUnspecified {
		t.Fatalf("reason %s err %v", reason, err)
	}
	for _, e := range []Endpoint{body.Leading, body.Trailing} {
		if math.IsNaN(e.SigmaM) || e.SigmaM < 0 {
			t.Fatalf("%s sigma %v, want a finite non-negative value", e.Extremity, e.SigmaM)
		}
	}
}
