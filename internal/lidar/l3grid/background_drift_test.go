package l3grid

import (
	"testing"
)

// ---------------------------------------------------------------------------
// CheckForSensorMovement
// ---------------------------------------------------------------------------

func TestCheckForSensorMovement_NilManager(t *testing.T) {
	var bm *BackgroundManager
	if bm.CheckForSensorMovement([]bool{true}) {
		t.Error("expected false for nil manager")
	}
}

func TestCheckForSensorMovement_EmptyMask(t *testing.T) {
	g := makeTestGrid(1, 4)
	if g.Manager.CheckForSensorMovement(nil) {
		t.Error("expected false for nil mask")
	}
	if g.Manager.CheckForSensorMovement([]bool{}) {
		t.Error("expected false for empty mask")
	}
}

func TestCheckForSensorMovement_DefaultThreshold(t *testing.T) {
	// SensorMovementForegroundThreshold is zero → default 0.20
	g := makeTestGrid(1, 4)
	g.Params.SensorMovementForegroundThreshold = 0

	// 1/5 = 0.20 → NOT above threshold (must exceed, not equal)
	mask := []bool{true, false, false, false, false}
	if g.Manager.CheckForSensorMovement(mask) {
		t.Error("expected false when ratio equals default threshold")
	}

	// 2/5 = 0.40 → above 0.20
	mask = []bool{true, true, false, false, false}
	if !g.Manager.CheckForSensorMovement(mask) {
		t.Error("expected true when ratio exceeds default threshold")
	}
}

func TestCheckForSensorMovement_CustomThreshold(t *testing.T) {
	g := makeTestGrid(1, 4)
	g.Params.SensorMovementForegroundThreshold = 0.5

	// 4/10 = 0.40 → below 0.50
	mask := make([]bool, 10)
	for i := 0; i < 4; i++ {
		mask[i] = true
	}
	if g.Manager.CheckForSensorMovement(mask) {
		t.Error("expected false when ratio below custom threshold")
	}

	// 6/10 = 0.60 → above 0.50
	for i := 0; i < 6; i++ {
		mask[i] = true
	}
	if !g.Manager.CheckForSensorMovement(mask) {
		t.Error("expected true when ratio exceeds custom threshold")
	}
}

// ---------------------------------------------------------------------------
// CheckBackgroundDrift
// ---------------------------------------------------------------------------

func TestCheckBackgroundDrift_NilManager(t *testing.T) {
	var bm *BackgroundManager
	drifted, metrics := bm.CheckBackgroundDrift()
	if drifted {
		t.Error("expected false for nil manager")
	}
	if metrics.DriftingCells != 0 {
		t.Error("expected zero drift metrics")
	}
}

func TestCheckBackgroundDrift_NilGrid(t *testing.T) {
	bm := &BackgroundManager{Grid: nil}
	drifted, _ := bm.CheckBackgroundDrift()
	if drifted {
		t.Error("expected false for nil grid")
	}
}

func TestCheckBackgroundDrift_NoSettledCells(t *testing.T) {
	g := makeTestGrid(1, 4)
	// Cells have TimesSeenCount = 0 → no settled cells
	g.Params.LockedBaselineThreshold = 10

	drifted, metrics := g.Manager.CheckBackgroundDrift()
	if drifted {
		t.Error("expected false when no cells have settled")
	}
	if metrics.DriftingCells != 0 || metrics.DriftRatio != 0 {
		t.Error("expected zero drift metrics when no settled cells")
	}
}

func TestCheckBackgroundDrift_DefaultThresholds(t *testing.T) {
	g := makeTestGrid(1, 4)
	// Ensure drift/ratio thresholds default to their built-in values
	g.Params.BackgroundDriftThresholdMeters = 0
	g.Params.BackgroundDriftRatioThreshold = 0
	g.Params.LockedBaselineThreshold = 5

	// Settle all cells with a locked baseline
	for i := range g.Cells {
		g.Cells[i].TimesSeenCount = 10
		g.Cells[i].LockedBaseline = 5.0
		g.Cells[i].AverageRangeMeters = 5.0 // no drift
	}

	drifted, metrics := g.Manager.CheckBackgroundDrift()
	if drifted {
		t.Error("expected false with no drift")
	}
	if metrics.DriftRatio != 0 {
		t.Errorf("expected zero drift ratio, got %f", metrics.DriftRatio)
	}
}

