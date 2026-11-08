package motion

import (
	"go/parser"
	"go/token"
	"math"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Noise-free constant velocity: CV and the IMM recover it, and the IMM
// settles on CV.
func TestConstantVelocityIsRecovered(t *testing.T) {
	const v = 6.0
	obs := observe(constantVelocity(v), 20, -1, -1)

	cv := mustRun(t, DefaultCVConfig(), obs)
	last := cv[len(cv)-1].Posterior
	if err := math.Abs(last.X[IdxVX] - v); err > 1e-3 {
		t.Errorf("CV velocity error %.2e m/s, want < 1e-3", err)
	}
	if err := math.Abs(last.X[IdxX] - v*20); err > 1e-3 {
		t.Errorf("CV position error %.2e m, want < 1e-3", err)
	}

	imm := mustRunIMM(t, DefaultIMMConfig(), obs)
	final := imm[len(imm)-1]
	if err := math.Abs(final.Posterior.X[IdxVX] - v); err > 1e-3 {
		t.Errorf("IMM velocity error %.2e m/s, want < 1e-3", err)
	}
	// After convergence (the last 5 s) CV carries at least 0.9.
	for _, s := range imm[150:] {
		if s.ModeProb[ModeCV] < 0.9 {
			t.Fatalf("IMM CV probability %.4f at %.1f s, want >= 0.9", s.ModeProb[ModeCV], float64(s.UnixNanos)/1e9)
		}
	}
	t.Logf("IMM mode probabilities at 20 s: CV %.4f, CA %.4f", final.ModeProb[ModeCV], final.ModeProb[ModeCA])
	if math.Abs(final.ModeProb[0]+final.ModeProb[1]-1) > 1e-12 {
		t.Errorf("mode probabilities sum to %v", final.ModeProb[0]+final.ModeProb[1])
	}
}

// Constant 1 m/s²: CV at the shipped tuning lags 0.71 m/s (maths review,
// Section 4.1); CA has none in steady state.
func TestConstantAccelerationVelocityLag(t *testing.T) {
	tr := constantAcceleration(0, 1)
	obs := observe(tr, 20, -1, -1)
	_, vTrue := tr(20)
	lag := func(cfg FilterConfig) float64 {
		steps := mustRun(t, cfg, obs)
		return vTrue - steps[len(steps)-1].Posterior.X[IdxVX]
	}
	shipped := lag(shippedCV())
	coupled := lag(DefaultCVConfig())
	ca := lag(DefaultCAConfig())
	t.Logf("velocity lag at 1 m/s²: shipped CV %.4f, coupled CV %.4f, CA %.2e m/s", shipped, coupled, ca)
	if shipped < 0.70 || shipped > 0.72 {
		t.Errorf("shipped CV lag %.4f m/s, want 0.71 ± 0.01", shipped)
	}
	if math.Abs(ca) > 1e-3 {
		t.Errorf("CA lag %.2e m/s, want negligible (< 1e-3)", ca)
	}

	imm := mustRunIMM(t, DefaultIMMConfig(), obs)
	final := imm[len(imm)-1]
	t.Logf("IMM at 1 m/s²: lag %.4f m/s, CA probability %.4f", vTrue-final.Posterior.X[IdxVX], final.ModeProb[ModeCA])
	m, _ := NewIMM(DefaultIMMConfig())
	if stationary := m.Rates().Stationary()[ModeCA]; final.ModeProb[ModeCA] <= stationary {
		t.Errorf("IMM CA probability %.4f under constant acceleration, not above its stationary %.4f", final.ModeProb[ModeCA], stationary)
	}
	if l := vTrue - final.Posterior.X[IdxVX]; l >= shipped || l < 0 {
		t.Errorf("IMM lag %.4f m/s, want between 0 and the shipped CV's %.4f", l, shipped)
	}
}

// Braking to a stop inside a 2.5 s gap (8 m/s at 2 m/s², stopped at 4 s, gap
// from 3 s to 5.5 s). Without the coast rule CA reverses the vehicle; with it
// the velocity along the course ends at zero and the reacquisition error is
// below CV's.
func TestBrakingToStopInsideGap(t *testing.T) {
	tr := brakingToStop(8, 2)
	obs := observe(tr, 7, 3, 5.5)
	xTrue, _ := tr(5.5)

	type result struct {
		alongVel, err float64
		constrained   bool
	}
	run := func(cfg FilterConfig) result {
		steps := mustRun(t, cfg, obs)
		i := gapStepIndex(t, steps)
		if !steps[i].Coasted || steps[i].SubSteps != 25 {
			t.Fatalf("gap step coasted=%v sub-steps=%d, want true and 25", steps[i].Coasted, steps[i].SubSteps)
		}
		pre := steps[i-1].Posterior.X
		ux := math.Copysign(1, pre[IdxVX]) // course along +x
		return result{ux * steps[i].Prior.X[IdxVX], steps[i].Prior.X[IdxX] - xTrue, steps[i].Constrained}
	}
	cv := run(DefaultCVConfig())
	caRule := run(DefaultCAConfig())
	caBare := run(noCoast(DefaultCAConfig()))
	t.Logf("reacquisition error: CV %+.3f m, CA with rule %+.3f m (end velocity %+.3f), CA without %+.3f m (end velocity %+.3f)",
		cv.err, caRule.err, caRule.alongVel, caBare.err, caBare.alongVel)

	if caBare.alongVel >= -1 {
		t.Errorf("CA without the coast rule ends at %+.3f m/s along the course, want reversing (< -1)", caBare.alongVel)
	}
	if caRule.alongVel < -1e-9 {
		t.Errorf("CA with the coast rule ends at %+.3e m/s along the course, want >= 0", caRule.alongVel)
	}
	if !caRule.constrained || caBare.constrained {
		t.Errorf("constraint fired: with rule %v, without %v", caRule.constrained, caBare.constrained)
	}
	if math.Abs(caRule.err) >= math.Abs(cv.err) {
		t.Errorf("CA with rule reacquisition |%.3f| m not below CV's |%.3f| m", caRule.err, cv.err)
	}

	// The IMM's CA mode coasts under the same rule. Mixing dilutes its
	// acceleration with CV's zero, so here it decays to rest without the
	// constraint having to fire; either way it must not reverse.
	imm := mustRunIMM(t, DefaultIMMConfig(), obs)
	for _, s := range imm {
		if s.DtSecs > 0.15 {
			ca := s.Modes[ModeCA]
			t.Logf("IMM CA mode ends the gap at %+.4f m/s (constrained %v)", ca.Prior.X[IdxVX], ca.Constrained)
			if ca.Prior.X[IdxVX] < -1e-9 || s.Prior.X[IdxVX] < -1e-9 {
				t.Errorf("IMM reverses: CA mode %+.3e m/s, combined %+.3e m/s", ca.Prior.X[IdxVX], s.Prior.X[IdxVX])
			}
			e := s.Prior.X[IdxX] - xTrue
			t.Logf("IMM reacquisition error %+.3f m, predicted mode probabilities %v", e, s.PredictedProb)
			if math.Abs(e) >= math.Abs(cv.err) {
				t.Errorf("IMM reacquisition |%.3f| m not below CV's |%.3f| m", e, cv.err)
			}
		}
	}
	t.Logf("IMM mode probabilities while braking (2.9 s): CV %.4f, CA %.4f", imm[29].ModeProb[ModeCV], imm[29].ModeProb[ModeCA])

	// With CA dominant (long CA dwell) mixing no longer dilutes the
	// deceleration, and the constraint fires inside the IMM's CA mode.
	cfg := DefaultIMMConfig()
	cfg.DwellCVSecs, cfg.DwellCASecs = 0.5, 100
	for _, s := range mustRunIMM(t, cfg, obs) {
		if s.DtSecs > 0.15 {
			ca := s.Modes[ModeCA]
			if !ca.Constrained || ca.Prior.X[IdxVX] < -1e-9 {
				t.Errorf("CA-dominant IMM: CA mode constrained %v, end velocity %+.3e m/s", ca.Constrained, ca.Prior.X[IdxVX])
			}
		}
	}
}

// The Singer decay alone relaxes an unobserved acceleration as e^{−t/τ}.
func TestCoastDecayRelaxesAcceleration(t *testing.T) {
	cfg := DefaultCAConfig()
	cfg.Coast.NoReversal = false
	f, err := NewFilter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var x Vec
	x[IdxVX], x[IdxAX] = 5, -1
	var p Mat
	for i := 0; i < StateDim; i++ {
		p[i][i] = 0.1
	}
	f.SetState(Estimate{X: x, P: p})
	est, err := f.Predict(2_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	want := -math.Exp(-2 / DefaultCoastTau)
	if math.Abs(est.X[IdxAX]-want) > 1e-12 {
		t.Errorf("acceleration after 2 s %.12f, want %.12f", est.X[IdxAX], want)
	}
	// Observed-interval predictions do not decay.
	obsEst, _ := f.Predict(100_000_000)
	if obsEst.X[IdxAX] != -1 {
		t.Errorf("a one-frame prediction decayed the acceleration to %v", obsEst.X[IdxAX])
	}
}

// Sub-steps compose exactly for the linear models: n sub-steps equal one.
func TestSubStepsComposeExactly(t *testing.T) {
	for _, model := range []ProcessModel{
		{Kind: ModelCV, Q: 0.2, PosVar: 0.05},
		{Kind: ModelCA, Q: 1},
		{Kind: ModelCA, Q: 1, SingerTau: 1.5},
	} {
		var x Vec
		x[IdxVX], x[IdxAX], x[IdxY], x[IdxAY] = 3, -0.5, 1, 0.2
		p := identity().scale(0.3)
		f1, _ := model.Discretise(1.0)
		one := quadratic(f1, p).add(embedQ(model, 1.0))
		xs, ps := x, p
		for i := 0; i < 10; i++ {
			fs, qs := model.Discretise(0.1)
			xs = fs.apply(xs)
			ps = quadratic(fs, ps).add(qs)
		}
		xo := f1.apply(x)
		for i := 0; i < StateDim; i++ {
			if math.Abs(xo[i]-xs[i]) > 1e-12 {
				t.Errorf("%+v: mean[%d] one step %v, ten %v", model, i, xo[i], xs[i])
			}
			for j := 0; j < StateDim; j++ {
				if math.Abs(one[i][j]-ps[i][j]) > 1e-12 {
					t.Errorf("%+v: P[%d][%d] one step %v, ten %v", model, i, j, one[i][j], ps[i][j])
				}
			}
		}
	}
}

func embedQ(m ProcessModel, dt float64) Mat {
	_, q := m.Discretise(dt)
	return q
}

// Singer's Q tends to the white-noise-jerk Q as τ → ∞, and the closed form
// and the quadrature agree across the crossover.
func TestSingerDiscretisation(t *testing.T) {
	const dt, q = 0.1, 1.3
	_, wnj := wnjBlocks(dt, q)
	_, singer := singerBlocks(dt, 1e6, q)
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if rel := math.Abs(singer[i][j]-wnj[i][j]) / wnj[i][j]; rel > 1e-5 {
				t.Errorf("Q[%d][%d]: Singer τ=1e6 %v, WNJ %v", i, j, singer[i][j], wnj[i][j])
			}
		}
	}
	for _, tau := range []float64{0.5, 1.5, 1.9} { // αT = 0.2, 0.067, 0.053
		_, closed := singerBlocks(dt, tau, 1)
		quad := singerQQuadrature(dt, 1/tau)
		for i := 0; i < 3; i++ {
			for j := i; j < 3; j++ {
				if rel := math.Abs(closed[i][j]-quad[i][j]) / quad[i][j]; rel > 1e-7 {
					t.Errorf("τ=%v Q[%d][%d]: closed %v, quadrature %v", tau, i, j, closed[i][j], quad[i][j])
				}
			}
		}
	}
	// Transition: a decays, and the velocity gain is ∫e^{−αs}ds.
	f, _ := singerBlocks(1, 2, 1)
	if math.Abs(f[2][2]-math.Exp(-0.5)) > 1e-15 || math.Abs(f[1][2]-2*(1-math.Exp(-0.5))) > 1e-14 {
		t.Errorf("Singer transition %v", f)
	}
}

