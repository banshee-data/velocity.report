package annotation

import (
	"bytes"
	"encoding/json"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func poseRequest(t *testing.T) (*Pack, FacetPoseRequest) {
	t.Helper()
	p, edit := registeredFeatureFixture(t)
	state, err := SaveFeatures(p, edit)
	if err != nil {
		t.Fatal(err)
	}
	return p, FacetPoseRequest{SchemaVersion: 1, PackDigest: p.Manifest.PackDigest, FeatureID: "mirror", FeatureRevision: state.Document.Revision, FeatureDigest: state.Digest, MembershipRevision: 1, MembershipDigest: state.MembershipDigest, SampleID: 1, TimestampNs: physTime(1), PointIndex: ptrU32(0), ReturnBoundM: .05, Confirmed: true, Author: "op", IdentityNote: "same rigid tip", Prior: FacetPosePrior{XM: fp(10.8), YM: fp(0), YawRad: fp(0), PositionBoundM: fp(.5), YawBoundRad: fp(.01), Source: "explicit held prior"}}
}
func ptrU32(x uint32) *uint32 { return &x }

func TestFacetPoseProposalUsesSavedSpotAndNeverMutatesOrRequiresTargetTruth(t *testing.T) {
	p, req := poseRequest(t)
	files := []string{featureFile, physicalReferenceFile}
	before := map[string][]byte{}
	for _, f := range files {
		before[f], _ = os.ReadFile(filepath.Join(p.Dir, f))
	}
	membershipBefore := membershipHead(p)
	result, err := ProposeFacetPose(p, req)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.Centre.XM-11) > 1e-6 || math.Abs(result.Centre.YM) > 1e-6 || result.Centre.BoundM < *req.Prior.PositionBoundM || result.Yaw.Rad != *req.Prior.YawRad || result.Status != "read_only_assisted_proposal" || result.Box == nil || result.FrontPrediction == nil {
		t.Fatal(result)
	}
	// One return cannot measure yaw: a compact spot keeps the prior's.
	if result.SchemaVersion != FacetPoseProposalSchemaVersion || result.Constraint != ConstraintPlanarPositionGivenPriorYaw || result.Yaw.Status != EvidencePriorOnly || result.Yaw.BoundRad != *req.Prior.YawBoundRad || !reflect.DeepEqual(result.Unresolved, []string{"yaw_from_prior_not_measured", "height_not_measured"}) {
		t.Fatal("compact spot claimed a measured yaw", result.Yaw, result.Unresolved)
	}
	for _, f := range files {
		after, _ := os.ReadFile(filepath.Join(p.Dir, f))
		if !bytes.Equal(after, before[f]) {
			t.Fatal("proposal wrote", f)
		}
	}
	if !bytes.Equal(membershipHead(p), membershipBefore) {
		t.Fatal("proposal changed membership")
	}
	refs, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	// Delete every target reference. Keep only the pinned source at sample zero.
	for i := range refs.Objects {
		var source []PhysicalKeyframe
		for _, k := range refs.Objects[i].Keyframes {
			if k.SampleID == 0 {
				source = append(source, k)
			}
		}
		refs.Objects[i].Keyframes = source
	}
	if err = SavePhysicalReferences(p, refs); err != nil {
		t.Fatal(err)
	}
	without, err := ProposeFacetPose(p, req)
	if err != nil || !reflect.DeepEqual(result, without) {
		t.Fatal("target truth influenced proposal", err)
	}
	// Reopening is an exact recomputation, not promotion into a reference.
	reopened, err := OpenPack(p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ProposeFacetPose(reopened, req)
	if err != nil || !reflect.DeepEqual(result, again) {
		t.Fatal(err)
	}
}

func TestFacetPoseSolverLineObservabilityIgnoresTangentSampling(t *testing.T) {
	_, req := poseRequest(t)
	feature := &pb.FeatureCandidate{ObjectId: "car", Anchor: &pb.FeatureAnchor{BodyId: "body", BoundM: .2, Line: &pb.FeatureLineConstraint{NormalY: 1, OffsetM: -1, NormalBoundRad: .02}}}
	target := &pb.FeatureObservation{PointIndices: []uint32{0, 1, 2}, Origin: "assisted_proposal"}
	body := &BodyGeometry{Length: DimensionBound{Status: EvidencePriorOnly, Span: SpanFull, LowerM: fp(4), UpperM: fp(5), ValueM: fp(4.3)}, Width: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(2), UpperM: fp(2.2)}}
	req.EndIndex = ptrU32(2)
	req.Prior.XM = fp(10)
	req.Prior.YM = fp(2.8)
	points := Points{X: []float32{8, 9, 10}, Y: []float32{2, 2, 2}, Z: []float32{1, 1, 1}}
	result, err := solveFacetPose(feature, target, body, points, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Centre.XM != 10 || result.Centre.YM != 3 || result.Constraint != ConstraintNormalPositionAndYaw || *result.TangentBoundM != .5 || result.Box.LengthM != 4.3 || !reflect.DeepEqual(result.Body, body) {
		t.Fatal(result)
	}
	if result.Yaw.Status != EvidenceObserved || result.Yaw.Rad != 0 || !reflect.DeepEqual(result.Unresolved, []string{"height_not_measured", "tangent_from_prior_not_measured"}) {
		t.Fatal("segment did not report its measured yaw", result.Yaw, result.Unresolved)
	}
	points.X = []float32{100, 101, 102}
	moved, err := solveFacetPose(feature, target, body, points, req)
	if err != nil || moved.Centre.XM != result.Centre.XM || moved.Centre.YM != result.Centre.YM {
		t.Fatal("visible midpoint became tangent anchor", err)
	}
	target.UncertainIndices = []uint32{1}
	if _, err = solveFacetPose(feature, target, body, points, req); err == nil {
		t.Fatal("sparse definite support accepted")
	}
}

// rotatedSegmentCase places a body whose body-frame line has normal angle
// normalRad and offset offsetM at centre (12, 3) and true yaw trueYaw, and
// samples three returns on that line in the target frame. The middle return
// sags in z, as a constant-elevation ring does across a flat side.
func rotatedSegmentCase(normalRad, offsetM, trueYaw float64) (*pb.FeatureCandidate, *pb.FeatureObservation, Points) {
	feature := &pb.FeatureCandidate{ObjectId: "car", Anchor: &pb.FeatureAnchor{BodyId: "body", BoundM: .2, Line: &pb.FeatureLineConstraint{NormalX: math.Cos(normalRad), NormalY: math.Sin(normalRad), OffsetM: offsetM, NormalBoundRad: .06}}}
	world := normalRad + trueYaw
	nx, ny := math.Cos(world), math.Sin(world)
	var points Points
	for i, along := range []float64{-1.5, .2, 1.5} {
		points.X = append(points.X, float32(12+offsetM*nx-along*ny))
		points.Y = append(points.Y, float32(3+offsetM*ny+along*nx))
		points.Z = append(points.Z, float32(1-.2*float64(i%2)))
	}
	return feature, &pb.FeatureObservation{PointIndices: []uint32{0, 1, 2}, Origin: "human_proposal"}, points
}

func segmentPoseRequest(t *testing.T, yaw, yawBound float64) FacetPoseRequest {
	t.Helper()
	_, req := poseRequest(t)
	req.EndIndex = ptrU32(2)
	req.Prior.XM, req.Prior.YM = fp(12.1), fp(2.95)
	req.Prior.YawRad, req.Prior.YawBoundRad = fp(yaw), fp(yawBound)
	return req
}

func TestFacetPoseSegmentMeasuresYawIndependentlyOfThePriorBound(t *testing.T) {
	const trueYaw = .7
	body := &BodyGeometry{Length: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(4), UpperM: fp(5)}, Width: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(1.9)}}
	feature, target, points := rotatedSegmentCase(.4, .9, trueYaw)
	// The prior is off by 0.06 rad (3.4 degrees) inside its 0.08 rad bound.
	req := segmentPoseRequest(t, trueYaw+.06, .08)
	result, err := solveFacetPose(feature, target, body, points, req)
	if err != nil {
		t.Fatal(err)
	}
	targetLine, _, err := featureSegmentRelation(points, target.PointIndices, 0, 2, PlanarBound{}, AngleBound{}, req.ReturnBoundM)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.Yaw.Rad-trueYaw) > 1e-6 || result.Yaw.Status != EvidenceObserved || result.Yaw.Axis != AxisResolved {
		t.Fatalf("measured yaw %v, want %v", result.Yaw, trueYaw)
	}
	if want := feature.Anchor.Line.NormalBoundRad + targetLine.NormalBoundRad; result.Yaw.BoundRad != want {
		t.Fatalf("yaw bound %v, want the two normal bounds %v", result.Yaw.BoundRad, want)
	}
	for _, u := range result.Unresolved {
		if u == "yaw_from_prior_not_measured" {
			t.Fatal("segment still claims its yaw is the prior's")
		}
	}
	// The predictions follow the measured yaw, not the prior's.
	if result.Box.YawRad != result.Yaw.Rad || math.Abs(math.Atan2(result.FrontPrediction[1]-result.Centre.YM, result.FrontPrediction[0]-result.Centre.XM)-trueYaw) > 1e-6 {
		t.Fatal("box or end prediction kept the prior yaw", result.Box, result.FrontPrediction)
	}
	// The normal is the target's normal up to sign, and the true centre
	// satisfies the body-frame relation along it within the stated bound.
	if math.Abs(math.Abs(result.Normal[0]*targetLine.NormalX+result.Normal[1]*targetLine.NormalY)-1) > 1e-9 {
		t.Fatal("normal not rotated by the measured yaw", result.Normal)
	}
	if offset := result.Normal[0]*(float64(points.X[1])-result.Centre.XM) + result.Normal[1]*(float64(points.Y[1])-result.Centre.YM); math.Abs(offset-.9) > 1e-5 {
		t.Fatal("centre does not satisfy the body relation", offset)
	}
	// Widening the prior's yaw bound changes neither the measurement nor the
	// normal bound: the prior's yaw bound no longer enters either.
	wider := segmentPoseRequest(t, trueYaw+.06, .3)
	again, err := solveFacetPose(feature, target, body, points, wider)
	if err != nil {
		t.Fatal(err)
	}
	if again.Yaw != result.Yaw || *again.NormalBoundM != *result.NormalBoundM || again.Centre != result.Centre {
		t.Fatal("prior yaw bound leaked into the measured yaw or normal bound", again.Yaw, *again.NormalBoundM, *result.NormalBoundM)
	}
}

