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

	// Initialize background at 10m for ring 0, azBin 0 using ProcessFramePolarWithMask
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

	// Initialize center and neighbors with 10m
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
