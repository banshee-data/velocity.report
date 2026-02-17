package l3grid

import (
	"testing"
	"time"
)

// TestSetEnableDiagnostics tests toggling diagnostics
func TestSetEnableDiagnostics(t *testing.T) {
	bm := &BackgroundManager{
		EnableDiagnostics: false,
	}

	// Enable diagnostics
	bm.SetEnableDiagnostics(true)
	if !bm.EnableDiagnostics {
		t.Error("SetEnableDiagnostics(true) did not enable diagnostics")
	}

	// Disable diagnostics
	bm.SetEnableDiagnostics(false)
	if bm.EnableDiagnostics {
		t.Error("SetEnableDiagnostics(false) did not disable diagnostics")
	}

	// Test with nil BackgroundManager (should not panic)
	var nilBM *BackgroundManager
	nilBM.SetEnableDiagnostics(true)
}

// TestGetAcceptanceMetrics tests retrieving acceptance metrics
func TestGetAcceptanceMetrics(t *testing.T) {
	// Test with nil BackgroundManager
	var nilBM *BackgroundManager
	metrics := nilBM.GetAcceptanceMetrics()
	if metrics == nil {
		t.Fatal("GetAcceptanceMetrics() with nil manager returned nil")
	}
	if len(metrics.BucketsMeters) != 0 {
		t.Error("Nil manager should return empty metrics")
	}

	// Create a BackgroundManager with a grid
	grid := &BackgroundGrid{
		SensorID:                "test-sensor",
		Rings:                   16,
		AzimuthBins:             360,
		AcceptanceBucketsMeters: []float64{5.0, 10.0, 20.0, 40.0},
		AcceptByRangeBuckets:    []int64{100, 200, 300, 400},
		RejectByRangeBuckets:    []int64{10, 20, 30, 40},
	}
	grid.Cells = make([]BackgroundCell, grid.Rings*grid.AzimuthBins)

	bm := &BackgroundManager{
		Grid: grid,
	}

	metrics = bm.GetAcceptanceMetrics()

	// Verify metrics are copied correctly
	if len(metrics.BucketsMeters) != 4 {
		t.Errorf("len(BucketsMeters) = %d, want 4", len(metrics.BucketsMeters))
	}
	if len(metrics.AcceptCounts) != 4 {
		t.Errorf("len(AcceptCounts) = %d, want 4", len(metrics.AcceptCounts))
	}
	if len(metrics.RejectCounts) != 4 {
		t.Errorf("len(RejectCounts) = %d, want 4", len(metrics.RejectCounts))
	}

	// Verify values
	expectedBuckets := []float64{5.0, 10.0, 20.0, 40.0}
	expectedAccept := []int64{100, 200, 300, 400}
	expectedReject := []int64{10, 20, 30, 40}

	for i := 0; i < 4; i++ {
		if metrics.BucketsMeters[i] != expectedBuckets[i] {
			t.Errorf("BucketsMeters[%d] = %f, want %f", i, metrics.BucketsMeters[i], expectedBuckets[i])
		}
		if metrics.AcceptCounts[i] != expectedAccept[i] {
			t.Errorf("AcceptCounts[%d] = %d, want %d", i, metrics.AcceptCounts[i], expectedAccept[i])
		}
		if metrics.RejectCounts[i] != expectedReject[i] {
			t.Errorf("RejectCounts[%d] = %d, want %d", i, metrics.RejectCounts[i], expectedReject[i])
		}
	}

	// Verify that returned slices are copies (modifying them shouldn't affect original)
	metrics.AcceptCounts[0] = 9999
	if grid.AcceptByRangeBuckets[0] == 9999 {
		t.Error("Modifying returned metrics affected the original grid data")
	}
}