// The transition matrix is stochastic, a semigroup, and matches the series
// for expm(ΛΔt).
func TestTransitionMatrix(t *testing.T) {
	r, err := RatesFromDwell(DefaultDwellCVSecs, DefaultDwellCASecs)
	if err != nil {
		t.Fatal(err)
	}
	mul := func(a, b [2][2]float64) (c [2][2]float64) {
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				for k := 0; k < 2; k++ {
					c[i][j] += a[i][k] * b[k][j]
				}
			}
		}
		return c
	}
	for _, dt := range []float64{0, 0.05, 0.1, 0.5, 2.5, 30} {
		p := r.Transition(dt)
		for i := 0; i < 2; i++ {
			if s := p[i][0] + p[i][1]; math.Abs(s-1) > 1e-15 {
				t.Errorf("Π(%v) row %d sums to %v", dt, i, s)
			}
			for j := 0; j < 2; j++ {
				if p[i][j] < 0 || p[i][j] > 1 {
					t.Errorf("Π(%v)[%d][%d] = %v", dt, i, j, p[i][j])
				}
			}
		}
	}
	for _, ab := range [][2]float64{{0.1, 0.1}, {0.1, 2.4}, {1.3, 0.7}, {5, 11}} {
		lhs := r.Transition(ab[0] + ab[1])
		rhs := mul(r.Transition(ab[0]), r.Transition(ab[1]))
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				if math.Abs(lhs[i][j]-rhs[i][j]) > 1e-14 {
					t.Errorf("Π(%v+%v)[%d][%d] = %v, Π(a)Π(b) = %v", ab[0], ab[1], i, j, lhs[i][j], rhs[i][j])
				}
			}
		}
	}
	// expm by its series against the closed form.
	dt := 0.7
	gen := [2][2]float64{{-r.CVToCA * dt, r.CVToCA * dt}, {r.CAToCV * dt, -r.CAToCV * dt}}
	series := [2][2]float64{{1, 0}, {0, 1}}
	term := series
	for k := 1; k < 30; k++ {
		term = mul(term, gen)
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				term[i][j] /= float64(k)
				series[i][j] += term[i][j]
			}
		}
	}
	closed := r.Transition(dt)
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			if math.Abs(series[i][j]-closed[i][j]) > 1e-15 {
				t.Errorf("Π[%d][%d] closed %v, series %v", i, j, closed[i][j], series[i][j])
			}
		}
	}
	// The stationary distribution is invariant.
	pi := r.Stationary()
	p := r.Transition(1)
	for j := 0; j < 2; j++ {
		if v := pi[0]*p[0][j] + pi[1]*p[1][j]; math.Abs(v-pi[j]) > 1e-15 {
			t.Errorf("stationary not invariant: %v vs %v", v, pi[j])
		}
	}
	if _, err := RatesFromDwell(0, 3); err == nil {
		t.Error("zero dwell accepted")
	}
}

