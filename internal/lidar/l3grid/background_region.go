package l3grid

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// NewRegionManager creates a RegionManager for the grid
func NewRegionManager(rings, azBins int) *RegionManager {
	totalCells := rings * azBins
	return &RegionManager{
		Regions:        make([]*Region, 0),
		CellToRegionID: make([]int, totalCells),
		SettlingMetrics: struct {
			VariancePerCell []float64
			FramesSampled   int
		}{
			VariancePerCell: make([]float64, totalCells),
			FramesSampled:   0,
		},
		IdentificationComplete: false,
	}
}

// UpdateVarianceMetrics accumulates variance data during settling
// Computes running mean of RangeSpreadMeters values per cell
func (rm *RegionManager) UpdateVarianceMetrics(cells []BackgroundCell) {
	if rm.IdentificationComplete {
		return
	}
	n := float64(rm.SettlingMetrics.FramesSampled)
	for i, cell := range cells {
		if cell.TimesSeenCount > 0 {
			// Incremental mean calculation: mean_new = mean_old + (x - mean_old) / (n + 1)
			variance := float64(cell.RangeSpreadMeters)
			if n == 0 {
				rm.SettlingMetrics.VariancePerCell[i] = variance
			} else {
				// Update running mean of spread values
				oldMean := rm.SettlingMetrics.VariancePerCell[i]
				rm.SettlingMetrics.VariancePerCell[i] = oldMean + (variance-oldMean)/(n+1)
			}
		}
	}
	rm.SettlingMetrics.FramesSampled++
}

// IdentifyRegions performs clustering based on variance characteristics
// and creates contiguous regions with distinct parameters
func (rm *RegionManager) IdentifyRegions(grid *BackgroundGrid, maxRegions int) error {
	if rm.IdentificationComplete {
		return fmt.Errorf("regions already identified")
	}

	totalCells := len(grid.Cells)
	if len(rm.SettlingMetrics.VariancePerCell) != totalCells {
		return fmt.Errorf("variance metrics size mismatch")
	}

	// Initialize all cells to unassigned
	for i := range rm.CellToRegionID {
		rm.CellToRegionID[i] = -1
	}

	// Step 1: Classify cells by variance into categories
	// Calculate variance percentiles for classification
	variances := make([]float64, 0, totalCells)
	for i, v := range rm.SettlingMetrics.VariancePerCell {
		if grid.Cells[i].TimesSeenCount > 0 {
			variances = append(variances, v)
		}
	}

	if len(variances) == 0 {
		// No data collected, create single default region
		return rm.createDefaultRegion(grid)
	}

	// Sort variances to find thresholds
	sortedVars := make([]float64, len(variances))
	copy(sortedVars, variances)
	sort.Float64s(sortedVars)

	// Define thresholds: 33rd and 66th percentiles
	lowThreshold := sortedVars[len(sortedVars)/3]
	highThreshold := sortedVars[2*len(sortedVars)/3]

	// Step 2: Assign variance category to each cell
	cellCategory := make([]int, totalCells) // 0=stable, 1=variable, 2=volatile, -1=empty
	for i := range cellCategory {
		cellCategory[i] = -1
		if grid.Cells[i].TimesSeenCount > 0 {
			v := rm.SettlingMetrics.VariancePerCell[i]
			if v < lowThreshold {
				cellCategory[i] = 0 // stable
			} else if v < highThreshold {
				cellCategory[i] = 1 // variable
			} else {
				cellCategory[i] = 2 // volatile (trees, glass)
			}
		}
	}

	// Step 3: Use connected components to find contiguous regions
	visited := make([]bool, totalCells)
	regionID := 0
	tempRegions := make([]*Region, 0)

	for cellIdx := 0; cellIdx < totalCells; cellIdx++ {
		if visited[cellIdx] || cellCategory[cellIdx] == -1 {
			continue
		}

		// BFS to find connected component
		category := cellCategory[cellIdx]
		queue := []int{cellIdx}
		visited[cellIdx] = true
		regionCells := []int{cellIdx}

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			// Get neighbours (same ring ± 1 az, same az ± 1 ring)
			ring := current / grid.AzimuthBins
			azBin := current % grid.AzimuthBins

			neighbours := []struct{ r, az int }{
				{ring, (azBin + 1) % grid.AzimuthBins},
				{ring, (azBin - 1 + grid.AzimuthBins) % grid.AzimuthBins},
			}
			if ring > 0 {
				neighbours = append(neighbours, struct{ r, az int }{ring - 1, azBin})
			}
			if ring < grid.Rings-1 {
				neighbours = append(neighbours, struct{ r, az int }{ring + 1, azBin})
			}

			for _, n := range neighbours {
				nIdx := grid.Idx(n.r, n.az)
				if !visited[nIdx] && cellCategory[nIdx] == category {
					visited[nIdx] = true
					queue = append(queue, nIdx)
					regionCells = append(regionCells, nIdx)
				}
			}
		}

		// Create region for this connected component
		if len(regionCells) > 0 {
			region := &Region{
				ID:        regionID,
				CellMask:  make([]bool, totalCells),
				CellList:  regionCells,
				CellCount: len(regionCells),
			}

			// Calculate mean variance for this region
			sumVariance := 0.0
			for _, cIdx := range regionCells {
				region.CellMask[cIdx] = true
				rm.CellToRegionID[cIdx] = regionID
				sumVariance += rm.SettlingMetrics.VariancePerCell[cIdx]
			}
			region.MeanVariance = sumVariance / float64(len(regionCells))

			// Assign parameters based on category
			region.Params = rm.assignRegionParams(category, grid.Params)

			tempRegions = append(tempRegions, region)
			regionID++
		}
	}

	// Step 4: Merge regions if we exceed maxRegions
	if len(tempRegions) > maxRegions {
		tempRegions = rm.mergeSmallestRegions(tempRegions, grid, maxRegions)
	}

	rm.Regions = tempRegions
	rm.IdentificationComplete = true
	rm.IdentificationTime = time.Now()

	diagf("[RegionManager] Identified %d regions from %d cells (target: max %d regions, variance samples: %d)",
		len(rm.Regions), totalCells, maxRegions, rm.SettlingMetrics.FramesSampled)

	return nil
}

