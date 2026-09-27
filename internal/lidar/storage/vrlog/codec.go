package vrlog

import (
	"fmt"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// marshalOptions are fixed so a writer's bytes depend only on the record.
// Stored bytes are still not a cross-language identity; the semantic digest is.
var marshalOptions = proto.MarshalOptions{Deterministic: true}

// unmarshalOptions bound recursion (the deepest record, frame → membership →
// cluster → summary → box, is five levels) and drop fields this reader does
// not know: an additive field is ignorable by definition, and anything a
// reader must not ignore is a manifest required feature instead.
var unmarshalOptions = proto.UnmarshalOptions{DiscardUnknown: true, RecursionLimit: 8}

// frameToProto maps a validated record. Float64, int64, uint32 and byte
// columns are shared with the record rather than copied: the message only
// lives until it is marshalled.
func frameToProto(f l4bobserve.FrameRecord) *pb.FrameRecord {
	m := &pb.FrameRecord{
		Sequence:              f.Sequence,
		SensorFrameId:         f.SensorFrameID,
		FrameUnixNanos:        f.FrameUnixNanos,
		CaptureStartUnixNanos: f.CaptureStartUnixNanos,
		CaptureEndUnixNanos:   f.CaptureEndUnixNanos,
		Completeness:          pb.Completeness(f.Completeness.State),
		LostPackets:           countToProto(f.Completeness.LostPackets),
		Background:            pb.BackgroundState(f.Background),
		Disposition:           pb.Disposition(f.Disposition.Kind),
		DispositionStage:      string(f.Disposition.Stage),
		DispositionReason:     f.Disposition.Reason,
	}
	if len(f.Stages) > 0 {
		m.Stages = make([]*pb.StageCount, len(f.Stages))
		for i, s := range f.Stages {
			m.Stages[i] = &pb.StageCount{Stage: string(s.Stage), Input: countToProto(s.Input),
				Output: countToProto(s.Output), Rejected: countToProto(s.Rejected)}
		}
	}
	if f.Payload.Has(l4bobserve.PayloadPoints) {
		p := f.Points
		m.Points = &pb.RetainedPoints{
			PointCount: uint32(p.Len()), Columns: uint32(p.Fields),
			X: p.X, Y: p.Y, Z: p.Z,
			TimeOffsetNanos: p.TimeOffsetNanos,
			Intensity:       p.Intensity,
			Channel:         widen16(p.Channel),
			ReturnIndex:     p.ReturnIndex,
			SourceOrdinal:   p.SourceOrdinal,
			PacketSequence:  p.PacketSequence,
			BlockIndex:      widen16(p.BlockIndex),
		}
	}
	if f.Payload.Has(l4bobserve.PayloadMembership) {
		membership := &pb.Membership{Unassigned: f.Unassigned}
		if len(f.UnassignedReasons) > 0 {
			membership.UnassignedReasons = make([]byte, len(f.UnassignedReasons))
			for i, r := range f.UnassignedReasons {
				membership.UnassignedReasons[i] = byte(r)
			}
		}
		if len(f.Clusters) > 0 {
			membership.Clusters = make([]*pb.Cluster, len(f.Clusters))
			for i, c := range f.Clusters {
				membership.Clusters[i] = &pb.Cluster{ClusterId: c.ClusterID, Members: c.Members, Summary: summaryToProto(c.Summary)}
			}
		}
		m.Membership = membership
	}
	return m
}

func summaryToProto(s l4bobserve.ClusterSummary) *pb.ClusterSummary {
	m := &pb.ClusterSummary{
		FirstMemberUnixNanos: s.FirstMemberUnixNanos,
		CentroidX:            s.CentroidX, CentroidY: s.CentroidY, CentroidZ: s.CentroidZ,
		BoundingBoxLength: s.BoundingBoxLength, BoundingBoxWidth: s.BoundingBoxWidth, BoundingBoxHeight: s.BoundingBoxHeight,
		HeightP95: s.HeightP95, IntensityMean: s.IntensityMean,
		PointsCount: uint64(s.PointsCount), GroundClipped: s.GroundClipped,
	}
	if o := s.OBB; o != nil {
		m.Obb = &pb.OrientedBox{CenterX: o.CenterX, CenterY: o.CenterY, CenterZ: o.CenterZ,
			Length: o.Length, Width: o.Width, Height: o.Height, HeadingRad: o.HeadingRad}
	}
	return m
}

func gapToProto(g l4bobserve.GapRecord) *pb.GapRecord {
	m := &pb.GapRecord{
		MissingFrames:  countToProto(g.MissingFrames),
		Time:           pb.GapTime(g.Time),
		StartUnixNanos: g.StartUnixNanos,
		EndUnixNanos:   g.EndUnixNanos,
		Cause:          g.Cause,
	}
	if g.HasSequenceRange {
		m.SequenceRange = &pb.SequenceRange{First: g.FirstSequence, Last: g.LastSequence}
	}
	return m
}

func countToProto(c l4bobserve.Count) *uint64 {
	if !c.Known {
		return nil
	}
	v := c.Value
	return &v
}

func countFromProto(v *uint64) l4bobserve.Count {
	if v == nil {
		return l4bobserve.Count{}
	}
	return l4bobserve.Count{Value: *v, Known: true}
}

func widen16(values []uint16) []uint32 {
	if len(values) == 0 {
		return nil
	}
	out := make([]uint32, len(values))
	for i, v := range values {
		out[i] = uint32(v)
	}
	return out
}

func narrow16(name string, values []uint32) ([]uint16, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]uint16, len(values))
	for i, v := range values {
		if v > math.MaxUint16 {
			return nil, fmt.Errorf("%s value %d at point %d exceeds 16 bits", name, v, i)
		}
		out[i] = uint16(v)
	}
	return out, nil
}

