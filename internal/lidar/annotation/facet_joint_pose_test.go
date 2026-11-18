package annotation

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

func jointPoseFixture(t *testing.T, change func(*Pack, *pb.FeatureEdit)) (*Pack, FacetJointPoseRequest) {
	t.Helper()
	p, e := registeredFeatureFixture(t)
	second := proto.Clone(e.Document.Features[0]).(*pb.FeatureCandidate)
	second.FeatureId = "front-tip"
	second.Anchor.XM = 2.25
	second.Anchor.SourcePointIndex = ptrU32(4)
	for _, o := range second.Observations {
		o.PointIndices = []uint32{4, 5}
		o.Sphere.XM += 4.5
	}
	e.Document.Features = append(e.Document.Features, second)
	if change != nil {
		change(p, e)
	}
	state, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	return p, FacetJointPoseRequest{SchemaVersion: 2, Pack: "kirk0/packs/joint", PackDigest: p.Manifest.PackDigest, FeatureRevision: state.Document.Revision, FeatureDigest: state.Digest, MembershipRevision: 1, MembershipDigest: state.MembershipDigest, SampleID: 1, TimestampNs: physTime(1), Author: "op", Prior: FacetPosePrior{XM: fp(10.8), YM: fp(0), YawRad: fp(.01), PositionBoundM: fp(.5), YawBoundRad: fp(.03), Source: "explicit prior"}, Matches: []FacetJointPoseMatch{{FeatureID: "mirror", PointIndex: ptrU32(0), ReturnBoundM: .05, Confirmed: true, IdentityNote: "rear rigid corner"}, {FeatureID: "front-tip", PointIndex: ptrU32(4), ReturnBoundM: .05, Confirmed: true, IdentityNote: "front rigid corner"}}}
}

