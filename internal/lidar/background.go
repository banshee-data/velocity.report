package lidar

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/monitoring"
)

// BackgroundParams configuration matching the param storage approach in schema
type BackgroundParams struct {
	BackgroundUpdateFraction       float32 // e.g., 0.02
	ClosenessSensitivityMultiplier float32 // e.g., 3.0
	SafetyMarginMeters             float32 // e.g., 0.5
	FreezeDurationNanos            int64   // e.g., 5e9 (5s)
	NeighborConfirmationCount      int     // e.g., 5 of 8 neighbors
	WarmupDurationNanos            int64   // optional extra settle time before emitting foreground
	WarmupMinFrames                int     // optional minimum frames before considering settled
	PostSettleUpdateFraction       float32 // optional lower alpha after settle for stability
	ForegroundMinClusterPoints     int     // min points for a cluster to be forwarded/considered
	ForegroundDBSCANEps            float32 // clustering radius for foreground gating
	// NoiseRelativeFraction is the fraction of range (distance) to treat as
	// expected measurement noise. This allows closeness thresholds to grow
	// with distance so that farther returns (which naturally have larger
	// absolute noise) aren't biased as foreground. Typical values: 0.01
	// (1%) to 0.02 (2%). If zero, a sensible default (0.01) is used.
	NoiseRelativeFraction float32

	// SeedFromFirstObservation, when true, will initialize empty background cells
	// from the first observation seen for that cell. This is useful for PCAP
	// replay mode where there is no prior live-warmup data; default: false.
	SeedFromFirstObservation bool

	// ReacquisitionBoostMultiplier controls how much faster cells re-acquire
	// background after a foreground event. When a cell that recently saw foreground
	// receives an observation matching the prior background, the effective alpha
	// is multiplied by this factor for faster convergence. Default: 5.0.
	// Set to 1.0 to disable the boost.
	ReacquisitionBoostMultiplier float32

	// MinConfidenceFloor is the minimum TimesSeenCount to preserve during foreground
	// observations. This prevents cells from completely "forgetting" their settled
	// background when a vehicle passes through. Default: 3.
	MinConfidenceFloor uint32

	// LockedBaselineThreshold is the minimum TimesSeenCount before we lock the
	// baseline. Once locked, the baseline only updates slowly. Default: 50.
	LockedBaselineThreshold uint32
	// LockedBaselineMultiplier controls how many times the LockedSpread defines
	// the acceptance window. Observations within LockedBaseline ± (LockedSpread *
	// LockedBaselineMultiplier) are considered background. Default: 4.0.
	LockedBaselineMultiplier float32

	// Debug logging region (only active if EnableDiagnostics is true)
	DebugRingMin int     // Min ring index (inclusive)
	DebugRingMax int     // Max ring index (inclusive)
	DebugAzMin   float32 // Min azimuth degrees (inclusive)
	DebugAzMax   float32 // Max azimuth degrees (inclusive)

	// Additional params for persistence matching schema requirements
	SettlingPeriodNanos        int64 // 5 minutes before first snapshot
	SnapshotIntervalNanos      int64 // 2 hours between snapshots
	ChangeThresholdForSnapshot int   // min changed cells to trigger snapshot
}

// RegionParams defines parameters that can vary per region
type RegionParams struct {
	NoiseRelativeFraction     float32 // noise threshold for this region
	NeighborConfirmationCount int     // neighbor confirmation for this region
	SettleUpdateFraction      float32 // alpha during settling for this region
}

// Region represents a contiguous spatial region with distinct parameters
type Region struct {
	ID       int          // unique region identifier
	CellMask []bool       // len = Rings * AzimuthBins; true if cell belongs to this region
	Params   RegionParams // parameters for this region
	CellList []int        // list of cell indices in this region (for efficient iteration)
	// Statistics for region characterization
	MeanVariance float64 // mean variance of cells in this region during settling
	CellCount    int     // number of cells in this region
}

// RegionManager handles dynamic region identification and management
type RegionManager struct {
	Regions         []*Region // list of identified regions
	CellToRegionID  []int     // maps cell index to region ID (-1 if unassigned)
	SettlingMetrics struct {
		VariancePerCell []float64 // variance observed per cell during settling
		FramesSampled   int       // frames sampled for variance calculation
	}
	IdentificationComplete bool      // true once regions are identified
	IdentificationTime     time.Time // when regions were identified
}

// HasDebugRange returns true if any debug range parameters are set.
func (p BackgroundParams) HasDebugRange() bool {
	return p.DebugRingMax > 0 || p.DebugRingMin > 0 || p.DebugAzMax > 0 || p.DebugAzMin > 0
}

// IsInDebugRange returns true if the given ring and azimuth fall within the configured debug range.
// If no range is configured, it returns false.
func (p BackgroundParams) IsInDebugRange(ring int, az float64) bool {
	hasRingLimit := p.DebugRingMax > 0 || p.DebugRingMin > 0
	hasAzLimit := p.DebugAzMax > 0 || p.DebugAzMin > 0

	if !hasRingLimit && !hasAzLimit {
		return false
	}

	if hasRingLimit {
		if ring < p.DebugRingMin || ring > p.DebugRingMax {
			return false
		}
	}

	if hasAzLimit {
		// Normalize azimuth into [0,360)
		normAz := math.Mod(az, 360.0)
		if normAz < 0 {
			normAz += 360.0
		}
		az32 := float32(normAz)
		if az32 < p.DebugAzMin || az32 > p.DebugAzMax {
			return false
		}
	}

	return true
}

// BackgroundCell matches the compressed storage format for schema persistence
type BackgroundCell struct {
	AverageRangeMeters   float32
	RangeSpreadMeters    float32
	TimesSeenCount       uint32
	LastUpdateUnixNanos  int64
	FrozenUntilUnixNanos int64
	// RecentForegroundCount tracks consecutive foreground observations.
	// Used for fast re-acquisition: when this is >0 and observation matches
	// background, we apply boosted alpha for faster convergence.
	RecentForegroundCount uint16

	// LockedBaseline is the stable reference distance that only updates when
	// we have high confidence (TimesSeenCount > LockedThreshold). This protects
	// against transit corruption where EMA average drifts during occlusion.
	LockedBaseline float32
	// LockedSpread is the acceptable variance around LockedBaseline. Observations
	// within LockedBaseline ± (LockedSpread * multiplier) are considered background.
	// This allows per-cell variance (trees, glass have more variance).
	LockedSpread float32
	// LockedAtCount is the TimesSeenCount when baseline was last locked.
	// Used to detect when we should update the locked values.
	LockedAtCount uint32
}

