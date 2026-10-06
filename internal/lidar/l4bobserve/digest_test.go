package l4bobserve

import (
	"math"
	"strings"
	"testing"
)

// digestFixture is a small, hand-written record whose digest is pinned
// below. It is independent of the builder, so a builder change cannot move
// the golden value; only a change to the digest's definition can.
func digestFixture() FrameRecord {
	return FrameRecord{
		Sequence:              7,
		SensorFrameID:         "frame-7",
		FrameUnixNanos:        1_700_000_000_000_000_000,
		CaptureStartUnixNanos: 1_700_000_000_000_000_000,
		CaptureEndUnixNanos:   1_700_000_000_100_000_000,
		Completeness:          Completeness{State: CompletenessComplete, LostPackets: KnownCount(0)},
		Background:            BackgroundSettled,
		Disposition:           Disposition{Kind: DispositionObserved},
		Payload:               PayloadPoints | PayloadMembership,
		Stages: []StageCount{
			{Stage: StageL2Frame, Output: KnownCount(10)},
			{Stage: StageL3Foreground, Input: KnownCount(10), Output: KnownCount(3), Rejected: KnownCount(7)},
			{Stage: StageL4Cluster, Input: KnownCount(3), Output: KnownCount(2), Rejected: KnownCount(1)},
		},
		Points: RetainedPoints{
			Fields: FieldAcquisitionTime | FieldIntensity | FieldChannel | FieldSourceOrdinal,
			// Points 0 and 1 are coincident: identical coordinates, distinct returns.
			X:               []float64{1.25, 1.25, math.Copysign(0, -1)},
			Y:               []float64{-2.5, -2.5, 3},
			Z:               []float64{0.5, 0.5, 5e-324},
			TimeOffsetNanos: []int64{0, 10, 99_999_999},
			Intensity:       []uint8{0, 255, 17},
			Channel:         []uint16{0, 1, 40},
			SourceOrdinal:   []uint32{2, 5, 9},
		},
		Clusters: []ClusterRecord{{
			ClusterID: 1,
			Members:   []uint32{0, 1},
			Summary: ClusterSummary{
				FirstMemberUnixNanos: 1_700_000_000_000_000_000,
				CentroidX:            1.25, CentroidY: -2.5, CentroidZ: 0.5,
				BoundingBoxLength: 0.1, IntensityMean: 127.5, PointsCount: 2,
				OBB: &OrientedBox{Length: 0.1, HeadingRad: float32(math.Pi)},
			},
		}},
		Unassigned:        []uint32{2},
		UnassignedReasons: []RejectionReason{RejectionClusterNoise},
	}
}

// goldenFrameDigest pins SemanticDigestVersion v1. It was cross-checked
// against an independent implementation of the rules in the digest.go doc
// comment, not merely recorded from this code. If this test fails, the
// digest's definition changed: that needs a new version, not a new golden.
const goldenFrameDigest = "11b2ff0e7d711297dda3d5ee2853a2ea025e659fcd5b2d8e54aa8e3521918c13"

func TestFrameDigestIsPinned(t *testing.T) {
	f := digestFixture()
	if err := f.Validate(); err != nil {
		t.Fatalf("fixture is not a valid record: %v", err)
	}
	if got := FrameDigest(f).String(); got != goldenFrameDigest {
		t.Fatalf("FrameDigest = %s, want %s: the %s definition changed", got, goldenFrameDigest, SemanticDigestVersion)
	}
}

