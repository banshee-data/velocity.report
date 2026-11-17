package egotrack

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"testing"
)

// drive is a synthetic 10 Hz car drive on a straight road heading 30° from
// +x: 5 s stopped, 10 s at +1.5 m/s², 15 s cruising at 15 m/s, 6 s at
// -2.5 m/s² to a standstill, 6 s stopped.
type drive struct {
	heading    float64
	pitchAmp   float64 // rad, at pitchHz
	pitchHz    float64
	zAmp       float64 // m, at zHz
	zHz        float64
	noise      float64 // per-axis position noise, m
	rate       float64
	sigmaStamp float64 // sigma_pos_m written into each sample's quality
}

func (d drive) along(t float64) (s, v, a float64) {
	switch {
	case t < 5:
		return 0, 0, 0
	case t < 15:
		u := t - 5
		return 0.75 * u * u, 1.5 * u, 1.5
	case t < 30:
		return 75 + 15*(t-15), 15, 0
	case t < 36:
		u := t - 30
		return 300 + 15*u - 1.25*u*u, 15 - 2.5*u, -2.5
	default:
		return 345, 0, 0
	}
}

func quat(yaw, pitch, roll float64) [4]float64 {
	cy, sy := math.Cos(yaw/2), math.Sin(yaw/2)
	cp, sp := math.Cos(pitch/2), math.Sin(pitch/2)
	cr, sr := math.Cos(roll/2), math.Sin(roll/2)
	return [4]float64{cr*cp*cy + sr*sp*sy, sr*cp*cy - cr*sp*sy, cr*sp*cy + sr*cp*sy, cr*cp*sy - sr*sp*cy}
}

func (d drive) trajectory(seed int64) *Trajectory {
	rng := rand.New(rand.NewSource(seed))
	tr := &Trajectory{Schema: SchemaV0, Frame: "local", TimeScale: "session_seconds"}
	for i := 0; float64(i)/d.rate <= 42; i++ {
		t := float64(i) / d.rate
		s, _, _ := d.along(t)
		p := [3]float64{s * math.Cos(d.heading), s * math.Sin(d.heading), d.zAmp * math.Sin(2*math.Pi*d.zHz*t)}
		for k := range p {
			p[k] += d.noise * rng.NormFloat64()
		}
		smp := Sample{Frame: i, T: t, P: p, Q: quat(d.heading, d.pitchAmp*math.Sin(2*math.Pi*d.pitchHz*t), 0)}
		if d.sigmaStamp > 0 {
			smp.Quality = &Quality{SigmaPosM: d.sigmaStamp}
		}
		tr.Samples = append(tr.Samples, smp)
	}
	return tr
}

func at(k []Kinematics, t float64) Kinematics {
	best := 0
	for i := range k {
		if math.Abs(k[i].T-t) < math.Abs(k[best].T-t) {
			best = i
		}
	}
	return k[best]
}

