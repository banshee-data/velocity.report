package l4bobserve

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Extraction identifies one run of the extractor over one ordered source. It
// is the header a later manifest carries once; frame records do not repeat it.
type Extraction struct {
	SourceID      string // l4bobserve.SourceID: ordered capture digests plus extractor identity
	CalibrationID string
	SensorID      string
	// CoordinateFrame names the frame of the retained XYZ, matching the legacy
	// records' cluster FrameID ("site/<sensor>").
	CoordinateFrame string
	Profile         Profile
}

// ExtractionBuilder assembles foreground-complete FrameRecords for one
// extraction. It owns the sequence counter, so every frame handed to
// BeginFrame and finished receives the next number: failed and empty frames
// included. It is not safe for concurrent use; the pipeline callback that
// drives it is serial.
type ExtractionBuilder struct {
	extraction      Extraction
	next            uint64
	lastPacket      uint32
	lastPacketKnown bool
	// codes is per-frame scratch, reused so a long capture does not allocate
	// a fresh accounting array for every frame. codesOwner guards against a
	// stale draft reading another frame's accounting.
	codes      []int32
	codesOwner *FrameDraft
}

// NewExtractionBuilder requires the identities a record must be traceable to.
// They are supplied by the capture and pose owner, never guessed from a
// sensor name. The builder produces only the foreground-complete profile; an
// empty Profile is filled in, and any other is refused rather than mislabelled.
func NewExtractionBuilder(e Extraction) (*ExtractionBuilder, error) {
	for name, value := range map[string]string{"source": e.SourceID, "calibration": e.CalibrationID, "sensor": e.SensorID, "coordinate frame": e.CoordinateFrame} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("extraction needs a %s identity", name)
		}
	}
	declared := ForegroundComplete()
	switch {
	case e.Profile.Name == "" && len(e.Profile.Capabilities.items) == 0:
		e.Profile = declared
	case e.Profile.Name != declared.Name || !slices.Equal(e.Profile.Capabilities.items, declared.Capabilities.items):
		return nil, fmt.Errorf("the frame builder produces only profile %q, not %q", declared.Name, e.Profile.Name)
	}
	return &ExtractionBuilder{extraction: e}, nil
}

// Extraction returns the header the records belong to.
func (b *ExtractionBuilder) Extraction() Extraction { return b.extraction }

// NextSequence is the number the next finished frame will receive.
func (b *ExtractionBuilder) NextSequence() uint64 { return b.next }

// RecordGap declares a missing interval at the current position. A gap over
// assigned sequences advances the counter past them. Either kind breaks packet
// continuity: the next frame's leading loss cannot be told from the gap's.
func (b *ExtractionBuilder) RecordGap(g GapRecord) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if g.HasSequenceRange {
		if g.FirstSequence != b.next {
			return fmt.Errorf("gap covers sequences from %d where %d is next", g.FirstSequence, b.next)
		}
		b.next = g.LastSequence + 1
	}
	b.lastPacketKnown = false
	return nil
}

// BeginFrame opens a draft for one L2 frame. A nil builder or frame yields a
// nil draft, whose methods all do nothing: that is how a disabled tap costs
// the pipeline one nil check per stage.
func (b *ExtractionBuilder) BeginFrame(frame *l2frames.LiDARFrame) *FrameDraft {
	if b == nil || frame == nil {
		return nil
	}
	d := &FrameDraft{
		builder:    b,
		frame:      frame,
		frameNanos: unixNanos(frame.StartTimestamp),
		stages:     make([]StageCount, 1, 6),
		lastStage:  StageL2Frame,
	}
	d.stages[0] = StageCount{Stage: StageL2Frame, Output: KnownCount(len(frame.PolarPoints))}
	first := true
	for seq := range frame.ReceivedPackets {
		if seq != 0 {
			d.numbered = true
		}
		if first || seq < d.minPacket {
			d.minPacket = seq
		}
		if first || seq > d.maxPacket {
			d.maxPacket = seq
		}
		first = false
	}
	return d
}

