package l4perception

import (
	"math"
	"testing"
	"time"
)

// The generator's contract is that every return is exactly where the geometry
// says, on a face the sensor can actually see. If that fails, every measurement
// conclusion drawn from it is worthless, so these tests check the geometry
// itself rather than the conclusions.

func TestSyntheticReturnsLandOnTheBoxSurface(t *testing.T) {
	pass := DefaultSyntheticPass()
	frames := GenerateSyntheticPass(pass)
	if len(frames) != pass.Frames {
		t.Fatalf("got %d frames, want %d", len(frames), pass.Frames)
	}

	v := pass.Vehicle
	for _, f := range frames {
		if len(f.Points) == 0 {
			t.Errorf("frame %d produced no returns at all", f.Index)
			continue
		}
		minX, maxX := f.TrueCentreX-v.Length/2, f.TrueCentreX+v.Length/2
		minY, maxY := f.TrueCentreY-v.Width/2, f.TrueCentreY+v.Width/2

		for _, p := range f.Points {
			// Inside the box's bounds, to within float slop.
			if p.X < minX-1e-6 || p.X > maxX+1e-6 ||
				p.Y < minY-1e-6 || p.Y > maxY+1e-6 ||
				p.Z < -1e-6 || p.Z > v.Height+1e-6 {
				t.Fatalf("frame %d: return (%.4f, %.4f, %.4f) is outside the box X[%.2f,%.2f] Y[%.2f,%.2f] Z[0,%.2f]",
					f.Index, p.X, p.Y, p.Z, minX, maxX, minY, maxY, v.Height)
			}
			// And on its surface, not inside it: at least one coordinate must
			// sit on a face.
			onFace := math.Abs(p.X-minX) < 1e-6 || math.Abs(p.X-maxX) < 1e-6 ||
				math.Abs(p.Y-minY) < 1e-6 || math.Abs(p.Y-maxY) < 1e-6 ||
				math.Abs(p.Z) < 1e-6 || math.Abs(p.Z-v.Height) < 1e-6
			if !onFace {
				t.Fatalf("frame %d: return (%.4f, %.4f, %.4f) is inside the box rather than on a face",
					f.Index, p.X, p.Y, p.Z)
			}
		}
	}
}

func TestSyntheticNeverReturnsTheFarFace(t *testing.T) {
	// The property the whole of Section 3 rests on: the far face is not
	// observed at all and must come from a prior. A generator that leaked
	// far-face returns would make every measurement definition look better
	// than it is.
	pass := DefaultSyntheticPass()
	frames := GenerateSyntheticPass(pass)
	v := pass.Vehicle

	for _, f := range frames {
		// The sensor is at the origin and the vehicle is at positive Y, so the
		// far lateral face is the one at the larger Y.
		farY := f.TrueCentreY + v.Width/2
		minX, maxX := f.TrueCentreX-v.Length/2, f.TrueCentreX+v.Length/2

		for _, p := range f.Points {
			if math.Abs(p.Y-farY) > 1e-6 {
				continue
			}
			// Reaching the far Y is legitimate on an *edge*, where a visible
			// face meets it: the roof, or whichever end face points toward the
			// sensor. Only the far face's interior is impossible, so the test
			// has to exclude the edges rather than the whole plane — checking
			// the plane would flag the corner of a legitimately visible end
			// face, which is what a real sensor does see.
			onRoofEdge := math.Abs(p.Z-v.Height) < 1e-6
			onEndEdge := math.Abs(p.X-minX) < 1e-6 || math.Abs(p.X-maxX) < 1e-6
			if onRoofEdge || onEndEdge {
				continue
			}
			t.Fatalf("frame %d: return (%.4f, %.4f, %.4f) is in the interior of the far lateral face",
				f.Index, p.X, p.Y, p.Z)
		}
	}
}

func TestSyntheticNearLateralFaceIsSampledThroughoutThePass(t *testing.T) {
	// The near-edge measurement depends on this face being densely and
	// reliably sampled. If the ring geometry ever stops reaching it, the
	// measurement's premise is gone and the tests below would be measuring
	// something else.
	pass := DefaultSyntheticPass()
	frames := GenerateSyntheticPass(pass)
	v := pass.Vehicle

	for _, f := range frames {
		nearY := f.TrueCentreY - v.Width/2
		var onNearFace int
		for _, p := range f.Points {
			if math.Abs(p.Y-nearY) < 1e-6 {
				onNearFace++
			}
		}
		if onNearFace < 5 {
			t.Errorf("frame %d sampled the near lateral face with only %d returns", f.Index, onNearFace)
		}
	}
}

