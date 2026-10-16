package l3grid

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sync"
	"time"
)

// BackgroundParams configuration matching the param storage approach in schema
type BackgroundParams struct {
	BackgroundUpdateFraction       float32 // e.g., 0.02
	ClosenessSensitivityMultiplier float32 // e.g., 3.0
	SafetyMarginMetres             float32 // e.g., 0.5
	FreezeDurationNanos            int64   // e.g., 5e9 (5s)
	FreezeThresholdMultiplier      float32 // e.g., 3.0
	NeighbourConfirmationCount     int     // e.g., 5 of 8 neighbours
	WarmupDurationNanos            int64   // settle-time ceiling: settle by here regardless
	WarmupMinFrames                int     // optional minimum frames before considering settled
	// SettlingThresholds, when set, lets a grid finish settling as soon as it
	// has demonstrably converged rather than waiting out WarmupDurationNanos.
	// The duration becomes a ceiling for scenes that never converge — busy
	// ones, or a sensor watching moving traffic — rather than a fixed cost
	// every scene pays. WarmupMinFrames is still required either way.
	SettlingThresholds *SettlingThresholds
	// SettlingCheckInterval is how many frames apart convergence is evaluated.
	// Evaluation walks every cell, so it is not run per frame. Zero means the
	// default.
	SettlingCheckInterval      int
	PostSettleUpdateFraction   float32 // optional lower alpha after settle for stability
	ForegroundMinClusterPoints int     // min points for a cluster to be forwarded/considered
	ForegroundDBSCANEps        float32 // clustering radius for foreground gating
	// ForegroundMaxInputPoints caps the number of points fed into the core DBSCAN
	// loop. When the input exceeds this value, uniform random subsampling is
	// applied to keep runtime bounded. If this value is zero or negative, a
	// sensible default cap of 8000 points is applied.
	ForegroundMaxInputPoints int
	// NoiseRelativeFraction is the fraction of range (distance) to treat as
	// expected measurement noise. This allows closeness thresholds to grow
	// with distance so that farther returns (which naturally have larger
	// absolute noise) aren't biased as foreground. Typical values: 0.01
	// (1%) to 0.02 (2%). If zero, a sensible default (0.01) is used.
	NoiseRelativeFraction float32

	// DisableRegionOverrides makes every cell use the global
	// NoiseRelativeFraction, NeighbourConfirmationCount and update fraction
	// even after regions have been identified (gap analysis B8). With it off,
	// the shipped behaviour, a settled cell runs at its region's values:
	// NoiseRelativeFraction x4 for the most stable third of cells, x3 for the
	// middle third and x8 for the most variable, fixed from the config in
	// force when settling completed, then persisted and restored. A later
	// change to the global value never reaches such a cell. Regions are still
	// identified and persisted with the option on; only their parameter
	// overrides are ignored. Go-level and default false, so the config
	// fingerprint does not move.
	DisableRegionOverrides bool

	// RegionOverrideScope limits the region overrides to one side of the
	// variance terciles: RegionOverrideScopeVolatile keeps them for the
	// volatile tercile only (x8 noise, +2 neighbours, halved update fraction)
	// and serves the globals to the stable and variable terciles;
	// RegionOverrideScopeQuiet keeps the x4 and x3 overrides of the stable
	// and variable terciles and serves the globals to the volatile one. The
	// beam-wise review proposed the first as its revised next arm; the second
	// is its inverse, so the two attribute the recall the overrides cost.
	// Volatility is the region's tercile at identification, which a restored
	// region does not carry, so after a restore every region reads as quiet.
	// DisableRegionOverrides wins when both are set. Go-level and default
	// RegionOverrideScopeAll, so the config fingerprint does not move.
	RegionOverrideScope RegionOverrideScope

	// RegionOverridesByCellTercile makes a settled cell run at the override
	// parameters of its own variance tercile (RegionManager.CellTercile at
	// identification) instead of its merged region's. Merging moves cells
	// into regions of another tercile (about 5,000 of kirk0's into volatile
	// ones); with this on, a stable cell merged into a volatile region keeps
	// the stable tercile's x4, and a volatile cell merged into a stable one
	// gets the volatile tercile's x8. The parameters for each tercile are the
	// ones identification assigned a region of that tercile, fixed from the
	// config in force then (RegionManager.TercileParams). A cell with no
	// tercile, unseen while the grid settled, runs at the globals; so does
	// every cell after a restore, which records no cell terciles.
	// RegionOverrideScope then tests the cell's own tercile, and
	// DisableRegionOverrides wins. Go-level and default false, so the config
	// fingerprint does not move (replay experiment
	// region_overrides_cell_tercile, l3bg0010).
	RegionOverridesByCellTercile bool

	// VolatileWindowCapMetres, when positive, bounds the foreground
	// acceptance windows of a cell whose parameters come from a volatile
	// tercile's override (its region's, or its own under
	// RegionOverridesByCellTercile): the closeness window and the
	// locked-baseline window are each capped at the larger of the window the
	// global parameters give that cell at that range and this many metres,
	// so the x8 relative window no longer grows several metres deep with
	// range. Neighbour confirmation's window is capped the same way, from the
	// cell under test's override. The cap never widens a window, so it cannot
	// take a cell below its global window; the freeze and stale-model
	// thresholds keep the uncapped closeness window, and the neighbour count
	// and update fraction overrides are untouched. Zero is off. Go-level and
	// default zero, so the config fingerprint does not move (replay
	// experiments volatile_window_cap_1m and volatile_window_cap_2m,
	// l3bg0010).
	VolatileWindowCapMetres float64

	// SupportFilter, when set, runs the range-image support filter on the
	// per-point foreground mask after every cell has been updated (the
	// beam-wise review's E4): a cell keeps its foreground only when enough
	// neighbouring cells hold foreground at a similar range, so isolated
	// speckle and flicker become background without touching the model.
	// Go-level and default nil, so the config fingerprint does not move; the
	// replay experiment support_filter sets it to DefaultSupportFilterParams.
	SupportFilter *SupportFilterParams

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

	// M3.5 Split Streaming parameters
	// SensorMovementForegroundThreshold is the fraction of points that must be
	// classified as foreground to trigger sensor movement detection. Default: 0.20 (20%).
	SensorMovementForegroundThreshold float32
	// SensorMovementDriftRatioThreshold is the fraction of settled cells whose
	// range has drifted from its locked baseline that indicates sustained sensor
	// motion (driving). Robust to busy parked scenes, which shift only a small
	// fraction of cells. Default: 0.5.
	SensorMovementDriftRatioThreshold float32
	// BackgroundDriftThresholdMetres is the drift distance in metres that indicates
	// a cell has drifted significantly. Default: 0.5m.
	BackgroundDriftThresholdMetres float32
	// BackgroundDriftRatioThreshold is the fraction of settled cells that must have
	// drifted to consider the entire background drifted. Default: 0.10 (10%).
	BackgroundDriftRatioThreshold float32

	// Settling convergence thresholds.
	SettlingMinCoverage        float32
	SettlingMaxSpreadDelta     float32
	SettlingMinRegionStability float32
	SettlingMinConfidence      float32

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

// RegionOverrideScope selects which variance terciles keep their region
// overrides once regions are identified. See BackgroundParams.RegionOverrideScope.
type RegionOverrideScope int

const (
	// RegionOverrideScopeAll is the shipped behaviour: every region overrides.
	RegionOverrideScopeAll RegionOverrideScope = iota
	// RegionOverrideScopeVolatile keeps the overrides of the volatile tercile only.
	RegionOverrideScopeVolatile
	// RegionOverrideScopeQuiet keeps the overrides of the stable and variable terciles only.
	RegionOverrideScopeQuiet
)

// RegionParams defines parameters that can vary per region
type RegionParams struct {
	NoiseRelativeFraction      float32 `json:"noise_relative_fraction"`     // noise threshold for this region
	NeighbourConfirmationCount int     `json:"neighbor_confirmation_count"` // neighbour confirmation for this region
	SettleUpdateFraction       float32 `json:"settle_update_fraction"`      // alpha during settling for this region
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
	// Volatile marks a region identified from the high-variance tercile.
	// It is not restored: a region restored from a snapshot reads false.
	Volatile bool
	// Tercile is the variance tercile the region was identified from. A
	// merged region keeps the tercile of the region that absorbed it, as it
	// keeps that region's parameters and Volatile flag. It is evidence only:
	// it is persisted in the region snapshot, never read by the overrides,
	// and not restored, so a restored region reads TercileNone.
	Tercile VarianceTercile
}

// VarianceTercile is the settling-spread tercile of a cell, or of the region
// it seeded, when regions were identified. It is persisted as its number.
type VarianceTercile int8

const (
	// TercileNone means no tercile: the cell was not seen while settling,
	// or none was recorded (a default region, a restored region).
	TercileNone VarianceTercile = iota
	// TercileStable is the lowest third of settling spread.
	TercileStable
	// TercileVariable is the middle third.
	TercileVariable
	// TercileVolatile is the highest third (trees, glass, and whatever
	// moved through a cell while the grid settled).
	TercileVolatile
)

// String names the tercile as the region code does.
func (t VarianceTercile) String() string {
	switch t {
	case TercileStable:
		return "stable"
	case TercileVariable:
		return "variable"
	case TercileVolatile:
		return "volatile"
	}
	return "none"
}

// varianceTercileForCategory maps IdentifyRegions' category codes (0 stable,
// 1 variable, 2 volatile, -1 unseen) to a VarianceTercile.
func varianceTercileForCategory(category int) VarianceTercile {
	if category < 0 || category > 2 {
		return TercileNone
	}
	return VarianceTercile(category + 1)
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
	// CellTercile is each cell's own variance tercile at identification,
	// before small regions were merged into their neighbours, and
	// TercileThresholds the two settling-spread cut points that split them
	// (a cell at or above a cut point falls in the tercile above it). Both
	// are persisted with the region snapshot as evidence; the pipeline reads
	// CellTercile only under BackgroundParams.RegionOverridesByCellTercile.
	// Nil until identification, and after a restore.
	CellTercile       []VarianceTercile
	TercileThresholds []float64
	// TercileParams are the override parameters identification assigns a
	// region of each tercile, indexed by VarianceTercile-1 (stable, variable,
	// volatile). Every region's Params are copied from here, so a cell
	// served by its own tercile under RegionOverridesByCellTercile runs at
	// exactly what a region of that tercile runs at. In memory only, never
	// persisted; nil until identification and after a restore.
	TercileParams []RegionParams
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
	// settlingCheckCounter paces convergence evaluation during warm-up.
	settlingCheckCounter int
	// settlingAnnounced ensures the settling plan is logged once per warm-up.
	settlingAnnounced bool
	SettlingComplete  bool

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
	// regionRestoreAttempted is set to true after the first attempt to restore
	// regions from the database during settling, to avoid repeated DB lookups.
	regionRestoreAttempted bool

	// prevSpreads stores per-cell RangeSpreadMeters from the previous
	// EvaluateSettling call for computing SpreadDeltaRate.
	prevSpreads []float32
	// prevRegionIDs stores per-cell region assignments from the previous
	// EvaluateSettling call for computing RegionStability.
	prevRegionIDs []int
}

// Helper to index Cells: idx = ring*AzimuthBins + azBin
func (g *BackgroundGrid) Idx(ring, azBin int) int { return ring*g.AzimuthBins + azBin }

// SceneSignature generates a hash representing the background scene characteristics.
// This allows detecting whether a saved region snapshot matches the current scene.
// The hash is based on the distribution of cell ranges, coverage patterns, and variance.
// Two PCAP files from the same physical location should produce similar scene signatures.
func (g *BackgroundGrid) SceneSignature() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.sceneSignatureUnlocked()
}

