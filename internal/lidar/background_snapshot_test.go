package lidar

import (
	"testing"
)

func TestBackgroundManager_GenerateBackgroundSnapshot(t *testing.T) {
	// Create a background manager with a simple grid
	params := BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		LockedBaselineThreshold:        10,
	}

	grid := &BackgroundGrid{
		Rings:       40,
		AzimuthBins: 1800,
		Cells:       make([]BackgroundCell, 40*1800),
		Params:      params,
		RingElevations: make([]float64, 40),
	}

	// Initialize ring elevations (-15° to +15° for Pandar40P-like)
	for i := 0; i < 40; i++ {
		grid.RingElevations[i] = -15.0 + float64(i)*0.75
	}

	// Add some settled cells with known values
	for ring := 0; ring < 5; ring++ {
		for azBin := 0; azBin < 100; azBin++ {
			idx := grid.Idx(ring, azBin)
			grid.Cells[idx] = BackgroundCell{
				AverageRangeMeters: float32(10.0 + float64(ring)),
				TimesSeenCount:     20, // Above threshold
			}
		}
	}

	mgr := &BackgroundManager{
		Grid: grid,
	}

	// Generate snapshot
	snapshot, err := mgr.GenerateBackgroundSnapshot()
	if err != nil {
		t.Fatalf("GenerateBackgroundSnapshot failed: %v", err)
	}

	if snapshot == nil {
		t.Fatal("Generated snapshot is nil")
	}

	// Check that we got points
	if len(snapshot.X) == 0 {
		t.Error("Expected non-zero point count in snapshot")
	}

	// Verify arrays have same length
	pointCount := len(snapshot.X)
	if len(snapshot.Y) != pointCount {
		t.Errorf("Y array length %d != X array length %d", len(snapshot.Y), pointCount)
	}
	if len(snapshot.Z) != pointCount {
		t.Errorf("Z array length %d != X array length %d", len(snapshot.Z), pointCount)
	}
	if len(snapshot.Confidence) != pointCount {
		t.Errorf("Confidence array length %d != X array length %d", len(snapshot.Confidence), pointCount)
	}

	// Verify grid metadata
	if snapshot.Rings != 40 {
		t.Errorf("Expected 40 rings, got %d", snapshot.Rings)
	}
	if snapshot.AzimuthBins != 1800 {
		t.Errorf("Expected 1800 azimuth bins, got %d", snapshot.AzimuthBins)
	}
	if len(snapshot.RingElevations) != 40 {
		t.Errorf("Expected 40 ring elevations, got %d", len(snapshot.RingElevations))
	}

	// Verify confidence values
	for i, conf := range snapshot.Confidence {
		if conf < 10 {
			t.Errorf("Point %d has confidence %d < threshold 10", i, conf)
		}
	}

	t.Logf("Generated snapshot with %d points", pointCount)
}

func TestBackgroundManager_GenerateBackgroundSnapshot_NoRingElevations(t *testing.T) {
	params := BackgroundParams{
		LockedBaselineThreshold: 10,
	}

	grid := &BackgroundGrid{
		Rings:          40,
		AzimuthBins:    1800,
		Cells:          make([]BackgroundCell, 40*1800),
		Params:         params,
		RingElevations: []float64{}, // Empty
	}

	mgr := &BackgroundManager{
		Grid: grid,
	}

	// Should fail due to missing ring elevations
	_, err := mgr.GenerateBackgroundSnapshot()
	if err == nil {
		t.Error("Expected error for missing ring elevations, got nil")
	}
}

func TestBackgroundManager_CheckForSensorMovement(t *testing.T) {
	mgr := &BackgroundManager{}

	// Test with low foreground ratio (< 20%)
	mask := make([]bool, 1000)
	for i := 0; i < 100; i++ {
		mask[i] = true // 10% foreground
	}
	if mgr.CheckForSensorMovement(mask) {
		t.Error("Expected no movement for 10% foreground ratio")
	}

	// Test with high foreground ratio (> 20%)
	mask = make([]bool, 1000)
	for i := 0; i < 300; i++ {
		mask[i] = true // 30% foreground
	}
	if !mgr.CheckForSensorMovement(mask) {
		t.Error("Expected movement detection for 30% foreground ratio")
	}
}

func TestBackgroundManager_CheckBackgroundDrift(t *testing.T) {
	params := BackgroundParams{
		LockedBaselineThreshold: 10,
	}

	grid := &BackgroundGrid{
		Rings:       10,
		AzimuthBins: 100,
		Cells:       make([]BackgroundCell, 10*100),
		Params:      params,
	}

	// Add settled cells with no drift
	for i := 0; i < 500; i++ {
		grid.Cells[i] = BackgroundCell{
			AverageRangeMeters: 10.0,
			LockedBaseline:     10.0,
			TimesSeenCount:     20,
		}
	}

	mgr := &BackgroundManager{
		Grid: grid,
	}

	// Should not detect drift
	drifted, metrics := mgr.CheckBackgroundDrift()
	if drifted {
		t.Error("Expected no drift for stable cells")
	}
	if metrics.DriftingCells != 0 {
		t.Errorf("Expected 0 drifting cells, got %d", metrics.DriftingCells)
	}

	// Add significant drift to >10% of cells
	for i := 0; i < 100; i++ {
		grid.Cells[i].AverageRangeMeters = 11.0 // 1.0m drift (> 0.5m threshold)
	}

	// Should detect drift
	drifted, metrics = mgr.CheckBackgroundDrift()
	if !drifted {
		t.Error("Expected drift detection for 20% drifted cells")
	}
	if metrics.DriftingCells == 0 {
		t.Error("Expected non-zero drifting cells")
	}

	t.Logf("Detected drift: %d cells, avg %.2fm, ratio %.2f",
		metrics.DriftingCells, metrics.AverageDrift, metrics.DriftRatio)
}

func TestBackgroundManager_GetBackgroundSequenceNumber(t *testing.T) {
	snapshotID := int64(42)
	grid := &BackgroundGrid{
		SnapshotID: &snapshotID,
	}

	mgr := &BackgroundManager{
		Grid: grid,
	}

	seq := mgr.GetBackgroundSequenceNumber()
	if seq != 42 {
		t.Errorf("Expected sequence number 42, got %d", seq)
	}

	// Test with nil snapshot ID
	grid.SnapshotID = nil
	seq = mgr.GetBackgroundSequenceNumber()
	if seq != 0 {
		t.Errorf("Expected sequence number 0 for nil snapshot ID, got %d", seq)
	}
}
