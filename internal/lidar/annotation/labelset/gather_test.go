package labelset

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

const (
	kirkStem = "20251205T1417_80858-75c_kirkham-9th"
	embStem  = "20260903T1317_80858-06c_embarcadero-folsom"
)

// gatherFixture is a local tree in the old layout and a dataset that has
// published one of its sets with a view: the state the annotation packs
// folder and the Hugging Face clone were in before the work moved into the
// clone.
type gatherFixture struct {
	from, dataset, into string
	refPack, gtPack     string
	refSet              *LabelSet
}

// framesFor gives each test pack its own returns, so each has its own digest.
func framesFor(seed uint64) []testFrame {
	return []testFrame{{0, 2, []uint64{5, 6, 7, 450 + seed}}, {2, 2, []uint64{800, 900 + seed}}}
}

// boundSet makes a pack of kirk0.pcapng at packDir with a label set at setDir
// bound to the dataset's kirkham-9th file, and one saved edit, so the set has
// a journal.
func boundSet(t *testing.T, packDir, setDir, run string, seed uint64) *LabelSet {
	t.Helper()
	p := writeCapturePack(t, packDir, "kirk0.pcapng", []uint64{10, 11, 12, 13}, framesFor(seed), run)
	layout, state, err := NewFromPack(p)
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{Name: filepath.Base(setDir), Layout: layout, AppliesTo: []Binding{{Name: "kirk0.pcapng", SHA256: "sha256:kirk"}}}
	ls, err := Create(setDir, m, state, Change{Author: "dd", Operation: "create"})
	if err != nil {
		t.Fatal(err)
	}
	ls.State.Records.Objects = append(ls.State.Records.Objects, annotation.Object{ObjectID: "obj_" + run, Class: "car", Status: annotation.StatusProposed, Confidence: 1})
	if err := ls.Save(Change{Author: "dd", Operation: "edit"}); err != nil {
		t.Fatal(err)
	}
	if err := annotation.WriteLabelSetLink(p, ls.Manifest.LabelSetID, setDir); err != nil {
		t.Fatal(err)
	}
	return ls
}