// One 10-sigma innovation on a steady track moves the Student-t IMM's mode
// probabilities less than the Gaussian's.
func TestStudentTModeUpdateResistsOutlier(t *testing.T) {
	obs := observe(constantVelocity(6), 10, -1, -1)
	shift := func(kind LikelihoodKind) (before, after float64, sigma float64) {
		cfg := DefaultIMMConfig()
		cfg.Likelihood = Likelihood{Kind: kind}
		m, _ := NewIMM(cfg)
		for _, o := range obs {
			if _, err := m.Step(o); err != nil {
				t.Fatal(err)
			}
		}
		before = m.ModeProb()[ModeCV]
		// The next step's CV innovation sd, read on a copy without the outlier.
		probe := m.Clone()
		next := Observation{UnixNanos: obs[len(obs)-1].UnixNanos + frameNanos, X: 6 * 10.1, R: IsotropicR(DefaultMeasurementVariance)}
		s, err := probe.Step(next)
		if err != nil {
			t.Fatal(err)
		}
		sigma = math.Sqrt(s.Modes[ModeCV].Innovation.S[1][1])
		next.Y = 10 * sigma
		out, err := m.Step(next)
		if err != nil {
			t.Fatal(err)
		}
		return before, out.ModeProb[ModeCV], sigma
	}
	gb, ga, sigma := shift(LikelihoodGaussian)
	tb, ta, _ := shift(LikelihoodStudentT)
	t.Logf("10σ (%.3f m) lateral outlier: Gaussian CV %.4f -> %.4f; Student-t(ν=4) CV %.4f -> %.4f", 10*sigma, gb, ga, tb, ta)
	if math.Abs(tb-ta) >= math.Abs(gb-ga) {
		t.Errorf("Student-t moved by %.4f, Gaussian by %.4f; want Student-t less", math.Abs(tb-ta), math.Abs(gb-ga))
	}
}

