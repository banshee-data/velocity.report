package lidar

import (
	"math"
	"testing"
)

// TestRegionManagerCreation verifies that RegionManager is correctly initialized
func TestRegionManagerCreation(t *testing.T) {
	rings := 4
	azBins := 10
	rm := NewRegionManager(rings, azBins)

	if rm == nil {
		t.Fatal("NewRegionManager returned nil")
	}

	expectedCells := rings * azBins
	if len(rm.CellToRegionID) != expectedCells {
		t.Errorf("Expected %d cells, got %d", expectedCells, len(rm.CellToRegionID))
	}

	if len(rm.SettlingMetrics.VariancePerCell) != expectedCells {
		t.Errorf("Expected %d variance cells, got %d", expectedCells, len(rm.SettlingMetrics.VariancePerCell))
	}

	if rm.IdentificationComplete {
		t.Error("Expected IdentificationComplete to be false initially")
	}
}

// TestUpdateVarianceMetrics verifies variance accumulation during settling
func TestUpdateVarianceMetrics(t *testing.T) {
	rings := 2
	azBins := 4
	rm := NewRegionManager(rings, azBins)

	// Create mock cells with different spreads
	cells := make([]BackgroundCell, rings*azBins)
	for i := range cells {
		cells[i].AverageRangeMeters = 10.0
		cells[i].RangeSpreadMeters = float32(i) * 0.1 // varying spread
		cells[i].TimesSeenCount = 5
	}

	// Update metrics multiple times
	for i := 0; i < 10; i++ {
		rm.UpdateVarianceMetrics(cells)
	}

	if rm.SettlingMetrics.FramesSampled != 10 {
		t.Errorf("Expected 10 frames sampled, got %d", rm.SettlingMetrics.FramesSampled)
	}

	// Verify that variance values are reasonable
	for i, v := range rm.SettlingMetrics.VariancePerCell {
		expectedVariance := float64(cells[i].RangeSpreadMeters)
		// Use approximate equality for floating point comparison
		if math.Abs(v-expectedVariance) > 0.0001 {
			t.Errorf("Cell %d: expected variance %.3f, got %.3f", i, expectedVariance, v)
		}
	}
}

// TestIdentifyRegionsWithUniformVariance tests region identification with uniform variance
func TestIdentifyRegionsWithUniformVariance(t *testing.T) {
	rings := 3
	azBins := 6
	grid := makeTestGrid(rings, azBins)
	rm := grid.RegionMgr

	// Initialize all cells with same variance
	for i := range grid.Cells {
		grid.Cells[i].AverageRangeMeters = 10.0
		grid.Cells[i].RangeSpreadMeters = 0.5
		grid.Cells[i].TimesSeenCount = 10
		rm.SettlingMetrics.VariancePerCell[i] = 0.5
	}
	rm.SettlingMetrics.FramesSampled = 20

	// Identify regions
	err := rm.IdentifyRegions(grid, 50)
	if err != nil {
		t.Fatalf("IdentifyRegions failed: %v", err)
	}

	if !rm.IdentificationComplete {
		t.Error("Expected IdentificationComplete to be true after identification")
	}

	// With uniform variance, we expect a single region or a few regions
	if len(rm.Regions) == 0 {
		t.Error("Expected at least one region")
	}

	// Verify all cells are assigned to some region
	for i, regionID := range rm.CellToRegionID {
		if grid.Cells[i].TimesSeenCount > 0 && regionID < 0 {
			t.Errorf("Cell %d with data is unassigned", i)
		}
	}
}

