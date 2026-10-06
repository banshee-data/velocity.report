package main

import (
	"testing"
	"time"
)

func TestProbeStatsCountsFramesBytesAndStrictGapBoundary(t *testing.T) {
	var stats probeStats
	threshold := time.Second

	if stats.observe(100, threshold, threshold) {
		t.Error("a gap exactly at the threshold was reported")
	}
	if !stats.observe(250, threshold+time.Nanosecond, threshold) {
		t.Error("a gap beyond the threshold was not reported")
	}
	if stats.frames != 2 || stats.bytesSeen != 350 {
		t.Errorf("totals = (%d frames, %d bytes), want (2, 350)", stats.frames, stats.bytesSeen)
	}
	if stats.gaps != 1 || stats.worstGap != threshold+time.Nanosecond {
		t.Errorf("gaps = (%d, worst %v), want (1, %v)", stats.gaps, stats.worstGap, threshold+time.Nanosecond)
	}
}

func TestProbeStatsKeepsTheWorstGap(t *testing.T) {
	var stats probeStats
	threshold := time.Second

	stats.observe(1, 2*time.Second, threshold)
	stats.observe(1, 1500*time.Millisecond, threshold)
	stats.observe(1, 3*time.Second, threshold)

	if stats.gaps != 3 {
		t.Errorf("gaps = %d, want 3", stats.gaps)
	}
	if stats.worstGap != 3*time.Second {
		t.Errorf("worst gap = %v, want 3s", stats.worstGap)
	}
}

func TestProbeStatsFramesPerSecondHandlesZeroAndElapsedTime(t *testing.T) {
	stats := probeStats{frames: 5}

	if got := stats.framesPerSecond(0); got != 0 {
		t.Errorf("zero elapsed rate = %v, want 0", got)
	}
	if got := stats.framesPerSecond(-time.Second); got != 0 {
		t.Errorf("negative elapsed rate = %v, want 0", got)
	}
	if got := stats.framesPerSecond(2 * time.Second); got != 2.5 {
		t.Errorf("two-second rate = %v, want 2.5", got)
	}
}
