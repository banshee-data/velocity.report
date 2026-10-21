package labelset

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

var nestFrames = []testFrame{{0, 2, []uint64{5, 6, 7, 450}}, {2, 2, []uint64{800, 900}}}

// labelledPack writes a pack of kirk0.pcapng at dir and gives it a label set
// at setDir with one mask, linked from the pack.
func labelledPack(t *testing.T, dir, setDir, run string) (*annotation.Pack, *LabelSet) {
	t.Helper()
	p := writeCapturePack(t, dir, "kirk0.pcapng", []uint64{10, 11, 12, 13}, nestFrames, run)
	layout, state, err := NewFromPack(p)
	if err != nil {
		t.Fatal(err)
	}
	ls, err := Create(setDir, Manifest{Name: filepath.Base(setDir), Layout: layout}, state, Change{Author: "dd", Operation: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if err := annotation.WriteLabelSetLink(p, ls.Manifest.LabelSetID, setDir); err != nil {
		t.Fatal(err)
	}
	return p, ls
}

func linkOf(t *testing.T, dir string) *annotation.LabelSetLink {
	t.Helper()
	link, err := annotation.ReadLabelSetLink(dir)
	if err != nil || link == nil {
		t.Fatalf("%s: link %v %v", dir, link, err)
	}
	return link
}

// The tree the tools wrote before the layout: packs at the root beside
// labels/<set>/, a split draft naming a pack by its folder.
func TestNestMovesTheOldLocalTree(t *testing.T) {
	root := t.TempDir()
	linked, ls := labelledPack(t, filepath.Join(root, "run-a-20260929"), filepath.Join(root, "labels", "kirk0-reference"), "ad8b9438-c1d4")
	writeCapturePack(t, filepath.Join(root, "run-b-20261014"), "kirk0.pcapng", []uint64{10, 11, 12, 13}, nestFrames, "e44ae459-ca38")
	draft := filepath.Join(root, "kirk0-draft.json")
	if err := os.WriteFile(draft, []byte(`{"packs": [{"dir": "run-a-20260929"}]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := Nest(root, true)
	if err != nil || len(plan.Moves) != 3 {
		t.Fatalf("dry run: %v %+v", err, plan)
	}
	if _, err := os.Stat(linked.Dir); err != nil {
		t.Fatalf("a dry run moved the pack: %v", err)
	}

	report, err := Nest(root, false)
	if err != nil {
		t.Fatalf("%v %+v", err, report)
	}
	set := filepath.Join(root, "kirk0", "kirk0-reference")
	view := filepath.Join(set, annotation.ViewsDir, annotation.ViewName(linked.Manifest))
	if l := linkOf(t, view); l.Path != annotation.ViewLinkPath || l.LabelSetID != ls.Manifest.LabelSetID {
		t.Fatalf("view link %+v", l)
	}
	if _, err := annotation.OpenPack(filepath.Join(root, "kirk0", annotation.PacksDir, "run-b-20261014")); err != nil {
		t.Fatalf("the unlinked pack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "labels")); !os.IsNotExist(err) {
		t.Fatalf("the emptied labels/ folder remains: %v", err)
	}
	if b, _ := os.ReadFile(draft); !strings.Contains(string(b), `"dir": "kirk0/kirk0-reference/views/`) {
		t.Fatalf("the draft still names the old folder: %s", b)
	}
	pack, err := annotation.OpenPack(view)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := annotation.LoadSidecar(pack); err != nil {
		t.Fatalf("the moved view's membership: %v", err)
	}
	if dirs, _ := annotation.ListPackDirs(root); len(dirs) != 2 {
		t.Fatalf("listed %v", dirs)
	}

	again, err := Nest(root, false)
	if err != nil || len(again.Moves) != 0 || len(again.Relinked) != 0 {
		t.Fatalf("a nested tree moved again: %v %+v", err, again)
	}
}

// A dataset's labels/<capture>/ held a set beside its view. The set stays in
// its capture folder, which is the dataset's name for the capture, not the
// pack's file name; the view moves into it.
func TestNestMovesADatasetViewIntoItsSet(t *testing.T) {
	root := filepath.Join(t.TempDir(), "labels")
	stem := filepath.Join(root, "20251205T1417_80858-75c_kirkham-9th")
	view, _ := labelledPack(t, filepath.Join(stem, "ad8b9438-c1d4-v2"), filepath.Join(stem, "kirk0-reference"), "ad8b9438-c1d4")
	if _, err := Nest(root, false); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(stem, "kirk0-reference", annotation.ViewsDir, annotation.ViewName(view.Manifest))
	if l := linkOf(t, moved); l.Path != annotation.ViewLinkPath {
		t.Fatalf("link %+v", l)
	}
	if _, err := CheckView(moved); err != nil {
		t.Fatalf("the moved view does not hold: %v", err)
	}
}

// A pack whose set is missing refuses the whole run.
func TestNestRefusesBeforeMovingAnything(t *testing.T) {
	root := t.TempDir()
	p, _ := labelledPack(t, filepath.Join(root, "run-a"), filepath.Join(root, "labels", "kirk0-reference"), "run-a")
	writeCapturePack(t, filepath.Join(root, "run-b"), "kirk0.pcapng", []uint64{10, 11, 12, 13}, nestFrames, "run-b")
	if err := os.WriteFile(filepath.Join(p.Dir, annotation.LabelSetLinkFile), []byte(`{"label_set_id": "x", "path": "../gone"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Nest(root, false)
	if !errors.Is(err, ErrNestRefused) || len(report.Problems) == 0 {
		t.Fatalf("%v %+v", err, report)
	}
	for _, dir := range []string{"run-a", "run-b", "labels/kirk0-reference"} {
		if _, err := os.Stat(filepath.Join(root, dir)); err != nil {
			t.Fatalf("%s moved: %v", dir, err)
		}
	}
}

// A first save from a pack in packs/ creates the set beside packs/; nest then
// moves the pack into the set's views/.
func TestFirstSaveInTheLayoutThenNest(t *testing.T) {
	root := t.TempDir()
	p := writeCapturePack(t, filepath.Join(root, "kirk0", annotation.PacksDir, "run-a"), "kirk0.pcapng", []uint64{10, 11, 12, 13}, nestFrames, "run-a")
	sc, err := annotation.LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	sc.Objects = []annotation.Object{{ObjectID: "obj_a", Class: "car", Status: annotation.StatusReviewed}}
	sc.Masks = []annotation.FrameMask{{ObjectID: "obj_a", SampleID: 0, PointIndices: []int{0, 1},
		Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed}}
	ls, err := SaveMembership(p, sc, Change{Author: "dd", Operation: "save_mask"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(ls.Dir) != filepath.Join(root, "kirk0") {
		t.Fatalf("the first save made %s", ls.Dir)
	}
	if _, err := Nest(root, false); err != nil {
		t.Fatal(err)
	}
	view := filepath.Join(ls.Dir, annotation.ViewsDir, annotation.ViewName(p.Manifest))
	moved, err := annotation.OpenPack(view)
	if err != nil {
		t.Fatal(err)
	}
	back, err := annotation.LoadSidecar(moved)
	if err != nil || len(back.Masks) != 1 {
		t.Fatalf("membership after the move: %+v %v", back, err)
	}
}

// A dataset holds packs in a set's views/ and in a capture's packs/, linked
// or not: check-tree holds a linked one in packs/ as it does a view, and
// finds a pack anywhere else in the tree.
func TestCheckTreeTakesPacksInPacksAndFindsOthers(t *testing.T) {
	root := t.TempDir()
	labelledPack(t, filepath.Join(root, "kirk0", annotation.PacksDir, "run-a"), filepath.Join(root, "kirk0", "kirk0-reference"), "run-a")
	writeCapturePack(t, filepath.Join(root, "kirk0", annotation.PacksDir, "run-b"), "kirk0.pcapng", []uint64{10, 11, 12, 13}, nestFrames, "run-b")
	r, err := CheckTree(root)
	if err != nil || len(r.Packs) != 2 || len(r.Views) != 1 {
		t.Fatalf("a linked and an unlinked pack in packs/: %v %+v", err, r)
	}
	writeCapturePack(t, filepath.Join(root, "kirk0", "beside-its-set"), "kirk0.pcapng", []uint64{10, 11, 12, 13}, nestFrames, "run-c")
	r, err = CheckTree(root)
	if err == nil || len(r.Problems) != 1 || !strings.Contains(r.Problems[0], "outside the layout") {
		t.Fatalf("a pack beside its set: %v %+v", err, r)
	}
}