func TestJointFacetPoseUsesTwoSavedLandmarksWithoutTargetTruthOrWrites(t *testing.T) {
	p, req := jointPoseFixture(t, nil)
	before := map[string][]byte{}
	for _, name := range []string{featureFile, physicalReferenceFile} {
		before[name], _ = os.ReadFile(filepath.Join(p.Dir, name))
	}
	membership := membershipHead(p)
	got, err := ProposeJointFacetPose(p, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != "velocity.report/facet-joint-pose-proposal" || got.SchemaVersion != 1 || got.Status != "read_only_assisted_proposal" || got.Confidence.State != "supported" || got.Confidence.DataRank != 3 || got.Confidence.Position != "supported" || got.Yaw.Status != EvidenceObserved || math.Abs(got.Yaw.Rad) > 1e-8 || math.Abs(got.Centre.XM-11) > 1e-6 || got.Centre.BoundM < *req.Prior.PositionBoundM || got.Box == nil || got.Box.YawRad != got.Yaw.Rad || got.FrontPrediction == nil || got.RearPrediction == nil || len(got.Contributors) != 2 || !reflect.DeepEqual(got.MatchedIndices, []uint32{0, 4}) {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(got.Unresolved, []string{UnresolvedHeight}) {
		t.Fatal(got.Unresolved)
	}
	for _, name := range []string{featureFile, physicalReferenceFile} {
		after, _ := os.ReadFile(filepath.Join(p.Dir, name))
		if !bytes.Equal(after, before[name]) {
			t.Fatal("wrote", name)
		}
	}
	if !bytes.Equal(membershipHead(p), membership) {
		t.Fatal("changed object masks")
	}
	// Deleting target reference poses does not change an estimate from source evidence.
	refs, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := range refs.Objects {
		refs.Objects[i].Keyframes = refs.Objects[i].Keyframes[:1]
	}
	if err = SavePhysicalReferences(p, refs); err != nil {
		t.Fatal(err)
	}
	again, err := ProposeJointFacetPose(p, req)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("target reference influenced joint fit", err)
	}
	// Numerical result and contributor order are independent of basket order.
	req.Matches = append([]FacetJointPoseMatch(nil), req.Matches...)
	req.Matches[0], req.Matches[1] = req.Matches[1], req.Matches[0]
	reversed, err := ProposeJointFacetPose(p, req)
	if err != nil {
		t.Fatal(err)
	}
	reversed.Request = got.Request
	if !reflect.DeepEqual(got, reversed) {
		t.Fatal("match order changed result")
	}
}

func TestJointFacetPoseRejectsInvalidEvidence(t *testing.T) {
	p, base := jointPoseFixture(t, nil)
	for _, tc := range []struct {
		name string
		edit func(*FacetJointPoseRequest)
	}{
		{"version", func(r *FacetJointPoseRequest) { r.SchemaVersion = 1 }},
		{"one", func(r *FacetJointPoseRequest) { r.Matches = r.Matches[:1] }},
		{"five", func(r *FacetJointPoseRequest) {
			r.Matches = append(r.Matches, r.Matches[0], r.Matches[0], r.Matches[0])
		}},
		{"empty id", func(r *FacetJointPoseRequest) { r.Matches[0].FeatureID = " " }},
		{"duplicate id", func(r *FacetJointPoseRequest) { r.Matches[1].FeatureID = r.Matches[0].FeatureID }},
		{"missing id", func(r *FacetJointPoseRequest) { r.Matches[1].FeatureID = "missing" }},
		{"feature hash", func(r *FacetJointPoseRequest) { r.FeatureDigest = "stale" }},
		{"feature revision", func(r *FacetJointPoseRequest) { r.FeatureRevision = 0 }},
		{"membership hash", func(r *FacetJointPoseRequest) { r.MembershipDigest = "stale" }},
		{"membership revision", func(r *FacetJointPoseRequest) { r.MembershipRevision++ }},
		{"pack hash", func(r *FacetJointPoseRequest) { r.PackDigest = "other" }},
		{"timestamp", func(r *FacetJointPoseRequest) { r.TimestampNs++ }},
		{"source frame", func(r *FacetJointPoseRequest) { r.SampleID = 0; r.TimestampNs = physTime(0) }},
		{"outside frame", func(r *FacetJointPoseRequest) { r.SampleID = 100 }},
		{"unconfirmed", func(r *FacetJointPoseRequest) { r.Matches[0].Confirmed = false }},
		{"identity", func(r *FacetJointPoseRequest) { r.Matches[0].IdentityNote = " " }},
		{"author", func(r *FacetJointPoseRequest) { r.Author = " " }},
		{"nil index", func(r *FacetJointPoseRequest) { r.Matches[0].PointIndex = nil }},
		{"wrong index", func(r *FacetJointPoseRequest) { r.Matches[0].PointIndex = ptrU32(7) }},
		{"compact end", func(r *FacetJointPoseRequest) { r.Matches[0].EndIndex = ptrU32(1) }},
		{"nan prior", func(r *FacetJointPoseRequest) { r.Prior.XM = fp(math.NaN()) }},
		{"missing prior", func(r *FacetJointPoseRequest) { r.Prior.YawRad = nil }},
		{"nan return", func(r *FacetJointPoseRequest) { r.Matches[0].ReturnBoundM = math.NaN() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.Matches = append([]FacetJointPoseMatch(nil), base.Matches...)
			tc.edit(&r)
			if _, err := ProposeJointFacetPose(p, r); err == nil {
				t.Fatal("accepted", tc.name)
			}
		})
	}
}

func TestJointFacetPoseRejectsCorrelatedOrMixedPins(t *testing.T) {
	for _, name := range []string{"overlap", "duplicate source", "mixed physical", "inactive", "uncertain", "missing", "rejected"} {
		t.Run(name, func(t *testing.T) {
			p, r := jointPoseFixture(t, func(p *Pack, e *pb.FeatureEdit) {
				second := e.Document.Features[1]
				switch name {
				case "overlap":
					second.Observations[1] = proto.Clone(e.Document.Features[0].Observations[1]).(*pb.FeatureObservation)
				case "duplicate source":
					second.Anchor = proto.Clone(e.Document.Features[0].Anchor).(*pb.FeatureAnchor)
					second.Observations[0] = proto.Clone(e.Document.Features[0].Observations[0]).(*pb.FeatureObservation)
				case "mixed physical":
					refs, err := LoadPhysicalReferences(p)
					if err != nil {
						t.Fatal(err)
					}
					if err = SavePhysicalReferences(p, refs); err != nil {
						t.Fatal(err)
					}
					second.Anchor.PhysicalRevision = uint64(refs.Revision)
					second.Anchor.PhysicalDigest = refs.Digest()
				case "inactive":
					second.Inactive = true
				case "uncertain":
					second.Observations[1].PointIndices = []uint32{5}
					second.Observations[1].UncertainIndices = []uint32{4}
				case "missing":
					o := second.Observations[1]
					o.SampleId, o.TimestampNs, o.SourceOrdinal = 2, physTime(2), 2
					o.Sphere.XM++
				case "rejected":
					later := proto.Clone(second.Observations[1]).(*pb.FeatureObservation)
					later.SampleId, later.TimestampNs, later.SourceOrdinal = 2, physTime(2), 2
					later.Sphere.XM++
					o := second.Observations[1]
					o.Decision = pb.FeatureDecision_FEATURE_DECISION_REJECTED
					o.PointIndices, o.UncertainIndices, o.Sphere = nil, nil, nil
					second.Observations = append(second.Observations, later)
				}
			})
			if name == "overlap" {
				r.Matches[1].PointIndex = ptrU32(0)
			}
			if name == "overlap" || name == "duplicate source" {
				r.Prior.PositionBoundM = fp(10)
			}
			if _, err := ProposeJointFacetPose(p, r); err == nil {
				t.Fatal("accepted", name)
			}
		})
	}
}

// Deterministic solver fixtures exercise the geometry independently from I/O.
func jointSolverInputs(t *testing.T, kind string) (FacetJointPoseRequest, []jointFacetInput, Points) {
	t.Helper()
	req := FacetJointPoseRequest{SchemaVersion: 2, Pack: "kirk0/packs/fixture", PackDigest: "sha256:pack", FeatureRevision: 1, FeatureDigest: "sha256:features", MembershipRevision: 1, MembershipDigest: "sha256:membership", SampleID: 1, TimestampNs: 9007199254740993, Author: "operator", Prior: FacetPosePrior{XM: fp(12.1), YM: fp(2.95), YawRad: fp(.75), PositionBoundM: fp(.5), YawBoundRad: fp(.1), Source: "declared prior"}}
	body := &BodyGeometry{BodyID: "body", AxisConvention: BodyAxisConvention, Height: DimensionBound{Status: EvidenceUnknown}, Review: independentReview(), Length: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(4), UpperM: fp(5)}, Width: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(1.9)}}
	var all Points
	var inputs []jointFacetInput
	for i := 0; i < 2; i++ {
		normal := .4
		if kind == "crossed" {
			normal += float64(i) * math.Pi / 2
		}
		if kind == "near parallel" {
			normal += float64(i) * .01
		}
		f, target, pts := rotatedSegmentCase(normal, .9+float64(i)*.3, .7)
		f.FeatureId = []string{"a", "b"}[i]
		f.Anchor.PhysicalRevision = 1
		f.Anchor.PhysicalDigest = "sha256:physical"
		f.Anchor.SourceSample = 0
		f.Anchor.Origin = "reference_seeded_proposal"
		offset := uint32(len(all.X))
		all.X = append(all.X, pts.X...)
		all.Y = append(all.Y, pts.Y...)
		all.Z = append(all.Z, pts.Z...)
		for j := range target.PointIndices {
			target.PointIndices[j] += offset
		}
		m := FacetJointPoseMatch{FeatureID: f.FeatureId, PointIndex: ptrU32(offset), EndIndex: ptrU32(offset + 2), ReturnBoundM: .02, Confirmed: true, IdentityNote: "same physical edge"}
		legacy := req.single(m)
		result, err := solveFacetPose(f, target, body, all, legacy)
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, jointFacetInput{f, m, result})
		req.Matches = append(req.Matches, m)
	}
	return req, inputs, all
}