func TestSyntheticOccluderRemovesReturnsOnlyOnItsFrames(t *testing.T) {
	pass := DefaultSyntheticPass()
	withOccluder := GenerateSyntheticPass(pass)

	clear := pass
	clear.Occluder.WidthDeg = 0
	withoutOccluder := GenerateSyntheticPass(clear)

	for i := range withOccluder {
		occluded := withOccluder[i].Occluded
		got, want := len(withOccluder[i].Points), len(withoutOccluder[i].Points)
		switch {
		case occluded && got >= want:
			t.Errorf("frame %d is occluded but kept all %d returns", i, got)
		case !occluded && got != want:
			t.Errorf("frame %d is not occluded but returns changed: %d against %d", i, got, want)
		}
	}

	// And the occluded frames are the ones configured, not a drifting set.
	for i, f := range withOccluder {
		wantOccluded := i >= pass.Occluder.FirstFrame && i <= pass.Occluder.LastFrame
		if f.Occluded != wantOccluded {
			t.Errorf("frame %d occluded = %v, want %v", i, f.Occluded, wantOccluded)
		}
	}
}

func TestSyntheticTruthIsExactAndConstantVelocity(t *testing.T) {
	pass := DefaultSyntheticPass()
	frames := GenerateSyntheticPass(pass)

	step := pass.Vehicle.SpeedMps * pass.Interval.Seconds()
	for i := 1; i < len(frames); i++ {
		gotStep := frames[i].TrueCentreX - frames[i-1].TrueCentreX
		if math.Abs(gotStep-step) > 1e-9 {
			t.Errorf("frame %d advanced %.9f m, want exactly %.9f", i, gotStep, step)
		}
		if frames[i].TrueCentreY != pass.Vehicle.LateralOffsetMetres {
			t.Errorf("frame %d drifted laterally to %v", i, frames[i].TrueCentreY)
		}
	}
}

func TestSyntheticIsDeterministic(t *testing.T) {
	// A generator that varied between runs would make a regression
	// indistinguishable from noise, which is the whole thing this corpus
	// exists to rule out.
	a := GenerateSyntheticPass(DefaultSyntheticPass())
	b := GenerateSyntheticPass(DefaultSyntheticPass())

	if len(a) != len(b) {
		t.Fatalf("frame counts differ: %d against %d", len(a), len(b))
	}
	for i := range a {
		if len(a[i].Points) != len(b[i].Points) {
			t.Fatalf("frame %d point counts differ: %d against %d", i, len(a[i].Points), len(b[i].Points))
		}
		for j := range a[i].Points {
			if a[i].Points[j] != b[i].Points[j] {
				t.Fatalf("frame %d point %d differs between runs", i, j)
			}
		}
	}
}

func TestSyntheticDegenerateConfigurationsProduceNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*SyntheticPass)
	}{
		{"no frames", func(p *SyntheticPass) { p.Frames = 0 }},
		{"negative frames", func(p *SyntheticPass) { p.Frames = -1 }},
		{"no interval", func(p *SyntheticPass) { p.Interval = 0 }},
		{"no azimuth step", func(p *SyntheticPass) { p.Sensor.AzimuthStepDeg = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pass := DefaultSyntheticPass()
			tc.mutate(&pass)
			if got := GenerateSyntheticPass(pass); got != nil {
				t.Errorf("got %d frames, want none", len(got))
			}
		})
	}
}

// TestMedoidBiasReproducesTheSectionThreeMechanism is the reason the generator
// exists. The exact per-frame figures in Section 3.2 are not reproduced —
// see Pandar40PElevationsDeg for why that table cannot have come from the ring
// band Section 3.1 describes — but the mechanism must, because every later
// decision in the plan rests on it.
func TestMedoidBiasReproducesTheSectionThreeMechanism(t *testing.T) {
	pass := DefaultSyntheticPass()
	frames := GenerateSyntheticPass(pass)
	clusterer := NewDBSCANClusterer(0.6, 5)
	halfWidth := pass.Vehicle.Width / 2

	var pinnedToNearFace, total int
	var worstHop, prevOffset float64
	first := true

	for _, f := range frames {
		cluster, ok := largestCluster(clusterer.Cluster(f.Points, pass.SensorID, f.Timestamp))
		if !ok {
			continue
		}
		total++
		offset := float64(cluster.CentroidY) - f.TrueCentreY

		// The medoid sits at exactly -W/2 whenever the near lateral face
		// dominates the point set: the centroid of a sampled plane is on that
		// plane, and nothing physical is at that position.
		if math.Abs(offset+halfWidth) < 1e-3 {
			pinnedToNearFace++
		}
		if !first {
			if hop := math.Abs(offset - prevOffset); hop > worstHop {
				worstHop = hop
			}
		}
		prevOffset, first = offset, false
	}

	if total < 30 {
		t.Fatalf("only %d frames clustered, too few to characterise the bias", total)
	}
	if pinnedToNearFace == 0 {
		t.Error("the medoid never pinned to the near face: Section 3.3's mechanism did not reproduce")
	}
	// It must also be a bias rather than noise: a substantial share of frames
	// sit at exactly the same wrong place.
	if share := float64(pinnedToNearFace) / float64(total); share < 0.25 {
		t.Errorf("only %.0f%% of frames pinned to -W/2, want a substantial share: the error should be deterministic, not noisy",
			100*share)
	}
	// And it must hop between adjacent frames while nothing physical moves,
	// which is what breaks the filter's zero-mean white-noise assumption.
	if worstHop < 0.3 {
		t.Errorf("worst frame-to-frame medoid hop %.3f m, want a substantial one: the jump is the defect", worstHop)
	}
	t.Logf("medoid pinned to -W/2 (%.3f m) on %d of %d clustered frames; worst hop %.3f m",
		-halfWidth, pinnedToNearFace, total, worstHop)
}