// BackgroundGrid enhanced for schema persistence and 100-track performance
type BackgroundGrid struct {
	SensorID    string
	SensorFrame FrameID // e.g., "sensor/hesai-01"

	Rings       int // e.g., 40 - matches schema rings INTEGER NOT NULL
	AzimuthBins int // e.g., 1800 for 0.2° - matches schema azimuth_bins INTEGER NOT NULL

	Cells []BackgroundCell // len = Rings * AzimuthBins

	Params BackgroundParams

	// Enhanced persistence tracking matching schema lidar_bg_snapshot table
	Manager              *BackgroundManager
	LastSnapshotTime     time.Time
	ChangesSinceSnapshot int
	SnapshotID           *int64 // tracks last persisted snapshot_id from schema

	// Performance tracking for system_events table integration
	LastProcessingTimeUs  int64
	WarmupFramesRemaining int
	SettlingComplete      bool

	// Telemetry for monitoring (feeds into system_events)
	ForegroundCount int64
	BackgroundCount int64
	// nonzeroCellCount tracks cells with TimesSeenCount > 0; guarded by mu.
	nonzeroCellCount int

	// Simple range-bucketed acceptance metrics to help tune NoiseRelativeFraction.
	// Buckets are upper bounds in meters; counts are number of accepted/rejected
	// observations that fell into that distance bucket. These are incremented
	// inside ProcessFramePolar while holding g.mu, and can be read via
	// BackgroundManager.GetAcceptanceMetrics().
	AcceptanceBucketsMeters []float64
	AcceptByRangeBuckets    []int64
	RejectByRangeBuckets    []int64

	// Thread safety for concurrent access during persistence
	// mu protects Cells and persistence-related fields when accessed concurrently
	mu sync.RWMutex
	// Optional per-ring elevation angles (degrees) for converting polar->cartesian.
	// If populated (len == Rings) ToASCPoints will use these to compute Z = r*sin(elev).
	RingElevations []float64
	// LastObservedNoiseRel tracks the last noise_relative value observed by
	// ProcessFramePolar so we can log when the runtime value changes.
	LastObservedNoiseRel float32

	// RegionMgr handles adaptive parameter regions for different settling characteristics
	RegionMgr *RegionManager
}