func TestDeriveRecoversSpeedAccelerationAndJerk(t *testing.T) {
	d := drive{heading: math.Pi / 6, noise: 0.01, rate: 10, sigmaStamp: 0.01}
	tr := d.trajectory(1)
	if err := tr.Validate(); err != nil {
		t.Fatal(err)
	}
	k := Derive(tr, Options{})
	for _, c := range []struct {
		t, speed, accel float64
	}{{10, 7.5, 1.5}, {22, 15, 0}, {33, 7.5, -2.5}} {
		g := at(k, c.t)
		if !g.Valid || !g.HeadingValid {
			t.Fatalf("t=%.1f: not valid: %s", c.t, g.Reason)
		}
		if math.Abs(g.Speed-c.speed) > 4*g.SpeedSigma+0.01 {
			t.Errorf("t=%.1f: speed %.3f want %.3f ±%.3f", c.t, g.Speed, c.speed, 4*g.SpeedSigma)
		}
		if math.Abs(g.AccelLong-c.accel) > 4*g.AccelSigma+0.05 {
			t.Errorf("t=%.1f: accel %.3f want %.3f ±%.3f", c.t, g.AccelLong, c.accel, 4*g.AccelSigma)
		}
		if math.Abs(g.AccelLat) > 4*g.AccelSigma+0.05 {
			t.Errorf("t=%.1f: lateral accel %.3f on a straight road", c.t, g.AccelLat)
		}
		if math.Abs(g.CourseRad-d.heading) > 0.01 {
			t.Errorf("t=%.1f: course %.4f want %.4f", c.t, g.CourseRad, d.heading)
		}
	}
	// A step in acceleration is a jerk impulse: its integral across the step
	// is the change in acceleration, whatever the smoothing does to its shape.
	for _, c := range []struct{ t, dA float64 }{{15, -1.5}, {30, -2.5}, {36, 2.5}} {
		sum := 0.0
		for i := 1; i < len(k); i++ {
			if math.Abs(k[i].T-c.t) <= 1 && k[i].HeadingValid {
				sum += k[i].JerkLong * (k[i].T - k[i-1].T)
			}
		}
		// At 36 s the car stops, so heading is lost before the step; only
		// check the steps where it is still moving on both sides.
		if c.t == 36 {
			continue
		}
		if math.Abs(sum-c.dA) > 0.25 {
			t.Errorf("jerk integral across t=%.0f s = %.3f, want %.2f", c.t, sum, c.dA)
		}
	}
	if g := at(k, 22); math.Abs(g.JerkLong) > 4*g.JerkSigma+0.1 {
		t.Errorf("cruise jerk %.3f exceeds 4σ=%.3f", g.JerkLong, 4*g.JerkSigma)
	}
}

func TestSigmaMatchesEmpiricalScatter(t *testing.T) {
	// Constant velocity: every deviation of the estimate from truth is noise,
	// so the stated sigma must match the observed scatter.
	const noise = 0.02
	var errs []float64
	var sig float64
	for seed := int64(0); seed < 20; seed++ {
		rng := rand.New(rand.NewSource(seed))
		tr := &Trajectory{Schema: SchemaV0, Frame: "local", TimeScale: "session_seconds"}
		for i := 0; i < 200; i++ {
			tt := float64(i) / 10
			tr.Samples = append(tr.Samples, Sample{Frame: i, T: tt, P: [3]float64{10*tt + noise*rng.NormFloat64(), noise * rng.NormFloat64(), noise * rng.NormFloat64()}, Q: [4]float64{1, 0, 0, 0}})
		}
		k := Derive(tr, Options{DefaultSigmaPosM: noise})
		for i := 20; i < 180; i += 10 {
			errs = append(errs, k[i].Vel[0]-10)
			sig = k[i].SpeedSigma
		}
	}
	ss := 0.0
	for _, e := range errs {
		ss += e * e
	}
	emp := math.Sqrt(ss / float64(len(errs)))
	if r := emp / sig; r < 0.75 || r > 1.3 {
		t.Fatalf("empirical speed scatter %.4f vs stated sigma %.4f (ratio %.2f)", emp, sig, r)
	}
}

func TestStopsAndDisplacement(t *testing.T) {
	d := drive{heading: 0, noise: 0.005, rate: 10}
	tr := d.trajectory(2)
	k := Derive(tr, Options{DefaultSigmaPosM: 0.005})
	stops := Stops(tr, k, Options{})
	if len(stops) != 2 {
		t.Fatalf("got %d stops, want 2: %+v", len(stops), stops)
	}
	if stops[0].DurationS < 4 || stops[1].DurationS < 4.5 {
		t.Errorf("stop durations %.1f, %.1f too short", stops[0].DurationS, stops[1].DurationS)
	}
	for _, s := range stops {
		if s.DisplacementM > 0.05 {
			t.Errorf("standstill displacement %.3f m from 5 mm noise", s.DisplacementM)
		}
	}
}

