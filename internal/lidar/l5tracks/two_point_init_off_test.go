package l5tracks

import (
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// TwoPointVelocityInit is default-off, and off it must leave the shipped
// arithmetic exactly as it was. The comparability and shadow guards
// (initialiseTwoPoint) read only the founding cluster's extent, which
// initTrack now retains, and other tracks' state, so with the option off
// nothing they add can reach a number the tracker publishes. This test pins
// it: every scene below is hashed frame by frame, association records
// included, with the option off. The off digests were taken on the code
// before the guards existed (origin/main 704b8e46).
//
// The on digests are the option's own. They moved with the guards, by
// design: the guards decline pairs the original option initialised, a
// reflection seen in a nearer body's shadow and a pair whose visible extent
// along the displacement changed. Each on digest below is the guarded
// option's; the one it replaced is in the comment beside it, so the change
// is visible in review. A scene the guards do not reach keeps its digest.
//
// Pinned on amd64 only: the Go compiler fuses x*y+z on arm64 and some other
// targets, so the same source gives other bits there.

// ghostScene is the defect l5est0006 root-caused on kirk0 and ashbury-downey,
// at its simplest. A car passes the sensor at 8 m/s; a thin, sparse cluster
// sits on the car's bearing at 2.2 times its range, as a reflection of the
// car does, so it moves at 2.2 times the car's speed, 17.6 m/s, for ten
// frames and then vanishes. A second vehicle, at 6 m/s, is first seen as a
// 0.6 m fragment at its rear and from the next frame whole, so the pair's
// reference point slides 2 m forward on top of the vehicle's 0.6 m.
func ghostScene() []syntheticPassFrame {
	start := time.Unix(1_700_000_000, 0)
	const ghostScale = 2.2
	dirX, dirY := float32(-0.55), float32(-0.835)
	heading := float32(math.Atan2(float64(dirY), float64(dirX)))
	var frames []syntheticPassFrame
	for i := 0; i < 40; i++ {
		ts := float32(i) * 0.1
		at := start.Add(time.Duration(i) * 100 * time.Millisecond)
		var cs []WorldCluster
		id := int64(i * 10)
		add := func(c WorldCluster) {
			id++
			c.ClusterID = id
			c.TSUnixNanos = at.UnixNano()
			cs = append(cs, c)
		}
		carX, carY := -1+8*ts*dirX, 12+8*ts*dirY
		add(ghostSceneCluster(carX, carY, 4.6, 1.9, heading, 900))
		if i >= 5 && i < 15 {
			add(ghostSceneCluster(ghostScale*carX, ghostScale*carY, 1.5, 0.3, heading, 18))
		}
		if i >= 2 {
			// The second vehicle travels along +X at y = -8; its first
			// cluster is the rear 0.6 m of a 4.6 m body.
			centreX := -20 + 6*ts
			if i == 2 {
				add(ghostSceneCluster(centreX-2, -8, 0.6, 1.6, 0, 60))
			} else {
				add(ghostSceneCluster(centreX, -8, 4.6, 1.8, 0, 700))
			}
		}
		frames = append(frames, syntheticPassFrame{at: at, clusters: cs})
	}
	return frames
}

func ghostSceneCluster(x, y, length, width, heading float32, points int) WorldCluster {
	return WorldCluster{
		CentroidX: x, CentroidY: y, CentroidZ: 0.8, SensorID: "test",
		BoundingBoxLength: length, BoundingBoxWidth: width, BoundingBoxHeight: 1.4,
		PointsCount: points,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX: x, CenterY: y, CenterZ: 0.8, Length: length, Width: width, Height: 1.4, HeadingRad: heading,
		},
	}
}

