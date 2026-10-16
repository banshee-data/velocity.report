package labelset

import (
	"errors"
	"fmt"
	"os"
	"reflect"
)

// Finishing a Git merge of a label set.
//
// Git merges a label set's head files line by line: two branches that
// edited different masks merge cleanly. labels.json cannot merge that way,
// since each branch moved its head to its own revision and digest, and the
// merged head is a state neither branch made. A merge driver for labels.json
// (MergeManifests) keeps this branch's head and records the merge in it;
// FinishMerge, run from the hooks before Git commits the merge, makes the
// merged files the next revision and clears the record. A set holding a
// record opens for nothing else, so a merge that skipped the hooks (a
// rebase, a clone without them) is refused rather than read as a head it is
// not.

// ErrUnfinishedMerge means labels.json records a Git merge whose head files
// are not a revision yet.
var ErrUnfinishedMerge = errors.New("the label set is in the middle of a Git merge")

// MergeRecord is what the merge driver leaves in labels.json: the heads of
// the merge base and of the branch merged in. This branch's head stays in
// head.
type MergeRecord struct {
	Base   Head `json:"base"`
	Theirs Head `json:"theirs"`
}

// MergeManifests merges three versions of labels.json, as a Git merge driver
// does: the merge base's, this branch's and the other branch's. The result is
// this branch's manifest with a merge record when both moved their heads, the
// other branch's bound files added, and any other field the other branch
// alone changed taken from it. conflict is true when the two cannot be one
// label set: different sets, layouts or storage, or a side still mid-merge.
// base may be empty, for a file both branches added.
func MergeManifests(base, ours, theirs []byte) (merged []byte, conflict string, err error) {
	parse := func(b []byte, which string) (*Manifest, error) {
		var m Manifest
		if err := strictUnmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s %s: %w", which, ManifestFile, err)
		}
		return &m, nil
	}
	o, err := parse(ours, "this branch's")
	if err != nil {
		return nil, "", err
	}
	t, err := parse(theirs, "the other branch's")
	if err != nil {
		return nil, "", err
	}
	b := &Manifest{}
	if len(base) > 0 {
		if b, err = parse(base, "the merge base's"); err != nil {
			return nil, "", err
		}
	}
	switch {
	case o.Merge != nil || t.Merge != nil:
		return nil, "a side is itself an unfinished merge: finish it first", nil
	case o.LabelSetID != t.LabelSetID:
		return nil, fmt.Sprintf("the branches hold different label sets (%s, %s)", o.LabelSetID, t.LabelSetID), nil
	case o.Layout != t.Layout || o.PacketKey != t.PacketKey || o.SchemaVersion != t.SchemaVersion:
		return nil, "the branches hold the label set in different layouts or versions", nil
	case o.Storage != t.Storage:
		return nil, "the branches store the head in different chunk spans: relayout one to match", nil
	}
	m := *o
	// A field only the other branch changed is the other branch's.
	pick := func(mine, other, was any, set func()) {
		if reflect.DeepEqual(mine, was) && !reflect.DeepEqual(other, was) {
			set()
		}
	}
	pick(o.Name, t.Name, b.Name, func() { m.Name = t.Name })
	pick(o.Note, t.Note, b.Note, func() { m.Note = t.Note })
	pick(o.MigratedFrom, t.MigratedFrom, b.MigratedFrom, func() { m.MigratedFrom = t.MigratedFrom })
	if t.MinReader > m.MinReader {
		m.MinReader = t.MinReader
	}
	// A file either branch verified stays verified.
	have := map[[2]string]bool{}
	for _, a := range m.AppliesTo {
		have[[2]string{a.Name, a.SHA256}] = true
	}
	for _, a := range t.AppliesTo {
		if !have[[2]string{a.Name, a.SHA256}] {
			m.AppliesTo = append(m.AppliesTo, a)
		}
	}
	switch {
	case o.Head == t.Head:
	case o.Head == b.Head:
		// Only the other branch moved: its head is the files' head, and Git
		// takes its head files as they are.
		m.Head = t.Head
	case t.Head == b.Head:
	default:
		m.Merge = &MergeRecord{Base: b.Head, Theirs: t.Head}
	}
	out, err := encodeManifest(&m)
	if err != nil {
		return nil, "", err
	}
	return out, "", nil
}

// FinishMerge makes a merged set's head files its next revision, as Rehead
// does, and clears the merge record. It returns nil with nothing written
// when the set records no merge.
func FinishMerge(dir string, change Change) (*ReheadResult, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	m, err := readManifestOfMerge(root)
	if err != nil {
		root.Close()
		return nil, err
	}
	if m.Merge == nil {
		root.Close()
		return nil, nil
	}
	lock, err := lockSet(root)
	if err != nil {
		root.Close()
		return nil, err
	}
	record := m.Merge
	m.Merge = nil
	err = writeManifest(root, m)
	lock.Close()
	root.Close()
	if err != nil {
		return nil, err
	}
	if change.Operation == "" {
		change.Operation = "merge"
	}
	result, err := Rehead(dir, change)
	if err != nil {
		// The merge is not finished: say so again, so nothing reads the
		// merged files as a head.
		if root, rerr := os.OpenRoot(dir); rerr == nil {
			if back, rerr := readManifest(root); rerr == nil {
				back.Merge = record
				_ = writeManifest(root, back)
			}
			root.Close()
		}
		return nil, err
	}
	return result, nil
}
