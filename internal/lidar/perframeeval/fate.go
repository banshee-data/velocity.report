package perframeeval

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

// The fate of every reviewed return: experiment geom0004's measure. For each
// return of a scored road-user mask in the selected episodes, frame by frame,
// it asks which stage of a replay lost it: L3 (the return is not in the
// frame's foreground), one of L4's filters (the rejection reason the
// foreground-complete observation container records for it), or none (it is
// in an accepted cluster, whose identity is kept so fragments are countable).
// The retention measure (retention.go) can say only that a return reached no
// cluster; this one says where it stopped.
//
// Evidence. A foreground-complete container (velocity lidar pcap-replay
// --observations) holds one record per frame the replay assembled. Its
// retained domain is every return the replay's L3 called foreground, before
// the height band, voxel reduction and the DBSCAN input cap, with each
// return's packet sequence, block and channel; clusters list their members
// in that domain and every other return carries a rejection reason. The
// container must declare the capabilities FateCapabilities names, or the
// measure is refused with l4bobserve's MissingCapabilitiesError.
//
// Joining. A version 2 pack cites each return as (packet key, block,
// channel), the key being the first 64 bits of SHA-256 over the packet's UDP
// payload. The domain cites it as (packet sequence, block, channel), the
// sequence being the number a sequence-enabled Pandar40P packet carries in
// its last four bytes. The bridge between the two is the capture file the
// container was extracted from: every packet in it has both a key and a
// sequence. The capture is read once, its SHA-256 checked against the
// container's manifest, and each pack return is then looked up by exact
// identity in the domain of the frame aligned with its sample. Coordinates
// play no part in the join. They are compared afterwards as a check: a
// matched return's float32 coordinates should be bit-identical to the pack's.
//
// A reviewed return is then
//
//   - matched: found in the frame's domain, with the fate the record gives it;
//   - proven absent: its identity resolved, but in neither the aligned
//     frame's domain nor either neighbouring frame's, and both neighbours
//     were read: each exists, carries its domain, and no gap record lies
//     between it and the aligned frame. The domain is complete foreground,
//     so the replay's L3 called the return background: l3_background. In a
//     frame that lost packets the return may instead never have reached L3;
//     the report counts measured frames by completeness and says so when any
//     is not complete;
//   - other_frame: found in a neighbouring frame's domain, so the replay put
//     the return in a different rotation from the recording. It is reported,
//     not attributed;
//   - unresolved: its packet is not in the capture, or its sequence is
//     carried by more than one packet, or two domain returns share its
//     identity; or it is absent from the aligned frame and from every
//     neighbour that could be read, but a neighbour could not be: the
//     aligned frame is the container's first or last, a neighbour carries
//     no returns (a failed frame, or one suppressed before L3), or a gap
//     record lies between them. Such a return may be in the rotation that
//     could not be read, so it is not counted as L3's. Each cause is
//     counted; when both neighbours fail, the predecessor's cause is.
//
// The match rate is matched plus proven absent over the measured returns.
//
// Frames. A pack sample is aligned with the container frame nearest its time
// within the frame tolerance, as the retention measure aligns evidence
// frames. A sample is not measured, and its instants are counted by reason
// rather than scored, when no frame lies within the tolerance (no_frame, or
// gap when the container records a gap there), when it lies outside the
// container's frames, when its frame precedes the replay's scoring start
// (warm_up: the capture's first packet plus the manifest's window offset),
// or when the frame is unsettled, suppressed or failed. Those are not losses.
//
// Relation to the retention measure. A frame in which L4 accepted no cluster
// has no row in the retention measure's evidence, so retention cannot
// measure its instants. Over the instants whose frame holds at least one
// cluster (the comparable rows of the report), the clustered share here is
// the exact count of which retention's is an estimate: equal where every
// cluster kept all its members in its sample, a Horvitz-Thompson expansion
// of it where a cluster was capped.
//
// Limits.
//
//   - The pack is foreground-only: a mask can hold only returns the recording
//     run's L3 called foreground. l3_background means this replay's L3
//     disagreed with the recording's, not that every return of the object is
//     accounted for.
//   - cluster_shape is the record's single reason for a DBSCAN cluster the
//     diameter or aspect filters rejected; it does not say which. A group too
//     small for DBSCAN's minimum points is cluster_noise.
//   - voxel_reduced returns are represented by another return of their voxel,
//     which may be clustered; the record keeps no contributor map.
//   - The capture must be the container's single source file.

// Fate report format.
const (
	FateSchema        = "velocity.report/return-fate"
	FateSchemaVersion = 1
)

// FateMethod names the join the report used.
const FateMethod = "packet_identity_via_capture_sequence"

// Fates, in report order. The rejection fates are the container's rejection
// reasons (l4bobserve.RejectionReason), one for one.
const (
	FateClustered           = "clustered"
	FateL3Background        = "l3_background"
	FateHeightBand          = "height_band"
	FateSurfaceBelowFloor   = "surface_below_floor"
	FateSurfaceAboveCeiling = "surface_above_ceiling"
	FateVoxelReduced        = "voxel_reduced"
	FateClusterInputCap     = "cluster_input_cap"
	FateClusterNoise        = "cluster_noise"
	FateClusterShape        = "cluster_shape"
	FateRejectionUnknown    = "rejection_unknown"
	FateOtherFrame          = "other_frame"
	FateUnresolved          = "unresolved"
)

// Reasons a sample's instants are not measured.
const (
	NotMeasuredOutsideContainer = "outside_container"
	NotMeasuredWarmUp           = "warm_up"
	NotMeasuredNoFrame          = "no_frame"
	NotMeasuredGap              = "gap"
	NotMeasuredUnsettled        = "unsettled"
	NotMeasuredSuppressed       = "suppressed"
	NotMeasuredFailed           = "failed"
)

