package l5tracks

import (
	"math"
	"testing"
)

// Tests for gaps K1 and K2 in data/maths/paper-implementation-gap-analysis.md.
//
// Both alternatives are opt-in, so these tests do two jobs: they prove the
// alternative forms have the properties claimed for them, and they pin the
// default to the shipped behaviour so enabling one stays a deliberate,
// measured decision rather than a silent drift.

// leadingMinors returns the four leading principal minors of a symmetric 4x4.
// All strictly positive is Sylvester's criterion for positive-definiteness.
func leadingMinors(p [16]float32) [4]float64 {
	m := func(i, j int) float64 { return float64(p[i*4+j]) }

	d1 := m(0, 0)
	d2 := m(0, 0)*m(1, 1) - m(0, 1)*m(1, 0)
	d3 := m(0, 0)*(m(1, 1)*m(2, 2)-m(1, 2)*m(2, 1)) -
		m(0, 1)*(m(1, 0)*m(2, 2)-m(1, 2)*m(2, 0)) +
		m(0, 2)*(m(1, 0)*m(2, 1)-m(1, 1)*m(2, 0))

	// 4x4 determinant by expansion along the first row, using 3x3 cofactors.
	det3 := func(r [3]int, c [3]int) float64 {
		return m(r[0], c[0])*(m(r[1], c[1])*m(r[2], c[2])-m(r[1], c[2])*m(r[2], c[1])) -
			m(r[0], c[1])*(m(r[1], c[0])*m(r[2], c[2])-m(r[1], c[2])*m(r[2], c[0])) +
			m(r[0], c[2])*(m(r[1], c[0])*m(r[2], c[1])-m(r[1], c[1])*m(r[2], c[0]))
	}
	rows := [3]int{1, 2, 3}
	d4 := m(0, 0)*det3(rows, [3]int{1, 2, 3}) -
		m(0, 1)*det3(rows, [3]int{0, 2, 3}) +
		m(0, 2)*det3(rows, [3]int{0, 1, 3}) -
		m(0, 3)*det3(rows, [3]int{0, 1, 2})

	return [4]float64{d1, d2, d3, d4}
}

// K1 ---------------------------------------------------------------------

func TestDiagonalProcessNoiseLeavesPositionAndVelocityUncorrelated(t *testing.T) {
	var p [16]float32
	addProcessNoise(&p, 0.05, 0.2, 0.1, false)

	if p[0*4+2] != 0 || p[2*4+0] != 0 {
		t.Errorf("diagonal Q wrote an x/vx cross term: %v", p[0*4+2])
	}
	if p[1*4+3] != 0 || p[3*4+1] != 0 {
		t.Errorf("diagonal Q wrote a y/vy cross term: %v", p[1*4+3])
	}
	// The shipped values, so a change to the default is visible here.
	if math.Abs(float64(p[0*4+0])-0.005) > 1e-7 {
		t.Errorf("position term = %v, want posVar*dt = 0.005", p[0*4+0])
	}
	if math.Abs(float64(p[2*4+2])-0.02) > 1e-7 {
		t.Errorf("velocity term = %v, want velVar*dt = 0.02", p[2*4+2])
	}
}

func TestCoupledProcessNoiseMatchesTheContinuousModel(t *testing.T) {
	const posVar, velVar, dt = 0.05, 0.2, 0.1
	var p [16]float32
	addProcessNoise(&p, posVar, velVar, dt, true)

	// Q = q * [[dt³/3, dt²/2], [dt²/2, dt]], plus the retained independent
	// position-jitter term.
	wantPos := posVar*dt + velVar*dt*dt*dt/3
	wantCross := velVar * dt * dt / 2
	wantVel := velVar * dt

	for _, tc := range []struct {
		name string
		got  float32
		want float64
	}{
		{"x,x", p[0*4+0], wantPos},
		{"y,y", p[1*4+1], wantPos},
		{"vx,vx", p[2*4+2], wantVel},
		{"vy,vy", p[3*4+3], wantVel},
		{"x,vx", p[0*4+2], wantCross},
		{"vx,x", p[2*4+0], wantCross},
		{"y,vy", p[1*4+3], wantCross},
		{"vy,y", p[3*4+1], wantCross},
	} {
		if math.Abs(float64(tc.got)-tc.want) > 1e-7 {
			t.Errorf("%s term = %v, want %v", tc.name, tc.got, tc.want)
		}
	}

	// The two axes stay independent: only each axis's own position and
	// velocity are coupled, so x must not correlate with vy.
	if p[0*4+3] != 0 || p[1*4+2] != 0 {
		t.Errorf("coupled Q correlated the two axes: x/vy=%v y/vx=%v", p[0*4+3], p[1*4+2])
	}
	if a := covarianceAsymmetry(p); a != 0 {
		t.Errorf("coupled Q is not symmetric, worst asymmetry %v", a)
	}
}

