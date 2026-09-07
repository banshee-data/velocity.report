//go:build pcap
// +build pcap

package network

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

// The reference capture and the port it was recorded on.
const (
	referenceCapture = "../../perf/pcap/kirk0.pcapng"
	referencePort    = 2369
)

// captureSlice describes one output file cut from the reference capture:
// the matching packets in [from, to).
type captureSlice struct {
	name string
	from int
	to   int
}

// cutCapture writes the requested slices of the reference capture as separate
// PCAP files and returns their paths in the order given.
//
// A gap between two slices is expressed by leaving packets out rather than by
// relabelling timestamps. That matters: frame assembly runs on the sensor clock
// carried inside the payload, so a capture-timestamp shift alone would leave the
// point stream perfectly continuous and simulate nothing.
func cutCapture(t *testing.T, slices []captureSlice) []string {
	t.Helper()
	if _, err := os.Stat(referenceCapture); err != nil {
		t.Skipf("reference capture unavailable: %v", err)
	}
	handle, err := pcap.OpenOffline(referenceCapture)
	if err != nil {
		t.Skipf("opening reference capture: %v", err)
	}
	defer handle.Close()
	if err := handle.SetBPFFilter("udp port " + itoa(referencePort)); err != nil {
		t.Fatalf("SetBPFFilter: %v", err)
	}

	dir := t.TempDir()
	paths := make([]string, len(slices))
	writers := make([]*pcapgo.Writer, len(slices))
	for i, s := range slices {
		paths[i] = filepath.Join(dir, s.name)
		f, err := os.Create(paths[i])
		if err != nil {
			t.Fatalf("creating %s: %v", paths[i], err)
		}
		defer f.Close()
		w := pcapgo.NewWriterNanos(f)
		if err := w.WriteFileHeader(65536, handle.LinkType()); err != nil {
			t.Fatalf("writing header for %s: %v", paths[i], err)
		}
		writers[i] = w
	}

	// One highest slice bound tells us when to stop reading.
	last := 0
	for _, s := range slices {
		if s.to > last {
			last = s.to
		}
	}

	index := 0
	for packet := range gopacket.NewPacketSource(handle, handle.LinkType()).Packets() {
		if index >= last {
			break
		}
		for i, s := range slices {
			if index < s.from || index >= s.to {
				continue
			}
			if err := writers[i].WritePacket(packet.Metadata().CaptureInfo, packet.Data()); err != nil {
				t.Fatalf("writing packet %d to %s: %v", index, paths[i], err)
			}
		}
		index++
	}
	if index < last {
		t.Skipf("reference capture holds only %d matching packets, need %d", index, last)
	}
	return paths
}

// packetTimes returns the capture timestamps of the first limit matching
// packets, so a test can express a gap in milliseconds rather than guessing a
// packet count.
func packetTimes(t *testing.T, limit int) []time.Time {
	t.Helper()
	handle, err := pcap.OpenOffline(referenceCapture)
	if err != nil {
		t.Skipf("opening reference capture: %v", err)
	}
	defer handle.Close()
	if err := handle.SetBPFFilter("udp port " + itoa(referencePort)); err != nil {
		t.Fatalf("SetBPFFilter: %v", err)
	}
	times := make([]time.Time, 0, limit)
	for packet := range gopacket.NewPacketSource(handle, handle.LinkType()).Packets() {
		if len(times) >= limit {
			break
		}
		times = append(times, packet.Metadata().Timestamp)
	}
	return times
}

