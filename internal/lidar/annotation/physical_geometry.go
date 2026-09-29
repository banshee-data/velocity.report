package annotation

import "math"

// What a keyframe establishes.
//
// A keyframe states an anchor, a yaw and its bumpers' evidence, and through
// its object a body. A scorer or an inspector needs the body centre, the two
// bumpers and the box, each with a bound, or the reason it cannot be had.
// They are derived here once, so the scorer and the inspector cannot disagree
// about them. A derived bound is the sum of its inputs' bounds, including the
// lateral swing a yaw bound gives a lever arm: worst-case propagation, which
// holds whatever the correlation between errors that share an observation.

// Reasons a derived component is unavailable. A component's own status is
// reported as itself: "unknown" or "prior_only".
const (
	UnavailableLowerBoundOnly      = "lower_bound_only"
	UnavailableNoBody              = "no_body"
	UnavailableBodyUnreviewed      = "body_unreviewed"
	UnavailableBodyTrackerAssisted = "body_tracker_assisted"
	UnavailableAxisUnknown         = "axis_unknown"
	UnavailableAxisAmbiguous       = "axis_ambiguous"
	UnavailableAnchorOffsetUnknown = "anchor_offset_unknown"
	UnavailablePosition            = "position_unavailable"
	UnavailableYaw                 = "yaw_unavailable"
	UnavailableCentre              = "centre_unavailable"
	UnavailableLength              = "length_unavailable"
	UnavailableIncompleteBox       = "incomplete_box"
)

// PlanarBound is a point in the pack's frame and the horizontal radius that
// bounds it.
type PlanarBound struct {
	XM     float64 `json:"x_m"`
	YM     float64 `json:"y_m"`
	BoundM float64 `json:"bound_m"`
}

// AngleBound is a yaw, its half-width, and what is known of its axis.
type AngleBound struct {
	Rad      float64        `json:"rad"`
	BoundRad float64        `json:"bound_rad"`
	Axis     AxisState      `json:"axis"`
	Status   EvidenceStatus `json:"status"`
}

// LinearBound is a dimension's interval and the value quoted from it.
type LinearBound struct {
	LowerM     float64        `json:"lower_m"`
	UpperM     float64        `json:"upper_m"`
	ValueM     float64        `json:"value_m"`
	HalfWidthM float64        `json:"half_width_m"`
	Status     EvidenceStatus `json:"status"`
}

// BoxBound is a complete reference footprint: centre, yaw, length and width.
// Under an ambiguous axis it is the same rectangle either way round.
type BoxBound struct {
	CentreXM float64 `json:"centre_x_m"`
	CentreYM float64 `json:"centre_y_m"`
	YawRad   float64 `json:"yaw_rad"`
	LengthM  float64 `json:"length_m"`
	WidthM   float64 `json:"width_m"`
}

