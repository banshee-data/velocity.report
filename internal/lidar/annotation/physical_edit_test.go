package annotation

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// editFrom is an edit of the current document made against the current
// membership.
func editFrom(t *testing.T, p *Pack, objects []PhysicalObject) PhysicalEdit {
	t.Helper()
	cur, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	s, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	return PhysicalEdit{BaseRevision: cur.Revision, BaseDigest: cur.Digest(), MembershipDigest: s.Digest(),
		Objects: objects, Change: Provenance{Author: "op", Session: "s1", Operation: "physical_edit"}}
}

func reviewOf(t *testing.T, p *Pack, kind PhysicalRecordKind, object, record string) PhysicalReviewRequest {
	t.Helper()
	e := editFrom(t, p, nil)
	return PhysicalReviewRequest{BaseRevision: e.BaseRevision, BaseDigest: e.BaseDigest, MembershipDigest: e.MembershipDigest,
		Kind: kind, ObjectID: object, RecordID: record, Reviewer: Provenance{Author: "reviewer", Session: "s2"}}
}

// car1Only is car-1's body and first two keyframes from validPhysical.
func car1Only(p *Pack) []PhysicalObject {
	o := validPhysical(p).Objects[0]
	o.Keyframes = o.Keyframes[:2]
	return []PhysicalObject{o}
}

func statusOf(t *testing.T, r *PhysicalReferenceSet, object, keyframe string) ReviewStatus {
	t.Helper()
	o, ok := r.Object(object)
	if !ok {
		t.Fatalf("object %q missing", object)
	}
	if keyframe == "" {
		return o.Body.Review.Status
	}
	for _, k := range o.Keyframes {
		if k.KeyframeID == keyframe {
			return k.Review.Status
		}
	}
	t.Fatalf("keyframe %q missing", keyframe)
	return ""
}

// A save cannot review anything: records that arrive claiming review are
// saved as proposals and reported, and review is a separate, named action on
// the saved record.
func TestPhysicalEditSavesProposalsAndReviewIsSeparate(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	sidecarBefore := readSaved(t, p)

	// A brand-new record validates as the save will see it: entering the
	// origin ledger, not missing from it.
	checked, err := ValidatePhysicalEdit(p, editFrom(t, p, car1Only(p)))
	if err != nil || !checked.Valid() {
		t.Fatalf("validate a new record: %v %+v", err, checked)
	}
	out, err := SavePhysicalEdit(p, editFrom(t, p, car1Only(p)))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	want := []string{"body/body-car-1", "keyframe/kf-car-1-s0", "keyframe/kf-car-1-s3"}
	if !reflect.DeepEqual(out.ResetReviews, want) {
		t.Fatalf("reset reviews %v, want %v", out.ResetReviews, want)
	}
	doc := out.Document
	if doc.Revision != 1 || statusOf(t, doc, "car-1", "") != StatusProposed || statusOf(t, doc, "car-1", "kf-car-1-s0") != StatusProposed {
		t.Fatalf("first save: revision %d, statuses not proposed", doc.Revision)
	}
	if !bytes.Equal(sidecarBefore, readSaved(t, p)) {
		t.Fatal("a physical edit changed the membership sidecar")
	}

	reviewedOut, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordBody, "car-1", "body-car-1"))
	if err != nil {
		t.Fatalf("review body: %v", err)
	}
	reviewed := reviewedOut.Document
	if statusOf(t, reviewed, "car-1", "") != StatusReviewed || statusOf(t, reviewed, "car-1", "kf-car-1-s0") != StatusProposed {
		t.Fatal("reviewing the body reviewed something else, or nothing")
	}
	o, _ := reviewed.Object("car-1")
	if o.Body.Review.Provenance.Author != "op" || reviewed.Change.Author != "reviewer" || reviewed.Change.Operation != "review_body" {
		t.Fatalf("review lost the author or the reviewer: record %+v change %+v", o.Body.Review.Provenance, reviewed.Change)
	}
	if reviewed.Revision != 2 {
		t.Fatalf("review revision %d, want 2", reviewed.Revision)
	}

	// Resaving the unchanged saved objects keeps the review.
	again, err := SavePhysicalEdit(p, editFrom(t, p, reviewed.Objects))
	if err != nil {
		t.Fatal(err)
	}
	if len(again.ResetReviews) != 0 || statusOf(t, again.Document, "car-1", "") != StatusReviewed {
		t.Fatalf("an unchanged resave reset %v", again.ResetReviews)
	}

	if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordKeyframe, "car-1", "kf-nope")); !errors.Is(err, ErrPhysicalRecordNotFound) {
		t.Fatalf("review of a missing record: %v", err)
	}
	if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, "mask", "car-1", "body-car-1")); err == nil {
		t.Fatal("a review of an unknown kind was accepted")
	}
	r := reviewOf(t, p, PhysicalRecordBody, "car-1", "body-car-1")
	r.Reviewer.Author = ""
	if _, err := ReviewPhysicalRecord(p, r); err == nil {
		t.Fatal("an anonymous review was accepted")
	}
}

