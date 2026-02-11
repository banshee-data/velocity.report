package lidar

import (
	"testing"
	"time"
)

// ---------- helpers ----------

// makeCoverageGrid builds a minimal grid with tight, deterministic params.
// SeedFromFirstObservation is on so a single call seeds the cell.
func makeCoverageGrid(rings, azBins int) *BackgroundGrid {
	cells := make([]BackgroundCell, rings*azBins)
	params := BackgroundParams{
		BackgroundUpdateFraction:       0.5,
		ClosenessSensitivityMultiplier: 2.0,
		SafetyMarginMeters:             0.5,
		FreezeDurationNanos:            int64(1 * time.Second),
		NeighborConfirmationCount:      0, // disable neighbour confirmation
		NoiseRelativeFraction:          0.01,
		SeedFromFirstObservation:       true,
	}
	g := &BackgroundGrid{
		SensorID:    "cov-sensor",
		SensorFrame: "sensor/cov",
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       cells,
		Params:      params,
	}
	g.Manager = &BackgroundManager{Grid: g}
	return g
}

// seedCell repeatedly sends a background observation to build confidence.
func seedCell(bm *BackgroundManager, channel int, azimuth, distance float64, n int) {
	pt := []PointPolar{{Channel: channel, Azimuth: azimuth, Distance: distance}}
	for i := 0; i < n; i++ {
		_, _ = bm.ProcessFramePolarWithMask(pt)
	}
}

// ---------- 1. nil BackgroundManager / nil Grid → return nil, nil ----------

func TestMask_NilGrid(t *testing.T) {
	bm := &BackgroundManager{Grid: nil}
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Distance: 5}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mask != nil {
		t.Errorf("expected nil mask when Grid is nil, got len=%d", len(mask))
	}
}

// ---------- 2. invalid grid dimensions → return nil, nil ----------

func TestMask_InvalidGridDimensions(t *testing.T) {
	tests := []struct {
		name   string
		rings  int
		azBins int
		nCells int
	}{
		{"rings_zero", 0, 8, 0},
		{"azBins_zero", 2, 0, 0},
		{"cells_mismatch", 2, 8, 5}, // 5 != 2*8
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &BackgroundGrid{
				SensorID:    "bad-dims",
				SensorFrame: "sensor/bad",
				Rings:       tc.rings,
				AzimuthBins: tc.azBins,
				Cells:       make([]BackgroundCell, tc.nCells),
				Params:      BackgroundParams{BackgroundUpdateFraction: 0.5},
			}
			bm := &BackgroundManager{Grid: g}
			mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Distance: 5}})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mask != nil {
				t.Errorf("expected nil mask for invalid dims (%s), got len=%d", tc.name, len(mask))
			}
		})
	}
}

// ---------- 3. bad alpha clamped to 0.02 ----------

func TestMask_BadAlphaClamped(t *testing.T) {
	for _, badAlpha := range []float32{0, -1, 1.5} {
		g := makeCoverageGrid(2, 4)
		g.Params.BackgroundUpdateFraction = badAlpha
		bm := g.Manager

		// Seed one point – should not panic and should still seed
		mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10}})
		if err != nil {
			t.Fatalf("alpha=%.2f: unexpected error: %v", badAlpha, err)
		}
		if mask == nil {
			t.Fatalf("alpha=%.2f: expected non-nil mask", badAlpha)
		}
		idx := g.Idx(0, 0)
		if g.Cells[idx].TimesSeenCount == 0 {
			t.Errorf("alpha=%.2f: cell was not seeded", badAlpha)
		}
	}
}

// ---------- 4. invalid ring / negative azimuth / azBin clamp ----------

