package l4bobserve

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func testExtraction() Extraction {
	return Extraction{SourceID: "source/v1/test", CalibrationID: "calibration/v1/test", SensorID: "lidar", CoordinateFrame: "site/lidar"}
}

func newTestBuilder(t *testing.T) *ExtractionBuilder {
	t.Helper()
	b, err := NewExtractionBuilder(testExtraction())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// syntheticReturn is one L2 return and whether L3 calls it foreground.
type syntheticReturn struct {
	az, el, dist float64
	foreground   bool
	role         string
}

// syntheticFrame lays out, in source order: background returns, two tight
// clusters, a tiny cluster the diameter filter rejects, an isolated noise
// return, and ground and overhead returns outside the default height band.
func syntheticFrame(start time.Time, firstPacket uint32) (*l2frames.LiDARFrame, []syntheticReturn) {
	var returns []syntheticReturn
	add := func(n int, az, step, el, dist float64, foreground bool, role string) {
		for i := 0; i < n; i++ {
			returns = append(returns, syntheticReturn{az: az + float64(i)*step, el: el, dist: dist, foreground: foreground, role: role})
		}
	}
	add(3, 200, 1, 0, 30, false, "background")
	add(12, 10, 0.1, 0, 10, true, "cluster-a")
	add(2, 250, 1, 0, 30, false, "background")
	add(12, 40, 0.1, 0, 10, true, "cluster-b")
	add(4, 60, 0.00001, 0, 10, true, "tiny")
	add(1, 80, 0, 0, 10, true, "noise")
	add(1, 120, 0, -20, 10, true, "ground")
	add(1, 150, 0, 15, 10, true, "overhead")
	polar := make([]l2frames.PointPolar, len(returns))
	frame := &l2frames.LiDARFrame{FrameID: "lidar-frame-1", SensorID: "lidar", StartTimestamp: start,
		SpinComplete: true, ReceivedPackets: map[uint32]bool{}}
	for i, r := range returns {
		packet := firstPacket + uint32(i/10)
		polar[i] = l2frames.PointPolar{Channel: 1 + i%40, Azimuth: r.az, Elevation: r.el, Distance: r.dist,
			Intensity: uint8(i), Timestamp: start.UnixNano() + int64(i)*1000, BlockID: i % 10, UDPSequence: packet}
		frame.ReceivedPackets[packet] = true
	}
	frame.PolarPoints = polar
	frame.EndTimestamp = time.Unix(0, polar[len(polar)-1].Timestamp)
	return frame, returns
}

type l4Params struct {
	voxelLeaf float64
	maxInput  int
}

// runL4 drives a draft exactly as the pipeline does, through the production
// L3 extraction, L4 transform, filters and DBSCAN, and returns the world
// points as they were before the in-place height filter.
func runL4(t *testing.T, d *FrameDraft, frame *l2frames.LiDARFrame, returns []syntheticReturn, p l4Params) []l4perception.WorldPoint {
	t.Helper()
	mask := make([]bool, len(returns))
	var foreground []l2frames.PointPolar
	for i, r := range returns {
		mask[i] = r.foreground
		if r.foreground {
			foreground = append(foreground, frame.PolarPoints[i])
		}
	}
	d.SetBackground(BackgroundSettled)
	d.SetForeground(mask, len(foreground))
	world := l4perception.TransformToWorld(foreground, nil, "lidar")
	d.Retain(world)
	before := slices.Clone(world)
	filtered := l4perception.DefaultHeightBandFilter().FilterVertical(world)
	d.HeightBandFiltered(filtered)
	if p.voxelLeaf > 0 {
		filtered = l4perception.VoxelGrid(filtered, p.voxelLeaf)
		d.Voxelled(filtered)
	}
	clusters, trace := l4perception.DBSCANWithTrace(filtered, l4perception.DBSCANParams{
		Eps: 0.3, MinPts: 3, MaxInputPoints: p.maxInput,
		MaxClusterDiameter: 10, MinClusterDiameter: 0.05, MaxClusterAspectRatio: 1000,
	})
	d.Clustered(trace, clusters)
	return before
}

func TestBuilderRecordsCompleteForegroundAndExactPartition(t *testing.T) {
	b := newTestBuilder(t)
	frame, returns := syntheticFrame(time.Unix(1_700_000_000, 0), 500)
	d := b.BeginFrame(frame)
	world := runL4(t, d, frame, returns, l4Params{})
	rec, err := d.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := rec.ValidateFor(ForegroundComplete()); err != nil {
		t.Fatal(err)
	}
	if rec.Sequence != 0 || rec.Disposition.Kind != DispositionObserved || !rec.Payload.Has(PayloadPoints|PayloadMembership) {
		t.Fatalf("record header = seq %d, %+v, payload %b", rec.Sequence, rec.Disposition, rec.Payload)
	}
	var wantOrdinals []uint32
	for i, r := range returns {
		if r.foreground {
			wantOrdinals = append(wantOrdinals, uint32(i))
		}
	}
	if !slices.Equal(rec.Points.SourceOrdinal, wantOrdinals) {
		t.Fatalf("ordinals = %v, want %v", rec.Points.SourceOrdinal, wantOrdinals)
	}
	for k, p := range world {
		// Bit-exact: the domain is L4's own float64 geometry, before filtering.
		if math.Float64bits(rec.Points.X[k]) != math.Float64bits(p.X) || math.Float64bits(rec.Points.Z[k]) != math.Float64bits(p.Z) {
			t.Fatalf("point %d geometry changed: %v vs %+v", k, rec.Points.X[k], p)
		}
		ordinal := rec.Points.SourceOrdinal[k]
		if rec.Points.TimeOffsetNanos[k] != int64(ordinal)*1000 || rec.Points.Intensity[k] != uint8(ordinal) ||
			rec.Points.Channel[k] != uint16(1+ordinal%40) || rec.Points.PacketSequence[k] != 500+ordinal/10 {
			t.Fatalf("point %d acquisition fields wrong", k)
		}
	}
	if rec.Points.Fields.Has(FieldReturnIndex) || rec.Points.ReturnIndex != nil {
		t.Fatal("return index claimed although L1 does not decode it")
	}
	role := func(index uint32) string { return returns[rec.Points.SourceOrdinal[index]].role }
	if len(rec.Clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(rec.Clusters))
	}
	for _, c := range rec.Clusters {
		first := role(c.Members[0])
		for _, member := range c.Members {
			if role(member) != first || !strings.HasPrefix(first, "cluster-") {
				t.Fatalf("cluster %d mixes %s and %s", c.ClusterID, first, role(member))
			}
		}
		if len(c.Members) != 12 || c.Summary.PointsCount != 12 || c.Summary.OBB == nil {
			t.Fatalf("cluster %d = %d members, summary %+v", c.ClusterID, len(c.Members), c.Summary)
		}
	}
	wantReason := map[string]RejectionReason{"tiny": RejectionClusterShape, "noise": RejectionClusterNoise,
		"ground": RejectionHeightBand, "overhead": RejectionHeightBand}
	if len(rec.Unassigned) != 7 {
		t.Fatalf("unassigned = %d, want 7", len(rec.Unassigned))
	}
	for i, index := range rec.Unassigned {
		if want := wantReason[role(index)]; rec.UnassignedReasons[i] != want {
			t.Fatalf("%s return rejected as %s, want %s", role(index), rec.UnassignedReasons[i], want)
		}
	}
	for stage, want := range map[Stage][3]Count{
		StageL2Frame:           {{}, KnownCount(36), {}},
		StageL3Foreground:      {KnownCount(36), KnownCount(31), KnownCount(5)},
		StageL4HeightFilter:    {KnownCount(31), KnownCount(29), KnownCount(2)},
		StageL4ClusterInputCap: {KnownCount(29), KnownCount(29), KnownCount(0)},
		StageL4Cluster:         {KnownCount(29), KnownCount(24), KnownCount(5)},
	} {
		got, ok := rec.Stage(stage)
		if !ok || got.Input != want[0] || got.Output != want[1] || got.Rejected != want[2] {
			t.Fatalf("stage %s = %+v, want %v", stage, got, want)
		}
	}
	if _, ok := rec.Stage(StageL4Voxel); ok {
		t.Fatal("recorded a voxel stage that did not run")
	}
	// The first frame of an extraction cannot verify its leading packet boundary.
	if rec.Completeness.State != CompletenessUnknown {
		t.Fatalf("first frame completeness = %+v", rec.Completeness)
	}
}

