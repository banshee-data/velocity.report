package lidar

import (
	"testing"
	"time"
)

// makeTestGridStrict creates a test grid with tighter thresholds for foreground detection
func makeTestGridStrict(rings, azBins int) *BackgroundGrid {
	cells := make([]BackgroundCell, rings*azBins)
	params := BackgroundParams{
		BackgroundUpdateFraction:       0.5, // use large alpha for deterministic updates
		ClosenessSensitivityMultiplier: 2.0,
		SafetyMarginMeters:             0.5, // tight safety margin for sensitive foreground detection
		FreezeDurationNanos:            int64(1 * time.Second),
		NeighborConfirmationCount:      5, // higher threshold to avoid neighbor confirmation
		NoiseRelativeFraction:          0.01,
		SeedFromFirstObservation:       true, // seed from first observation to build background
	}
	g := &BackgroundGrid{
		SensorID:    "test-sensor",
		SensorFrame: "sensor/test",
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       cells,
		Params:      params,
	}
	g.Manager = &BackgroundManager{Grid: g}
	return g
}

func TestProcessFramePolarWithMask_BasicClassification(t *testing.T) {
	// Use strict grid with tighter thresholds for this test
	g := makeTestGridStrict(2, 8)
	bm := g.Manager

	// Initialise background at 10m for ring 0, azBin 0 using ProcessFramePolarWithMask
	// This will seed the cell due to SeedFromFirstObservation=true
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})

	// Verify background cell was initialized
	idx := g.Idx(0, 0)
	if g.Cells[idx].TimesSeenCount == 0 {
		t.Fatalf("background cell should be initialized after seeding")
	}
	t.Logf("Background cell initialized: avg=%.2f, spread=%.4f, times_seen=%d",
		g.Cells[idx].AverageRangeMeters, g.Cells[idx].RangeSpreadMeters, g.Cells[idx].TimesSeenCount)

	// Test points: one background-like (10m), one foreground (3m - far from 10m background)
	points := []PointPolar{
		{Channel: 1, Azimuth: 0.0, Distance: 10.0}, // Should be background
		{Channel: 1, Azimuth: 0.0, Distance: 3.0},  // Should be foreground (7m different from background)
	}

	mask, err := bm.ProcessFramePolarWithMask(points)
	if err != nil {
		t.Fatalf("ProcessFramePolarWithMask failed: %v", err)
	}

	if len(mask) != 2 {
		t.Fatalf("expected mask length 2, got %d", len(mask))
	}

	// First point (10m) should be classified as background (false)
	if mask[0] {
		t.Errorf("expected point at 10m to be background (false), got foreground (true)")
	}

	// Second point (3m) should be classified as foreground (true)
	if !mask[1] {
		t.Errorf("expected point at 3m to be foreground (true), got background (false)")
	}
}

func TestProcessFramePolarWithMask_EmptyInput(t *testing.T) {
	g := makeTestGrid(2, 8)
	bm := g.Manager

	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mask) != 0 {
		t.Errorf("expected empty mask, got length %d", len(mask))
	}
}

func TestProcessFramePolarWithMask_NilManager(t *testing.T) {
	var bm *BackgroundManager

	mask, err := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mask != nil {
		t.Errorf("expected nil mask for nil manager")
	}
}

func TestProcessFramePolarWithMask_InvalidChannel(t *testing.T) {
	g := makeTestGrid(2, 8) // 2 rings = channels 1-2 valid
	bm := g.Manager

	points := []PointPolar{
		{Channel: 0, Azimuth: 0.0, Distance: 10.0},  // Invalid (channel 0)
		{Channel: 99, Azimuth: 0.0, Distance: 10.0}, // Invalid (channel too high)
		{Channel: 1, Azimuth: 0.0, Distance: 10.0},  // Valid
	}

	mask, err := bm.ProcessFramePolarWithMask(points)
	if err != nil {
		t.Fatalf("ProcessFramePolarWithMask failed: %v", err)
	}

	// Invalid channels should be treated as foreground
	if !mask[0] {
		t.Errorf("expected invalid channel 0 to be foreground")
	}
	if !mask[1] {
		t.Errorf("expected invalid channel 99 to be foreground")
	}
}

