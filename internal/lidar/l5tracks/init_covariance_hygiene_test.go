package l5tracks

import (
	"hash/fnv"
	"math"
	"runtime"
	"sort"
	"testing"
	"time"
)

// Tests for the three default-off options of data/maths/tracking-maths.md
// §2.3: TwoPointVelocityInit, ConsistentCovarianceCap and
// ConsistentSpeedClamp. Each must leave the shipped arithmetic exactly as it
// was when off, and have the property claimed for it when on.

// hygieneScenario runs a fixed two-vehicle scene through the tracker and
// returns a digest of every live track's state and covariance bits after
// every frame. The scene is built to reach each site the hygiene options
// touch: both tracks take a second association, the fast one's velocity
// passes MaxReasonableSpeedMps, and the slow one coasts 22 frames so the
// coast inflation and the prediction both meet the covariance cap.
//
//   - A: from (5, 2) at 6.5 m/s along +X, unobserved on frames 30 to 51;
//   - B: from (-40, -10) along +X at 10 m/s, accelerating at 5 m/s² from
//     0.5 s, so it passes 30 m/s at 4.5 s and reaches 37 m/s; observed
//     throughout. It starts slowly because the shipped tracker cannot hold a
//     faster vehicle from birth (see
//     TestTwoPointVelocityInitHoldsAFastVehicleFromBirth).
//
// Frames are 0.1 s apart from a fixed capture time, and tracks are visited in
// creation order, so the digest depends only on the arithmetic.
func hygieneScenario(t *testing.T, cfg TrackerConfig) (digest uint64, created int) {
	t.Helper()
	tr := NewTracker(cfg)
	t0 := time.Unix(1_700_000_000, 0)
	h := fnv.New64a()
	put := func(v float32) {
		b := math.Float32bits(v)
		_, _ = h.Write([]byte{byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24)})
	}
	const frames = 60
	for i := 0; i < frames; i++ {
		now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
		var clusters []WorldCluster
		if i < 30 || i > 51 {
			clusters = append(clusters, hygieneCluster(5+6.5*0.1*float32(i), 2, now))
		}
		clusters = append(clusters, hygieneCluster(acceleratingX(0.1*float32(i)), -10, now))
		tr.Update(clusters, now)

		tracks := make([]*TrackedObject, 0, len(tr.Tracks))
		for _, track := range tr.Tracks {
			tracks = append(tracks, track)
		}
		sort.Slice(tracks, func(a, b int) bool { return tracks[a].CreationSequence < tracks[b].CreationSequence })
		for _, track := range tracks {
			put(float32(track.CreationSequence))
			put(track.X)
			put(track.Y)
			put(track.VX)
			put(track.VY)
			for _, v := range track.P {
				put(v)
			}
			put(float32(track.Hits))
			put(float32(track.Misses))
		}
	}
	return h.Sum64(), tr.TracksCreated
}

// acceleratingX is vehicle B's position at ts seconds.
func acceleratingX(ts float32) float32 {
	x := -40 + 10*ts
	if ts > 0.5 {
		x += 0.5 * 5 * (ts - 0.5) * (ts - 0.5)
	}
	return x
}

func hygieneCluster(x, y float32, now time.Time) WorldCluster {
	return WorldCluster{
		CentroidX: x, CentroidY: y, CentroidZ: 1, SensorID: "test",
		BoundingBoxLength: 4, BoundingBoxWidth: 2, BoundingBoxHeight: 1.5,
		PointsCount: 100, TSUnixNanos: now.UnixNano(),
	}
}

// hygieneScenarioConfig is the shipped tracker with the coupled process noise
// on, so the covariance carries the position-velocity cross terms the cap can
// leave inconsistent, and a cap small enough for A's coast to reach but above
// the two-point velocity variance 2R/Δt² = 10 m²/s², which the initialisation
// refuses to exceed.
func hygieneScenarioConfig() TrackerConfig {
	cfg := DefaultTrackerConfig()
	cfg.CoupledProcessNoise = true
	cfg.MaxCovarianceDiag = 11
	return cfg
}

// The scene's digests as the tracker computed them before the options
// existed (origin/main at the commit that added them). They pin the shipped
// arithmetic to the bit. Go may fuse a multiply and add into one instruction
// on arm64 and some other architectures, which changes float results there,
// so the digests are amd64's; elsewhere the test asserts only what does not
// depend on fusion. A deliberate change to the shipped tracker moves them:
// rerun with -v and replace them from the logged values in that change.
const (
	hygieneGoldenScenario = 0x05d4b67616bdf689 // hygieneScenarioConfig
	hygieneGoldenDefault  = 0xf98d0d7fdddb5981 // DefaultTrackerConfig
)

