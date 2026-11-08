package annotation

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// spotProposal is a compact spot proposed through the full read path: planar
// position given the prior yaw, centre (11, 0).
func spotProposal(t *testing.T) *FacetPoseProposal {
	t.Helper()
	p, req := poseRequest(t)
	result, err := ProposeFacetPose(p, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Constraint != ConstraintPlanarPositionGivenPriorYaw {
		t.Fatalf("constraint %q", result.Constraint)
	}
	return result
}

// segmentProposal is a straight edge on a body at yaw 0: its normal is +y,
// its centre (10, 3), so the normal offset n . c is 3.
func segmentProposal(t *testing.T) *FacetPoseProposal {
	t.Helper()
	_, req := poseRequest(t)
	feature := &pb.FeatureCandidate{ObjectId: "car", Anchor: &pb.FeatureAnchor{BodyId: "body", BoundM: .2, Line: &pb.FeatureLineConstraint{NormalY: 1, OffsetM: -1, NormalBoundRad: .02}}}
	target := &pb.FeatureObservation{PointIndices: []uint32{0, 1, 2}, Origin: "assisted_proposal"}
	req.EndIndex = ptrU32(2)
	req.Prior.XM, req.Prior.YM = fp(10), fp(2.8)
	points := Points{X: []float32{8, 9, 10}, Y: []float32{2, 2, 2}, Z: []float32{1, 1, 1}}
	result, err := solveFacetPose(feature, target, &BodyGeometry{}, points, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Constraint != ConstraintNormalPositionAndYaw || result.Yaw.Status != EvidenceObserved {
		t.Fatalf("segment %+v", result)
	}
	return result
}

func fmNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("%s = %.15g, want %.15g", name, got, want)
	}
}

func TestBoundToSigmaRules(t *testing.T) {
	for rule, want := range map[BoundToSigma]float64{"": 0.3 / math.Sqrt(3), BoundToSigmaUniform: 0.3 / math.Sqrt(3), BoundToSigmaThreeSigma: 0.1} {
		got, err := rule.Sigma(0.3)
		if err != nil {
			t.Fatal(err)
		}
		fmNear(t, string(rule), got, want)
	}
	// The default is the conservative, wider rule.
	uniform, _ := BoundToSigmaUniform.Sigma(1)
	three, _ := BoundToSigmaThreeSigma.Sigma(1)
	if DefaultBoundToSigma != BoundToSigmaUniform || !(uniform > three) {
		t.Fatalf("default %q, uniform %g, three-sigma %g", DefaultBoundToSigma, uniform, three)
	}
	for _, bound := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := BoundToSigmaUniform.Sigma(bound); err == nil {
			t.Fatalf("bound %g converted", bound)
		}
	}
	if _, err := BoundToSigma("gaussian").Sigma(1); err == nil || !strings.Contains(err.Error(), "unknown bound-to-sigma rule") {
		t.Fatalf("unknown rule: %v", err)
	}
	if _, err := FacetMeasurementFromProposal(spotProposal(t), "gaussian"); err == nil {
		t.Fatal("an unknown rule made a measurement")
	}
}

func TestFacetMeasurementOfACompactSpot(t *testing.T) {
	p := spotProposal(t)
	for _, tc := range []struct {
		rule BoundToSigma
		div  float64
	}{{"", math.Sqrt(3)}, {BoundToSigmaUniform, math.Sqrt(3)}, {BoundToSigmaThreeSigma, 3}} {
		m, err := FacetMeasurementFromProposal(p, tc.rule)
		if err != nil {
			t.Fatal(err)
		}
		if m.Rule != tc.rule.resolved() || m.Constraint != ConstraintPlanarPositionGivenPriorYaw || m.Position == nil || m.Normal != nil {
			t.Fatalf("%q: %+v", tc.rule, m)
		}
		fmNear(t, "x", m.Position.XM, p.Centre.XM)
		fmNear(t, "y", m.Position.YM, p.Centre.YM)
		fmNear(t, "bound", m.Position.BoundM, p.Centre.BoundM)
		fmNear(t, "sigma", m.Position.SigmaM, p.Centre.BoundM/tc.div)
		fmNear(t, "variance", m.Position.VarianceM2, math.Pow(p.Centre.BoundM/tc.div, 2))
		// The proposal carries the prior's yaw bound, and it must not become a
		// sigma: a compact spot measures no yaw.
		if p.Yaw.BoundRad <= 0 || m.Yaw != nil {
			t.Fatalf("%q: the prior's yaw became a measurement: %+v", tc.rule, m.Yaw)
		}
		if !reflect.DeepEqual(m.Unmeasured, []string{UnresolvedYawFromPrior, UnresolvedHeight}) {
			t.Fatalf("unmeasured %v", m.Unmeasured)
		}
	}
}

