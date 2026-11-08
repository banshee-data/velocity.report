package motion

import "math"

// Covariance hygiene (maths review 2026-11-08, Section 4.6).
//
// Two operations can leave P inconsistent with the state it describes: a cap
// applied to the diagonal alone, which lets a correlation exceed one, and a
// state change made without moving P. Mixing in an IMM propagates either into
// every mode. The helpers here are the only ways this package bounds or
// constrains a covariance:
//
//   - Symmetrise averages P with its transpose, removing rounding asymmetry.
//   - CapCovariance scales a row and its column together, P ← D P D, so every
//     correlation coefficient is unchanged and P stays positive semi-definite.
//   - constrainLinear (in coast.go) conditions the state on a linear equality
//     through its regression, moving the mean and P together.
//
// Nothing here clamps one diagonal entry by itself.

// Symmetrise returns (P + Pᵀ)/2.
func Symmetrise(p Mat) Mat {
	for i := 0; i < StateDim; i++ {
		for j := i + 1; j < StateDim; j++ {
			m := 0.5 * (p[i][j] + p[j][i])
			p[i][j], p[j][i] = m, m
		}
	}
	return p
}

// CapCovariance bounds every variance at maxVar without breaking positive
// semi-definiteness. With d_i = min(1, sqrt(maxVar / P_ii)) it returns
// D P D: each capped row and column is scaled by the same factor, so the
// correlation between any two components is what it was. maxVar ≤ 0 disables
// the cap. The result is symmetrised.
func CapCovariance(p Mat, maxVar float64) Mat {
	if maxVar <= 0 {
		return Symmetrise(p)
	}
	var d [StateDim]float64
	capped := false
	for i := 0; i < StateDim; i++ {
		d[i] = 1
		if p[i][i] > maxVar {
			d[i] = math.Sqrt(maxVar / p[i][i])
			capped = true
		}
	}
	if capped {
		for i := 0; i < StateDim; i++ {
			for j := 0; j < StateDim; j++ {
				p[i][j] *= d[i] * d[j]
			}
		}
	}
	return Symmetrise(p)
}

// IsSymmetricPSD reports whether p is symmetric to within tol (relative to
// its largest diagonal) and positive semi-definite. It is an audit check for
// tests and offline reports, not a repair.
func IsSymmetricPSD(p Mat, tol float64) bool {
	if !finiteMat(p) {
		return false
	}
	scale := 0.0
	for i := 0; i < StateDim; i++ {
		scale = math.Max(scale, math.Abs(p[i][i]))
	}
	for i := 0; i < StateDim; i++ {
		for j := i + 1; j < StateDim; j++ {
			if math.Abs(p[i][j]-p[j][i]) > tol*math.Max(scale, 1e-300) {
				return false
			}
		}
	}
	for _, ev := range symmetricEigenvalues(p) {
		if ev < -tol*math.Max(scale, 1e-300) {
			return false
		}
	}
	return true
}
