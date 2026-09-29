package annotation

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func nearPoint(t *testing.T, what string, p *PlanarBound, x, y, bound float64) {
	t.Helper()
	if p == nil {
		t.Fatalf("%s: unavailable", what)
	}
	if !near(p.XM, x) || !near(p.YM, y) || !near(p.BoundM, bound) {
		t.Fatalf("%s = (%v, %v) ± %v, want (%v, %v) ± %v", what, p.XM, p.YM, p.BoundM, x, y, bound)
	}
}

// What each keyframe of the fixture establishes, worked by hand: a centre is
// the anchor or the anchor moved by its offset; a bumper is half the length
// along the axis, unless the anchor is that face; every derived bound adds
// its inputs' bounds and the swing a yaw bound gives the lever arm.
func TestPhysicalGeometryDerivations(t *testing.T) {
	p := physPack(t)
	r := validPhysical(p)
	car1, car2 := r.Objects[0], r.Objects[1]

	g := car1.Geometry(car1.Keyframes[0])
	if !g.Truth || !g.BodyTruth || g.Anchor != AnchorBodyCentre || len(g.SharedErrors) != 1 {
		t.Fatalf("body-centre keyframe: %+v", g)
	}
	nearPoint(t, "centre", g.Centre, 10, 0, 0.2)
	bumper := 0.2 + 0.2/2 + 2.25*2*math.Sin(0.05/2)
	nearPoint(t, "front", g.Front, 12.25, 0, bumper)
	nearPoint(t, "rear", g.Rear, 7.75, 0, bumper)
	if len(g.Ends) != 2 || g.Box == nil || !near(g.Box.LengthM, 4.5) || !near(g.Box.WidthM, 1.8) {
		t.Fatalf("ends %+v box %+v", g.Ends, g.Box)
	}
	if !near(g.Length.HalfWidthM, 0.2) || !near(g.Width.ValueM, 1.8) || g.Height != nil || g.HeightUnavailable != UnavailableLowerBoundOnly {
		t.Fatalf("dimensions: %+v %+v %q", g.Length, g.Width, g.HeightUnavailable)
	}

	// Rear face with a declared offset: the centre is the offset forward of
	// it, and the rear bumper is the anchor itself.
	g = car1.Geometry(car1.Keyframes[1])
	nearPoint(t, "centre from rear face", g.Centre, 13, 0, 0.15+0.2+2.25*2*math.Sin(0.1/2))
	nearPoint(t, "rear from anchor", g.Rear, 10.75, 0, 0.15)
	if g.Front != nil || g.FrontUnavailable != string(EvidenceUnknown) {
		t.Fatalf("an unknown front was placed: %+v %q", g.Front, g.FrontUnavailable)
	}

	// An ambiguous axis: no signed bumper, but both ends unlabelled.
	g = car1.Geometry(car1.Keyframes[2])
	if g.Truth || g.FrontUnavailable != UnavailableAxisAmbiguous || g.RearUnavailable != UnavailableAxisAmbiguous || len(g.Ends) != 2 || g.Box == nil {
		t.Fatalf("ambiguous-axis keyframe: %+v", g)
	}

	// An unknown width and a prior-only height leave no box.
	g = car2.Geometry(car2.Keyframes[0])
	if g.WidthUnavailable != string(EvidenceUnknown) || g.HeightUnavailable != string(EvidencePriorOnly) ||
		g.Box != nil || g.BoxUnavailable != UnavailableIncompleteBox {
		t.Fatalf("incomplete body: %+v", g)
	}
	if gs := r.Geometries(); len(gs) != 2 || len(gs["car-1"]) != 3 {
		t.Fatalf("geometries: %v", gs)
	}
}

