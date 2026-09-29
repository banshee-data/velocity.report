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
	s := physSidecar(t, p)
	s.Masks = append(s.Masks, mask("car-1", 99, 0))
	r := validPhysical(p)
	r.Objects[0].Body.Height = DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(1), UpperM: fp(2), Support: frames(99)}
	if err := r.ValidateLinks(p, s); err == nil || !strings.Contains(err.Error(), "outside the pack") {
		t.Fatalf("a span was read from a sample outside the pack: %v", err)
	}
	// Likewise a bumper checked at a keyframe outside the pack.
	r = validPhysical(p)
	k := &r.Objects[0].Keyframes[0]
	k.SampleID = 99
	k.Position.Support, k.Yaw.Support, k.Front.Support, k.Rear.Support = frames(99), frames(99), frames(99), frames(99)
	if err := r.ValidateLinks(p, s); err == nil || !strings.Contains(err.Error(), "outside the pack") {
		t.Fatalf("a bumper was checked at a sample outside the pack: %v", err)
	}
}

// A rejected mask holds no returns for the object, and a side face with its
// offset places the centre a bumper is checked from.
func TestPhysicalLinksReadOnlyLiveMasksAndSideFaces(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	for i := range s.Masks {
		if s.Masks[i].ObjectID == "car-1" && s.Masks[i].SampleID == 2 {
			s.Masks[i].Status = StatusRejected
		}
	}
	r := validPhysical(p)
	if err := r.ValidateLinks(p, s); err == nil || !strings.Contains(err.Error(), `yaw cites frame 2, where "car-1" has no returns`) {
		t.Fatalf("a rejected mask counted as returns: %v", err)
	}
	s = physSidecar(t, physPack(t))
	for _, side := range []struct {
		kind AnchorKind
		y    float64
	}{{AnchorLeftFace, 0.9}, {AnchorRightFace, -0.9}} {
		r := validPhysical(p)
		k := &r.Objects[0].Keyframes[0]
		k.Anchor = PhysicalAnchor{Kind: side.kind, OffsetM: fp(0.9), OffsetBoundM: fp(0.1)}
		*k.Position.YM = side.y
		if err := r.ValidateLinks(p, s); err != nil {
			t.Errorf("bumpers from the %s: %v", side.kind, err)
		}
		*k.Position.XM = 16
		if err := r.ValidateLinks(p, s); err == nil {
			t.Errorf("a front 6 m beyond the returns was observed from the %s", side.kind)
		}
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
