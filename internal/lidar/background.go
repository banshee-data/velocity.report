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
	// NoiseRelativeFraction is the fraction of range (distance) to treat as
	// expected measurement noise. This allows closeness thresholds to grow
	// with distance so that farther returns (which naturally have larger
	// absolute noise) aren't biased as foreground. Typical values: 0.01
	// (1%) to 0.02 (2%). If zero, a sensible default (0.01) is used.
	NoiseRelativeFraction float32

	// Additional params for persistence matching schema requirements
	SettlingPeriodNanos        int64 // 5 minutes before first snapshot
	SnapshotIntervalNanos      int64 // 2 hours between snapshots
	ChangeThresholdForSnapshot int   // min changed cells to trigger snapshot
}

// BackgroundCell matches the compressed storage format for schema persistence
type BackgroundCell struct {
	AverageRangeMeters   float32
	RangeSpreadMeters    float32
	TimesSeenCount       uint32
	LastUpdateUnixNanos  int64
	FrozenUntilUnixNanos int64
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
}

// Helper to index Cells: idx = ring*AzimuthBins + azBin
func (g *BackgroundGrid) Idx(ring, azBin int) int { return ring*g.AzimuthBins + azBin }

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
		return nil
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

	for i := range g.Cells {
		g.Cells[i].AverageRangeMeters = 0
		g.Cells[i].RangeSpreadMeters = 0
		g.Cells[i].TimesSeenCount = 0
		g.Cells[i].LastUpdateUnixNanos = 0
		g.Cells[i].FrozenUntilUnixNanos = 0
	}
	for i := range g.AcceptByRangeBuckets {
		g.AcceptByRangeBuckets[i] = 0
		g.RejectByRangeBuckets[i] = 0
	}
	g.ChangesSinceSnapshot = 0
	g.ForegroundCount = 0
	g.BackgroundCount = 0
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
	for _, p := range points {
		ring := p.Channel - 1
		if ring < 0 || ring >= rings {
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
	alpha := float64(g.Params.BackgroundUpdateFraction)
	if alpha <= 0 || alpha > 1 {
		alpha = 0.02
	}
	// Defer reading runtime-tunable params that may be updated concurrently
	// (via setters which take g.mu) until we hold the grid lock to avoid
	// data races and inconsistent thresholds.
	var closenessMultiplier float64
	var neighConfirm int
	safety := float64(g.Params.SafetyMarginMeters)
	freezeDur := g.Params.FreezeDurationNanos

	// We'll read Params under lock so updates via SetNoiseRelativeFraction
	// and other setters are visible immediately and we can detect changes.

	foregroundCount := int64(0)
	backgroundCount := int64(0)

	// Iterate over observed cells and update grid
	g.mu.Lock()
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
					neighborCloseness := closenessMultiplier * (float64(neighborCell.RangeSpreadMeters) + noiseRel*float64(neighborCell.AverageRangeMeters) + 0.01)
					if neighborDiff <= neighborCloseness {
						neighborConfirmCount++
					}
				}
			}

			// closeness threshold based on existing spread and safety margin
			// closeness threshold scales with the cell's spread plus a fraction of
			// the measured distance (noiseRel*observationMean). This avoids biasing
			// toward small absolute deviations at long range where noise grows.
			closenessThreshold := closenessMultiplier*(float64(cell.RangeSpreadMeters)+noiseRel*observationMean+0.01) + safety
			cellDiff := math.Abs(float64(cell.AverageRangeMeters) - observationMean)

			// Decide if this observation is background-like or foreground-like
			isBackgroundLike := cellDiff <= closenessThreshold || neighborConfirmCount >= neighConfirm

			if isBackgroundLike {
				// update EMA for average and spread
				if cell.TimesSeenCount == 0 {
					// initialize
					cell.AverageRangeMeters = float32(observationMean)
					cell.RangeSpreadMeters = float32((maxDistances[cellIdx] - minDistances[cellIdx]) / 2.0)
					cell.TimesSeenCount = 1
				} else {
					oldAvg := float64(cell.AverageRangeMeters)
					newAvg := (1.0-alpha)*oldAvg + alpha*observationMean
					// update spread as EMA of absolute deviation from the previous mean
					// using oldAvg avoids scaling the deviation by alpha twice (alpha^2)
					deviation := math.Abs(observationMean - oldAvg)
					newSpread := (1.0-alpha)*float64(cell.RangeSpreadMeters) + alpha*deviation
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
	log.Printf("[BackgroundManager] Persisted snapshot: sensor=%s, reason=%s, nonzero_cells=%d/%d, grid_blob_size=%d bytes", g.SensorID, reason, nonzero, len(cellsCopy), len(blob))

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
func (bm *BackgroundManager) ExportBackgroundGridToASC(filePath string) error {
	points := bm.ToASCPoints()
	return ExportPointsToASC(points, filePath, " AverageRangeMeters TimesSeenCount")
}
