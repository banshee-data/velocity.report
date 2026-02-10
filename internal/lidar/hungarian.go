package lidar

import "math"

// hungarian implements the Kuhn–Munkres (Hungarian) algorithm for optimal
// cluster-to-track assignment. It solves the balanced assignment problem in
// O(n³) time, replacing the greedy nearest-neighbour approach which could
// cause track splitting when two clusters compete for the same track.
//
// The cost matrix entry C[i][j] is the squared Mahalanobis distance between
// cluster i and track j. Entries exceeding the gating threshold are set to
// +Inf so that the solver never selects them.
//
// Returns assignments[i] = j meaning cluster i → track j, or -1 if
// cluster i is unassigned (no track within gating distance).

const hungarianlnf = 1e18 // Stand-in for infinity in cost matrix

// HungarianAssign solves the rectangular assignment problem for an n×m cost
// matrix. It returns assignments[i] = column index assigned to row i, or -1
// if unassigned. Costs ≥ hungarianlnf are treated as forbidden.
//
// For n ≤ m it pads nothing; for n > m it pads columns with hungarianlnf so
// excess rows stay unassigned.
func HungarianAssign(cost [][]float32) []int {
	n := len(cost)
	if n == 0 {
		return nil
	}
	m := len(cost[0])
	if m == 0 {
		result := make([]int, n)
		for i := range result {
			result[i] = -1
		}
		return result
	}

	// Make the matrix square by padding.
	dim := n
	if m > dim {
		dim = m
	}

	// Build a float64 padded square matrix for numerical stability.
	c := make([][]float64, dim)
	for i := 0; i < dim; i++ {
		c[i] = make([]float64, dim)
		for j := 0; j < dim; j++ {
			if i < n && j < m {
				c[i][j] = float64(cost[i][j])
			} else {
				c[i][j] = hungarianlnf
			}
		}
	}

	// Kuhn-Munkres with potentials (Jonker-Volgenant variant).
	// Uses 1-indexed arrays internally for cleaner index arithmetic.
	const inf = math.MaxFloat64 / 2

	u := make([]float64, dim+1) // Row potentials
	v := make([]float64, dim+1) // Column potentials
	p := make([]int, dim+1)     // p[j] = row assigned to column j
	way := make([]int, dim+1)   // way[j] = previous column in augmenting path
	minv := make([]float64, dim+1)
	used := make([]bool, dim+1)

	for i := 1; i <= dim; i++ {
		p[0] = i
		j0 := 0 // Virtual column

		for j := 1; j <= dim; j++ {
			minv[j] = inf
			used[j] = false
		}

		for {
			used[j0] = true
			i0 := p[j0]
			delta := inf
			j1 := -1

			for j := 1; j <= dim; j++ {
				if used[j] {
					continue
				}
				cur := c[i0-1][j-1] - u[i0] - v[j]
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
				break
			}

			for j := 0; j <= dim; j++ {
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

	// Extract assignments (row → column).
	rowAssign := make([]int, dim)
	for i := range rowAssign {
		rowAssign[i] = -1
	}
	for j := 1; j <= dim; j++ {
		if p[j] > 0 && p[j] <= dim {
			rowAssign[p[j]-1] = j - 1
		}
	}

	// Trim to original dimensions and reject forbidden assignments.
	result := make([]int, n)
	for i := 0; i < n; i++ {
		col := rowAssign[i]
		if col < 0 || col >= m || cost[i][col] >= float32(hungarianlnf) {
			result[i] = -1
		} else {
			result[i] = col
		}
	}

	return result
}
