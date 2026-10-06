package l3grid

import (
	"strings"
	"testing"
	"time"
)

// armedThresholds is a convergence set every measurement below meets, so tests
// that care about the decision path rather than the numbers can settle on
// demand.
func armedThresholds(p BackgroundParams) BackgroundParams {
	p.SettlingMinCoverage = 0.01
	p.SettlingMaxSpreadDelta = 1000
	p.SettlingMinRegionStability = 0.01
	p.SettlingMinConfidence = 0.5
	return p
}

// seedAllCells gives every cell an observation so coverage and confidence clear
// any threshold, letting convergence turn on the criteria under test.
func seedAllCells(g *BackgroundGrid) {
	for i := range g.Cells {
		g.Cells[i].AverageRangeMeters = 10
		g.Cells[i].RangeSpreadMeters = 0.1
		g.Cells[i].TimesSeenCount = 20
	}
}

// TestSettlingCompletesOnConvergenceBeforeCeiling covers the early-exit path:
// a grid that has demonstrably converged finishes settling without waiting out
// the warm-up duration, which is the whole point of arming convergence.
func TestSettlingCompletesOnConvergenceBeforeCeiling(t *testing.T) {
	bm := newSettlingGrid(t, armedThresholds(BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          10,
		WarmupDurationNanos:      int64(time.Hour), // ceiling nowhere near reached
		SettlingCheckInterval:    1,
	}))

	bm.Grid.mu.Lock()
	defer bm.Grid.mu.Unlock()
	seedAllCells(bm.Grid)
	bm.Grid.WarmupFramesRemaining = 0

	if !bm.settlingCompleteLocked(time.Now().UnixNano()) {
		t.Error("a converged grid should settle before the duration ceiling")
	}
}

// TestSettlingCheckIntervalDefaultsWhenUnset covers the interval fallback.
// Evaluation walks every cell, so a zero interval must mean "use the default
// pacing", not "evaluate on every frame".
func TestSettlingCheckIntervalDefaultsWhenUnset(t *testing.T) {
	bm := newSettlingGrid(t, armedThresholds(BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          10,
		WarmupDurationNanos:      int64(time.Hour),
		SettlingCheckInterval:    0, // unset
	}))

	bm.Grid.mu.Lock()
	defer bm.Grid.mu.Unlock()
	seedAllCells(bm.Grid)

	// The first call short of the default interval must not evaluate.
	if bm.Grid.settlingConvergedLocked() {
		t.Fatal("converged on the first call despite the default check interval")
	}
	if bm.Grid.settlingCheckCounter != 1 {
		t.Fatalf("check counter = %d, want 1", bm.Grid.settlingCheckCounter)
	}

	// Reaching the default interval evaluates and resets the counter.
	for i := 1; i < defaultSettlingCheckInterval; i++ {
		bm.Grid.settlingConvergedLocked()
	}
	if bm.Grid.settlingCheckCounter != 0 {
		t.Errorf("check counter = %d after %d calls, want a reset to 0",
			bm.Grid.settlingCheckCounter, defaultSettlingCheckInterval)
	}
}

// TestUnmetCriteriaNamesEachFailingThreshold covers the reporting branches. The
// log line exists so an operator can see which criterion is holding settling
// up; a branch that never fires is a criterion that silently never reports.
func TestUnmetCriteriaNamesEachFailingThreshold(t *testing.T) {
	thresholds := SettlingThresholds{
		MinCoverage:        0.80,
		MaxSpreadDelta:     0.001,
		MinRegionStability: 0.95,
		MinConfidence:      10.0,
	}

	tests := []struct {
		name    string
		metrics SettlingMetrics
		want    string
	}{
		{
			name:    "coverage",
			metrics: SettlingMetrics{CoverageRate: 0.5, SpreadDeltaRate: 0, RegionStability: 1, MeanConfidence: 20},
			want:    "coverage",
		},
		{
			name:    "spread delta",
			metrics: SettlingMetrics{CoverageRate: 0.9, SpreadDeltaRate: 0.5, RegionStability: 1, MeanConfidence: 20},
			want:    "spread_delta",
		},
		{
			name:    "region stability",
			metrics: SettlingMetrics{CoverageRate: 0.9, SpreadDeltaRate: 0, RegionStability: 0.1, MeanConfidence: 20},
			want:    "region_stability",
		},
		{
			name:    "mean confidence",
			metrics: SettlingMetrics{CoverageRate: 0.9, SpreadDeltaRate: 0, RegionStability: 1, MeanConfidence: 1},
			want:    "mean_confidence",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unmetCriteria(tc.metrics, thresholds)
			if !strings.Contains(got, tc.want) {
				t.Errorf("unmetCriteria() = %q, want it to name %q", got, tc.want)
			}
		})
	}
}

// TestUnmetCriteriaReportsAllMet covers the case where the caller asks what is
// unmet and nothing is. It should say so rather than return an empty string,
// which in a log line is indistinguishable from a formatting bug.
func TestUnmetCriteriaReportsAllMet(t *testing.T) {
	thresholds := DefaultSettlingThresholds()
	metrics := SettlingMetrics{
		CoverageRate:    0.99,
		SpreadDeltaRate: 0,
		RegionStability: 1,
		MeanConfidence:  50,
	}

	if got := unmetCriteria(metrics, thresholds); got != "all criteria met" {
		t.Errorf("unmetCriteria() = %q, want %q", got, "all criteria met")
	}
}
