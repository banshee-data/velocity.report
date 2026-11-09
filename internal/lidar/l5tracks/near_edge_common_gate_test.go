package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Tests for NearEdgeCommonGate (l5id0022). What carries the weight: a long
// body's medoid is not a jump of the body, so its cluster stays the body's
// track's instead of seeding a second track; a real jump is still refused;
// the displacement is measured on A2's directions; and off, or under A1, the
// gate is what it was (near_edge_common_gate_off_test.go pins the bits).

// faceStateTrackedConfig is the benchmark's near-edge arm in Go: the solid
// body with the face state on the tracked filter.
func faceStateTrackedConfig() TrackerConfig {
	cfg := faceStateConfig()
	cfg.NearEdgeTracking = true
	return cfg
}

func TestNearEdgeCommonGateIsOffByDefault(t *testing.T) {
	if DefaultTrackerConfig().NearEdgeCommonGate {
		t.Fatal("the common gate is on by default")
	}
}

func TestBodyCentreDisplacementAllowsTheMedoidItsHalfExtent(t *testing.T) {
	cases := []struct {
		name string
		dirs [2]faceResidualDirection
		want float64
	}{
		{"medoid inside the half-length", [2]faceResidualDirection{{residual: 2.4, loose: true, half: 2.6}, {residual: 0.3, loose: true, half: 1}}, 0},
		{"medoid beyond the half-length", [2]faceResidualDirection{{residual: 3.2, loose: true, half: 2.6}, {residual: 0, loose: true, half: 1}}, 0.6},
		{"either side", [2]faceResidualDirection{{residual: -3.2, loose: true, half: 2.6}, {residual: 0, loose: true, half: 1}}, 0.6},
		// A face's residual is the body's own displacement along its normal:
		// nothing is allowed for geometry there.
		{"a face and a loose direction", [2]faceResidualDirection{{residual: 0.4}, {residual: 1.6, loose: true, half: 1}}, math.Hypot(0.4, 0.6)},
		{"two faces", [2]faceResidualDirection{{residual: 3.1}, {residual: -0.2}}, math.Hypot(3.1, 0.2)},
	}
	for _, c := range cases {
		if got := float64(bodyCentreDisplacement(c.dirs)); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: displacement %.6f, want %.6f", c.name, got, c.want)
		}
	}
}

// countTracks runs a pass and returns the tracks created and the most
// confirmed tracks live on one frame: on a pass of one vehicle, more than one
// is a duplicate.
func countTracks(t *testing.T, cfg TrackerConfig, pass l4perception.SyntheticPass) (created, concurrent int) {
	t.Helper()
	tracker := NewTracker(cfg)
	for _, f := range syntheticPassFrames(t, pass) {
		tracker.Update(f.clusters, f.at)
		live := 0
		for _, tr := range tracker.Tracks {
			if tr.TrackState == TrackConfirmed {
				live++
			}
		}
		concurrent = max(concurrent, live)
	}
	return int(tracker.NextTrackID - 1), concurrent
}

func TestNearEdgeCommonGateKeepsOneTruckOneTrack(t *testing.T) {
	// A 9.6 m truck on the tracked filter: its medoid sits more than 3 m
	// (MaxReasonableSpeedMps over a 0.1 s frame) from its believed centre,
	// so the shipped plausibility check refuses the truck's own cluster as a
	// jump, the track coasts and the cluster seeds a second track on the same
	// body. With the common gate it is one track at a time, as the default
	// tracker and the shadow keep it, except on the face-state arm's through
	// pass, where it cuts five at once to two: the second is not this
	// defect, and is left to a separate change (l5id0022).
	cases := []struct {
		name           string
		config         func() TrackerConfig
		pass           func() l4perception.SyntheticPass
		wantConcurrent int
	}{
		{"tracked_through", nearEdgeTrackingConfig, throughTruckPass, 1},
		{"face_state_end_on", faceStateTrackedConfig, endOnTruckPass, 1},
		{"face_state_through", faceStateTrackedConfig, throughTruckPass, 2},
	}
	for _, c := range cases {
		cfg := c.config()
		offCreated, offConcurrent := countTracks(t, cfg, c.pass())
		cfg.NearEdgeCommonGate = true
		onCreated, onConcurrent := countTracks(t, cfg, c.pass())
		t.Logf("%s: off %d created, at most %d confirmed at once; on %d created, at most %d", c.name, offCreated, offConcurrent, onCreated, onConcurrent)
		if onConcurrent > c.wantConcurrent || onConcurrent >= offConcurrent {
			t.Errorf("%s: %d confirmed tracks at once on one truck with the common gate (%d without), want at most %d",
				c.name, onConcurrent, offConcurrent, c.wantConcurrent)
		}
		if onCreated >= offCreated {
			t.Errorf("%s: the common gate created %d tracks against %d without it; want fewer", c.name, onCreated, offCreated)
		}
	}
}

// shiftedCluster is c moved by (dx, dy), its points with it.
func shiftedCluster(c WorldCluster, dx, dy float32) WorldCluster {
	c.CentroidX += dx
	c.CentroidY += dy
	move := func(points []l4perception.WorldPoint) []l4perception.WorldPoint {
		out := make([]l4perception.WorldPoint, len(points))
		for i, p := range points {
			p.X += float64(dx)
			p.Y += float64(dy)
			out[i] = p
		}
		return out
	}
	c.Members, c.RetainedPoints = move(c.Members), move(c.RetainedPoints)
	if c.OBB != nil {
		obb := *c.OBB
		obb.CenterX += dx
		obb.CenterY += dy
		c.OBB = &obb
	}
	return c
}