// Helper to index Cells: idx = ring*AzimuthBins + azBin
func (g *BackgroundGrid) Idx(ring, azBin int) int { return ring*g.AzimuthBins + azBin }

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
func (rm *RegionManager) UpdateVarianceMetrics(cells []BackgroundCell) {
	if rm.IdentificationComplete {
		return
	}
	n := float64(rm.SettlingMetrics.FramesSampled)
	for i, cell := range cells {
		if cell.TimesSeenCount > 0 {
			// Incremental variance calculation using Welford's method
			variance := float64(cell.RangeSpreadMeters)
			if n == 0 {
				rm.SettlingMetrics.VariancePerCell[i] = variance
			} else {
				// Update running mean of variance
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
	for i := 0; i < len(sortedVars); i++ {
		for j := i + 1; j < len(sortedVars); j++ {
			if sortedVars[j] < sortedVars[i] {
				sortedVars[i], sortedVars[j] = sortedVars[j], sortedVars[i]
			}
		}
	}

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

			// Get neighbors (same ring ± 1 az, same az ± 1 ring)
			ring := current / grid.AzimuthBins
			azBin := current % grid.AzimuthBins

			neighbors := []struct{ r, az int }{
				{ring, (azBin + 1) % grid.AzimuthBins},
				{ring, (azBin - 1 + grid.AzimuthBins) % grid.AzimuthBins},
			}
			if ring > 0 {
				neighbors = append(neighbors, struct{ r, az int }{ring - 1, azBin})
			}
			if ring < grid.Rings-1 {
				neighbors = append(neighbors, struct{ r, az int }{ring + 1, azBin})
			}

			for _, n := range neighbors {
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
				ID:       regionID,
				CellMask: make([]bool, totalCells),
				CellList: regionCells,
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

	log.Printf("[RegionManager] Identified %d regions from %d cells (target: max %d regions, variance samples: %d)",
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
	baseNeighbor := baseParams.NeighborConfirmationCount
	if baseNeighbor <= 0 {
		baseNeighbor = 3
	}
	baseAlpha := baseParams.BackgroundUpdateFraction
	if baseAlpha <= 0 {
		baseAlpha = 0.02
	}

	switch category {
	case 0: // stable (low variance) - tighter thresholds, faster settling
		return RegionParams{
			NoiseRelativeFraction:     baseNoise * 0.8,         // tighter noise tolerance
			NeighborConfirmationCount: baseNeighbor,            // standard neighbor check
			SettleUpdateFraction:      baseAlpha * 1.5,         // faster settling
		}
	case 1: // variable (medium variance) - standard parameters
		return RegionParams{
			NoiseRelativeFraction:     baseNoise,
			NeighborConfirmationCount: baseNeighbor,
			SettleUpdateFraction:      baseAlpha,
		}
	case 2: // volatile (high variance - trees, glass) - looser thresholds, slower settling
		return RegionParams{
			NoiseRelativeFraction:     baseNoise * 2.0,         // much looser noise tolerance
			NeighborConfirmationCount: baseNeighbor + 2,        // require more neighbors
			SettleUpdateFraction:      baseAlpha * 0.5,         // slower settling to handle variance
		}
	default:
		return RegionParams{
			NoiseRelativeFraction:     baseNoise,
			NeighborConfirmationCount: baseNeighbor,
			SettleUpdateFraction:      baseAlpha,
		}
	}
}

// mergeSmallestRegions reduces the number of regions by merging smallest adjacent regions
func (rm *RegionManager) mergeSmallestRegions(regions []*Region, grid *BackgroundGrid, targetMax int) []*Region {
	// Sort regions by size (smallest first)
	for i := 0; i < len(regions); i++ {
		for j := i + 1; j < len(regions); j++ {
			if regions[j].CellCount < regions[i].CellCount {
				regions[i], regions[j] = regions[j], regions[i]
			}
		}
	}

	// Merge smallest regions into nearest neighbors until we reach target
	for len(regions) > targetMax {
		// Take the smallest region
		smallest := regions[0]
		regions = regions[1:]

		// Find the nearest region (by checking neighbors of cells in smallest)
		nearestRegionID := -1
		for _, cellIdx := range smallest.CellList {
			ring := cellIdx / grid.AzimuthBins
			azBin := cellIdx % grid.AzimuthBins

			// Check all neighbors
			neighbors := []int{
				grid.Idx(ring, (azBin+1)%grid.AzimuthBins),
				grid.Idx(ring, (azBin-1+grid.AzimuthBins)%grid.AzimuthBins),
			}
			if ring > 0 {
				neighbors = append(neighbors, grid.Idx(ring-1, azBin))
			}
			if ring < grid.Rings-1 {
				neighbors = append(neighbors, grid.Idx(ring+1, azBin))
			}

			for _, nIdx := range neighbors {
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

		// Merge into nearest region (or first region if no neighbor found)
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
			NoiseRelativeFraction:     grid.Params.NoiseRelativeFraction,
			NeighborConfirmationCount: grid.Params.NeighborConfirmationCount,
			SettleUpdateFraction:      grid.Params.BackgroundUpdateFraction,
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
	log.Printf("[RegionManager] Created single default region covering %d cells", totalCells)
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

// BackgroundManager handles automatic persistence following schema lidar_bg_snapshot pattern
type BackgroundManager struct {
	Grid            *BackgroundGrid
	SettlingTimer   *time.Timer
	PersistTimer    *time.Timer
	HasSettled      bool
	LastPersistTime time.Time
	StartTime       time.Time

	// Persistence callback to main app - should save to schema lidar_bg_snapshot table
	PersistCallback func(snapshot *BgSnapshot) error
	// EnableDiagnostics controls whether this manager emits diagnostic messages
	// via the shared monitoring logger. Default: false.
	EnableDiagnostics bool
	// frameProcessCount tracks the number of ProcessFramePolar calls for rate-limited diagnostics.
	// Accessed atomically to allow concurrent ProcessFramePolar invocations.
	frameProcessCount int64
}

// GetParams returns a copy of the BackgroundParams for the manager's grid.
func (bm *BackgroundManager) GetParams() BackgroundParams {
	if bm == nil || bm.Grid == nil {
		return BackgroundParams{}
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.Params
}

// SetParams replaces the manager's BackgroundParams atomically.
func (bm *BackgroundManager) SetParams(p BackgroundParams) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	g.Params = p
	g.mu.Unlock()
	return nil
}

// SetNoiseRelativeFraction safely updates the NoiseRelativeFraction parameter.
func (bm *BackgroundManager) SetNoiseRelativeFraction(v float32) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	g.Params.NoiseRelativeFraction = v
	g.mu.Unlock()
	return nil
}

// SetClosenessSensitivityMultiplier safely updates the ClosenessSensitivityMultiplier parameter.
func (bm *BackgroundManager) SetClosenessSensitivityMultiplier(v float32) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	g.Params.ClosenessSensitivityMultiplier = v
	g.mu.Unlock()
	return nil
}

// SetNeighborConfirmationCount safely updates the NeighborConfirmationCount parameter.
func (bm *BackgroundManager) SetNeighborConfirmationCount(v int) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	g.Params.NeighborConfirmationCount = v
	g.mu.Unlock()
	return nil
}

// SetSeedFromFirstObservation toggles seeding empty cells from the first observation.
func (bm *BackgroundManager) SetSeedFromFirstObservation(v bool) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	g.Params.SeedFromFirstObservation = v
	g.mu.Unlock()
	return nil
}

// SetWarmupParams updates settle duration/frame requirements.
func (bm *BackgroundManager) SetWarmupParams(durationNanos int64, minFrames int) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	g.Params.WarmupDurationNanos = durationNanos
	g.Params.WarmupMinFrames = minFrames
	g.mu.Unlock()
	return nil
}

// SetPostSettleUpdateFraction updates the post-settle alpha.
func (bm *BackgroundManager) SetPostSettleUpdateFraction(v float32) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	g.Params.PostSettleUpdateFraction = v
	g.mu.Unlock()
	return nil
}

// SetForegroundClusterParams updates the minimum cluster size and eps used for foreground gating.
func (bm *BackgroundManager) SetForegroundClusterParams(minPts int, eps float32) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	if minPts > 0 {
		g.Params.ForegroundMinClusterPoints = minPts
	}
	if eps > 0 {
		g.Params.ForegroundDBSCANEps = eps
	}
	g.mu.Unlock()
	return nil
}

// SetEnableDiagnostics toggles emission of diagnostics for this manager.
func (bm *BackgroundManager) SetEnableDiagnostics(v bool) {
	if bm == nil {
		return
	}
	bm.EnableDiagnostics = v
}

// AcceptanceMetrics exposes the acceptance/rejection counts per range bucket.
type AcceptanceMetrics struct {
	BucketsMeters []float64
	AcceptCounts  []int64
	RejectCounts  []int64
}

// GetAcceptanceMetrics returns a snapshot of the acceptance metrics. The
// returned slices are copies and safe for the caller to inspect without
// further synchronization.
func (bm *BackgroundManager) GetAcceptanceMetrics() *AcceptanceMetrics {
	if bm == nil || bm.Grid == nil {
		return &AcceptanceMetrics{}
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()
	if len(g.AcceptanceBucketsMeters) == 0 {
		return &AcceptanceMetrics{}
	}
	buckets := make([]float64, len(g.AcceptanceBucketsMeters))
	copy(buckets, g.AcceptanceBucketsMeters)
	accept := make([]int64, len(g.AcceptByRangeBuckets))
	copy(accept, g.AcceptByRangeBuckets)
	reject := make([]int64, len(g.RejectByRangeBuckets))
	copy(reject, g.RejectByRangeBuckets)
	return &AcceptanceMetrics{BucketsMeters: buckets, AcceptCounts: accept, RejectCounts: reject}
}

// ResetAcceptanceMetrics zeros the acceptance/rejection counters for the grid.
// This is intended for clean A/B testing when tuning parameters.
func (bm *BackgroundManager) ResetAcceptanceMetrics() error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.AcceptByRangeBuckets) != len(g.AcceptanceBucketsMeters) {
		g.AcceptByRangeBuckets = make([]int64, len(g.AcceptanceBucketsMeters))
	} else {
		for i := range g.AcceptByRangeBuckets {
			g.AcceptByRangeBuckets[i] = 0
		}
	}
	if len(g.RejectByRangeBuckets) != len(g.AcceptanceBucketsMeters) {
		g.RejectByRangeBuckets = make([]int64, len(g.AcceptanceBucketsMeters))
	} else {
		for i := range g.RejectByRangeBuckets {
			g.RejectByRangeBuckets[i] = 0
		}
	}
	return nil
}

// GridStatus returns a simple snapshot of grid-level statistics useful for
// debugging settling behavior. The returned map includes total_cells, frozen_cells,
// a times-seen distribution (string->count) and foreground/background counters.
func (bm *BackgroundManager) GridStatus() map[string]interface{} {
	if bm == nil || bm.Grid == nil {
		return nil
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	total := len(g.Cells)
	frozen := 0
	timesSeenDist := map[string]int{}
	for _, c := range g.Cells {
		if c.FrozenUntilUnixNanos > time.Now().UnixNano() {
			frozen++
		}
		key := fmt.Sprintf("%d", c.TimesSeenCount)
		timesSeenDist[key]++
	}

	return map[string]interface{}{
		"total_cells":      total,
		"frozen_cells":     frozen,
		"times_seen_dist":  timesSeenDist,
		"foreground_count": g.ForegroundCount,
		"background_count": g.BackgroundCount,
	}
}

// CoarseBucket represents aggregated metrics for a spatial bucket
type CoarseBucket struct {
	Ring            int     `json:"ring"`
	AzimuthDegStart float64 `json:"azimuth_deg_start"`
	AzimuthDegEnd   float64 `json:"azimuth_deg_end"`
	TotalCells      int     `json:"total_cells"`
	FilledCells     int     `json:"filled_cells"`
	SettledCells    int     `json:"settled_cells"`
	FrozenCells     int     `json:"frozen_cells"`
	MeanTimesSeen   float64 `json:"mean_times_seen"`
	MeanRangeMeters float64 `json:"mean_range_meters"`
	MinRangeMeters  float64 `json:"min_range_meters"`
	MaxRangeMeters  float64 `json:"max_range_meters"`
}

// GridHeatmap represents the full aggregated grid state
type GridHeatmap struct {
	SensorID      string                 `json:"sensor_id"`
	Timestamp     time.Time              `json:"timestamp"`
	GridParams    map[string]interface{} `json:"grid_params"`
	HeatmapParams map[string]interface{} `json:"heatmap_params"`
	Summary       map[string]interface{} `json:"summary"`
	Buckets       []CoarseBucket         `json:"buckets"`
}

// GetGridHeatmap aggregates the fine-grained grid into coarse spatial buckets
// for visualization and analysis. Returns nil if the manager or grid is nil.
//
// Parameters:
//   - azimuthBucketDeg: size of each azimuth bucket in degrees (e.g., 3.0 for 120 buckets)
//   - settledThreshold: minimum TimesSeenCount to consider a cell "settled"
func (bm *BackgroundManager) GetGridHeatmap(azimuthBucketDeg float64, settledThreshold uint32) *GridHeatmap {
	if bm == nil || bm.Grid == nil {
		return nil
	}

	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Calculate bucket dimensions
	azBinResDeg := 360.0 / float64(g.AzimuthBins)
	numAzBuckets := int(360.0 / azimuthBucketDeg)
	cellsPerAzBucket := int(azimuthBucketDeg / azBinResDeg)

	// Initialise buckets
	buckets := make([]CoarseBucket, 0, g.Rings*numAzBuckets)
	nowNanos := time.Now().UnixNano()

	totalFilled := 0
	totalSettled := 0
	totalFrozen := 0

	// Aggregate by ring and azimuth bucket
	for ring := 0; ring < g.Rings; ring++ {
		for azBucket := 0; azBucket < numAzBuckets; azBucket++ {
			bucket := CoarseBucket{
				Ring:            ring,
				AzimuthDegStart: float64(azBucket) * azimuthBucketDeg,
				AzimuthDegEnd:   float64(azBucket+1) * azimuthBucketDeg,
				TotalCells:      cellsPerAzBucket,
				MinRangeMeters:  math.MaxFloat64,
			}

			// Aggregate stats from fine cells in this bucket
			startAzBin := azBucket * cellsPerAzBucket
			endAzBin := startAzBin + cellsPerAzBucket

			var sumTimesSeen uint32
			var sumRange float64
			filledCount := 0

			for azBin := startAzBin; azBin < endAzBin && azBin < g.AzimuthBins; azBin++ {
				idx := g.Idx(ring, azBin)
				cell := g.Cells[idx]

				if cell.TimesSeenCount > 0 {
					bucket.FilledCells++
					filledCount++
					sumTimesSeen += cell.TimesSeenCount
					sumRange += float64(cell.AverageRangeMeters)

					if cell.TimesSeenCount >= settledThreshold {
						bucket.SettledCells++
					}

					if cell.AverageRangeMeters < float32(bucket.MinRangeMeters) {
						bucket.MinRangeMeters = float64(cell.AverageRangeMeters)
					}
					if cell.AverageRangeMeters > float32(bucket.MaxRangeMeters) {
						bucket.MaxRangeMeters = float64(cell.AverageRangeMeters)
					}
				}

				if cell.FrozenUntilUnixNanos > nowNanos {
					bucket.FrozenCells++
				}
			}

			// Calculate means
			if filledCount > 0 {
				bucket.MeanTimesSeen = float64(sumTimesSeen) / float64(filledCount)
				bucket.MeanRangeMeters = sumRange / float64(filledCount)
			}
			if bucket.FilledCells == 0 {
				bucket.MinRangeMeters = 0
				bucket.MaxRangeMeters = 0
			}

			totalFilled += bucket.FilledCells
			totalSettled += bucket.SettledCells
			totalFrozen += bucket.FrozenCells

			buckets = append(buckets, bucket)
		}
	}

	totalCells := g.Rings * g.AzimuthBins

	return &GridHeatmap{
		SensorID:  g.SensorID,
		Timestamp: time.Now(),
		GridParams: map[string]interface{}{
			"total_rings":                g.Rings,
			"total_azimuth_bins":         g.AzimuthBins,
			"azimuth_bin_resolution_deg": 360.0 / float64(g.AzimuthBins),
			"total_cells":                totalCells,
		},
		HeatmapParams: map[string]interface{}{
			"azimuth_bucket_deg": azimuthBucketDeg,
			"azimuth_buckets":    numAzBuckets,
			"ring_buckets":       g.Rings,
			"settled_threshold":  settledThreshold,
			"cells_per_bucket":   cellsPerAzBucket,
		},
		Summary: map[string]interface{}{
			"total_filled":  totalFilled,
			"total_settled": totalSettled,
			"total_frozen":  totalFrozen,
			"fill_rate":     float64(totalFilled) / float64(totalCells),
			"settle_rate":   float64(totalSettled) / float64(totalCells),
		},
		Buckets: buckets,
	}
}

// ResetGrid zeros per-cell stats (AverageRangeMeters, RangeSpreadMeters, TimesSeenCount,
// LastUpdateUnixNanos, FrozenUntilUnixNanos) and acceptance counters. Intended for
// testing and A/B sweeps only.
func (bm *BackgroundManager) ResetGrid() error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	defer g.mu.Unlock()

	// Count nonzero cells BEFORE reset (Log A: grid reset diagnostics)
	nonzeroBefore := 0
	for i := range g.Cells {
		if g.Cells[i].AverageRangeMeters != 0 || g.Cells[i].RangeSpreadMeters != 0 || g.Cells[i].TimesSeenCount != 0 {
			nonzeroBefore++
		}
	}

	for i := range g.Cells {
		g.Cells[i].AverageRangeMeters = 0
		g.Cells[i].RangeSpreadMeters = 0
		g.Cells[i].TimesSeenCount = 0
		g.Cells[i].LastUpdateUnixNanos = 0
		g.Cells[i].FrozenUntilUnixNanos = 0
		// Also reset foreground tracking and locked baseline fields
		g.Cells[i].RecentForegroundCount = 0
		g.Cells[i].LockedBaseline = 0
		g.Cells[i].LockedSpread = 0
		g.Cells[i].LockedAtCount = 0
	}
	for i := range g.AcceptByRangeBuckets {
		g.AcceptByRangeBuckets[i] = 0
		g.RejectByRangeBuckets[i] = 0
	}
	g.ChangesSinceSnapshot = 0
	g.ForegroundCount = 0
	g.BackgroundCount = 0
	g.nonzeroCellCount = 0

	// Count nonzero cells AFTER reset (should be 0)
	nonzeroAfter := 0
	for i := range g.Cells {
		if g.Cells[i].AverageRangeMeters != 0 || g.Cells[i].RangeSpreadMeters != 0 || g.Cells[i].TimesSeenCount != 0 {
			nonzeroAfter++
		}
	}

	// Log with timestamp and counts
	log.Printf("[ResetGrid] sensor=%s nonzero_before=%d nonzero_after=%d total_cells=%d timestamp=%d",
		g.SensorID, nonzeroBefore, nonzeroAfter, len(g.Cells), time.Now().UnixNano())

	return nil
}

// Simple registry for BackgroundManager instances keyed by SensorID.
// This allows an external API to look up managers and trigger persistence.
var (
	bgMgrRegistry   = map[string]*BackgroundManager{}
	bgMgrRegistryMu = &sync.RWMutex{}
)

// RegisterBackgroundManager registers a BackgroundManager for a sensor ID.
func RegisterBackgroundManager(sensorID string, mgr *BackgroundManager) {
	if sensorID == "" || mgr == nil {
		return
	}
	bgMgrRegistryMu.Lock()
	defer bgMgrRegistryMu.Unlock()
	bgMgrRegistry[sensorID] = mgr
}

// GetBackgroundManager returns a registered manager or nil
func GetBackgroundManager(sensorID string) *BackgroundManager {
	bgMgrRegistryMu.RLock()
	defer bgMgrRegistryMu.RUnlock()
	return bgMgrRegistry[sensorID]
}

// NewBackgroundManager creates a BackgroundGrid and manager, registers it under sensorID,
// and optionally wires a BgStore for persistence (sets PersistCallback to call Persist).
func NewBackgroundManager(sensorID string, rings, azBins int, params BackgroundParams, store BgStore) *BackgroundManager {
	if sensorID == "" || rings <= 0 || azBins <= 0 {
		return nil
	}
	cells := make([]BackgroundCell, rings*azBins)
	grid := &BackgroundGrid{
		SensorID:    sensorID,
		SensorFrame: FrameID(sensorID),
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       cells,
		Params:      params,
		RegionMgr:   NewRegionManager(rings, azBins), // Initialize region manager
	}
	mgr := &BackgroundManager{Grid: grid}
	grid.Manager = mgr

	// initialize simple acceptance metric buckets (meters)
	grid.AcceptanceBucketsMeters = []float64{1, 2, 4, 8, 10, 12, 16, 20, 50, 100, 200}
	grid.AcceptByRangeBuckets = make([]int64, len(grid.AcceptanceBucketsMeters))
	grid.RejectByRangeBuckets = make([]int64, len(grid.AcceptanceBucketsMeters))

	// If a store is provided, set PersistCallback to call Persist which will serialize and write
	if store != nil {
		mgr.PersistCallback = func(s *BgSnapshot) error {
			// use provided reason when present
			reason := "manual"
			if s != nil && s.SnapshotReason != "" {
				reason = s.SnapshotReason
			}
			return mgr.Persist(store, reason)
		}
	} else {
		// Explicit runtime log to indicate persistence is disabled for this manager
		log.Printf("BackgroundManager for sensor '%s' created without a BgStore: persistence disabled", sensorID)
	}

	RegisterBackgroundManager(sensorID, mgr)
	return mgr
}

// SetRingElevations sets per-ring elevation angles (degrees) on the BackgroundGrid.
// The provided slice must have length equal to the grid's Rings; values are copied.
func (bm *BackgroundManager) SetRingElevations(elevations []float64) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	if elevations == nil {
		bm.Grid.RingElevations = nil
		return nil
	}
	if len(elevations) != bm.Grid.Rings {
		return fmt.Errorf("elevations length %d does not match rings %d", len(elevations), bm.Grid.Rings)
	}
	// copy values under lock
	bm.Grid.mu.Lock()
	bm.Grid.RingElevations = make([]float64, len(elevations))
	copy(bm.Grid.RingElevations, elevations)
	bm.Grid.mu.Unlock()
	return nil
}

