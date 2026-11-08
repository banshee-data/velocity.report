package likelihood

import (
	"math"
	"testing"
)

// The values the motion package's GaussianLogLikelihood and
// StudentTLogLikelihood returned on these inputs before the functions moved
// here, printed with %.17g from origin/main at 99a9627. The shared functions
// must reproduce them, so moving the density down a package changed no
// mode probability.
var motionReference = []struct {
	dim                int
	d2, logDetS        float64
	gauss, t4, t1, t30 float64
}{
	{2, 0, 0, -1.8378770664093453, -1.8378770664093453, -1.8378770664093453, -1.8378770664093453},
	{2, 3.7, -1.25, -3.0628770664093454, -3.1776549696285881, -3.534220829483365, -3.0736843575479158},
	{2, 100, 2.5, -53.087877066409348, -12.862166680473791, -10.010557841671236, -26.549270167104179},
	{1, 0.4, -3.1, 0.43106146679532742, 0.33089529747746171, 0.06879787752938693, 0.41742865999583578},
	{1, 49, 0.7, -25.768938533204675, -7.7908231340923049, -5.406752891277546, -16.28515262285427},
	{2, 1e-09, -13.815510557964274, 5.0698782120727914, 5.0698782118227914, 5.0698782110727914, 5.0698782120394581},
}

func TestSharedLikelihoodMatchesTheMotionPackagesPreviousOutputs(t *testing.T) {
	const tol = 1e-12
	for _, c := range motionReference {
		for _, check := range []struct {
			name      string
			got, want float64
		}{
			{"gaussian", GaussianLogLikelihood(c.dim, c.d2, c.logDetS), c.gauss},
			{"student_t ν=4", StudentTLogLikelihood(c.dim, c.d2, c.logDetS, 4), c.t4},
			{"student_t ν=1", StudentTLogLikelihood(c.dim, c.d2, c.logDetS, 1), c.t1},
			{"student_t ν=30", StudentTLogLikelihood(c.dim, c.d2, c.logDetS, 30), c.t30},
		} {
			if math.Abs(check.got-check.want) > tol {
				t.Errorf("m=%d d²=%v ln|S|=%v %s: %.17g, want %.17g", c.dim, c.d2, c.logDetS, check.name, check.got, check.want)
			}
		}
	}
}

func TestNegativeLogLikelihoodIsTheExactNegation(t *testing.T) {
	for _, c := range motionReference {
		if GaussianNegLogLikelihood(c.dim, c.d2, c.logDetS) != -GaussianLogLikelihood(c.dim, c.d2, c.logDetS) {
			t.Errorf("m=%d d²=%v: Gaussian negation is not exact", c.dim, c.d2)
		}
		if StudentTNegLogLikelihood(c.dim, c.d2, c.logDetS, 4) != -StudentTLogLikelihood(c.dim, c.d2, c.logDetS, 4) {
			t.Errorf("m=%d d²=%v: Student-t negation is not exact", c.dim, c.d2)
		}
	}
}

func TestNuDefaultsWhatIsNotAUsableDegreesOfFreedom(t *testing.T) {
	for _, nu := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := Nu(nu); got != DefaultStudentTNu {
			t.Errorf("Nu(%v) = %v, want the default %v", nu, got, DefaultStudentTNu)
		}
	}
	for _, nu := range []float64{0.5, 4, 30} {
		if got := Nu(nu); got != nu {
			t.Errorf("Nu(%v) = %v, want it unchanged", nu, got)
		}
	}
}

// The Student-t tends to the Gaussian as ν grows, so the robust cost is a
// relaxation of the likelihood cost, not a different family.
func TestStudentTTendsToTheGaussian(t *testing.T) {
	for _, c := range motionReference {
		g := GaussianLogLikelihood(c.dim, c.d2, c.logDetS)
		st := StudentTLogLikelihood(c.dim, c.d2, c.logDetS, 1e9)
		if math.Abs(st-g) > 1e-5*math.Max(1, math.Abs(g)) {
			t.Errorf("m=%d d²=%v: ν=1e9 gives %v, Gaussian %v", c.dim, c.d2, st, g)
		}
	}
}

// One 10-sigma innovation must not dominate: relative to an inlier at one
// sigma with the same S, the Gaussian charges it about 49.5 nats more and the
// Student-t at ν=4 about 9 — a polynomial tail, not an exponential one.
func TestATenSigmaOutlierCostsPolynomiallyUnderStudentT(t *testing.T) {
	const logDet = 0.3
	inlier, outlier := 1.0, 100.0
	gauss := GaussianNegLogLikelihood(2, outlier, logDet) - GaussianNegLogLikelihood(2, inlier, logDet)
	robust := StudentTNegLogLikelihood(2, outlier, logDet, DefaultStudentTNu) -
		StudentTNegLogLikelihood(2, inlier, logDet, DefaultStudentTNu)
	if !(robust > 0) || !(robust < gauss/4) {
		t.Fatalf("10-sigma excess over a 1-sigma inlier: Student-t %.3f, Gaussian %.3f; want positive and under a quarter", robust, gauss)
	}
	if want := 3 * math.Log((1+outlier/4)/(1+inlier/4)); math.Abs(robust-want) > 1e-12 {
		t.Fatalf("Student-t excess %.15f, want ((ν+m)/2) ln((1+d²ₒ/ν)/(1+d²ᵢ/ν)) = %.15f", robust, want)
	}
	t.Logf("10-sigma excess over 1 sigma: Gaussian %.3f nats, Student-t ν=4 %.3f nats", gauss, robust)
}
