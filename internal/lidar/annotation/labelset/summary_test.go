package labelset

import (
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

func TestSummariseNamesWhatChanged(t *testing.T) {
	a := testState()
	b := a.Clone()
	if s := Summarise(&a, &b); !s.Empty() || !strings.Contains(s.Text(), "no label changed") {
		t.Fatalf("an unchanged state: %+v\n%s", s, s.Text())
	}
	// obj_a's window 0 mask gains a return and loses none; obj_b becomes a
	// cyclist; obj_c is new, with a mask in window 1.
	for i := range b.Masks {
		if b.Masks[i].ObjectID == "obj_a" && b.Masks[i].WindowID == 0 {
			b.Masks[i].Members = Union(b.Masks[i].Members, Ranges{{40, 1}})
		}
	}
	for i := range b.Records.Objects {
		if b.Records.Objects[i].ObjectID == "obj_b" {
			b.Records.Objects[i].Class = "cyclist"
		}
	}
	b.Records.Objects = append(b.Records.Objects, annotation.Object{ObjectID: "obj_c", Class: "pedestrian", Status: annotation.StatusProposed})
	added := b.Masks[0]
	added.ObjectID, added.WindowID = "obj_c", 1
	b.Masks = append(b.Masks, added)

	s := Summarise(&a, &b)
	s.Name, s.From, s.To = "kirk0-reference", Head{Revision: 7}, Head{Revision: 8}
	if len(s.ObjectsAdded) != 1 || s.ObjectsAdded[0].ObjectID != "obj_c" ||
		len(s.ObjectsChanged) != 1 || strings.Join(s.ObjectsChanged[0].Fields, ",") != "class" ||
		len(s.MasksAdded) != 1 || len(s.MasksChanged) != 1 || s.MasksChanged[0].Gained != 1 || s.MasksChanged[0].Lost != 0 {
		t.Fatalf("summary %+v", s)
	}
	md := s.Markdown()
	for _, want := range []string{
		"### kirk0-reference: revision 7 → 8",
		"| Objects | 1 | 0 | 1 |",
		"| Masks   | 1 | 0 | 1 |",
		"- obj_c (pedestrian)",
		"- obj_b (cyclist): class",
		"- obj_a: 1 mask changed, window 0 (+1 −0 returns)",
		"- obj_c: 1 mask added, window 1",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	if !strings.Contains(s.Text(), "Masks changed:\n  obj_a: 1 mask changed") {
		t.Errorf("text:\n%s", s.Text())
	}
}

func TestMergeManifests(t *testing.T) {
	_, ls := newSet(t)
	base := ls.Manifest
	enc := func(m Manifest) []byte {
		b, err := encodeManifest(&m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	ours, theirs := base, base
	ours.Head = Head{Revision: 2, State: "sha256:ours"}
	theirs.Head = Head{Revision: 2, State: "sha256:theirs"}
	theirs.AppliesTo = []Binding{{Name: "kirk0.pcapng", SHA256: "sha256:file"}}
	theirs.Note = "bound on the other branch"

	out, conflict, err := MergeManifests(enc(base), enc(ours), enc(theirs))
	if err != nil || conflict != "" {
		t.Fatalf("%v %q", err, conflict)
	}
	var m Manifest
	if err := strictUnmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m.Head != ours.Head || m.Merge == nil || m.Merge.Theirs != theirs.Head || m.Merge.Base != base.Head ||
		len(m.AppliesTo) != 1 || m.Note != theirs.Note {
		t.Fatalf("merged %+v", m)
	}

	// Only the other branch moved: its head, and no merge to finish.
	out, _, _ = MergeManifests(enc(base), enc(base), enc(theirs))
	var one Manifest
	if err := strictUnmarshal(out, &one); err != nil || one.Head != theirs.Head || one.Merge != nil {
		t.Fatalf("one side moved: %+v %v", one, err)
	}

	other := theirs
	other.LabelSetID = "another"
	if _, conflict, _ := MergeManifests(enc(base), enc(ours), enc(other)); conflict == "" {
		t.Fatal("two label sets merged")
	}
	chunked := theirs
	chunked.Storage.WindowChunk = 50
	if _, conflict, _ := MergeManifests(enc(base), enc(ours), enc(chunked)); conflict == "" {
		t.Fatal("two chunk spans merged")
	}
}
