package vrlog

import (
	"math"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// roundTrip encodes and decodes one frame through the codec alone.
func roundTrip(t *testing.T, f l4bobserve.FrameRecord, limits Limits) l4bobserve.FrameRecord {
	t.Helper()
	payload, err := marshalOptions.Marshal(frameToProto(f))
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeFrame(payload, limits)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// assertSame requires exact equality two ways: the field-by-field comparison
// and the semantic digest, which must agree for direct and decoded frames.
func assertSame(t *testing.T, want, got l4bobserve.FrameRecord) {
	t.Helper()
	if err := l4bobserve.DiffFrames(want, got); err != nil {
		t.Fatalf("decoded frame differs: %v", err)
	}
	if l4bobserve.FrameDigest(want) != l4bobserve.FrameDigest(got) {
		t.Fatal("semantic digests differ for an identical frame")
	}
}

// G-OBS-FID on synthetic evidence: coincident points, boundary indices,
// -0/subnormal/NaN-payload float bits, empty domains and every disposition
// survive the codec exactly.
func TestFrameCodecRoundTripsExactly(t *testing.T) {
	limits := DefaultLimits()
	frames := append([]l4bobserve.FrameRecord{synthFrame(0, 1), synthFrame(1, 2), synthFrame(2, 257)}, dispositionFrames(3)...)
	for _, f := range frames {
		if err := f.ValidateFor(l4bobserve.ForegroundComplete()); err != nil {
			t.Fatalf("fixture %d is invalid: %v", f.Sequence, err)
		}
		got := roundTrip(t, f, limits)
		assertSame(t, f, got)
		if err := got.ValidateFor(l4bobserve.ForegroundComplete()); err != nil {
			t.Fatalf("decoded frame %d is invalid: %v", f.Sequence, err)
		}
	}
	got := roundTrip(t, synthFrame(9, 5), limits)
	if math.Float64bits(got.Points.X[2]) != 1<<63 || got.Points.Z[2] != math.SmallestNonzeroFloat64 {
		t.Fatal("-0 or a subnormal coordinate lost its bits")
	}
	if math.Float32bits(got.Clusters[0].Summary.IntensityMean) != 0x7fc00123 {
		t.Fatal("a NaN summary lost its payload bits")
	}
}

// Every presence combination the domain can express round-trips: each
// optional point column on or off, points and membership payloads, known and
// unknown counts, an oriented box or none.
func TestFrameCodecPreservesEveryPresenceCombination(t *testing.T) {
	limits := DefaultLimits()
	all := []l4bobserve.PointFields{l4bobserve.FieldAcquisitionTime, l4bobserve.FieldIntensity, l4bobserve.FieldChannel,
		l4bobserve.FieldReturnIndex, l4bobserve.FieldSourceOrdinal, l4bobserve.FieldPacketSequence, l4bobserve.FieldBlockIndex}
	combinations := 0
	for mask := range 1 << len(all) {
		for _, n := range []int{0, 4} {
			f := synthFrame(1, n)
			f.Points.ReturnIndex = make([]uint8, n)
			f.Points.Fields |= l4bobserve.FieldReturnIndex
			for i, field := range all {
				if mask&(1<<i) != 0 {
					continue
				}
				f.Points.Fields &^= field
				switch field {
				case l4bobserve.FieldAcquisitionTime:
					f.Points.TimeOffsetNanos = nil
				case l4bobserve.FieldIntensity:
					f.Points.Intensity = nil
				case l4bobserve.FieldChannel:
					f.Points.Channel = nil
				case l4bobserve.FieldReturnIndex:
					f.Points.ReturnIndex = nil
				case l4bobserve.FieldSourceOrdinal:
					f.Points.SourceOrdinal = nil
				case l4bobserve.FieldPacketSequence:
					f.Points.PacketSequence = nil
				case l4bobserve.FieldBlockIndex:
					f.Points.BlockIndex = nil
				}
			}
			// Alternate the scalar presences with the mask so each appears
			// both ways across the combinations.
			if mask&1 == 0 {
				f.Completeness = l4bobserve.Completeness{State: l4bobserve.CompletenessUnknown}
				f.Stages[0].Output = l4bobserve.Count{}
			}
			if mask&2 == 0 && len(f.Clusters) > 0 {
				f.Clusters[0].Summary.OBB = nil
			}
			if mask&4 == 0 {
				f.Payload = l4bobserve.PayloadPoints
				f.Clusters, f.Unassigned, f.UnassignedReasons = nil, nil, nil
				f.Stages = f.Stages[:2]
				f.Disposition = l4bobserve.Disposition{Kind: l4bobserve.DispositionFailed, Stage: l4bobserve.StageL4Cluster, Reason: "test"}
			}
			if err := f.Validate(); err != nil {
				t.Fatalf("mask %#x n %d: fixture invalid: %v", mask, n, err)
			}
			assertSame(t, f, roundTrip(t, f, limits))
			combinations++
		}
	}
	// No payload at all: a failed frame that carries only its identity.
	failed := dispositionFrames(0)[3]
	assertSame(t, failed, roundTrip(t, failed, limits))
	if combinations != 256 {
		t.Fatalf("covered %d combinations", combinations)
	}
}

func TestGapCodecRoundTripsEveryPresence(t *testing.T) {
	for _, g := range []l4bobserve.GapRecord{
		{Cause: "l2-queue"},
		{Time: l4bobserve.GapTimeBounded, StartUnixNanos: -5, EndUnixNanos: 10, MissingFrames: l4bobserve.KnownCount(0), Cause: "a"},
		{HasSequenceRange: true, FirstSequence: 0, LastSequence: 0, MissingFrames: l4bobserve.KnownCount(1), Cause: "salvage"},
		{HasSequenceRange: true, FirstSequence: 3, LastSequence: 1 << 40, Cause: "salvage"},
	} {
		payload, err := marshalOptions.Marshal(gapToProto(g))
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeGap(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := l4bobserve.DiffGaps(g, got); err != nil || got != g {
			t.Fatalf("gap changed: %+v vs %+v (%v)", g, got, err)
		}
	}
}

// Non-finite coordinates are refused on both sides of the codec: a writer
// never stores one, and a record carrying one fails validation when read.
func TestNonFiniteCoordinatesAreRefused(t *testing.T) {
	for name, value := range map[string]float64{"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1)} {
		f := synthFrame(0, 6)
		f.Points.Y[3] = value
		w, _ := createTest(t, testManifest(t))
		if err := w.AppendFrame(f); err == nil || !strings.Contains(err.Error(), "non-finite") {
			t.Fatalf("%s: writer accepted it: %v", name, err)
		}
		payload, err := marshalOptions.Marshal(frameToProto(f))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeFrame(payload, DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if err := decoded.Validate(); err == nil {
			t.Fatalf("%s: decoded record validated", name)
		}
	}
}

// The prescan refuses every count that would allocate beyond the limits,
// and every malformed repeated field, before protobuf decodes anything.
func TestPrescanBoundsAllocationBeforeDecoding(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxPointsPerFrame, limits.MaxClustersPerFrame, limits.MaxStagesPerFrame = 50, 2, 3
	encode := func(m *pb.FrameRecord) []byte {
		b, err := marshalOptions.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	base := frameToProto(synthFrame(0, 20))
	if _, err := decodeFrame(encode(base), limits); err != nil {
		t.Fatalf("a frame within the limits was refused: %v", err)
	}
	for name, tc := range map[string]struct {
		payload []byte
		want    string
	}{
		"points": {encode(frameToProto(synthFrame(0, 51))), "retained points exceed"},
		"clusters": {func() []byte {
			m := frameToProto(synthFrame(0, 20))
			m.Membership.Clusters = append(m.Membership.Clusters, m.Membership.Clusters[1])
			return encode(m)
		}(), "clusters exceed"},
		"stages": {func() []byte {
			m := frameToProto(synthFrame(0, 3))
			m.Stages = append(m.Stages, m.Stages[0])
			return encode(m)
		}(), "stage counts exceed"},
		"short column":   {func() []byte { m := frameToProto(synthFrame(0, 20)); m.Points.Y = m.Points.Y[:19]; return encode(m) }(), "holds 19 values for 20 points"},
		"absent column":  {func() []byte { m := frameToProto(synthFrame(0, 20)); m.Points.Columns &^= 2; return encode(m) }(), "holds 20 values for 0 points"},
		"unknown column": {func() []byte { m := frameToProto(synthFrame(0, 20)); m.Points.Columns |= 1 << 9; return encode(m) }(), "unknown point column"},
		"member overflow": {func() []byte {
			m := frameToProto(synthFrame(0, 20))
			m.Membership.Unassigned = append(m.Membership.Unassigned, 1, 2)
			m.Membership.UnassignedReasons = append(m.Membership.UnassignedReasons, 0, 0)
			return encode(m)
		}(), "membership lists"},
		"reasons": {func() []byte {
			m := frameToProto(synthFrame(0, 20))
			m.Membership.UnassignedReasons = m.Membership.UnassignedReasons[1:]
			return encode(m)
		}(), "reasons"},
		// A points field encoded as a varint is not a message.
		"wire type":         {protowire.AppendVarint(protowire.AppendTag(nil, fFramePoints, protowire.VarintType), 1), "wire type"},
		"truncated varint":  {append(protowire.AppendTag(nil, fFramePoints, protowire.BytesType), 3, byte(fPointColumns<<3), 0x80, 0x80), "frame wire format"},
		"split packed x":    {append(encode(base), splitPacked(t, fFramePoints, 1)...), "holds"},
		"packed double len": {append(protowire.AppendTag(nil, fFramePoints, protowire.BytesType), 4, 0x1a, 2, 0, 0), "packed 8-byte"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeFrame(tc.payload, limits)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// splitPacked is a second occurrence of the points message holding one more
// x value: protobuf merges it into the first, so the prescan must sum it.
func splitPacked(t *testing.T, field protowire.Number, extra int) []byte {
	t.Helper()
	var x []byte
	for range extra {
		x = protowire.AppendFixed64(x, math.Float64bits(1))
	}
	inner := protowire.AppendBytes(protowire.AppendTag(nil, fieldNumber(&pb.RetainedPoints{}, "x"), protowire.BytesType), x)
	return protowire.AppendBytes(protowire.AppendTag(nil, field, protowire.BytesType), inner)
}

// Protobuf readers must accept repeated scalars unpacked, one field per
// value; the prescan counts them exactly as it counts packed ones.
func TestPrescanCountsUnpackedEncodings(t *testing.T) {
	var points []byte
	points = protowire.AppendVarint(protowire.AppendTag(points, fPointCount, protowire.VarintType), 2)
	for _, name := range []string{"x", "y", "z"} {
		for _, v := range []float64{1.5, -2} {
			points = protowire.AppendTag(points, fieldNumber(&pb.RetainedPoints{}, protoreflectName(name)), protowire.Fixed64Type)
			points = protowire.AppendFixed64(points, math.Float64bits(v))
		}
	}
	var cluster []byte
	for _, m := range []uint64{0, 1} {
		cluster = protowire.AppendVarint(protowire.AppendTag(cluster, fClusterMembers, protowire.VarintType), m)
	}
	cluster = protowire.AppendBytes(protowire.AppendTag(cluster, fieldNumber(&pb.Cluster{}, "summary"), protowire.BytesType), nil)
	membership := protowire.AppendBytes(protowire.AppendTag(nil, fMembershipClust, protowire.BytesType), cluster)
	payload := protowire.AppendBytes(protowire.AppendTag(nil, fFramePoints, protowire.BytesType), points)
	payload = protowire.AppendBytes(protowire.AppendTag(payload, fFrameMembership, protowire.BytesType), membership)

	f, err := decodeFrame(payload, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if f.Points.Len() != 2 || f.Points.Y[1] != -2 || len(f.Clusters) != 1 || len(f.Clusters[0].Members) != 2 {
		t.Fatalf("decoded %+v", f)
	}
	limits := DefaultLimits()
	limits.MaxPointsPerFrame = 1
	if _, err := decodeFrame(payload, limits); err == nil {
		t.Fatal("unpacked values escaped the point limit")
	}
}

func protoreflectName(s string) protoreflect.Name { return protoreflect.Name(s) }

// Unknown enum values are refused rather than mapped onto a default.
func TestDecodeRefusesUnknownEnums(t *testing.T) {
	for name, mutate := range map[string]func(*pb.FrameRecord){
		"completeness": func(m *pb.FrameRecord) { m.Completeness = 9 },
		"background":   func(m *pb.FrameRecord) { m.Background = 9 },
		"disposition":  func(m *pb.FrameRecord) { m.Disposition = 9 },
		"channel":      func(m *pb.FrameRecord) { m.Points.Channel[0] = 1 << 16 },
	} {
		m := frameToProto(synthFrame(0, 4))
		mutate(m)
		payload, err := marshalOptions.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeFrame(payload, DefaultLimits()); err == nil {
			t.Fatalf("%s: an unknown value decoded", name)
		}
	}
	payload, _ := marshalOptions.Marshal(&pb.GapRecord{Time: 5, Cause: "x"})
	if _, err := decodeGap(payload); err == nil {
		t.Fatal("an unknown gap time certainty decoded")
	}
}
