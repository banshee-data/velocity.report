package l8analytics

import (
	"encoding/json"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks/motion"
)

// faceFix builds a near-edge fix's record by hand.
func faceFix(prior [4]float32, faces []l5tracks.FaceInnovation, cross float32, nis float32) l5tracks.SolidBodyMeasurement {
	m := l5tracks.SolidBodyMeasurement{
		Source: l5tracks.MeasurementNearEdgeCandidateV1, Rank: len(faces), NIS: nis,
		FaceInnovationCount: len(faces), FaceInnovationCross: cross,
		PriorX: prior[0], PriorY: prior[1], PriorVX: prior[2], PriorVY: prior[3],
	}
	copy(m.FaceInnovations[:], faces)
	return m
}

// The adapter's reading of hand-built fixes, worked by hand. The course is
// +x at 5 m/s. A rank-two fix by a rear face (normal −x) and a left face
// (normal +y) has its rear innovation along the course with the sign flipped,
// its left innovation across it, and the observation is the implied centre
// with R the faces' noise plus their extent variance on their own normals. A
// rank-one fix by the rear face alone is along only; a rank-one fix by the
// left face alone is across only. The dimension is the rank, the NIS the
// fix's, and a repeated reading of the same fix is ignored.
func TestGEST1AdapterReadsFaceFixesByRank(t *testing.T) {
	c := NewGEST1Collector(GEST1CollectorConfig{})
	prior := [4]float32{10, 2, 5, 0}
	rear := l5tracks.FaceInnovation{Face: l5tracks.FaceRear, NormalX: -1, Innovation: 0.3, Variance: 0.09,
		NoiseVariance: 0.05, ExtentVariance: 0.02}
	left := l5tracks.FaceInnovation{Face: l5tracks.FaceLeft, NormalY: 1, Innovation: -0.1, Variance: 0.0625,
		NoiseVariance: 0.05}
	const ms = int64(time.Millisecond)
	two := faceFix(prior, []l5tracks.FaceInnovation{rear, left}, 0.01, 1.4)
	c.ObserveSolidBodyFix("a", 1, true, two, 100*ms, 100*ms)
	c.ObserveSolidBodyFix("a", 1, true, two, 100*ms, 200*ms) // the same fix, read again
	c.ObserveSolidBodyFix("a", 1, true, faceFix(prior, []l5tracks.FaceInnovation{rear}, 0, 1.0), 300*ms, 300*ms)
	c.ObserveSolidBodyFix("a", 1, true, faceFix(prior, []l5tracks.FaceInnovation{left}, 0, 0.16), 400*ms, 400*ms)
	medoid := two
	medoid.Source = l5tracks.MeasurementMedoidV0
	c.ObserveSolidBodyFix("a", 1, true, medoid, 500*ms, 500*ms) // not face-referenced

	chains := c.Chains(GEST1SourceSolidBody)
	if len(chains) != 1 || len(chains[0].Residuals) != 3 || len(chains[0].Observations) != 3 {
		t.Fatalf("chains %+v", chains)
	}
	r := chains[0].Residuals
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-6 }
	// Rank two: y = (−0.3, −0.1) in the site frame; along u = +x is −0.3, across
	// (+y) is −0.1; S in the site frame is [[0.09, −0.01], [−0.01, 0.0625]].
	if r[0].Dim != 2 || !near(r[0].NIS, 1.4) || !r[0].HasAlong || !r[0].HasAcross ||
		!near(r[0].Along, -0.3) || !near(r[0].AlongSD, 0.3) || !near(r[0].Across, -0.1) || !near(r[0].AcrossSD, 0.25) {
		t.Errorf("rank two residual %+v", r[0])
	}
	if r[1].Dim != 1 || !r[1].HasAlong || r[1].HasAcross || !near(r[1].Along, -0.3) || !near(r[1].AlongSD, 0.3) {
		t.Errorf("rank one rear residual %+v", r[1])
	}
	if r[2].Dim != 1 || r[2].HasAlong || !r[2].HasAcross || !near(r[2].Across, -0.1) || !near(r[2].AcrossSD, 0.25) {
		t.Errorf("rank one left residual %+v", r[2])
	}
	o := chains[0].Observations
	if o[0].RankOne || !near(o[0].X, 9.7) || !near(o[0].Y, 1.9) ||
		!near(o[0].R[0][0], 0.07) || !near(o[0].R[1][1], 0.05) || o[0].R[0][1] != 0 || !near(o[0].LiveNIS, 1.4) {
		t.Errorf("rank two observation %+v", o[0])
	}
	if !o[1].RankOne || o[1].NX != -1 || o[1].NY != 0 || !near(-o[1].X, -9.7) || !near(o[1].R[0][0], 0.07) {
		t.Errorf("rank one rear observation %+v", o[1])
	}
	if !o[2].RankOne || o[2].NY != 1 || !near(o[2].Y, 1.9) || !near(o[2].R[1][1], 0.05) {
		t.Errorf("rank one left observation %+v", o[2])
	}

	// A chain whose first fixes are rank one keeps them as residuals and
	// drops them as observations until a two-dimensional one founds it.
	c.ObserveSolidBodyFix("b", 2, true, faceFix(prior, []l5tracks.FaceInnovation{rear}, 0, 1), 100*ms, 100*ms)
	c.ObserveSolidBodyFix("b", 2, true, two, 200*ms, 200*ms)
	b := c.Chains(GEST1SourceSolidBody)[1]
	if b.LeadingRankOne != 1 || len(b.Residuals) != 2 || len(b.Observations) != 1 || b.Observations[0].RankOne {
		t.Errorf("chain b %+v", b)
	}
}