// Causes of an unresolved return. The first three are failures of the join;
// the last three say why a return absent from its frame could not be proven
// absent, because a neighbouring frame could not be read.
const (
	UnresolvedNotInCapture      = "packet_not_in_capture"
	UnresolvedAmbiguousSequence = "sequence_shared_by_packets"
	UnresolvedAmbiguousDomain   = "identity_repeated_in_domain"
	UnresolvedNoNeighbour       = "no_neighbouring_frame"
	UnresolvedNeighbourNoPoints = "neighbouring_frame_without_returns"
	UnresolvedGapBesideFrame    = "gap_beside_frame"
)

// neighbourCauses are the unresolved causes that a neighbour could not be
// read, in the order the caveat names them.
var neighbourCauses = []string{UnresolvedNoNeighbour, UnresolvedNeighbourNoPoints, UnresolvedGapBesideFrame}

type fate uint8

const (
	fateClustered fate = iota
	fateL3Background
	fateHeightBand
	fateSurfaceBelowFloor
	fateSurfaceAboveCeiling
	fateVoxelReduced
	fateClusterInputCap
	fateClusterNoise
	fateClusterShape
	fateRejectionUnknown
	fateOtherFrame
	fateUnresolved
	numFates
)

var fateNames = [numFates]string{
	FateClustered, FateL3Background, FateHeightBand, FateSurfaceBelowFloor, FateSurfaceAboveCeiling,
	FateVoxelReduced, FateClusterInputCap, FateClusterNoise, FateClusterShape, FateRejectionUnknown,
	FateOtherFrame, FateUnresolved,
}

// FateNames lists every fate in report order.
func FateNames() []string { return slices.Clone(fateNames[:]) }

// rejectionFate maps a recorded rejection reason to its fate. A reason this
// build does not know is reported as unknown rather than guessed.
func rejectionFate(r l4bobserve.RejectionReason) fate {
	switch r {
	case l4bobserve.RejectionHeightBand:
		return fateHeightBand
	case l4bobserve.RejectionSurfaceBelowFloor:
		return fateSurfaceBelowFloor
	case l4bobserve.RejectionSurfaceAboveCeiling:
		return fateSurfaceAboveCeiling
	case l4bobserve.RejectionVoxelReduced:
		return fateVoxelReduced
	case l4bobserve.RejectionClusterInputCap:
		return fateClusterInputCap
	case l4bobserve.RejectionClusterNoise:
		return fateClusterNoise
	case l4bobserve.RejectionClusterShape:
		return fateClusterShape
	}
	return fateRejectionUnknown
}

// FateCapabilities are what the measure reads from a container: a record for
// every frame and gap, the complete L3 foreground, each return's channel and
// float64 coordinates, and a partition into clusters and unassigned returns
// with their reasons. The packet sequence and block are point fields, not
// capabilities, and are checked on each measured frame.
func FateCapabilities() []l4bobserve.Capability {
	return []l4bobserve.Capability{
		l4bobserve.CapabilityFrameRecords,
		l4bobserve.CapabilityGapRecords,
		l4bobserve.CapabilityCompleteForeground,
		l4bobserve.CapabilityClusterMembership,
		l4bobserve.CapabilityUnassignedPoints,
		l4bobserve.CapabilityPointChannel,
		l4bobserve.CapabilityPointFloat64Geometry,
	}
}

// RequireFateEvidence refuses evidence of a profile that lacks any of
// FateCapabilities, with a *l4bobserve.MissingCapabilitiesError naming all of
// them.
func RequireFateEvidence(p l4bobserve.Profile) error {
	return p.Require(FateCapabilities()...)
}

// FateOptions choose the reference, the evidence and the strata.
type FateOptions struct {
	Reference ReferenceOptions
	// ObservationsDir is the foreground-complete container, opened read-only.
	ObservationsDir string
	// CapturePath is the capture file the container was extracted from, read
	// to map packet keys to sequences. Empty uses the path the container's
	// manifest names. Either way its SHA-256 must be the manifest's.
	CapturePath string
	// Port is the UDP port of the sensor's packets in the capture.
	Port int
	// FrameToleranceNanos is how far a container frame's time may be from a
	// pack sample's and still be that sample.
	FrameToleranceNanos int64
	// SensorXM and SensorYM are where the sensor is in the pack's frame; an
	// object's range is from here to its reference position.
	SensorXM, SensorYM float64
	// RangeBandsM are the ascending upper edges of the range bands, in
	// metres; the last band is open.
	RangeBandsM []float64
}

// DefaultFateOptions are the retention measure's defaults (reviewed-only
// policy, 10 ms frame tolerance, 20, 35 and 50 m band edges) and the kirk0
// capture's UDP port.
func DefaultFateOptions() FateOptions {
	r := DefaultRetentionOptions()
	return FateOptions{Reference: r.Reference, Port: 2369, FrameToleranceNanos: r.FrameToleranceNanos, RangeBandsM: r.RangeBandsM}
}

// Validate refuses options that cannot measure.
func (o FateOptions) Validate() error {
	if o.ObservationsDir == "" {
		return fmt.Errorf("no observation container named")
	}
	if o.Port < 1 || o.Port > 65535 {
		return fmt.Errorf("UDP port %d outside 1 to 65535", o.Port)
	}
	r := RetentionOptions{Reference: o.Reference, DBPath: o.ObservationsDir, FrameToleranceNanos: o.FrameToleranceNanos,
		RangeBandsM: o.RangeBandsM, MinDomainShare: 0}
	return r.Validate()
}

// FateCluster is one cluster an instant's returns fell into.
type FateCluster struct {
	ClusterID int64 `json:"cluster_id"`
	// Points are the object's returns in the cluster; ClusterPoints all of
	// the cluster's.
	Points        int `json:"points"`
	ClusterPoints int `json:"cluster_points"`
}