func TestProcessFramePolarWithMask_SeedFromFirstObservation(t *testing.T) {
	g := makeTestGrid(2, 8)
	g.Params.SeedFromFirstObservation = true
	bm := g.Manager

	// First observation should seed the cell and be classified as background
	points := []PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}}

	mask, err := bm.ProcessFramePolarWithMask(points)
	if err != nil {
		t.Fatalf("ProcessFramePolarWithMask failed: %v", err)
	}

	if mask[0] {
		t.Errorf("expected first observation with SeedFromFirstObservation=true to be background")
	}

	// Verify cell was initialized
	idx := g.Idx(0, 0)
	cell := g.Cells[idx]
	if cell.TimesSeenCount == 0 {
		t.Errorf("expected cell to be initialized after seeding")
	}
	if cell.AverageRangeMeters != 10.0 {
		t.Errorf("expected cell average to be 10.0, got %v", cell.AverageRangeMeters)
	}
}

func TestExtractForegroundPoints(t *testing.T) {
	points := []PointPolar{
		{Channel: 1, Distance: 5.0},
		{Channel: 1, Distance: 10.0},
		{Channel: 1, Distance: 15.0},
		{Channel: 1, Distance: 20.0},
	}
	mask := []bool{true, false, true, false}

	foreground := ExtractForegroundPoints(points, mask)

	if len(foreground) != 2 {
		t.Fatalf("expected 2 foreground points, got %d", len(foreground))
	}
	if foreground[0].Distance != 5.0 {
		t.Errorf("expected first foreground distance 5.0, got %v", foreground[0].Distance)
	}
	if foreground[1].Distance != 15.0 {
		t.Errorf("expected second foreground distance 15.0, got %v", foreground[1].Distance)
	}
}

func TestExtractForegroundPoints_EmptyInput(t *testing.T) {
	points := []PointPolar{}
	mask := []bool{}

	result := ExtractForegroundPoints(points, mask)
	if result != nil {
		t.Errorf("expected nil result for empty input")
	}
}

func TestExtractForegroundPoints_MismatchedLengths(t *testing.T) {
	points := []PointPolar{{Channel: 1}}
	mask := []bool{true, false}

	result := ExtractForegroundPoints(points, mask)
	if result != nil {
		t.Errorf("expected nil result for mismatched lengths")
	}
}

func TestComputeFrameMetrics(t *testing.T) {
	mask := []bool{true, true, false, false, false}

	metrics := ComputeFrameMetrics(mask, 1500)

	if metrics.TotalPoints != 5 {
		t.Errorf("expected TotalPoints 5, got %d", metrics.TotalPoints)
	}
	if metrics.ForegroundPoints != 2 {
		t.Errorf("expected ForegroundPoints 2, got %d", metrics.ForegroundPoints)
	}
	if metrics.BackgroundPoints != 3 {
		t.Errorf("expected BackgroundPoints 3, got %d", metrics.BackgroundPoints)
	}
	expectedFraction := 0.4
	if metrics.ForegroundFraction != expectedFraction {
		t.Errorf("expected ForegroundFraction %v, got %v", expectedFraction, metrics.ForegroundFraction)
	}
	if metrics.ProcessingTimeUs != 1500 {
		t.Errorf("expected ProcessingTimeUs 1500, got %d", metrics.ProcessingTimeUs)
	}
}

func TestComputeFrameMetrics_EmptyMask(t *testing.T) {
	mask := []bool{}
	metrics := ComputeFrameMetrics(mask, 0)

	if metrics.TotalPoints != 0 {
		t.Errorf("expected TotalPoints 0 for empty mask")
	}
	if metrics.ForegroundFraction != 0 {
		t.Errorf("expected ForegroundFraction 0 for empty mask")
	}
}

