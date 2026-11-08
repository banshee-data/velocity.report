package motion

import (
	"fmt"
	"math"
)

// SmoothCV runs the Rauch–Tung–Striebel backward pass over a CV filter's
// whole run and returns the smoothed estimate at every step's time:
//
//	C_k   = P(k|k) Fᵀ P(k+1|k)⁻¹
//	x(k|N) = x(k|k) + C_k (x(k+1|N) − x(k+1|k))
//	P(k|N) = P(k|k) + C_k (P(k+1|N) − P(k+1|k)) C_kᵀ
//
// over the position and velocity block, F the CV transition across step
// k+1's DtSecs. The priors and posteriors are the filter's own (Step.Prior,
// Step.Posterior), so the pass uses the covariance cap and the sub-steps as
// the filter applied them; across a sub-stepped interval the CV mean
// composes exactly, so one F spans it. CV carries no acceleration, so the
// acceleration rows of every smoothed estimate are zero.
//
// It is an offline label, not an estimator anything ships: every smoothed
// state uses observations on both sides of it. steps must be RunFilter's
// output for a CV configuration: the first founded, no other.
func SmoothCV(steps []Step) ([]Estimate, error) {
	if len(steps) == 0 {
		return nil, nil
	}
	if !steps[0].Founded {
		return nil, fmt.Errorf("motion: a smoothed run starts at its founding step")
	}
	for k := 1; k < len(steps); k++ {
		if steps[k].Founded {
			return nil, fmt.Errorf("motion: step %d founds a second track", k)
		}
	}
	for k, s := range steps {
		for _, i := range []int{IdxAX, IdxAY} {
			if s.Posterior.X[i] != 0 || s.Posterior.P[i][i] != 0 {
				return nil, fmt.Errorf("motion: step %d carries an acceleration; SmoothCV smooths CV runs only", k)
			}
		}
	}
	n := len(steps)
	out := make([]Estimate, n)
	out[n-1] = steps[n-1].Posterior
	for k := n - 2; k >= 0; k-- {
		post := steps[k].Posterior
		next := steps[k+1]
		f := cvBlock(next.DtSecs)
		pf := block4(post.P)
		pp := block4(next.Prior.P)
		inv, ok := invert4(pp)
		if !ok {
			return nil, fmt.Errorf("motion: step %d's prior covariance is singular", k+1)
		}
		c := mul4(mul4(pf, transpose4(f)), inv)
		var dx [4]float64
		for i := 0; i < 4; i++ {
			dx[i] = out[k+1].X[i] - next.Prior.X[i]
		}
		smoothed := post
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				smoothed.X[i] += c[i][j] * dx[j]
			}
		}
		var dp [4][4]float64
		ps := block4(out[k+1].P)
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				dp[i][j] = ps[i][j] - pp[i][j]
			}
		}
		corr := mul4(mul4(c, dp), transpose4(c))
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				smoothed.P[i][j] = pf[i][j] + 0.5*(corr[i][j]+corr[j][i])
			}
		}
		if !finiteVec(smoothed.X) || !finiteMat(smoothed.P) {
			return nil, fmt.Errorf("motion: step %d's smoothed estimate is not finite", k)
		}
		out[k] = smoothed
	}
	return out, nil
}

// cvBlock is the CV transition over [x, y, vx, vy] for dt seconds.
func cvBlock(dt float64) [4][4]float64 {
	return [4][4]float64{{1, 0, dt, 0}, {0, 1, 0, dt}, {0, 0, 1, 0}, {0, 0, 0, 1}}
}

func block4(p Mat) [4][4]float64 {
	var b [4][4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			b[i][j] = p[i][j]
		}
	}
	return b
}

func mul4(a, b [4][4]float64) [4][4]float64 {
	var out [4][4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			for k := 0; k < 4; k++ {
				out[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return out
}

func transpose4(a [4][4]float64) [4][4]float64 {
	var out [4][4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			out[i][j] = a[j][i]
		}
	}
	return out
}

// invert4 inverts a 4×4 matrix by Gauss–Jordan elimination with partial
// pivoting, and false when a pivot is negligible against the matrix's scale.
func invert4(a [4][4]float64) ([4][4]float64, bool) {
	var inv [4][4]float64
	scale := 0.0
	for i := 0; i < 4; i++ {
		inv[i][i] = 1
		for j := 0; j < 4; j++ {
			scale = math.Max(scale, math.Abs(a[i][j]))
		}
	}
	if !(scale > 0) {
		return inv, false
	}
	for c := 0; c < 4; c++ {
		p := c
		for r := c + 1; r < 4; r++ {
			if math.Abs(a[r][c]) > math.Abs(a[p][c]) {
				p = r
			}
		}
		if math.Abs(a[p][c]) <= 1e-12*scale {
			return inv, false
		}
		a[c], a[p] = a[p], a[c]
		inv[c], inv[p] = inv[p], inv[c]
		d := a[c][c]
		for k := 0; k < 4; k++ {
			a[c][k] /= d
			inv[c][k] /= d
		}
		for r := 0; r < 4; r++ {
			if r == c || a[r][c] == 0 {
				continue
			}
			f := a[r][c]
			for k := 0; k < 4; k++ {
				a[r][k] -= f * a[c][k]
				inv[r][k] -= f * inv[c][k]
			}
		}
	}
	return inv, true
}
