package l5tracks

import (
	"math"
	"math/rand"
	"testing"
)

// Gap H1 in data/maths/paper-implementation-gap-analysis.md. The padded
// solver let the 1e18 sentinel into its potentials, where float64 cannot hold
// a cost of order 0.01-36 beside it, and it chose pairings by row order
// whenever rows outnumbered columns or some row had to go unassigned. These
// tests hold HungarianAssign to its contract exactly: the most allowed pairs,
// then the least total cost, checked by enumeration.

const forbid = float32(hungarianlnf)

// bruteAssignment enumerates every one-to-one pairing of rows with columns
// over allowed cells and returns the best: the most pairs, then the least
// cost. It is the contract, written the slow way.
func bruteAssignment(cost [][]float32) (int, float64) {
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
			if used[j] || hungarianForbidden(cost[i][j]) {
				continue
			}
			used[j] = true
			visit(i+1, pairs+1, total+float64(cost[i][j]))
			used[j] = false
		}
	}
	visit(0, 0, 0)
	return bestPairs, bestCost
}

// scoreAssignment checks the result is a legal assignment for cost and
// returns its number of pairs and total cost.
func scoreAssignment(t *testing.T, cost [][]float32, assign []int) (int, float64) {
	t.Helper()
	if len(assign) != len(cost) {
		t.Fatalf("%d assignments for %d rows", len(assign), len(cost))
	}
	pairs, total := 0, 0.0
	seen := map[int]bool{}
	for i, j := range assign {
		if j == -1 {
			continue
		}
		if j < 0 || j >= len(cost[i]) {
			t.Fatalf("row %d assigned to column %d of %d", i, j, len(cost[i]))
		}
		if hungarianForbidden(cost[i][j]) {
			t.Fatalf("row %d assigned to forbidden column %d (cost %v)", i, j, cost[i][j])
		}
		if seen[j] {
			t.Fatalf("column %d assigned twice: %v", j, assign)
		}
		seen[j] = true
		pairs++
		total += float64(cost[i][j])
	}
	return pairs, total
}

// sameCost compares totals of float32 costs summed in float64. Distinct
// assignments of these costs differ by far more than the tolerance.
func sameCost(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b))
}

// randomCostMatrix draws one n×m matrix of a given kind. Every kind is one
// the solver meets or could meet: uniform costs, heavy ties, the tracker's own
// shape, negative costs, whole rows and columns forbidden, and non-finite
// entries standing in for a caller's arithmetic gone wrong.
func randomCostMatrix(rng *rand.Rand, n, m, kind int) [][]float32 {
	cost := make([][]float32, n)
	for i := range cost {
		cost[i] = make([]float32, m)
	}
	forbidP := []float64{0, 0.3, 0.7}[rng.Intn(3)]
	for i := range cost {
		for j := range cost[i] {
			var v float32
			switch kind {
			case 1: // Ties: a handful of integer costs.
				v = float32(rng.Intn(3))
			case 2: // Tracker-shaped: each column has one near row, the rest
				// sit anywhere in the d² ≤ 36 gate or outside it.
				if i == j%n {
					v = float32(rng.Float64() * 2)
				} else {
					v = float32(2 + rng.Float64()*34)
				}
			case 3: // Negative costs are outside every caller but not the contract.
				v = float32(rng.Float64()*20 - 10)
			default:
				v = float32(rng.Float64() * 36)
			}
			if rng.Float64() < forbidP {
				v = forbid
			}
			cost[i][j] = v
		}
	}
	switch rng.Intn(6) {
	case 0: // One row with nothing in its gate.
		r := rng.Intn(n)
		for j := range cost[r] {
			cost[r][j] = forbid
		}
	case 1: // One column no row may take.
		c := rng.Intn(m)
		for i := range cost {
			cost[i][c] = forbid
		}
	case 2: // Non-finite and over-sentinel entries: every one must be forbidden.
		bad := []float32{
			float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)),
			2 * forbid, math.MaxFloat32,
		}
		for k := 0; k < 1+rng.Intn(3); k++ {
			cost[rng.Intn(n)][rng.Intn(m)] = bad[rng.Intn(len(bad))]
		}
	}
	return cost
}

func TestHungarianAssign_MatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(20260925))
	for trial := 0; trial < 6000; trial++ {
		n, m := 1+rng.Intn(6), 1+rng.Intn(6)
		kind := trial % 4
		cost := randomCostMatrix(rng, n, m, kind)

		pairs, total := scoreAssignment(t, cost, HungarianAssign(cost))
		wantPairs, wantCost := bruteAssignment(cost)
		if pairs != wantPairs || (pairs > 0 && !sameCost(total, wantCost)) {
			t.Fatalf("trial %d (%d×%d, kind %d): %d pairs costing %.9g, brute force %d costing %.9g\ncost %v\ngot  %v",
				trial, n, m, kind, pairs, total, wantPairs, wantCost, cost, HungarianAssign(cost))
		}
	}
}

