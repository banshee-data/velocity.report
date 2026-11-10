package l8analytics

import (
	"math"
	"math/rand"
	"path/filepath"
	"testing"
)

// Jerk worked by hand. Speeds at 10 Hz of 10, 10, 10.2, 10.2, 10 m/s give
// accelerations 0, 2, 0, −2 m/s² and jerks 20, −20, −20 m/s³: three samples,
// all beyond 15, and one sign change over the 0.2 s between consecutive jerk
// samples. A 0.5 s interval, or a speed under the minimum, breaks the
// sequence, so the samples after either start again.
func TestJerkCountHandComputed(t *testing.T) {
	const ms = int64(1e6)
	times := []int64{0, 100 * ms, 200 * ms, 300 * ms, 400 * ms}
	vx := []float64{10, 10, 10.2, 10.2, 10}
	vy := make([]float64, len(vx))
	got := jerkCount("cv", times, vx, vy, 15, 0.15, 0.5)
	if got.N != 3 || got.Over != 3 || got.SignChanges != 1 || math.Abs(got.Secs-0.2) > 1e-12 || got.Estimator != "cv" {
		t.Fatalf("jerk %+v", got)
	}
	// Halve the swing: jerks of 10 are inside the bound.
	half := []float64{10, 10, 10.1, 10.1, 10}
	if got := jerkCount("cv", times, half, vy, 15, 0.15, 0.5); got.N != 3 || got.Over != 0 {
		t.Fatalf("half swing %+v", got)
	}
	// A gap after the third observation leaves one jerk before it and none
	// after (two intervals are needed).
	gap := []int64{0, 100 * ms, 200 * ms, 700 * ms, 800 * ms}
	if got := jerkCount("cv", gap, vx, vy, 15, 0.15, 0.5); got.N != 1 || got.SignChanges != 0 || got.Secs != 0 {
		t.Fatalf("gap %+v", got)
	}
	slow := []float64{10, 10, 0.1, 10.2, 10}
	if got := jerkCount("cv", times, slow, vy, 15, 0.15, 0.5); got.N != 0 {
		t.Fatalf("slow %+v", got)
	}
	nan := []float64{10, 10, math.NaN(), 10.2, 10, 10}
	if got := jerkCount("live", append(times, 500*ms), nan, append(vy, 0), 15, 0.15, 0.5); got.N != 1 {
		t.Fatalf("NaN velocity %+v", got)
	}
}

// The moment-based partial correlation the bootstrap uses equals the
// Gram–Schmidt one on the same data.
func TestPartialFromMomentsMatchesPartialCorrelation(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	n := 500
	y, x := make([]float64, n), make([]float64, n)
	cov := [][]float64{make([]float64, n), make([]float64, n), make([]float64, n)}
	var m pcMoments
	for i := 0; i < n; i++ {
		c0, c1, c2 := 10+30*rng.Float64(), 1.5*rng.Float64(), float64(20+rng.Intn(2000))
		a := rng.NormFloat64() + 0.01*c0
		y[i] = 0.5*a + 0.002*c2 + rng.NormFloat64()
		x[i] = a
		cov[0][i], cov[1][i], cov[2][i] = c0, c1, c2
		m.add([pcVars]float64{y[i] - 1, x[i], c0 - 25, c1, c2 - 1000})
	}
	want, err := PartialCorrelation(y, x, cov, 0.95)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := partialFromMoments(&m)
	if !ok || math.Abs(got-want.R) > 1e-9 {
		t.Fatalf("moments %v (%v), Gram–Schmidt %v", got, ok, want.R)
	}
	var constant pcMoments
	for i := 0; i < 20; i++ {
		constant.add([pcVars]float64{float64(i), float64(i % 3), 1, 2, 3})
	}
	if _, ok := partialFromMoments(&constant); ok {
		t.Fatal("a constant covariate must make the covariance singular")
	}
}

// corpusSample is a complete one-step sample with z along.
func corpusSample(z, age float64) GEST1CorpusSample {
	return GEST1CorpusSample{OneStep: true, Dim: 2, HasAlong: true, ZAlong: z, HasAcross: true, ZAcross: z / 2,
		HasState: true, Speed: 8, Range: 25, HasAspect: true, Aspect: math.Pi / 4, HasAccel: true, Accel: 0.5,
		Points: 120, AgeSecs: age}
}

