package annotation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// ErrSidecarConflict means the caller must reload and resolve its edits, not retry blindly.
var ErrSidecarConflict = errors.New("annotation revision changed; reload before saving")

// ErrSidecarBusy means another process owns the short-lived save transaction.
var ErrSidecarBusy = errors.New("annotation writer busy")

const (
	annotationLock = ".annotations.lock"
	revisionDir    = "annotation-revisions"
	// MaxSidecarBytes bounds JSON decoding independently of the point pack.
	MaxSidecarBytes = 64 << 20
)

// SaveSidecar commits an edit of the revision returned by LoadSidecar. The
// caller does not increment Revision. A fresh document saves as revision 1;
// subsequent saves increment it and archive the exact preceding bytes first.
// Failed saves leave the caller untouched. After an uncertain durability error,
// reload before retrying. Change.Author/Session/Operation are supplied by the
// caller; the store supplies revision numbers and UTC time, never a human identity.
func SaveSidecar(p *Pack, s *Sidecar) error {
	return saveSidecar(p, s, 0)
}

func saveSidecar(p *Pack, s *Sidecar, restoredFrom int) error {
	// Clone before canonicalising: a failed save must not change a dirty session.
	next := *s
	next.Objects = append([]Object(nil), s.Objects...)
	next.Masks = append([]FrameMask(nil), s.Masks...)
	next.Correspondences = append([]TrackCorrespondence(nil), s.Correspondences...)
	// Sorting is permitted; silently deduplicating an imported mask is not.
	for i := range next.Masks {
		next.Masks[i].PointIndices = append([]int(nil), s.Masks[i].PointIndices...)
		next.Masks[i].UncertainIndices = append([]int(nil), s.Masks[i].UncertainIndices...)
		sort.Ints(next.Masks[i].PointIndices)
		sort.Ints(next.Masks[i].UncertainIndices)
	}
	if err := next.Validate(p); err != nil {
		return fmt.Errorf("invalid annotation edit: %w", err)
	}
	next.Canonicalise()
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := root.OpenFile(annotationLock, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		lock, err = root.OpenFile(annotationLock, os.O_RDWR, 0o600)
	}
	if err != nil {
		return fmt.Errorf("open annotation lock: %w", err)
	}
	defer lock.Close() // Closing releases the kernel lock, including after a process exit.
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("%w: %v", ErrSidecarBusy, err)
	}
	// Never unlink the lock file: a second inode would permit a second writer.
	previous, err := readAnnotationFile(root, sidecarFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read current annotation: %w", err)
	}
	parent := 0
	if err == nil {
		current, err := decodeSidecar(p, previous)
		if err != nil {
			return err // Corrupt current data must not be overwritten by a fresh session.
		}
		if s.Revision != current.Revision || s.baseDigest != current.baseDigest {
			return ErrSidecarConflict
		}
		parent = current.Revision
	} else {
		if s.baseDigest != "" || s.Revision != 1 {
			return ErrSidecarConflict // A deleted current file is not a new annotation set.
		}
		if _, err := root.Stat(revisionDir); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("current annotation missing but history exists or is unreadable")
		}
	}
	if parent >= math.MaxInt-1 {
		return fmt.Errorf("annotation revision space exhausted")
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
	b, err := json.MarshalIndent(&next, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if len(b) > MaxSidecarBytes {
		return fmt.Errorf("annotation exceeds %d bytes", MaxSidecarBytes)
	}
	if parent > 0 {
		if err := root.MkdirAll(revisionDir, 0o700); err != nil {
			return err
		}
		info, err := root.Lstat(revisionDir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("annotation history must be a real directory")
		}
		name := revisionName(parent)
		archived, err := readAnnotationFile(root, name)
		switch {
		case err == nil && !bytes.Equal(archived, previous):
			return fmt.Errorf("revision %d archive differs; refusing overwrite", parent)
		case errors.Is(err, os.ErrNotExist):
			if err := writeAnnotationFile(root, name, previous); err != nil {
				return fmt.Errorf("archive revision %d: %w", parent, err)
			}
		case err != nil:
			return err
		}
	}
	if err := writeAnnotationFile(root, sidecarFile, b); err != nil {
		return fmt.Errorf("commit annotation (reload before retry): %w", err)
	}
	next.baseDigest = sha256Hex(b)
	*s = next
	return nil
}

// LoadSidecar opens a bounded, validated annotation snapshot. Legacy v1 files
// remain readable and acquire history on their next save. Missing current data
// with existing history is an error, not permission to restart revision numbering.
func LoadSidecar(p *Pack) (*Sidecar, error) {
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	b, err := readAnnotationFile(root, sidecarFile)
	if errors.Is(err, os.ErrNotExist) {
		if _, historyErr := root.Stat(revisionDir); !errors.Is(historyErr, os.ErrNotExist) {
			return nil, fmt.Errorf("current annotation missing but history exists or is unreadable")
		}
		return NewSidecar(p), nil
	}
	if err != nil {
		return nil, err
	}
	return decodeSidecar(p, b)
}

// LoadSidecarRevision reads a specific retained snapshot. Its original token is
// deliberately stale: saving an old revision directly cannot roll back the head.
func LoadSidecarRevision(p *Pack, revision int) (*Sidecar, error) {
	if revision < 1 {
		return nil, fmt.Errorf("invalid requested revision %d", revision)
	}
	current, err := LoadSidecar(p)
	if err == nil && current.baseDigest != "" && current.Revision == revision {
		return current, nil
	}
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	b, err := readAnnotationFile(root, revisionName(revision))
	if err != nil {
		return nil, err
	}
	s, err := decodeSidecar(p, b)
	if err != nil {
		return nil, err
	}
	if s.Revision != revision {
		return nil, fmt.Errorf("archive revision does not match requested revision %d", revision)
	}
	return s, nil
}

// RestoreSidecarRevision implements persisted undo/redo as a new revision. The
// current session's author/session identify the action; restored masks keep their
// original review status and provenance. A stale current session is still refused.
func RestoreSidecarRevision(p *Pack, current *Sidecar, revision int) error {
	old, err := LoadSidecarRevision(p, revision)
	if err != nil {
		return err
	}
	old.Revision, old.baseDigest = current.Revision, current.baseDigest
	old.Change = Provenance{Author: current.Change.Author, Session: current.Change.Session,
		Operation: "restore"}
	if err := saveSidecar(p, old, revision); err != nil {
		return err
	}
	*current = *old
	return nil
}

func revisionName(revision int) string {
	return fmt.Sprintf("%s/%010d.json", revisionDir, revision)
}

func decodeSidecar(p *Pack, b []byte) (*Sidecar, error) {
	var s Sidecar
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse annotation: %w", err)
	}
	if err := s.Validate(p); err != nil {
		return nil, err
	}
	s.baseDigest = sha256Hex(b)
	return &s, nil
}