func TestMask_InvalidRingAndAzimuthEdgeCases(t *testing.T) {
	g := makeCoverageGrid(2, 8) // rings 0-1, channels 1-2
	bm := g.Manager

	points := []PointPolar{
		{Channel: -1, Azimuth: 0, Distance: 5},       // ring < 0 → foreground
		{Channel: 100, Azimuth: 0, Distance: 5},      // ring >= rings → foreground
		{Channel: 1, Azimuth: -90.0, Distance: 10.0}, // negative azimuth normalised to 270°
		{Channel: 1, Azimuth: 360.0, Distance: 10.0}, // azBin == azBins → clamped to azBins-1
	}

	mask, err := bm.ProcessFramePolarWithMask(points)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mask) != 4 {
		t.Fatalf("expected 4 mask entries, got %d", len(mask))
	}

	// Invalid channels → foreground
	if !mask[0] {
		t.Errorf("channel -1 should be foreground")
	}
	if !mask[1] {
		t.Errorf("channel 100 should be foreground")
	}

	// Negative azimuth should normalise and not panic. After seeding the cell
	// is background (SeedFromFirstObservation=true).
	if mask[2] {
		t.Errorf("negative azimuth (-90°→270°) with seed should be background, got foreground")
	}

	// 360° should clamp and not panic.
	if mask[3] {
		t.Errorf("azimuth 360° clamped should be background (seed), got foreground")
	}
}

// ---------- 5. frozen cell → foreground ----------

func TestMask_FrozenCellForeground(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	bm := g.Manager

	// Pre-populate cell with some background data
	idx := g.Idx(0, 0)
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.1
	g.Cells[idx].TimesSeenCount = 50
	// Freeze the cell 10 seconds into the future
	g.Cells[idx].FrozenUntilUnixNanos = time.Now().Add(10 * time.Second).UnixNano()

	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mask[0] {
		t.Errorf("frozen cell should always classify as foreground")
	}

	// Verify RecentForegroundCount was NOT incremented (frozen path skips it)
	if g.Cells[idx].RecentForegroundCount != 0 {
		t.Errorf("frozen cell should not accumulate recFg, got %d", g.Cells[idx].RecentForegroundCount)
	}
}

// ---------- 6. thaw detection resets recFg ----------

func TestMask_ThawResetsRecentForegroundCount(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	bm := g.Manager

	idx := g.Idx(0, 0)
	// Simulate a cell that was frozen but has now expired (freeze in the past).
	// FrozenUntilUnixNanos > 0 AND expired at least ThawGracePeriodNanos ago.
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.1
	g.Cells[idx].TimesSeenCount = 20
	g.Cells[idx].RecentForegroundCount = 15 // artificially high from freeze period
	// Expired 100ms ago — well past the 1ms grace period
	g.Cells[idx].FrozenUntilUnixNanos = time.Now().Add(-100 * time.Millisecond).UnixNano()

	// Observation matching background → should thaw and reset recFg
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The observation at 10m matches the 10m background, so should be background
	if mask[0] {
		t.Errorf("expected background after thaw, got foreground")
	}

	// recFg should have been reset to 0 by thaw, then possibly decremented to 0
	if g.Cells[idx].RecentForegroundCount > 0 {
		t.Errorf("expected RecentForegroundCount reset to 0 after thaw, got %d", g.Cells[idx].RecentForegroundCount)
	}

	// FrozenUntilUnixNanos should have been cleared
	if g.Cells[idx].FrozenUntilUnixNanos != 0 {
		t.Errorf("expected FrozenUntilUnixNanos to be cleared after thaw, got %d", g.Cells[idx].FrozenUntilUnixNanos)
	}
}

// ---------- 7. locked baseline classification ----------

func TestMask_LockedBaselineClassification(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.LockedBaselineThreshold = 10
	g.Params.LockedBaselineMultiplier = 4.0
	g.Params.SafetyMarginMeters = 0.1
	g.Params.ClosenessSensitivityMultiplier = 1.0
	g.Params.NoiseRelativeFraction = 0.01
	bm := g.Manager

	idx := g.Idx(0, 0)
	// Cell with EMA that has drifted away from original, but locked baseline is stable
	g.Cells[idx].AverageRangeMeters = 12.0 // EMA drifted to 12m
	g.Cells[idx].RangeSpreadMeters = 0.05
	g.Cells[idx].TimesSeenCount = 60
	g.Cells[idx].LockedBaseline = 10.0 // true background at 10m
	g.Cells[idx].LockedSpread = 0.05   // very tight spread
	g.Cells[idx].LockedAtCount = 50    // locked early

	// Point at 10.1m: far from EMA (12m, diff=1.9) but within locked range (10 ± 0.05*4 + noise + safety)
	// LockedWindow = 4*0.05 + 0.01*10.1 + 0.1 = 0.2 + 0.101 + 0.1 = 0.401
	// LockedDiff = |10.0 - 10.1| = 0.1 ≤ 0.401 → isWithinLockedRange
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mask[0] {
		t.Errorf("expected background via locked baseline (diff=0.1 within locked window), got foreground")
	}
}

