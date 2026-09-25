package l8analytics

import (
	"math"
	"math/rand"
	"testing"
)

// bruteAssignment enumerates every one-to-one pairing of rows with columns
// over allowed cells and returns the best: the most pairs, then the least
// cost. That is the objective the sentinel in the old matcher meant, and the
// one assignMinCost must meet exactly.
func bruteAssignment(cost [][]float64, allowed [][]bool) (int, float64) {
	n, m := len(cost), len(cost[0])
	bestPairs, bestCost := -1, math.Inf(1)
	used := make([]bool, m)
	var visit func(i, pairs int, total float64)
	visit = func(i, pairs int, total float64) {
		if i == n {
			if pairs > bestPairs || (pairs == bestPairs && total < bestCost) {
				bestPairs, bestCost = pairs, total
			}
			return
		}
		visit(i+1, pairs, total)
		for j := 0; j < m; j++ {
			if used[j] || !allowed[i][j] {
				continue
			}
			used[j] = true
			visit(i+1, pairs+1, total+cost[i][j])
			used[j] = false
		}
	}
	visit(0, 0, 0)
	return bestPairs, bestCost
}

func TestAssignMinCostMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for trial := 0; trial < 3000; trial++ {
		n, m := 1+rng.Intn(5), 1+rng.Intn(5)
		forbidProbability := []float64{0, 0.35, 0.8}[trial%3]
		cost := make([][]float64, n)
		allowed := make([][]bool, n)
		for i := range cost {
			cost[i] = make([]float64, m)
			allowed[i] = make([]bool, m)
			for j := range cost[i] {
				// Quantised to float32, as the solver sees them, so the
				// comparison is exact rather than within rounding.
				cost[i][j] = float64(float32(rng.Float64() * 3))
				allowed[i][j] = rng.Float64() >= forbidProbability
			}
		}

		assign := assignMinCost(cost, allowed)
		if len(assign) != n {
			t.Fatalf("trial %d: %d assignments for %d rows", trial, len(assign), n)
		}
		pairs, total := 0, 0.0
		seen := map[int]bool{}
		for i, j := range assign {
			if j < 0 {
				continue
			}
			if !allowed[i][j] {
				t.Fatalf("trial %d: row %d assigned to forbidden column %d", trial, i, j)
			}
			if seen[j] {
				t.Fatalf("trial %d: column %d assigned twice", trial, j)
			}
			seen[j] = true
			pairs++
			total += cost[i][j]
		}
		wantPairs, wantCost := bruteAssignment(cost, allowed)
		if pairs != wantPairs || math.Abs(total-wantCost) > 1e-6 {
			t.Fatalf("trial %d (%d×%d): got %d pairs costing %.6f, brute force %d costing %.6f\ncost %v\nallowed %v",
				trial, n, m, pairs, total, wantPairs, wantCost, cost, allowed)
		}
	}
}

// The defect the wrapper exists for, in its smallest form: one hypothesis
// between two references, nearer the second. Sorted, the nearer one is the
// second row, and the padded solver gave the hypothesis to the first.
func TestMatcherPrefersTheNearerReferenceWhenReferencesOutnumberHypotheses(t *testing.T) {
	ref := []TrackSeries{track("a", span(0, 0, 0)), track("b", span(0, 0, 1.5))}
	hyp := []TrackSeries{track("h", span(0, 0, 0.9))}
	m := ComputeTrackMetrics(ref, hyp, 1.0)
	if m.Matches != 1 || math.Abs(m.MOTP-0.6) > 1e-6 {
		t.Fatalf("got %+v; want the hypothesis matched to b at 0.6 m", m)
	}
}
