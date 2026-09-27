package l4bobserve

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Capability names one kind of evidence a record family can reproduce. A
// reader asks for the capabilities its experiment depends on, not for a format
// version: two families can share a codec and still differ in what they can
// prove. The string values are the durable identifiers a later manifest and
// protobuf mapping carry, so they are append-only.
type Capability string

const (
	// CapabilityClusterSummaries is L4's per-cluster centroid, extent, counts
	// and height/intensity descriptors, computed from the members DBSCAN
	// clustered. Summaries are observations, not corrected body centres.
	CapabilityClusterSummaries Capability = "cluster-summaries"
	// CapabilityClusterSample is a content-seeded sample of each accepted
	// cluster's members, capped at 1,024 points. A sample is not membership:
	// it cannot say which returns were left out or why.
	CapabilityClusterSample Capability = "cluster-retained-sample"
	// CapabilityPointFloat64Geometry means retained XYZ are the float64 values
	// L4 computed, not a float32 display copy.
	CapabilityPointFloat64Geometry Capability = "point-float64-geometry"
	CapabilityPointAcquisitionTime Capability = "point-acquisition-time"
	CapabilityPointIntensity       Capability = "point-intensity"
	CapabilityPointChannel         Capability = "point-channel"
	// CapabilityPointSourceOrdinal means each retained return carries its
	// index in the L2 frame, assigned before any filtering.
	CapabilityPointSourceOrdinal Capability = "point-source-ordinal"
	// CapabilityFrameRecords means every frame the extractor received has a
	// record in source order, including empty, unsettled, suppressed and
	// failed ones. Without it, an absent frame cannot be told from a quiet one.
	CapabilityFrameRecords Capability = "frame-records"
	// CapabilityGapRecords means missing intervals are explicit records with
	// bounds and certainty, never inferred from silence.
	CapabilityGapRecords Capability = "gap-records"
	// CapabilityStageCounts means per-stage input, output and rejection counts
	// are recorded where known, with unknown distinct from zero.
	CapabilityStageCounts Capability = "stage-counts"
	// CapabilityCompleteForeground means the retained domain is every
	// L3-labelled foreground return in the frame, before destructive
	// ground/height filtering, voxel reduction or per-cluster caps.
	CapabilityCompleteForeground Capability = "complete-l3-foreground"
	// CapabilityClusterMembership means clusters refer to sorted, unique
	// retained-point indices that form an exclusive partition together with
	// the unassigned points.
	CapabilityClusterMembership Capability = "cluster-membership"
	// CapabilityUnassignedPoints means points outside every cluster are listed
	// with a rejection reason where the extractor knows one.
	CapabilityUnassignedPoints Capability = "unassigned-points"
)

// ErrMissingCapability is matched by every MissingCapabilitiesError, so a
// caller can branch with errors.Is without depending on which were missing.
var ErrMissingCapability = errors.New("evidence lacks a required capability")

// MissingCapabilitiesError names exactly which requested capabilities the
// evidence cannot supply. Reporting the full list, not the first miss, lets an
// operator see at once whether a re-extraction from PCAP would be enough.
type MissingCapabilitiesError struct {
	Profile ProfileName // empty when the set was not a named profile
	Missing []Capability
}

func (e *MissingCapabilitiesError) Error() string {
	names := make([]string, len(e.Missing))
	for i, capability := range e.Missing {
		names[i] = string(capability)
	}
	if e.Profile == "" {
		return fmt.Sprintf("evidence lacks required capabilities: %s", strings.Join(names, ", "))
	}
	return fmt.Sprintf("evidence profile %q lacks required capabilities: %s", e.Profile, strings.Join(names, ", "))
}

// Is reports ErrMissingCapability.
func (e *MissingCapabilitiesError) Is(target error) bool { return target == ErrMissingCapability }

// CapabilitySet is an immutable, sorted set of capabilities. Its zero value
// is empty and supplies nothing.
type CapabilitySet struct{ items []Capability }

// NewCapabilitySet returns the set of the given capabilities, sorted and
// deduplicated so equal sets compare and serialise identically.
func NewCapabilitySet(capabilities ...Capability) CapabilitySet {
	items := slices.Clone(capabilities)
	slices.Sort(items)
	return CapabilitySet{items: slices.Compact(items)}
}