// FateInstant is one object in one measured frame.
type FateInstant struct {
	SampleID      int    `json:"sample_id"`
	TimestampNs   int64  `json:"timestamp_ns"`
	FrameSequence uint64 `json:"frame_sequence"`
	ObjectID      string `json:"object_id"`
	Class         string `json:"class"`
	// RangeM is the horizontal distance from the sensor to the mask's
	// reference position.
	RangeM  float64 `json:"range_m"`
	Band    string  `json:"range_band"`
	Returns int     `json:"returns"`
	// Fates counts the returns by fate; fates with none are left out.
	Fates    map[string]int `json:"fates"`
	Clusters []FateCluster  `json:"clusters,omitempty"`
	// FrameClusters is how many clusters L4 accepted in the frame. A frame
	// with none has no row in the retention measure's evidence.
	FrameClusters int `json:"frame_clusters"`
}

// FateSummary pools a set of instants.
type FateSummary struct {
	Name     string `json:"name"`
	Class    string `json:"class,omitempty"`
	Objects  int    `json:"objects"`
	Instants int    `json:"instants"`
	Returns  int    `json:"returns"`
	// Fates and Shares cover every fate; shares are of Returns.
	Fates  map[string]int     `json:"fates"`
	Shares map[string]float64 `json:"shares"`
	// Missed instants had no return in a cluster, detected instants at
	// least one; fragmented instants two clusters or more, a share of the
	// detected.
	Missed          int     `json:"missed_instants"`
	Detected        int     `json:"detected_instants"`
	Fragmented      int     `json:"fragmented_instants"`
	FragmentedShare float64 `json:"fragmented_share"`
	MeanClusters    float64 `json:"mean_clusters"`
	MaxClusters     int     `json:"max_clusters"`
	// GapReturns are the returns of the fragmented instants that are in none
	// of the object's fragments: what the gaps between fragments are made
	// of, by fate. Their shares are of GapReturns.
	GapReturns int                `json:"gap_returns"`
	GapFates   map[string]int     `json:"gap_fates"`
	GapShares  map[string]float64 `json:"gap_shares"`
	// The comparable instants are those whose frame holds at least one
	// accepted cluster: the ones the retention measure can see.
	// ComparableRetention is their clustered returns over their returns, the
	// exact counterpart of retention's pooled retention.
	ComparableInstants  int     `json:"comparable_instants"`
	ComparableReturns   int     `json:"comparable_returns"`
	ComparableClustered int     `json:"comparable_clustered"`
	ComparableRetention float64 `json:"comparable_retention"`
}

// FateNotMeasured counts the instants of one reason.
type FateNotMeasured struct {
	Reason   string `json:"reason"`
	Samples  int    `json:"samples"`
	Instants int    `json:"instants"`
	Returns  int    `json:"returns"`
}

// FateJoin describes how the reviewed returns were joined to the domain.
type FateJoin struct {
	// MaskReturns are the measured instants' returns.
	MaskReturns  int `json:"mask_returns"`
	Matched      int `json:"matched"`
	ProvenAbsent int `json:"proven_absent"`
	OtherFrame   int `json:"other_frame"`
	Unresolved   int `json:"unresolved"`
	// UnresolvedBy counts unresolved returns by cause.
	UnresolvedBy map[string]int `json:"unresolved_by"`
	// MatchRate is matched plus proven absent over the measured returns.
	MatchRate float64 `json:"match_rate"`
	// CoordinatesAgree counts matched returns whose domain coordinates, as
	// float32, are bit-identical to the pack's.
	CoordinatesAgree int `json:"coordinates_agree"`
	// Over every return of the measured samples, not only the reviewed ones:
	// the pack's returns found in the frame's domain, and the domain's
	// returns found in the pack. The first is how much of the recording's
	// foreground this replay's L3 also called foreground.
	PackReturns         int     `json:"pack_returns"`
	PackReturnsInDomain int     `json:"pack_returns_in_domain"`
	DomainReturns       int     `json:"domain_returns"`
	PackInDomainShare   float64 `json:"pack_in_domain_share"`
	DomainInPackShare   float64 `json:"domain_in_pack_share"`
}

// FateEvidence describes the container and the capture it was read with.
type FateEvidence struct {
	// Container is the directory's base name.
	Container    string `json:"container"`
	Profile      string `json:"profile"`
	State        string `json:"state"`
	SourceID     string `json:"source_id"`
	ExtractorID  string `json:"extractor_id"`
	ClipID       string `json:"clip_id"`
	BuildVersion string `json:"build_version"`
	BuildGitSHA  string `json:"build_git_sha"`
	ParamsHash   string `json:"params_hash"`
	// The replay window the manifest declares, relative to the capture's
	// first packet.
	WindowStartNs int64 `json:"window_start_ns"`
	WarmupNs      int64 `json:"warmup_ns"`
	DurationNs    int64 `json:"duration_ns"`
	// ScoringStartNs is the capture's first packet plus the window start:
	// frames before it are warm-up.
	ScoringStartNs int64 `json:"scoring_start_ns"`
	// Capture is the capture file's base name.
	Capture           string `json:"capture"`
	CaptureSHA256     string `json:"capture_sha256"`
	CapturePackets    int    `json:"capture_packets"`
	UnnumberedPackets int    `json:"unnumbered_packets"`
	// AmbiguousSequences are sequence numbers more than one distinct packet
	// carries; returns of those packets cannot be joined.
	AmbiguousSequences  int            `json:"ambiguous_sequences"`
	Frames              int            `json:"frames"`
	Gaps                int            `json:"gaps"`
	FramesByDisposition map[string]int `json:"frames_by_disposition"`
	// MeasuredSamples are the samples whose instants were measured, each in
	// one frame; MeasuredCompleteness counts those frames by completeness.
	MeasuredSamples      int            `json:"measured_samples"`
	MeasuredCompleteness map[string]int `json:"measured_frame_completeness"`
}