func TestNearEdgeCommonGateAdmitsTheBodyAndRefusesAJump(t *testing.T) {
	// Frame by frame on the through-truck pass, the body-centre track's own
	// next cluster is judged with the option off and on. So is the same
	// cluster moved 15 m along the road or 8 m across it: jumps no part of a
	// 9.6 by 2.5 m body can make in a frame, since its medoid lies inside
	// the body. A shorter jump along so long a body is not impossible from
	// the medoid alone, so the bound admits it and A2's faces judge it; the
	// share of 8 m jumps the whole gate admits is logged.
	cfg := faceStateTrackedConfig()
	cfg.NearEdgeCommonGate = true
	tracker := NewTracker(cfg)
	frames := syntheticPassFrames(t, throughTruckPass())
	const dt = 0.1
	var bodyFrames, offRefused, onRefused, jumpsAdmitted, shortJumpsGated int
	for i := 0; i+1 < len(frames); i++ {
		tracker.Update(frames[i].clusters, frames[i].at)
		next := frames[i+1].clusters
		if len(next) == 0 {
			continue
		}
		if len(tracker.GetActiveTracks()) == 0 {
			continue
		}
		track := mainTrack(t, tracker)
		if !track.solidBody.tracked || track.solidBody.reference != ReferenceBodyCentre || track.TrackState != TrackConfirmed {
			continue
		}
		biggest := 0
		for ci := range next {
			if next[ci].PointsCount > next[biggest].PointsCount {
				biggest = ci
			}
		}
		snapshot := *track
		gate := func(on bool, clusters []WorldCluster) float32 {
			tracker.Config.NearEdgeCommonGate = on
			clear(tracker.nearEdgePairs)
			probe := snapshot
			d2, _ := tracker.gateInnovation(&probe, clusters, biggest, dt)
			return d2
		}
		bodyFrames++
		if gate(false, next) >= SingularDistanceRejection {
			offRefused++
		}
		if gate(true, next) >= SingularDistanceRejection {
			onRefused++
		}
		for _, shift := range [][2]float32{{15, 0}, {0, 8}, {0, -8}} {
			jumped := append([]WorldCluster(nil), next...)
			jumped[biggest] = shiftedCluster(next[biggest], shift[0], shift[1])
			if gate(true, jumped) < SingularDistanceRejection {
				jumpsAdmitted++
			}
		}
		short := append([]WorldCluster(nil), next...)
		short[biggest] = shiftedCluster(next[biggest], 8, 0)
		if gate(true, short) <= cfg.GatingDistanceSquared {
			shortJumpsGated++
		}
		tracker.Config.NearEdgeCommonGate = true
		clear(tracker.nearEdgePairs)
	}
	t.Logf("body-centre frames %d: the shipped check refused the body's cluster on %d, the common gate on %d; the gate admitted an 8 m jump along on %d",
		bodyFrames, offRefused, onRefused, shortJumpsGated)
	if bodyFrames < 10 {
		t.Fatalf("only %d body-centre frames to judge", bodyFrames)
	}
	if offRefused == 0 {
		t.Error("the shipped plausibility check never refused the truck's own cluster; the pass no longer shows the defect")
	}
	if onRefused >= offRefused {
		t.Errorf("the common gate refused the body's own cluster on %d frames, the shipped check on %d", onRefused, offRefused)
	}
	if jumpsAdmitted > 0 {
		t.Errorf("the common gate admitted %d impossible jumps over %d frames", jumpsAdmitted, bodyFrames)
	}
}

func TestNearEdgeCommonGateLeavesA1AndMedoidTracksAlone(t *testing.T) {
	// Under A1 the gate is the medoid's by definition, and a track still
	// referenced to the medoid is gated as before: the option changes
	// neither, frame by frame.
	for _, medoidGate := range []bool{false, true} {
		cfg := nearEdgeTrackingConfig()
		cfg.NearEdgeMedoidGate = medoidGate
		tracker := NewTracker(cfg)
		frames := syntheticPassFrames(t, endOnTruckPass())
		for i := 0; i+1 < len(frames); i++ {
			tracker.Update(frames[i].clusters, frames[i].at)
			next := frames[i+1].clusters
			for _, track := range tracker.Tracks {
				if !medoidGate && track.solidBody.reference == ReferenceBodyCentre {
					continue
				}
				for ci := range next {
					snapshot := *track
					tracker.Config.NearEdgeCommonGate = false
					clear(tracker.nearEdgePairs)
					off, _ := tracker.gateInnovation(&snapshot, next, ci, 0.1)
					snapshot = *track
					tracker.Config.NearEdgeCommonGate = true
					clear(tracker.nearEdgePairs)
					on, _ := tracker.gateInnovation(&snapshot, next, ci, 0.1)
					if math.Float32bits(off) != math.Float32bits(on) {
						t.Fatalf("A1 %v, frame %d, reference %v: d² %v off, %v on", medoidGate, i, track.solidBody.reference, off, on)
					}
				}
			}
			tracker.Config.NearEdgeCommonGate = false
			clear(tracker.nearEdgePairs)
		}
	}
}