// TestGetAcceptanceMetrics_EmptyBuckets tests with empty acceptance buckets
func TestGetAcceptanceMetrics_EmptyBuckets(t *testing.T) {
	grid := &BackgroundGrid{
		SensorID:                "test-sensor",
		Rings:                   16,
		AzimuthBins:             360,
		AcceptanceBucketsMeters: []float64{},
	}
	grid.Cells = make([]BackgroundCell, grid.Rings*grid.AzimuthBins)

	bm := &BackgroundManager{
		Grid: grid,
	}

	metrics := bm.GetAcceptanceMetrics()

	if metrics == nil {
		t.Fatal("GetAcceptanceMetrics() returned nil")
	}
	if len(metrics.BucketsMeters) != 0 {
		t.Errorf("Expected empty BucketsMeters, got length %d", len(metrics.BucketsMeters))
	}
}

// TestResetAcceptanceMetrics tests resetting acceptance counters
func TestResetAcceptanceMetrics(t *testing.T) {
	// Test with nil BackgroundManager
	var nilBM *BackgroundManager
	err := nilBM.ResetAcceptanceMetrics()
	if err == nil {
		t.Error("Expected error for nil BackgroundManager")
	}

	// Create a BackgroundManager with populated metrics
	grid := &BackgroundGrid{
		SensorID:                "test-sensor",
		Rings:                   16,
		AzimuthBins:             360,
		AcceptanceBucketsMeters: []float64{5.0, 10.0, 20.0},
		AcceptByRangeBuckets:    []int64{100, 200, 300},
		RejectByRangeBuckets:    []int64{50, 60, 70},
	}
	grid.Cells = make([]BackgroundCell, grid.Rings*grid.AzimuthBins)

	bm := &BackgroundManager{
		Grid: grid,
	}

	// Reset metrics
	err = bm.ResetAcceptanceMetrics()
	if err != nil {
		t.Errorf("ResetAcceptanceMetrics() returned error: %v", err)
	}

	// Verify all counters are zero
	for i := 0; i < 3; i++ {
		if grid.AcceptByRangeBuckets[i] != 0 {
			t.Errorf("AcceptByRangeBuckets[%d] = %d, want 0", i, grid.AcceptByRangeBuckets[i])
		}
		if grid.RejectByRangeBuckets[i] != 0 {
			t.Errorf("RejectByRangeBuckets[%d] = %d, want 0", i, grid.RejectByRangeBuckets[i])
		}
	}

	// Verify buckets array is preserved
	if len(grid.AcceptanceBucketsMeters) != 3 {
		t.Errorf("AcceptanceBucketsMeters length changed: %d", len(grid.AcceptanceBucketsMeters))
	}
}

// TestResetAcceptanceMetrics_ReinitializeSlices tests slice reinitialization
func TestResetAcceptanceMetrics_ReinitializeSlices(t *testing.T) {
	grid := &BackgroundGrid{
		SensorID:                "test-sensor",
		Rings:                   16,
		AzimuthBins:             360,
		AcceptanceBucketsMeters: []float64{5.0, 10.0},
		AcceptByRangeBuckets:    []int64{},    // Wrong length
		RejectByRangeBuckets:    []int64{100}, // Wrong length
	}
	grid.Cells = make([]BackgroundCell, grid.Rings*grid.AzimuthBins)

	bm := &BackgroundManager{
		Grid: grid,
	}

	err := bm.ResetAcceptanceMetrics()
	if err != nil {
		t.Errorf("ResetAcceptanceMetrics() returned error: %v", err)
	}

	// Verify slices were reinitialized with correct length
	if len(grid.AcceptByRangeBuckets) != 2 {
		t.Errorf("AcceptByRangeBuckets length = %d, want 2", len(grid.AcceptByRangeBuckets))
	}
	if len(grid.RejectByRangeBuckets) != 2 {
		t.Errorf("RejectByRangeBuckets length = %d, want 2", len(grid.RejectByRangeBuckets))
	}
}

