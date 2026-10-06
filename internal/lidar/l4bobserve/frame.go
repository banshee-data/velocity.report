package l4bobserve

import (
	"fmt"
	"math"
)

// Count is a tally whose absence is distinct from zero. The zero value is
// unknown: a stage that reported nothing must never read as "rejected none".
type Count struct {
	Value uint64
	Known bool
}

// KnownCount returns a known tally. A negative n is a caller bug and yields
// an unknown count rather than a plausible-looking number.
func KnownCount(n int) Count {
	if n < 0 {
		return Count{}
	}
	return Count{Value: uint64(n), Known: true}
}

func (c Count) String() string {
	if !c.Known {
		return "unknown"
	}
	return fmt.Sprintf("%d", c.Value)
}

// Stage names a processing boundary a frame's returns pass through. Values
// are durable identifiers.
type Stage string

const (
	// StageL2Frame is the assembled rotation. Its output is the returns L2
	// delivered; its input and rejections are unknown because L1 discards
	// zero-range returns without counting them.
	StageL2Frame Stage = "l2-frame"
	// StageL3Foreground splits L2 returns into background and foreground.
	StageL3Foreground Stage = "l3-foreground"
	// StageL4Transform is the start of L4. It carries no counts; it is named
	// when L4 is suppressed or fails before any filtering.
	StageL4Transform       Stage = "l4-transform"
	StageL4HeightFilter    Stage = "l4-height-filter"
	StageL4Voxel           Stage = "l4-voxel"
	StageL4ClusterInputCap Stage = "l4-cluster-input-cap"
	// StageL4Cluster is DBSCAN plus its size/aspect acceptance: output is the
	// points in accepted clusters, rejections are noise and rejected clusters.
	StageL4Cluster Stage = "l4-cluster"
)

// StageCount records one stage's point flow for one frame.
type StageCount struct {
	Stage    Stage
	Input    Count
	Output   Count
	Rejected Count
}

func (s StageCount) validate() error {
	if s.Stage == "" {
		return fmt.Errorf("stage count has no stage")
	}
	if s.Input.Known && s.Output.Known && s.Output.Value > s.Input.Value {
		return fmt.Errorf("stage %s outputs %d of %d inputs", s.Stage, s.Output.Value, s.Input.Value)
	}
	if s.Input.Known && s.Rejected.Known && s.Rejected.Value > s.Input.Value {
		return fmt.Errorf("stage %s rejects %d of %d inputs", s.Stage, s.Rejected.Value, s.Input.Value)
	}
	if s.Input.Known && s.Output.Known && s.Rejected.Known && s.Input.Value != s.Output.Value+s.Rejected.Value {
		return fmt.Errorf("stage %s: input %d != output %d + rejected %d", s.Stage, s.Input.Value, s.Output.Value, s.Rejected.Value)
	}
	return nil
}

// CompletenessState says how much of the rotation reached the extractor. The
// zero value is unknown: no default may claim a complete frame.
type CompletenessState uint8

const (
	CompletenessUnknown CompletenessState = iota
	CompletenessComplete
	CompletenessPartial
)

// Completeness is independent of processing disposition: an unsettled frame
// can also have lost packets.
type Completeness struct {
	State CompletenessState
	// LostPackets counts source packets known to be missing from this frame,
	// with a loss at a rotation boundary attributed to the later frame. It is
	// unknown when the source does not number its packets or the preceding
	// packet is not known to this extraction.
	LostPackets Count
}

func (s CompletenessState) String() string {
	switch s {
	case CompletenessUnknown:
		return "unknown"
	case CompletenessComplete:
		return "complete"
	case CompletenessPartial:
		return "partial"
	}
	return fmt.Sprintf("completeness(%d)", uint8(s))
}