func TestFacetMeasurementOfASegment(t *testing.T) {
	p := segmentProposal(t)
	for _, tc := range []struct {
		rule BoundToSigma
		div  float64
	}{{BoundToSigmaUniform, math.Sqrt(3)}, {BoundToSigmaThreeSigma, 3}} {
		m, err := FacetMeasurementFromProposal(p, tc.rule)
		if err != nil {
			t.Fatal(err)
		}
		// A normal measurement and a yaw; no planar position, because the
		// tangent is the prior's.
		if m.Normal == nil || m.Yaw == nil || m.Position != nil || m.Rule != tc.rule {
			t.Fatalf("%q: %+v", tc.rule, m)
		}
		if m.Normal.Normal != *p.Normal {
			t.Fatalf("normal %v, want %v", m.Normal.Normal, *p.Normal)
		}
		fmNear(t, "offset n.c", m.Normal.OffsetM, 3)
		fmNear(t, "normal sigma", m.Normal.SigmaM, *p.NormalBoundM/tc.div)
		fmNear(t, "normal variance", m.Normal.VarianceM2, math.Pow(*p.NormalBoundM/tc.div, 2))
		fmNear(t, "yaw", m.Yaw.Rad, p.Yaw.Rad)
		fmNear(t, "yaw sigma", m.Yaw.SigmaRad, p.Yaw.BoundRad/tc.div)
		fmNear(t, "yaw variance", m.Yaw.VarianceRad2, math.Pow(p.Yaw.BoundRad/tc.div, 2))
		if !m.Yaw.ModPi {
			t.Fatal("a line normal's yaw is known only modulo pi")
		}
		if !reflect.DeepEqual(m.Unmeasured, []string{UnresolvedHeight, UnresolvedTangentFromPrior}) {
			t.Fatalf("unmeasured %v", m.Unmeasured)
		}
	}
	// The tangent bound is the prior's position bound, and no sigma is made
	// from it under either rule.
	if p.TangentBoundM == nil || *p.TangentBoundM <= 0 {
		t.Fatalf("fixture has no tangent bound: %+v", p)
	}
	raw, err := json.Marshal(mustMeasure(t, p))
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	for k := range keys {
		switch k {
		case "constraint", "bound_to_sigma", "normal", "yaw", "unmeasured":
		default:
			t.Fatalf("a segment's measurement carries %q: %s", k, raw)
		}
	}
}

