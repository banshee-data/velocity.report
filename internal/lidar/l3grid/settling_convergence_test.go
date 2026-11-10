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
	converged := bm.Grid.settlingConvergedLocked(0)
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
		if bm.Grid.settlingConvergedLocked(0) {
			t.Fatalf("call %d evaluated and reported convergence before the interval elapsed", i)
		}
		if bm.Grid.settlingCheckCounter != i {
			t.Fatalf("counter = %d after call %d, want %d", bm.Grid.settlingCheckCounter, i, i)
		}
	}

	// The fifth evaluates and resets the counter.
	bm.Grid.settlingConvergedLocked(0)
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
	converged := bm.Grid.settlingConvergedLocked(0)
	bm.Grid.mu.Unlock()

	if converged {
		t.Error("an empty grid reported convergence; settling would complete with no background at all")
	}
}

// Every evaluation is recorded with its time and its four measures, so a
// replay can read the convergence trajectory rather than only its verdict
// (l3bg0022). Checks skipped by the interval are not evaluations and are not
// recorded; a reset empties the record with the rest of the warm-up state.
func TestSettlingTrajectoryRecordsEachEvaluation(t *testing.T) {
	params := convergenceParams()
	params.SettlingCheckInterval = 2
	params.SettlingHonestFirstCheck = true
	bm := NewBackgroundManagerDI("trajectory", 4, 8, params, nil)

	if got := bm.SettlingTrajectory(); len(got) != 0 {
		t.Fatalf("new grid has %d checks recorded, want none", len(got))
	}
	bm.Grid.mu.Lock()
	for i := 1; i <= 4; i++ {
		bm.Grid.settlingConvergedLocked(time.Duration(i) * time.Second)
	}
	bm.Grid.mu.Unlock()

	got := bm.SettlingTrajectory()
	if len(got) != 2 {
		t.Fatalf("recorded %d checks over four calls at interval 2, want 2", len(got))
	}
	if got[0].ElapsedSeconds != 2 || got[1].ElapsedSeconds != 4 {
		t.Errorf("elapsed = %v, %v; want 2, 4", got[0].ElapsedSeconds, got[1].ElapsedSeconds)
	}
	if got[0].Measured || !got[1].Measured {
		t.Errorf("measured = %v, %v; want the first check unmeasured and the second measured", got[0].Measured, got[1].Measured)
	}
	if got[0].Converged || got[1].Converged {
		t.Error("an empty grid's checks were recorded as converged")
	}

	// The copy is the caller's: changing it leaves the record alone.
	got[0].ElapsedSeconds = 99
	if bm.SettlingTrajectory()[0].ElapsedSeconds != 2 {
		t.Error("SettlingTrajectory returned the grid's own slice")
	}

	if err := bm.ResetGrid(); err != nil {
		t.Fatal(err)
	}
	if n := len(bm.SettlingTrajectory()); n != 0 {
		t.Errorf("%d checks survive a reset, want none", n)
	}
}

// A relative ratio judges the spread delta against a share of the mean
// spread, so a busy scene whose delta sits above the absolute threshold by
// traffic alone can still settle, while the other criteria still bind
// (l3bg0030).
func TestRelativeThresholdsJudgeTheDeltaAgainstTheMeanSpread(t *testing.T) {
	bm := NewBackgroundManagerDI("relative-thresholds", 4, 8, convergenceParams(), nil)
	thresholds, armed := bm.Grid.settlingThresholdsLocked()
	if !armed {
		t.Fatal("setup: convergence is not armed")
	}
	busy := SettlingMetrics{CoverageRate: 0.95, SpreadDeltaRate: 0.01, RegionStability: 1, MeanConfidence: 60, MeanSpread: 0.2}
	if busy.IsConverged(relativeThresholds(thresholds, busy, 0)) {
		t.Fatal("ratio off: a delta of 0.01 passed the absolute 0.001")
	}
	if !busy.IsConverged(relativeThresholds(thresholds, busy, 0.075)) {
		t.Fatal("ratio 0.075 of a 0.2 m mean spread is 0.015; a delta of 0.01 should pass")
	}
	unsettled := busy
	unsettled.SpreadDeltaRate = 0.02
	if unsettled.IsConverged(relativeThresholds(thresholds, unsettled, 0.075)) {
		t.Fatal("a delta of 0.02 passed a relative threshold of 0.015")
	}
	sparse := busy
	sparse.CoverageRate = 0.5
	if sparse.IsConverged(relativeThresholds(thresholds, sparse, 0.075)) {
		t.Fatal("the relative ratio let a grid under its coverage criterion settle")
	}
	if got := relativeThresholds(thresholds, busy, 0); got != thresholds {
		t.Fatalf("ratio off changed the thresholds: %+v", got)
	}
}

func TestSetSettlingRelativeSpreadRatio(t *testing.T) {
	bm := NewBackgroundManagerDI("relative", 4, 8, convergenceParams(), nil)
	if err := bm.SetSettlingRelativeSpreadRatio(0.075); err != nil {
		t.Fatal(err)
	}
	if bm.Grid.Params.SettlingRelativeSpreadRatio != 0.075 {
		t.Fatalf("ratio %v", bm.Grid.Params.SettlingRelativeSpreadRatio)
	}
	if err := bm.SetSettlingRelativeSpreadRatio(-1); err == nil {
		t.Fatal("a negative ratio was accepted")
	}
}

// SettledAfter reports when warm-up ended and what ended it, by the frames'
// clock from the first frame, so a replay can measure windows after the
// settle (l3bg0030's spurious-cluster read). It reports nothing before the
// grid settles and nothing after a reset.
func TestSettledAfterRecordsTheCeiling(t *testing.T) {
	params := convergenceParams()
	params.WarmupDurationNanos = int64(30 * time.Second)
	bm := NewBackgroundManagerDI("settled-after", 4, 8, params, nil)
	if _, _, ok := bm.SettledAfter(); ok {
		t.Fatal("a new grid reports a settle")
	}
	start := time.Unix(1000, 0)
	bm.StartTime = start
	bm.Grid.mu.Lock()
	early := bm.settlingCompleteLocked(start.Add(10 * time.Second).UnixNano())
	bm.Grid.mu.Unlock()
	if early {
		t.Fatal("an empty grid settled before the ceiling")
	}
	if _, _, ok := bm.SettledAfter(); ok {
		t.Fatal("an unsettled grid reports a settle")
	}
	bm.Grid.mu.Lock()
	done := bm.settlingCompleteLocked(start.Add(31 * time.Second).UnixNano())
	bm.Grid.mu.Unlock()
	if !done {
		t.Fatal("the grid did not settle at the ceiling")
	}
	elapsed, onConvergence, ok := bm.SettledAfter()
	if !ok || onConvergence || elapsed != 31*time.Second {
		t.Fatalf("SettledAfter = %v, %v, %v; want 31s at the ceiling", elapsed, onConvergence, ok)
	}
	if err := bm.ResetGrid(); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := bm.SettledAfter(); ok {
		t.Error("a settle survives a reset")
	}
}