func TestJointFacetLinesPreserveNullDirectionAndMeasureComplementaryNormals(t *testing.T) {
	for _, kind := range []string{"parallel", "near parallel", "crossed"} {
		t.Run(kind, func(t *testing.T) {
			req, in, points := jointSolverInputs(t, kind)
			got, err := solveJointFacetPose(req, in, points)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got.Yaw.Rad-.7) > 1e-5 || got.Yaw.Status != EvidenceObserved {
				t.Fatal(got.Yaw)
			}
			if kind == "crossed" {
				if got.Confidence.State != "supported" || got.Confidence.DataRank != 3 || math.Hypot(got.Centre.XM-12, got.Centre.YM-3) > 1e-5 || got.Normal != nil {
					t.Fatal(got)
				}
			} else {
				if got.Confidence.State != "partial" || got.Confidence.DataRank != 2 || got.Confidence.Position != "partial" || got.TangentBoundM == nil || *got.TangentBoundM != .5 || got.Normal == nil {
					t.Fatal(got)
				}
				// Translation along the common plane must not become a measurement.
				nx, ny := got.Normal[0], got.Normal[1]
				if math.Abs((-ny)*(got.Centre.XM-*req.Prior.XM)+nx*(got.Centre.YM-*req.Prior.YM)) > 1e-6 {
					t.Fatal("tangent prior moved")
				}
			}
		})
	}
}

