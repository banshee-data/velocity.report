package motion

import (
	"math"
	"testing"
)

// Over two observations the smoothed first state is the founding estimate
// conditioned on the second observation, worked as a Gaussian conditioning:
// x0|1 = x0 + P0 Fᵀ Hᵀ S⁻¹ (z1 − H F x0), with S = H (F P0 Fᵀ + Q) Hᵀ + R.
func TestSmoothCVIsTheConditionalMeanOverTwoObservations(t *testing.T) {
	cfg := DefaultCVConfig()
	obs := []Observation{
		{UnixNanos: 0, X: 1, Y: 2, R: IsotropicR(0.05)},
		{UnixNanos: frameNanos, X: 1.7, Y: 1.9, R: IsotropicR(0.05)},
	}
	steps := mustRun(t, cfg, obs)
	smoothed, err := SmoothCV(steps)
	if err != nil {
		t.Fatal(err)
	}
	if smoothed[1].X != steps[1].Posterior.X {
		t.Fatalf("the last smoothed state %v is not the last posterior %v", smoothed[1].X, steps[1].Posterior.X)
	}
	x0, p0 := steps[0].Posterior.X, steps[0].Posterior.P
	f, q := cfg.Model.Discretise(0.1)
	prior := quadratic(f, p0).add(q)
	// H picks (x, y); S = prior's position block + R.
	s := [2][2]float64{{prior[0][0] + 0.05, prior[0][1]}, {prior[1][0], prior[1][1] + 0.05}}
	inv, _, ok := invertSPD(s, 2)
	if !ok {
		t.Fatal("S is singular")
	}
	fx := f.apply(x0)
	y := [2]float64{obs[1].X - fx[0], obs[1].Y - fx[1]}
	g := p0.mul(f.transpose()) // P0 Fᵀ; Hᵀ keeps its first two columns
	for i := 0; i < 4; i++ {
		want := x0[i]
		for a := 0; a < 2; a++ {
			for b := 0; b < 2; b++ {
				want += g[i][a] * inv[a][b] * y[b]
			}
		}
		if math.Abs(smoothed[0].X[i]-want) > 1e-9 {
			t.Errorf("x0|1[%d] = %.9f, want %.9f", i, smoothed[0].X[i], want)
		}
	}
}

// On a noise-free constant-velocity track the smoothed velocity is the true
// one from the first observation on, where the filter's starts at zero, and
// smoothing never widens a variance.
func TestSmoothCVRecoversAConstantVelocityFromTheStart(t *testing.T) {
	steps := mustRun(t, DefaultCVConfig(), observe(constantVelocity(6), 3, -1, -1))
	smoothed, err := SmoothCV(steps)
	if err != nil {
		t.Fatal(err)
	}
	if v := steps[1].Posterior.X[IdxVX]; math.Abs(v-6) < 1 {
		t.Fatalf("the filter's second velocity %.3f is already near 6; the test needs a converging filter", v)
	}
	for k, e := range smoothed {
		if math.Abs(e.X[IdxVX]-6) > 0.05 || math.Abs(e.X[IdxVY]) > 1e-9 {
			t.Errorf("step %d: smoothed velocity (%.4f, %.4f), want (6, 0)", k, e.X[IdxVX], e.X[IdxVY])
		}
		for i := 0; i < 4; i++ {
			if e.P[i][i] > steps[k].Posterior.P[i][i]+1e-12 {
				t.Errorf("step %d: smoothed variance %d %.6g exceeds the filtered %.6g", k, i, e.P[i][i], steps[k].Posterior.P[i][i])
			}
		}
	}
}

// A run that is not CV's, or does not start at its founding, is refused.
func TestSmoothCVRefusesWhatItCannotSmooth(t *testing.T) {
	obs := observe(constantAcceleration(5, -1), 2, -1, -1)
	ca := mustRun(t, DefaultCAConfig(), obs)
	if _, err := SmoothCV(ca); err == nil {
		t.Error("a CA run was smoothed as CV")
	}
	cv := mustRun(t, DefaultCVConfig(), obs)
	if _, err := SmoothCV(cv[1:]); err == nil {
		t.Error("a run without its founding step was smoothed")
	}
	if out, err := SmoothCV(nil); err != nil || out != nil {
		t.Errorf("empty run: %v %v", out, err)
	}
}
