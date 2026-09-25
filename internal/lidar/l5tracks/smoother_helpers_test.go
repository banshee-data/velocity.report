package l5tracks

import (
	"math"
	"math/rand"
	"testing"
)

// Shared scaffolding for the smoother tests. Every scene here runs the real
// Tracker, so the smoother consumes exactly the priors, posteriors and
// prediction intervals the filter produced, recorded through the same hooks
// the replay harness uses.

// smootherArms fans one tracker's filter record out to several smoothers and
// keeps what each released, per arm, in release order.
type smootherArms struct {
	smoothers []*FixedLagSmoother
	released  [][]SmoothedState
	frames    []FilterFrame
}

func newSmootherArms(t *testing.T, lags ...SmootherLag) *smootherArms {
	t.Helper()
	arms := &smootherArms{released: make([][]SmoothedState, len(lags))}
	for _, lag := range lags {
		s, err := NewFixedLagSmoother(SmootherConfig{Lag: lag})
		if err != nil {
			t.Fatal(err)
		}
		arms.smoothers = append(arms.smoothers, s)
	}
	return arms
}

func (a *smootherArms) ObserveFilterFrame(frame FilterFrame) {
	a.frames = append(a.frames, frame)
	for i, s := range a.smoothers {
		a.released[i] = append(a.released[i], s.Observe(frame)...)
	}
}

func (a *smootherArms) flush() {
	for i, s := range a.smoothers {
		a.released[i] = append(a.released[i], s.Flush()...)
	}
}

// byTrack groups one arm's states per track in capture order.
func byTrack(states []SmoothedState) map[int64][]SmoothedState {
	out := map[int64][]SmoothedState{}
	for _, s := range states {
		out[s.CreationSequence] = append(out[s.CreationSequence], s)
	}
	for seq, list := range out {
		for i := 1; i < len(list); i++ {
			if list[i].StateUnixNanos < list[i-1].StateUnixNanos {
				panic("smoothed states out of capture order")
			}
		}
		out[seq] = list
	}
	return out
}

// scenePath is a road user's true planar position at capture time t seconds.
type scenePath func(t float64) (x, y float64)

// runScene feeds a single object along path, sampled at hz with Gaussian
// position noise of sigma metres (fixed seed), through a tracker with arms
// attached, and flushes the arms at the end. skip, when non-nil, drops the
// cluster on frames it returns true for, so the track coasts there.
//
// The shipped filter initialises velocity at zero with a 1 m/s sigma, and a
// point target at road speed outruns its gate before the filter has learned
// the speed: an initialisation property, not what these scenes measure. So the
// first frame runs unrecorded, the track's velocity is seeded with the truth
// (as the time-domain tests do), and the recorder is attached from the second
// frame, where the chain starts unlinked.
func runScene(t *testing.T, cfg TrackerConfig, path scenePath, hz float64, frames int, sigma float64, seed int64,
	skip func(k int) bool, lags ...SmootherLag) (*Tracker, *smootherArms) {
	t.Helper()
	tk := NewTracker(cfg)
	arms := newSmootherArms(t, lags...)
	rng := rand.New(rand.NewSource(seed))
	for k := 0; k < frames; k++ {
		secs := 1 + float64(k)/hz
		x, y := path(secs - 1)
		var clusters []WorldCluster
		if skip == nil || !skip(k) {
			c := tdCluster(float32(x+sigma*rng.NormFloat64()), float32(y+sigma*rng.NormFloat64()), 0)
			c.ClusterID = int64(k + 1)
			clusters = append(clusters, c)
		}
		tk.Update(clusters, tdAt(secs))
		if k == 0 {
			truth := truthSeries(path, []float64{0})
			for _, track := range tk.Tracks {
				track.VX, track.VY = float32(truth.vx[0]), float32(truth.vy[0])
			}
			tk.SetFilterStepObserver(arms)
		}
	}
	arms.flush()
	return tk, arms
}

