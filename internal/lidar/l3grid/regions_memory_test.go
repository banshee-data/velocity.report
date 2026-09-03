package l3grid

import (
	"runtime"
	"testing"
)

// fragmentGrid fills a grid so that connected-component identification produces
// one region per cell: neighbouring cells always land in different variance
// categories, so nothing merges during the component walk.
//
// This is the shape a freshly settled outdoor grid takes before merging, and
// the shape that made region identification allocate a full-grid mask for every
// one of thousands of throwaway components.
func fragmentGrid(grid *BackgroundGrid) {
	rm := grid.RegionMgr
	for i := range grid.Cells {
		grid.Cells[i].AverageRangeMeters = 10.0
		grid.Cells[i].TimesSeenCount = 10
		// Three variance bands in a repeating pattern. The 33rd/66th percentile
		// thresholds then split adjacent cells apart in both ring and azimuth.
		band := float64((i%3)+1) * 1.0
		grid.Cells[i].RangeSpreadMeters = float32(band)
		rm.SettlingMetrics.VariancePerCell[i] = band
	}
	rm.SettlingMetrics.FramesSampled = 20
}

// TestIdentifyRegionsBoundsMaskMemory pins the memory cost of region
// identification to the regions that survive merging.
//
// Every region carries a CellMask the size of the whole grid. Allocating one
// per intermediate component, and then dropping those components with a
// reslice that keeps the backing array — and every mask in it — reachable, cost
// ~915 MB on a 40x1800 production grid. Both halves of that are load-bearing
// here: the mask has to be built late, and the discarded slot has to be
// cleared, or this test fails.
func TestIdentifyRegionsBoundsMaskMemory(t *testing.T) {
	const (
		rings      = 40
		azBins     = 360
		maxRegions = 8
	)
	totalCells := rings * azBins

	grid := makeTestGrid(rings, azBins)
	fragmentGrid(grid)

	// Retained bytes are measured after a collection, so this is live heap, not
	// allocation churn. A mask per component would leave tens of megabytes
	// behind; the survivors account for a few hundred kilobytes.
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	if err := grid.RegionMgr.IdentifyRegions(grid, maxRegions); err != nil {
		t.Fatalf("IdentifyRegions: %v", err)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)

	// Masks for the survivors plus one cell-index entry per cell, with a wide
	// allowance for everything else the walk leaves behind. A mask per
	// component exceeds this by more than an order of magnitude.
	budget := int64(maxRegions*totalCells + totalCells*8)
	budget *= 4

	if retained > budget {
		t.Errorf("region identification retained %d bytes, budget %d (%d cells, %d regions kept): "+
			"a full-grid CellMask is being kept for regions the merge step discarded",
			retained, budget, totalCells, len(grid.RegionMgr.Regions))
	}

	if got := len(grid.RegionMgr.Regions); got > maxRegions {
		t.Errorf("expected at most %d regions after merging, got %d", maxRegions, got)
	}
}

// TestIdentifyRegionsMaterialisesSurvivorMasks verifies that deferring mask
// construction to the surviving regions still leaves every region with a mask
// that agrees with its cell list — including cells absorbed during merging.
func TestIdentifyRegionsMaterialisesSurvivorMasks(t *testing.T) {
	const (
		rings      = 8
		azBins     = 24
		maxRegions = 3
	)
	totalCells := rings * azBins

	grid := makeTestGrid(rings, azBins)
	fragmentGrid(grid)

	if err := grid.RegionMgr.IdentifyRegions(grid, maxRegions); err != nil {
		t.Fatalf("IdentifyRegions: %v", err)
	}

	regions := grid.RegionMgr.Regions
	if len(regions) == 0 {
		t.Fatal("expected at least one region")
	}

	seen := make([]bool, totalCells)
	for _, r := range regions {
		if len(r.CellMask) != totalCells {
			t.Fatalf("region %d: CellMask has %d entries, want %d", r.ID, len(r.CellMask), totalCells)
		}
		masked := 0
		for _, set := range r.CellMask {
			if set {
				masked++
			}
		}
		if masked != len(r.CellList) {
			t.Errorf("region %d: CellMask marks %d cells, CellList has %d", r.ID, masked, len(r.CellList))
		}
		for _, cellIdx := range r.CellList {
			if !r.CellMask[cellIdx] {
				t.Errorf("region %d: cell %d is in CellList but not marked in CellMask", r.ID, cellIdx)
			}
			if seen[cellIdx] {
				t.Errorf("cell %d appears in more than one region", cellIdx)
			}
			seen[cellIdx] = true
		}
	}

	// Every observed cell belongs to exactly one surviving region.
	for i := range grid.Cells {
		if grid.Cells[i].TimesSeenCount > 0 && !seen[i] {
			t.Errorf("cell %d has data but belongs to no region", i)
		}
	}
}