func TestFacetPoseSegmentYawFoldsToTheModeNearestThePrior(t *testing.T) {
	const trueYaw = .7
	body := &BodyGeometry{}
	// A reversed body-frame normal is the same line; the fold recovers the
	// same yaw from it.
	feature, target, points := rotatedSegmentCase(.4, .9, trueYaw)
	feature.Anchor.Line.NormalX, feature.Anchor.Line.NormalY, feature.Anchor.Line.OffsetM = -feature.Anchor.Line.NormalX, -feature.Anchor.Line.NormalY, -feature.Anchor.Line.OffsetM
	result, err := solveFacetPose(feature, target, body, points, segmentPoseRequest(t, trueYaw-.05, .08))
	if err != nil || math.Abs(result.Yaw.Rad-trueYaw) > 1e-6 {
		t.Fatal("reversed normal changed the measured yaw", err, result)
	}
	// A line through the centre cannot tell the body from its half-turn; the
	// prior picks the mode, and the result stays beside the prior, not wrapped.
	feature, target, points = rotatedSegmentCase(.4, 0, trueYaw)
	for _, tc := range []struct{ prior, want float64 }{
		{trueYaw + .05, trueYaw},
		{trueYaw + math.Pi + .05, trueYaw + math.Pi},
		{trueYaw - math.Pi - .05, trueYaw - math.Pi},
		{trueYaw + 4*math.Pi - .05, trueYaw + 4*math.Pi},
	} {
		result, err = solveFacetPose(feature, target, body, points, segmentPoseRequest(t, tc.prior, .08))
		if err != nil {
			t.Fatal(tc.prior, err)
		}
		if math.Abs(result.Yaw.Rad-tc.want) > 1e-6 {
			t.Fatalf("prior %v: yaw %v, want %v", tc.prior, result.Yaw.Rad, tc.want)
		}
	}
}

