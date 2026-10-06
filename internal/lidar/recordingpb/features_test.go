package recordingpb

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// This fixture is decoded by Swift as well. It deliberately uses sample/point
// zero, explicit optional-zero presence, overlapping support and a timestamp
// beyond JavaScript's exact-integer range. It is a wire fixture, not a real pack.
func TestFeatureSharedWireFixture(t *testing.T) {
	b, err := os.ReadFile("../../../proto/velocity_recording/v1/testdata/feature-proposals.pb")
	if err != nil {
		t.Fatal(err)
	}
	d := new(FeatureAnnotations)
	if err = proto.Unmarshal(b, d); err != nil {
		t.Fatal(err)
	}
	if d.GetSchema() != "velocity.report/feature-proposals" || d.GetSchemaVersion() != 1 || d.GetPointDomain() != "legacy_pack" || d.GetRevision() != 2 || d.GetPackDigest() != "sha256:shared-fixture" || d.GetDatasetId() != "shared-fixture" || d.GetAuthor() != "fixture-operator" || d.GetUpdatedUtc() != "2026-09-30T23:00:00Z" {
		t.Fatal(d)
	}
	f := d.GetFeatures()[0]
	if f.GetFeatureId() != "persistent-corner" || f.GetObjectId() != "car-1" || f.GetName() != "Corner near headlight?" || f.GetGeometry() != FeatureGeometry_FEATURE_GEOMETRY_CORNER || f.GetSemanticHint() != "headlight" || f.GetPartId() != "body" || f.GetPartRelation() != "unknown" || f.GetAnchor() != nil {
		t.Fatal(f)
	}
	o := f.GetObservations()[0]
	if o.GetSampleId() != 1 || o.GetTimestampNs() != 9007199254740993 || o.GetSourceOrdinal() != 101 || len(o.GetPointIndices()) != 2 || o.GetPointIndices()[0] != 0 || o.GetUncertainIndices()[0] != 18 || o.GetDecision() != FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL || o.GetMethod() != "object_mask_translation_v1" || o.GetOrigin() != "assisted_proposal" || o.GetAuthor() != "fixture-operator" || o.GetMembershipRevision() != 3 || o.GetMembershipDigest() != "sha256:membership" || o.ProposedFromSample == nil || o.GetProposedFromSample() != 0 || o.GetNote() != "" {
		t.Fatal(o)
	}
	s := o.GetSphere()
	if s.GetXM() != 1.25 || s.GetYM() != -2.5 || s.GetZM() != 0.75 || s.GetRadiusM() != 0.2 {
		t.Fatal(s)
	}
	if d.Features[1].Observations[0].ProposedFromSample != nil {
		t.Fatal("absent zero became present")
	}
	out, err := (proto.MarshalOptions{Deterministic: true}).Marshal(d)
	if err != nil || !bytes.Equal(b, out) {
		t.Fatalf("wire fixture changed: %v", err)
	}
	// Unknown future fields survive a read-only round-trip; the authoritative
	// annotation editor separately refuses them rather than losing semantics.
	future := append(append([]byte(nil), b...), 0xa0, 0x06, 0x01)
	if err = proto.Unmarshal(future, d); err != nil {
		t.Fatal(err)
	}
	out, err = proto.Marshal(d)
	if err != nil || !bytes.Equal(out, future) {
		t.Fatalf("future field lost: %v", err)
	}
}

func TestFeatureTransportAndReservedAnchorPresence(t *testing.T) {
	a := &FeatureAnchor{PartFrameId: "body-stable", PartFrameRevision: 2, XM: 1.2, YM: -0.4, ZM: 0.8, PhysicalRevision: 7, PhysicalDigest: "sha256:physical", BodyId: "body", KeyframeId: "pose"}
	if a.GetPartFrameId() != "body-stable" || a.GetPartFrameRevision() != 2 || a.GetXM() != 1.2 || a.GetYM() != -0.4 || a.GetZM() != 0.8 || a.GetPhysicalRevision() != 7 || a.GetPhysicalDigest() != "sha256:physical" || a.GetBodyId() != "body" || a.GetKeyframeId() != "pose" {
		t.Fatal(a)
	}
	d := &FeatureAnnotations{SchemaVersion: 1, Features: []*FeatureCandidate{{FeatureId: "x", Anchor: a}}}
	state := &FeatureState{Document: d, Digest: "sha256:feature", MembershipDigest: "sha256:member", PackDirectory: "/packs/clip"}
	edit := &FeatureEdit{Document: d, BaseDigest: state.GetDigest(), MembershipDigest: state.GetMembershipDigest()}
	if state.GetDocument() != d || state.GetPackDirectory() != "/packs/clip" || edit.GetDocument() != d || edit.GetBaseDigest() != "sha256:feature" || edit.GetMembershipDigest() != "sha256:member" {
		t.Fatal("transport tokens changed")
	}
	for _, message := range []proto.Message{a, d, state, edit} {
		b, err := proto.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		copy := message.ProtoReflect().New().Interface()
		if err := proto.Unmarshal(b, copy); err != nil || !proto.Equal(message, copy) {
			t.Fatalf("round trip %T: %v", message, err)
		}
	}
}

