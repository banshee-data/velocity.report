package annotation

import (
	"fmt"
	"math"
	"sort"
	"strings"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// FacetJointPoseMatch names confirmed saved support, not an automatic match.
// Optional indices preserve the valid sample-local return ordinal zero.
type FacetJointPoseMatch struct {
	FeatureID    string  `json:"feature_id"`
	PointIndex   *uint32 `json:"point_index,omitempty"`
	EndIndex     *uint32 `json:"end_index,omitempty"`
	ReturnBoundM float64 `json:"return_bound_m"`
	Confirmed    bool    `json:"correspondence_confirmed"`
	IdentityNote string  `json:"identity_note"`
}

type FacetJointPoseRequest struct {
	SchemaVersion      int                   `json:"schema_version"`
	Pack               string                `json:"pack"`
	PackDigest         string                `json:"pack_digest"`
	FeatureRevision    uint64                `json:"feature_revision"`
	FeatureDigest      string                `json:"feature_digest"`
	MembershipRevision int                   `json:"membership_revision"`
	MembershipDigest   string                `json:"membership_digest"`
	SampleID           int                   `json:"sample_id"`
	TimestampNs        int64                 `json:"timestamp_ns"`
	Author             string                `json:"author"`
	Prior              FacetPosePrior        `json:"prior"`
	Matches            []FacetJointPoseMatch `json:"matches"`
}

func (r FacetJointPoseRequest) single(m FacetJointPoseMatch) FacetPoseRequest {
	return FacetPoseRequest{SchemaVersion: 1, Pack: r.Pack, PackDigest: r.PackDigest, FeatureID: m.FeatureID, FeatureRevision: r.FeatureRevision, FeatureDigest: r.FeatureDigest, MembershipRevision: r.MembershipRevision, MembershipDigest: r.MembershipDigest, SampleID: r.SampleID, TimestampNs: r.TimestampNs, Author: r.Author, Prior: r.Prior, PointIndex: m.PointIndex, EndIndex: m.EndIndex, ReturnBoundM: m.ReturnBoundM, Confirmed: m.Confirmed, IdentityNote: m.IdentityNote}
}

// Support states are geometric evidence, not calibrated match probabilities.
type FacetPoseConfidence struct {
	State    string   `json:"state"`
	Position string   `json:"position"`
	Yaw      string   `json:"yaw"`
	DataRank int      `json:"data_rank"`
	Reasons  []string `json:"reasons"`
}

type FacetPoseContributor struct {
	FeatureID              string   `json:"feature_id"`
	SourcePhysicalRevision uint64   `json:"source_physical_revision"`
	SourcePhysicalDigest   string   `json:"source_physical_digest"`
	SourceSample           uint32   `json:"source_sample"`
	SourceOrigin           string   `json:"source_origin"`
	TargetOrigin           string   `json:"target_origin"`
	MatchedIndices         []uint32 `json:"matched_indices"`
}

type FacetJointPoseProposal struct {
	Schema          string                 `json:"schema"`
	SchemaVersion   int                    `json:"schema_version"`
	Status          string                 `json:"status"`
	Request         FacetJointPoseRequest  `json:"request"`
	ObjectID        string                 `json:"object_id"`
	Body            *BodyGeometry          `json:"body"`
	Centre          PlanarBound            `json:"centre"`
	Yaw             AngleBound             `json:"yaw"`
	Constraint      string                 `json:"constraint"`
	Unresolved      []string               `json:"unresolved"`
	MatchedIndices  []uint32               `json:"matched_indices"`
	Contributors    []FacetPoseContributor `json:"contributors"`
	Confidence      FacetPoseConfidence    `json:"confidence"`
	Normal          *[2]float64            `json:"normal,omitempty"`
	NormalBoundM    *float64               `json:"normal_bound_m,omitempty"`
	TangentBoundM   *float64               `json:"tangent_bound_m,omitempty"`
	Box             *BoxBound              `json:"box,omitempty"`
	BoxUnavailable  string                 `json:"box_unavailable,omitempty"`
	FrontPrediction *[2]float64            `json:"front_prediction,omitempty"`
	RearPrediction  *[2]float64            `json:"rear_prediction,omitempty"`
}

type jointFacetInput struct {
	feature *pb.FeatureCandidate
	match   FacetJointPoseMatch
	result  *FacetPoseProposal
}

// ProposeJointFacetPose extends the existing source-pinned proposal path. It
// reads no target physical pose, saves nothing, and never accepts a reference.
func ProposeJointFacetPose(p *Pack, req FacetJointPoseRequest) (*FacetJointPoseProposal, error) {
	if req.SchemaVersion != 2 || len(req.Matches) < 2 || len(req.Matches) > 4 {
		return nil, fmt.Errorf("joint request version 2 requires two to four saved facets")
	}
	state, err := LoadFeatureRevision(p, req.FeatureRevision)
	if err != nil {
		return nil, err
	}
	if state.Digest != req.FeatureDigest {
		return nil, ErrSidecarConflict
	}
	points, err := p.PointsAt(req.SampleID)
	if err != nil {
		return nil, err
	}
	byID := map[string]*pb.FeatureCandidate{}
	for _, f := range state.Document.Features {
		byID[f.FeatureId] = f
	}
	ids, returns := map[string]bool{}, map[uint32]bool{}
	var inputs []jointFacetInput
	for _, m := range req.Matches {
		if strings.TrimSpace(m.FeatureID) == "" || ids[m.FeatureID] {
			return nil, fmt.Errorf("joint facets must have distinct saved identities")
		}
		ids[m.FeatureID] = true
		result, err := ProposeFacetPose(p, req.single(m))
		if err != nil {
			return nil, fmt.Errorf("facet %s: %w", m.FeatureID, err)
		}
		f := byID[m.FeatureID]
		if len(inputs) > 0 {
			a, b := inputs[0].feature, f
			if a.ObjectId != b.ObjectId || a.Anchor.BodyId != b.Anchor.BodyId || a.Anchor.PhysicalRevision != b.Anchor.PhysicalRevision || a.Anchor.PhysicalDigest != b.Anchor.PhysicalDigest || a.Anchor.PartFrameId != b.Anchor.PartFrameId || a.Anchor.PartFrameRevision != b.Anchor.PartFrameRevision {
				return nil, fmt.Errorf("joint facets require one object, source body revision and part frame")
			}
			for _, old := range inputs {
				if f.Anchor.Line == nil && old.feature.Anchor.Line == nil && f.Anchor.SourceSample == old.feature.Anchor.SourceSample && *f.Anchor.SourcePointIndex == *old.feature.Anchor.SourcePointIndex {
					return nil, fmt.Errorf("duplicate source landmark is not independent support")
				}
			}
		}
		for _, i := range result.MatchedIndices {
			if returns[i] {
				return nil, fmt.Errorf("overlapping target support cannot supply independent anchors")
			}
			returns[i] = true
		}
		inputs = append(inputs, jointFacetInput{f, m, result})
	}
	// Input order is a UI concern; numerical selection and diagnostics are stable.
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].feature.FeatureId < inputs[j].feature.FeatureId })
	return solveJointFacetPose(req, inputs, points)
}

