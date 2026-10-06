//go:build pcap

package network

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// richPCAPPath carries many packets across many rotations, so pacing,
// progress reporting and seeking all have something to work on. The 20Hz
// sample used elsewhere is a single degenerate frame.
const richPCAPPath = "../../perf/pcap/kirk0.pcapng"

// fastReplay is a config that replays as quickly as the pacer allows.
func fastReplay() RealtimeReplayConfig {
	return RealtimeReplayConfig{
		SpeedMultiplier: 1000,
		DurationSeconds: -1,
	}
}

func TestReadPCAPFileRealtimeDefaultsSpeedMultiplier(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}
	parser := &replayTestParser{motorSpeed: 600}

	// A non-positive multiplier means "unset" and must become 1.0 rather
	// than freezing or dividing by zero.
	cfg := RealtimeReplayConfig{SpeedMultiplier: 0, DurationSeconds: 0.2}

	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		parser, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("ReadPCAPFileRealtime: %v", err)
	}
	if parser.parseCalls == 0 {
		t.Error("parser was never called")
	}
}

func TestReadPCAPFileRealtimeSeeksToPacketOffset(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}
	count, err := CountPCAPPackets(richPCAPPath, 2369)
	if err != nil {
		t.Fatalf("CountPCAPPackets: %v", err)
	}
	if count.Count < 100 {
		t.Fatalf("fixture has only %d packets, too few to seek within", count.Count)
	}

	full := &replayTestParser{motorSpeed: 600}
	cfg := fastReplay()
	cfg.DurationSeconds = 0.5
	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		full, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("full replay: %v", err)
	}

	// Seeking past most of the file must parse strictly fewer packets: the
	// skipped ones are stepped over without being handed to the parser.
	seeked := &replayTestParser{motorSpeed: 600}
	cfg.PacketOffset = count.Count - 10
	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		seeked, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("seeked replay: %v", err)
	}

	if seeked.parseCalls >= full.parseCalls {
		t.Errorf("seeked replay parsed %d packets, want fewer than the full %d",
			seeked.parseCalls, full.parseCalls)
	}
}

func TestReadPCAPFileRealtimeReportsProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}
	count, err := CountPCAPPackets(richPCAPPath, 2369)
	if err != nil {
		t.Fatalf("CountPCAPPackets: %v", err)
	}

	var lastCurrent, lastTotal uint64
	calls := 0
	cfg := fastReplay()
	cfg.DurationSeconds = 0.5
	cfg.TotalPackets = count.Count
	cfg.OnProgress = func(current, total uint64) {
		calls++
		lastCurrent, lastTotal = current, total
	}

	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		&replayTestParser{motorSpeed: 600}, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("ReadPCAPFileRealtime: %v", err)
	}

	if calls == 0 {
		t.Fatal("OnProgress was never called despite TotalPackets being set")
	}
	if lastTotal != count.Count {
		t.Errorf("progress total = %d, want the counted %d", lastTotal, count.Count)
	}
	// The seek bar would jump past the end if this were not bounded.
	if lastCurrent > lastTotal {
		t.Errorf("progress current %d exceeds total %d", lastCurrent, lastTotal)
	}
}

func TestReadPCAPFileRealtimeForwardsPackets(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}
	forwarder, err := NewPacketForwarder("127.0.0.1", 12401, &MockPacketStats{}, time.Minute)
	if err != nil {
		t.Fatalf("NewPacketForwarder: %v", err)
	}
	defer forwarder.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forwarder.Start(ctx)

	cfg := fastReplay()
	cfg.DurationSeconds = 0.3
	cfg.PacketForwarder = forwarder

	if err := ReadPCAPFileRealtime(ctx, richPCAPPath, 2369,
		&replayTestParser{motorSpeed: 600}, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("ReadPCAPFileRealtime: %v", err)
	}
}

func TestReadPCAPFileRealtimeFeedsFrameBuilder(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}
	fb := &MockFrameBuilder{}
	cfg := fastReplay()
	cfg.DurationSeconds = 0.3

	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		&replayTestParser{motorSpeed: 600}, fb, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("ReadPCAPFileRealtime: %v", err)
	}
}