// FateReportOpts records the measure's own choices.
type FateReportOpts struct {
	FrameToleranceNanos int64     `json:"frame_tolerance_ns"`
	Port                int       `json:"udp_port"`
	SensorXM            float64   `json:"sensor_x_m"`
	SensorYM            float64   `json:"sensor_y_m"`
	RangeBandsM         []float64 `json:"range_band_edges_m"`
}

// FateReport is the measure over one split against one replay.
type FateReport struct {
	Schema        string            `json:"schema"`
	SchemaVersion int               `json:"schema_version"`
	Reference     ReferenceIdentity `json:"reference"`
	Evidence      FateEvidence      `json:"evidence"`
	Join          FateJoin          `json:"join"`
	Options       FateReportOpts    `json:"options"`
	Method        string            `json:"method"`
	Fates         []string          `json:"fates"`
	Total         FateSummary       `json:"total"`
	ByClass       []FateSummary     `json:"by_class"`
	ByRange       []FateSummary     `json:"by_range"`
	ByObject      []FateSummary     `json:"by_object"`
	NotMeasured   []FateNotMeasured `json:"not_measured"`
	Instants      []FateInstant     `json:"instants"`
	Caveats       []string          `json:"caveats"`
}

// MeasureFate binds the reference as the per-frame evaluator does, reads the
// capture to map packet keys to sequences, streams the container's frames,
// and gives every reviewed return of the selected episodes its fate.
func MeasureFate(opts FateOptions) (*FateReport, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	reader, err := vrlog.Open(opts.ObservationsDir, vrlog.Options{})
	if err != nil {
		return nil, fmt.Errorf("open observations: %w", err)
	}
	defer reader.Close()
	if err := RequireFateEvidence(reader.Profile()); err != nil {
		return nil, fmt.Errorf("%s: %w", opts.ObservationsDir, err)
	}
	manifest := reader.Manifest()
	capturePath, captureDigest, err := fateCapture(manifest, opts.CapturePath)
	if err != nil {
		return nil, err
	}

	ref, bound, err := loadBoundReference(opts.Reference)
	if err != nil {
		return nil, err
	}
	layout := bound.pack.Manifest.ReturnLayout
	if layout == nil {
		return nil, fmt.Errorf("pack %s carries no return identity: the fate measure needs a version 2 pack", bound.pack.Manifest.DatasetID)
	}
	masks, err := retentionMasks(bound, RetentionOptions{Reference: opts.Reference, SensorXM: opts.SensorXM, SensorYM: opts.SensorYM})
	if err != nil {
		return nil, err
	}

	bridge, err := scanCaptureSequences(capturePath, opts.Port)
	if err != nil {
		return nil, err
	}
	if bridge.sha256 != captureDigest {
		return nil, fmt.Errorf("capture %s is not the container's source: its SHA-256 is %s, the container names %s",
			capturePath, bridge.sha256, captureDigest)
	}

	st := reader.Status()
	w := manifest.Extraction.Window
	p := manifest.Provenance
	report := &FateReport{
		Schema: FateSchema, SchemaVersion: FateSchemaVersion, Reference: ref.Identity, Method: FateMethod, Fates: FateNames(),
		Evidence: FateEvidence{
			Container: filepath.Base(filepath.Clean(opts.ObservationsDir)), Profile: string(manifest.Profile.Name),
			State: st.State.String(), SourceID: manifest.Extraction.SourceID, ExtractorID: manifest.Extraction.ExtractorID,
			ClipID: manifest.Extraction.ClipID, BuildVersion: p.BuildVersion, BuildGitSHA: p.BuildGitSHA,
			ParamsHash: p.ParamsHash, WindowStartNs: w.StartOffsetNanos, WarmupNs: w.WarmupNanos, DurationNs: w.DurationNanos,
			ScoringStartNs: bridge.firstNs + w.StartOffsetNanos,
			Capture:        filepath.Base(capturePath), CaptureSHA256: bridge.sha256, CapturePackets: bridge.packets,
			UnnumberedPackets: bridge.unnumbered, AmbiguousSequences: len(bridge.ambiguous),
			FramesByDisposition: map[string]int{}, MeasuredCompleteness: map[string]int{},
		},
		Join: FateJoin{UnresolvedBy: map[string]int{}},
		Options: FateReportOpts{FrameToleranceNanos: opts.FrameToleranceNanos, Port: opts.Port, SensorXM: opts.SensorXM,
			SensorYM: opts.SensorYM, RangeBandsM: slices.Clone(opts.RangeBandsM)},
	}
	m := &fateMeasurer{report: report, pack: bound.pack, bridge: bridge, tol: opts.FrameToleranceNanos,
		per: layout.PerPacket(), channels: uint64(layout.Channels), notMeasured: map[string]*FateNotMeasured{}}
	m.groups = groupFateMasks(bound.pack, masks)
	if err := m.stream(reader); err != nil {
		return nil, err
	}
	// Samples between the container's frames that no frame lies near mean
	// the replay's frames are not the pack's, not that the road was empty.
	switch {
	case m.aligned == 0:
		return nil, fmt.Errorf("no measured sample lies within %d ns of a container frame: is it a replay of this pack's capture?", m.tol)
	case m.aligned < m.unaligned:
		return nil, fmt.Errorf("only %d of %d samples inside the container's frames lie within %d ns of one: "+
			"the replay's frames do not align with the pack's samples", m.aligned, m.aligned+m.unaligned, m.tol)
	}
	m.finish()
	summariseFateReport(report, opts.RangeBandsM)
	return report, nil
}