func (c Completeness) validate() error {
	switch c.State {
	case CompletenessUnknown:
		if c.LostPackets.Known && c.LostPackets.Value > 0 {
			return fmt.Errorf("frame with %d known lost packets must be partial, not unknown", c.LostPackets.Value)
		}
	case CompletenessComplete:
		if !c.LostPackets.Known || c.LostPackets.Value != 0 {
			return fmt.Errorf("a complete frame needs a known loss of zero packets, got %s", c.LostPackets)
		}
	case CompletenessPartial:
	default:
		return fmt.Errorf("unknown completeness state %s", c.State)
	}
	return nil
}

// BackgroundState is the L3 settling state after the frame was processed.
type BackgroundState uint8

const (
	BackgroundUnknown BackgroundState = iota
	BackgroundSettling
	BackgroundSettled
)

// DispositionKind is what processing concluded for the frame (§3.3 of the
// VRLOG plan). Precedence when several apply: failed, suppressed, unsettled,
// observed. The settling state remains in FrameRecord.Background regardless.
type DispositionKind uint8

const (
	// DispositionUnspecified is invalid in a record: every frame states what
	// happened to it.
	DispositionUnspecified DispositionKind = iota
	// DispositionObserved: processing completed; an empty domain is a real
	// observation of nothing in the declared domain.
	DispositionObserved
	// DispositionUnsettled: processing completed while the background was
	// still settling; retained data and excluded stages remain stated.
	DispositionUnsettled
	// DispositionSuppressed: a named policy or profile skipped a stage.
	DispositionSuppressed
	// DispositionFailed: a stage failed; Stage and Reason say which and why.
	DispositionFailed
)

func (k DispositionKind) String() string {
	switch k {
	case DispositionUnspecified:
		return "unspecified"
	case DispositionObserved:
		return "observed"
	case DispositionUnsettled:
		return "unsettled"
	case DispositionSuppressed:
		return "suppressed"
	case DispositionFailed:
		return "failed"
	}
	return fmt.Sprintf("disposition(%d)", uint8(k))
}

func (s BackgroundState) String() string {
	switch s {
	case BackgroundUnknown:
		return "unknown"
	case BackgroundSettling:
		return "settling"
	case BackgroundSettled:
		return "settled"
	}
	return fmt.Sprintf("background(%d)", uint8(s))
}

// Disposition names the processing outcome. Stage and Reason are required for
// suppressed and failed frames and absent otherwise.
type Disposition struct {
	Kind   DispositionKind
	Stage  Stage
	Reason string
}

// Payload flags which evidence the record carries. It is separate from the
// disposition so that, for example, a frame that failed after L3 can still
// carry its complete retained foreground.
type Payload uint8

const (
	// PayloadPoints: the retained-point domain is present, possibly empty.
	PayloadPoints Payload = 1 << iota
	// PayloadMembership: clusters and unassigned points partition the domain.
	PayloadMembership

	knownPayload = PayloadPoints | PayloadMembership
)

// Has reports whether every flag in want is set.
func (p Payload) Has(want Payload) bool { return p&want == want }

// PointFields flags which optional per-point columns are present. Absence is
// declared here, never encoded as zero: ring zero and intensity zero are data.
type PointFields uint16

const (
	FieldAcquisitionTime PointFields = 1 << iota
	FieldIntensity
	FieldChannel
	// FieldReturnIndex is declared but not yet populated: L1 does not carry
	// the return identity of a dual-return firing.
	FieldReturnIndex
	FieldSourceOrdinal
	// FieldPacketSequence is present only when the source numbers its packets.
	FieldPacketSequence
	FieldBlockIndex

	knownPointFields = FieldAcquisitionTime | FieldIntensity | FieldChannel | FieldReturnIndex |
		FieldSourceOrdinal | FieldPacketSequence | FieldBlockIndex
)

// Has reports whether every flag in want is set.
func (f PointFields) Has(want PointFields) bool { return f&want == want }

