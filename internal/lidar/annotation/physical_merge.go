package annotation

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
)

// MergePhysicalReferences merges three versions of a view's
// physical-references.json, as a Git merge driver does: the merge base's,
// this branch's and the other branch's. Objects merge by object id and
// following records by following id: a record only one branch changed is
// that branch's, and a record both changed differently is a conflict, named
// in conflicts. The document's own fields (revision, time, change) are this
// branch's, and its reader level is the higher of the two. merged is this
// branch's version of any conflicting record. base may be empty, for a file
// both branches added.
func MergePhysicalReferences(base, ours, theirs []byte) (merged []byte, conflicts []string, err error) {
	parse := func(b []byte, which string) (*PhysicalReferenceSet, error) {
		var r PhysicalReferenceSet
		if err := decodePhysicalJSON(b, &r, which+" physical references"); err != nil {
			return nil, err
		}
		return &r, nil
	}
	o, err := parse(ours, "this branch's")
	if err != nil {
		return nil, nil, err
	}
	t, err := parse(theirs, "the other branch's")
	if err != nil {
		return nil, nil, err
	}
	b := &PhysicalReferenceSet{}
	if len(base) > 0 {
		if b, err = parse(base, "the merge base's"); err != nil {
			return nil, nil, err
		}
	}
	if o.Schema != t.Schema || o.SchemaVersion != t.SchemaVersion || o.DatasetID != t.DatasetID || o.PackDigest != t.PackDigest {
		return nil, []string{"the branches hold references to different packs or versions"}, nil
	}
	m := *o
	if !reflect.DeepEqual(o.Source, t.Source) {
		switch {
		case reflect.DeepEqual(o.Source, b.Source):
			m.Source = t.Source
		case !reflect.DeepEqual(t.Source, b.Source):
			conflicts = append(conflicts, "source")
		}
	}
	if t.MinReader > m.MinReader {
		m.MinReader = t.MinReader
	}

	key := func(v any) []byte { j, _ := json.Marshal(v); return j }
	objects := func(r *PhysicalReferenceSet) map[string]PhysicalObject {
		out := map[string]PhysicalObject{}
		for _, x := range r.Objects {
			out[x.ObjectID] = x
		}
		return out
	}
	ob, oo, ot := objects(b), objects(o), objects(t)
	m.Objects = nil
	for _, id := range sortedUnion(oo, ot, ob) {
		x, inO := oo[id]
		y, inT := ot[id]
		z, inB := ob[id]
		pick, keep, conflict := threeWay(inO, inT, inB, key(x), key(y), key(z))
		if conflict {
			conflicts = append(conflicts, "object "+id)
		}
		switch {
		case !keep:
		case pick == "theirs":
			m.Objects = append(m.Objects, y)
		default:
			m.Objects = append(m.Objects, x)
		}
	}
	following := func(r *PhysicalReferenceSet) map[string]FollowingReference {
		out := map[string]FollowingReference{}
		for _, x := range r.Following {
			out[x.FollowingID] = x
		}
		return out
	}
	fb, fo, ft := following(b), following(o), following(t)
	m.Following = nil
	for _, id := range sortedUnion(fo, ft, fb) {
		x, inO := fo[id]
		y, inT := ft[id]
		z, inB := fb[id]
		pick, keep, conflict := threeWay(inO, inT, inB, key(x), key(y), key(z))
		if conflict {
			conflicts = append(conflicts, "following "+id)
		}
		switch {
		case !keep:
		case pick == "theirs":
			m.Following = append(m.Following, y)
		default:
			m.Following = append(m.Following, x)
		}
	}
	// The origin ledger only ever grows; a record both ledgers name must
	// carry one origin.
	m.RecordOrigins = map[string]ReferenceOrigin{}
	for k, v := range o.RecordOrigins {
		m.RecordOrigins[k] = v
	}
	if err := mergeOrigins(m.RecordOrigins, t.RecordOrigins); err != nil {
		conflicts = append(conflicts, "record origins: "+err.Error())
	}
	m.canonicalise()
	out, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return append(out, '\n'), conflicts, nil
}

// threeWay decides one record: whether to keep it, from which side, and
// whether the two sides changed it differently.
func threeWay(inO, inT, inB bool, o, t, b []byte) (pick string, keep, conflict bool) {
	same := func(inX, inY bool, x, y []byte) bool { return inX == inY && (!inX || bytes.Equal(x, y)) }
	switch {
	case same(inO, inT, o, t):
		return "ours", inO, false
	case same(inO, inB, o, b):
		return "theirs", inT, false
	case same(inT, inB, t, b):
		return "ours", inO, false
	}
	return "ours", inO || inT, true
}

func sortedUnion[V any](maps ...map[string]V) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}