func TestFacetPoseSegmentRefusesAYawThatContradictsThePrior(t *testing.T) {
	feature, target, points := rotatedSegmentCase(.4, .9, .7)
	// Measured bound ~0.094 plus the prior's 0.05 is under the 0.3 offset.
	_, err := solveFacetPose(feature, target, &BodyGeometry{}, points, segmentPoseRequest(t, 1, .05))
	if err == nil || !strings.Contains(err.Error(), "contradicts the prior yaw") {
		t.Fatal("contradicting yaw accepted", err)
	}
	// Bounds that cannot choose between the two modes are refused outright.
	if _, err = solveFacetPose(feature, target, &BodyGeometry{}, points, segmentPoseRequest(t, .7, math.Pi/2)); err == nil || !strings.Contains(err.Error(), "unconstrained") {
		t.Fatal("ambiguous mode accepted", err)
	}
	// A zero body normal has no angle; it must not read as yaw zero.
	feature.Anchor.Line.NormalX, feature.Anchor.Line.NormalY = 0, 0
	if _, err = solveFacetPose(feature, target, &BodyGeometry{}, points, segmentPoseRequest(t, .7, .08)); err == nil || !strings.Contains(err.Error(), "unit vector") {
		t.Fatal("degenerate body normal accepted", err)
	}
}