// The restated read over two hand-built tracks, worked by hand. Track a has
// one-step along z of 2 and 0, track b 0, 0, 1 and −3, and b also has a
// multi-step 5. One-step: RMS √(14/6), two of six beyond 1.96. All: RMS
// √(39/7), three of seven. Across is half of along. The strata put every
// value in one speed, range, aspect and point bin, and split age at 1 s; the
// jerk, turning, run and offline counts add up.
func TestReadGEST1CorpusHandBuilt(t *testing.T) {
	a := GEST1CorpusTrack{TrackID: "a", Samples: []GEST1CorpusSample{corpusSample(2, 0.5), corpusSample(0, 1.5)},
		RunTracked: true, RunFlagged: true,
		OfflineAlong: [][]float64{{1, 2}, {1, 1}, {0.5, 1}}, OfflineAcross: [][]float64{{1}, {1}, {2}},
		Jerk:       []GEST1JerkCount{{Estimator: "live", N: 10, Over: 1, SignChanges: 2, Secs: 1}},
		TurnFrames: 10, TurnOver: []int{4, 1},
		Occlusion: []GEST1OcclusionRecord{{HorizonSecs: 0.5, Errors: []float64{1, 2, 3, 4}}},
	}
	multi := corpusSample(5, 3)
	multi.OneStep = false
	b := GEST1CorpusTrack{TrackID: "b", Samples: []GEST1CorpusSample{corpusSample(0, 0.2), corpusSample(0, 1.2),
		corpusSample(1, 2), corpusSample(-3, 3), multi}, RunTracked: true,
		OfflineAlong: [][]float64{{4}, {2}, {2}}, OfflineAcross: [][]float64{{1}, {1}, {1}},
		Jerk:       []GEST1JerkCount{{Estimator: "live", N: 30, Over: 3, SignChanges: 1, Secs: 2}},
		TurnFrames: 30, TurnOver: []int{6, 1},
		Occlusion: []GEST1OcclusionRecord{{HorizonSecs: 0.5, Decelerating: true, Errors: []float64{3, 1, 1, 1}}},
	}
	cfg := DefaultGEST1CorpusConfig()
	cfg.BootstrapDraws = 200
	r := ReadGEST1Corpus([]GEST1CorpusTrack{a, b}, []float64{0.1, 0.2}, cfg)
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-12 }
	if r.Tracks != 2 || r.Samples != 7 {
		t.Fatalf("tracks %d samples %d", r.Tracks, r.Samples)
	}
	if r.Along.N != 6 || !near(r.Along.RMS, math.Sqrt(14.0/6)) || !near(r.Along.Beyond, 2.0/6) || !r.Along.OverDispersed {
		t.Errorf("along %+v", r.Along)
	}
	if r.AlongAll.N != 7 || !near(r.AlongAll.RMS, math.Sqrt(39.0/7)) || !near(r.AlongAll.Beyond, 3.0/7) {
		t.Errorf("along, all %+v", r.AlongAll)
	}
	if r.Across.N != 6 || !near(r.Across.RMS, math.Sqrt(14.0/6)/2) || r.Across.Beyond != 0 || r.Across.OverDispersed {
		t.Errorf("across %+v", r.Across)
	}
	if !(r.Along.RMSLo <= r.Along.RMS && r.Along.RMS <= r.Along.RMSHi) || r.Along.RMSLo == r.Along.RMSHi {
		t.Errorf("along bootstrap [%v, %v] around %v", r.Along.RMSLo, r.Along.RMSHi, r.Along.RMS)
	}
	// Per track: a's RMS √2 (two values, under MinTrackSamples 5, so out);
	// b's one-step RMS √(10/4) is the only one in the distribution.
	if r.Along.TrackRMSTracks != 0 {
		t.Errorf("per-track distribution over %d tracks", r.Along.TrackRMSTracks)
	}
	wantStrata := map[string][2]int{ // n, beyond count
		"speed_mps/5-10": {6, 2}, "range_m/20-40": {6, 2}, "aspect_deg/30-60": {6, 2}, "points/50-199": {6, 2},
		"age_s/under 1": {2, 1}, "age_s/1+": {4, 1},
	}
	if len(r.Strata) != len(wantStrata) {
		t.Fatalf("strata %+v", r.Strata)
	}
	for _, s := range r.Strata {
		w, ok := wantStrata[s.Factor+"/"+s.Bin]
		if !ok || s.N != w[0] || s.BeyondCount != w[1] {
			t.Errorf("stratum %+v, want %v", s, w)
		}
	}
	if r.YoungTracks != 2 || !near(r.YoungTailShare, 0.5) {
		t.Errorf("young tracks %d, tail share %v", r.YoungTracks, r.YoungTailShare)
	}
	if len(r.Jerk) != 1 || r.Jerk[0].N != 40 || !near(r.Jerk[0].ShareOver, 0.1) || !near(r.Jerk[0].SignChangesPerSec, 1) || r.Jerk[0].Tracks != 2 {
		t.Errorf("jerk %+v", r.Jerk)
	}
	if r.Turning.Frames != 40 || !near(r.Turning.Shares[0], 0.25) || !near(r.Turning.Shares[1], 0.05) {
		t.Errorf("turning %+v", r.Turning)
	}
	if r.Criterion3.Tracks != 2 || r.Criterion3.Flagged != 1 || r.Criterion3.Share != 0.5 {
		t.Errorf("criterion 3 %+v", r.Criterion3)
	}
	c4 := r.Criterion4
	if c4.NAlong != 3 || c4.AlongP99[0] != 4 || c4.AlongP99[1] != 2 || c4.AlongP99[2] != 2 || c4.AlongRatio[1] != 0.5 || c4.AcrossRatio[2] != 2 {
		t.Errorf("criterion 4 %+v", c4)
	}
	if r.Criterion5N != 2 || len(r.Criterion5) == 0 {
		t.Errorf("criterion 5 %d cases %+v", r.Criterion5N, r.Criterion5)
	}
	// Constant covariates: the point estimate drops them, as
	// PartialCorrelation does, and every bootstrap draw, whose moments are
	// then singular, is counted as failed rather than read.
	if r.Correlation.N != 6 || r.Correlation.R.DroppedCovariates != 3 || r.Correlation.BootFailed != cfg.BootstrapDraws {
		t.Errorf("correlation %+v", r.Correlation)
	}
}