// completeness attributes packets lost at a rotation boundary to the later
// frame. A packet split across a seam appears in both frames, so a first
// packet equal to the previous frame's last is continuity, not loss.
func (b *ExtractionBuilder) completeness(d *FrameDraft) Completeness {
	frame := d.frame
	if !d.numbered {
		// Unnumbered packets (1262-byte Hesai packets) cannot show loss at all,
		// and give the next numbered frame no boundary to measure from.
		b.lastPacketKnown = false
		if !frame.SpinComplete {
			return Completeness{State: CompletenessPartial}
		}
		return Completeness{State: CompletenessUnknown}
	}
	var lost Count
	if b.lastPacketKnown && frame.PacketGaps >= 0 {
		if step := d.minPacket - b.lastPacket; step < math.MaxInt32 { // modular: forward steps only
			leading := uint64(0)
			if step > 1 {
				leading = uint64(step - 1)
			}
			lost = Count{Value: uint64(frame.PacketGaps) + leading, Known: true}
		}
	}
	b.lastPacket, b.lastPacketKnown = d.maxPacket, true
	switch {
	case lost.Known && lost.Value == 0 && frame.SpinComplete:
		return Completeness{State: CompletenessComplete, LostPackets: lost}
	case (lost.Known && lost.Value > 0) || !frame.SpinComplete:
		return Completeness{State: CompletenessPartial, LostPackets: lost}
	default:
		// The first frame, or the first after a gap: its leading boundary is
		// unverifiable, so completeness is unknown rather than assumed.
		return Completeness{State: CompletenessUnknown}
	}
}

// Accounting codes for a retained return while a draft is open. Non-negative
// codes are cluster slots; codeAlive means it survived every stage so far.
const codeAlive int32 = -1

func rejectedCode(reason RejectionReason) int32 { return -2 - int32(reason) }

// FrameDraft accumulates one frame's evidence as the pipeline reports each
// stage. Its methods never return errors to the pipeline: a pipeline failure
// is evidence (Fail), and a broken lineage turns the frame into a failed
// record at Finish, so the hot path stays a sequence of plain calls.
type FrameDraft struct {
	builder    *ExtractionBuilder
	frame      *l2frames.LiDARFrame
	frameNanos int64
	numbered   bool
	minPacket  uint32
	maxPacket  uint32

	background   BackgroundState
	mask         []bool
	l3Done       bool
	l3Foreground int
	retained     bool
	points       RetainedPoints
	codes        []int32
	alive        int
	clustered    bool
	clusters     []ClusterRecord
	members      int
	stages       []StageCount
	lastStage    Stage

	failure     *Disposition
	suppression *Disposition
	err         error
	finished    bool
}

func (d *FrameDraft) open() bool {
	return d != nil && !d.finished && d.failure == nil && d.suppression == nil
}

// Finished reports whether Finish has been called. A nil draft is finished.
func (d *FrameDraft) Finished() bool { return d == nil || d.finished }

// SetBackground records the L3 settling state after this frame.
func (d *FrameDraft) SetBackground(state BackgroundState) {
	if d != nil && !d.finished {
		d.background = state
	}
}

// Fail records a pipeline failure at stage. The first failure wins.
func (d *FrameDraft) Fail(stage Stage, reason string) {
	if d != nil && !d.finished {
		d.setFailure(stage, reason)
	}
}

func (d *FrameDraft) setFailure(stage Stage, reason string) {
	if d.failure == nil {
		d.failure = &Disposition{Kind: DispositionFailed, Stage: stage, Reason: reason}
	}
}

// Suppress records that a named policy skipped stage and everything after it.
func (d *FrameDraft) Suppress(stage Stage, policy string) {
	if d.open() {
		d.suppression = &Disposition{Kind: DispositionSuppressed, Stage: stage, Reason: policy}
	}
}

func (d *FrameDraft) lineageFailure(stage Stage, format string, args ...any) {
	err := fmt.Errorf("frame %s %s: %s", d.frame.FrameID, stage, fmt.Sprintf(format, args...))
	if d.err == nil {
		d.err = err
	}
	d.setFailure(stage, "acquisition lineage broken: "+err.Error())
}