// decodeFrame bounds, decodes and maps one frame payload. It refuses enum
// values it does not know rather than mapping them onto a default, but it
// does not apply the profile: the caller validates the record it returns.
func decodeFrame(payload []byte, limits Limits) (l4bobserve.FrameRecord, error) {
	if err := prescanFrame(payload, limits); err != nil {
		return l4bobserve.FrameRecord{}, err
	}
	var m pb.FrameRecord
	if err := unmarshalOptions.Unmarshal(payload, &m); err != nil {
		return l4bobserve.FrameRecord{}, fmt.Errorf("decode frame: %w", err)
	}
	switch {
	case m.Completeness < 0 || m.Completeness > pb.Completeness_COMPLETENESS_PARTIAL:
		return l4bobserve.FrameRecord{}, fmt.Errorf("unknown completeness %d", m.Completeness)
	case m.Background < 0 || m.Background > pb.BackgroundState_BACKGROUND_STATE_SETTLED:
		return l4bobserve.FrameRecord{}, fmt.Errorf("unknown background state %d", m.Background)
	case m.Disposition < 0 || m.Disposition > pb.Disposition_DISPOSITION_FAILED:
		return l4bobserve.FrameRecord{}, fmt.Errorf("unknown disposition %d", m.Disposition)
	}
	f := l4bobserve.FrameRecord{
		Sequence:              m.Sequence,
		SensorFrameID:         m.SensorFrameId,
		FrameUnixNanos:        m.FrameUnixNanos,
		CaptureStartUnixNanos: m.CaptureStartUnixNanos,
		CaptureEndUnixNanos:   m.CaptureEndUnixNanos,
		Completeness: l4bobserve.Completeness{
			State:       l4bobserve.CompletenessState(m.Completeness),
			LostPackets: countFromProto(m.LostPackets),
		},
		Background: l4bobserve.BackgroundState(m.Background),
		Disposition: l4bobserve.Disposition{
			Kind: l4bobserve.DispositionKind(m.Disposition), Stage: l4bobserve.Stage(m.DispositionStage), Reason: m.DispositionReason,
		},
	}
	if len(m.Stages) > 0 {
		f.Stages = make([]l4bobserve.StageCount, len(m.Stages))
		for i, s := range m.Stages {
			f.Stages[i] = l4bobserve.StageCount{Stage: l4bobserve.Stage(s.Stage), Input: countFromProto(s.Input),
				Output: countFromProto(s.Output), Rejected: countFromProto(s.Rejected)}
		}
	}
	if p := m.Points; p != nil {
		f.Payload |= l4bobserve.PayloadPoints
		channel, err := narrow16("channel", p.Channel)
		if err != nil {
			return l4bobserve.FrameRecord{}, err
		}
		block, err := narrow16("block index", p.BlockIndex)
		if err != nil {
			return l4bobserve.FrameRecord{}, err
		}
		f.Points = l4bobserve.RetainedPoints{
			Fields: l4bobserve.PointFields(p.Columns),
			X:      p.X, Y: p.Y, Z: p.Z,
			TimeOffsetNanos: p.TimeOffsetNanos,
			Intensity:       p.Intensity,
			Channel:         channel,
			ReturnIndex:     p.ReturnIndex,
			SourceOrdinal:   p.SourceOrdinal,
			PacketSequence:  p.PacketSequence,
			BlockIndex:      block,
		}
	}
	if mm := m.Membership; mm != nil {
		f.Payload |= l4bobserve.PayloadMembership
		f.Unassigned = mm.Unassigned
		if len(mm.UnassignedReasons) > 0 {
			f.UnassignedReasons = make([]l4bobserve.RejectionReason, len(mm.UnassignedReasons))
			for i, r := range mm.UnassignedReasons {
				f.UnassignedReasons[i] = l4bobserve.RejectionReason(r)
			}
		}
		if len(mm.Clusters) > 0 {
			f.Clusters = make([]l4bobserve.ClusterRecord, len(mm.Clusters))
			for i, c := range mm.Clusters {
				if c.Summary == nil {
					return l4bobserve.FrameRecord{}, fmt.Errorf("cluster %d has no summary", c.ClusterId)
				}
				if c.Summary.PointsCount > uint64(limits.MaxPointsPerFrame) {
					return l4bobserve.FrameRecord{}, fmt.Errorf("cluster %d summary counts %d points, over the limit of %d", c.ClusterId, c.Summary.PointsCount, limits.MaxPointsPerFrame)
				}
				f.Clusters[i] = l4bobserve.ClusterRecord{ClusterID: c.ClusterId, Members: c.Members, Summary: summaryFromProto(c.Summary)}
			}
		}
	}
	return f, nil
}