// correlatedTracks builds tracks whose z_along² rises with acceleration by
// effect (zero: independent), with varying covariates.
func correlatedTracks(nTracks, perTrack int, effect float64, seed int64) []GEST1CorpusTrack {
	rng := rand.New(rand.NewSource(seed))
	out := make([]GEST1CorpusTrack, nTracks)
	for i := range out {
		for j := 0; j < perTrack; j++ {
			s := corpusSample(0, float64(j)/10)
			s.Accel = math.Abs(rng.NormFloat64())
			s.Range, s.Aspect, s.Points = 5+40*rng.Float64(), math.Pi/2*rng.Float64(), 10+rng.Intn(800)
			s.ZAlong = rng.NormFloat64() * math.Sqrt(1+effect*s.Accel)
			out[i].Samples = append(out[i].Samples, s)
		}
	}
	return out
}

// The bootstrap's 5th percentile sits above 0.1 when z² clearly rises with
// acceleration and near or below zero when it does not; the read reports the
// interval's ends around the point estimate.
func TestZCorrelationBootstrap(t *testing.T) {
	cfg := DefaultGEST1CorpusConfig()
	cfg.BootstrapDraws = 300
	strong := ReadGEST1Corpus(correlatedTracks(60, 40, 4, 1), nil, cfg).Correlation
	if strong.Err != "" || !(strong.R.R > 0.2) || !(strong.BootP05 > 0.1) || !(strong.BootLo <= strong.R.R && strong.R.R <= strong.BootHi) {
		t.Fatalf("strong %+v", strong)
	}
	null := ReadGEST1Corpus(correlatedTracks(60, 40, 0, 2), nil, cfg).Correlation
	if null.Err != "" || !(null.BootP05 < 0.05) || null.Tracks != 60 || null.N != 2400 {
		t.Fatalf("null %+v", null)
	}
}