// ---------- 8. deadlock breaker ----------

func TestMask_DeadlockBreaker(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.MinConfidenceFloor = 3
	g.Params.ClosenessSensitivityMultiplier = 2.0
	g.Params.SafetyMarginMeters = 0.1
	g.Params.NoiseRelativeFraction = 0.01
	g.Params.FreezeDurationNanos = int64(1 * time.Second)
	bm := g.Manager

	idx := g.Idx(0, 0)
	// Cell at confidence floor with high recFg — "stuck as foreground"
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.05
	g.Cells[idx].TimesSeenCount = 3        // at MinConfidenceFloor
	g.Cells[idx].RecentForegroundCount = 6 // > 4 required

	// The closeness threshold (with warmup multiplier for low seen count):
	// warmupMultiplier = 1.0 + 3.0*(100-3)/100 = 3.91
	// closenessThreshold = 2.0*(0.05 + 0.01*11.0 + 0.01)*3.91 + 0.1
	//                    = 2.0*(0.05 + 0.11 + 0.01)*3.91 + 0.1
	//                    = 2.0*0.17*3.91 + 0.1 = 1.329 + 0.1 = 1.429
	// cellDiff = |10.0 - 11.0| = 1.0
	// Not within closeness (1.0 < 1.429 — actually IS within closeness at this distance)
	// We need cellDiff > closenessThreshold but cellDiff <= freezeThresh
	// freezeThresh = 3.0 * closenessThreshold

	// So we need a point that:
	//  - is NOT within closenessThreshold of EMA average
	//  - IS within FreezeThresholdMultiplier * closenessThreshold
	// With avg=10.0, spread=0.05, seen=3, distance=12.0:
	// warmupMultiplier = 1 + 3*(100-3)/100 = 3.91
	// closenessThreshold = 2*(0.05 + 0.01*12 + 0.01)*3.91 + 0.1 = 2*0.18*3.91 + 0.1 = 1.408 + 0.1 = 1.508
	// cellDiff = |10 - 12| = 2.0 > 1.508 → NOT background
	// freezeThresh = 3.0 * 1.508 = 4.524 → cellDiff=2.0 ≤ 4.524 → deadlock breaker kicks in
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 12.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deadlock breaker should force background classification
	if mask[0] {
		t.Errorf("expected deadlock breaker to force background, got foreground (seen=%d, recFg=%d)",
			g.Cells[idx].TimesSeenCount, g.Cells[idx].RecentForegroundCount)
	}
}

// ---------- 9. cell freeze on high divergence ----------

func TestMask_CellFreezeOnHighDivergence(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.FreezeDurationNanos = int64(2 * time.Second)
	g.Params.ClosenessSensitivityMultiplier = 2.0
	g.Params.SafetyMarginMeters = 0.1
	g.Params.NoiseRelativeFraction = 0.01
	bm := g.Manager

	idx := g.Idx(0, 0)
	// Cell with moderate confidence — low enough that freeze branch fires (< 100)
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.05
	g.Cells[idx].TimesSeenCount = 50

	// Send an observation very far from background to exceed FreezeThresholdMultiplier
	// warmupMultiplier = 1.0 + 3.0*(100-50)/100 = 2.5
	// closenessThreshold = 2*(0.05 + 0.01*50.0 + 0.01)*2.5 + 0.1 = 2*0.56*2.5 + 0.1 = 2.8 + 0.1 = 2.9
	// freezeThresh = 3.0 * 2.9 = 8.7
	// cellDiff = |10 - 50| = 40 >> 8.7 → freeze triggered
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 50.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be classified as foreground
	if !mask[0] {
		t.Errorf("expected extreme divergence to be foreground")
	}

	// Cell should now be frozen
	if g.Cells[idx].FrozenUntilUnixNanos <= time.Now().UnixNano() {
		t.Errorf("expected cell to be frozen after high divergence, FrozenUntilUnixNanos=%d",
			g.Cells[idx].FrozenUntilUnixNanos)
	}
}