// ProcessFramePolar ingests sensor-frame polar points and updates the BackgroundGrid.
// Behavior:
//   - Bins points by ring (channel) and azimuth bin.
//   - Uses an EMA (BackgroundUpdateFraction) to update AverageRangeMeters and RangeSpreadMeters.
//   - Tracks a simple two-level confidence via TimesSeenCount (increment on close matches,
//     decrement on mismatches). When a cell deviates strongly repeatedly it is frozen for
//     FreezeDurationNanos to avoid corrupting the background model.
//   - Uses neighbor confirmation: updates are applied more readily when adjacent cells
//     agree (helps suppress isolated noise).
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) {
	if bm == nil || bm.Grid == nil || len(points) == 0 {
		return
	}

	// Quick diagnostics when enabled to see what's arriving
	if bm != nil && bm.EnableDiagnostics && len(points) > 0 {
		sample := points[0]
		log.Printf("[BackgroundManager] Received %d points; sample -> Channel=%d Az=%.2f Dist=%.2f", len(points), sample.Channel, sample.Azimuth, sample.Distance)
	}

	g := bm.Grid
	rings := g.Rings
	azBins := g.AzimuthBins
	if rings <= 0 || azBins <= 0 || len(g.Cells) != rings*azBins {
		return
	}

	now := time.Now()
	nowNanos := now.UnixNano()

	// Temporary accumulators per cell
	totalCells := rings * azBins
	counts := make([]int, totalCells)
	sums := make([]float64, totalCells)
	minDistances := make([]float64, totalCells)
	maxDistances := make([]float64, totalCells)
	for i := range minDistances {
		minDistances[i] = math.Inf(1)
		maxDistances[i] = math.Inf(-1)
	}

	// Bin incoming polar points
	skippedInvalid := 0
	for _, p := range points {
		ring := p.Channel - 1
		if ring < 0 || ring >= rings {
			skippedInvalid++
			continue
		}
		// Normalize azimuth into [0,360)
		az := math.Mod(p.Azimuth, 360.0)
		if az < 0 {
			az += 360.0
		}
		azBin := int((az / 360.0) * float64(azBins))
		if azBin < 0 {
			azBin = 0
		}
		if azBin >= azBins {
			azBin = azBins - 1
		}

		cellIdx := g.Idx(ring, azBin)
		counts[cellIdx]++
		sums[cellIdx] += p.Distance
		if p.Distance < minDistances[cellIdx] {
			minDistances[cellIdx] = p.Distance
		}
		if p.Distance > maxDistances[cellIdx] {
			maxDistances[cellIdx] = p.Distance
		}
	}

	// Parameters with safe defaults
	// Emit a single summary log if we encountered points with invalid channels
	if skippedInvalid > 0 && bm != nil && bm.EnableDiagnostics {
		log.Printf("[BackgroundManager] Skipped %d invalid points due to channel out-of-range (rings=%d)", skippedInvalid, rings)
	}
	alpha := float64(g.Params.BackgroundUpdateFraction)
	if alpha <= 0 || alpha > 1 {
		alpha = 0.02
	}
	effectiveAlpha := alpha
	// Defer reading runtime-tunable params that may be updated concurrently
	// (via setters which take g.mu) until we hold the grid lock to avoid
	// data races and inconsistent thresholds.
	var closenessMultiplier float64
	var neighConfirm int
	var seedFromFirst bool
	safety := float64(g.Params.SafetyMarginMeters)
	freezeDur := g.Params.FreezeDurationNanos

	// We'll read Params under lock so updates via SetNoiseRelativeFraction
	// and other setters are visible immediately and we can detect changes.

	foregroundCount := int64(0)
	backgroundCount := int64(0)

	// Log E: Diagnostic sample counters for decision logging (gated by EnableDiagnostics)
	var acceptSampleCount, rejectSampleCount int
	const maxSamplesPerType = 10

	// Iterate over observed cells and update grid
	g.mu.Lock()
	if bm.StartTime.IsZero() {
		bm.StartTime = now
	}
	if g.WarmupFramesRemaining == 0 && g.Params.WarmupMinFrames > 0 && !g.SettlingComplete {
		g.WarmupFramesRemaining = g.Params.WarmupMinFrames
	}
	postSettleAlpha := float64(g.Params.PostSettleUpdateFraction)
	if postSettleAlpha > 0 && postSettleAlpha <= 1 && g.SettlingComplete {
		effectiveAlpha = postSettleAlpha
	}
	if !g.SettlingComplete {
		framesReady := g.Params.WarmupMinFrames <= 0 || g.WarmupFramesRemaining <= 0
		durReady := g.Params.WarmupDurationNanos <= 0 || (nowNanos-bm.StartTime.UnixNano() >= g.Params.WarmupDurationNanos)
		if framesReady && durReady {
			g.SettlingComplete = true
			if postSettleAlpha > 0 && postSettleAlpha <= 1 {
				effectiveAlpha = postSettleAlpha
			}
			// Trigger region identification when settling completes
			if g.RegionMgr != nil && !g.RegionMgr.IdentificationComplete {
				err := g.RegionMgr.IdentifyRegions(g, 50) // max 50 regions
				if err != nil {
					log.Printf("[BackgroundManager] Failed to identify regions: %v", err)
				}
			}
		} else {
			if g.WarmupFramesRemaining > 0 {
				g.WarmupFramesRemaining--
			}
			effectiveAlpha = alpha
			// Collect variance metrics during settling
			if g.RegionMgr != nil && !g.RegionMgr.IdentificationComplete {
				g.RegionMgr.UpdateVarianceMetrics(g.Cells)
			}
		}
	}
	if effectiveAlpha <= 0 || effectiveAlpha > 1 {
		effectiveAlpha = alpha
	}
	// read noiseRel under lock
	noiseRel := float64(g.Params.NoiseRelativeFraction)
	if noiseRel <= 0 {
		noiseRel = 0.01 // default to 1% if not configured
	}
	// Read other runtime-tunable params under the same lock to avoid races.
	closenessMultiplier = float64(g.Params.ClosenessSensitivityMultiplier)
	if closenessMultiplier <= 0 {
		closenessMultiplier = 3.0
	}
	neighConfirm = g.Params.NeighborConfirmationCount
	if neighConfirm <= 0 {
		neighConfirm = 3
	}
	// read seed-from-first flag
	seedFromFirst = g.Params.SeedFromFirstObservation
	// if the manager requested diagnostics, and the observed noise changed,
	// emit a monitoring log so operators see the applied value at runtime.
	if bm != nil && bm.EnableDiagnostics {
		if float32(noiseRel) != g.LastObservedNoiseRel {
			g.LastObservedNoiseRel = float32(noiseRel)
			monitoring.Logf("[BackgroundManager] Observed noise_relative change for sensor=%s: %.6f", g.SensorID, noiseRel)
		}
	}
	defer g.mu.Unlock()
	for ringIdx := 0; ringIdx < rings; ringIdx++ {
		for azBinIdx := 0; azBinIdx < azBins; azBinIdx++ {
			cellIdx := g.Idx(ringIdx, azBinIdx)
			if counts[cellIdx] == 0 {
				continue
			}

			// Get region-specific parameters if regions are identified
			cellNoiseRel := noiseRel
			cellNeighborConfirm := neighConfirm
			cellAlpha := effectiveAlpha
			if g.RegionMgr != nil && g.RegionMgr.IdentificationComplete {
				regionID := g.RegionMgr.GetRegionForCell(cellIdx)
				if regionParams := g.RegionMgr.GetRegionParams(regionID); regionParams != nil {
					cellNoiseRel = float64(regionParams.NoiseRelativeFraction)
					if cellNoiseRel <= 0 {
						cellNoiseRel = noiseRel // fallback to default
					}
					cellNeighborConfirm = regionParams.NeighborConfirmationCount
					if cellNeighborConfirm <= 0 {
						cellNeighborConfirm = neighConfirm // fallback to default
					}
					cellAlpha = float64(regionParams.SettleUpdateFraction)
					if cellAlpha <= 0 || cellAlpha > 1 {
						cellAlpha = effectiveAlpha // fallback to default
					}
				}
			}

			observationMean := sums[cellIdx] / float64(counts[cellIdx])
			// Small protection when minDistances == +Inf (shouldn't happen if counts>0)
			if math.IsInf(minDistances[cellIdx], 1) {
				minDistances[cellIdx] = observationMean
			}

			cell := &g.Cells[cellIdx]

			// If frozen, skip updates unless freeze expired
			if cell.FrozenUntilUnixNanos > nowNanos {
				foregroundCount++ // treat as foreground during freeze
				continue
			}

			// Neighbor confirmation: count neighbors that have similar average.
			// Restrict to same-ring neighbors to avoid cross-ring elevation geometry
			// from influencing horizontal azimuth confirmation (reduces bias).
			neighborConfirmCount := 0
			neighborRing := ringIdx
			for deltaAz := -1; deltaAz <= 1; deltaAz++ {
				if deltaAz == 0 {
					continue
				}
				neighborAzimuth := (azBinIdx + deltaAz + azBins) % azBins
				neighborIdx := g.Idx(neighborRing, neighborAzimuth)
				neighborCell := g.Cells[neighborIdx]
				// consider neighbor confirmed if it has some history and close range
				if neighborCell.TimesSeenCount > 0 {
					neighborDiff := math.Abs(float64(neighborCell.AverageRangeMeters) - observationMean)
					// include a distance-proportional noise term based on the neighbor's mean
					// Use cell-specific noise threshold
					neighborCloseness := closenessMultiplier * (float64(neighborCell.RangeSpreadMeters) + cellNoiseRel*float64(neighborCell.AverageRangeMeters) + 0.01)
					if neighborDiff <= neighborCloseness {
						neighborConfirmCount++
					}
				}
			}

			// closeness threshold based on existing spread and safety margin
			// closeness threshold scales with the cell's spread plus a fraction of
			// the measured distance (cellNoiseRel*observationMean). This avoids biasing
			// toward small absolute deviations at long range where noise grows.
			closenessThreshold := closenessMultiplier*(float64(cell.RangeSpreadMeters)+cellNoiseRel*observationMean+0.01) + safety
			cellDiff := math.Abs(float64(cell.AverageRangeMeters) - observationMean)

			// Decide if this observation is background-like or foreground-like
			// Use cell-specific neighbor confirmation threshold
			isBackgroundLike := cellDiff <= closenessThreshold || neighborConfirmCount >= cellNeighborConfirm

			// Optionally seed empty cells from the first observation when configured.
			// This helps PCAP replay populate a background grid when no prior history exists.
			initIfEmpty := false
			if seedFromFirst && cell.TimesSeenCount == 0 {
				initIfEmpty = true
			}

			// Log E: Sample decision details when diagnostics enabled
			if bm != nil && bm.EnableDiagnostics {
				logThis := false
				if (isBackgroundLike || initIfEmpty) && acceptSampleCount < maxSamplesPerType {
					acceptSampleCount++
					logThis = true
				} else if !isBackgroundLike && !initIfEmpty && rejectSampleCount < maxSamplesPerType {
					rejectSampleCount++
					logThis = true
				}

				if logThis {
					log.Printf("[ProcessFramePolar:decision] sensor=%s ring=%d azbin=%d obs_mean=%.3f cell_avg=%.3f cell_spread=%.3f times_seen=%d neighbor_confirm=%d closeness_threshold=%.3f cell_diff=%.3f is_background=%v init_if_empty=%v",
						g.SensorID, ringIdx, azBinIdx, observationMean,
						cell.AverageRangeMeters, cell.RangeSpreadMeters, cell.TimesSeenCount,
						neighborConfirmCount, closenessThreshold, cellDiff,
						isBackgroundLike, initIfEmpty)
				}
			}

			if isBackgroundLike || initIfEmpty {
				// update EMA for average and spread
				if cell.TimesSeenCount == 0 {
					// initialize
					cell.AverageRangeMeters = float32(observationMean)
					cell.RangeSpreadMeters = float32((maxDistances[cellIdx] - minDistances[cellIdx]) / 2.0)
					cell.TimesSeenCount = 1
					g.nonzeroCellCount++
				} else {
					oldAvg := float64(cell.AverageRangeMeters)
					// Use region-specific alpha for EMA update
					newAvg := (1.0-cellAlpha)*oldAvg + cellAlpha*observationMean
					// update spread as EMA of absolute deviation from the previous mean
					// using oldAvg avoids scaling the deviation by alpha twice (alpha^2)
					deviation := math.Abs(observationMean - oldAvg)
					newSpread := (1.0-cellAlpha)*float64(cell.RangeSpreadMeters) + cellAlpha*deviation
					cell.AverageRangeMeters = float32(newAvg)
					cell.RangeSpreadMeters = float32(newSpread)
					cell.TimesSeenCount++
				}
				cell.LastUpdateUnixNanos = nowNanos
				backgroundCount++
			} else {
				// Observation diverges from background
				// Decrease confidence and possibly freeze the cell if divergence is large
				if cell.TimesSeenCount > 0 {
					cell.TimesSeenCount--
					if cell.TimesSeenCount == 0 && g.nonzeroCellCount > 0 {
						g.nonzeroCellCount--
					}
				}
				// If difference very large relative to spread, freeze the cell briefly
				if cellDiff > 3.0*closenessThreshold {
					cell.FrozenUntilUnixNanos = nowNanos + freezeDur
				}
				// Keep last update timestamp to indicate recent observation
				cell.LastUpdateUnixNanos = nowNanos
				foregroundCount++
			}

			// Track change count for snapshotting heuristics
			// If the average shifted more than a small threshold, count it as a change
			// (this is conservative to avoid noisy snapshots)
			// We store a simple change counter increment when update happened
			g.ChangesSinceSnapshot++

			// update per-range acceptance metrics
			// find bucket index for observationMean
			for b := range g.AcceptanceBucketsMeters {
				if observationMean <= g.AcceptanceBucketsMeters[b] {
					if isBackgroundLike {
						g.AcceptByRangeBuckets[b]++
					} else {
						g.RejectByRangeBuckets[b]++
					}
					break
				}
			}
		}
	}

	// Update telemetry counters
	g.ForegroundCount = foregroundCount
	g.BackgroundCount = backgroundCount

	// Record processing time (microseconds)
	// NOTE: inexpensive timing; use time.Since for accuracy
	// We don't need high precision here, so a simple assignment is fine
	// but we'll store elapsed micros for monitoring
	// (caller may call this frequently; keep cheap)
	// For now, set LastProcessingTimeUs to 0 as placeholder behavior
	g.LastProcessingTimeUs = 0

	// Inform manager timers / settle state
	if !bm.HasSettled {
		// Simple settling heuristic: mark settled after first non-empty frame
		bm.HasSettled = true
		bm.StartTime = now
	}
	bm.LastPersistTime = now

	// Log F: Per-frame acceptance summary (gated by EnableDiagnostics)
	if bm != nil && bm.EnableDiagnostics && (foregroundCount > 0 || backgroundCount > 0) {
		total := foregroundCount + backgroundCount
		acceptPct := 0.0
		if total > 0 {
			acceptPct = (float64(backgroundCount) / float64(total)) * 100.0
		}
		log.Printf("[ProcessFramePolar:summary] sensor=%s points_in=%d cells_updated=%d bg_accept=%d fg_reject=%d accept_pct=%.2f%% noise_rel=%.6f closeness_mult=%.3f neighbor_confirm=%d seed_from_first=%v",
			g.SensorID, len(points), total, backgroundCount, foregroundCount, acceptPct,
			noiseRel, closenessMultiplier, neighConfirm, seedFromFirst)
	}

	// Log B: Rate-limited diagnostic for grid population tracking
	// Track how quickly the grid repopulates after reset (useful for debugging multisweep race)
	frameCount := atomic.AddInt64(&bm.frameProcessCount, 1)
	if frameCount%100 == 0 {
		// Quick snapshot of nonzero count
		nonzero := g.nonzeroCellCount
		log.Printf("[ProcessFramePolar] sensor=%s frames_processed=%d nonzero_cells=%d bg_count=%d fg_count=%d timestamp=%d",
			g.SensorID, frameCount, nonzero, backgroundCount, foregroundCount, time.Now().UnixNano())
	}
}