// Editing one keyframe returns only that keyframe to proposed; revising the
// body's geometry is a new body and returns every keyframe of that object,
// and nothing else, to proposed.
func TestPhysicalEditResetsDependentReviews(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	objects := append(car1Only(p), validPhysical(p).Objects[1])
	if _, err := SavePhysicalEdit(p, editFrom(t, p, objects)); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		kind           PhysicalRecordKind
		object, record string
	}{
		{PhysicalRecordBody, "car-1", "body-car-1"}, {PhysicalRecordKeyframe, "car-1", "kf-car-1-s0"},
		{PhysicalRecordKeyframe, "car-1", "kf-car-1-s3"}, {PhysicalRecordBody, "car-2", "body-car-2"},
		{PhysicalRecordKeyframe, "car-2", "kf-car-2-s0"},
	} {
		if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, r.kind, r.object, r.record)); err != nil {
			t.Fatalf("review %s %s: %v", r.kind, r.record, err)
		}
	}
	cur, _ := LoadPhysicalReferences(p)

	// A keyframe edit.
	edited := clonePhysicalObjects(cur.Objects)
	edited[0].Keyframes[0].Position.BoundM = fp(0.25)
	out, err := SavePhysicalEdit(p, editFrom(t, p, edited))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.ResetReviews, []string{"keyframe/kf-car-1-s0"}) {
		t.Fatalf("a keyframe edit reset %v", out.ResetReviews)
	}
	if statusOf(t, out.Document, "car-1", "kf-car-1-s3") != StatusReviewed || statusOf(t, out.Document, "car-1", "") != StatusReviewed {
		t.Fatal("a keyframe edit reset its neighbours")
	}
	if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordKeyframe, "car-1", "kf-car-1-s0")); err != nil {
		t.Fatal(err)
	}

	// A body revision.
	cur, _ = LoadPhysicalReferences(p)
	edited = clonePhysicalObjects(cur.Objects)
	edited[0].Body.Length.UpperM = fp(4.8)
	out, err = SavePhysicalEdit(p, editFrom(t, p, edited))
	if err != nil {
		t.Fatal(err)
	}
	newID := out.RenamedBodies["body-car-1"]
	if !strings.HasPrefix(newID, "body_") {
		t.Fatalf("a revised body kept or lost its identity: %v", out.RenamedBodies)
	}
	want := []string{"body/" + newID, "keyframe/kf-car-1-s0", "keyframe/kf-car-1-s3"}
	if !reflect.DeepEqual(out.ResetReviews, want) {
		t.Fatalf("a body revision reset %v, want %v", out.ResetReviews, want)
	}
	if statusOf(t, out.Document, "car-2", "") != StatusReviewed || statusOf(t, out.Document, "car-2", "kf-car-2-s0") != StatusReviewed {
		t.Fatal("a body revision reset another object's reviews")
	}
	saved, _ := LoadPhysicalReferences(p)
	if saved.RecordOrigins["body/body-car-1"] != OriginIndependent || saved.RecordOrigins["body/"+newID] != OriginIndependent {
		t.Fatalf("the ledger lost the old body or missed the new one: %v", saved.RecordOrigins)
	}
	if o, _ := saved.Object("car-1"); o.Body.BodyID != newID || *o.Body.Length.UpperM != 4.8 {
		t.Fatalf("saved body %+v", o.Body)
	}
	old, err := LoadPhysicalReferenceRevision(p, saved.Revision-1)
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := old.Object("car-1"); o.Body.BodyID != "body-car-1" || o.Body.Review.Status != StatusReviewed {
		t.Fatal("the previous reviewed body is not in history")
	}

	// Removing the body is a change the keyframes lean on too.
	edited = clonePhysicalObjects(saved.Objects)
	if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordKeyframe, "car-1", "kf-car-1-s0")); err != nil {
		t.Fatal(err)
	}
	saved, _ = LoadPhysicalReferences(p)
	edited = clonePhysicalObjects(saved.Objects)
	edited[0].Body = nil
	edited[0].Keyframes = edited[0].Keyframes[:1]
	edited[0].Keyframes[0].Anchor = PhysicalAnchor{Kind: AnchorBodyCentre}
	edited[0].Keyframes[0].Front = EndpointEvidence{Status: EvidenceUnknown}
	edited[0].Keyframes[0].Rear = EndpointEvidence{Status: EvidenceUnknown}
	out, err = SavePhysicalEdit(p, editFrom(t, p, edited))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.ResetReviews, []string{"keyframe/kf-car-1-s0"}) {
		t.Fatalf("removing the body reset %v", out.ResetReviews)
	}
}

