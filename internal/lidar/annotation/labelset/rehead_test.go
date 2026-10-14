package labelset

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeMerged puts a state's head files under dir, as a merge would leave
// them, without touching labels.json.
func writeMerged(t *testing.T, dir string, s *State, st Storage) {
	t.Helper()
	files, err := encodeStorage(s, st)
	if err != nil {
		t.Fatal(err)
	}
	saved := headFiles(t, dir)
	for name := range saved {
		if name != ManifestFile {
			delete(saved, name)
		}
	}
	for name, b := range files {
		saved[name] = b
	}
	putHead(t, dir, saved)
}

func maskOf(s *State, object string, window int) *Mask {
	for i := range s.Masks {
		if s.Masks[i].ObjectID == object && s.Masks[i].WindowID == window {
			return &s.Masks[i]
		}
	}
	return nil
}

func TestReheadMakesMergedFilesTheHead(t *testing.T) {
	dir, ls := newSet(t)
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "a"}); err != nil {
		t.Fatal(err)
	}
	if r, err := Rehead(dir, Change{Author: "dd"}); err != nil || r.Revised || r.Relaid {
		t.Fatalf("rehead of an unchanged head: %+v %v", r, err)
	}

	// Another branch's edit to window 1, merged in by Git.
	merged := ls.State.Clone()
	m1 := maskOf(&merged, "obj_a", 1)
	m1.Members = Union(m1.Members, Ranges{{50, 1}})
	writeMerged(t, dir, &merged, ls.Manifest.Storage)
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "labels rehead") {
		t.Fatalf("merged files opened without a rehead: %v", err)
	}
	r, err := Rehead(dir, Change{Author: "dd"})
	if err != nil || !r.Revised || r.From.Revision != 2 || r.To.Revision != 3 {
		t.Fatalf("rehead: %+v %v", r, err)
	}
	back, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !maskOf(&back.State, "obj_a", 0).Members.Contains(40) || !maskOf(&back.State, "obj_a", 1).Members.Contains(50) {
		t.Fatal("the reheaded state lost one branch's edit")
	}
	if n, err := Verify(dir); err != nil || n != 3 {
		t.Fatalf("the journal does not replay through the merge: %d, %v", n, err)
	}

	// A layout change alone is rewritten, not revised.
	b, _ := os.ReadFile(filepath.Join(dir, WindowsFile))
	os.WriteFile(filepath.Join(dir, WindowsFile), []byte(strings.ReplaceAll(string(b), "\n  ", "\n")), 0o644)
	if r, err := Rehead(dir, Change{Author: "dd"}); err != nil || r.Revised || !r.Relaid {
		t.Fatalf("rehead of a reformatted file: %+v %v", r, err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
}

func TestReheadRefuses(t *testing.T) {
	// Two branches gave one return to two objects: not a label set.
	dir, ls := newSet(t)
	bad := ls.State.Clone()
	b0 := maskOf(&bad, "obj_b", 0)
	b0.Members = Union(b0.Members, Ranges{{10, 1}}) // obj_a's return 10
	writeMerged(t, dir, &bad, ls.Manifest.Storage)
	before := headFiles(t, dir)
	if _, err := Rehead(dir, Change{Author: "dd"}); err == nil || !strings.Contains(err.Error(), "not a valid label set") {
		t.Fatalf("rehead of a double claim: %v", err)
	}
	if after := headFiles(t, dir); len(after) != len(before) {
		t.Fatal("a refused rehead wrote files")
	}

	// The other branch's labels.json was kept over this copy's journal.
	dir, ls = newSet(t)
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "a"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	theirs := strings.Replace(string(b), `"revision": 2`, `"revision": 5`, 1)
	theirs = strings.Replace(theirs, ls.StateDigest(), "sha256:theirs", 1)
	os.WriteFile(filepath.Join(dir, ManifestFile), []byte(theirs), 0o644)
	if _, err := Rehead(dir, Change{Author: "dd"}); err == nil || !strings.Contains(err.Error(), "keep this copy's own") {
		t.Fatalf("rehead over a mismatched journal: %v", err)
	}
}

// Two people edit different masks of one published label set in their own
// clones. Git merges the masks file itself and conflicts only on labels.json;
// keeping this clone's labels.json and running rehead finishes the merge.
func TestTwoClonesMergeInGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	git := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com",
			"-c", "commit.gpgsign=false", "-c", "pull.rebase=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	must := func(dir string, args ...string) string {
		t.Helper()
		out, err := git(dir, args...)
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return out
	}
	src, _ := newSet(t)
	seed := filepath.Join(base, "seed")
	set := filepath.Join("labels", "stem", "kirk0-reference")
	if err := Publish(src, filepath.Join(seed, set)); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(seed, ".gitignore"), []byte("labels/**/journal.ndjson\nlabels/**/journal-base/\nlabels/**/.lock\nlabels/**/.pending\nlabels/**/*.tmp\n"), 0o644)
	os.WriteFile(filepath.Join(seed, ".gitattributes"), []byte("labels/** -text\n"), 0o644)
	must(seed, "init", "-q", "-b", "main")
	must(seed, "add", "-A")
	must(seed, "commit", "-q", "-m", "publish")
	must(base, "clone", "-q", "--bare", "seed", "origin.git")
	must(base, "clone", "-q", "origin.git", "a")
	must(base, "clone", "-q", "origin.git", "b")

	edit := func(clone string, object string, window int, ret uint64) {
		t.Helper()
		ls, err := Open(filepath.Join(base, clone, set))
		if err != nil {
			t.Fatal(err)
		}
		m := maskOf(&ls.State, object, window)
		m.Members = Union(m.Members, Ranges{{ret, 1}})
		if err := ls.Save(Change{Author: clone}); err != nil {
			t.Fatal(err)
		}
		must(filepath.Join(base, clone), "commit", "-q", "-am", clone+" edits "+object)
	}
	edit("b", "obj_a", 1, 50)
	must(filepath.Join(base, "b"), "push", "-q", "origin", "main")
	edit("a", "obj_a", 0, 40)

	a := filepath.Join(base, "a")
	if out, err := git(a, "pull", "-q", "origin", "main"); err == nil {
		t.Fatalf("the two heads merged without a conflict:\n%s", out)
	}
	conflicted := strings.Fields(must(a, "diff", "--name-only", "--diff-filter=U"))
	if len(conflicted) != 1 || conflicted[0] != filepath.ToSlash(filepath.Join(set, ManifestFile)) {
		t.Fatalf("conflicts in %v, want only labels.json", conflicted)
	}
	must(a, "checkout", "--ours", "--", filepath.Join(set, ManifestFile))
	r, err := Rehead(filepath.Join(a, set), Change{Author: "a"})
	if err != nil || !r.Revised {
		t.Fatalf("rehead after the merge: %+v %v", r, err)
	}
	merged, err := Open(filepath.Join(a, set))
	if err != nil {
		t.Fatal(err)
	}
	if !maskOf(&merged.State, "obj_a", 0).Members.Contains(40) || !maskOf(&merged.State, "obj_a", 1).Members.Contains(50) {
		t.Fatal("the merge lost an edit")
	}
	must(a, "add", "-A")
	must(a, "commit", "-q", "-m", "merge b")
	if out := must(a, "status", "--porcelain"); out != "" {
		t.Fatalf("after the merge commit git reports\n%s", out)
	}
	if _, err := Verify(filepath.Join(a, set)); err != nil {
		t.Fatalf("clone a's journal after the merge: %v", err)
	}
}

// An interrupted relayout is finished by running it again: before
// labels.json moves, the new span's files are partial output; after, the old
// span's are leftovers.
func TestRelayoutFinishesAnInterruptedRun(t *testing.T) {
	dir, ls := chunkedSet(t)
	want := ls.StateDigest()
	// Interrupted before labels.json: a partial file of the new span.
	os.WriteFile(filepath.Join(dir, "masks", "000000-000099.ndjson"), []byte("{\"partial\n"), 0o644)
	if err := Relayout(dir, 100); err != nil {
		t.Fatalf("relayout over a partial run: %v", err)
	}
	// Interrupted after labels.json: a leftover of the old span.
	os.WriteFile(filepath.Join(dir, "masks", "000001-000001.ndjson"), []byte("left over\n"), 0o644)
	if _, err := Open(dir); err == nil {
		t.Fatal("a leftover chunk of the old span opened")
	}
	if err := Relayout(dir, 100); err != nil {
		t.Fatalf("relayout over leftovers: %v", err)
	}
	back, err := Open(dir)
	if err != nil || back.StateDigest() != want {
		t.Fatalf("after finishing the relayout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "masks", "000001-000001.ndjson")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the leftover survived")
	}
}
