package annotation

import (
	"fmt"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"math"
	"strings"
)

// FacetPosePrior is an explicit experiment input, never a target reference.
// Pointers distinguish a stated zero bound from an omitted uncertainty.
type FacetPosePrior struct {
	XM             *float64 `json:"x_m"`
	YM             *float64 `json:"y_m"`
	YawRad         *float64 `json:"yaw_rad"`
	PositionBoundM *float64 `json:"position_bound_m"`
	YawBoundRad    *float64 `json:"yaw_bound_rad"`
	Source         string   `json:"source"`
}

type FacetPoseRequest struct {
	SchemaVersion      int            `json:"schema_version"`
	Pack               string         `json:"pack"`
	PackDigest         string         `json:"pack_digest"`
	FeatureID          string         `json:"feature_id"`
	FeatureRevision    uint64         `json:"feature_revision"`
	FeatureDigest      string         `json:"feature_digest"`
	MembershipRevision int            `json:"membership_revision"`
	MembershipDigest   string         `json:"membership_digest"`
	SampleID           int            `json:"sample_id"`
	TimestampNs        int64          `json:"timestamp_ns"`
	PointIndex         *uint32        `json:"point_index,omitempty"`
	EndIndex           *uint32        `json:"end_index,omitempty"`
	ReturnBoundM       float64        `json:"return_bound_m"`
	Confirmed          bool           `json:"correspondence_confirmed"`
	Author             string         `json:"author"`
	IdentityNote       string         `json:"identity_note"`
	Prior              FacetPosePrior `json:"prior"`
}

// A proposal is not a PhysicalKeyframe and has no review/write semantics.
// The prior is retained verbatim, yaw is never estimated, and hidden ends are
// predictions from the fixed source body, not newly measured endpoints.
type FacetPoseProposal struct {
	Schema                 string           `json:"schema"`
	SchemaVersion          int              `json:"schema_version"`
	Status                 string           `json:"status"`
	Request                FacetPoseRequest `json:"request"`
	ObjectID               string           `json:"object_id"`
	SourcePhysicalRevision uint64           `json:"source_physical_revision"`
	SourcePhysicalDigest   string           `json:"source_physical_digest"`
	SourceSample           uint32           `json:"source_sample"`
	SourceOrigin           string           `json:"source_origin"`
	TargetOrigin           string           `json:"target_origin"`
	Body                   *BodyGeometry    `json:"body"`
	Centre                 PlanarBound      `json:"centre"`
	Yaw                    AngleBound       `json:"yaw"`
	Constraint             string           `json:"constraint"`
	Normal                 *[2]float64      `json:"normal,omitempty"`
	NormalBoundM           *float64         `json:"normal_bound_m,omitempty"`
	TangentBoundM          *float64         `json:"tangent_bound_m,omitempty"`
	Unresolved             []string         `json:"unresolved"`
	MatchedIndices         []uint32         `json:"matched_indices"`
	Box                    *BoxBound        `json:"box,omitempty"`
	BoxUnavailable         string           `json:"box_unavailable,omitempty"`
	FrontPrediction        *[2]float64      `json:"front_prediction,omitempty"`
	RearPrediction         *[2]float64      `json:"rear_prediction,omitempty"`
}

