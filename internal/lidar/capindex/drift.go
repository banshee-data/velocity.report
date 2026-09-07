package capindex

import (
	"sort"
	"time"
)

// Indexed is what the index already holds about a file, as much of it as
// drift detection needs.
type Indexed struct {
	RelPath    string
	SizeBytes  int64
	ModifiedAt time.Time
	ContentTag string
	// Present is false for a file the index has seen before but that was gone
	// at the last scan.
	Present bool
}

// ChangeKind says how a file on disk differs from what the index holds.
type ChangeKind string

const (
	// ChangeResized means the file's length differs, which for a capture means
	// it was truncated or is still being written.
	ChangeResized ChangeKind = "resized"
	// ChangeRewritten means the length matches but the content tag does not:
	// the file was rewritten in place.
	ChangeRewritten ChangeKind = "rewritten"
	// ChangeTouched means only the modification time moved. Copying a volume
	// does this to every file, so it is reported separately from the two that
	// mean the data itself moved.
	ChangeTouched ChangeKind = "touched"
)

// Change is one file whose on-disk state no longer matches the index.
type Change struct {
	RelPath string
	Kind    ChangeKind
	// Was and Now describe the file before and after, for reporting.
	Was Indexed
	Now File
}

// Drift is the difference between a root's index and what a scan just found.
type Drift struct {
	// Added were not in the index, or were recorded as gone and are back.
	Added []File
	// Missing are indexed as present but were not found.
	Missing []Indexed
	// Changed were found but differ from what the index holds.
	Changed []Change
	// Unchanged is how many matched exactly, so a report can say "3 new of 195"
	// rather than just "3 new".
	Unchanged int
}

// Any reports whether the scan found anything to act on.
func (d Drift) Any() bool {
	return len(d.Added) > 0 || len(d.Missing) > 0 || len(d.Changed) > 0
}

// Total is how many files the scan saw.
func (d Drift) Total() int {
	return len(d.Added) + len(d.Changed) + d.Unchanged
}

// DiffScan compares what a scan found against what the index holds for the same
// root.
//
// Modification time alone is a weak signal — copying a volume moves every
// file's mtime without touching a byte — so a file whose size and content tag
// both still match is reported as merely touched rather than changed, and a
// caller can decide that costs nothing. A size or tag difference is the real
// thing and means any packet extent already probed is stale.
func DiffScan(indexed []Indexed, found []File) Drift {
	known := make(map[string]Indexed, len(indexed))
	for _, f := range indexed {
		known[f.RelPath] = f
	}
	seen := make(map[string]struct{}, len(found))

	var drift Drift
	for _, f := range found {
		seen[f.RelPath] = struct{}{}
		prev, ok := known[f.RelPath]
		if !ok || !prev.Present {
			// A file the index recorded as gone and that is back counts as
			// added: whatever was probed about it before cannot be trusted.
			drift.Added = append(drift.Added, f)
			continue
		}
		switch {
		case prev.SizeBytes != f.SizeBytes:
			drift.Changed = append(drift.Changed, Change{
				RelPath: f.RelPath, Kind: ChangeResized, Was: prev, Now: f})
		case prev.ContentTag != "" && f.ContentTag != "" && prev.ContentTag != f.ContentTag:
			drift.Changed = append(drift.Changed, Change{
				RelPath: f.RelPath, Kind: ChangeRewritten, Was: prev, Now: f})
		case !prev.ModifiedAt.Equal(f.ModifiedAt):
			drift.Changed = append(drift.Changed, Change{
				RelPath: f.RelPath, Kind: ChangeTouched, Was: prev, Now: f})
		default:
			drift.Unchanged++
		}
	}

	for _, f := range indexed {
		if !f.Present {
			continue
		}
		if _, ok := seen[f.RelPath]; !ok {
			drift.Missing = append(drift.Missing, f)
		}
	}

	sort.Slice(drift.Added, func(i, j int) bool { return drift.Added[i].RelPath < drift.Added[j].RelPath })
	sort.Slice(drift.Missing, func(i, j int) bool { return drift.Missing[i].RelPath < drift.Missing[j].RelPath })
	sort.Slice(drift.Changed, func(i, j int) bool { return drift.Changed[i].RelPath < drift.Changed[j].RelPath })
	return drift
}

// NeedsProbe reports whether a change invalidates a packet extent already
// probed. A file that was only touched keeps its extent; one whose bytes moved
// does not.
func (c Change) NeedsProbe() bool {
	return c.Kind != ChangeTouched
}