func newGatherFixture(t *testing.T) *gatherFixture {
	t.Helper()
	f := &gatherFixture{from: t.TempDir(), dataset: t.TempDir()}
	f.into = filepath.Join(f.dataset, "labels")
	rows := `[{"raw_path": "raw/lidar/` + kirkStem + `.pcapng", "raw_sha256": "sha256:kirk"},
 {"raw_path": "raw/lidar/` + embStem + `.pcapng", "raw_sha256": "sha256:emb"}]`
	if err := os.WriteFile(filepath.Join(f.dataset, "manifest.json"), []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	f.refPack = filepath.Join(f.from, "ad8b9438-v2")
	f.refSet = boundSet(t, f.refPack, filepath.Join(f.from, "labels", "kirk0-reference"), "ad8b9438", 1)
	f.gtPack = filepath.Join(f.from, "8e422582-v2")
	boundSet(t, f.gtPack, filepath.Join(f.from, "labels", "kirk0-sweep-gt"), "8e422582", 2)
	writeCapturePack(t, filepath.Join(f.from, "10715f01-run"), embStem+".pcapng", []uint64{10, 11, 12, 13}, framesFor(3), "10715f01")
	// The working state a pack collects as it is labelled.
	if err := os.MkdirAll(filepath.Join(f.refPack, "proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"kirk0-draft.json":  `{"packs": [{"dir": "ad8b9438-v2"}, {"dir": "8e422582-v2"}]}` + "\n",
		"splits/s1.json":    `{"split": 1}` + "\n",
		"splits/.DS_Store":  "",
		"labels/.DS_Store":  "",
		"ad8b9438-v2/x.tmp": "",
	} {
		path := filepath.Join(f.from, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The dataset published kirk0-reference at its head, with its view.
	pubSet := filepath.Join(f.into, kirkStem, "kirk0-reference")
	if err := Publish(f.refSet.Dir, pubSet); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishView(f.refPack, pubSet, filepath.Join(pubSet, annotation.ViewsDir, "20260929-184718-ad8b9438")); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestGatherMovesALocalTreeIntoTheDataset(t *testing.T) {
	f := newGatherFixture(t)
	opts := GatherOptions{Dataset: f.dataset, DryRun: true}
	plan, err := Gather(f.from, f.into, opts)
	if err != nil {
		t.Fatalf("dry run: %v %+v", err, plan)
	}
	if len(plan.Adopted) != 2 || len(plan.Drafts) != 1 {
		t.Fatalf("dry run planned %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(f.into, kirkStem, "kirk0-reference", JournalFile)); !os.IsNotExist(err) {
		t.Fatalf("a dry run moved the journal: %v", err)
	}

	opts.DryRun = false
	report, err := Gather(f.from, f.into, opts)
	if err != nil {
		t.Fatalf("%v %+v", err, report)
	}
	// The published set keeps its place and takes the local history: it
	// opens at the same head, and its journal replays.
	ref := filepath.Join(f.into, kirkStem, "kirk0-reference")
	ls, err := Open(ref)
	if err != nil || ls.Revision() != f.refSet.Revision() {
		t.Fatalf("the dataset's set after gathering: %v", err)
	}
	if revs, err := Verify(ref); err != nil || revs != f.refSet.Revision() {
		t.Fatalf("its journal: %d revisions, %v", revs, err)
	}
	// The published view takes the local pack's working state.
	view := filepath.Join(ref, annotation.ViewsDir, "20260929-184718-ad8b9438")
	if _, err := os.Stat(filepath.Join(view, "proposals")); err != nil {
		t.Fatalf("the view did not take the proposals: %v", err)
	}
	// The unpublished set goes to the dataset capture its file names, and
	// its pack into its views/; the pack in no set into its capture's packs/.
	gt := filepath.Join(f.into, kirkStem, "kirk0-sweep-gt")
	if _, err := Open(gt); err != nil {
		t.Fatal(err)
	}
	if views := SetViews(gt); len(views) != 1 {
		t.Fatalf("kirk0-sweep-gt views %v", views)
	}
	if _, err := annotation.OpenPack(filepath.Join(f.into, embStem, annotation.PacksDir, "10715f01-run")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(f.into, "kirk0-draft.json")); err != nil || !strings.Contains(string(b), `"dir": "`+kirkStem+`/kirk0-reference/views/20260929-184718-ad8b9438"`) {
		t.Fatalf("the draft: %s %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(f.into, "splits", "s1.json")); err != nil {
		t.Fatal(err)
	}
	// The local copies the dataset already had are set aside, not deleted.
	for _, kept := range []string{"labels/kirk0-reference/labels.json", "ad8b9438-v2/manifest.json", "kirk0-draft.json"} {
		if _, err := os.Stat(filepath.Join(f.from, filepath.FromSlash(report.Archive), filepath.FromSlash(kept))); err != nil {
			t.Fatalf("%s was not kept aside: %v", kept, err)
		}
	}
	if r, err := CheckTree(f.into); err != nil {
		t.Fatalf("the dataset after gathering: %v %+v", err, r)
	}
}

func TestGatherRefusesBeforeMovingAnything(t *testing.T) {
	f := newGatherFixture(t)
	// The local set moves on after it was published.
	f.refSet.State.Records.Objects = append(f.refSet.State.Records.Objects, annotation.Object{ObjectID: "obj_new", Class: "car", Status: annotation.StatusProposed, Confidence: 1})
	if err := f.refSet.Save(Change{Author: "dd", Operation: "edit"}); err != nil {
		t.Fatal(err)
	}
	// And a pack of a capture the dataset does not hold.
	writeCapturePack(t, filepath.Join(f.from, "s2-run"), "s2_sf_3_20260902132037_00002.pcap", []uint64{10, 11, 12, 13}, framesFor(4), "s2")
	report, err := Gather(f.from, f.into, GatherOptions{Dataset: f.dataset})
	if !errors.Is(err, ErrNestRefused) {
		t.Fatalf("%v %+v", err, report)
	}
	problems := strings.Join(report.Problems, "\n")
	for _, want := range []string{"the local copy is at revision", "which the dataset does not hold"} {
		if !strings.Contains(problems, want) {
			t.Errorf("problems lack %q:\n%s", want, problems)
		}
	}
	for _, still := range []string{"labels/kirk0-reference", "labels/kirk0-sweep-gt", "8e422582-v2", "10715f01-run"} {
		if _, err := os.Stat(filepath.Join(f.from, still)); err != nil {
			t.Fatalf("%s moved: %v", still, err)
		}
	}
}
