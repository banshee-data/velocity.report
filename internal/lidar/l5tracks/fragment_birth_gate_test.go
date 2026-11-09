package l5tracks

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// fragCluster is a 0.3 m scrap at (x, y), under the fragment guard's
// half-metre extent: a piece of a body the guard refuses a metre-scale track.
func fragCluster(id int64, x, y float64) WorldCluster {
	pts := visibleFaces(x, y, 0, 0.3, 0.2, 20)
	obb := l4perception.EstimateOBBFromCluster(pts)
	cx, cy := medoid(pts)
	return WorldCluster{ClusterID: id, SensorID: "box-test", CentroidX: float32(cx), CentroidY: float32(cy),
		BoundingBoxLength: obb.Length, BoundingBoxWidth: obb.Width, BoundingBoxHeight: 0.4,
		PointsCount: len(pts), OBB: &obb, Members: pts}
}

func fragmentBirthGateConfig(on bool) TrackerConfig {
	cfg := DefaultTrackerConfig()
	cfg.RobustLikelihoodAssociationCost = true
	cfg.FragmentBirthGate = on
	return cfg
}

// runFragment drives a car on cluster 1 along x at 5 m/s, observed on frames
// before carUntil and coasting after, and from frame from a scrap on
// cluster 2 placed by at(i, car) relative to the car's track as the previous
// frame left it. It returns the tracker and how many tracks the scrap seeded.
func runFragment(t *testing.T, cfg TrackerConfig, from, carUntil, end int, at func(i int, car *TrackedObject) (float64, float64)) (*Tracker, int) {
	t.Helper()
	tracker := NewTracker(cfg)
	start := time.Unix(1_700_000_000, 0)
	var car *TrackedObject
	seeded := map[string]bool{}
	for i := 0; i <= end; i++ {
		var clusters []WorldCluster
		if i < carUntil {
			clusters = append(clusters, carCluster(1, 10+0.5*float64(i), 8, 0))
		}
		if i >= from && car != nil {
			x, y := at(i, car)
			clusters = append(clusters, fragCluster(2, x, y))
		}
		tracker.Update(clusters, start.Add(time.Duration(i)*100*time.Millisecond))
		for id, tr := range tracker.Tracks {
			if tr.LastClusterID == 1 && car == nil {
				car = tr
			}
			if tr.LastClusterID == 2 && tr.ObservationCount > 0 {
				seeded[id] = true
			}
		}
	}
	return tracker, len(seeded)
}

// onBody puts the scrap at the car track's predicted centre, offset along x.
func onBody(along, lateral float64) func(int, *TrackedObject) (float64, float64) {
	return func(_ int, car *TrackedObject) (float64, float64) {
		return float64(car.X+car.VX*0.1) + along, float64(car.Y) + lateral
	}
}

func TestFragmentBirthGateAbsorbsAScrapInsideAConfirmedBody(t *testing.T) {
	// Without the gate, the guard refuses the car the scrap and it seeds a
	// track of its own: a second track on one body.
	_, seeded := runFragment(t, fragmentBirthGateConfig(false), 8, 30, 20, onBody(0.8, 0))
	if seeded == 0 {
		t.Fatal("setup: without the gate the scrap seeded no track")
	}
	tracker, seeded := runFragment(t, fragmentBirthGateConfig(true), 8, 30, 20, onBody(0.8, 0))
	if seeded != 0 {
		t.Fatalf("the gate let a scrap inside a confirmed body seed %d tracks", seeded)
	}
	if got := tracker.ContinuityStats().FragmentBirthsAbsorbed; got != 13 {
		t.Errorf("%d births absorbed, want the scrap's 13 frames", got)
	}
	if got := len(tracker.GetConfirmedTracks()); got != 1 {
		t.Errorf("%d confirmed tracks, want the car's alone", got)
	}
}

func TestFragmentBirthGateSparesAScrapBesideTheBody(t *testing.T) {
	// Three metres to the side is beyond the car's 1.8 m box, the quarter
	// metre and a converged track's position sigma: a small road user, born.
	tracker, seeded := runFragment(t, fragmentBirthGateConfig(true), 8, 30, 20, onBody(0, 3))
	if seeded != 1 {
		t.Fatalf("a scrap beside the car seeded %d tracks, want 1", seeded)
	}
	if got := tracker.ContinuityStats().FragmentBirthsAbsorbed; got != 0 {
		t.Errorf("%d births absorbed beside the body", got)
	}
}

func TestFragmentBirthGateNeedsTheFragmentGuard(t *testing.T) {
	// With the guard off nothing is a fragment, so the gate changes nothing.
	off := fragmentBirthGateConfig(false)
	off.MinAssociableExtentMetres = 0
	on := off
	on.FragmentBirthGate = true
	frames := fragmentFrames()
	a := associationFingerprint(off, frames)
	b := associationFingerprint(on, frames)
	if a != b {
		t.Fatalf("with the fragment guard off the gate changed the tracker: %s against %s", b, a)
	}
	// With the guard on, the same frames do change, so the comparison above
	// is not vacuous.
	guarded := fragmentBirthGateConfig(true)
	if associationFingerprint(guarded, frames) == associationFingerprint(fragmentBirthGateConfig(false), frames) {
		t.Fatal("setup: the gate changes nothing on these frames even with the guard on")
	}
}

// fragmentFrames is a car with a scrap inside its body from frame 8, as
// frames for a digest.
func fragmentFrames() []syntheticPassFrame {
	start := time.Unix(1_700_000_000, 0)
	var frames []syntheticPassFrame
	for i := 0; i <= 20; i++ {
		x := 10 + 0.5*float64(i)
		cs := []WorldCluster{carCluster(1, x, 8, 0)}
		if i >= 8 {
			cs = append(cs, fragCluster(2, x+0.8, 8))
		}
		frames = append(frames, syntheticPassFrame{at: start.Add(time.Duration(i) * 100 * time.Millisecond), clusters: cs})
	}
	return frames
}

func TestFragmentBirthGateBoundsACoastingTracksGrowth(t *testing.T) {
	// The car is lost after frame 12 and coasts; its position sigma grows
	// past the one-metre bound. A scrap at its prediction is still absorbed;
	// one a metre and a half beyond the bounded box's end is born.
	tracker, seeded := runFragment(t, fragmentBirthGateConfig(true), 18, 13, 24, onBody(0, 0))
	if seeded != 0 {
		t.Fatalf("a scrap at a coasting car's prediction seeded %d tracks", seeded)
	}
	var car *TrackedObject
	for _, tr := range tracker.Tracks {
		if tr.LastClusterID == 1 {
			car = tr
		}
	}
	if car == nil || car.TrackState != TrackConfirmed || car.Misses < 5 {
		t.Fatalf("setup: the car is not a coasting confirmed track: %+v", car)
	}
	if s := math.Sqrt(float64(car.P[0])); s <= fragmentBirthMaxSigmaMetres {
		t.Fatalf("setup: the coasting sigma %.2f m is inside the bound; the test does not reach it", s)
	}
	beyond := 4.5/2 + overlapEndBodyMargin + fragmentBirthMaxSigmaMetres + 1.5
	_, seeded = runFragment(t, fragmentBirthGateConfig(true), 18, 13, 24, onBody(beyond, 0))
	if seeded != 1 {
		t.Fatalf("a scrap beyond the bounded box seeded %d tracks, want 1", seeded)
	}
}

func TestFragmentBirthGateDefaultsOff(t *testing.T) {
	if DefaultTrackerConfig().FragmentBirthGate {
		t.Fatal("FragmentBirthGate defaults on; it must be measured against the robust cost alone first")
	}
}