// ---------- 10. warmup suppression ----------

func TestMask_WarmupSuppression(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.WarmupMinFrames = 100                         // require many frames
	g.Params.WarmupDurationNanos = int64(10 * time.Minute) // require long duration too
	g.Params.SeedFromFirstObservation = true
	bm := g.Manager

	// Seed a cell so we have a known background
	seedCell(bm, 1, 0, 10.0, 3)
	idx := g.Idx(0, 0)
	if g.Cells[idx].TimesSeenCount == 0 {
		t.Fatalf("cell not seeded")
	}

	// Now send a clear foreground point — should be suppressed during warmup
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 2.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// During warmup, all foreground is suppressed (mask[i] forced false)
	if mask[0] {
		t.Errorf("expected warmup to suppress foreground, but got foreground=true (warmupRemaining=%d, settlingComplete=%v)",
			g.WarmupFramesRemaining, g.SettlingComplete)
	}

	// Verify settling is still incomplete
	if g.SettlingComplete {
		t.Errorf("settling should not be complete yet")
	}
}

// ---------- combined: freeze then thaw then re-acquire ----------

func TestMask_FreezeThawReacquire(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.FreezeDurationNanos = int64(50 * time.Millisecond) // short freeze
	g.Params.ClosenessSensitivityMultiplier = 2.0
	g.Params.SafetyMarginMeters = 0.5
	g.Params.SeedFromFirstObservation = true
	g.Params.ReacquisitionBoostMultiplier = 5.0
	g.Params.MinConfidenceFloor = 3
	bm := g.Manager

	// Build solid background at 10m
	seedCell(bm, 1, 0, 10.0, 20)

	idx := g.Idx(0, 0)
	if g.Cells[idx].TimesSeenCount < 10 {
		t.Fatalf("expected decent confidence, got %d", g.Cells[idx].TimesSeenCount)
	}

	// Send far-away observation to trigger freeze (if confidence < 100 and cellDiff >> threshold)
	// With 20 observations, TimesSeenCount won't be very high after EMA processing, but
	// let's manually set to a moderate level for the freeze branch
	g.Cells[idx].TimesSeenCount = 50

	// Trigger freeze — very large divergence
	mask, _ := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 100.0}})
	if !mask[0] {
		t.Errorf("expected large divergence to be foreground")
	}
	frozenAt := g.Cells[idx].FrozenUntilUnixNanos
	if frozenAt <= 0 {
		t.Fatalf("expected cell to be frozen")
	}

	// While frozen, subsequent observations are foreground
	mask, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}})
	if !mask[0] {
		t.Errorf("expected frozen cell to classify as foreground")
	}

	// Wait for freeze to expire
	time.Sleep(80 * time.Millisecond)

	// After thaw: observation at background distance should be classified as background
	recFgBefore := g.Cells[idx].RecentForegroundCount
	mask, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}})
	if mask[0] {
		t.Errorf("expected post-thaw background observation to be background")
	}

	// recFg should have been reset by thaw detection
	if g.Cells[idx].RecentForegroundCount >= recFgBefore && recFgBefore > 0 {
		t.Errorf("expected recFg to decrease after thaw, before=%d after=%d",
			recFgBefore, g.Cells[idx].RecentForegroundCount)
	}
}

// ---------- locked baseline: point within locked range but outside EMA ----------