func TestHygieneOptionsOffKeepTheShippedArithmetic(t *testing.T) {
	cfg := DefaultTrackerConfig()
	if cfg.TwoPointVelocityInit || cfg.ConsistentCovarianceCap || cfg.ConsistentSpeedClamp {
		t.Fatal("the hygiene options must be opt-in")
	}
	scenario, created := hygieneScenario(t, hygieneScenarioConfig())
	if created != 2 {
		t.Fatalf("the scene made %d tracks, want 2: it no longer reaches the sites it is built for", created)
	}
	shipped, _ := hygieneScenario(t, DefaultTrackerConfig())
	t.Logf("digests: scenario=%#x default=%#x", scenario, shipped)
	if runtime.GOARCH == "amd64" {
		if scenario != hygieneGoldenScenario {
			t.Errorf("scenario digest %#x, want the shipped %#x", scenario, uint64(hygieneGoldenScenario))
		}
		if shipped != hygieneGoldenDefault {
			t.Errorf("default digest %#x, want the shipped %#x", shipped, uint64(hygieneGoldenDefault))
		}
	}

	// Each option, on alone, reaches the scene: the digest moves.
	for name, set := range map[string]func(*TrackerConfig){
		"two_point_velocity_init":   func(c *TrackerConfig) { c.TwoPointVelocityInit = true },
		"consistent_covariance_cap": func(c *TrackerConfig) { c.ConsistentCovarianceCap = true },
		"consistent_speed_clamp":    func(c *TrackerConfig) { c.ConsistentSpeedClamp = true },
	} {
		on := hygieneScenarioConfig()
		set(&on)
		got, n := hygieneScenario(t, on)
		if got == scenario {
			t.Errorf("%s did not change the scene: the option did not reach its site", name)
		}
		// Not asserted: B exceeds the speed limit, which no option is meant
		// to track faithfully, so how often it is re-born is not a property.
		t.Logf("%s: %d tracks created", name, n)
	}
}

// --- Two-point velocity initialisation -------------------------------------

func twoPointTracker(mutate func(*TrackerConfig)) *Tracker {
	cfg := DefaultTrackerConfig()
	cfg.TwoPointVelocityInit = true
	if mutate != nil {
		mutate(&cfg)
	}
	return NewTracker(cfg)
}