// assignRegionParams determines parameters for a region based on its variance category
func (rm *RegionManager) assignRegionParams(category int, baseParams BackgroundParams) RegionParams {
	// Base parameters from grid defaults
	baseNoise := baseParams.NoiseRelativeFraction
	if baseNoise <= 0 {
		baseNoise = 0.01
	}
	baseNeighbour := baseParams.NeighbourConfirmationCount
	if baseNeighbour <= 0 {
		baseNeighbour = 3
	}
	baseAlpha := baseParams.BackgroundUpdateFraction
	if baseAlpha <= 0 {
		baseAlpha = 0.02
	}

	switch category {
	case 0: // stable (low variance) - EXTREMELY high tolerance to eliminate false positives, faster settling
		return RegionParams{
			NoiseRelativeFraction:      baseNoise * 4.0, // EXTREMELY high tolerance - stable surfaces should almost never flag FG
			NeighbourConfirmationCount: baseNeighbour,   // standard neighbour check
			SettleUpdateFraction:       baseAlpha * 1.5, // faster settling
		}
	case 1: // variable (medium variance) - significantly higher tolerance
		return RegionParams{
			NoiseRelativeFraction:      baseNoise * 3.0, // significantly higher tolerance
			NeighbourConfirmationCount: baseNeighbour,
			SettleUpdateFraction:       baseAlpha,
		}
	case 2: // volatile (high variance - trees, glass) - extremely high tolerance, slower settling
		return RegionParams{
			NoiseRelativeFraction:      baseNoise * 8.0,   // EXTREMELY high tolerance for dynamic surfaces
			NeighbourConfirmationCount: baseNeighbour + 2, // require more neighbours
			SettleUpdateFraction:       baseAlpha * 0.5,   // slower settling to handle variance
		}
	default:
		return RegionParams{
			NoiseRelativeFraction:      baseNoise,
			NeighbourConfirmationCount: baseNeighbour,
			SettleUpdateFraction:       baseAlpha,
		}
	}
}

