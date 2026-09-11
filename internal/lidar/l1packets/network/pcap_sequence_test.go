package network

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// recordedRead captures one stepReader invocation.
type recordedRead struct {
	path         string
	startSecs    float64
	durationSecs float64
	totalPackets uint64
}

// stubSteps installs a stepReader that records its calls and optionally reports
// progress, then restores the real one when the test ends.
func stubSteps(t *testing.T, reads *[]recordedRead, progressAt []uint64, fail map[string]error) {
	t.Helper()
	original := stepReader
	t.Cleanup(func() { stepReader = original })

	stepReader = func(ctx context.Context, pcapFile string, udpPort int, parser Parser,
		frameBuilder FrameBuilder, stats PacketStatsInterface, forwarder *PacketForwarder,
		startSeconds float64, durationSeconds float64, packetOffset uint64,
		totalPackets uint64, onProgress func(current, total uint64)) error {
		*reads = append(*reads, recordedRead{
			path:         pcapFile,
			startSecs:    startSeconds,
			durationSecs: durationSeconds,
			totalPackets: totalPackets,
		})
		if err, ok := fail[pcapFile]; ok {
			return err
		}
		if onProgress != nil {
			for _, at := range progressAt {
				onProgress(at, totalPackets)
			}
		}
		return nil
	}
}

// seamBuilderStub counts DropNextFrame calls. It satisfies both FrameBuilder
// and SeamAwareFrameBuilder.
type seamBuilderStub struct {
	drops    int
	inFlight bool
}

func (s *seamBuilderStub) AddPointsPolar(_ []l2frames.PointPolar) {}
func (s *seamBuilderStub) SetMotorSpeed(_ uint16)                 {}
func (s *seamBuilderStub) DropNextFrame() bool {
	s.drops++
	return s.inFlight
}

// plainBuilderStub is a frame builder with no seam awareness.
type plainBuilderStub struct{}

func (plainBuilderStub) AddPointsPolar(_ []l2frames.PointPolar) {}
func (plainBuilderStub) SetMotorSpeed(_ uint16)                 {}

func step(path string, start, duration float64, drop bool, packets uint64) capseq.ReadStep {
	return capseq.ReadStep{
		Path:             path,
		StartSecs:        start,
		DurationSecs:     duration,
		DropFrameAtStart: drop,
		PacketCount:      packets,
	}
}

func TestReadPCAPSequenceRejectsEmptyPlan(t *testing.T) {
	_, err := ReadPCAPSequence(context.Background(), nil, SequenceReplayConfig{})
	if err == nil {
		t.Fatal("empty sequence was accepted, want an error")
	}
}

func TestReadPCAPSequenceReadsEveryStepInOrder(t *testing.T) {
	var reads []recordedRead
	stubSteps(t, &reads, nil, nil)

	steps := []capseq.ReadStep{
		step("a.pcap", 120, -1, false, 1000),
		step("b.pcap", 0, -1, false, 1200),
		step("c.pcap", 0, 90, false, 1100),
	}
	res, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
		UDPPort:      2368,
		FrameBuilder: &seamBuilderStub{inFlight: true},
	})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	if res.StepsCompleted != 3 {
		t.Errorf("StepsCompleted = %d, want 3", res.StepsCompleted)
	}
	if len(reads) != 3 {
		t.Fatalf("made %d reads, want 3", len(reads))
	}
	for i, want := range steps {
		if reads[i].path != want.Path {
			t.Errorf("read %d path = %q, want %q", i, reads[i].path, want.Path)
		}
		if reads[i].startSecs != want.StartSecs {
			t.Errorf("read %d startSecs = %v, want %v", i, reads[i].startSecs, want.StartSecs)
		}
		if reads[i].durationSecs != want.DurationSecs {
			t.Errorf("read %d durationSecs = %v, want %v", i, reads[i].durationSecs, want.DurationSecs)
		}
		if reads[i].totalPackets != want.PacketCount {
			t.Errorf("read %d totalPackets = %d, want %d", i, reads[i].totalPackets, want.PacketCount)
		}
	}
}

func TestReadPCAPSequenceDropsAFrameOnlyWhereTheStepAsks(t *testing.T) {
	var reads []recordedRead
	stubSteps(t, &reads, nil, nil)

	fb := &seamBuilderStub{inFlight: true}
	steps := []capseq.ReadStep{
		step("a.pcap", 0, -1, false, 100),
		step("b.pcap", 0, -1, false, 100), // seamless join
		step("c.pcap", 0, -1, true, 100),  // acceptable join
		step("d.pcap", 0, -1, true, 100),  // acceptable join
	}
	res, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{FrameBuilder: fb})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	if fb.drops != 2 {
		t.Errorf("DropNextFrame called %d times, want 2", fb.drops)
	}
	if res.FramesDropped != 2 {
		t.Errorf("FramesDropped = %d, want 2", res.FramesDropped)
	}
}