func solveJointFacetPose(req FacetJointPoseRequest, inputs []jointFacetInput, points Points) (*FacetJointPoseProposal, error) {
	prior := req.Prior
	out := &FacetJointPoseProposal{Schema: "velocity.report/facet-joint-pose-proposal", SchemaVersion: 1, Status: "read_only_assisted_proposal", Request: req, ObjectID: inputs[0].feature.ObjectId, Body: inputs[0].result.Body, Yaw: inputs[0].result.Yaw, Unresolved: []string{UnresolvedHeight}, Confidence: FacetPoseConfidence{State: "partial", Position: "partial", Yaw: "prior_only", Reasons: []string{"Support states are not calibrated match probabilities"}}}
	yaw, yawBound := *prior.YawRad, *prior.YawBoundRad
	measured := false
	yawLower, yawUpper := yaw-yawBound, yaw+yawBound
	// Keep one bounded yaw observation; correlated observations never shrink it.
	acceptYaw := func(y, b float64) error {
		if !finiteFeature(y) || !finiteFeature(b) || b <= 0 || b >= math.Pi/2 || jointULP(*prior.YawRad) > b/64 {
			return fmt.Errorf("joint yaw is unresolved at the declared numerical precision")
		}
		y = *prior.YawRad + math.Remainder(y-*prior.YawRad, 2*math.Pi)
		if math.Abs(y-*prior.YawRad) > b+*prior.YawBoundRad+1e-9 {
			return fmt.Errorf("joint yaw contradicts declared prior")
		}
		yawLower, yawUpper = math.Max(yawLower, y-b), math.Min(yawUpper, y+b)
		if yawLower > yawUpper {
			return fmt.Errorf("joint anchor yaw constraints conflict")
		}
		if !measured || b < yawBound {
			yaw, yawBound, measured = y, b, true
		}
		return nil
	}
	var compact []jointFacetInput
	for _, in := range inputs {
		r, a := in.result, in.feature.Anchor
		out.Contributors = append(out.Contributors, FacetPoseContributor{in.feature.FeatureId, a.PhysicalRevision, a.PhysicalDigest, a.SourceSample, a.Origin, r.TargetOrigin, r.MatchedIndices})
		out.MatchedIndices = append(out.MatchedIndices, r.MatchedIndices...)
		if a.Line == nil {
			compact = append(compact, in)
		} else if err := acceptYaw(r.Yaw.Rad, r.Yaw.BoundRad); err != nil {
			return nil, err
		}
	}
	for i, a := range compact {
		for _, b := range compact[i+1:] {
			ax, ay := b.feature.Anchor.XM-a.feature.Anchor.XM, b.feature.Anchor.YM-a.feature.Anchor.YM
			px, py := float64(points.X[*b.match.PointIndex])-float64(points.X[*a.match.PointIndex]), float64(points.Y[*b.match.PointIndex])-float64(points.Y[*a.match.PointIndex])
			source, target := math.Hypot(ax, ay), math.Hypot(px, py)
			sb, tb := a.feature.Anchor.BoundM+b.feature.Anchor.BoundM, a.match.ReturnBoundM+b.match.ReturnBoundM
			if math.Abs(source-target) > sb+tb {
				return nil, fmt.Errorf("matched landmark separation contradicts source body")
			}
			if source <= sb || target <= tb {
				continue
			}
			bound := math.Asin(sb/source) + math.Asin(tb/target)
			if bound >= math.Pi/2 {
				continue
			}
			if err := acceptYaw(math.Atan2(py, px)-math.Atan2(ay, ax), bound); err != nil {
				return nil, err
			}
		}
	}
	out.Yaw = AngleBound{Rad: yaw, BoundRad: yawBound, Axis: AxisResolved, Status: EvidencePriorOnly}
	if measured {
		out.Yaw.Status = EvidenceObserved
		out.Confidence.Yaw = "supported"
	} else {
		out.Unresolved = append(out.Unresolved, UnresolvedYawFromPrior)
		out.Confidence.Reasons = append(out.Confidence.Reasons, "Landmark baseline is unresolved at the declared uncertainty; yaw retained from prior")
	}
	c, s := math.Cos(yaw), math.Sin(yaw)
	var centres []PlanarBound
	var lines []jointNormal
	for _, in := range inputs {
		a, m := in.feature.Anchor, in.match
		if a.Line == nil {
			x, y := float64(points.X[*m.PointIndex]), float64(points.Y[*m.PointIndex])
			centres = append(centres, PlanarBound{XM: x - (c*a.XM - s*a.YM), YM: y - (s*a.XM + c*a.YM), BoundM: a.BoundM + m.ReturnBoundM + math.Hypot(a.XM, a.YM)*2*math.Sin(yawBound/2)})
		} else {
			n, err := jointNormalFor(in, points, prior, yaw, yawBound)
			if err != nil {
				return nil, err
			}
			lines = append(lines, n)
		}
	}
	if len(centres) > 0 {
		// An envelope, not a Gaussian average or a 1/sqrt(N) confidence claim.
		best := centres[0]
		for _, p := range centres[1:] {
			if p.BoundM < best.BoundM {
				best = p
			}
		}
		out.Centre = best
		out.Centre.BoundM = math.Max(best.BoundM, *prior.PositionBoundM)
		for _, p := range centres {
			out.Centre.BoundM = math.Max(out.Centre.BoundM, math.Hypot(p.XM-best.XM, p.YM-best.YM)+p.BoundM)
		}
		out.Constraint = ConstraintPlanarPositionGivenPriorYaw
		out.Confidence.DataRank = 2
		if measured {
			out.Constraint = "joint_planar_pose"
			out.Confidence.Position = "supported"
			out.Confidence.DataRank = 3
			out.Confidence.State = "supported"
		}
	} else {
		// At least two validated line inputs exist when there are no compact inputs.
		bestI, bestJ, det := 0, 0, 0.0
		for i, a := range lines {
			for j := i + 1; j < len(lines); j++ {
				b := lines[j]
				angle := math.Acos(math.Min(1, math.Abs(a.nx*b.nx+a.ny*b.ny)))
				d := a.nx*b.ny - a.ny*b.nx
				if angle > a.angular+b.angular && math.Abs(d) > math.Abs(det) {
					bestI, bestJ, det = i, j, d
				}
			}
		}
		a := lines[bestI]
		if det == 0 {
			shift := a.r - a.nx**prior.XM - a.ny**prior.YM
			out.Centre = PlanarBound{XM: *prior.XM + a.nx*shift, YM: *prior.YM + a.ny*shift, BoundM: *prior.PositionBoundM + a.bound}
			out.Normal = &[2]float64{a.nx, a.ny}
			out.NormalBoundM = fp64(a.bound)
			out.TangentBoundM = prior.PositionBoundM
			out.Constraint = ConstraintNormalPositionAndYaw
			out.Unresolved = append(out.Unresolved, UnresolvedTangentFromPrior)
			out.Confidence.DataRank = 2
			out.Confidence.Reasons = append(out.Confidence.Reasons, "Parallel or uncertainty-overlapping normals leave tangent position at the prior")
		} else {
			b := lines[bestJ]
			out.Centre = PlanarBound{XM: (a.r*b.ny - a.ny*b.r) / det, YM: (a.nx*b.r - a.r*b.nx) / det, BoundM: math.Max(*prior.PositionBoundM, math.Hypot((math.Abs(b.ny)*a.bound+math.Abs(a.ny)*b.bound)/math.Abs(det), (math.Abs(b.nx)*a.bound+math.Abs(a.nx)*b.bound)/math.Abs(det)))}
			out.Constraint = "joint_planar_pose"
			out.Confidence = FacetPoseConfidence{State: "supported", Position: "supported", Yaw: "supported", DataRank: 3, Reasons: out.Confidence.Reasons}
		}
	}
	var tangent *[2]float64
	if out.Normal != nil {
		tangent = &[2]float64{-out.Normal[1], out.Normal[0]}
	}
	x, y, feasible := jointFeasiblePoint(prior, out.Centre, centres, lines, tangent)
	if !feasible {
		return nil, fmt.Errorf("joint point, line and prior bounds have no resolvable common pose")
	}
	dx, dy := x-out.Centre.XM, y-out.Centre.YM
	out.Centre.BoundM += math.Hypot(dx, dy)
	if out.Normal != nil {
		*out.NormalBoundM += math.Abs(out.Normal[0]*dx + out.Normal[1]*dy)
	}
	out.Centre.XM, out.Centre.YM = x, y

	sort.Slice(out.MatchedIndices, func(i, j int) bool { return out.MatchedIndices[i] < out.MatchedIndices[j] })
	return finishJointFacetPose(out)
}

