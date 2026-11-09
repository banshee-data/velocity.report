package l5tracks

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The guards of initialiseTwoPoint (two_point_init.go): a measurement in a
// confirmed track's shadow, and a reference point that slides with the
// visible extent, are not evidence of a body's velocity.

// confirmedSpeeds runs frames and returns each track that was ever confirmed
// with the highest speed it published while confirmed, and how many tracks
// the tracker created.
func confirmedSpeeds(cfg TrackerConfig, frames []syntheticPassFrame) (map[string]float32, int) {
	tr := NewTracker(cfg)
	speeds := map[string]float32{}
	for _, f := range frames {
		tr.Update(f.clusters, f.at)
		for _, tk := range tr.GetAllTracks() {
			if tk.TrackState != TrackConfirmed {
				continue
			}
			if s := float32(math.Hypot(float64(tk.VX), float64(tk.VY))); s > speeds[tk.TrackID] {
				speeds[tk.TrackID] = s
			} else if _, ok := speeds[tk.TrackID]; !ok {
				speeds[tk.TrackID] = s
			}
		}
	}
	return speeds, tr.TracksCreated
}

// The defect l5est0006 found, as a scene. Before the guards, the option
// confirmed the reflection at 17.6 m/s, which the shipped tracker never
// confirms, and published the fragment-born 6 m/s vehicle at 26 m/s. With
// them it confirms what the shipped tracker confirms: the two vehicles, at
// their own speeds.
func TestTwoPointVelocityInitDoesNotHoldAReflection(t *testing.T) {
	frames := ghostScene()
	for _, on := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.TwoPointVelocityInit = on
		speeds, created := confirmedSpeeds(cfg, frames)
		t.Logf("two-point %v: %d tracks created, confirmed speeds %v", on, created, speeds)
		if created <= 2 {
			t.Fatalf("two-point %v: %d tracks created; the reflection was never born, so the scene no longer reaches the guard", on, created)
		}
		if len(speeds) != 2 {
			t.Errorf("two-point %v: %d confirmed tracks, want the two vehicles", on, len(speeds))
		}
		for id, s := range speeds {
			if s > 8.5 {
				t.Errorf("two-point %v: confirmed track %s reached %.2f m/s; neither vehicle exceeds 8", on, id, s)
			}
		}
	}
}

func TestInConfirmedShadow(t *testing.T) {
	newTracker := func(state TrackState, sensorY float32) (*Tracker, *TrackedObject) {
		cfg := DefaultTrackerConfig()
		cfg.NoiseSensorY = sensorY
		tr := NewTracker(cfg)
		body := &TrackedObject{X: 0, Y: 10, OBBLength: 4.6, OBBWidth: 1.9}
		body.TrackState = state
		tr.Tracks["body"] = body
		return tr, body
	}
	self := &TrackedObject{}
	cases := []struct {
		name    string
		state   TrackState
		sensorY float32
		x, y    float32
		want    bool
	}{
		{"behind the body", TrackConfirmed, 0, 0, 20, true},
		{"behind its corner bearing", TrackConfirmed, 0, 2, 20, true},
		{"beside its shadow", TrackConfirmed, 0, 8, 20, false},
		{"nearer than the body", TrackConfirmed, 0, 0, 5, false},
		{"a tentative body casts none", TrackTentative, 0, 0, 20, false},
		{"from a sensor beyond the body, the far side", TrackConfirmed, 30, 0, 0, true},
		{"from a sensor beyond the body, the near side", TrackConfirmed, 30, 0, 20, false},
	}
	for _, c := range cases {
		tr, _ := newTracker(c.state, c.sensorY)
		if got := tr.inConfirmedShadow(self, c.x, c.y); got != c.want {
			t.Errorf("%s: inConfirmedShadow(%v, %v) = %v, want %v", c.name, c.x, c.y, got, c.want)
		}
	}
	// A track does not shadow itself.
	tr, body := newTracker(TrackConfirmed, 0)
	if tr.inConfirmedShadow(body, 0, 20) {
		t.Error("a track shadowed its own measurement")
	}
	// A box without extent casts no shadow.
	body.OBBLength, body.OBBWidth = 0, 0
	if tr.inConfirmedShadow(self, 0, 20) {
		t.Error("a box without extent cast a shadow")
	}
}

func twoPointFootprintCluster(x, length float32, at time.Time) WorldCluster {
	c := hygieneCluster(x, 2, at)
	c.BoundingBoxLength = length
	c.OBB = &l4perception.OrientedBoundingBox{CenterX: x, CenterY: 2, Length: length, Width: 1.8, Height: 1.5}
	return c
}

