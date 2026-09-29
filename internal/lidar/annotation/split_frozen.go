package annotation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

// Frozen splits (split manifest schema version 2).
//
// A version 1 manifest partitions one pack's objects, and a person writes it
// by hand. It says what is held out; it does not say whether anyone finished
// reviewing what it holds out, it pins the annotation revision only if its
// author remembered to, and nothing notices when the pack's manifest, its
// selection record or a retained revision is edited afterwards. Two packs cut
// from overlapping stretches of one capture can put the same vehicle on both
// sides of the partition without either manifest knowing.
//
// Freezing closes those gaps. FreezeSplit builds a split over one or more
// packs from an operator's draft and refuses unless:
//
//   - every object it partitions has completed membership review: the object
//     is reviewed, it has at least one reviewed mask, no mask of it is still a
//     proposal, and every reviewed mask states its completeness;
//   - every episode's objects have a reviewed mask inside its frames;
//   - no physical object can sit in two partitions: object IDs are disjoint
//     within a pack, and packs cut from overlapping stretches of one source,
//     each widened by GuardSeconds, feed a single partition between them;
//   - a pack whose selection record says it was chosen for tuning feeds no
//     held-out partition, since its window may have been chosen by where the
//     tracker failed;
//   - a pack cut from a declared corpus case feeds only partitions of that
//     case's role.
//
// It pins, per pack, the pack digest; the digest of manifest.json, which
// holds the source provenance, coverage and coordinate contract outside the
// pack digest; the digest of the selection record; and the annotation
// revision with the digest of its exact bytes. It records who froze the
// split, when, with which build, and optionally the configuration it was
// frozen to judge. SplitDigest is the SHA-256 of the split's canonical
// encoding, and a file whose content no longer matches it is refused.
//
// Geometry review is recorded beside membership review, never folded into
// it. It is read from the optional per-mask pose, the only geometry record a
// sidecar carries today, and says whether anyone reviewed a pose, not whether
// an independent physical reference exists; that record is separate work.
//
// Bind re-checks every pin against the pack an evaluator opens. Editing a
// reference after freezing saves a new annotation revision. The frozen split
// keeps scoring the revision it pinned; scoring the new one takes a new split
// revision (Supersedes), never a silently different split.

// FrozenSplitSchemaVersion is the frozen split's layout version. Version 1,
// SplitSchemaVersion, remains the hand-written single-pack manifest.
const FrozenSplitSchemaVersion = 2

// SplitRoleScreen is a corpus case replayed as a label-free regression
// screen: nothing is tuned on it and nothing is held out from it. Only a case
// may hold it; a partition is tuning or held out.
const SplitRoleScreen SplitRole = "screen"

// DefaultSplitGuardSeconds widens each pack's capture span when deciding
// whether two packs of one source could hold the same physical object. A
// vehicle at a light, or a pedestrian waiting to cross, stays in view for tens
// of seconds, and overlap alone only catches the certain case.
const DefaultSplitGuardSeconds = 30

// maxSplitGuardSeconds bounds the guard at a day: no capture here is longer,
// and the bound keeps the widened spans inside int64 nanoseconds.
const maxSplitGuardSeconds = 86400

// SplitDraftSchema names the operator's input to FreezeSplit.
const SplitDraftSchema = "velocity.report/annotation-split-draft"

// SplitDraftSchemaVersion is the draft layout version.
const SplitDraftSchemaVersion = 1

// segmentRecordFile is the selection record the segments flow writes beside a
// pack's manifest.
const segmentRecordFile = "segment.json"

// FrozenSplit is a reviewed split frozen over zero or more packs, with the
// corpus cases it assigns roles to.
type FrozenSplit struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	// Revision counts the freezes of one split's lineage, from 1.
	// Supersedes is the SplitDigest of the revision this one replaces.
	Revision   int          `json:"revision"`
	Supersedes string       `json:"supersedes,omitempty"`
	Frozen     FreezeRecord `json:"frozen"`
	Note       string       `json:"note,omitempty"`
	// GuardSeconds is the widening the cross-pack disjointness check used.
	GuardSeconds float64          `json:"guard_seconds"`
	Partitions   []SplitPartition `json:"partitions,omitempty"`
	Cases        []SplitCase      `json:"cases,omitempty"`
	Packs        []FrozenPack     `json:"packs,omitempty"`
	// SplitDigest is the SHA-256 of the canonical encoding of every field
	// above. It is what an evaluator records and what a later revision cites.
	SplitDigest string `json:"split_digest"`

	// FileDigest is the SHA-256 of the file's bytes, set on load, as a
	// version 1 manifest's Digest is.
	FileDigest string `json:"-"`
}

// FreezeRecord says who froze a split, when, and with what.
type FreezeRecord struct {
	Author       string `json:"author"`
	FrozenUTC    string `json:"frozen_utc"`
	BuildVersion string `json:"build_version"`
	BuildGitSHA  string `json:"build_git_sha"`
	// ForConfigHash and ForParamsHash, when set, name the configuration the
	// split was frozen to judge: the choice a held-out score confirms.
	ForConfigHash string `json:"for_config_hash,omitempty"`
	ForParamsHash string `json:"for_params_hash,omitempty"`
}