// Over a long noisy run with gaps, braking and stops, every posterior and
// prior covariance stays symmetric positive semi-definite.
func TestCovarianceStaysSymmetricPSD(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var obs []Observation
	x, y, vx, vy := 0.0, 0.0, 8.0, 1.0
	tNanos := int64(0)
	for k := 0; k < 6000; k++ { // ten minutes at 10 Hz
		dt := 0.1
		if rng.Float64() < 0.02 {
			dt += float64(rng.Intn(30)) * 0.1 // gaps up to 3 s
		}
		ax := 0.0
		if (k/200)%3 == 1 {
			ax = -1.5
		}
		vx = math.Max(0, vx+ax*dt)
		if vx == 0 && (k/200)%3 == 2 {
			vx = 6
		}
		x += vx * dt
		y += vy * dt
		tNanos += int64(math.Round(dt * 1e9))
		r := 0.03 + 0.1*rng.Float64()
		obs = append(obs, Observation{
			UnixNanos: tNanos, X: x + rng.NormFloat64()*math.Sqrt(r), Y: y + rng.NormFloat64()*math.Sqrt(r),
			R: [2][2]float64{{r, 0.2 * r}, {0.2 * r, r}},
		})
	}
	check := func(name string, i int, p Mat) {
		if !IsSymmetricPSD(p, 1e-9) {
			t.Fatalf("%s step %d: covariance not symmetric PSD: %v", name, i, p)
		}
	}
	for name, cfg := range map[string]FilterConfig{"cv": DefaultCVConfig(), "ca": DefaultCAConfig(), "ca_bare": noCoast(DefaultCAConfig())} {
		for i, s := range mustRun(t, cfg, obs) {
			check(name+" prior", i, s.Prior.P)
			check(name+" posterior", i, s.Posterior.P)
		}
	}
	for i, s := range mustRunIMM(t, DefaultIMMConfig(), obs) {
		check("imm prior", i, s.Prior.P)
		check("imm posterior", i, s.Posterior.P)
		for j := 0; j < nModes; j++ {
			check("imm mode posterior", i, s.Modes[j].Posterior.P)
		}
	}
}

