package l5tracks

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