// An edit made from a document or membership that has since changed is
// refused, and validation writes nothing.
func TestPhysicalEditRefusesStaleBasesAndValidatesWithoutWriting(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)

	stale := editFrom(t, p, car1Only(p))
	if _, err := SavePhysicalEdit(p, editFrom(t, p, car1Only(p))); err != nil {
		t.Fatal(err)
	}
	if _, err := SavePhysicalEdit(p, stale); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("a stale physical base: %v", err)
	}
	if _, err := ValidatePhysicalEdit(p, stale); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("validation of a stale base: %v", err)
	}

	e := editFrom(t, p, car1Only(p))
	e.MembershipDigest = "sha256:elsewhere"
	if _, err := SavePhysicalEdit(p, e); !errors.Is(err, ErrMembershipChanged) {
		t.Fatalf("a stale membership digest: %v", err)
	}
	r := reviewOf(t, p, PhysicalRecordBody, "car-1", "body-car-1")
	r.MembershipDigest = ""
	if _, err := ReviewPhysicalRecord(p, r); !errors.Is(err, ErrMembershipChanged) {
		t.Fatalf("a review against stale membership: %v", err)
	}

	// The pin holds under the lock, not only in the pre-check: membership
	// saved between the check and the commit is refused there.
	cur, _ := LoadPhysicalReferences(p)
	pinned := s.Digest()
	s.Change = Provenance{Author: "other", Operation: "label"}
	if err := saveMembership(p, s); err != nil {
		t.Fatal(err)
	}
	if err := savePhysicalPinned(p, cur, 0, &pinned); !errors.Is(err, ErrMembershipChanged) {
		t.Fatalf("membership changed under the lock: %v", err)
	}

	before := readPhysical(t, p)
	bad := editFrom(t, p, car1Only(p))
	bad.Objects[0].Keyframes[0].Position.Support = frames(1) // observed, but not at its own sample
	out, err := ValidatePhysicalEdit(p, bad)
	if err != nil {
		t.Fatal(err)
	}
	if out.Valid() || !strings.Contains(out.Invalid, "own sample") {
		t.Fatalf("validation missed an uncited observation: %+v", out)
	}
	unlinked := editFrom(t, p, car1Only(p))
	unlinked.Objects[0].ObjectID = "ghost"
	out, err = ValidatePhysicalEdit(p, unlinked)
	if err != nil {
		t.Fatal(err)
	}
	if out.Valid() || out.Invalid != "" || len(out.LinkProblems) == 0 {
		t.Fatalf("validation missed a rejected object: %+v", out)
	}
	if _, err := SavePhysicalEdit(p, unlinked); err == nil {
		t.Fatal("a reference to a rejected object was saved")
	}
	if !bytes.Equal(before, readPhysical(t, p)) {
		t.Fatal("validation or a refused save wrote the document")
	}
}