// syntheticTrackerRun replays a synthetic pass through a tracker with the
// shadow solid body under the face-state options, feeding a collector as the
// replay harness does, and returns every fix the bodies published, by track.
func syntheticTrackerRun(t *testing.T) (*GEST1Collector, map[string][]l5tracks.SolidBodyMeasurement) {
	t.Helper()
	cfg := l5tracks.DefaultTrackerConfig()
	cfg.SolidBody = l5tracks.SolidBodyOptions{Enabled: true, OriginSource: "test: synthetic pass in the sensor frame",
		CourseAlignedFaces: true, ExtentState: true, CensoredExtent: true, SpanBoundsBelief: true,
		HeadingFaceNoise: true, AnisotropicReferenceBias: true}
	tracker := l5tracks.NewTracker(cfg)
	c := NewGEST1Collector(GEST1CollectorConfig{MeasurementVariance: float64(cfg.MeasurementNoise)})
	tracker.SetFilterStepObserver(c)
	params := l4perception.DefaultDBSCANParams()
	params.Eps, params.MinPts, params.MaxSamplePoints = 0.6, 5, 512
	params.MeanCentre, params.MaxClusterAspectRatio = false, 15
	base := time.Unix(1_700_000_000, 0).Sub(time.Unix(0, 0))
	pass := l4perception.DefaultSyntheticPass()
	pass.Frames = 60
	fixes := map[string][]l5tracks.SolidBodyMeasurement{}
	last := map[string]int64{}
	for _, f := range l4perception.GenerateSyntheticPass(pass) {
		points := make([]l4perception.WorldPoint, len(f.Points))
		for i, p := range f.Points {
			p.Timestamp = p.Timestamp.Add(base)
			points[i] = p
		}
		at := f.Timestamp.Add(base)
		clusters := l4perception.DBSCAN(points, params)
		tracker.Update(clusters, at)
		tracks := tracker.GetActiveTracks()
		c.ObserveTap(at.UnixNano(), true, clusters, tracker.GetLastAssociations(), tracks)
		for _, tr := range tracks {
			reading, ok := tr.SolidBody()
			if !ok || reading.Measurement.Source != l5tracks.MeasurementNearEdgeCandidateV1 {
				continue
			}
			if at := reading.Estimate.LastObservedUnixNanos; at > last[tr.TrackID] {
				last[tr.TrackID] = at
				fixes[tr.TrackID] = append(fixes[tr.TrackID], reading.Measurement)
			}
		}
	}
	return c, fixes
}

