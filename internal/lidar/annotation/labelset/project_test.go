package labelset

import (
	"errors"
	"path/filepath"
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
	if err := annotation.SaveSidecar(pack, head); !errors.Is(err, annotation.ErrMembershipInLabelSet) {
		t.Fatalf("sidecar save on a linked pack: %v", err)
	}
}