// SplitPartition is a named partition and its role.
type SplitPartition struct {
	Name string    `json:"name"`
	Role SplitRole `json:"role"`
}

// SplitCase gives a replay corpus case its role: tuning, held_out or screen.
type SplitCase struct {
	CaseID string    `json:"case_id"`
	Role   SplitRole `json:"role"`
}

// FrozenPack is one pack's frozen partition and the pins that bind it.
type FrozenPack struct {
	PackDigest string `json:"pack_digest"`
	DatasetID  string `json:"dataset_id"`
	// CaseID, when set, is the corpus case the pack was cut from.
	CaseID         string           `json:"case_id,omitempty"`
	ManifestSHA256 string           `json:"manifest_sha256"`
	Source         FrozenSource     `json:"source"`
	Selection      *FrozenSelection `json:"selection,omitempty"`
	// SidecarRevision and SidecarSHA256 pin the annotation revision and the
	// exact bytes it was frozen against.
	SidecarRevision int            `json:"sidecar_revision"`
	SidecarSHA256   string         `json:"sidecar_sha256"`
	Objects         []FrozenObject `json:"objects"`
	Episodes        []Episode      `json:"episodes"`
}

// FrozenSource is the pack's declared source, copied from its manifest so a
// frozen split can be read without the pack, and the capture span of its
// samples.
type FrozenSource struct {
	PCAPBasename   string `json:"pcap_basename,omitempty"`
	VRLOGHeaderSHA string `json:"vrlog_header_sha256,omitempty"`
	VRLOGFramesSHA string `json:"vrlog_frames_sha256,omitempty"`
	SensorID       string `json:"sensor_id,omitempty"`
	ConfigHash     string `json:"config_hash,omitempty"`
	ParamsHash     string `json:"params_hash,omitempty"`
	BuildGitSHA    string `json:"build_git_sha,omitempty"`
	FirstSampleNs  int64  `json:"first_sample_ns"`
	LastSampleNs   int64  `json:"last_sample_ns"`
}

// FrozenSelection pins the pack's selection record (segment.json). A pack
// without one, cut by hand, has none, and gaining one later is a change.
type FrozenSelection struct {
	SHA256    string `json:"sha256"`
	Role      string `json:"role"`
	Finder    string `json:"finder"`
	SegmentID string `json:"segment_id,omitempty"`
}

// FrozenObject is one reference object and the partition it was frozen into.
type FrozenObject struct {
	ObjectID      string         `json:"object_id"`
	Partition     string         `json:"partition"`
	Class         string         `json:"class"`
	ReviewedMasks int            `json:"reviewed_masks"`
	Geometry      GeometryReview `json:"geometry_review"`
}

// GeometryReviewStatus summarises the poses on an object's reviewed masks.
type GeometryReviewStatus string

const (
	// GeometryNone: no reviewed mask carries a pose.
	GeometryNone GeometryReviewStatus = "none"
	// GeometryProposed: poses exist, but only as proposals.
	GeometryProposed GeometryReviewStatus = "proposed"
	// GeometryPartial: some reviewed masks carry a reviewed pose.
	GeometryPartial GeometryReviewStatus = "partial"
	// GeometryComplete: every reviewed mask carries a reviewed pose.
	GeometryComplete GeometryReviewStatus = "complete"
)

// GeometryReview is an object's pose review, counted over its reviewed masks.
type GeometryReview struct {
	Status        GeometryReviewStatus `json:"status"`
	ReviewedPoses int                  `json:"reviewed_poses"`
	ProposedPoses int                  `json:"proposed_poses"`
}

// SplitDraft is what an operator writes and FreezeSplit freezes.
type SplitDraft struct {
	Schema        string      `json:"schema"`
	SchemaVersion int         `json:"schema_version"`
	Note          string      `json:"note,omitempty"`
	Cases         []SplitCase `json:"cases,omitempty"`
	Packs         []DraftPack `json:"packs,omitempty"`
}

// DraftPack is one pack's draft partition: either a version 1 manifest for
// the pack, or its splits and episodes written inline in the same form.
type DraftPack struct {
	// Dir is the pack directory, absolute or relative to the draft file.
	Dir    string `json:"dir"`
	CaseID string `json:"case_id,omitempty"`
	// SplitManifest names a version 1 manifest, absolute or relative to the
	// draft file, frozen as it stands.
	SplitManifest string `json:"split_manifest,omitempty"`
	// SidecarRevision pins an annotation revision; zero freezes the current.
	SidecarRevision int       `json:"sidecar_revision,omitempty"`
	Splits          []Split   `json:"splits,omitempty"`
	Episodes        []Episode `json:"episodes,omitempty"`
}

// FreezeOptions is everything FreezeSplit needs besides the packs.
type FreezeOptions struct {
	Draft *SplitDraft
	// BaseDir resolves the draft's relative paths: the draft file's directory.
	BaseDir string
	// Author is required. The store never invents a human identity, and a
	// frozen split without one cannot say who answered for it.
	Author string
	Now    time.Time
	// BuildVersion and BuildGitSHA identify the build that froze the split.
	BuildVersion, BuildGitSHA    string
	ForConfigHash, ForParamsHash string
	GuardSeconds                 float64
	// Supersedes, when set, is the frozen revision this one replaces.
	Supersedes *FrozenSplit
}