// TestGridStatus tests retrieving grid status
func TestGridStatus(t *testing.T) {
	// Test with nil BackgroundManager
	var nilBM *BackgroundManager
	status := nilBM.GridStatus()
	if status != nil {
		t.Error("Expected nil status for nil BackgroundManager")
	}

	// Create a BackgroundManager with some cells
	grid := &BackgroundGrid{
		Rings:                   4,
		AzimuthBins:             8,
		nonzeroCellCount:        20,
		AcceptanceBucketsMeters: []float64{10.0, 20.0},
		AcceptByRangeBuckets:    []int64{50, 100},
		RejectByRangeBuckets:    []int64{5, 10},
	}
	numCells := grid.Rings * grid.AzimuthBins
	grid.Cells = make([]BackgroundCell, numCells)

	// Populate some cells with different states
	now := time.Now().UnixNano()
	settleTime := now - int64(10*time.Second)

	for i := 0; i < numCells; i++ {
		if i%4 == 0 {
			// Frozen cell
			grid.Cells[i].TimesSeenCount = 100
			grid.Cells[i].FrozenUntilUnixNanos = now + int64(5*time.Second)
		} else if i%4 == 1 {
			// Settled cell
			grid.Cells[i].TimesSeenCount = 50
			grid.Cells[i].LastUpdateUnixNanos = settleTime
		} else if i%4 == 2 {
			// Recently updated cell
			grid.Cells[i].TimesSeenCount = 10
			grid.Cells[i].LastUpdateUnixNanos = now
		}
		// else: empty cell (TimesSeenCount = 0)
	}

	bm := &BackgroundManager{
		Grid: grid,
	}

	status = bm.GridStatus()

	if status == nil {
		t.Fatal("GridStatus() returned nil")
	}

	// Verify basic structure
	if _, ok := status["total_cells"]; !ok {
		t.Error("Status missing 'total_cells'")
	}
	if _, ok := status["frozen_cells"]; !ok {
		t.Error("Status missing 'frozen_cells'")
	}
	if _, ok := status["times_seen_dist"]; !ok {
		t.Error("Status missing 'times_seen_dist'")
	}

	totalCells, ok := status["total_cells"].(int)
	if !ok || totalCells != numCells {
		t.Errorf("total_cells = %v, want %d", status["total_cells"], numCells)
	}
}

