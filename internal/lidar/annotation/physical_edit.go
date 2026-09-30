package annotation

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Editing physical references through one authoritative writer.
//
// A client edits a copy of the document it loaded and sends the objects back
// with the revision, the exact-byte digest it loaded, and the membership
// sidecar digest it was looking at. The service applies the edit to the
// document as it stands on disk, not to the client's copy, and holds the
// edit to rules a client cannot be trusted to apply:
//
//   - A record's review is set only by ReviewPhysicalRecord. A save leaves an
//     untouched reviewed record reviewed and returns every changed or new
//     record to proposed.
//   - A body whose geometry changed is a new body belief under a new ID; the
//     previous belief stays in the revision history. Every keyframe of that
//     object leans on the body, so each returns to proposed.
//   - The membership the operator saw must still be the membership on disk
//     when the edit commits, checked under the same lock as the write.

// ErrMembershipChanged means the membership sidecar is no longer the
// revision the edit was made against. The client reloads membership and
// revalidates before saving again.
var ErrMembershipChanged = errors.New("membership annotation changed since the physical edit was made; reload and revalidate")

// ErrPhysicalInvalid wraps every refusal of a physical reference edit on its
// content: a rule, a link to the membership, or an origin.
var ErrPhysicalInvalid = errors.New("invalid physical reference edit")

// ErrPhysicalRecordNotFound means a review named a record the current
// document does not hold.
var ErrPhysicalRecordNotFound = errors.New("physical reference record not found")

// PhysicalEdit is one proposed change to a pack's physical references.
type PhysicalEdit struct {
	// BaseRevision and BaseDigest identify the document the edit was made
	// from: the revision and exact-byte digest a load returned. A pack with no
	// references yet has revision 1 and an empty digest.
	BaseRevision int
	BaseDigest   string
	// MembershipDigest is the exact-byte digest of the membership sidecar the
	// edit was made against; empty for a pack with no sidecar yet.
	MembershipDigest string
	// Objects replaces the document's objects. Following references are
	// carried over from the current document unchanged.
	Objects []PhysicalObject
	// Change names the author, session and operation of this revision.
	Change Provenance
}

// PhysicalEditOutcome is what an edit did, or would do.
type PhysicalEditOutcome struct {
	// Document is the edited document: saved when committed, otherwise the
	// candidate that would be saved.
	Document *PhysicalReferenceSet
	// ResetReviews lists every record, by ledger key, whose review the edit
	// returned to proposed.
	ResetReviews []string
	// RenamedBodies maps a body ID whose geometry changed to the new ID the
	// revised belief was saved under.
	RenamedBodies map[string]string
	// LinkProblems are the records that do not hold against the membership
	// sidecar. A committed edit has none.
	LinkProblems []LinkProblem
	// Invalid is the document-level refusal, empty when the document is valid.
	Invalid string
	// Membership is the sidecar the edit was checked against.
	Membership *Sidecar
}

// Valid reports whether the edit could be committed as it stands.
func (o *PhysicalEditOutcome) Valid() bool { return o.Invalid == "" && len(o.LinkProblems) == 0 }

// ValidatePhysicalEdit applies an edit's rules without writing. A stale base
// or membership digest is still an error: diagnostics against a document the
// client has not seen would describe the wrong edit.
func ValidatePhysicalEdit(p *Pack, e PhysicalEdit) (*PhysicalEditOutcome, error) {
	current, sidecar, err := loadPhysicalForEdit(p)
	if err != nil {
		return nil, err
	}
	if err := checkEditBase(current, sidecar, e.BaseRevision, e.BaseDigest, e.MembershipDigest); err != nil {
		return nil, err
	}
	out, next := applyPhysicalEdit(current, e)
	out.Membership = sidecar
	candidate := next
	candidate.Objects = clonePhysicalObjects(next.Objects)
	candidate.RecordOrigins = make(map[string]ReferenceOrigin, len(next.RecordOrigins))
	for key, origin := range next.RecordOrigins {
		candidate.RecordOrigins[key] = origin
	}
	// The ledger merge a save performs: a record keeps the origin it was
	// created under, and a new record enters the ledger with its own.
	if err := mergeOrigins(candidate.RecordOrigins, candidate.currentOrigins()); err != nil {
		out.Invalid = err.Error()
	}
	candidate.canonicalise()
	if out.Invalid == "" {
		if err := candidate.Validate(p); err != nil {
			out.Invalid = err.Error()
		}
	}
	if out.Invalid == "" {
		if problems := candidate.LinkProblems(p, sidecar); problems != nil {
			out.LinkProblems = problems
		}
	}
	out.Document = &candidate
	return out, nil
}