// SetForeground records L3's output: mask over the L2 returns and the number
// of foreground returns the pipeline extracted from it.
func (d *FrameDraft) SetForeground(mask []bool, foreground int) {
	if !d.open() {
		return
	}
	if d.l3Done {
		d.lineageFailure(StageL3Foreground, "foreground reported twice")
		return
	}
	d.mask, d.l3Done, d.l3Foreground, d.lastStage = mask, true, foreground, StageL3Foreground
	d.stages = append(d.stages, StageCount{Stage: StageL3Foreground, Input: KnownCount(len(mask)),
		Output: KnownCount(foreground), Rejected: KnownCount(len(mask) - foreground)})
}

func (d *FrameDraft) pointFields() PointFields {
	fields := FieldAcquisitionTime | FieldIntensity | FieldChannel | FieldSourceOrdinal | FieldBlockIndex
	if d.numbered {
		fields |= FieldPacketSequence
	}
	return fields
}

// Retain copies the retained domain: the L3 foreground returns in source
// order, with world holding exactly the L4 coordinates computed for them. It
// must run before any filter, because the height-band filter compacts world
// in place. It also stamps each world point's source ordinal, which is how
// the later stages' outputs are traced back to this domain.
func (d *FrameDraft) Retain(world []l4perception.WorldPoint) {
	if !d.open() {
		return
	}
	d.lastStage = StageL4Transform
	polar := d.frame.PolarPoints
	switch {
	case !d.l3Done || d.retained:
		d.lineageFailure(StageL4Transform, "retained domain reported out of order")
		return
	case len(d.mask) != len(polar):
		d.lineageFailure(StageL4Transform, "L3 mask covers %d of %d returns", len(d.mask), len(polar))
		return
	case len(world) != d.l3Foreground:
		d.lineageFailure(StageL4Transform, "%d world points for %d foreground returns", len(world), d.l3Foreground)
		return
	}
	n := len(world)
	xyz := make([]float64, 3*n)
	points := RetainedPoints{
		Fields: d.pointFields(),
		X:      xyz[0:n:n], Y: xyz[n : 2*n : 2*n], Z: xyz[2*n : 3*n : 3*n],
		TimeOffsetNanos: make([]int64, n),
		Intensity:       make([]uint8, n),
		Channel:         make([]uint16, n),
		SourceOrdinal:   make([]uint32, n),
		BlockIndex:      make([]uint16, n),
	}
	if d.numbered {
		points.PacketSequence = make([]uint32, n)
	}
	k := 0
	for i, foreground := range d.mask {
		if !foreground {
			continue
		}
		if k == n {
			d.lineageFailure(StageL4Transform, "L3 mask marks more foreground than the %d extracted", n)
			return
		}
		p, w := polar[i], &world[k]
		// World points carry intensity and acquisition time through the
		// transform unchanged, so a mismatch means the domain is misaligned.
		if w.Intensity != p.Intensity || w.Timestamp.UnixNano() != p.Timestamp {
			d.lineageFailure(StageL4Transform, "world point %d is not the transform of return %d", k, i)
			return
		}
		if p.Channel < 0 || p.Channel > math.MaxUint16 || p.BlockID < 0 || p.BlockID > math.MaxUint16 || !w.SetSourceOrdinal(i) {
			d.lineageFailure(StageL4Transform, "return %d has an unrepresentable channel, block or ordinal", i)
			return
		}
		points.X[k], points.Y[k], points.Z[k] = w.X, w.Y, w.Z
		points.TimeOffsetNanos[k] = p.Timestamp - d.frameNanos
		points.Intensity[k] = p.Intensity
		points.Channel[k] = uint16(p.Channel)
		points.SourceOrdinal[k] = uint32(i)
		points.BlockIndex[k] = uint16(p.BlockID)
		if d.numbered {
			points.PacketSequence[k] = p.UDPSequence
		}
		k++
	}
	if k != n {
		d.lineageFailure(StageL4Transform, "L3 mask marks %d foreground returns, %d were extracted", k, n)
		return
	}
	b := d.builder
	if cap(b.codes) < n {
		b.codes = make([]int32, n)
	}
	b.codesOwner = d
	d.codes = b.codes[:n]
	for i := range d.codes {
		d.codes[i] = codeAlive
	}
	d.points, d.retained, d.alive = points, true, n
}