// On a synthetic solid-body run the adapter yields one residual per fix the
// bodies published, with the fix's NIS at its rank, and components that
// reproduce the hand projection Σ_k y_k (n_k·u) and Σ_jk S_jk (n_j·u)(n_k·u)
// of the recorded face innovations. The tracked filter's residuals agree with
// the NIS the filter computed, and every residual has its cluster's points.
func TestGEST1AdapterOnASyntheticSolidBodyRun(t *testing.T) {
	c, fixes := syntheticTrackerRun(t)
	chains := c.Chains(GEST1SourceSolidBody)
	if len(chains) == 0 {
		t.Fatal("the synthetic run produced no face fixes")
	}
	checked := 0
	for _, ch := range chains {
		want := fixes[ch.TrackID]
		if len(ch.Residuals) != len(want) {
			t.Fatalf("track %s: %d residuals, %d fixes published", ch.TrackID, len(ch.Residuals), len(want))
		}
		for i, r := range ch.Residuals {
			m := want[i]
			if r.Dim != m.Rank || r.NIS != float64(m.NIS) || r.Points <= 0 {
				t.Fatalf("track %s fix %d: dim %d NIS %v points %d, want rank %d NIS %v", ch.TrackID, i, r.Dim, r.NIS, r.Points, m.Rank, m.NIS)
			}
			if joint, ok := m.JointFaceNIS(); !ok || math.Abs(joint-r.NIS) > 1e-3*math.Max(1, r.NIS) {
				t.Fatalf("track %s fix %d: joint NIS %v against %v", ch.TrackID, i, joint, r.NIS)
			}
			speed := math.Hypot(float64(m.PriorVX), float64(m.PriorVY))
			if speed < 0.3 || m.Rank != 2 {
				continue
			}
			ux, uy := float64(m.PriorVX)/speed, float64(m.PriorVY)/speed
			var along, alongVar float64
			proj := [2]float64{}
			for k := 0; k < 2; k++ {
				f := m.FaceInnovations[k]
				proj[k] = float64(f.NormalX)*ux + float64(f.NormalY)*uy
				along += float64(f.Innovation) * proj[k]
				alongVar += float64(f.Variance) * proj[k] * proj[k]
			}
			alongVar += 2 * float64(m.FaceInnovationCross) * proj[0] * proj[1]
			if math.Abs(r.Along-along) > 1e-6 || math.Abs(r.AlongSD-math.Sqrt(alongVar)) > 1e-6 {
				t.Fatalf("track %s fix %d: along %v sd %v, by hand %v sd %v", ch.TrackID, i, r.Along, r.AlongSD, along, math.Sqrt(alongVar))
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no moving rank-two fix to check by hand")
	}
	if c.solidRecompute > 1e-3 || c.nisRecompute > 1e-3 {
		t.Errorf("recomputed NIS differs: solid %v tracked %v", c.solidRecompute, c.nisRecompute)
	}
	tracked := c.Chains(GEST1SourceTracked)
	residuals := 0
	for _, ch := range tracked {
		for _, r := range ch.Residuals {
			residuals++
			if r.Dim != 2 || r.Points <= 0 {
				t.Fatalf("tracked residual %+v", r)
			}
		}
	}
	if residuals == 0 {
		t.Fatal("no tracked residuals")
	}
}

// The report reads both sources of the synthetic run: each source counts its
// moving track, criterion 2 runs or says why not, criterion 5 scores the
// default models, and the cross-read joins the two. It serialises.
func TestBuildGEST1ReportOnASyntheticRun(t *testing.T) {
	c, _ := syntheticTrackerRun(t)
	cfg := DefaultGEST1Config(2.5)
	cfg.Occlusion.WarmupSecs = 0.5
	rep, err := BuildGEST1Report(c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Sources) != 2 || rep.Sources[0].Source != GEST1SourceTracked || rep.Sources[1].Source != GEST1SourceSolidBody {
		t.Fatalf("sources %+v", rep.Sources)
	}
	for _, s := range rep.Sources {
		if s.MovingTracks < 1 || s.Residuals == 0 || s.Samples == 0 || s.Samples > s.Residuals {
			t.Errorf("%s: moving %d residuals %d samples %d", s.Source, s.MovingTracks, s.Residuals, s.Samples)
		}
		if s.Criterion2.N != s.Samples || (s.Criterion2.Err == "" && s.Criterion2.R.N != s.Samples) {
			t.Errorf("%s: criterion 2 %+v over %d samples", s.Source, s.Criterion2, s.Samples)
		}
		if s.Criterion3.Tracks == 0 {
			t.Errorf("%s: criterion 3 saw no track", s.Source)
		}
		if s.Criterion5Cases == 0 || len(s.Criterion5) == 0 {
			t.Errorf("%s: criterion 5 scored nothing", s.Source)
		}
		if s.FilterErrors != 0 {
			t.Errorf("%s: %d offline filter errors", s.Source, s.FilterErrors)
		}
	}
	if rep.CrossRead == nil || rep.CrossRead.N == 0 {
		t.Errorf("cross-read %+v", rep.CrossRead)
	}
	if _, err := json.Marshal(rep); err != nil {
		t.Errorf("marshal: %v", err)
	}
}

// trackSamples builds samples for nTracks tracks of perTrack each. NIS and
// acceleration share a per-track effect of size shared plus independent unit
// noise; the covariates are independent noise.
func trackSamples(nTracks, perTrack int, shared float64, seed int64) []gest1Sample {
	rng := rand.New(rand.NewSource(seed))
	var out []gest1Sample
	for k := 0; k < nTracks; k++ {
		effect := shared * rng.NormFloat64()
		key := gest1ChainKey("t", int64(k))
		for i := 0; i < perTrack; i++ {
			out = append(out, gest1Sample{
				res:      GEST1Residual{Dim: 2, NIS: 2 * (5 + effect + rng.NormFloat64()), Points: 20 + rng.Intn(50)},
				accel:    effect + rng.NormFloat64(),
				rng:      10 + 30*rng.Float64(),
				aspect:   rng.Float64() * math.Pi / 2,
				chainKey: key,
			})
		}
	}
	return out
}

// The track bootstrap: where four tracks carry a shared effect, the pooled
// Fisher interval is narrow and the bootstrap over tracks is not; where
// forty tracks are independent noise, the two agree in width. A rerun
// reproduces it.
func TestBootstrapTracksWidensForFewCorrelatedTracks(t *testing.T) {
	cfg := DefaultGEST1Config(2.5)
	few := trackSamples(4, 200, 3, 1)
	c := correlate("few", few, cfg.Level)
	bootstrapTracks(&c, few, cfg)
	fisher, boot := c.R.Hi-c.R.Lo, c.BootHi-c.BootLo
	if c.Tracks != 4 || c.BootDraws != 2000 || !(boot > 5*fisher) {
		t.Errorf("four tracks: r %.3f Fisher [%.3f, %.3f] bootstrap [%.3f, %.3f] (failed %d)",
			c.R.R, c.R.Lo, c.R.Hi, c.BootLo, c.BootHi, c.BootFailed)
	}
	again := correlate("few", few, cfg.Level)
	bootstrapTracks(&again, few, cfg)
	if again.BootLo != c.BootLo || again.BootHi != c.BootHi {
		t.Error("the seeded bootstrap did not reproduce")
	}
	many := trackSamples(40, 20, 0, 2)
	m := correlate("many", many, cfg.Level)
	bootstrapTracks(&m, many, cfg)
	ratio := (m.BootHi - m.BootLo) / (m.R.Hi - m.R.Lo)
	if ratio < 0.6 || ratio > 1.6 || m.R.R < m.BootLo || m.R.R > m.BootHi {
		t.Errorf("forty independent tracks: Fisher [%.3f, %.3f], bootstrap [%.3f, %.3f], width ratio %.2f",
			m.R.Lo, m.R.Hi, m.BootLo, m.BootHi, ratio)
	}
}

// Ranks run from one and ties share their mean rank.
func TestAverageRanks(t *testing.T) {
	got := averageRanks([]float64{3, 1, 4, 1, 5, 9, 2, 6})
	want := []float64{4, 1.5, 5, 1.5, 6, 8, 3, 7}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranks %v, want %v", got, want)
		}
	}
}