func TestProcessFramePolarWithMask_NeighborConfirmation(t *testing.T) {
	g := makeTestGrid(3, 3)
	bm := g.Manager

	// Initialise center and neighbors with 10m
	azStep := 360.0 / float64(g.AzimuthBins)
	for da := -1; da <= 1; da++ {
		a := 1 + da
		az := (float64(a) + 0.5) * azStep
		bm.ProcessFramePolar([]PointPolar{{Channel: 2, Azimuth: az, Distance: 10.0}})
		bm.ProcessFramePolar([]PointPolar{{Channel: 2, Azimuth: az, Distance: 10.0}})
	}

	// Test point with similar distance - should be background due to neighbor confirmation
	centerAz := (float64(1) + 0.5) * azStep
	points := []PointPolar{{Channel: 2, Azimuth: centerAz, Distance: 10.2}}

	mask, err := bm.ProcessFramePolarWithMask(points)
	if err != nil {
		t.Fatalf("ProcessFramePolarWithMask failed: %v", err)
	}

	if mask[0] {
		t.Errorf("expected point to be background due to neighbor confirmation")
	}
}

func TestProcessFramePolarWithMask_FrozenCell(t *testing.T) {
	g := makeTestGrid(2, 8)
	bm := g.Manager

	// Manually freeze a cell
	idx := g.Idx(0, 0)
	g.Cells[idx].FrozenUntilUnixNanos = time.Now().Add(10 * time.Second).UnixNano()

	points := []PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}}
	mask, err := bm.ProcessFramePolarWithMask(points)
	if err != nil {
		t.Fatalf("ProcessFramePolarWithMask failed: %v", err)
	}

	// Frozen cell should classify as foreground
	if !mask[0] {
		t.Errorf("expected frozen cell to be treated as foreground")
	}
}