// A body first seen as its rear 0.6 m and then whole: the reference point
// slides 2 m forward on the body's 0.6 m of motion, an implied 26 m/s. The
// slack, 2 m, puts the velocity variance at 400 m²/s², above the cap, so the
// pair declines and the shipped update runs exactly.
func TestTwoPointVelocityInitDeclinesASlidingReferencePoint(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	run := func(on bool) *TrackedObject {
		tr := twoPointTracker(func(cfg *TrackerConfig) { cfg.TwoPointVelocityInit = on })
		track := tr.initTrack(twoPointFootprintCluster(3, 0.6, t0), t0.UnixNano())
		tr.predict(track, 0.1)
		tr.update(track, twoPointFootprintCluster(5.6, 4.6, t0.Add(100*time.Millisecond)), t0.Add(100*time.Millisecond).UnixNano())
		return track
	}
	off, on := run(false), run(true)
	if on.LastResidual.Disposition != "" {
		t.Errorf("disposition %q, want the shipped update", on.LastResidual.Disposition)
	}
	if on.X != off.X || on.Y != off.Y || on.VX != off.VX || on.VY != off.VY || on.P != off.P {
		t.Errorf("the decline differs from the shipped update:\n on %v %v\noff %v %v",
			[4]float32{on.X, on.Y, on.VX, on.VY}, on.P, [4]float32{off.X, off.Y, off.VX, off.VY}, off.P)
	}
}

// A body whose visible length grows by 0.6 m along its 0.6 m displacement:
// the pair initialises, with the velocity it implies, and the slack of
// 0.3 m widens only the velocity variance along the displacement.
func TestTwoPointVelocityInitWidensTheVelocityAlongTheSlack(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	tr := twoPointTracker(nil)
	track := tr.initTrack(twoPointFootprintCluster(5, 4.0, t0), t0.UnixNano())
	second := twoPointFootprintCluster(5.6, 4.6, t0.Add(100*time.Millisecond))
	if !tr.initialiseTwoPoint(track, second, tr.measurementForCluster(second, 0), nil, [4]float32{1, 0, 0, 1}) {
		t.Fatal("declined a comparable pair")
	}
	const r, dt, slack = 0.05, 0.1, 0.3
	if math.Abs(float64(track.VX)-6) > 1e-4 || track.VY != 0 {
		t.Errorf("velocity (%v, %v), want (6, 0)", track.VX, track.VY)
	}
	want := map[[2]int]float64{
		{0, 0}: r, {1, 1}: r, {0, 2}: r / dt, {1, 3}: r / dt,
		{2, 2}: (2*r + slack*slack) / (dt * dt), {3, 3}: 2 * r / (dt * dt),
		{0, 1}: 0, {2, 3}: 0, {0, 3}: 0, {1, 2}: 0,
	}
	for ij, w := range want {
		for _, idx := range [][2]int{ij, {ij[1], ij[0]}} {
			if got := float64(track.P[idx[0]*4+idx[1]]); math.Abs(got-w) > 1e-5*math.Max(1, w) {
				t.Errorf("P[%d,%d] = %v, want %v", idx[0], idx[1], got, w)
			}
		}
	}
	if _, ok := cholesky4(symmetric64(track.P)); !ok {
		t.Errorf("the widened covariance is not positive definite: %v", track.P)
	}
}

func TestReferenceSlack(t *testing.T) {
	along := clusterFootprint{length: 4, width: 2, heading: 0, oriented: true}
	across := clusterFootprint{length: 4, width: 2, heading: math.Pi / 2, oriented: true}
	for _, c := range []struct {
		name          string
		first, second clusterFootprint
		ux, uy        float64
		dir           bool
		want          float64
	}{
		{"same footprint", along, along, 1, 0, true, 0},
		// Turned a quarter turn: along x the extent goes from 4 to 2.
		{"quarter turn, along x", along, across, 1, 0, true, 1},
		// At 45° both project to the same extent, 3√2.
		{"quarter turn, at 45°", along, across, math.Sqrt2 / 2, math.Sqrt2 / 2, true, 0},
		{"no direction", along, clusterFootprint{length: 5, width: 2, oriented: true}, 0, 0, false, 0.5},
		// Without a heading the extent is the larger dimension.
		{"unoriented", clusterFootprint{length: 4, width: 2}, clusterFootprint{length: 5, width: 2}, 0, 1, true, 0.5},
	} {
		if got := referenceSlack(c.first, c.second, c.ux, c.uy, c.dir); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: slack %v, want %v", c.name, got, c.want)
		}
	}
}