// Each face's inward normal, at two yaws, and the reasons a centre or a
// bumper cannot be derived.
func TestPhysicalGeometryFacesAndGaps(t *testing.T) {
	body := &BodyGeometry{
		BodyID: "b", AxisConvention: BodyAxisConvention, Review: independentReview(),
		Length: DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(4), UpperM: fp(4), Support: frames(0)},
		Width:  DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(2), UpperM: fp(2), Support: frames(0)},
		Height: DimensionBound{Status: EvidenceUnknown},
	}
	key := func(kind AnchorKind, x, y, offset, yaw float64) PhysicalKeyframe {
		return PhysicalKeyframe{
			KeyframeID: "k", Anchor: PhysicalAnchor{Kind: kind, OffsetM: fp(offset), OffsetBoundM: fp(0)},
			Position: PositionBound{Status: EvidenceObserved, XM: fp(x), YM: fp(y), BoundM: fp(0), Support: frames(0)},
			Yaw:      YawBound{Status: EvidenceObserved, Axis: AxisResolved, YawRad: fp(yaw), BoundRad: fp(0), Support: frames(0)},
			Front:    EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
			Rear:     EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
			Review:   independentReview(),
		}
	}
	o := PhysicalObject{ObjectID: "o", Body: body}
	for _, c := range []struct {
		kind         AnchorKind
		x, y, offset float64
		yaw          float64
	}{
		{AnchorFrontFace, 2, 0, 2, 0}, {AnchorRearFace, -2, 0, 2, 0},
		{AnchorLeftFace, 0, 1, 1, 0}, {AnchorRightFace, 0, -1, 1, 0},
		{AnchorLeftFace, -1, 0, 1, math.Pi / 2}, {AnchorFrontFace, 0, 2, 2, math.Pi / 2},
	} {
		g := o.Geometry(key(c.kind, c.x, c.y, c.offset, c.yaw))
		nearPoint(t, string(c.kind)+" centre", g.Centre, 0, 0, 0)
	}
	// A front-face anchor is the front bumper.
	g := o.Geometry(key(AnchorFrontFace, 2, 0, 2, 0))
	nearPoint(t, "front from anchor", g.Front, 2, 0, 0)
	nearPoint(t, "rear through centre", g.Rear, -2, 0, 0)

	cases := []struct {
		name   string
		o      PhysicalObject
		mutate func(*PhysicalKeyframe)
		check  func(PhysicalGeometry) bool
	}{
		{"face without offset", o, func(k *PhysicalKeyframe) { k.Anchor.OffsetM, k.Anchor.OffsetBoundM = nil, nil },
			func(g PhysicalGeometry) bool {
				return g.CentreUnavailable == UnavailableAnchorOffsetUnknown && g.Front != nil && g.RearUnavailable == UnavailableCentre
			}},
		{"position unknown", o, func(k *PhysicalKeyframe) { k.Position = PositionBound{Status: EvidenceUnknown} },
			func(g PhysicalGeometry) bool {
				return g.AnchorUnavailable == "unknown" && g.CentreUnavailable == UnavailablePosition && g.FrontUnavailable == UnavailableCentre
			}},
		{"yaw a prior", o, func(k *PhysicalKeyframe) { k.Yaw.Status = EvidencePriorOnly },
			func(g PhysicalGeometry) bool {
				return g.YawUnavailable == "prior_only" && g.CentreUnavailable == UnavailableYaw &&
					g.Front == nil && g.FrontUnavailable == UnavailableYaw && g.RearUnavailable == UnavailableYaw
			}},
		{"axis unknown", o, func(k *PhysicalKeyframe) {
			k.Anchor = PhysicalAnchor{Kind: AnchorBodyCentre}
			k.Yaw = YawBound{Status: EvidenceUnknown, Axis: AxisUnknown}
		}, func(g PhysicalGeometry) bool {
			return g.YawUnavailable == UnavailableAxisUnknown && g.FrontUnavailable == UnavailableAxisUnknown && g.Ends == nil && g.Box == nil
		}},
		{"no body", PhysicalObject{ObjectID: "o"}, func(k *PhysicalKeyframe) { k.Anchor = PhysicalAnchor{Kind: AnchorBodyCentre} },
			func(g PhysicalGeometry) bool {
				return g.LengthUnavailable == UnavailableNoBody && g.FrontUnavailable == UnavailableLength && !g.BodyTruth
			}},
		{"body proposed", PhysicalObject{ObjectID: "o", Body: withReview(body, StatusProposed, OriginIndependent)}, func(*PhysicalKeyframe) {},
			func(g PhysicalGeometry) bool { return g.WidthUnavailable == UnavailableBodyUnreviewed }},
		{"body tracker-assisted", PhysicalObject{ObjectID: "o", Body: withReview(body, StatusReviewed, OriginTrackerAssisted)}, func(*PhysicalKeyframe) {},
			func(g PhysicalGeometry) bool { return g.HeightUnavailable == UnavailableBodyTrackerAssisted }},
	}
	for _, c := range cases {
		k := key(AnchorFrontFace, 2, 0, 2, 0)
		c.mutate(&k)
		if g := c.o.Geometry(k); !c.check(g) {
			t.Errorf("%s: %+v", c.name, g)
		}
	}
}

