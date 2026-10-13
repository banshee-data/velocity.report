package labelset

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// ErrConflict means the head moved since it was loaded: reload and reconcile
// the unsaved edit rather than retrying blindly.
var ErrConflict = errors.New("label set revision changed; reload before saving")

// ErrBusy means another writer holds the label set's lock.
var ErrBusy = errors.New("label set writer busy")

// ErrHistoryNotRetained means a revision older than the local journal's base
// was asked for. Published label sets carry no journal; their history is the
// dataset's Git history.
var ErrHistoryNotRetained = errors.New("revision predates the history this copy retains")

// Head is the revision the head files hold.
type Head struct {
	Revision   int    `json:"revision"`
	State      string `json:"state"`
	UpdatedUTC string `json:"updated_utc"`
}

// Binding records a capture file verified to hold the label set's packets.
type Binding struct {
	Name            string `json:"name"`
	SHA256          string `json:"sha256"`
	Bytes           int64  `json:"bytes"`
	UDPPort         int    `json:"udp_port"`
	PacketsFound    int    `json:"packets_found"`
	PacketsMissing  int    `json:"packets_missing"`
	WindowsComplete int    `json:"windows_complete"`
	WindowsPartial  int    `json:"windows_partial"`
	WindowsMissing  int    `json:"windows_missing"`
	VerifiedUTC     string `json:"verified_utc"`
	VerifiedBuild   string `json:"verified_build,omitempty"`
}

// Migration records where a migrated label set came from and the mapping
// that carried it across.
type Migration struct {
	PackDigest      string `json:"pack_digest"`
	DatasetID       string `json:"dataset_id"`
	PackDir         string `json:"pack_dir,omitempty"`
	SidecarRevision int    `json:"sidecar_revision"`
	PCAPSHA256      string `json:"pcap_sha256"`
	PCAPName        string `json:"pcap_name"`
	DecoderBuild    string `json:"decoder_build,omitempty"`
	MappingDigest   string `json:"mapping_digest"`
	Note            string `json:"note,omitempty"`
}

// Manifest is labels.json: identity, layout, bindings and the head. It sits
// outside the state digest, so binding a file never makes a revision.
type Manifest struct {
	SchemaVersion int        `json:"schema_version"`
	LabelSetID    string     `json:"label_set_id"`
	Name          string     `json:"name,omitempty"`
	Layout        Layout     `json:"layout"`
	PacketKey     string     `json:"packet_key"`
	AppliesTo     []Binding  `json:"applies_to,omitempty"`
	Head          Head       `json:"head"`
	MigratedFrom  *Migration `json:"migrated_from,omitempty"`
	Note          string     `json:"note,omitempty"`
}

// LabelSet is an opened label set. Edit State, then Save.
type LabelSet struct {
	Dir      string
	Manifest Manifest
	State    State

	baseRevision int
	baseState    string
}

// Revision is the loaded head revision.
func (ls *LabelSet) Revision() int { return ls.baseRevision }

// StateDigest is the loaded head's state digest.
func (ls *LabelSet) StateDigest() string { return ls.baseState }

// Create writes a new label set as revision 1. dir must not exist, or be
// empty.
func Create(dir string, m Manifest, initial State, change Change) (*LabelSet, error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("refusing to create a label set in non-empty %s", dir)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	m.SchemaVersion = SchemaVersion
	m.PacketKey = PacketKeyScheme
	if m.LabelSetID == "" {
		m.LabelSetID = uuid.NewString()
	}
	next := initial.Clone()
	next.Canonicalise()
	if err := next.Validate(m.Layout); err != nil {
		return nil, fmt.Errorf("invalid label set: %w", err)
	}
	empty := State{}
	empty.Canonicalise()
	emptyFiles, err := EncodeState(&empty)
	if err != nil {
		return nil, err
	}
	ls := &LabelSet{Dir: dir, Manifest: m, State: next, baseState: StateDigest(emptyFiles)}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	lock, err := lockSet(root)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := ls.commit(root, &empty, change, 0); err != nil {
		return nil, err
	}
	return ls, nil
}

// Open reads and verifies a label set. A save interrupted after its journal
// line but before its head was replaced is rolled forward from the journal.
func Open(dir string) (*LabelSet, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	ls, err := load(dir, root)
	if err == nil {
		return ls, nil
	}
	if !errors.Is(err, errNeedsRecovery) {
		return nil, err
	}
	lock, lerr := lockSet(root)
	if lerr != nil {
		return nil, lerr
	}
	defer lock.Close()
	if err := recoverHead(root); err != nil {
		return nil, fmt.Errorf("recover interrupted save: %w", err)
	}
	return load(dir, root)
}

