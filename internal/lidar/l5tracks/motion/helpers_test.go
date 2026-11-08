package motion

import (
	"math"
	"testing"
)

const frameNanos = int64(100_000_000) // 10 Hz

// trajectory gives the true along-x position and velocity at t seconds.
type trajectory func(t float64) (x, v float64)

func constantVelocity(v0 float64) trajectory {
	return func(t float64) (float64, float64) { return v0 * t, v0 }
}

func constantAcceleration(v0, a float64) trajectory {
	return func(t float64) (float64, float64) { return v0*t + 0.5*a*t*t, v0 + a*t }
}

// brakingToStop decelerates from v0 at a (> 0) to rest and holds there.
func brakingToStop(v0, a float64) trajectory {
	stop := v0 / a
	return func(t float64) (float64, float64) {
		if t >= stop {
			return v0 * stop / 2, 0
		}
		return v0*t - 0.5*a*t*t, v0 - a*t
	}
}

// observe samples a trajectory noise-free at 10 Hz on [0, endSecs] along x,
// omitting samples strictly inside (gapFrom, gapTo).
func observe(tr trajectory, endSecs, gapFrom, gapTo float64) []Observation {
	var out []Observation
	n := int(math.Round(endSecs * 10))
	for k := 0; k <= n; k++ {
		t := float64(k) / 10
		if t > gapFrom+1e-9 && t < gapTo-1e-9 {
			continue
		}
		x, _ := tr(t)
		out = append(out, Observation{UnixNanos: int64(k) * frameNanos, X: x, Y: 0, R: IsotropicR(DefaultMeasurementVariance)})
	}
	return out
}

func mustRun(t *testing.T, cfg FilterConfig, obs []Observation) []Step {
	t.Helper()
	steps, err := RunFilter(cfg, obs)
	if err != nil {
		t.Fatalf("RunFilter: %v", err)
	}
	return steps
}

func mustRunIMM(t *testing.T, cfg IMMConfig, obs []Observation) []IMMStep {
	t.Helper()
	steps, err := RunIMM(cfg, obs)
	if err != nil {
		t.Fatalf("RunIMM: %v", err)
	}
	return steps
}

// shippedCV is the live tracker's filter at its shipped tuning in this
// space: diagonal Q, process_noise_pos 0.05, process_noise_vel 0.2.
func shippedCV() FilterConfig {
	cfg := DefaultCVConfig()
	cfg.Model.Diagonal = true
	cfg.Init.VelVar = 1
	return cfg
}

// noCoast is CA with every part of the coast rule switched off.
func noCoast(cfg FilterConfig) FilterConfig {
	cfg.Coast = CoastRule{}
	return cfg
}

// gapStepIndex returns the index of the first step whose prediction spanned
// more than one frame.
func gapStepIndex(t *testing.T, steps []Step) int {
	t.Helper()
	for i, s := range steps {
		if s.DtSecs > 0.15 {
			return i
		}
	}
	t.Fatal("no gap in steps")
	return -1
}