func TestReadPCAPFileRealtimeAppliesDebugRangeFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}
	sensorID := "sensor-debugrange-" + t.Name()
	bg := l3grid.NewBackgroundManager(sensorID, 40, 360, l3grid.BackgroundParams{}, nil)

	fg := &ForegroundForwarder{channel: make(chan []l2frames.PointPolar, 64)}

	cfg := fastReplay()
	cfg.DurationSeconds = 0.3
	cfg.SensorID = sensorID
	cfg.BackgroundManager = bg
	cfg.ForegroundForwarder = fg
	// Narrow debug window: only ring 1 and azimuths around the stub's 90°.
	cfg.DebugRingMin, cfg.DebugRingMax = 1, 2
	cfg.DebugAzMin, cfg.DebugAzMax = 80, 100

	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		&replayTestParser{motorSpeed: 600}, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("ReadPCAPFileRealtime: %v", err)
	}
}

func TestReadPCAPFileRealtimeStopsOnCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}
	// Real-time pacing (multiplier 1) means the fixture takes far longer than
	// the timeout, so cancellation is what ends the replay.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	cfg := RealtimeReplayConfig{SpeedMultiplier: 1, DurationSeconds: -1}

	start := time.Now()
	err := ReadPCAPFileRealtime(ctx, richPCAPPath, 2369,
		&replayTestParser{motorSpeed: 600}, nil, &MockFullPacketStats{}, cfg)
	elapsed := time.Since(start)

	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReadPCAPFileRealtime = %v, want nil or a context error", err)
	}
	if elapsed > 10*time.Second {
		t.Errorf("replay ran for %v after cancellation, want it to stop promptly", elapsed)
	}
}

func TestReadPCAPFileRealtimeSkipsToStartSeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}

	// Without an offset the replay processes from the first packet.
	full := &replayTestParser{motorSpeed: 600}
	cfg := fastReplay()
	cfg.DurationSeconds = 0.5
	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		full, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("full replay: %v", err)
	}

	// StartSeconds skips packets by capture timestamp rather than index, so
	// the parser sees fewer of them for the same duration window.
	skipped := &replayTestParser{motorSpeed: 600}
	cfg.StartSeconds = 2.0
	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		skipped, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("offset replay: %v", err)
	}

	if skipped.parseCalls >= full.parseCalls {
		t.Errorf("replay from %.1fs parsed %d packets, want fewer than the full %d",
			cfg.StartSeconds, skipped.parseCalls, full.parseCalls)
	}
}

// slowFrameBuilder stalls the pipeline so the replay pacer falls behind and
// its dynamic backoff engages.
type slowFrameBuilder struct {
	delay time.Duration
	calls int
}

func (s *slowFrameBuilder) AddPointsPolar([]l2frames.PointPolar) {
	s.calls++
	time.Sleep(s.delay)
}

func (s *slowFrameBuilder) SetMotorSpeed(uint16) {}

func TestReadPCAPFileRealtimeBacksOffWhenPipelineLags(t *testing.T) {
	if testing.Short() {
		t.Skip("must exceed the 3s startup grace period before backoff engages")
	}

	// Real-time pacing plus a deliberately slow frame builder means the
	// pipeline cannot keep up, which is exactly the condition the dynamic
	// backoff exists for. The replay has to run past pcapStartupGracePeriod
	// (3s) before backoff is allowed to engage at all.
	fb := &slowFrameBuilder{delay: 3 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	cfg := RealtimeReplayConfig{SpeedMultiplier: 1, DurationSeconds: -1}

	err := ReadPCAPFileRealtime(ctx, richPCAPPath, 2369,
		&replayTestParser{motorSpeed: 600}, fb, &MockFullPacketStats{}, cfg)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReadPCAPFileRealtime = %v, want nil or a context error", err)
	}

	if fb.calls == 0 {
		t.Error("frame builder was never called")
	}
}

func TestReadPCAPFileRealtimeRejectsMissingFile(t *testing.T) {
	err := ReadPCAPFileRealtime(context.Background(),
		"/nonexistent/capture.pcapng", 2369,
		&replayTestParser{}, nil, &MockFullPacketStats{}, fastReplay())
	if err == nil {
		t.Fatal("ReadPCAPFileRealtime on a missing capture succeeded, want an error")
	}
}

func TestReadPCAPFileRealtimeToleratesParserErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}
	// A packet the parser rejects must not abort the whole replay — captures
	// routinely contain frames from other devices on the same port.
	parser := &replayTestParser{motorSpeed: 600, parseErr: errors.New("bad packet")}
	cfg := fastReplay()
	cfg.DurationSeconds = 0.3

	if err := ReadPCAPFileRealtime(context.Background(), richPCAPPath, 2369,
		parser, nil, &MockFullPacketStats{}, cfg); err != nil {
		t.Fatalf("ReadPCAPFileRealtime: %v", err)
	}
	if parser.parseCalls == 0 {
		t.Error("parser was never called")
	}
}