// TestIdentifyRegionsWithVariedVariance tests region identification with varied variance
func TestIdentifyRegionsWithVariedVariance(t *testing.T) {
	rings := 4
	azBins := 8
	grid := makeTestGrid(rings, azBins)
	rm := grid.RegionMgr

	// Create three zones: low, medium, high variance
	for ring := 0; ring < rings; ring++ {
		for azBin := 0; azBin < azBins; azBin++ {
			idx := grid.Idx(ring, azBin)
			grid.Cells[idx].TimesSeenCount = 10
			grid.Cells[idx].AverageRangeMeters = 10.0

			// Create distinct variance zones
			if ring == 0 {
				// Low variance zone
				grid.Cells[idx].RangeSpreadMeters = 0.1
				rm.SettlingMetrics.VariancePerCell[idx] = 0.1
			} else if ring < 3 {
				// Medium variance zone
				grid.Cells[idx].RangeSpreadMeters = 0.5
				rm.SettlingMetrics.VariancePerCell[idx] = 0.5
			} else {
				// High variance zone (trees/glass)
				grid.Cells[idx].RangeSpreadMeters = 1.5
				rm.SettlingMetrics.VariancePerCell[idx] = 1.5
			}
		}
	}
	rm.SettlingMetrics.FramesSampled = 20

	// Identify regions
	err := rm.IdentifyRegions(grid, 50)
	if err != nil {
		t.Fatalf("IdentifyRegions failed: %v", err)
	}

	if len(rm.Regions) == 0 {
		t.Fatal("Expected at least one region")
	}

	// Verify regions have distinct parameters based on variance
	lowVarianceRegions := 0
	highVarianceRegions := 0

	for _, region := range rm.Regions {
		if region.MeanVariance < 0.3 {
			lowVarianceRegions++
			// Low variance should have tighter noise tolerance
			if region.Params.NoiseRelativeFraction >= grid.Params.NoiseRelativeFraction {
				t.Errorf("Low variance region should have tighter noise tolerance, got %.4f vs base %.4f",
					region.Params.NoiseRelativeFraction, grid.Params.NoiseRelativeFraction)
			}
		} else if region.MeanVariance > 1.0 {
			highVarianceRegions++
			// High variance should have looser noise tolerance
			if region.Params.NoiseRelativeFraction <= grid.Params.NoiseRelativeFraction {
				t.Errorf("High variance region should have looser noise tolerance, got %.4f vs base %.4f",
					region.Params.NoiseRelativeFraction, grid.Params.NoiseRelativeFraction)
			}
		}
	}

	if lowVarianceRegions == 0 && highVarianceRegions == 0 {
		t.Error("Expected to find both low and high variance regions")
	}
}

// TestRegionMerging tests that regions are merged when exceeding max count
func TestRegionMerging(t *testing.T) {
	rings := 10
	azBins := 20
	grid := makeTestGrid(rings, azBins)
	rm := grid.RegionMgr

	// Create many small regions by alternating variance
	for ring := 0; ring < rings; ring++ {
		for azBin := 0; azBin < azBins; azBin++ {
			idx := grid.Idx(ring, azBin)
			grid.Cells[idx].TimesSeenCount = 10
			grid.Cells[idx].AverageRangeMeters = 10.0

			// Alternating variance to create many small regions
			if (ring+azBin)%2 == 0 {
				grid.Cells[idx].RangeSpreadMeters = 0.2
				rm.SettlingMetrics.VariancePerCell[idx] = 0.2
			} else {
				grid.Cells[idx].RangeSpreadMeters = 1.2
				rm.SettlingMetrics.VariancePerCell[idx] = 1.2
			}
		}
	}
	rm.SettlingMetrics.FramesSampled = 20

	maxRegions := 10
	err := rm.IdentifyRegions(grid, maxRegions)
	if err != nil {
		t.Fatalf("IdentifyRegions failed: %v", err)
	}

	if len(rm.Regions) > maxRegions {
		t.Errorf("Expected at most %d regions, got %d", maxRegions, len(rm.Regions))
	}

	// Verify all cells are still assigned
	for i, regionID := range rm.CellToRegionID {
		if grid.Cells[i].TimesSeenCount > 0 && regionID < 0 {
			t.Errorf("Cell %d with data is unassigned after merging", i)
		}
	}
}

// TestRegionParameterApplication tests that region-specific parameters are correctly applied
func TestRegionParameterApplication(t *testing.T) {
	rings := 3
	azBins := 6
	grid := makeTestGrid(rings, azBins)
	rm := grid.RegionMgr

	// Setup cells with high variance in one region
	for ring := 0; ring < rings; ring++ {
		for azBin := 0; azBin < azBins; azBin++ {
			idx := grid.Idx(ring, azBin)
			grid.Cells[idx].TimesSeenCount = 10
			grid.Cells[idx].AverageRangeMeters = 10.0

			if ring == 2 {
				// High variance region
				grid.Cells[idx].RangeSpreadMeters = 2.0
				rm.SettlingMetrics.VariancePerCell[idx] = 2.0
			} else {
				// Low variance region
				grid.Cells[idx].RangeSpreadMeters = 0.2
				rm.SettlingMetrics.VariancePerCell[idx] = 0.2
			}
		}
	}
	rm.SettlingMetrics.FramesSampled = 20

	err := rm.IdentifyRegions(grid, 50)
	if err != nil {
		t.Fatalf("IdentifyRegions failed: %v", err)
	}

	// Get region params for a high variance cell
	highVarianceCellIdx := grid.Idx(2, 0)
	highVarianceRegionID := rm.GetRegionForCell(highVarianceCellIdx)
	highVarianceParams := rm.GetRegionParams(highVarianceRegionID)

	if highVarianceParams == nil {
		t.Fatal("Expected to get region params for high variance cell")
	}

	// Verify high variance region has more relaxed parameters
	if highVarianceParams.NeighborConfirmationCount <= grid.Params.NeighborConfirmationCount {
		t.Errorf("High variance region should require more neighbor confirmation, got %d vs base %d",
			highVarianceParams.NeighborConfirmationCount, grid.Params.NeighborConfirmationCount)
	}
}