func TestTwoPointVelocityInitIsExactOnANoiseFreeTrack(t *testing.T) {
	const vx, vy = 6.5, -1.2
	for _, on := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.TwoPointVelocityInit = on
		tr := NewTracker(cfg)
		t0 := time.Unix(1_700_000_000, 0)
		var track *TrackedObject
		for i := 0; i < 3; i++ {
			now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
			tr.Update([]WorldCluster{hygieneCluster(5+vx*0.1*float32(i), 2+vy*0.1*float32(i), now)}, now)
			if len(tr.Tracks) != 1 {
				t.Fatalf("frame %d: %d tracks, want 1", i, len(tr.Tracks))
			}
			for _, tk := range tr.Tracks {
				track = tk
			}
			if i != 1 {
				continue
			}
			speedErr := math.Hypot(float64(track.VX)-vx, float64(track.VY)-vy)
			if !on {
				// The defect the option fixes: the shipped second update
				// leaves the velocity near its zero prior.
				if speedErr < 0.5*math.Hypot(vx, vy) {
					t.Errorf("shipped: velocity error %.3f m/s after two frames; the zero-velocity prior no longer dominates", speedErr)
				}
				continue
			}
			if speedErr > 1e-4 {
				t.Errorf("velocity after the second frame = (%v, %v), want (%v, %v)", track.VX, track.VY, vx, vy)
			}
			wantX, wantY := float32(5+vx*0.1), float32(2+vy*0.1)
			if track.X != wantX || track.Y != wantY {
				t.Errorf("position (%v, %v), want the second measurement (%v, %v)", track.X, track.Y, wantX, wantY)
			}
			const r, dt = 0.05, 0.1
			for _, c := range []struct {
				name string
				i, j int
				want float64
			}{
				{"x,x", 0, 0, r}, {"y,y", 1, 1, r},
				{"x,vx", 0, 2, r / dt}, {"y,vy", 1, 3, r / dt},
				{"vx,vx", 2, 2, 2 * r / (dt * dt)}, {"vy,vy", 3, 3, 2 * r / (dt * dt)},
				{"x,y", 0, 1, 0}, {"x,vy", 0, 3, 0}, {"y,vx", 1, 2, 0}, {"vx,vy", 2, 3, 0},
			} {
				for _, idx := range [][2]int{{c.i, c.j}, {c.j, c.i}} {
					if got := float64(track.P[idx[0]*4+idx[1]]); math.Abs(got-c.want) > 1e-5*math.Max(1, c.want) {
						t.Errorf("P[%s] = %v, want %v", c.name, got, c.want)
					}
				}
			}
			res := track.LastResidual
			if !res.Valid || res.Disposition != ResidualTwoPointInit || res.Reason == "" {
				t.Errorf("residual %+v, want a valid %s record", res, ResidualTwoPointInit)
			}
			if res.InnovationX != wantX-5 || res.InnovationY != wantY-2 {
				t.Errorf("innovation (%v, %v), want the measurement against the zero-velocity prediction", res.InnovationX, res.InnovationY)
			}
			var n int
			for _, b := range track.Residuals {
				n += b.Count
			}
			if n != 0 {
				t.Errorf("the initialisation entered %d consistency samples; it applied no gain", n)
			}
			if track.ObservationCount != 2 || track.Hits != 2 || len(track.History) != 2 {
				t.Errorf("bookkeeping: observations=%d hits=%d history=%d, want 2, 2, 2",
					track.ObservationCount, track.Hits, len(track.History))
			}
		}
		if on {
			// The third frame is an ordinary update again.
			if track.LastResidual.Disposition != "" {
				t.Errorf("third frame disposition %q, want an ordinary update", track.LastResidual.Disposition)
			}
			if e := math.Hypot(float64(track.VX)-vx, float64(track.VY)-vy); e > 1e-3 {
				t.Errorf("third frame velocity error %.5f m/s on a noise-free track", e)
			}
		}
	}
}

// The defect at its sharpest. A noise-free vehicle born at 20 m/s: after the
// shipped second update its velocity is still near zero, so the third frame
// lands about 2 m ahead of a prediction whose position variance is about a
// tenth of a square metre, outside the gate, and the vehicle is re-born every
// few frames. Two-point initialisation predicts the third frame exactly.
func TestTwoPointVelocityInitHoldsAFastVehicleFromBirth(t *testing.T) {
	created := map[bool]int{}
	for _, on := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.TwoPointVelocityInit = on
		tr := NewTracker(cfg)
		t0 := time.Unix(1_700_000_000, 0)
		for i := 0; i < 30; i++ {
			now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
			tr.Update([]WorldCluster{hygieneCluster(-40+2*float32(i), -10, now)}, now)
		}
		created[on] = tr.TracksCreated
		if on && len(tr.GetConfirmedTracks()) != 1 {
			t.Errorf("two-point: %d confirmed tracks, want the one vehicle", len(tr.GetConfirmedTracks()))
		}
	}
	t.Logf("tracks created for one 20 m/s vehicle over 3 s: shipped %d, two-point %d", created[false], created[true])
	if created[true] != 1 {
		t.Errorf("two-point: %d tracks for one vehicle, want 1", created[true])
	}
	if created[false] <= 1 {
		t.Errorf("shipped: %d track; the cold-start fragmentation this test documents has gone", created[false])
	}
}