func TestMask_LockedBaselineOverridesEMA(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.LockedBaselineThreshold = 5
	g.Params.LockedBaselineMultiplier = 4.0
	g.Params.ClosenessSensitivityMultiplier = 1.0
	g.Params.SafetyMarginMeters = 0.0
	g.Params.NoiseRelativeFraction = 0.005
	g.Params.NeighborConfirmationCount = 0
	bm := g.Manager

	idx := g.Idx(0, 0)
	// EMA has drifted far from the true background
	g.Cells[idx].AverageRangeMeters = 20.0
	g.Cells[idx].RangeSpreadMeters = 0.1
	g.Cells[idx].TimesSeenCount = 100

	// Lock baseline at 10m with very tight spread
	g.Cells[idx].LockedBaseline = 10.0
	g.Cells[idx].LockedSpread = 0.1
	g.Cells[idx].LockedAtCount = 50

	// Observation at 10.2m — within locked range but far from EMA (20m)
	// lockedWindow = 4*0.1 + 0.005*10.2 + 0.0 = 0.4 + 0.051 = 0.451
	// lockedDiff = |10.0 - 10.2| = 0.2 ≤ 0.451 → within locked range
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.2}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mask[0] {
		t.Errorf("expected locked baseline to classify 10.2m as background (locked at 10m)")
	}
}

// ---------- locked baseline threshold not met → no locked classification ----------

func TestMask_LockedBaselineNotMetThreshold(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.LockedBaselineThreshold = 100 // very high threshold
	g.Params.LockedBaselineMultiplier = 4.0
	g.Params.ClosenessSensitivityMultiplier = 1.0
	g.Params.SafetyMarginMeters = 0.0
	g.Params.NoiseRelativeFraction = 0.005
	g.Params.NeighborConfirmationCount = 0
	bm := g.Manager

	idx := g.Idx(0, 0)
	g.Cells[idx].AverageRangeMeters = 20.0
	g.Cells[idx].RangeSpreadMeters = 0.1
	g.Cells[idx].TimesSeenCount = 110
	g.Cells[idx].LockedBaseline = 10.0
	g.Cells[idx].LockedSpread = 0.1
	g.Cells[idx].LockedAtCount = 30 // below threshold of 100

	// Without locked baseline, EMA diff = |20 - 10.2| = 9.8 which is huge → foreground
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.2}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mask[0] {
		t.Errorf("expected foreground when locked baseline threshold not met")
	}
}

// ---------- warmup frames countdown ----------

func TestMask_WarmupFramesCountdown(t *testing.T) {
	g := makeCoverageGrid(2, 4)
	g.Params.WarmupMinFrames = 5
	g.Params.WarmupDurationNanos = 0 // only frame-gated
	g.Params.SeedFromFirstObservation = true
	bm := g.Manager

	pt := []PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}}

	// Process enough frames to exit warmup (5 warmup + settle)
	for i := 0; i < 6; i++ {
		_, _ = bm.ProcessFramePolarWithMask(pt)
	}

	if !g.SettlingComplete {
		t.Errorf("expected settling complete after %d frames (warmupMinFrames=5)", 6)
	}

	// Now foreground should NOT be suppressed
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 0.5}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 0.5m vs 10m background — clear foreground
	if !mask[0] {
		t.Errorf("expected foreground after settling complete, got background")
	}
}

// ---------- negative azimuth normalisation ----------

func TestMask_NegativeAzimuthNormalisation(t *testing.T) {
	g := makeCoverageGrid(2, 360) // 1 degree bins
	bm := g.Manager

	// -10° should normalise to 350°, which maps to azBin 350
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: -10.0, Distance: 5.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mask) != 1 {
		t.Fatalf("expected 1 mask entry, got %d", len(mask))
	}

	// The cell at ring 0, azBin 350 should have been seeded (SeedFromFirstObservation=true)
	idx := g.Idx(0, 350)
	if g.Cells[idx].TimesSeenCount == 0 {
		t.Errorf("expected cell at azBin 350 to be seeded from -10° azimuth")
	}
}

// ---------- azBin clamping at boundary ----------

func TestMask_AzBinClampAtBoundary(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	bm := g.Manager

	// Azimuth exactly 360.0 → mod 360 = 0.0 → azBin = 0
	// Azimuth 359.99 → azBin = int((359.99/360)*8) = int(7.9997) = 7
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{
		{Channel: 1, Azimuth: 359.99, Distance: 8.0},
		{Channel: 1, Azimuth: 720.0, Distance: 8.0}, // 720 mod 360 = 0 → azBin 0
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mask) != 2 {
		t.Fatalf("expected 2 mask entries, got %d", len(mask))
	}
	// Both should be seeded as background
	if mask[0] || mask[1] {
		t.Errorf("expected both boundary azimuth points to seed as background")
	}
}

