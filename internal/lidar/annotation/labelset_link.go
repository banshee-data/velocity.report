package annotation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LabelSetLinkFile, in a pack's directory, names the label set that holds the
// pack's membership. It sits outside the pack digest: linking a
// pack to labels does not change its points.
const LabelSetLinkFile = "label-set-link.json"

// LabelSetLink is the content of LabelSetLinkFile. Path is relative to the
// pack directory.
type LabelSetLink struct {
	LabelSetID string `json:"label_set_id"`
	Path       string `json:"path"`
}

// MembershipProjector returns the Sidecar encoding of a label set's
// membership as a linked pack shows it: point indices of that pack, at
// a label set revision (0 for the head). It also returns the label set's
// state digest at that revision, which is the projected sidecar's membership
// token: pins on a linked pack's membership (physical references, features,
// frozen splits) name the label set's state, as the macOS client does.
// Package labelset registers one; annotation cannot import it.
type MembershipProjector func(p *Pack, link LabelSetLink, revision int) (b []byte, stateDigest string, err error)

var projector MembershipProjector

// RegisterMembershipProjector installs the label set projector. Package
// labelset calls it from init, so any binary that links labelset reads
// linked packs' membership through it.
func RegisterMembershipProjector(fn MembershipProjector) { projector = fn }

// ReadLabelSetLink returns a pack directory's label set link, or nil when it
// has none.
func ReadLabelSetLink(packDir string) (*LabelSetLink, error) {
	b, err := os.ReadFile(filepath.Join(packDir, LabelSetLinkFile))
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

// LabelSetDir resolves a link to the label set's directory.
func (l LabelSetLink) LabelSetDir(packDir string) string {
	return filepath.Join(packDir, filepath.FromSlash(l.Path))
}

// WriteLabelSetLink links a pack to a label set directory.
func WriteLabelSetLink(p *Pack, labelSetID, labelSetDir string) error {
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
	link, err := ReadLabelSetLink(p.Dir)
	if err != nil {
		return nil, true, err
	}
	if link == nil {
		return nil, false, nil
	}
	if projector == nil {
		return nil, true, fmt.Errorf("pack %s keeps its membership in label set %s, and this binary was built without label set support", p.Dir, link.LabelSetID)
	}
	b, stateDigest, err := projector(p, *link, revision)
	if err != nil {
		return nil, true, fmt.Errorf("project label set %s: %w", link.LabelSetID, err)
	}
	s, err = decodeSidecar(p, b)
	if err != nil {
		return nil, true, fmt.Errorf("projected membership: %w", err)
	}
	if stateDigest == "" {
		return nil, true, fmt.Errorf("label set %s projected no state digest", link.LabelSetID)
	}
	s.baseDigest = stateDigest
	if revision > 0 && s.Revision != revision {
		return nil, true, fmt.Errorf("label set projected revision %d for requested revision %d", s.Revision, revision)
	}
	return s, true, nil
}

// LabelSetCaptures returns the capture files a linked pack's label set has
// verified hold its packets, by SHA-256, or nil for a pack with no link. A
// split freezing the pack binds its case to these, so a replay is checked by
// content whichever verified file it reads.
func LabelSetCaptures(packDir string) ([]CaseCapture, error) {
	link, err := ReadLabelSetLink(packDir)
	if err != nil || link == nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(link.LabelSetDir(packDir), "labels.json"))
	if err != nil {
		return nil, fmt.Errorf("read label set manifest: %w", err)
	}
	var m struct {
		LabelSetID string `json:"label_set_id"`
		AppliesTo  []struct {
			Name   string `json:"name"`
			SHA256 string `json:"sha256"`
		} `json:"applies_to"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse label set manifest: %w", err)
	}
	if m.LabelSetID != link.LabelSetID {
		return nil, fmt.Errorf("pack links label set %s, the directory holds %s", link.LabelSetID, m.LabelSetID)
	}
	var out []CaseCapture
	for _, a := range m.AppliesTo {
		out = append(out, CaseCapture{Basename: a.Name, SHA256: a.SHA256})
	}
	sortCaptures(out)
	return out, nil
}
