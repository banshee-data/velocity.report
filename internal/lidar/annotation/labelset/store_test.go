package labelset

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

func newSet(t *testing.T) (string, *LabelSet) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "kirk0-reference")
	ls, err := Create(dir, Manifest{Name: "kirk0-reference", Layout: Pandar40P}, testState(), Change{Author: "dd", Operation: "create"})
	if err != nil {
		t.Fatal(err)
	}
	return dir, ls
}

func addMember(t *testing.T, ls *LabelSet, offset uint64) {
	t.Helper()
	for i := range ls.State.Masks {
		m := &ls.State.Masks[i]
		if m.ObjectID == "obj_a" && m.WindowID == 0 {
			m.Members = Union(m.Members, Ranges{{offset, 1}})
			return
		}
	}
	t.Fatal("obj_a window 0 missing")
}

func TestCreateOpenSaveReplay(t *testing.T) {
	dir, ls := newSet(t)
	if ls.Revision() != 1 {
		t.Fatalf("created at revision %d", ls.Revision())
	}
	for i, off := range []uint64{40, 41, 45} {
		addMember(t, ls, off)
		if err := ls.Save(Change{Author: "dd", Session: "s1", Operation: "save_mask"}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	if ls.Revision() != 4 {
		t.Fatalf("revision %d, want 4", ls.Revision())
	}
	before := ls.Revision()
	if err := ls.Save(Change{Author: "dd"}); err != nil || ls.Revision() != before {
		t.Fatalf("an unchanged save made revision %d (%v)", ls.Revision(), err)
	}

	back, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if back.Revision() != 4 || back.StateDigest() != ls.StateDigest() {
		t.Fatalf("reopened at %d %s", back.Revision(), back.StateDigest())
	}
	if n, err := Verify(dir); err != nil || n != 4 {
		t.Fatalf("verify replayed %d revisions: %v", n, err)
	}
	r2, err := LoadRevision(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range r2.Masks {
		if m.ObjectID == "obj_a" && m.WindowID == 0 && (!m.Members.Contains(40) || m.Members.Contains(41)) {
			t.Fatalf("revision 2 members %v", m.Members)
		}
	}
	journal, _ := os.ReadFile(filepath.Join(dir, JournalFile))
	if got := strings.Count(string(journal), "\n"); got != 4 {
		t.Fatalf("journal has %d lines", got)
	}
	// A typical save is small: one return of one mask.
	lines := strings.Split(strings.TrimSpace(string(journal)), "\n")
	if len(lines[3]) > 600 {
		t.Fatalf("one-return save wrote a %d-byte journal line: %s", len(lines[3]), lines[3])
	}
}

func TestSaveConflict(t *testing.T) {
	dir, a := newSet(t)
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	addMember(t, a, 40)
	if err := a.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	addMember(t, b, 41)
	if err := b.Save(Change{Author: "dd"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	// Editing the head outside the store is caught too.
	if err := os.WriteFile(filepath.Join(dir, RecordsFile), []byte(`{"correspondences":[],"dismissed_proposals":[],"objects":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	addMember(t, a, 42)
	if err := a.Save(Change{Author: "dd"}); err == nil {
		t.Fatal("save over an externally edited head succeeded")
	}
}

func TestSaveRefusesInvalidEdit(t *testing.T) {
	_, ls := newSet(t)
	ls.State.Masks[0].Members = Ranges{{700, 1}} // outside the domain
	if err := ls.Save(Change{Author: "dd"}); err == nil {
		t.Fatal("invalid edit saved")
	}
}

func TestRestore(t *testing.T) {
	dir, ls := newSet(t)
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	first, _ := LoadRevision(dir, 1)
	want, _ := DigestOf(first)
	if err := ls.Restore(1, Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	if ls.Revision() != 3 || ls.StateDigest() != want {
		t.Fatalf("restore gave revision %d %s, want 3 %s", ls.Revision(), ls.StateDigest(), want)
	}
	tip, _, _ := journalTipOf(t, dir)
	if tip.RestoredFrom != 1 || tip.Change.Operation != "restore" {
		t.Fatalf("restore line %+v", tip)
	}
}

func journalTipOf(t *testing.T, dir string) (*Delta, int, error) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	return journalTip(root)
}

func TestPublishCarriesStateOnly(t *testing.T) {
	dir, ls := newSet(t)
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "published")
	if err := Publish(dir, dest); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{JournalFile, JournalBaseDir, LockFile} {
		if _, err := os.Stat(filepath.Join(dest, absent)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("published copy has %s", absent)
		}
	}
	pub, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	if pub.Revision() != 2 || pub.StateDigest() != ls.StateDigest() {
		t.Fatalf("published at %d %s", pub.Revision(), pub.StateDigest())
	}
	if _, err := LoadRevision(dest, 1); !errors.Is(err, ErrHistoryNotRetained) {
		t.Fatalf("published revision 1: %v", err)
	}
	// Editing a published copy starts a journal from its head.
	addMember(t, pub, 41)
	if err := pub.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	if n, err := Verify(dest); err != nil || n != 2 {
		t.Fatalf("verify after base: %d revisions, %v", n, err)
	}
	if _, err := LoadRevision(dest, 2); err != nil {
		t.Fatalf("base revision: %v", err)
	}
	if _, err := LoadRevision(dest, 1); !errors.Is(err, ErrHistoryNotRetained) {
		t.Fatalf("pre-base revision: %v", err)
	}
}

func TestOpenRollsForwardInterruptedSave(t *testing.T) {
	dir, ls := newSet(t)
	saved := map[string][]byte{}
	for _, name := range append(append([]string(nil), stateFiles...), ManifestFile) {
		saved[name], _ = os.ReadFile(filepath.Join(dir, name))
	}
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	want := ls.StateDigest()
	// Put the old head back while keeping the new journal line: a crash
	// after the append and before the head was replaced.
	for name, b := range saved {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	back, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if back.Revision() != 2 || back.StateDigest() != want {
		t.Fatalf("recovered to %d %s, want 2 %s", back.Revision(), back.StateDigest(), want)
	}
}

func TestInterruptedAppendIsDiscarded(t *testing.T) {
	dir, ls := newSet(t)
	f, err := os.OpenFile(filepath.Join(dir, JournalFile), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"revision":2,"parent`)
	f.Close()
	if _, err := Open(dir); err != nil {
		t.Fatalf("open with a torn final line: %v", err)
	}
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	if n, err := Verify(dir); err != nil || n != 2 {
		t.Fatalf("verify %d: %v", n, err)
	}
}

func TestCreateRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644)
	if _, err := Create(dir, Manifest{Layout: Pandar40P}, testState(), Change{Author: "dd"}); err == nil {
		t.Fatal("created over existing files")
	}
}

func TestReviewStatusSurvivesRoundTrip(t *testing.T) {
	dir, _ := newSet(t)
	ls, _ := Open(dir)
	for _, m := range ls.State.Masks {
		if m.ObjectID == "obj_b" && m.Status != annotation.StatusProposed {
			t.Fatalf("a proposal became %s", m.Status)
		}
	}
}