func TestReadPCAPSequenceCountsOnlyRealDrops(t *testing.T) {
	// A builder reporting no revolution in flight has nothing to discard, so
	// the result must not claim a drop happened.
	var reads []recordedRead
	stubSteps(t, &reads, nil, nil)

	fb := &seamBuilderStub{inFlight: false}
	steps := []capseq.ReadStep{
		step("a.pcap", 0, -1, false, 100),
		step("b.pcap", 0, -1, true, 100),
	}
	res, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{FrameBuilder: fb})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	if fb.drops != 1 {
		t.Errorf("DropNextFrame called %d times, want 1", fb.drops)
	}
	if res.FramesDropped != 0 {
		t.Errorf("FramesDropped = %d, want 0 when nothing was in flight", res.FramesDropped)
	}
}

func TestReadPCAPSequenceToleratesASeamUnawareBuilder(t *testing.T) {
	// Replay must still complete; the operator loses the drop, not the run.
	var reads []recordedRead
	stubSteps(t, &reads, nil, nil)

	steps := []capseq.ReadStep{
		step("a.pcap", 0, -1, false, 100),
		step("b.pcap", 0, -1, true, 100),
	}
	res, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
		FrameBuilder: plainBuilderStub{},
	})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	if res.StepsCompleted != 2 {
		t.Errorf("StepsCompleted = %d, want 2", res.StepsCompleted)
	}
	if res.FramesDropped != 0 {
		t.Errorf("FramesDropped = %d, want 0", res.FramesDropped)
	}
}

func TestReadPCAPSequenceHandlesANilFrameBuilder(t *testing.T) {
	var reads []recordedRead
	stubSteps(t, &reads, nil, nil)

	steps := []capseq.ReadStep{step("a.pcap", 0, -1, true, 100)}
	if _, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{}); err != nil {
		t.Fatalf("ReadPCAPSequence with a nil frame builder: %v", err)
	}
}

func TestReadPCAPSequenceAggregatesProgress(t *testing.T) {
	var reads []recordedRead
	// Each stubbed step reports progress at 50 and 100 packets.
	stubSteps(t, &reads, []uint64{50, 100}, nil)

	var got [][2]uint64
	steps := []capseq.ReadStep{
		step("a.pcap", 0, -1, false, 100),
		step("b.pcap", 0, -1, false, 100),
		step("c.pcap", 0, -1, false, 100),
	}
	_, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
		FrameBuilder: &seamBuilderStub{inFlight: true},
		OnProgress: func(current, total uint64) {
			got = append(got, [2]uint64{current, total})
		},
	})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}

	// Progress must run monotonically across the whole sequence, not restart
	// at each file, and total must be the sum of the steps.
	want := [][2]uint64{{50, 300}, {100, 300}, {150, 300}, {200, 300}, {250, 300}, {300, 300}}
	if len(got) != len(want) {
		t.Fatalf("got %d progress reports %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("progress %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestReadPCAPSequenceReportsUnknownTotalWhenAStepIsUnindexed(t *testing.T) {
	var reads []recordedRead
	stubSteps(t, &reads, []uint64{10}, nil)

	var totals []uint64
	steps := []capseq.ReadStep{
		step("a.pcap", 0, -1, false, 100),
		step("b.pcap", 0, -1, false, 0), // never indexed
	}
	_, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
		FrameBuilder: &seamBuilderStub{inFlight: true},
		OnProgress:   func(_, total uint64) { totals = append(totals, total) },
	})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	for i, total := range totals {
		if total != 0 {
			t.Errorf("progress %d total = %d, want 0 (unknown)", i, total)
		}
	}
}