// Every refusal leaves the shipped update to run, exactly.
func TestTwoPointVelocityInitFallsBackToTheShippedUpdate(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0).UnixNano()
	cases := []struct {
		name         string
		firstTS      int64
		secondTS     int64
		dx           float32
		maxSpeed     float32
		maxDiag      float32
		nearEdgeOwns bool
	}{
		{name: "zero interval", firstTS: t0, secondTS: t0, dx: 0.65},
		{name: "negative interval", firstTS: t0, secondTS: t0 - 1e8, dx: 0.65},
		{name: "implied speed above the limit", firstTS: t0, secondTS: t0 + 1e8, dx: 0.65, maxSpeed: 5},
		// 2R/Δt² = 10 m²/s² at 0.1 s, above a cap of 5; and 2.5 ms apart,
		// 16 000 m²/s², above the shipped 100.
		{name: "velocity variance above the cap", firstTS: t0, secondTS: t0 + 1e8, dx: 0.65, maxDiag: 5},
		{name: "measurements too close in time", firstTS: t0, secondTS: t0 + 2.5e6, dx: 0.01},
		{name: "near-edge tracked state", firstTS: t0, secondTS: t0 + 1e8, dx: 0.65, nearEdgeOwns: true},
	}
	for _, c := range cases {
		run := func(on bool) *TrackedObject {
			tr := twoPointTracker(func(cfg *TrackerConfig) {
				cfg.TwoPointVelocityInit = on
				if c.maxSpeed > 0 {
					cfg.MaxReasonableSpeedMps = c.maxSpeed
				}
				if c.maxDiag > 0 {
					cfg.MaxCovarianceDiag = c.maxDiag
				}
			})
			first := hygieneCluster(5, 2, time.Unix(0, c.firstTS))
			track := tr.initTrack(first, t0)
			tr.predict(track, 0.1)
			second := hygieneCluster(5+c.dx, 2, time.Unix(0, c.secondTS))
			if c.nearEdgeOwns {
				// Only the gate is under test: initialiseTwoPoint must refuse
				// a track whose state the near-edge machine owns.
				track.solidBody.tracked = true
				if tr.initialiseTwoPoint(track, second, tr.measurementForCluster(second, t0+1e8), nil, [4]float32{1, 0, 0, 1}) {
					t.Errorf("%s: the initialisation ran on a near-edge tracked state", c.name)
				}
				return nil
			}
			tr.update(track, second, t0+1e8)
			return track
		}
		off, on := run(false), run(true)
		if c.nearEdgeOwns {
			continue
		}
		if on.LastResidual.Disposition != "" {
			t.Errorf("%s: disposition %q, want the shipped update", c.name, on.LastResidual.Disposition)
		}
		if on.X != off.X || on.Y != off.Y || on.VX != off.VX || on.VY != off.VY || on.P != off.P {
			t.Errorf("%s: the fallback differs from the shipped update:\n on %+v %v\noff %+v %v",
				c.name, [4]float32{on.X, on.Y, on.VX, on.VY}, on.P, [4]float32{off.X, off.Y, off.VX, off.VY}, off.P)
		}
	}
}

// A track that has updated once already is never re-initialised, even with
// a zero velocity.
func TestTwoPointVelocityInitAppliesOnlyToTheSecondMeasurement(t *testing.T) {
	tr := twoPointTracker(nil)
	t0 := time.Unix(1_700_000_000, 0)
	track := tr.initTrack(hygieneCluster(5, 2, t0), t0.UnixNano())
	track.ObservationCount = 2
	second := hygieneCluster(5.65, 2, t0.Add(100*time.Millisecond))
	if tr.initialiseTwoPoint(track, second, tr.measurementForCluster(second, 0), nil, [4]float32{1, 0, 0, 1}) {
		t.Error("initialised a track with two measurements behind it")
	}
	track.ObservationCount = 1
	track.VX = 0.1
	if tr.initialiseTwoPoint(track, second, tr.measurementForCluster(second, 0), nil, [4]float32{1, 0, 0, 1}) {
		t.Error("initialised a track that already has velocity information")
	}
}

// Under the adaptive measurement model the blocks scale that measurement's R,
// its x-y term included.
func TestTwoPointVelocityInitUsesTheAdaptiveR(t *testing.T) {
	tr := twoPointTracker(nil)
	t0 := time.Unix(1_700_000_000, 0)
	track := tr.initTrack(hygieneCluster(5, 2, t0), t0.UnixNano())
	r := MeasurementCovariance{XX: 0.09, XY: 0.02, YY: 0.04}
	const dt = 0.2
	second := hygieneCluster(6, 2.5, t0.Add(200*time.Millisecond))
	if !tr.initialiseTwoPoint(track, second, tr.measurementForCluster(second, 0), &r, [4]float32{1, 0, 0, 1}) {
		t.Fatal("declined a valid pair")
	}
	want := [16]float64{
		0.09, 0.02, 0.09 / dt, 0.02 / dt,
		0.02, 0.04, 0.02 / dt, 0.04 / dt,
		0.09 / dt, 0.02 / dt, 2 * 0.09 / (dt * dt), 2 * 0.02 / (dt * dt),
		0.02 / dt, 0.04 / dt, 2 * 0.02 / (dt * dt), 2 * 0.04 / (dt * dt),
	}
	for i := range want {
		if math.Abs(float64(track.P[i])-want[i]) > 1e-5*math.Max(1, want[i]) {
			t.Errorf("P[%d] = %v, want %v", i, track.P[i], want[i])
		}
	}
	if _, ok := cholesky4(symmetric64(track.P)); !ok {
		t.Errorf("the two-point covariance is not positive definite: %v", track.P)
	}
}