func TestProcessFramePolarWithMask_FastReacquisition(t *testing.T) {
	// Test that cells recover quickly from foreground events
	// Use a larger grid to allow neighbor confirmation to work
	g := makeTestGridStrict(4, 16)
	g.Params.ReacquisitionBoostMultiplier = 5.0 // 5x faster re-acquisition
	g.Params.MinConfidenceFloor = 3             // Preserve minimum confidence
	g.Params.BackgroundUpdateFraction = 0.1     // 10% base alpha
	g.Params.SafetyMarginMeters = 0.1           // Tight safety for this test
	g.Params.NeighborConfirmationCount = 0      // 0 to disable neighbor confirmation
	g.Params.ClosenessSensitivityMultiplier = 1.0
	g.Params.NoiseRelativeFraction = 0.01
	g.Params.FreezeDurationNanos = 0 // Disable freeze for this test
	bm := g.Manager

	// Initialise background at 10m with multiple observations to build confidence
	for i := 0; i < 10; i++ {
		_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})
	}

	idx := g.Idx(0, 0)
	initialSeenCount := g.Cells[idx].TimesSeenCount
	if initialSeenCount < 5 {
		t.Fatalf("expected high confidence after 10 observations, got TimesSeenCount=%d", initialSeenCount)
	}

	// Calculate expected threshold
	spread := g.Cells[idx].RangeSpreadMeters
	noiseRel := g.Params.NoiseRelativeFraction
	closenessMultiplier := g.Params.ClosenessSensitivityMultiplier
	safety := g.Params.SafetyMarginMeters
	threshold := float64(closenessMultiplier)*(float64(spread)+float64(noiseRel)*10.0+0.01) + float64(safety)
	t.Logf("After 10 bg observations: avg=%.2f, spread=%.4f, seen=%d, recentFg=%d, threshold=%.4f",
		g.Cells[idx].AverageRangeMeters, spread, g.Cells[idx].TimesSeenCount,
		g.Cells[idx].RecentForegroundCount, threshold)

	// Now simulate a vehicle passing through (foreground at 3m - 7m from 10m background)
	for i := 0; i < 5; i++ {
		mask, _ := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 3.0}})
		diff := 7.0 // |10-3|
		if !mask[0] {
			t.Errorf("iteration %d: expected 3m to be foreground (diff=%.2f > threshold=%.4f)", i, diff, threshold)
		}
	}

	// Check that RecentForegroundCount increased and confidence was preserved
	afterFgSeenCount := g.Cells[idx].TimesSeenCount
	recentFgCount := g.Cells[idx].RecentForegroundCount
	t.Logf("After 5 fg observations: avg=%.2f, seen=%d, recentFg=%d",
		g.Cells[idx].AverageRangeMeters, afterFgSeenCount, recentFgCount)

	// All 5 should have been foreground
	if recentFgCount != 5 {
		t.Errorf("expected RecentForegroundCount=5 after 5 foreground events, got %d", recentFgCount)
	}

	// TimesSeenCount should have decremented but not below floor
	expectedSeen := uint32(10) - 5 // Started at 10, decremented 5 times
	if expectedSeen < 3 {
		expectedSeen = 3 // Floor kicks in
	}
	if afterFgSeenCount != expectedSeen && afterFgSeenCount < 3 {
		t.Errorf("expected TimesSeenCount ~= %d (floor=3), got %d", expectedSeen, afterFgSeenCount)
	}

	// Now vehicle leaves - observation at 10m should match background
	avgBefore := g.Cells[idx].AverageRangeMeters
	mask, _ := bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})

	if mask[0] {
		t.Errorf("expected 10m observation to be classified as background after vehicle leaves")
	}

	avgAfter := g.Cells[idx].AverageRangeMeters
	t.Logf("Re-acquisition: avg before=%.3f, after=%.3f, delta=%.4f",
		avgBefore, avgAfter, avgAfter-avgBefore)

	// RecentForegroundCount should have decremented
	afterReacqFgCount := g.Cells[idx].RecentForegroundCount
	if afterReacqFgCount >= recentFgCount {
		t.Errorf("expected RecentForegroundCount to decrease after background observation, was %d now %d",
			recentFgCount, afterReacqFgCount)
	}
}

func TestProcessFramePolarWithMask_MinConfidenceFloor(t *testing.T) {
	// Test that MinConfidenceFloor prevents complete confidence drain
	g := makeTestGridStrict(2, 8)
	g.Params.MinConfidenceFloor = 5 // Higher floor for this test
	bm := g.Manager

	// Build up significant confidence
	for i := 0; i < 20; i++ {
		_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})
	}

	idx := g.Idx(0, 0)
	t.Logf("After 20 observations: TimesSeenCount=%d", g.Cells[idx].TimesSeenCount)

	// Hammer with foreground observations
	for i := 0; i < 50; i++ {
		_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 3.0}})
	}

	// TimesSeenCount should not drop below MinConfidenceFloor
	finalCount := g.Cells[idx].TimesSeenCount
	t.Logf("After 50 foreground observations: TimesSeenCount=%d", finalCount)

	if finalCount < 5 {
		t.Errorf("expected TimesSeenCount >= MinConfidenceFloor (5), got %d", finalCount)
	}

	// Average should be preserved at original value since we didn't update it
	if g.Cells[idx].AverageRangeMeters < 9.5 || g.Cells[idx].AverageRangeMeters > 10.5 {
		t.Errorf("expected AverageRangeMeters to be preserved around 10m, got %.2f", g.Cells[idx].AverageRangeMeters)
	}
}

