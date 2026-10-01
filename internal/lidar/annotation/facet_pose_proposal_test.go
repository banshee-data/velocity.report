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
	files := []string{sidecarFile, featureFile, physicalReferenceFile}
	before := map[string][]byte{}
	for _, f := range files {
		before[f], _ = os.ReadFile(filepath.Join(p.Dir, f))
	}
	result, err := ProposeFacetPose(p, req)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.Centre.XM-11) > 1e-6 || math.Abs(result.Centre.YM) > 1e-6 || result.Centre.BoundM < *req.Prior.PositionBoundM || result.Yaw.Rad != *req.Prior.YawRad || result.Status != "read_only_assisted_proposal" || result.Box == nil || result.FrontPrediction == nil {
		t.Fatal(result)
	}
	for _, f := range files {
		after, _ := os.ReadFile(filepath.Join(p.Dir, f))
		if !bytes.Equal(after, before[f]) {
			t.Fatal("proposal wrote", f)
		}
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
	if result.Centre.XM != 10 || result.Centre.YM != 3 || result.Constraint != "normal_position_given_prior_yaw" || *result.TangentBoundM != .5 || result.Box.LengthM != 4.3 || !reflect.DeepEqual(result.Body, body) {
		t.Fatal(result)
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

func TestFacetPoseSolverRefusesUnsupportedInputs(t *testing.T) {
	_, original := poseRequest(t)
	original.Prior.XM = fp(7)
	original.Prior.YM = fp(-.9)
	base := &pb.FeatureCandidate{Anchor: &pb.FeatureAnchor{XM: 1, BoundM: .1}}
	target := &pb.FeatureObservation{PointIndices: []uint32{0, 1, 2}}
	pts := straightSegment()
	for _, name := range []string{"missing prior", "nonfinite prior", "negative prior", "wide prior", "missing source", "return bound", "missing point", "uncertain point", "outside", "nonfinite point", "compact end", "missing end", "uncertain end", "curve", "wide line", "contradiction", "prior conflict", "line prior conflict", "overflow"} {
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
				os.WriteFile(filepath.Join(p.Dir, sidecarFile), []byte("bad JSON"), 0600)
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
					if err = SaveSidecar(p, membership); err != nil {
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

// A deterministic Go-produced wire fixture shared with native decoding tests.
func TestFacetPoseProposalSharedFixture(t *testing.T) {
	req := FacetPoseRequest{SchemaVersion: 1, Pack: "clip/pack", PackDigest: "sha256:pack", FeatureID: "tip", FeatureRevision: 7, FeatureDigest: "sha256:feature", MembershipRevision: 3, MembershipDigest: "sha256:members", SampleID: 2, TimestampNs: 9007199254740993, PointIndex: ptrU32(0), ReturnBoundM: .05, Confirmed: true, Author: "op", IdentityNote: "outer rigid tip and hard return bound", Prior: FacetPosePrior{XM: fp(9.8), YM: fp(5.2), YawRad: fp(math.Pi / 2), PositionBoundM: fp(.4), YawBoundRad: fp(.03), Source: "declared prior with conservative hard bounds"}}
	feature := &pb.FeatureCandidate{ObjectId: "car", Anchor: &pb.FeatureAnchor{BodyId: "body", PhysicalRevision: 5, PhysicalDigest: "sha256:physical", SourceSample: 0, Origin: "reference_seeded_proposal", XM: 2, YM: -1, BoundM: .2}}
	target := &pb.FeatureObservation{PointIndices: []uint32{0, 1}, Origin: "human_proposal"}
	body := &BodyGeometry{BodyID: "body", AxisConvention: BodyAxisConvention, Length: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(4), UpperM: fp(5), ValueM: fp(4.3), Support: EvidenceSupport{External: "source body"}}, Width: DimensionBound{Status: EvidencePriorOnly, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(1.9), Support: EvidenceSupport{External: "source width"}}, Height: DimensionBound{Status: EvidenceUnknown}, Review: independentReview()}
	result, err := solveFacetPose(feature, target, body, Points{X: []float32{11, 12}, Y: []float32{7, 7}, Z: []float32{1, 1}}, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if path := os.Getenv("FACET_POSE_FIXTURE_PATH"); path != "" {
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile("testdata/facet-pose-proposal.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, b) {
		t.Fatal("shared offline pose contract fixture changed")
	}
}
