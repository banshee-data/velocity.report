package l5tracks

import (
	"testing"
	"time"
)

// A car hidden for 3 frames, seen again, then coasting out for 8 frames
// after it leaves: all 11 misses are occlusions to the live counters, but
// only the 3-frame gap closed by a re-observation is to the closed ones.
func TestClosedOcclusionCountersLeaveOutTheExitCoast(t *testing.T) {
	const gapFrom, gapTo, leaves, frames = 25, 28, 40, 48
	body := uncCar(-30, 15, 8)
	body.hidden = func(f int) bool { return (f >= gapFrom && f < gapTo) || f >= leaves }
	scene := uncScene{frames: frames, period: 100 * time.Millisecond, seed: 13,
		bodies: []uncBody{body}, noise: uncNoise{0.03, 0.08}}

	tk := NewTracker(DefaultTrackerConfig())
	var seq int64
	for i, f := range scene.render() {
		tk.Update(f.clusters, f.at)
		if i == gapFrom-1 {
			for _, tr := range tk.Tracks {
				if tr.TrackState == TrackConfirmed {
					seq = tr.CreationSequence
				}
			}
		}
	}
	if seq == 0 {
		t.Fatal("no confirmed track before the gap")
	}
	var track *TrackedObject
	for _, tr := range tk.Tracks {
		if tr.CreationSequence == seq {
			track = tr
		}
	}
	if track == nil || track.TrackState == TrackDeleted {
		t.Fatal("the car's track did not survive to the end of the capture")
	}
	if track.Misses != frames-leaves {
		t.Fatalf("track has %d misses, want %d coasting out", track.Misses, frames-leaves)
	}
	if track.OcclusionCount != 11 || track.MaxOcclusionFrames != 8 {
		t.Errorf("live counters %d / %d, want 11 / 8", track.OcclusionCount, track.MaxOcclusionFrames)
	}
	if track.ClosedOcclusionCount != 3 || track.MaxClosedOcclusionFrames != 3 {
		t.Errorf("closed counters %d / %d, want 3 / 3", track.ClosedOcclusionCount, track.MaxClosedOcclusionFrames)
	}
}