// fateCapture names the container's single capture file, or checks that the
// operator's names a file, and returns the digest it must have.
func fateCapture(m vrlog.Manifest, override string) (string, string, error) {
	files := m.Extraction.CaptureFiles
	if len(files) != 1 {
		return "", "", fmt.Errorf("the container's source is %d capture files; the fate measure maps the packets of exactly one", len(files))
	}
	path := files[0].Path
	if override != "" {
		path = override
	}
	return path, "sha256:" + strings.ToLower(files[0].SHA256), nil
}

// captureSequences maps the capture's packet keys to the sequence numbers
// their payloads carry.
type captureSequences struct {
	sha256     string
	packets    int
	unnumbered int
	firstNs    int64
	// keySeq holds every numbered packet whose sequence no other distinct
	// packet carries; ambiguousKeys the packets whose sequence another does.
	keySeq        map[uint64]uint32
	ambiguous     map[uint32]bool
	ambiguousKeys map[uint64]bool
}

// scanCaptureSequences reads every packet on port in the capture, digesting
// the whole file.
func scanCaptureSequences(path string, port int) (*captureSequences, error) {
	c := &captureSequences{keySeq: map[uint64]uint32{}, ambiguous: map[uint32]bool{}, ambiguousKeys: map[uint64]bool{}}
	seqKey := map[uint32]uint64{}
	first := true
	res, err := labelset.ScanCapture(path, port, func(p labelset.Packet) error {
		if first {
			c.firstNs, first = p.CaptureNs, false
		}
		seq, ok := parse.PacketSequence(p.Payload)
		if !ok {
			c.unnumbered++
			return nil
		}
		key := uint64(p.Key)
		if other, seen := seqKey[seq]; seen && other != key {
			c.ambiguous[seq] = true
		}
		seqKey[seq] = key
		c.keySeq[key] = seq
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read capture %s: %w", path, err)
	}
	if res.Packets == 0 {
		return nil, fmt.Errorf("capture %s holds no packets on UDP port %d", path, port)
	}
	if c.unnumbered == res.Packets {
		return nil, fmt.Errorf("capture %s: none of its %d packets on UDP port %d carries a sequence number (1262-byte packets), "+
			"so the container's returns cannot be joined to the pack's by identity", path, res.Packets, port)
	}
	for key, seq := range c.keySeq {
		if c.ambiguous[seq] {
			delete(c.keySeq, key)
			c.ambiguousKeys[key] = true
		}
	}
	c.sha256, c.packets = res.SHA256, res.Packets
	return c, nil
}

// returnID is a return's identity in the domain.
func returnID(seq uint32, block, channel uint16) uint64 {
	return uint64(seq)<<32 | uint64(block)<<16 | uint64(channel)
}

// fateGroup is one sample's masks.
type fateGroup struct {
	sampleID int
	ts       int64
	masks    []RetentionMask
}

func groupFateMasks(pack *annotation.Pack, masks []RetentionMask) []fateGroup {
	var groups []fateGroup
	for _, mk := range masks {
		if n := len(groups); n > 0 && groups[n-1].sampleID == mk.SampleID {
			groups[n-1].masks = append(groups[n-1].masks, mk)
			continue
		}
		groups = append(groups, fateGroup{sampleID: mk.SampleID, ts: pack.Samples[mk.SampleID].TimestampNs, masks: []RetentionMask{mk}})
	}
	slices.SortStableFunc(groups, func(a, b fateGroup) int { return cmp.Compare(a.ts, b.ts) })
	return groups
}

// fateFrame is one container frame and, once a sample needs it, its domain
// indexed by identity.
type fateFrame struct {
	rec       l4bobserve.FrameRecord
	gapBefore bool
	indexed   bool
	// ids maps a return's identity to its domain index, or -1 when two
	// returns share it. codes gives each domain index its cluster slot
	// (zero or more) or -1 minus its fate.
	ids   map[uint64]int32
	codes []int32
}

const locatorFields = l4bobserve.FieldPacketSequence | l4bobserve.FieldBlockIndex | l4bobserve.FieldChannel

func (f *fateFrame) index() error {
	if f.indexed {
		return nil
	}
	f.indexed = true
	p := f.rec.Points
	if !p.Fields.Has(locatorFields) {
		return fmt.Errorf("frame %d carries no packet sequence, block and channel for its returns (the capture's packets are "+
			"unnumbered?), so they cannot be joined to the pack's by identity", f.rec.Sequence)
	}
	n := p.Len()
	f.ids = make(map[uint64]int32, n)
	for i := 0; i < n; i++ {
		id := returnID(p.PacketSequence[i], p.BlockIndex[i], p.Channel[i])
		if _, dup := f.ids[id]; dup {
			f.ids[id] = -1
			continue
		}
		f.ids[id] = int32(i)
	}
	f.codes = make([]int32, n)
	for slot, c := range f.rec.Clusters {
		for _, k := range c.Members {
			f.codes[k] = int32(slot)
		}
	}
	for j, k := range f.rec.Unassigned {
		f.codes[k] = -1 - int32(rejectionFate(f.rec.UnassignedReasons[j]))
	}
	return nil
}

// unreadable says why a neighbouring frame cannot show where a return absent
// from the aligned frame went, or "" when it can. gap reports a gap record
// between the neighbour and the aligned frame: the rotation beside the
// aligned frame is then lost, and the neighbour is not it.
func unreadable(n *fateFrame, gap bool) string {
	switch {
	case n == nil:
		return UnresolvedNoNeighbour
	case gap:
		return UnresolvedGapBesideFrame
	case n.rec.Payload&l4bobserve.PayloadPoints == 0:
		return UnresolvedNeighbourNoPoints
	}
	return ""
}

// has reports whether a frame's domain holds the identity. A nil frame, or
// one without points, holds nothing; unreadable says whether that absence
// proves anything.
func (f *fateFrame) has(id uint64) (bool, error) {
	if f == nil {
		return false, nil
	}
	if f.rec.Payload&l4bobserve.PayloadPoints == 0 {
		return false, nil
	}
	if err := f.index(); err != nil {
		return false, err
	}
	_, ok := f.ids[id]
	return ok, nil
}

type fateMeasurer struct {
	report      *FateReport
	pack        *annotation.Pack
	bridge      *captureSequences
	tol         int64
	per         uint64
	channels    uint64
	groups      []fateGroup
	next        int // the first group not yet settled
	trailingGap bool
	// aligned counts samples given a frame, whatever its disposition;
	// unaligned those between frames with none near them and no gap.
	aligned, unaligned int
	notMeasured        map[string]*FateNotMeasured
}

// stream reads the container in order, holding three frames: each frame is
// settled once its successor is known, so a sample is aligned with its
// nearest frame and that frame's neighbours are at hand.
func (m *fateMeasurer) stream(r *vrlog.Reader) error {
	ev := &m.report.Evidence
	pendingGap := false
	var window [3]*fateFrame
	push := func(f *fateFrame) error {
		window[0], window[1], window[2] = window[1], window[2], f
		if window[1] != nil {
			return m.settle(window[0], window[1], window[2])
		}
		return nil
	}
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read observations: %w", err)
		}
		switch rec.Kind {
		case vrlog.RecordGap:
			ev.Gaps++
			pendingGap = true
		case vrlog.RecordFrame:
			ev.Frames++
			ev.FramesByDisposition[rec.Frame.Disposition.Kind.String()]++
			if err := push(&fateFrame{rec: rec.Frame, gapBefore: pendingGap}); err != nil {
				return err
			}
			pendingGap = false
		}
	}
	if ev.Frames == 0 {
		return fmt.Errorf("the container holds no frame records")
	}
	m.trailingGap = pendingGap
	return push(nil)
}

