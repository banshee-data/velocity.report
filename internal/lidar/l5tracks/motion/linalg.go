package motion

import "math"

// StateDim is the size of the common state [x, y, vx, vy, ax, ay].
const StateDim = 6

// State indices.
const (
	IdxX = iota
	IdxY
	IdxVX
	IdxVY
	IdxAX
	IdxAY
)

// Vec is a state vector.
type Vec [StateDim]float64

// Mat is a row-major state-space matrix.
type Mat [StateDim][StateDim]float64

// axisIndices are the [position, velocity, acceleration] indices of each
// axis. Every transition and process-noise matrix here is block-diagonal over
// the two axes, built from one 3×3 per-axis block.
var axisIndices = [2][3]int{{IdxX, IdxVX, IdxAX}, {IdxY, IdxVY, IdxAY}}

// block3 is a per-axis [position, velocity, acceleration] matrix.
type block3 [3][3]float64

// embedAxes places the same per-axis block on both axes of a state matrix.
func embedAxes(b block3) Mat {
	var m Mat
	for _, idx := range axisIndices {
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				m[idx[i]][idx[j]] = b[i][j]
			}
		}
	}
	return m
}

func identity() Mat {
	var m Mat
	for i := 0; i < StateDim; i++ {
		m[i][i] = 1
	}
	return m
}

func (m Mat) mul(n Mat) Mat {
	var out Mat
	for i := 0; i < StateDim; i++ {
		for k := 0; k < StateDim; k++ {
			a := m[i][k]
			if a == 0 {
				continue
			}
			for j := 0; j < StateDim; j++ {
				out[i][j] += a * n[k][j]
			}
		}
	}
	return out
}

func (m Mat) transpose() Mat {
	var out Mat
	for i := 0; i < StateDim; i++ {
		for j := 0; j < StateDim; j++ {
			out[j][i] = m[i][j]
		}
	}
	return out
}

func (m Mat) add(n Mat) Mat {
	for i := 0; i < StateDim; i++ {
		for j := 0; j < StateDim; j++ {
			m[i][j] += n[i][j]
		}
	}
	return m
}

func (m Mat) scale(s float64) Mat {
	for i := 0; i < StateDim; i++ {
		for j := 0; j < StateDim; j++ {
			m[i][j] *= s
		}
	}
	return m
}

func (m Mat) apply(v Vec) Vec {
	var out Vec
	for i := 0; i < StateDim; i++ {
		var s float64
		for j := 0; j < StateDim; j++ {
			s += m[i][j] * v[j]
		}
		out[i] = s
	}
	return out
}

// quadratic returns F P Fᵀ.
func quadratic(f, p Mat) Mat { return f.mul(p).mul(f.transpose()) }

func (v Vec) sub(w Vec) Vec {
	for i := range v {
		v[i] -= w[i]
	}
	return v
}

func outer(a, b Vec) Mat {
	var m Mat
	for i := 0; i < StateDim; i++ {
		for j := 0; j < StateDim; j++ {
			m[i][j] = a[i] * b[j]
		}
	}
	return m
}

func dot(a, b Vec) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func finiteVec(v Vec) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

func finiteMat(m Mat) bool {
	for i := range m {
		if !finiteVec(m[i]) {
			return false
		}
	}
	return true
}

// symmetricEigenvalues returns the eigenvalues of a symmetric matrix by the
// cyclic Jacobi method. Six states converge in a handful of sweeps; the
// routine is for audit checks, not the filter's hot path.
func symmetricEigenvalues(m Mat) Vec {
	a := Symmetrise(m)
	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for i := 0; i < StateDim; i++ {
			for j := i + 1; j < StateDim; j++ {
				off += a[i][j] * a[i][j]
			}
		}
		if off < 1e-300 {
			break
		}
		for p := 0; p < StateDim; p++ {
			for q := p + 1; q < StateDim; q++ {
				if a[p][q] == 0 {
					continue
				}
				theta := (a[q][q] - a[p][p]) / (2 * a[p][q])
				t := math.Copysign(1, theta) / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for k := 0; k < StateDim; k++ {
					akp, akq := a[k][p], a[k][q]
					a[k][p] = c*akp - s*akq
					a[k][q] = s*akp + c*akq
				}
				for k := 0; k < StateDim; k++ {
					apk, aqk := a[p][k], a[q][k]
					a[p][k] = c*apk - s*aqk
					a[q][k] = s*apk + c*aqk
				}
			}
		}
	}
	var ev Vec
	for i := 0; i < StateDim; i++ {
		ev[i] = a[i][i]
	}
	return ev
}