// Absent records must stay absent: callers use generated getters while opening
// old documents, and must not acquire a fabricated anchor, sample or support.
func TestFeatureAbsentRecordContract(t *testing.T) {
	messages := []proto.Message{(*FeatureAnnotations)(nil), (*FeatureCandidate)(nil), (*FeatureObservation)(nil), (*FeatureSphere)(nil), (*FeatureAnchor)(nil), (*FeatureLineConstraint)(nil), (*FeatureState)(nil), (*FeatureEdit)(nil)}
	for _, absent := range messages {
		t.Run(string(absent.ProtoReflect().Descriptor().Name()), func(t *testing.T) {
			empty := absent.ProtoReflect().New().Interface()
			nilValue, emptyValue := reflect.ValueOf(absent), reflect.ValueOf(empty)
			for i := 0; i < nilValue.Type().NumMethod(); i++ {
				method := nilValue.Type().Method(i)
				if !strings.HasPrefix(method.Name, "Get") {
					continue
				}
				nilResult := nilValue.Method(i).Call(nil)
				emptyResult := emptyValue.Method(i).Call(nil)
				if len(nilResult) != 1 || !reflect.DeepEqual(nilResult[0].Interface(), emptyResult[0].Interface()) {
					t.Fatalf("absent %s has nondefault value", method.Name)
				}
			}
			if b, err := proto.Marshal(empty); err != nil || len(b) != 0 {
				t.Fatalf("empty record wrote evidence: %x %v", b, err)
			}
			// Legacy descriptor consumers must resolve the same message as the
			// reflection API; Swift-generated names are pinned by the fixture.
			legacy := empty.(interface{ Descriptor() ([]byte, []int) })
			gz, index := legacy.Descriptor()
			reader, err := gzip.NewReader(bytes.NewReader(gz))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			fd := new(descriptorpb.FileDescriptorProto)
			if err := proto.Unmarshal(raw, fd); err != nil {
				t.Fatal(err)
			}
			if fd.GetPackage() != "velocity.recording.v1" || fd.MessageType[index[0]].GetName() != string(empty.ProtoReflect().Descriptor().Name()) {
				t.Fatal("descriptor moved domain")
			}
			empty.(interface{ ProtoMessage() }).ProtoMessage()
			if empty.(interface{ String() string }).String() != "" {
				t.Fatal("empty message claimed values")
			}
			empty.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
			proto.Reset(empty)
			if len(empty.ProtoReflect().GetUnknown()) != 0 {
				t.Fatal("reset retained a stale interpretation")
			}
		})
	}
}

func TestFeatureEnumCompatibility(t *testing.T) {
	for _, value := range []FeatureGeometry{FeatureGeometry_FEATURE_GEOMETRY_UNKNOWN, FeatureGeometry_FEATURE_GEOMETRY_EDGE, FeatureGeometry_FEATURE_GEOMETRY_CORNER, FeatureGeometry_FEATURE_GEOMETRY_PATCH, FeatureGeometry_FEATURE_GEOMETRY_PROTRUSION, 99} {
		if *value.Enum() != value || int32(value.Number()) != int32(value) || value.Descriptor().FullName() != "velocity.recording.v1.FeatureGeometry" {
			t.Fatal("geometry value changed")
		}
		if value.String() == "" {
			t.Fatal("geometry lacks diagnostic value")
		}
		if b, path := value.EnumDescriptor(); len(b) == 0 || len(path) != 1 {
			t.Fatal("geometry descriptor missing")
		}
	}
	for _, value := range []FeatureDecision{FeatureDecision_FEATURE_DECISION_UNSPECIFIED, FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL, FeatureDecision_FEATURE_DECISION_REJECTED, FeatureDecision_FEATURE_DECISION_MISSING, FeatureDecision_FEATURE_DECISION_OCCLUDED, 99} {
		if *value.Enum() != value || int32(value.Number()) != int32(value) || value.Descriptor().FullName() != "velocity.recording.v1.FeatureDecision" {
			t.Fatal("decision value changed")
		}
		if value.String() == "" {
			t.Fatal("decision lacks diagnostic value")
		}
		if b, path := value.EnumDescriptor(); len(b) == 0 || len(path) != 1 {
			t.Fatal("decision descriptor missing")
		}
	}
}

