package main

import (
	"math"
	"testing"
)

// The reference path is E1.1's measuring stick. If the fit or the signed
// offset is wrong, every conditional mean is wrong in a way that still looks
// like a plausible table, so both are pinned here.

func straightTrack(x0, y0, dx, dy float64, n int) []trackEstimate {
	var out []trackEstimate
	for i := 0; i < n; i++ {
		t := float64(i)
		out = append(out, trackEstimate{
			x: x0 + t*dx, y: y0 + t*dy,
			vx: dx * 10, vy: dy * 10, // 10 Hz
			frameUnixNanos: int64(i) * 100_000_000,
		})
	}
	return out
}

func TestFitTrackPathRecoversAStraightLineInAnyOrientation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dx, dy float64
	}{
		{"along X", 1.2, 0},
		{"along Y", 0, 1.2},
		{"diagonal", 0.85, 0.85},
		{"shallow", 1.2, 0.1},
		{"negative direction", -1.2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, ok := fitTrackPath(straightTrack(5, -3, tc.dx, tc.dy, 30))
			if !ok {
				t.Fatal("fit failed on a straight track")
			}
			// A perfect line has no residual.
			if path.residualRMS > 1e-9 {
				t.Errorf("residual RMS = %v on an exactly straight track", path.residualRMS)
			}
			// The direction must be parallel to the motion, up to sign: a line
			// has no preferred direction.
			cross := tc.dx*path.dirY - tc.dy*path.dirX
			if math.Abs(cross) > 1e-6*math.Hypot(tc.dx, tc.dy) {
				t.Errorf("fitted direction (%v,%v) is not parallel to (%v,%v)", path.dirX, path.dirY, tc.dx, tc.dy)
			}
			// Unit length, since offsets are distances.
			if mag := math.Hypot(path.dirX, path.dirY); math.Abs(mag-1) > 1e-9 {
				t.Errorf("direction magnitude = %v, want 1", mag)
			}
			// Every point on the line is at zero offset.
			for _, e := range straightTrack(5, -3, tc.dx, tc.dy, 30) {
				if off := path.lateralOffset(e.x, e.y); math.Abs(off) > 1e-9 {
					t.Fatalf("a point on the track is at offset %v", off)
				}
			}
			wantSpeed := math.Hypot(tc.dx, tc.dy) * 10
			if math.Abs(path.speedMps-wantSpeed) > 1e-9 {
				t.Errorf("speed = %v, want %v", path.speedMps, wantSpeed)
			}
		})
	}
}

func TestLateralOffsetIsSignedAndPerpendicular(t *testing.T) {
	// A track along +X through y=0: the offset must be the point's own y, and
	// must not respond to displacement along the track.
	path, ok := fitTrackPath(straightTrack(0, 0, 1, 0, 20))
	if !ok {
		t.Fatal("fit failed")
	}

	for _, tc := range []struct {
		x, y, want float64
	}{
		{0, 2, 2},
		{50, 2, 2},    // moving along the track changes nothing
		{-50, -3, -3}, // and the sign follows the side
		{7, 0, 0},
	} {
		got := path.lateralOffset(tc.x, tc.y)
		if math.Abs(math.Abs(got)-math.Abs(tc.want)) > 1e-9 {
			t.Errorf("offset at (%v,%v) = %v, want magnitude %v", tc.x, tc.y, got, tc.want)
		}
	}
	// Opposite sides must get opposite signs, which is what makes a
	// conditional mean meaningful rather than a mean of absolute values.
	above := path.lateralOffset(0, 2)
	below := path.lateralOffset(0, -2)
	if above*below >= 0 {
		t.Errorf("offsets on opposite sides share a sign: %v and %v", above, below)
	}
}

func TestFitTrackPathReportsCurvatureThroughItsResidual(t *testing.T) {
	// A curved track must be rejected as a reference, because a line fitted to
	// a curve produces residuals that vary along the track for reasons that
	// have nothing to do with aspect angle.
	var curved []trackEstimate
	for i := 0; i < 40; i++ {
		t := float64(i) * 0.1
		curved = append(curved, trackEstimate{
			x: t * 10, y: 0.5 * t * t, // constant lateral acceleration
			vx: 10, vy: t,
		})
	}
	path, ok := fitTrackPath(curved)
	if !ok {
		t.Fatal("fit failed")
	}
	if path.residualRMS < 0.5 {
		t.Errorf("residual RMS = %v on a clearly curved track; it would pass as a straight reference", path.residualRMS)
	}

	straight, _ := fitTrackPath(straightTrack(0, 0, 1, 0, 40))
	if straight.residualRMS >= path.residualRMS {
		t.Errorf("a straight track's residual %v is not below a curved one's %v", straight.residualRMS, path.residualRMS)
	}
}

func TestFitTrackPathRefusesTooFewPoints(t *testing.T) {
	for _, n := range []int{0, 1, 2} {
		if _, ok := fitTrackPath(straightTrack(0, 0, 1, 0, n)); ok {
			t.Errorf("accepted a fit over %d points", n)
		}
	}
	if _, ok := fitTrackPath(straightTrack(0, 0, 1, 0, 3)); !ok {
		t.Error("refused a fit over three points")
	}
}

func TestFitTrackPathHandlesAStationaryTrack(t *testing.T) {
	// Every estimate at the same place: the covariance is zero, so there is no
	// principal axis. It must not produce NaNs, since a stationary track is
	// rejected later on speed rather than here.
	var still []trackEstimate
	for i := 0; i < 25; i++ {
		still = append(still, trackEstimate{x: 4, y: 4})
	}
	path, ok := fitTrackPath(still)
	if !ok {
		t.Fatal("fit failed on a stationary track")
	}
	for name, v := range map[string]float64{
		"dirX": path.dirX, "dirY": path.dirY,
		"residualRMS": path.residualRMS, "speedMps": path.speedMps,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("%s is not finite: %v", name, v)
		}
	}
	if mag := math.Hypot(path.dirX, path.dirY); math.Abs(mag-1) > 1e-9 {
		t.Errorf("direction magnitude = %v, want a unit fallback", mag)
	}
	if path.speedMps != 0 {
		t.Errorf("speed = %v, want 0", path.speedMps)
	}
}