func compactSolverCase(t *testing.T) (FacetJointPoseRequest, []jointFacetInput, Points) {
	t.Helper()
	req, _, _ := jointSolverInputs(t, "parallel")
	req.Matches = nil
	body := &BodyGeometry{BodyID: "body", AxisConvention: BodyAxisConvention, Height: DimensionBound{Status: EvidenceUnknown}, Review: independentReview(), Length: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(4), UpperM: fp(5)}, Width: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(1.9)}}
	var points Points
	var inputs []jointFacetInput
	for i, a := range [][2]float64{{-2, -.8}, {2, .8}} {
		x, y := 12+math.Cos(.7)*a[0]-math.Sin(.7)*a[1], 3+math.Sin(.7)*a[0]+math.Cos(.7)*a[1]
		points.X = append(points.X, float32(x))
		points.Y = append(points.Y, float32(y))
		points.Z = append(points.Z, 1)
		f := &pb.FeatureCandidate{FeatureId: []string{"a", "b"}[i], ObjectId: "car", Anchor: &pb.FeatureAnchor{BodyId: "body", XM: a[0], YM: a[1], BoundM: .1, PhysicalRevision: 1, PhysicalDigest: "sha256:physical", Origin: "reference_seeded_proposal"}}
		m := FacetJointPoseMatch{FeatureID: f.FeatureId, PointIndex: ptrU32(uint32(i)), ReturnBoundM: .02, Confirmed: true, IdentityNote: "same rigid tip"}
		result, err := solveFacetPose(f, &pb.FeatureObservation{PointIndices: []uint32{uint32(i)}, Origin: "human_proposal"}, body, points, req.single(m))
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, jointFacetInput{f, m, result})
		req.Matches = append(req.Matches, m)
	}
	return req, inputs, points
}

func TestJointFacetSolverBoundsAndFailures(t *testing.T) {
	req, in, p := compactSolverCase(t)
	got, err := solveJointFacetPose(req, in, p)
	if err != nil || math.Hypot(got.Centre.XM-12, got.Centre.YM-3) > 1e-5 {
		t.Fatal(got, err)
	}
	// Opposed yaw or translation cannot be hidden by averaging matches.
	for _, kind := range []string{"separation", "yaw prior", "translation prior", "overflow", "partial body", "weak baseline"} {
		t.Run(kind, func(t *testing.T) {
			r, in, p := compactSolverCase(t)
			switch kind {
			case "separation":
				p.X[1] += 10
			case "yaw prior":
				r.Prior.YawRad = fp(2)
			case "translation prior":
				r.Prior.XM = fp(100)
			case "overflow":
				in[0].result.Body.Length = DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(math.MaxFloat64), UpperM: fp(math.MaxFloat64)}
			case "partial body":
				in[0].result.Body.Length.Span = SpanPartial
			case "weak baseline":
				for i := range in {
					in[i].feature.Anchor.BoundM = 10
				}
			}
			got, err := solveJointFacetPose(r, in, p)
			if kind == "partial body" {
				if err != nil || got.Box != nil || got.BoxUnavailable == "" {
					t.Fatal(got, err)
				}
			} else if kind == "weak baseline" {
				if err != nil || got.Confidence.State != "partial" || got.Confidence.Yaw != "prior_only" || got.Yaw.Rad != *r.Prior.YawRad {
					t.Fatal(got, err)
				}
			} else if err == nil {
				t.Fatal("accepted", kind)
			}
		})
	}
}