// sceneSignatureUnlocked is the lock-free implementation of SceneSignature.
// Caller must hold g.mu (read or write lock).
func (g *BackgroundGrid) sceneSignatureUnlocked() string {
	if len(g.Cells) == 0 {
		return ""
	}

	// Build signature from:
	// 1. Grid dimensions (ensures same sensor config)
	// 2. Histogram of range values (captures scene geometry)
	// 3. Coverage pattern (which cells have data)
	// 4. Spread distribution (captures surface characteristics)

	// Quantise ranges into buckets (0-5m, 5-10m, 10-20m, 20-50m, 50-100m, >100m)
	rangeBuckets := [6]int{}
	spreadBuckets := [4]int{} // 0-0.05m, 0.05-0.1m, 0.1-0.2m, >0.2m
	coverage := 0

	for _, cell := range g.Cells {
		if cell.TimesSeenCount == 0 {
			continue
		}
		coverage++

		r := cell.AverageRangeMeters
		switch {
		case r < 5:
			rangeBuckets[0]++
		case r < 10:
			rangeBuckets[1]++
		case r < 20:
			rangeBuckets[2]++
		case r < 50:
			rangeBuckets[3]++
		case r < 100:
			rangeBuckets[4]++
		default:
			rangeBuckets[5]++
		}

		s := cell.RangeSpreadMeters
		switch {
		case s < 0.05:
			spreadBuckets[0]++
		case s < 0.1:
			spreadBuckets[1]++
		case s < 0.2:
			spreadBuckets[2]++
		default:
			spreadBuckets[3]++
		}
	}

	// Create a deterministic signature string
	sig := fmt.Sprintf("v1:%d:%d:%d:r%d.%d.%d.%d.%d.%d:s%d.%d.%d.%d",
		g.Rings, g.AzimuthBins, coverage,
		rangeBuckets[0], rangeBuckets[1], rangeBuckets[2],
		rangeBuckets[3], rangeBuckets[4], rangeBuckets[5],
		spreadBuckets[0], spreadBuckets[1], spreadBuckets[2], spreadBuckets[3])

	// Hash the signature for a fixed-length result
	h := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(h[:16]) // Use first 16 bytes (32 hex chars)
}