// The cap scales rows and columns together, so a correlation survives and P
// stays PSD; clamping the diagonal alone would not.
func TestCapCovariancePreservesCorrelation(t *testing.T) {
	var p Mat
	p[0][0], p[2][2], p[0][2], p[2][0] = 400, 4, 39, 39 // correlation 0.975
	for i := 1; i < StateDim; i++ {
		if i != 2 {
			p[i][i] = 1
		}
	}
	capped := CapCovariance(p, 100)
	if capped[0][0] != 100 || capped[2][2] != 4 {
		t.Fatalf("diagonal after cap %v, %v", capped[0][0], capped[2][2])
	}
	corr := func(m Mat) float64 { return m[0][2] / math.Sqrt(m[0][0]*m[2][2]) }
	if math.Abs(corr(capped)-corr(p)) > 1e-12 {
		t.Errorf("correlation %v became %v", corr(p), corr(capped))
	}
	if !IsSymmetricPSD(capped, 1e-12) {
		t.Error("capped covariance not PSD")
	}
	naive := p
	naive[0][0] = 100
	if IsSymmetricPSD(naive, 1e-12) {
		t.Error("a diagonal-only clamp stayed PSD; the example no longer demonstrates the defect")
	}
}

// The constraint conditions through the regression: the constrained
// component is exact, its correlate moves by cov/var times the correction,
// and P loses exactly the variance along h.
func TestConstrainLinear(t *testing.T) {
	var x Vec
	x[IdxX], x[IdxVX] = 1, -0.4
	var p Mat
	p[IdxX][IdxX], p[IdxVX][IdxVX], p[IdxX][IdxVX], p[IdxVX][IdxX] = 0.5, 0.2, 0.15, 0.15
	p[IdxY][IdxY], p[IdxVY][IdxVY], p[IdxAX][IdxAX], p[IdxAY][IdxAY] = 1, 1, 1, 1
	var h Vec
	h[IdxVX] = 1
	cx, cp := constrainLinear(x, p, h, 0)
	if cx[IdxVX] != 0 {
		t.Errorf("constrained velocity %v", cx[IdxVX])
	}
	if want := 1 + 0.15/0.2*0.4; math.Abs(cx[IdxX]-want) > 1e-15 {
		t.Errorf("position %v, want %v (regression)", cx[IdxX], want)
	}
	if want := 0.5 - 0.15*0.15/0.2; math.Abs(cp[IdxX][IdxX]-want) > 1e-15 {
		t.Errorf("position variance %v, want %v", cp[IdxX][IdxX], want)
	}
	if cp[IdxVX][IdxVX] > 1e-15 || cp[IdxX][IdxVX] > 1e-15 {
		t.Errorf("variance along h remains: %v, %v", cp[IdxVX][IdxVX], cp[IdxX][IdxVX])
	}
	if !IsSymmetricPSD(cp, 1e-12) {
		t.Error("constrained covariance not PSD")
	}
}