// ---------- diagnostic logging branches (EnableDiagnostics=true) ----------

func TestMask_DiagnosticsFrozenCellLogging(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.DebugRingMin = 0
	g.Params.DebugRingMax = 1
	g.Params.DebugAzMin = 0
	g.Params.DebugAzMax = 360
	bm := g.Manager
	bm.EnableDiagnostics = true

	idx := g.Idx(0, 0)
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].TimesSeenCount = 20
	g.Cells[idx].FrozenUntilUnixNanos = time.Now().Add(10 * time.Second).UnixNano()

	// Hits the diagnostic branch inside the frozen-cell path (lines 226-230)
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mask[0] {
		t.Errorf("frozen cell should be foreground")
	}
}

func TestMask_DiagnosticsThawLogging(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.DebugRingMin = 0
	g.Params.DebugRingMax = 1
	g.Params.DebugAzMin = 0
	g.Params.DebugAzMax = 360
	bm := g.Manager
	bm.EnableDiagnostics = true

	idx := g.Idx(0, 0)
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.1
	g.Cells[idx].TimesSeenCount = 30
	g.Cells[idx].RecentForegroundCount = 8
	g.Cells[idx].FrozenUntilUnixNanos = time.Now().Add(-100 * time.Millisecond).UnixNano()

	// Hits thaw diagnostic branch (lines 241-244)
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mask[0] {
		t.Errorf("expected background after thaw")
	}
}

func TestMask_DiagnosticsFreezeLogging(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.FreezeDurationNanos = int64(1 * time.Second)
	g.Params.DebugRingMin = 0
	g.Params.DebugRingMax = 1
	g.Params.DebugAzMin = 0
	g.Params.DebugAzMax = 360
	bm := g.Manager
	bm.EnableDiagnostics = true

	idx := g.Idx(0, 0)
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.05
	g.Cells[idx].TimesSeenCount = 50

	// Huge divergence → foreground + freeze + diagnostic log (lines 413-417)
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 100.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mask[0] {
		t.Errorf("expected foreground for extreme divergence")
	}
}

func TestMask_DiagnosticsDebugLogging(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.DebugRingMin = 0
	g.Params.DebugRingMax = 1
	g.Params.DebugAzMin = 0
	g.Params.DebugAzMax = 360
	bm := g.Manager
	bm.EnableDiagnostics = true

	// Populate a cell, then observe it to trigger debug log (lines 424-429)
	idx := g.Idx(0, 0)
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.1
	g.Cells[idx].TimesSeenCount = 10

	_, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMask_DiagnosticsWarmupLogging(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.WarmupMinFrames = 100
	g.Params.WarmupDurationNanos = int64(10 * time.Minute)
	bm := g.Manager
	bm.EnableDiagnostics = true

	// During warmup with diagnostics → hits warmup diagnostic branch (lines 456-459)
	_, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------- parameter default branches ----------

func TestMask_DefaultClosenessSensitivity(t *testing.T) {
	g := makeCoverageGrid(2, 4)
	g.Params.ClosenessSensitivityMultiplier = 0 // → defaults to 3.0 (lines 104-106)
	bm := g.Manager

	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mask == nil {
		t.Errorf("expected non-nil mask")
	}
}

func TestMask_NegativeNeighborConfirmCount(t *testing.T) {
	g := makeCoverageGrid(2, 4)
	g.Params.NeighborConfirmationCount = -1 // → defaults to 3 (lines 108-110)
	bm := g.Manager

	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mask == nil {
		t.Errorf("expected non-nil mask")
	}
}

func TestMask_ConfidenceFloorDefaultsWhenZero(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.MinConfidenceFloor = 0 // 0 defaults to DefaultMinConfidenceFloor=3
	bm := g.Manager

	idx := g.Idx(0, 0)
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.05
	g.Cells[idx].TimesSeenCount = 5
	g.nonzeroCellCount = 1

	// Hammer with foreground — confidence should drain to floor=3 (default) not 0
	for i := 0; i < 10; i++ {
		_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 100.0}})
	}

	if g.Cells[idx].TimesSeenCount < DefaultMinConfidenceFloor {
		t.Errorf("expected TimesSeenCount >= %d (default floor), got %d",
			DefaultMinConfidenceFloor, g.Cells[idx].TimesSeenCount)
	}
}

