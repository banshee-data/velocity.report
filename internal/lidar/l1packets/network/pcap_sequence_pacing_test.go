package network

import (
	"context"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
)

func TestPacingAnchorFixesTheOriginOnce(t *testing.T) {
	var a PacingAnchor
	if a.Anchored() {
		t.Fatal("a fresh anchor must not claim an origin")
	}
	capture := time.Date(2026, 9, 2, 10, 58, 30, 0, time.UTC)
	wall := time.Now()
	a.Anchor(capture, wall)
	if !a.Anchored() {
		t.Fatal("Anchor did not fix the origin")
	}
	// A later step must not move the origin, or the join it sits behind would
	// reset the pacer — the thing the anchor exists to prevent.
	a.Anchor(capture.Add(5*time.Minute), wall.Add(5*time.Minute))
	if !a.CaptureStart.Equal(capture) || !a.WallStart.Equal(wall) {
		t.Errorf("a second Anchor moved the origin to %v/%v", a.CaptureStart, a.WallStart)
	}
}

func TestPacingAnchorTolerariesNil(t *testing.T) {
	var a *PacingAnchor // a single-file replay passes no anchor
	if a.Anchored() {
		t.Fatal("a nil anchor must report unanchored")
	}
	a.Anchor(time.Now(), time.Now()) // must not panic
	a.AddYield(time.Second)          // must not panic
}

func TestPacingAnchorAccumulatesYieldAcrossSteps(t *testing.T) {
	var a PacingAnchor
	a.AddYield(30 * time.Millisecond)
	a.AddYield(70 * time.Millisecond)
	if a.CumulativeYield != 100*time.Millisecond {
		t.Errorf("CumulativeYield = %v, want 100ms: a later step would read earlier "+
			"backoff as lateness it had not earned", a.CumulativeYield)
	}
}

// TestSequenceUsesThePacedReaderWhenAskedTo pins the routing: a sequence with a
// speed multiplier must go through the paced reader, and every step must share
// one anchor so the pacing clock survives the joins.
func TestSequenceUsesThePacedReaderWhenAskedTo(t *testing.T) {
	steps := []capseq.ReadStep{
		{Path: "a.pcap", PacketCount: 10},
		{Path: "b.pcap", PacketCount: 10, DropFrameAtStart: true},
		{Path: "c.pcap", PacketCount: 10},
	}

	var anchors []*PacingAnchor
	var speeds []float64
	restore := stepReaderRealtime
	stepReaderRealtime = func(_ context.Context, _ string, _ int, _ Parser, _ FrameBuilder,
		_ PacketStatsInterface, cfg RealtimeReplayConfig) error {
		anchors = append(anchors, cfg.PacingAnchor)
		speeds = append(speeds, cfg.SpeedMultiplier)
		return nil
	}
	defer func() { stepReaderRealtime = restore }()

	unpacedCalls := 0
	restoreUnpaced := stepReader
	stepReader = func(context.Context, string, int, Parser, FrameBuilder, PacketStatsInterface,
		*PacketForwarder, float64, float64, uint64, uint64, func(uint64, uint64)) error {
		unpacedCalls++
		return nil
	}
	defer func() { stepReader = restoreUnpaced }()

	res, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{Paced: RealtimeReplayConfig{SpeedMultiplier: 0.5}})
	if err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	if res.StepsCompleted != 3 {
		t.Fatalf("StepsCompleted = %d, want 3", res.StepsCompleted)
	}
	if unpacedCalls != 0 {
		t.Errorf("%d step(s) went to the unpaced reader; a paced sequence must not", unpacedCalls)
	}
	if len(anchors) != 3 {
		t.Fatalf("paced reader saw %d steps, want 3", len(anchors))
	}
	for i, a := range anchors {
		if a == nil {
			t.Fatalf("step %d got no anchor, so it would re-anchor at the join", i)
		}
		if a != anchors[0] {
			t.Errorf("step %d has its own anchor; every step must share the sequence's", i)
		}
		if speeds[i] != 0.5 {
			t.Errorf("step %d replayed at %v, want 0.5", i, speeds[i])
		}
	}
}

// TestSequenceStaysUnpacedForAnalysis is the other half: an analysis run must
// keep reading as fast as the pipeline accepts packets.
func TestSequenceStaysUnpacedForAnalysis(t *testing.T) {
	steps := []capseq.ReadStep{{Path: "a.pcap", PacketCount: 4}, {Path: "b.pcap", PacketCount: 4}}

	pacedCalls := 0
	restorePaced := stepReaderRealtime
	stepReaderRealtime = func(context.Context, string, int, Parser, FrameBuilder,
		PacketStatsInterface, RealtimeReplayConfig) error {
		pacedCalls++
		return nil
	}
	defer func() { stepReaderRealtime = restorePaced }()

	unpaced := 0
	restore := stepReader
	stepReader = func(context.Context, string, int, Parser, FrameBuilder, PacketStatsInterface,
		*PacketForwarder, float64, float64, uint64, uint64, func(uint64, uint64)) error {
		unpaced++
		return nil
	}
	defer func() { stepReader = restore }()

	if _, err := ReadPCAPSequence(context.Background(), steps, SequenceReplayConfig{}); err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	if pacedCalls != 0 {
		t.Errorf("analysis mode used the paced reader %d time(s)", pacedCalls)
	}
	if unpaced != 2 {
		t.Errorf("unpaced reader saw %d steps, want 2", unpaced)
	}
}

// TestPacedStepsKeepTheirOwnWindow guards a regression: sharing a pacing origin
// across a sequence must not share the window with it.
//
// The anchor answers "when did the sequence start playing", which the pacer
// needs. A step's StartSeconds and DurationSeconds answer "which part of this
// file do we want", which is per file. Conflating them made every step after
// the first measure its end against the sequence's start, so the end threshold
// was already in the past and the step returned on its first packet — a replay
// that asked for 120 s of street and recorded 60.
func TestPacedStepsKeepTheirOwnWindow(t *testing.T) {
	steps := []capseq.ReadStep{
		{Path: "a.pcap", StartSecs: 240, DurationSecs: 60, PacketCount: 10},
		{Path: "b.pcap", StartSecs: 0, DurationSecs: 60, PacketCount: 10},
		{Path: "c.pcap", StartSecs: 0, DurationSecs: 30, PacketCount: 10},
	}

	var got []RealtimeReplayConfig
	restore := stepReaderRealtime
	stepReaderRealtime = func(_ context.Context, _ string, _ int, _ Parser, _ FrameBuilder,
		_ PacketStatsInterface, cfg RealtimeReplayConfig) error {
		got = append(got, cfg)
		return nil
	}
	defer func() { stepReaderRealtime = restore }()

	if _, err := ReadPCAPSequence(context.Background(), steps,
		SequenceReplayConfig{Paced: RealtimeReplayConfig{SpeedMultiplier: 0.5}}); err != nil {
		t.Fatalf("ReadPCAPSequence: %v", err)
	}
	if len(got) != len(steps) {
		t.Fatalf("read %d steps, want %d", len(got), len(steps))
	}
	for i, step := range steps {
		if got[i].StartSeconds != step.StartSecs {
			t.Errorf("step %d StartSeconds = %v, want %v", i, got[i].StartSeconds, step.StartSecs)
		}
		if got[i].DurationSeconds != step.DurationSecs {
			t.Errorf("step %d DurationSeconds = %v, want %v: a step must keep its own window",
				i, got[i].DurationSeconds, step.DurationSecs)
		}
	}
}
