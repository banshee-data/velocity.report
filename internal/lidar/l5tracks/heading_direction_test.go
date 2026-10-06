package l5tracks

import (
	"math"
	"testing"
)

// Gap P3 in data/maths/paper-implementation-gap-analysis.md: PCA's heading has
// a 180-degree ambiguity that L5 resolves with velocity, then displacement,
// then smooths. No test drove that chain end to end for a track moving in
// each cardinal direction while every PCA measurement arrives flipped, nor for
// a stationary track, where neither resolver has anything to work with.

func angleDiffDeg(a, b float64) float64 {
	d := math.Mod(a-b+3*math.Pi, 2*math.Pi) - math.Pi
	return math.Abs(d) * 180 / math.Pi
}

// directionTracker seeds a confirmed track moving at 10 m/s along dirDeg with
// its smoothed heading already there, then feeds frames whose PCA heading is
// the flipped axis (dirDeg + 180) as the track advances.
func drive(t *testing.T, flipRule bool, dirDeg float64, speed float32, pcaDeg func(frame int) float64) *TrackedObject {
	t.Helper()
	cfg := DefaultTrackerConfig()
	cfg.OBBHeadingFlipRule = flipRule
	tk := NewTracker(cfg)
	dir := dirDeg * degToRad
	tr := &TrackedObject{
		TrackID:          "dir",
		TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed},
		VX:               speed * float32(math.Cos(dir)),
		VY:               speed * float32(math.Sin(dir)),
		OBBHeadingRad:    float32(dir),
		P:                [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	}
	tr.ObservationCount = 5
	tk.Tracks[tr.TrackID] = tr
	for frame := 1; frame <= 40; frame++ {
		dt := 0.1 * float64(frame)
		x := float32(float64(speed) * dt * math.Cos(dir))
		y := float32(float64(speed) * dt * math.Sin(dir))
		tk.update(tr, elongatedCluster(x, y, pcaDeg(frame)), int64(dt*1e9))
	}
	return tr
}

func TestFlippedPCAHeadingIsResolvedTowardTravelInEveryCardinalDirection(t *testing.T) {
	for _, dirDeg := range []float64{0, 90, 180, 270} {
		tr := drive(t, false, dirDeg, 10, func(int) float64 { return dirDeg + 180 })
		if d := angleDiffDeg(float64(tr.OBBHeadingRad), dirDeg*degToRad); d > 10 {
			t.Errorf("travel %.0f deg with every PCA heading flipped: smoothed heading ended %.1f deg from travel (source %v)",
				dirDeg, d, tr.HeadingSource)
		}
	}
}

// A stationary track alternating between the two PCA signs of one axis. The
// velocity and displacement resolvers are silent, and Guard 3 rejects only
// 60-120 degree jumps, so a 180 degree flip reaches the smoother, which eases
// toward each measurement along the shorter arc. Alternating signs therefore
// walk the heading sideways, off the axis every measurement lay on: 26.6
// degrees in 40 frames when this test was written. The flip rule holds it.
func TestStationaryAxisFlipWalksTheHeadingWithoutTheFlipRule(t *testing.T) {
	alternating := func(frame int) float64 {
		if frame%2 == 0 {
			return 180
		}
		return 0
	}
	without := angleDiffDeg(float64(drive(t, false, 0, 0, alternating).OBBHeadingRad), 0)
	with := angleDiffDeg(float64(drive(t, true, 0, 0, alternating).OBBHeadingRad), 0)
	t.Logf("stationary alternating PCA sign: heading ended %.1f deg from start without the rule, %.1f with it", without, with)

	if without < 10 {
		t.Errorf("without the flip rule the heading moved only %.1f deg; the walk P3 describes did not reproduce, "+
			"so this test no longer measures what it claims", without)
	}
	if with > 5 {
		t.Errorf("with the flip rule the heading ended %.1f deg from its start; the rule should hold a stationary axis", with)
	}
}

func TestHeadingFlipRuleOffByDefault(t *testing.T) {
	if DefaultTrackerConfig().OBBHeadingFlipRule {
		t.Fatal("OBBHeadingFlipRule defaults on; it must be measured against labelled turns first")
	}
}