// resumeAfterGap returns the first packet index at least gap after the packet
// before cut — the index at which a later file would resume having lost that
// much data.
func resumeAfterGap(t *testing.T, times []time.Time, cut int, gap time.Duration) int {
	t.Helper()
	if cut <= 0 || cut >= len(times) {
		t.Fatalf("cut %d out of range for %d packets", cut, len(times))
	}
	target := times[cut-1].Add(gap)
	for i := cut; i < len(times); i++ {
		if !times[i].Before(target) {
			return i
		}
	}
	t.Skipf("reference capture does not span %v past packet %d", gap, cut)
	return 0
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// replayOutcome is what a replay produced, reduced to the figures a seam can
// change.
type replayOutcome struct {
	frames      int
	points      int
	seamDropped uint64
}

// replay runs fn against a freshly built parser and frame builder and reports
// the frames it produced. Each call gets its own builder so runs cannot leak
// state into one another.
func replay(t *testing.T, fn func(parser Parser, fb *l2frames.FrameBuilder) error) replayOutcome {
	t.Helper()

	parserCfg, err := parse.LoadPandar40PConfig()
	if err != nil {
		t.Fatalf("loading parser config: %v", err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	// Replay must read the sensor's own clock from the payload, as the server
	// does for every PCAP replay. On the default system-time mode every point
	// would be stamped with the moment it was decoded, so a gap in the capture
	// would leave no trace in the frames these tests inspect.
	parser.SetTimestampMode(parse.TimestampModeLiDAR)

	var mu sync.Mutex
	var out replayOutcome
	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID:        "seq-integration",
		FrameChCapacity: 32,
		FrameCallback: func(frame *l2frames.LiDARFrame) {
			if frame == nil {
				return
			}
			mu.Lock()
			out.frames++
			out.points += frame.PointCount
			mu.Unlock()
		},
	})
	// Offline replay must not drop rotations for queue pressure, or the frame
	// counts these tests compare would reflect scheduling, not seams.
	fb.SetBlockOnFrameChannel(true)

	if err := fn(parser, fb); err != nil {
		fb.Close()
		t.Fatalf("replay: %v", err)
	}
	out.seamDropped = fb.StraddlingFramesDropped()
	fb.Close()

	mu.Lock()
	defer mu.Unlock()
	return out
}

// TestSequenceReplayMatchesTheWholeCapture is the load-bearing check: a capture
// cut in two mid-revolution and replayed as a sequence must produce exactly
// what reading the uncut capture produces. If sequencing were not transparent,
// the frame straddling the cut would differ.
func TestSequenceReplayMatchesTheWholeCapture(t *testing.T) {
	const total = 30000
	const cut = 15000

	files := cutCapture(t, []captureSlice{
		{name: "whole.pcap", from: 0, to: total},
		{name: "part-a.pcap", from: 0, to: cut},
		{name: "part-b.pcap", from: cut, to: total},
	})
	whole, partA, partB := files[0], files[1], files[2]

	single := replay(t, func(parser Parser, fb *l2frames.FrameBuilder) error {
		return ReadPCAPFile(context.Background(), whole, referencePort, parser, fb,
			nil, nil, 0, -1, 0, 0, nil)
	})
	if single.frames == 0 {
		t.Fatal("the uncut capture produced no frames; the fixture cannot support this test")
	}

	seq := replay(t, func(parser Parser, fb *l2frames.FrameBuilder) error {
		steps := []capseq.ReadStep{
			{Path: partA, DurationSecs: -1},
			{Path: partB, DurationSecs: -1},
		}
		_, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
			UDPPort:      referencePort,
			Parser:       parser,
			FrameBuilder: fb,
		})
		return err
	})

	if seq.seamDropped != 0 {
		t.Errorf("a seamless cut dropped %d revolution(s), want 0", seq.seamDropped)
	}
	if seq.frames != single.frames {
		t.Errorf("sequence replay produced %d frames, the uncut capture %d: "+
			"splitting the file changed the result", seq.frames, single.frames)
	}
	if seq.points != single.points {
		t.Errorf("sequence replay produced %d points, the uncut capture %d",
			seq.points, single.points)
	}
}