// TestResetGrid tests resetting the grid
func TestResetGrid(t *testing.T) {
	// Test with nil BackgroundManager
	var nilBM *BackgroundManager
	err := nilBM.ResetGrid()
	if err == nil {
		t.Error("Expected error for nil BackgroundManager")
	}

	// Create a BackgroundManager with populated grid
	grid := &BackgroundGrid{
		Rings:                   4,
		AzimuthBins:             8,
		AcceptanceBucketsMeters: []float64{10.0},
		AcceptByRangeBuckets:    []int64{100},
		RejectByRangeBuckets:    []int64{50},
	}
	numCells := grid.Rings * grid.AzimuthBins
	grid.Cells = make([]BackgroundCell, numCells)

	now := time.Now().UnixNano()

	// Populate cells with data
	for i := 0; i < numCells; i++ {
		grid.Cells[i].AverageRangeMeters = float32(i) * 1.5
		grid.Cells[i].RangeSpreadMeters = 0.5
		grid.Cells[i].TimesSeenCount = uint32(i * 10)
		grid.Cells[i].LastUpdateUnixNanos = now
		grid.Cells[i].FrozenUntilUnixNanos = now + int64(time.Second)
		grid.Cells[i].RecentForegroundCount = 5
		grid.Cells[i].LockedBaseline = 10.0
		grid.Cells[i].LockedSpread = 0.3
		grid.Cells[i].LockedAtCount = 100
	}

	bm := &BackgroundManager{
		Grid:       grid,
		StartTime:  time.Now(),
		HasSettled: true,
	}

	// Reset the grid
	err = bm.ResetGrid()
	if err != nil {
		t.Errorf("ResetGrid() returned error: %v", err)
	}

	// Verify BackgroundManager state was reset
	if !bm.StartTime.IsZero() {
		t.Error("StartTime was not reset to zero")
	}
	if bm.HasSettled {
		t.Error("HasSettled should be false after reset")
	}

	// Verify all cells were reset
	for i := 0; i < numCells; i++ {
		cell := grid.Cells[i]
		if cell.AverageRangeMeters != 0 {
			t.Errorf("Cell %d AverageRangeMeters = %f, want 0", i, cell.AverageRangeMeters)
		}
		if cell.RangeSpreadMeters != 0 {
			t.Errorf("Cell %d RangeSpreadMeters = %f, want 0", i, cell.RangeSpreadMeters)
		}
		if cell.TimesSeenCount != 0 {
			t.Errorf("Cell %d TimesSeenCount = %d, want 0", i, cell.TimesSeenCount)
		}
		if cell.LastUpdateUnixNanos != 0 {
			t.Errorf("Cell %d LastUpdateUnixNanos = %d, want 0", i, cell.LastUpdateUnixNanos)
		}
		if cell.FrozenUntilUnixNanos != 0 {
			t.Errorf("Cell %d FrozenUntilUnixNanos = %d, want 0", i, cell.FrozenUntilUnixNanos)
		}
		if cell.RecentForegroundCount != 0 {
			t.Errorf("Cell %d RecentForegroundCount = %d, want 0", i, cell.RecentForegroundCount)
		}
		if cell.LockedBaseline != 0 {
			t.Errorf("Cell %d LockedBaseline = %f, want 0", i, cell.LockedBaseline)
		}
		if cell.LockedSpread != 0 {
			t.Errorf("Cell %d LockedSpread = %f, want 0", i, cell.LockedSpread)
		}
		if cell.LockedAtCount != 0 {
			t.Errorf("Cell %d LockedAtCount = %d, want 0", i, cell.LockedAtCount)
		}
	}

	// Verify acceptance counters were reset
	if grid.AcceptByRangeBuckets[0] != 0 {
		t.Errorf("AcceptByRangeBuckets not reset: %d", grid.AcceptByRangeBuckets[0])
	}
	if grid.RejectByRangeBuckets[0] != 0 {
		t.Errorf("RejectByRangeBuckets not reset: %d", grid.RejectByRangeBuckets[0])
	}
}

// TestGetGridCells tests retrieving all non-empty grid cells
func TestGetGridCells(t *testing.T) {
	// Test with nil BackgroundManager
	var nilBM *BackgroundManager
	cells := nilBM.GetGridCells()
	if cells != nil {
		t.Error("Expected nil for nil BackgroundManager")
	}

	// Create a BackgroundManager with some populated cells
	grid := &BackgroundGrid{
		Rings:       4,
		AzimuthBins: 8,
	}
	numCells := grid.Rings * grid.AzimuthBins
	grid.Cells = make([]BackgroundCell, numCells)

	now := time.Now().UnixNano()

	// Populate every other cell
	nonEmptyCount := 0
	for i := 0; i < numCells; i++ {
		if i%2 == 0 {
			grid.Cells[i].TimesSeenCount = uint32(i + 1)
			grid.Cells[i].AverageRangeMeters = float32(i) * 2.0
			grid.Cells[i].RangeSpreadMeters = 0.5
			grid.Cells[i].LastUpdateUnixNanos = now
			grid.Cells[i].FrozenUntilUnixNanos = now + int64(time.Second)
			nonEmptyCount++
		}
	}

	bm := &BackgroundManager{
		Grid: grid,
	}

	cells = bm.GetGridCells()

	if len(cells) != nonEmptyCount {
		t.Errorf("GetGridCells() returned %d cells, want %d", len(cells), nonEmptyCount)
	}

	// Verify cell data
	for _, cell := range cells {
		if cell.TimesSeen == 0 && cell.Range == 0 {
			t.Error("Found empty cell in result")
		}

		// Verify ring and azimuth are within valid ranges
		if cell.Ring < 0 || cell.Ring >= grid.Rings {
			t.Errorf("Invalid ring: %d", cell.Ring)
		}
		if cell.AzimuthDeg < 0 || cell.AzimuthDeg >= 360 {
			t.Errorf("Invalid azimuth: %f", cell.AzimuthDeg)
		}
	}
}