// RetainedPoints is a frame's ordered retained-point domain in columnar form,
// ready for packed arrays in a later protobuf mapping. Index i in every
// present column is the same return; indices are frame-local and are what
// cluster membership refers to.
type RetainedPoints struct {
	Fields PointFields
	// X, Y and Z are exactly the float64 coordinates L4 computed. They are
	// always present together; a record may not carry a float32 substitute.
	X, Y, Z []float64
	// TimeOffsetNanos is the acquisition time minus FrameRecord.FrameUnixNanos.
	TimeOffsetNanos []int64
	// Intensity is the sensor's native reflectivity scale (Hesai: 0-255).
	Intensity []uint8
	// Channel is the laser channel as L1 decodes it (Hesai: 1-based ring).
	Channel     []uint16
	ReturnIndex []uint8
	// SourceOrdinal is the return's index in its L2 frame, assigned before any
	// filtering; the domain is in ascending source order.
	SourceOrdinal []uint32
	// PacketSequence and BlockIndex locate the return in the UDP stream.
	PacketSequence []uint32
	BlockIndex     []uint16
}

// Len is the number of retained returns.
func (p RetainedPoints) Len() int { return len(p.X) }

func (p RetainedPoints) empty() bool {
	return p.Fields == 0 && len(p.X) == 0 && len(p.Y) == 0 && len(p.Z) == 0 && len(p.TimeOffsetNanos) == 0 &&
		len(p.Intensity) == 0 && len(p.Channel) == 0 && len(p.ReturnIndex) == 0 && len(p.SourceOrdinal) == 0 &&
		len(p.PacketSequence) == 0 && len(p.BlockIndex) == 0
}

// RejectionReason says why a retained return is in no cluster. Values are
// durable identifiers for a later protobuf enum: append only.
type RejectionReason uint8

const (
	// RejectionUnknown is permitted: the extractor may not know the reason.
	RejectionUnknown RejectionReason = iota
	// RejectionHeightBand: outside the absolute sensor-frame height band
	// (below the floor or above the ceiling; the band filter does not say which).
	RejectionHeightBand
	// RejectionSurfaceBelowFloor: below the floor above the fitted ground surface.
	RejectionSurfaceBelowFloor
	// RejectionSurfaceAboveCeiling: above the ceiling over the fitted surface.
	RejectionSurfaceAboveCeiling
	// RejectionVoxelReduced: in an occupied voxel whose representative is
	// another retained return.
	RejectionVoxelReduced
	// RejectionClusterInputCap: left out by the DBSCAN input-cap subsample.
	RejectionClusterInputCap
	// RejectionClusterNoise: DBSCAN noise.
	RejectionClusterNoise
	// RejectionClusterShape: member of a DBSCAN cluster that the diameter or
	// aspect filters rejected.
	RejectionClusterShape

	maxRejectionReason = RejectionClusterShape
)

var rejectionReasonNames = [...]string{
	RejectionUnknown:             "unknown",
	RejectionHeightBand:          "height-band",
	RejectionSurfaceBelowFloor:   "surface-below-floor",
	RejectionSurfaceAboveCeiling: "surface-above-ceiling",
	RejectionVoxelReduced:        "voxel-reduced",
	RejectionClusterInputCap:     "cluster-input-cap",
	RejectionClusterNoise:        "cluster-noise",
	RejectionClusterShape:        "cluster-shape",
}

func (r RejectionReason) String() string {
	if int(r) < len(rejectionReasonNames) {
		return rejectionReasonNames[r]
	}
	return fmt.Sprintf("rejection(%d)", uint8(r))
}

// OrientedBox is L4's PCA box in float32, the precision L4 computed it at.
// HeadingRad is an axis, not a direction: θ and θ+π describe the same box.
type OrientedBox struct {
	CenterX, CenterY, CenterZ float32
	Length, Width, Height     float32
	HeadingRad                float32
}