// effectiveCellParams returns the region-adaptive parameters for a given cell,
// falling back to the provided defaults when no region override is active.
// For NeighbourConfirmationCount: zero and negative values are treated as unset
// and defer to the default. This matches the original inline behaviour where
// <= 0 fell back to the global default — critical because many persisted
// region snapshots store 0 (the Go zero value) meaning "not explicitly set".
// Using 0 as "disable" would cause neighbourConfirmCount >= 0 to be always true
// in the foreground classifier, absorbing all points into the background.
//
// The overrides come from the cell's merged region, or, under
// RegionOverridesByCellTercile, from the cell's own tercile. volatile reports
// that they were a volatile tercile's, which is what VolatileWindowCapMetres
// bounds; it is false whenever the globals are served.
func (g *BackgroundGrid) effectiveCellParams(cellIdx int, defaultNoiseRel float64, defaultNeighbourConfirm int, defaultAlpha float64) (noiseRel float64, neighbourConfirm int, alpha float64, volatile bool) {
	noiseRel = defaultNoiseRel
	neighbourConfirm = defaultNeighbourConfirm
	alpha = defaultAlpha
	if g.Params.DisableRegionOverrides || g.RegionMgr == nil || !g.RegionMgr.IdentificationComplete {
		return
	}
	var regionParams *RegionParams
	var fromVolatile bool
	if g.Params.RegionOverridesByCellTercile {
		tercile := g.RegionMgr.GetCellTercile(cellIdx)
		regionParams = g.RegionMgr.GetTercileParams(tercile)
		fromVolatile = tercile == TercileVolatile
	} else {
		regionID := g.RegionMgr.GetRegionForCell(cellIdx)
		regionParams = g.RegionMgr.GetRegionParams(regionID)
		fromVolatile = g.RegionMgr.IsVolatileRegion(regionID)
	}
	switch g.Params.RegionOverrideScope {
	case RegionOverrideScopeVolatile:
		if !fromVolatile {
			return
		}
	case RegionOverrideScopeQuiet:
		if fromVolatile {
			return
		}
	}
	if regionParams == nil {
		return
	}
	volatile = fromVolatile
	if v := float64(regionParams.NoiseRelativeFraction); v > 0 {
		noiseRel = v
	}
	if v := regionParams.NeighbourConfirmationCount; v > 0 {
		neighbourConfirm = v
	}
	if v := float64(regionParams.SettleUpdateFraction); v > 0 && v <= 1 {
		alpha = v
	}
	return
}
