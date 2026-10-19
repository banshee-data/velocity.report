package l5tracks

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// planeSurface is a fitted ground surface for tests: z = a·x + b·y + c.
type planeSurface struct{ a, b, c float64 }

func (p planeSurface) HeightAt(x, y float64) float64 { return p.a*x + p.b*y + p.c }

// passingCar is frame i of a 4.5 × 1.8 m car passing at speed along psi:
// the faces the sensor sees, and a cluster box whose lowest return is low and
// whose top is top, as the frame's foreground left them.
func passingCar(i int, psi, speed, low, top float64) BoxInput {
	d := -15 + speed*0.1*float64(i)
	cx, cy := 12+d*math.Cos(psi), -6+d*math.Sin(psi)
	pts := visibleFaces(cx, cy, psi, 4.5, 1.8, 20)
	in := boxInput(int64(i)*1e8, pts, speed*math.Cos(psi), speed*math.Sin(psi))
	obb := l4perception.EstimateOBBFromCluster(pts)
	obb.CenterZ, obb.Height = float32(low), float32(top-low)
	in.Cluster.OBB = &obb
	return in
}

// lowestReturns is a lowest return that hops between frames as it does on the
// published scenes: the bumper, a wheel arch, the doors, never the road.
var lowestReturns = []float64{-2.0, -1.4, -0.8}

