package annotation

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"time"
)

// Revision-safe storage for physical references, beside the sidecar and on
// the same protocol: a load returns a private token over the exact bytes
// read; a save of that document takes the pack's annotation lock, refuses
// unless the current file is still those bytes at that revision, archives
// the exact prior bytes, and replaces the head atomically. The sidecar's
// errors mean the same here: ErrSidecarBusy is another writer, and
// ErrSidecarConflict is a changed base to reload and reconcile.
//
// The lock is the sidecar's own. A save here and a membership save by a
// writer that takes the same lock do not interleave, so the links are checked
// against the sidecar as it stands at this commit. The lock says nothing about
// later membership saves, which do not read these references: rejecting an
// object or removing a mask a reference cites can invalidate it afterwards.
// Nothing here refuses such a save, because the macOS client saves the
// sidecar without this code, so a refusal here would be a guarantee that
// half the writers ignore. Instead every load re-checks the links against
// the current sidecar and lists the records that no longer hold (Stale), and
// the next save refuses them until they are repaired or removed. No file is
// rewritten to mark them, so the sidecar, the references and their history
// keep their bytes.

const (
	physicalReferenceFile = "physical-references.json"
	physicalRevisionDir   = "physical-reference-revisions"
)

// SavePhysicalReferences commits an edit of the document LoadPhysicalReferences
// returned, as a new revision. The document is validated against the pack
// and against the sidecar current at commit time. Failed saves leave the
// caller untouched; after an uncertain durability error, reload before
// retrying. Change.Author, Session and Operation are the caller's.
func SavePhysicalReferences(p *Pack, r *PhysicalReferenceSet) error {
	return savePhysicalPinned(p, r, 0, nil)
}

func savePhysical(p *Pack, r *PhysicalReferenceSet, restoredFrom int) error {
	return savePhysicalPinned(p, r, restoredFrom, nil)
}

// savePhysicalPinned is savePhysical with an optional membership pin: when
// membership is non-nil, the commit is refused unless the sidecar read under
// the lock has exactly that digest.
func savePhysicalPinned(p *Pack, r *PhysicalReferenceSet, restoredFrom int, membership *string) error {
	// Clone before canonicalising: a failed save must not change a dirty session.
	next := *r
	next.Objects = clonePhysicalObjects(r.Objects)
	next.Following = cloneFollowing(r.Following)
	next.RecordOrigins = make(map[string]ReferenceOrigin, len(r.RecordOrigins))
	for key, origin := range r.RecordOrigins {
		next.RecordOrigins[key] = origin
	}
	if err := mergeOrigins(next.RecordOrigins, next.currentOrigins()); err != nil {
		return fmt.Errorf("%w: %w", ErrPhysicalInvalid, err)
	}
	next.canonicalise()
	if err := next.Validate(p); err != nil {
		return fmt.Errorf("%w: %w", ErrPhysicalInvalid, err)
	}

	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := lockAnnotations(root)
	if err != nil {
		return err
	}
	defer lock.Close()

	// The sidecar is read under the lock that its own writers take, so the
	// objects checked here are the objects at commit.
	sidecar, err := readSidecar(p, root)
	if err != nil {
		return err
	}
	if membership != nil && *membership != sidecar.baseDigest {
		return ErrMembershipChanged
	}
	previous, err := readAnnotationFile(root, physicalReferenceFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read current physical references: %w", err)
	}
	parent := 0
	if err == nil {
		current, err := decodePhysical(p, previous)
		if err != nil {
			return err // Corrupt current data must not be overwritten by a fresh session.
		}
		if r.Revision != current.Revision || r.baseDigest != current.baseDigest {
			return ErrSidecarConflict
		}
		parent = current.Revision
		// The parent's ledger is authoritative: an origin recorded there
		// holds for every later revision, whatever the caller's copy says.
		if err := mergeOrigins(next.RecordOrigins, current.RecordOrigins); err != nil {
			return fmt.Errorf("%w: %w", ErrPhysicalInvalid, err)
		}
	} else {
		if r.baseDigest != "" || r.Revision != 1 {
			return ErrSidecarConflict // A deleted current file is not a new reference set.
		}
		if _, err := root.Stat(physicalRevisionDir); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("current physical references missing but history exists or is unreadable")
		}
	}
	if parent >= math.MaxInt-1 {
		return fmt.Errorf("physical reference revision space exhausted")
	}
	next.Revision = parent + 1
	next.UpdatedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	next.Change.ParentRev, next.Change.Revision = parent, next.Revision
	next.Change.CreatedUTC = next.UpdatedUTC
	next.RestoredFrom = restoredFrom
	if restoredFrom > 0 {
		next.Change.Operation = "restore"
	} else if next.Change.Operation == "" || next.Change.Operation == "restore" {
		next.Change.Operation = "save"
	}
	// Merging the parent's ledger refuses any origin it disagrees with, so
	// the document validated above is still valid; its links are checked
	// now, against the sidecar at commit.
	if err := next.ValidateLinks(p, sidecar); err != nil {
		return fmt.Errorf("%w: %w", ErrPhysicalInvalid, err)
	}

	b, err := json.MarshalIndent(&next, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if len(b) > MaxSidecarBytes {
		return fmt.Errorf("physical references exceed %d bytes", MaxSidecarBytes)
	}
	if parent > 0 {
		if err := archiveRevision(root, physicalRevisionDir, physicalRevisionName(parent), previous, parent); err != nil {
			return err
		}
	}
	if err := writeAnnotationFile(root, physicalReferenceFile, b); err != nil {
		return fmt.Errorf("commit physical references (reload before retry): %w", err)
	}
	next.baseDigest = sha256Hex(b)
	next.stale, next.staleAgainst = nil, sidecar.Revision
	*r = next
	return nil
}