// --- Covariance cap ---------------------------------------------------------

// worstCorrelation is the largest |P_ij| / sqrt(P_ii P_jj) over i ≠ j, the
// 2×2 principal minors' test: above one, P is indefinite.
func worstCorrelation(p [16]float32) float64 {
	worst := 0.0
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			den := math.Sqrt(float64(p[i*4+i]) * float64(p[j*4+j]))
			if den == 0 {
				if p[i*4+j] != 0 {
					return math.Inf(1)
				}
				continue
			}
			worst = math.Max(worst, math.Abs(float64(p[i*4+j]))/den)
		}
	}
	return worst
}

// A confirmed track coasting under the coupled process noise with a small
// cap: the shipped clamp drives a position-velocity correlation above one,
// which documents the defect, and the consistent cap never does.
func TestCovarianceCapOnALongCoast(t *testing.T) {
	for _, consistent := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.CoupledProcessNoise = true
		cfg.MaxCovarianceDiag = 2
		cfg.MaxMissesConfirmed = 200
		cfg.ConsistentCovarianceCap = consistent
		tr := NewTracker(cfg)
		t0 := time.Unix(1_700_000_000, 0)
		var now time.Time
		for i := 0; i < 8; i++ {
			now = t0.Add(time.Duration(i) * 100 * time.Millisecond)
			tr.Update([]WorldCluster{hygieneCluster(5+0.65*float32(i), 2, now)}, now)
		}
		var track *TrackedObject
		for _, tk := range tr.Tracks {
			track = tk
		}
		if track == nil || track.TrackState != TrackConfirmed {
			t.Fatalf("consistent=%v: no confirmed track to coast", consistent)
		}
		worst := 0.0
		for i := 0; i < 80; i++ {
			now = now.Add(100 * time.Millisecond)
			tr.Update(nil, now)
			if track.TrackState == TrackDeleted {
				t.Fatalf("consistent=%v: the track ended on coast frame %d", consistent, i)
			}
			for d := 0; d < 4; d++ {
				if track.P[d*4+d] > cfg.MaxCovarianceDiag {
					t.Fatalf("consistent=%v: P[%d,%d] = %v above the cap", consistent, d, d, track.P[d*4+d])
				}
			}
			worst = math.Max(worst, worstCorrelation(track.P))
			if consistent {
				if c := worstCorrelation(track.P); c > 1+1e-5 {
					t.Fatalf("coast frame %d: correlation %v above one: %v", i, c, track.P)
				}
				if _, ok := cholesky4(symmetric64(track.P)); !ok {
					t.Fatalf("coast frame %d: P is not positive definite: %v", i, track.P)
				}
			}
		}
		t.Logf("consistent=%v: worst correlation over the coast %.3f", consistent, worst)
		if !consistent && worst <= 1 {
			t.Errorf("the shipped cap kept every correlation within one (worst %.3f); the scene no longer shows the defect", worst)
		}
	}
}

func TestCapCovarianceConsistentKeepsCorrelations(t *testing.T) {
	p := [16]float32{
		9, 1, 2.5, 0,
		1, 4, 0, 1.5,
		2.5, 0, 1, 0.1,
		0, 1.5, 0.1, 0.81,
	}
	before := p
	capCovarianceConsistent(&p, 2)
	if p[0] != 2 || p[5] != 2 {
		t.Errorf("capped diagonals %v, %v, want exactly 2", p[0], p[5])
	}
	if p[10] != before[10] || p[15] != before[15] || p[11] != before[11] {
		t.Errorf("an uncapped block changed: %v", p)
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rb := float64(before[i*4+j]) / math.Sqrt(float64(before[i*4+i])*float64(before[j*4+j]))
			ra := float64(p[i*4+j]) / math.Sqrt(float64(p[i*4+i])*float64(p[j*4+j]))
			if math.Abs(ra-rb) > 1e-6 {
				t.Errorf("correlation (%d,%d) %v, want %v kept", i, j, ra, rb)
			}
		}
	}
	if a := covarianceAsymmetry(p); a != 0 {
		t.Errorf("asymmetry %v", a)
	}

	under := before
	capCovarianceConsistent(&under, 100)
	if under != before {
		t.Error("a covariance under the cap was changed")
	}
	zero := before
	capCovarianceConsistent(&zero, 0)
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			if zero[i*4+j] != 0 {
				t.Fatalf("a zero cap left P[%d,%d] = %v", i, j, zero[i*4+j])
			}
		}
	}
}

