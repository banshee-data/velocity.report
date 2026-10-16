package labelset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// chunkedSet is a label set whose two windows each get their own chunk file.
func chunkedSet(t *testing.T) (string, *LabelSet) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "kirk0-reference")
	ls, err := Create(dir, Manifest{Name: "kirk0-reference", Layout: Pandar40P, Storage: Storage{WindowChunk: 1}},
		testState(), Change{Author: "dd", Operation: "create"})
	if err != nil {
		t.Fatal(err)
	}
	return dir, ls
}

// The layout is a storage choice, not part of the state: two chunk spans hold
// the same state under the same digest, and the head reads back to it.
func TestStorageLayoutKeepsTheDigest(t *testing.T) {
	dir, ls := chunkedSet(t)
	_, wide := newSet(t)
	if ls.StateDigest() != wide.StateDigest() {
		t.Fatalf("chunk span changed the digest: %s and %s", ls.StateDigest(), wide.StateDigest())
	}
	got := map[string]bool{}
	for name := range headFiles(t, dir) {
		got[name] = true
	}
	for _, name := range []string{ManifestFile, PacketsFile, RecordsFile, DomainsFile, WindowsFile,
		"masks/000000-000000.ndjson", "masks/000001-000001.ndjson",
		"domain-returns/000000-000000.ndjson", "domain-returns/000001-000001.ndjson"} {
		if !got[name] {
			t.Errorf("%s missing from the head: %v", name, got)
		}
	}
	if len(got) != 9 {
		t.Errorf("head holds %v", got)
	}
	back, err := Open(dir)
	if err != nil || back.StateDigest() != ls.StateDigest() {
		t.Fatalf("reopened %v", err)
	}
	if n, err := Verify(dir); err != nil || n != 1 {
		t.Fatalf("verify: %d, %v", n, err)
	}
}

// A save rewrites only the files its edit changed, and removes a chunk whose
// masks are all gone.
func TestSaveRewritesOnlyChangedChunks(t *testing.T) {
	dir, ls := chunkedSet(t)
	untouched := filepath.Join(dir, "masks", "000001-000001.ndjson")
	before, _ := os.Stat(untouched)
	time.Sleep(20 * time.Millisecond)
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.Stat(untouched); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("a window-0 edit rewrote window 1's chunk")
	}
	// Every mask of window 1 goes.
	kept := ls.State.Masks[:0]
	for _, m := range ls.State.Masks {
		if m.WindowID != 1 {
			kept = append(kept, m)
		}
	}
	ls.State.Masks = kept
	if err := ls.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(untouched); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an emptied chunk was left behind: %v", err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if n, err := Verify(dir); err != nil || n != 3 {
		t.Fatalf("verify: %d, %v", n, err)
	}
}

// The reader holds the files to the layout: a stray file, a chunk of the
// wrong span, a mask in the wrong chunk, a reformatted file and the old
// single file are refused; Finder's .DS_Store and an interrupted write are
// passed over.
func TestOpenHoldsTheHeadToTheLayout(t *testing.T) {
	for name, damage := range map[string]struct {
		do   func(dir string)
		want string
	}{
		"stray file": {func(dir string) {
			os.WriteFile(filepath.Join(dir, "masks", "notes.txt"), []byte("x"), 0o644)
		}, "not a chunk file"},
		"wrong span": {func(dir string) {
			os.Rename(filepath.Join(dir, "masks", "000001-000001.ndjson"), filepath.Join(dir, "masks", "000001-000002.ndjson"))
		}, "does not span"},
		"mask in the wrong chunk": {func(dir string) {
			b, _ := os.ReadFile(filepath.Join(dir, "masks", "000001-000001.ndjson"))
			f, _ := os.OpenFile(filepath.Join(dir, "masks", "000000-000000.ndjson"), os.O_APPEND|os.O_WRONLY, 0)
			f.Write(b)
			f.Close()
			os.Remove(filepath.Join(dir, "masks", "000001-000001.ndjson"))
		}, "outside"},
		"reformatted": {func(dir string) {
			b, _ := os.ReadFile(filepath.Join(dir, WindowsFile))
			var v any
			json.Unmarshal(b, &v)
			compact, _ := json.Marshal(v)
			os.WriteFile(filepath.Join(dir, WindowsFile), append(compact, '\n'), 0o644)
		}, "labels rehead"},
		"old single file beside": {func(dir string) {
			os.WriteFile(filepath.Join(dir, MasksFile), nil, 0o644)
		}, "single-file layout"},
	} {
		dir, _ := chunkedSet(t)
		damage.do(dir)
		if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), damage.want) {
			t.Errorf("%s: %v, want %q", name, err, damage.want)
		}
	}
	dir, _ := chunkedSet(t)
	os.WriteFile(filepath.Join(dir, "masks", ".DS_Store"), []byte("finder"), 0o644)
	os.WriteFile(filepath.Join(dir, "masks", "000000-000000.ndjson.1234.tmp"), []byte("half"), 0o644)
	if _, err := Open(dir); err != nil {
		t.Fatalf("hidden and interrupted files: %v", err)
	}
}