// index maps a stage's output point back to the retained domain.
func (d *FrameDraft) index(p l4perception.WorldPoint) (int, bool) {
	ordinal, ok := p.SourceOrdinal()
	if !ok || ordinal > math.MaxUint32 {
		return 0, false
	}
	return slices.BinarySearch(d.points.SourceOrdinal, uint32(ordinal))
}

// narrow applies one filtering stage: every return still alive is rejected
// with reason unless it appears in survivors; returns in special are rejected
// with specialReason instead. A survivor that was not alive, or appears twice,
// is a broken lineage.
func (d *FrameDraft) narrow(stage Stage, survivors []l4perception.WorldPoint, reason RejectionReason, special []l4perception.WorldPoint, specialReason RejectionReason) {
	if !d.open() {
		return
	}
	d.lastStage = stage
	if !d.retained || d.clustered {
		d.lineageFailure(stage, "stage reported outside the retained domain's lifetime")
		return
	}
	input, rejected := d.alive, rejectedCode(reason)
	for k, code := range d.codes {
		if code == codeAlive {
			d.codes[k] = rejected
		}
	}
	for _, p := range special {
		k, ok := d.index(p)
		if !ok || d.codes[k] != rejected {
			d.lineageFailure(stage, "rejected point has no live source ordinal")
			return
		}
		d.codes[k] = rejectedCode(specialReason)
	}
	for _, p := range survivors {
		k, ok := d.index(p)
		if !ok || d.codes[k] != rejected {
			d.lineageFailure(stage, "surviving point has no live source ordinal, or appears twice")
			return
		}
		d.codes[k] = codeAlive
	}
	d.alive = len(survivors)
	d.stages = append(d.stages, StageCount{Stage: stage, Input: KnownCount(input),
		Output: KnownCount(len(survivors)), Rejected: KnownCount(input - len(survivors))})
}

// HeightBandFiltered reports the absolute sensor-frame height band's output.
func (d *FrameDraft) HeightBandFiltered(kept []l4perception.WorldPoint) {
	d.narrow(StageL4HeightFilter, kept, RejectionHeightBand, nil, RejectionUnknown)
}

// SurfaceFiltered reports the surface-relative height filter's output and the
// returns it rejected below the floor; the remainder were above the ceiling.
func (d *FrameDraft) SurfaceFiltered(kept, lowerRejected []l4perception.WorldPoint) {
	d.narrow(StageL4HeightFilter, kept, RejectionSurfaceAboveCeiling, lowerRejected, RejectionSurfaceBelowFloor)
}

// Voxelled reports voxel downsampling's representatives. Each representative
// is an original return, so it keeps its own identity; the other returns in
// its voxel are rejected as reduced. The voxel-to-representative contributor
// map is not recorded yet.
func (d *FrameDraft) Voxelled(representatives []l4perception.WorldPoint) {
	d.narrow(StageL4Voxel, representatives, RejectionVoxelReduced, nil, RejectionUnknown)
}