// TestGetGridCells_EmptyGrid tests with a completely empty grid
func TestGetGridCells_EmptyGrid(t *testing.T) {
	grid := &BackgroundGrid{
		SensorID:    "test-sensor",
		Rings:       2,
		AzimuthBins: 4,
	}
	grid.Cells = make([]BackgroundCell, grid.Rings*grid.AzimuthBins)

	bm := &BackgroundManager{
		Grid: grid,
	}

	cells := bm.GetGridCells()

	if len(cells) != 0 {
		t.Errorf("Expected 0 cells from empty grid, got %d", len(cells))
	}
}

// TestGetGridCells_AzimuthCalculation tests correct azimuth degree calculation
func TestGetGridCells_AzimuthCalculation(t *testing.T) {
	grid := &BackgroundGrid{
		SensorID:    "test-sensor",
		Rings:       1,
		AzimuthBins: 360, // 1 degree per bin
	}
	grid.Cells = make([]BackgroundCell, grid.Rings*grid.AzimuthBins)

	// Set specific cells
	testBins := []int{0, 90, 180, 270, 359}
	for _, bin := range testBins {
		grid.Cells[bin].TimesSeenCount = 1
		grid.Cells[bin].AverageRangeMeters = 10.0
	}

	bm := &BackgroundManager{
		Grid: grid,
	}

	cells := bm.GetGridCells()

	if len(cells) != len(testBins) {
		t.Fatalf("Expected %d cells, got %d", len(testBins), len(cells))
	}

	// Verify azimuth calculations
	expectedAzimuths := map[float32]bool{
		0.0:   true,
		90.0:  true,
		180.0: true,
		270.0: true,
		359.0: true,
	}

	for _, cell := range cells {
		if !expectedAzimuths[cell.AzimuthDeg] {
			t.Errorf("Unexpected azimuth: %f", cell.AzimuthDeg)
		}
	}
}

// TestBackgroundManager_NilSafety tests that all functions handle nil gracefully
func TestBackgroundManager_NilSafety(t *testing.T) {
	var nilBM *BackgroundManager

	// These should not panic
	nilBM.SetEnableDiagnostics(true)

	metrics := nilBM.GetAcceptanceMetrics()
	if metrics == nil {
		t.Error("GetAcceptanceMetrics should return empty metrics, not nil")
	}

	err := nilBM.ResetAcceptanceMetrics()
	if err == nil {
		t.Error("Expected error from ResetAcceptanceMetrics with nil manager")
	}

	status := nilBM.GridStatus()
	if status != nil {
		t.Error("GridStatus should return nil for nil manager")
	}

	err = nilBM.ResetGrid()
	if err == nil {
		t.Error("Expected error from ResetGrid with nil manager")
	}

	cells := nilBM.GetGridCells()
	if cells != nil {
		t.Error("GetGridCells should return nil for nil manager")
	}
}

// Helper function to create a test BackgroundManager
func createTestBackgroundManager(rings, azBins int) *BackgroundManager {
	grid := &BackgroundGrid{
		SensorID:                "test-sensor",
		Rings:                   rings,
		AzimuthBins:             azBins,
		AcceptanceBucketsMeters: []float64{10.0, 20.0},
		AcceptByRangeBuckets:    []int64{0, 0},
		RejectByRangeBuckets:    []int64{0, 0},
		Params: BackgroundParams{
			WarmupDurationNanos: int64(5 * time.Second),
		},
	}
	grid.Cells = make([]BackgroundCell, rings*azBins)

	return &BackgroundManager{
		Grid: grid,
	}
}

