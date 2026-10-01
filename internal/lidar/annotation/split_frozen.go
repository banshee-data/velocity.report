package annotation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

// Frozen splits (split manifest schema versions 2, 3 and 4).
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
//     within a pack, and two packs of one source whose capture spans come
//     within GuardSeconds of each other (the gap between them is at most
//     GuardSeconds, overlap included) feed a single partition between them;
//   - a pack whose selection record says it was chosen for tuning feeds no
//     held-out partition, since its window may have been chosen by where the
//     tracker failed;
//   - every corpus case names the captures it replays, and a pack cut from
//     one of them feeds only partitions of that case's role, whether or not
//     the pack names the case;
//   - nothing held out was tuned on anywhere in the split's lineage (Tuned).
//
// A case is bound to its captures, not to its name. A capture a draft
// declares carries its file's SHA-256, which is what a replay under the split
// is checked against (CheckCaseCaptures). A pack records only its capture's
// basename (manifest.json pcap_basename), so packs are matched to cases by
// basename, and a capture a case gains only from a pack that names it is
// matched by basename too.
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
// Physical references (physical.go) are pinned beside the sidecar, from
// version 3. A pack with a physical-reference document is frozen with the
// revision the draft names (physical_revision), or its head: the revision
// number, the SHA-256 of its exact bytes and of its content, and two
// summaries derived from the document. The review summary is per object:
// the body's review status and whether it is independent of the tracker,
// and the keyframes counted by review status and by tracker assistance. The
// coverage summary is per component, position, yaw, length, width, height,
// front and rear, over the reviewed independent keyframes: how many the
// reference layer (PhysicalObject.Geometry) can score and how many it
// cannot. Both are re-derived by BindPhysical from the pinned bytes, as the
// membership summaries are by Bind. A pack with no physical references is
// frozen with no pin, as every pack of a version 2 split was. Freezing
// refuses, listing every problem at once, a document whose links do not
// hold against the pinned annotation revision, or whose reviews were made
// against other membership than the pinned revision in a frame they rest on
// (ReviewDrift); a component honestly stated unknown refuses nothing. A
// version 2 split, frozen before physical pins existed, still parses, binds
// and scores as it did, and carries no pin.
//
// Facet proposals are opt-in from version 4. A draft's feature_revision
// pins an exact retained proposal revision, or its saved head at zero.
// Omitting the field retains the feature-free version 3 layout. The pin
// records byte/content digests and proposal counts, including retirement,
// absence and tracker-seeded registrations. BindFeatures rederives them
// against the frozen definite membership; none is physical scoring truth.
//
// PreviewFreeze does everything FreezeSplit does short of producing the
// split: it opens and pins the packs, lists the review problems, and, when
// there are none, completes the split far enough to say what its digest
// would be. A Freeze action shows that before it commits; the checks are
// the same code, so the button is not where they are enforced.
//
// Bind re-checks every pin against the pack an evaluator opens, and derives
// again from the pinned bytes everything the file copies from them: the
// selection record's role, finder and segment, the source provenance and
// capture span, and each object's class, reviewed masks and geometry review.
// Editing a reference after freezing saves a new annotation revision. The
// frozen split keeps scoring the revision it pinned; scoring the new one
// takes a new split revision (Supersedes), never a silently different split.
//
// Tuned is the lineage's cumulative record of what it tuned on: each tuning
// pack's source, capture span and tuned objects, and each tuning case's
// captures. A revision's record is its predecessor's plus its own tuning, so
// a pack or case a later revision drops stays in it. A held-out object is
// refused if the record holds it, or holds a pack of the same source within
// the guard of its pack, or a tuned case's capture its pack was cut from; a
// held-out case is refused if the record holds it by ID or by capture, or a
// tuned pack cut from one of its captures. A revision frozen without
// Supersedes starts a new lineage whose record is its own tuning alone.
//
// What the digest does not do. SplitDigest is unkeyed: it detects an
// accidental edit, not a deliberate one, since whoever edits the file can
// recompute it. Everything Bind can derive from pinned bytes it derives, so
// such an edit cannot change them; what remains is the operator's own
// statement: the partitions and their roles, the episodes, the cases and
// their captures, the Tuned record and the freeze record. And the held-out
// guarantee binds only a run given the split: a replay or evaluation run
// without one is not checked against it, and records no split.

// FrozenSplitSchemaVersion is the feature-free layout FreezeSplit writes. Version 1,
// SplitSchemaVersion, remains the hand-written single-pack manifest.
const FrozenSplitSchemaVersion = 3

// Version 4 is written only when an operator opts into facet proposal pins.
const FrozenSplitSchemaVersionFeatures = 4

// FrozenSplitSchemaVersionMembershipOnly is the version 2 layout, frozen
// before physical-reference pins existed. It is still read, and carries no
// physical pin.
const FrozenSplitSchemaVersionMembershipOnly = 2

// isFrozenSchemaVersion reports whether v is a frozen split layout.
func isFrozenSchemaVersion(v int) bool {
	return v == FrozenSplitSchemaVersionFeatures || v == FrozenSplitSchemaVersion || v == FrozenSplitSchemaVersionMembershipOnly
}

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
	// Tuned is everything this split's lineage has tuned on, up to and
	// including this revision.
	Tuned TunedLedger `json:"tuned"`
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

// SplitCase gives a replay corpus case its role, tuning, held_out or screen,
// and names the captures the case replays: the role binds to them, not to
// the ID.
type SplitCase struct {
	CaseID   string        `json:"case_id"`
	Role     SplitRole     `json:"role"`
	Captures []CaseCapture `json:"captures,omitempty"`
}

// CaseCapture is one capture file of a case.
type CaseCapture struct {
	// Basename is the capture file's name, as a pack's manifest.json records
	// it (pcap_basename): what matches a pack to the case.
	Basename string `json:"basename"`
	// SHA256 is the capture file's SHA-256 ("sha256:<hex>"). A draft
	// declares it for every capture it names; a capture the case gains only
	// from a pack that names the case has none, since a pack records only
	// the basename. With it, a replay is checked by content; without it, by
	// basename.
	SHA256 string `json:"sha256,omitempty"`
}

// TunedLedger is a split lineage's cumulative record of what it tuned on.
type TunedLedger struct {
	Spans []TunedSpan `json:"spans,omitempty"`
	Cases []TunedCase `json:"cases,omitempty"`
}

// TunedSpan is one pack's tuned objects and the stretch of capture they were
// seen in.
type TunedSpan struct {
	// Revision is the first revision of the lineage that tuned on the pack.
	Revision   int    `json:"revision"`
	PackDigest string `json:"pack_digest"`
	// Source is the recording the pack was cut from, pcap:<basename> or
	// vrlog:<frames SHA-256>; empty when the pack names neither.
	Source        string `json:"source,omitempty"`
	FirstSampleNs int64  `json:"first_sample_ns"`
	LastSampleNs  int64  `json:"last_sample_ns"`
	// GuardSeconds is the widest guard any revision tuned the pack under;
	// a held-out pack is kept at least this far from it.
	GuardSeconds float64  `json:"guard_seconds"`
	ObjectIDs    []string `json:"object_ids"`
}

