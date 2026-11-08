package l5tracks

import "math"

// Covariance maths for the four-state CV filter, split out so the two known
// gaps in it (K1 and K2 in the paper-implementation gap analysis) can be
// measured against the shipped forms rather than argued about.
//
// Both alternatives are opt-in Go-level options with the shipped behaviour as
// the default. That is deliberate: TuningConfig.Fingerprint hashes the whole
// resolved tuning config, so exposing these as tuning keys would move the
// fingerprint and make the committed perf baselines refuse to compare. The
// decision to switch either on belongs with the measurement that justifies it
// and the baseline recapture that follows.

// addProcessNoise adds one prediction step's process noise to P.
//
// The shipped form is diagonal: position and velocity uncertainty each grow on
// their own, with no cross term. The continuous white-noise-acceleration model
// the filter is otherwise derived from says otherwise — an unknown
// acceleration over dt moves position and velocity together, so their
// uncertainties are correlated:
//
//	Q = q * | dt³/3  dt²/2 |
//	        | dt²/2  dt    |
//
// Leaving the off-diagonal terms out makes the gating ellipse overconfident in
// the position-velocity correlation direction, which is exactly the direction
// a manoeuvring target departs along.
//
// `velVar` is the acceleration spectral density q: the shipped diagonal form
// already adds q·dt to the velocity terms, so the coupled form is a strict
// addition of the terms that were missing rather than a reinterpretation of
// the tuned value. `posVar` stays an independent position-jitter term in both
// forms, since it models something the continuous model does not claim to.
func addProcessNoise(p *[16]float32, posVar, velVar, dt float32, coupled bool) {
	if !coupled {
		p[0*4+0] += posVar * dt
		p[1*4+1] += posVar * dt
		p[2*4+2] += velVar * dt
		p[3*4+3] += velVar * dt
		return
	}

	// Computed in float64: dt³ at a 0.1 s step is 1e-3, and multiplying that
	// by a small q in float32 loses bits that the cross term needs.
	d := float64(dt)
	q := float64(velVar)
	posTerm := float32(float64(posVar)*d + q*d*d*d/3)
	crossTerm := float32(q * d * d / 2)
	velTerm := float32(q * d)

	p[0*4+0] += posTerm
	p[1*4+1] += posTerm
	p[2*4+2] += velTerm
	p[3*4+3] += velTerm
	// x with vx, and y with vy. The two axes stay independent of each other;
	// only each axis's own position and velocity are coupled.
	p[0*4+2] += crossTerm
	p[2*4+0] += crossTerm
	p[1*4+3] += crossTerm
	p[3*4+1] += crossTerm
}

// naiveCovarianceUpdate is the shipped posterior covariance, P' = (I - KH)P.
//
// Correct in exact arithmetic for the optimal gain, but it is not a symmetric
// expression, so float error accumulates asymmetrically and a long-running
// track's P can drift away from symmetry.
func naiveCovarianceUpdate(p [16]float32, k [8]float32) [16]float32 {
	a := identityMinusKH(k)
	return multiply4x4(a, p)
}

// josephCovarianceUpdate is the Joseph stabilised posterior covariance,
// P' = (I - KH)P(I - KH)ᵀ + KRKᵀ.
//
// Algebraically identical to the naive form at the optimal gain, but every
// term is a symmetric product, so the result stays symmetric and positive
// semi-definite under float error instead of merely starting that way.
// measurementNoise is R's diagonal: the filter uses isotropic measurement
// noise, so R = r·I and KRKᵀ is r·KKᵀ.
func josephCovarianceUpdate(p [16]float32, k [8]float32, measurementNoise float32) [16]float32 {
	a := identityMinusKH(k)
	// (I-KH) P (I-KH)ᵀ
	out := multiply4x4(multiply4x4(a, p), transpose4x4(a))
	// + r K Kᵀ
	r := measurementNoise
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			out[i*4+j] += r * (k[i*2+0]*k[j*2+0] + k[i*2+1]*k[j*2+1])
		}
	}
	return out
}

// josephCovarianceUpdateFull is the Joseph form for a full symmetric R, as the
// adaptive measurement model produces: P' = (I - KH)P(I - KH)ᵀ + KRKᵀ with
// (KRKᵀ)[i,j] = Σ K[i,a]·R[a,b]·K[j,b]. The isotropic form above is kept
// separately so the shipped option's arithmetic does not change.
func josephCovarianceUpdateFull(p [16]float32, k [8]float32, r MeasurementCovariance) [16]float32 {
	a := identityMinusKH(k)
	out := multiply4x4(multiply4x4(a, p), transpose4x4(a))
	for i := 0; i < 4; i++ {
		ki0, ki1 := k[i*2+0], k[i*2+1]
		// Row i of K·R.
		kr0 := ki0*r.XX + ki1*r.XY
		kr1 := ki0*r.XY + ki1*r.YY
		for j := 0; j < 4; j++ {
			out[i*4+j] += kr0*k[j*2+0] + kr1*k[j*2+1]
		}
	}
	return out
}

// identityMinusKH builds I - KH for the position-only measurement model,
// where H selects x and y: (KH)[i,j] is K[i,0] for j==0, K[i,1] for j==1, and
// zero otherwise.
func identityMinusKH(k [8]float32) [16]float32 {
	var a [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var v float32
			if i == j {
				v = 1
			}
			switch j {
			case 0:
				v -= k[i*2+0]
			case 1:
				v -= k[i*2+1]
			}
			a[i*4+j] = v
		}
	}
	return a
}

func multiply4x4(a, b [16]float32) [16]float32 {
	var out [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[i*4+k] * b[k*4+j]
			}
			out[i*4+j] = sum
		}
	}
	return out
}