// LoadSplitDraft reads a draft. Unknown fields are refused, as they are in a
// split manifest.
func LoadSplitDraft(path string) (*SplitDraft, error) {
	b, err := readSplitFile(path, "split draft")
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var d SplitDraft
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("parse split draft: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("parse split draft: trailing data after the draft object")
	}
	if d.Schema != SplitDraftSchema || d.SchemaVersion != SplitDraftSchemaVersion {
		return nil, fmt.Errorf("split draft schema %q version %d, want %q version %d",
			d.Schema, d.SchemaVersion, SplitDraftSchema, SplitDraftSchemaVersion)
	}
	return &d, nil
}

// LoadAnySplit reads a split manifest of either version: a hand-written
// version 1 manifest of one pack, or a frozen version 2 split. Exactly one of
// the two results is set.
func LoadAnySplit(path string) (*SplitManifest, *FrozenSplit, error) {
	b, err := readSplitFile(path, "split manifest")
	if err != nil {
		return nil, nil, err
	}
	// Peek at the version only; each parser then decodes strictly.
	var head struct {
		SchemaVersion int `json:"schema_version"`
	}
	if json.Unmarshal(b, &head) == nil && head.SchemaVersion == FrozenSplitSchemaVersion {
		f, err := ParseFrozenSplit(b)
		return nil, f, err
	}
	m, err := ParseSplitManifest(b)
	return m, nil, err
}

// LoadFrozenSplit reads a frozen split and refuses a version 1 manifest: a
// caller that needs pinned review and digests cannot take a hand-written one.
func LoadFrozenSplit(path string) (*FrozenSplit, error) {
	v1, f, err := LoadAnySplit(path)
	if err != nil {
		return nil, err
	}
	if v1 != nil {
		return nil, fmt.Errorf("split manifest %s is version 1, not frozen: freeze it first (velocity lidar annotation-split freeze)", path)
	}
	return f, nil
}

// ParseFrozenSplit decodes a frozen split, checks its structure, and refuses
// it unless its content still has the digest it was frozen with.
func ParseFrozenSplit(b []byte) (*FrozenSplit, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var f FrozenSplit
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse frozen split: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("parse frozen split: trailing data after the split object")
	}
	if err := f.validate(); err != nil {
		return nil, fmt.Errorf("invalid frozen split: %w", err)
	}
	if got := f.contentDigest(); got != f.SplitDigest {
		return nil, fmt.Errorf("frozen split content has digest %s, but it was frozen as %s: it was edited after freezing; freeze a new revision instead",
			got, f.SplitDigest)
	}
	f.FileDigest = sha256Hex(b)
	return &f, nil
}

// readSplitFile reads a split manifest or draft, bounded by
// MaxSplitManifestBytes.
func readSplitFile(path, what string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", what, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxSplitManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	if len(b) > MaxSplitManifestBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", what, MaxSplitManifestBytes)
	}
	return b, nil
}

// packManifestDigest is the SHA-256 of a pack's manifest.json bytes.
func packManifestDigest(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return "", fmt.Errorf("read pack manifest: %w", err)
	}
	return sha256Hex(b), nil
}

// contentDigest is the SHA-256 of the split's canonical encoding without its
// digest. encoding/json writes struct fields in declaration order, and every
// list is written in the order freezing sorted it, so the encoding is stable.
// Encoding these types cannot fail: they hold only strings, numbers, bools
// and slices of the same.
func (f FrozenSplit) contentDigest() string {
	f.SplitDigest = ""
	b, _ := json.Marshal(f)
	return sha256Hex(b)
}