func TestJitterFindsMountVibration(t *testing.T) {
	d := drive{heading: 0.4, noise: 0.002, rate: 10, pitchAmp: 0.3 * math.Pi / 180, pitchHz: 2, zAmp: 0.004, zHz: 3}
	tr := d.trajectory(3)
	sum, _, err := Analyse(tr, nil, Options{DefaultSigmaPosM: 0.002})
	if err != nil {
		t.Fatal(err)
	}
	j := sum.Jitter
	if j.Segments == 0 {
		t.Fatalf("no spectrum segments: %v", j.Skipped)
	}
	if math.Abs(j.NyquistHz-5) > 1e-9 {
		t.Errorf("nyquist %.3f", j.NyquistHz)
	}
	if math.Abs(j.PeakPitchHz-2) > 0.2 {
		t.Errorf("pitch peak %.3f Hz, want 2", j.PeakPitchHz)
	}
	if math.Abs(j.PeakZHz-3) > 0.2 {
		t.Errorf("z peak %.3f Hz, want 3", j.PeakZHz)
	}
	// The residual is the vibration through |1 - H(f)|, so its RMS is not
	// the sine's 0.21°; the peak amplitude divides that gain out.
	if math.Abs(j.PeakPitchAmpDeg-0.3) > 0.05 {
		t.Errorf("pitch amplitude %.3f°, want 0.3 (residual RMS %.3f°)", j.PeakPitchAmpDeg, j.RMSPitchDeg)
	}
	if math.Abs(j.PeakZAmpM-0.004) > 0.0008 {
		t.Errorf("z amplitude %.4f m, want 0.004", j.PeakZAmpM)
	}
	if g := j.ResidualGain; len(g) != len(j.FreqHz) || g[0] > 1e-6 {
		t.Errorf("residual gain must be zero at DC (the fit follows the path): %v", g)
	}
}

func TestAnchorsAndSummary(t *testing.T) {
	d := drive{heading: 0, noise: 0.005, rate: 10}
	tr := d.trajectory(4)
	yaw := 0.0
	anchors := []Anchor{
		{Name: "site-a", At: "start", P: [3]float64{0, 0, 0}, YawDeg: &yaw, SigmaHM: 0.05},
		{Name: "site-b", At: "end", P: [3]float64{345.5, 0, 0}, SigmaHM: 0.1, HeldOut: true},
	}
	sum, _, err := Analyse(tr, anchors, Options{DefaultSigmaPosM: 0.005})
	if err != nil {
		t.Fatal(err)
	}
	a, b := sum.Anchors[0], sum.Anchors[1]
	if a.HorizErrM > 0.01 || a.Within2Sigma == nil || !*a.Within2Sigma {
		t.Errorf("start anchor: %+v", a)
	}
	if math.Abs(b.HorizErrM-0.5) > 0.01 || *b.Within2Sigma {
		t.Errorf("end anchor: %+v", b)
	}
	if math.Abs(sum.DistanceM-345) > 2 {
		t.Errorf("distance %.2f, want 345", sum.DistanceM)
	}
	if math.Abs(sum.MaxSpeedMS-15) > 0.2 {
		t.Errorf("max speed %.3f", sum.MaxSpeedMS)
	}
	if _, err := json.Marshal(sum); err != nil {
		t.Fatalf("summary does not marshal: %v", err)
	}
	if _, err := CheckAnchors(tr, []Anchor{{Name: "x", At: "middle"}}, Options{}); err == nil {
		t.Error("accepted anchor at middle")
	}
}

func TestGapsSplitFitsAndPoseAtRefusesThem(t *testing.T) {
	d := drive{heading: 0, noise: 0.001, rate: 10}
	tr := d.trajectory(5)
	tr.Samples = append(tr.Samples[:200], tr.Samples[210:]...)
	if n := len(tr.segments(3)); n != 2 {
		t.Fatalf("segments %d, want 2", n)
	}
	k := Derive(tr, Options{})
	if k[199].Valid || k[200].Valid {
		t.Error("samples at the gap edge should be one-sided and invalid")
	}
	if _, _, err := tr.PoseAt(20.45, 0.25); err == nil {
		t.Error("PoseAt interpolated across a 1.1 s gap")
	}
	p, q, err := tr.PoseAt(10.05, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	if want := 0.5 * (tr.Samples[100].P[0] + tr.Samples[101].P[0]); math.Abs(p[0]-want) > 1e-9 {
		t.Errorf("PoseAt x %.6f want %.6f", p[0], want)
	}
	if n := q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3]; math.Abs(n-1) > 1e-9 {
		t.Errorf("interpolated quaternion norm %.9f", n)
	}
	if _, _, err := tr.PoseAt(-1, 1); err == nil {
		t.Error("PoseAt accepted a time before the trajectory")
	}
}

