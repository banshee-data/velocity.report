package motion

import (
	"errors"
	"fmt"
	"math"
)

// Observation is one measured position of a track at a capture time.
//
// R is the 2×2 measurement covariance of (X, Y) in m². When RankOne is set
// the observation measures only the component of position along the unit
// direction (NX, NY): z = NX·X + NY·Y with variance nᵀRn, which is the form a
// face measurement takes. Otherwise it measures both coordinates.
type Observation struct {
	UnixNanos int64
	X, Y      float64
	R         [2][2]float64
	RankOne   bool
	NX, NY    float64

	// LiveNIS is the live CV filter's normalised innovation squared for this
	// observation when it came from the filter-step record; HasLiveNIS is
	// false for a founding observation or synthetic input. G-EST-1's NIS
	// analyses read it; no estimator here does.
	LiveNIS    float64
	HasLiveNIS bool
	ClusterID  int64
}

// IsotropicR returns σ²·I for a scalar measurement variance.
func IsotropicR(variance float64) [2][2]float64 {
	return [2][2]float64{{variance, 0}, {0, variance}}
}

// Innovation is a measurement residual against a prior: y = z − Hx with
// covariance S = HPHᵀ + R. Dim is 2 for a position, 1 for a rank-one
// measurement, in which case only Y[0] and S[0][0] are used.
type Innovation struct {
	Dim int
	Y   [2]float64
	S   [2][2]float64
	// NIS is yᵀS⁻¹y, the squared Mahalanobis distance d².
	NIS float64
	// LogDetS is ln det S, the term a bare d² cost leaves out.
	LogDetS float64
}

// ErrSingularInnovation is returned when S is not positive definite, so no
// update or likelihood is defined.
var ErrSingularInnovation = errors.New("motion: innovation covariance is not positive definite")

// measurementRows returns H (rows beyond m unused), z and R for obs.
func measurementRows(obs Observation) (m int, h [2]Vec, z [2]float64, r [2][2]float64, err error) {
	if !finiteF(obs.X) || !finiteF(obs.Y) {
		return 0, h, z, r, fmt.Errorf("motion: observation position is not finite")
	}
	if obs.RankOne {
		norm := math.Hypot(obs.NX, obs.NY)
		if !(norm > 0) || !finiteF(norm) {
			return 0, h, z, r, fmt.Errorf("motion: rank-one observation needs a non-zero direction")
		}
		nx, ny := obs.NX/norm, obs.NY/norm
		h[0][IdxX], h[0][IdxY] = nx, ny
		z[0] = nx*obs.X + ny*obs.Y
		r[0][0] = nx*nx*obs.R[0][0] + nx*ny*(obs.R[0][1]+obs.R[1][0]) + ny*ny*obs.R[1][1]
		return 1, h, z, r, nil
	}
	h[0][IdxX] = 1
	h[1][IdxY] = 1
	z = [2]float64{obs.X, obs.Y}
	r = obs.R
	r[0][1] = 0.5 * (obs.R[0][1] + obs.R[1][0])
	r[1][0] = r[0][1]
	return 2, h, z, r, nil
}

// innovationFor computes y, S and their scalar summaries against (x, P).
// It also returns PHᵀ, which the gain needs.
func innovationFor(x Vec, p Mat, obs Observation) (Innovation, [2]Vec, [2]Vec, [2][2]float64, error) {
	m, h, z, r, err := measurementRows(obs)
	if err != nil {
		return Innovation{}, h, h, r, err
	}
	in := Innovation{Dim: m}
	var pht [2]Vec // column j of PHᵀ stored as pht[j]
	for j := 0; j < m; j++ {
		pht[j] = p.apply(h[j])
	}
	for i := 0; i < m; i++ {
		in.Y[i] = z[i] - dot(h[i], x)
		for j := 0; j < m; j++ {
			in.S[i][j] = dot(h[i], pht[j]) + r[i][j]
		}
	}
	if m == 2 {
		s := 0.5 * (in.S[0][1] + in.S[1][0])
		in.S[0][1], in.S[1][0] = s, s
	}
	inv, logDet, ok := invertSPD(in.S, m)
	if !ok {
		return in, h, pht, r, ErrSingularInnovation
	}
	in.LogDetS = logDet
	for i := 0; i < m; i++ {
		for j := 0; j < m; j++ {
			in.NIS += in.Y[i] * inv[i][j] * in.Y[j]
		}
	}
	return in, h, pht, r, nil
}

