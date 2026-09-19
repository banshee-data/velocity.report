package pipeline

import (
	"sync/atomic"
	"testing"
	"time"
)

// The frame-rate throttle exists for one situation: a PCAP replaying faster
// than real time, where unbounded catch-up floods clustering and tracking. It
// used to run unconditionally, justified by setting the cap above the sensor's
// maximum rotation rate.
//
// That reasons from rotation rate while the throttle measures arrival spacing
// at the callback. Frames are assembled from packets and delivered in clumps,
// so two completing within the minimum interval trip the gate however slowly
// the sensor turns — 5,500 live frames were throttled against a 25 fps cap in
// sixteen minutes on 2026-08-26, on a sensor turning at 10 Hz. Each throttled
// frame also skips AdvanceMisses, so live track ageing ran slower than
// configured.

func replayFlag(active bool) *atomic.Bool {
	var b atomic.Bool
	b.Store(active)
	return &b
}

func TestShouldThrottleFrame(t *testing.T) {
	const interval = 40 * time.Millisecond // 25 fps
	now := time.Unix(1_700_000_000, 0)

	// Two frames arriving 1ms apart: inside the interval, so the only thing
	// that decides the outcome is whether a replay is driving the pipeline.
	clumped := now.Add(-1 * time.Millisecond)
	spaced := now.Add(-100 * time.Millisecond)

	tests := []struct {
		name               string
		replayActive       *atomic.Bool
		analysisModeActive *atomic.Bool
		interval           time.Duration
		lastProcess        time.Time
		want               bool
	}{
		{"live input is never throttled, however clumped", replayFlag(false), nil, interval, clumped, false},
		{"an unwired flag is treated as live", nil, nil, interval, clumped, false},
		{"a replay delivering faster than the cap is throttled", replayFlag(true), nil, interval, clumped, true},
		{"a replay within the cap is not throttled", replayFlag(true), nil, interval, spaced, false},
		{"no interval configured means no throttle", replayFlag(true), nil, 0, clumped, false},
		{"the first frame of a replay is never throttled", replayFlag(true), nil, interval, time.Time{}, false},
		{"an unwired analysis flag preserves the replay-throttle outcome", replayFlag(true), nil, interval, clumped, true},
		{"analysis mode is never throttled, even flooding past the cap", replayFlag(true), replayFlag(true), interval, clumped, false},
		{"analysis mode explicitly off behaves like the unwired case", replayFlag(true), replayFlag(false), interval, clumped, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldThrottleFrame(tt.replayActive, tt.analysisModeActive, tt.interval, tt.lastProcess, now)
			if got != tt.want {
				t.Errorf("shouldThrottleFrame() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLiveIsNotThrottledAtAnyArrivalSpacing walks the spacings a real sensor
// produces, including the pathological clumping that caused the original
// symptom. None of them may throttle live input.
func TestLiveIsNotThrottledAtAnyArrivalSpacing(t *testing.T) {
	const interval = 40 * time.Millisecond
	now := time.Unix(1_700_000_000, 0)

	for _, gap := range []time.Duration{
		0,                      // two frames finalised in the same instant
		time.Millisecond,       // a clump
		39 * time.Millisecond,  // just inside the cap
		100 * time.Millisecond, // a 10 Hz sensor's nominal spacing
	} {
		if shouldThrottleFrame(replayFlag(false), nil, interval, now.Add(-gap), now) {
			t.Errorf("live input throttled at %v arrival spacing", gap)
		}
	}
}

// TestAnalysisModeIsNeverThrottledAtAnyArrivalSpacing mirrors the live-input
// guarantee above for analysis-mode replays: persisted analysis output must
// reach clustering and tracking regardless of how fast frames arrive,
// because a throttled frame is recorded as empty rather than skipped, and an
// analysis run/VRLOG consumer has no way to tell "empty" from "not looked at".
func TestAnalysisModeIsNeverThrottledAtAnyArrivalSpacing(t *testing.T) {
	const interval = 40 * time.Millisecond
	now := time.Unix(1_700_000_000, 0)

	for _, gap := range []time.Duration{
		0, // an unpaced "analysis" speed-mode catch-up flood
		time.Millisecond,
		39 * time.Millisecond,
	} {
		if shouldThrottleFrame(replayFlag(true), replayFlag(true), interval, now.Add(-gap), now) {
			t.Errorf("analysis-mode replay throttled at %v arrival spacing", gap)
		}
	}
}