// validate checks everything that can be checked without the packs. It runs
// on every parse and at the end of every freeze, so a split FreezeSplit
// writes always reads back.
func (f *FrozenSplit) validate() error {
	if f.Schema != SplitSchema {
		return fmt.Errorf("schema %q, want %q", f.Schema, SplitSchema)
	}
	if f.SchemaVersion != FrozenSplitSchemaVersion {
		return fmt.Errorf("schema version %d, want %d", f.SchemaVersion, FrozenSplitSchemaVersion)
	}
	if f.Revision < 1 {
		return fmt.Errorf("revision %d: revisions count from 1", f.Revision)
	}
	if (f.Revision == 1) != (f.Supersedes == "") {
		return fmt.Errorf("revision %d with supersedes %q: only revision 1 supersedes nothing", f.Revision, f.Supersedes)
	}
	if f.Supersedes != "" && !strings.HasPrefix(f.Supersedes, "sha256:") {
		return fmt.Errorf("supersedes %q is not a sha256: digest", f.Supersedes)
	}
	if strings.TrimSpace(f.Frozen.Author) == "" {
		return fmt.Errorf("frozen.author is empty: say who froze the split")
	}
	if _, err := time.Parse(time.RFC3339, f.Frozen.FrozenUTC); err != nil {
		return fmt.Errorf("frozen.frozen_utc %q is not an RFC 3339 time", f.Frozen.FrozenUTC)
	}
	if f.Frozen.BuildVersion == "" || f.Frozen.BuildGitSHA == "" {
		return fmt.Errorf("frozen build version and git SHA are required, even when unstamped")
	}
	if math.IsNaN(f.GuardSeconds) || f.GuardSeconds < 0 || f.GuardSeconds > maxSplitGuardSeconds {
		return fmt.Errorf("guard_seconds %g must be between 0 and %d", f.GuardSeconds, maxSplitGuardSeconds)
	}
	if len(f.Packs) == 0 && len(f.Cases) == 0 {
		return fmt.Errorf("no packs and no cases: nothing is partitioned")
	}

	roles := map[string]SplitRole{}
	for i, p := range f.Partitions {
		if p.Name == "" {
			return fmt.Errorf("partition %d has no name", i)
		}
		if _, dup := roles[p.Name]; dup {
			return fmt.Errorf("partition %q is declared twice", p.Name)
		}
		if p.Role != SplitRoleTuning && p.Role != SplitRoleHeldOut {
			return fmt.Errorf("partition %q has role %q (want %q or %q)", p.Name, p.Role, SplitRoleTuning, SplitRoleHeldOut)
		}
		roles[p.Name] = p.Role
	}
	cases, err := caseRoles(f.Cases)
	if err != nil {
		return err
	}

	digests := map[string]bool{}
	episodes := map[string]string{}
	for _, p := range f.Packs {
		if err := p.validate(roles, cases); err != nil {
			return fmt.Errorf("pack %s: %w", p.PackDigest, err)
		}
		if digests[p.PackDigest] {
			return fmt.Errorf("pack %s is listed twice", p.PackDigest)
		}
		digests[p.PackDigest] = true
		if err := f.packView(p).validateStructure(); err != nil {
			return fmt.Errorf("pack %s: %w", p.PackDigest, err)
		}
		for _, e := range p.Episodes {
			if other, dup := episodes[e.EpisodeID]; dup {
				return fmt.Errorf("episode %q is in pack %s and pack %s: episode IDs are split-wide", e.EpisodeID, other, p.PackDigest)
			}
			episodes[e.EpisodeID] = p.PackDigest
		}
	}
	return checkSourceDisjoint(f.Packs, f.GuardSeconds)
}

func caseRoles(cases []SplitCase) (map[string]SplitRole, error) {
	out := make(map[string]SplitRole, len(cases))
	for i, c := range cases {
		if strings.TrimSpace(c.CaseID) == "" {
			return nil, fmt.Errorf("case %d has no id", i)
		}
		if _, dup := out[c.CaseID]; dup {
			return nil, fmt.Errorf("case %q is declared twice", c.CaseID)
		}
		if c.Role != SplitRoleTuning && c.Role != SplitRoleHeldOut && c.Role != SplitRoleScreen {
			return nil, fmt.Errorf("case %q has role %q (want %q, %q or %q)", c.CaseID, c.Role,
				SplitRoleTuning, SplitRoleHeldOut, SplitRoleScreen)
		}
		out[c.CaseID] = c.Role
	}
	return out, nil
}

func (p FrozenPack) validate(roles, cases map[string]SplitRole) error {
	for name, digest := range map[string]string{
		"pack_digest": p.PackDigest, "manifest_sha256": p.ManifestSHA256, "sidecar_sha256": p.SidecarSHA256,
	} {
		if !strings.HasPrefix(digest, "sha256:") {
			return fmt.Errorf("%s %q is not a sha256: digest", name, digest)
		}
	}
	if p.DatasetID == "" {
		return fmt.Errorf("no dataset_id")
	}
	if p.SidecarRevision < 1 {
		return fmt.Errorf("sidecar_revision %d: a frozen split pins a saved revision", p.SidecarRevision)
	}
	caseRole, hasCase := cases[p.CaseID]
	if p.CaseID != "" && !hasCase {
		return fmt.Errorf("case %q is not declared in the split's cases", p.CaseID)
	}
	if hasCase && caseRole == SplitRoleScreen {
		return fmt.Errorf("case %q is a screen case: a screen site holds no reference partition", p.CaseID)
	}
	if s := p.Selection; s != nil {
		if !strings.HasPrefix(s.SHA256, "sha256:") {
			return fmt.Errorf("selection sha256 %q is not a sha256: digest", s.SHA256)
		}
		if s.Role != string(SplitRoleTuning) && s.Role != string(SplitRoleHeldOut) {
			return fmt.Errorf("selection role %q (want tuning or held_out)", s.Role)
		}
	}
	if len(p.Objects) == 0 {
		return fmt.Errorf("no objects")
	}
	seen := map[string]bool{}
	for _, o := range p.Objects {
		if o.ObjectID == "" {
			return fmt.Errorf("an object has no id")
		}
		if seen[o.ObjectID] {
			return fmt.Errorf("object %q is listed twice: a pack's partitions are object-disjoint", o.ObjectID)
		}
		seen[o.ObjectID] = true
		role, ok := roles[o.Partition]
		if !ok {
			return fmt.Errorf("object %q names unknown partition %q", o.ObjectID, o.Partition)
		}
		if o.ReviewedMasks < 1 {
			return fmt.Errorf("object %q has no reviewed mask", o.ObjectID)
		}
		if err := o.Geometry.validate(o.ReviewedMasks); err != nil {
			return fmt.Errorf("object %q geometry review: %w", o.ObjectID, err)
		}
		// A window chosen for tuning may have been chosen by where the
		// tracker failed, which is exactly what a held-out number must not
		// be drawn from.
		if role == SplitRoleHeldOut && p.Selection != nil && p.Selection.Role == string(SplitRoleTuning) {
			return fmt.Errorf("object %q is held out, but the pack was selected for tuning (finder %s)", o.ObjectID, p.Selection.Finder)
		}
		if hasCase && role != caseRole {
			return fmt.Errorf("object %q is in %s partition %q, but case %q is %s: a case's references take its role",
				o.ObjectID, role, o.Partition, p.CaseID, caseRole)
		}
	}
	return nil
}