// TestGetRegionDebugInfo tests the export of region debug information
func TestGetRegionDebugInfo(t *testing.T) {
	rings := 3
	azBins := 6
	grid := makeTestGrid(rings, azBins)
	bm := grid.Manager
	rm := grid.RegionMgr

	// Setup and identify regions
	for i := range grid.Cells {
		grid.Cells[i].TimesSeenCount = 10
		grid.Cells[i].AverageRangeMeters = 10.0
		grid.Cells[i].RangeSpreadMeters = 0.5
		rm.SettlingMetrics.VariancePerCell[i] = 0.5
	}
	rm.SettlingMetrics.FramesSampled = 20

	err := rm.IdentifyRegions(grid, 50)
	if err != nil {
		t.Fatalf("IdentifyRegions failed: %v", err)
	}

	// Get debug info without cell details
	info := bm.GetRegionDebugInfo(false)
	if info == nil {
		t.Fatal("GetRegionDebugInfo returned nil")
	}

	if info.SensorID != grid.SensorID {
		t.Errorf("Expected SensorID %s, got %s", grid.SensorID, info.SensorID)
	}

	if !info.IdentificationComplete {
		t.Error("Expected IdentificationComplete to be true")
	}

	if info.RegionCount != len(rm.Regions) {
		t.Errorf("Expected %d regions, got %d", len(rm.Regions), info.RegionCount)
	}

	// Verify grid mapping is exported
	if len(info.GridMapping) != rings*azBins {
		t.Errorf("Expected grid mapping of size %d, got %d", rings*azBins, len(info.GridMapping))
	}

	// Get debug info with cell details
	infoWithCells := bm.GetRegionDebugInfo(true)
	if infoWithCells == nil {
		t.Fatal("GetRegionDebugInfo with cells returned nil")
	}

	// Verify cells are included
	for _, region := range infoWithCells.Regions {
		if region.CellCount > 0 && len(region.Cells) == 0 {
			t.Errorf("Region %d has CellCount=%d but no cells exported", region.ID, region.CellCount)
		}
	}
}

// TestRegionIdentificationDuringSettling tests that regions are identified when settling completes
func TestRegionIdentificationDuringSettling(t *testing.T) {
	rings := 3
	azBins := 6

	// Use NewBackgroundManager to properly initialize all fields
	bm := NewBackgroundManager("test-sensor", rings, azBins, BackgroundParams{
		BackgroundUpdateFraction:       0.5,
		ClosenessSensitivityMultiplier: 2.0,
		SafetyMarginMeters:             20.0,
		FreezeDurationNanos:            int64(1000000000),
		NeighborConfirmationCount:      2,
		NoiseRelativeFraction:          0.01,
		WarmupMinFrames:                5,
		WarmupDurationNanos:            0, // disable duration check
	}, nil)

	grid := bm.Grid

	// Process frames to trigger settling
	// Need to process enough frames to complete warmup (WarmupMinFrames + 1 extra for trigger)
	for i := 0; i < 11; i++ {
		points := make([]PointPolar, 0)
		for ring := 0; ring < rings; ring++ {
			for azBin := 0; azBin < azBins; azBin++ {
				az := float64(azBin) * 360.0 / float64(azBins)
				points = append(points, PointPolar{
					Channel:  ring + 1,
					Azimuth:  az,
					Distance: 10.0 + float64(ring)*0.5, // varying distance per ring
				})
			}
		}
		bm.ProcessFramePolar(points)
	}

	// Verify settling completed
	if !grid.SettlingComplete {
		t.Errorf("Expected settling to complete after 11 frames (target: %d, remaining: %d)",
			grid.Params.WarmupMinFrames, grid.WarmupFramesRemaining)
	}

	// Verify regions were identified
	if grid.RegionMgr == nil {
		t.Fatal("RegionMgr is nil")
	}

	if !grid.RegionMgr.IdentificationComplete {
		t.Errorf("Expected region identification to complete after settling (sampled frames: %d)",
			grid.RegionMgr.SettlingMetrics.FramesSampled)
	}

	if len(grid.RegionMgr.Regions) == 0 {
		t.Error("Expected at least one region to be identified")
	}
}
