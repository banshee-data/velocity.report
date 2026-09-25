package l5tracks

import (
	"math"
	"testing"
)

// Gap V1 in data/maths/paper-implementation-gap-analysis.md: nothing checked
// that a coasting constant-velocity prediction's error grows the way the model
// says it must against a target that is not at constant velocity. For a
// target accelerating at a, the CV prediction error after t seconds is
// exactly a t^2 / 2, so error / t^2 is a constant, and the covariance the
// filter carries for that prediction must grow monotonically over the coast.
func TestCoastErrorGrowsQuadraticallyAgainstAnAcceleratingTarget(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	tr := &TrackedObject{TrackID: "coast", VX: 10}

	const (
		accel = 2.0 // m/s^2, the target's true acceleration
		step  = 0.1 // s, one sensor period, well under MaxPredictDt
	)
	horizons := []int{10, 30, 50, 100} // steps: 1, 3, 5 and 10 seconds

	var (
		steps    int
		lastPxx  float32
		ratioAt1 float64
	)
	for _, h := range horizons {
		for steps < h {
			tk.predict(tr, step)
			steps++
		}
		elapsed := float64(steps) * step
		truth := 10*elapsed + 0.5*accel*elapsed*elapsed
		err := math.Abs(float64(tr.X) - truth)
		ratio := err / (elapsed * elapsed)
		if ratioAt1 == 0 {
			ratioAt1 = ratio
		}
		// a/2 = 1.0 for accel 2; allow float32 accumulation over 100 steps.
		if math.Abs(ratio-accel/2) > 0.02 {
			t.Fatalf("after %.0f s the CV error is %.3f m, error/t^2 = %.4f, want %.2f (quadratic growth)",
				elapsed, err, ratio, accel/2)
		}
		if math.Abs(ratio-ratioAt1) > 0.02 {
			t.Fatalf("error/t^2 drifted from %.4f at 1 s to %.4f at %.0f s", ratioAt1, ratio, elapsed)
		}
		if tr.P[0] <= lastPxx {
			t.Fatalf("position variance did not grow over the coast: %.4f after %.0f s, was %.4f",
				tr.P[0], elapsed, lastPxx)
		}
		lastPxx = tr.P[0]
	}
}
