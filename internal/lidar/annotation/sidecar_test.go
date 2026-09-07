package annotation

import (
	"os"
	"path/filepath"
	"testing"
)

func reviewedObject(id, class string) Object {
	return Object{ObjectID: id, Class: class, Confidence: 1, Status: StatusReviewed}
}

func mask(objectID string, sampleID int, idx ...int) FrameMask {
	return FrameMask{
		ObjectID: objectID, SampleID: sampleID, PointIndices: idx,
		Completeness: MaskComplete, Visibility: VisiblePresent, Status: StatusReviewed,
	}
}

// Gate: automatic proposals cannot become reviewed labels through export or
// reload. The filter lives on the read path scoring calls, so a save-and-load
// cycle cannot launder a suggestion into evidence.
func TestProposalsNeverBecomeReference(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{
		reviewedObject("obj_real", "car"),
		{ObjectID: "obj_guess", Class: "car", Status: StatusProposed,
			Provenance: Provenance{Algorithm: "dbscan-seed", AlgorithmVersion: "v1"}},
	}
	s.Masks = []FrameMask{
		mask("obj_real", 0, 0, 1),
		// A confirmed mask on a speculative object is still speculative.
		mask("obj_guess", 0, 2, 3),
		// A proposed mask on a confirmed object likewise.
		{ObjectID: "obj_real", SampleID: 1, PointIndices: []int{0},
			Completeness: MaskPartial, Visibility: VisiblePresent, Status: StatusProposed},
	}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded, err := LoadSidecar(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	reviewed := reloaded.ReviewedMasks()
	if len(reviewed) != 1 {
		t.Fatalf("reviewed masks = %d, want 1; a proposal reached the reference set", len(reviewed))
	}
	if reviewed[0].ObjectID != "obj_real" || reviewed[0].SampleID != 0 {
		t.Fatalf("wrong mask survived: %+v", reviewed[0])
	}
	// The proposals are still on disk — rejected, not deleted, so the same
	// suggestion is not offered again as though it were new.
	if len(reloaded.Masks) != 3 {
		t.Fatalf("proposals were dropped from storage: %d masks", len(reloaded.Masks))
	}
}

// Two objects cannot own the same return. A conflict is for a person to
// resolve, not something to average away.
func TestConflictingMembershipIsRejected(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_a", "car"), reviewedObject("obj_b", "car")}
	s.Masks = []FrameMask{mask("obj_a", 0, 0, 1), mask("obj_b", 0, 1, 2)}

	if err := SaveSidecar(p, s); err == nil {
		t.Fatal("a point claimed by two objects was written")
	}
	// The same point in different samples is not a conflict.
	s.Masks = []FrameMask{mask("obj_a", 0, 0, 1), mask("obj_b", 1, 0, 1)}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatalf("the same index in different samples was refused: %v", err)
	}
}

func TestUncertainPointsCannotAlsoBeMembers(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_a", "car")}
	m := mask("obj_a", 0, 0, 1)
	m.UncertainIndices = []int{1, 2}
	s.Masks = []FrameMask{m}

	if err := SaveSidecar(p, s); err == nil {
		t.Fatal("a point was both a confirmed member and uncertain")
	}
}

// Gate: reference identities survive a predicted split, merge, and fresh
// pipeline run. Nothing here is keyed on a track UUID, so re-running the
// tracker cannot change the reference.
func TestReferenceIdentityIsIndependentOfPredictedTracks(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_car", "car")}
	s.Masks = []FrameMask{mask("obj_car", 0, 0, 1, 2), mask("obj_car", 1, 0, 1)}
	// One reference object mapping to two predicted tracks is the split case,
	// recorded as a fact about the prediction rather than about the object.
	s.Correspondences = []TrackCorrespondence{{
		ObjectID: "obj_car",
		TrackIDs: []string{"trk_18952226", "trk_04e4ebd5"},
		StartNs:  1_000_000_000, EndNs: 2_000_000_000,
		Note: "predicted split; one physical vehicle",
	}}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatalf("save: %v", err)
	}

	// A fresh pipeline run produces entirely different track IDs. The
	// reference must not notice.
	reloaded, err := LoadSidecar(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	reloaded.Correspondences[0].TrackIDs = []string{"trk_totally_different"}
	if err := SaveSidecar(p, reloaded); err != nil {
		t.Fatalf("save after a fresh run: %v", err)
	}

	again, err := LoadSidecar(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(again.ReviewedMasks()) != 2 {
		t.Fatalf("reference masks changed with the prediction: %d", len(again.ReviewedMasks()))
	}
	if again.Objects[0].ObjectID != "obj_car" {
		t.Fatalf("reference identity changed: %+v", again.Objects[0])
	}
}

func TestSidecarRejectsUnknownReferences(t *testing.T) {
	p := synthPack(t)
	cases := map[string]func(*Sidecar){
		"mask for an undeclared object": func(s *Sidecar) {
			s.Masks = []FrameMask{mask("obj_ghost", 0, 0)}
		},
		"correspondence for an undeclared object": func(s *Sidecar) {
			s.Correspondences = []TrackCorrespondence{{ObjectID: "obj_ghost"}}
		},
		"duplicate object id": func(s *Sidecar) {
			s.Objects = append(s.Objects, reviewedObject("obj_a", "bus"))
		},
		"unknown status": func(s *Sidecar) {
			s.Objects[0].Status = "probably"
		},
		"unknown visibility": func(s *Sidecar) {
			m := mask("obj_a", 0, 0)
			m.Visibility = "squinting"
			s.Masks = []FrameMask{m}
		},
		"index past the sample": func(s *Sidecar) {
			s.Masks = []FrameMask{mask("obj_a", 2, 5)}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := NewSidecar(p)
			s.Objects = []Object{reviewedObject("obj_a", "car")}
			mutate(s)
			if err := SaveSidecar(p, s); err == nil {
				t.Fatal("invalid sidecar was written")
			}
		})
	}
}

// A damaged sidecar must report why rather than opening as if empty, which
// would silently discard a session's labelling.
func TestDamagedSidecarFailsLoudly(t *testing.T) {
	p := synthPack(t)
	if err := os.WriteFile(filepath.Join(p.Dir, sidecarFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSidecar(p); err == nil {
		t.Fatal("a corrupt sidecar loaded as though it were valid")
	}
}

func TestMissingSidecarStartsEmpty(t *testing.T) {
	p := synthPack(t)
	s, err := LoadSidecar(p)
	if err != nil {
		t.Fatalf("a pack with no annotations yet should not be an error: %v", err)
	}
	if len(s.Objects) != 0 || s.PackDigest != p.Manifest.PackDigest {
		t.Fatalf("fresh sidecar is wrong: %+v", s)
	}
}

// Saving an unchanged document must produce identical bytes, or a diff cannot
// distinguish a real edit from a reordering.
func TestSaveIsCanonicalAndStable(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_b", "car"), reviewedObject("obj_a", "bus")}
	s.Masks = []FrameMask{mask("obj_b", 1, 3, 1), mask("obj_a", 0, 2, 0)}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatalf("save: %v", err)
	}

	first, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	if first.Objects[0].ObjectID != "obj_a" {
		t.Fatalf("objects not sorted: %+v", first.Objects)
	}
	if first.Masks[0].SampleID != 0 || first.Masks[0].PointIndices[0] != 0 {
		t.Fatalf("masks not canonical: %+v", first.Masks)
	}
}