func TestFacetPoseSolverRefusesUnsupportedInputs(t *testing.T) {
	_, original := poseRequest(t)
	original.Prior.XM = fp(7)
	original.Prior.YM = fp(-.9)
	base := &pb.FeatureCandidate{Anchor: &pb.FeatureAnchor{XM: 1, BoundM: .1}}
	target := &pb.FeatureObservation{PointIndices: []uint32{0, 1, 2}}
	pts := straightSegment()
	for _, name := range []string{"missing prior", "nonfinite prior", "negative prior", "wide prior", "missing source", "return bound", "missing point", "uncertain point", "outside", "nonfinite point", "compact end", "missing end", "uncertain end", "curve", "wide line", "degenerate normal", "contradiction", "prior conflict", "line prior conflict", "overflow"} {
		t.Run(name, func(t *testing.T) {
			req := original
			req.Prior = original.Prior
			feature := proto.Clone(base).(*pb.FeatureCandidate)
			obs := proto.Clone(target).(*pb.FeatureObservation)
			points := Points{X: append([]float32(nil), pts.X...), Y: append([]float32(nil), pts.Y...), Z: append([]float32(nil), pts.Z...)}
			switch name {
			case "missing prior":
				req.Prior.XM = nil
			case "nonfinite prior":
				req.Prior.YM = fp(math.Inf(1))
			case "negative prior":
				req.Prior.PositionBoundM = fp(-1)
			case "wide prior":
				req.Prior.YawBoundRad = fp(4)
			case "missing source":
				req.Prior.Source = " "
			case "return bound":
				req.ReturnBoundM = 0
			case "missing point":
				req.PointIndex = nil
			case "uncertain point":
				obs.UncertainIndices = []uint32{0}
			case "outside":
				req.PointIndex = ptrU32(99)
				obs.PointIndices = append(obs.PointIndices, 99)
			case "nonfinite point":
				points.Z[0] = float32(math.NaN())
			case "compact end":
				req.EndIndex = ptrU32(2)
			case "prior conflict":
				req.Prior.XM = fp(100)
			case "overflow":
				feature.Anchor.BoundM = math.MaxFloat64
				req.ReturnBoundM = math.MaxFloat64
			default:
				feature.Anchor.Line = &pb.FeatureLineConstraint{NormalY: 1, NormalBoundRad: .02}
				req.EndIndex = ptrU32(2)
				switch name {
				case "missing end":
					req.EndIndex = nil
				case "uncertain end":
					obs.UncertainIndices = []uint32{2}
				case "curve":
					points.Y[1] += 1
				case "wide line":
					feature.Anchor.Line.NormalBoundRad = math.Pi / 2
				case "degenerate normal":
					feature.Anchor.Line.NormalY = 0
				case "line prior conflict":
					req.Prior.YM = fp(100)
				case "contradiction":
					feature.Anchor.Line.NormalX = 1
					feature.Anchor.Line.NormalY = 0
				}
			}
			if _, err := solveFacetPose(feature, obs, &BodyGeometry{}, points, req); err == nil {
				t.Fatal("accepted", name)
			}
		})
	}
	feature := proto.Clone(base).(*pb.FeatureCandidate)
	result, err := solveFacetPose(feature, target, &BodyGeometry{}, pts, original)
	if err != nil || result.Box != nil || result.BoxUnavailable == "" {
		t.Fatal(err, result)
	}
	if proposalDimension(DimensionBound{Status: EvidenceObserved, Span: SpanPartial, LowerM: fp(2)}) != nil {
		t.Fatal("partial dimension invented a full body")
	}
	if proposalDimension(DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(2)}) != nil {
		t.Fatal("unbounded full dimension invented an upper end")
	}
}