// An edit cannot launder a tracker-assisted record into an independent one,
// and reviewing it leaves it assisted.
func TestPhysicalEditKeepsTrackerAssistance(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	o := validPhysical(p).Objects[0]
	o.Body = nil
	o.Keyframes = o.Keyframes[2:] // the tracker-assisted proposal at sample 5
	if _, err := SavePhysicalEdit(p, editFrom(t, p, []PhysicalObject{o})); err != nil {
		t.Fatal(err)
	}
	reviewedOut, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordKeyframe, "car-1", "kf-car-1-s5-proposal"))
	if err != nil {
		t.Fatal(err)
	}
	reviewed := reviewedOut.Document
	// Validation refuses the laundering the save would refuse.
	launderedCheck := clonePhysicalObjects(reviewed.Objects)
	launderedCheck[0].Keyframes[0].Review = independentReview()
	if out, err := ValidatePhysicalEdit(p, editFrom(t, p, launderedCheck)); err != nil || out.Valid() || !strings.Contains(out.Invalid, "cannot become") {
		t.Fatalf("validation of a laundered origin: %v %+v", err, out)
	}
	kf, _ := reviewed.Keyframe("car-1", 5)
	if kf.Review.Status != StatusReviewed || kf.Review.ScoredAsTruth() {
		t.Fatal("review made a tracker-assisted keyframe truth")
	}
	laundered := clonePhysicalObjects(reviewed.Objects)
	laundered[0].Keyframes[0].Review = independentReview()
	if _, err := SavePhysicalEdit(p, editFrom(t, p, laundered)); err == nil {
		t.Fatal("an edit turned a tracker-assisted keyframe independent")
	}
	// Deleting it and adding it back under its ID is the same claim.
	if _, err := SavePhysicalEdit(p, editFrom(t, p, []PhysicalObject{{ObjectID: "car-1", Keyframes: []PhysicalKeyframe{}}})); err != nil {
		t.Fatal(err)
	}
	if _, err := SavePhysicalEdit(p, editFrom(t, p, laundered)); err == nil {
		t.Fatal("deleting and re-adding made a tracker-assisted keyframe independent")
	}
}