// mergeSmallestRegions reduces the number of regions by merging smallest adjacent regions
func (rm *RegionManager) mergeSmallestRegions(regions []*Region, grid *BackgroundGrid, targetMax int) []*Region {
	// Sort regions by size (smallest first) using sort.Slice
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].CellCount < regions[j].CellCount
	})

	// Merge smallest regions into nearest neighbours until we reach target
	for len(regions) > targetMax {
		// Take the smallest region
		smallest := regions[0]
		regions = regions[1:]

		// Find the nearest region (by checking neighbours of cells in smallest)
		nearestRegionID := -1
		for _, cellIdx := range smallest.CellList {
			ring := cellIdx / grid.AzimuthBins
			azBin := cellIdx % grid.AzimuthBins

			// Check all neighbours
			neighbours := []int{
				grid.Idx(ring, (azBin+1)%grid.AzimuthBins),
				grid.Idx(ring, (azBin-1+grid.AzimuthBins)%grid.AzimuthBins),
			}
			if ring > 0 {
				neighbours = append(neighbours, grid.Idx(ring-1, azBin))
			}
			if ring < grid.Rings-1 {
				neighbours = append(neighbours, grid.Idx(ring+1, azBin))
			}

			for _, nIdx := range neighbours {
				nRegionID := rm.CellToRegionID[nIdx]
				if nRegionID >= 0 && nRegionID != smallest.ID {
					nearestRegionID = nRegionID
					break
				}
			}
			if nearestRegionID >= 0 {
				break
			}
		}

		// Merge into nearest region (or first region if no neighbour found)
		if nearestRegionID < 0 && len(regions) > 0 {
			nearestRegionID = regions[0].ID
		}

		// Find the target region and merge
		for _, r := range regions {
			if r.ID == nearestRegionID {
				// Merge smallest into this region
				for _, cellIdx := range smallest.CellList {
					r.CellMask[cellIdx] = true
					r.CellList = append(r.CellList, cellIdx)
					rm.CellToRegionID[cellIdx] = r.ID
				}
				r.CellCount += smallest.CellCount
				// Update mean variance
				r.MeanVariance = (r.MeanVariance*float64(r.CellCount-smallest.CellCount) +
					smallest.MeanVariance*float64(smallest.CellCount)) / float64(r.CellCount)
				break
			}
		}
	}

	// Reassign IDs sequentially
	for i, r := range regions {
		oldID := r.ID
		r.ID = i
		// Update mapping
		for _, cellIdx := range r.CellList {
			if rm.CellToRegionID[cellIdx] == oldID {
				rm.CellToRegionID[cellIdx] = i
			}
		}
	}

	return regions
}

// createDefaultRegion creates a single default region covering all cells
func (rm *RegionManager) createDefaultRegion(grid *BackgroundGrid) error {
	totalCells := len(grid.Cells)
	region := &Region{
		ID:        0,
		CellMask:  make([]bool, totalCells),
		CellList:  make([]int, 0, totalCells),
		CellCount: totalCells,
		Params: RegionParams{
			NoiseRelativeFraction:      grid.Params.NoiseRelativeFraction,
			NeighbourConfirmationCount: grid.Params.NeighbourConfirmationCount,
			SettleUpdateFraction:       grid.Params.BackgroundUpdateFraction,
		},
	}
	for i := 0; i < totalCells; i++ {
		region.CellMask[i] = true
		region.CellList = append(region.CellList, i)
		rm.CellToRegionID[i] = 0
	}
	rm.Regions = []*Region{region}
	rm.IdentificationComplete = true
	rm.IdentificationTime = time.Now()
	diagf("[RegionManager] Created single default region covering %d cells", totalCells)
	return nil
}

// GetRegionForCell returns the region ID for a given cell index
func (rm *RegionManager) GetRegionForCell(cellIdx int) int {
	if !rm.IdentificationComplete || cellIdx < 0 || cellIdx >= len(rm.CellToRegionID) {
		return -1
	}
	return rm.CellToRegionID[cellIdx]
}

// GetRegionParams returns the parameters for a given region ID
func (rm *RegionManager) GetRegionParams(regionID int) *RegionParams {
	if !rm.IdentificationComplete || regionID < 0 || regionID >= len(rm.Regions) {
		return nil
	}
	return &rm.Regions[regionID].Params
}

