package l8analytics

import (
	"math"
	"math/rand"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks/motion"
)

// Criterion 2 recovers an injected partial correlation, and returns about
// zero when NIS depends on acceleration only through a shared covariate.
func TestPartialCorrelation(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const n = 20000
	c1, c2, c3 := make([]float64, n), make([]float64, n), make([]float64, n)
	acc, nis, null := make([]float64, n), make([]float64, n), make([]float64, n)
	const beta = 0.6
	for i := 0; i < n; i++ {
		c1[i], c2[i], c3[i] = rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()
		acc[i] = 0.8*c1[i] + rng.NormFloat64()
		nis[i] = beta*acc[i] + c2[i] + 0.5*c3[i] + rng.NormFloat64()
		null[i] = 2*c1[i] + rng.NormFloat64()
	}
	covs := [][]float64{c1, c2, c3}

	// Residuals are e_a and β e_a + e_n, so ρ = β / √(β² + 1).
	want := beta / math.Sqrt(beta*beta+1)
	got, err := PartialCorrelation(nis, acc, covs, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("injected ρ %.4f: r %.4f [%.4f, %.4f], n %d", want, got.R, got.Lo, got.Hi, got.N)
	if math.Abs(got.R-want) > 0.02 || got.Lo > want || got.Hi < want {
		t.Errorf("partial correlation %.4f [%.4f, %.4f], want %.4f inside", got.R, got.Lo, got.Hi, want)
	}
	if got.N != n || got.Covariates != 3 || got.Level != 0.95 {
		t.Errorf("result metadata %+v", got)
	}
	if se := (math.Atanh(got.Hi) - math.Atanh(got.R)) / 1.959963984540054; math.Abs(se-1/math.Sqrt(n-6)) > 1e-9 {
		t.Errorf("Fisher-z standard error %v, want %v", se, 1/math.Sqrt(n-6))
	}

	// NIS depends only on c1, which also drives acceleration: the raw
	// correlation is large, the partial one is not.
	raw, _ := PartialCorrelation(null, acc, nil, 0)
	part, err := PartialCorrelation(null, acc, covs, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("covariate-only NIS: raw r %.4f, partial r %.4f [%.4f, %.4f]", raw.R, part.R, part.Lo, part.Hi)
	if raw.R < 0.4 {
		t.Errorf("raw correlation %.4f; the null case no longer confounds", raw.R)
	}
	if math.Abs(part.R) > 0.03 || part.Lo > 0 || part.Hi < 0 {
		t.Errorf("partial correlation %.4f [%.4f, %.4f], want ~0 with 0 inside", part.R, part.Lo, part.Hi)
	}

	// A collinear covariate is dropped, not amplified.
	twice := make([]float64, n)
	for i := range twice {
		twice[i] = 2*c1[i] - 3
	}
	dup, err := PartialCorrelation(nis, acc, [][]float64{c1, c2, c3, twice}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if dup.DroppedCovariates != 1 || dup.Covariates != 3 || math.Abs(dup.R-got.R) > 1e-9 {
		t.Errorf("collinear covariate: %+v, want the same r as without it", dup)
	}

	if _, err := PartialCorrelation([]float64{1, 2, 3, 4}, []float64{1, 3, 2, 4}, [][]float64{{1, 2, 3, 5}}, 0); err != ErrTooFewSamples {
		t.Errorf("four samples, one covariate: %v", err)
	}
	if _, err := PartialCorrelation([]float64{1, math.NaN()}, []float64{1, 2}, nil, 0); err == nil {
		t.Error("non-finite sample accepted")
	}
}

func TestNISAccelerationCorrelationAssembly(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	var samples []NISAccelerationSample
	for i := 0; i < 500; i++ {
		r := 5 + 40*rng.Float64()
		a := rng.NormFloat64()
		samples = append(samples, NISAccelerationSample{
			NIS: 0.05*r + 0.5*a + 0.1*rng.NormFloat64(), Acceleration: a,
			Range: r, Aspect: rng.Float64(), Points: 10 + rng.Intn(200),
		})
	}
	res, err := NISAccelerationCorrelation(samples, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if res.Covariates != 3 || res.Level != 0.9 || res.R < 0.9 {
		t.Errorf("assembled criterion 2: %+v", res)
	}
}

func TestRangeAndAspect(t *testing.T) {
	r, a := RangeAndAspect(13, 4, 1, 0, 1, -1)
	if r != 13 || math.Abs(a-math.Acos(12.0/13)) > 1e-15 {
		t.Errorf("range %v aspect %v", r, a)
	}
	if _, a := RangeAndAspect(10, 0, 0, 3, 0, 0); math.Abs(a-math.Pi/2) > 1e-15 {
		t.Errorf("broadside aspect %v", a)
	}
	if _, a := RangeAndAspect(10, 0, -2, 0, 0, 0); a != 0 {
		t.Errorf("receding end-on aspect %v", a)
	}
	if _, a := RangeAndAspect(10, 0, 0, 0, 0, 0); !math.IsNaN(a) {
		t.Errorf("aspect at rest %v, want NaN", a)
	}
}

// Criterion 3 on hand-made residual sequences.
func TestRunLengthSignature(t *testing.T) {
	tracks := [][]float64{
		{1.5, 1.2, 1.1},                 // run of 3: flagged
		{1.5, -1.2, 1.1, 1.3},           // sign change: longest 2
		{0.5, 2, 2, 0.9, 2, 2, 2},       // second run of 3: flagged
		{-1.1, -1.1, -1.1, -1.1},        // negative run of 4: flagged
		{1.0, 1.0, 1.0},                 // not beyond 1 sigma
		{},                              // not a track
		{2, 2},                          // too short to be eligible
		{1.2, 1.2, 0, 1.2, -3, -3, 0.5}, // interrupted runs
	}
	res := RunLengthSignature(tracks, 0, 0)
	if res.Tracks != 7 || res.Eligible != 6 || res.Flagged != 3 || math.Abs(res.Share-3.0/7) > 1e-15 || res.MinRun != 3 || res.Threshold != 1 {
		t.Errorf("run-length signature %+v", res)
	}
	if got := RunLengthSignature(tracks, 4, 1); got.Flagged != 1 {
		t.Errorf("min run 4 flagged %d, want 1", got.Flagged)
	}
}

func TestLongitudinalResiduals(t *testing.T) {
	step := func(vx, vy, yx, yy float64) motion.Step {
		var s motion.Step
		s.Prior.X[motion.IdxVX], s.Prior.X[motion.IdxVY] = vx, vy
		s.Innovation = motion.Innovation{Dim: 2, Y: [2]float64{yx, yy}, S: [2][2]float64{{0.04, 0}, {0, 0.09}}}
		return s
	}
	founded := step(3, 4, 1, 1)
	founded.Founded = true
	rankOne := step(3, 4, 1, 0)
	rankOne.Innovation.Dim = 1
	steps := []motion.Step{founded, step(3, 4, 0.3, 0.4), step(0.1, 0, 5, 5), rankOne, step(-2, 0, 0.1, 9)}
	got := LongitudinalResiduals(steps, 0.3)
	// Along (0.6, 0.8): y·u = 0.5, uᵀSu = 0.36·0.04 + 0.64·0.09 = 0.072.
	want := []float64{0.5 / math.Sqrt(0.072), -0.1 / 0.2}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Errorf("residual %d: %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSmoothedKinematicsRecoversQuadratic(t *testing.T) {
	var obs []motion.Observation
	for k := 0; k <= 40; k++ {
		tt := float64(k) / 10
		obs = append(obs, motion.Observation{UnixNanos: int64(k) * 100_000_000, X: 1 + 2*tt - 0.75*tt*tt, Y: -tt + 0.25*tt*tt})
	}
	k, ok := SmoothedKinematics(obs, 20, 0.5)
	if !ok || k.N != 11 {
		t.Fatalf("fit ok=%v n=%d", ok, k.N)
	}
	if math.Abs(k.VX-(2-1.5*2)) > 1e-9 || math.Abs(k.VY-(-1+0.5*2)) > 1e-9 || math.Abs(k.AX+1.5) > 1e-9 || math.Abs(k.AY-0.5) > 1e-9 {
		t.Errorf("kinematics %+v", k)
	}
	// At the track's start the window is one-sided but still exact.
	if k, ok := SmoothedKinematics(obs, 0, 0.5); !ok || k.N != 6 || math.Abs(k.AX+1.5) > 1e-9 {
		t.Errorf("one-sided fit %+v ok=%v", k, ok)
	}
	if _, ok := SmoothedKinematics(obs[:4], 1, 0.5); ok {
		t.Error("fit on four observations accepted")
	}
	// Speed (−1, 0) at t = 2 s: longitudinal acceleration is −(−1.5).
	if lon := k.Longitudinal(0.3); math.Abs(lon-1.5) > 1e-9 {
		t.Errorf("longitudinal %v", lon)
	}
}

// holdEstimator predicts the last observed position: its error across a gap
// is the distance travelled, which is computable by hand.
type holdEstimator struct{ x, y float64 }

func (h *holdEstimator) Observe(o motion.Observation) error { h.x, h.y = o.X, o.Y; return nil }
func (h *holdEstimator) PredictPosition(int64) (float64, float64, error) {
	return h.x, h.y, nil
}
func (h *holdEstimator) Current() motion.Estimate         { return motion.Estimate{} }
func (h *holdEstimator) CloneEstimator() motion.Estimator { c := *h; return &c }

func sampledTrack(id string, seq int64, endSecs float64, pos func(t float64) float64) motion.TrackObservations {
	tr := motion.TrackObservations{TrackID: id, CreationSequence: seq}
	for k := 0; k <= int(math.Round(endSecs*10)); k++ {
		tt := float64(k) / 10
		tr.Observations = append(tr.Observations, motion.Observation{
			UnixNanos: int64(k) * 100_000_000, X: pos(tt), R: motion.IsotropicR(motion.DefaultMeasurementVariance),
		})
	}
	return tr
}

// Criterion 5 on a synthetic set whose errors are computed by hand: two
// constant-speed tracks and one braking track, scored with an estimator that
// holds the last position (error = distance travelled across the gap).
func TestScoreOcclusionHandComputed(t *testing.T) {
	tracks := []motion.TrackObservations{
		sampledTrack("a", 1, 4, func(t float64) float64 { return 10 * t }),
		sampledTrack("b", 2, 4, func(t float64) float64 { return 4 * t }),
		sampledTrack("c", 3, 4, func(t float64) float64 { return 10*t - t*t }), // 10 − 2t m/s
	}
	cfg := DefaultOcclusionConfig(1.5)
	models := []OcclusionModel{{Name: "hold", New: func() (motion.Estimator, error) { return &holdEstimator{}, nil }}}
	rep, err := ScoreOcclusion(tracks, models, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Starts at 1, 2 and 3 s; the 1.5 s horizon has no end for the 3 s start.
	if len(rep.Cases) != 3*(3+3+2) {
		t.Fatalf("%d cases, want 24", len(rep.Cases))
	}
	for _, c := range rep.Cases {
		s := float64(c.StartUnixNanos) / 1e9
		h := c.HorizonSecs
		var want float64
		switch c.TrackID {
		case "a":
			want = 10 * h
		case "b":
			want = 4 * h
		case "c":
			want = (10-2*s)*h - h*h
		}
		if math.Abs(c.Errors[0]-want) > 1e-9 || math.Abs(c.GapSecs-h) > 1e-9 {
			t.Errorf("%s start %.1f horizon %.1f: error %v gap %v, want %v", c.TrackID, s, h, c.Errors[0], c.GapSecs, want)
		}
		if math.Abs(c.LongitudinalErrors[0]+want) > 1e-9 {
			t.Errorf("%s: longitudinal error %v, want %v (behind)", c.TrackID, c.LongitudinalErrors[0], -want)
		}
		if c.Decelerating != (c.TrackID == "c") {
			t.Errorf("%s start %.1f: decelerating %v (a %v)", c.TrackID, s, c.Decelerating, c.LongitudinalAccel)
		}
	}
	want := map[[2]float64]map[string][3]float64{ // horizon → split → {n, median, p95}
		{0.5}: {SplitAll: {9, 2.75, 5}, SplitDecelerating: {3, 2.75, 3.75}},
		{1.0}: {SplitAll: {9, 5, 10}, SplitDecelerating: {3, 5, 7}},
		{1.5}: {SplitAll: {6, 6.75, 15}, SplitDecelerating: {2, 6.75, 9.75}},
	}
	if len(rep.Summary) != 6 {
		t.Fatalf("summary rows %d, want 6: %+v", len(rep.Summary), rep.Summary)
	}
	for _, s := range rep.Summary {
		w := want[[2]float64{s.HorizonSecs}][s.Split]
		if s.Model != "hold" || float64(s.N) != w[0] || math.Abs(s.Median-w[1]) > 1e-9 || math.Abs(s.P95-w[2]) > 1e-9 {
			t.Errorf("summary %+v, want n %v median %v p95 %v", s, w[0], w[1], w[2])
		}
	}
}

func TestOcclusionNoWorseThan(t *testing.T) {
	rep := OcclusionReport{Summary: []OcclusionSummary{
		{Model: "cv", HorizonSecs: 0.5, Split: SplitAll, Median: 1, P95: 3},
		{Model: "ca", HorizonSecs: 0.5, Split: SplitAll, Median: 0.9, P95: 3},
		{Model: "cv", HorizonSecs: 1, Split: SplitDecelerating, Median: 2, P95: 4},
		{Model: "ca", HorizonSecs: 1, Split: SplitDecelerating, Median: 2.1, P95: 4.5},
		{Model: "ca", HorizonSecs: 2, Split: SplitAll, Median: 9, P95: 9}, // no baseline row
	}}
	v := rep.NoWorseThan("ca", "cv")
	if len(v) != 2 {
		t.Errorf("violations %v, want median and p95 at 1 s decelerating", v)
	}
	if v := rep.NoWorseThan("cv", "cv"); len(v) != 0 {
		t.Errorf("a model is worse than itself: %v", v)
	}
}

// The four default arms on noise-free braking-to-stop tracks: the coast rule
// keeps CA from reversing across a long gap, and CA with it beats CV where
// the track decelerates.
func TestScoreOcclusionDefaultModelsOnBraking(t *testing.T) {
	var tracks []motion.TrackObservations
	for i, v0 := range []float64{6, 8, 10, 12} {
		const a = 2.0
		stop := v0 / a
		tracks = append(tracks, sampledTrack("brake", int64(i), stop+3, func(t float64) float64 {
			if t >= stop {
				return v0 * stop / 2
			}
			return v0*t - a*t*t/2
		}))
	}
	cfg := DefaultOcclusionConfig(2.5)
	cfg.StrideSecs = 0.5
	models := DefaultOcclusionModels()
	rep, err := ScoreOcclusion(tracks, models, cfg)
	if err != nil {
		t.Fatal(err)
	}
	idx := map[string]int{}
	for i, m := range rep.Models {
		idx[m] = i
	}
	for _, s := range rep.Summary {
		t.Logf("%-12s %.1f s %-12s n %2d median %6.3f p95 %6.3f", s.Model, s.HorizonSecs, s.Split, s.N, s.Median, s.P95)
	}
	if v := rep.NoWorseThan("ca_coast", "cv"); len(v) != 0 {
		for _, line := range v {
			t.Errorf("criterion 5 on noise-free braking: %s", line)
		}
	}
	// Without the rule CA carries the deceleration through the stop and ends
	// behind the vehicle on some long gaps. The rule only ever moves the
	// prediction forward of bare CA (decay weakens a deceleration, and the
	// constraint removes reversal), so where braking persists through the
	// gap it costs accuracy: that trade is what the decay τ is selected on.
	behind := 0
	for _, c := range rep.Cases {
		if !c.Decelerating {
			continue
		}
		bare, rule := c.LongitudinalErrors[idx["ca_no_coast"]], c.LongitudinalErrors[idx["ca_coast"]]
		if c.HorizonSecs == 2.5 && bare < -0.25 {
			behind++
		}
		if rule < bare-1e-9 {
			t.Errorf("gap at %.1f s, %.1f s: coast rule %.3f m behind bare CA's %.3f m",
				float64(c.StartUnixNanos)/1e9, c.HorizonSecs, rule, bare)
		}
	}
	t.Logf("2.5 s decelerating gaps where bare CA ends more than 0.25 m behind the vehicle: %d", behind)
	if behind == 0 {
		t.Error("bare CA never ended behind a stopped vehicle; the scenario no longer exercises reversal")
	}
}