// The Joseph form equals the standard (I − KH)P at the optimal gain.
func TestJosephUpdateMatchesStandardForm(t *testing.T) {
	var x Vec
	p := identity().scale(0.4)
	p[IdxX][IdxVX], p[IdxVX][IdxX] = 0.1, 0.1
	p[IdxY][IdxAY], p[IdxAY][IdxY] = -0.05, -0.05
	obs := Observation{X: 0.3, Y: -0.2, R: [2][2]float64{{0.05, 0.01}, {0.01, 0.07}}}
	xj, pj, in, err := josephUpdate(x, p, obs)
	if err != nil {
		t.Fatal(err)
	}
	// Standard form with K = PHᵀS⁻¹.
	inv, _, _ := invertSPD(in.S, 2)
	var k [StateDim][2]float64
	for s := 0; s < StateDim; s++ {
		for c := 0; c < 2; c++ {
			k[s][0] += p[s][c] * inv[c][0]
			k[s][1] += p[s][c] * inv[c][1]
		}
	}
	for i := 0; i < StateDim; i++ {
		want := x[i] + k[i][0]*in.Y[0] + k[i][1]*in.Y[1]
		if math.Abs(xj[i]-want) > 1e-14 {
			t.Errorf("mean[%d] %v, want %v", i, xj[i], want)
		}
		for j := 0; j < StateDim; j++ {
			std := p[i][j] - k[i][0]*p[0][j] - k[i][1]*p[1][j]
			if math.Abs(pj[i][j]-std) > 1e-14 {
				t.Errorf("P[%d][%d] Joseph %v, standard %v", i, j, pj[i][j], std)
			}
		}
	}
}

