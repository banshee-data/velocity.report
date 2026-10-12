package labelset

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

func TestLinkedPackReadsProjectedMembership(t *testing.T) {
	base := t.TempDir()
	pack := writeTestPack(t, filepath.Join(base, "pack-a"), []uint64{10, 11, 12, 13},
		[]testFrame{{0, 2, []uint64{5, 6, 7, 450}}, {2, 2, []uint64{800, 900}}}, "run-a")
	layout, state, err := NewFromPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewViewMap(layout, &state, pack)
	if err != nil {
		t.Fatal(err)
	}
	w0, _ := vm.SampleWindow(0)
	members, _ := vm.Offsets(0, []int{0, 1})
	state.Records.Objects = []annotation.Object{{ObjectID: "obj_a", Class: "car", Status: annotation.StatusReviewed, Confidence: 1}}
	state.Masks = []Mask{{ObjectID: "obj_a", WindowID: w0, DomainID: state.Domains[0].DomainID, Members: members,
		Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed}}
	dir := filepath.Join(base, "labels", "kirk0-reference")
	ls, err := Create(dir, Manifest{Name: "kirk0-reference", Layout: layout}, state, Change{Author: "dd", Operation: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if err := annotation.WriteLabelSetLink(pack, ls.Manifest.LabelSetID, dir); err != nil {
		t.Fatal(err)
	}

	sc, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Revision != 1 || len(sc.Masks) != 1 || sc.Masks[0].SampleID != 0 || len(sc.Masks[0].PointIndices) != 2 {
		t.Fatalf("projected %+v", sc)
	}
	first := sc.Digest()
	if first != ls.StateDigest() {
		t.Fatalf("a linked pack's membership token is %s, the label set's state is %s", first, ls.StateDigest())
	}

	// A save through the label set is a new projected revision; the old one
	// still projects to the same bytes.
	ls.State.Masks[0].Members = Union(ls.State.Masks[0].Members, Ranges{{450, 1}})
	if err := ls.Save(Change{Author: "dd", Operation: "save_mask"}); err != nil {
		t.Fatal(err)
	}
	head, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	if head.Revision != 2 || len(head.Masks[0].PointIndices) != 3 || head.Change.Author != "dd" || head.Digest() != ls.StateDigest() {
		t.Fatalf("head %+v", head)
	}
	r1, err := annotation.LoadSidecarRevision(pack, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Digest() != first {
		t.Fatalf("revision 1 projects to %s, first read %s", r1.Digest(), first)
	}
}

func TestSaveMembershipCreatesLinksAndKeepsHiddenLabels(t *testing.T) {
	base := t.TempDir()
	a := writeTestPack(t, filepath.Join(base, "a"), []uint64{10, 11, 12, 13},
		[]testFrame{{0, 2, []uint64{5, 6, 7, 450}}, {2, 2, []uint64{800, 900}}}, "run-a")

	// A pack with no label set has empty membership; its first save makes one
	// beside it and links it.
	sc, err := annotation.LoadSidecar(a)
	if err != nil || len(sc.Masks) != 0 || sc.Digest() != "" {
		t.Fatalf("unlinked pack membership %+v (%v)", sc, err)
	}
	sc.Objects = []annotation.Object{{ObjectID: "obj_a", Class: "car", Status: annotation.StatusReviewed}}
	sc.Masks = []annotation.FrameMask{{ObjectID: "obj_a", SampleID: 0, PointIndices: []int{0, 1, 2},
		Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed}}
	ls, err := SaveMembership(a, sc, Change{Author: "dd", Operation: "save_mask"})
	if err != nil {
		t.Fatal(err)
	}
	link, _ := annotation.ReadLabelSetLink(a.Dir)
	if link == nil || link.LabelSetID != ls.Manifest.LabelSetID || filepath.Dir(link.LabelSetDir(a.Dir)) != filepath.Join(base, "labels") {
		t.Fatalf("link %+v for set %s in %s", link, ls.Manifest.LabelSetID, ls.Dir)
	}
	if ls.Revision() != 1 {
		t.Fatalf("created with its membership at revision %d, want 1", ls.Revision())
	}
	back, err := annotation.LoadSidecar(a)
	if err != nil || len(back.Masks) != 1 || len(back.Masks[0].PointIndices) != 3 || back.Digest() != ls.StateDigest() {
		t.Fatalf("reloaded %+v (%v)", back, err)
	}

	// Another run of the capture, attached, edits the mask through its view:
	// it shows returns 5, 6 and 450 but not 7, which survives.
	b := writeTestPack(t, filepath.Join(base, "b"), []uint64{9, 10, 11, 12, 13},
		[]testFrame{{1, 2, []uint64{405, 406, 850}}, {3, 2, []uint64{1300}}}, "run-b")
	if err := annotation.WriteLabelSetLink(b, ls.Manifest.LabelSetID, ls.Dir); err != nil {
		t.Fatal(err)
	}
	viaB, err := annotation.LoadSidecar(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(viaB.Masks) != 1 || len(viaB.Masks[0].PointIndices) != 2 {
		t.Fatalf("run B sees %+v", viaB.Masks)
	}
	viaB.Masks[0].PointIndices = []int{0, 1, 2} // add return 450
	if _, err := SaveMembership(b, viaB, Change{Author: "dd", Operation: "save_mask"}); err != nil {
		t.Fatal(err)
	}
	ls, _ = Open(ls.Dir)
	if got := ls.State.Masks[0].Members; !got.Contains(7) || !got.Contains(450) || got.Count() != 4 {
		t.Fatalf("members after run B's edit %v", got)
	}

	// Deleting the mask through run B would lose return 7, which B does not
	// show: refused.
	viaB, _ = annotation.LoadSidecar(b)
	viaB.Masks = nil
	if _, err := SaveMembership(b, viaB, Change{Author: "dd"}); err == nil || !strings.Contains(err.Error(), "does not show") {
		t.Fatalf("deleting a mask with hidden returns: %v", err)
	}
}