// ProposeFacetPose resolves exact saved evidence and reads only the SOURCE
// physical body/registration. No target pose is selected or passed to solver.
// All reads are side-effect free. The result carries the pins it actually used.
func ProposeFacetPose(p *Pack, req FacetPoseRequest) (*FacetPoseProposal, error) {
	if req.SchemaVersion != 1 || req.PackDigest != p.Manifest.PackDigest || !req.Confirmed || strings.TrimSpace(req.Author) == "" || strings.TrimSpace(req.IdentityNote) == "" {
		return nil, fmt.Errorf("name and confirm the manual correspondence in this pack")
	}
	membership, err := LoadSidecar(p)
	if err != nil {
		return nil, err
	}
	if membership.Digest() != req.MembershipDigest || membership.Revision != req.MembershipRevision {
		return nil, ErrMembershipChanged
	}
	points, err := p.PointsAt(req.SampleID)
	if err != nil {
		return nil, err
	}
	state, err := LoadFeatureRevision(p, req.FeatureRevision)
	if err != nil {
		return nil, err
	}
	if state.Digest != req.FeatureDigest {
		return nil, ErrSidecarConflict
	}
	var feature *pb.FeatureCandidate
	for _, f := range state.Document.Features {
		if f.FeatureId == req.FeatureID {
			feature = f
		}
	}
	if feature == nil || feature.Inactive || feature.Anchor == nil {
		return nil, fmt.Errorf("choose an active saved body registration")
	}
	var target *pb.FeatureObservation
	for _, o := range feature.Observations {
		if int(o.SampleId) == req.SampleID {
			target = o
		}
	}
	if req.SampleID < 0 || req.SampleID >= len(p.Samples) || target == nil || req.SampleID == int(feature.Anchor.SourceSample) || target.Decision != pb.FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL || target.TimestampNs != req.TimestampNs || p.Samples[req.SampleID].TimestampNs != req.TimestampNs {
		return nil, fmt.Errorf("target needs accepted support at the exact sample timestamp, separate from the source registration frame")
	}
	if target.MembershipDigest != req.MembershipDigest || target.MembershipRevision != uint64(req.MembershipRevision) {
		return nil, ErrMembershipChanged
	}
	// LoadFeatureRevision validated this registration against its pinned
	// body; resolve that body by the same rule rather than restating it.
	source, err := newFeatureEvidence(p).registeredObject(feature)
	if err != nil {
		return nil, fmt.Errorf("source body pin no longer resolves: %w", err)
	}
	return solveFacetPose(feature, target, source.Body, points, req)
}