func TestBuilderTracesVoxelAndInputCapReductions(t *testing.T) {
	b := newTestBuilder(t)
	frame, returns := syntheticFrame(time.Unix(1_700_000_000, 0), 1)
	d := b.BeginFrame(frame)
	runL4(t, d, frame, returns, l4Params{voxelLeaf: 0.05, maxInput: 6})
	rec, err := d.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	voxel, _ := rec.Stage(StageL4Voxel)
	capStage, _ := rec.Stage(StageL4ClusterInputCap)
	if voxel.Rejected.Value == 0 || capStage.Output.Value != 6 || capStage.Rejected.Value == 0 {
		t.Fatalf("voxel %+v cap %+v: the fixture should exercise both reductions", voxel, capStage)
	}
	counts := map[RejectionReason]uint64{}
	for _, reason := range rec.UnassignedReasons {
		counts[reason]++
	}
	if counts[RejectionVoxelReduced] != voxel.Rejected.Value || counts[RejectionClusterInputCap] != capStage.Rejected.Value {
		t.Fatalf("reasons %v disagree with stage counts voxel=%s cap=%s", counts, voxel.Rejected, capStage.Rejected)
	}
}

func TestBuilderAssignsDenseSequencesAndRecordsEveryDisposition(t *testing.T) {
	b := newTestBuilder(t)
	stream := NewStreamValidator(ForegroundComplete())
	start := time.Unix(1_700_000_000, 0)
	finish := func(d *FrameDraft) FrameRecord {
		t.Helper()
		rec, err := d.Finish()
		if err != nil {
			t.Fatalf("finish: %v", err)
		}
		if err := stream.AddFrame(rec); err != nil {
			t.Fatalf("stream: %v", err)
		}
		return rec
	}

	// An L2 frame with no returns is an observed empty frame, not a drop.
	empty := finish(b.BeginFrame(&l2frames.LiDARFrame{FrameID: "empty", StartTimestamp: start, EndTimestamp: start}))
	if empty.Disposition.Kind != DispositionObserved || !empty.Payload.Has(PayloadPoints|PayloadMembership) || empty.Points.Len() != 0 {
		t.Fatalf("empty frame = %+v", empty)
	}

	frame, returns := syntheticFrame(start.Add(100*time.Millisecond), 10)
	d := b.BeginFrame(frame)
	d.SetBackground(BackgroundSettling)
	d.SetForeground(make([]bool, len(returns)), 0)
	quiet := finish(d)
	if quiet.Disposition.Kind != DispositionUnsettled || !quiet.Payload.Has(PayloadMembership) {
		t.Fatalf("unsettled empty foreground = %+v", quiet.Disposition)
	}

	frame, returns = syntheticFrame(start.Add(200*time.Millisecond), 14)
	d = b.BeginFrame(frame)
	d.SetBackground(BackgroundSettled)
	d.SetForeground(make([]bool, len(returns)), 31)
	d.Suppress(StageL4Transform, "replay frame-rate throttle")
	suppressed := finish(d)
	if suppressed.Disposition.Kind != DispositionSuppressed || suppressed.Payload != 0 {
		t.Fatalf("suppressed = %+v payload %b", suppressed.Disposition, suppressed.Payload)
	}

	frame, _ = syntheticFrame(start.Add(300*time.Millisecond), 18)
	d = b.BeginFrame(frame)
	d.Fail(StageL3Foreground, "foreground mask unavailable")
	failed := finish(d)
	if failed.Disposition != (Disposition{Kind: DispositionFailed, Stage: StageL3Foreground, Reason: "foreground mask unavailable"}) || failed.Payload != 0 {
		t.Fatalf("failed = %+v", failed)
	}
	if got := []uint64{empty.Sequence, quiet.Sequence, suppressed.Sequence, failed.Sequence}; !slices.Equal(got, []uint64{0, 1, 2, 3}) {
		t.Fatalf("sequences = %v", got)
	}
}