func mustMeasure(t *testing.T, p *FacetPoseProposal) *FacetMeasurement {
	t.Helper()
	m, err := FacetMeasurementFromProposal(p, "")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestFacetMeasurementOmitsAPriorOnlyYaw(t *testing.T) {
	// A segment whose yaw came back prior-only, and says so, keeps its normal
	// measurement and loses its yaw: an unresolved direction has no sigma.
	p := segmentProposal(t)
	p.Yaw.Status = EvidencePriorOnly
	p.Unresolved = append(p.Unresolved, UnresolvedYawFromPrior)
	m := mustMeasure(t, p)
	if m.Yaw != nil || m.Normal == nil {
		t.Fatalf("%+v", m)
	}
	intervals, err := FacetIntervals(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range intervals {
		if in.Quantity == FacetIntervalYaw {
			t.Fatalf("a prior-only yaw became an interval: %+v", in)
		}
	}
}

func TestFacetIntervalsAreTheBound(t *testing.T) {
	spot := spotProposal(t)
	got, err := FacetIntervals(spot)
	if err != nil {
		t.Fatal(err)
	}
	b := spot.Centre.BoundM
	want := []FacetInterval{
		{Quantity: FacetIntervalPositionX, Direction: &[2]float64{1, 0}, Unit: "m", Lower: spot.Centre.XM - b, Upper: spot.Centre.XM + b, HalfWidth: b},
		{Quantity: FacetIntervalPositionY, Direction: &[2]float64{0, 1}, Unit: "m", Lower: spot.Centre.YM - b, Upper: spot.Centre.YM + b, HalfWidth: b},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spot intervals %+v, want %+v", got, want)
	}

	seg := segmentProposal(t)
	got, err = FacetIntervals(seg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Quantity != FacetIntervalNormalPosition || got[1].Quantity != FacetIntervalYaw {
		t.Fatalf("segment intervals %+v", got)
	}
	nb, yb := *seg.NormalBoundM, seg.Yaw.BoundRad
	if *got[0].Direction != *seg.Normal || got[0].HalfWidth != nb || got[0].Lower != 3-nb || got[0].Upper != 3+nb || got[0].Unit != "m" {
		t.Fatalf("normal interval %+v, bound %g", got[0], nb)
	}
	if got[1].Direction != nil || got[1].HalfWidth != yb || got[1].Lower != seg.Yaw.Rad-yb || got[1].Upper != seg.Yaw.Rad+yb || got[1].Unit != "rad" {
		t.Fatalf("yaw interval %+v, bound %g", got[1], yb)
	}
	// No rule enters the interval: it is the bound, and the Gaussian forms
	// are its half-width over sqrt(3) or 3.
	m := mustMeasure(t, seg)
	fmNear(t, "interval against sigma", got[0].HalfWidth/math.Sqrt(3), m.Normal.SigmaM)
}

func TestFacetMeasurementRefusesAProposalItCannotRead(t *testing.T) {
	cases := map[string]func() *FacetPoseProposal{
		"nil": func() *FacetPoseProposal { return nil },
		"unknown unresolved": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Unresolved = append(p.Unresolved, "roll_not_measured")
			return p
		},
		"unknown constraint": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Constraint = "surface_normal"
			return p
		},
		"segment yaw observed but listed unresolved": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Unresolved = append(p.Unresolved, UnresolvedYawFromPrior)
			return p
		},
		"spot yaw prior-only but not listed": func() *FacetPoseProposal {
			p := spotProposal(t)
			p.Unresolved = []string{UnresolvedHeight}
			return p
		},
		"spot claims a measured yaw": func() *FacetPoseProposal {
			p := spotProposal(t)
			p.Yaw.Status = EvidenceObserved
			p.Unresolved = []string{UnresolvedHeight}
			return p
		},
		"spot lists the tangent unresolved": func() *FacetPoseProposal {
			p := spotProposal(t)
			p.Unresolved = append(p.Unresolved, UnresolvedTangentFromPrior)
			return p
		},
		"segment omits its tangent": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Unresolved = []string{UnresolvedHeight}
			return p
		},
		"segment without a normal": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Normal = nil
			return p
		},
		"segment normal not unit": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Normal = &[2]float64{0, 2}
			return p
		},
		"zero normal bound": func() *FacetPoseProposal {
			p := segmentProposal(t)
			zero := 0.0
			p.NormalBoundM = &zero
			return p
		},
		"zero yaw bound": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Yaw.BoundRad = 0
			return p
		},
		"non-finite yaw": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Yaw.Rad = math.NaN()
			return p
		},
		"non-finite segment centre": func() *FacetPoseProposal {
			p := segmentProposal(t)
			p.Centre.XM = math.Inf(1)
			return p
		},
		"zero spot bound": func() *FacetPoseProposal {
			p := spotProposal(t)
			p.Centre.BoundM = 0
			return p
		},
		"non-finite spot centre": func() *FacetPoseProposal {
			p := spotProposal(t)
			p.Centre.YM = math.NaN()
			return p
		},
	}
	for name, build := range cases {
		if _, err := FacetMeasurementFromProposal(build(), ""); err == nil {
			t.Errorf("%s: measured", name)
		}
		if _, err := FacetIntervals(build()); err == nil {
			t.Errorf("%s: interval", name)
		}
	}
}
