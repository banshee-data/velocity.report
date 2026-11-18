package server

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

func facetPoseAPIFixture(t *testing.T) (*Server, *annotation.Pack, annotation.FacetPoseRequest) {
	t.Helper()
	root := t.TempDir()
	p := physicalTestPack(t, root, "clip")
	membership, err := annotation.LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	obj := physicalTestObject()
	obj.Body.Width = annotation.DimensionBound{Status: annotation.EvidenceInferred, Span: annotation.SpanFull, LowerM: physF(1.7), UpperM: physF(1.9), Support: annotation.EvidenceSupport{External: "known source width"}}
	pin := &annotation.MembershipPin{Revision: membership.Revision, Digest: membership.Digest()}
	obj.Body.Review.ReviewedAgainst = pin
	obj.Keyframes[0].Review.ReviewedAgainst = pin
	refs := annotation.NewPhysicalReferenceSet(p)
	refs.Objects = []annotation.PhysicalObject{obj}
	if err = annotation.SavePhysicalReferences(p, refs); err != nil {
		t.Fatal(err)
	}
	state, err := annotation.LoadFeatures(p)
	if err != nil {
		t.Fatal(err)
	}
	zero := uint32(0)
	feature := &pb.FeatureCandidate{FeatureId: "tip", ObjectId: "car-1", Name: "tip", PartId: "body", PartRelation: "rigid_proposal", Geometry: pb.FeatureGeometry_FEATURE_GEOMETRY_PROTRUSION, Anchor: &pb.FeatureAnchor{PartFrameId: "body_a_xy", PartFrameRevision: 1, XM: -2.25, YM: float64(float32(-.9)), BoundM: .25 + math.Hypot(2.25, .9)*2*math.Sin(.025), BodyId: "body_a", KeyframeId: "kf_a", PhysicalRevision: uint64(refs.Revision), PhysicalDigest: refs.Digest(), CoordinateDomain: "body_xy", SourceSample: 0, SourcePointIndex: &zero, ReturnBoundM: .05, Origin: "reference_seeded_proposal", Method: "manual_named_return_v1", IdentityNote: "same rigid tip"}}
	for i := 0; i < 2; i++ {
		feature.Observations = append(feature.Observations, &pb.FeatureObservation{SampleId: uint32(i), TimestampNs: p.Samples[i].TimestampNs, SourceOrdinal: uint32(i), PointIndices: []uint32{0, 1}, Sphere: &pb.FeatureSphere{XM: 7.75 + float64(i), YM: -.9, ZM: .9, RadiusM: .7}, Decision: pb.FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL, MembershipRevision: uint64(membership.Revision), MembershipDigest: membership.Digest(), Method: "manual_sphere", Origin: "human_proposal", Author: "op"})
	}
	state.Document.Author = "op"
	state.Document.Features = []*pb.FeatureCandidate{feature}
	state, err = annotation.SaveFeatures(p, &pb.FeatureEdit{Document: state.Document, MembershipDigest: state.MembershipDigest})
	if err != nil {
		t.Fatal(err)
	}
	req := annotation.FacetPoseRequest{SchemaVersion: 1, Pack: "kirk0/packs/clip", PackDigest: p.Manifest.PackDigest, FeatureID: "tip", FeatureRevision: state.Document.Revision, FeatureDigest: state.Digest, MembershipRevision: membership.Revision, MembershipDigest: membership.Digest(), SampleID: 1, TimestampNs: p.Samples[1].TimestampNs, PointIndex: &zero, ReturnBoundM: .05, Confirmed: true, Author: "op", IdentityNote: "same tip", Prior: annotation.FacetPosePrior{XM: physF(10.8), YM: physF(0), YawRad: physF(0), PositionBoundM: physF(.5), YawBoundRad: physF(.02), Source: "declared experiment prior"}}
	return &Server{annotationPacksDir: root}, p, req
}

