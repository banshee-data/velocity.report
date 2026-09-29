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
	if err := SaveSidecar(p, s); err != nil {
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
	if err := SaveSidecar(p, s); err != nil {
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
		"sidecar":       func() { writeFile(t, filepath.Join(p.Dir, sidecarFile), []byte("{")) },
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
