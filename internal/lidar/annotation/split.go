package annotation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Object-disjoint dataset splits (annotation plan §7).
//
// A split manifest freezes which reference objects belong to which partition
// of one pack, and which frame intervals of that pack are scored as episodes.
// It is the thing that makes "held out" checkable rather than asserted: a
// score quoted as held-out names a manifest, the manifest names a pack digest,
// and an episode whose objects sit in a tuning partition is refused rather
// than scored.
//
// Split by object, not by frame. Adjacent frames of one vehicle are not
// independent examples, so every frame, propagated mask and revised identity
// of an object stays in the partition it was frozen into. An episode is a
// frame interval and the objects it scores; any other object present in those
// frames, from this partition or another, is uncertifiable for that episode
// and is ignored rather than scored. That is how a tuning object that happens
// to share frames with a held-out one stays out of the held-out number.
//
// The format is deliberately small: the plan defines partitions but no file,
// and a frozen contract that is easy to read by eye is worth more than one
// that anticipates every future need. Unknown fields are refused, so a
// manifest written for a later schema fails loudly instead of losing a field.

// SplitSchema names the manifest kind, so a file of some other kind is refused
// by name rather than half-parsed.
const SplitSchema = "velocity.report/annotation-split"

// SplitSchemaVersion is the manifest layout version.
const SplitSchemaVersion = 1

// MaxSplitManifestBytes bounds a manifest on read.
const MaxSplitManifestBytes = 16 << 20

// SplitRole says what a partition may be used for.
type SplitRole string

const (
	// SplitRoleTuning is free to tune against: the development partition of
	// the state-estimation plan's §16.4.
	SplitRoleTuning SplitRole = "tuning"
	// SplitRoleHeldOut is touched only to confirm a change. Tuning against it
	// invalidates it.
	SplitRoleHeldOut SplitRole = "held_out"
)

// ErrNotHeldOut marks a refusal to score a non-held-out split as held out.
var ErrNotHeldOut = errors.New("split is not held out")

// SplitManifest is one pack's frozen partition.
type SplitManifest struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	// PackDigest binds the manifest to one point domain. A manifest is never
	// applied to another pack, even one cut from the same recording.
	PackDigest string `json:"pack_digest"`
	// DatasetID, when set, must match the pack's.
	DatasetID string `json:"dataset_id,omitempty"`
	// SidecarRevision, when set, pins the annotation revision the split was
	// frozen against. Zero scores whatever revision is current, and the
	// revision used is recorded with every result either way.
	SidecarRevision int       `json:"sidecar_revision,omitempty"`
	Note            string    `json:"note,omitempty"`
	Splits          []Split   `json:"splits"`
	Episodes        []Episode `json:"episodes"`

	// Digest is the SHA-256 of the manifest file's bytes, set on load.
	Digest string `json:"-"`
}

// Split is one partition's objects.
type Split struct {
	Name      string    `json:"name"`
	Role      SplitRole `json:"role"`
	ObjectIDs []string  `json:"object_ids"`
}

// Episode is a stretch of the pack scored as one sequence.
type Episode struct {
	EpisodeID string `json:"episode_id"`
	// Split names the partition the episode belongs to. Every object it
	// scores must be in that partition.
	Split          string          `json:"split"`
	ObjectIDs      []string        `json:"object_ids"`
	FrameIntervals []FrameInterval `json:"frame_intervals"`
	Note           string          `json:"note,omitempty"`
}

// FrameInterval is an inclusive range of pack sample IDs.
type FrameInterval struct {
	FirstSample int `json:"first_sample"`
	LastSample  int `json:"last_sample"`
}

// Contains reports whether a sample is inside the interval.
func (f FrameInterval) Contains(sampleID int) bool {
	return sampleID >= f.FirstSample && sampleID <= f.LastSample
}

// ContainsSample reports whether a sample is inside any of the episode's
// intervals.
func (e Episode) ContainsSample(sampleID int) bool {
	for _, f := range e.FrameIntervals {
		if f.Contains(sampleID) {
			return true
		}
	}
	return false
}