// Every value the record carries moves the digest, and DiffFrames names the
// same difference; values that are not part of the record's meaning do not.
func TestFrameDigestCoversEveryValue(t *testing.T) {
	base := FrameDigest(digestFixture())
	for name, mutate := range map[string]func(*FrameRecord){
		"sequence":            func(f *FrameRecord) { f.Sequence++ },
		"sensor frame":        func(f *FrameRecord) { f.SensorFrameID = "frame-8" },
		"frame time":          func(f *FrameRecord) { f.FrameUnixNanos++ },
		"capture start":       func(f *FrameRecord) { f.CaptureStartUnixNanos-- },
		"capture end":         func(f *FrameRecord) { f.CaptureEndUnixNanos++ },
		"completeness":        func(f *FrameRecord) { f.Completeness.State = CompletenessPartial },
		"lost packets known":  func(f *FrameRecord) { f.Completeness.LostPackets = Count{} },
		"lost packets value":  func(f *FrameRecord) { f.Completeness.LostPackets = KnownCount(1) },
		"background":          func(f *FrameRecord) { f.Background = BackgroundUnknown },
		"disposition":         func(f *FrameRecord) { f.Disposition.Kind = DispositionUnsettled },
		"disposition reason":  func(f *FrameRecord) { f.Disposition.Reason = "x" },
		"payload":             func(f *FrameRecord) { f.Payload = PayloadPoints },
		"stage order":         func(f *FrameRecord) { f.Stages[0], f.Stages[1] = f.Stages[1], f.Stages[0] },
		"stage count unknown": func(f *FrameRecord) { f.Stages[1].Rejected = Count{} },
		"field presence":      func(f *FrameRecord) { f.Points.Fields |= FieldBlockIndex },
		"negative zero":       func(f *FrameRecord) { f.Points.X[2] = 0 },
		"subnormal":           func(f *FrameRecord) { f.Points.Z[2] = 0 },
		"coincident order":    func(f *FrameRecord) { f.Points.TimeOffsetNanos[0], f.Points.TimeOffsetNanos[1] = 10, 0 },
		"intensity zero":      func(f *FrameRecord) { f.Points.Intensity[0] = 1 },
		"channel zero":        func(f *FrameRecord) { f.Points.Channel[0] = 1 },
		"ordinal":             func(f *FrameRecord) { f.Points.SourceOrdinal[2] = 10 },
		"return index column": func(f *FrameRecord) { f.Points.ReturnIndex = []uint8{0, 0, 0} },
		"cluster id":          func(f *FrameRecord) { f.Clusters[0].ClusterID = 2 },
		"members":             func(f *FrameRecord) { f.Clusters[0].Members = []uint32{0, 2} },
		"summary float bits":  func(f *FrameRecord) { f.Clusters[0].Summary.IntensityMean = math.Nextafter32(127.5, 128) },
		"summary nan payload": func(f *FrameRecord) { f.Clusters[0].Summary.HeightP95 = math.Float32frombits(0x7fc00001) },
		"ground clipped":      func(f *FrameRecord) { f.Clusters[0].Summary.GroundClipped = true },
		"obb absent":          func(f *FrameRecord) { f.Clusters[0].Summary.OBB = nil },
		"obb heading":         func(f *FrameRecord) { f.Clusters[0].Summary.OBB.HeadingRad = 0 },
		"unassigned reason":   func(f *FrameRecord) { f.UnassignedReasons[0] = RejectionClusterShape },
	} {
		t.Run(name, func(t *testing.T) {
			f := digestFixture()
			mutate(&f)
			if FrameDigest(f) == base {
				t.Fatal("digest did not move")
			}
			if err := DiffFrames(digestFixture(), f); err == nil {
				t.Fatal("DiffFrames found no difference")
			}
		})
	}

	for name, mutate := range map[string]func(*FrameRecord){
		"stray unknown value": func(f *FrameRecord) { f.Stages[0].Input = Count{Value: 9} },
		"empty not nil":       func(f *FrameRecord) { f.Points.PacketSequence = []uint32{} },
	} {
		t.Run("neutral "+name, func(t *testing.T) {
			f := digestFixture()
			mutate(&f)
			if FrameDigest(f) != base {
				t.Fatal("digest moved for a value that carries no meaning")
			}
			if err := DiffFrames(digestFixture(), f); err != nil {
				t.Fatalf("DiffFrames: %v", err)
			}
		})
	}
}

func TestGapAndStreamDigests(t *testing.T) {
	gap := GapRecord{HasSequenceRange: true, FirstSequence: 3, LastSequence: 4, MissingFrames: KnownCount(2),
		Time: GapTimeBounded, StartUnixNanos: 10, EndUnixNanos: 20, Cause: "salvage"}
	base := GapDigest(gap)
	for name, mutate := range map[string]func(*GapRecord){
		"range":   func(g *GapRecord) { g.LastSequence = 5 },
		"missing": func(g *GapRecord) { g.MissingFrames = Count{} },
		"time":    func(g *GapRecord) { g.Time, g.StartUnixNanos, g.EndUnixNanos = GapTimeUnknown, 0, 0 },
		"start":   func(g *GapRecord) { g.StartUnixNanos = 11 },
		"cause":   func(g *GapRecord) { g.Cause = "l2-queue" },
	} {
		g := gap
		mutate(&g)
		if GapDigest(g) == base || DiffGaps(gap, g) == nil {
			t.Fatalf("%s: gap digest did not move", name)
		}
	}

	frame := FrameDigest(digestFixture())
	ordered, reordered, relabelled := NewStreamDigest(), NewStreamDigest(), NewStreamDigest()
	ordered.AddFrame(frame)
	ordered.AddGap(base)
	reordered.AddGap(base)
	reordered.AddFrame(frame)
	relabelled.AddFrame(frame)
	relabelled.AddFrame(base)
	if ordered.Sum() == reordered.Sum() || ordered.Sum() == relabelled.Sum() {
		t.Fatal("stream digest ignores record order or kind")
	}
	if ordered.Sum() != ordered.Sum() {
		t.Fatal("Sum changed the stream state")
	}
	if !strings.HasPrefix(SemanticDigestVersion, "l4bobserve.semantic/") {
		t.Fatalf("unexpected digest version %q", SemanticDigestVersion)
	}
}