// invertSPD inverts a 1×1 or 2×2 symmetric positive-definite matrix and
// returns ln det.
func invertSPD(s [2][2]float64, m int) ([2][2]float64, float64, bool) {
	var inv [2][2]float64
	if m == 1 {
		if !(s[0][0] > 0) || !finiteF(s[0][0]) {
			return inv, 0, false
		}
		inv[0][0] = 1 / s[0][0]
		return inv, math.Log(s[0][0]), true
	}
	det := s[0][0]*s[1][1] - s[0][1]*s[1][0]
	if !(s[0][0] > 0) || !(det > 0) || !finiteF(det) {
		return inv, 0, false
	}
	inv[0][0] = s[1][1] / det
	inv[1][1] = s[0][0] / det
	inv[0][1] = -s[0][1] / det
	inv[1][0] = -s[1][0] / det
	return inv, math.Log(det), true
}

// josephUpdate applies the measurement to (x, P) with the Joseph form
// P⁺ = (I − KH) P (I − KH)ᵀ + K R Kᵀ, which keeps P symmetric positive
// semi-definite for any gain, and returns the innovation it used.
func josephUpdate(x Vec, p Mat, obs Observation) (Vec, Mat, Innovation, error) {
	in, h, pht, r, err := innovationFor(x, p, obs)
	if err != nil {
		return x, p, in, err
	}
	inv, _, _ := invertSPD(in.S, in.Dim)
	// K = PHᵀ S⁻¹, stored as columns k[j].
	var k [2]Vec
	for j := 0; j < in.Dim; j++ {
		for c := 0; c < in.Dim; c++ {
			for s := 0; s < StateDim; s++ {
				k[j][s] += pht[c][s] * inv[c][j]
			}
		}
	}
	for j := 0; j < in.Dim; j++ {
		for s := 0; s < StateDim; s++ {
			x[s] += k[j][s] * in.Y[j]
		}
	}
	a := identity()
	for j := 0; j < in.Dim; j++ {
		a = a.add(outer(k[j], h[j]).scale(-1))
	}
	post := quadratic(a, p)
	for i := 0; i < in.Dim; i++ {
		for j := 0; j < in.Dim; j++ {
			if r[i][j] != 0 {
				post = post.add(outer(k[i], k[j]).scale(r[i][j]))
			}
		}
	}
	return x, Symmetrise(post), in, nil
}

func finiteF(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// LikelihoodKind names the density a mode probability is updated with.
type LikelihoodKind string

const (
	// LikelihoodGaussian is exp(−d²/2)/√det(2πS).
	LikelihoodGaussian LikelihoodKind = "gaussian"
	// LikelihoodStudentT is the multivariate Student-t with scale S and ν
	// degrees of freedom, whose polynomial tail keeps one outlying
	// innovation from moving most of the probability to the wider mode.
	LikelihoodStudentT LikelihoodKind = "student_t"
)

// DefaultStudentTNu is the default degrees of freedom of the robust
// likelihood: heavy enough that a 10-sigma innovation costs a polynomial,
// not an exponential, penalty, and finite-variance (ν > 2).
const DefaultStudentTNu = 4.0

// Likelihood selects the density used to score an innovation. It is the one
// likelihood an IMM's mode probabilities use, and the function an association
// cost could share so that the cluster a track wins and the mode it credits
// are scored on the same terms (maths review, Section 4.5).
type Likelihood struct {
	Kind LikelihoodKind
	// Nu is the Student-t degrees of freedom; zero means DefaultStudentTNu.
	Nu float64
}

// LogDensity returns the log likelihood of the innovation.
func (l Likelihood) LogDensity(in Innovation) float64 {
	if l.Kind == LikelihoodStudentT {
		nu := l.Nu
		if nu <= 0 {
			nu = DefaultStudentTNu
		}
		return StudentTLogLikelihood(in, nu)
	}
	return GaussianLogLikelihood(in)
}

// GaussianLogLikelihood is ln N(y; 0, S) = −d²/2 − (m/2) ln 2π − ½ ln det S.
// The ln det S term is what a bare d² association cost omits; without it a
// wide (coasting, or CA) prior is not charged for its width.
func GaussianLogLikelihood(in Innovation) float64 {
	m := float64(in.Dim)
	return -0.5*in.NIS - 0.5*m*math.Log(2*math.Pi) - 0.5*in.LogDetS
}

// StudentTLogLikelihood is the log density of the m-variate Student-t with
// scale S and ν degrees of freedom:
//
//	ln Γ((ν+m)/2) − ln Γ(ν/2) − (m/2) ln(νπ) − ½ ln det S − ((ν+m)/2) ln(1 + d²/ν)
//
// It tends to the Gaussian as ν → ∞.
func StudentTLogLikelihood(in Innovation, nu float64) float64 {
	m := float64(in.Dim)
	a, _ := math.Lgamma((nu + m) / 2)
	b, _ := math.Lgamma(nu / 2)
	return a - b - 0.5*m*math.Log(nu*math.Pi) - 0.5*in.LogDetS - 0.5*(nu+m)*math.Log1p(in.NIS/nu)
}