// oldLayout puts a label set back in the single-file layout of 2026-10-12:
// the canonical files at their own names and a labels.json with no storage.
func oldLayout(t *testing.T, dir string, heads ...string) {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, head := range heads {
		root, err := os.OpenRoot(filepath.Join(dir, head))
		if err != nil {
			t.Fatal(err)
		}
		state, err := readHeadState(root, ".", m.Storage)
		root.Close()
		if err != nil {
			t.Fatal(err)
		}
		files, err := EncodeState(state)
		if err != nil {
			t.Fatal(err)
		}
		os.RemoveAll(filepath.Join(dir, head, MasksDir))
		os.RemoveAll(filepath.Join(dir, head, DomainReturnsDir))
		for name, b := range files {
			os.WriteFile(filepath.Join(dir, head, name), b, 0o644)
		}
	}
	var raw map[string]any
	json.Unmarshal(b, &raw)
	delete(raw, "storage")
	b, _ = json.MarshalIndent(raw, "", "  ")
	os.WriteFile(filepath.Join(dir, ManifestFile), b, 0o644)
}

// A label set in the single-file layout is refused with the way out, and
// relayout converts it, journal base included, with its digest and journal
// unchanged.
func TestRelayoutConvertsTheSingleFileLayout(t *testing.T) {
	src, ls := newSet(t)
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	// A published copy, edited, so it has a journal base too.
	copyDir := filepath.Join(t.TempDir(), "published")
	if err := Publish(src, copyDir); err != nil {
		t.Fatal(err)
	}
	pub, err := Open(copyDir)
	if err != nil {
		t.Fatal(err)
	}
	addMember(t, pub, 41)
	if err := pub.Save(Change{Author: "dd"}); err != nil {
		t.Fatal(err)
	}
	want := pub.StateDigest()
	oldLayout(t, copyDir, ".", JournalBaseDir)

	if _, err := Open(copyDir); !errors.Is(err, ErrOldLayout) {
		t.Fatalf("an old-layout set opened: %v", err)
	}
	if err := Relayout(copyDir, 1); err != nil {
		t.Fatalf("relayout: %v", err)
	}
	back, err := Open(copyDir)
	if err != nil || back.StateDigest() != want || back.Manifest.Storage.WindowChunk != 1 {
		t.Fatalf("after relayout: %v %+v", err, back)
	}
	for _, gone := range []string{MasksFile, DomainReturnsFile, filepath.Join(JournalBaseDir, MasksFile)} {
		if _, err := os.Stat(filepath.Join(copyDir, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left behind", gone)
		}
	}
	if n, err := Verify(copyDir); err != nil || n != 2 {
		t.Fatalf("verify after relayout: %d, %v", n, err)
	}
	// A second relayout changes only the chunk span.
	if err := Relayout(copyDir, 100); err != nil {
		t.Fatal(err)
	}
	if back, err := Open(copyDir); err != nil || back.StateDigest() != want {
		t.Fatalf("after a second relayout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(copyDir, "masks", "000001-000001.ndjson")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the narrow chunks survived a wider relayout")
	}
	if err := Relayout(copyDir, 0); err == nil {
		t.Fatal("a zero chunk span was accepted")
	}
}
