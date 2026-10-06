package l3grid

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// Gaps B1 and B2 in data/maths/paper-implementation-gap-analysis.md. The
// per-cell spread is an EMA of absolute deviation about the previous mean,
// not a variance, and the settling criterion's spread-delta floor is
// proportional to the update fraction. Both are stated in the maths docs;
// neither had a test against a known distribution.

// spreadTestGrid is one cell, updated with a small alpha, with every guard
// that could reject a Gaussian sample turned off.
func spreadTestGrid(alpha float32) (*BackgroundGrid, *BackgroundManager) {
	g := makeTestGridStrict(2, 8)
	g.Params.BackgroundUpdateFraction = alpha
	g.Params.ClosenessSensitivityMultiplier = 3.0
	g.Params.SafetyMarginMetres = 0.15
	g.Params.NoiseRelativeFraction = 0.01
	g.Params.NeighbourConfirmationCount = 0
	g.Params.FreezeDurationNanos = 0
	g.Params.SeedFromFirstObservation = true
	return g, g.Manager
}

// feedGaussian sends n frames of one return into the cell at ring 0, azimuth
// bin 0, drawn from N(mean, sigma^2), advancing capture time 100 ms per frame.
// It returns the per-frame absolute change in the cell's spread.
func feedGaussian(bm *BackgroundManager, g *BackgroundGrid, rng *rand.Rand, n int, mean, sigma float64, start time.Time) []float64 {
	idx := g.Idx(0, 0)
	deltas := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		before := float64(g.Cells[idx].RangeSpreadMeters)
		d := mean + sigma*rng.NormFloat64()
		_, _ = bm.ProcessFramePolarWithMaskAt(
			[]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: d, Intensity: 100}},
			start.Add(time.Duration(i)*100*time.Millisecond))
		deltas = append(deltas, math.Abs(float64(g.Cells[idx].RangeSpreadMeters)-before))
	}
	return deltas
}

// B1: for Gaussian deviations the mean absolute deviation is 0.798 sigma, so
// the spread converges to that, not to sigma.
func TestSpreadConvergesToMeanAbsoluteDeviation(t *testing.T) {
	const alpha, sigma = 0.02, 0.10
	g, bm := spreadTestGrid(alpha)
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic sample
	start := time.Unix(1_700_000_000, 0)
	feedGaussian(bm, g, rng, 10_000, 5.0, sigma, start)

	got := float64(g.Cells[g.Idx(0, 0)].RangeSpreadMeters)
	want := 0.798 * sigma
	// The EMA of |d| has its own standard error of about sigma*sqrt(alpha/2)
	// times the deviation's spread; 0.012 is roughly two of those.
	if math.Abs(got-want) > 0.012 {
		t.Fatalf("spread = %.4f m after 10,000 N(5, 0.1^2) samples, want %.4f (0.798 sigma), not sigma = %.2f", got, want, sigma)
	}
	if math.Abs(got-sigma) < math.Abs(got-want) {
		t.Fatalf("spread %.4f sits closer to sigma than to 0.798 sigma; the doc's MAD statement would be wrong", got)
	}
}

// B2: once settled, E|delta s| = 0.483 alpha sigma (derived in the gap
// analysis and confirmed there by simulation), so the settling criterion's
// fixed spread-delta threshold is a joint constraint on alpha and sigma.
func TestSettledSpreadDeltaFloorIsProportionalToAlpha(t *testing.T) {
	const sigma = 0.10
	rng := rand.New(rand.NewSource(2)) //nolint:gosec // deterministic sample
	start := time.Unix(1_700_000_000, 0)
	floors := map[float32]float64{}
	for _, alpha := range []float32{0.02, 0.1} {
		g, bm := spreadTestGrid(alpha)
		feedGaussian(bm, g, rng, 3_000, 5.0, sigma, start) // settle
		deltas := feedGaussian(bm, g, rng, 6_000, 5.0, sigma, start.Add(300*time.Second))
		sum := 0.0
		for _, d := range deltas {
			sum += d
		}
		mean := sum / float64(len(deltas))
		want := 0.483 * float64(alpha) * sigma
		if math.Abs(mean-want)/want > 0.15 {
			t.Fatalf("alpha %.2f: mean |delta s| = %.6f, want %.6f (0.483 alpha sigma) within 15%%", alpha, mean, want)
		}
		floors[alpha] = mean
	}
	if ratio := floors[0.1] / floors[0.02]; ratio < 4 || ratio > 6 {
		t.Fatalf("spread-delta floor grew %.2fx from alpha 0.02 to 0.1, want about 5x (proportional to alpha)", ratio)
	}
}