type jointNormal struct{ nx, ny, r, bound, angular float64 }

func jointNormalFor(in jointFacetInput, points Points, prior FacetPosePrior, yaw, yawBound float64) (jointNormal, error) {
	a, m := in.feature.Anchor, in.match
	c, s := math.Cos(yaw), math.Sin(yaw)
	nx, ny := c*a.Line.NormalX-s*a.Line.NormalY, s*a.Line.NormalX+c*a.Line.NormalY
	start, end := *m.PointIndex, *m.EndIndex
	mx, my := (float64(points.X[start])+float64(points.X[end]))/2, (float64(points.Y[start])+float64(points.Y[end]))/2
	angular := yawBound + a.Line.NormalBoundRad
	if angular >= math.Pi/2 {
		return jointNormal{}, fmt.Errorf("joint normal uncertainty is unbounded")
	}
	displacement := nx*(mx-*prior.XM) + ny*(my-*prior.YM) - a.Line.OffsetM
	bound := a.BoundM + m.ReturnBoundM + (math.Hypot(mx-*prior.XM, my-*prior.YM)+math.Abs(a.Line.OffsetM)+*prior.PositionBoundM)*2*math.Sin(angular/2)
	bound = bound/math.Cos(angular) + (math.Abs(displacement)+*prior.PositionBoundM)*(1/math.Cos(angular)-1)
	return jointNormal{nx, ny, nx*mx + ny*my - a.Line.OffsetM, bound, angular}, nil
}