// The smoothed acceleration is centred: on observations of a constant
// 1.5 m/s² deceleration, at the shipped measurement noise and noise-free, it
// reads −1.5 in a chain's interior, nothing where the window does not fit,
// and an attenuated value of the right sign within about 1.5 s of an end,
// where the CV smoother's velocity is the filter's and lags. The attenuation
// is the estimator's, so it is pinned here and named in the report.
func TestSmoothedAccelerationOnAConstantDeceleration(t *testing.T) {
	ch := GEST1Chain{TrackID: "a", CreationSequence: 1, Confirmed: true}
	for k := 0; k <= 60; k++ {
		tt := float64(k) / 10
		ch.Observations = append(ch.Observations, motion.Observation{
			UnixNanos: int64(k) * 100_000_000, X: 12*tt - 0.75*tt*tt, R: motion.IsotropicR(motion.DefaultMeasurementVariance),
		})
	}
	k := smoothedKinematics(ch, DefaultGEST1Config(2.5))
	if k.err != nil || !k.moving {
		t.Fatalf("kinematics: err %v moving %v", k.err, k.moving)
	}
	for i, a := range k.accelLong {
		inWindow := i >= 5 && i <= 55
		if inWindow != !math.IsNaN(a) {
			t.Errorf("observation %d: acceleration %v, window fits %v", i, a, inWindow)
			continue
		}
		switch {
		case !inWindow:
		case i >= 20 && i <= 40:
			if math.Abs(a+1.5) > 0.05 {
				t.Errorf("observation %d: interior acceleration %.3f, want −1.5", i, a)
			}
		default:
			if !(a < -0.4 && a > -1.55) {
				t.Errorf("observation %d: end acceleration %.3f, want attenuated toward zero and negative", i, a)
			}
		}
	}
}