func TestCheckBackgroundDrift_LockedBaselineZero(t *testing.T) {
	g := makeTestGrid(1, 4)
	g.Params.LockedBaselineThreshold = 5
	g.Params.BackgroundDriftThresholdMeters = 0.5
	g.Params.BackgroundDriftRatioThreshold = 0.10

	// Settled but LockedBaseline is 0 → skipped (continue branch)
	for i := range g.Cells {
		g.Cells[i].TimesSeenCount = 10
		g.Cells[i].LockedBaseline = 0
		g.Cells[i].AverageRangeMeters = 5.0
	}

	drifted, metrics := g.Manager.CheckBackgroundDrift()
	if drifted {
		t.Error("expected false when locked baselines are zero")
	}
	if metrics.DriftingCells != 0 {
		t.Error("expected zero drifting cells")
	}
}

func TestCheckBackgroundDrift_DriftDetected(t *testing.T) {
	g := makeTestGrid(1, 4)
	g.Params.LockedBaselineThreshold = 5
	g.Params.BackgroundDriftThresholdMeters = 0.5
	g.Params.BackgroundDriftRatioThreshold = 0.10

	// All 4 cells are settled with significant drift
	for i := range g.Cells {
		g.Cells[i].TimesSeenCount = 10
		g.Cells[i].LockedBaseline = 5.0
		g.Cells[i].AverageRangeMeters = 6.0 // drift = 1.0 > 0.5
	}

	drifted, metrics := g.Manager.CheckBackgroundDrift()
	if !drifted {
		t.Error("expected drift to be detected")
	}
	if metrics.DriftingCells != 4 {
		t.Errorf("expected 4 drifting cells, got %d", metrics.DriftingCells)
	}
	if metrics.DriftRatio != 1.0 {
		t.Errorf("expected drift ratio 1.0, got %f", metrics.DriftRatio)
	}
	if metrics.AverageDrift != 1.0 {
		t.Errorf("expected average drift 1.0, got %f", metrics.AverageDrift)
	}
}

// ---------------------------------------------------------------------------
// GenerateBackgroundSnapshot
// ---------------------------------------------------------------------------

func TestGenerateBackgroundSnapshot_NilManager(t *testing.T) {
	var bm *BackgroundManager
	snap, err := bm.GenerateBackgroundSnapshot()
	if err == nil {
		t.Error("expected error for nil manager")
	}
	if snap != nil {
		t.Error("expected nil snapshot")
	}
}

func TestGenerateBackgroundSnapshot_WithSnapshotID(t *testing.T) {
	g := makeTestGrid(1, 4)
	g.RingElevations = []float64{0.0}

	// Set a snapshot ID
	id := int64(42)
	g.SnapshotID = &id

	// Nothing settled → empty snapshot but no error
	snap, err := g.Manager.GenerateBackgroundSnapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.SequenceNumber != 42 {
		t.Errorf("expected sequence 42, got %d", snap.SequenceNumber)
	}
}

func TestGenerateBackgroundSnapshot_ZeroCellCount(t *testing.T) {
	g := makeTestGrid(1, 4)
	g.RingElevations = []float64{0.0}
	g.nonzeroCellCount = 0 // triggers default capacity branch

	snap, err := g.Manager.GenerateBackgroundSnapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
}

// ---------------------------------------------------------------------------
// GetBackgroundSequenceNumber
// ---------------------------------------------------------------------------

func TestGetBackgroundSequenceNumber_NilManager(t *testing.T) {
	var bm *BackgroundManager
	if bm.GetBackgroundSequenceNumber() != 0 {
		t.Error("expected 0 for nil manager")
	}
}

func TestGetBackgroundSequenceNumber_NilGrid(t *testing.T) {
	bm := &BackgroundManager{Grid: nil}
	if bm.GetBackgroundSequenceNumber() != 0 {
		t.Error("expected 0 for nil grid")
	}
}

func TestGetBackgroundSequenceNumber_WithSnapshotID(t *testing.T) {
	g := makeTestGrid(1, 4)
	id := int64(99)
	g.SnapshotID = &id

	seq := g.Manager.GetBackgroundSequenceNumber()
	if seq != 99 {
		t.Errorf("expected 99, got %d", seq)
	}
}

func TestGetBackgroundSequenceNumber_NilSnapshotID(t *testing.T) {
	g := makeTestGrid(1, 4)
	g.SnapshotID = nil

	seq := g.Manager.GetBackgroundSequenceNumber()
	if seq != 0 {
		t.Errorf("expected 0, got %d", seq)
	}
}
