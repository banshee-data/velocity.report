package labelset

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// HistoryWriter builds a label set from an existing sequence of revisions,
// as a migration does: each revision is one journal line carrying its
// original author, session, operation and time, and the head is written once
// at the end. Revision numbers follow the source's, so a pin on revision n of
// the source names revision n here.
type HistoryWriter struct {
	dir      string
	m        Manifest
	root     *os.Root
	lock     *os.File
	journal  *os.File
	buf      *bufio.Writer
	prev     *State
	prevHash string
	rev      int
}

// NewHistory starts a label set in an empty directory.
func NewHistory(dir string, m Manifest) (*HistoryWriter, error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("refusing to write a label set into non-empty %s", dir)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := m.Layout.Validate(); err != nil {
		return nil, err
	}
	m.SchemaVersion, m.PacketKey = SchemaVersion, PacketKeyScheme
	if m.LabelSetID == "" {
		m.LabelSetID = uuid.NewString()
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	lock, err := lockSet(root)
	if err != nil {
		root.Close()
		return nil, err
	}
	f, err := root.OpenFile(JournalFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		lock.Close()
		root.Close()
		return nil, err
	}
	empty := State{}
	empty.Canonicalise()
	hash, _ := DigestOf(&empty)
	return &HistoryWriter{dir: dir, m: m, root: root, lock: lock, journal: f, buf: bufio.NewWriterSize(f, 1<<20), prev: &empty, prevHash: hash}, nil
}

// Base starts the history at revision n without the revisions before it,
// as a published label set's journal does: s is kept in journal-base/.
func (h *HistoryWriter) Base(revision int, s State, change Change) (string, error) {
	if h.rev != 0 {
		return "", fmt.Errorf("a base must come first")
	}
	next := s.Clone()
	next.Canonicalise()
	if err := next.Validate(h.m.Layout); err != nil {
		return "", fmt.Errorf("revision %d: %w", revision, err)
	}
	files, err := EncodeState(&next)
	if err != nil {
		return "", err
	}
	digest := StateDigest(files)
	if err := h.root.MkdirAll(JournalBaseDir, 0o755); err != nil {
		return "", err
	}
	for _, name := range stateFiles {
		if err := writeFileAtomic(h.root, filepath.Join(JournalBaseDir, name), files[name]); err != nil {
			return "", err
		}
	}
	line, err := canonicalLine(Delta{Revision: revision, ParentRevision: revision, ParentState: digest, State: digest, Base: true, Change: change})
	if err != nil {
		return "", err
	}
	if _, err := h.buf.Write(line); err != nil {
		return "", err
	}
	h.prev, h.prevHash, h.rev = &next, digest, revision
	return digest, nil
}

// Add appends the next revision. An unchanged state is still a revision, so
// numbering stays aligned with the source.
func (h *HistoryWriter) Add(s State, change Change, restoredFrom int) (string, error) {
	next := s.Clone()
	next.Canonicalise()
	if err := next.Validate(h.m.Layout); err != nil {
		return "", fmt.Errorf("revision %d: %w", h.rev+1, err)
	}
	d, err := Diff(h.prev, &next)
	if err != nil {
		return "", fmt.Errorf("revision %d: %w", h.rev+1, err)
	}
	digest, err := DigestOf(&next)
	if err != nil {
		return "", err
	}
	if change.CreatedUTC == "" {
		change.CreatedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if change.Operation == "" {
		change.Operation = "save"
	}
	d.Revision, d.ParentRevision, d.ParentState, d.State = h.rev+1, h.rev, h.prevHash, digest
	d.Change, d.RestoredFrom = change, restoredFrom
	line, err := canonicalLine(d)
	if err != nil {
		return "", err
	}
	if len(line) > MaxFileBytes {
		return "", fmt.Errorf("revision %d journal line is %d bytes", d.Revision, len(line))
	}
	if _, err := h.buf.Write(line); err != nil {
		return "", err
	}
	h.prev, h.prevHash, h.rev = &next, digest, d.Revision
	return digest, nil
}

// Finish syncs the journal, writes the head and the manifest, and releases
// the lock.
func (h *HistoryWriter) Finish() (*LabelSet, error) {
	defer h.root.Close()
	defer h.lock.Close()
	if h.rev == 0 {
		h.journal.Close()
		return nil, fmt.Errorf("no revisions written")
	}
	if err := errors.Join(h.buf.Flush(), h.journal.Sync(), h.journal.Close()); err != nil {
		return nil, err
	}
	files, err := EncodeState(h.prev)
	if err != nil {
		return nil, err
	}
	for _, name := range stateFiles {
		if err := writeFileAtomic(h.root, name, files[name]); err != nil {
			return nil, err
		}
	}
	h.m.Head = Head{Revision: h.rev, State: h.prevHash, UpdatedUTC: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := writeManifest(h.root, &h.m); err != nil {
		return nil, err
	}
	return Open(h.dir)
}

// Abort releases the writer without finishing.
func (h *HistoryWriter) Abort() {
	h.journal.Close()
	h.lock.Close()
	h.root.Close()
}
