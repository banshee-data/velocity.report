package l3grid

import (
	"fmt"
	"math"
	"strings"
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
	return g.evaluateSettlingLocked(frameNumber)
}

// evaluateSettlingLocked is EvaluateSettling's body, for callers that already
// hold the grid lock. ProcessFramePolar decides settling completion while
// holding it, so it cannot go back through the exported form.
//
// The caller must hold g.mu for writing: this updates the delta-tracking state
// that makes SpreadDeltaRate and RegionStability frame-over-frame measures.
func (g *BackgroundGrid) evaluateSettlingLocked(frameNumber int) SettlingMetrics {
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

// defaultSettlingCheckInterval is how many frames apart convergence is
// evaluated when the caller does not choose. Evaluation walks every cell, so it
// is deliberately not run per frame; at ten frames a second this is about once
// a second, which is fine resolution for a decision measured in seconds.
const defaultSettlingCheckInterval = 10

// settlingConvergedLocked reports whether the grid has met its convergence
// criteria, evaluating at most once per SettlingCheckInterval frames.
//
// The caller must hold g.mu for writing. Returns false when no thresholds are
// configured, which leaves settling on the frame-count-and-duration rule alone.
func (g *BackgroundGrid) settlingConvergedLocked() bool {
	thresholds, ok := g.settlingThresholdsLocked()
	if !ok {
		return false
	}

	interval := g.Params.SettlingCheckInterval
	if interval <= 0 {
		interval = defaultSettlingCheckInterval
	}

	g.settlingCheckCounter++
	if g.settlingCheckCounter < interval {
		return false
	}
	g.settlingCheckCounter = 0

	metrics := g.evaluateSettlingLocked(0)
	converged := metrics.IsConverged(thresholds)

	// Report the measurement, not just the verdict. "Still settling" with no
	// numbers gives an operator nothing to act on, and the criteria are only
	// worth having if it is visible which one is holding completion up.
	if !converged {
		diagf("[BackgroundManager] Settling not converged for sensor=%s: %s",
			g.SensorID, unmetCriteria(metrics, thresholds))
	}
	return converged
}

// unmetCriteria names the thresholds a measurement has not reached, with the
// values, so the log says which one to look at rather than that something is
// unmet.
func unmetCriteria(m SettlingMetrics, t SettlingThresholds) string {
	var unmet []string
	if m.CoverageRate < t.MinCoverage {
		unmet = append(unmet, fmt.Sprintf("coverage %.3f < %.3f", m.CoverageRate, t.MinCoverage))
	}
	if m.SpreadDeltaRate > t.MaxSpreadDelta {
		unmet = append(unmet, fmt.Sprintf("spread_delta %.6f > %.6f", m.SpreadDeltaRate, t.MaxSpreadDelta))
	}
	if m.RegionStability < t.MinRegionStability {
		unmet = append(unmet, fmt.Sprintf("region_stability %.3f < %.3f", m.RegionStability, t.MinRegionStability))
	}
	if m.MeanConfidence < t.MinConfidence {
		unmet = append(unmet, fmt.Sprintf("mean_confidence %.1f < %.1f", m.MeanConfidence, t.MinConfidence))
	}
	if len(unmet) == 0 {
		return "all criteria met"
	}
	return strings.Join(unmet, ", ")
}

// settlingThresholdsLocked builds the convergence criteria from the grid's
// parameters, reporting false when they are not configured.
//
// All four must be set. A partially configured set would let a grid settle on
// whichever dimensions happened to be filled in, which is a worse guarantee
// than the duration it would be replacing.
func (g *BackgroundGrid) settlingThresholdsLocked() (SettlingThresholds, bool) {
	p := g.Params
	if p.SettlingMinCoverage <= 0 || p.SettlingMaxSpreadDelta <= 0 ||
		p.SettlingMinRegionStability <= 0 || p.SettlingMinConfidence <= 0 {
		return SettlingThresholds{}, false
	}
	return SettlingThresholds{
		MinCoverage:        float64(p.SettlingMinCoverage),
		MaxSpreadDelta:     float64(p.SettlingMaxSpreadDelta),
		MinRegionStability: float64(p.SettlingMinRegionStability),
		MinConfidence:      float64(p.SettlingMinConfidence),
	}, true
}