func summaryFromProto(m *pb.ClusterSummary) l4bobserve.ClusterSummary {
	s := l4bobserve.ClusterSummary{
		FirstMemberUnixNanos: m.FirstMemberUnixNanos,
		CentroidX:            m.CentroidX, CentroidY: m.CentroidY, CentroidZ: m.CentroidZ,
		BoundingBoxLength: m.BoundingBoxLength, BoundingBoxWidth: m.BoundingBoxWidth, BoundingBoxHeight: m.BoundingBoxHeight,
		HeightP95: m.HeightP95, IntensityMean: m.IntensityMean,
		// decodeFrame has bounded this by the point limit.
		PointsCount:   int(m.PointsCount),
		GroundClipped: m.GroundClipped,
	}
	if o := m.Obb; o != nil {
		s.OBB = &l4bobserve.OrientedBox{CenterX: o.CenterX, CenterY: o.CenterY, CenterZ: o.CenterZ,
			Length: o.Length, Width: o.Width, Height: o.Height, HeadingRad: o.HeadingRad}
	}
	return s
}

func decodeGap(payload []byte) (l4bobserve.GapRecord, error) {
	var m pb.GapRecord
	if err := unmarshalOptions.Unmarshal(payload, &m); err != nil {
		return l4bobserve.GapRecord{}, fmt.Errorf("decode gap: %w", err)
	}
	if m.Time < 0 || m.Time > pb.GapTime_GAP_TIME_BOUNDED {
		return l4bobserve.GapRecord{}, fmt.Errorf("unknown gap time certainty %d", m.Time)
	}
	g := l4bobserve.GapRecord{
		MissingFrames:  countFromProto(m.MissingFrames),
		Time:           l4bobserve.TimeCertainty(m.Time),
		StartUnixNanos: m.StartUnixNanos,
		EndUnixNanos:   m.EndUnixNanos,
		Cause:          m.Cause,
	}
	if r := m.SequenceRange; r != nil {
		g.HasSequenceRange, g.FirstSequence, g.LastSequence = true, r.First, r.Last
	}
	return g, nil
}