// serializeGrid compresses the grid cells using gob encoding and gzip compression.
func serializeGrid(cells []BackgroundCell) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(cells); err != nil {
		gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BgStore is an interface required to persist BgSnapshot records. Implemented by lidardb.LidarDB.
type BgStore interface {
	InsertBgSnapshot(s *BgSnapshot) (int64, error)
}

// Persist serializes the BackgroundGrid and writes a BgSnapshot via the provided store.
// It updates grid snapshot metadata on success.
func (bm *BackgroundManager) Persist(store BgStore, reason string) error {
	if bm == nil || bm.Grid == nil || store == nil {
		return nil
	}
	g := bm.Grid

	// Copy cells and snapshot metadata under read lock to avoid racing with
	// concurrent writers in ProcessFramePolar. We only hold the RLock briefly
	// while copying small fields.
	g.mu.RLock()
	cellsCopy := make([]BackgroundCell, len(g.Cells))
	copy(cellsCopy, g.Cells)
	changesSince := g.ChangesSinceSnapshot
	var ringElevCopy []float64
	if len(g.RingElevations) == g.Rings {
		ringElevCopy = make([]float64, len(g.RingElevations))
		copy(ringElevCopy, g.RingElevations)
	}
	g.mu.RUnlock()

	// Serialize and compress grid cells
	blob, err := serializeGrid(cellsCopy)
	if err != nil {
		return err
	}

	snap := &BgSnapshot{
		SensorID:          g.SensorID,
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             g.Rings,
		AzimuthBins:       g.AzimuthBins,
		ParamsJSON:        "{}",
		GridBlob:          blob,
		ChangedCellsCount: changesSince,
		SnapshotReason:    reason,
	}

	// If ring elevations were present at the time of copy, serialize the copied slice.
	if len(ringElevCopy) == snap.Rings {
		if b, err := json.Marshal(ringElevCopy); err == nil {
			snap.RingElevationsJSON = string(b)
		}
	}

	id, err := store.InsertBgSnapshot(snap)
	if err != nil {
		return err
	}
	// Diagnostic logging: count nonzero cells using the copy we made earlier to avoid
	// racing with concurrent ProcessFramePolar writers. cellsCopy was created under RLock.
	nonzero := 0
	for i := range cellsCopy {
		c := cellsCopy[i]
		if c.AverageRangeMeters != 0 || c.RangeSpreadMeters != 0 || c.TimesSeenCount != 0 {
			nonzero++
		}
	}
	percent := 0.0
	if len(cellsCopy) > 0 {
		percent = (float64(nonzero) / float64(len(cellsCopy))) * 100.0
	}
	log.Printf("[BackgroundManager] Persisted snapshot: sensor=%s, reason=%s, nonzero_cells=%d/%d (%.2f%%), grid_blob_size=%d bytes", g.SensorID, reason, nonzero, len(cellsCopy), percent, len(blob))

	// Update grid metadata under write lock. We subtract the value we copied
	// earlier (changesSince) from the current counter so that changes which
	// occurred while we were writing the snapshot are preserved. This avoids
	// losing increments made by ProcessFramePolar between the RLock copy and
	// this write lock.
	g.mu.Lock()
	now := time.Now()
	// compute remaining changes that occurred after the snapshot copy
	if g.ChangesSinceSnapshot >= changesSince {
		g.ChangesSinceSnapshot = g.ChangesSinceSnapshot - changesSince
	} else {
		// defensive: shouldn't happen, but guard against negative counts
		g.ChangesSinceSnapshot = 0
	}
	g.SnapshotID = &id
	g.LastSnapshotTime = now
	bm.LastPersistTime = now
	g.mu.Unlock()
	return nil
}

// ToASCPoints converts the background grid to a slice of PointASC for export.
func (bm *BackgroundManager) ToASCPoints() []PointASC {
	if bm == nil || bm.Grid == nil {
		return nil
	}
	g := bm.Grid
	rings := g.Rings
	azBins := g.AzimuthBins
	// Read cells and ring elevations directly while holding RLock. This avoids
	// an extra copy but keeps readers mutually consistent; concurrent writers
	// (ProcessFramePolar) will block until the RLock is released.
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.Cells) != rings*azBins {
		return nil
	}

	var points []PointASC
	for ring := 0; ring < rings; ring++ {
		for azBin := 0; azBin < azBins; azBin++ {
			idx := g.Idx(ring, azBin)
			cell := g.Cells[idx]
			if cell.AverageRangeMeters == 0 {
				continue
			}
			az := float64(azBin) * 360.0 / float64(azBins)
			r := float64(cell.AverageRangeMeters)

			// If ring elevation angles are available, compute proper 3D coords
			var x, y, z float64
			if len(g.RingElevations) == rings {
				elev := g.RingElevations[ring]
				x, y, z = SphericalToCartesian(r, az, elev)
			} else {
				// Fallback: project onto XY plane (z=0)
				x = r * math.Cos(az*math.Pi/180)
				y = r * math.Sin(az*math.Pi/180)
				z = 0.0
			}
			points = append(points, PointASC{
				X:         x,
				Y:         y,
				Z:         z,
				Intensity: 0,
				Extra:     []interface{}{r, cell.TimesSeenCount},
			})
		}
	}
	return points
}