func readAnnotationFile(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("annotation %s is not a regular file", name)
	}
	f, err := root.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readAnnotationBytes(f)
}

func readAnnotationBytes(f *os.File) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("annotation descriptor is not a regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxSidecarBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxSidecarBytes {
		return nil, fmt.Errorf("annotation exceeds %d bytes", MaxSidecarBytes)
	}
	return b, nil
}

func writeAnnotationFile(root *os.Root, name string, b []byte) error {
	// Keep temporary data beside its destination so rename remains atomic.
	tmp := name + "." + uuid.NewString() + ".tmp"
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	if err := flushAnnotationFile(f, b); err != nil {
		return err
	}
	if err := root.Rename(tmp, name); err != nil {
		return err
	}
	dir := "."
	if name != sidecarFile {
		dir = revisionDir
	}
	d, err := root.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return err
	}
	// Persist creation of the history directory before replacing the head.
	if dir != "." {
		return rootSync(root)
	}
	return nil
}

func rootSync(root *os.Root) error {
	d, err := root.Open(".")
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// flushAnnotationFile always closes the descriptor, including write/sync failure.
func flushAnnotationFile(f interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}, b []byte) error {
	n, writeErr := f.Write(b)
	if n != len(b) && writeErr == nil {
		writeErr = io.ErrShortWrite
	}
	return errors.Join(writeErr, f.Sync(), f.Close())
}
