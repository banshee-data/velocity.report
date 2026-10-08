package annotation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// draftStore saves the valid document without its following reference,
// changed first by mutate, and returns the pack and the stored document.
func draftStore(t *testing.T, mutate func(*PhysicalReferenceSet)) (*Pack, *PhysicalReferenceSet) {
	t.Helper()
	p := physPack(t)
	physSidecar(t, p)
	r := validPhysical(p)
	r.Following = nil
	if mutate != nil {
		mutate(r)
	}
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatalf("save: %v", err)
	}
	doc, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return p, doc
}

func carOneFollowsCarTwo() FollowingDraft {
	return FollowingDraft{
		FollowingID: "follow-car-1", FollowerObjectID: "car-1", Decision: FollowingLeader, LeaderObjectID: "car-2",
		Interval: FrameInterval{FirstSample: 0, LastSample: 5}, Reviewed: true, Author: "op",
	}
}

// The gap at sample 0 is car-2's rear minus car-1's front along car-1's axis:
// (20 - 4.2/2) - (10 + 4.5/2) = 5.65 m. Each bumper's bound is its centre's
// 0.2 m, half its length's half-width and the chord the 0.05 rad yaw bound
// sweeps over half the length; the gap adds both, and the chord car-1's yaw
// bound sweeps over the separation.
func TestDraftFollowingDerivesGapsFromReviewedKeyframes(t *testing.T) {
	p, doc := draftStore(t, nil)
	imp, skipped, err := DraftFollowing(doc, carOneFollowsCarTwo())
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Following) != 1 || len(imp.Objects) != 0 || imp.PackDigest != doc.PackDigest || imp.Source != doc.Source {
		t.Fatalf("import holds %+v", imp)
	}
	f := imp.Following[0]
	if f.GapDefinition != GapAlongFollowerAxis || len(f.Gaps) != 1 {
		t.Fatalf("drafted %d gaps under %q", len(f.Gaps), f.GapDefinition)
	}
	sw := swing(0.05)
	front := 0.2 + 0.1 + 2.25*sw
	rear := 0.2 + 0.1 + 2.1*sw
	half := 5.65*sw + (front+rear)*(1+sw)
	g := f.Gaps[0]
	for name, got := range map[string][2]float64{
		"value": {*g.ValueM, 5.65}, "lower": {*g.LowerM, 5.65 - half}, "upper": {*g.UpperM, 5.65 + half},
	} {
		if math.Abs(got[0]-got[1]) > 1e-9 {
			t.Errorf("%s %.6f, want %.6f", name, got[0], got[1])
		}
	}
	want := FollowingGap{
		SampleID: 0, TimestampNs: physTime(0), Status: EvidenceObserved, LowerM: g.LowerM, UpperM: g.UpperM, ValueM: g.ValueM,
		FollowerFront: EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
		LeaderRear:    EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
		Support:       frames(0),
	}
	if !reflect.DeepEqual(g, want) {
		t.Fatalf("gap\n got %+v\nwant %+v", g, want)
	}
	if f.Review.Status != StatusReviewed || f.Review.Origin != OriginIndependent || f.Review.Method != DraftFollowingMethod ||
		f.Review.Provenance.Author != "op" || f.Review.Provenance.CreatedUTC == "" || f.Review.UncertaintyAssumptions == "" {
		t.Fatalf("review %+v", f.Review)
	}
	wantSkipped := []SkippedGap{
		{3, `leader "car-2" has no keyframe at this sample`},
		{5, `follower "car-1"'s keyframe "kf-car-1-s5-proposal" is proposed and tracker_assisted, not reviewed and independent`},
	}
	if !reflect.DeepEqual(skipped, wantSkipped) {
		t.Fatalf("skipped %+v", skipped)
	}

	// The draft is an import the store accepts, and survives the strict
	// decode a file goes through.
	b, err := json.Marshal(imp)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePhysicalImport(b)
	if err != nil {
		t.Fatalf("parse the draft: %v", err)
	}
	stored, err := ImportPhysicalReferences(p, parsed, Provenance{Author: "op"}, false)
	if err != nil {
		t.Fatalf("import the draft: %v", err)
	}
	if len(stored.Following) != 1 || stored.RecordOrigins["following/follow-car-1"] != OriginIndependent {
		t.Fatalf("stored %+v, origins %v", stored.Following, stored.RecordOrigins)
	}
}

