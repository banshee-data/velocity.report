package labelset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

func pfloat(v float64) *float64 { return &v }

// viewSource is a working copy as the labeller has it: a pack of run-a with
// two objects' masks in a label set beside it, a reviewed physical keyframe
// pinned to the label set's head, and the working state the editors leave
// in a pack (a lock, proposal layers, facets).
func viewSource(t *testing.T) (packDir, setDir string) {
	t.Helper()
	base := t.TempDir()
	pack := writeTestPack(t, filepath.Join(base, "pack-a"), []uint64{10, 11, 12, 13},
		[]testFrame{{0, 2, []uint64{5, 6, 7, 450, 451}}, {2, 2, []uint64{800, 900}}}, "run-a")
	layout, state, err := NewFromPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewViewMap(layout, &state, pack)
	if err != nil {
		t.Fatal(err)
	}
	w0, _ := vm.SampleWindow(0)
	a, _ := vm.Offsets(0, []int{0, 1})
	b, _ := vm.Offsets(0, []int{3})
	state.Records.Objects = []annotation.Object{
		{ObjectID: "obj_a", Class: "car", Status: annotation.StatusReviewed, Confidence: 1},
		{ObjectID: "obj_b", Class: "car", Status: annotation.StatusReviewed, Confidence: 1},
	}
	d := state.Domains[0].DomainID
	state.Masks = []Mask{
		{ObjectID: "obj_a", WindowID: w0, DomainID: d, Members: a, Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed},
		{ObjectID: "obj_b", WindowID: w0, DomainID: d, Members: b, Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed},
	}
	setDir = filepath.Join(base, "labels", "kirk0-reference")
	ls, err := Create(setDir, Manifest{Name: "kirk0-reference", Layout: layout}, state, Change{Author: "dd", Operation: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if err := annotation.WriteLabelSetLink(pack, ls.Manifest.LabelSetID, setDir); err != nil {
		t.Fatal(err)
	}
	pinPhysical(t, pack, ls)
	for name, body := range map[string]string{".annotations.lock": "", annotation.FeatureProposalsFile: "facets"} {
		if err := os.WriteFile(filepath.Join(pack.Dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(pack.Dir, "proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	return pack.Dir, setDir
}

// pinPhysical saves one reviewed keyframe of obj_b, resting on sample 0 and
// pinned to the label set's current head.
func pinPhysical(t *testing.T, p *annotation.Pack, ls *LabelSet) {
	t.Helper()
	doc, err := annotation.LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	review := annotation.PhysicalReview{
		Status: annotation.StatusReviewed, Origin: annotation.OriginIndependent, Method: "manual_box",
		UncertaintyAssumptions: "hard bounds", Provenance: annotation.Provenance{Author: "dd"},
		ReviewedAgainst: &annotation.MembershipPin{Revision: ls.Revision(), Digest: ls.StateDigest()},
	}
	doc.Change = annotation.Provenance{Author: "dd", Operation: "author"}
	doc.Objects = []annotation.PhysicalObject{{
		ObjectID: "obj_b",
		Keyframes: []annotation.PhysicalKeyframe{{
			KeyframeID: "kf-b-0", SampleID: 0, TimestampNs: p.Samples[0].TimestampNs,
			Anchor: annotation.PhysicalAnchor{Kind: annotation.AnchorBodyCentre},
			Position: annotation.PositionBound{Status: annotation.EvidenceObserved, XM: pfloat(450), YM: pfloat(0), BoundM: pfloat(0.2),
				Support: annotation.EvidenceSupport{Frames: []int{0}}},
			Yaw:    annotation.YawBound{Status: annotation.EvidenceUnknown, Axis: annotation.AxisUnknown},
			Front:  annotation.EndpointEvidence{Status: annotation.EvidenceUnknown},
			Rear:   annotation.EndpointEvidence{Status: annotation.EvidenceUnknown},
			Review: review,
		}},
	}}
	if err := annotation.SavePhysicalReferences(p, doc); err != nil {
		t.Fatal(err)
	}
}

// publishViewBeside publishes the set and the view into dataset/labels/stem.
func publishViewBeside(t *testing.T, packDir, setDir string) (view, pubSet string, report *ViewReport) {
	t.Helper()
	stem := filepath.Join(t.TempDir(), "dataset", "labels", "20251205T1417_80858-75c_kirkham-9th")
	pubSet = filepath.Join(stem, "kirk0-reference")
	if err := Publish(setDir, pubSet); err != nil {
		t.Fatal(err)
	}
	view = filepath.Join(stem, "pack-a")
	report, err := PublishView(packDir, pubSet, view)
	if err != nil {
		t.Fatalf("publish view: %v (%+v)", err, report)
	}
	return view, pubSet, report
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		sum := sha256.Sum256(b)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func changedLines(a, b []byte) int {
	x, y := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	if len(x) != len(y) {
		return -1
	}
	n := 0
	for i := range x {
		if !bytes.Equal(x[i], y[i]) {
			n++
		}
	}
	return n
}

// A published view carries the pack's data and physical references and
// nothing else, opens as the source does, and an edit through it changes
// exactly the label set's head line and one mask line: the files someone
// would commit. Once the editor re-pins the review the edit did not touch,
// a fresh copy of those files holds.
func TestPublishViewIsEditableAndCommitsSmall(t *testing.T) {
	packDir, setDir := viewSource(t)
	view, pubSet, report := publishViewBeside(t, packDir, setDir)

	sort.Strings(report.Omitted)
	if strings.Join(report.Omitted, ",") != ".annotations.lock,feature-proposals.pb,proposals" {
		t.Fatalf("omitted %v", report.Omitted)
	}
	for _, absent := range []string{annotation.FeatureProposalsFile, "proposals", ".annotations.lock"} {
		if _, err := os.Stat(filepath.Join(view, absent)); !os.IsNotExist(err) {
			t.Fatalf("the view carries %s", absent)
		}
	}
	if !strings.Contains(strings.Join(report.Tracked, ","), "pack-a/physical-references.json") || strings.Contains(strings.Join(report.Tracked, ","), ".publish-view-") {
		t.Fatalf("tracked %v", report.Tracked)
	}
	link, _ := annotation.ReadLabelSetLink(view)
	if link == nil || link.Path != "../kirk0-reference" {
		t.Fatalf("view link %+v", link)
	}
	if entries, _ := filepath.Glob(filepath.Join(filepath.Dir(view), ".publish-view-*")); len(entries) != 0 {
		t.Fatalf("staging left %v", entries)
	}
	src, _ := annotation.OpenPack(packDir)
	pub, err := annotation.OpenPack(view)
	if err != nil || pub.Manifest.PackDigest != src.Manifest.PackDigest {
		t.Fatalf("view pack %v", err)
	}
	was, _ := annotation.LoadSidecar(src)
	sc, err := annotation.LoadSidecar(pub)
	if err != nil || sc.Digest() != was.Digest() || len(sc.Masks) != len(was.Masks) {
		t.Fatalf("the view projects %+v (%v), the source %+v", sc, err, was)
	}

	// A researcher edits obj_a's mask through the view.
	viewBefore := hashTree(t, view)
	setBefore := headFiles(t, pubSet)
	for i := range sc.Masks {
		if sc.Masks[i].ObjectID == "obj_a" {
			sc.Masks[i].PointIndices = []int{0, 1, 2}
		}
	}
	if _, err := SaveMembership(pub, sc, Change{Author: "researcher", Operation: "save_mask"}); err != nil {
		t.Fatal(err)
	}
	if after := hashTree(t, view); len(after) != len(viewBefore) {
		t.Fatalf("a membership save wrote into the view: %v", after)
	} else {
		for name, h := range viewBefore {
			if after[name] != h {
				t.Fatalf("a membership save changed the view's %s", name)
			}
		}
	}
	setAfter := headFiles(t, pubSet)
	if len(setAfter) != len(setBefore) {
		t.Fatalf("a one-mask edit changed the head's files from %d to %d", len(setBefore), len(setAfter))
	}
	chunk := MasksDir + "/000000-000099.ndjson"
	for name, b := range setBefore {
		now := setAfter[name]
		switch name {
		case ManifestFile:
			if n := changedLines(b, now); n != 3 {
				t.Fatalf("labels.json changed %d lines, want the head's 3:\n%s", n, now)
			}
		case chunk:
			if n := changedLines(b, now); n != 1 {
				t.Fatalf("%s changed %d lines, want 1", chunk, n)
			}
		default:
			if !bytes.Equal(b, now) {
				t.Fatalf("a one-mask edit changed %s", name)
			}
		}
	}
	var local []string
	entries, _ := os.ReadDir(pubSet)
	for _, e := range entries {
		if e.Name() == JournalFile || e.Name() == JournalBaseDir || e.Name() == LockFile {
			local = append(local, e.Name())
		}
	}
	sort.Strings(local)
	if strings.Join(local, ",") != ".lock,journal-base,journal.ndjson" {
		t.Fatalf("the edit left %v beside the head files", local)
	}

	// The review rests on sample 0, which the edit changed for obj_a but not
	// obj_b: a fresh copy cannot rebuild the pinned revision until the
	// editor re-pins it.
	if r, err := CheckView(view); err == nil || !strings.Contains(strings.Join(r.Problems, "\n"), "no longer retained") {
		t.Fatalf("an unpinned edit passed the fresh-copy check: %v %+v", err, r)
	}
	n, drifted, err := annotation.RepinUnchangedReviews(pub, "researcher", "")
	if err != nil || n != 1 || drifted != 0 {
		t.Fatalf("re-pin: %d re-pinned, %d drifted, %v", n, drifted, err)
	}
	if r, err := CheckView(view); err != nil {
		t.Fatalf("after the re-pin: %v %+v", err, r)
	}
}

// A path that holds nothing is named as missing, with where it was looked
// for, not reported as a pack without a label set.
func TestCheckViewNamesAMissingPack(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := CheckView(filepath.Join("labels", "no-such-view"))
	if !errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "not linked") {
		t.Fatalf("CheckView of a missing directory: %v", err)
	}
	wd, _ := os.Getwd()
	if !strings.Contains(err.Error(), filepath.Join(wd, "labels", "no-such-view")) {
		t.Fatalf("the error does not say where it looked: %v", err)
	}
}

// What a committer sees: git reports the label set's two edited files and,
// after the re-pin, the view's physical references, with every other file
// the editors wrote ignored by the dataset's rules.
func TestPublishedViewEditInGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	packDir, setDir := viewSource(t)
	view, pubSet, _ := publishViewBeside(t, packDir, setDir)
	dataset := filepath.Dir(filepath.Dir(filepath.Dir(view)))
	// The ignore rules tools/s2-archive/stage-label-sets.py writes; the LFS
	// filter is left out so the test does not need git-lfs.
	ignore := strings.Join([]string{
		"labels/**/journal.ndjson", "labels/**/journal-base/", "labels/**/.lock", "labels/**/*.tmp",
		"labels/**/.annotations.lock", "labels/**/physical-reference-revisions/", "labels/**/feature-proposal-revisions/",
		"labels/**/proposals/", "labels/**/.publish-view-*/",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dataset, ".gitignore"), []byte(ignore), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataset, ".gitattributes"), []byte("labels/** -text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dataset
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "publish")
	git("checkout", "-q", "-b", "researcher-edit")

	pub, _ := annotation.OpenPack(view)
	sc, _ := annotation.LoadSidecar(pub)
	for i := range sc.Masks {
		if sc.Masks[i].ObjectID == "obj_a" {
			sc.Masks[i].PointIndices = []int{0, 1, 2}
		}
	}
	if _, err := SaveMembership(pub, sc, Change{Author: "researcher", Operation: "save_mask"}); err != nil {
		t.Fatal(err)
	}
	rel := func(p string) string { r, _ := filepath.Rel(dataset, p); return filepath.ToSlash(r) }
	want := " M " + rel(filepath.Join(pubSet, ManifestFile)) + "\n M " + rel(filepath.Join(pubSet, MasksDir, "000000-000099.ndjson")) + "\n"
	if got := git("status", "--porcelain"); got != want {
		t.Fatalf("after the edit git reports\n%s\nwant\n%s", got, want)
	}
	if _, _, err := annotation.RepinUnchangedReviews(pub, "researcher", ""); err != nil {
		t.Fatal(err)
	}
	want += " M " + rel(filepath.Join(view, annotation.PhysicalReferencesFile)) + "\n"
	if got := git("status", "--porcelain"); got != want {
		t.Fatalf("after the re-pin git reports\n%s\nwant\n%s", got, want)
	}
}

func TestPublishViewRefuses(t *testing.T) {
	packDir, setDir := viewSource(t)
	stem := filepath.Join(t.TempDir(), "labels", "stem")
	pubSet := filepath.Join(stem, "kirk0-reference")
	if err := Publish(setDir, pubSet); err != nil {
		t.Fatal(err)
	}
	refused := func(name, pack, set, dest, want string) {
		t.Helper()
		if _, err := PublishView(pack, set, dest); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", name, err, want)
		}
		if entries, _ := filepath.Glob(filepath.Join(stem, ".publish-view-*")); len(entries) != 0 {
			t.Errorf("%s left %v", name, entries)
		}
	}

	refused("a view away from its set", packDir, pubSet, filepath.Join(t.TempDir(), "pack-a"), "beside its label set")
	if err := os.MkdirAll(filepath.Join(stem, "full"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stem, "full", "x"), nil, 0o644)
	refused("a non-empty destination", packDir, pubSet, filepath.Join(stem, "full"), "non-empty")

	other, _ := newSet(t)
	otherPub := filepath.Join(stem, "other")
	if err := Publish(other, otherPub); err != nil {
		t.Fatal(err)
	}
	refused("another label set", packDir, otherPub, filepath.Join(stem, "pack-a"), "is linked to label set")

	unknown := filepath.Join(packDir, "notes.txt")
	os.WriteFile(unknown, []byte("?"), 0o644)
	refused("an unknown file", packDir, pubSet, filepath.Join(stem, "pack-a"), "notes.txt")
	os.Remove(unknown)

	// Another run of the capture, linked but never attached: its foreground
	// is not a domain of the set.
	runB := writeTestPack(t, filepath.Join(filepath.Dir(packDir), "pack-b"), []uint64{9, 10, 11, 12, 13},
		[]testFrame{{1, 2, []uint64{405, 406, 850}}}, "run-b")
	ls, _ := Open(setDir)
	if err := annotation.WriteLabelSetLink(runB, ls.Manifest.LabelSetID, setDir); err != nil {
		t.Fatal(err)
	}
	refused("an unattached run", runB.Dir, pubSet, filepath.Join(stem, "pack-b"), "labels attach")

	// The source moves on: the published set is behind it. Republished, the
	// review stays pinned to the old head, which a fresh copy cannot rebuild.
	ls.State.Masks[0].Members = Union(ls.State.Masks[0].Members, Ranges{{ls.State.Masks[0].Members[0].Start + 2, 1}})
	if err := ls.Save(Change{Author: "dd", Operation: "save_mask"}); err != nil {
		t.Fatal(err)
	}
	refused("a published set behind the pack's", packDir, pubSet, filepath.Join(stem, "pack-a"), "republish the label set first")
	if err := Republish(setDir, pubSet); err != nil {
		t.Fatal(err)
	}
	r, err := PublishView(packDir, pubSet, filepath.Join(stem, "pack-a"))
	if err == nil || !strings.Contains(strings.Join(r.Problems, "\n"), "no longer retained") {
		t.Fatalf("a review pinned before the head was published as holding: %v %+v", err, r)
	}
	if _, err := os.Stat(filepath.Join(stem, "pack-a")); !os.IsNotExist(err) {
		t.Fatalf("a view that would not hold was put in place: %v", err)
	}
}
