package annotation

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withoutMasks is the sidecar with some objects' masks at some samples
// removed, as a later membership edit might leave it.
func withoutMasks(s *Sidecar, drop map[string][]int) *Sidecar {
	c := *s
	c.Masks = nil
	for _, m := range s.Masks {
		keep := true
		for _, sample := range drop[m.ObjectID] {
			keep = keep && m.SampleID != sample
		}
		if keep {
			c.Masks = append(c.Masks, m)
		}
	}
	return &c
}

func problemFor(problems []LinkProblem, record string) string {
	for _, lp := range problems {
		if lp.Record == record {
			return lp.Problem
		}
	}
	return ""
}

// Observed claims are held to the returns that are supposed to show them:
// every cited frame must hold the object, an observed dimension must be
// measurable in one of them, and an observed bumper must be reached by the
// keyframe's own returns along its axis.
func TestPhysicalObservedClaimsAreHeldToTheReturns(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	cases := []struct {
		name    string
		mutate  func(*PhysicalReferenceSet)
		sidecar *Sidecar
		record  string
		want    string
	}{
		{"front observed where only the rear returned", func(r *PhysicalReferenceSet) {
			// Sample 3 returned the rear 1.5 m; the front of a car at least
			// 4.3 m long is well beyond it.
			r.Objects[0].Keyframes[1].Front = EndpointEvidence{Status: EvidenceObserved, Support: frames(3)}
		}, s, `object "car-1" keyframe "kf-car-1-s3"`, "short of the front at 15.150 m"},
		{"rear observed behind the returns", func(r *PhysicalReferenceSet) {
			*r.Objects[1].Keyframes[0].Position.XM = 17
		}, s, `object "car-2" keyframe "kf-car-2-s0"`, "observed rear: the returns at sample 0 reach back to 17.900 m"},
		{"front face beyond the returns", func(r *PhysicalReferenceSet) {
			k := &r.Objects[0].Keyframes[0]
			k.Anchor = PhysicalAnchor{Kind: AnchorFrontFace, OffsetM: fp(2.25), OffsetBoundM: fp(0.2)}
			*k.Position.XM = 14.25
		}, s, `object "car-1" keyframe "kf-car-1-s0"`, "short of the front at 14.250 m"},
		{"bumper with no position", func(r *PhysicalReferenceSet) {
			r.Objects[0].Keyframes[0].Position = PositionBound{Status: EvidenceUnknown}
		}, s, `object "car-1" keyframe "kf-car-1-s0"`, "the keyframe states no position"},
		{"bumper from a face with no offset", func(r *PhysicalReferenceSet) {
			k := &r.Objects[0].Keyframes[1]
			k.Anchor.OffsetM, k.Anchor.OffsetBoundM = nil, nil
			k.Front = EndpointEvidence{Status: EvidenceObserved, Support: frames(3)}
		}, s, `object "car-1" keyframe "kf-car-1-s3"`, "neither that face nor the body centre"},
		{"bumper with no body", func(r *PhysicalReferenceSet) { r.Objects[1].Body = nil }, s,
			`object "car-2" keyframe "kf-car-2-s0"`, "the body states no length to place it by"},
		{"partial height beyond the returns", func(r *PhysicalReferenceSet) { *r.Objects[0].Body.Height.LowerM = 2 }, s,
			`object "car-1" body "body-car-1"`, "observed partial height is at least 2.000 m, but its supporting frames' returns span at most 1.200 m"},
		{"yaw citing a frame without the car", func(*PhysicalReferenceSet) {}, withoutMasks(s, map[string][]int{"car-1": {2}}),
			`object "car-1" keyframe "kf-car-1-s3"`, `yaw cites frame 2, where "car-1" has no returns in its mask`},
		{"gap citing a follower that is not there", func(*PhysicalReferenceSet) {}, withoutMasks(s, map[string][]int{"car-1": {0}}),
			`following "follow-car-1"`, "gap at sample 0 follower_front cites frame 0"},
		{"gap citing a leader that is not there", func(*PhysicalReferenceSet) {}, withoutMasks(s, map[string][]int{"car-2": {0}}),
			`following "follow-car-1"`, "gap at sample 0 leader_rear cites frame 0"},
		{"gap frames without one party", func(r *PhysicalReferenceSet) {
			r.Following[0].Gaps[0].Support = frames(0, 2)
		}, withoutMasks(s, map[string][]int{"car-2": {2}}), `following "follow-car-1"`, `gap at sample 0 cites frame 2, where "car-2" has no returns`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := validPhysical(p)
			c.mutate(r)
			r.RecordOrigins = r.currentOrigins()
			if err := r.Validate(p); err != nil {
				t.Fatalf("the mutation broke structural validation: %v", err)
			}
			got := problemFor(r.LinkProblems(p, c.sidecar), c.record)
			if !strings.Contains(got, c.want) {
				t.Fatalf("%s: %q (want %q); all: %v", c.record, got, c.want, r.LinkProblems(p, c.sidecar))
			}
			if err := r.ValidateLinks(p, c.sidecar); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ValidateLinks: %v", err)
			}
		})
	}
	// A front face that the returns do reach is observed.
	r := validPhysical(p)
	k := &r.Objects[0].Keyframes[0]
	k.Anchor = PhysicalAnchor{Kind: AnchorFrontFace, OffsetM: fp(2.25), OffsetBoundM: fp(0.2)}
	*k.Position.XM = 12.25
	if err := r.ValidateLinks(p, s); err != nil {
		t.Fatalf("a front face at the returns' front: %v", err)
	}
}

