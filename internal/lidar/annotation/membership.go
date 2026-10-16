package annotation

import (
	"encoding/json"
	"fmt"
)

// A pack's membership (its objects and per-sample masks, in the pack's point
// indices) lives in the label set the pack is linked to. LoadSidecar and
// LoadSidecarRevision read it projected onto the pack; nothing writes a
// sidecar file. A pack with no label set yet has no membership.

// LoadSidecar returns the pack's membership at its label set's head, or an
// empty document for a pack not yet linked to a label set.
func LoadSidecar(p *Pack) (*Sidecar, error) {
	if s, linked, err := projectedSidecar(p, 0); linked {
		return s, err
	}
	return NewSidecar(p), nil
}

// LoadSidecarRevision returns the pack's membership at a label set revision.
func LoadSidecarRevision(p *Pack, revision int) (*Sidecar, error) {
	if revision < 1 {
		return nil, fmt.Errorf("invalid requested revision %d", revision)
	}
	s, linked, err := projectedSidecar(p, revision)
	if !linked {
		return nil, fmt.Errorf("pack %s has no label set, so no membership revision %d", p.Dir, revision)
	}
	return s, err
}

// Digest is the membership token: the label set's state digest at this
// revision, which pins physical references, features and frozen splits.
// Empty for a pack with no label set.
func (s *Sidecar) Digest() string { return s.baseDigest }

func decodeSidecar(p *Pack, b []byte) (*Sidecar, error) {
	var s Sidecar
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse membership: %w", err)
	}
	if err := s.Validate(p); err != nil {
		return nil, err
	}
	s.baseDigest = sha256Hex(b)
	return &s, nil
}