func (g GeometryReview) validate(reviewedMasks int) error {
	if g.ReviewedPoses < 0 || g.ProposedPoses < 0 || g.ReviewedPoses+g.ProposedPoses > reviewedMasks {
		return fmt.Errorf("%d reviewed and %d proposed poses on %d reviewed masks", g.ReviewedPoses, g.ProposedPoses, reviewedMasks)
	}
	if want := geometryStatus(g.ReviewedPoses, g.ProposedPoses, reviewedMasks); g.Status != want {
		return fmt.Errorf("status %q, but the counts make it %q", g.Status, want)
	}
	return nil
}

func geometryStatus(reviewedPoses, proposedPoses, reviewedMasks int) GeometryReviewStatus {
	switch {
	case reviewedPoses > 0 && reviewedPoses == reviewedMasks:
		return GeometryComplete
	case reviewedPoses > 0:
		return GeometryPartial
	case proposedPoses > 0:
		return GeometryProposed
	}
	return GeometryNone
}

// sourceKey names the recording a pack was cut from, when the pack says: the
// capture file, else the VRLOG. Two replays of one capture write different
// VRLOGs, so the capture is the stronger key.
func (s FrozenSource) sourceKey() string {
	if s.PCAPBasename != "" {
		return "pcap:" + s.PCAPBasename
	}
	if s.VRLOGFramesSHA != "" {
		return "vrlog:" + s.VRLOGFramesSHA
	}
	return ""
}