// A rejected record stays rejected through an edit; a record the membership
// no longer supports cannot be reviewed; damaged stored state refuses every
// operation rather than editing around it.
func TestPhysicalEditRejectionStaleLinksAndDamage(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	objects := car1Only(p)
	objects[0].Keyframes[1].Review.Status = StatusRejected
	out, err := SavePhysicalEdit(p, editFrom(t, p, objects))
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(t, out.Document, "car-1", "kf-car-1-s3") != StatusRejected {
		t.Fatal("a save un-rejected a record")
	}
	edited := clonePhysicalObjects(out.Document.Objects)
	edited[0].Keyframes[1].Position.BoundM = fp(0.3)
	out, err = SavePhysicalEdit(p, editFrom(t, p, edited))
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(t, out.Document, "car-1", "kf-car-1-s3") != StatusRejected || strings.Contains(strings.Join(out.ResetReviews, ","), "kf-car-1-s3") {
		t.Fatalf("editing a rejected record changed its status or reported a reset: %v", out.ResetReviews)
	}

	// Rejecting the object in membership leaves the references stale; a
	// review is a save, so it is refused until they are repaired.
	s.Objects[0].Status = StatusRejected
	if err := saveMembership(p, s); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordBody, "car-1", "body-car-1")); !errors.Is(err, ErrPhysicalInvalid) {
		t.Fatalf("review of a stale record: %v", err)
	}

	// Damage: an unreadable physical head, then an unreadable sidecar.
	head := readPhysical(t, p)
	edit := editFrom(t, p, car1Only(p))
	review := reviewOf(t, p, PhysicalRecordBody, "car-1", "body-car-1")
	for name, damage := range map[string]func(){
		"physical head": func() { writeFile(t, filepath.Join(p.Dir, physicalReferenceFile), []byte("{")) },
		"sidecar":       func() { breakMembership(p) },
	} {
		damage()
		if _, err := ValidatePhysicalEdit(p, edit); err == nil {
			t.Fatalf("%s: validation over damaged state", name)
		}
		if _, err := SavePhysicalEdit(p, edit); err == nil {
			t.Fatalf("%s: save over damaged state", name)
		}
		if _, err := ReviewPhysicalRecord(p, review); err == nil {
			t.Fatalf("%s: review over damaged state", name)
		}
		writeFile(t, filepath.Join(p.Dir, physicalReferenceFile), head)
	}
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A review records the membership it was looking at; an edit clears it; a
// later membership change in a frame the record rests on is reported as
// drift until the record is reviewed again.
func TestPhysicalReviewPinsMembershipAndReportsDrift(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	if _, err := SavePhysicalEdit(p, editFrom(t, p, car1Only(p))); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		kind PhysicalRecordKind
		id   string
	}{{PhysicalRecordBody, "body-car-1"}, {PhysicalRecordKeyframe, "kf-car-1-s0"}, {PhysicalRecordKeyframe, "kf-car-1-s3"}} {
		if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, r.kind, "car-1", r.id)); err != nil {
			t.Fatal(err)
		}
	}
	cur, _ := LoadPhysicalReferences(p)
	kf, _ := cur.Keyframe("car-1", 0)
	if kf.Review.ReviewedAgainst == nil || kf.Review.ReviewedAgainst.Revision != s.Revision || kf.Review.ReviewedAgainst.Digest != s.Digest() ||
		kf.Review.ReviewedAgainst.Masks != s.SupportMasksDigest("car-1", keyframeFrames(kf)) {
		t.Fatalf("review did not pin membership and masks: %+v", kf.Review.ReviewedAgainst)
	}
	if cur.MinReader != MaskPinReaderLevel {
		t.Fatalf("a document with mask pins says min_reader %d", cur.MinReader)
	}
	if d := cur.ReviewDrift(p, s); len(d) != 0 {
		t.Fatalf("drift with unchanged membership: %v", d)
	}

	// A change in a frame nothing cites: no drift.
	s.Masks[len(s.Masks)-2].PointIndices = []int{0, 1, 2, 3, 4, 5, 6} // car-1 at sample 5
	s.Change = Provenance{Author: "op", Operation: "label"}
	if err := saveMembership(p, s); err != nil {
		t.Fatal(err)
	}
	if d := cur.ReviewDrift(p, s); len(d) != 0 {
		t.Fatalf("drift from an uncited frame: %v", d)
	}
	// A change at sample 3, which keyframe s3 rests on and nothing else does.
	for i := range s.Masks {
		if s.Masks[i].ObjectID == "car-1" && s.Masks[i].SampleID == 3 {
			s.Masks[i].UncertainIndices = []int{7}
			s.Masks[i].PointIndices = []int{0, 1, 2, 3, 4, 5, 6}
		}
	}
	if err := saveMembership(p, s); err != nil {
		t.Fatal(err)
	}
	drift := cur.ReviewDrift(p, s)
	if len(drift) != 1 || !strings.Contains(drift[0].Record, "kf-car-1-s3") || !strings.Contains(drift[0].Problem, "sample 3") {
		t.Fatalf("drift %v", drift)
	}
	if keys := cur.DriftedRecords(p, s); len(keys) != 1 || !keys["keyframe/kf-car-1-s3"] {
		t.Fatalf("drifted record keys %v", keys)
	}
	// Every review here was made through the service, so every reviewed
	// record carries a pin; a record reviewed without one is counted.
	if n := cur.UnpinnedReviews(); n != 0 {
		t.Fatalf("%d unpinned reviews among service reviews", n)
	}
	unpinned := validPhysical(p)
	if n := unpinned.UnpinnedReviews(); n != 5 {
		t.Fatalf("%d unpinned reviews in the fixture, want 5 (two bodies and three reviewed keyframes; the following record is not counted)", n)
	}
	// A mask removed from a cited frame is a change too.
	removed := *s
	removed.Masks = nil
	for _, m := range s.Masks {
		if !(m.ObjectID == "car-1" && m.SampleID == 1) {
			removed.Masks = append(removed.Masks, m)
		}
	}
	if d := cur.ReviewDrift(p, &removed); len(d) != 2 {
		t.Fatalf("drift after a cited mask was removed: %v", d)
	}
	// Reviewing again against the current membership clears it.
	out, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordKeyframe, "car-1", "kf-car-1-s3"))
	if err != nil {
		t.Fatal(err)
	}
	if d := out.Document.ReviewDrift(p, s); len(d) != 0 {
		t.Fatalf("drift after re-review: %v", d)
	}
	// Membership history lost: a pin of the masks still tells a change in a
	// cited frame from none, without the revision it was made against.
	forgetMembershipHistory(p)
	if d := out.Document.ReviewDrift(p, s); len(d) != 0 {
		t.Fatalf("drift without history and without a change: %v", d)
	}
	for i := range s.Masks {
		if s.Masks[i].ObjectID == "car-1" && s.Masks[i].SampleID == 0 {
			s.Masks[i].PointIndices = s.Masks[i].PointIndices[1:]
		}
	}
	if d := out.Document.ReviewDrift(p, s); len(d) == 0 || !strings.Contains(d[0].Problem, "reviewed samples changed") {
		t.Fatalf("lost history drift %v", d)
	}

	// An edit clears the pin with the review.
	edited := clonePhysicalObjects(out.Document.Objects)
	edited[0].Keyframes[0].Position.BoundM = fp(0.4)
	e := editFrom(t, p, edited)
	saved, err := SavePhysicalEdit(p, e)
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := saved.Document.Keyframe("car-1", 0); k.Review.ReviewedAgainst != nil || k.Review.Status != StatusProposed {
		t.Fatalf("an edited record kept its review pin: %+v", k.Review)
	}
}