func TestDraftFollowingSaysWhyASampleHasNoGap(t *testing.T) {
	_, doc := draftStore(t, func(r *PhysicalReferenceSet) {
		car2 := &r.Objects[1]
		// car-2 is seen at samples 3 and 5 too, and its front at sample 0.
		for _, s := range []int{3, 5} {
			car2.Keyframes = append(car2.Keyframes, PhysicalKeyframe{
				KeyframeID: fmt.Sprintf("kf-car-2-s%d", s), SampleID: s, TimestampNs: physTime(s),
				Anchor:   PhysicalAnchor{Kind: AnchorBodyCentre},
				Position: PositionBound{Status: EvidenceObserved, XM: fp(float64(20 + s)), YM: fp(0), BoundM: fp(0.2), Support: frames(s)},
				Yaw:      YawBound{Status: EvidenceObserved, Axis: AxisResolved, YawRad: fp(0), BoundRad: fp(0.05), Support: frames(s)},
				Front:    EndpointEvidence{Status: EvidenceUnknown},
				Rear:     EndpointEvidence{Status: EvidenceObserved, Support: frames(s)},
				Review:   independentReview(),
			})
		}
		car2.Keyframes[0].Front = EndpointEvidence{Status: EvidenceObserved, Support: frames(0)}
	})
	_, skipped, err := DraftFollowing(doc, carOneFollowsCarTwo())
	if err != nil {
		t.Fatal(err)
	}
	want := []SkippedGap{
		// car-1 was seen only from behind at sample 3.
		{3, `follower "car-1"'s keyframe "kf-car-1-s3" places no front: unknown`},
		{5, `follower "car-1"'s keyframe "kf-car-1-s5-proposal" is proposed and tracker_assisted, not reviewed and independent`},
	}
	if !reflect.DeepEqual(skipped, want) {
		t.Fatalf("skipped\n got %+v\nwant %+v", skipped, want)
	}

	// The other way round, car-1's rear is behind car-2's front: a decision
	// the operator got backwards gets no gap.
	reversed := carOneFollowsCarTwo()
	reversed.FollowerObjectID, reversed.LeaderObjectID = "car-2", "car-1"
	reversed.Interval = FrameInterval{FirstSample: 0, LastSample: 0}
	imp, skipped, err := DraftFollowing(doc, reversed)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Following[0].Gaps) != 0 || imp.Following[0].GapDefinition != "" || len(skipped) != 1 ||
		!strings.Contains(skipped[0].Reason, "the leader's rear is 14.35 m behind the follower's front") {
		t.Fatalf("reversed pair drafted %+v, skipped %+v", imp.Following[0].Gaps, skipped)
	}
}

// A bumper inferred from outside the pack makes an inferred gap, which rests
// on the keyframes it was derived from rather than on frames.
func TestDraftFollowingInferredBumper(t *testing.T) {
	p, doc := draftStore(t, func(r *PhysicalReferenceSet) {
		r.Objects[0].Keyframes[0].Front = EndpointEvidence{Status: EvidenceInferred,
			Support: EvidenceSupport{External: "bumper surveyed against the kerb marks"}}
	})
	imp, _, err := DraftFollowing(doc, carOneFollowsCarTwo())
	if err != nil {
		t.Fatal(err)
	}
	g := imp.Following[0].Gaps[0]
	if g.Status != EvidenceInferred || g.FollowerFront.Status != EvidenceInferred || g.LeaderRear.Status != EvidenceObserved ||
		len(g.Support.Frames) != 0 || g.Support.External != `derived from keyframes "kf-car-1-s0" and "kf-car-2-s0"` {
		t.Fatalf("gap %+v", g)
	}
	if _, err := PreparePhysicalImport(p, imp, false); err != nil {
		t.Fatalf("an inferred gap was refused: %v", err)
	}
}

