package labelset

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/google/uuid"
)

// SaveMembership writes a pack's membership, given in the pack's own samples
// and point indices, into the label set the pack is linked to. A pack not
// linked yet gets a label set of its own beside it, in
// <pack parent>/labels/<name>, and a link to it. It is the Go counterpart of
// the macOS window's save.
//
// sc is the whole membership as this pack shows it. What the pack does not
// show is kept: masks of windows it has no sample for, and members of a mask
// that only another run's foreground holds. A change that would lose such
// labels (deleting a mask or an object that still has them) is refused. A
// label set created here holds the membership as its first revision.
func SaveMembership(p *annotation.Pack, sc *annotation.Sidecar, change Change) (*LabelSet, error) {
	link, err := annotation.ReadLabelSetLink(p.Dir)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return createForPack(p, sc, change)
	}
	ls, err := Open(link.LabelSetDir(p.Dir))
	if err != nil {
		return nil, err
	}
	if ls.Manifest.LabelSetID != link.LabelSetID {
		return nil, fmt.Errorf("pack links label set %s, the directory holds %s", link.LabelSetID, ls.Manifest.LabelSetID)
	}
	next, err := applyMembership(ls.Manifest.Layout, &ls.State, p, sc)
	if err != nil {
		return nil, err
	}
	ls.State = *next
	if err := ls.Save(change); err != nil {
		return nil, err
	}
	return ls, nil
}

// createForPack starts a label set from the pack holding sc, and links the
// pack to it.
func createForPack(p *annotation.Pack, sc *annotation.Sidecar, change Change) (*LabelSet, error) {
	layout, start, err := NewFromPack(p)
	if err != nil {
		return nil, err
	}
	state, err := applyMembership(layout, &start, p, sc)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	stem := strings.TrimSuffix(p.Manifest.Source.PCAPBasename, filepath.Ext(p.Manifest.Source.PCAPBasename))
	if stem == "" {
		stem = "pack"
	}
	name := stem + "-" + id[:8]
	dir := filepath.Join(filepath.Dir(filepath.Clean(p.Dir)), "labels", name)
	ls, err := Create(dir, Manifest{LabelSetID: id, Name: name, Layout: layout}, *state, change)
	if err != nil {
		return nil, err
	}
	if err := annotation.WriteLabelSetLink(p, id, dir); err != nil {
		return nil, err
	}
	return ls, nil
}

// applyMembership folds a pack's membership into a label set state.
func applyMembership(layout Layout, base *State, p *annotation.Pack, sc *annotation.Sidecar) (*State, error) {
	next := base.Clone()
	domain := DomainOf(p)
	if !hasDomain(&next, domain.DomainID) {
		if _, err := AddPackDomain(layout, &next, p); err != nil {
			return nil, err
		}
	}
	vm, err := NewViewMap(layout, &next, p)
	if err != nil {
		return nil, err
	}
	next.Records = Records{
		Objects:            append([]annotation.Object{}, sc.Objects...),
		Correspondences:    append([]annotation.TrackCorrespondence{}, sc.Correspondences...),
		DismissedProposals: append([]string{}, sc.DismissedProposals...),
	}

	type key struct {
		object string
		window int
	}
	given := make(map[key]annotation.FrameMask, len(sc.Masks))
	for _, m := range sc.Masks {
		window, ok := vm.SampleWindow(m.SampleID)
		if !ok {
			return nil, fmt.Errorf("sample %d of pack %s is not a window of the label set", m.SampleID, p.Dir)
		}
		given[key{m.ObjectID, window}] = m
	}

	kept := next.Masks[:0:0]
	byKey := make(map[key]Mask, len(next.Masks))
	for _, m := range next.Masks {
		k := key{m.ObjectID, m.WindowID}
		sample, shown := vm.WindowSample(m.WindowID)
		if _, ok := given[k]; ok || !shown {
			byKey[k] = m
			if !shown {
				kept = append(kept, m)
			}
			continue
		}
		// The pack shows this window and no longer holds the mask: drop it,
		// unless it holds returns the pack does not show.
		_, hiddenMembers := vm.Shown(sample, m.Members)
		_, hiddenUncertain := vm.Shown(sample, m.Uncertain)
		if hiddenMembers+hiddenUncertain > 0 {
			return nil, fmt.Errorf("deleting %s in window %d would lose %d returns this pack does not show",
				m.ObjectID, m.WindowID, hiddenMembers+hiddenUncertain)
		}
	}
	for k, fm := range given {
		sample := fm.SampleID
		prev := byKey[k]
		members, err := vm.Edit(sample, prev.Members, fm.PointIndices)
		if err != nil {
			return nil, err
		}
		uncertain, err := vm.Edit(sample, prev.Uncertain, fm.UncertainIndices)
		if err != nil {
			return nil, err
		}
		m := Mask{ObjectID: k.object, WindowID: k.window, Members: members, Uncertain: uncertain,
			Completeness: fm.Completeness, Visibility: fm.Visibility, Status: fm.Status, Pose: fm.Pose, Provenance: fm.Provenance}
		// A mask keeps its domain while that still holds every return it
		// cites; otherwise it moves to this pack's foreground. One that would
		// mix two runs' foregrounds is refused by validation.
		m.DomainID = domain.DomainID
		if prev.DomainID != "" && covers(&next, prev.DomainID, k.window, Union(members, uncertain)) {
			m.DomainID = prev.DomainID
		}
		kept = append(kept, m)
	}
	next.Masks = kept
	next.Canonicalise()
	if err := next.Validate(layout); err != nil {
		return nil, fmt.Errorf("membership of pack %s does not fit its label set: %w", p.Dir, err)
	}
	return &next, nil
}

func hasDomain(s *State, id string) bool {
	for _, d := range s.Domains {
		if d.DomainID == id {
			return true
		}
	}
	return false
}

func covers(s *State, domain string, window int, set Ranges) bool {
	for _, d := range s.DomainReturns {
		if d.DomainID == domain && d.WindowID == window {
			return len(Difference(set, d.Returns)) == 0
		}
	}
	return false
}
