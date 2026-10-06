package l3grid

import (
	"math"
	"testing"
	"time"
)

// Settling used to end only when both a frame count and a fixed duration had
// elapsed, so every scene paid the full warm-up — thirty seconds of empty
// screen on live input — however quickly its background had actually
// stabilised. The convergence criteria to do better already existed in this
// package and in config; nothing consulted them.
//
// The duration is now a ceiling for scenes that never converge rather than a
// toll every scene pays.

func convergenceParams() BackgroundParams {
	return BackgroundParams{
		SeedFromFirstObservation:   true,
		BackgroundUpdateFraction:   0.5,
		WarmupMinFrames:            2,
		WarmupDurationNanos:        int64(time.Hour), // effectively never
		SettlingMinCoverage:        0.8,
		SettlingMaxSpreadDelta:     0.001,
		SettlingMinRegionStability: 0.95,
		SettlingMinConfidence:      10.0,
		SettlingCheckInterval:      1,
	}
}

func TestConvergenceThresholdsRequireAllFour(t *testing.T) {
	full := convergenceParams()

	tests := []struct {
		name   string
		mutate func(*BackgroundParams)
	}{
		{"no coverage", func(p *BackgroundParams) { p.SettlingMinCoverage = 0 }},
		{"no spread delta", func(p *BackgroundParams) { p.SettlingMaxSpreadDelta = 0 }},
		{"no region stability", func(p *BackgroundParams) { p.SettlingMinRegionStability = 0 }},
		{"no confidence", func(p *BackgroundParams) { p.SettlingMinConfidence = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := full
			tt.mutate(&params)
			bm := NewBackgroundManagerDI("thresholds-"+tt.name, 4, 8, params, nil)

			bm.Grid.mu.Lock()
			_, ok := bm.Grid.settlingThresholdsLocked()
			bm.Grid.mu.Unlock()

			if ok {
				t.Error("a partially configured threshold set was accepted; the grid could settle on whichever dimensions happened to be filled in")
			}
		})
	}

	bm := NewBackgroundManagerDI("thresholds-full", 4, 8, full, nil)
	bm.Grid.mu.Lock()
	got, ok := bm.Grid.settlingThresholdsLocked()
	bm.Grid.mu.Unlock()

	if !ok {
		t.Fatal("a fully configured threshold set was rejected")
	}
	// The params hold float32, so compare with a tolerance rather than exactly.
	if math.Abs(got.MinCoverage-0.8) > 1e-6 || math.Abs(got.MinConfidence-10.0) > 1e-6 {
		t.Errorf("thresholds = %+v, want the configured values", got)
	}
}

// Without thresholds the behaviour is unchanged: settling stays on the
// frame-count-and-duration rule.
func TestNoConvergenceCheckWithoutThresholds(t *testing.T) {
	params := convergenceParams()
	params.SettlingMinCoverage = 0
	bm := NewBackgroundManagerDI("no-thresholds", 4, 8, params, nil)

	bm.Grid.mu.Lock()
	converged := bm.Grid.settlingConvergedLocked()
	bm.Grid.mu.Unlock()

	if converged {
		t.Error("reported convergence with no thresholds configured")
	}
}

// Evaluation walks every cell, so it must not run on every frame.
func TestConvergenceIsEvaluatedOnAnInterval(t *testing.T) {
	params := convergenceParams()
	params.SettlingCheckInterval = 5
	bm := NewBackgroundManagerDI("interval", 4, 8, params, nil)

	bm.Grid.mu.Lock()
	defer bm.Grid.mu.Unlock()

	// The first four calls are skipped outright, without evaluating.
	for i := 1; i < 5; i++ {
		if bm.Grid.settlingConvergedLocked() {
			t.Fatalf("call %d evaluated and reported convergence before the interval elapsed", i)
		}
		if bm.Grid.settlingCheckCounter != i {
			t.Fatalf("counter = %d after call %d, want %d", bm.Grid.settlingCheckCounter, i, i)
		}
	}

	// The fifth evaluates and resets the counter.
	bm.Grid.settlingConvergedLocked()
	if bm.Grid.settlingCheckCounter != 0 {
		t.Errorf("counter = %d after the interval elapsed, want 0", bm.Grid.settlingCheckCounter)
	}
}

// An empty grid has no coverage and no confidence, so it must not be mistaken
// for a converged one — that would settle instantly and hand foreground
// extraction a background of nothing.
func TestAnEmptyGridDoesNotConverge(t *testing.T) {
	bm := NewBackgroundManagerDI("empty", 4, 8, convergenceParams(), nil)

	bm.Grid.mu.Lock()
	converged := bm.Grid.settlingConvergedLocked()
	bm.Grid.mu.Unlock()

	if converged {
		t.Error("an empty grid reported convergence; settling would complete with no background at all")
	}
}