func TestBodyRegistrationSharedWireFixture(t *testing.T) {
	b, err := os.ReadFile("../../../proto/velocity_recording/v1/testdata/body-registration.pb")
	if err != nil {
		t.Fatal(err)
	}
	d := new(FeatureAnnotations)
	if err := proto.Unmarshal(b, d); err != nil {
		t.Fatal(err)
	}
	a := d.Features[0].Anchor
	if a == nil || a.SourcePointIndex == nil || *a.SourcePointIndex != 0 || a.CoordinateDomain != "body_xy" || a.ZM != 0 ||
		a.XM != 1.25 || a.YM != -0.875 || a.BoundM != 0.35 || a.ReturnBoundM != 0.05 ||
		a.PhysicalRevision != 7 || a.PartFrameRevision != 1 || a.Origin != "reference_seeded_proposal" ||
		d.Features[0].Observations[0].TimestampNs != 9007199254740993 || !d.Features[1].Inactive {
		t.Fatalf("registration wire contract changed: %v", d)
	}
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(d)
	if err != nil || !bytes.Equal(encoded, b) {
		t.Fatalf("exact wire roundtrip changed: %v", err)
	}
}

func TestFeatureSegmentWirePresenceAndWeakDirectionFields(t *testing.T) {
	start, end := uint32(0), uint32(2)
	line := &FeatureLineConstraint{NormalX: 1, NormalY: 0, OffsetM: -0.9, NormalBoundRad: 0.1, SourceStartIndex: &start, SourceEndIndex: &end}
	a := &FeatureAnchor{Method: "manual_named_segment_v1", Line: line}
	if a.GetLine() != line || line.GetNormalX() != 1 || line.GetNormalY() != 0 || line.GetOffsetM() != -0.9 || line.GetNormalBoundRad() != 0.1 || line.GetSourceStartIndex() != 0 || line.GetSourceEndIndex() != 2 {
		t.Fatal("segment contract changed")
	}
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var decoded FeatureAnchor
	if err = proto.Unmarshal(b, &decoded); err != nil || !proto.Equal(a, &decoded) || decoded.SourcePointIndex != nil || decoded.Line.SourceStartIndex == nil {
		t.Fatal("line acquired a point or lost optional zero", err)
	}
	proto.Reset(line)
	if line.SourceStartIndex != nil || line.SourceEndIndex != nil || line.GetOffsetM() != 0 {
		t.Fatal("reset retained line evidence")
	}
}

func TestFeatureSegmentSharedWireFixture(t *testing.T) {
	bytesIn, err := os.ReadFile("../../../proto/velocity_recording/v1/testdata/segment-registration.pb")
	if err != nil {
		t.Fatal(err)
	}
	doc := new(FeatureAnnotations)
	if err := proto.Unmarshal(bytesIn, doc); err != nil {
		t.Fatal(err)
	}
	feature := doc.Features[0]
	a := feature.Anchor
	if feature.Geometry != FeatureGeometry_FEATURE_GEOMETRY_EDGE || a.SourcePointIndex != nil || a.XM != 0 || a.YM != 0 || a.Line == nil || a.Line.SourceStartIndex == nil || a.Line.GetSourceStartIndex() != 0 || a.Line.GetSourceEndIndex() != 2 || a.Line.OffsetM != -0.875 || a.Line.NormalBoundRad != 0.1 || feature.Observations[0].TimestampNs != 9007199254740993 || !doc.Features[1].Inactive {
		t.Fatal("shared line contract changed")
	}
	out, err := (proto.MarshalOptions{Deterministic: true}).Marshal(doc)
	if err != nil || !bytes.Equal(out, bytesIn) {
		t.Fatal("shared segment fixture changed", err)
	}
}
