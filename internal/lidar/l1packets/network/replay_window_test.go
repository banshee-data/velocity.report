package network

import (
	"testing"
	"time"
)

func TestReplayWindow(t *testing.T) {
	base := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	at := func(s float64) time.Time { return base.Add(time.Duration(s * float64(time.Second))) }

	for _, tc := range []struct {
		name            string
		start, duration float64
		times           []float64
		want            []WindowAction
	}{
		{
			name:  "whole file",
			start: 0, duration: -1,
			times: []float64{0, 1, 2, 3},
			want:  []WindowAction{WindowKeep, WindowKeep, WindowKeep, WindowKeep},
		},
		{
			name:  "zero duration is the whole file",
			start: 0, duration: 0,
			times: []float64{0, 100},
			want:  []WindowAction{WindowKeep, WindowKeep},
		},
		{
			name:  "middle window is closed at both ends",
			start: 1, duration: 2,
			times: []float64{0, 0.5, 1, 2, 3, 3.000000001},
			want:  []WindowAction{WindowSkip, WindowSkip, WindowKeep, WindowKeep, WindowKeep, WindowStop},
		},
		{
			name:  "duration is measured from the start, not the first packet",
			start: 0, duration: 1,
			times: []float64{0.5, 1.5, 1.6},
			// Anchored on the first packet offered (0.5 s), so end is 1.5 s.
			want: []WindowAction{WindowKeep, WindowKeep, WindowStop},
		},
		{
			name:  "once open, an earlier out-of-order packet is kept",
			start: 1, duration: -1,
			times: []float64{0, 1.2, 0.9},
			want:  []WindowAction{WindowSkip, WindowKeep, WindowKeep},
		},
		{
			name:  "window past the end skips everything",
			start: 10, duration: 5,
			times: []float64{0, 1, 2},
			want:  []WindowAction{WindowSkip, WindowSkip, WindowSkip},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := NewReplayWindow(tc.start, tc.duration)
			for i, s := range tc.times {
				if got := w.Admit(at(s)); got != tc.want[i] {
					t.Errorf("packet %d at %.9fs: Admit = %d, want %d", i, s, got, tc.want[i])
				}
			}
		})
	}
}

func TestReplayWindowSkipping(t *testing.T) {
	base := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	w := NewReplayWindow(1, -1)
	if !w.Skipping() {
		t.Fatal("a window with a start offset should be skipping before any packet")
	}
	w.Admit(base)
	if !w.Skipping() {
		t.Error("still skipping after a packet before start")
	}
	w.Admit(base.Add(time.Second))
	if w.Skipping() {
		t.Error("the packet at start should open the window")
	}
	if NewReplayWindow(0, 5).Skipping() {
		t.Error("a window without a start offset is open from the first packet")
	}
}