func TestJointFacetPoseSharedFixtures(t *testing.T) {
	for _, kind := range []string{"compact", "parallel"} {
		var r FacetJointPoseRequest
		var in []jointFacetInput
		var p Points
		if kind == "compact" {
			r, in, p = compactSolverCase(t)
		} else {
			r, in, p = jointSolverInputs(t, "parallel")
		}
		got, err := solveJointFacetPose(r, in, p)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("testdata", "facet-joint-pose-"+kind+".json")
		if os.Getenv("UPDATE_FACET_JOINT_FIXTURE") == "1" {
			if err = os.WriteFile(path, append(encoded, '\n'), 0644); err != nil {
				t.Fatal(err)
			}
		}
		fixture, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.TrimSpace(fixture), encoded) {
			t.Fatalf("%s differs from Go solver", path)
		}
	}
}

func TestJointFacetSolverRejectsGeometryHiddenByAggregateBounds(t *testing.T) {
	t.Run("incompatible parallel strips", func(t *testing.T) {
		req, in, p := jointSolverInputs(t, "parallel")
		req.Prior.PositionBoundM = fp(10)
		for i := range in {
			in[i].feature.Anchor.BoundM = .001
			in[i].feature.Anchor.Line.NormalBoundRad = .0001
			in[i].match.ReturnBoundM = .001
			in[i].result.Yaw.BoundRad = .0001
		}
		in[1].feature.Anchor.Line.OffsetM += 2
		if _, err := solveJointFacetPose(req, in, p); err == nil {
			t.Fatal("broad prior concealed incompatible normal strips")
		}
	})
	t.Run("three pairwise overlapping disks", func(t *testing.T) {
		req, in, _ := compactSolverCase(t)
		req.Prior = FacetPosePrior{XM: fp(0), YM: fp(0), YawRad: fp(0), PositionBoundM: fp(10), YawBoundRad: fp(.1), Source: "explicit broad prior"}
		third := in[1]
		third.feature = proto.Clone(third.feature).(*pb.FeatureCandidate)
		third.feature.FeatureId = "c"
		in = append(in, third)
		h := 1.9 * math.Sqrt(3) / 2
		p := Points{X: []float32{-.95, .95, 0}, Y: []float32{float32(-h / 3), float32(-h / 3), float32(2 * h / 3)}, Z: []float32{1, 1, 1}}
		for i := range in {
			in[i].feature.Anchor.XM = 0
			in[i].feature.Anchor.YM = 0
			in[i].feature.Anchor.BoundM = .5
			in[i].match.PointIndex = ptrU32(uint32(i))
			in[i].match.ReturnBoundM = .5
		}
		if _, err := solveJointFacetPose(req, in, p); err == nil {
			t.Fatal("pairwise overlaps concealed empty common intersection")
		}
	})
	t.Run("unresolvable broad finite prior", func(t *testing.T) {
		req, in, p := compactSolverCase(t)
		req.Prior.PositionBoundM = fp(1e15)
		if _, err := solveJointFacetPose(req, in, p); err == nil {
			t.Fatal("normalisation swallowed informative anchor bounds")
		}
	})
}

func TestJointFacetSolverRejectsConflictingYawAndUnboundedNormals(t *testing.T) {
	for _, name := range []string{"yaw conflict", "normal unbounded", "angle baseline unbounded"} {
		t.Run(name, func(t *testing.T) {
			req, in, p := jointSolverInputs(t, "parallel")
			switch name {
			case "yaw conflict":
				req.Prior.YawBoundRad = fp(.6)
				in[1].result.Yaw.Rad += .5
			case "normal unbounded":
				for i := range in {
					in[i].result.Yaw.BoundRad = 1.4
					in[i].feature.Anchor.Line.NormalBoundRad = .3
				}
			case "angle baseline unbounded":
				req, in, p = compactSolverCase(t)
				for i := range in {
					in[i].feature.Anchor.BoundM = 1.9
					in[i].match.ReturnBoundM = 1.9
				}
			}
			got, err := solveJointFacetPose(req, in, p)
			if name == "angle baseline unbounded" {
				if err != nil || got.Confidence.State != "partial" || got.Yaw.Status != EvidencePriorOnly {
					t.Fatal(got, err)
				}
			} else if err == nil {
				t.Fatal("accepted", name)
			}
		})
	}
}

