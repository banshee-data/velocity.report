package l3grid

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/monitoring"
)

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
	// store retains the BgStore reference for region persistence and restoration.
	// When the store also implements RegionStore, regions are persisted on settling
	// completion and can be restored on subsequent PCAP runs.
	store BgStore
	// EnableDiagnostics controls whether this manager emits high-frequency
	// trace / monitoring diagnostics (per-frame processing logs) via the
	// shared monitoring logger. Human-readable diagnostic logs are enabled
	// separately via SetLogWriters on the owning manager. Default: false.
	EnableDiagnostics bool
	// frameProcessCount tracks the number of ProcessFramePolar calls for rate-limited diagnostics.
	// Accessed atomically to allow concurrent ProcessFramePolar invocations.
	frameProcessCount int64

	// maskBuf is a reusable buffer for ProcessFramePolarWithMask to avoid
	// allocating a new []bool (69k elements, ~69 KB) every frame.
	// Safe because the pipeline callback runs synchronously on a single goroutine.
	maskBuf []bool

	// SourcePath stores the current data source path (e.g., PCAP filename).
	// Used for region restoration: matching by source path is faster and more
	// reliable than scene hash matching during early settling.
	// Protected by sourceMu.
	sourcePath string
	sourceMu   sync.RWMutex
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

// SetNeighbourConfirmationCount safely updates the NeighbourConfirmationCount parameter.
func (bm *BackgroundManager) SetNeighbourConfirmationCount(v int) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	g := bm.Grid
	g.mu.Lock()
	g.Params.NeighbourConfirmationCount = v
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

// SetSourcePath sets the current data source path (e.g., PCAP filename).
// This is used for region restoration: when processing the same PCAP file
// again, regions can be restored immediately by matching the source path.
func (bm *BackgroundManager) SetSourcePath(path string) {
	if bm == nil {
		return
	}
	bm.sourceMu.Lock()
	bm.sourcePath = path
	bm.sourceMu.Unlock()
	diagf("[BackgroundManager] Source path set to: %s", path)
}

