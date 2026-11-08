package motion

import (
	"fmt"
	"math"
)

// ModelKind names a motion model in the common six-state space.
type ModelKind string

const (
	// ModelCV is constant velocity: acceleration held at zero with zero
	// variance, driven by white-noise acceleration.
	ModelCV ModelKind = "cv"
	// ModelCA is constant acceleration, driven by white-noise jerk, or by a
	// Singer acceleration when SingerTau is set.
	ModelCA ModelKind = "ca"
)

// ProcessModel is one motion model's transition and process noise.
//
// CV (white-noise acceleration, spectral density Q in m²/s³), per axis over
// [p, v, a]:
//
//	F = |1 T 0|    Q = q |T³/3 T²/2 0|  + PosVar·T on p
//	    |0 1 0|          |T²/2 T    0|
//	    |0 0 0|          |0    0    0|
//
// The zero third row is the embedding: whatever acceleration the state
// carries (an IMM mixes one in from CA) is mapped to zero, and nothing adds
// variance to it, so CV's acceleration rows and columns of P are exactly
// zero after a prediction. PosVar is the live filter's independent position
// term (process_noise_pos), which the continuous model does not have; zero
// leaves the pure continuous form. Diagonal reproduces the live filter's
// diagonal Q, diag(PosVar·T, q·T), for comparison with it only.
//
// CA (white-noise jerk, spectral density Q in m²/s⁵), per axis:
//
//	F = |1 T T²/2|    Q = q |T⁵/20 T⁴/8 T³/6|
//	    |0 1 T   |          |T⁴/8  T³/3 T²/2|
//	    |0 0 1   |          |T³/6  T²/2 T   |
//
// CA with SingerTau > 0 is Singer's model: ȧ = −a/τ + w, w white with
// spectral density q, discretised exactly (singerBlocks).
type ProcessModel struct {
	Kind      ModelKind
	Q         float64
	PosVar    float64
	Diagonal  bool
	SingerTau float64
}

// Validate reports a configuration the filter cannot run.
func (m ProcessModel) Validate() error {
	switch m.Kind {
	case ModelCV, ModelCA:
	default:
		return fmt.Errorf("motion: unknown model kind %q", m.Kind)
	}
	if !(m.Q >= 0) || math.IsInf(m.Q, 0) {
		return fmt.Errorf("motion: process noise q must be finite and non-negative, got %v", m.Q)
	}
	if !(m.PosVar >= 0) || math.IsInf(m.PosVar, 0) {
		return fmt.Errorf("motion: position process noise must be finite and non-negative, got %v", m.PosVar)
	}
	if !(m.SingerTau >= 0) || math.IsInf(m.SingerTau, 0) {
		return fmt.Errorf("motion: Singer time constant must be finite and non-negative, got %v", m.SingerTau)
	}
	if m.Diagonal && m.Kind != ModelCV {
		return fmt.Errorf("motion: the diagonal live-filter form is defined for CV only")
	}
	return nil
}

// Discretise returns the transition F and process noise Q for one step of
// length dt seconds.
func (m ProcessModel) Discretise(dt float64) (Mat, Mat) {
	var f, q block3
	switch {
	case m.Kind == ModelCV:
		f = block3{{1, dt, 0}, {0, 1, 0}, {0, 0, 0}}
		if m.Diagonal {
			q = block3{{m.PosVar * dt, 0, 0}, {0, m.Q * dt, 0}, {0, 0, 0}}
		} else {
			q = block3{
				{m.Q*dt*dt*dt/3 + m.PosVar*dt, m.Q * dt * dt / 2, 0},
				{m.Q * dt * dt / 2, m.Q * dt, 0},
				{0, 0, 0},
			}
		}
	case m.SingerTau > 0:
		f, q = singerBlocks(dt, m.SingerTau, m.Q)
	default:
		f, q = wnjBlocks(dt, m.Q)
	}
	return embedAxes(f), embedAxes(q)
}

// wnjBlocks is the white-noise-jerk CA discretisation.
func wnjBlocks(dt, q float64) (block3, block3) {
	t2, t3 := dt*dt, dt*dt*dt
	t4, t5 := t2*t2, t2*t3
	f := block3{{1, dt, t2 / 2}, {0, 1, dt}, {0, 0, 1}}
	qb := block3{
		{q * t5 / 20, q * t4 / 8, q * t3 / 6},
		{q * t4 / 8, q * t3 / 3, q * t2 / 2},
		{q * t3 / 6, q * t2 / 2, q * dt},
	}
	return f, qb
}

// singerCrossover is the αT below which the closed-form Singer Q loses its
// leading digits to cancellation (q11 is O((αT)⁵) built from O(1) terms) and
// the same integral is evaluated by quadrature instead.
const singerCrossover = 0.05