func TestJointFacetOptionalDiagnosticsRefuseNumericOverflow(t *testing.T) {
	req, in, p := jointSolverInputs(t, "parallel")
	out, err := solveJointFacetPose(req, in, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"normal", "normal bound", "tangent bound"} {
		candidate := *out
		switch name {
		case "normal":
			candidate.Normal = &[2]float64{math.NaN(), 0}
		case "normal bound":
			candidate.NormalBoundM = fp(math.Inf(1))
		case "tangent bound":
			candidate.TangentBoundM = fp(math.NaN())
		}
		if _, err := finishJointFacetPose(&candidate); err == nil {
			t.Fatal("nonfinite diagnostic accepted", name)
		}
	}
}

func TestJointFacetPoseCombinesTwoSourcePinnedSavedLines(t *testing.T) {
	p, e := registeredSegmentFixture(t)
	second := proto.Clone(e.Document.Features[0]).(*pb.FeatureCandidate)
	second.FeatureId = "end-edge"
	for _, o := range second.Observations {
		o.PointIndices = []uint32{4, 5, 6}
		o.Sphere = &pb.FeatureSphere{XM: 12.25 + float64(o.SampleId), YM: 0, ZM: .9, RadiusM: 1.1}
	}
	source, err := p.PointsAt(0)
	if err != nil {
		t.Fatal(err)
	}
	second.Anchor.Line, second.Anchor.BoundM, err = featureSegmentRelation(source, []uint32{4, 5, 6}, 4, 6, PlanarBound{XM: 10, BoundM: .2}, AngleBound{BoundRad: .05}, second.Anchor.ReturnBoundM)
	if err != nil {
		t.Fatal(err)
	}
	e.Document.Features = append(e.Document.Features, second)
	state, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	r := FacetJointPoseRequest{SchemaVersion: 2, Pack: "kirk0/packs/lines", PackDigest: p.Manifest.PackDigest, FeatureRevision: state.Document.Revision, FeatureDigest: state.Digest, MembershipRevision: 1, MembershipDigest: state.MembershipDigest, SampleID: 1, TimestampNs: physTime(1), Author: "op", Prior: FacetPosePrior{XM: fp(10.8), YM: fp(0), YawRad: fp(.01), PositionBoundM: fp(.5), YawBoundRad: fp(.03), Source: "explicit prior"}, Matches: []FacetJointPoseMatch{{FeatureID: e.Document.Features[0].FeatureId, PointIndex: ptrU32(0), EndIndex: ptrU32(2), ReturnBoundM: .05, Confirmed: true, IdentityNote: "same side line"}, {FeatureID: second.FeatureId, PointIndex: ptrU32(4), EndIndex: ptrU32(6), ReturnBoundM: .05, Confirmed: true, IdentityNote: "same end line"}}}
	out, err := ProposeJointFacetPose(p, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.Confidence.State != "supported" || out.Confidence.DataRank != 3 || out.Normal != nil || len(out.MatchedIndices) != 6 || math.Abs(out.Centre.XM-11) > 1e-6 || math.Abs(out.Centre.YM) > 1e-6 {
		t.Fatal(out)
	}
}

func TestJointFacetSolverRespectsNumericalAndExactPriorBounds(t *testing.T) {
	t.Run("smaller compact envelope", func(t *testing.T) {
		r, in, p := compactSolverCase(t)
		in[1].feature.Anchor.BoundM = .01
		got, err := solveJointFacetPose(r, in, p)
		if err != nil || math.Hypot(got.Centre.XM-12, got.Centre.YM-3) > 1e-5 {
			t.Fatal(got, err)
		}
	})
	t.Run("huge yaw", func(t *testing.T) {
		r, in, p := compactSolverCase(t)
		r.Prior.YawRad = fp(1e308)
		if _, err := solveJointFacetPose(r, in, p); err == nil {
			t.Fatal("unrepresentable yaw correction accepted")
		}
	})
	for _, x := range []float64{12, 100} {
		r, in, p := compactSolverCase(t)
		r.Prior.XM, r.Prior.YM, r.Prior.PositionBoundM = fp(x), fp(3), fp(0)
		got, err := solveJointFacetPose(r, in, p)
		if x == 100 {
			if err == nil {
				t.Fatal("incompatible hard prior accepted")
			}
			continue
		}
		if err != nil || got.Centre.XM != x || got.Centre.YM != 3 {
			t.Fatal("hard prior moved", got, err)
		}
	}
}