// TunedCase is a case the lineage tuned on, and its captures.
type TunedCase struct {
	// Revision is the first revision of the lineage that tuned on the case.
	Revision int           `json:"revision"`
	CaseID   string        `json:"case_id"`
	Captures []CaseCapture `json:"captures"`
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
	// Physical pins the pack's physical-reference revision. Nil for a pack
	// with no physical references, and for every pack of a version 2 split.
	Physical *FrozenPhysical `json:"physical,omitempty"`
	Features *FrozenFeatures `json:"features,omitempty"`
}

// FrozenPhysical pins one pack's physical references: the revision, the
// digest of its exact bytes, the digest of its content, and what that
// document reviews and can score. The summaries are derived from the pinned
// bytes; BindPhysical derives them again and refuses a disagreement.
type FrozenPhysical struct {
	Revision      int    `json:"revision"`
	SHA256        string `json:"sha256"`
	ContentSHA256 string `json:"content_sha256"`
	// Objects summarises every object of the document, sorted by ID.
	Objects []FrozenPhysicalObject `json:"objects"`
	// Coverage counts, per component, the reviewed independent keyframes
	// the reference layer can score and those it cannot.
	Coverage PhysicalCoverage `json:"coverage"`
}

// FrozenPhysicalObject is one object's physical review. It is separate from
// GeometryReview, which counts sidecar poses and is not physical review.
type FrozenPhysicalObject struct {
	ObjectID  string                 `json:"object_id"`
	Body      PhysicalBodyReview     `json:"body"`
	Keyframes PhysicalKeyframeCounts `json:"keyframes"`
}

// PhysicalBodyStatus is a body record's review as the pin summarises it.
type PhysicalBodyStatus string

const (
	// PhysicalBodyNone: the object has no body record.
	PhysicalBodyNone PhysicalBodyStatus = "none"
	// PhysicalBodyProposed: the body is a proposal no one has confirmed.
	PhysicalBodyProposed PhysicalBodyStatus = "proposed"
	// PhysicalBodyReviewed: a person confirmed the body.
	PhysicalBodyReviewed PhysicalBodyStatus = "reviewed"
	// PhysicalBodyRejected: the body was examined and found wrong, and is
	// kept rather than deleted, as the store keeps it.
	PhysicalBodyRejected PhysicalBodyStatus = "rejected"
)

// PhysicalBodyReview is an object's body record: its review status, and
// whether it was authored without tracker output. Independent is false for
// an object without a body.
type PhysicalBodyReview struct {
	Status      PhysicalBodyStatus `json:"status"`
	Independent bool               `json:"independent"`
}

// PhysicalKeyframeCounts counts an object's keyframes by review and origin.
// A tracker-assisted keyframe is counted under its review status as well.
type PhysicalKeyframeCounts struct {
	Total           int `json:"total"`
	Reviewed        int `json:"reviewed"`
	Proposed        int `json:"proposed"`
	TrackerAssisted int `json:"tracker_assisted"`
}

// PhysicalCoverage is, per component, how many reviewed independent
// keyframes the reference layer (PhysicalObject.Geometry) can score, and how
// many it cannot, whatever the reason: the component is unknown, a class
// prior only, a partial span, withheld by an unresolved axis, or withheld by
// a body that is missing, unreviewed or tracker-assisted. Position is the
// body centre the keyframe establishes, which is what a scorer matches and
// scores. A keyframe not reviewed, or not independent, is truth nowhere and
// counts nowhere.
type PhysicalCoverage struct {
	Position ComponentCoverage `json:"position"`
	Yaw      ComponentCoverage `json:"yaw"`
	Length   ComponentCoverage `json:"length"`
	Width    ComponentCoverage `json:"width"`
	Height   ComponentCoverage `json:"height"`
	Front    ComponentCoverage `json:"front"`
	Rear     ComponentCoverage `json:"rear"`
}

// ComponentCoverage is one component's counts of reviewed independent
// keyframes with and without scorable evidence.
type ComponentCoverage struct {
	Scorable    int `json:"scorable"`
	Unavailable int `json:"unavailable"`
}

func (c *ComponentCoverage) count(scorable bool) {
	if scorable {
		c.Scorable++
	} else {
		c.Unavailable++
	}
}

// NamedComponentCoverage is one component of a PhysicalCoverage with its
// name, for listing.
type NamedComponentCoverage struct {
	Name     string
	Coverage ComponentCoverage
}

// Components lists the coverage in report order.
func (c PhysicalCoverage) Components() []NamedComponentCoverage {
	return []NamedComponentCoverage{
		{"position", c.Position}, {"yaw", c.Yaw}, {"length", c.Length}, {"width", c.Width},
		{"height", c.Height}, {"front", c.Front}, {"rear", c.Rear},
	}
}

func (fp *FrozenPhysical) validate() error {
	if fp.Revision < 1 {
		return fmt.Errorf("revision %d: a frozen split pins a saved revision", fp.Revision)
	}
	for name, digest := range map[string]string{"sha256": fp.SHA256, "content_sha256": fp.ContentSHA256} {
		if !strings.HasPrefix(digest, "sha256:") {
			return fmt.Errorf("%s %q is not a sha256: digest", name, digest)
		}
	}
	for i, o := range fp.Objects {
		if o.ObjectID == "" {
			return fmt.Errorf("object %d has no id", i)
		}
		if i > 0 && o.ObjectID <= fp.Objects[i-1].ObjectID {
			return fmt.Errorf("objects must be sorted by id and distinct: %q follows %q", o.ObjectID, fp.Objects[i-1].ObjectID)
		}
		switch o.Body.Status {
		case PhysicalBodyNone, PhysicalBodyProposed, PhysicalBodyReviewed, PhysicalBodyRejected:
		default:
			return fmt.Errorf("object %q body status %q", o.ObjectID, o.Body.Status)
		}
		if o.Body.Status == PhysicalBodyNone && o.Body.Independent {
			return fmt.Errorf("object %q has no body, so none can be independent", o.ObjectID)
		}
		k := o.Keyframes
		if k.Total < 0 || k.Reviewed < 0 || k.Proposed < 0 || k.TrackerAssisted < 0 ||
			k.Reviewed+k.Proposed > k.Total || k.TrackerAssisted > k.Total {
			return fmt.Errorf("object %q keyframe counts %+v do not add up", o.ObjectID, k)
		}
	}
	for _, c := range fp.Coverage.Components() {
		if c.Coverage.Scorable < 0 || c.Coverage.Unavailable < 0 {
			return fmt.Errorf("%s coverage %+v is negative", c.Name, c.Coverage)
		}
	}
	return nil
}

// summarisePhysical derives a pin's summaries from a document: each object's
// body review and keyframe counts, sorted by object ID, and the coverage over
// every reviewed independent keyframe.
func summarisePhysical(doc *PhysicalReferenceSet) ([]FrozenPhysicalObject, PhysicalCoverage) {
	objects := make([]FrozenPhysicalObject, 0, len(doc.Objects))
	var cov PhysicalCoverage
	for _, o := range doc.Objects {
		fo := FrozenPhysicalObject{ObjectID: o.ObjectID, Body: PhysicalBodyReview{Status: PhysicalBodyNone}}
		if b := o.Body; b != nil {
			fo.Body = PhysicalBodyReview{Status: PhysicalBodyStatus(b.Review.Status), Independent: b.Review.Origin == OriginIndependent}
		}
		for _, k := range o.Keyframes {
			fo.Keyframes.Total++
			switch k.Review.Status {
			case StatusReviewed:
				fo.Keyframes.Reviewed++
			case StatusProposed:
				fo.Keyframes.Proposed++
			}
			if k.Review.Origin == OriginTrackerAssisted {
				fo.Keyframes.TrackerAssisted++
			}
			if !k.Review.ScoredAsTruth() {
				continue
			}
			g := o.Geometry(k)
			cov.Position.count(g.Centre != nil)
			cov.Yaw.count(g.Yaw != nil)
			cov.Length.count(g.Length != nil)
			cov.Width.count(g.Width != nil)
			cov.Height.count(g.Height != nil)
			cov.Front.count(g.Front != nil)
			cov.Rear.count(g.Rear != nil)
		}
		objects = append(objects, fo)
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].ObjectID < objects[j].ObjectID })
	return objects, cov
}