func TestCoupledProcessNoiseGrowsPositionUncertaintyFasterOverAGap(t *testing.T) {
	// Over a long coast the dt³ term is what the diagonal form is missing:
	// position uncertainty should grow faster, not the same.
	const posVar, velVar = 0.05, 0.2
	for _, dt := range []float32{0.1, 0.5} {
		var diag, coupled [16]float32
		addProcessNoise(&diag, posVar, velVar, dt, false)
		addProcessNoise(&coupled, posVar, velVar, dt, true)

		if coupled[0] <= diag[0] {
			t.Errorf("dt=%v: coupled position term %v is not above diagonal %v", dt, coupled[0], diag[0])
		}
		// Velocity growth is unchanged: the coupled form adds the missing
		// terms rather than reinterpreting the tuned velocity noise.
		if math.Abs(float64(coupled[2*4+2]-diag[2*4+2])) > 1e-7 {
			t.Errorf("dt=%v: coupled changed the velocity term", dt)
		}
	}
}

func TestCoupledProcessNoiseLowersTheNormalisedDistanceToAManoeuvringTarget(t *testing.T) {
	// The gap analysis proposed this test as "verify the full-Q ellipse
	// captures the target when diagonal does not". It does not: measured at
	// the shipped tuning, both forms accept the target with room to spare, and
	// that measurement is the point of the test. See the Measured outcome note
	// in data/maths/paper-implementation-gap-analysis.md.
	const accel = 2.0
	const dt float32 = 0.5 // MaxPredictDt: the longest single step the filter takes

	// Same starting belief for both: a track moving at 10 m/s along +X with a
	// modest, uncorrelated covariance.
	startingTrack := func() *TrackedObject {
		return &TrackedObject{
			X: 0, Y: 0, VX: 10, VY: 0,
			P: [16]float32{
				0.1, 0, 0, 0,
				0, 0.1, 0, 0,
				0, 0, 0.5, 0,
				0, 0, 0, 0.5,
			},
		}
	}

	// Where the target really is after dt, having accelerated: further along
	// than a constant-velocity prediction expects.
	trueX := 10*float64(dt) + 0.5*accel*float64(dt)*float64(dt)
	cluster := WorldCluster{CentroidX: float32(trueX), CentroidY: 0}

	dist := map[bool]float32{}
	for _, coupled := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.CoupledProcessNoise = coupled
		tr := NewTracker(cfg)
		track := startingTrack()
		tr.predict(track, dt)
		dist[coupled] = tr.mahalanobisDistanceSquared(track, cluster, dt)
	}

	// The direction is the real, reproducible effect: the added cross term
	// widens the ellipse along the position-velocity correlation direction,
	// which is the direction the target departed along.
	if dist[true] >= dist[false] {
		t.Errorf("coupled Q should give the manoeuvring target a smaller normalised distance: coupled=%v diagonal=%v",
			dist[true], dist[false])
	}

	// The magnitude is the finding. Both distances sit far inside the gate, so
	// the diagonal form's overconfidence costs nothing here: what governs
	// association at this tuning is the width of the gate, not the Q form.
	// Asserted rather than logged so that a tuning change tight enough to make
	// K1 matter surfaces as a failure here instead of going unnoticed.
	gate := DefaultTrackerConfig().GatingDistanceSquared
	if dist[false] > gate {
		t.Errorf("the shipped diagonal form now rejects a 2 m/s² manoeuvre (d²=%v, gate=%v): K1 has become a real gating defect and enabling the coupled form is worth measuring",
			dist[false], gate)
	}
	t.Logf("d² against a 2 m/s² manoeuvre over %.1fs: diagonal=%.4f coupled=%.4f (gate=%v, margin %.0fx)",
		dt, dist[false], dist[true], gate, float64(gate)/float64(dist[false]))
}

