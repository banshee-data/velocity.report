package labelset

import (
	"fmt"
	"os"
	"time"
)

// Merging label sets in Git.
//
// The head files are plain text, one record per line, so Git merges two
// branches that edited different masks line by line. What it cannot merge is
// labels.json's head, which each branch moved to its own revision and digest.
// Rehead takes the files on disk, whichever branch's labels.json was kept, and
// makes them the head.

// ReheadResult says what Rehead did.
type ReheadResult struct {
	From Head `json:"from"`
	To   Head `json:"to"`
	// Revised is true when the files held another state, which is now a new
	// revision; Relaid when they held the head's state in another layout.
	Revised bool `json:"revised"`
	Relaid  bool `json:"relaid"`
}

// Rehead makes the head files on disk the label set's head: what a Git merge,
// or a hand-resolved conflict, leaves behind. The files must decode, in the
// declared layout, to a valid state; Validate is what catches two branches
// claiming one return for different objects. A state other than the head's
// becomes the next revision. A local journal gets one line from its tip to
// that state, so its history replays through the merge; a copy without one
// just has a new head. The files are rewritten in their exact layout. Nothing
// is written when they already are the head.
func Rehead(dir string, change Change) (*ReheadResult, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	lock, err := lockSet(root)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	m, err := readManifest(root)
	if err != nil {
		return nil, fmt.Errorf("%w (after a merge conflict, keep either branch's %s: rehead rewrites its head)", err, ManifestFile)
	}
	s, have, err := readStorage(root, ".", m.Storage)
	if err != nil {
		return nil, fmt.Errorf("the head files do not read as a label set (resolve any merge conflict in them first): %w", err)
	}
	s.Canonicalise()
	if err := s.Validate(m.Layout); err != nil {
		return nil, fmt.Errorf("the merged head is not a valid label set: %w", err)
	}
	digest, err := DigestOf(s)
	if err != nil {
		return nil, err
	}
	files, err := encodeStorage(s, m.Storage)
	if err != nil {
		return nil, err
	}
	out := &ReheadResult{From: m.Head, To: m.Head}
	if digest == m.Head.State {
		if checkLayout(s, m.Storage, have) == nil {
			return out, nil
		}
		out.Relaid = true
		return out, writeHead(root, ".", files, have)
	}

	tip, _, err := journalTip(root)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if change.Operation == "" {
		change.Operation = "rehead"
	}
	change.CreatedUTC = now
	revision := m.Head.Revision + 1
	if tip != nil {
		// The journal is this copy's own history, so labels.json must be the
		// one this copy last wrote: in a merge, keep your own branch's.
		if tip.Revision != m.Head.Revision || tip.State != m.Head.State {
			return nil, fmt.Errorf("the local journal ends at revision %d and labels.json says %d: keep this copy's own %s (git checkout --ours), or open the label set if a save was interrupted",
				tip.Revision, m.Head.Revision, ManifestFile)
		}
		base, err := LoadRevision(dir, tip.Revision)
		if err != nil {
			return nil, fmt.Errorf("rebuild the journal's tip, revision %d: %w", tip.Revision, err)
		}
		delta, err := Diff(base, s)
		if err != nil {
			return nil, err
		}
		delta.Revision, delta.ParentRevision, delta.ParentState, delta.State = revision, tip.Revision, tip.State, digest
		delta.Change = change
		line, err := canonicalLine(delta)
		if err != nil {
			return nil, err
		}
		if err := markPending(root, revision, tip.State); err != nil {
			return nil, err
		}
		if err := appendJournal(root, line); err != nil {
			return nil, fmt.Errorf("append journal: %w", err)
		}
	}
	if err := writeHead(root, ".", files, have); err != nil {
		return nil, err
	}
	m.Head = Head{Revision: revision, State: digest, UpdatedUTC: now}
	if err := writeManifest(root, m); err != nil {
		return nil, err
	}
	if err := clearPending(root); err != nil {
		return nil, err
	}
	out.To, out.Revised = m.Head, true
	return out, nil
}