// ClusterSummary is the extractor's per-cluster summary at its original
// precision. It is an observation, not a corrected body centre.
type ClusterSummary struct {
	// FirstMemberUnixNanos is the acquisition time of the first point DBSCAN
	// bucketed into the cluster (WorldCluster.TSUnixNanos), not a fitted time.
	FirstMemberUnixNanos int64
	// Centroid is L4's medoid: the member closest to the arithmetic mean.
	CentroidX, CentroidY, CentroidZ float32
	BoundingBoxLength               float32
	BoundingBoxWidth                float32
	BoundingBoxHeight               float32
	HeightP95                       float32
	IntensityMean                   float32
	PointsCount                     int
	GroundClipped                   bool
	// OBB is nil when L4 produced none.
	OBB *OrientedBox
}

// ClusterRecord is one extractor cluster. ClusterID is frame-local and never
// implies a persistent object.
type ClusterRecord struct {
	ClusterID int64
	// Members are sorted, unique indices into the frame's retained domain.
	Members []uint32
	Summary ClusterSummary
}

// FrameRecord is the evidence for one frame the extractor received. It is a
// pure value: slices are owned by the record and nothing refers to storage.
type FrameRecord struct {
	// Sequence is assigned monotonically, from zero, to every frame within one
	// extraction in the order received. It orders capture; timestamps describe
	// physical time and may repeat or step.
	Sequence uint64
	// SensorFrameID is L2's label for the rotation. It is informational: the
	// extraction and Sequence identify the record.
	SensorFrameID string
	// FrameUnixNanos is the declared frame time: the basis for point time
	// offsets and the legacy ObservationID. L2 declares its earliest return.
	FrameUnixNanos        int64
	CaptureStartUnixNanos int64
	CaptureEndUnixNanos   int64
	Completeness          Completeness
	Background            BackgroundState
	Disposition           Disposition
	Payload               Payload
	Stages                []StageCount
	Points                RetainedPoints
	Clusters              []ClusterRecord
	// Unassigned lists, in ascending order, the retained indices in no
	// cluster; UnassignedReasons is parallel to it.
	Unassigned        []uint32
	UnassignedReasons []RejectionReason
}

// Stage returns the frame's count for stage, if recorded.
func (f FrameRecord) Stage(stage Stage) (StageCount, bool) {
	for _, s := range f.Stages {
		if s.Stage == stage {
			return s, true
		}
	}
	return StageCount{}, false
}

// Validate checks the record's internal contract: disposition and payload
// agree, the point columns are consistent, coordinates are finite, times lie
// inside the capture interval, source order is ascending, stage arithmetic
// holds, and membership is an exact partition of the retained domain.
func (f FrameRecord) Validate() error {
	if err := f.validateDisposition(); err != nil {
		return fmt.Errorf("frame %d: %w", f.Sequence, err)
	}
	if err := f.Completeness.validate(); err != nil {
		return fmt.Errorf("frame %d: %w", f.Sequence, err)
	}
	if f.CaptureStartUnixNanos > f.CaptureEndUnixNanos {
		return fmt.Errorf("frame %d: capture starts after it ends", f.Sequence)
	}
	if f.FrameUnixNanos < f.CaptureStartUnixNanos || f.FrameUnixNanos > f.CaptureEndUnixNanos {
		return fmt.Errorf("frame %d: declared frame time lies outside the capture interval", f.Sequence)
	}
	if f.Payload&^knownPayload != 0 {
		return fmt.Errorf("frame %d: unknown payload flags %#x", f.Sequence, uint8(f.Payload))
	}
	if f.Payload.Has(PayloadMembership) && !f.Payload.Has(PayloadPoints) {
		return fmt.Errorf("frame %d: membership without its retained domain", f.Sequence)
	}
	seen := map[Stage]bool{}
	for _, stage := range f.Stages {
		if seen[stage.Stage] {
			return fmt.Errorf("frame %d: stage %s recorded twice", f.Sequence, stage.Stage)
		}
		seen[stage.Stage] = true
		if err := stage.validate(); err != nil {
			return fmt.Errorf("frame %d: %w", f.Sequence, err)
		}
	}
	if !f.Payload.Has(PayloadPoints) {
		if !f.Points.empty() {
			return fmt.Errorf("frame %d: points present without the points payload flag", f.Sequence)
		}
	} else if err := f.validatePoints(); err != nil {
		return fmt.Errorf("frame %d: %w", f.Sequence, err)
	}
	if !f.Payload.Has(PayloadMembership) {
		if len(f.Clusters) != 0 || len(f.Unassigned) != 0 || len(f.UnassignedReasons) != 0 {
			return fmt.Errorf("frame %d: membership present without the membership payload flag", f.Sequence)
		}
	} else if err := f.validateMembership(); err != nil {
		return fmt.Errorf("frame %d: %w", f.Sequence, err)
	}
	return nil
}