func TestFacetPoseAPIIsBoundedReadOnlyAndUsesSourceEvidence(t *testing.T) {
	ws, p, req := facetPoseAPIFixture(t)
	call := func(method string, b []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		ws.handleFacetPoseProposal(w, httptest.NewRequest(method, "/api/lidar/annotations/features/pose-proposal", bytes.NewReader(b)))
		return w
	}
	before := map[string][]byte{}
	for _, f := range []string{labelSetManifest(t, p.Dir), filepath.Join(p.Dir, "feature-proposals.pb"), filepath.Join(p.Dir, "physical-references.json")} {
		before[f], _ = os.ReadFile(f)
	}
	b, _ := json.Marshal(req)
	res := call(http.MethodPost, b)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	var proposal annotation.FacetPoseProposal
	if err := json.Unmarshal(res.Body.Bytes(), &proposal); err != nil || proposal.Box == nil || proposal.Centre.XM != 11 || proposal.Yaw.Status != annotation.EvidencePriorOnly {
		t.Fatal(err, proposal)
	}
	// The wire carries the result's version 2 contract; a compact spot still
	// reports the prior yaw and says so.
	if proposal.SchemaVersion != 2 || proposal.Constraint != annotation.ConstraintPlanarPositionGivenPriorYaw || proposal.Yaw.Rad != *req.Prior.YawRad || len(proposal.Unresolved) == 0 || proposal.Unresolved[0] != "yaw_from_prior_not_measured" {
		t.Fatal("API lost the proposal contract", proposal.SchemaVersion, proposal.Constraint, proposal.Unresolved)
	}
	for f, b := range before {
		after, _ := os.ReadFile(f)
		if !bytes.Equal(b, after) {
			t.Fatal("wrote", f)
		}
	}
	if res = call(http.MethodGet, nil); res.Code != 405 {
		t.Fatal(res.Code)
	}
	for _, bad := range [][]byte{[]byte("{"), []byte(`{"unknown":1}`), append(append([]byte{}, b...), []byte(` {}`)...), []byte(`{"author":"` + string(bytes.Repeat([]byte("x"), maxPhysicalRequestBytes)) + `"}`)} {
		if res = call(http.MethodPost, bad); res.Code != 400 {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	cases := []struct {
		name   string
		mutate func(*annotation.FacetPoseRequest)
		code   int
	}{
		{"pack", func(r *annotation.FacetPoseRequest) { r.Pack = "../escape" }, 400},
		{"feature conflict", func(r *annotation.FacetPoseRequest) { r.FeatureDigest = "wrong" }, 409},
		{"membership conflict", func(r *annotation.FacetPoseRequest) { r.MembershipRevision++ }, 409},
		{"unconfirmed", func(r *annotation.FacetPoseRequest) { r.Confirmed = false }, 422},
	}
	for _, tc := range cases {
		r := req
		tc.mutate(&r)
		payload, _ := json.Marshal(r)
		out := call(http.MethodPost, payload)
		if out.Code != tc.code {
			t.Fatal(tc.name, out.Code, out.Body.String())
		}
	}
}

func jointFacetPoseAPIFixture(t *testing.T) (*Server, *annotation.Pack, annotation.FacetJointPoseRequest) {
	t.Helper()
	ws, p, legacy := facetPoseAPIFixture(t)
	state, err := annotation.LoadFeatures(p)
	if err != nil {
		t.Fatal(err)
	}
	second := proto.Clone(state.Document.Features[0]).(*pb.FeatureCandidate)
	second.FeatureId = "front-tip"
	second.Anchor.XM = 2.25
	four := uint32(4)
	second.Anchor.SourcePointIndex = &four
	for _, o := range second.Observations {
		o.PointIndices = []uint32{4, 5}
		o.Sphere.XM += 4.5
	}
	state.Document.Features = append(state.Document.Features, second)
	state, err = annotation.SaveFeatures(p, &pb.FeatureEdit{Document: state.Document, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest})
	if err != nil {
		t.Fatal(err)
	}
	req := annotation.FacetJointPoseRequest{SchemaVersion: 2, Pack: legacy.Pack, PackDigest: legacy.PackDigest, FeatureRevision: state.Document.Revision, FeatureDigest: state.Digest, MembershipRevision: legacy.MembershipRevision, MembershipDigest: legacy.MembershipDigest, SampleID: legacy.SampleID, TimestampNs: legacy.TimestampNs, Author: legacy.Author, Prior: legacy.Prior, Matches: []annotation.FacetJointPoseMatch{{FeatureID: "tip", PointIndex: legacy.PointIndex, ReturnBoundM: .05, Confirmed: true, IdentityNote: "same rear tip"}, {FeatureID: "front-tip", PointIndex: &four, ReturnBoundM: .05, Confirmed: true, IdentityNote: "same front tip"}}}
	return ws, p, req
}

func TestFacetPoseAPIVersionDispatchPreservesJointEvidenceAndLegacyContract(t *testing.T) {
	ws, p, req := jointFacetPoseAPIFixture(t)
	call := func(b []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		ws.handleFacetPoseProposal(w, httptest.NewRequest(http.MethodPost, "/api/lidar/annotations/features/pose-proposal", bytes.NewReader(b)))
		return w
	}
	before := map[string][]byte{}
	for _, f := range []string{labelSetManifest(t, p.Dir), filepath.Join(p.Dir, "feature-proposals.pb"), filepath.Join(p.Dir, "physical-references.json")} {
		before[f], _ = os.ReadFile(f)
	}
	payload, _ := json.Marshal(req)
	response := call(payload)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var result annotation.FacetJointPoseProposal
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Schema != "velocity.report/facet-joint-pose-proposal" || result.SchemaVersion != 1 || result.Confidence.DataRank != 3 || result.Confidence.State != "supported" || len(result.Contributors) != 2 || result.Request.Matches[0].PointIndex == nil || *result.Request.Matches[0].PointIndex != 0 {
		t.Fatal(err, result)
	}
	for f, b := range before {
		after, _ := os.ReadFile(f)
		if !bytes.Equal(b, after) {
			t.Fatal("joint endpoint wrote", f)
		}
	}
	// The v1 endpoint still produces precisely its own result schema.
	legacy := annotation.FacetPoseRequest{SchemaVersion: 1, Pack: req.Pack, PackDigest: req.PackDigest, FeatureID: req.Matches[0].FeatureID, FeatureRevision: req.FeatureRevision, FeatureDigest: req.FeatureDigest, MembershipRevision: req.MembershipRevision, MembershipDigest: req.MembershipDigest, SampleID: req.SampleID, TimestampNs: req.TimestampNs, PointIndex: req.Matches[0].PointIndex, ReturnBoundM: .05, Confirmed: true, IdentityNote: "same tip", Author: req.Author, Prior: req.Prior}
	legacyBytes, _ := json.Marshal(legacy)
	response = call(legacyBytes)
	var single annotation.FacetPoseProposal
	if err := json.Unmarshal(response.Body.Bytes(), &single); err != nil || response.Code != 200 || single.SchemaVersion != 2 || single.Schema != "velocity.report/facet-pose-proposal" {
		t.Fatal(err, response.Code, response.Body.String())
	}
	cases := []struct {
		name string
		body []byte
		code int
	}{
		{"unsupported", []byte(`{"schema_version":3}`), 400},
		{"missing version", []byte(`{"matches":[]}`), 400},
		{"string version", []byte(`{"schema_version":"2"}`), 400},
		{"array", []byte(`[]`), 400},
		{"null", []byte(`null`), 400},
		{"legacy fields in joint", bytes.Replace(payload, []byte(`"matches":`), []byte(`"feature_id":"tip","matches":`), 1), 400},
		{"joint fields in legacy", bytes.Replace(legacyBytes, []byte(`"feature_id":`), []byte(`"matches":[],"feature_id":`), 1), 400},
		{"unknown joint match", bytes.Replace(payload, []byte(`"return_bound_m":`), []byte(`"invented":1,"return_bound_m":`), 1), 400},
	}
	for _, tc := range cases {
		if out := call(tc.body); out.Code != tc.code {
			t.Fatal(tc.name, out.Code, out.Body.String())
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*annotation.FacetJointPoseRequest)
		code   int
	}{
		{"pack", func(r *annotation.FacetJointPoseRequest) { r.Pack = "../escape" }, 400},
		{"feature hash", func(r *annotation.FacetJointPoseRequest) { r.FeatureDigest = "stale" }, 409},
		{"membership", func(r *annotation.FacetJointPoseRequest) { r.MembershipRevision++ }, 409},
		{"unsupported count", func(r *annotation.FacetJointPoseRequest) { r.Matches = r.Matches[:1] }, 422},
		{"missing confirmation", func(r *annotation.FacetJointPoseRequest) { r.Matches[0].Confirmed = false }, 422},
	} {
		r := req
		r.Matches = append([]annotation.FacetJointPoseMatch(nil), req.Matches...)
		tc.mutate(&r)
		b, _ := json.Marshal(r)
		if out := call(b); out.Code != tc.code {
			t.Fatal(tc.name, out.Code, out.Body.String())
		}
	}
}

func TestJointFacetPoseVersionDecoderPreservesLargeNanosecondsAndIndexZero(t *testing.T) {
	payload := []byte(`{"schema_version":2,"sample_id":0,"timestamp_ns":9007199254740993,"matches":[{"feature_id":"a","point_index":0},{"feature_id":"b","point_index":1}]}`)
	var req annotation.FacetJointPoseRequest
	if err := decodeFacetPoseVersion(payload, &req); err != nil {
		t.Fatal(err)
	}
	if req.SchemaVersion != 2 || req.TimestampNs != 9007199254740993 || req.Matches[0].PointIndex == nil || *req.Matches[0].PointIndex != 0 {
		t.Fatal("joint decoder changed identity", req)
	}
	again, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(again, []byte(`"timestamp_ns":9007199254740993`)) || !bytes.Contains(again, []byte(`"point_index":0`)) {
		t.Fatal("joint JSON lost exact identity", string(again))
	}
}