func TestDraftFollowingDecisionsWithoutALeader(t *testing.T) {
	p, doc := draftStore(t, nil)
	for _, decision := range []FollowingDecision{FollowingNoLeader, FollowingAmbiguous} {
		imp, skipped, err := DraftFollowing(doc, FollowingDraft{
			FollowerObjectID: "car-1", Decision: decision, Interval: FrameInterval{FirstSample: 1, LastSample: 4}, Author: "op",
		})
		if err != nil {
			t.Fatal(err)
		}
		f := imp.Following[0]
		if !strings.HasPrefix(f.FollowingID, "following_") || len(f.Gaps) != 0 || f.GapDefinition != "" ||
			f.Review.UncertaintyAssumptions != "" || f.Review.Status != StatusProposed || len(skipped) != 0 {
			t.Fatalf("%s drafted %+v, skipped %+v", decision, f, skipped)
		}
		if _, err := PreparePhysicalImport(p, imp, false); err != nil {
			t.Fatalf("%s: %v", decision, err)
		}
	}
	for name, d := range map[string]FollowingDraft{
		"leader without a leader": {FollowerObjectID: "car-1", Decision: FollowingLeader},
		"no leader naming one":    {FollowerObjectID: "car-1", Decision: FollowingNoLeader, LeaderObjectID: "car-2"},
		"ambiguous naming one":    {FollowerObjectID: "car-1", Decision: FollowingAmbiguous, LeaderObjectID: "car-2"},
		"no decision":             {FollowerObjectID: "car-1", LeaderObjectID: "car-2"},
	} {
		if _, _, err := DraftFollowing(doc, d); err == nil {
			t.Errorf("%s was drafted", name)
		}
	}
}

// A following record is reviewed in its import: a proposal is reviewed by
// drafting it again, reviewed, and importing it with replace. Its origin
// holds across the redraft.
func TestDraftFollowingReviewByRedraft(t *testing.T) {
	p, doc := draftStore(t, nil)
	proposal := carOneFollowsCarTwo()
	proposal.Reviewed = false
	proposal.TrackerSource = "lidar_track_solid_bodies cv_kf_v1 seq-000001"
	imp, _, err := DraftFollowing(doc, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if r := imp.Following[0].Review; r.Origin != OriginTrackerAssisted || r.TrackerSource != proposal.TrackerSource || r.Status != StatusProposed {
		t.Fatalf("review %+v", r)
	}
	if _, err := ImportPhysicalReferences(p, imp, Provenance{Author: "op"}, false); err != nil {
		t.Fatal(err)
	}

	reviewed := proposal
	reviewed.Reviewed = true
	if imp, _, err = DraftFollowing(doc, reviewed); err != nil {
		t.Fatal(err)
	}
	if _, err := PreparePhysicalImport(p, imp, false); err == nil || !strings.Contains(err.Error(), "following follow-car-1 already exists") {
		t.Fatalf("a redraft replaced without replace: %v", err)
	}
	stored, err := ImportPhysicalReferences(p, imp, Provenance{Author: "reviewer"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if r := stored.Following[0].Review; r.Status != StatusReviewed || r.ScoredAsTruth() {
		t.Fatalf("reviewed tracker-assisted record %+v", r)
	}

	// Drafting the same ID as independent cannot launder its origin.
	independent := reviewed
	independent.TrackerSource = ""
	if imp, _, err = DraftFollowing(doc, independent); err != nil {
		t.Fatal(err)
	}
	if _, err := PreparePhysicalImport(p, imp, true); err == nil || !strings.Contains(err.Error(), "created tracker_assisted and cannot become independent") {
		t.Fatalf("a tracker-assisted record became independent: %v", err)
	}
}