// TestSequenceReplayDropsOneRevolutionPerNonSeamlessJoin checks the cost of a
// join the plan marks: exactly one revolution, and only when marked.
//
// What this cannot check on this fixture is the shape of the discarded
// revolution. The capture's payload clock does not advance across a gap in the
// capture the way its pcap timestamps do, so the frame builder sees a
// continuous point stream either side of 400 ms of missing packets and cannot
// itself tell that anything is wrong. That is exactly why the drop is decided
// from capture-time sequencing at plan time rather than inferred downstream —
// and why the straddling revolution's contents are asserted in the L2 unit
// tests, where it can be constructed deterministically.
func TestSequenceReplayDropsOneRevolutionPerNonSeamlessJoin(t *testing.T) {
	const total = 40000
	const cut = 15000
	const gap = 400 * time.Millisecond

	times := packetTimes(t, total)
	if len(times) < total {
		t.Skipf("reference capture holds only %d matching packets, need %d", len(times), total)
	}
	resume := resumeAfterGap(t, times, cut, gap)

	files := cutCapture(t, []captureSlice{
		{name: "part-a.pcap", from: 0, to: cut},
		{name: "part-b.pcap", from: resume, to: total},
	})
	partA, partB := files[0], files[1]

	run := func(drop bool) replayOutcome {
		return replay(t, func(parser Parser, fb *l2frames.FrameBuilder) error {
			_, err := ReadPCAPSequence(context.Background(), []capseq.ReadStep{
				{Path: partA, DurationSecs: -1},
				{Path: partB, DurationSecs: -1, DropFrameAtStart: drop},
			}, SequenceReplayConfig{UDPPort: referencePort, Parser: parser, FrameBuilder: fb})
			return err
		})
	}

	undropped := run(false)
	dropped := run(true)

	if undropped.frames == 0 {
		t.Fatal("the replay produced no frames; the fixture cannot support this test")
	}
	if undropped.seamDropped != 0 {
		t.Errorf("an unmarked join dropped %d revolution(s), want 0", undropped.seamDropped)
	}
	if dropped.seamDropped != 1 {
		t.Errorf("StraddlingFramesDropped = %d, want 1 for a single marked join",
			dropped.seamDropped)
	}
	if want := undropped.frames - 1; dropped.frames != want {
		t.Errorf("marked replay produced %d frames, want %d — exactly one fewer than the "+
			"unmarked replay's %d", dropped.frames, want, undropped.frames)
	}
	// Only the straddling revolution goes; the rest of the capture is untouched.
	if dropped.points >= undropped.points {
		t.Errorf("marked replay kept %d points, unmarked %d: nothing was discarded",
			dropped.points, undropped.points)
	}
}

// TestSequencePlanFromRealExtents runs the whole path the server takes: probe
// each file, build the sequence from those extents, plan it, and replay the
// plan. The gap must show up as an acceptable join that costs one revolution —
// nobody hand-sets DropFrameAtStart here.
func TestSequencePlanFromRealExtents(t *testing.T) {
	const total = 40000
	const cut = 15000
	const gap = 400 * time.Millisecond

	times := packetTimes(t, total)
	if len(times) < total {
		t.Skipf("reference capture holds only %d matching packets, need %d", len(times), total)
	}
	resume := resumeAfterGap(t, times, cut, gap)

	files := cutCapture(t, []captureSlice{
		{name: "part-a.pcap", from: 0, to: cut},
		{name: "part-b.pcap", from: resume, to: total},
	})

	segments := make([]capseq.Segment, 0, len(files))
	for _, path := range files {
		count, err := CountPCAPPackets(path, referencePort)
		if err != nil {
			t.Fatalf("counting %s: %v", path, err)
		}
		segments = append(segments, capseq.Segment{
			Path:        path,
			FirstPacket: time.Unix(0, count.FirstTimestampNs),
			LastPacket:  time.Unix(0, count.LastTimestampNs),
			PacketCount: count.Count,
		})
	}

	seq, err := capseq.Build(segments, capseq.DefaultTolerances())
	if err != nil {
		t.Fatalf("capseq.Build: %v", err)
	}
	if seq.Worst != capseq.SeamAcceptable {
		t.Fatalf("Worst = %q (join gap %v), want %q", seq.Worst, seq.Seams[0].Gap, capseq.SeamAcceptable)
	}
	steps, err := seq.Plan(0, -1)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 2 || !steps[1].DropFrameAtStart {
		t.Fatalf("plan did not mark the acceptable join for a frame drop: %+v", steps)
	}

	var result SequenceResult
	out := replay(t, func(parser Parser, fb *l2frames.FrameBuilder) error {
		var err error
		result, err = ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{
			UDPPort:      referencePort,
			Parser:       parser,
			FrameBuilder: fb,
		})
		return err
	})

	if result.StepsCompleted != 2 {
		t.Errorf("StepsCompleted = %d, want 2", result.StepsCompleted)
	}
	if result.FramesDropped != 1 {
		t.Errorf("FramesDropped = %d, want 1", result.FramesDropped)
	}
	if out.frames == 0 {
		t.Error("the planned replay produced no frames")
	}
}