// LoadSplitManifest reads and structurally validates a manifest. It does not
// know the pack; ValidateAgainst binds it to one.
func LoadSplitManifest(path string) (*SplitManifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open split manifest: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxSplitManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read split manifest: %w", err)
	}
	if len(b) > MaxSplitManifestBytes {
		return nil, fmt.Errorf("split manifest exceeds %d bytes", MaxSplitManifestBytes)
	}
	return ParseSplitManifest(b)
}

// ParseSplitManifest decodes and structurally validates manifest bytes.
func ParseSplitManifest(b []byte) (*SplitManifest, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var m SplitManifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse split manifest: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("parse split manifest: trailing data after the manifest object")
	}
	if err := m.validateStructure(); err != nil {
		return nil, fmt.Errorf("invalid split manifest: %w", err)
	}
	m.Digest = sha256Hex(b)
	return &m, nil
}

func (m *SplitManifest) validateStructure() error {
	if m.Schema != SplitSchema {
		return fmt.Errorf("schema %q, want %q", m.Schema, SplitSchema)
	}
	if m.SchemaVersion != SplitSchemaVersion {
		return fmt.Errorf("schema version %d, this build reads %d", m.SchemaVersion, SplitSchemaVersion)
	}
	if !strings.HasPrefix(m.PackDigest, "sha256:") {
		return fmt.Errorf("pack_digest %q is not a sha256: digest", m.PackDigest)
	}
	if m.SidecarRevision < 0 {
		return fmt.Errorf("sidecar_revision %d is negative", m.SidecarRevision)
	}
	if len(m.Splits) == 0 {
		return fmt.Errorf("no splits")
	}

	// Object-disjoint: an object belongs to exactly one partition.
	owner := map[string]string{}
	roles := map[string]SplitRole{}
	for i, s := range m.Splits {
		if s.Name == "" {
			return fmt.Errorf("split %d has no name", i)
		}
		if _, dup := roles[s.Name]; dup {
			return fmt.Errorf("split %q is declared twice", s.Name)
		}
		if s.Role != SplitRoleTuning && s.Role != SplitRoleHeldOut {
			return fmt.Errorf("split %q has role %q (want %q or %q)", s.Name, s.Role, SplitRoleTuning, SplitRoleHeldOut)
		}
		roles[s.Name] = s.Role
		if len(s.ObjectIDs) == 0 {
			return fmt.Errorf("split %q has no objects", s.Name)
		}
		for _, id := range s.ObjectIDs {
			if id == "" {
				return fmt.Errorf("split %q lists an empty object id", s.Name)
			}
			if other, taken := owner[id]; taken {
				if other == s.Name {
					return fmt.Errorf("split %q lists object %q twice", s.Name, id)
				}
				return fmt.Errorf("object %q is in both split %q and split %q: splits must be object-disjoint", id, other, s.Name)
			}
			owner[id] = s.Name
		}
	}

	if len(m.Episodes) == 0 {
		return fmt.Errorf("no episodes")
	}
	seen := map[string]bool{}
	for i, e := range m.Episodes {
		if e.EpisodeID == "" {
			return fmt.Errorf("episode %d has no id", i)
		}
		if seen[e.EpisodeID] {
			return fmt.Errorf("episode %q is declared twice", e.EpisodeID)
		}
		seen[e.EpisodeID] = true
		if _, ok := roles[e.Split]; !ok {
			return fmt.Errorf("episode %q names unknown split %q", e.EpisodeID, e.Split)
		}
		if len(e.ObjectIDs) == 0 {
			return fmt.Errorf("episode %q scores no objects", e.EpisodeID)
		}
		listed := map[string]bool{}
		for _, id := range e.ObjectIDs {
			if listed[id] {
				return fmt.Errorf("episode %q lists object %q twice", e.EpisodeID, id)
			}
			listed[id] = true
			// The load-bearing check: an episode may only score objects of
			// its own partition. A held-out episode that scored a tuning
			// object would quote a tuned number as held out.
			if got := owner[id]; got != e.Split {
				if got == "" {
					return fmt.Errorf("episode %q scores object %q, which is in no split", e.EpisodeID, id)
				}
				return fmt.Errorf("episode %q (split %q) scores object %q of split %q", e.EpisodeID, e.Split, id, got)
			}
		}
		if len(e.FrameIntervals) == 0 {
			return fmt.Errorf("episode %q has no frame intervals", e.EpisodeID)
		}
		for k, f := range e.FrameIntervals {
			if f.FirstSample < 0 || f.LastSample < f.FirstSample {
				return fmt.Errorf("episode %q interval %d [%d, %d] is empty or negative", e.EpisodeID, k, f.FirstSample, f.LastSample)
			}
			// Ordered and disjoint, so no frame is scored twice.
			if k > 0 && f.FirstSample <= e.FrameIntervals[k-1].LastSample {
				return fmt.Errorf("episode %q interval %d overlaps or precedes interval %d", e.EpisodeID, k, k-1)
			}
		}
	}
	return nil
}