// PhysicalGeometry is one keyframe's reference layer: what it states, what
// follows from it, and why anything that does not follow is missing. It
// carries the keyframe's review, and is truth only when that review is.
type PhysicalGeometry struct {
	ObjectID     string          `json:"object_id"`
	KeyframeID   string          `json:"keyframe_id"`
	SampleID     int             `json:"sample_id"`
	TimestampNs  int64           `json:"timestamp_ns"`
	ReviewStatus ReviewStatus    `json:"review_status"`
	Origin       ReferenceOrigin `json:"origin"`
	// Truth is a reviewed, independent keyframe; BodyTruth the same of the
	// object's body.
	Truth     bool       `json:"truth"`
	BodyTruth bool       `json:"body_truth"`
	Anchor    AnchorKind `json:"anchor"`

	AnchorPoint       *PlanarBound `json:"anchor_point,omitempty"`
	AnchorUnavailable string       `json:"anchor_unavailable,omitempty"`
	Centre            *PlanarBound `json:"centre,omitempty"`
	CentreUnavailable string       `json:"centre_unavailable,omitempty"`
	Yaw               *AngleBound  `json:"yaw,omitempty"`
	YawUnavailable    string       `json:"yaw_unavailable,omitempty"`

	Length            *LinearBound `json:"length,omitempty"`
	LengthUnavailable string       `json:"length_unavailable,omitempty"`
	Width             *LinearBound `json:"width,omitempty"`
	WidthUnavailable  string       `json:"width_unavailable,omitempty"`
	Height            *LinearBound `json:"height,omitempty"`
	HeightUnavailable string       `json:"height_unavailable,omitempty"`

	Front            *PlanarBound `json:"front,omitempty"`
	FrontUnavailable string       `json:"front_unavailable,omitempty"`
	Rear             *PlanarBound `json:"rear,omitempty"`
	RearUnavailable  string       `json:"rear_unavailable,omitempty"`
	// Ends are both bumpers without saying which is which: available under
	// an ambiguous axis, where a signed front or rear is not.
	Ends           []PlanarBound `json:"ends,omitempty"`
	Box            *BoxBound     `json:"box,omitempty"`
	BoxUnavailable string        `json:"box_unavailable,omitempty"`

	SharedErrors []SharedError `json:"shared_errors,omitempty"`
}

// Geometry derives what one of the object's keyframes establishes.
func (o PhysicalObject) Geometry(k PhysicalKeyframe) PhysicalGeometry {
	g := PhysicalGeometry{
		ObjectID: o.ObjectID, KeyframeID: k.KeyframeID, SampleID: k.SampleID, TimestampNs: k.TimestampNs,
		ReviewStatus: k.Review.Status, Origin: k.Review.Origin, Truth: k.Review.ScoredAsTruth(),
		Anchor: k.Anchor.Kind, SharedErrors: k.SharedErrors,
	}
	if k.Position.Status.Scorable() {
		g.AnchorPoint = &PlanarBound{XM: *k.Position.XM, YM: *k.Position.YM, BoundM: *k.Position.BoundM}
	} else {
		g.AnchorUnavailable = string(k.Position.Status)
	}
	switch {
	case k.Yaw.Axis == AxisUnknown:
		g.YawUnavailable = UnavailableAxisUnknown
	case !k.Yaw.Status.Scorable():
		g.YawUnavailable = string(k.Yaw.Status)
	default:
		g.Yaw = &AngleBound{Rad: *k.Yaw.YawRad, BoundRad: *k.Yaw.BoundRad, Axis: k.Yaw.Axis, Status: k.Yaw.Status}
	}
	g.body(o.Body)
	g.centre(k.Anchor)
	g.Front, g.FrontUnavailable = g.endpoint(k.Front, 1, AnchorFrontFace)
	g.Rear, g.RearUnavailable = g.endpoint(k.Rear, -1, AnchorRearFace)
	if g.Centre != nil && g.Yaw != nil && g.Length != nil {
		for _, sign := range []float64{1, -1} {
			g.Ends = append(g.Ends, g.alongAxis(sign))
		}
	}
	if g.Centre != nil && g.Yaw != nil && g.Length != nil && g.Width != nil {
		g.Box = &BoxBound{CentreXM: g.Centre.XM, CentreYM: g.Centre.YM, YawRad: g.Yaw.Rad,
			LengthM: g.Length.ValueM, WidthM: g.Width.ValueM}
	} else {
		g.BoxUnavailable = UnavailableIncompleteBox
	}
	return g
}

func (g *PhysicalGeometry) body(b *BodyGeometry) {
	reason := ""
	switch {
	case b == nil:
		reason = UnavailableNoBody
	case b.Review.Origin != OriginIndependent:
		reason = UnavailableBodyTrackerAssisted
	case b.Review.Status != StatusReviewed:
		reason = UnavailableBodyUnreviewed
	}
	if reason != "" {
		g.LengthUnavailable, g.WidthUnavailable, g.HeightUnavailable = reason, reason, reason
		return
	}
	g.BodyTruth = true
	g.Length, g.LengthUnavailable = linearBound(b.Length)
	g.Width, g.WidthUnavailable = linearBound(b.Width)
	g.Height, g.HeightUnavailable = linearBound(b.Height)
}