func TestBuilderCompletenessFollowsPacketContinuity(t *testing.T) {
	b := newTestBuilder(t)
	start := time.Unix(1_700_000_000, 0)
	next := func(first uint32, gaps int, spin bool) Completeness {
		t.Helper()
		frame, returns := syntheticFrame(start, first)
		frame.PacketGaps, frame.SpinComplete = gaps, spin
		d := b.BeginFrame(frame)
		d.SetForeground(make([]bool, len(returns)), 0)
		rec, err := d.Finish()
		if err != nil {
			t.Fatal(err)
		}
		start = start.Add(100 * time.Millisecond)
		return rec.Completeness
	}
	// The synthetic frame spans packets first..first+3; 107 is lost below.
	if c := next(100, 0, true); c.State != CompletenessUnknown {
		t.Fatalf("first frame = %+v", c)
	}
	// A seam packet shared with the previous frame is continuity.
	if c := next(103, 0, true); c.State != CompletenessComplete {
		t.Fatalf("contiguous frame = %+v", c)
	}
	if c := next(108, 0, true); c != (Completeness{State: CompletenessPartial, LostPackets: KnownCount(1)}) {
		t.Fatalf("one packet lost at the boundary = %+v", c)
	}
	if c := next(112, 2, true); c != (Completeness{State: CompletenessPartial, LostPackets: KnownCount(2)}) {
		t.Fatalf("two lost inside the frame = %+v", c)
	}
	if c := next(116, 0, false); c.State != CompletenessPartial || c.LostPackets != KnownCount(0) {
		t.Fatalf("short rotation = %+v", c)
	}
	if err := b.RecordGap(GapRecord{MissingFrames: KnownCount(2), Cause: "l2 queue overflow"}); err != nil {
		t.Fatal(err)
	}
	if c := next(140, 0, true); c.State != CompletenessUnknown {
		t.Fatalf("frame after a gap claimed %+v", c)
	}
	unnumbered, returns := syntheticFrame(start, 0)
	unnumbered.ReceivedPackets = map[uint32]bool{0: true}
	d := b.BeginFrame(unnumbered)
	d.SetForeground(make([]bool, len(returns)), 0)
	rec, err := d.Finish()
	if err != nil || rec.Completeness.State != CompletenessUnknown || rec.Points.Fields.Has(FieldPacketSequence) {
		t.Fatalf("unnumbered packets = %+v, %v", rec.Completeness, err)
	}
}