func groundBoxConfig() BoxModelConfig {
	cfg := enabledBoxConfig()
	cfg.GroundAnchor = true
	return cfg
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestBoxGroundAnchorStandsOnTheSurfaceWhateverTheLowestReturn(t *testing.T) {
	// The road is 2.3 m below the sensor and the car's roof 1.5 m above it.
	road := planeSurface{c: -2.3}
	e := NewBoxEstimator(groundBoxConfig())
	above := 0
	for i := 0; i < 30; i++ {
		low := lowestReturns[i%len(lowestReturns)]
		if low > -2.3+boxLowAboveBaseMetres {
			above++
		}
		in := passingCar(i, 0.4, 8, low, -0.8)
		in.Surface = road
		b := e.Observe(in)
		if !near(b.Base, -2.3) || !b.OnSurface {
			t.Fatalf("frame %d: base %.3f (on surface %v), want the road at -2.3 whatever the lowest return (%.1f)", i, b.Base, b.OnSurface, low)
		}
		if !near(b.Height, 1.5) {
			t.Fatalf("frame %d: height %.3f, want the roof 1.5 m above the road", i, b.Height)
		}
	}
	if e.Stats.LowAboveBase != above || e.Stats.LowBelowBase != 0 {
		t.Fatalf("stats %+v, want %d frames whose lowest return lay above the base and none below", e.Stats, above)
	}
}

func TestBoxGroundAnchorFollowsTheSurfaceUnderTheBox(t *testing.T) {
	// A 4 % grade along x: the base is the surface under the box's own
	// centre, frame by frame, not one height for the whole pass.
	grade := planeSurface{a: 0.04, c: -2.3}
	e := NewBoxEstimator(groundBoxConfig())
	for i := 0; i < 20; i++ {
		in := passingCar(i, 0, 8, -1.4, -0.5)
		in.Surface = grade
		b := e.Observe(in)
		if want := grade.HeightAt(b.X, b.Y); !near(b.Base, want) {
			t.Fatalf("frame %d: base %.3f, want the surface under the centre %.3f", i, b.Base, want)
		}
	}
}

func TestBoxGroundAnchorHoldsTheTopTheFrameNoLongerSees(t *testing.T) {
	// The roof is seen for ten frames, then only its lower edge: the height
	// is a held belief, not this frame's.
	road := planeSurface{c: -2.3}
	e := NewBoxEstimator(groundBoxConfig())
	var b BoxState
	for i := 0; i < 20; i++ {
		top := -0.8
		if i >= 10 {
			top = -1.3
		}
		in := passingCar(i, 0.4, 8, -1.4, top)
		in.Surface = road
		b = e.Observe(in)
	}
	if !near(b.Height, 1.5) {
		t.Fatalf("height %.3f after the roof left view, want the 1.5 m it was seen at", b.Height)
	}
}

func TestBoxGroundAnchorKeepsAMergeOutOfTheHeight(t *testing.T) {
	// A truck beside the car merges with it for three frames: those frames'
	// boxes hold the merged points, and the car's height comes back after.
	road := planeSurface{c: -2.3}
	e := NewBoxEstimator(groundBoxConfig())
	for i := 0; i < 16; i++ {
		top := -0.8
		in := passingCar(i, 0.4, 8, -1.4, top)
		if i >= 10 && i < 13 {
			in.Cluster.OBB.Height = float32(1.2 - -1.4)
			in.Merge = true
		}
		in.Surface = road
		b := e.Observe(in)
		switch {
		case in.Merge && !near(b.Height, 3.5):
			t.Fatalf("frame %d: merged frame drawn %.3f high, want 3.5 to hold its points", i, b.Height)
		case !in.Merge && !near(b.Height, 1.5):
			t.Fatalf("frame %d: height %.3f, want the car's 1.5 m, the merge kept out of the belief", i, b.Height)
		}
	}
}

func TestBoxGroundAnchorLeavesThePlaneAsItWas(t *testing.T) {
	// Nothing on the plane reads the vertical: centre, heading, extents and
	// label are the same with the anchor as without, frame for frame.
	off := NewBoxEstimator(enabledBoxConfig())
	on := NewBoxEstimator(groundBoxConfig())
	road := planeSurface{a: 0.02, c: -2.3}
	for i := 0; i < 40; i++ {
		in := passingCar(i, 0.6, 7, lowestReturns[i%len(lowestReturns)], -0.8)
		if i%9 == 8 {
			in.Cluster = nil // a coasting frame
		}
		in.Surface = road
		a, b := off.Observe(in), on.Observe(in)
		if a.X != b.X || a.Y != b.Y || a.HeadingRad != b.HeadingRad || a.Length != b.Length || a.Width != b.Width || a.Labelled != b.Labelled {
			t.Fatalf("frame %d: the anchor moved the plane:\n off %+v\n  on %+v", i, a, b)
		}
		if a.Base != 0 || a.Height != 0 || a.OnSurface {
			t.Fatalf("frame %d: anchor off, but the box carries a vertical %+v", i, a)
		}
	}
}

func TestBoxGroundAnchorWithoutASurfaceIsTheCluster(t *testing.T) {
	// Before the background settles there is no surface: the base and the
	// height are the cluster's, what a viewer draws without the option, and
	// a coasting frame keeps the last.
	e := NewBoxEstimator(groundBoxConfig())
	var b BoxState
	for i := 0; i < 6; i++ {
		low := lowestReturns[i%len(lowestReturns)]
		b = e.Observe(passingCar(i, 0.4, 8, low, -0.5))
		if b.OnSurface || !near(b.Base, low) || !near(b.Height, -0.5-low) {
			t.Fatalf("frame %d: %+v, want the cluster's base %.1f and height %.1f", i, b, low, -0.5-low)
		}
	}
	coast := passingCar(6, 0.4, 8, 0, 0)
	coast.Cluster = nil
	if c := e.Observe(coast); c.Base != b.Base || c.Height != b.Height {
		t.Fatalf("coasting frame drew %.3f / %.3f, want the last %.3f / %.3f", c.Base, c.Height, b.Base, b.Height)
	}
}

func TestBoxGroundAnchorCoastsOnTheSurface(t *testing.T) {
	grade := planeSurface{a: 0.05, c: -2.3}
	e := NewBoxEstimator(groundBoxConfig())
	for i := 0; i < 10; i++ {
		in := passingCar(i, 0, 8, -1.4, -0.5)
		in.Surface = grade
		e.Observe(in)
	}
	coast := passingCar(10, 0, 8, 0, 0)
	coast.Cluster, coast.Surface = nil, grade
	b := e.Observe(coast)
	if want := grade.HeightAt(b.X, b.Y); !near(b.Base, want) || !b.OnSurface {
		t.Fatalf("coasting base %.3f, want the surface under the coasted centre %.3f", b.Base, want)
	}
	if b.Height <= boxMinExtentMetres {
		t.Fatalf("coasting height %.3f, want the held belief", b.Height)
	}
}

func TestTrackerStandsThePublishedBoxOnTheSurface(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.BoxModel = groundBoxConfig()
	tracker := NewTracker(cfg)
	start := time.Unix(1_700_000_000, 0)
	psi := 25 * math.Pi / 180
	car := func(i int) WorldCluster {
		d := float64(i) * 0.8
		c := carCluster(int64(i+1), 8+d*math.Cos(psi), -12+d*math.Sin(psi), psi)
		low := lowestReturns[i%len(lowestReturns)]
		c.OBB.CenterZ, c.OBB.Height = float32(low), float32(-0.8-low)
		return c
	}
	// The first frames come before the background has settled: no surface.
	for i := 0; i < 5; i++ {
		tracker.Update([]WorldCluster{car(i)}, start.Add(time.Duration(i)*100*time.Millisecond))
	}
	for _, tr := range tracker.GetActiveTracks() {
		if tr.BoxHeight > 0 && tr.BoxZ != tr.LatestZ {
			t.Fatalf("no surface yet, but the box's base %.2f is not the cluster's %.2f", tr.BoxZ, tr.LatestZ)
		}
	}
	tracker.SetGroundSurface(planeSurface{c: -2.3})
	var last WorldCluster
	for i := 5; i < 25; i++ {
		last = car(i)
		tracker.Update([]WorldCluster{last}, start.Add(time.Duration(i)*100*time.Millisecond))
	}
	tracks := tracker.GetConfirmedTracks()
	if len(tracks) != 1 {
		t.Fatalf("%d confirmed tracks, want 1", len(tracks))
	}
	tr := tracks[0]
	if !near(float64(tr.BoxZ), -2.3) || !near(float64(tr.BoxHeight), 1.5) {
		t.Fatalf("published base %.3f, height %.3f; want the road at -2.3 and the roof 1.5 above it", tr.BoxZ, tr.BoxHeight)
	}
	// The frame's own values stay on the track as diagnostics.
	if tr.LatestZ != last.OBB.CenterZ || tr.OBBHeight != last.OBB.Height {
		t.Fatalf("LatestZ %.2f and OBBHeight %.2f, want the frame's cluster %.2f and %.2f", tr.LatestZ, tr.OBBHeight, last.OBB.CenterZ, last.OBB.Height)
	}
}