// Clustered reports DBSCAN's trace and the clusters that survived its size
// and aspect filters, after any later adjustment of their summaries (such as
// MarkGroundClipped). It concludes the frame's membership.
func (d *FrameDraft) Clustered(trace l4perception.DBSCANTrace, clusters []l4perception.WorldCluster) {
	if !d.open() {
		return
	}
	d.lastStage = StageL4Cluster
	switch {
	case !d.retained || d.clustered:
		d.lineageFailure(StageL4Cluster, "clusters reported outside the retained domain's lifetime")
		return
	case trace.InputPoints != d.alive:
		d.lineageFailure(StageL4Cluster, "DBSCAN received %d points, %d survived filtering", trace.InputPoints, d.alive)
		return
	case len(trace.Labels) != len(trace.Processed):
		d.lineageFailure(StageL4Cluster, "%d labels for %d processed points", len(trace.Labels), len(trace.Processed))
		return
	}
	input, capped := d.alive, rejectedCode(RejectionClusterInputCap)
	for k, code := range d.codes {
		if code == codeAlive {
			d.codes[k] = capped
		}
	}
	maxLabel := 0
	for _, label := range trace.Labels {
		maxLabel = max(maxLabel, label)
	}
	slots := make([]int32, maxLabel+1)
	for i := range slots {
		slots[i] = -1
	}
	for i, cluster := range clusters {
		if cluster.ClusterID < 1 || cluster.ClusterID > int64(maxLabel) || slots[cluster.ClusterID] >= 0 {
			d.lineageFailure(StageL4Cluster, "cluster %d has no unique DBSCAN label", cluster.ClusterID)
			return
		}
		slots[cluster.ClusterID] = int32(i)
	}
	counts := make([]int, len(clusters))
	for i, p := range trace.Processed {
		k, ok := d.index(p)
		if !ok || d.codes[k] != capped {
			d.lineageFailure(StageL4Cluster, "clustered point has no live source ordinal, or appears twice")
			return
		}
		switch label := trace.Labels[i]; {
		case label < 0:
			d.codes[k] = rejectedCode(RejectionClusterNoise)
		case label == 0:
			d.lineageFailure(StageL4Cluster, "DBSCAN left processed point %d unvisited", i)
			return
		case slots[label] < 0:
			d.codes[k] = rejectedCode(RejectionClusterShape)
		default:
			d.codes[k] = slots[label]
			counts[slots[label]]++
		}
	}
	members := 0
	for i, cluster := range clusters {
		if counts[i] != cluster.PointsCount {
			d.lineageFailure(StageL4Cluster, "cluster %d summary counts %d points, lineage finds %d", cluster.ClusterID, cluster.PointsCount, counts[i])
			return
		}
		members += counts[i]
	}
	backing := make([]uint32, members)
	d.clusters = make([]ClusterRecord, len(clusters))
	offset := 0
	for i, cluster := range clusters {
		d.clusters[i] = ClusterRecord{ClusterID: cluster.ClusterID, Members: backing[offset : offset : offset+counts[i]], Summary: summarise(cluster)}
		offset += counts[i]
	}
	processed := len(trace.Processed)
	d.stages = append(d.stages,
		StageCount{Stage: StageL4ClusterInputCap, Input: KnownCount(input), Output: KnownCount(processed), Rejected: KnownCount(input - processed)},
		StageCount{Stage: StageL4Cluster, Input: KnownCount(processed), Output: KnownCount(members), Rejected: KnownCount(processed - members)})
	d.alive, d.members, d.clustered = 0, members, true
}

func summarise(c l4perception.WorldCluster) ClusterSummary {
	s := ClusterSummary{
		FirstMemberUnixNanos: c.TSUnixNanos,
		CentroidX:            c.CentroidX, CentroidY: c.CentroidY, CentroidZ: c.CentroidZ,
		BoundingBoxLength: c.BoundingBoxLength, BoundingBoxWidth: c.BoundingBoxWidth, BoundingBoxHeight: c.BoundingBoxHeight,
		HeightP95: c.HeightP95, IntensityMean: c.IntensityMean,
		PointsCount: c.PointsCount, GroundClipped: c.GroundClipped,
	}
	if c.OBB != nil {
		s.OBB = &OrientedBox{
			CenterX: c.OBB.CenterX, CenterY: c.OBB.CenterY, CenterZ: c.OBB.CenterZ,
			Length: c.OBB.Length, Width: c.OBB.Width, Height: c.OBB.Height, HeadingRad: c.OBB.HeadingRad,
		}
	}
	return s
}

var (
	errNilDraft      = errors.New("frame draft is nil")
	errDraftFinished = errors.New("frame draft already finished")
)

