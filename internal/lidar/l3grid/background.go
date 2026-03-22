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
	WarmupDurationNanos            int64   // optional extra settle time before emitting foreground
	WarmupMinFrames                int     // optional minimum frames before considering settled
	PostSettleUpdateFraction       float32 // optional lower alpha after settle for stability
	ForegroundMinClusterPoints     int     // min points for a cluster to be forwarded/considered
	ForegroundDBSCANEps            float32 // clustering radius for foreground gating
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
func (g *BackgroundGrid) effectiveCellParams(cellIdx int, defaultNoiseRel float64, defaultNeighbourConfirm int, defaultAlpha float64) (noiseRel float64, neighbourConfirm int, alpha float64) {
	noiseRel = defaultNoiseRel
	neighbourConfirm = defaultNeighbourConfirm
	alpha = defaultAlpha
	if g.RegionMgr == nil || !g.RegionMgr.IdentificationComplete {
		return
	}
	regionID := g.RegionMgr.GetRegionForCell(cellIdx)
	regionParams := g.RegionMgr.GetRegionParams(regionID)
	if regionParams == nil {
		return
	}
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