func (f FrameRecord) validateDisposition() error {
	d := f.Disposition
	switch d.Kind {
	case DispositionObserved, DispositionUnsettled:
		if d.Stage != "" || d.Reason != "" {
			return fmt.Errorf("a completed frame names no failing or suppressed stage")
		}
		if (d.Kind == DispositionUnsettled) != (f.Background == BackgroundSettling) {
			return fmt.Errorf("disposition %s disagrees with background state %s", d.Kind, f.Background)
		}
	case DispositionSuppressed, DispositionFailed:
		if d.Stage == "" || d.Reason == "" {
			return fmt.Errorf("a suppressed or failed frame must name its stage and reason")
		}
	default:
		return fmt.Errorf("disposition is unspecified")
	}
	return nil
}

func (f FrameRecord) validatePoints() error {
	p := f.Points
	n := p.Len()
	if len(p.Y) != n || len(p.Z) != n {
		return fmt.Errorf("coordinate columns have lengths %d, %d, %d", len(p.X), len(p.Y), len(p.Z))
	}
	if p.Fields&^knownPointFields != 0 {
		return fmt.Errorf("unknown point field flags %#x", uint16(p.Fields))
	}
	for _, column := range []struct {
		field  PointFields
		name   string
		length int
	}{
		{FieldAcquisitionTime, "time offset", len(p.TimeOffsetNanos)},
		{FieldIntensity, "intensity", len(p.Intensity)},
		{FieldChannel, "channel", len(p.Channel)},
		{FieldReturnIndex, "return index", len(p.ReturnIndex)},
		{FieldSourceOrdinal, "source ordinal", len(p.SourceOrdinal)},
		{FieldPacketSequence, "packet sequence", len(p.PacketSequence)},
		{FieldBlockIndex, "block index", len(p.BlockIndex)},
	} {
		want := 0
		if p.Fields.Has(column.field) {
			want = n
		}
		if column.length != want {
			return fmt.Errorf("%s column has %d values for %d points (present=%v)", column.name, column.length, n, p.Fields.Has(column.field))
		}
	}
	for i := 0; i < n; i++ {
		if !finite(p.X[i]) || !finite(p.Y[i]) || !finite(p.Z[i]) {
			return fmt.Errorf("point %d has a non-finite coordinate", i)
		}
	}
	if p.Fields.Has(FieldAcquisitionTime) {
		// Validate has already ordered start <= frame <= end, so lo <= 0 <= hi;
		// saturation keeps a hostile record from wrapping the bounds.
		lo := saturatingSub(f.CaptureStartUnixNanos, f.FrameUnixNanos)
		hi := saturatingSub(f.CaptureEndUnixNanos, f.FrameUnixNanos)
		for i, offset := range p.TimeOffsetNanos {
			if offset < lo || offset > hi {
				return fmt.Errorf("point %d was acquired outside the frame's capture interval", i)
			}
		}
	}
	for i := 1; i < len(p.SourceOrdinal); i++ {
		if p.SourceOrdinal[i] <= p.SourceOrdinal[i-1] {
			return fmt.Errorf("source ordinals are not strictly ascending at point %d", i)
		}
	}
	return nil
}

