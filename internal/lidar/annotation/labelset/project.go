package labelset

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

func init() {
	annotation.RegisterMembershipProjector(func(p *annotation.Pack, link annotation.LabelSetLink, revision int) ([]byte, string, error) {
		return ProjectAt(link.LabelSetDir(p.Dir), link.LabelSetID, p, revision)
	})
}

// ProjectAt renders a label set's membership at a revision (0 for the head)
// in the pack's own terms: the masks of the windows the pack shows, in its
// point indices, encoded as the annotation package's Sidecar. Consumers of a
// pack's membership (splits, physical references, features, evaluators) read
// it this way.
// It returns the label set's state digest at that revision too: the token
// that pins a linked pack's membership, the same whichever pack shows it.
func ProjectAt(dir, labelSetID string, p *annotation.Pack, revision int) ([]byte, string, error) {
	ls, err := Open(dir)
	if err != nil {
		return nil, "", err
	}
	if labelSetID != "" && ls.Manifest.LabelSetID != labelSetID {
		return nil, "", fmt.Errorf("%s holds label set %s, the pack links %s", dir, ls.Manifest.LabelSetID, labelSetID)
	}
	state, change, rev, digest := &ls.State, Change{}, ls.Revision(), ls.StateDigest()
	if revision > 0 && revision != ls.Revision() {
		old, d, err := revisionWithDelta(dir, revision)
		if err != nil {
			return nil, "", err
		}
		state, change, rev, digest = old, d.Change, revision, d.State
	} else if tip, err := tipOf(dir); err == nil && tip != nil {
		change = tip.Change
	}
	b, err := Project(ls.Manifest.Layout, state, rev, change, p)
	return b, digest, err
}

func tipOf(dir string) (*Delta, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	d, _, err := journalTip(root)
	return d, err
}

func revisionWithDelta(dir string, revision int) (*State, *Delta, error) {
	var out *State
	var delta *Delta
	err := Replay(dir, func(rev int, s *State, d *Delta) (bool, error) {
		if rev == revision {
			c := s.Clone()
			out, delta = &c, d
			return false, nil
		}
		return rev < revision, nil
	})
	if err != nil {
		return nil, nil, err
	}
	if out == nil {
		return nil, nil, fmt.Errorf("revision %d: %w", revision, ErrHistoryNotRetained)
	}
	if got, err := DigestOf(out); err != nil || got != delta.State {
		return nil, nil, fmt.Errorf("revision %d rebuilds to %s, journal says %s (%v)", revision, got, delta.State, err)
	}
	return out, delta, nil
}

// Project renders a state's membership as a pack shows it. Masks whose window
// the pack does not show are left out, as are members it does not show; the
// label set keeps both.
func Project(layout Layout, s *State, revision int, change Change, p *annotation.Pack) ([]byte, error) {
	vm, err := NewViewMap(layout, s, p)
	if err != nil {
		return nil, err
	}
	sc := annotation.Sidecar{
		SchemaVersion: annotation.SidecarSchemaVersion,
		DatasetID:     p.Manifest.DatasetID,
		PackDigest:    p.Manifest.PackDigest,
		Revision:      revision,
		UpdatedUTC:    change.CreatedUTC,
		Change: annotation.Provenance{
			Author: change.Author, Session: change.Session, CreatedUTC: change.CreatedUTC,
			Operation: change.Operation, Revision: revision,
		},
		Objects:            append([]annotation.Object{}, s.Records.Objects...),
		Masks:              []annotation.FrameMask{},
		Correspondences:    append([]annotation.TrackCorrespondence(nil), s.Records.Correspondences...),
		DismissedProposals: append([]string(nil), s.Records.DismissedProposals...),
	}
	if revision > 1 {
		sc.Change.ParentRev = revision - 1
	}
	for _, m := range s.Masks {
		sample, ok := vm.WindowSample(m.WindowID)
		if !ok {
			continue
		}
		members, _ := vm.Shown(sample, m.Members)
		uncertain, _ := vm.Shown(sample, m.Uncertain)
		if members == nil {
			members = []int{}
		}
		sc.Masks = append(sc.Masks, annotation.FrameMask{
			ObjectID: m.ObjectID, SampleID: sample, PointIndices: members, UncertainIndices: uncertain,
			Completeness: m.Completeness, Visibility: m.Visibility, Status: m.Status, Pose: m.Pose,
			Provenance: m.Provenance,
		})
	}
	sc.Canonicalise()
	b, err := json.MarshalIndent(&sc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