func TestPredictDefaultsToTheShippedDiagonalForm(t *testing.T) {
	// The default must not move: it would change tracker output and make the
	// committed perf and corpus baselines incomparable.
	cfg := DefaultTrackerConfig()
	if cfg.CoupledProcessNoise {
		t.Error("coupled process noise must be opt-in")
	}
	if cfg.JosephCovarianceUpdate {
		t.Error("Joseph covariance update must be opt-in")
	}

	tr := NewTracker(cfg)
	track := &TrackedObject{VX: 1, VY: 1}
	tr.predict(track, 0.1)
	if track.P[0*4+2] != 0 {
		t.Errorf("default predict wrote a cross term: %v", track.P[0*4+2])
	}
}

// K2 ---------------------------------------------------------------------

// kalmanGain is the optimal gain for the position-only measurement model,
// matching what update() computes. Both covariance forms are algebraically
// equal only at this gain, which is the premise the comparison rests on.
func kalmanGain(p [16]float32, measurementNoise float32) [8]float32 {
	s00 := p[0*4+0] + measurementNoise
	s01 := p[0*4+1]
	s10 := p[1*4+0]
	s11 := p[1*4+1] + measurementNoise
	det := s00*s11 - s01*s10

	inv00 := s11 / det
	inv01 := -s01 / det
	inv10 := -s10 / det
	inv11 := s00 / det

	var k [8]float32
	for i := 0; i < 4; i++ {
		k[i*2+0] = p[i*4+0]*inv00 + p[i*4+1]*inv10
		k[i*2+1] = p[i*4+0]*inv01 + p[i*4+1]*inv11
	}
	return k
}

func TestJosephAndNaiveAgreeOnASingleStepAtTheOptimalGain(t *testing.T) {
	// They are the same expression at the optimal gain. If they disagreed
	// here, the Joseph implementation would be wrong rather than merely more
	// stable, so this is the correctness check that licenses the comparison.
	p := [16]float32{
		2.0, 0.3, 0.5, 0.1,
		0.3, 1.8, 0.1, 0.4,
		0.5, 0.1, 1.2, 0.2,
		0.1, 0.4, 0.2, 1.1,
	}
	const r float32 = 0.05
	k := kalmanGain(p, r)

	naive := naiveCovarianceUpdate(p, k)
	joseph := josephCovarianceUpdate(p, k, r)

	for i := 0; i < 16; i++ {
		if math.Abs(float64(naive[i]-joseph[i])) > 1e-5 {
			t.Fatalf("element %d differs: naive=%v joseph=%v", i, naive[i], joseph[i])
		}
	}
}

func TestJosephFormKeepsCovarianceSymmetricAndPositiveDefiniteOverManyCycles(t *testing.T) {
	// The gap analysis's own test for K2: 10,000 predict-update cycles with
	// adversarial measurement noise. The row predicted that the naive form,
	// not being a symmetric expression, would accumulate float error
	// asymmetrically. It does not — see the note at the end of this test.
	const cycles = 10_000
	const r float32 = 0.01 // tight measurement noise makes the gain aggressive
	const dt float32 = 0.1

	run := func(joseph bool) (worstAsymmetry float32, finalP [16]float32) {
		p := [16]float32{
			1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		}
		for i := 0; i < cycles; i++ {
			addProcessNoise(&p, 0.05, 0.2, dt, false)
			k := kalmanGain(p, r)
			if joseph {
				p = josephCovarianceUpdate(p, k, r)
			} else {
				p = naiveCovarianceUpdate(p, k)
			}
			if a := covarianceAsymmetry(p); a > worstAsymmetry {
				worstAsymmetry = a
			}
		}
		return worstAsymmetry, p
	}

	naiveWorst, naiveP := run(false)
	josephWorst, josephP := run(true)

	// Joseph must hold symmetry to float precision.
	if josephWorst > 1e-6 {
		t.Errorf("Joseph form lost symmetry: worst asymmetry %v over %d cycles", josephWorst, cycles)
	}
	// It must not be worse than the shipped form.
	if josephWorst > naiveWorst {
		t.Errorf("Joseph asymmetry %v is worse than naive %v", josephWorst, naiveWorst)
	}

	// Positive-definite by Sylvester's criterion. A covariance that loses
	// this makes the gating inverse meaningless rather than merely imprecise.
	for _, minor := range leadingMinors(josephP) {
		if minor <= 0 {
			t.Errorf("Joseph form left P not positive-definite: minors %v", leadingMinors(josephP))
			break
		}
	}

	// The measured answer to K2, which is not the one the gap analysis
	// anticipated: at this filter's conditioning the shipped form does not
	// drift either. Both hold symmetry exactly over 10,000 cycles and settle
	// to the same variance, so there is no instability here for the Joseph
	// form to fix. That confirms the gap's own "Low impact" rating and is the
	// reason the default is unchanged — switching it would be churn against
	// the perf and corpus baselines for no measured gain. Asserted, not just
	// logged, so a future change that does introduce drift shows up here.
	if naiveWorst > 1e-6 {
		t.Errorf("the shipped form drifted (worst asymmetry %v): K2 now has a measurable cost and the default is worth revisiting", naiveWorst)
	}
	if math.Abs(float64(naiveP[0]-josephP[0])) > 1e-6 {
		t.Errorf("the two forms settled to different variances: naive=%v joseph=%v", naiveP[0], josephP[0])
	}
	t.Logf("steady-state position variance: naive=%v joseph=%v", naiveP[0], josephP[0])
	t.Logf("worst asymmetry over %d cycles: naive=%v joseph=%v", cycles, naiveWorst, josephWorst)
}