// soleTrackStates returns the one track an arm released for a single-object
// scene, failing if association broke it into several.
func soleTrackStates(t *testing.T, states []SmoothedState) []SmoothedState {
	t.Helper()
	tracks := byTrack(states)
	if len(tracks) != 1 {
		t.Fatalf("scene produced %d tracks, want 1: association broke the object, which is not what this test measures", len(tracks))
	}
	for _, list := range tracks {
		return list
	}
	return nil
}

// sceneSecs is a state's time on the scene clock used by runScene's paths.
func sceneSecs(s SmoothedState) float64 {
	return float64(s.StateUnixNanos-tdAt(1).UnixNano()) / 1e9
}

// series extracts an estimate's time series: position and velocity, from
// either the smoothed or the online moments.
type estimateSeries struct {
	t, x, y, vx, vy []float64
}

func seriesOf(states []SmoothedState, smoothed bool) estimateSeries {
	var s estimateSeries
	for _, st := range states {
		m := st.Online
		if smoothed {
			m = st.Smoothed
		}
		s.t = append(s.t, sceneSecs(st))
		s.x = append(s.x, float64(m.X))
		s.y = append(s.y, float64(m.Y))
		s.vx = append(s.vx, float64(m.VX))
		s.vy = append(s.vy, float64(m.VY))
	}
	return s
}

// peakRate returns the largest sign·Δf/Δt over the series for differences
// taken exactly W seconds apart, the explicit bandwidth every rate figure here
// carries (plan §13.2: a peak without a bandwidth is a property of the
// filter), and the midpoint time at which it occurs.
func peakRate(ts, f []float64, window float64, sign float64) (peak, at float64) {
	for i := range ts {
		for j := i + 1; j < len(ts); j++ {
			span := ts[j] - ts[i]
			if span < window-1e-6 {
				continue
			}
			if span > window+1e-6 {
				break
			}
			if rate := sign * (f[j] - f[i]) / span; rate > peak {
				peak, at = rate, 0.5*(ts[i]+ts[j])
			}
			break
		}
	}
	return peak, at
}

func speeds(s estimateSeries) []float64 {
	out := make([]float64, len(s.t))
	for i := range s.t {
		out[i] = math.Hypot(s.vx[i], s.vy[i])
	}
	return out
}

func courses(s estimateSeries) []float64 {
	out := make([]float64, len(s.t))
	for i := range s.t {
		out[i] = math.Atan2(s.vy[i], s.vx[i])
	}
	return out
}

// truthSeries samples a path's true velocity by central difference at the
// estimate's own times, so truth and estimate share a clock and a bandwidth.
func truthSeries(path scenePath, ts []float64) estimateSeries {
	var s estimateSeries
	const h = 1e-4
	for _, t := range ts {
		x, y := path(t)
		x1, y1 := path(t + h)
		x0, y0 := path(t - h)
		s.t = append(s.t, t)
		s.x = append(s.x, x)
		s.y = append(s.y, y)
		s.vx = append(s.vx, (x1-x0)/(2*h))
		s.vy = append(s.vy, (y1-y0)/(2*h))
	}
	return s
}

// brakingPath: 15 m/s for 3 s, then 7 m/s² (0.71 g) to rest, then stationary.
func brakingPath(t float64) (float64, float64) {
	const v0, a, cruise = 15.0, 7.0, 3.0
	stop := v0 / a
	switch {
	case t < cruise:
		return v0 * t, 10
	case t < cruise+stop:
		tb := t - cruise
		return v0*cruise + v0*tb - 0.5*a*tb*tb, 10
	default:
		return v0*cruise + v0*stop - 0.5*a*stop*stop, 10
	}
}

// laneChangePath: 3.5 m lateral in 3 s at 15 m/s, a cycloidal profile whose
// lateral acceleration is one sine period (peak 2.44 m/s²).
func laneChangePath(t float64) (float64, float64) {
	const v, width, start, dur = 15.0, 3.5, 3.0, 3.0
	x := v * t
	switch {
	case t < start:
		return x, 10
	case t < start+dur:
		tau := (t - start) / dur
		return x, 10 + width*(tau-math.Sin(2*math.Pi*tau)/(2*math.Pi))
	default:
		return x, 10 + width
	}
}
