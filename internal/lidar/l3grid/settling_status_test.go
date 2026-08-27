package l3grid

import (
	"testing"
	"time"
)

// Until the grid settles there is no usable background, so foreground
// extraction yields nothing and the visualiser shows an empty scene — which
// looks exactly like a broken sensor. The progress is reported so an operator
// can tell "warming up" from "nothing is arriving".

func TestSettlingStatusReportsCompletion(t *testing.T) {
	bm := NewBackgroundManagerDI("settle-complete", 16, 360, BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          0,
		WarmupDurationNanos:      0,
	}, nil)

	bm.Grid.mu.Lock()
	bm.Grid.SettlingComplete = true
	bm.Grid.mu.Unlock()

	got := bm.SettlingStatus()
	if !got.Complete {
		t.Error("Complete = false after settling finished")
	}
	if got.Progress != 1 {
		t.Errorf("Progress = %v, want 1 once complete", got.Progress)
	}
}

// Settling needs both a frame count and a duration. Progress must follow
// whichever is further from being met, or it reports nearly done while the
// other requirement still has most of its wait left.
func TestSettlingStatusFollowsTheLaggingRequirement(t *testing.T) {
	bm := NewBackgroundManagerDI("settle-lagging", 16, 360, BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          100,
		WarmupDurationNanos:      int64(60 * time.Second),
	}, nil)

	// Nearly all frames seen, but the clock has barely started.
	bm.StartTime = time.Now().Add(-6 * time.Second) // 10% of the duration
	bm.Grid.mu.Lock()
	bm.Grid.WarmupFramesRemaining = 5 // 95% of the frames
	bm.Grid.mu.Unlock()

	got := bm.SettlingStatus()
	if got.Complete {
		t.Fatal("reported complete while both requirements are outstanding")
	}
	if got.Progress > 0.2 {
		t.Errorf("Progress = %.2f, want the duration's ~0.1: progress must follow the requirement still holding completion up", got.Progress)
	}
}

func TestSettlingStatusStaysInRange(t *testing.T) {
	bm := NewBackgroundManagerDI("settle-range", 16, 360, BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          10,
		WarmupDurationNanos:      int64(time.Second),
	}, nil)

	// Long past the duration, and more frames seen than required.
	bm.StartTime = time.Now().Add(-time.Hour)
	bm.Grid.mu.Lock()
	bm.Grid.WarmupFramesRemaining = -5
	bm.Grid.mu.Unlock()

	if got := bm.SettlingStatus().Progress; got != 1 {
		t.Errorf("Progress = %v, want it clamped to 1", got)
	}
}

func TestSettlingStatusOnNilManager(t *testing.T) {
	var bm *BackgroundManager
	if got := bm.SettlingStatus(); got.Complete || got.Progress != 0 {
		t.Errorf("nil manager reported %+v, want the zero status", got)
	}
}