// settle aligns and measures every pending sample whose nearest frame is cur.
// prev is cur's predecessor, nil when cur is the first frame; next its
// successor, nil when cur is the last.
func (m *fateMeasurer) settle(prev, cur, next *fateFrame) error {
	t := cur.rec.FrameUnixNanos
	for m.next < len(m.groups) {
		g := m.groups[m.next]
		if g.ts > t+m.tol {
			break
		}
		if g.ts < t-m.tol {
			// No frame lies within the tolerance: before the first frame, or
			// between prev and cur.
			switch {
			case prev == nil:
				m.skip(NotMeasuredOutsideContainer, g)
			case cur.gapBefore:
				m.skip(NotMeasuredGap, g)
			default:
				m.skip(NotMeasuredNoFrame, g)
				m.unaligned++
			}
			m.next++
			continue
		}
		if next != nil {
			if d := abs64(next.rec.FrameUnixNanos - g.ts); d <= m.tol && d < abs64(t-g.ts) {
				break // nearer the next frame, and within the tolerance: it settles there
			}
		}
		if err := m.measure(g, prev, cur, next); err != nil {
			return err
		}
		m.next++
	}
	if next == nil {
		// cur is the last frame: whatever remains lies after it.
		for ; m.next < len(m.groups); m.next++ {
			if m.trailingGap {
				m.skip(NotMeasuredGap, m.groups[m.next])
			} else {
				m.skip(NotMeasuredOutsideContainer, m.groups[m.next])
			}
		}
	}
	return nil
}

func (m *fateMeasurer) skip(reason string, g fateGroup) {
	nm := m.notMeasured[reason]
	if nm == nil {
		nm = &FateNotMeasured{Reason: reason}
		m.notMeasured[reason] = nm
	}
	nm.Samples++
	for _, mk := range g.masks {
		nm.Instants++
		nm.Returns += len(mk.Indices)
	}
}

// identity resolves a pack return to the domain's identity, or names why it
// cannot be.
func (m *fateMeasurer) identity(r uint64) (uint64, string) {
	packet, within := r/m.per, r%m.per
	if packet >= uint64(len(m.pack.Packets)) {
		return 0, UnresolvedNotInCapture
	}
	key := m.pack.Packets[packet]
	seq, ok := m.bridge.keySeq[key]
	if !ok {
		if m.bridge.ambiguousKeys[key] {
			return 0, UnresolvedAmbiguousSequence
		}
		return 0, UnresolvedNotInCapture
	}
	return returnID(seq, uint16(within/m.channels), uint16(within%m.channels+1)), ""
}

