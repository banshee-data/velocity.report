package l5tracks

import "math"

// hungarian implements the Kuhn–Munkres (Hungarian) algorithm for optimal
// cluster-to-track assignment, replacing the greedy nearest-neighbour approach
// which could cause track splitting when two clusters compete for the same
// track.
//
// The cost matrix entry C[i][j] is the squared Mahalanobis distance between
// cluster i and track j, plus any configured likelihood and extent terms.
// Entries outside the gate are set to hungarianlnf so that the solver never
// selects them.

// hungarianlnf is the forbidden-pair sentinel callers write into the cost
// matrix. It is a marker only: HungarianAssign never lets it into the solver's
// arithmetic, because in float64 the spacing between neighbouring values near
// 1e18 is 128, larger than every real cost, so a sentinel that reaches the
// dual potentials erases the costs it sits beside. Before this was known the
// solver padded missing columns with it and let forbidden cells carry it.
// Against brute force on random matrices up to 4×4 that returned a worse
// assignment for 3,138 of 4,621 with more rows than columns, where it
// followed row order rather than cost, and for 91 of 4,770 of the rest that
// held a forbidden cell. On kirk0 it was 6 of 705 association frames, at a
// median 17.6 more summed cost than the optimum.
const hungarianlnf = 1e18

// HungarianAssign solves the rectangular assignment problem for an n×m cost
// matrix. It returns assignments[i] = column index assigned to row i, or -1
// if row i is unassigned. A cost ≥ hungarianlnf forbids its pair, as does a
// NaN or infinite cost: a NaN would otherwise poison every comparison it
// meets, and -Inf is no more a real cost than +Inf.
//
// The objective is lexicographic, and it is what the sentinel always meant:
// first the most allowed pairs, then, among assignments with that many, the
// least total cost. Every caller relies on both halves. Association would
// rather accept a slightly worse pairing than leave a track coasting beside
// a cluster in its gate; the evaluators count a match before they weigh it.
//
// The result is optimal to float64 rounding, which for the costs the tracker
// and the evaluators produce is far finer than the float32 inputs' own
// resolution; hungarian_exact_test.go checks it against brute-force
// enumeration. Two things keep the solver's arithmetic in that range.
// The smaller side is always the rows, transposing when n > m, so nothing is
// padded. And a forbidden cell costs a finite penalty just larger than any
// complete assignment of allowed pairs could, so one more allowed pair always
// pays for itself and the penalty stays within a few orders of magnitude of
// the real costs. Penalty pairs are dropped from the result.
//
// Time is O(k²·K) for k = min(n, m) and K = max(n, m), within the O(n³) the
// padded solver cost.
func HungarianAssign(cost [][]float32) []int {
	n := len(cost)
	if n == 0 {
		return nil
	}
	result := make([]int, n)
	for i := range result {
		result[i] = -1
	}
	m := len(cost[0])
	if m == 0 {
		return result
	}

	// The solver below needs no more rows than columns: each row it places
	// must find a free column. Transpose when the caller has more rows.
	transposed := n > m
	rows, cols := n, m
	if transposed {
		rows, cols = m, n
	}
	at := func(r, c int) float32 {
		if transposed {
			return cost[c][r]
		}
		return cost[r][c]
	}

	// The allowed costs' range sets the penalty. With no allowed pair at all
	// there is nothing to solve.
	lo, hi := math.Inf(1), math.Inf(-1)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			v := at(r, c)
			if hungarianForbidden(v) {
				continue
			}
			lo = math.Min(lo, float64(v))
			hi = math.Max(hi, float64(v))
		}
	}
	if math.IsInf(lo, 1) {
		return result
	}

	// Shift allowed costs to be non-negative, which only happens if a caller
	// passes negative ones: every in-tree caller's costs are already ≥ 0 and
	// reach the solver unchanged. Among assignments with the same number of
	// allowed pairs a uniform shift changes nothing, and the penalty handles
	// assignments with different numbers.
	//
	// Every row is placed, so an assignment is k = rows pairs, p of them on
	// the penalty. With shifted allowed costs in [0, span], an assignment with
	// p penalty pairs costs at least p·penalty, and one with fewer, p' < p,
	// costs at most p'·penalty + k·span. penalty > k·span therefore makes the
	// assignment with fewer penalty pairs strictly cheaper: the solver
	// maximises allowed pairs first. Among assignments with the same p the
	// penalty contributes equally, so it then minimises the allowed cost.
	shift := math.Min(lo, 0)
	span := hi - shift
	penalty := (span + 1) * float64(rows+1)

	c := make([]float64, rows*cols)
	for r := 0; r < rows; r++ {
		for j := 0; j < cols; j++ {
			v := at(r, j)
			if hungarianForbidden(v) {
				c[r*cols+j] = penalty
			} else {
				c[r*cols+j] = float64(v) - shift
			}
		}
	}

	// Kuhn-Munkres with potentials (Jonker-Volgenant shortest augmenting
	// path), rows ≤ cols. Uses 1-indexed arrays internally, with column 0 as
	// the virtual start column, for cleaner index arithmetic.
	const inf = math.MaxFloat64 / 2

	u := make([]float64, rows+1) // Row potentials
	v := make([]float64, cols+1) // Column potentials
	p := make([]int, cols+1)     // p[j] = row assigned to column j, 0 if free
	way := make([]int, cols+1)   // way[j] = previous column in augmenting path
	minv := make([]float64, cols+1)
	used := make([]bool, cols+1)

	for i := 1; i <= rows; i++ {
		p[0] = i
		j0 := 0

		for j := 1; j <= cols; j++ {
			minv[j] = inf
			used[j] = false
		}

		// Grow the shortest-path tree until it reaches a free column. One
		// always exists: every cost is finite and fewer than cols rows have
		// been placed.
		for {
			used[j0] = true
			i0 := p[j0]
			delta := inf
			j1 := -1

			for j := 1; j <= cols; j++ {
				if used[j] {
					continue
				}
				cur := c[(i0-1)*cols+(j-1)] - u[i0] - v[j]
				if cur < minv[j] {
					minv[j] = cur
					way[j] = j0
				}
				if minv[j] < delta {
					delta = minv[j]
					j1 = j
				}
			}
			if j1 < 0 {
				// Unreachable while the matrix is finite and rows ≤ cols.
				// The padded solver broke out here and augmented a
				// half-built path; fail where the invariant broke instead.
				panic("l5tracks: HungarianAssign found no reachable column")
			}

			for j := 0; j <= cols; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}

			j0 = j1
			if p[j0] == 0 {
				break
			}
		}

		// Augment along the path.
		for j0 != 0 {
			p[j0] = p[way[j0]]
			j0 = way[j0]
		}
	}

	// Every row now holds a column. Keep the allowed pairs, in the caller's
	// orientation.
	for j := 1; j <= cols; j++ {
		r := p[j] - 1
		if r < 0 || hungarianForbidden(at(r, j-1)) {
			continue
		}
		if transposed {
			result[j-1] = r
		} else {
			result[r] = j - 1
		}
	}
	return result
}

// hungarianForbidden reports whether a cost forbids its pair: at or above the
// sentinel, +Inf, NaN (for which every comparison is false, hence the negated
// form), or -Inf.
func hungarianForbidden(v float32) bool {
	return !(v < float32(hungarianlnf)) || math.IsInf(float64(v), -1)
}