// singerBlocks is Singer's exact discretisation (Singer 1970) of
// ȧ = −αa + w with α = 1/τ and w of spectral density q:
//
//	F = |1 T (αT − 1 + e^{−αT})/α²|
//	    |0 1 (1 − e^{−αT})/α       |
//	    |0 0 e^{−αT}               |
//
// and Q = q·[q_ij] with (writing E = e^{−αT})
//
//	q11 = (1 − E² + 2αT + 2α³T³/3 − 2α²T² − 4αT·E) / (2α⁵)
//	q12 = (E² + 1 − 2E + 2αT·E − 2αT + α²T²)       / (2α⁴)
//	q13 = (1 − E² − 2αT·E)                          / (2α³)
//	q22 = (4E − 3 − E² + 2αT)                       / (2α³)
//	q23 = (E² + 1 − 2E)                             / (2α²)
//	q33 = (1 − E²)                                  / (2α)
//
// Singer writes the noise as 2ασ_m² with σ_m² the manoeuvre variance; here q
// is the spectral density itself, so σ_m² = qτ/2 and the model tends to the
// white-noise-jerk CA with the same q as τ → ∞ (TestSingerTendsToWNJ).
func singerBlocks(dt, tau, q float64) (block3, block3) {
	alpha := 1 / tau
	x := alpha * dt
	e := math.Exp(-x)
	f := block3{
		{1, dt, dt * dt * phi2(x)},
		{0, 1, dt * phi1(x)},
		{0, 0, e},
	}
	var qb block3
	if x < singerCrossover {
		qb = singerQQuadrature(dt, alpha)
	} else {
		e2 := e * e
		a2, a3 := alpha*alpha, alpha*alpha*alpha
		a4, a5 := a2*a2, a2*a3
		qb[0][0] = (1 - e2 + 2*x + 2*x*x*x/3 - 2*x*x - 4*x*e) / (2 * a5)
		qb[0][1] = (e2 + 1 - 2*e + 2*x*e - 2*x + x*x) / (2 * a4)
		qb[0][2] = (1 - e2 - 2*x*e) / (2 * a3)
		qb[1][1] = (4*e - 3 - e2 + 2*x) / (2 * a3)
		qb[1][2] = (e2 + 1 - 2*e) / (2 * a2)
		qb[2][2] = (1 - e2) / (2 * alpha)
	}
	// Both branches fill the upper triangle; scale it and mirror it.
	for i := 0; i < 3; i++ {
		for j := i; j < 3; j++ {
			qb[i][j] *= q
			qb[j][i] = qb[i][j]
		}
	}
	return f, qb
}

// singerQQuadrature evaluates the unit-density Singer Q as its defining
// integral, ∫₀ᵀ g(s) g(s)ᵀ ds with g(s) = [s²φ₂(αs), sφ₁(αs), e^{−αs}] the
// response at T to a unit impulse at T − s, by 10-point Gauss–Legendre. The
// integrand is analytic and the interval short, so the rule is exact to
// rounding. Only the upper triangle is filled.
func singerQQuadrature(dt, alpha float64) block3 {
	var qb block3
	half := dt / 2
	for k := range gaussLegendre10Nodes {
		s := half * (gaussLegendre10Nodes[k] + 1)
		w := half * gaussLegendre10Weights[k]
		g := [3]float64{s * s * phi2(alpha*s), s * phi1(alpha*s), math.Exp(-alpha * s)}
		for i := 0; i < 3; i++ {
			for j := i; j < 3; j++ {
				qb[i][j] += w * g[i] * g[j]
			}
		}
	}
	return qb
}

// phi1(x) = (1 − e^{−x})/x, stable at x → 0.
func phi1(x float64) float64 {
	if x == 0 {
		return 1
	}
	return -math.Expm1(-x) / x
}

// phi2(x) = (x − 1 + e^{−x})/x², by its series below 0.1, where the closed
// form cancels.
func phi2(x float64) float64 {
	if math.Abs(x) < 0.1 {
		// Σ (−x)^k / (k+2)!, k = 0..7: truncation below 1e-15 at x = 0.1.
		sum, term := 0.0, 0.5
		for k := 0; k < 8; k++ {
			sum += term
			term *= -x / float64(k+3)
		}
		return sum
	}
	return (x + math.Expm1(-x)) / (x * x)
}

var gaussLegendre10Nodes = [10]float64{
	-0.9739065285171717, -0.8650633666889845, -0.6794095682990244, -0.4333953941292472, -0.1488743389816312,
	0.1488743389816312, 0.4333953941292472, 0.6794095682990244, 0.8650633666889845, 0.9739065285171717,
}

var gaussLegendre10Weights = [10]float64{
	0.0666713443086881, 0.1494513491505806, 0.2190863625159820, 0.2692667193099963, 0.2955242247147529,
	0.2955242247147529, 0.2692667193099963, 0.2190863625159820, 0.1494513491505806, 0.0666713443086881,
}
