package l8analytics

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// assignMinCost is the evaluator's one entry to the assignment solver.
//
// Why a wrapper rather than calling l5tracks.HungarianAssign directly: that
// solver is exact only for a matrix with no more rows than columns and no
// forbidden cells. It pads missing columns, and marks forbidden pairs, with a
// 1e18 sentinel, and in float64 1e18 + 0.9 equals 1e18 + 0.6: once a sentinel
// enters the potentials, real costs lose every significant bit. Measured
// against brute force on random matrices up to 4 × 4, it returned a worse
// assignment for about half of those with more rows than columns, and for about
// one in seventy of the others that contained a forbidden cell. The per-frame
// matcher puts references on the rows, so every frame where the tracker had
// fewer hypotheses than there were objects was solved by row order rather than
// distance.
//
// The wrapper keeps the solver inside its exact range. The smaller side is
// always the rows, and a forbidden pair costs a finite penalty larger than any
// complete assignment of allowed pairs could, so the solver still prefers more
// allowed pairs over fewer and cheaper among equals, which is what the
// sentinel meant. Pairs on the penalty are then dropped. assign_test.go checks
// the result against brute force.
//
// cost[i][j] is the cost of pairing row i with column j; allowed[i][j] false
// forbids it. The result maps each row to its column, or -1.
func assignMinCost(cost [][]float64, allowed [][]bool) []int {
	n := len(cost)
	out := make([]int, n)
	for i := range out {
		out[i] = -1
	}
	if n == 0 || len(cost[0]) == 0 {
		return out
	}
	m := len(cost[0])

	transposed := n > m
	rows, cols := n, m
	if transposed {
		rows, cols = m, n
	}
	at := func(r, c int) (float64, bool) {
		if transposed {
			return cost[c][r], allowed[c][r]
		}
		return cost[r][c], allowed[r][c]
	}

	// The penalty must exceed the cost of any complete assignment of allowed
	// pairs, so that swapping one forbidden pair for an allowed one always
	// pays. rows+1 times the largest allowed cost, plus one, does.
	maxAllowed := 0.0
	anyAllowed := false
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if v, ok := at(r, c); ok {
				anyAllowed = true
				maxAllowed = math.Max(maxAllowed, math.Abs(v))
			}
		}
	}
	if !anyAllowed {
		return out
	}
	penalty := (maxAllowed+1)*float64(rows+1) + 1

	matrix := make([][]float32, rows)
	for r := range matrix {
		matrix[r] = make([]float32, cols)
		for c := range matrix[r] {
			if v, ok := at(r, c); ok {
				matrix[r][c] = float32(v)
			} else {
				matrix[r][c] = float32(penalty)
			}
		}
	}

	for r, c := range l5tracks.HungarianAssign(matrix) {
		if c < 0 || c >= cols {
			continue
		}
		if _, ok := at(r, c); !ok {
			continue
		}
		if transposed {
			out[c] = r
		} else {
			out[r] = c
		}
	}
	return out
}