// What was seen at a keyframe or a gap is cited from that instant, and a
// gap's bumper is no better known than the party's own keyframe there says.
func TestPhysicalInstantClaimsAreTiedToTheirInstant(t *testing.T) {
	p := physPack(t)
	kf := func(r *PhysicalReferenceSet) *PhysicalKeyframe { return &r.Objects[0].Keyframes[0] }
	gap := func(r *PhysicalReferenceSet) *FollowingGap { return &r.Following[0].Gaps[0] }
	cases := []struct {
		name   string
		mutate func(*PhysicalReferenceSet)
		want   string
	}{
		{"keyframe position seen elsewhere", func(r *PhysicalReferenceSet) { kf(r).Position.Support = frames(1) },
			"position is observed but does not cite its own sample 0"},
		{"keyframe bumper seen elsewhere", func(r *PhysicalReferenceSet) { kf(r).Front.Support = frames(1) },
			"front is observed but does not cite its own sample 0"},
		{"gap seen elsewhere", func(r *PhysicalReferenceSet) { gap(r).Support = frames(1) },
			"gap is observed but does not cite its own sample 0"},
		{"gap bumper seen elsewhere", func(r *PhysicalReferenceSet) { gap(r).FollowerFront.Support = frames(1) },
			"follower_front is observed but does not cite its own sample 0"},
		{"observed bumper with no keyframe there", func(r *PhysicalReferenceSet) {
			g := gap(r)
			g.SampleID, g.TimestampNs = 1, physTime(1)
			g.Support, g.FollowerFront.Support, g.LeaderRear.Support = frames(1), frames(1), frames(1)
		}, `an observed follower_front needs "car-1"'s keyframe at sample 1`},
		{"bumper of an ambiguous axis", func(r *PhysicalReferenceSet) {
			k := kf(r)
			k.Yaw.Axis = AxisFrontRearAmbiguous
			k.Front, k.Rear = EndpointEvidence{Status: EvidenceUnknown}, EndpointEvidence{Status: EvidenceUnknown}
		}, `follower_front cannot be named: "car-1"'s keyframe at sample 0 has a front_rear_ambiguous axis`},
		{"bumper better known than its keyframe", func(r *PhysicalReferenceSet) {
			r.Objects[1].Keyframes[0].Rear = EndpointEvidence{Status: EvidenceInferred, Support: frames(0)}
		}, `observed leader_rear cannot rest on "car-2"'s keyframe at sample 0, which states that bumper inferred`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := validPhysical(p)
			c.mutate(r)
			r.RecordOrigins = r.currentOrigins()
			if err := r.Validate(p); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("refused for the wrong reason: %v (want %q)", err, c.want)
			}
		})
	}
	// An inferred gap needs no keyframe at its instant.
	r := validPhysical(p)
	r.Following[0].Gaps = append(r.Following[0].Gaps, FollowingGap{
		SampleID: 1, TimestampNs: physTime(1), Status: EvidenceInferred, LowerM: fp(5), UpperM: fp(6),
		FollowerFront: EndpointEvidence{Status: EvidenceInferred, Support: EvidenceSupport{External: "constant speed"}},
		LeaderRear:    EndpointEvidence{Status: EvidenceInferred, Support: EvidenceSupport{External: "constant speed"}},
		Support:       EvidenceSupport{External: "constant speed"},
	})
	r.RecordOrigins = r.currentOrigins()
	if err := r.Validate(p); err != nil {
		t.Fatalf("an inferred gap between keyframes: %v", err)
	}
}