func transpose4x4(a [16]float32) [16]float32 {
	var out [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			out[i*4+j] = a[j*4+i]
		}
	}
	return out
}

// capCovarianceConsistent caps every diagonal of P at maxDiag by scaling its
// row and column (ConsistentCovarianceCap):
//
//	D = diag(min(1, sqrt(maxDiag / P_ii))),   P ← D P D.
//
// D P D is a congruence, so every correlation P_ij/sqrt(P_ii P_jj) is kept and
// a positive semi-definite P stays so. The shipped cap clamps the diagonal and
// leaves the off-diagonals, which lowers P_ii while P_ij keeps its size, so
// |P_ij| can pass sqrt(P_ii P_jj) and P becomes indefinite.
//
// Computed in float64 and stored once; a capped diagonal is stored at exactly
// maxDiag so the bound holds to the bit, as the shipped cap's does. A
// diagonal that is not above maxDiag (including NaN, which the callers'
// finite-state guards then refuse) is untouched, and P is untouched when no
// diagonal is above it. A cap of zero or below scales the over-cap rows and
// columns to zero, the consistent reading of a variance bound that admits no
// variance.
func capCovarianceConsistent(p *[16]float32, maxDiag float32) {
	var d [4]float64
	capped := false
	for i := 0; i < 4; i++ {
		d[i] = 1
		if v := p[i*4+i]; v > maxDiag {
			capped = true
			d[i] = 0
			if maxDiag > 0 {
				d[i] = math.Sqrt(float64(maxDiag) / float64(v))
			}
		}
	}
	if !capped {
		return
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			if d[i] != 1 || d[j] != 1 {
				p[i*4+j] = float32(float64(p[i*4+j]) * d[i] * d[j])
			}
		}
	}
	for i := 0; i < 4; i++ {
		if d[i] != 1 && p[i*4+i] > maxDiag {
			p[i*4+i] = maxDiag
		}
	}
}

// minSpeedConstraintVariance is the smallest velocity variance along the
// direction of travel the consistent speed clamp will regress on. Below it
// the gain P hᵀ / (h P hᵀ) is float noise divided by float noise, so the clamp
// falls back to rescaling the velocity alone.
const minSpeedConstraintVariance = 1e-9

// constrainSpeedConsistent applies a speed limit to a four-state CV estimate
// as a conditional update (ConsistentSpeedClamp), in place of rescaling the
// velocity and leaving P.
//
// With u the unit vector along the current velocity and h = [0, 0, ux, uy],
// h·x is the speed. The limit is treated as a perfect measurement of that
// component: the state moves to it, and every other component moves with it
// through its regression on it,
//
//	k = P hᵀ / (h P hᵀ),   x ← x + k (vmax − h·x),   P ← P − k h P.
//
// That is a projection of the estimate onto the constraint surface h·x =
// vmax, not a truncation of the density: the along-track velocity variance
// becomes zero, and prediction's process noise restores it. Position moves
// too, by its covariance with the along-track speed, so the clamp no longer
// leaves the position believing a speed the velocity no longer says.
//
// P − k h P is symmetric for a symmetric P; the result is symmetrised against
// float error and any diagonal driven below zero by it is floored at zero.
// Velocity across the direction of travel also moves by its covariance with
// the along-track component, which can leave the speed fractionally above
// vmax; that residue, second order in the excess, is removed by the shipped
// rescale of the velocity alone so the speed bound holds as it always has. A
// velocity variance along u below minSpeedConstraintVariance, or a
// non-finite one, falls back to the shipped rescale outright. The caller has
// already found the speed above vmax.
func constrainSpeedConsistent(x *[4]float32, p *[16]float32, vmax float32) {
	vx, vy := float64(x[2]), float64(x[3])
	speed := math.Hypot(vx, vy)
	limit := float64(vmax)
	if !(speed > 0) || math.IsInf(speed, 0) {
		return
	}
	ux, uy := vx/speed, vy/speed
	var ph [4]float64 // P hᵀ
	for i := 0; i < 4; i++ {
		ph[i] = float64(p[i*4+2])*ux + float64(p[i*4+3])*uy
	}
	s := ph[2]*ux + ph[3]*uy // h P hᵀ
	if !(s > minSpeedConstraintVariance) || math.IsInf(s, 0) {
		rescaleVelocity(x, vmax)
		return
	}
	excess := limit - speed
	for i := 0; i < 4; i++ {
		x[i] = float32(float64(x[i]) + ph[i]/s*excess)
	}
	var out [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			pij := 0.5 * (float64(p[i*4+j]) + float64(p[j*4+i]))
			out[i*4+j] = float32(pij - ph[i]*ph[j]/s)
		}
	}
	for i := 0; i < 4; i++ {
		if out[i*4+i] < 0 {
			out[i*4+i] = 0
		}
	}
	*p = out
	rescaleVelocity(x, vmax)
}

// rescaleVelocity is the shipped speed clamp on a state vector: the velocity
// scaled to vmax along its own direction, the covariance untouched.
func rescaleVelocity(x *[4]float32, vmax float32) {
	speed := float32(math.Sqrt(float64(x[2]*x[2] + x[3]*x[3])))
	if speed > vmax && speed > 0 {
		scale := vmax / speed
		x[2] *= scale
		x[3] *= scale
	}
}

// covarianceAsymmetry is the largest |P[i,j] - P[j,i]| in P.
//
// A symmetric covariance is not a cosmetic property: the gating inverse and
// the NIS it feeds both assume it, so drift here quietly biases association.
func covarianceAsymmetry(p [16]float32) float32 {
	var worst float32
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			d := p[i*4+j] - p[j*4+i]
			if d < 0 {
				d = -d
			}
			if d > worst {
				worst = d
			}
		}
	}
	return worst
}
