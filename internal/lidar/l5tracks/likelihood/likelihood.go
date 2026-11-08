// Package likelihood is the one density an L5 innovation is scored with.
//
// The interacting-multiple-model filter (l5tracks/motion) weighs its modes by
// it, and the tracker's association cost (l5tracks,
// RobustLikelihoodAssociationCost) ranks pairings by it, so the cluster a
// track wins and the mode it credits are scored on the same terms (maths
// review 20261108, Section 4.5 item 2). It is a leaf: l5tracks and motion both
// import it, and it imports neither, because motion imports l5tracks.
//
// Every function takes an innovation's sufficient statistics rather than the
// innovation itself: its dimension m (2 for a position, 1 for a rank-one face
// measurement), its squared Mahalanobis distance d² = yᵀS⁻¹y, and ln det S.
// The caller computes d² and ln det S from the same S, which is what keeps the
// gate (a test on d²) and the cost (a density) consistent.
package likelihood

import "math"

// DefaultStudentTNu is the default degrees of freedom of the robust
// likelihood: heavy enough that a 10-sigma innovation costs a polynomial,
// not an exponential, penalty, and finite-variance (ν > 2). K9 measured 7 to
// 17 % of moving innovations beyond the Gaussian 95 % bound, which is the tail
// this is for.
const DefaultStudentTNu = 4.0

// Nu returns ν when it is a usable degrees of freedom, finite and positive,
// and DefaultStudentTNu otherwise. Zero is how a caller asks for the default.
func Nu(nu float64) float64 {
	if !(nu > 0) || math.IsInf(nu, 0) {
		return DefaultStudentTNu
	}
	return nu
}

// GaussianLogLikelihood is ln N(y; 0, S) = −d²/2 − (m/2) ln 2π − ½ ln det S.
// The ln det S term is what a bare d² association cost omits; without it a
// wide (coasting, or CA) prior is not charged for its width.
func GaussianLogLikelihood(dim int, d2, logDetS float64) float64 {
	m := float64(dim)
	return -0.5*d2 - 0.5*m*math.Log(2*math.Pi) - 0.5*logDetS
}

// StudentTLogLikelihood is the log density of the m-variate Student-t with
// scale S and ν degrees of freedom:
//
//	ln Γ((ν+m)/2) − ln Γ(ν/2) − (m/2) ln(νπ) − ½ ln det S − ((ν+m)/2) ln(1 + d²/ν)
//
// It tends to the Gaussian as ν → ∞. ν is used as given; pass it through Nu
// to apply the default.
func StudentTLogLikelihood(dim int, d2, logDetS, nu float64) float64 {
	m := float64(dim)
	a, _ := math.Lgamma((nu + m) / 2)
	b, _ := math.Lgamma(nu / 2)
	return a - b - 0.5*m*math.Log(nu*math.Pi) - 0.5*logDetS - 0.5*(nu+m)*math.Log1p(d2/nu)
}

// GaussianNegLogLikelihood is −GaussianLogLikelihood, the form a cost
// minimises. Negation is exact in floating point, so the two never disagree
// by a rounding.
func GaussianNegLogLikelihood(dim int, d2, logDetS float64) float64 {
	return -GaussianLogLikelihood(dim, d2, logDetS)
}

// StudentTNegLogLikelihood is −StudentTLogLikelihood.
func StudentTNegLogLikelihood(dim int, d2, logDetS, nu float64) float64 {
	return -StudentTLogLikelihood(dim, d2, logDetS, nu)
}