func (m *fateMeasurer) measure(g fateGroup, prev, cur, next *fateFrame) error {
	m.aligned++
	rec := &cur.rec
	switch {
	case rec.FrameUnixNanos < m.report.Evidence.ScoringStartNs:
		m.skip(NotMeasuredWarmUp, g)
		return nil
	case rec.Disposition.Kind == l4bobserve.DispositionUnsettled:
		m.skip(NotMeasuredUnsettled, g)
		return nil
	case rec.Disposition.Kind == l4bobserve.DispositionSuppressed:
		m.skip(NotMeasuredSuppressed, g)
		return nil
	case rec.Disposition.Kind != l4bobserve.DispositionObserved || !rec.Payload.Has(l4bobserve.PayloadPoints|l4bobserve.PayloadMembership):
		m.skip(NotMeasuredFailed, g)
		return nil
	}
	if err := cur.index(); err != nil {
		return err
	}
	pts, err := m.pack.PointsAt(g.sampleID)
	if err != nil {
		return err
	}
	returns, err := m.pack.ReturnsAt(g.sampleID)
	if err != nil {
		return err
	}
	// A return in neither neighbour is proven absent only if both could be
	// read; otherwise it is unresolved, under the first neighbour's cause.
	unchecked := unreadable(prev, cur.gapBefore)
	if unchecked == "" {
		unchecked = unreadable(next, next != nil && next.gapBefore)
	}
	ev, join := &m.report.Evidence, &m.report.Join
	ev.MeasuredSamples++
	ev.MeasuredCompleteness[rec.Completeness.State.String()]++
	join.PackReturns += len(returns)
	join.DomainReturns += rec.Points.Len()
	for _, r := range returns {
		if id, cause := m.identity(r); cause == "" {
			if k, ok := cur.ids[id]; ok && k >= 0 {
				join.PackReturnsInDomain++
			}
		}
	}

	for _, mk := range g.masks {
		in := FateInstant{SampleID: mk.SampleID, TimestampNs: mk.TimestampNs, FrameSequence: rec.Sequence,
			ObjectID: mk.ObjectID, Class: mk.Class, RangeM: mk.RangeM, Returns: len(mk.Indices), FrameClusters: len(rec.Clusters)}
		var tally [numFates]int
		bySlot := map[int32]int{}
		for _, i := range mk.Indices {
			join.MaskReturns++
			id, cause := m.identity(returns[i])
			if cause != "" {
				tally[fateUnresolved]++
				join.UnresolvedBy[cause]++
				continue
			}
			k, inDomain := cur.ids[id]
			switch {
			case inDomain && k < 0:
				tally[fateUnresolved]++
				join.UnresolvedBy[UnresolvedAmbiguousDomain]++
			case inDomain:
				join.Matched++
				p := rec.Points
				if math.Float32bits(float32(p.X[k])) == math.Float32bits(pts.X[i]) &&
					math.Float32bits(float32(p.Y[k])) == math.Float32bits(pts.Y[i]) &&
					math.Float32bits(float32(p.Z[k])) == math.Float32bits(pts.Z[i]) {
					join.CoordinatesAgree++
				}
				if code := cur.codes[k]; code >= 0 {
					tally[fateClustered]++
					bySlot[code]++
				} else {
					tally[fate(-1-code)]++
				}
			default:
				inPrev, err := prev.has(id)
				if err != nil {
					return err
				}
				inNext, err := next.has(id)
				if err != nil {
					return err
				}
				switch {
				case inPrev || inNext:
					tally[fateOtherFrame]++
					join.OtherFrame++
				case unchecked != "":
					tally[fateUnresolved]++
					join.UnresolvedBy[unchecked]++
				default:
					tally[fateL3Background]++
					join.ProvenAbsent++
				}
			}
		}
		in.Fates = map[string]int{}
		for f, n := range tally {
			if n > 0 {
				in.Fates[fateNames[f]] = n
			}
		}
		slots := make([]int32, 0, len(bySlot))
		for s := range bySlot {
			slots = append(slots, s)
		}
		slices.Sort(slots)
		for _, s := range slots {
			c := rec.Clusters[s]
			in.Clusters = append(in.Clusters, FateCluster{ClusterID: c.ClusterID, Points: bySlot[s], ClusterPoints: len(c.Members)})
		}
		m.report.Instants = append(m.report.Instants, in)
	}
	return nil
}

// finish fills the join's totals and the not-measured list.
func (m *fateMeasurer) finish() {
	j := &m.report.Join
	for _, n := range j.UnresolvedBy {
		j.Unresolved += n
	}
	if j.MaskReturns > 0 {
		j.MatchRate = float64(j.Matched+j.ProvenAbsent) / float64(j.MaskReturns)
	}
	if j.PackReturns > 0 {
		j.PackInDomainShare = float64(j.PackReturnsInDomain) / float64(j.PackReturns)
	}
	if j.DomainReturns > 0 {
		j.DomainInPackShare = float64(j.PackReturnsInDomain) / float64(j.DomainReturns)
	}
	for _, reason := range []string{NotMeasuredOutsideContainer, NotMeasuredWarmUp, NotMeasuredNoFrame, NotMeasuredGap,
		NotMeasuredUnsettled, NotMeasuredSuppressed, NotMeasuredFailed} {
		if nm := m.notMeasured[reason]; nm != nil {
			m.report.NotMeasured = append(m.report.NotMeasured, *nm)
		}
	}
	if m.report.NotMeasured == nil {
		m.report.NotMeasured = []FateNotMeasured{}
	}
	if m.report.Instants == nil {
		m.report.Instants = []FateInstant{}
	}
}

type fateAccum struct {
	s                    FateSummary
	objects              map[string]bool
	tally, gaps          [numFates]int
	sumClusters          int
	comparableClustered  int
	comparableReturns    int
	comparableInstantCnt int
}

func (a *fateAccum) add(in FateInstant) {
	if a.objects == nil {
		a.objects = map[string]bool{}
	}
	a.objects[in.ObjectID] = true
	a.s.Instants++
	a.s.Returns += in.Returns
	var t [numFates]int
	for f, name := range fateNames {
		t[f] = in.Fates[name]
		a.tally[f] += t[f]
	}
	if in.FrameClusters > 0 {
		a.comparableInstantCnt++
		a.comparableReturns += in.Returns
		a.comparableClustered += t[fateClustered]
	}
	n := len(in.Clusters)
	if n == 0 {
		a.s.Missed++
		return
	}
	a.s.Detected++
	a.sumClusters += n
	a.s.MaxClusters = max(a.s.MaxClusters, n)
	if n < 2 {
		return
	}
	a.s.Fragmented++
	for f := range t {
		if fate(f) != fateClustered {
			a.gaps[f] += t[f]
			a.s.GapReturns += t[f]
		}
	}
}