// Field numbers the prescan counts, read from the generated descriptors so
// the scan cannot drift from the schema.
var (
	fFrameStages     = fieldNumber(&pb.FrameRecord{}, "stages")
	fFramePoints     = fieldNumber(&pb.FrameRecord{}, "points")
	fFrameMembership = fieldNumber(&pb.FrameRecord{}, "membership")
	fPointCount      = fieldNumber(&pb.RetainedPoints{}, "point_count")
	fPointColumns    = fieldNumber(&pb.RetainedPoints{}, "columns")
	fMembershipClust = fieldNumber(&pb.Membership{}, "clusters")
	fMembershipUnasg = fieldNumber(&pb.Membership{}, "unassigned")
	fMembershipWhy   = fieldNumber(&pb.Membership{}, "unassigned_reasons")
	fClusterMembers  = fieldNumber(&pb.Cluster{}, "members")

	// pointColumns lists each retained-point column with its presence bit
	// (zero for the coordinates, which are always present).
	pointColumns = []struct {
		number protowire.Number
		bit    uint32
		wire   columnWire
	}{
		{fieldNumber(&pb.RetainedPoints{}, "x"), 0, wireFixed64},
		{fieldNumber(&pb.RetainedPoints{}, "y"), 0, wireFixed64},
		{fieldNumber(&pb.RetainedPoints{}, "z"), 0, wireFixed64},
		{fieldNumber(&pb.RetainedPoints{}, "time_offset_nanos"), uint32(l4bobserve.FieldAcquisitionTime), wireVarint},
		{fieldNumber(&pb.RetainedPoints{}, "intensity"), uint32(l4bobserve.FieldIntensity), wireBytes},
		{fieldNumber(&pb.RetainedPoints{}, "channel"), uint32(l4bobserve.FieldChannel), wireVarint},
		{fieldNumber(&pb.RetainedPoints{}, "return_index"), uint32(l4bobserve.FieldReturnIndex), wireBytes},
		{fieldNumber(&pb.RetainedPoints{}, "source_ordinal"), uint32(l4bobserve.FieldSourceOrdinal), wireVarint},
		{fieldNumber(&pb.RetainedPoints{}, "packet_sequence"), uint32(l4bobserve.FieldPacketSequence), wireVarint},
		{fieldNumber(&pb.RetainedPoints{}, "block_index"), uint32(l4bobserve.FieldBlockIndex), wireVarint},
	}
	knownColumns = func() uint32 {
		var bits uint32
		for _, c := range pointColumns {
			bits |= c.bit
		}
		return bits
	}()
)

func fieldNumber(m proto.Message, name protoreflect.Name) protowire.Number {
	field := m.ProtoReflect().Descriptor().Fields().ByName(name)
	if field == nil {
		panic(fmt.Sprintf("recording schema has no field %s.%s", m.ProtoReflect().Descriptor().FullName(), name))
	}
	return field.Number()
}

// columnWire is how one element of a repeated scalar column is encoded.
type columnWire uint8

const (
	wireFixed64 columnWire = iota // packed double / sfixed64: 8 bytes each
	wireVarint                    // packed varint: one terminating byte each
	wireBytes                     // a bytes field: one byte per element
)

// prescanFrame walks a frame payload's wire format and checks every count
// that decoding would allocate for (stages, point columns, clusters, members
// and unassigned points) against the limits, before protobuf allocates
// anything. Repeated fields and merged sub-messages are summed across
// occurrences, as a decoder would concatenate them.
func prescanFrame(payload []byte, limits Limits) error {
	var s frameScan
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, b []byte) (int, error) {
		switch num {
		case fFrameStages:
			s.stages++
			return consumeMessage(b, typ, nil)
		case fFramePoints:
			s.havePoints = true
			return consumeMessage(b, typ, s.scanPoints)
		case fFrameMembership:
			return consumeMessage(b, typ, s.scanMembership)
		}
		return skipField(num, typ, b)
	})
	if err != nil {
		return fmt.Errorf("frame wire format: %w", err)
	}
	return s.check(limits)
}

type frameScan struct {
	stages     uint64
	havePoints bool
	pointCount uint64
	columns    uint64
	counts     [16]uint64 // by pointColumns index
	clusters   uint64
	members    uint64
	unassigned uint64
	reasons    uint64
}

func (s *frameScan) scanPoints(b []byte) error {
	return walkFields(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, error) {
		switch num {
		case fPointCount, fPointColumns:
			if typ != protowire.VarintType {
				return 0, fmt.Errorf("field %d has wire type %d", num, typ)
			}
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return 0, protowire.ParseError(n)
			}
			// A repeated scalar field keeps its last value when merged.
			if num == fPointCount {
				s.pointCount = v
			} else {
				s.columns = v
			}
			return n, nil
		}
		for i, column := range pointColumns {
			if column.number == num {
				count, n, err := scalarCount(b, typ, column.wire)
				s.counts[i] += count
				return n, err
			}
		}
		return skipField(num, typ, b)
	})
}

func (s *frameScan) scanMembership(b []byte) error {
	return walkFields(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, error) {
		switch num {
		case fMembershipClust:
			s.clusters++
			return consumeMessage(b, typ, func(cluster []byte) error {
				return walkFields(cluster, func(num protowire.Number, typ protowire.Type, b []byte) (int, error) {
					if num == fClusterMembers {
						count, n, err := scalarCount(b, typ, wireVarint)
						s.members += count
						return n, err
					}
					return skipField(num, typ, b)
				})
			})
		case fMembershipUnasg:
			count, n, err := scalarCount(b, typ, wireVarint)
			s.unassigned += count
			return n, err
		case fMembershipWhy:
			count, n, err := scalarCount(b, typ, wireBytes)
			s.reasons += count
			return n, err
		}
		return skipField(num, typ, b)
	})
}