func TestBuilderTurnsBrokenLineageIntoAFailedRecord(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	for name, breakIt := range map[string]func(d *FrameDraft, world []l4perception.WorldPoint){
		"unstamped survivor": func(d *FrameDraft, world []l4perception.WorldPoint) {
			d.HeightBandFiltered([]l4perception.WorldPoint{{X: 1}})
		},
		"duplicated survivor": func(d *FrameDraft, world []l4perception.WorldPoint) {
			d.HeightBandFiltered([]l4perception.WorldPoint{world[0], world[0]})
		},
		"ended early": func(d *FrameDraft, world []l4perception.WorldPoint) {},
		"summary disagrees": func(d *FrameDraft, world []l4perception.WorldPoint) {
			clusters, trace := l4perception.DBSCANWithTrace(world, l4perception.DBSCANParams{Eps: 0.3, MinPts: 3, MaxClusterDiameter: 10, MaxClusterAspectRatio: 1000})
			clusters[0].PointsCount++
			d.Clustered(trace, clusters)
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := newTestBuilder(t)
			frame, returns := syntheticFrame(start, 1)
			d := b.BeginFrame(frame)
			mask := make([]bool, len(returns))
			var foreground []l2frames.PointPolar
			for i, r := range returns {
				if r.foreground && r.role != "ground" && r.role != "overhead" {
					mask[i] = true
					foreground = append(foreground, frame.PolarPoints[i])
				}
			}
			d.SetForeground(mask, len(foreground))
			world := l4perception.TransformToWorld(foreground, nil, "lidar")
			d.Retain(world)
			breakIt(d, world)
			rec, err := d.Finish()
			if err == nil {
				t.Fatal("broken lineage finished without an error")
			}
			if rec.Disposition.Kind != DispositionFailed || rec.Payload.Has(PayloadMembership) || !rec.Payload.Has(PayloadPoints) {
				t.Fatalf("record = %+v payload %b: want a failed frame keeping its retained domain", rec.Disposition, rec.Payload)
			}
			if err := rec.ValidateFor(ForegroundComplete()); err != nil {
				t.Fatalf("failed record is itself invalid: %v", err)
			}
		})
	}
}

