package perframeeval

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// reviewPinned takes a record through the editing service: proposed, saved,
// then reviewed, so it carries the membership it was reviewed against.
func reviewPinned(t *testing.T, pack *annotation.Pack, kind annotation.PhysicalRecordKind, object, record string,
	demote func(*annotation.PhysicalObject)) {
	t.Helper()
	doc, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	for i := range doc.Objects {
		if doc.Objects[i].ObjectID == object {
			demote(&doc.Objects[i])
		}
	}
	doc.Change = annotation.Provenance{Author: "op", Operation: "demote"}
	if err := annotation.SavePhysicalReferences(pack, doc); err != nil {
		t.Fatal(err)
	}
	sidecar, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := annotation.ReviewPhysicalRecord(pack, annotation.PhysicalReviewRequest{
		BaseRevision: doc.Revision, BaseDigest: doc.Digest(), MembershipDigest: sidecar.Digest(),
		Kind: kind, ObjectID: object, RecordID: record, Reviewer: annotation.Provenance{Author: "rev"},
	}); err != nil {
		t.Fatalf("review %s %s: %v", kind, record, err)
	}
}

// doubtAPoint changes an object's membership at a sample without touching a
// link: a point of another object is marked uncertain in the mask. The
// returns a review rested on are then not the returns on disk.
func doubtAPoint(t *testing.T, pack *annotation.Pack, object string, sample int) {
	t.Helper()
	s, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	for i := range s.Masks {
		if s.Masks[i].ObjectID != object || s.Masks[i].SampleID != sample {
			continue
		}
		own := map[int]bool{}
		for _, idx := range s.Masks[i].PointIndices {
			own[idx] = true
		}
		doubted := 0
		for own[doubted] {
			doubted++
		}
		s.Masks[i].UncertainIndices = append(s.Masks[i].UncertainIndices, doubted)
	}
	s.Change = annotation.Provenance{Author: "op", Operation: "doubt"}
	if err := annotation.SaveSidecar(pack, s); err != nil {
		t.Fatal(err)
	}
}

// A keyframe reviewed against one membership, whose frame's membership then
// changed, is shown and not scored; the same keyframe under a frozen split
// that pins the membership it was reviewed against scores as before.
func TestPhysicalScoringRefusesADriftedKeyframe(t *testing.T) {
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	frozenPath := filepath.Join(filepath.Dir(f.PackDir), "frozen.json")
	if _, err := f.WriteFrozen(frozenPath); err != nil {
		t.Fatal(err)
	}
	// The manifest binds the current membership; the frozen split pins
	// revision 1.
	m := f.Manifest()
	m.SidecarRevision = 0
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	reviewPinned(t, pack, annotation.PhysicalRecordKeyframe, evalfixture.Follower, "kf-follow-5", func(o *annotation.PhysicalObject) {
		for i := range o.Keyframes {
			if o.Keyframes[i].KeyframeID == "kf-follow-5" {
				o.Keyframes[i].Review.Status = annotation.StatusProposed
			}
		}
	})

	before := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	wantOutcome(t, instantOf(t, before, evalfixture.Follower, 5), ComponentCentre, OutcomeScored, "")
	for _, c := range before.Caveats {
		if strings.Contains(c, "Review them again") {
			t.Fatalf("drift reported with unchanged membership: %s", c)
		}
	}

	doubtAPoint(t, pack, evalfixture.Follower, 5)
	after := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	in := instantOf(t, after, evalfixture.Follower, 5)
	for _, c := range PhysicalComponents() {
		wantOutcome(t, in, c, OutcomeUnknownGeometry, ReasonReferenceMembershipDrift)
	}
	if in.Reference == nil || in.Reference.Truth {
		t.Fatalf("a drifted keyframe was hidden or still truth: %+v", in.Reference)
	}
	// Its neighbours, reviewed without a pin, are scored as before and the
	// report says that they could not be checked.
	wantOutcome(t, instantOf(t, after, evalfixture.Follower, 0), ComponentCentre, OutcomeScored, "")
	joined := strings.Join(after.Caveats, "\n")
	if !strings.Contains(joined, "keyframe/kf-follow-5") || !strings.Contains(joined, "no membership pin") {
		t.Fatalf("caveats do not name the drift and the unpinned reviews:\n%s", joined)
	}
	if after.Accounting.Components[ComponentCentre].Scored != before.Accounting.Components[ComponentCentre].Scored-1 {
		t.Fatalf("accounting: %+v then %+v", before.Accounting.Components[ComponentCentre], after.Accounting.Components[ComponentCentre])
	}

	// Under the frozen split the pinned revision is the one reviewed against.
	opts := physRefOpts(f)
	opts.SplitManifestPath = frozenPath
	pr, err := LoadPhysicalReference(opts, DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.drifted) != 0 {
		t.Fatalf("drift under the pinned revision: %v", pr.drifted)
	}
}

// A body reviewed against one membership, whose cited frame then changed,
// lends no dimension to any keyframe, and says so; centres still score.
func TestPhysicalScoringRefusesADriftedBody(t *testing.T) {
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	m := f.Manifest()
	m.SidecarRevision = 0
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	doc, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	follower, _ := doc.Object(evalfixture.Follower)
	cited := follower.Body.Length.Support.Frames[0]
	reviewPinned(t, pack, annotation.PhysicalRecordBody, evalfixture.Follower, follower.Body.BodyID, func(o *annotation.PhysicalObject) {
		o.Body.Review.Status = annotation.StatusProposed
	})
	before := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	wantOutcome(t, instantOf(t, before, evalfixture.Follower, 0), ComponentLength, OutcomeScored, "")

	doubtAPoint(t, pack, evalfixture.Follower, cited)
	after := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	in := instantOf(t, after, evalfixture.Follower, 0)
	wantOutcome(t, in, ComponentCentre, OutcomeScored, "")
	for _, c := range []PhysicalComponent{ComponentLength, ComponentWidth, ComponentHeight} {
		wantOutcome(t, in, c, OutcomeUnknownGeometry, annotation.UnavailableBodyMembershipDrift)
	}
	wantOutcome(t, in, ComponentBox, OutcomeUnknownGeometry, annotation.UnavailableIncompleteBox)
	if in.Reference == nil || in.Reference.BodyTruth || in.Reference.Length != nil {
		t.Fatalf("a drifted body still lent its dimensions: %+v", in.Reference)
	}
	if !strings.Contains(strings.Join(after.Caveats, "\n"), "body/"+follower.Body.BodyID) {
		t.Fatalf("caveats do not name the drifted body: %v", after.Caveats)
	}
}