// The solid body's own cap sites take the same form under the option.
func TestConsistentCovarianceCapReachesTheSolidBody(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.CoupledProcessNoise = true
	cfg.MaxCovarianceDiag = 2
	cfg.ConsistentCovarianceCap = true
	tr := NewTracker(cfg)
	track := &TrackedObject{TrackID: "trk_sb"}
	sb := &track.solidBody
	sb.seeded = true
	sb.state = [4]float32{0, 0, 5, 0}
	sb.p = [16]float32{
		1.5, 0, 0.6, 0,
		0, 1.5, 0, 0.6,
		0.6, 0, 0.5, 0,
		0, 0.6, 0, 0.5,
	}
	for i := 0; i < 40; i++ {
		tr.predictSolidBodyStep(track, 0.1)
		if !sb.seeded {
			t.Fatal("the solid body was dropped")
		}
		if c := worstCorrelation(sb.p); c > 1+1e-5 {
			t.Fatalf("step %d: solid-body correlation %v above one: %v", i, c, sb.p)
		}
	}
	if sb.p[0] != 2 {
		t.Errorf("solid-body position variance %v, want the cap 2 reached", sb.p[0])
	}
}

// --- Speed clamp ------------------------------------------------------------

// A covariance with the position-velocity coupling a converged track has.
func speedClampCovariance() [16]float32 {
	return [16]float32{
		0.04, 0.002, 0.08, 0.01,
		0.002, 0.05, 0.01, 0.09,
		0.08, 0.01, 0.5, 0.05,
		0.01, 0.09, 0.05, 0.4,
	}
}

// numericUpperTruncatedMoments integrates N(mu, sigma²) on (−∞, upper] by
// Simpson's rule over twelve standard deviations below the bound, an
// independent check on the closed form.
func numericUpperTruncatedMoments(mu, sigma, upper float64) (mean, variance float64) {
	lo := math.Min(upper, mu) - 12*sigma
	const n = 20000
	h := (upper - lo) / n
	var m0, m1, m2 float64
	for i := 0; i <= n; i++ {
		x := lo + float64(i)*h
		w := 2.0
		switch {
		case i == 0 || i == n:
			w = 1
		case i%2 == 1:
			w = 4
		}
		f := w * math.Exp(-0.5*((x-mu)/sigma)*((x-mu)/sigma))
		m0 += f
		m1 += f * x
		m2 += f * x * x
	}
	mean = m1 / m0
	return mean, m2/m0 - mean*mean
}

func TestUpperTruncatedNormalMoments(t *testing.T) {
	for _, c := range []struct{ mu, sigma, upper float64 }{
		{32, 0.7, 30}, {30.1, 2, 30}, {28, 1, 30}, {35, 0.5, 30}, {30, 1, 30},
	} {
		mean, variance := upperTruncatedNormalMoments(c.mu, c.sigma, c.upper)
		wantMean, wantVar := numericUpperTruncatedMoments(c.mu, c.sigma, c.upper)
		if math.Abs(mean-wantMean) > 1e-6*c.sigma || math.Abs(variance-wantVar) > 1e-5*c.sigma*c.sigma {
			t.Errorf("%+v: moments (%v, %v), want (%v, %v)", c, mean, variance, wantMean, wantVar)
		}
		if mean > c.upper || !(variance > 0) || variance >= c.sigma*c.sigma {
			t.Errorf("%+v: mean %v above the bound or variance %v outside (0, σ²)", c, mean, variance)
		}
	}
	// Far past the bound the continued fraction takes over; the two forms
	// agree across the switch, and the moments approach the bound with
	// variance σ²/z².
	const sigma = 0.1
	below, _ := upperTruncatedNormalMoments(30+(millsContinuedFractionFrom-1e-9)*sigma, sigma, 30)
	above, _ := upperTruncatedNormalMoments(30+(millsContinuedFractionFrom+1e-9)*sigma, sigma, 30)
	if math.Abs(below-above) > 1e-9 {
		t.Errorf("mean jumps across the continued-fraction switch: %v, %v", below, above)
	}
	for _, z := range []float64{40, 1e3, 1e6} {
		mean, variance := upperTruncatedNormalMoments(30+z*sigma, sigma, 30)
		if math.IsNaN(mean) || math.IsNaN(variance) || mean > 30 || variance < 0 {
			t.Fatalf("z=%v: moments (%v, %v)", z, mean, variance)
		}
		if want := 30 - sigma/z; math.Abs(mean-want) > 3*sigma/(z*z*z)+1e-9 {
			t.Errorf("z=%v: mean %v, want about %v", z, mean, want)
		}
		if z <= 1e3 {
			if want := sigma * sigma / (z * z); math.Abs(variance-want) > 0.2*want {
				t.Errorf("z=%v: variance %v, want about %v", z, variance, want)
			}
		}
	}
}