// A frame whose own header is inconsistent cannot yield a valid record of its
// contents. It is still recorded, as a failed frame carrying only what cannot
// be wrong, so the sequence stays dense and the fault is named.
func TestBuilderReducesAContractViolationToAFailedFrame(t *testing.T) {
	b := newTestBuilder(t)
	frame, returns := syntheticFrame(time.Unix(1_700_000_000, 0), 1)
	frame.EndTimestamp = frame.StartTimestamp.Add(-time.Millisecond)
	d := b.BeginFrame(frame)
	runL4(t, d, frame, returns, l4Params{})
	rec, err := d.Finish()
	if err == nil || !strings.Contains(err.Error(), "capture starts after it ends") {
		t.Fatalf("finish error = %v", err)
	}
	if rec.Sequence != 0 || rec.Disposition.Kind != DispositionFailed || rec.Payload != 0 || len(rec.Stages) != 1 ||
		!strings.Contains(rec.Disposition.Reason, "evidence contract") {
		t.Fatalf("record = %+v", rec)
	}
	if err := rec.ValidateFor(ForegroundComplete()); err != nil {
		t.Fatalf("reduced record is invalid: %v", err)
	}
	if b.NextSequence() != 1 {
		t.Fatalf("next sequence = %d", b.NextSequence())
	}
}

func TestBuilderRefusesMissingIdentityAndOtherProfiles(t *testing.T) {
	for _, change := range []func(*Extraction){
		func(e *Extraction) { e.SourceID = "" },
		func(e *Extraction) { e.CalibrationID = " " },
		func(e *Extraction) { e.SensorID = "" },
		func(e *Extraction) { e.CoordinateFrame = "" },
		func(e *Extraction) { e.Profile = ReducedClusterSample() },
		func(e *Extraction) {
			e.Profile = Profile{Name: ProfileForegroundComplete, Capabilities: NewCapabilitySet(CapabilityFrameRecords)}
		},
	} {
		e := testExtraction()
		change(&e)
		if _, err := NewExtractionBuilder(e); err == nil {
			t.Fatalf("accepted extraction %+v", e)
		}
	}
	b := newTestBuilder(t)
	if b.Extraction().Profile.Name != ProfileForegroundComplete {
		t.Fatalf("profile = %q", b.Extraction().Profile.Name)
	}
	var nilDraft *FrameDraft
	nilDraft.SetForeground(nil, 0)
	nilDraft.Retain(nil)
	nilDraft.Clustered(l4perception.DBSCANTrace{}, nil)
	if _, err := nilDraft.Finish(); !errors.Is(err, errNilDraft) || !nilDraft.Finished() {
		t.Fatalf("nil draft finish = %v", err)
	}
	if (*ExtractionBuilder)(nil).BeginFrame(&l2frames.LiDARFrame{}) != nil || b.BeginFrame(nil) != nil {
		t.Fatal("a nil builder or frame opened a draft")
	}
	d := b.BeginFrame(&l2frames.LiDARFrame{})
	if _, err := d.Finish(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Finish(); !errors.Is(err, errDraftFinished) {
		t.Fatalf("second finish = %v", err)
	}
}

func TestRecordGapPlacement(t *testing.T) {
	b := newTestBuilder(t)
	if err := b.RecordGap(GapRecord{HasSequenceRange: true, FirstSequence: 0, LastSequence: 2, Cause: "salvage"}); err != nil {
		t.Fatal(err)
	}
	if b.NextSequence() != 3 {
		t.Fatalf("next = %d, want 3", b.NextSequence())
	}
	if err := b.RecordGap(GapRecord{HasSequenceRange: true, FirstSequence: 5, LastSequence: 6, Cause: "salvage"}); err == nil {
		t.Fatal("accepted a gap that leaves sequences 3-4 uncovered")
	}
	if err := b.RecordGap(GapRecord{}); err == nil {
		t.Fatal("accepted a gap without a cause")
	}
}
