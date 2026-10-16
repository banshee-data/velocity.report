package labelset

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// The merge drivers, as 'velocity lidar labels merge-driver' runs them, from
// this test binary: Git calls it with the driver's arguments and
// LABELSET_TEST_DRIVER naming the driver.
func TestMain(m *testing.M) {
	if kind := os.Getenv("LABELSET_TEST_DRIVER"); kind != "" {
		os.Exit(runTestDriver(kind, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func runTestDriver(kind string, args []string) int {
	base, _ := os.ReadFile(args[0])
	ours, err := os.ReadFile(args[1])
	if err != nil {
		return 2
	}
	theirs, err := os.ReadFile(args[2])
	if err != nil {
		return 2
	}
	var merged []byte
	var conflicted bool
	switch kind {
	case "head":
		var conflict string
		merged, conflict, err = MergeManifests(base, ours, theirs)
		conflicted = conflict != ""
	case "refs":
		var conflicts []string
		merged, conflicts, err = annotation.MergePhysicalReferences(base, ours, theirs)
		conflicted = len(conflicts) > 0
	}
	if err != nil {
		return 2
	}
	if merged != nil && os.WriteFile(args[1], merged, 0o644) != nil {
		return 2
	}
	if conflicted {
		return 1
	}
	return 0
}

// datasetGit runs git in a dataset clone, failing the test on an error.
func datasetGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// Two researchers edit one published view in two clones: one changes obj_a's
// mask in window 0, the other adds a pedestrian with a mask in window 1. Git
// merges the chunk file line by line, the head driver records the merge in
// labels.json, and FinishMerges makes the merged files the next revision.
func TestGitFinishesAMergeOfLabelSets(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	packDir, setDir := viewSource(t)
	view, pubSet, _ := publishViewBeside(t, packDir, setDir)
	a := filepath.Dir(filepath.Dir(filepath.Dir(pubSet)))
	root := filepath.Join(a, "labels")
	rules := map[string]string{
		".gitignore": strings.Join([]string{
			"labels/**/journal.ndjson", "labels/**/journal-base/", "labels/**/.lock", "labels/**/.pending", "labels/**/*.tmp",
			"labels/**/.annotations.lock", "labels/**/physical-reference-revisions/", "labels/**/feature-proposal-revisions/",
			"labels/**/proposals/", "labels/**/.publish-view-*/",
		}, "\n") + "\n",
		".gitattributes": "labels/** -text\nlabels/**/labels.json merge=labelset-head\nlabels/**/physical-references.json merge=labelset-refs\n",
	}
	for name, body := range rules {
		if err := os.WriteFile(filepath.Join(a, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	datasetGit(t, a, "init", "-q", "-b", "main")
	datasetGit(t, a, "add", "-A")
	datasetGit(t, a, "commit", "-q", "-m", "publish")
	b := filepath.Join(t.TempDir(), "clone-b")
	datasetGit(t, filepath.Dir(b), "clone", "-q", a, b)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"head", "refs"} {
		datasetGit(t, a, "config", "merge.labelset-"+kind+".driver", "env LABELSET_TEST_DRIVER="+kind+" '"+exe+"' %O %A %B %P")
	}
	rel, _ := filepath.Rel(a, view)
	setRel, _ := filepath.Rel(a, pubSet)
	start, _ := Open(pubSet)

	// Researcher B, in their clone: a pedestrian in sample 1 (window 1).
	viewB := filepath.Join(b, rel)
	pb, err := annotation.OpenPack(viewB)
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := annotation.LoadSidecar(pb)
	sc.Objects = append(sc.Objects, annotation.Object{ObjectID: "ped_1", Class: "pedestrian", Status: annotation.StatusReviewed, Confidence: 1})
	sc.Masks = append(sc.Masks, annotation.FrameMask{ObjectID: "ped_1", SampleID: 1, PointIndices: []int{0},
		Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed})
	if _, err := SaveMembership(pb, sc, Change{Author: "b", Operation: "save_mask"}); err != nil {
		t.Fatal(err)
	}
	datasetGit(t, b, "commit", "-q", "-am", "b: a pedestrian")

	// Researcher A, in theirs: obj_a gains a return in window 0.
	pa, _ := annotation.OpenPack(view)
	sc, _ = annotation.LoadSidecar(pa)
	for i := range sc.Masks {
		if sc.Masks[i].ObjectID == "obj_a" {
			sc.Masks[i].PointIndices = []int{0, 1, 2}
		}
	}
	if _, err := SaveMembership(pa, sc, Change{Author: "a", Operation: "save_mask"}); err != nil {
		t.Fatal(err)
	}
	datasetGit(t, a, "commit", "-q", "-am", "a: obj_a")

	datasetGit(t, a, "pull", "-q", "--no-rebase", "--no-commit", b, "main")
	// Mid-merge, the set opens for nothing, and a tree check says why.
	if _, err := Open(pubSet); !errors.Is(err, ErrUnfinishedMerge) {
		t.Fatalf("a set mid-merge opened: %v", err)
	}
	if r, err := CheckTree(root); err == nil || !strings.Contains(strings.Join(r.Problems, "\n"), "middle of a Git merge") {
		t.Fatalf("check-tree mid-merge: %v %+v", err, r)
	}

	done, paths, err := FinishMerges(root, Change{Author: "a"})
	if err != nil || len(done) != 1 {
		t.Fatalf("finish: %v %+v", err, done)
	}
	datasetGit(t, a, append([]string{"add", "--"}, paths...)...)
	datasetGit(t, a, "commit", "-q", "-m", "merge b")
	if got := datasetGit(t, a, "status", "--porcelain"); got != "" {
		t.Fatalf("after the merge git reports\n%s", got)
	}

	merged, err := Open(pubSet)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Revision() != start.Revision()+2 {
		t.Fatalf("merged at revision %d, from %d", merged.Revision(), start.Revision())
	}
	sc, err = annotation.LoadSidecar(pa)
	if err != nil {
		t.Fatal(err)
	}
	var objA, ped bool
	for _, m := range sc.Masks {
		objA = objA || (m.ObjectID == "obj_a" && len(m.PointIndices) == 3)
		ped = ped || m.ObjectID == "ped_1"
	}
	if !objA || !ped {
		t.Fatalf("the merge lost an edit: %+v", sc.Masks)
	}
	if r, err := CheckTree(root); err != nil {
		t.Fatalf("check-tree after the merge: %v %+v", err, r)
	}

	// What the merge brought in, from Git: the pedestrian.
	then, cleanup, err := OpenRevision(a, "HEAD~1", filepath.ToSlash(setRel))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	s := Summarise(&then.State, &merged.State)
	if len(s.ObjectsAdded) != 1 || s.ObjectsAdded[0].ObjectID != "ped_1" || len(s.MasksAdded) != 1 || len(s.MasksChanged) != 0 {
		t.Fatalf("the merge's diff from HEAD~1: %+v", s)
	}
}