func withReview(b *BodyGeometry, status ReviewStatus, origin ReferenceOrigin) *BodyGeometry {
	c := *b
	c.Review.Status, c.Review.Origin = status, origin
	return &c
}

// A yaw bound B lets a point h from the pivot land anywhere on an arc whose
// chord from the nominal point is 2h sin(B/2), up to the full diameter at
// B = π. The stated bumper and face-offset centre bounds must cover the
// farthest point on that arc, not only its lateral part h sin B.
func TestPhysicalGeometryBoundsCoverTheYawSwing(t *testing.T) {
	const length = 4.6
	body := &BodyGeometry{
		BodyID: "b", AxisConvention: BodyAxisConvention, Review: independentReview(),
		Length: DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(length), UpperM: fp(length), Support: frames(0)},
		Width:  DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(1.8), UpperM: fp(1.8), Support: frames(0)},
		Height: DimensionBound{Status: EvidenceUnknown},
	}
	o := PhysicalObject{ObjectID: "o", Body: body}
	worst := func(h, yaw, b float64) float64 {
		nx, ny := h*math.Cos(yaw), h*math.Sin(yaw)
		far := 0.0
		for i := 0; i <= 1000; i++ {
			a := yaw - b + 2*b*float64(i)/1000
			far = math.Max(far, math.Hypot(h*math.Cos(a)-nx, h*math.Sin(a)-ny))
		}
		return far
	}
	for _, b := range []float64{0.3, 1.0, math.Pi / 2, math.Pi} {
		k := PhysicalKeyframe{
			KeyframeID: "k", Anchor: PhysicalAnchor{Kind: AnchorBodyCentre},
			Position: PositionBound{Status: EvidenceObserved, XM: fp(0), YM: fp(0), BoundM: fp(0), Support: frames(0)},
			Yaw:      YawBound{Status: EvidenceObserved, Axis: AxisResolved, YawRad: fp(0.4), BoundRad: fp(b), Support: frames(0)},
			Front:    EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
			Rear:     EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
			Review:   independentReview(),
		}
		g := o.Geometry(k)
		if g.Front == nil || g.Front.BoundM < worst(length/2, 0.4, b)-1e-9 {
			t.Errorf("B=%v: front bound %+v does not cover the swing %v", b, g.Front, worst(length/2, 0.4, b))
		}
		k.Anchor = PhysicalAnchor{Kind: AnchorRearFace, OffsetM: fp(length / 2), OffsetBoundM: fp(0)}
		if g = o.Geometry(k); g.Centre == nil || g.Centre.BoundM < worst(length/2, 0.4, b)-1e-9 {
			t.Errorf("B=%v: centre bound %+v does not cover the swing %v", b, g.Centre, worst(length/2, 0.4, b))
		}
	}
}