// A rank-one observation informs only the direction it measures.
func TestRankOneObservation(t *testing.T) {
	f, err := NewFilter(DefaultCVConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Step(Observation{X: 0, Y: 0, R: IsotropicR(0.05)}); err != nil {
		t.Fatal(err)
	}
	s, err := f.Step(Observation{UnixNanos: frameNanos, X: 2, Y: 0.5, R: IsotropicR(0.05), RankOne: true, NX: 0, NY: 2})
	if err != nil {
		t.Fatal(err)
	}
	if s.Innovation.Dim != 1 || math.Abs(s.Innovation.Y[0]-0.5) > 1e-15 {
		t.Fatalf("innovation %+v, want 1-D 0.5", s.Innovation)
	}
	if s.Posterior.X[IdxX] != s.Prior.X[IdxX] || s.Posterior.X[IdxY] <= 0 {
		t.Errorf("rank-one along y moved x %v→%v, y to %v", s.Prior.X[IdxX], s.Posterior.X[IdxX], s.Posterior.X[IdxY])
	}
	g, _ := NewFilter(DefaultCVConfig())
	if _, err := g.Step(Observation{RankOne: true, NX: 1, R: IsotropicR(1)}); err == nil {
		t.Error("a track was founded on a rank-one observation")
	}
}

// The likelihoods: the Gaussian's closed form, the Student-t's Gaussian limit
// and its heavier tail.
func TestLikelihoods(t *testing.T) {
	in := Innovation{Dim: 2, NIS: 3.2, LogDetS: math.Log(0.01)}
	want := -1.6 - math.Log(2*math.Pi) - 0.5*math.Log(0.01)
	if got := GaussianLogLikelihood(in); math.Abs(got-want) > 1e-14 {
		t.Errorf("Gaussian %v, want %v", got, want)
	}
	if got := StudentTLogLikelihood(in, 1e7); math.Abs(got-want) > 1e-5 {
		t.Errorf("Student-t ν→∞ %v, Gaussian %v", got, want)
	}
	far := Innovation{Dim: 2, NIS: 100, LogDetS: math.Log(0.01)}
	if StudentTLogLikelihood(far, 4) <= GaussianLogLikelihood(far) {
		t.Error("Student-t tail not heavier than the Gaussian's at d² = 100")
	}
	if (Likelihood{Kind: LikelihoodStudentT}).LogDensity(far) != StudentTLogLikelihood(far, DefaultStudentTNu) {
		t.Error("zero ν does not default to DefaultStudentTNu")
	}
	// The 1-D Student-t at ν = 1 is the Cauchy density.
	c := Innovation{Dim: 1, NIS: 4, LogDetS: 0}
	if got, want := StudentTLogLikelihood(c, 1), -math.Log(math.Pi*5); math.Abs(got-want) > 1e-14 {
		t.Errorf("Cauchy %v, want %v", got, want)
	}
}

// CV holds acceleration at zero with zero covariance whatever it is given.
func TestCVEmbedding(t *testing.T) {
	f, _ := NewFilter(DefaultCVConfig())
	var x Vec
	x[IdxVX], x[IdxAX], x[IdxAY] = 4, 2, -1
	p := identity()
	p[IdxVX][IdxAX], p[IdxAX][IdxVX] = 0.5, 0.5
	f.SetState(Estimate{X: x, P: p})
	est, _ := f.Predict(frameNanos * 5)
	for _, i := range []int{IdxAX, IdxAY} {
		if est.X[i] != 0 {
			t.Errorf("CV acceleration[%d] = %v", i, est.X[i])
		}
		for j := 0; j < StateDim; j++ {
			if est.P[i][j] != 0 || est.P[j][i] != 0 {
				t.Errorf("CV acceleration covariance P[%d][%d] = %v", i, j, est.P[i][j])
			}
		}
	}
	if math.Abs(est.X[IdxX]-2) > 1e-15 {
		t.Errorf("CV position after 0.5 s at 4 m/s: %v", est.X[IdxX])
	}
}

func TestConfigValidation(t *testing.T) {
	bad := []FilterConfig{
		func() FilterConfig { c := DefaultCAConfig(); c.MaxStepSecs = 0; return c }(),
		func() FilterConfig { c := DefaultCAConfig(); c.Model.Q = -1; return c }(),
		func() FilterConfig { c := DefaultCAConfig(); c.Model.Kind = "ctrv"; return c }(),
		func() FilterConfig { c := DefaultCAConfig(); c.Model.Diagonal = true; return c }(),
		func() FilterConfig { c := DefaultCAConfig(); c.Coast.DecayTau = math.NaN(); return c }(),
	}
	for i, c := range bad {
		if _, err := NewFilter(c); err == nil {
			t.Errorf("config %d accepted", i)
		}
	}
	cfg := DefaultIMMConfig()
	cfg.CV, cfg.CA = cfg.CA, cfg.CV
	if _, err := NewIMM(cfg); err == nil {
		t.Error("IMM with swapped modes accepted")
	}
	cfg = DefaultIMMConfig()
	cfg.InitialProb = [2]float64{0.7, 0.7}
	if _, err := NewIMM(cfg); err == nil {
		t.Error("IMM with a non-distribution accepted")
	}
	f, _ := NewFilter(DefaultCVConfig())
	if _, err := f.Step(Observation{UnixNanos: 10, R: IsotropicR(0.05)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Step(Observation{UnixNanos: 5, R: IsotropicR(0.05)}); err != ErrTimeOrder {
		t.Errorf("out-of-order observation: %v", err)
	}
	if _, err := f.Step(Observation{UnixNanos: 20, X: math.NaN(), R: IsotropicR(0.05)}); err == nil {
		t.Error("non-finite observation accepted")
	}
}

// TestMotionDoesNotReadTheClock holds the package's time-domain rule: the
// estimators take capture time from their input and never import time.
func TestMotionDoesNotReadTheClock(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	scanned := 0
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, spec := range file.Imports {
			p, _ := strconv.Unquote(spec.Path.Value)
			if p == "time" || strings.HasSuffix(p, "/timeutil") {
				t.Errorf("%s imports %s", path, p)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no sources scanned")
	}
}