// TestHungarianAssign_OrderInvariant checks a property brute force cannot
// reach at a realistic size: permuting rows and columns must not change the
// optimum. The padded solver failed it, because it chose by row order.
func TestHungarianAssign_OrderInvariant(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 200; trial++ {
		n, m := 5+rng.Intn(30), 5+rng.Intn(30)
		cost := randomCostMatrix(rng, n, m, 2)
		pairs, total := scoreAssignment(t, cost, HungarianAssign(cost))

		rowPerm, colPerm := rng.Perm(n), rng.Perm(m)
		shuffled := make([][]float32, n)
		for i := range shuffled {
			shuffled[i] = make([]float32, m)
			for j := range shuffled[i] {
				shuffled[i][j] = cost[rowPerm[i]][colPerm[j]]
			}
		}
		sPairs, sTotal := scoreAssignment(t, shuffled, HungarianAssign(shuffled))

		transposed := make([][]float32, m)
		for j := range transposed {
			transposed[j] = make([]float32, n)
			for i := range cost {
				transposed[j][i] = cost[i][j]
			}
		}
		tPairs, tTotal := scoreAssignment(t, transposed, HungarianAssign(transposed))

		if sPairs != pairs || !sameCost(sTotal, total) || tPairs != pairs || !sameCost(tTotal, total) {
			t.Fatalf("trial %d (%d×%d): %d pairs costing %.9g; permuted %d costing %.9g; transposed %d costing %.9g",
				trial, n, m, pairs, total, sPairs, sTotal, tPairs, tTotal)
		}
	}
}

// Each case is a matrix on which the padded solver returned the assignment in
// its comment; the optimum is unique, so the whole assignment is pinned.
func TestHungarianAssign_PaddedSolverRegressions(t *testing.T) {
	cases := []struct {
		name string
		cost [][]float32
		want []int
	}{
		{
			// Two clusters, one track, the second nearer. Padded: [0 -1],
			// the first cluster by row order at d² 0.9.
			name: "one column, nearer row second",
			cost: [][]float32{{0.9}, {0.6}},
			want: []int{-1, 0},
		},
		{
			// Padded: [1 0 -1] costing 2.0 against the optimum's 1.5.
			name: "three rows, two columns, cheapest pair on the last row",
			cost: [][]float32{
				{3, 1},
				{1, 3},
				{2, 0.5},
			},
			want: []int{-1, 0, 1},
		},
		{
			// Square, but only one track is reachable at all, so some row
			// had to take a sentinel. Padded: [1 -1] at d² 5.1.
			name: "square with an unreachable column",
			cost: [][]float32{
				{forbid, 5.1},
				{forbid, 0.8},
			},
			want: []int{-1, 1},
		},
		{
			// Square, every column reachable, yet no perfect assignment
			// exists: rows 1 and 2 both need column 1. Padded: [2 1 -1]
			// costing 4.0 against the optimum's 2.5.
			name: "square, two rows compete for one column",
			cost: [][]float32{
				{3.5, 5, 1.8},
				{forbid, 2.2, forbid},
				{forbid, 0.7, forbid},
			},
			want: []int{2, -1, 1},
		},
		{
			// Padded: [1 0 -1] costing 11.5 against the optimum's 6.7.
			name: "more rows than columns with forbidden cells",
			cost: [][]float32{
				{forbid, 5.8},
				{5.7, forbid},
				{5.2, 1},
			},
			want: []int{-1, 0, 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := HungarianAssign(tc.cost)
			for i := range tc.want {
				if len(got) != len(tc.want) || got[i] != tc.want[i] {
					t.Fatalf("assignment = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// A NaN would fail every comparison inside the solver, and ±Inf would carry
// into its potentials; each must be treated as forbidden, like the sentinel.
// In each matrix the bad cell sits where an unguarded solver would take it.
func TestHungarianAssign_NonFiniteCostsAreForbidden(t *testing.T) {
	for _, bad := range []float32{
		float32(math.NaN()),
		float32(math.Inf(1)),
		float32(math.Inf(-1)),
		forbid,
		2 * forbid,
		math.MaxFloat32,
	} {
		cost := [][]float32{
			{bad, 5},
			{1, 1},
		}
		if got := HungarianAssign(cost); got[0] != 1 || got[1] != 0 {
			t.Errorf("cost %v: assignment = %v, want [1 0]", bad, got)
		}
		lone := [][]float32{{bad}}
		if got := HungarianAssign(lone); got[0] != -1 {
			t.Errorf("1×1 cost %v: assignment = %v, want [-1]", bad, got)
		}
		wide := [][]float32{{bad, bad, 2}}
		if got := HungarianAssign(wide); got[0] != 2 {
			t.Errorf("1×3 with %v: assignment = %v, want [2]", bad, got)
		}
	}
}

func TestHungarianAssign_DegenerateShapes(t *testing.T) {
	if got := HungarianAssign([][]float32{}); got != nil {
		t.Errorf("0×m: got %v, want nil", got)
	}
	got := HungarianAssign([][]float32{{}, {}, {}})
	if len(got) != 3 || got[0] != -1 || got[1] != -1 || got[2] != -1 {
		t.Errorf("3×0: got %v, want [-1 -1 -1]", got)
	}
	if got := HungarianAssign([][]float32{{0}}); len(got) != 1 || got[0] != 0 {
		t.Errorf("1×1 zero cost: got %v, want [0]", got)
	}
	tall := [][]float32{{forbid}, {forbid}, {4}, {forbid}}
	if got := HungarianAssign(tall); got[0] != -1 || got[1] != -1 || got[2] != 0 || got[3] != -1 {
		t.Errorf("4×1, one allowed: got %v, want [-1 -1 0 -1]", got)
	}
}

// Association calls the solver every frame, often twice. Sizes are a busy
// street: a few dozen clusters against a few dozen tracks.
func BenchmarkHungarianAssign(b *testing.B) {
	for _, size := range []struct {
		name string
		n, m int
	}{
		{"12x8", 12, 8},
		{"40x30", 40, 30},
		{"30x40", 30, 40},
	} {
		b.Run(size.name, func(b *testing.B) {
			cost := randomCostMatrix(rand.New(rand.NewSource(1)), size.n, size.m, 2)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				HungarianAssign(cost)
			}
		})
	}
}