func TestPhysicalReviewPinIsOnlyOnReviewedRecords(t *testing.T) {
	p := physPack(t)
	r := validPhysical(p)
	r.Objects[0].Keyframes[2].Review.ReviewedAgainst = &MembershipPin{Revision: 1, Digest: "sha256:x"}
	if err := r.Validate(p); err == nil || !strings.Contains(err.Error(), "cannot carry") {
		t.Fatalf("a proposal carried a review pin: %v", err)
	}
	r = validPhysical(p)
	r.Objects[0].Keyframes[0].Review.ReviewedAgainst = &MembershipPin{Revision: 0}
	if err := r.Validate(p); err == nil || !strings.Contains(err.Error(), "revision 0") {
		t.Fatalf("a pin to revision 0: %v", err)
	}
}

// History lists every revision newest first; restore makes an old one
// current as a new revision, and refuses a stale base or a missing author.
func TestPhysicalHistoryAndRestore(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	if h, err := PhysicalReferenceHistory(p); err != nil || len(h) != 0 {
		t.Fatalf("history of an untouched pack: %v %v", h, err)
	}
	first, err := SavePhysicalEdit(p, editFrom(t, p, car1Only(p)))
	if err != nil {
		t.Fatal(err)
	}
	firstContent, _ := first.Document.ContentDigest()
	if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordBody, "car-1", "body-car-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordKeyframe, "car-1", "kf-car-1-s0")); err != nil {
		t.Fatal(err)
	}
	if _, err := SavePhysicalEdit(p, editFrom(t, p, []PhysicalObject{})); err != nil {
		t.Fatal(err)
	}
	h, err := PhysicalReferenceHistory(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 4 || h[0].Revision != 4 || !h[0].Head || h[3].Head || h[1].Reviewed != 2 || h[3].Keyframes != 2 || h[1].Change.Operation != "review_keyframe" {
		t.Fatalf("history %+v", h)
	}

	req := func() PhysicalRestoreRequest {
		e := editFrom(t, p, nil)
		return PhysicalRestoreRequest{BaseRevision: e.BaseRevision, BaseDigest: e.BaseDigest,
			MembershipDigest: e.MembershipDigest, Revision: 1, Author: "op", Session: "s"}
	}
	stale := req()
	restored, err := RestorePhysicalRevision(p, req())
	if err != nil {
		t.Fatal(err)
	}
	content, _ := restored.Document.ContentDigest()
	if restored.Document.Revision != 5 || restored.Document.RestoredFrom != 1 || content != firstContent || restored.Document.Change.Operation != "restore" {
		t.Fatalf("restore: revision %d from %d, same content %v", restored.Document.Revision, restored.Document.RestoredFrom, content == firstContent)
	}
	if _, err := RestorePhysicalRevision(p, stale); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("restore from a stale base: %v", err)
	}
	anonymous := req()
	anonymous.Author = ""
	if _, err := RestorePhysicalRevision(p, anonymous); !errors.Is(err, ErrPhysicalInvalid) {
		t.Fatalf("anonymous restore: %v", err)
	}
	missing := req()
	missing.Revision = 99
	if _, err := RestorePhysicalRevision(p, missing); err == nil {
		t.Fatal("restored a revision that never existed")
	}
	membership := req()
	membership.MembershipDigest = "sha256:elsewhere"
	if _, err := RestorePhysicalRevision(p, membership); !errors.Is(err, ErrMembershipChanged) {
		t.Fatalf("restore against stale membership: %v", err)
	}
	// A restored revision is held to today's membership: one that cites a
	// rejected object is refused.
	s, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Objects[0].Status = StatusRejected
	if err := saveMembership(p, s); err != nil {
		t.Fatal(err)
	}
	if _, err := RestorePhysicalRevision(p, req()); !errors.Is(err, ErrPhysicalInvalid) {
		t.Fatalf("restore of references to a rejected object: %v", err)
	}

	// A damaged retained revision fails the history rather than hiding.
	writeFile(t, filepath.Join(p.Dir, physicalRevisionName(2)), []byte("{"))
	if _, err := PhysicalReferenceHistory(p); err == nil {
		t.Fatal("history skipped a damaged revision")
	}
	writeFile(t, filepath.Join(p.Dir, physicalReferenceFile), []byte("{"))
	if _, err := PhysicalReferenceHistory(p); err == nil {
		t.Fatal("history read past a damaged head")
	}
	if _, err := RestorePhysicalRevision(p, stale); err == nil {
		t.Fatal("restore over a damaged head")
	}
}