// SavePhysicalEdit commits an edit as a new revision. A refused edit leaves
// the stored document as it was and returns the reason.
func SavePhysicalEdit(p *Pack, e PhysicalEdit) (*PhysicalEditOutcome, error) {
	current, sidecar, err := loadPhysicalForEdit(p)
	if err != nil {
		return nil, err
	}
	if err := checkEditBase(current, sidecar, e.BaseRevision, e.BaseDigest, e.MembershipDigest); err != nil {
		return nil, err
	}
	out, next := applyPhysicalEdit(current, e)
	out.Membership = sidecar
	if err := savePhysicalPinned(p, &next, 0, &e.MembershipDigest); err != nil {
		return nil, err
	}
	out.Document = &next
	return out, nil
}

// PhysicalRecordKind names the kind of record a review applies to.
type PhysicalRecordKind string

const (
	PhysicalRecordBody     PhysicalRecordKind = "body"
	PhysicalRecordKeyframe PhysicalRecordKind = "keyframe"
)

// PhysicalReview names one saved record to mark reviewed.
type PhysicalReviewRequest struct {
	BaseRevision     int
	BaseDigest       string
	MembershipDigest string
	Kind             PhysicalRecordKind
	ObjectID         string
	RecordID         string
	// Reviewer is the person confirming the record. The record keeps its
	// author; the reviewer is this revision's change author.
	Reviewer Provenance
}

