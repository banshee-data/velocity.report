package annotation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Objects and following records merge by id: what one branch changed is
// that branch's, and a record both changed differently is a conflict.
func TestMergePhysicalReferences(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	if err := SavePhysicalReferences(p, validPhysical(p)); err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile(filepath.Join(p.Dir, PhysicalReferencesFile))
	if err != nil {
		t.Fatal(err)
	}
	edit := func(object string, bound float64) []byte {
		t.Helper()
		doc, err := LoadPhysicalReferences(p)
		if err != nil {
			t.Fatal(err)
		}
		for i := range doc.Objects {
			if doc.Objects[i].ObjectID == object {
				doc.Objects[i].Keyframes[0].Position.BoundM = &bound
			}
		}
		next := *doc
		next.Revision++
		b, err := encodePhysicalForTest(&next)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	ours, theirs := edit("car-1", 0.31), edit("car-2", 0.42)
	merged, conflicts, err := MergePhysicalReferences(base, ours, theirs)
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("%v %v", err, conflicts)
	}
	var m PhysicalReferenceSet
	if err := decodePhysicalJSON(merged, &m, "merged"); err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, o := range m.Objects {
		got[o.ObjectID] = *o.Keyframes[0].Position.BoundM
	}
	if got["car-1"] != 0.31 || got["car-2"] != 0.42 {
		t.Fatalf("merged bounds %v", got)
	}

	_, conflicts, err = MergePhysicalReferences(base, ours, edit("car-1", 0.5))
	if err != nil || len(conflicts) != 1 || !strings.Contains(conflicts[0], "car-1") {
		t.Fatalf("a record both changed: %v %v", err, conflicts)
	}

	// This branch deleted car-1 and the other changed it: a conflict, with
	// the other's car-1 standing and no empty record written.
	doc, _ := LoadPhysicalReferences(p)
	deleted := *doc
	deleted.Objects = nil
	for _, o := range doc.Objects {
		if o.ObjectID != "car-1" {
			deleted.Objects = append(deleted.Objects, o)
		}
	}
	gone, err := encodePhysicalForTest(&deleted)
	if err != nil {
		t.Fatal(err)
	}
	merged, conflicts, err = MergePhysicalReferences(base, gone, ours)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("delete against change: %v %v", err, conflicts)
	}
	var d PhysicalReferenceSet
	if err := decodePhysicalJSON(merged, &d, "merged"); err != nil {
		t.Fatal(err)
	}
	for _, o := range d.Objects {
		if o.ObjectID == "" {
			t.Fatalf("an empty record was written: %+v", d.Objects)
		}
	}
	if len(d.Objects) != len(doc.Objects) {
		t.Fatalf("merged %d objects, want %d", len(d.Objects), len(doc.Objects))
	}
}

// encodePhysicalForTest encodes a document as the store writes it.
func encodePhysicalForTest(r *PhysicalReferenceSet) ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	return append(b, '\n'), err
}
