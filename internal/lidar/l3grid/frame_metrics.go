package l3grid

import "time"

// FrameSettlingMetrics is a point-in-time snapshot of grid settling state,
// suitable for per-frame motion/static classification by the pcap-split tool.
//
// All Percent* fields are fractions in [0,1] (consistent with GridHeatmap's
// fill_rate / settle_rate and SettlingMetrics.CoverageRate), not 0–100 values.
type FrameSettlingMetrics struct {
	TotalCells      int     `json:"total_cells"`
	NonzeroCells    int     `json:"nonzero_cells"`    // TimesSeenCount > 0
	SettledCells    int     `json:"settled_cells"`    // TimesSeenCount >= settledThreshold
	FrozenCells     int     `json:"frozen_cells"`     // FrozenUntilUnixNanos > now
	PercentNonzero  float64 `json:"percent_nonzero"`  // fraction in [0,1]
	PercentSettled  float64 `json:"percent_settled"`  // fraction in [0,1]
	PercentFrozen   float64 `json:"percent_frozen"`   // fraction in [0,1]
	ForegroundCount int64   `json:"foreground_count"` // cumulative, from last ProcessFramePolar
	BackgroundCount int64   `json:"background_count"` // cumulative, from last ProcessFramePolar
}

// noiseBoundsMajority is the fraction of observed cells that must fall within
// the expected noise envelope for IsWithinNoiseBounds to return true.
const noiseBoundsMajority = 0.9

// defaultNoiseRelativeFraction keeps the expected-noise floor non-degenerate
// when Params.NoiseRelativeFraction is unset.
const defaultNoiseRelativeFraction = 0.05

// GetFrameSettlingMetrics returns a snapshot of how settled the grid is, using
// settledThreshold as the minimum TimesSeenCount for a cell to count as
// "settled". It is read-only and safe to call concurrently with frame
// processing; the counts mirror those exposed by GridStatus and GetGridHeatmap.
func (bm *BackgroundManager) GetFrameSettlingMetrics(settledThreshold uint32) FrameSettlingMetrics {
	if bm == nil || bm.Grid == nil {
		return FrameSettlingMetrics{}
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	total := len(g.Cells)
	nowNanos := time.Now().UnixNano()

	var nonzero, settled, frozen int
	for i := range g.Cells {
		c := &g.Cells[i]
		if c.TimesSeenCount > 0 {
			nonzero++
			if c.TimesSeenCount >= settledThreshold {
				settled++
			}
		}
		if c.FrozenUntilUnixNanos > nowNanos {
			frozen++
		}
	}

	m := FrameSettlingMetrics{
		TotalCells:      total,
		NonzeroCells:    nonzero,
		SettledCells:    settled,
		FrozenCells:     frozen,
		ForegroundCount: g.ForegroundCount,
		BackgroundCount: g.BackgroundCount,
	}
	if total > 0 {
		inv := 1.0 / float64(total)
		m.PercentNonzero = float64(nonzero) * inv
		m.PercentSettled = float64(settled) * inv
		m.PercentFrozen = float64(frozen) * inv
	}
	return m
}

// expectedNoiseFloor returns the model's expected range spread (metres) for a
// cell at the given mean range: a small constant floor plus a range-proportional
// term. This mirrors the noise term in ProcessFramePolar's closeness test.
// Caller must hold g.mu.
func (g *BackgroundGrid) expectedNoiseFloor(rangeMeters float64) float64 {
	noiseRel := float64(g.Params.NoiseRelativeFraction)
	if noiseRel <= 0 {
		noiseRel = defaultNoiseRelativeFraction
	}
	return noiseRel*rangeMeters + 0.01
}

// GetNoiseBoundsDeviation returns the mean ratio of observed per-cell range
// spread to the expected noise floor, averaged over observed cells
// (TimesSeenCount > 0). A value near 1.0 indicates spreads in line with
// expectations; higher values indicate a noisier or less-settled scene.
// Returns 0 when no cells have been observed.
//
// NOTE: this is a proxy. BackgroundCell stores RangeSpreadMeters as an EMA of
// absolute deviation, not a true Welford variance, so the result is a
// spread-to-expectation ratio rather than a Gaussian sigma. Treat it as a
// relative indicator, not an absolute statistical bound.
func (bm *BackgroundManager) GetNoiseBoundsDeviation() float64 {
	if bm == nil || bm.Grid == nil {
		return 0
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	var sum float64
	var observed int
	for i := range g.Cells {
		c := &g.Cells[i]
		if c.TimesSeenCount == 0 {
			continue
		}
		expected := g.expectedNoiseFloor(float64(c.AverageRangeMeters))
		sum += float64(c.RangeSpreadMeters) / expected
		observed++
	}
	if observed == 0 {
		return 0
	}
	return sum / float64(observed)
}

// IsWithinNoiseBounds reports whether most observed cells have a range spread
// within `threshold` times their expected noise floor. "Most" means at least
// noiseBoundsMajority of observed cells. A grid with no observed cells is
// reported as within bounds. Like GetNoiseBoundsDeviation, this is a proxy over
// the EMA spread, not a true variance bound.
func (bm *BackgroundManager) IsWithinNoiseBounds(threshold float64) bool {
	if bm == nil || bm.Grid == nil {
		return true
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	var within, observed int
	for i := range g.Cells {
		c := &g.Cells[i]
		if c.TimesSeenCount == 0 {
			continue
		}
		observed++
		expected := g.expectedNoiseFloor(float64(c.AverageRangeMeters))
		if float64(c.RangeSpreadMeters) <= threshold*expected {
			within++
		}
	}
	if observed == 0 {
		return true
	}
	return float64(within) >= noiseBoundsMajority*float64(observed)
}