func TestMask_PostSettleUpdateFraction(t *testing.T) {
	g := makeCoverageGrid(2, 4)
	g.Params.PostSettleUpdateFraction = 0.1 // valid post-settle alpha (lines 135-137)
	g.Params.WarmupMinFrames = 0
	g.Params.WarmupDurationNanos = 0
	bm := g.Manager

	// First frame triggers settling completion
	_, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !g.SettlingComplete {
		t.Errorf("expected settling to complete immediately with no warmup requirements")
	}
}

func TestMask_SettlingWithRegionManager(t *testing.T) {
	g := makeCoverageGrid(2, 4)
	g.Params.WarmupMinFrames = 0
	g.Params.WarmupDurationNanos = 0
	g.RegionMgr = NewRegionManager(2, 4) // triggers region identification on settle (lines 143-151)
	bm := g.Manager

	_, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !g.SettlingComplete {
		t.Errorf("expected settling complete")
	}
	// RegionMgr should have attempted identification
	if g.RegionMgr != nil && !g.RegionMgr.IdentificationComplete {
		t.Logf("region identification may not have completed (expected for small grid)")
	}
}

func TestMask_LockedBaselineMinWindow(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.LockedBaselineThreshold = 5
	g.Params.LockedBaselineMultiplier = 0.001 // very small multiplier
	g.Params.SafetyMarginMeters = 0.0
	g.Params.NoiseRelativeFraction = 0.0001
	g.Params.ClosenessSensitivityMultiplier = 0.001
	g.Params.NeighborConfirmationCount = 0
	bm := g.Manager

	idx := g.Idx(0, 0)
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.001 // very tiny
	g.Cells[idx].TimesSeenCount = 100
	g.Cells[idx].LockedBaseline = 10.0
	g.Cells[idx].LockedSpread = 0.001 // extremely tight
	g.Cells[idx].LockedAtCount = 50

	// lockedWindow = 0.001*0.001 + 0.0001*10.05 + 0.0 = ~0.001 → clamped to 0.1 (line 303-305)
	// lockedDiff = |10.0 - 10.05| = 0.05 ≤ 0.1 → within locked range
	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10.05}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mask[0] {
		t.Errorf("expected background via locked baseline minimum window clamp")
	}
}

func TestMask_WarmupWithRegionVarianceMetrics(t *testing.T) {
	g := makeCoverageGrid(2, 4)
	g.Params.WarmupMinFrames = 50
	g.Params.WarmupDurationNanos = int64(10 * time.Minute)
	g.RegionMgr = NewRegionManager(2, 4) // triggers variance metric collection (lines 163-165)
	bm := g.Manager

	_, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 10}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.SettlingComplete {
		t.Errorf("expected settling to still be active")
	}
}

// ---------- high-confidence cell does NOT freeze ----------

func TestMask_HighConfidenceCellDoesNotFreeze(t *testing.T) {
	g := makeCoverageGrid(2, 8)
	g.Params.FreezeDurationNanos = int64(5 * time.Second)
	bm := g.Manager

	idx := g.Idx(0, 0)
	// Very high confidence — freeze guard (TimesSeenCount < 100) should block freeze
	g.Cells[idx].AverageRangeMeters = 10.0
	g.Cells[idx].RangeSpreadMeters = 0.05
	g.Cells[idx].TimesSeenCount = 200

	// Large divergence that would normally trigger freeze
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0, Distance: 100.0}})

	// Should NOT be frozen because TimesSeenCount >= 100
	if g.Cells[idx].FrozenUntilUnixNanos > 0 {
		t.Errorf("high-confidence cell (seen=%d) should not freeze, but FrozenUntilUnixNanos=%d",
			g.Cells[idx].TimesSeenCount, g.Cells[idx].FrozenUntilUnixNanos)
	}
}
