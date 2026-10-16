package main

import (
	"errors"
	"math"
	"slices"
)

// The fit needs three small dense operations: a symmetric eigen-decomposition
// (3x3 for surface normals, 6x6 for observability), a Cholesky solve and
// inverse (6x6 normal equations), and the rotation exponential and logarithm.
// They are written out here rather than taking a matrix library for them.

type vec3 [3]float64
type mat3 [3][3]float64

func (a vec3) add(b vec3) vec3      { return vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a vec3) sub(b vec3) vec3      { return vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a vec3) scale(s float64) vec3 { return vec3{a[0] * s, a[1] * s, a[2] * s} }
func (a vec3) dot(b vec3) float64   { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a vec3) norm() float64        { return math.Sqrt(a.dot(a)) }
func (a vec3) cross(b vec3) vec3 {
	return vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func (m mat3) mul(v vec3) vec3 {
	return vec3{m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2], m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2], m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2]}
}
func (m mat3) mulT(v vec3) vec3 {
	return vec3{m[0][0]*v[0] + m[1][0]*v[1] + m[2][0]*v[2], m[0][1]*v[0] + m[1][1]*v[1] + m[2][1]*v[2], m[0][2]*v[0] + m[1][2]*v[1] + m[2][2]*v[2]}
}
func identity3() mat3 { return mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} }
func (m mat3) mulMat(b mat3) (r mat3) {
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r[i][j] = m[i][0]*b[0][j] + m[i][1]*b[1][j] + m[i][2]*b[2][j]
		}
	}
	return r
}

// expSO3 is the rotation for rotation vector w (radians), by Rodrigues.
func expSO3(w vec3) mat3 {
	theta := w.norm()
	k := [3][3]float64{{0, -w[2], w[1]}, {w[2], 0, -w[0]}, {-w[1], w[0], 0}}
	var a, b float64
	if theta < 1e-12 {
		a, b = 1, 0.5
	} else {
		a = math.Sin(theta) / theta
		b = (1 - math.Cos(theta)) / (theta * theta)
	}
	r := identity3()
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			kk := 0.0
			for l := 0; l < 3; l++ {
				kk += k[i][l] * k[l][j]
			}
			r[i][j] += a*k[i][j] + b*kk
		}
	}
	return r
}

// logSO3 is the rotation vector (radians) of rotation r, for angles well
// below pi, which is all this tool meets.
func logSO3(r mat3) vec3 {
	c := (r[0][0] + r[1][1] + r[2][2] - 1) / 2
	c = math.Max(-1, math.Min(1, c))
	theta := math.Acos(c)
	v := vec3{r[2][1] - r[1][2], r[0][2] - r[2][0], r[1][0] - r[0][1]}
	if theta < 1e-9 {
		return v.scale(0.5)
	}
	return v.scale(theta / (2 * math.Sin(theta)))
}