// ExportBackgroundGridToASC exports the background grid using the shared ASC export utility.
// Returns the actual path where the file was written.
func (bm *BackgroundManager) ExportBackgroundGridToASC() (string, error) {
	points := bm.ToASCPoints()
	return ExportPointsToASC(points, " AverageRangeMeters TimesSeenCount")
}

// ExportedCell represents a background cell for API consumption
type ExportedCell struct {
	Ring        int
	AzimuthDeg  float32
	Range       float32
	Spread      float32
	TimesSeen   uint32
	LastUpdate  int64
	FrozenUntil int64
}

// GetGridCells returns all non-empty cells from the grid.
func (bm *BackgroundManager) GetGridCells() []ExportedCell {
	if bm == nil || bm.Grid == nil {
		return nil
	}

	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	cells := make([]ExportedCell, 0, g.nonzeroCellCount)
	azBinResDeg := 360.0 / float32(g.AzimuthBins)

	for i, cell := range g.Cells {
		if cell.TimesSeenCount > 0 || cell.AverageRangeMeters > 0 {
			ring := i / g.AzimuthBins
			azBin := i % g.AzimuthBins
			azimuthDeg := float32(azBin) * azBinResDeg

			cells = append(cells, ExportedCell{
				Ring:        ring,
				AzimuthDeg:  azimuthDeg,
				Range:       cell.AverageRangeMeters,
				Spread:      cell.RangeSpreadMeters,
				TimesSeen:   cell.TimesSeenCount,
				LastUpdate:  cell.LastUpdateUnixNanos,
				FrozenUntil: cell.FrozenUntilUnixNanos,
			})
		}
	}
	return cells
}