func TestConstrainSpeedConsistentIsATruncation(t *testing.T) {
	const vmax = 30
	x := [4]float32{10, 4, 32, 9}
	p := speedClampCovariance()
	x0, p0 := x, p
	constrainSpeedConsistent(&x, &p, vmax)

	speed := math.Hypot(float64(x[2]), float64(x[3]))
	if speed > vmax+1e-4 {
		t.Errorf("speed %v above the limit", speed)
	}
	// The along-track marginal is replaced by its truncation to (−∞, vmax],
	// and every component moves by its regression on it.
	s0 := math.Hypot(float64(x0[2]), float64(x0[3]))
	ux, uy := float64(x0[2])/s0, float64(x0[3])/s0
	var ph [4]float64
	for i := 0; i < 4; i++ {
		ph[i] = float64(p0[i*4+2])*ux + float64(p0[i*4+3])*uy
	}
	sigma2 := ph[2]*ux + ph[3]*uy
	mean, variance := numericUpperTruncatedMoments(s0, math.Sqrt(sigma2), vmax)
	for i := 0; i < 2; i++ {
		want := float64(x0[i]) + ph[i]/sigma2*(mean-s0)
		if math.Abs(float64(x[i])-want) > 1e-4 {
			t.Errorf("x[%d] = %v, want %v from the regression", i, x[i], want)
		}
	}
	if x[0] == x0[0] {
		t.Error("position did not move with the clamped speed despite their covariance")
	}
	along := ux*ux*float64(p[10]) + 2*ux*uy*float64(p[11]) + uy*uy*float64(p[15])
	if math.Abs(along-variance) > 1e-5 {
		t.Errorf("along-track velocity variance %v, want the truncated %v", along, variance)
	}
	if !(along > 0) || along >= sigma2 {
		t.Errorf("along-track velocity variance %v, want within (0, %v)", along, sigma2)
	}
	t.Logf("along-track speed %.3f ± %.3f m/s truncated at %v: %.3f ± %.3f m/s",
		s0, math.Sqrt(sigma2), vmax, mean, math.Sqrt(variance))
	if a := covarianceAsymmetry(p); a != 0 {
		t.Errorf("asymmetry %v", a)
	}
	if _, ok := cholesky4(symmetric64(p)); !ok {
		t.Errorf("truncated P is not positive definite: %v", p)
	}
	for d := 0; d < 4; d++ {
		if p[d*4+d] < 0 || p[d*4+d] > p0[d*4+d] {
			t.Errorf("P[%d,%d] = %v, want within [0, %v]", d, d, p[d*4+d], p0[d*4+d])
		}
	}
}

// With no velocity variance along the direction of travel there is nothing to
// regress on, and the clamp is the shipped rescale.
func TestConstrainSpeedConsistentFallsBackWithoutVelocityVariance(t *testing.T) {
	x := [4]float32{10, 4, 32, 9}
	var p [16]float32
	p[0], p[5] = 0.1, 0.1
	p0 := p
	constrainSpeedConsistent(&x, &p, 30)
	shipped := [4]float32{10, 4, 32, 9}
	rescaleVelocity(&shipped, 30)
	if x != shipped || p != p0 {
		t.Errorf("fallback (%v, %v), want the shipped rescale %v with P untouched", x, p, shipped)
	}
}