// GetSourcePath returns the current data source path.
func (bm *BackgroundManager) GetSourcePath() string {
	if bm == nil {
		return ""
	}
	bm.sourceMu.RLock()
	defer bm.sourceMu.RUnlock()
	return bm.sourcePath
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

// IsSettlingComplete returns whether the background grid has completed settling.
// It acquires a read lock internally so it is safe to call concurrently with
// ProcessFramePolar.
func (bm *BackgroundManager) IsSettlingComplete() bool {
	if bm == nil || bm.Grid == nil {
		return false
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.SettlingComplete
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

	// Reset BackgroundManager timing state
	bm.StartTime = time.Time{} // Reset to zero value
	bm.HasSettled = false

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

	// Reset settling state for region identification
	g.SettlingComplete = false
	// Reset WarmupFramesRemaining to 0 so ProcessFramePolar reinitializes it on next call
	g.WarmupFramesRemaining = 0
	// Allow region restoration attempt on next settling cycle
	g.regionRestoreAttempted = false

	// Reset region manager to allow re-identification
	if g.RegionMgr != nil {
		g.RegionMgr = NewRegionManager(g.Rings, g.AzimuthBins)
	}

	// Count nonzero cells AFTER reset (should be 0)
	nonzeroAfter := 0
	for i := range g.Cells {
		if g.Cells[i].AverageRangeMeters != 0 || g.Cells[i].RangeSpreadMeters != 0 || g.Cells[i].TimesSeenCount != 0 {
			nonzeroAfter++
		}
	}

	// Log with timestamp and counts
	diagf("[ResetGrid] sensor=%s nonzero_before=%d nonzero_after=%d total_cells=%d timestamp=%d",
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
	mgr := &BackgroundManager{Grid: grid, store: store}
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
		opsf("BackgroundManager for sensor '%s' created without a BgStore: persistence disabled", sensorID)
	}

	RegisterBackgroundManager(sensorID, mgr)
	return mgr
}

// NewBackgroundManagerDI creates a BackgroundManager without registering it
// in the global registry. Prefer this constructor when wiring dependencies
// explicitly via pipeline.SensorRuntime.
func NewBackgroundManagerDI(sensorID string, rings, azBins int, params BackgroundParams, store BgStore) *BackgroundManager {
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
		RegionMgr:   NewRegionManager(rings, azBins),
	}
	mgr := &BackgroundManager{Grid: grid, store: store}
	grid.Manager = mgr

	grid.AcceptanceBucketsMeters = []float64{1, 2, 4, 8, 10, 12, 16, 20, 50, 100, 200}
	grid.AcceptByRangeBuckets = make([]int64, len(grid.AcceptanceBucketsMeters))
	grid.RejectByRangeBuckets = make([]int64, len(grid.AcceptanceBucketsMeters))

	if store != nil {
		mgr.PersistCallback = func(s *BgSnapshot) error {
			reason := "manual"
			if s != nil && s.SnapshotReason != "" {
				reason = s.SnapshotReason
			}
			return mgr.Persist(store, reason)
		}
	} else {
		opsf("BackgroundManager for sensor '%s' created without a BgStore: persistence disabled", sensorID)
	}

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
//   - Uses neighbour confirmation: updates are applied more readily when adjacent cells
//     agree (helps suppress isolated noise).
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) {
	if bm == nil || bm.Grid == nil || len(points) == 0 {
		return
	}

	// Quick diagnostics when enabled to see what's arriving
	if bm != nil && bm.EnableDiagnostics && len(points) > 0 {
		sample := points[0]
		tracef("[BackgroundManager] Received %d points; sample -> Channel=%d Az=%.2f Dist=%.2f", len(points), sample.Channel, sample.Azimuth, sample.Distance)
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
		opsf("[BackgroundManager] Skipped %d invalid points due to channel out-of-range (rings=%d)", skippedInvalid, rings)
	}
	// Declare variables that will be populated under the grid lock.
	var alpha, effectiveAlpha float64
	var closenessMultiplier float64
	var neighConfirm int
	var seedFromFirst bool
	var safety float64
	var freezeDur int64

	foregroundCount := int64(0)
	backgroundCount := int64(0)

	// Log E: Diagnostic sample counters for decision logging (gated by EnableDiagnostics)
	var acceptSampleCount, rejectSampleCount int
	const maxSamplesPerType = 10

	// Read all runtime-tunable params under the grid lock to avoid data
	// races with SetParams / tuning and to see a consistent snapshot.
	g.mu.Lock()
	alpha = float64(g.Params.BackgroundUpdateFraction)
	if alpha <= 0 || alpha > 1 {
		alpha = 0.02
	}
	effectiveAlpha = alpha
	safety = float64(g.Params.SafetyMarginMetres)
	freezeDur = g.Params.FreezeDurationNanos
	startTimeWasZero := bm.StartTime.IsZero()
	if startTimeWasZero {
		bm.StartTime = now
	}
	// Initialize WarmupFramesRemaining on first call if configured
	if startTimeWasZero && g.WarmupFramesRemaining == 0 && g.Params.WarmupMinFrames > 0 && !g.SettlingComplete {
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
					opsf("[BackgroundManager] Failed to identify regions: %v", err)
				}
				// Persist regions immediately so future runs can skip settling
				bm.persistRegionsOnSettleLocked()
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
			// Attempt early region restoration from DB
			if !g.regionRestoreAttempted && bm.store != nil {
				framesProcessed := g.Params.WarmupMinFrames - g.WarmupFramesRemaining
				if framesProcessed >= regionRestoreMinFrames {
					if bm.tryRestoreRegionsFromStoreLocked() {
						if postSettleAlpha > 0 && postSettleAlpha <= 1 {
							effectiveAlpha = postSettleAlpha
						}
					}
				}
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
	neighConfirm = g.Params.NeighbourConfirmationCount
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
			cellNoiseRel, cellNeighbourConfirm, cellAlpha := g.effectiveCellParams(cellIdx, noiseRel, neighConfirm, effectiveAlpha)

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

			// Neighbour confirmation: count neighbours that have similar average.
			// Restrict to same-ring neighbours to avoid cross-ring elevation geometry
			// from influencing horizontal azimuth confirmation (reduces bias).
			neighbourConfirmCount := 0
			neighbourRing := ringIdx
			for deltaAz := -1; deltaAz <= 1; deltaAz++ {
				if deltaAz == 0 {
					continue
				}
				neighbourAzimuth := (azBinIdx + deltaAz + azBins) % azBins
				neighbourIdx := g.Idx(neighbourRing, neighbourAzimuth)
				neighbourCell := g.Cells[neighbourIdx]
				// consider neighbour confirmed if it has some history and close range
				if neighbourCell.TimesSeenCount > 0 {
					neighbourDiff := math.Abs(float64(neighbourCell.AverageRangeMeters) - observationMean)
					// include a distance-proportional noise term based on the neighbour's mean
					// Use cell-specific noise threshold
					neighbourCloseness := closenessMultiplier * (float64(neighbourCell.RangeSpreadMeters) + cellNoiseRel*float64(neighbourCell.AverageRangeMeters) + 0.01)
					if neighbourDiff <= neighbourCloseness {
						neighbourConfirmCount++
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
			// Use cell-specific neighbour confirmation threshold
			isBackgroundLike := cellDiff <= closenessThreshold || neighbourConfirmCount >= cellNeighbourConfirm

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
					tracef("[ProcessFramePolar:decision] sensor=%s ring=%d azbin=%d obs_mean=%.3f cell_avg=%.3f cell_spread=%.3f times_seen=%d neighbour_confirm=%d closeness_threshold=%.3f cell_diff=%.3f is_background=%v init_if_empty=%v",
						g.SensorID, ringIdx, azBinIdx, observationMean,
						cell.AverageRangeMeters, cell.RangeSpreadMeters, cell.TimesSeenCount,
						neighbourConfirmCount, closenessThreshold, cellDiff,
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
		tracef("[ProcessFramePolar:summary] sensor=%s points_in=%d cells_updated=%d bg_accept=%d fg_reject=%d accept_pct=%.2f%% noise_rel=%.6f closeness_mult=%.3f neighbour_confirm=%d seed_from_first=%v",
			g.SensorID, len(points), total, backgroundCount, foregroundCount, acceptPct,
			noiseRel, closenessMultiplier, neighConfirm, seedFromFirst)
	}

	// Log B: Rate-limited diagnostic for grid population tracking
	// Track how quickly the grid repopulates after reset (useful for debugging multisweep race)
	frameCount := atomic.AddInt64(&bm.frameProcessCount, 1)
	if frameCount%100 == 0 {
		// Quick snapshot of nonzero count
		nonzero := g.nonzeroCellCount
		tracef("[ProcessFramePolar] sensor=%s frames_processed=%d nonzero_cells=%d bg_count=%d fg_count=%d timestamp=%d",
			g.SensorID, frameCount, nonzero, backgroundCount, foregroundCount, time.Now().UnixNano())
	}
}