// stripMaskPins rewrites every pin as an older build wrote it: revision and
// digest, no masks.
func stripMaskPins(t *testing.T, p *Pack) {
	t.Helper()
	doc, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := range doc.Objects {
		o := &doc.Objects[i]
		if o.Body != nil && o.Body.Review.ReviewedAgainst != nil {
			o.Body.Review.ReviewedAgainst.Masks = ""
		}
		for k := range o.Keyframes {
			if pin := o.Keyframes[k].Review.ReviewedAgainst; pin != nil {
				pin.Masks = ""
			}
		}
	}
	doc.Change = Provenance{Author: "older build", Operation: "save"}
	if err := SavePhysicalReferences(p, doc); err != nil {
		t.Fatal(err)
	}
}

// A review pinned to its masks holds through an edit elsewhere with nothing
// to move, even for a copy without membership history. A re-pin brings an
// older pin, which names only a revision, to the current membership and its
// masks, when its frames are unchanged; a review whose frames changed, or
// whose pinned membership is gone, keeps its pin and is counted as drifted.
func TestRepinUnchangedReviews(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	if _, err := SavePhysicalEdit(p, editFrom(t, p, car1Only(p))); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		kind PhysicalRecordKind
		id   string
	}{{PhysicalRecordBody, "body-car-1"}, {PhysicalRecordKeyframe, "kf-car-1-s0"}, {PhysicalRecordKeyframe, "kf-car-1-s3"}} {
		if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, r.kind, "car-1", r.id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := RepinUnchangedReviews(p, " ", ""); err == nil {
		t.Fatal("a re-pin without an author was accepted")
	}
	before, _ := LoadPhysicalReferences(p)
	if n, d, err := RepinUnchangedReviews(p, "op", ""); err != nil || n != 0 || d != 0 {
		t.Fatalf("re-pin with nothing to move: %d %d %v", n, d, err)
	}
	if after, _ := LoadPhysicalReferences(p); after.Revision != before.Revision {
		t.Fatalf("a re-pin that moved nothing wrote revision %d", after.Revision)
	}

	// An edit in a frame nothing cites: every review still holds, by its
	// masks, and nothing needs moving.
	s.Masks[len(s.Masks)-2].PointIndices = []int{0, 1, 2, 3, 4, 5, 6} // car-1 at sample 5
	s.Change = Provenance{Author: "op", Operation: "label"}
	if err := saveMembership(p, s); err != nil {
		t.Fatal(err)
	}
	if n, d, err := RepinUnchangedReviews(p, "op", "S1"); err != nil || n != 0 || d != 0 {
		t.Fatalf("re-pin of mask pins after an uncited edit: %d re-pinned, %d drifted, %v", n, d, err)
	}
	// The same reviews as an older build pinned them: by revision only.
	stripMaskPins(t, p)
	n, d, err := RepinUnchangedReviews(p, "op", "S1")
	if err != nil || n != 3 || d != 0 {
		t.Fatalf("re-pin after an uncited edit: %d re-pinned, %d drifted, %v", n, d, err)
	}
	head, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := LoadPhysicalReferences(p)
	if cur.Change.Operation != "repin_reviews" || cur.Change.Author != "op" {
		t.Fatalf("re-pin change %+v", cur.Change)
	}
	for _, k := range cur.Objects[0].Keyframes {
		want := MembershipPin{Revision: head.Revision, Digest: head.Digest(), Masks: head.SupportMasksDigest("car-1", keyframeFrames(k))}
		if k.Review.Status == StatusReviewed && *k.Review.ReviewedAgainst != want {
			t.Fatalf("keyframe %s pin %+v, want %+v", k.KeyframeID, k.Review.ReviewedAgainst, want)
		}
	}
	// What a published copy sees: no earlier membership at all.
	forgetMembershipHistory(p)
	if drift := cur.ReviewDrift(p, head); len(drift) != 0 {
		t.Fatalf("re-pinned reviews drift without history: %v", drift)
	}
}