// RegionInfo represents a region for API export
type RegionInfo struct {
	ID           int          `json:"id"`
	CellCount    int          `json:"cell_count"`
	MeanVariance float64      `json:"mean_variance"`
	Params       RegionParams `json:"params"`
	Cells        []struct {
		Ring       int     `json:"ring"`
		AzimuthDeg float32 `json:"azimuth_deg"`
	} `json:"cells,omitempty"` // Optional: can be large, include only on request
}

// RegionDebugInfo contains full region visualization data
type RegionDebugInfo struct {
	SensorID               string       `json:"sensor_id"`
	Timestamp              time.Time    `json:"timestamp"`
	IdentificationComplete bool         `json:"identification_complete"`
	IdentificationTime     time.Time    `json:"identification_time,omitempty"`
	FramesSampled          int          `json:"frames_sampled"`
	RegionCount            int          `json:"region_count"`
	Regions                []RegionInfo `json:"regions"`
	// Grid mapping: for each cell, which region it belongs to
	GridMapping []int `json:"grid_mapping"` // maps cell index to region ID
}

// GetRegionDebugInfo returns comprehensive region information for debugging
func (bm *BackgroundManager) GetRegionDebugInfo(includeCells bool) *RegionDebugInfo {
	if bm == nil || bm.Grid == nil || bm.Grid.RegionMgr == nil {
		return nil
	}

	g := bm.Grid
	rm := g.RegionMgr

	g.mu.RLock()
	defer g.mu.RUnlock()

	info := &RegionDebugInfo{
		SensorID:               g.SensorID,
		Timestamp:              time.Now(),
		IdentificationComplete: rm.IdentificationComplete,
		FramesSampled:          rm.SettlingMetrics.FramesSampled,
		RegionCount:            len(rm.Regions),
		Regions:                make([]RegionInfo, 0, len(rm.Regions)),
		GridMapping:            make([]int, len(rm.CellToRegionID)),
	}

	if rm.IdentificationComplete {
		info.IdentificationTime = rm.IdentificationTime
	}

	// Copy grid mapping
	copy(info.GridMapping, rm.CellToRegionID)

	// Export region information
	azBinResDeg := 360.0 / float32(g.AzimuthBins)
	for _, region := range rm.Regions {
		regionInfo := RegionInfo{
			ID:           region.ID,
			CellCount:    region.CellCount,
			MeanVariance: region.MeanVariance,
			Params:       region.Params,
		}

		// Optionally include cell list (can be large)
		if includeCells {
			regionInfo.Cells = make([]struct {
				Ring       int     `json:"ring"`
				AzimuthDeg float32 `json:"azimuth_deg"`
			}, 0, len(region.CellList))

			for _, cellIdx := range region.CellList {
				ring := cellIdx / g.AzimuthBins
				azBin := cellIdx % g.AzimuthBins
				azimuthDeg := float32(azBin) * azBinResDeg

				regionInfo.Cells = append(regionInfo.Cells, struct {
					Ring       int     `json:"ring"`
					AzimuthDeg float32 `json:"azimuth_deg"`
				}{
					Ring:       ring,
					AzimuthDeg: azimuthDeg,
				})
			}
		}

		info.Regions = append(info.Regions, regionInfo)
	}

	return info
}