func TestLateralPercentileFindsTheSampledSurface(t *testing.T) {
	// A plane of points sampled along +Y: the 5th percentile along Y must sit
	// on the plane, not somewhere inside the body it bounds.
	var points []WorldPoint
	for i := 0; i < 100; i++ {
		points = append(points, WorldPoint{X: float64(i) * 0.05, Y: 4.1, Z: 0.5})
	}
	got, ok := LateralPercentile(points, 0, 1, NearEdgePercentile)
	if !ok {
		t.Fatal("percentile unavailable for a well-formed point set")
	}
	if math.Abs(got-4.1) > 1e-9 {
		t.Errorf("percentile = %v, want the sampled plane at 4.1", got)
	}

	// It must resist a handful of strays, which is why it is a percentile and
	// not the minimum.
	strays := append([]WorldPoint{}, points...)
	for i := 0; i < 3; i++ {
		strays = append(strays, WorldPoint{X: 1, Y: 1.0, Z: 0.5})
	}
	robust, ok := LateralPercentile(strays, 0, 1, NearEdgePercentile)
	if !ok {
		t.Fatal("percentile unavailable")
	}
	if math.Abs(robust-4.1) > 1e-9 {
		t.Errorf("percentile = %v with three strays present, want the plane at 4.1", robust)
	}
	// The minimum, by contrast, is captured by them: this is the comparison
	// that justifies the choice.
	if minimum, _ := LateralPercentile(strays, 0, 1, 0); math.Abs(minimum-1.0) > 1e-9 {
		t.Errorf("the 0th percentile = %v, want the stray at 1.0", minimum)
	}
}

func TestLateralPercentileRejectsNonsense(t *testing.T) {
	points := []WorldPoint{{X: 1, Y: 2}}
	for _, tc := range []struct {
		name       string
		points     []WorldPoint
		dx, dy     float64
		percentile float64
	}{
		{"no points", nil, 0, 1, 5},
		{"zero direction", points, 0, 0, 5},
		{"percentile below zero", points, 0, 1, -1},
		{"percentile above 100", points, 0, 1, 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := LateralPercentile(tc.points, tc.dx, tc.dy, tc.percentile); ok {
				t.Error("accepted a nonsensical request")
			}
		})
	}
}

func TestCastRayMissesWhenPointedAway(t *testing.T) {
	s := DefaultSyntheticSensor()
	v := DefaultSyntheticVehicle()
	// The vehicle is at positive Y; a ray at azimuth -90 degrees points to
	// negative Y and must miss.
	if _, hit := castRayAtBox(s, v, -10, v.LateralOffsetMetres, -90, -5); hit {
		t.Error("a ray pointed away from the vehicle reported a hit")
	}
	// Straight up must miss a box below the sensor.
	if _, hit := castRayAtBox(s, v, -10, v.LateralOffsetMetres, 0, 89); hit {
		t.Error("a ray pointed at the sky reported a hit")
	}
}

func TestAngleDeltaDegWrapsAtTheBranchCut(t *testing.T) {
	for _, tc := range []struct{ a, b, want float64 }{
		{10, 0, 10},
		{0, 10, -10},
		{179, -179, -2},
		{-179, 179, 2},
		{0, 360, 0},
	} {
		if got := angleDeltaDeg(tc.a, tc.b); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("angleDeltaDeg(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// largestCluster picks the cluster with the most points, which is what the
// production fragment guard effectively does: the sparse steep rings of a real
// Pandar40P detach small slivers from a nearby vehicle, and those are rejected
// on extent before they reach a tracker.
func largestCluster(clusters []WorldCluster) (WorldCluster, bool) {
	best, found := WorldCluster{}, false
	for _, c := range clusters {
		if !found || c.PointsCount > best.PointsCount {
			best, found = c, true
		}
	}
	return best, found
}

var _ = time.Now
