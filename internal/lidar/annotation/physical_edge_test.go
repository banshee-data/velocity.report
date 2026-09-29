package annotation

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Documents that are valid in ways the fixture does not exercise: a face
// anchor that locates only its face, a face offset on an object with no body,
// an unknown position, an unknown gap, and records given in no order.
func TestPhysicalValidationAcceptsHonestUnknowns(t *testing.T) {
	p := physPack(t)
	r := validPhysical(p)
	rear := &r.Objects[0].Keyframes[1]
	rear.Anchor.OffsetM, rear.Anchor.OffsetBoundM = nil, nil
	r.Objects[1].Keyframes[0].Anchor = PhysicalAnchor{Kind: AnchorRearFace, OffsetM: fp(2.1), OffsetBoundM: fp(0.2)}
	r.Objects[1].Body = nil
	r.Objects[0].Keyframes[0].Position = PositionBound{Status: EvidenceUnknown}
	r.Following[0].Gaps = append(r.Following[0].Gaps, FollowingGap{
		SampleID: 2, TimestampNs: physTime(2), Status: EvidenceUnknown,
		FollowerFront: EndpointEvidence{Status: EvidenceUnknown}, LeaderRear: EndpointEvidence{Status: EvidenceUnknown},
	})
	r.Following = append(r.Following, FollowingReference{
		FollowingID: "follow-car-2", FollowerObjectID: "car-2", Decision: FollowingNoLeader,
		Interval: FrameInterval{FirstSample: 0, LastSample: 5}, Review: independentReview(),
	})
	r.Following[0], r.Following[1] = r.Following[1], r.Following[0]
	kf := &r.Objects[0].Keyframes[0]
	kf.SharedErrors = append(kf.SharedErrors, SharedError{Observation: "a rear fit", Components: []string{"position", "rear"}})
	r.RecordOrigins = r.currentOrigins()
	if err := r.Validate(p); err != nil {
		t.Fatalf("honest unknowns refused: %v", err)
	}
	before, _ := r.ContentDigest()
	r.canonicalise()
	if r.Following[0].FollowingID != "follow-car-1" || kf.SharedErrors[0].Observation != "a rear fit" {
		t.Fatalf("canonical order: %s, %s", r.Following[0].FollowingID, kf.SharedErrors[0].Observation)
	}
	if after, _ := r.ContentDigest(); after != before {
		t.Fatal("canonicalising changed the content digest")
	}

	// A digest of a document that cannot be encoded is an error, not a hash.
	r.Objects[0].Body.Length.LowerM = fp(math.NaN())
	if _, err := r.ContentDigest(); err == nil {
		t.Fatal("a NaN bound was digested")
	}
}

func TestPhysicalValidationRefusesUnnamedRecordsAndOtherUnits(t *testing.T) {
	p := physPack(t)
	for name, mutate := range map[string]func(*PhysicalReferenceSet){
		"body without id":      func(r *PhysicalReferenceSet) { r.Objects[0].Body.BodyID = "" },
		"following without id": func(r *PhysicalReferenceSet) { r.Following[0].FollowingID = "" },
	} {
		r := validPhysical(p)
		mutate(r)
		r.RecordOrigins = r.currentOrigins()
		if err := r.Validate(p); err == nil || !strings.Contains(err.Error(), "has no id") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A pack in some other unit cannot hold references in metres, and a
	// reference that follows it into that unit is refused all the same.
	feet := *p
	feet.Manifest.Coordinate.Units = "feet"
	r := validPhysical(p)
	r.Source.Units = "feet"
	r.RecordOrigins = r.currentOrigins()
	if err := r.Validate(&feet); err == nil || !strings.Contains(err.Error(), "need a pack in metres") {
		t.Fatalf("a pack in feet: %v", err)
	}
}

// ValidateLinks is public, so it holds up against inputs nobody validated:
// a mask at a sample the pack does not have cannot be read for its span.
func TestPhysicalLinksAgainstUnvalidatedInputs(t *testing.T) {
	p := physPack(t)
	r := validPhysical(p)
	r.Objects[0].Body.Height = DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(1), UpperM: fp(2), Support: frames(99)}
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("car-1", "car"), reviewedObject("car-2", "car")}
	s.Masks = []FrameMask{mask("car-1", 99, 0)}
	if err := r.ValidateLinks(p, s); err == nil {
		t.Fatal("a span was read from a sample outside the pack")
	}
}

func TestPhysicalSaveRefusesInvalidAndForeignDocuments(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	r := validPhysical(p)
	r.Objects[0].Keyframes[0].SampleID = 99
	before, _ := json.Marshal(r)
	if err := SavePhysicalReferences(p, r); err == nil || !strings.Contains(err.Error(), "invalid physical reference edit") {
		t.Fatalf("a keyframe outside the pack was saved: %v", err)
	}
	if after, _ := json.Marshal(r); !bytes.Equal(before, after) {
		t.Fatal("a refused save changed the caller")
	}
	if _, err := os.Stat(filepath.Join(p.Dir, physicalReferenceFile)); !os.IsNotExist(err) {
		t.Fatal("a refused save wrote a file")
	}
	// A well-formed document for another pack, placed as the head, does not
	// load here.
	foreign := validPhysical(p)
	foreign.PackDigest = "sha256:another-pack"
	foreign.RecordOrigins = foreign.currentOrigins()
	b, _ := json.MarshalIndent(foreign, "", "  ")
	if err := os.WriteFile(filepath.Join(p.Dir, physicalReferenceFile), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReferences(p); err == nil || !strings.Contains(err.Error(), "written against pack") {
		t.Fatalf("another pack's references loaded: %v", err)
	}
}