// mergeOrigins adds src to dst, refusing a record whose origin differs.
func mergeOrigins(dst, src map[string]ReferenceOrigin) error {
	for key, origin := range src {
		if have, ok := dst[key]; ok && have != origin {
			return fmt.Errorf("record %s was created %s and cannot become %s: author a new record under a new id", key, have, origin)
		}
		dst[key] = origin
	}
	return nil
}

// LoadPhysicalReferences opens the bounded, validated current document and
// checks its links against the current sidecar; records a later membership
// edit invalidated are listed by Stale, not refused, so they can be seen and
// repaired. An untouched pack starts empty. Missing current data with
// existing history is an error, not permission to restart revision numbering.
func LoadPhysicalReferences(p *Pack) (*PhysicalReferenceSet, error) {
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	b, err := readAnnotationFile(root, physicalReferenceFile)
	if errors.Is(err, os.ErrNotExist) {
		if _, historyErr := root.Stat(physicalRevisionDir); !errors.Is(historyErr, os.ErrNotExist) {
			return nil, fmt.Errorf("current physical references missing but history exists or is unreadable")
		}
		return NewPhysicalReferenceSet(p), nil
	}
	if err != nil {
		return nil, err
	}
	r, err := decodePhysical(p, b)
	if err != nil {
		return nil, err
	}
	if err := r.markStale(p, root); err != nil {
		return nil, err
	}
	return r, nil
}

// markStale checks the document's links against the current sidecar and
// records what no longer holds.
func (r *PhysicalReferenceSet) markStale(p *Pack, root *os.Root) error {
	sidecar, err := readSidecar(p, root)
	if err != nil {
		return fmt.Errorf("check physical reference links: %w", err)
	}
	r.stale, r.staleAgainst = r.LinkProblems(p, sidecar), sidecar.Revision
	return nil
}

// LoadPhysicalReferenceRevision reads a retained revision. Its token is
// deliberately stale: saving an old revision directly cannot roll back the
// head; restore it instead.
func LoadPhysicalReferenceRevision(p *Pack, revision int) (*PhysicalReferenceSet, error) {
	if revision < 1 {
		return nil, fmt.Errorf("invalid requested revision %d", revision)
	}
	current, err := LoadPhysicalReferences(p)
	if err == nil && current.baseDigest != "" && current.Revision == revision {
		return current, nil
	}
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	b, err := readAnnotationFile(root, physicalRevisionName(revision))
	if err != nil {
		return nil, err
	}
	r, err := decodePhysical(p, b)
	if err != nil {
		return nil, err
	}
	if r.Revision != revision {
		return nil, fmt.Errorf("archive revision does not match requested revision %d", revision)
	}
	if err := r.markStale(p, root); err != nil {
		return nil, err
	}
	return r, nil
}

// RestorePhysicalReferenceRevision implements undo and redo as a new
// revision. Restored records keep their review status and origin; the
// origin ledger carries forward, so a restore cannot make a tracker-assisted
// record independent either.
func RestorePhysicalReferenceRevision(p *Pack, current *PhysicalReferenceSet, revision int) error {
	old, err := LoadPhysicalReferenceRevision(p, revision)
	if err != nil {
		return err
	}
	old.Revision, old.baseDigest = current.Revision, current.baseDigest
	old.Change = Provenance{Author: current.Change.Author, Session: current.Change.Session, Operation: "restore"}
	if err := savePhysical(p, old, revision); err != nil {
		return err
	}
	*current = *old
	return nil
}

func physicalRevisionName(revision int) string {
	return fmt.Sprintf("%s/%010d.json", physicalRevisionDir, revision)
}

func decodePhysical(p *Pack, b []byte) (*PhysicalReferenceSet, error) {
	var r PhysicalReferenceSet
	if err := decodePhysicalJSON(b, &r, "physical references"); err != nil {
		return nil, err
	}
	if err := r.Validate(p); err != nil {
		return nil, err
	}
	r.baseDigest = sha256Hex(b)
	return &r, nil
}

// readSidecar reads the current sidecar through an open root. An untouched
// pack has an empty one. A caller that needs it to stay current holds the
// annotation lock.
func readSidecar(p *Pack, root *os.Root) (*Sidecar, error) {
	b, err := readAnnotationFile(root, sidecarFile)
	if errors.Is(err, os.ErrNotExist) {
		if _, historyErr := root.Stat(revisionDir); !errors.Is(historyErr, os.ErrNotExist) {
			return nil, fmt.Errorf("current annotation missing but history exists or is unreadable")
		}
		return NewSidecar(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read current annotation: %w", err)
	}
	return decodeSidecar(p, b)
}