func (f FrameRecord) validateMembership() error {
	n := f.Points.Len()
	assigned := make([]uint64, (n+63)/64)
	mark := func(index uint32) error {
		if int(index) >= n {
			return fmt.Errorf("index %d outside a %d-point domain", index, n)
		}
		word, bit := index/64, uint64(1)<<(index%64)
		if assigned[word]&bit != 0 {
			return fmt.Errorf("point %d belongs to more than one partition cell", index)
		}
		assigned[word] |= bit
		return nil
	}
	total := 0
	ids := make(map[int64]bool, len(f.Clusters))
	for _, cluster := range f.Clusters {
		if ids[cluster.ClusterID] {
			return fmt.Errorf("cluster %d appears twice", cluster.ClusterID)
		}
		ids[cluster.ClusterID] = true
		if len(cluster.Members) == 0 {
			return fmt.Errorf("cluster %d has no members", cluster.ClusterID)
		}
		if cluster.Summary.PointsCount != len(cluster.Members) {
			return fmt.Errorf("cluster %d summary counts %d points, membership has %d", cluster.ClusterID, cluster.Summary.PointsCount, len(cluster.Members))
		}
		if !finite32(cluster.Summary.CentroidX) || !finite32(cluster.Summary.CentroidY) || !finite32(cluster.Summary.CentroidZ) {
			return fmt.Errorf("cluster %d has a non-finite centroid", cluster.ClusterID)
		}
		for i, member := range cluster.Members {
			if i > 0 && member <= cluster.Members[i-1] {
				return fmt.Errorf("cluster %d members are not sorted and unique", cluster.ClusterID)
			}
			if err := mark(member); err != nil {
				return fmt.Errorf("cluster %d: %w", cluster.ClusterID, err)
			}
		}
		total += len(cluster.Members)
	}
	if len(f.UnassignedReasons) != len(f.Unassigned) {
		return fmt.Errorf("%d unassigned points have %d reasons", len(f.Unassigned), len(f.UnassignedReasons))
	}
	for i, index := range f.Unassigned {
		if i > 0 && index <= f.Unassigned[i-1] {
			return fmt.Errorf("unassigned points are not sorted and unique")
		}
		if f.UnassignedReasons[i] > maxRejectionReason {
			return fmt.Errorf("unassigned point %d has unknown reason %d", index, f.UnassignedReasons[i])
		}
		if err := mark(index); err != nil {
			return fmt.Errorf("unassigned: %w", err)
		}
	}
	if covered := total + len(f.Unassigned); covered != n {
		return fmt.Errorf("partition covers %d of %d retained points", covered, n)
	}
	if stage, ok := f.Stage(StageL4Cluster); ok && stage.Output.Known && stage.Output.Value != uint64(total) {
		return fmt.Errorf("cluster stage outputs %d points, membership has %d", stage.Output.Value, total)
	}
	return nil
}