// Through the tracker: the shipped clamp leaves P as it was, so the next
// update's velocity gain is the unclamped one and pulls the velocity back
// toward the measurement. The consistent clamp leaves P positive
// semi-definite, and because the truncation keeps some along-track velocity
// variance, the next gain along the course is smaller but still positive: a
// projection onto the limit would have left it at zero.
func TestConsistentSpeedClampBoundsTheNextVelocityGain(t *testing.T) {
	gains := map[bool]float64{}
	for _, consistent := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.ConsistentSpeedClamp = consistent
		tr := NewTracker(cfg)
		track := &TrackedObject{X: 10, Y: 4, VX: 32, VY: 9, P: speedClampCovariance()}
		before := track.P
		tr.clampVelocity(track)
		if s := math.Hypot(float64(track.VX), float64(track.VY)); s > 30+1e-4 {
			t.Fatalf("consistent=%v: speed %v after the clamp", consistent, s)
		}
		if !consistent && track.P != before {
			t.Fatal("the shipped clamp changed P")
		}
		if consistent {
			if a := covarianceAsymmetry(track.P); a != 0 {
				t.Errorf("asymmetry %v after the clamp", a)
			}
		}
		u := [2]float64{float64(track.VX), float64(track.VY)}
		n := math.Hypot(u[0], u[1])
		u[0], u[1] = u[0]/n, u[1]/n

		tr.predict(track, 0.1)
		jittered := symmetric64(track.P)
		for d := 0; d < 4; d++ {
			jittered[d*4+d] += 1e-9
		}
		if _, ok := cholesky4(jittered); !ok {
			t.Errorf("consistent=%v: predicted P is not positive semi-definite: %v", consistent, track.P)
		}
		// The along-course velocity gain for an along-course innovation:
		// u·K_v·u with K = P Hᵀ S⁻¹.
		r := float64(cfg.MeasurementNoise)
		s00, s01, s11 := float64(track.P[0])+r, float64(track.P[1]), float64(track.P[5])+r
		det := s00*s11 - s01*s01
		i00, i01, i11 := s11/det, -s01/det, s00/det
		var kv [2][2]float64
		for a := 0; a < 2; a++ {
			pa0, pa1 := float64(track.P[(2+a)*4+0]), float64(track.P[(2+a)*4+1])
			kv[a][0] = pa0*i00 + pa1*i01
			kv[a][1] = pa0*i01 + pa1*i11
		}
		g := u[0]*(kv[0][0]*u[0]+kv[0][1]*u[1]) + u[1]*(kv[1][0]*u[0]+kv[1][1]*u[1])
		if math.IsNaN(g) || math.IsInf(g, 0) {
			t.Fatalf("consistent=%v: along-course velocity gain %v", consistent, g)
		}
		gains[consistent] = g
	}
	t.Logf("along-course velocity gain after the clamp: shipped %.3f s⁻¹, consistent %.3f s⁻¹", gains[false], gains[true])
	// Positive: the truncation leaves the speed free to move, unlike a
	// projection, whose gain here was zero to rounding.
	if !(gains[true] > 0.01) {
		t.Errorf("consistent along-course velocity gain %.4f s⁻¹, want clearly positive", gains[true])
	}
	// Bounded: no larger than the shipped gain, which the truncation's
	// smaller variance and covariance can only reduce, and below 1/dt.
	if gains[true] > gains[false] {
		t.Errorf("the consistent clamp left a velocity gain %.3f above the shipped %.3f", gains[true], gains[false])
	}
	if gains[true] > 1/0.1 {
		t.Errorf("consistent along-course velocity gain %.3f s⁻¹ is not bounded by 1/dt", gains[true])
	}
}

// The solid body's speed clamp takes the same form under the option.
func TestConsistentSpeedClampReachesTheSolidBody(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.ConsistentSpeedClamp = true
	tr := NewTracker(cfg)
	track := &TrackedObject{TrackID: "trk_sb"}
	sb := &track.solidBody
	sb.seeded = true
	sb.state = [4]float32{10, 4, 32, 9}
	sb.p = speedClampCovariance()
	tr.finishSolidBodyState(track, "test")
	if s := math.Hypot(float64(sb.state[2]), float64(sb.state[3])); s > 30+1e-4 {
		t.Errorf("solid-body speed %v above the limit", s)
	}
	if sb.p == speedClampCovariance() {
		t.Error("the solid body's covariance was left as the shipped clamp leaves it")
	}
	if sb.state[0] == 10 {
		t.Error("the solid body's position did not move with its clamped speed")
	}
}
