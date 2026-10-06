package l8analytics

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"testing"
)

const scFrame = int64(100_000_000) // 100 ms

// scScene builds scorecard inputs frame by frame.
type scScene struct {
	estimates []ScorecardEstimate
	clusters  []ScorecardCluster
	nextObs   int
}

// cluster adds a cluster at a frame index and returns its observation id.
func (s *scScene) cluster(frame int, x, y float64, points int) string {
	s.nextObs++
	id := fmt.Sprintf("obs/%06d", s.nextObs)
	s.clusters = append(s.clusters, ScorecardCluster{ObservationID: id, FrameUnixNanos: int64(frame) * scFrame, X: x, Y: y, PointsCount: points})
	return id
}

// observe adds a cluster and the estimate of the track that accepted it. The
// posterior sits on the measurement unless an innovation is given.
func (s *scScene) observe(track int64, frame int, x, y, vx, vy float64) {
	s.estimates = append(s.estimates, ScorecardEstimate{
		CreationSequence: track, FrameUnixNanos: int64(frame) * scFrame, ObservationID: s.cluster(frame, x, y, 50),
		X: x, Y: y, VX: vx, VY: vy, MeasurementX: x, MeasurementY: y,
	})
}

// pad keeps the frame timeline running to `last`, far from every track, so
// the frame period is defined and tracks that ended early are not censored.
func (s *scScene) pad(last int) {
	for f := 0; f <= last; f++ {
		s.cluster(f, 500, 500, 5)
	}
}

func classCount(t *testing.T, sc TrackScorecard, class string) int {
	t.Helper()
	for _, c := range sc.Termination.ByClass {
		if c.Class == class {
			return c.Count
		}
	}
	t.Fatalf("class %q missing from the scorecard", class)
	return 0
}

func TestScorecardPopulationExcludesWarmupBornTracksAndCountsLifetimes(t *testing.T) {
	var s scScene
	s.pad(100)
	for f := 0; f <= 30; f++ { // born in warm-up: not in the population
		s.observe(1, f, 10, float64(f)*0.1, 0, 1)
	}
	for f := 20; f <= 24; f++ { // 0.4 s
		s.observe(2, f, 20, float64(f)*0.1, 0, 1)
	}
	for f := 30; f <= 60; f += 2 { // 3.0 s, associated on every other frame
		s.observe(3, f, 30, float64(f)*0.1, 0, 1)
	}
	s.observe(4, 40, 40, 0, 0, 0) // a single estimate

	sc := ComputeTrackScorecard(s.estimates, s.clusters, ScorecardOptions{ScoringStartNanos: 10 * scFrame})
	p := sc.Population
	if sc.FramePeriodNanos != scFrame {
		t.Fatalf("frame period %d, want %d", sc.FramePeriodNanos, scFrame)
	}
	if p.Tracks != 3 {
		t.Fatalf("population has %d tracks, want 3 (the warm-up-born track is excluded)", p.Tracks)
	}
	if p.SingleEstimateTracks != 1 {
		t.Errorf("single-estimate tracks = %d, want 1", p.SingleEstimateTracks)
	}
	if math.Abs(p.LifetimeMedianSecs-0.4) > 1e-9 {
		t.Errorf("median lifetime %.3f s, want 0.4 (lifetimes 0, 0.4, 3.0)", p.LifetimeMedianSecs)
	}
	if math.Abs(p.ShareUnderOneSecond-2.0/3.0) > 1e-9 {
		t.Errorf("share under one second %.3f, want 2/3", p.ShareUnderOneSecond)
	}
	if p.LifetimeHistogram[0] != 1 || p.LifetimeHistogram[4] != 1 || p.LifetimeHistogram[30] != 1 {
		t.Errorf("lifetime histogram bins 0, 4 and 30 = %d, %d, %d; want 1 each",
			p.LifetimeHistogram[0], p.LifetimeHistogram[4], p.LifetimeHistogram[30])
	}
	// Spanned frames: 1 + 5 + 31 = 37; estimates 1 + 5 + 16 = 22.
	if math.Abs(p.AssociationDensity-22.0/37.0) > 1e-9 {
		t.Errorf("association density %.4f, want %.4f", p.AssociationDensity, 22.0/37.0)
	}
	// Track 1 has 21 estimates at or after the scoring start.
	if p.Estimates != 21+22 {
		t.Errorf("scored estimates = %d, want %d", p.Estimates, 21+22)
	}
}