// Finish assigns the frame its sequence number and returns its record, which
// is always valid for the extraction's profile. A nil error means the record
// is exactly what the pipeline reported. A non-nil error means the draft broke
// its own contract (a lost lineage, or a record failing validation); the
// record is then a failed frame naming the stage, so the stream stays dense
// and the fault stays visible, and the error carries the detail.
func (d *FrameDraft) Finish() (FrameRecord, error) {
	if d == nil {
		return FrameRecord{}, errNilDraft
	}
	if d.finished {
		return FrameRecord{}, errDraftFinished
	}
	d.finished = true
	b := d.builder
	if d.retained && b.codesOwner != d {
		d.lineageFailure(d.lastStage, "accounting scratch was reused by a later frame")
	}
	rec := FrameRecord{
		Sequence:              b.next,
		SensorFrameID:         d.frame.FrameID,
		FrameUnixNanos:        d.frameNanos,
		CaptureStartUnixNanos: d.frameNanos,
		CaptureEndUnixNanos:   unixNanos(d.frame.EndTimestamp),
		Completeness:          b.completeness(d),
		Background:            d.background,
		Stages:                d.stages,
	}
	b.next++
	emptyDomain := (!d.l3Done && len(d.frame.PolarPoints) == 0) || (d.l3Done && d.l3Foreground == 0)
	concluded := emptyDomain || (d.retained && (d.alive == 0 || d.clustered))
	switch {
	case d.failure != nil:
		rec.Disposition = *d.failure
	case d.suppression != nil:
		rec.Disposition = *d.suppression
	case !concluded:
		d.lineageFailure(d.lastStage, "extraction ended before every retained return was accounted for")
		rec.Disposition = *d.failure
	case d.background == BackgroundSettling:
		rec.Disposition = Disposition{Kind: DispositionUnsettled}
	default:
		rec.Disposition = Disposition{Kind: DispositionObserved}
	}
	switch {
	case d.retained:
		rec.Payload |= PayloadPoints
		rec.Points = d.points
	case emptyDomain:
		rec.Payload |= PayloadPoints
		rec.Points = RetainedPoints{Fields: d.pointFields()}
	}
	completed := rec.Disposition.Kind == DispositionObserved || rec.Disposition.Kind == DispositionUnsettled
	if concluded && completed {
		rec.Payload |= PayloadMembership
		d.membership(&rec)
	}
	if d.retained && b.codesOwner == d {
		b.codesOwner = nil
	}
	if err := rec.ValidateFor(b.extraction.Profile); err != nil {
		return d.contractFailure(rec, err), err
	}
	return rec, d.err
}

func (d *FrameDraft) membership(rec *FrameRecord) {
	if !d.retained {
		return // an empty domain has no clusters and nothing unassigned
	}
	unassigned := len(d.codes) - d.members
	rec.Unassigned = make([]uint32, 0, unassigned)
	rec.UnassignedReasons = make([]RejectionReason, 0, unassigned)
	for k, code := range d.codes {
		if code >= 0 {
			d.clusters[code].Members = append(d.clusters[code].Members, uint32(k))
			continue
		}
		// codeAlive cannot remain once concluded; if it did, recording it as
		// an unknown rejection keeps the partition exact and the reason honest.
		reason := RejectionUnknown
		if code != codeAlive {
			reason = RejectionReason(-code - 2)
		}
		rec.Unassigned = append(rec.Unassigned, uint32(k))
		rec.UnassignedReasons = append(rec.UnassignedReasons, reason)
	}
	rec.Clusters = d.clusters
}

// contractFailure reduces a record that failed validation to a failed frame
// that carries only what cannot be wrong: its identity, times and L2 count.
func (d *FrameDraft) contractFailure(rec FrameRecord, err error) FrameRecord {
	if d.err == nil {
		d.err = err
	}
	return FrameRecord{
		Sequence:              rec.Sequence,
		SensorFrameID:         rec.SensorFrameID,
		FrameUnixNanos:        rec.FrameUnixNanos,
		CaptureStartUnixNanos: rec.CaptureStartUnixNanos,
		CaptureEndUnixNanos:   max(rec.CaptureStartUnixNanos, rec.CaptureEndUnixNanos),
		Background:            rec.Background,
		Disposition:           Disposition{Kind: DispositionFailed, Stage: d.lastStage, Reason: "record failed its evidence contract: " + err.Error()},
		Stages:                []StageCount{{Stage: StageL2Frame, Output: KnownCount(len(d.frame.PolarPoints))}},
	}
}

// unixNanos maps the zero time to zero: time.Time{}.UnixNano is undefined,
// and a frame without a timestamp must not acquire an arbitrary one.
func unixNanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}