// newFrozenPhysical pins a saved document.
func newFrozenPhysical(doc *PhysicalReferenceSet) (*FrozenPhysical, error) {
	content, err := doc.ContentDigest()
	if err != nil {
		return nil, err
	}
	objects, coverage := summarisePhysical(doc)
	return &FrozenPhysical{Revision: doc.Revision, SHA256: doc.Digest(), ContentSHA256: content,
		Objects: objects, Coverage: coverage}, nil
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
	SidecarRevision int `json:"sidecar_revision,omitempty"`
	// PhysicalRevision pins a physical-reference revision; zero, or the
	// field absent, freezes the pack's current one. A pack with no physical
	// references freezes with no pin, and cannot name a revision.
	// FeatureRevision is opt-in: absent pins no facets, zero pins their head,
	// and a positive value pins that retained revision.
	FeatureRevision  *int      `json:"feature_revision,omitempty"`
	PhysicalRevision int       `json:"physical_revision,omitempty"`
	Splits           []Split   `json:"splits,omitempty"`
	Episodes         []Episode `json:"episodes,omitempty"`
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
	if !atEndOfJSON(dec) {
		return nil, fmt.Errorf("parse split draft: trailing data after the draft object")
	}
	if d.Schema != SplitDraftSchema || d.SchemaVersion != SplitDraftSchemaVersion {
		return nil, fmt.Errorf("split draft schema %q version %d, want %q version %d",
			d.Schema, d.SchemaVersion, SplitDraftSchema, SplitDraftSchemaVersion)
	}
	return &d, nil
}