func (s *frameScan) check(limits Limits) error {
	if s.stages > uint64(limits.MaxStagesPerFrame) {
		return fmt.Errorf("%d stage counts exceed the limit of %d", s.stages, limits.MaxStagesPerFrame)
	}
	if s.pointCount > uint64(limits.MaxPointsPerFrame) {
		return fmt.Errorf("%d retained points exceed the limit of %d", s.pointCount, limits.MaxPointsPerFrame)
	}
	if s.columns&^uint64(knownColumns) != 0 {
		return fmt.Errorf("unknown point column bits %#x", s.columns&^uint64(knownColumns))
	}
	for i, column := range pointColumns {
		want := s.pointCount
		if column.bit != 0 && s.columns&uint64(column.bit) == 0 {
			want = 0
		}
		if s.counts[i] != want {
			return fmt.Errorf("point column %d holds %d values for %d points", column.number, s.counts[i], want)
		}
	}
	if s.clusters > uint64(limits.MaxClustersPerFrame) {
		return fmt.Errorf("%d clusters exceed the limit of %d", s.clusters, limits.MaxClustersPerFrame)
	}
	if s.members+s.unassigned > s.pointCount {
		return fmt.Errorf("membership lists %d indices for %d retained points", s.members+s.unassigned, s.pointCount)
	}
	if s.reasons != s.unassigned {
		return fmt.Errorf("%d unassigned points have %d reasons", s.unassigned, s.reasons)
	}
	return nil
}

// walkFields calls visit for each field in a message's wire bytes; visit
// returns how many value bytes it consumed.
func walkFields(b []byte, visit func(protowire.Number, protowire.Type, []byte) (int, error)) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]
		m, err := visit(num, typ, b)
		if err != nil {
			return err
		}
		b = b[m:]
	}
	return nil
}

func skipField(num protowire.Number, typ protowire.Type, b []byte) (int, error) {
	n := protowire.ConsumeFieldValue(num, typ, b)
	if n < 0 {
		return 0, protowire.ParseError(n)
	}
	return n, nil
}

// consumeMessage consumes a length-delimited sub-message, scanning it when
// scan is non-nil. Any other wire type for a message field is malformed.
func consumeMessage(b []byte, typ protowire.Type, scan func([]byte) error) (int, error) {
	if typ != protowire.BytesType {
		return 0, fmt.Errorf("message field has wire type %d", typ)
	}
	v, n := protowire.ConsumeBytes(b)
	if n < 0 {
		return 0, protowire.ParseError(n)
	}
	if scan != nil {
		if err := scan(v); err != nil {
			return 0, err
		}
	}
	return n, nil
}

// scalarCount returns how many elements one occurrence of a repeated scalar
// field holds. Protobuf readers must accept both the packed and the unpacked
// encoding of a repeated scalar, so both are counted.
func scalarCount(b []byte, typ protowire.Type, wire columnWire) (uint64, int, error) {
	if typ == protowire.BytesType {
		v, n := protowire.ConsumeBytes(b)
		if n < 0 {
			return 0, 0, protowire.ParseError(n)
		}
		switch wire {
		case wireBytes:
			return uint64(len(v)), n, nil
		case wireFixed64:
			if len(v)%8 != 0 {
				return 0, 0, fmt.Errorf("packed 8-byte field of %d bytes", len(v))
			}
			return uint64(len(v) / 8), n, nil
		}
		// Every varint ends in exactly one byte below 0x80.
		var count uint64
		for _, c := range v {
			if c < 0x80 {
				count++
			}
		}
		if len(v) > 0 && v[len(v)-1] >= 0x80 {
			return 0, 0, fmt.Errorf("packed varint field ends mid-value")
		}
		return count, n, nil
	}
	switch {
	case wire == wireFixed64 && typ == protowire.Fixed64Type:
		_, n := protowire.ConsumeFixed64(b)
		if n < 0 {
			return 0, 0, protowire.ParseError(n)
		}
		return 1, n, nil
	case wire == wireVarint && typ == protowire.VarintType:
		_, n := protowire.ConsumeVarint(b)
		if n < 0 {
			return 0, 0, protowire.ParseError(n)
		}
		return 1, n, nil
	}
	return 0, 0, fmt.Errorf("repeated field has wire type %d", typ)
}