// hygieneFrames is hygieneScenario's two-vehicle scene as frames.
func hygieneFrames() []syntheticPassFrame {
	t0 := time.Unix(1_700_000_000, 0)
	var frames []syntheticPassFrame
	for i := 0; i < 60; i++ {
		now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
		var clusters []WorldCluster
		if i < 30 || i > 51 {
			clusters = append(clusters, hygieneCluster(5+6.5*0.1*float32(i), 2, now))
		}
		clusters = append(clusters, hygieneCluster(acceleratingX(0.1*float32(i)), -10, now))
		frames = append(frames, syntheticPassFrame{at: now, clusters: clusters})
	}
	return frames
}

type twoPointDigestScenario struct {
	name    string
	frames  func(t *testing.T) []syntheticPassFrame
	config  func() TrackerConfig
	off, on string
}

func twoPointDigestScenarios() []twoPointDigestScenario {
	scene := func(f func() []syntheticPassFrame) func(*testing.T) []syntheticPassFrame {
		return func(*testing.T) []syntheticPassFrame { return f() }
	}
	pass := func(p func() l4perception.SyntheticPass) func(t *testing.T) []syntheticPassFrame {
		return func(t *testing.T) []syntheticPassFrame { return syntheticPassFrames(t, p()) }
	}
	coupled := func() TrackerConfig {
		cfg := DefaultTrackerConfig()
		cfg.CoupledProcessNoise = true
		cfg.JosephCovarianceUpdate = true
		return cfg
	}
	adaptive := func() TrackerConfig {
		cfg := DefaultTrackerConfig()
		cfg.AdaptiveMeasurementNoise = true
		return cfg
	}
	return []twoPointDigestScenario{
		// Unguarded on: dd2cd9d40d0cd9da, 31ffaa36219da742, 6f37c542db616e21.
		// The guards decline the reflection (shadow) and the fragment-born
		// pair (slack above the cap).
		{"ghost_default", scene(ghostScene), DefaultTrackerConfig, "d09fb7f2b4f767fe", "f8ca06c4ee93852e"},
		{"ghost_coupled_joseph", scene(ghostScene), coupled, "cc2d76248066403a", "0c97c050d608f982"},
		{"ghost_adaptive", scene(ghostScene), adaptive, "25fcb03bcd2e746b", "fe255392a0e28399"},
		// Not reached by the guards: the on digests did not move.
		{"association_default", scene(associationScene), DefaultTrackerConfig, "b305f4eafd3428db", "3e279dd9cf2ea3bc"},
		{"hygiene_default", scene(hygieneFrames), DefaultTrackerConfig, "e859d2002eb60fdf", "6c7276fe42ebe1a2"},
		// Unguarded on: fd8171208eb422e5 and 8c651045b8cb9596. The DBSCAN
		// clusters of a passing body change extent from frame to frame, so
		// the slack widens the velocity block: that alone moves the truck's
		// digest. The default pass also moves with the shadow guard alone
		// disabled, so both guards reach it.
		{"default_pass", pass(l4perception.DefaultSyntheticPass), DefaultTrackerConfig, "3d00f68b3514a436", "f522a097b8a39ff3"},
		{"end_on_truck", pass(endOnTruckPass), DefaultTrackerConfig, "8962df4f854023c0", "99250cffc4accb64"},
	}
}

func TestTwoPointVelocityInitOffKeepsTheShippedArithmetic(t *testing.T) {
	for _, sc := range twoPointDigestScenarios() {
		frames := sc.frames(t)
		cfg := sc.config()
		if cfg.TwoPointVelocityInit {
			t.Fatalf("%s: the option must be opt-in", sc.name)
		}
		off := associationFingerprint(cfg, frames)
		cfg.TwoPointVelocityInit = true
		on := associationFingerprint(cfg, frames)
		t.Logf("%s: off=%s on=%s", sc.name, off, on)
		if runtime.GOARCH != "amd64" {
			continue
		}
		if off != sc.off {
			t.Errorf("%s: off digest %s, want the shipped %s", sc.name, off, sc.off)
		}
		if on != sc.on {
			t.Errorf("%s: on digest %s, want %s", sc.name, on, sc.on)
		}
	}
}