// A review whose frame an edit changed keeps its old pin; a review pinned by
// revision only, whose membership cannot be rebuilt, is left alone too.
func TestRepinLeavesChangedAndUnprovableReviews(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	if _, err := SavePhysicalEdit(p, editFrom(t, p, car1Only(p))); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"kf-car-1-s0", "kf-car-1-s3"} {
		if _, err := ReviewPhysicalRecord(p, reviewOf(t, p, PhysicalRecordKeyframe, "car-1", id)); err != nil {
			t.Fatal(err)
		}
	}
	pinned, _ := LoadPhysicalReferences(p)
	old := *pinned.Objects[0].Keyframes[1].Review.ReviewedAgainst
	for i := range s.Masks {
		if s.Masks[i].ObjectID == "car-1" && s.Masks[i].SampleID == 3 {
			s.Masks[i].UncertainIndices = []int{7}
			s.Masks[i].PointIndices = []int{0, 1, 2, 3, 4, 5, 6}
		}
	}
	s.Change = Provenance{Author: "op", Operation: "label"}
	if err := saveMembership(p, s); err != nil {
		t.Fatal(err)
	}
	if n, d, err := RepinUnchangedReviews(p, "op", ""); err != nil || n != 0 || d != 1 {
		t.Fatalf("re-pin after a cited edit: %d re-pinned, %d drifted, %v", n, d, err)
	}
	cur, _ := LoadPhysicalReferences(p)
	if kf, _ := cur.Keyframe("car-1", 3); *kf.Review.ReviewedAgainst != old {
		t.Fatalf("a drifted review was re-pinned: %+v", kf.Review.ReviewedAgainst)
	}

	// Another edit, then the membership history is lost: kf-s0's pin is one
	// revision old and can no longer be shown to hold.
	s.Masks[len(s.Masks)-2].PointIndices = []int{0, 1, 2, 3, 4, 5, 6}
	if err := saveMembership(p, s); err != nil {
		t.Fatal(err)
	}
	forgetMembershipHistory(p)
	// kf-s0 is pinned to its masks and holds without history; kf-s3 still
	// drifts.
	if n, d, err := RepinUnchangedReviews(p, "op", ""); err != nil || n != 0 || d != 1 {
		t.Fatalf("re-pin of mask pins without history: %d re-pinned, %d drifted, %v", n, d, err)
	}
	// Pinned by revision only, neither can be shown to hold.
	stripMaskPins(t, p)
	if n, d, err := RepinUnchangedReviews(p, "op", ""); err != nil || n != 0 || d != 2 {
		t.Fatalf("re-pin without history: %d re-pinned, %d drifted, %v", n, d, err)
	}
}