func TestReadValidates(t *testing.T) {
	good := `{"schema":"` + SchemaV0 + `","frame":"local","time_scale":"session_seconds","samples":[` +
		`{"frame":0,"t_s":0,"p_m":[0,0,0],"q_wxyz":[1,0,0,0]},{"frame":1,"t_s":0.1,"p_m":[1,0,0],"q_wxyz":[1,0,0,0.0001]}]}`
	if _, err := Read(strings.NewReader(good)); err != nil {
		t.Fatalf("good trajectory refused: %v", err)
	}
	for name, bad := range map[string]string{
		"schema":  strings.Replace(good, SchemaV0, "other/v9", 1),
		"time":    strings.Replace(good, `"t_s":0.1`, `"t_s":0`, 1),
		"quat":    strings.Replace(good, `[1,0,0,0.0001]`, `[2,0,0,0]`, 1),
		"unknown": strings.Replace(good, `"frame":"local"`, `"frame":"local","extra":1`, 1),
	} {
		if _, err := Read(bytes.NewBufferString(bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestRotationRate runs the same drive at 10 and 20 Hz. A 20 Hz rotation
// carries half the points, so its poses are given √2 the noise; the per-second
// sample count doubles, so speed σ over the same ±0.5 s window should come out
// about the same. A 7 Hz mount vibration is inside the 20 Hz band (Nyquist
// 10 Hz) but aliases to 3 Hz at 10 Hz (Nyquist 5 Hz): the 10 Hz spectrum must
// not be read as measuring it.
func TestRotationRate(t *testing.T) {
	base := drive{heading: 0.3, pitchAmp: 0.2 * math.Pi / 180, pitchHz: 7}
	run := func(rate, noise float64) (Summary, []Kinematics) {
		d := base
		d.rate, d.noise = rate, noise
		sum, k, err := Analyse(d.trajectory(11), nil, Options{DefaultSigmaPosM: noise})
		if err != nil {
			t.Fatal(err)
		}
		return sum, k
	}
	s10, _ := run(10, 0.01)
	s20, _ := run(20, 0.01*math.Sqrt2)
	if r := s20.MedianSpeedSigma / s10.MedianSpeedSigma; r < 0.8 || r > 1.25 {
		t.Errorf("speed sigma 20 Hz / 10 Hz = %.2f, want about 1", r)
	}
	if math.Abs(s20.Jitter.NyquistHz-10) > 1e-6 || math.Abs(s10.Jitter.NyquistHz-5) > 1e-6 {
		t.Errorf("nyquist %v / %v", s20.Jitter.NyquistHz, s10.Jitter.NyquistHz)
	}
	if s20.Jitter.SegmentSamples != 2*s10.Jitter.SegmentSamples {
		t.Errorf("segment samples %d at 20 Hz, %d at 10 Hz: the segment must be fixed in seconds", s20.Jitter.SegmentSamples, s10.Jitter.SegmentSamples)
	}
	if math.Abs(s20.Jitter.PeakPitchHz-7) > 0.2 {
		t.Errorf("20 Hz pitch peak %.2f Hz, want 7", s20.Jitter.PeakPitchHz)
	}
	if math.Abs(s10.Jitter.PeakPitchHz-3) > 0.2 {
		t.Errorf("10 Hz pitch peak %.2f Hz, want the 3 Hz alias of 7 Hz", s10.Jitter.PeakPitchHz)
	}
}

func TestPoseAtRefusesUnusableInput(t *testing.T) {
	tr := &Trajectory{Schema: SchemaV0, Frame: "local", TimeScale: "s", Samples: []Sample{
		{T: 0, P: [3]float64{0, 0, 0}, Q: [4]float64{1, 0, 0, 0}},
		{T: 0.1, P: [3]float64{1, 0, 0}, Q: [4]float64{1, 0, 0, 0}},
		{T: 0.4, P: [3]float64{4, 0, 0}, Q: [4]float64{1, 0, 0, 0}},
	}}
	for name, c := range map[string]struct{ tq, gap float64 }{
		"NaN time":     {math.NaN(), 1},
		"NaN max gap":  {0.05, math.NaN()},
		"negative gap": {0.05, -0.1},
	} {
		if _, _, err := tr.PoseAt(c.tq, c.gap); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, _, err := (&Trajectory{}).PoseAt(0, 1); err == nil {
		t.Error("empty trajectory: accepted")
	}
	// A gap exactly equal to maxGap is allowed; anything wider is refused.
	if p, _, err := tr.PoseAt(0.25, 0.3); err != nil || math.Abs(p[0]-2.5) > 1e-9 {
		t.Errorf("gap equal to maxGap: p=%v err=%v", p, err)
	}
	// 0.4 − 0.1 is 0.30000000000000004 in floating point; the lookup must
	// still treat that gap as equal to 0.3 s, and refuse one 2 µs wider.
	if _, _, err := tr.PoseAt(0.25, 0.3-2e-6); err == nil {
		t.Error("gap 2 µs over maxGap: interpolated")
	}
}

func TestAnalyseRejectsADegreeThatCannotGiveJerk(t *testing.T) {
	tr := drive{heading: 0, noise: 0.001, rate: 10}.trajectory(1)
	for _, d := range []int{1, 2} {
		if _, _, err := Analyse(tr, nil, Options{Degree: d}); err == nil || !strings.Contains(err.Error(), "degree") {
			t.Errorf("degree %d: %v", d, err)
		}
	}
	if _, _, err := Analyse(tr, nil, Options{Degree: 4}); err != nil {
		t.Errorf("degree 4 refused: %v", err)
	}
}

// A car on a circle of radius R at speed v turns at v/R and accelerates
// towards the centre at v²/R; with a left turn, lateral acceleration is
// positive and longitudinal acceleration is zero.
func TestYawRateAndLateralAccelerationOnACircle(t *testing.T) {
	const v, R = 10.0, 50.0
	omega := v / R
	rng := rand.New(rand.NewSource(31))
	tr := &Trajectory{Schema: SchemaV0, Frame: "local", TimeScale: "s"}
	for i := 0; i < 600; i++ {
		tt := float64(i) / 10
		th := omega * tt
		p := [3]float64{R*math.Sin(th) + 0.005*rng.NormFloat64(), R*(1-math.Cos(th)) + 0.005*rng.NormFloat64(), 0}
		tr.Samples = append(tr.Samples, Sample{Frame: i, T: tt, P: p, Q: quat(th+0.0005*rng.NormFloat64(), 0, 0)})
	}
	k := Derive(tr, Options{DefaultSigmaPosM: 0.005, SigmaYawRad: 0.0005})
	for _, i := range []int{100, 300, 500} {
		g := k[i]
		if !g.Valid || !g.HeadingValid {
			t.Fatalf("sample %d invalid: %s", i, g.Reason)
		}
		if math.Abs(g.YawRate-omega) > 4*g.YawRateSigma+1e-4 {
			t.Errorf("sample %d: yaw rate %.5f, want %.5f ±%.5f", i, g.YawRate, omega, 4*g.YawRateSigma)
		}
		// A local cubic over ±0.5 s on a circle biases the second derivative
		// by about (ω h)²/6 of v²/R, under 0.2% here.
		if want := v * v / R; math.Abs(g.AccelLat-want) > 4*g.AccelSigma+0.01*want {
			t.Errorf("sample %d: lateral accel %.4f, want %.4f", i, g.AccelLat, want)
		}
		if math.Abs(g.AccelLong) > 4*g.AccelSigma+0.01 {
			t.Errorf("sample %d: longitudinal accel %.4f on a constant-speed circle", i, g.AccelLong)
		}
		if math.Abs(g.Speed-v) > 4*g.SpeedSigma+0.01 {
			t.Errorf("sample %d: speed %.4f, want %.1f", i, g.Speed, v)
		}
	}
}

// At constant velocity the true acceleration and jerk are zero, so the stated
// σ of each must match the observed scatter of the estimates.
func TestAccelerationAndJerkSigmaMatchScatter(t *testing.T) {
	const noise = 0.01
	var ea, ej []float64
	var sa, sj float64
	for seed := int64(0); seed < 30; seed++ {
		rng := rand.New(rand.NewSource(1000 + seed))
		tr := &Trajectory{Schema: SchemaV0, Frame: "local", TimeScale: "s"}
		for i := 0; i < 200; i++ {
			tt := float64(i) / 10
			tr.Samples = append(tr.Samples, Sample{Frame: i, T: tt, P: [3]float64{10*tt + noise*rng.NormFloat64(), noise * rng.NormFloat64(), noise * rng.NormFloat64()}, Q: [4]float64{1, 0, 0, 0}})
		}
		k := Derive(tr, Options{DefaultSigmaPosM: noise})
		for i := 20; i < 180; i += 11 {
			ea, ej = append(ea, k[i].AccelLong), append(ej, k[i].JerkLong)
			sa, sj = k[i].AccelSigma, k[i].JerkSigma
		}
	}
	rms := func(v []float64) float64 {
		s := 0.0
		for _, x := range v {
			s += x * x
		}
		return math.Sqrt(s / float64(len(v)))
	}
	if r := rms(ea) / sa; r < 0.75 || r > 1.3 {
		t.Errorf("acceleration: empirical %.4f vs stated %.4f (ratio %.2f)", rms(ea), sa, r)
	}
	if r := rms(ej) / sj; r < 0.75 || r > 1.3 {
		t.Errorf("jerk: empirical %.4f vs stated %.4f (ratio %.2f)", rms(ej), sj, r)
	}
}

func TestRollUnwrapsForAnInvertedMount(t *testing.T) {
	tr := &Trajectory{Schema: SchemaV0, Frame: "local", TimeScale: "s"}
	for i := 0; i < 300; i++ {
		tt := float64(i) / 10
		// Roll oscillates about π, so atan2 jumps between +π and −π.
		tr.Samples = append(tr.Samples, Sample{Frame: i, T: tt, P: [3]float64{5 * tt, 0, 0}, Q: quat(0, 0, math.Pi+0.01*math.Sin(2*math.Pi*0.7*tt))})
	}
	sum, _, err := Analyse(tr, nil, Options{DefaultSigmaPosM: 0.001})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Jitter.RMSRollDeg > 1 {
		t.Errorf("roll residual %.2f° RMS: a 0.57° wobble about π reads as wraps", sum.Jitter.RMSRollDeg)
	}
}

// On a regular 10 Hz trajectory every interior sample's ±0.5 s window holds
// the same 11 samples, so its stated σ is identical; floating-point rounding
// of the timestamps must not drop the outermost sample on one side.
func TestFitWindowIsTheSameAtEveryInteriorSample(t *testing.T) {
	tr := &Trajectory{Schema: SchemaV0, Frame: "local", TimeScale: "s"}
	for i := 0; i < 400; i++ {
		tt := float64(i) * 0.1
		tr.Samples = append(tr.Samples, Sample{Frame: i, T: tt, P: [3]float64{12 * tt, 0, 0}, Q: [4]float64{1, 0, 0, 0}})
	}
	k := Derive(tr, Options{DefaultSigmaPosM: 0.01})
	want := k[200].SpeedSigma
	for i := 10; i < 390; i++ {
		if math.Abs(k[i].SpeedSigma-want) > 1e-12 {
			t.Fatalf("sample %d: σ %.12f differs from %.12f; its window changed size", i, k[i].SpeedSigma, want)
		}
	}
}