// ToSnapshot serialises the RegionManager's region data into a form suitable for database persistence.
// Returns nil if regions have not yet been identified.
func (rm *RegionManager) ToSnapshot(sensorID string, snapshotID int64) *RegionSnapshot {
	if !rm.IdentificationComplete || len(rm.Regions) == 0 {
		return nil
	}

	// Convert regions to serialisable form
	regionData := make([]RegionData, len(rm.Regions))
	for i, r := range rm.Regions {
		regionData[i] = RegionData{
			ID:           r.ID,
			Params:       r.Params,
			CellList:     r.CellList,
			MeanVariance: r.MeanVariance,
			CellCount:    r.CellCount,
		}
	}

	regionsJSON, err := json.Marshal(regionData)
	if err != nil {
		opsf("[RegionManager] ToSnapshot: failed to marshal regions: %v", err)
		return nil
	}

	// Optionally include variance data for debugging
	var varianceJSON string
	if len(rm.SettlingMetrics.VariancePerCell) > 0 {
		type VarianceData struct {
			VariancePerCell []float64 `json:"variance_per_cell"`
			FramesSampled   int       `json:"frames_sampled"`
		}
		vd := VarianceData{
			VariancePerCell: rm.SettlingMetrics.VariancePerCell,
			FramesSampled:   rm.SettlingMetrics.FramesSampled,
		}
		if b, err := json.Marshal(vd); err == nil {
			varianceJSON = string(b)
		}
	}

	return &RegionSnapshot{
		SnapshotID:       snapshotID,
		SensorID:         sensorID,
		CreatedUnixNanos: time.Now().UnixNano(),
		RegionCount:      len(rm.Regions),
		RegionsJSON:      string(regionsJSON),
		VarianceDataJSON: varianceJSON,
		SettlingFrames:   rm.SettlingMetrics.FramesSampled,
		GridHash:         "", // Will be set by caller with SceneSignature()
	}
}

// RestoreFromSnapshot rebuilds the RegionManager state from a persisted snapshot.
// This allows skipping the settling period when the scene hash matches.
func (rm *RegionManager) RestoreFromSnapshot(snap *RegionSnapshot, totalCells int) error {
	if snap == nil {
		return fmt.Errorf("nil snapshot")
	}

	var regionData []RegionData
	if err := json.Unmarshal([]byte(snap.RegionsJSON), &regionData); err != nil {
		return fmt.Errorf("failed to unmarshal regions_json: %w", err)
	}

	if len(regionData) == 0 {
		return fmt.Errorf("empty regions data")
	}

	// Rebuild CellToRegionID mapping
	rm.CellToRegionID = make([]int, totalCells)
	for i := range rm.CellToRegionID {
		rm.CellToRegionID[i] = -1
	}

	// Rebuild regions
	rm.Regions = make([]*Region, len(regionData))
	for i, rd := range regionData {
		region := &Region{
			ID:           rd.ID,
			Params:       rd.Params,
			CellList:     rd.CellList,
			MeanVariance: rd.MeanVariance,
			CellCount:    rd.CellCount,
			CellMask:     make([]bool, totalCells),
		}
		// Rebuild CellMask from CellList
		for _, cellIdx := range rd.CellList {
			if cellIdx >= 0 && cellIdx < totalCells {
				region.CellMask[cellIdx] = true
				rm.CellToRegionID[cellIdx] = rd.ID
			}
		}
		rm.Regions[i] = region
	}

	// Restore variance data if present
	if snap.VarianceDataJSON != "" {
		var vd struct {
			VariancePerCell []float64 `json:"variance_per_cell"`
			FramesSampled   int       `json:"frames_sampled"`
		}
		if err := json.Unmarshal([]byte(snap.VarianceDataJSON), &vd); err == nil {
			rm.SettlingMetrics.VariancePerCell = vd.VariancePerCell
			rm.SettlingMetrics.FramesSampled = vd.FramesSampled
		}
	}

	rm.IdentificationComplete = true
	rm.IdentificationTime = time.Unix(0, snap.CreatedUnixNanos)

	diagf("[RegionManager] Restored %d regions from snapshot (settling_frames=%d, grid_hash=%s)",
		len(rm.Regions), snap.SettlingFrames, snap.GridHash)

	return nil
}