// Benchmark for GetGridCells with large grid
func BenchmarkGetGridCells(b *testing.B) {
	bm := createTestBackgroundManager(40, 3600) // Typical LiDAR dimensions

	// Populate 50% of cells
	for i := range bm.Grid.Cells {
		if i%2 == 0 {
			bm.Grid.Cells[i].TimesSeenCount = 100
			bm.Grid.Cells[i].AverageRangeMeters = 15.0
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bm.GetGridCells()
	}
}

// TestNewBackgroundManager_EdgeCases tests edge cases for NewBackgroundManager
func TestNewBackgroundManager_EdgeCases(t *testing.T) {
	// Empty sensorID should return nil
	mgr := NewBackgroundManager("", 10, 360, BackgroundParams{}, nil)
	if mgr != nil {
		t.Error("Expected nil for empty sensorID")
	}

	// Zero rings should return nil
	mgr = NewBackgroundManager("test-zero-rings", 0, 360, BackgroundParams{}, nil)
	if mgr != nil {
		t.Error("Expected nil for zero rings")
	}

	// Zero azBins should return nil
	mgr = NewBackgroundManager("test-zero-az", 10, 0, BackgroundParams{}, nil)
	if mgr != nil {
		t.Error("Expected nil for zero azBins")
	}

	// Valid parameters should return non-nil
	sensorID := "test-valid-" + time.Now().Format("150405")
	mgr = NewBackgroundManager(sensorID, 10, 360, BackgroundParams{}, nil)
	if mgr == nil {
		t.Error("Expected non-nil for valid parameters")
	}
	// Cleanup: deregister the manager to avoid leaking global state
	defer RegisterBackgroundManager(sensorID, nil)
}

// TestSetRingElevations_EdgeCases tests edge cases for SetRingElevations
func TestSetRingElevations_EdgeCases(t *testing.T) {
	// Test with nil BackgroundManager
	var nilBM *BackgroundManager
	err := nilBM.SetRingElevations([]float64{1.0, 2.0})
	if err == nil {
		t.Error("Expected error for nil BackgroundManager")
	}

	// Test with nil grid
	bm := &BackgroundManager{Grid: nil}
	err = bm.SetRingElevations([]float64{1.0, 2.0})
	if err == nil {
		t.Error("Expected error for nil Grid")
	}

	// Test with nil elevations (should clear)
	grid := &BackgroundGrid{Rings: 3, AzimuthBins: 10}
	grid.RingElevations = []float64{1.0, 2.0, 3.0}
	bm = &BackgroundManager{Grid: grid}
	err = bm.SetRingElevations(nil)
	if err != nil {
		t.Errorf("Unexpected error for nil elevations: %v", err)
	}
	if bm.Grid.RingElevations != nil {
		t.Error("Expected RingElevations to be nil after setting nil")
	}

	// Test with wrong length
	err = bm.SetRingElevations([]float64{1.0, 2.0})
	if err == nil {
		t.Error("Expected error for wrong length elevations")
	}

	// Test with correct length
	err = bm.SetRingElevations([]float64{1.0, 2.0, 3.0})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestGetRegionForCell_ViaRegionManager tests GetRegionForCell through RegionManager
func TestGetRegionForCell_ViaRegionManager(t *testing.T) {
	rm := NewRegionManager(4, 8)

	// Test with cell index out of range
	region := rm.GetRegionForCell(-1)
	if region != -1 {
		t.Error("Expected -1 for negative index")
	}

	region = rm.GetRegionForCell(100)
	if region != -1 {
		t.Error("Expected -1 for out-of-range index")
	}

	// Test with valid index
	region = rm.GetRegionForCell(0)
	t.Logf("GetRegionForCell(0) returned: %v", region)
}