// The box takes a dimension's best value by DimensionBound.Best, the one
// definition the physical geometry and the Swift port share.
func TestFacetPoseBoxUsesTheDimensionsBestValue(t *testing.T) {
	for _, d := range []DimensionBound{
		{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(1.9)},
		{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(4.1), UpperM: fp(4.7), ValueM: fp(4.3)},
	} {
		want, _, _ := d.Best()
		if got := proposalDimension(d); got == nil || *got != want {
			t.Fatalf("%+v: %v, want %v", d, got, want)
		}
	}
	// A finite interval whose midpoint overflows is refused, not proposed as an infinite box.
	_, req := poseRequest(t)
	req.Prior.XM, req.Prior.YM = fp(7), fp(-.9)
	feature := &pb.FeatureCandidate{Anchor: &pb.FeatureAnchor{XM: 1, BoundM: .1}}
	target := &pb.FeatureObservation{PointIndices: []uint32{0, 1, 2}}
	huge := DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(math.MaxFloat64), UpperM: fp(math.MaxFloat64)}
	body := &BodyGeometry{Length: huge, Width: DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(1.9)}}
	if _, err := solveFacetPose(feature, target, body, straightSegment(), req); err == nil || err.Error() != "proposal numeric overflow" {
		t.Fatalf("infinite box proposed: %v", err)
	}
	body.Length = DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(4), UpperM: fp(5)}
	if result, err := solveFacetPose(feature, target, body, straightSegment(), req); err != nil || result.Box == nil || result.Box.LengthM != 4.5 {
		t.Fatal(err, result)
	}
}

func TestFacetPoseProposalRejectsStaleAndAbsentEvidence(t *testing.T) {
	for _, name := range []string{"schema", "pack", "unconfirmed", "author", "identity", "feature revision", "feature digest", "membership", "feature missing", "target missing", "source frame", "time", "physical unreadable", "membership unreadable", "invalid sample", "inactive", "absent", "stale target"} {
		t.Run(name, func(t *testing.T) {
			p, req := poseRequest(t)
			switch name {
			case "schema":
				req.SchemaVersion = 2
			case "pack":
				req.PackDigest = "wrong"
			case "unconfirmed":
				req.Confirmed = false
			case "author":
				req.Author = " "
			case "identity":
				req.IdentityNote = " "
			case "feature revision":
				req.FeatureRevision = 99
			case "feature digest":
				req.FeatureDigest = "wrong"
			case "membership":
				req.MembershipRevision++
			case "feature missing":
				req.FeatureID = "other"
			case "source frame":
				req.SampleID = 0
				req.TimestampNs = physTime(0)
			case "target missing":
				req.SampleID = 2
			case "time":
				req.TimestampNs++
			case "membership unreadable":
				breakMembership(p)
			case "physical unreadable":
				os.Remove(filepath.Join(p.Dir, physicalReferenceFile))
			case "invalid sample":
				req.SampleID = -1
			default:
				state, err := LoadFeatures(p)
				if err != nil {
					t.Fatal(err)
				}
				f := state.Document.Features[0]
				switch name {
				case "inactive":
					f.Inactive = true
				case "absent":
					f.Observations[1].Decision = pb.FeatureDecision_FEATURE_DECISION_OCCLUDED
					f.Observations[1].PointIndices = nil
					f.Observations[1].Sphere = nil
					f.Anchor = nil
					f.PartRelation = "unknown"
				case "stale target":
					membership, err := LoadSidecar(p)
					if err != nil {
						t.Fatal(err)
					}
					membership.Masks[1].UncertainIndices = []int{7}
					if err = saveMembership(p, membership); err != nil {
						t.Fatal(err)
					}
					req.MembershipRevision = membership.Revision
					req.MembershipDigest = membership.Digest()
				}
				if name != "stale target" {
					state, err = SaveFeatures(p, &pb.FeatureEdit{Document: state.Document, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest})
				}
				if err != nil {
					t.Fatal(err)
				}
				req.FeatureRevision = state.Document.Revision
				req.FeatureDigest = state.Digest
			}
			if _, err := ProposeFacetPose(p, req); err == nil {
				t.Fatal("accepted", name)
			}
		})
	}
}