func TestScorecardClassifiesHowTracksEnd(t *testing.T) {
	var s scScene
	s.pad(100)
	// Bystander: alive throughout, passes the point where "contested" ends.
	for f := 0; f <= 80; f++ {
		s.observe(10, f, 0, 0.3, 0, 0)
	}
	// contested: ends at frame 20 at the origin; the next frame's cluster at
	// its predicted position belongs to the pre-existing bystander (above).
	for f := 10; f <= 20; f++ {
		s.observe(11, f, 0, 0, 0, 0)
	}
	// replaced: ends at frame 20; a track born afterwards takes over its path.
	for f := 10; f <= 20; f++ {
		s.observe(12, f, 50, float64(f), 0, 10)
	}
	for f := 22; f <= 40; f++ {
		s.observe(13, f, 50, float64(f), 0, 10)
	}
	// unassigned_nearby: ends at frame 20; a cluster nobody accepted sits on
	// its prediction in the next frame.
	for f := 10; f <= 20; f++ {
		s.observe(14, f, 100, 0, 0, 0)
	}
	s.cluster(21, 100.2, 0, 8)
	// vanished: ends at frame 20 with nothing within 2 m afterwards.
	for f := 10; f <= 20; f++ {
		s.observe(15, f, 150, 0, 0, 0)
	}
	// other: the nearest later cluster is 1.5 m away.
	for f := 10; f <= 20; f++ {
		s.observe(16, f, 200, 0, 0, 0)
	}
	s.cluster(21, 201.5, 0, 8)
	// censored: still associated at the last frame.
	for f := 90; f <= 100; f++ {
		s.observe(17, f, 250, 0, 0, 0)
	}

	sc := ComputeTrackScorecard(s.estimates, s.clusters, ScorecardOptions{})
	// Vanished is 3: the purpose-built track, plus the bystander (ends at 80)
	// and the replacement (ends at 40), neither of which has anything near it
	// afterwards.
	for class, want := range map[string]int{"contested": 1, "replaced": 1, "unassigned_nearby": 1, "vanished": 3, "other": 1} {
		if got := classCount(t, sc, class); got != want {
			t.Errorf("%s = %d, want %d", class, got, want)
		}
	}
	if sc.Termination.Ended != 7 {
		t.Errorf("ended = %d, want 7 (the censored track must not be classified)", sc.Termination.Ended)
	}
	if sc.Population.CensoredAtEnd != 1 {
		t.Errorf("censored at end = %d, want 1", sc.Population.CensoredAtEnd)
	}
}

func coastCell(sc TrackScorecard, horizon, speedFloor float64, decel string) (ScorecardCoastCell, bool) {
	for _, c := range sc.Coast {
		if c.HorizonSeconds == horizon && c.SpeedFloorMps == speedFloor && c.Deceleration == decel {
			return c, true
		}
	}
	return ScorecardCoastCell{}, false
}

func TestScorecardShadowCoastIsExactForConstantVelocityAndOvershootsBraking(t *testing.T) {
	var s scScene
	s.pad(120)
	// Constant 10 m/s along +x: every coast prediction lands on the measurement.
	for f := 0; f <= 60; f++ {
		s.observe(1, f, float64(f), 0, 10, 0)
	}
	// Braking at 2 m/s^2 from 12 m/s along +y, state always exact.
	for f := 0; f <= 50; f++ {
		tt := float64(f) * 0.1
		s.observe(2, f, 300, 12*tt-tt*tt, 0, 12-2*tt)
	}
	sc := ComputeTrackScorecard(s.estimates, s.clusters, ScorecardOptions{})

	steady, ok := coastCell(sc, 1.0, 10, "steady")
	if !ok {
		t.Fatal("no steady 10 m/s cell at a 1.0 s horizon")
	}
	if steady.RMSMetres > 1e-6 || steady.P95Metres > scorecardCoastBinMetres {
		t.Errorf("constant-velocity coast error rms=%.6f p95=%.3f, want zero", steady.RMSMetres, steady.P95Metres)
	}

	// A CV coast from a braking state overshoots by a*h^2/2 = 1.0 m at h = 1 s.
	var brakingCount int
	var brakingBias float64
	for _, c := range sc.Coast {
		if c.HorizonSeconds == 1.0 && c.Deceleration == "decelerating" {
			brakingCount += c.Count
			brakingBias += c.AlongBiasMetres * float64(c.Count)
		}
	}
	if brakingCount == 0 {
		t.Fatal("no decelerating cells at a 1.0 s horizon")
	}
	if bias := brakingBias / float64(brakingCount); math.Abs(bias-1.0) > 1e-6 {
		t.Errorf("braking along-track bias %.4f m at 1.0 s, want +1.0 (overshoot of a*h^2/2)", bias)
	}
}

