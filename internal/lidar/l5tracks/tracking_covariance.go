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

// speedClampVarianceFloor is the least along-track velocity variance the
// consistent speed clamp leaves, (1 cm/s)²: like the containment truncation's
// floor, the truncation never claims the speed to better than that, and a
// float32 diagonal can otherwise round below zero. It never raises a variance
// already below it.
const speedClampVarianceFloor = 1e-4

// millsContinuedFractionFrom is the standardised excess beyond which the
// inverse Mills ratio is taken from its continued fraction rather than from
// φ/Φ, whose denominator underflows near 37 standard deviations.
const millsContinuedFractionFrom = 30

// constrainSpeedConsistent applies a speed limit to a four-state CV estimate
// as a Gaussian truncation through the state's regression
// (ConsistentSpeedClamp), in place of rescaling the velocity and leaving P.
//
// With u the unit vector along the current velocity and h = [0, 0, ux, uy],
// μ = h·x is the speed and σ² = h P hᵀ its variance. The limit is an
// inequality, h·x ≤ vmax, so the marginal N(μ, σ²) is truncated to
// (−∞, vmax] (upperTruncatedNormalMoments) and its mean m and variance s²
// replace the marginal's through every component's regression on it, the
// form truncateAxis uses for the containment bound:
//
//	k = P hᵀ / σ²,   x ← x + k (m − μ),   P ← P + (s² − σ²) k kᵀ.
//
// The velocity's along-track component moves to m, below the limit, and
// keeps the variance s², which is less than σ² but not zero, so the next
// update can still move it. Position moves too, by its covariance with the
// along-track speed. A projection onto h·x = vmax, by contrast, would treat
// the limit as an equality and zero that variance.
//
// P is symmetrised first; s² is floored at speedClampVarianceFloor (never
// above σ²), and any diagonal float error takes below zero is floored at zero.
// Velocity across the direction of travel also moves by its covariance with
// the along-track component, which can leave the speed above vmax; the
// shipped rescale of the velocity alone then removes that residue, so the
// speed bound holds as it always has. A velocity variance along u below
// minSpeedConstraintVariance, or a non-finite one, falls back to the shipped
// rescale outright. The caller has already found the speed above vmax.
func constrainSpeedConsistent(x *[4]float32, p *[16]float32, vmax float32) {
	constrainSpeedConsistentJoint(x, p, nil, vmax)
}

// constrainSpeedConsistentJoint is constrainSpeedConsistent with, when ext is
// not nil, the half-extent block of the joint state under ExtentState: the
// regression runs over all six states, so hL and hW move by their covariance
// with the along-track speed, P_hh gains (s² − σ²) k_h k_hᵀ and the cross
// block (s² − σ²) k k_hᵀ. The four-state arithmetic is the same either way.
func constrainSpeedConsistentJoint(x *[4]float32, p *[16]float32, ext *solidBodyExtent, vmax float32) {
	vx, vy := float64(x[2]), float64(x[3])
	speed := math.Hypot(vx, vy)
	if !(speed > 0) || math.IsInf(speed, 0) {
		return
	}
	ux, uy := vx/speed, vy/speed
	var sym [16]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			sym[i*4+j] = 0.5 * (float64(p[i*4+j]) + float64(p[j*4+i]))
		}
	}
	var k [4]float64 // P hᵀ, then divided by σ²
	for i := 0; i < 4; i++ {
		k[i] = sym[i*4+2]*ux + sym[i*4+3]*uy
	}
	sigma2 := k[2]*ux + k[3]*uy // h P hᵀ
	if !(sigma2 > minSpeedConstraintVariance) || math.IsInf(sigma2, 0) {
		rescaleVelocity(x, vmax)
		return
	}
	mean, variance := upperTruncatedNormalMoments(speed, math.Sqrt(sigma2), float64(vmax))
	variance = math.Min(sigma2, math.Max(variance, speedClampVarianceFloor))
	shift := mean - speed
	d := variance - sigma2
	for i := range k {
		k[i] /= sigma2
		x[i] = float32(float64(x[i]) + k[i]*shift)
	}
	if ext != nil {
		// The half-extents' regression on the speed, from their covariance
		// with the velocity rows, which the four-state update below does not
		// touch.
		var kh [2]float64
		for j := 0; j < 2; j++ {
			kh[j] = (float64(ext.pxh[2*2+j])*ux + float64(ext.pxh[3*2+j])*uy) / sigma2
			ext.h[j] = float32(float64(ext.h[j]) + kh[j]*shift)
		}
		for i := 0; i < 4; i++ {
			for j := 0; j < 2; j++ {
				ext.pxh[i*2+j] = float32(float64(ext.pxh[i*2+j]) + d*k[i]*kh[j])
			}
		}
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				v := float64(ext.phh[i*2+j]) + d*kh[i]*kh[j]
				if i == j && v < 0 {
					v = 0
				}
				ext.phh[i*2+j] = float32(v)
			}
		}
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			v := sym[i*4+j] + d*k[i]*k[j]
			if i == j && v < 0 {
				v = 0
			}
			p[i*4+j] = float32(v)
		}
	}
	rescaleVelocity(x, vmax)
}

// upperTruncatedNormalMoments is the mean and variance of N(mu, sigma²)
// truncated to (−∞, upper], with β = (upper − mu)/σ and λ = φ(β)/Φ(β), the
// inverse Mills ratio:
//
//	mean = mu − σ λ,   variance = σ² (1 − β λ − λ²).
//
// The infinite lower bound is taken explicitly: the two-sided form
// (truncatedNormalMoments) would form α φ(α) = ∞ · 0 at α = −∞. λ is φ/Φ
// with Φ from erfc, which stays accurate in relative terms, until β falls
// below −millsContinuedFractionFrom; there Φ approaches underflow and λ is
// the continued fraction for φ(z)/(1 − Φ(z)) at z = −β,
// z + 1/(z + 2/(z + 3/(z + …))), which converges in a few terms that far
// out. The mean is held at or below upper and the variance at or above zero
// against rounding; σ must be positive.
func upperTruncatedNormalMoments(mu, sigma, upper float64) (mean, variance float64) {
	beta := (upper - mu) / sigma
	var lambda float64
	if z := -beta; z > millsContinuedFractionFrom {
		t := z
		for n := 40; n >= 1; n-- {
			t = z + float64(n)/t
		}
		lambda = t
	} else {
		pdf := math.Exp(-beta*beta/2) / math.Sqrt(2*math.Pi)
		cdf := 0.5 * math.Erfc(-beta/math.Sqrt2)
		lambda = pdf / cdf
	}
	mean = math.Min(mu-sigma*lambda, upper)
	variance = math.Max(sigma*sigma*(1-beta*lambda-lambda*lambda), 0)
	return mean, variance
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