var errNeedsRecovery = errors.New("head and journal disagree")

func load(dir string, root *os.Root) (*LabelSet, error) {
	m, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	files, err := readHeadFiles(root)
	if err != nil {
		return nil, err
	}
	digest := StateDigest(files)
	tip, _, err := journalTip(root)
	if err != nil {
		return nil, err
	}
	if digest != m.Head.State || (tip != nil && (tip.Revision != m.Head.Revision || tip.State != m.Head.State)) {
		if tip != nil && tip.Revision >= m.Head.Revision {
			return nil, errNeedsRecovery
		}
		return nil, fmt.Errorf("head files digest %s, manifest says %s at revision %d", digest, m.Head.State, m.Head.Revision)
	}
	s, err := DecodeState(files)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(m.Layout); err != nil {
		return nil, fmt.Errorf("invalid label set: %w", err)
	}
	return &LabelSet{Dir: dir, Manifest: *m, State: *s, baseRevision: m.Head.Revision, baseState: m.Head.State}, nil
}

// Save commits the edited State as the next revision. It returns ErrConflict
// if the head moved since this label set was loaded. A save that changes
// nothing writes nothing.
func (ls *LabelSet) Save(change Change) error {
	return ls.save(change, 0)
}

func (ls *LabelSet) save(change Change, restoredFrom int) error {
	next := ls.State.Clone()
	next.Canonicalise()
	if err := next.Validate(ls.Manifest.Layout); err != nil {
		return fmt.Errorf("invalid label set edit: %w", err)
	}
	root, err := os.OpenRoot(ls.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := lockSet(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	m, err := readManifest(root)
	if err != nil {
		return err
	}
	if m.Head.Revision != ls.baseRevision || m.Head.State != ls.baseState {
		return ErrConflict
	}
	files, err := readHeadFiles(root)
	if err != nil {
		return err
	}
	if StateDigest(files) != m.Head.State {
		return fmt.Errorf("head files were changed outside the store: %w", ErrConflict)
	}
	base, err := DecodeState(files)
	if err != nil {
		return err
	}
	base.Canonicalise()
	pending := &LabelSet{Dir: ls.Dir, Manifest: *m, State: next, baseRevision: ls.baseRevision, baseState: ls.baseState}
	if err := pending.commit(root, base, change, restoredFrom); err != nil {
		return err
	}
	*ls = *pending
	return nil
}

// commit appends the delta from base to ls.State and replaces the head. The
// caller holds the lock.
func (ls *LabelSet) commit(root *os.Root, base *State, change Change, restoredFrom int) error {
	delta, err := Diff(base, &ls.State)
	if err != nil {
		return err
	}
	if delta.Empty() && ls.baseRevision > 0 && restoredFrom == 0 {
		return nil
	}
	files, err := EncodeState(&ls.State)
	if err != nil {
		return err
	}
	for name, b := range files {
		if len(b) > MaxFileBytes {
			return fmt.Errorf("%s would be %d bytes, above the %d limit", name, len(b), MaxFileBytes)
		}
	}
	if ls.baseRevision >= math.MaxInt-1 {
		return fmt.Errorf("revision space exhausted")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	change.CreatedUTC = now
	if restoredFrom > 0 {
		change.Operation = "restore"
	} else if change.Operation == "" || change.Operation == "restore" {
		change.Operation = "save"
	}
	delta.Revision = ls.baseRevision + 1
	delta.ParentRevision = ls.baseRevision
	delta.ParentState = ls.baseState
	delta.State = StateDigest(files)
	delta.Change = change
	delta.RestoredFrom = restoredFrom

	if err := ensureJournal(root, ls.baseRevision, ls.baseState); err != nil {
		return err
	}
	line, err := canonicalLine(delta)
	if err != nil {
		return err
	}
	if len(line) > MaxFileBytes {
		return fmt.Errorf("journal line would be %d bytes, above the %d limit", len(line), MaxFileBytes)
	}
	if err := appendJournal(root, line); err != nil {
		return fmt.Errorf("append journal: %w", err)
	}
	for _, name := range stateFiles {
		if err := writeFileAtomic(root, name, files[name]); err != nil {
			return fmt.Errorf("replace %s (reopen to recover): %w", name, err)
		}
	}
	ls.Manifest.Head = Head{Revision: delta.Revision, State: delta.State, UpdatedUTC: now}
	if err := writeManifest(root, &ls.Manifest); err != nil {
		return fmt.Errorf("replace %s (reopen to recover): %w", ManifestFile, err)
	}
	ls.baseRevision, ls.baseState = delta.Revision, delta.State
	return nil
}

// ensureJournal gives a label set without a journal (one published, or one
// created by copying its head) a base to grow from: a copy of the head in
// journal-base/ and a base line naming it.
func ensureJournal(root *os.Root, revision int, state string) error {
	if _, err := root.Stat(JournalFile); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if revision == 0 {
		return nil // a new set's first line is its creation
	}
	files, err := readHeadFiles(root)
	if err != nil {
		return err
	}
	if StateDigest(files) != state {
		return fmt.Errorf("head does not match revision %d's state", revision)
	}
	if err := root.MkdirAll(JournalBaseDir, 0o755); err != nil {
		return err
	}
	for _, name := range stateFiles {
		if err := writeFileAtomic(root, filepath.Join(JournalBaseDir, name), files[name]); err != nil {
			return err
		}
	}
	line, err := canonicalLine(Delta{Revision: revision, ParentRevision: revision, ParentState: state, State: state, Base: true,
		Change: Change{Author: "store", Operation: "journal_base", CreatedUTC: time.Now().UTC().Format(time.RFC3339Nano)}})
	if err != nil {
		return err
	}
	return appendJournal(root, line)
}

// Restore makes an earlier revision current again as a new revision. Earlier
// lines are not rewritten, and restoring never promotes a proposal.
func (ls *LabelSet) Restore(revision int, change Change) error {
	old, err := LoadRevision(ls.Dir, revision)
	if err != nil {
		return err
	}
	ls.State = old.Clone()
	return ls.save(change, revision)
}

// LoadRevision rebuilds a revision from the journal and checks it against the
// digest recorded when it was saved.
func LoadRevision(dir string, revision int) (*State, error) {
	var out *State
	err := Replay(dir, func(rev int, s *State, d *Delta) (bool, error) {
		if rev == revision {
			if got, err := DigestOf(s); err != nil {
				return false, err
			} else if got != d.State {
				return false, fmt.Errorf("revision %d rebuilds to %s, journal says %s", rev, got, d.State)
			}
			c := s.Clone()
			out = &c
			return false, nil
		}
		return rev < revision, nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("revision %d: %w", revision, ErrHistoryNotRetained)
	}
	return out, nil
}

// Replay applies the journal in order, calling fn after each revision with
// the state it produces. fn returns false to stop.
func Replay(dir string, fn func(revision int, s *State, d *Delta) (bool, error)) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	f, err := root.Open(JournalFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrHistoryNotRetained
		}
		return err
	}
	defer f.Close()
	state := &State{}
	state.Canonicalise()
	prevRev, prevState := 0, ""
	if empty, err := DigestOf(state); err == nil {
		prevState = empty
	}
	r := bufio.NewReader(f)
	for n := 1; ; n++ {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return nil // an unterminated final line is an interrupted append, never committed
		}
		if err != nil {
			return err
		}
		var d Delta
		if err := strictUnmarshal(line, &d); err != nil {
			return fmt.Errorf("journal line %d: %w", n, err)
		}
		if d.Base {
			base, err := readStateDir(root, JournalBaseDir)
			if err != nil {
				return fmt.Errorf("journal base: %w", err)
			}
			if got, _ := DigestOf(base); got != d.State {
				return fmt.Errorf("journal base digest %s, line says %s", got, d.State)
			}
			state, prevRev, prevState = base, d.Revision, d.State
			more, err := fn(d.Revision, state, &d)
			if err != nil || !more {
				return err
			}
			continue
		}
		if d.ParentRevision != prevRev || d.ParentState != prevState || d.Revision != prevRev+1 {
			return fmt.Errorf("journal line %d (revision %d) does not follow revision %d", n, d.Revision, prevRev)
		}
		next, err := Apply(state, &d)
		if err != nil {
			return fmt.Errorf("journal revision %d: %w", d.Revision, err)
		}
		state, prevRev, prevState = next, d.Revision, d.State
		more, err := fn(d.Revision, state, &d)
		if err != nil || !more {
			return err
		}
	}
}

// Verify replays the whole journal, checking every revision's digest and that
// the last one is the head.
func Verify(dir string) (revisions int, err error) {
	ls, err := Open(dir)
	if err != nil {
		return 0, err
	}
	last := 0
	err = Replay(dir, func(rev int, s *State, d *Delta) (bool, error) {
		got, err := DigestOf(s)
		if err != nil {
			return false, err
		}
		if got != d.State {
			return false, fmt.Errorf("revision %d rebuilds to %s, journal says %s", rev, got, d.State)
		}
		last = rev
		revisions++
		return true, nil
	})
	if errors.Is(err, ErrHistoryNotRetained) {
		return 0, nil
	}
	if err != nil {
		return revisions, err
	}
	if last != 0 && last != ls.Revision() {
		return revisions, fmt.Errorf("journal ends at revision %d, head is %d", last, ls.Revision())
	}
	return revisions, nil
}

// Publish copies the current state and manifest to dest, without the journal,
// its base or the lock. A published label set carries its current state only;
// the dataset's history is its Git history.
func Publish(dir, dest string) error {
	ls, err := Open(dir)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(dest); err == nil && len(entries) > 0 {
		return fmt.Errorf("refusing to publish into non-empty %s", dest)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()
	return writePublished(root, ls)
}

// Republish brings a published copy up to its source's current state. The
// copy must be the same label set, hold no local history (a copy someone has
// edited is a working copy, and its edits would be lost), and sit at a
// revision the source can show it passed through with the same state. The
// head files are written first and labels.json last, as a save commits.
func Republish(dir, dest string) error {
	src, err := Open(dir)
	if err != nil {
		return err
	}
	pub, err := Open(dest)
	if err != nil {
		return fmt.Errorf("published copy: %w", err)
	}
	if pub.Manifest.LabelSetID != src.Manifest.LabelSetID {
		return fmt.Errorf("%s holds label set %s, not %s", dest, pub.Manifest.LabelSetID, src.Manifest.LabelSetID)
	}
	for _, local := range []string{JournalFile, JournalBaseDir} {
		if _, err := os.Stat(filepath.Join(dest, local)); err == nil {
			return fmt.Errorf("%s has %s: it has been edited since it was published; publish those edits from it, or set it aside", dest, local)
		}
	}
	switch {
	case pub.Revision() > src.Revision():
		return fmt.Errorf("%s is at revision %d, ahead of the source's %d", dest, pub.Revision(), src.Revision())
	case pub.Revision() == src.Revision():
		if pub.StateDigest() != src.StateDigest() {
			return fmt.Errorf("%s and the source are both at revision %d with different states", dest, pub.Revision())
		}
	default:
		then, err := LoadRevision(dir, pub.Revision())
		if err != nil {
			return fmt.Errorf("cannot show %s's revision %d is in the source's history: %w", dest, pub.Revision(), err)
		}
		if d, err := DigestOf(then); err != nil {
			return err
		} else if d != pub.StateDigest() {
			return fmt.Errorf("%s's revision %d is %s, the source's was %s", dest, pub.Revision(), pub.StateDigest(), d)
		}
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := lockSet(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	return writePublished(root, src)
}

// writePublished writes a label set's current state and labels.json, without
// its journal.
func writePublished(root *os.Root, ls *LabelSet) error {
	files, err := EncodeState(&ls.State)
	if err != nil {
		return err
	}
	if StateDigest(files) != ls.StateDigest() {
		return fmt.Errorf("state re-encodes to a different digest")
	}
	for _, name := range stateFiles {
		if err := writeFileAtomic(root, name, files[name]); err != nil {
			return err
		}
	}
	return writeManifest(root, &ls.Manifest)
}

// recoverHead rebuilds the head from the journal after an interrupted save.
func recoverHead(root *os.Root) error {
	m, err := readManifest(root)
	if err != nil {
		return err
	}
	tip, _, err := journalTip(root)
	if err != nil || tip == nil {
		return fmt.Errorf("no journal to recover from: %v", err)
	}
	dir := root.Name()
	var rebuilt *State
	err = Replay(dir, func(rev int, s *State, d *Delta) (bool, error) {
		if rev == tip.Revision {
			c := s.Clone()
			rebuilt = &c
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	if rebuilt == nil {
		if tip.Base && tip.Revision == m.Head.Revision {
			base, err := readStateDir(root, JournalBaseDir)
			if err != nil {
				return err
			}
			rebuilt = base
		} else {
			return fmt.Errorf("journal does not rebuild revision %d", tip.Revision)
		}
	}
	files, err := EncodeState(rebuilt)
	if err != nil {
		return err
	}
	if StateDigest(files) != tip.State {
		return fmt.Errorf("journal rebuilds revision %d to a different digest", tip.Revision)
	}
	for _, name := range stateFiles {
		if err := writeFileAtomic(root, name, files[name]); err != nil {
			return err
		}
	}
	m.Head = Head{Revision: tip.Revision, State: tip.State, UpdatedUTC: tip.Change.CreatedUTC}
	return writeManifest(root, m)
}

// journalTip returns the last complete journal line, and truncates an
// interrupted final line, which was never committed.
func journalTip(root *os.Root) (*Delta, int, error) {
	b, err := readFile(root, JournalFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	lines := bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n"))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 || len(lines[len(lines)-1]) == 0 {
		return nil, 0, nil
	}
	var d Delta
	if err := strictUnmarshal(lines[len(lines)-1], &d); err != nil {
		return nil, 0, fmt.Errorf("journal tip: %w", err)
	}
	return &d, len(lines), nil
}

func readManifest(root *os.Root) (*Manifest, error) {
	b, err := readFile(root, ManifestFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ManifestFile, err)
	}
	var m Manifest
	if err := strictUnmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ManifestFile, err)
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("label set schema version %d, this build reads %d", m.SchemaVersion, SchemaVersion)
	}
	if m.PacketKey != PacketKeyScheme {
		return nil, fmt.Errorf("packet key %q, this build reads %q", m.PacketKey, PacketKeyScheme)
	}
	if err := m.Layout.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func writeManifest(root *os.Root, m *Manifest) error {
	b, err := encodeManifest(m)
	if err != nil {
		return err
	}
	return writeFileAtomic(root, ManifestFile, b)
}

// encodeManifest renders labels.json as the macOS client does: the canonical
// encoding, keys sorted, indented two spaces, then a newline. labels.json is
// outside the state digest, but both writers save it on every revision, so a
// second encoding would rewrite the whole file whenever the writer changed.
func encodeManifest(m *Manifest) ([]byte, error) {
	b, err := Canonical(m)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func readHeadFiles(root *os.Root) (map[string][]byte, error) {
	return readStateFiles(root, ".")
}

func readStateFiles(root *os.Root, dir string) (map[string][]byte, error) {
	files := make(map[string][]byte, len(stateFiles))
	for _, name := range stateFiles {
		b, err := readFile(root, filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		files[name] = b
	}
	return files, nil
}

func readStateDir(root *os.Root, dir string) (*State, error) {
	files, err := readStateFiles(root, dir)
	if err != nil {
		return nil, err
	}
	s, err := DecodeState(files)
	if err != nil {
		return nil, err
	}
	s.Canonicalise()
	return s, nil
}

func readFile(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readLimited(f)
}

// writeFileAtomic writes beside the destination, syncs, renames over it and
// syncs the directory.
func writeFileAtomic(root *os.Root, name string, b []byte) error {
	tmp := name + "." + uuid.NewString() + ".tmp"
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	n, werr := f.Write(b)
	if werr == nil && n != len(b) {
		werr = io.ErrShortWrite
	}
	if err := errors.Join(werr, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err := root.Rename(tmp, name); err != nil {
		return err
	}
	return syncDir(root, filepath.Dir(name))
}

func appendJournal(root *os.Root, line []byte) error {
	// Drop an interrupted final line before appending, so it cannot be read
	// as the start of this one.
	if b, err := readFile(root, JournalFile); err == nil && len(b) > 0 && b[len(b)-1] != '\n' {
		keep := bytes.LastIndexByte(b, '\n') + 1
		f, err := root.OpenFile(JournalFile, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		if err := errors.Join(f.Truncate(int64(keep)), f.Sync(), f.Close()); err != nil {
			return err
		}
	}
	f, err := root.OpenFile(JournalFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	n, werr := f.Write(line)
	if werr == nil && n != len(line) {
		werr = io.ErrShortWrite
	}
	if err := errors.Join(werr, f.Sync(), f.Close()); err != nil {
		return err
	}
	return syncDir(root, ".")
}

func syncDir(root *os.Root, dir string) error {
	d, err := root.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func lockSet(root *os.Root) (*os.File, error) {
	lock, err := root.OpenFile(LockFile, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open label set lock: %w", err)
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("%w: %v", ErrBusy, err)
	}
	return lock, nil
}
