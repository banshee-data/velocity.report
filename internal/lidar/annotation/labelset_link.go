package annotation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LabelSetLinkFile, in a version 2 pack's directory, names the label set that
// holds the pack's membership. It sits outside the pack digest: linking a
// pack to labels does not change its points.
const LabelSetLinkFile = "label-set-link.json"

// LabelSetLink is the content of LabelSetLinkFile. Path is relative to the
// pack directory.
type LabelSetLink struct {
	LabelSetID string `json:"label_set_id"`
	Path       string `json:"path"`
}

// ErrMembershipInLabelSet means a pack's membership lives in a label set and
// is saved through it, never as a sidecar in the pack.
var ErrMembershipInLabelSet = errors.New("this pack's membership is stored in a label set; save through the label set")

// MembershipProjector returns the version 1 sidecar encoding of a label
// set's membership as a linked pack shows it: point indices of that pack, at
// a label set revision (0 for the head). Package labelset registers one;
// annotation cannot import it.
type MembershipProjector func(p *Pack, link LabelSetLink, revision int) ([]byte, error)

var projector MembershipProjector

// RegisterMembershipProjector installs the label set projector. Package
// labelset calls it from init, so any binary that links labelset reads
// linked packs' membership through it.
func RegisterMembershipProjector(fn MembershipProjector) { projector = fn }

// ReadLabelSetLink returns the pack's label set link, or nil when it has none.
func ReadLabelSetLink(p *Pack) (*LabelSetLink, error) {
	b, err := os.ReadFile(filepath.Join(p.Dir, LabelSetLinkFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var link LabelSetLink
	if err := json.Unmarshal(b, &link); err != nil {
		return nil, fmt.Errorf("parse %s: %w", LabelSetLinkFile, err)
	}
	if link.Path == "" || link.LabelSetID == "" {
		return nil, fmt.Errorf("%s names no label set", LabelSetLinkFile)
	}
	return &link, nil
}

// WriteLabelSetLink links a version 2 pack to a label set directory.
func WriteLabelSetLink(p *Pack, labelSetID, labelSetDir string) error {
	if !p.HasReturns() {
		return fmt.Errorf("only a version 2 pack can be linked to a label set; %s is version %d", p.Dir, p.Manifest.SchemaVersion)
	}
	rel, err := filepath.Rel(p.Dir, labelSetDir)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(LabelSetLink{LabelSetID: labelSetID, Path: filepath.ToSlash(rel)}, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(p.Dir, LabelSetLinkFile+".tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(p.Dir, LabelSetLinkFile)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// projectedSidecar returns a linked pack's membership projected from its
// label set. linked is false when the pack has no link.
func projectedSidecar(p *Pack, revision int) (s *Sidecar, linked bool, err error) {
	link, err := ReadLabelSetLink(p)
	if err != nil {
		return nil, true, err
	}
	if link == nil {
		return nil, false, nil
	}
	if projector == nil {
		return nil, true, fmt.Errorf("pack %s keeps its membership in label set %s, and this binary was built without label set support", p.Dir, link.LabelSetID)
	}
	b, err := projector(p, *link, revision)
	if err != nil {
		return nil, true, fmt.Errorf("project label set %s: %w", link.LabelSetID, err)
	}
	s, err = decodeSidecar(p, b)
	if err != nil {
		return nil, true, fmt.Errorf("projected membership: %w", err)
	}
	if revision > 0 && s.Revision != revision {
		return nil, true, fmt.Errorf("label set projected revision %d for requested revision %d", s.Revision, revision)
	}
	return s, true, nil
}