// Two reviewed answers to what one follower was following, over the same
// samples, cannot both stand.
func TestPhysicalFollowingRecordsCannotContradict(t *testing.T) {
	p := physPack(t)
	second := func(decision FollowingDecision, leader string, iv FrameInterval) FollowingReference {
		return FollowingReference{FollowingID: "follow-car-1-again", FollowerObjectID: "car-1", Decision: decision,
			LeaderObjectID: leader, Interval: iv, Review: independentReview()}
	}
	whole := FrameInterval{FirstSample: 0, LastSample: 5}
	cases := []struct {
		name string
		f    func() FollowingReference
		want string
	}{
		{"leader against no leader", func() FollowingReference {
			return second(FollowingNoLeader, "", FrameInterval{FirstSample: 3, LastSample: 4})
		},
			"over samples [3, 4] and disagree"},
		{"two leaders", func() FollowingReference { return second(FollowingLeader, "car-3", whole) }, "disagree"},
		{"two gaps at one sample", func() FollowingReference {
			f := second(FollowingLeader, "car-2", whole)
			f.GapDefinition, f.Gaps = GapAlongFollowerAxis, validPhysical(p).Following[0].Gaps
			return f
		}, `both give follower "car-1" a gap at sample 0`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := validPhysical(p)
			r.Following = append(r.Following, c.f())
			r.RecordOrigins = r.currentOrigins()
			if err := r.Validate(p); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("refused for the wrong reason: %v (want %q)", err, c.want)
			}
		})
	}
	// Apart in time, or one of them only a proposal, they may differ.
	for name, f := range map[string]FollowingReference{
		"disjoint": second(FollowingNoLeader, "", FrameInterval{FirstSample: 5, LastSample: 5}),
		"proposal": func() FollowingReference {
			f := second(FollowingNoLeader, "", whole)
			f.Review.Status = StatusProposed
			return f
		}(),
	} {
		r := validPhysical(p)
		r.Following[0].Interval.LastSample = 4
		r.Following = append(r.Following, f)
		r.RecordOrigins = r.currentOrigins()
		if err := r.Validate(p); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A membership edit made after the references were saved is not refused;
// the next load lists the references it invalidated, the next save refuses
// them, and removing them clears it. No file is rewritten to say so.
func TestPhysicalReferencesMadeStaleByAMembershipEdit(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	r := validPhysical(p)
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	if loaded, err := LoadPhysicalReferences(p); err != nil || len(loaded.Stale()) != 0 || loaded.StaleAgainst() != 1 {
		t.Fatalf("fresh references: %v %v", err, loaded.Stale())
	}
	physicalBytes := readPhysical(t, p)

	// Reject car-2 and drop car-1's mask at sample 3.
	s, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := range s.Objects {
		if s.Objects[i].ObjectID == "car-2" {
			s.Objects[i].Status = StatusRejected
		}
	}
	s = withoutMasks(s, map[string][]int{"car-1": {3}})
	s.Change.Operation = "review"
	if err := SaveSidecar(p, s); err != nil {
		t.Fatalf("a membership edit was refused: %v", err)
	}
	if !bytes.Equal(physicalBytes, readPhysical(t, p)) {
		t.Fatal("a membership edit rewrote the references")
	}

	loaded, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	stale := loaded.Stale()
	if loaded.StaleAgainst() != 2 || len(stale) != 3 ||
		!strings.Contains(problemFor(stale, `object "car-1" keyframe "kf-car-1-s3"`), "cites frame 3") ||
		!strings.Contains(problemFor(stale, `object "car-2"`), "rejected in annotation revision 2") ||
		!strings.Contains(problemFor(stale, `following "follow-car-1"`), `leader "car-2" is rejected`) {
		t.Fatalf("stale against revision %d: %v", loaded.StaleAgainst(), stale)
	}
	if old, err := LoadPhysicalReferenceRevision(p, 1); err != nil || len(old.Stale()) != 3 {
		t.Fatalf("a retained revision's links were not checked: %v", err)
	}
	if err := SavePhysicalReferences(p, loaded); err == nil || !strings.Contains(err.Error(), "do not hold against annotation revision 2") {
		t.Fatalf("stale references were saved: %v", err)
	}

	// Repair: drop car-2's references and the gap, and the keyframe whose
	// frame no longer holds car-1.
	loaded.Objects = loaded.Objects[:1]
	loaded.Objects[0].Keyframes = []PhysicalKeyframe{loaded.Objects[0].Keyframes[0], loaded.Objects[0].Keyframes[2]}
	loaded.Following = nil
	loaded.Change.Operation = "repair"
	if err := SavePhysicalReferences(p, loaded); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Stale()) != 0 || loaded.StaleAgainst() != 2 {
		t.Fatalf("after repair: %v", loaded.Stale())
	}

	// A sidecar that cannot be read cannot be checked against.
	if err := os.WriteFile(filepath.Join(p.Dir, sidecarFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReferences(p); err == nil || !strings.Contains(err.Error(), "check physical reference links") {
		t.Fatalf("loaded against a damaged sidecar: %v", err)
	}
	if _, err := LoadPhysicalReferenceRevision(p, 1); err == nil {
		t.Fatal("a retained revision loaded against a damaged sidecar")
	}
}

// The dry run makes the import's own checks against what is stored and
// writes nothing; an import that is sound alone but not merged is refused.
func TestPreparePhysicalImportChecksTheMerge(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	if _, err := ImportPhysicalReferences(p, importOf(p), Provenance{Author: "a"}, false); err != nil {
		t.Fatal(err)
	}
	head := readPhysical(t, p)
	// A keyframe for car-1 at a new sample: it needs car-1's stored body to
	// place its bumpers, which only the merge has.
	add := importOf(p)
	k := validPhysical(p).Objects[0].Keyframes[0]
	k.KeyframeID, k.SampleID, k.TimestampNs = "kf-car-1-s4", 4, physTime(4)
	*k.Position.XM = 14
	k.Position.Support, k.Yaw.Support, k.Front.Support, k.Rear.Support = frames(4), frames(4), frames(4), frames(4)
	add.Objects, add.Following = []PhysicalObject{{ObjectID: "car-1", Keyframes: []PhysicalKeyframe{k}}}, nil
	merged, err := PreparePhysicalImport(p, add, false)
	if err != nil {
		t.Fatalf("a keyframe for a stored body: %v", err)
	}
	if _, ok := merged.Keyframe("car-1", 4); !ok || !bytes.Equal(head, readPhysical(t, p)) {
		t.Fatal("the dry run did not merge, or wrote")
	}
	// A following record that contradicts the stored one.
	contra := importOf(p)
	contra.Objects = nil
	contra.Following = []FollowingReference{{FollowingID: "follow-car-1-none", FollowerObjectID: "car-1",
		Decision: FollowingNoLeader, Interval: FrameInterval{FirstSample: 0, LastSample: 5}, Review: independentReview()}}
	if _, err := PreparePhysicalImport(p, contra, false); err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("a contradicting import passed the dry run: %v", err)
	}
	// Stored references a membership edit invalidated stop every import
	// until they are repaired.
	s, _ := LoadSidecar(p)
	s = withoutMasks(s, map[string][]int{"car-2": {0}})
	s.Change.Operation = "edit"
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	if _, err := PreparePhysicalImport(p, add, false); err == nil || !strings.Contains(err.Error(), `object "car-2"`) {
		t.Fatalf("an import over stale references: %v", err)
	}
	if !bytes.Equal(head, readPhysical(t, p)) {
		t.Fatal("a refused dry run wrote")
	}
	for name, broken := range map[string]func(){
		"damaged references": func() { os.WriteFile(filepath.Join(p.Dir, physicalReferenceFile), []byte("{"), 0o600) },
		"damaged annotation": func() { os.WriteFile(filepath.Join(p.Dir, sidecarFile), []byte("{"), 0o600) },
	} {
		broken()
		if _, err := PreparePhysicalImport(p, add, false); err == nil {
			t.Errorf("%s: prepared", name)
		}
	}
}

// Nothing but whitespace may follow the document: a stray closing brace or
// bracket ends no value, so a decoder that only asks whether another value
// follows would let it through.
func TestPhysicalDocumentsRefuseTrailingData(t *testing.T) {
	p := physPack(t)
	valid, err := json.Marshal(importOf(p))
	if err != nil {
		t.Fatal(err)
	}
	for _, tail := range []string{"}", "]]]", "0", `"x"`, "{}"} {
		if _, err := ParsePhysicalImport(append(append([]byte(nil), valid...), tail...)); err == nil || !strings.Contains(err.Error(), "trailing data") {
			t.Errorf("trailing %q: %v", tail, err)
		}
	}
	if _, err := ParsePhysicalImport(append(valid, " \n\t"...)); err != nil {
		t.Errorf("trailing whitespace refused: %v", err)
	}
	physSidecar(t, p)
	if err := SavePhysicalReferences(p, validPhysical(p)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Dir, physicalReferenceFile), append(readPhysical(t, p), '}'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReferences(p); err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("a stored document with a trailing brace: %v", err)
	}
}