func TestProcessFramePolarWithMask_ReacquisitionBoostCapped(t *testing.T) {
	// Test that boosted alpha is capped at 0.5 to prevent instability
	g := makeTestGridStrict(2, 8)
	g.Params.ReacquisitionBoostMultiplier = 100.0 // Extreme boost
	g.Params.BackgroundUpdateFraction = 0.2       // 20% base
	g.Params.MinConfidenceFloor = 0               // Allow full drain for this test
	bm := g.Manager

	// Initialise at 10m
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 10.0}})

	idx := g.Idx(0, 0)

	// Create foreground condition
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 3.0}})
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 3.0}})

	avgBefore := g.Cells[idx].AverageRangeMeters

	// Return to background - should use capped alpha of 0.5
	_, _ = bm.ProcessFramePolarWithMask([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 20.0}})

	avgAfter := g.Cells[idx].AverageRangeMeters
	delta := float64(avgAfter - avgBefore)

	// With alpha=0.5 and target=20, delta should be 0.5*(20-10)=5 max
	// (actual depends on current avg which may have drifted)
	t.Logf("Capped boost test: avgBefore=%.2f, avgAfter=%.2f, delta=%.2f", avgBefore, avgAfter, delta)

	// The key test is that we didn't jump all the way to 20m (which would be alpha=1.0)
	if avgAfter >= 20.0 {
		t.Errorf("expected boosted alpha to be capped, but avg jumped to %.2f", avgAfter)
	}
}

// TestProcessFramePolarWithMask_AcceptanceCounting verifies that per-range
// acceptance metrics are accumulated during ProcessFramePolarWithMask calls.
// This is the root cause fix for accept_rate=0 in auto-tune sweeps.
func TestProcessFramePolarWithMask_AcceptanceCounting(t *testing.T) {
	// Use NewBackgroundManager to get proper acceptance bucket initialisation
	bm := NewBackgroundManager("test-acceptance-counting", 4, 180, BackgroundParams{
		BackgroundUpdateFraction:       0.5,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.4,
		NoiseRelativeFraction:          0.01,
		SeedFromFirstObservation:       true,
		NeighborConfirmationCount:      5,
	}, nil)
	if bm == nil {
		t.Fatal("failed to create background manager")
	}
	// Clean up global registry after test
	defer func() {
		bgMgrRegistryMu.Lock()
		delete(bgMgrRegistry, "test-acceptance-counting")
		bgMgrRegistryMu.Unlock()
	}()

	// Verify acceptance buckets are initialised
	g := bm.Grid
	if len(g.AcceptanceBucketsMeters) == 0 {
		t.Fatal("AcceptanceBucketsMeters not initialised")
	}

	// Seed a point at 5m (channel 1 = ring 0, azimuth 0)
	seedPoints := []PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 5.0}}
	_, err := bm.ProcessFramePolarWithMask(seedPoints)
	if err != nil {
		t.Fatalf("seed frame failed: %v", err)
	}

	// Process several more frames with the same point (should be accepted as background)
	for i := 0; i < 10; i++ {
		_, err := bm.ProcessFramePolarWithMask(seedPoints)
		if err != nil {
			t.Fatalf("frame %d failed: %v", i, err)
		}
	}

	// Check that acceptance metrics are non-zero
	metrics := bm.GetAcceptanceMetrics()
	if metrics == nil {
		t.Fatal("GetAcceptanceMetrics returned nil")
	}

	var totalAccept, totalReject int64
	for i := range metrics.AcceptCounts {
		totalAccept += metrics.AcceptCounts[i]
		totalReject += metrics.RejectCounts[i]
	}

	total := totalAccept + totalReject
	if total == 0 {
		t.Fatal("expected non-zero acceptance metrics after processing frames, got 0 total")
	}

	// After seeding + 10 background frames, most should be accepted
	if totalAccept == 0 {
		t.Errorf("expected non-zero accept count, got 0 (reject=%d)", totalReject)
	}

	t.Logf("Acceptance counting: accept=%d, reject=%d, total=%d, rate=%.2f%%",
		totalAccept, totalReject, total, float64(totalAccept)/float64(total)*100)
}