// symEigen decomposes symmetric matrix a (n x n, not modified) by cyclic
// Jacobi rotations. Eigenvalues are returned ascending; vecs[k] is the unit
// eigenvector for vals[k].
func symEigen(a [][]float64) ([]float64, [][]float64) {
	n := len(a)
	m := make([][]float64, n)
	v := make([][]float64, n)
	for i := range m {
		m[i] = append([]float64(nil), a[i]...)
		v[i] = make([]float64, n)
		v[i][i] = 1
	}
	for sweep := 0; sweep < 100; sweep++ {
		off, diag := 0.0, 0.0
		for i := 0; i < n; i++ {
			diag += m[i][i] * m[i][i]
			for j := i + 1; j < n; j++ {
				off += m[i][j] * m[i][j]
			}
		}
		// Stop once the off-diagonal part is negligible against the matrix.
		if off <= 1e-28*diag || off < 1e-300 {
			break
		}
		for p := 0; p < n; p++ {
			for q := p + 1; q < n; q++ {
				if math.Abs(m[p][q]) < 1e-300 {
					continue
				}
				theta := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := 1 / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				if theta < 0 {
					t = -t
				}
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for k := 0; k < n; k++ {
					mkp, mkq := m[k][p], m[k][q]
					m[k][p] = c*mkp - s*mkq
					m[k][q] = s*mkp + c*mkq
				}
				for k := 0; k < n; k++ {
					mpk, mqk := m[p][k], m[q][k]
					m[p][k] = c*mpk - s*mqk
					m[q][k] = s*mpk + c*mqk
				}
				for k := 0; k < n; k++ {
					vkp, vkq := v[k][p], v[k][q]
					v[k][p] = c*vkp - s*vkq
					v[k][q] = s*vkp + c*vkq
				}
			}
		}
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	// Insertion sort: n is at most six.
	for i := 1; i < n; i++ {
		for j := i; j > 0 && m[idx[j]][idx[j]] < m[idx[j-1]][idx[j-1]]; j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	vals := make([]float64, n)
	vecs := make([][]float64, n)
	for k, i := range idx {
		vals[k] = m[i][i]
		vecs[k] = make([]float64, n)
		for r := 0; r < n; r++ {
			vecs[k][r] = v[r][i]
		}
	}
	return vals, vecs
}

var errNotPositiveDefinite = errors.New("matrix is not positive definite")

// cholesky returns lower-triangular L with a = L L^T.
func cholesky(a [6][6]float64) ([6][6]float64, error) {
	var l [6][6]float64
	for i := 0; i < 6; i++ {
		for j := 0; j <= i; j++ {
			s := a[i][j]
			for k := 0; k < j; k++ {
				s -= l[i][k] * l[j][k]
			}
			if i == j {
				if s <= 0 || math.IsNaN(s) {
					return l, errNotPositiveDefinite
				}
				l[i][i] = math.Sqrt(s)
			} else {
				l[i][j] = s / l[j][j]
			}
		}
	}
	return l, nil
}

func choleskySolve(l [6][6]float64, b [6]float64) [6]float64 {
	var y, x [6]float64
	for i := 0; i < 6; i++ {
		s := b[i]
		for k := 0; k < i; k++ {
			s -= l[i][k] * y[k]
		}
		y[i] = s / l[i][i]
	}
	for i := 5; i >= 0; i-- {
		s := y[i]
		for k := i + 1; k < 6; k++ {
			s -= l[k][i] * x[k]
		}
		x[i] = s / l[i][i]
	}
	return x
}

func choleskyInverse(l [6][6]float64) [6][6]float64 {
	var inv [6][6]float64
	for c := 0; c < 6; c++ {
		var e [6]float64
		e[c] = 1
		col := choleskySolve(l, e)
		for r := 0; r < 6; r++ {
			inv[r][c] = col[r]
		}
	}
	return inv
}

// median returns the median of xs, reordering xs.
// It selects rather than sorts: the fit takes two medians of every candidate
// cell per iteration.
func median(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return math.NaN()
	}
	hi := selectKth(xs, n/2)
	if n%2 == 1 {
		return hi
	}
	// The lower middle is the largest value left below the upper one.
	lo := slices.Max(xs[:n/2])
	return (lo + hi) / 2
}

// selectKth reorders xs so xs[k] holds the k-th smallest value, everything
// before it no larger and everything after no smaller, and returns it.
func selectKth(xs []float64, k int) float64 {
	lo, hi := 0, len(xs)-1
	for lo < hi {
		mid := lo + (hi-lo)/2
		// Median of three as the pivot keeps sorted input linear.
		if xs[mid] < xs[lo] {
			xs[mid], xs[lo] = xs[lo], xs[mid]
		}
		if xs[hi] < xs[lo] {
			xs[hi], xs[lo] = xs[lo], xs[hi]
		}
		if xs[hi] < xs[mid] {
			xs[hi], xs[mid] = xs[mid], xs[hi]
		}
		pivot := xs[mid]
		i, j := lo, hi
		for i <= j {
			for xs[i] < pivot {
				i++
			}
			for xs[j] > pivot {
				j--
			}
			if i <= j {
				xs[i], xs[j] = xs[j], xs[i]
				i++
				j--
			}
		}
		switch {
		case k <= j:
			hi = j
		case k >= i:
			lo = i
		default:
			return xs[k]
		}
	}
	return xs[k]
}