func TestScorecardResidualsDecomposeInTheSensorFrame(t *testing.T) {
	var s scScene
	s.pad(60)
	add := func(track int64, frame int, mx, my, ix, iy float64, points int, nis float64) {
		s.estimates = append(s.estimates, ScorecardEstimate{
			CreationSequence: track, FrameUnixNanos: int64(frame) * scFrame, ObservationID: s.cluster(frame, mx, my, points),
			X: mx, Y: my, MeasurementX: mx, MeasurementY: my, InnovationX: ix, InnovationY: iy, NIS: nis,
		})
	}
	// Line of sight along +x at 15 m: an x innovation is radial.
	for f := 0; f < 10; f++ {
		add(1, f, 15, 0, 0.3, 0, 40, 1)
	}
	// Line of sight along +y at 35 m: an x innovation is tangential.
	for f := 0; f < 10; f++ {
		add(2, f, 0, 35, 0.4, 0, 5, 9)
	}
	sc := ComputeTrackScorecard(s.estimates, s.clusters, ScorecardOptions{})
	var near, far *ScorecardResidCell
	for i := range sc.Residuals {
		c := &sc.Residuals[i]
		if c.RangeFloorMetres == 10 && c.PointsFloor == 30 {
			near = c
		}
		if c.RangeFloorMetres == 30 && c.PointsFloor == 0 {
			far = c
		}
	}
	if near == nil || far == nil {
		t.Fatalf("expected strata missing: %+v", sc.Residuals)
	}
	if math.Abs(near.RadialRMS-0.3) > 1e-9 || near.TangentialRMS > 1e-9 || near.NISExceedance != 0 {
		t.Errorf("15 m cell radial=%.3f tangential=%.3f exceedance=%.2f, want 0.3, 0, 0", near.RadialRMS, near.TangentialRMS, near.NISExceedance)
	}
	if math.Abs(far.TangentialRMS-0.4) > 1e-9 || far.RadialRMS > 1e-9 || far.NISExceedance != 1 {
		t.Errorf("35 m cell radial=%.3f tangential=%.3f exceedance=%.2f, want 0, 0.4, 1", far.RadialRMS, far.TangentialRMS, far.NISExceedance)
	}
	if math.Abs(far.TangentialBias+0.4) > 1e-9 {
		t.Errorf("35 m cell tangential bias %.3f, want -0.4 (tangential is +90 degrees from radial)", far.TangentialBias)
	}
}

// The requirement this whole package is built around: the same evidence in
// any input order gives byte-identical JSON.
func TestScorecardIsIndependentOfInputOrder(t *testing.T) {
	var s scScene
	s.pad(80)
	for track := int64(1); track <= 12; track++ {
		start := int(track) * 3
		for f := start; f <= start+int(track)*2; f++ {
			s.observe(track, f, float64(track)*7+0.013*float64(f), 0.37*float64(f), 1.1, 3.7)
		}
	}
	encode := func(e []ScorecardEstimate, c []ScorecardCluster) string {
		b, err := json.Marshal(ComputeTrackScorecard(e, c, ScorecardOptions{ScoringStartNanos: 5 * scFrame}))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	want := encode(s.estimates, s.clusters)
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 5; trial++ {
		e := append([]ScorecardEstimate(nil), s.estimates...)
		c := append([]ScorecardCluster(nil), s.clusters...)
		rng.Shuffle(len(e), func(i, j int) { e[i], e[j] = e[j], e[i] })
		rng.Shuffle(len(c), func(i, j int) { c[i], c[j] = c[j], c[i] })
		if got := encode(e, c); got != want {
			t.Fatalf("trial %d: scorecard changed with input order", trial)
		}
	}
}

func TestScorecardHandlesEmptyEvidence(t *testing.T) {
	sc := ComputeTrackScorecard(nil, nil, ScorecardOptions{})
	if sc.Population.Tracks != 0 || sc.Termination.Ended != 0 {
		t.Fatalf("empty evidence produced %+v", sc)
	}
	// Canonical output: empty sections are empty arrays, never null, so an
	// empty scorecard has one encoding.
	if sc.Coast == nil || sc.Residuals == nil || sc.Population.LifetimeHistogram == nil ||
		len(sc.Termination.ByClass) != len(ScorecardTerminationClasses) {
		t.Fatalf("empty evidence left nil sections: %+v", sc)
	}
}
