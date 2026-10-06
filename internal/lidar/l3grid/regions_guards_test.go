package l3grid

import (
	"math"
	"testing"
)

// TestUpdateVarianceMetricsStopsAfterIdentification checks that variance
// sampling stops once regions are fixed. Continuing to accumulate would move
// the percentile thresholds under regions that have already been assigned from
// them.
func TestUpdateVarianceMetricsStopsAfterIdentification(t *testing.T) {
	rm := NewRegionManager(2, 4)
	cells := make([]BackgroundCell, 8)
	for i := range cells {
		cells[i].TimesSeenCount = 5
		cells[i].RangeSpreadMeters = 0.5
	}

	rm.UpdateVarianceMetrics(cells)
	if rm.SettlingMetrics.FramesSampled != 1 {
		t.Fatalf("expected 1 frame sampled, got %d", rm.SettlingMetrics.FramesSampled)
	}

	rm.IdentificationComplete = true
	rm.UpdateVarianceMetrics(cells)

	if rm.SettlingMetrics.FramesSampled != 1 {
		t.Errorf("expected sampling to stop after identification, got %d frames",
			rm.SettlingMetrics.FramesSampled)
	}
}

// TestIdentifyRegionsRejectsSecondCall covers the guard against re-identifying.
// Regions are fixed once settling completes; a second pass would renumber them
// underneath the CellToRegionID mapping callers already hold.
func TestIdentifyRegionsRejectsSecondCall(t *testing.T) {
	grid := makeTestGrid(4, 8)
	fragmentGrid(grid)

	if err := grid.RegionMgr.IdentifyRegions(grid, 4); err != nil {
		t.Fatalf("first IdentifyRegions: %v", err)
	}

	err := grid.RegionMgr.IdentifyRegions(grid, 4)
	if err == nil {
		t.Fatal("expected the second call to be rejected")
	}
	if got := err.Error(); got != "regions already identified" {
		t.Errorf("unexpected error %q", got)
	}
}

// TestIdentifyRegionsRejectsVarianceSizeMismatch covers the guard against a
// variance array that does not describe this grid. Indexing cells by a
// mismatched array would either panic or silently classify against another
// grid's statistics.
func TestIdentifyRegionsRejectsVarianceSizeMismatch(t *testing.T) {
	grid := makeTestGrid(4, 8)
	fragmentGrid(grid)

	// Shrink the variance array so it no longer matches the cell count.
	grid.RegionMgr.SettlingMetrics.VariancePerCell = make([]float64, 3)

	err := grid.RegionMgr.IdentifyRegions(grid, 4)
	if err == nil {
		t.Fatal("expected a size mismatch to be rejected")
	}
	if got := err.Error(); got != "variance metrics size mismatch" {
		t.Errorf("unexpected error %q", got)
	}
}

// TestAssignRegionParamsUnknownCategory covers the fallback for a variance
// category outside the three the classifier produces. It returns the grid's own
// parameters unmodified, so an unrecognised category degrades to "treat this
// region like everywhere else" rather than to a zero-valued config.
func TestAssignRegionParamsUnknownCategory(t *testing.T) {
	rm := NewRegionManager(2, 2)
	base := BackgroundParams{
		NoiseRelativeFraction:      0.02,
		NeighbourConfirmationCount: 4,
		BackgroundUpdateFraction:   0.05,
	}

	got := rm.assignRegionParams(99, base)

	if got.NoiseRelativeFraction != base.NoiseRelativeFraction {
		t.Errorf("NoiseRelativeFraction = %v, want the base value %v",
			got.NoiseRelativeFraction, base.NoiseRelativeFraction)
	}
	if got.NeighbourConfirmationCount != base.NeighbourConfirmationCount {
		t.Errorf("NeighbourConfirmationCount = %v, want the base value %v",
			got.NeighbourConfirmationCount, base.NeighbourConfirmationCount)
	}
	if got.SettleUpdateFraction != base.BackgroundUpdateFraction {
		t.Errorf("SettleUpdateFraction = %v, want the base alpha %v",
			got.SettleUpdateFraction, base.BackgroundUpdateFraction)
	}
}

// TestToSnapshotReturnsNilOnUnmarshallableRegion covers the marshal failure
// path. A non-finite variance cannot be represented in JSON, and the snapshot
// must be abandoned rather than persisted half-formed.
func TestToSnapshotReturnsNilOnUnmarshallableRegion(t *testing.T) {
	grid := makeTestGrid(4, 8)
	fragmentGrid(grid)
	rm := grid.RegionMgr

	if err := rm.IdentifyRegions(grid, 4); err != nil {
		t.Fatalf("IdentifyRegions: %v", err)
	}
	if len(rm.Regions) == 0 {
		t.Fatal("expected at least one region")
	}

	// Infinity has no JSON representation, so encoding the region set fails.
	rm.Regions[0].MeanVariance = math.Inf(1)

	if snap := rm.ToSnapshot("test-sensor", 1); snap != nil {
		t.Error("expected a nil snapshot when the region set cannot be marshalled")
	}
}

// TestMaterialiseCellMasksSkipsNilRegions covers the nil guard. The merge step
// clears slots as it drops regions, so a nil can reach this function if the
// slice is ever passed before compaction.
func TestMaterialiseCellMasksSkipsNilRegions(t *testing.T) {
	const totalCells = 16
	regions := []*Region{
		nil,
		{ID: 1, CellList: []int{2, 5, 9}},
		nil,
	}

	materialiseCellMasks(regions, totalCells)

	r := regions[1]
	if len(r.CellMask) != totalCells {
		t.Fatalf("CellMask has %d entries, want %d", len(r.CellMask), totalCells)
	}
	for _, idx := range r.CellList {
		if !r.CellMask[idx] {
			t.Errorf("cell %d in CellList is not marked in CellMask", idx)
		}
	}
	marked := 0
	for _, set := range r.CellMask {
		if set {
			marked++
		}
	}
	if marked != len(r.CellList) {
		t.Errorf("CellMask marks %d cells, CellList has %d", marked, len(r.CellList))
	}
}

// TestMaterialiseCellMasksIgnoresOutOfRangeCells checks the bounds guard. A
// snapshot restored against a grid of a different size would otherwise index
// past the mask.
func TestMaterialiseCellMasksIgnoresOutOfRangeCells(t *testing.T) {
	const totalCells = 8
	regions := []*Region{{ID: 0, CellList: []int{-1, 3, 99}}}

	materialiseCellMasks(regions, totalCells)

	mask := regions[0].CellMask
	if len(mask) != totalCells {
		t.Fatalf("CellMask has %d entries, want %d", len(mask), totalCells)
	}
	if !mask[3] {
		t.Error("in-range cell 3 should be marked")
	}
	marked := 0
	for _, set := range mask {
		if set {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("expected only the in-range cell to be marked, got %d", marked)
	}
}