func finishJointFacetPose(out *FacetJointPoseProposal) (*FacetJointPoseProposal, error) {
	values := []float64{out.Centre.XM, out.Centre.YM, out.Centre.BoundM, out.Yaw.Rad, out.Yaw.BoundRad}
	if out.Normal != nil {
		values = append(values, out.Normal[0], out.Normal[1])
	}
	if out.NormalBoundM != nil {
		values = append(values, *out.NormalBoundM)
	}
	if out.TangentBoundM != nil {
		values = append(values, *out.TangentBoundM)
	}
	l, w := proposalDimension(out.Body.Length), proposalDimension(out.Body.Width)
	if l == nil || w == nil {
		out.BoxUnavailable = "source body lacks full length/width intervals"
	} else {
		c, s := math.Cos(out.Yaw.Rad), math.Sin(out.Yaw.Rad)
		out.Box = &BoxBound{CentreXM: out.Centre.XM, CentreYM: out.Centre.YM, YawRad: out.Yaw.Rad, LengthM: *l, WidthM: *w}
		out.FrontPrediction = &[2]float64{out.Centre.XM + c**l/2, out.Centre.YM + s**l/2}
		out.RearPrediction = &[2]float64{out.Centre.XM - c**l/2, out.Centre.YM - s**l/2}
		values = append(values, *l, *w, out.FrontPrediction[0], out.FrontPrediction[1], out.RearPrediction[0], out.RearPrediction[1])
	}
	for _, v := range values {
		if !finiteFeature(v) {
			return nil, fmt.Errorf("joint proposal numeric overflow")
		}
	}
	return out, nil
}