// checkSourceDisjoint refuses two packs of one source whose capture spans,
// widened by the guard, overlap, unless every object of both sits in one
// partition. Object IDs are pack-local, so nothing else can say whether a
// car in one pack is a car in the other; while their spans could share a
// vehicle, they may only share a partition.
func checkSourceDisjoint(packs []FrozenPack, guardSeconds float64) error {
	guard := int64(guardSeconds * 1e9)
	for i := range packs {
		for j := i + 1; j < len(packs); j++ {
			a, b := packs[i], packs[j]
			key := a.Source.sourceKey()
			if key == "" || key != b.Source.sourceKey() {
				continue
			}
			if a.Source.FirstSampleNs-guard > b.Source.LastSampleNs || b.Source.FirstSampleNs-guard > a.Source.LastSampleNs {
				continue
			}
			partitions := map[string]bool{}
			for _, p := range []FrozenPack{a, b} {
				for _, o := range p.Objects {
					partitions[o.Partition] = true
				}
			}
			if len(partitions) > 1 {
				return fmt.Errorf("packs %s and %s are cut from %s within %gs of each other and feed partitions %s: "+
					"the same physical object could sit in two partitions; cut them further apart or put both in one partition",
					a.PackDigest, b.PackDigest, key, guardSeconds, strings.Join(sortedKeys(partitions), ", "))
			}
		}
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// packView is one pack's partition in version 1 form, which ValidateAgainst
// and SelectEpisodes already know how to hold to account.
func (f *FrozenSplit) packView(p FrozenPack) *SplitManifest {
	byPartition := map[string][]string{}
	for _, o := range p.Objects {
		byPartition[o.Partition] = append(byPartition[o.Partition], o.ObjectID)
	}
	m := &SplitManifest{
		Schema: SplitSchema, SchemaVersion: SplitSchemaVersion, PackDigest: p.PackDigest, DatasetID: p.DatasetID,
		SidecarRevision: p.SidecarRevision, Note: f.Note, Episodes: p.Episodes, Digest: f.FileDigest,
	}
	for _, part := range f.Partitions {
		if ids := byPartition[part.Name]; len(ids) > 0 {
			m.Splits = append(m.Splits, Split{Name: part.Name, Role: part.Role, ObjectIDs: ids})
		}
	}
	return m
}

// FreezeSplit freezes a draft. Every refusal names what to fix; review
// problems are listed for every object at once, so one pass of review can
// clear them all.
func FreezeSplit(opts FreezeOptions) (*FrozenSplit, error) {
	if opts.Draft == nil {
		return nil, fmt.Errorf("no draft to freeze")
	}
	if opts.Now.IsZero() {
		return nil, fmt.Errorf("no freezing time")
	}
	f := &FrozenSplit{
		Schema: SplitSchema, SchemaVersion: FrozenSplitSchemaVersion, Revision: 1,
		Frozen: FreezeRecord{
			Author: strings.TrimSpace(opts.Author), FrozenUTC: opts.Now.UTC().Format(time.RFC3339),
			BuildVersion: opts.BuildVersion, BuildGitSHA: opts.BuildGitSHA,
			ForConfigHash: opts.ForConfigHash, ForParamsHash: opts.ForParamsHash,
		},
		Note: opts.Draft.Note, GuardSeconds: opts.GuardSeconds,
		Cases: append([]SplitCase(nil), opts.Draft.Cases...),
	}
	sort.Slice(f.Cases, func(i, j int) bool { return f.Cases[i].CaseID < f.Cases[j].CaseID })

	roles := map[string]SplitRole{}
	var problems []string
	for i, dp := range opts.Draft.Packs {
		p, packRoles, packProblems, err := freezePack(dp, opts.BaseDir)
		if err != nil {
			return nil, fmt.Errorf("draft pack %d (%s): %w", i, dp.Dir, err)
		}
		for name, role := range packRoles {
			if other, ok := roles[name]; ok && other != role {
				return nil, fmt.Errorf("partition %q is %s in one pack and %s in another", name, other, role)
			}
			roles[name] = role
		}
		problems = append(problems, packProblems...)
		f.Packs = append(f.Packs, p)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("membership review is not complete, so the split cannot be frozen:\n  %s",
			strings.Join(problems, "\n  "))
	}
	for name, role := range roles {
		f.Partitions = append(f.Partitions, SplitPartition{Name: name, Role: role})
	}
	sort.Slice(f.Partitions, func(i, j int) bool { return f.Partitions[i].Name < f.Partitions[j].Name })
	sort.Slice(f.Packs, func(i, j int) bool { return f.Packs[i].PackDigest < f.Packs[j].PackDigest })

	if prev := opts.Supersedes; prev != nil {
		if err := checkSupersedes(prev, f); err != nil {
			return nil, err
		}
		f.Revision, f.Supersedes = prev.Revision+1, prev.SplitDigest
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	f.SplitDigest = f.contentDigest()
	return f, nil
}

// freezePack opens one draft pack, pins it and checks its review. It returns
// review problems separately from errors, so they can be reported together.
func freezePack(dp DraftPack, baseDir string) (FrozenPack, map[string]SplitRole, []string, error) {
	resolve := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(baseDir, path)
	}
	if dp.Dir == "" {
		return FrozenPack{}, nil, nil, fmt.Errorf("no pack dir")
	}
	// Digest the manifest before opening, so the digest pinned is of the
	// bytes the opened pack was read from; Bind checks it again.
	manifestDigest, err := packManifestDigest(resolve(dp.Dir))
	if err != nil {
		return FrozenPack{}, nil, nil, err
	}
	pack, err := OpenPack(resolve(dp.Dir))
	if err != nil {
		return FrozenPack{}, nil, nil, err
	}
	m := &SplitManifest{
		Schema: SplitSchema, SchemaVersion: SplitSchemaVersion, PackDigest: pack.Manifest.PackDigest,
		DatasetID: pack.Manifest.DatasetID, SidecarRevision: dp.SidecarRevision, Splits: dp.Splits, Episodes: dp.Episodes,
	}
	if dp.SplitManifest != "" {
		if dp.SidecarRevision != 0 || len(dp.Splits) != 0 || len(dp.Episodes) != 0 {
			return FrozenPack{}, nil, nil, fmt.Errorf("give split_manifest or inline splits, episodes and revision, not both")
		}
		if m, err = LoadSplitManifest(resolve(dp.SplitManifest)); err != nil {
			return FrozenPack{}, nil, nil, err
		}
	} else if err := m.validateStructure(); err != nil {
		return FrozenPack{}, nil, nil, err
	}

	var s *Sidecar
	if m.SidecarRevision > 0 {
		s, err = LoadSidecarRevision(pack, m.SidecarRevision)
	} else {
		s, err = LoadSidecar(pack)
	}
	if err != nil {
		return FrozenPack{}, nil, nil, fmt.Errorf("load annotation: %w", err)
	}
	if s.baseDigest == "" {
		return FrozenPack{}, nil, nil, fmt.Errorf("the pack has no saved annotation to freeze")
	}
	m.SidecarRevision = s.Revision
	if err := m.ValidateAgainst(pack, s); err != nil {
		return FrozenPack{}, nil, nil, err
	}

	selection, err := readSelection(pack)
	if err != nil {
		return FrozenPack{}, nil, nil, err
	}
	src := pack.Manifest.Source
	first, last := sampleSpan(pack.Samples)
	p := FrozenPack{
		PackDigest: pack.Manifest.PackDigest, DatasetID: pack.Manifest.DatasetID, CaseID: dp.CaseID,
		ManifestSHA256: manifestDigest, Selection: selection,
		Source: FrozenSource{
			PCAPBasename: src.PCAPBasename, VRLOGHeaderSHA: src.VRLOGHeaderSHA, VRLOGFramesSHA: src.VRLOGFramesSHA,
			SensorID: src.SensorID, ConfigHash: src.ConfigHash, ParamsHash: src.ParamsHash, BuildGitSHA: src.BuildGitSHA,
			FirstSampleNs: first, LastSampleNs: last,
		},
		SidecarRevision: s.Revision, SidecarSHA256: s.baseDigest, Episodes: m.Episodes,
	}
	roles := map[string]SplitRole{}
	for _, split := range m.Splits {
		roles[split.Name] = split.Role
		for _, id := range split.ObjectIDs {
			p.Objects = append(p.Objects, frozenObject(s, id, split.Name))
		}
	}
	sort.Slice(p.Objects, func(i, j int) bool { return p.Objects[i].ObjectID < p.Objects[j].ObjectID })
	return p, roles, reviewProblems(pack.Manifest.PackDigest, s, m), nil
}

func sampleSpan(samples []Sample) (first, last int64) {
	for i, s := range samples {
		if i == 0 || s.TimestampNs < first {
			first = s.TimestampNs
		}
		if i == 0 || s.TimestampNs > last {
			last = s.TimestampNs
		}
	}
	return first, last
}

// readSelection pins segment.json, when the pack has one. A record that does
// not validate, or names another pack, is refused rather than ignored: it is
// the only statement of why this window was chosen.
func readSelection(p *Pack) (*FrozenSelection, error) {
	b, err := os.ReadFile(filepath.Join(p.Dir, segmentRecordFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read selection record: %w", err)
	}
	var r segments.Record
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse selection record: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("selection record: %w", err)
	}
	if r.PackDigest != p.Manifest.PackDigest {
		return nil, fmt.Errorf("selection record names pack %s; this pack is %s", r.PackDigest, p.Manifest.PackDigest)
	}
	return &FrozenSelection{SHA256: sha256Hex(b), Role: r.Role, Finder: r.Finder, SegmentID: r.Segment.ID}, nil
}

func frozenObject(s *Sidecar, objectID, partition string) FrozenObject {
	o := FrozenObject{ObjectID: objectID, Partition: partition}
	for _, obj := range s.Objects {
		if obj.ObjectID == objectID {
			o.Class = obj.Class
		}
	}
	for _, m := range s.Masks {
		if m.ObjectID != objectID || m.Status != StatusReviewed {
			continue
		}
		o.ReviewedMasks++
		if m.Pose != nil && m.Pose.Status == StatusReviewed {
			o.Geometry.ReviewedPoses++
		} else if m.Pose != nil && m.Pose.Status == StatusProposed {
			o.Geometry.ProposedPoses++
		}
	}
	o.Geometry.Status = geometryStatus(o.Geometry.ReviewedPoses, o.Geometry.ProposedPoses, o.ReviewedMasks)
	return o
}

// reviewProblems lists why membership review of the split's objects is not
// complete, one line per object, and episodes whose objects have no reviewed
// mask in their frames. Empty means complete.
func reviewProblems(packDigest string, s *Sidecar, m *SplitManifest) []string {
	status := map[string]ReviewStatus{}
	for _, o := range s.Objects {
		status[o.ObjectID] = o.Status
	}
	var out []string
	for _, split := range m.Splits {
		for _, id := range split.ObjectIDs {
			var reviewed, proposed, unstated int
			for _, mask := range s.Masks {
				if mask.ObjectID != id {
					continue
				}
				switch {
				case mask.Status == StatusProposed:
					proposed++
				case mask.Status == StatusReviewed && mask.Completeness == MaskUnreviewed:
					unstated++
				case mask.Status == StatusReviewed:
					reviewed++
				}
			}
			var why []string
			if status[id] != StatusReviewed {
				why = append(why, fmt.Sprintf("object is %s", status[id]))
			}
			if proposed > 0 {
				why = append(why, fmt.Sprintf("%d mask(s) still proposed", proposed))
			}
			if unstated > 0 {
				why = append(why, fmt.Sprintf("%d reviewed mask(s) without a stated completeness", unstated))
			}
			if reviewed+unstated == 0 {
				why = append(why, "no reviewed mask")
			}
			if len(why) > 0 {
				out = append(out, fmt.Sprintf("pack %s object %s: %s", packDigest, id, strings.Join(why, ", ")))
			}
		}
	}
	for _, e := range m.Episodes {
		for _, id := range e.ObjectIDs {
			found := false
			for _, mask := range s.Masks {
				found = found || (mask.ObjectID == id && mask.Status == StatusReviewed && e.ContainsSample(mask.SampleID))
			}
			if !found {
				out = append(out, fmt.Sprintf("pack %s episode %s: object %s has no reviewed mask in the episode's frames",
					packDigest, e.EpisodeID, id))
			}
		}
	}
	sort.Strings(out)
	return out
}

// checkSupersedes refuses a revision that would hold out what an earlier
// revision gave to tuning. A tuned object, or a tuned case, has been looked
// at; no later revision can make it unseen.
func checkSupersedes(prev, next *FrozenSplit) error {
	tuned := map[string]bool{}
	prevRoles := map[string]SplitRole{}
	for _, p := range prev.Partitions {
		prevRoles[p.Name] = p.Role
	}
	for _, p := range prev.Packs {
		for _, o := range p.Objects {
			if prevRoles[o.Partition] == SplitRoleTuning {
				tuned[p.PackDigest+"/"+o.ObjectID] = true
			}
		}
	}
	nextRoles := map[string]SplitRole{}
	for _, p := range next.Partitions {
		nextRoles[p.Name] = p.Role
	}
	for _, p := range next.Packs {
		for _, o := range p.Objects {
			if tuned[p.PackDigest+"/"+o.ObjectID] && nextRoles[o.Partition] == SplitRoleHeldOut {
				return fmt.Errorf("pack %s object %s was a tuning object in revision %d and cannot be held out in its successor",
					p.PackDigest, o.ObjectID, prev.Revision)
			}
		}
	}
	prevCases, _ := caseRoles(prev.Cases)
	for _, c := range next.Cases {
		if prevCases[c.CaseID] == SplitRoleTuning && c.Role == SplitRoleHeldOut {
			return fmt.Errorf("case %s was tuning in revision %d and cannot be held out in its successor", c.CaseID, prev.Revision)
		}
	}
	return nil
}

// Bind checks the frozen split against one of its packs, as an evaluator
// opens it, and returns that pack's partition in version 1 form with the
// annotation revision it pins. Every pin is re-checked: the pack's manifest
// and selection record, the pinned revision's bytes, the objects that
// revision still carries, and the membership review the freeze certified.
func (f *FrozenSplit) Bind(p *Pack) (*SplitManifest, *Sidecar, error) {
	var entry *FrozenPack
	for i := range f.Packs {
		if f.Packs[i].PackDigest == p.Manifest.PackDigest {
			entry = &f.Packs[i]
		}
	}
	if entry == nil {
		return nil, nil, fmt.Errorf("pack %s is not in frozen split %s", p.Manifest.PackDigest, f.SplitDigest)
	}
	if entry.DatasetID != p.Manifest.DatasetID {
		return nil, nil, fmt.Errorf("frozen split names dataset %q; this pack is %q", entry.DatasetID, p.Manifest.DatasetID)
	}
	manifestDigest, err := packManifestDigest(p.Dir)
	if err != nil {
		return nil, nil, err
	}
	if manifestDigest != entry.ManifestSHA256 {
		return nil, nil, fmt.Errorf("pack %s manifest.json changed after freezing (%s, now %s)",
			entry.PackDigest, entry.ManifestSHA256, manifestDigest)
	}
	selection, err := readSelection(p)
	if err != nil {
		return nil, nil, err
	}
	if (selection == nil) != (entry.Selection == nil) || (selection != nil && selection.SHA256 != entry.Selection.SHA256) {
		return nil, nil, fmt.Errorf("pack %s selection record changed after freezing", entry.PackDigest)
	}
	s, err := LoadSidecarRevision(p, entry.SidecarRevision)
	if err != nil {
		return nil, nil, fmt.Errorf("load pinned annotation revision %d: %w", entry.SidecarRevision, err)
	}
	if s.baseDigest != entry.SidecarSHA256 {
		return nil, nil, fmt.Errorf("annotation revision %d of pack %s is not the bytes that were frozen (%s, now %s): "+
			"a changed reference is a new revision; freeze a new split revision against it",
			entry.SidecarRevision, entry.PackDigest, entry.SidecarSHA256, s.baseDigest)
	}
	view := f.packView(*entry)
	if err := view.ValidateAgainst(p, s); err != nil {
		return nil, nil, err
	}
	if problems := reviewProblems(entry.PackDigest, s, view); len(problems) > 0 {
		return nil, nil, fmt.Errorf("the pinned revision does not support the frozen review:\n  %s", strings.Join(problems, "\n  "))
	}
	return view, s, nil
}

// CaseRoles returns the role each named corpus case holds, and refuses a
// replay the split forbids. Every case must be declared. A held-out score
// takes held-out cases only, as SelectEpisodes refuses a tuning split for
// held-out scoring; any other replay takes no held-out case, since tuning
// against a held-out case invalidates it.
func (f *FrozenSplit) CaseRoles(caseIDs []string, heldOut bool) (map[string]SplitRole, error) {
	// Parsing and freezing validated the cases; an invalid set declares none.
	declared, _ := caseRoles(f.Cases)
	out := make(map[string]SplitRole, len(caseIDs))
	for _, id := range caseIDs {
		role, ok := declared[id]
		switch {
		case !ok:
			return nil, fmt.Errorf("case %q has no role in frozen split %s", id, f.SplitDigest)
		case heldOut && role != SplitRoleHeldOut:
			return nil, fmt.Errorf("%w: case %q has role %q, and a held-out score was requested", ErrNotHeldOut, id, role)
		case !heldOut && role == SplitRoleHeldOut:
			return nil, fmt.Errorf("case %q is held out in frozen split %s: replay it only as a held-out score", id, f.SplitDigest)
		}
		out[id] = role
	}
	return out, nil
}

// splitOutput is the file WriteFrozenSplit writes through.
type splitOutput interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

// WriteFrozenSplit writes a frozen split to a new file. It never replaces
// one: a frozen split is write-once, and a change is a new revision.
func WriteFrozenSplit(path string, f *FrozenSplit) error {
	return writeFrozenSplitWithOpen(path, f, func(p string) (splitOutput, error) {
		return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	})
}

func writeFrozenSplitWithOpen(path string, f *FrozenSplit, open func(string) (splitOutput, error)) error {
	b, _ := json.MarshalIndent(f, "", "  ")
	out, err := open(path)
	if err != nil {
		return fmt.Errorf("create frozen split: %w", err)
	}
	if err := flushAnnotationFile(out, append(b, '\n')); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("write frozen split: %w", err)
	}
	return nil
}