// LoadAnySplit reads a split manifest of any version: a hand-written version
// 1 manifest of one pack, or a frozen split of version 2 or 3. Exactly one of
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
	if json.Unmarshal(b, &head) == nil && isFrozenSchemaVersion(head.SchemaVersion) {
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
	if !atEndOfJSON(dec) {
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
// Encoding these types cannot fail: they hold only strings, finite numbers
// (validate refuses the rest, and JSON cannot carry them), bools and slices
// of the same.
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
	if !isFrozenSchemaVersion(f.SchemaVersion) {
		return fmt.Errorf("schema version %d, want %d, %d or %d",
			f.SchemaVersion, FrozenSplitSchemaVersionMembershipOnly, FrozenSplitSchemaVersion, FrozenSplitSchemaVersionFeatures)
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
	if !validGuard(f.GuardSeconds) {
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
	cases, err := validateCases(f.Cases)
	if err != nil {
		return err
	}

	digests := map[string]bool{}
	episodes := map[string]string{}
	for _, p := range f.Packs {
		if err := p.validate(roles, cases); err != nil {
			return fmt.Errorf("pack %s: %w", p.PackDigest, err)
		}
		if f.SchemaVersion == FrozenSplitSchemaVersionMembershipOnly && p.Physical != nil {
			return fmt.Errorf("pack %s: a version %d split carries no physical pin", p.PackDigest, f.SchemaVersion)
		}
		if f.SchemaVersion != FrozenSplitSchemaVersionFeatures && p.Features != nil {
			return fmt.Errorf("a version %d split carries no facet pin", f.SchemaVersion)
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
	if err := checkSourceDisjoint(f.Packs, f.GuardSeconds); err != nil {
		return err
	}
	return f.checkTuned()
}

func validGuard(seconds float64) bool {
	return !math.IsNaN(seconds) && seconds >= 0 && seconds <= maxSplitGuardSeconds
}

// splitCases is a split's cases, by ID and by capture basename.
type splitCases struct {
	byID map[string]SplitCase
	// byBasename maps a capture basename to a case that replays it. Cases
	// that share a capture share a role, so any of them will do.
	byBasename map[string]SplitCase
}

// caseRoles is each case's role by ID. It refuses what validateCases
// refuses of the IDs and roles alone.
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

// validateCases checks the cases and their captures: every case names at
// least one, and no capture, by basename or by content, belongs to two cases
// of different roles, since a pack or a replay of it could then take either.
func validateCases(cases []SplitCase) (splitCases, error) {
	out := splitCases{byID: map[string]SplitCase{}, byBasename: map[string]SplitCase{}}
	if _, err := caseRoles(cases); err != nil {
		return out, err
	}
	bySHA := map[string]SplitCase{}
	for _, c := range cases {
		out.byID[c.CaseID] = c
		if len(c.Captures) == 0 {
			return out, fmt.Errorf("case %q names no capture: a case's role binds to its captures, so declare them "+
				"(basename and sha256), or cut a pack that names the case from one", c.CaseID)
		}
		seen := map[string]bool{}
		for _, cp := range c.Captures {
			if err := cp.validate(); err != nil {
				return out, fmt.Errorf("case %q: %w", c.CaseID, err)
			}
			if seen[cp.Basename] {
				return out, fmt.Errorf("case %q names capture %q twice", c.CaseID, cp.Basename)
			}
			seen[cp.Basename] = true
			if other, ok := out.byBasename[cp.Basename]; ok && other.Role != c.Role {
				return out, fmt.Errorf("capture %q is in %s case %q and %s case %q: one capture cannot take two roles",
					cp.Basename, other.Role, other.CaseID, c.Role, c.CaseID)
			}
			out.byBasename[cp.Basename] = c
			if cp.SHA256 == "" {
				continue
			}
			if other, ok := bySHA[cp.SHA256]; ok && other.Role != c.Role {
				return out, fmt.Errorf("capture %s is in %s case %q and %s case %q: one capture cannot take two roles",
					cp.SHA256, other.Role, other.CaseID, c.Role, c.CaseID)
			}
			bySHA[cp.SHA256] = c
		}
	}
	return out, nil
}

func (c CaseCapture) validate() error {
	if c.Basename == "" || c.Basename == "." || c.Basename == ".." || strings.ContainsAny(c.Basename, `/\`) {
		return fmt.Errorf("capture basename %q is not a file name", c.Basename)
	}
	if c.SHA256 != "" && !isSHA256Digest(c.SHA256) {
		return fmt.Errorf("capture %q sha256 %q is not a sha256:<64 hex> digest", c.Basename, c.SHA256)
	}
	return nil
}

// isSHA256Digest reports whether s is "sha256:" and 64 lower-case hex digits.
func isSHA256Digest(s string) bool {
	hexPart, ok := strings.CutPrefix(s, "sha256:")
	if !ok || len(hexPart) != 64 {
		return false
	}
	for _, r := range hexPart {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// hasBasename reports whether the case names a capture of that basename.
func (c SplitCase) hasBasename(basename string) bool {
	for _, cp := range c.Captures {
		if cp.Basename == basename {
			return true
		}
	}
	return false
}

func (p FrozenPack) validate(roles map[string]SplitRole, cases splitCases) error {
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
	bound, err := p.boundCase(cases)
	if err != nil {
		return err
	}
	if s := p.Selection; s != nil {
		if !strings.HasPrefix(s.SHA256, "sha256:") {
			return fmt.Errorf("selection sha256 %q is not a sha256: digest", s.SHA256)
		}
		if s.Role != string(SplitRoleTuning) && s.Role != string(SplitRoleHeldOut) {
			return fmt.Errorf("selection role %q (want tuning or held_out)", s.Role)
		}
	}
	if p.Physical != nil {
		if err := p.Physical.validate(); err != nil {
			return fmt.Errorf("physical pin: %w", err)
		}
	}
	if p.Features != nil {
		if err := p.Features.validate(); err != nil {
			return fmt.Errorf("facet pin: %w", err)
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
		if bound != nil && role != bound.Role {
			return fmt.Errorf("object %q is in %s partition %q, but case %q is %s: a pack cut from a case's capture (%s) takes the case's role",
				o.ObjectID, role, o.Partition, bound.CaseID, bound.Role, p.Source.PCAPBasename)
		}
	}
	return nil
}

// boundCase is the case whose role the pack's objects take: the case it
// names, which must name the pack's capture, or else any case that replays
// the capture the pack was cut from. A screen case holds no references.
func (p FrozenPack) boundCase(cases splitCases) (*SplitCase, error) {
	var bound *SplitCase
	if p.CaseID != "" {
		c, ok := cases.byID[p.CaseID]
		if !ok {
			return nil, fmt.Errorf("case %q is not declared in the split's cases", p.CaseID)
		}
		if p.Source.PCAPBasename == "" {
			return nil, fmt.Errorf("the pack names case %q but records no capture file (manifest.json pcap_basename), "+
				"so nothing binds it to the case's captures", p.CaseID)
		}
		if !c.hasBasename(p.Source.PCAPBasename) {
			return nil, fmt.Errorf("the pack names case %q, but its capture %q is not one of the case's captures", p.CaseID, p.Source.PCAPBasename)
		}
		bound = &c
	} else if c, ok := cases.byBasename[p.Source.PCAPBasename]; ok && p.Source.PCAPBasename != "" {
		bound = &c
	}
	if bound != nil && bound.Role == SplitRoleScreen {
		return nil, fmt.Errorf("the pack is cut from capture %q of case %q, a screen case: a screen site holds no reference partition",
			p.Source.PCAPBasename, bound.CaseID)
	}
	return bound, nil
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

// withinGuard reports whether two capture spans come within guardSeconds of
// each other: they overlap, or the gap between them is at most the guard.
// Only one span is widened, so the guard is the largest gap still refused.
func withinGuard(aFirst, aLast, bFirst, bLast int64, guardSeconds float64) bool {
	guard := int64(guardSeconds * 1e9)
	return aFirst-guard <= bLast && bFirst-guard <= aLast
}

// checkSourceDisjoint refuses two packs of one source whose capture spans
// come within the guard of each other, unless every object of both sits in
// one partition. Object IDs are pack-local, so nothing else can say whether
// a car in one pack is a car in the other; while their spans could share a
// vehicle, they may only share a partition.
func checkSourceDisjoint(packs []FrozenPack, guardSeconds float64) error {
	for i := range packs {
		for j := i + 1; j < len(packs); j++ {
			a, b := packs[i], packs[j]
			key := a.Source.sourceKey()
			if key == "" || key != b.Source.sourceKey() {
				continue
			}
			if !withinGuard(a.Source.FirstSampleNs, a.Source.LastSampleNs, b.Source.FirstSampleNs, b.Source.LastSampleNs, guardSeconds) {
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
// clear them all, and physical-reference problems beside them.
func FreezeSplit(opts FreezeOptions) (*FrozenSplit, error) {
	a, err := assembleFreeze(opts)
	if err != nil {
		return nil, err
	}
	if err := a.reviewError(); err != nil {
		return nil, err
	}
	if err := a.complete(opts); err != nil {
		return nil, err
	}
	return a.split, nil
}

// FreezePreview is what FreezeSplit would freeze from a draft, and why it
// would refuse, without producing the split: what a Freeze action shows
// before it commits.
type FreezePreview struct {
	Packs []FreezePreviewPack `json:"packs"`
	Cases []SplitCase         `json:"cases,omitempty"`
	// MembershipProblems and PhysicalProblems are the review refusals, one
	// line each, as FreezeSplit would list them. Both empty means review is
	// complete.
	MembershipProblems []string `json:"membership_problems"`
	PhysicalProblems   []string `json:"physical_problems"`
	FacetProblems      []string `json:"facet_problems,omitempty"`
	// Refusal is why a draft whose review is complete still would not
	// freeze: a structural refusal, such as holding out what the lineage
	// tuned on. Empty when review is incomplete or the draft would freeze.
	Refusal     string `json:"refusal,omitempty"`
	WouldFreeze bool   `json:"would_freeze"`
	// SplitDigest is the digest the frozen split would have, when it would
	// freeze from these options: the same options given to FreezeSplit
	// produce the same digest.
	SplitDigest string `json:"split_digest,omitempty"`
}

// FreezePreviewPack is one draft pack as it would be pinned.
type FreezePreviewPack struct {
	// Dir is the pack directory as the draft names it.
	Dir             string `json:"dir"`
	PackDigest      string `json:"pack_digest"`
	DatasetID       string `json:"dataset_id"`
	CaseID          string `json:"case_id,omitempty"`
	SidecarRevision int    `json:"sidecar_revision"`
	SidecarSHA256   string `json:"sidecar_sha256"`
	// Physical is the pin with its summaries, or null for a pack with no
	// physical references.
	Physical *FrozenPhysical `json:"physical"`
	Features *FrozenFeatures `json:"features,omitempty"`
	Objects  []FrozenObject  `json:"objects"`
	Episodes int             `json:"episodes"`
}

// PreviewFreeze runs every check FreezeSplit runs and reports the outcome
// instead of the split. Errors are what FreezeSplit returns before review
// is checked: a draft or pack that cannot be opened or pinned.
func PreviewFreeze(opts FreezeOptions) (*FreezePreview, error) {
	a, err := assembleFreeze(opts)
	if err != nil {
		return nil, err
	}
	pv := &FreezePreview{Packs: []FreezePreviewPack{}, Cases: a.split.Cases,
		MembershipProblems: append([]string{}, a.membership...), PhysicalProblems: append([]string{}, a.physical...), FacetProblems: append([]string{}, a.facets...)}
	for _, p := range a.split.Packs {
		pv.Packs = append(pv.Packs, FreezePreviewPack{
			Dir: a.dirs[p.PackDigest], PackDigest: p.PackDigest, DatasetID: p.DatasetID, CaseID: p.CaseID,
			SidecarRevision: p.SidecarRevision, SidecarSHA256: p.SidecarSHA256, Physical: p.Physical, Features: p.Features,
			Objects: p.Objects, Episodes: len(p.Episodes),
		})
	}
	if a.reviewError() != nil {
		return pv, nil
	}
	if err := a.complete(opts); err != nil {
		pv.Refusal = err.Error()
		return pv, nil
	}
	pv.WouldFreeze, pv.SplitDigest = true, a.split.SplitDigest
	return pv, nil
}

// freezeAssembly is a draft opened, pinned and checked, short of completing
// the split: what FreezeSplit and PreviewFreeze share.
type freezeAssembly struct {
	split *FrozenSplit
	// dirs is each pack's draft directory by pack digest.
	dirs map[string]string
	// membership and physical are the review problems, one line each.
	membership, physical, facets []string
}

// assembleFreeze checks the draft, opens and pins every pack, and collects
// the partitions and review problems. It refuses only what cannot be
// reported as a review problem: a bad draft, a pack that cannot be opened or
// pinned, or a partition with two roles.
func assembleFreeze(opts FreezeOptions) (*freezeAssembly, error) {
	if opts.Draft == nil {
		return nil, fmt.Errorf("no draft to freeze")
	}
	if opts.Now.IsZero() {
		return nil, fmt.Errorf("no freezing time")
	}
	cases, err := draftCases(opts.Draft.Cases)
	if err != nil {
		return nil, err
	}
	f := &FrozenSplit{
		Schema: SplitSchema, SchemaVersion: FrozenSplitSchemaVersion, Revision: 1,
		Frozen: FreezeRecord{
			Author: strings.TrimSpace(opts.Author), FrozenUTC: opts.Now.UTC().Format(time.RFC3339),
			BuildVersion: opts.BuildVersion, BuildGitSHA: opts.BuildGitSHA,
			ForConfigHash: opts.ForConfigHash, ForParamsHash: opts.ForParamsHash,
		},
		Note: opts.Draft.Note, GuardSeconds: opts.GuardSeconds, Cases: cases,
	}
	a := &freezeAssembly{split: f, dirs: map[string]string{}}
	roles := map[string]SplitRole{}
	for i, dp := range opts.Draft.Packs {
		r, err := freezePack(dp, opts.BaseDir)
		if err != nil {
			return nil, fmt.Errorf("draft pack %d (%s): %w", i, dp.Dir, err)
		}
		for name, role := range r.roles {
			if other, ok := roles[name]; ok && other != role {
				return nil, fmt.Errorf("partition %q is %s in one pack and %s in another", name, other, role)
			}
			roles[name] = role
		}
		a.membership = append(a.membership, r.membership...)
		a.physical = append(a.physical, r.physical...)
		a.facets = append(a.facets, r.facets...)
		if r.pack.Features != nil {
			f.SchemaVersion = FrozenSplitSchemaVersionFeatures
		}
		a.dirs[r.pack.PackDigest] = dp.Dir
		f.Packs = append(f.Packs, r.pack)
	}
	for name, role := range roles {
		f.Partitions = append(f.Partitions, SplitPartition{Name: name, Role: role})
	}
	sort.Slice(f.Partitions, func(i, j int) bool { return f.Partitions[i].Name < f.Partitions[j].Name })
	sort.Slice(f.Packs, func(i, j int) bool { return f.Packs[i].PackDigest < f.Packs[j].PackDigest })
	f.deriveCaseCaptures()
	return a, nil
}

// reviewError is the refusal the review problems make, or nil when review
// is complete and every physical reference holds.
func (a *freezeAssembly) reviewError() error {
	var parts []string
	if len(a.membership) > 0 {
		parts = append(parts, "membership review is not complete, so the split cannot be frozen:\n  "+strings.Join(a.membership, "\n  "))
	}
	if len(a.physical) > 0 {
		parts = append(parts, "the physical references do not hold against the pinned annotation revision, so the split cannot be frozen:\n  "+
			strings.Join(a.physical, "\n  "))
	}
	if len(a.facets) > 0 {
		parts = append(parts, "facet proposals do not hold against the pinned membership:\n  "+strings.Join(a.facets, "\n  "))
	}
	if len(parts) == 0 {
		return nil
	}
	return errors.New(strings.Join(parts, "\n"))
}

// complete carries the lineage's tuned record forward, validates the split
// and digests it.
func (a *freezeAssembly) complete(opts FreezeOptions) error {
	f := a.split
	// The record of what the lineage tuned on carries forward, so what a
	// revision drops stays tuned; validate then refuses to hold any of it
	// out.
	var inherited TunedLedger
	if prev := opts.Supersedes; prev != nil {
		f.Revision, f.Supersedes = prev.Revision+1, prev.SplitDigest
		inherited = prev.Tuned
	}
	f.Tuned = mergeLedgers(inherited, f.ownTuning())
	if err := f.validate(); err != nil {
		return err
	}
	f.SplitDigest = f.contentDigest()
	return nil
}

// draftCases copies the draft's cases, sorted by ID. A capture the draft
// declares names the SHA-256 of its file: it is what a replay under the
// split is checked against, and the operator freezing the split has the file
// to hash.
func draftCases(in []SplitCase) ([]SplitCase, error) {
	var out []SplitCase
	for _, c := range in {
		for _, cp := range c.Captures {
			if cp.SHA256 == "" {
				return nil, fmt.Errorf("draft case %q capture %q has no sha256: declare the SHA-256 of each capture file "+
					"(sha256sum), which every replay of the case under the split is checked against", c.CaseID, cp.Basename)
			}
		}
		c.Captures = append([]CaseCapture(nil), c.Captures...)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CaseID < out[j].CaseID })
	return out, nil
}

// deriveCaseCaptures gives a case the capture of every pack that names it,
// by basename, the only identity a pack records, and sorts each case's
// captures. A pack that names an undeclared case, or records no capture, is
// left for validate to refuse.
func (f *FrozenSplit) deriveCaseCaptures() {
	index := map[string]int{}
	for i, c := range f.Cases {
		index[c.CaseID] = i
	}
	for _, p := range f.Packs {
		i, ok := index[p.CaseID]
		if !ok || p.Source.PCAPBasename == "" || f.Cases[i].hasBasename(p.Source.PCAPBasename) {
			continue
		}
		f.Cases[i].Captures = append(f.Cases[i].Captures, CaseCapture{Basename: p.Source.PCAPBasename})
	}
	for i := range f.Cases {
		sortCaptures(f.Cases[i].Captures)
	}
}

func sortCaptures(captures []CaseCapture) {
	sort.Slice(captures, func(i, j int) bool {
		if captures[i].Basename != captures[j].Basename {
			return captures[i].Basename < captures[j].Basename
		}
		return captures[i].SHA256 < captures[j].SHA256
	})
}

// ownTuning is what this revision tunes on, stamped with its revision: each
// pack's tuning objects with the pack's source and capture span, and each
// tuning case with its captures.
func (f *FrozenSplit) ownTuning() TunedLedger {
	roles := map[string]SplitRole{}
	for _, p := range f.Partitions {
		roles[p.Name] = p.Role
	}
	var l TunedLedger
	for _, p := range f.Packs {
		var ids []string
		for _, o := range p.Objects {
			if roles[o.Partition] == SplitRoleTuning {
				ids = append(ids, o.ObjectID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		sort.Strings(ids)
		l.Spans = append(l.Spans, TunedSpan{Revision: f.Revision, PackDigest: p.PackDigest, Source: p.Source.sourceKey(),
			FirstSampleNs: p.Source.FirstSampleNs, LastSampleNs: p.Source.LastSampleNs, GuardSeconds: f.GuardSeconds, ObjectIDs: ids})
	}
	for _, c := range f.Cases {
		if c.Role == SplitRoleTuning {
			l.Cases = append(l.Cases, TunedCase{Revision: f.Revision, CaseID: c.CaseID, Captures: append([]CaseCapture(nil), c.Captures...)})
		}
	}
	return l
}

// key identifies a tuned span: one pack, as cut from one source.
func (s TunedSpan) key() string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%d", s.PackDigest, s.Source, s.FirstSampleNs, s.LastSampleNs)
}

// mergeLedgers is the union of tuned records: a span or case in both keeps
// the earlier revision, the wider guard and every object and capture either
// names. The result is sorted, so it encodes the same whatever the order of
// its parts.
func mergeLedgers(ledgers ...TunedLedger) TunedLedger {
	spans := map[string]*TunedSpan{}
	cases := map[string]*TunedCase{}
	for _, l := range ledgers {
		for _, s := range l.Spans {
			if have, ok := spans[s.key()]; ok {
				have.Revision, have.GuardSeconds = min(have.Revision, s.Revision), max(have.GuardSeconds, s.GuardSeconds)
				have.ObjectIDs = unionSorted(have.ObjectIDs, s.ObjectIDs)
				continue
			}
			s.ObjectIDs = unionSorted(nil, s.ObjectIDs)
			spans[s.key()] = &s
		}
		for _, c := range l.Cases {
			if have, ok := cases[c.CaseID]; ok {
				have.Revision = min(have.Revision, c.Revision)
				have.Captures = unionCaptures(have.Captures, c.Captures)
				continue
			}
			c.Captures = unionCaptures(nil, c.Captures)
			cases[c.CaseID] = &c
		}
	}
	var out TunedLedger
	for _, k := range sortedMapKeys(spans) {
		out.Spans = append(out.Spans, *spans[k])
	}
	for _, k := range sortedMapKeys(cases) {
		out.Cases = append(out.Cases, *cases[k])
	}
	return out
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unionSorted(a, b []string) []string {
	set := map[string]bool{}
	for _, s := range append(append([]string(nil), a...), b...) {
		set[s] = true
	}
	return sortedMapKeys(set)
}

func unionCaptures(a, b []CaseCapture) []CaseCapture {
	var out []CaseCapture
	for _, c := range append(append([]CaseCapture(nil), a...), b...) {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	sortCaptures(out)
	return out
}

func capturesHaveBasename(captures []CaseCapture, basename string) bool {
	for _, c := range captures {
		if c.Basename == basename {
			return true
		}
	}
	return false
}

// sameCapture reports whether two captures are one file: by content when
// both name it, else by basename.
func sameCapture(a, b CaseCapture) bool {
	if a.SHA256 != "" && b.SHA256 != "" {
		return a.SHA256 == b.SHA256
	}
	return a.Basename == b.Basename
}

// checkTuned checks the Tuned record: well formed, holding this revision's
// own tuning, and holding nothing this revision holds out.
func (f *FrozenSplit) checkTuned() error {
	spans := map[string]TunedSpan{}
	for i, s := range f.Tuned.Spans {
		if err := s.validate(f.Revision); err != nil {
			return fmt.Errorf("tuned span %d: %w", i, err)
		}
		if _, dup := spans[s.key()]; dup {
			return fmt.Errorf("tuned span %d repeats pack %s over the same capture span", i, s.PackDigest)
		}
		spans[s.key()] = s
	}
	cases := map[string]TunedCase{}
	for i, c := range f.Tuned.Cases {
		if c.Revision < 1 || c.Revision > f.Revision {
			return fmt.Errorf("tuned case %d: revision %d is outside this lineage's 1 to %d", i, c.Revision, f.Revision)
		}
		if strings.TrimSpace(c.CaseID) == "" {
			return fmt.Errorf("tuned case %d has no id", i)
		}
		if _, dup := cases[c.CaseID]; dup {
			return fmt.Errorf("tuned case %q is recorded twice", c.CaseID)
		}
		if len(c.Captures) == 0 {
			return fmt.Errorf("tuned case %q names no capture", c.CaseID)
		}
		for _, cp := range c.Captures {
			if err := cp.validate(); err != nil {
				return fmt.Errorf("tuned case %q: %w", c.CaseID, err)
			}
		}
		cases[c.CaseID] = c
	}
	own := f.ownTuning()
	for _, s := range own.Spans {
		have, ok := spans[s.key()]
		if !ok || have.GuardSeconds < s.GuardSeconds || len(unionSorted(have.ObjectIDs, s.ObjectIDs)) != len(have.ObjectIDs) {
			return fmt.Errorf("the tuned record does not hold pack %s's tuning in this revision", s.PackDigest)
		}
	}
	for _, c := range own.Cases {
		have, ok := cases[c.CaseID]
		if !ok || len(unionCaptures(have.Captures, c.Captures)) != len(have.Captures) {
			return fmt.Errorf("the tuned record does not hold case %q, tuned on in this revision", c.CaseID)
		}
	}
	return f.checkHeldOutAgainstTuned()
}

func (s TunedSpan) validate(revision int) error {
	if s.Revision < 1 || s.Revision > revision {
		return fmt.Errorf("revision %d is outside this lineage's 1 to %d", s.Revision, revision)
	}
	if !strings.HasPrefix(s.PackDigest, "sha256:") {
		return fmt.Errorf("pack_digest %q is not a sha256: digest", s.PackDigest)
	}
	if s.FirstSampleNs > s.LastSampleNs {
		return fmt.Errorf("capture span [%d, %d] ends before it starts", s.FirstSampleNs, s.LastSampleNs)
	}
	if !validGuard(s.GuardSeconds) {
		return fmt.Errorf("guard_seconds %g must be between 0 and %d", s.GuardSeconds, maxSplitGuardSeconds)
	}
	if len(s.ObjectIDs) == 0 {
		return fmt.Errorf("pack %s names no tuned object", s.PackDigest)
	}
	if ids := unionSorted(nil, s.ObjectIDs); len(ids) != len(s.ObjectIDs) || ids[0] == "" {
		return fmt.Errorf("pack %s's tuned object ids must be distinct and non-empty", s.PackDigest)
	}
	return nil
}

// checkHeldOutAgainstTuned refuses to hold out anything the lineage tuned
// on: an object it holds, a pack of the same source within the guard of a
// tuned pack, a pack cut from a tuned case's capture, a tuned case by ID or
// by capture, and a case one of whose captures a tuned pack was cut from.
func (f *FrozenSplit) checkHeldOutAgainstTuned() error {
	roles := map[string]SplitRole{}
	for _, p := range f.Partitions {
		roles[p.Name] = p.Role
	}
	for _, p := range f.Packs {
		var held []string
		for _, o := range p.Objects {
			if roles[o.Partition] == SplitRoleHeldOut {
				held = append(held, o.ObjectID)
			}
		}
		if len(held) == 0 {
			continue
		}
		key := p.Source.sourceKey()
		for _, s := range f.Tuned.Spans {
			if s.PackDigest == p.PackDigest {
				for _, id := range held {
					if slices.Contains(s.ObjectIDs, id) {
						return fmt.Errorf("pack %s object %s is held out, but revision %d of this split's lineage tuned on it: "+
							"no later revision can make it unseen", p.PackDigest, id, s.Revision)
					}
				}
				continue
			}
			guard := max(f.GuardSeconds, s.GuardSeconds)
			if key != "" && key == s.Source &&
				withinGuard(p.Source.FirstSampleNs, p.Source.LastSampleNs, s.FirstSampleNs, s.LastSampleNs, guard) {
				return fmt.Errorf("pack %s holds out object %s, but pack %s, tuned on in revision %d, is cut from %s within %gs of it: "+
					"the same physical object could have been tuned on", p.PackDigest, held[0], s.PackDigest, s.Revision, key, guard)
			}
		}
		for _, c := range f.Tuned.Cases {
			if b := p.Source.PCAPBasename; b != "" && capturesHaveBasename(c.Captures, b) {
				return fmt.Errorf("pack %s holds out object %s, but it is cut from capture %s of case %q, tuned on in revision %d",
					p.PackDigest, held[0], b, c.CaseID, c.Revision)
			}
		}
	}
	for _, c := range f.Cases {
		if c.Role != SplitRoleHeldOut {
			continue
		}
		for _, t := range f.Tuned.Cases {
			if t.CaseID == c.CaseID {
				return fmt.Errorf("case %q is held out, but revision %d of this split's lineage tuned on it: "+
					"no later revision can make it unseen", c.CaseID, t.Revision)
			}
			for _, a := range c.Captures {
				for _, b := range t.Captures {
					if sameCapture(a, b) {
						return fmt.Errorf("case %q is held out, but its capture %s is case %q's, tuned on in revision %d",
							c.CaseID, a.Basename, t.CaseID, t.Revision)
					}
				}
			}
		}
		for _, s := range f.Tuned.Spans {
			for _, a := range c.Captures {
				if s.Source == "pcap:"+a.Basename {
					return fmt.Errorf("case %q is held out, but pack %s, tuned on in revision %d, is cut from its capture %s",
						c.CaseID, s.PackDigest, s.Revision, a.Basename)
				}
			}
		}
	}
	return nil
}

// frozenPackResult is one draft pack pinned: its partition roles, and its
// membership and physical-reference problems, kept apart from errors so
// every pack's problems can be reported together.
type frozenPackResult struct {
	pack                         FrozenPack
	roles                        map[string]SplitRole
	membership, physical, facets []string
}

// freezePack opens one draft pack, pins it and checks its review.
func freezePack(dp DraftPack, baseDir string) (frozenPackResult, error) {
	resolve := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(baseDir, path)
	}
	var none frozenPackResult
	if dp.Dir == "" {
		return none, fmt.Errorf("no pack dir")
	}
	// Digest the manifest before opening, so the digest pinned is of the
	// bytes the opened pack was read from; Bind checks it again.
	manifestDigest, err := packManifestDigest(resolve(dp.Dir))
	if err != nil {
		return none, err
	}
	pack, err := OpenPack(resolve(dp.Dir))
	if err != nil {
		return none, err
	}
	m := &SplitManifest{
		Schema: SplitSchema, SchemaVersion: SplitSchemaVersion, PackDigest: pack.Manifest.PackDigest,
		DatasetID: pack.Manifest.DatasetID, SidecarRevision: dp.SidecarRevision, Splits: dp.Splits, Episodes: dp.Episodes,
	}
	if dp.SplitManifest != "" {
		if dp.SidecarRevision != 0 || len(dp.Splits) != 0 || len(dp.Episodes) != 0 {
			return none, fmt.Errorf("give split_manifest or inline splits, episodes and revision, not both")
		}
		if m, err = LoadSplitManifest(resolve(dp.SplitManifest)); err != nil {
			return none, err
		}
	} else if err := m.validateStructure(); err != nil {
		return none, err
	}

	var s *Sidecar
	if m.SidecarRevision > 0 {
		s, err = LoadSidecarRevision(pack, m.SidecarRevision)
	} else {
		s, err = LoadSidecar(pack)
	}
	if err != nil {
		return none, fmt.Errorf("load annotation: %w", err)
	}
	if s.baseDigest == "" {
		return none, fmt.Errorf("the pack has no saved annotation to freeze")
	}
	m.SidecarRevision = s.Revision
	if err := m.ValidateAgainst(pack, s); err != nil {
		return none, err
	}

	selection, err := readSelection(pack)
	if err != nil {
		return none, err
	}
	physical, physicalProblems, err := freezePhysical(pack, s, dp.PhysicalRevision)
	if err != nil {
		return none, err
	}
	features, facetProblems, err := freezeFeatures(pack, s, dp.FeatureRevision)
	if err != nil {
		return none, err
	}
	p := FrozenPack{
		PackDigest: pack.Manifest.PackDigest, DatasetID: pack.Manifest.DatasetID, CaseID: dp.CaseID,
		ManifestSHA256: manifestDigest, Selection: selection, Source: frozenSource(pack),
		SidecarRevision: s.Revision, SidecarSHA256: s.baseDigest, Episodes: m.Episodes, Physical: physical, Features: features,
	}
	roles := map[string]SplitRole{}
	for _, split := range m.Splits {
		roles[split.Name] = split.Role
		for _, id := range split.ObjectIDs {
			p.Objects = append(p.Objects, frozenObject(s, id, split.Name))
		}
	}
	sort.Slice(p.Objects, func(i, j int) bool { return p.Objects[i].ObjectID < p.Objects[j].ObjectID })
	return frozenPackResult{pack: p, roles: roles, membership: reviewProblems(pack.Manifest.PackDigest, s, m),
		physical: physicalProblems, facets: facetProblems}, nil
}

// freezePhysical pins the pack's physical references at the revision the
// draft names, or the head, and lists why the document does not hold
// against the pinned annotation revision. A pack with no references pins
// nothing, and cannot name a revision.
func freezePhysical(pack *Pack, s *Sidecar, revision int) (*FrozenPhysical, []string, error) {
	doc, err := LoadPhysicalReferences(pack)
	if err != nil {
		return nil, nil, fmt.Errorf("load physical references: %w", err)
	}
	if doc.Digest() == "" {
		if revision != 0 {
			return nil, nil, fmt.Errorf("physical_revision %d is named, but the pack has no physical references to pin", revision)
		}
		return nil, nil, nil
	}
	if revision != 0 && revision != doc.Revision {
		if doc, err = LoadPhysicalReferenceRevision(pack, revision); err != nil {
			return nil, nil, fmt.Errorf("load physical reference revision %d: %w", revision, err)
		}
	}
	pin, err := newFrozenPhysical(doc)
	if err != nil {
		return nil, nil, err
	}
	return pin, physicalProblems(pack, s, doc), nil
}

// physicalProblems lists the records of a document that do not hold against
// the annotation revision a split pins: a link that no longer holds, or a
// review made against other membership in a frame it rests on. Empty means
// the document holds; a component honestly stated unknown is not a problem.
func physicalProblems(pack *Pack, s *Sidecar, doc *PhysicalReferenceSet) []string {
	var out []string
	for _, lp := range append(doc.LinkProblems(pack, s), doc.ReviewDrift(pack, s)...) {
		out = append(out, fmt.Sprintf("pack %s physical revision %d %s: %s", pack.Manifest.PackDigest, doc.Revision, lp.Record, lp.Problem))
	}
	sort.Strings(out)
	return out
}

// frozenSource is the pack's source as its manifest declares it, with the
// capture span of its samples.
func frozenSource(pack *Pack) FrozenSource {
	src := pack.Manifest.Source
	first, last := sampleSpan(pack.Samples)
	return FrozenSource{
		PCAPBasename: src.PCAPBasename, VRLOGHeaderSHA: src.VRLOGHeaderSHA, VRLOGFramesSHA: src.VRLOGFramesSHA,
		SensorID: src.SensorID, ConfigHash: src.ConfigHash, ParamsHash: src.ParamsHash, BuildGitSHA: src.BuildGitSHA,
		FirstSampleNs: first, LastSampleNs: last,
	}
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

// Bind checks the frozen split against one of its packs, as an evaluator
// opens it, and returns that pack's partition in version 1 form with the
// annotation revision it pins. Every pin is re-checked: the pack's manifest
// and selection record, the pinned revision's bytes, the objects that
// revision still carries, and the membership review the freeze certified.
// What the file copies from those pinned bytes, the selection record's
// content, the source and capture span and each object's review counts, is
// derived from them again and must agree, so an edit that recomputed the
// split digest cannot change it.
func (f *FrozenSplit) Bind(p *Pack) (*SplitManifest, *Sidecar, error) {
	entry, err := f.entry(p)
	if err != nil {
		return nil, nil, err
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
	if source := frozenSource(p); source != entry.Source {
		return nil, nil, fmt.Errorf("pack %s: the frozen split's source %+v is not the pack's %+v, which its pinned manifest and samples give",
			entry.PackDigest, entry.Source, source)
	}
	selection, err := readSelection(p)
	if err != nil {
		return nil, nil, err
	}
	if (selection == nil) != (entry.Selection == nil) || (selection != nil && selection.SHA256 != entry.Selection.SHA256) {
		return nil, nil, fmt.Errorf("pack %s selection record changed after freezing", entry.PackDigest)
	}
	if selection != nil && *selection != *entry.Selection {
		return nil, nil, fmt.Errorf("pack %s: the frozen split's selection %+v is not the pinned record's %+v",
			entry.PackDigest, *entry.Selection, *selection)
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
	for _, o := range entry.Objects {
		if derived := frozenObject(s, o.ObjectID, o.Partition); derived != o {
			return nil, nil, fmt.Errorf("pack %s object %s: the frozen split records %+v, but the pinned revision gives %+v",
				entry.PackDigest, o.ObjectID, o, derived)
		}
	}
	return view, s, nil
}

// entry is the split's record of a pack.
func (f *FrozenSplit) entry(p *Pack) (*FrozenPack, error) {
	for i := range f.Packs {
		if f.Packs[i].PackDigest == p.Manifest.PackDigest {
			return &f.Packs[i], nil
		}
	}
	return nil, fmt.Errorf("pack %s is not in frozen split %s", p.Manifest.PackDigest, f.SplitDigest)
}

// BindPhysical loads the physical-reference revision the split pins for a
// pack, or nil for a pack it pins none for, and holds the revision to the
// pin: its exact bytes and its content must have the pinned digests, its
// links must hold against the pinned annotation revision, its reviews must
// rest on that membership, and the summaries the file copies must derive
// again from it. s is the annotation revision Bind returned; the head is
// never consulted, so a membership save after freezing changes nothing here.
func (f *FrozenSplit) BindPhysical(p *Pack, s *Sidecar) (*PhysicalReferenceSet, error) {
	entry, err := f.entry(p)
	if err != nil {
		return nil, err
	}
	pin := entry.Physical
	if pin == nil {
		return nil, nil
	}
	if s == nil || s.Revision != entry.SidecarRevision || s.baseDigest != entry.SidecarSHA256 {
		return nil, fmt.Errorf("pack %s: physical references bind against the pinned annotation revision %d (%s), which Bind returns, "+
			"not the annotation given", entry.PackDigest, entry.SidecarRevision, entry.SidecarSHA256)
	}
	doc, err := LoadPhysicalReferenceRevision(p, pin.Revision)
	if err != nil {
		return nil, fmt.Errorf("load pinned physical reference revision %d: %w", pin.Revision, err)
	}
	if doc.Digest() != pin.SHA256 {
		return nil, fmt.Errorf("physical reference revision %d of pack %s is not the bytes that were frozen (%s, now %s): "+
			"a changed reference is a new revision; freeze a new split revision against it",
			pin.Revision, entry.PackDigest, pin.SHA256, doc.Digest())
	}
	content, err := doc.ContentDigest()
	if err != nil {
		return nil, err
	}
	if content != pin.ContentSHA256 {
		return nil, fmt.Errorf("physical reference revision %d of pack %s has content %s, but it was frozen as %s",
			pin.Revision, entry.PackDigest, content, pin.ContentSHA256)
	}
	if problems := physicalProblems(p, s, doc); len(problems) > 0 {
		return nil, fmt.Errorf("the pinned physical references do not hold against the pinned annotation revision:\n  %s",
			strings.Join(problems, "\n  "))
	}
	objects, coverage := summarisePhysical(doc)
	if !slices.Equal(objects, pin.Objects) || coverage != pin.Coverage {
		return nil, fmt.Errorf("pack %s: the frozen split summarises physical revision %d as %+v with coverage %+v, "+
			"but the pinned bytes give %+v with coverage %+v",
			entry.PackDigest, pin.Revision, pin.Objects, pin.Coverage, objects, coverage)
	}
	return doc, nil
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

// CheckCaseCaptures refuses a replay of the named case unless every capture
// file it replays is one of the case's captures: by SHA-256 when the case
// declares it, by basename only for a capture the case gained from a pack.
// A case's role binds to its captures, so a capture replayed under another
// case's name cannot take that case's role. Files are hashed only when the
// case declares a digest to compare against.
func (f *FrozenSplit) CheckCaseCaptures(caseID string, paths []string) error {
	var c *SplitCase
	for i := range f.Cases {
		if f.Cases[i].CaseID == caseID {
			c = &f.Cases[i]
		}
	}
	if c == nil {
		return fmt.Errorf("case %q has no role in frozen split %s", caseID, f.SplitDigest)
	}
	if len(paths) == 0 {
		return fmt.Errorf("case %q: no capture to check against frozen split %s", caseID, f.SplitDigest)
	}
	hash := false
	for _, cp := range c.Captures {
		hash = hash || cp.SHA256 != ""
	}
	for _, path := range paths {
		file := CaseCapture{Basename: filepath.Base(path)}
		if hash {
			sum, err := captureFileSHA256(path)
			if err != nil {
				return fmt.Errorf("case %q: hash capture: %w", caseID, err)
			}
			file.SHA256 = sum
		}
		if !c.replays(file) {
			return fmt.Errorf("capture %s (%s %s) is not one of case %q's captures in frozen split %s (%s): "+
				"a case's role binds to its captures, not to its name", path, file.Basename, file.SHA256, caseID, f.SplitDigest, describeCaptures(c.Captures))
		}
	}
	return nil
}

// replays reports whether the file, its basename and (when the case declares
// any digest) its SHA-256, is one of the case's captures. A declared digest
// must match; a capture with none matches by basename.
func (c SplitCase) replays(file CaseCapture) bool {
	for _, cp := range c.Captures {
		if (cp.SHA256 != "" && cp.SHA256 == file.SHA256) || (cp.SHA256 == "" && cp.Basename == file.Basename) {
			return true
		}
	}
	return false
}

func describeCaptures(captures []CaseCapture) string {
	parts := make([]string, len(captures))
	for i, cp := range captures {
		parts[i] = cp.Basename
		if cp.SHA256 != "" {
			parts[i] += " " + cp.SHA256
		}
	}
	return strings.Join(parts, ", ")
}

// captureFileSHA256 is the SHA-256 of a capture file's content, as
// "sha256:<hex>", streamed: a capture runs to hundreds of megabytes.
func captureFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
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
