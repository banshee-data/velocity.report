package l3grid

import (
	"math"
	"time"
)

// SettlingMetrics tracks convergence indicators during warmup.
// Each metric captures a different dimension of background model stability.
type SettlingMetrics struct {
	// CoverageRate is the fraction of cells with TimesSeenCount > 0.
	CoverageRate float64 `json:"coverage_rate"`
	// SpreadDeltaRate is the mean absolute frame-over-frame change in
	// RangeSpreadMeters across all observed cells. A low value indicates
	// that per-cell spread estimates have stabilised.
	SpreadDeltaRate float64 `json:"spread_delta_rate"`
	// RegionStability is 1 − (fraction of cells whose region assignment
	// differs from the previous evaluation). 1.0 means perfectly stable.
	RegionStability float64 `json:"region_stability"`
	// MeanConfidence is the mean TimesSeenCount across all observed cells.
	MeanConfidence float64 `json:"mean_confidence"`
	// EvaluatedAt is the wall-clock time of this evaluation.
	EvaluatedAt time.Time `json:"evaluated_at"`
	// FrameNumber is the logical frame index at evaluation time.
	FrameNumber int `json:"frame_number"`
}

// SettlingThresholds defines the convergence criteria.
// All four conditions must be met simultaneously for IsConverged to return true.
type SettlingThresholds struct {
	// MinCoverage is the minimum CoverageRate (e.g. 0.80 for 80 %).
	MinCoverage float64 `json:"min_coverage"`
	// MaxSpreadDelta is the maximum acceptable SpreadDeltaRate per frame.
	MaxSpreadDelta float64 `json:"max_spread_delta"`
	// MinRegionStability is the minimum RegionStability (e.g. 0.95).
	MinRegionStability float64 `json:"min_region_stability"`
	// MinConfidence is the minimum MeanConfidence (e.g. 10.0).
	MinConfidence float64 `json:"min_confidence"`
}

// DefaultSettlingThresholds returns conservative convergence thresholds
// suitable for a typical outdoor LiDAR scene.
func DefaultSettlingThresholds() SettlingThresholds {
	return SettlingThresholds{
		MinCoverage:        0.80,
		MaxSpreadDelta:     0.001,
		MinRegionStability: 0.95,
		MinConfidence:      10.0,
	}
}

// IsConverged returns true when every metric meets or exceeds its threshold.
func (m SettlingMetrics) IsConverged(t SettlingThresholds) bool {
	return m.CoverageRate >= t.MinCoverage &&
		m.SpreadDeltaRate <= t.MaxSpreadDelta &&
		m.RegionStability >= t.MinRegionStability &&
		m.MeanConfidence >= t.MinConfidence
}

// prevSpreads holds the per-cell RangeSpreadMeters from the previous
// EvaluateSettling call. It lives on BackgroundGrid so the delta calculation
// is stateful across consecutive evaluations.
// prevRegionIDs stores per-cell region assignments from the last evaluation.
// Both are lazily allocated by EvaluateSettling.

// EvaluateSettling computes convergence metrics for the current grid state.
// It is safe to call while the grid is actively processing frames; the method
// acquires a write lock internally because it updates delta-tracking state
// (prevSpreads, prevRegionIDs). Successive calls track frame-over-frame
// deltas for SpreadDeltaRate and RegionStability.
func (bm *BackgroundManager) EvaluateSettling(frameNumber int) SettlingMetrics {
	if bm == nil || bm.Grid == nil {
		return SettlingMetrics{FrameNumber: frameNumber, EvaluatedAt: time.Now()}
	}

	g := bm.Grid
	g.mu.Lock()
	defer g.mu.Unlock()

	total := len(g.Cells)
	if total == 0 {
		return SettlingMetrics{FrameNumber: frameNumber, EvaluatedAt: time.Now()}
	}

	// --- Coverage & confidence ---
	observed := 0
	var sumConfidence float64
	for i := range g.Cells {
		if g.Cells[i].TimesSeenCount > 0 {
			observed++
			sumConfidence += float64(g.Cells[i].TimesSeenCount)
		}
	}
	coverage := float64(observed) / float64(total)
	var meanConf float64
	if observed > 0 {
		meanConf = sumConfidence / float64(observed)
	}

	// --- Spread delta ---
	var spreadDelta float64
	curSpreads := make([]float32, total)
	for i := range g.Cells {
		curSpreads[i] = g.Cells[i].RangeSpreadMeters
	}
	if len(g.prevSpreads) == total {
		var sumDelta float64
		var deltaCount int
		for i := range curSpreads {
			if g.Cells[i].TimesSeenCount == 0 {
				continue
			}
			sumDelta += math.Abs(float64(curSpreads[i] - g.prevSpreads[i]))
			deltaCount++
		}
		if deltaCount > 0 {
			spreadDelta = sumDelta / float64(deltaCount)
		}
	}
	g.prevSpreads = curSpreads

	// --- Region stability ---
	regionStability := 1.0
	curRegionIDs := make([]int, total)
	if g.RegionMgr != nil && len(g.RegionMgr.CellToRegionID) == total {
		copy(curRegionIDs, g.RegionMgr.CellToRegionID)
	}
	if len(g.prevRegionIDs) == total {
		changed := 0
		for i := range curRegionIDs {
			if curRegionIDs[i] != g.prevRegionIDs[i] {
				changed++
			}
		}
		if total > 0 {
			regionStability = 1.0 - float64(changed)/float64(total)
		}
	}
	g.prevRegionIDs = curRegionIDs

	return SettlingMetrics{
		CoverageRate:    coverage,
		SpreadDeltaRate: spreadDelta,
		RegionStability: regionStability,
		MeanConfidence:  meanConf,
		EvaluatedAt:     time.Now(),
		FrameNumber:     frameNumber,
	}
}