func solveFacetPose(feature *pb.FeatureCandidate, target *pb.FeatureObservation, body *BodyGeometry, points Points, req FacetPoseRequest) (*FacetPoseProposal, error) {
	prior := req.Prior
	for _, value := range []*float64{prior.XM, prior.YM, prior.YawRad, prior.PositionBoundM, prior.YawBoundRad} {
		if value == nil || !finiteFeature(*value) {
			return nil, fmt.Errorf("state finite prior position, yaw and explicit uncertainty bounds")
		}
	}
	if *prior.PositionBoundM < 0 || *prior.YawBoundRad < 0 || *prior.YawBoundRad > math.Pi || strings.TrimSpace(prior.Source) == "" || !finiteFeature(req.ReturnBoundM) || req.ReturnBoundM <= 0 {
		return nil, fmt.Errorf("invalid prior source or uncertainty")
	}
	a := feature.Anchor
	uncertain := map[uint32]bool{}
	for _, i := range target.UncertainIndices {
		uncertain[i] = true
	}
	definite := map[uint32]bool{}
	for _, i := range target.PointIndices {
		if !uncertain[i] {
			definite[i] = true
		}
	}
	if req.PointIndex == nil || !definite[*req.PointIndex] {
		return nil, fmt.Errorf("choose a definite matched return")
	}
	if int(*req.PointIndex) >= len(points.X) || int(*req.PointIndex) >= len(points.Y) || int(*req.PointIndex) >= len(points.Z) {
		return nil, fmt.Errorf("matched return outside sample")
	}
	c, s := math.Cos(*prior.YawRad), math.Sin(*prior.YawRad)
	result := &FacetPoseProposal{Schema: "velocity.report/facet-pose-proposal", SchemaVersion: 1, Status: "read_only_assisted_proposal", Request: req, ObjectID: feature.ObjectId, SourcePhysicalRevision: a.PhysicalRevision, SourcePhysicalDigest: a.PhysicalDigest, SourceSample: a.SourceSample, SourceOrigin: a.Origin, TargetOrigin: target.Origin, Body: body, Yaw: AngleBound{Rad: *prior.YawRad, BoundRad: *prior.YawBoundRad, Axis: AxisResolved, Status: EvidencePriorOnly}, Unresolved: []string{"yaw_from_prior_not_measured", "height_not_measured"}}
	if a.Line == nil {
		if req.EndIndex != nil {
			return nil, fmt.Errorf("compact spot takes one matched return")
		}
		i := *req.PointIndex
		x, y, z := float64(points.X[i]), float64(points.Y[i]), float64(points.Z[i])
		if !finiteFeature(x) || !finiteFeature(y) || !finiteFeature(z) {
			return nil, fmt.Errorf("non-finite matched return")
		}
		evidenceBound := a.BoundM + req.ReturnBoundM + math.Hypot(a.XM, a.YM)*2*math.Sin(*prior.YawBoundRad/2)
		result.Centre = PlanarBound{XM: x - (c*a.XM - s*a.YM), YM: y - (s*a.XM + c*a.YM), BoundM: math.Max(*prior.PositionBoundM, evidenceBound)}
		if math.Hypot(result.Centre.XM-*prior.XM, result.Centre.YM-*prior.YM) > *prior.PositionBoundM+evidenceBound {
			return nil, fmt.Errorf("matched spot contradicts the declared prior bounds; correct the prior or correspondence")
		}
		result.Constraint = "planar_position_given_prior_yaw"
		result.MatchedIndices = []uint32{i}
	} else {
		if req.EndIndex == nil || !definite[*req.EndIndex] {
			return nil, fmt.Errorf("choose two definite direction returns")
		}
		indices := []uint32{}
		for _, i := range target.PointIndices {
			if definite[i] {
				indices = append(indices, i)
			}
		}
		targetLine, _, err := featureSegmentRelation(points, indices, *req.PointIndex, *req.EndIndex, PlanarBound{}, AngleBound{}, req.ReturnBoundM)
		if err != nil {
			return nil, err
		}
		nx, ny := c*a.Line.NormalX-s*a.Line.NormalY, s*a.Line.NormalX+c*a.Line.NormalY
		angular := a.Line.NormalBoundRad + *prior.YawBoundRad + targetLine.NormalBoundRad
		if angular >= math.Pi/2 {
			return nil, fmt.Errorf("combined normal uncertainty is unconstrained")
		}
		dot := nx*targetLine.NormalX + ny*targetLine.NormalY
		if math.Acos(math.Min(1, math.Abs(dot))) > angular+1e-9 {
			return nil, fmt.Errorf("target edge direction contradicts the prior; yaw is not solved")
		}
		start, end := *req.PointIndex, *req.EndIndex
		mx, my := (float64(points.X[start])+float64(points.X[end]))/2, (float64(points.Y[start])+float64(points.Y[end]))/2
		displacement := nx*(mx-*prior.XM) + ny*(my-*prior.YM) - a.Line.OffsetM
		normalBound := a.BoundM + req.ReturnBoundM + (math.Hypot(mx-*prior.XM, my-*prior.YM)+math.Abs(a.Line.OffsetM)+*prior.PositionBoundM)*2*math.Sin(angular/2)
		normalBound = normalBound/math.Cos(angular) + (math.Abs(displacement)+*prior.PositionBoundM)*(1/math.Cos(angular)-1)
		if math.Abs(displacement) > *prior.PositionBoundM+normalBound {
			return nil, fmt.Errorf("matched edge contradicts the declared prior normal bounds; correct the prior or correspondence")
		}
		result.Centre = PlanarBound{XM: *prior.XM + nx*displacement, YM: *prior.YM + ny*displacement, BoundM: *prior.PositionBoundM + normalBound}
		result.Constraint = "normal_position_given_prior_yaw"
		result.Normal = &[2]float64{nx, ny}
		result.NormalBoundM = &normalBound
		result.TangentBoundM = prior.PositionBoundM
		result.Unresolved = append(result.Unresolved, "tangent_from_prior_not_measured")
		result.MatchedIndices = indices
	}
	values := []float64{result.Centre.XM, result.Centre.YM, result.Centre.BoundM}
	length, width := proposalDimension(body.Length), proposalDimension(body.Width)
	if length == nil || width == nil {
		result.BoxUnavailable = "source body lacks full length/width intervals"
	} else {
		result.Box = &BoxBound{CentreXM: result.Centre.XM, CentreYM: result.Centre.YM, YawRad: *prior.YawRad, LengthM: *length, WidthM: *width}
		result.FrontPrediction = &[2]float64{result.Centre.XM + c**length/2, result.Centre.YM + s**length/2}
		result.RearPrediction = &[2]float64{result.Centre.XM - c**length/2, result.Centre.YM - s**length/2}
		values = append(values, *length, *width, result.FrontPrediction[0], result.FrontPrediction[1], result.RearPrediction[0], result.RearPrediction[1])
	}
	// A finite body interval can still have an infinite midpoint or end.
	for _, v := range values {
		if !finiteFeature(v) {
			return nil, fmt.Errorf("proposal numeric overflow")
		}
	}
	return result, nil
}

// proposalDimension is a full, stated dimension's best value, by the one
// definition DimensionBound.Best gives; nil for an unknown or partial one.
func proposalDimension(d DimensionBound) *float64 {
	if d.Status == EvidenceUnknown || d.Span != SpanFull {
		return nil
	}
	value, _, ok := d.Best()
	if !ok {
		return nil
	}
	return &value
}