// Has reports whether the set supplies capability.
func (s CapabilitySet) Has(capability Capability) bool {
	_, found := slices.BinarySearch(s.items, capability)
	return found
}

// List returns an owned, sorted copy of the capabilities.
func (s CapabilitySet) List() []Capability { return slices.Clone(s.items) }

// Require returns a *MissingCapabilitiesError naming every required
// capability the set lacks, or nil when all are present.
func (s CapabilitySet) Require(required ...Capability) error {
	return s.require("", required)
}

func (s CapabilitySet) require(profile ProfileName, required []Capability) error {
	var missing []Capability
	for _, capability := range NewCapabilitySet(required...).items {
		if !s.Has(capability) {
			missing = append(missing, capability)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &MissingCapabilitiesError{Profile: profile, Missing: missing}
}

// ProfileName identifies a declared evidence profile. Like capabilities, the
// values are durable identifiers.
type ProfileName string

const (
	// ProfileReducedClusterSample is what the schema-1 JSON observation records
	// in SQLite are: one record per accepted cluster with its summary and a
	// capped, content-seeded retained sample. There are no frame records, so
	// empty frames and gaps are invisible, and no membership or lineage. The
	// cap (1,024 in the domain, often configured at 256 or 64) survives any
	// migration: a later container cannot turn a sample into complete evidence.
	ProfileReducedClusterSample ProfileName = "reduced-cluster-sample"
	// ProfileForegroundComplete is the accuracy-development profile: every
	// frame recorded in source order, and within each frame every L3-labelled
	// foreground return before destructive filtering, with acquisition
	// lineage, stage counts and an exclusive cluster/unassigned partition.
	ProfileForegroundComplete ProfileName = "foreground-complete"
)

// Profile is a named, declared capability set. The name is a label for
// people and manifests; decisions are made on the capabilities.
type Profile struct {
	Name         ProfileName
	Capabilities CapabilitySet
}

// Require names the profile in any MissingCapabilitiesError it returns.
func (p Profile) Require(required ...Capability) error {
	return p.Capabilities.require(p.Name, required)
}

// Satisfies reports whether evidence of profile p can serve a request for
// profile want, naming every capability of want that p lacks.
func (p Profile) Satisfies(want Profile) error {
	return p.Require(want.Capabilities.items...)
}

// ReducedClusterSample returns the legacy reduced profile. Functions rather
// than package variables keep a caller from redefining a declared profile.
func ReducedClusterSample() Profile {
	return Profile{Name: ProfileReducedClusterSample, Capabilities: NewCapabilitySet(
		CapabilityClusterSummaries,
		CapabilityClusterSample,
		CapabilityPointFloat64Geometry,
		CapabilityPointAcquisitionTime,
		CapabilityPointIntensity,
	)}
}

// ForegroundComplete returns the full accuracy profile. It deliberately does
// not claim CapabilityClusterSample: the complete membership supersedes a
// sample, but it does not reproduce the legacy sample's exact draw.
func ForegroundComplete() Profile {
	return Profile{Name: ProfileForegroundComplete, Capabilities: NewCapabilitySet(
		CapabilityClusterSummaries,
		CapabilityPointFloat64Geometry,
		CapabilityPointAcquisitionTime,
		CapabilityPointIntensity,
		CapabilityPointChannel,
		CapabilityPointSourceOrdinal,
		CapabilityFrameRecords,
		CapabilityGapRecords,
		CapabilityStageCounts,
		CapabilityCompleteForeground,
		CapabilityClusterMembership,
		CapabilityUnassignedPoints,
	)}
}

// LookupProfile resolves a declared profile name. An unknown name is an error:
// a reader must not guess what a newer writer's profile promises.
func LookupProfile(name ProfileName) (Profile, error) {
	switch name {
	case ProfileReducedClusterSample:
		return ReducedClusterSample(), nil
	case ProfileForegroundComplete:
		return ForegroundComplete(), nil
	default:
		return Profile{}, fmt.Errorf("unknown evidence profile %q", name)
	}
}