func TestFacetPoseProposalJSONPreservesZeroAndLargeTimestamp(t *testing.T) {
	_, req := poseRequest(t)
	req.TimestampNs = 9007199254740993
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got FacetPoseRequest
	if err = json.Unmarshal(b, &got); err != nil || got.TimestampNs != req.TimestampNs || got.PointIndex == nil || *got.PointIndex != 0 {
		t.Fatal(err)
	}
}

// Deterministic Go-produced wire fixtures shared with native decoding tests:
// a compact spot at the prior yaw and a segment that measures yaw. Set
// FACET_POSE_FIXTURE_DIR to regenerate both into that directory.
func TestFacetPoseProposalSharedFixture(t *testing.T) {
	body := &BodyGeometry{BodyID: "body", AxisConvention: BodyAxisConvention, Length: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(4), UpperM: fp(5), ValueM: fp(4.3), Support: EvidenceSupport{External: "source body"}}, Width: DimensionBound{Status: EvidencePriorOnly, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(1.9), Support: EvidenceSupport{External: "source width"}}, Height: DimensionBound{Status: EvidenceUnknown}, Review: independentReview()}
	request := func(note string) FacetPoseRequest {
		return FacetPoseRequest{SchemaVersion: 1, Pack: "clip/pack", PackDigest: "sha256:pack", FeatureID: "tip", FeatureRevision: 7, FeatureDigest: "sha256:feature", MembershipRevision: 3, MembershipDigest: "sha256:members", SampleID: 2, TimestampNs: 9007199254740993, PointIndex: ptrU32(0), ReturnBoundM: .05, Confirmed: true, Author: "op", IdentityNote: note, Prior: FacetPosePrior{XM: fp(9.8), YM: fp(5.2), YawRad: fp(math.Pi / 2), PositionBoundM: fp(.4), YawBoundRad: fp(.03), Source: "declared prior with conservative hard bounds"}}
	}
	spot := request("outer rigid tip and hard return bound")
	segment := request("straight left side, flat panel, hard return bound")
	segment.EndIndex = ptrU32(2)
	cases := []struct {
		file    string
		feature *pb.FeatureCandidate
		target  *pb.FeatureObservation
		points  Points
		req     FacetPoseRequest
	}{
		{"facet-pose-proposal.json", &pb.FeatureCandidate{ObjectId: "car", Anchor: &pb.FeatureAnchor{BodyId: "body", PhysicalRevision: 5, PhysicalDigest: "sha256:physical", SourceSample: 0, Origin: "reference_seeded_proposal", XM: 2, YM: -1, BoundM: .2}}, &pb.FeatureObservation{PointIndices: []uint32{0, 1}, Origin: "human_proposal"}, Points{X: []float32{11, 12}, Y: []float32{7, 7}, Z: []float32{1, 1}}, spot},
		// The body's left side (body normal +y, offset 0.9 m) seen 0.02 rad
		// off the prior yaw; the middle return sits 0.2 m lower on the panel.
		{"facet-pose-proposal-segment.json", &pb.FeatureCandidate{ObjectId: "car", Anchor: &pb.FeatureAnchor{BodyId: "body", PhysicalRevision: 5, PhysicalDigest: "sha256:physical", SourceSample: 0, Origin: "reference_seeded_proposal", BoundM: .2, Line: &pb.FeatureLineConstraint{NormalY: 1, OffsetM: .9, NormalBoundRad: .06}}}, &pb.FeatureObservation{PointIndices: []uint32{0, 1, 2}, Origin: "human_proposal"}, Points{X: []float32{8.9, 8.88, 8.86}, Y: []float32{4, 5, 6}, Z: []float32{.7, .5, .7}}, segment},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			result, err := solveFacetPose(tc.feature, tc.target, body, tc.points, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			b = append(b, '\n')
			if dir := os.Getenv("FACET_POSE_FIXTURE_DIR"); dir != "" {
				if err = os.WriteFile(filepath.Join(dir, tc.file), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			expected, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(expected, b) {
				t.Fatal("shared offline pose contract fixture changed")
			}
		})
	}
}