// ValidateAgainst binds the manifest to one pack and one annotation snapshot.
// It fails closed on every disagreement: a manifest frozen against other
// points, another dataset, another revision, or objects the annotation no
// longer carries cannot say what was held out.
func (m *SplitManifest) ValidateAgainst(p *Pack, s *Sidecar) error {
	if m.PackDigest != p.Manifest.PackDigest {
		return fmt.Errorf("split manifest was frozen against pack %s; this pack is %s", m.PackDigest, p.Manifest.PackDigest)
	}
	if m.DatasetID != "" && m.DatasetID != p.Manifest.DatasetID {
		return fmt.Errorf("split manifest names dataset %q; this pack is %q", m.DatasetID, p.Manifest.DatasetID)
	}
	if s.PackDigest != p.Manifest.PackDigest {
		return fmt.Errorf("annotation was written against pack %s; this pack is %s", s.PackDigest, p.Manifest.PackDigest)
	}
	if m.SidecarRevision != 0 && m.SidecarRevision != s.Revision {
		return fmt.Errorf("split manifest pins annotation revision %d; revision %d was loaded", m.SidecarRevision, s.Revision)
	}
	status := make(map[string]ReviewStatus, len(s.Objects))
	for _, o := range s.Objects {
		status[o.ObjectID] = o.Status
	}
	for _, split := range m.Splits {
		for _, id := range split.ObjectIDs {
			st, ok := status[id]
			if !ok {
				return fmt.Errorf("split %q lists object %q, which annotation revision %d does not carry", split.Name, id, s.Revision)
			}
			if st == StatusRejected {
				return fmt.Errorf("split %q lists object %q, which annotation revision %d rejects: the split is stale", split.Name, id, s.Revision)
			}
		}
	}
	for _, e := range m.Episodes {
		for k, f := range e.FrameIntervals {
			if f.LastSample >= len(p.Samples) {
				return fmt.Errorf("episode %q interval %d ends at sample %d; the pack has %d samples", e.EpisodeID, k, f.LastSample, len(p.Samples))
			}
		}
	}
	return nil
}

// SplitByName returns a partition.
func (m *SplitManifest) SplitByName(name string) (Split, bool) {
	for _, s := range m.Splits {
		if s.Name == name {
			return s, true
		}
	}
	return Split{}, false
}

// SelectEpisodes returns the named split's episodes in manifest order, or only
// those listed in episodeIDs. With requireHeldOut, a split whose role is not
// held_out is refused with ErrNotHeldOut.
func (m *SplitManifest) SelectEpisodes(split string, episodeIDs []string, requireHeldOut bool) ([]Episode, error) {
	s, ok := m.SplitByName(split)
	if !ok {
		return nil, fmt.Errorf("split manifest has no split %q", split)
	}
	if requireHeldOut && s.Role != SplitRoleHeldOut {
		return nil, fmt.Errorf("%w: split %q has role %q, and held-out scoring was requested", ErrNotHeldOut, split, s.Role)
	}
	filtered := len(episodeIDs) > 0
	wanted := map[string]bool{}
	for _, id := range episodeIDs {
		wanted[id] = true
	}
	found := map[string]bool{}
	var out []Episode
	for _, e := range m.Episodes {
		if filtered && !wanted[e.EpisodeID] {
			continue
		}
		if e.Split != split {
			if filtered {
				return nil, fmt.Errorf("episode %q belongs to split %q, not %q", e.EpisodeID, e.Split, split)
			}
			continue
		}
		found[e.EpisodeID] = true
		out = append(out, e)
	}
	for _, id := range episodeIDs {
		if !found[id] {
			return nil, fmt.Errorf("split manifest has no episode %q", id)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("split %q has no episodes", split)
	}
	return out, nil
}