func (a *fateAccum) summary(name string) FateSummary {
	s := a.s
	s.Name, s.Objects = name, len(a.objects)
	s.Fates, s.Shares = fateMaps(a.tally, s.Returns)
	s.GapFates, s.GapShares = fateMaps(a.gaps, s.GapReturns)
	if s.Detected > 0 {
		s.FragmentedShare = float64(s.Fragmented) / float64(s.Detected)
		s.MeanClusters = float64(a.sumClusters) / float64(s.Detected)
	}
	s.ComparableInstants, s.ComparableReturns, s.ComparableClustered = a.comparableInstantCnt, a.comparableReturns, a.comparableClustered
	if s.ComparableReturns > 0 {
		s.ComparableRetention = float64(s.ComparableClustered) / float64(s.ComparableReturns)
	}
	return s
}

func fateMaps(t [numFates]int, total int) (map[string]int, map[string]float64) {
	counts, shares := make(map[string]int, numFates), make(map[string]float64, numFates)
	for f, name := range fateNames {
		counts[name] = t[f]
		shares[name] = 0
		if total > 0 {
			shares[name] = float64(t[f]) / float64(total)
		}
	}
	return counts, shares
}

// summariseFateReport pools the instants overall, by class, by range band and
// by object, and writes the caveats.
func summariseFateReport(r *FateReport, edges []float64) {
	var total fateAccum
	byClass, byRange, byObject := map[string]*fateAccum{}, map[string]*fateAccum{}, map[string]*fateAccum{}
	classOf := map[string]string{}
	get := func(m map[string]*fateAccum, k string) *fateAccum {
		if m[k] == nil {
			m[k] = &fateAccum{}
		}
		return m[k]
	}
	for i := range r.Instants {
		in := &r.Instants[i]
		in.Band = rangeBand(in.RangeM, edges)
		total.add(*in)
		get(byClass, in.Class).add(*in)
		get(byRange, in.Band).add(*in)
		get(byObject, in.ObjectID).add(*in)
		classOf[in.ObjectID] = in.Class
	}
	r.Total = total.summary("all")
	for _, k := range sortedMapKeys(byClass) {
		r.ByClass = append(r.ByClass, byClass[k].summary(k))
	}
	for _, k := range rangeBandNames(edges) {
		a := byRange[k]
		if a == nil {
			a = &fateAccum{}
		}
		r.ByRange = append(r.ByRange, a.summary(k))
	}
	for _, k := range sortedMapKeys(byObject) {
		s := byObject[k].summary(k)
		s.Class = classOf[k]
		r.ByObject = append(r.ByObject, s)
	}
	r.Caveats = fateCaveats(r)
}

func fateCaveats(r *FateReport) []string {
	c := []string{
		"Reviewed returns are joined to the replay's foreground by packet identity: the pack's (packet key, block, channel), " +
			"mapped through the capture to the container's (packet sequence, block, channel). Coordinates are not used to join.",
		"The pack is foreground-only: l3_background means this replay's L3 called background a return the recording run's L3 " +
			"called foreground, not that every return the object produced is accounted for.",
		"cluster_shape is a DBSCAN cluster the diameter or aspect filters rejected (the record does not say which); " +
			"a group smaller than the minimum cluster points is cluster_noise.",
	}
	j := r.Join
	if j.MatchRate < 1 && j.MaskReturns > 0 {
		c = append(c, fmt.Sprintf("Match rate %.4f: %d returns were found in a neighbouring frame and %d could not be resolved (%s).",
			j.MatchRate, j.OtherFrame, j.Unresolved, describeCounts(j.UnresolvedBy)))
	}
	unchecked, parts := 0, []string(nil)
	for _, cause := range neighbourCauses {
		if n := j.UnresolvedBy[cause]; n > 0 {
			unchecked += n
			parts = append(parts, fmt.Sprintf("%s %d", cause, n))
		}
	}
	if unchecked > 0 {
		c = append(c, fmt.Sprintf("%d returns absent from their frame are unresolved, not l3_background, because a neighbouring "+
			"frame could not be read (%s): the container's first or last frame has one neighbour, a frame without returns "+
			"cannot be searched, and a gap may hold the rotation the replay put the return in.", unchecked, strings.Join(parts, ", ")))
	}
	if j.Matched > 0 && j.CoordinatesAgree < j.Matched {
		c = append(c, fmt.Sprintf("Only %d of %d matched returns have the pack's float32 coordinates: the replay placed some returns "+
			"elsewhere (another transform?), which the identity join does not depend on.", j.CoordinatesAgree, j.Matched))
	}
	if n := r.Evidence.MeasuredSamples - r.Evidence.MeasuredCompleteness[l4bobserve.CompletenessComplete.String()]; n > 0 {
		c = append(c, fmt.Sprintf("%d measured frames are not known to be complete (%s): a return absent from one may have been "+
			"in a lost packet rather than called background by L3.", n, describeCounts(r.Evidence.MeasuredCompleteness)))
	}
	if r.Total.Fates[FateVoxelReduced] > 0 {
		c = append(c, "voxel_reduced returns are represented by another return of their voxel, which may be clustered; "+
			"the record keeps no contributor map.")
	}
	for _, nm := range r.NotMeasured {
		c = append(c, fmt.Sprintf("Not measured (%s): %d instants at %d samples.", nm.Reason, nm.Instants, nm.Samples))
	}
	if r.Total.ComparableInstants < r.Total.Instants {
		c = append(c, fmt.Sprintf("%d of %d measured instants are in frames where L4 accepted no cluster, which the retention "+
			"measure's evidence has no row for; the comparable columns leave them out.",
			r.Total.Instants-r.Total.ComparableInstants, r.Total.Instants))
	}
	if !r.Reference.HeldOut {
		c = append(c, fmt.Sprintf("Split %q is role %q, not held out: a tuning measure, not to be quoted as held out.", r.Reference.Split, r.Reference.SplitRole))
	}
	return c
}

func describeCounts(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(m))
	for _, k := range sortedMapKeys(m) {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}