func TestReadPCAPSequenceKeepsProgressMonotonicWithAnUnindexedStep(t *testing.T) {
	var reads []recordedRead
	stubSteps(t, &reads, []uint64{10}, nil)

	var got [][2]uint64
	steps := []capseq.ReadStep{
		step("a.pcap", 0, -1, false, 100),
		step("b.pcap", 0, -1, false, 0),
		step("c.pcap", 0, -1, false, 100),
	}
	_, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
		FrameBuilder: &seamBuilderStub{inFlight: true},
		OnProgress: func(current, total uint64) {
			got = append(got, [2]uint64{current, total})
		},
	})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	want := [][2]uint64{{10, 0}, {20, 0}, {30, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
}

func TestReadPCAPSequenceWrapsAStepError(t *testing.T) {
	var reads []recordedRead
	sentinel := errors.New("corrupt capture")
	stubSteps(t, &reads, nil, map[string]error{"b.pcap": sentinel})

	steps := []capseq.ReadStep{
		step("a.pcap", 0, -1, false, 100),
		step("b.pcap", 0, -1, false, 100),
		step("c.pcap", 0, -1, false, 100),
	}
	res, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
		FrameBuilder: &seamBuilderStub{inFlight: true},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want it to wrap the step error", err)
	}
	// The message must locate the failure within the sequence.
	for _, want := range []string{"step 2 of 3", "b.pcap"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if res.StepsCompleted != 1 {
		t.Errorf("StepsCompleted = %d, want 1 (a.pcap only)", res.StepsCompleted)
	}
	// Replay must stop at the failure rather than carrying on to c.pcap.
	if len(reads) != 2 {
		t.Errorf("made %d reads, want 2 (stopped at the failure)", len(reads))
	}
}

func TestReadPCAPSequenceHonoursCancellationBetweenSteps(t *testing.T) {
	var reads []recordedRead
	original := stepReader
	t.Cleanup(func() { stepReader = original })

	ctx, cancel := context.WithCancel(context.Background())
	stepReader = func(_ context.Context, pcapFile string, _ int, _ Parser, _ FrameBuilder,
		_ PacketStatsInterface, _ *PacketForwarder, _, _ float64, _, _ uint64,
		_ func(current, total uint64)) error {
		reads = append(reads, recordedRead{path: pcapFile})
		// Cancel while the first file is being read; the sequence must not open
		// the second.
		cancel()
		return nil
	}

	steps := []capseq.ReadStep{
		step("a.pcap", 0, -1, false, 100),
		step("b.pcap", 0, -1, false, 100),
	}
	res, err := ReadPCAPSequence(ctx, steps, SequenceReplayConfig{
		FrameBuilder: &seamBuilderStub{inFlight: true},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(reads) != 1 {
		t.Errorf("made %d reads, want 1 before cancellation took effect", len(reads))
	}
	if res.StepsCompleted != 1 {
		t.Errorf("StepsCompleted = %d, want 1", res.StepsCompleted)
	}
}

func TestReadPCAPSequenceRefusesAPreCancelledContext(t *testing.T) {
	var reads []recordedRead
	stubSteps(t, &reads, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	steps := []capseq.ReadStep{step("a.pcap", 0, -1, false, 100)}
	if _, err := ReadPCAPSequence(ctx, steps, SequenceReplayConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(reads) != 0 {
		t.Errorf("made %d reads with a cancelled context, want 0", len(reads))
	}
}

// TestReadPCAPSequenceFromAPlan wires the two halves together: capseq builds
// and plans the sequence, and the reader executes what it produced.
func TestReadPCAPSequenceFromAPlan(t *testing.T) {
	var reads []recordedRead
	stubSteps(t, &reads, nil, nil)

	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	fiveMin := 5 * time.Minute
	segs := []capseq.Segment{
		{Path: "a.pcap", FirstPacket: base, LastPacket: base.Add(fiveMin), PacketCount: 1000},
		// Seamless: 4ms roll-over.
		{Path: "b.pcap", FirstPacket: base.Add(fiveMin + 4*time.Millisecond),
			LastPacket: base.Add(2*fiveMin + 4*time.Millisecond), PacketCount: 1000},
		// Acceptable: 300ms lost.
		{Path: "c.pcap", FirstPacket: base.Add(2*fiveMin + 304*time.Millisecond),
			LastPacket: base.Add(3*fiveMin + 304*time.Millisecond), PacketCount: 1000},
	}
	seq, err := capseq.Build(segs, capseq.DefaultTolerances())
	if err != nil {
		t.Fatalf("capseq.Build: %v", err)
	}
	steps, err := seq.Plan(0, -1)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	fb := &seamBuilderStub{inFlight: true}
	res, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
		UDPPort:      2368,
		FrameBuilder: fb,
	})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	if res.StepsCompleted != 3 {
		t.Errorf("StepsCompleted = %d, want 3", res.StepsCompleted)
	}
	// Exactly one join was non-seamless, so exactly one revolution is dropped.
	if res.FramesDropped != 1 {
		t.Errorf("FramesDropped = %d, want 1 (only the b→c join was non-seamless)", res.FramesDropped)
	}
	if len(reads) != 3 {
		t.Fatalf("made %d reads, want 3", len(reads))
	}
	for i, want := range []string{"a.pcap", "b.pcap", "c.pcap"} {
		if reads[i].path != want {
			t.Errorf("read %d = %q, want %q", i, reads[i].path, want)
		}
	}
}