func TestBothFormsReduceAnAlreadyAsymmetricCovariance(t *testing.T) {
	// Handed a covariance that has already drifted, the Joseph form reduces
	// the asymmetry — but so does the shipped form, by the same amount. Worth
	// pinning explicitly: the Joseph form's advantage is that it cannot
	// introduce asymmetry, not that it repairs existing asymmetry better.
	p := [16]float32{
		2.0, 0.30, 0.5, 0.1,
		0.25, 1.8, 0.1, 0.4, // 0.25 against 0.30: drifted
		0.5, 0.1, 1.2, 0.2,
		0.1, 0.4, 0.2, 1.1,
	}
	const r float32 = 0.05
	k := kalmanGain(p, r)

	before := covarianceAsymmetry(p)
	naive := covarianceAsymmetry(naiveCovarianceUpdate(p, k))
	joseph := covarianceAsymmetry(josephCovarianceUpdate(p, k, r))

	if joseph >= before {
		t.Errorf("Joseph form did not reduce asymmetry: before=%v after=%v", before, joseph)
	}
	if math.Abs(float64(naive-joseph)) > 1e-6 {
		t.Errorf("expected both forms to repair equally from an asymmetric input: naive=%v joseph=%v", naive, joseph)
	}
}

func TestUpdateUsesTheConfiguredCovarianceForm(t *testing.T) {
	// The wiring, not the maths: a tracker with the option on must actually
	// take the Joseph path. Checked by the posterior's values rather than by
	// its symmetry, because the two forms are equally symmetric here — they
	// differ only in float rounding, which is the honest observable.
	posterior := func(joseph bool) [16]float32 {
		cfg := DefaultTrackerConfig()
		cfg.JosephCovarianceUpdate = joseph
		tr := NewTracker(cfg)
		track := &TrackedObject{
			X: 0, Y: 0, VX: 1, VY: 0,
			P: [16]float32{
				2.0, 0.3, 0.5, 0.1,
				0.25, 1.8, 0.1, 0.4,
				0.5, 0.1, 1.2, 0.2,
				0.1, 0.4, 0.2, 1.1,
			},
		}
		tr.update(track, WorldCluster{CentroidX: 1.2, CentroidY: 0.1}, 1_000)
		return track.P
	}

	naive := posterior(false)
	joseph := posterior(true)

	differs := false
	for i := 0; i < 16; i++ {
		if naive[i] != joseph[i] {
			differs = true
			break
		}
	}
	if !differs {
		t.Error("update produced an identical posterior with the option on and off: the Joseph path is not wired up")
	}
	// Same answer, different arithmetic: the difference must stay at rounding
	// scale, or one of the two forms is wrong rather than merely different.
	for i := 0; i < 16; i++ {
		if math.Abs(float64(naive[i]-joseph[i])) > 1e-5 {
			t.Errorf("element %d differs beyond rounding: naive=%v joseph=%v", i, naive[i], joseph[i])
		}
	}
}