// ValidateFor applies Validate and then the guarantees profile makes about
// every record it labels: required point columns, a domain equal to the L3
// foreground output, recorded stage counts, and membership on every
// completed frame.
func (f FrameRecord) ValidateFor(profile Profile) error {
	if err := f.Validate(); err != nil {
		return err
	}
	caps := profile.Capabilities
	if caps.Has(CapabilityStageCounts) {
		if l2, ok := f.Stage(StageL2Frame); !ok || !l2.Output.Known {
			return fmt.Errorf("frame %d: profile %s requires the L2 return count", f.Sequence, profile.Name)
		}
	}
	completed := f.Disposition.Kind == DispositionObserved || f.Disposition.Kind == DispositionUnsettled
	if caps.Has(CapabilityClusterMembership) && completed && !f.Payload.Has(PayloadMembership) {
		return fmt.Errorf("frame %d: profile %s requires membership on a completed frame", f.Sequence, profile.Name)
	}
	if !f.Payload.Has(PayloadPoints) {
		return nil
	}
	for _, need := range []struct {
		capability Capability
		field      PointFields
	}{
		{CapabilityPointAcquisitionTime, FieldAcquisitionTime},
		{CapabilityPointIntensity, FieldIntensity},
		{CapabilityPointChannel, FieldChannel},
		{CapabilityPointSourceOrdinal, FieldSourceOrdinal},
	} {
		if caps.Has(need.capability) && !f.Points.Fields.Has(need.field) {
			return fmt.Errorf("frame %d: profile %s requires capability %s", f.Sequence, profile.Name, need.capability)
		}
	}
	if caps.Has(CapabilityCompleteForeground) {
		l3, ok := f.Stage(StageL3Foreground)
		switch {
		case ok && l3.Output.Known && l3.Output.Value != uint64(f.Points.Len()):
			return fmt.Errorf("frame %d: retained %d points of %d L3 foreground returns", f.Sequence, f.Points.Len(), l3.Output.Value)
		case (!ok || !l3.Output.Known) && f.Points.Len() != 0:
			return fmt.Errorf("frame %d: retained points without a known L3 foreground count", f.Sequence)
		}
	}
	return nil
}

// TimeCertainty qualifies a gap's time bounds.
type TimeCertainty uint8

const (
	// GapTimeUnknown: the loss cannot be placed in capture time.
	GapTimeUnknown TimeCertainty = iota
	// GapTimeBounded: the missing interval lies within [Start, End]; it need
	// not fill it.
	GapTimeBounded
)

// GapRecord makes a missing interval explicit. Readers must not treat a gap
// as evidence that the road was empty.
type GapRecord struct {
	// HasSequenceRange marks a gap over sequence numbers that had been
	// assigned before their records were lost (for example, a salvaged
	// capture). Without it, the gap lies between two received frames and
	// consumes no sequence numbers: the missing rotations were never received,
	// and inventing sequence numbers for them would invent frames.
	HasSequenceRange bool
	FirstSequence    uint64
	LastSequence     uint64
	// MissingFrames is the number of source rotations lost, when the source
	// can count them.
	MissingFrames  Count
	Time           TimeCertainty
	StartUnixNanos int64
	EndUnixNanos   int64
	// Cause names the mechanism, e.g. an L2 callback-queue overflow.
	Cause string
}

// Validate checks the gap on its own; placement is the stream's concern.
func (g GapRecord) Validate() error {
	if g.Cause == "" {
		return fmt.Errorf("gap needs a cause")
	}
	if g.HasSequenceRange {
		if g.FirstSequence > g.LastSequence {
			return fmt.Errorf("gap sequence range %d..%d is reversed", g.FirstSequence, g.LastSequence)
		}
		if span := g.LastSequence - g.FirstSequence + 1; g.MissingFrames.Known && g.MissingFrames.Value != span {
			return fmt.Errorf("gap over %d sequences claims %d missing frames", span, g.MissingFrames.Value)
		}
	} else if g.FirstSequence != 0 || g.LastSequence != 0 {
		return fmt.Errorf("gap without a sequence range names sequences")
	}
	switch g.Time {
	case GapTimeUnknown:
		if g.StartUnixNanos != 0 || g.EndUnixNanos != 0 {
			return fmt.Errorf("gap with unknown time carries time bounds")
		}
	case GapTimeBounded:
		if g.StartUnixNanos > g.EndUnixNanos {
			return fmt.Errorf("gap time bounds are reversed")
		}
	default:
		return fmt.Errorf("unknown gap time certainty %d", g.Time)
	}
	return nil
}

func finite(v float64) bool   { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func finite32(v float32) bool { return finite(float64(v)) }

func saturatingSub(a, b int64) int64 {
	d := a - b
	switch {
	case b < 0 && d < a: // positive overflow
		return math.MaxInt64
	case b > 0 && d > a: // negative overflow
		return math.MinInt64
	}
	return d
}
