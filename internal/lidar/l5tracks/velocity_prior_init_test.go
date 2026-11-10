package l5tracks

import (
	"math"
	"runtime"
	"testing"
	"time"
)

// Tests for VelocityPriorInit (experiment l5est0009): the birth velocity
// stays zero with variance (7.5 m/s)² per axis. Off, the shipped arithmetic
// must be exactly as it was; on, a vehicle's velocity must converge within a
// few updates rather than about ten.

func TestVelocityPriorInitIsOptIn(t *testing.T) {
	if DefaultTrackerConfig().VelocityPriorInit {
		t.Fatal("VelocityPriorInit must be off by default")
	}
	if VelocityPriorInitVariance != 56.25 {
		t.Fatalf("VelocityPriorInitVariance = %v, want (7.5 m/s)² = 56.25", VelocityPriorInitVariance)
	}
}

// The founding covariance: only the velocity diagonal changes.
func TestVelocityPriorInitFoundingCovariance(t *testing.T) {
	for _, on := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.VelocityPriorInit = on
		tr := NewTracker(cfg)
		now := time.Unix(1_700_000_000, 0)
		tr.Update([]WorldCluster{hygieneCluster(5, 2, now)}, now)
		if len(tr.Tracks) != 1 {
			t.Fatalf("on=%v: %d tracks, want 1", on, len(tr.Tracks))
		}
		for _, track := range tr.Tracks {
			if track.VX != 0 || track.VY != 0 {
				t.Errorf("on=%v: birth velocity (%v, %v), want zero", on, track.VX, track.VY)
			}
			vv := float32(1)
			if on {
				vv = VelocityPriorInitVariance
			}
			want := [16]float32{
				10, 0, 0, 0,
				0, 10, 0, 0,
				0, 0, vv, 0,
				0, 0, 0, vv,
			}
			if track.P != want {
				t.Errorf("on=%v: founding P = %v, want %v", on, track.P, want)
			}
		}
	}
}

// updatesToConverge runs a noise-free vehicle at (vx, vy) and returns the
// first update (counting the founding measurement as the first) after which
// the track's velocity is within 10 % of the truth and stays there.
func updatesToConverge(t *testing.T, cfg TrackerConfig, vx, vy float64) int {
	t.Helper()
	tr := NewTracker(cfg)
	t0 := time.Unix(1_700_000_000, 0)
	speed := math.Hypot(vx, vy)
	converged := 0
	const frames = 30
	for i := 0; i < frames; i++ {
		now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
		x := 5 + vx*0.1*float64(i)
		y := 2 + vy*0.1*float64(i)
		tr.Update([]WorldCluster{hygieneCluster(float32(x), float32(y), now)}, now)
		if tr.TracksCreated != 1 {
			t.Fatalf("frame %d: %d tracks created, want the one vehicle", i, tr.TracksCreated)
		}
		var track *TrackedObject
		for _, tk := range tr.Tracks {
			track = tk
		}
		err := math.Hypot(float64(track.VX)-vx, float64(track.VY)-vy)
		if err <= 0.1*speed {
			if converged == 0 {
				converged = i + 1
			}
		} else {
			converged = 0
		}
	}
	if converged == 0 {
		t.Fatalf("velocity never within 10 %% of %.2f m/s in %d updates", speed, frames)
	}
	return converged
}

func TestVelocityPriorInitConvergesByTheFourthUpdate(t *testing.T) {
	const vx, vy = 6.5, 0
	shipped := updatesToConverge(t, DefaultTrackerConfig(), vx, vy)
	cfg := DefaultTrackerConfig()
	cfg.VelocityPriorInit = true
	prior := updatesToConverge(t, cfg, vx, vy)
	t.Logf("updates until within 10 %% of 6.5 m/s: shipped %d, velocity prior %d", shipped, prior)
	if prior > 4 {
		t.Errorf("velocity prior: within 10 %% only from update %d, want by the fourth", prior)
	}
	if shipped < 8 {
		t.Errorf("shipped prior: within 10 %% from update %d; the slow convergence this option addresses (about ten) has gone", shipped)
	}

	// A diagonal course converges on both axes alike.
	diag := updatesToConverge(t, cfg, 6.5/math.Sqrt2, -6.5/math.Sqrt2)
	if diag > 4 {
		t.Errorf("velocity prior, diagonal course: within 10 %% only from update %d, want by the fourth", diag)
	}
}

// Off, the option leaves every pinned scene's digest at the shipped value:
// the two-point scenes (whose off digests predate this option) and the
// hygiene scene. On, it reaches each scene.
func TestVelocityPriorInitOffKeepsTheShippedArithmetic(t *testing.T) {
	for _, sc := range twoPointDigestScenarios() {
		frames := sc.frames(t)
		cfg := sc.config()
		if cfg.VelocityPriorInit {
			t.Fatalf("%s: the option must be opt-in", sc.name)
		}
		off := associationFingerprint(cfg, frames)
		cfg.VelocityPriorInit = true
		on := associationFingerprint(cfg, frames)
		t.Logf("%s: off=%s on=%s", sc.name, off, on)
		if on == off {
			t.Errorf("%s: the option did not change the scene", sc.name)
		}
		if runtime.GOARCH != "amd64" {
			continue
		}
		if off != sc.off {
			t.Errorf("%s: off digest %s, want the shipped %s", sc.name, off, sc.off)
		}
	}

	off, _ := hygieneScenario(t, DefaultTrackerConfig())
	if runtime.GOARCH == "amd64" && off != hygieneGoldenDefault {
		t.Errorf("hygiene default digest %#x, want the shipped %#x", off, uint64(hygieneGoldenDefault))
	}
	cfg := DefaultTrackerConfig()
	cfg.VelocityPriorInit = true
	if on, _ := hygieneScenario(t, cfg); on == off {
		t.Error("the option did not change the hygiene scene")
	}
}

// With the covariance cap below the prior, the cap applies on the first
// prediction exactly as it does to the shipped prior's growth: the option
// changes the founding covariance and nothing downstream of it.
func TestVelocityPriorInitComposesWithTheCovarianceCap(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.VelocityPriorInit = true
	cfg.MaxCovarianceDiag = 20
	tr := NewTracker(cfg)
	t0 := time.Unix(1_700_000_000, 0)
	for i := 0; i < 2; i++ {
		now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
		tr.Update([]WorldCluster{hygieneCluster(5+0.65*float32(i), 2, now)}, now)
	}
	for _, track := range tr.Tracks {
		for _, d := range []int{0, 5, 10, 15} {
			if track.P[d] > cfg.MaxCovarianceDiag || math.IsNaN(float64(track.P[d])) {
				t.Errorf("P[%d] = %v after an update under a cap of %v", d, track.P[d], cfg.MaxCovarianceDiag)
			}
		}
	}
}
