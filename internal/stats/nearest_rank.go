// Package stats holds the project's shared aggregate statistics.
//
// Aggregate speed percentiles (p50, p85, p98) are statistics over a
// population of vehicle maximum speeds, never over one track's observations.
// Every published aggregate uses NearestRank, so a report, a chart and a scene
// summary computed from the same population print the same value. The rule was
// chosen in the maths decision review (docs/platform/operations/
// decision-review-maths-2026-10.md, entry E-03) and is recorded in DECISIONS.md
// under D-18.
package stats

import "math"

// nearestRankTolerance absorbs the rounding in p*n, so that a rank which is
// an exact integer in decimal arithmetic is not pushed up by one. Ranks are
// counts, so anything this close to an integer is that integer.
const nearestRankTolerance = 1e-9

// NearestRank returns the p-th quantile of sorted (ascending) values by the
// nearest-rank rule: the value at 1-based rank ceil(p*n), clamped to the
// first and last values. It always returns an observed value, never an
// interpolation between two. p is a fraction in [0, 1]: 0.85 for p85.
//
// It returns NaN for an empty slice. The caller sorts; NearestRank does not
// copy or check the order.
//
// Percentiles are not merged: the p85 of per-bucket p85 values is not the p85
// of the population, so an aggregate over several buckets is computed again
// from the underlying values.
func NearestRank(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	rank := p * float64(n)
	if r := math.Round(rank); math.Abs(rank-r) <= nearestRankTolerance*math.Max(1, r) {
		rank = r
	}
	idx := int(math.Ceil(rank)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