// A record written and loaded is the same, gzipped or not.
func TestGEST1CorpusRecordRoundTrip(t *testing.T) {
	rec := GEST1CorpusRecord{Case: "site", Config: DefaultGEST1Config(2.5),
		Sources: []GEST1CorpusSource{{Source: GEST1SourceTracked, Tracks: correlatedTracks(2, 3, 1, 4)}}}
	for _, name := range []string{"r.json", "r.json.gz"} {
		path := filepath.Join(t.TempDir(), name)
		if err := WriteGEST1CorpusRecord(path, rec); err != nil {
			t.Fatal(err)
		}
		got, err := LoadGEST1CorpusRecord(path)
		if err != nil {
			t.Fatal(err)
		}
		tracks := got.Source(GEST1SourceTracked)
		if got.Case != "site" || len(tracks) != 2 || tracks[1].Samples[2] != rec.Sources[0].Tracks[1].Samples[2] ||
			got.Config.JerkThreshold != 15 || got.Source(GEST1SourceSolidBody) != nil {
			t.Fatalf("%s: %+v", name, got)
		}
	}
}

// The yaw rate of a constant turn is its rate.
func TestYawRateOnAConstantTurn(t *testing.T) {
	const omega, speed = 0.3, 10.0
	var k chainKinematics
	for i := 0; i < 40; i++ {
		tt := float64(i) / 10
		k.times = append(k.times, int64(i)*1e8)
		k.vx = append(k.vx, speed*math.Cos(omega*tt))
		k.vy = append(k.vy, speed*math.Sin(omega*tt))
	}
	cfg := DefaultGEST1Config(2.5)
	if got := k.yawRate(20, cfg); math.Abs(got-omega) > 1e-9 {
		t.Fatalf("yaw rate %v, want %v", got, omega)
	}
	if got := k.yawRate(2, cfg); !math.IsNaN(got) {
		t.Fatalf("yaw rate without a window %v", got)
	}
	for i := range k.vx {
		k.vx[i], k.vy[i] = k.vx[i]/10, k.vy[i]/10
	}
	if got := k.yawRate(20, cfg); !math.IsNaN(got) {
		t.Fatalf("yaw rate at 1 m/s %v", got)
	}
}

// On the synthetic solid-body run every moving track of each source has a
// record behind the restated read, with the live estimator's jerk and every
// offline model's, and the live velocities are read for both sources.
func TestBuildGEST1ReportRecordsTracks(t *testing.T) {
	c, _ := syntheticTrackerRun(t)
	cfg := DefaultGEST1Config(2.5)
	cfg.Occlusion.WarmupSecs = 0.5
	rep, err := BuildGEST1Report(c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range rep.Sources {
		if s.Restated == nil || len(s.CorpusTracks) != s.MovingTracks || s.Restated.Tracks != s.MovingTracks {
			t.Fatalf("%s: %d records for %d moving tracks", s.Source, len(s.CorpusTracks), s.MovingTracks)
		}
		samples := 0
		for _, tr := range s.CorpusTracks {
			samples += len(tr.Samples)
			if len(tr.Jerk) != 1+len(GEST1OfflineModels) || tr.Jerk[0].Estimator != GEST1EstimatorLive ||
				len(tr.OfflineAlong) != len(GEST1OfflineModels) || len(tr.TurnOver) != 2 {
				t.Fatalf("%s track %s: jerk %+v offline %d turn %v", s.Source, tr.TrackID, tr.Jerk, len(tr.OfflineAlong), tr.TurnOver)
			}
			if tr.Jerk[0].N == 0 {
				t.Errorf("%s track %s: the live estimator gave no jerk sample", s.Source, tr.TrackID)
			}
		}
		if samples != s.Residuals || s.Restated.Along.N != s.PostHoc.AlongOneStep.N ||
			math.Abs(s.Restated.Along.RMS-s.PostHoc.AlongOneStep.RMS) > 1e-12 {
			t.Errorf("%s: %d samples for %d residuals; restated along %+v, post-hoc %+v",
				s.Source, samples, s.Residuals, s.Restated.Along, s.PostHoc.AlongOneStep)
		}
	}
	for _, src := range []string{GEST1SourceTracked, GEST1SourceSolidBody} {
		for _, ch := range c.Chains(src) {
			if len(ch.LiveVX) != len(ch.Observations) {
				t.Fatalf("%s %s: %d live velocities for %d observations", src, ch.TrackID, len(ch.LiveVX), len(ch.Observations))
			}
			for i, v := range ch.LiveVX {
				if math.IsNaN(v) || math.IsNaN(ch.LiveVY[i]) {
					t.Fatalf("%s %s: live velocity %d not read", src, ch.TrackID, i)
				}
			}
		}
	}
}