// linearBound is a dimension a scorer may use, or why not. A partial span is
// a lower bound, not a dimension.
func linearBound(d DimensionBound) (*LinearBound, string) {
	if !d.Status.Scorable() {
		return nil, string(d.Status)
	}
	value, half, ok := d.Best()
	if !ok {
		return nil, UnavailableLowerBoundOnly
	}
	return &LinearBound{LowerM: *d.LowerM, UpperM: *d.UpperM, ValueM: value, HalfWidthM: half, Status: d.Status}, ""
}

// centre is the anchor itself for a body-centre anchor, and the anchor moved
// by its declared offset along the face's inward normal otherwise.
func (g *PhysicalGeometry) centre(a PhysicalAnchor) {
	switch {
	case g.AnchorPoint == nil:
		g.CentreUnavailable = UnavailablePosition
	case a.Kind == AnchorBodyCentre:
		c := *g.AnchorPoint
		g.Centre = &c
	case a.OffsetM == nil:
		g.CentreUnavailable = UnavailableAnchorOffsetUnknown
	case g.Yaw == nil:
		g.CentreUnavailable = UnavailableYaw
	default:
		nx, ny := inwardNormal(a.Kind, g.Yaw.Rad)
		off := *a.OffsetM
		g.Centre = &PlanarBound{
			XM: g.AnchorPoint.XM + off*nx, YM: g.AnchorPoint.YM + off*ny,
			BoundM: g.AnchorPoint.BoundM + *a.OffsetBoundM + off*swing(g.Yaw.BoundRad),
		}
	}
}

// endpoint is one bumper: the anchor itself when the anchor is that face,
// and otherwise half the length from the centre along the axis.
func (g *PhysicalGeometry) endpoint(e EndpointEvidence, sign float64, face AnchorKind) (*PlanarBound, string) {
	switch {
	case g.Yaw != nil && g.Yaw.Axis == AxisFrontRearAmbiguous:
		return nil, UnavailableAxisAmbiguous
	case g.YawUnavailable == UnavailableAxisUnknown:
		return nil, UnavailableAxisUnknown
	case !e.Status.Scorable():
		return nil, string(e.Status)
	case g.Anchor == face && g.AnchorPoint != nil:
		p := *g.AnchorPoint
		return &p, ""
	case g.Yaw == nil:
		return nil, UnavailableYaw
	case g.Centre == nil:
		return nil, UnavailableCentre
	case g.Length == nil:
		return nil, UnavailableLength
	}
	p := g.alongAxis(sign)
	return &p, ""
}

// alongAxis is the point half the length from the centre, forward for +1.
func (g *PhysicalGeometry) alongAxis(sign float64) PlanarBound {
	half := g.Length.ValueM / 2
	return PlanarBound{
		XM:     g.Centre.XM + sign*half*math.Cos(g.Yaw.Rad),
		YM:     g.Centre.YM + sign*half*math.Sin(g.Yaw.Rad),
		BoundM: g.Centre.BoundM + g.Length.HalfWidthM/2 + half*swing(g.Yaw.BoundRad),
	}
}

// swing is the lateral displacement per metre of lever arm that a yaw bound
// allows.
func swing(boundRad float64) float64 { return math.Sin(math.Min(boundRad, math.Pi/2)) }

// Geometries derives every keyframe of every object, keyed by object and
// sample.
func (r *PhysicalReferenceSet) Geometries() map[string]map[int]PhysicalGeometry {
	out := make(map[string]map[int]PhysicalGeometry, len(r.Objects))
	for _, o := range r.Objects {
		byFrame := make(map[int]PhysicalGeometry, len(o.Keyframes))
		for _, k := range o.Keyframes {
			byFrame[k.SampleID] = o.Geometry(k)
		}
		out[o.ObjectID] = byFrame
	}
	return out
}
