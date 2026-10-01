package server

import (
	"bytes"
	"encoding/json"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
	req := annotation.FacetPoseRequest{SchemaVersion: 1, Pack: "clip/pack", PackDigest: p.Manifest.PackDigest, FeatureID: "tip", FeatureRevision: state.Document.Revision, FeatureDigest: state.Digest, MembershipRevision: membership.Revision, MembershipDigest: membership.Digest(), SampleID: 1, TimestampNs: p.Samples[1].TimestampNs, PointIndex: &zero, ReturnBoundM: .05, Confirmed: true, Author: "op", IdentityNote: "same tip", Prior: annotation.FacetPosePrior{XM: physF(10.8), YM: physF(0), YawRad: physF(0), PositionBoundM: physF(.5), YawBoundRad: physF(.02), Source: "declared experiment prior"}}
	return &Server{annotationPacksDir: root}, p, req
}

func TestFacetPoseAPIIsBoundedReadOnlyAndUsesSourceEvidence(t *testing.T) {
	ws, p, req := facetPoseAPIFixture(t)
	call := func(method string, b []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		ws.handleFacetPoseProposal(w, httptest.NewRequest(method, "/api/annotations/features/pose-proposal", bytes.NewReader(b)))
		return w
	}
	before := map[string][]byte{}
	for _, f := range []string{"annotations.json", "feature-proposals.pb", "physical-references.json"} {
		before[f], _ = os.ReadFile(filepath.Join(p.Dir, f))
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
	for f, b := range before {
		after, _ := os.ReadFile(filepath.Join(p.Dir, f))
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