// ReviewPhysicalRecord marks one saved body or keyframe reviewed, as a new
// revision. It reviews exactly the saved record: there is no payload to
// replace it with, so an unsaved draft cannot be reviewed into truth. It
// changes neither the record's origin nor any other record.
func ReviewPhysicalRecord(p *Pack, req PhysicalReviewRequest) (*PhysicalEditOutcome, error) {
	if req.Reviewer.Author == "" {
		return nil, fmt.Errorf("%w: a review names its reviewer", ErrPhysicalInvalid)
	}
	current, sidecar, err := loadPhysicalForEdit(p)
	if err != nil {
		return nil, err
	}
	if err := checkEditBase(current, sidecar, req.BaseRevision, req.BaseDigest, req.MembershipDigest); err != nil {
		return nil, err
	}
	next := *current
	next.Objects = clonePhysicalObjects(current.Objects)
	// The membership pin is checked again under the lock at commit, so the
	// revision recorded here is the one the reviewer saw.
	pin := &MembershipPin{Revision: sidecar.Revision, Digest: sidecar.baseDigest}
	found := false
	for i := range next.Objects {
		o := &next.Objects[i]
		if o.ObjectID != req.ObjectID {
			continue
		}
		switch req.Kind {
		case PhysicalRecordBody:
			if o.Body != nil && o.Body.BodyID == req.RecordID {
				o.Body.Review.Status = StatusReviewed
				o.Body.Review.ReviewedAgainst = pin
				found = true
			}
		case PhysicalRecordKeyframe:
			for k := range o.Keyframes {
				if o.Keyframes[k].KeyframeID == req.RecordID {
					o.Keyframes[k].Review.Status = StatusReviewed
					o.Keyframes[k].Review.ReviewedAgainst = pin
					found = true
				}
			}
		default:
			return nil, fmt.Errorf("%w: review kind %q (want %q or %q)", ErrPhysicalInvalid, req.Kind, PhysicalRecordBody, PhysicalRecordKeyframe)
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: %s %q of object %q", ErrPhysicalRecordNotFound, req.Kind, req.RecordID, req.ObjectID)
	}
	next.Change = Provenance{Author: req.Reviewer.Author, Session: req.Reviewer.Session,
		Operation: "review_" + string(req.Kind)}
	if err := savePhysicalPinned(p, &next, 0, &req.MembershipDigest); err != nil {
		return nil, err
	}
	return &PhysicalEditOutcome{Document: &next, ResetReviews: []string{}, RenamedBodies: map[string]string{}, Membership: sidecar}, nil
}

// loadPhysicalForEdit reads the current document and sidecar.
func loadPhysicalForEdit(p *Pack) (*PhysicalReferenceSet, *Sidecar, error) {
	sidecar, err := LoadSidecar(p)
	if err != nil {
		return nil, nil, err
	}
	current, err := LoadPhysicalReferences(p)
	if err != nil {
		return nil, nil, err
	}
	return current, sidecar, nil
}

// checkEditBase refuses an edit made from anything but the current document
// and membership. The store's own token is kept on current, so a racing save
// between this check and the commit is still refused there.
func checkEditBase(current *PhysicalReferenceSet, sidecar *Sidecar, revision int, digest, membership string) error {
	if revision != current.Revision || digest != current.Digest() {
		return ErrSidecarConflict
	}
	if membership != sidecar.baseDigest {
		return ErrMembershipChanged
	}
	return nil
}

// applyPhysicalEdit builds the next document from the current one and the
// edit's objects, applying the review and body-revision rules.
func applyPhysicalEdit(current *PhysicalReferenceSet, e PhysicalEdit) (*PhysicalEditOutcome, PhysicalReferenceSet) {
	out := &PhysicalEditOutcome{RenamedBodies: map[string]string{}, ResetReviews: []string{}, LinkProblems: []LinkProblem{}}
	next := *current
	next.Following = cloneFollowing(current.Following)
	next.RecordOrigins = make(map[string]ReferenceOrigin, len(current.RecordOrigins))
	for k, v := range current.RecordOrigins {
		next.RecordOrigins[k] = v
	}
	next.Change = e.Change

	before := map[string]PhysicalObject{}
	bodies := map[string]BodyGeometry{}
	keyframes := map[string]PhysicalKeyframe{}
	for _, o := range current.Objects {
		before[o.ObjectID] = o
		if o.Body != nil {
			bodies[o.Body.BodyID] = *o.Body
		}
		for _, k := range o.Keyframes {
			keyframes[k.KeyframeID] = k
		}
	}
	reset := map[string]bool{}
	edited := clonePhysicalObjects(e.Objects)
	for i := range edited {
		o := &edited[i]
		// bodyChanged is whether the body belief the keyframes lean on is not
		// the one they were reviewed against.
		bodyChanged := false
		if prev, ok := before[o.ObjectID]; ok && (prev.Body == nil) != (o.Body == nil) {
			bodyChanged = true
		}
		if o.Body != nil {
			b := o.Body
			old, existed := bodies[b.BodyID]
			if !existed || !reflect.DeepEqual(old, *b) {
				wasReviewed := (existed && old.Review.Status == StatusReviewed) || b.Review.Status == StatusReviewed
				if existed && !sameBodyGeometry(old, *b) {
					// A revised belief is a new record. The old one stays in
					// the ledger and in history under its own ID and origin.
					id := newPhysicalID("body")
					out.RenamedBodies[b.BodyID] = id
					b.BodyID = id
					wasReviewed = true
				}
				bodyChanged = bodyChanged || !existed || !sameBodyGeometry(old, *b)
				demote(&b.Review, "body/"+b.BodyID, reset, wasReviewed)
				stampRecord(&b.Review)
			}
		}
		for k := range o.Keyframes {
			kf := &o.Keyframes[k]
			old, existed := keyframes[kf.KeyframeID]
			changed := !existed || !reflect.DeepEqual(old, *kf)
			if !changed && !bodyChanged {
				continue
			}
			demote(&kf.Review, "keyframe/"+kf.KeyframeID, reset,
				(existed && old.Review.Status == StatusReviewed) || kf.Review.Status == StatusReviewed)
			if changed {
				stampRecord(&kf.Review)
			}
		}
	}
	next.Objects = edited
	for key := range reset {
		out.ResetReviews = append(out.ResetReviews, key)
	}
	sort.Strings(out.ResetReviews)
	return out, next
}

// demote returns a record's review to proposed and reports it when it had
// been, or claimed to be, reviewed.
func demote(r *PhysicalReview, key string, reset map[string]bool, report bool) {
	r.ReviewedAgainst = nil
	if r.Status == StatusRejected {
		return
	}
	if report || r.Status == StatusReviewed {
		reset[key] = true
	}
	r.Status = StatusProposed
}

// stampRecord dates a new or changed record's provenance.
func stampRecord(r *PhysicalReview) {
	r.Provenance.CreatedUTC = time.Now().UTC().Format(time.RFC3339Nano)
}

// sameBodyGeometry compares what a body claims about the object, ignoring
// its review.
func sameBodyGeometry(a, b BodyGeometry) bool {
	return a.AxisConvention == b.AxisConvention &&
		reflect.DeepEqual(a.Length, b.Length) && reflect.DeepEqual(a.Width, b.Width) && reflect.DeepEqual(a.Height, b.Height)
}

// newPhysicalID is a fresh record ID. crypto/rand.Read never returns an
// error: it crashes the program rather than hand back weak bytes.
func newPhysicalID(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}

// PhysicalRestoreRequest names a retained revision to make current again.
type PhysicalRestoreRequest struct {
	BaseRevision     int
	BaseDigest       string
	MembershipDigest string
	Revision         int
	Author, Session  string
}

// RestorePhysicalRevision makes a retained revision current, as a new
// revision on top of the head: the old revision is never overwritten, the
// origin ledger carries forward, and the links are checked against the
// membership the operator is looking at. Restored reviews keep their status
// and the membership they were reviewed against, so ReviewDrift reports any
// that membership has since moved away from.
func RestorePhysicalRevision(p *Pack, req PhysicalRestoreRequest) (*PhysicalEditOutcome, error) {
	if strings.TrimSpace(req.Author) == "" {
		return nil, fmt.Errorf("%w: a restore names its author", ErrPhysicalInvalid)
	}
	current, sidecar, err := loadPhysicalForEdit(p)
	if err != nil {
		return nil, err
	}
	if err := checkEditBase(current, sidecar, req.BaseRevision, req.BaseDigest, req.MembershipDigest); err != nil {
		return nil, err
	}
	old, err := LoadPhysicalReferenceRevision(p, req.Revision)
	if err != nil {
		return nil, err
	}
	old.Revision, old.baseDigest = current.Revision, current.baseDigest
	old.Change = Provenance{Author: req.Author, Session: req.Session, Operation: "restore"}
	if err := savePhysicalPinned(p, old, req.Revision, &req.MembershipDigest); err != nil {
		return nil, err
	}
	return &PhysicalEditOutcome{Document: old, ResetReviews: []string{}, RenamedBodies: map[string]string{},
		LinkProblems: []LinkProblem{}, Membership: sidecar}, nil
}

// PhysicalRevisionSummary is one retained revision, for a history list.
type PhysicalRevisionSummary struct {
	Revision      int        `json:"revision"`
	UpdatedUTC    string     `json:"updated_utc"`
	Change        Provenance `json:"change"`
	RestoredFrom  int        `json:"restored_from,omitempty"`
	Digest        string     `json:"digest"`
	ContentDigest string     `json:"content_digest"`
	Head          bool       `json:"head"`
	Objects       int        `json:"objects"`
	Keyframes     int        `json:"keyframes"`
	Reviewed      int        `json:"reviewed"`
}

// PhysicalReferenceHistory lists every retained revision and the head,
// newest first. A retained revision that no longer reads is an error, not a
// gap in the list.
func PhysicalReferenceHistory(p *Pack) ([]PhysicalRevisionSummary, error) {
	head, err := LoadPhysicalReferences(p)
	if err != nil {
		return nil, err
	}
	if head.Digest() == "" {
		return []PhysicalRevisionSummary{}, nil
	}
	out := []PhysicalRevisionSummary{}
	for rev := head.Revision; rev >= 1; rev-- {
		doc := head
		if rev != head.Revision {
			if doc, err = LoadPhysicalReferenceRevision(p, rev); err != nil {
				return nil, fmt.Errorf("revision %d: %w", rev, err)
			}
		}
		// Every revision read here passed validation, which refuses the
		// non-finite values that are all that could fail to encode.
		content, _ := doc.ContentDigest()
		s := PhysicalRevisionSummary{Revision: doc.Revision, UpdatedUTC: doc.UpdatedUTC, Change: doc.Change,
			RestoredFrom: doc.RestoredFrom, Digest: doc.Digest(), ContentDigest: content, Head: rev == head.Revision,
			Objects: len(doc.Objects)}
		for _, o := range doc.Objects {
			if o.Body != nil && o.Body.Review.Status == StatusReviewed {
				s.Reviewed++
			}
			for _, k := range o.Keyframes {
				s.Keyframes++
				if k.Review.Status == StatusReviewed {
					s.Reviewed++
				}
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// ReviewDrift lists the reviewed records whose membership has changed since
// their review, in the frames the record rests on: its own sample for a
// keyframe, and every frame any of its components cites. Links can all still
// hold while the returns a person judged are different; such a record needs
// reviewing again. A record reviewed against a membership revision that is
// no longer retained is listed too, since nothing can show it unchanged.
func (r *PhysicalReferenceSet) ReviewDrift(p *Pack, current *Sidecar) []LinkProblem {
	cache := map[int]*Sidecar{current.Revision: current}
	membership := func(rev int) *Sidecar {
		if s, ok := cache[rev]; ok {
			return s
		}
		s, err := LoadSidecarRevision(p, rev)
		if err != nil {
			s = nil
		}
		cache[rev] = s
		return s
	}
	var out []LinkProblem
	check := func(record, object string, review PhysicalReview, frames []int) {
		pin := review.ReviewedAgainst
		if review.Status != StatusReviewed || pin == nil || pin.Digest == current.baseDigest {
			return
		}
		then := membership(pin.Revision)
		if then == nil || then.baseDigest != pin.Digest {
			out = append(out, LinkProblem{Record: record, Problem: fmt.Sprintf(
				"reviewed against membership revision %d, which is no longer retained as reviewed: review again", pin.Revision)})
			return
		}
		for _, f := range frames {
			if !sameMask(then.mask(object, f), current.mask(object, f)) {
				out = append(out, LinkProblem{Record: record, Problem: fmt.Sprintf(
					"membership at sample %d changed after review (revision %d, now %d): review again", f, pin.Revision, current.Revision)})
				return
			}
		}
	}
	for _, o := range r.Objects {
		if o.Body != nil {
			b := o.Body
			check(fmt.Sprintf("object %q body %q", o.ObjectID, b.BodyID), o.ObjectID, b.Review,
				supportFrames(b.Length.Support, b.Width.Support, b.Height.Support))
		}
		for _, k := range o.Keyframes {
			frames := supportFrames(k.Position.Support, k.Yaw.Support, k.Front.Support, k.Rear.Support)
			frames = append(frames, k.SampleID)
			check(fmt.Sprintf("object %q keyframe %q", o.ObjectID, k.KeyframeID), o.ObjectID, k.Review, frames)
		}
	}
	return out
}

func supportFrames(s ...EvidenceSupport) []int {
	seen := map[int]bool{}
	var out []int
	for _, e := range s {
		for _, f := range e.Frames {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	sort.Ints(out)
	return out
}

// mask is an object's mask at a sample, or nil.
func (s *Sidecar) mask(object string, sample int) *FrameMask {
	for i := range s.Masks {
		if s.Masks[i].ObjectID == object && s.Masks[i].SampleID == sample {
			return &s.Masks[i]
		}
	}
	return nil
}

// sameMask compares what a review could have relied on: the member and
// uncertain returns and the mask's status.
func sameMask(a, b *FrameMask) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	sa, sb := append([]int(nil), a.PointIndices...), append([]int(nil), b.PointIndices...)
	ua, ub := append([]int(nil), a.UncertainIndices...), append([]int(nil), b.UncertainIndices...)
	sort.Ints(sa)
	sort.Ints(sb)
	sort.Ints(ua)
	sort.Ints(ub)
	return a.Status == b.Status && reflect.DeepEqual(sa, sb) && reflect.DeepEqual(ua, ub)
}
