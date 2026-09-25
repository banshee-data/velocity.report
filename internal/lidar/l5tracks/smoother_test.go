package l5tracks

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// The smoother's claims, each pinned where it can break:
//
//   - it computes what RTS says (against an independent batch solve);
//   - fixed-lag converges to full-track RTS in a track's interior, and equals
//     it exactly over the tail;
//   - it does not flatten manoeuvres at the comparator horizon, and the
//     flattening at longer horizons belongs to the process-noise model;
//   - an impact stays identifiable in the revision audit and distinguishable
//     from a measurement anomaly (plan §12, Phase 8 acceptance);
//   - memory is bounded, output is deterministic, coasted states stay
//     unobserved, and no state moves without evidence.

var comparatorLags = []SmootherLag{LagFrames(3), LagSeconds(0.5), LagSeconds(1), LagSeconds(2), LagTrackEnd()}

func straightPath(t float64) (float64, float64) { return 12 * t, 10 + 0.4*t }

// gentleCurve changes course slowly enough that no cap or clamp engages, so
// the filter is exactly the linear-Gaussian model the batch solve assumes.
func gentleCurve(t float64) (float64, float64) {
	return 10*t + 0.2*t*t, 5 + 3*math.Sin(0.4*t)
}

// solveDense solves A x = b for a symmetric positive-definite A by Cholesky.
func solveDense(a [][]float64, b []float64) []float64 {
	n := len(b)
	l := make([][]float64, n)
	for i := range l {
		l[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum := a[i][j]
			for k := 0; k < j; k++ {
				sum -= l[i][k] * l[j][k]
			}
			if i == j {
				l[i][i] = math.Sqrt(sum)
			} else {
				l[i][j] = sum / l[j][j]
			}
		}
	}
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := b[i]
		for k := 0; k < i; k++ {
			sum -= l[i][k] * y[k]
		}
		y[i] = sum / l[i][i]
	}
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := y[i]
		for k := i + 1; k < n; k++ {
			sum -= l[k][i] * x[k]
		}
		x[i] = sum / l[i][i]
	}
	return x
}

func invert4(m [16]float64) [16]float64 {
	l, ok := cholesky4(m)
	if !ok {
		panic("not positive definite")
	}
	var identity [16]float64
	for d := 0; d < 4; d++ {
		identity[d*4+d] = 1
	}
	return choleskySolve4(l, identity)
}

// TestFullTrackSmootherMatchesBatchLeastSquares checks the backward pass
// against the answer RTS is a recursion for: the joint maximum a posteriori
// trajectory of the linear-Gaussian model, solved here as one dense system
// from the raw measurements, the configured noise and the recorded prediction
// intervals. None of the filter's own recursion is reused, so agreement means
// the smoother computed the smoothing problem, not merely its own algebra.
func TestFullTrackSmootherMatchesBatchLeastSquares(t *testing.T) {
	cfg := DefaultTrackerConfig()
	_, arms := runScene(t, cfg, gentleCurve, 10, 40, 0.15, 7, nil, LagTrackEnd())
	states := soleTrackStates(t, arms.released[0])

	// The chain the smoother saw, step by step.
	var steps []FilterStep
	for _, frame := range arms.frames {
		steps = append(steps, frame.Steps...)
	}
	if len(steps) != len(states) {
		t.Fatalf("%d steps recorded, %d states released", len(steps), len(states))
	}
	n := len(steps)
	dim := 4 * n
	a := make([][]float64, dim)
	for i := range a {
		a[i] = make([]float64, dim)
	}
	b := make([]float64, dim)
	add := func(r, c int, m [16]float64) {
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				a[4*r+i][4*c+j] += m[i*4+j]
			}
		}
	}

	// Prior: the first step's posterior, which already holds all evidence
	// before the recorder was attached.
	p0 := invert4(symmetric64(steps[0].Posterior.P))
	add(0, 0, p0)
	m0 := vector64(steps[0].Posterior)
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			b[i] += p0[i*4+j] * m0[j]
		}
	}
	r := float64(cfg.MeasurementNoise)
	for k := 1; k < n; k++ {
		tau := float64(steps[k].PredictedSecs)
		if !steps[k].Observed || !steps[k].Linked {
			t.Fatalf("step %d: the scene must be fully observed and linked", k)
		}
		var q [16]float64 // the shipped diagonal process noise, per step
		q[0], q[5] = float64(cfg.ProcessNoisePos)*tau, float64(cfg.ProcessNoisePos)*tau
		q[10], q[15] = float64(cfg.ProcessNoiseVel)*tau, float64(cfg.ProcessNoiseVel)*tau
		qi := invert4(q)
		var f [16]float64
		for d := 0; d < 4; d++ {
			f[d*4+d] = 1
		}
		f[0*4+2], f[1*4+3] = tau, tau
		ft := transpose64(f)
		add(k, k, qi)
		add(k-1, k-1, multiply64(multiply64(ft, qi), f))
		negQF := multiply64(qi, f)
		negFtQ := multiply64(ft, qi)
		for e := range negQF {
			negQF[e], negFtQ[e] = -negQF[e], -negFtQ[e]
		}
		add(k, k-1, negQF)
		add(k-1, k, negFtQ)
		a[4*k][4*k] += 1 / r
		a[4*k+1][4*k+1] += 1 / r
		b[4*k] += float64(steps[k].Observation.X) / r
		b[4*k+1] += float64(steps[k].Observation.Y) / r
	}
	batch := solveDense(a, b)

	worstPos, worstVel := 0.0, 0.0
	for k, s := range states {
		dp := math.Hypot(float64(s.Smoothed.X)-batch[4*k], float64(s.Smoothed.Y)-batch[4*k+1])
		dv := math.Hypot(float64(s.Smoothed.VX)-batch[4*k+2], float64(s.Smoothed.VY)-batch[4*k+3])
		worstPos, worstVel = math.Max(worstPos, dp), math.Max(worstVel, dv)
		if s.Stage != RefinementFinal || s.Release != ReleaseChainEnd {
			t.Fatalf("state %d: stage %s released by %s, want final at chain end", k, s.Stage, s.Release)
		}
	}
	// The filter stores float32; the batch solve is float64 throughout.
	if worstPos > 2e-4 || worstVel > 2e-4 {
		t.Fatalf("smoothed trajectory differs from the batch solution by %.2e m and %.2e m/s", worstPos, worstVel)
	}

	// The smoothed covariance is the batch posterior covariance: the diagonal
	// block of the inverse information matrix, checked mid-track.
	mid := n / 2
	for d := 0; d < 4; d++ {
		e := make([]float64, dim)
		e[4*mid+d] = 1
		col := solveDense(a, e)
		want := col[4*mid+d]
		got := float64(states[mid].Smoothed.P[d*4+d])
		if math.Abs(got-want) > 1e-3*want {
			t.Fatalf("smoothed variance [%d] = %.6g, batch posterior %.6g", d, got, want)
		}
	}
	t.Logf("%d states: max |smoothed - batch| %.2e m, %.2e m/s", n, worstPos, worstVel)
}

// TestFixedLagConvergesToFullTrackInTheInterior: a fixed-lag estimate is the
// full-track estimate with the future beyond its lag withheld. In a track's
// interior the difference must shrink as the lag grows, and over the tail,
// where the track ended inside the window, the two are the same computation
// and must agree to the bit.
func TestFixedLagConvergesToFullTrackInTheInterior(t *testing.T) {
	_, arms := runScene(t, DefaultTrackerConfig(), gentleCurve, 10, 150, 0.15, 11, nil, comparatorLags...)
	final := soleTrackStates(t, arms.released[len(comparatorLags)-1])
	n := len(final)
	tailStart := func(lag SmootherLag) int { // first state whose lag the track end cut short
		for k, s := range final {
			if lag.Frames > 0 && n-1-k < lag.Frames {
				return k
			}
			if lag.Secs > 0 && float64(final[n-1].StateUnixNanos-s.StateUnixNanos)/1e9 < lag.Secs {
				return k
			}
		}
		return n
	}
	previous := math.Inf(1)
	for i, lag := range comparatorLags[:len(comparatorLags)-1] {
		fixed := soleTrackStates(t, arms.released[i])
		if len(fixed) != n {
			t.Fatalf("%s released %d states, full-track %d: every state is released exactly once per arm", lag, len(fixed), n)
		}
		tail := tailStart(lag)
		worst := 0.0
		for k := 0; k < n; k++ {
			a, b := fixed[k], final[k]
			if a.StateUnixNanos != b.StateUnixNanos || a.Stage != RefinementFixedLag {
				t.Fatalf("%s state %d misaligned or mis-staged: %s", lag, k, a.Stage)
			}
			d := math.Hypot(float64(a.Smoothed.X-b.Smoothed.X), float64(a.Smoothed.Y-b.Smoothed.Y))
			if k >= tail {
				if a.Smoothed != b.Smoothed || a.Release != ReleaseChainEnd || !a.LookaheadTruncated {
					t.Fatalf("%s tail state %d: %+v differs from full-track %+v (release %s)", lag, k, a.Smoothed, b.Smoothed, a.Release)
				}
				continue
			}
			if a.Release != ReleaseLag || a.LookaheadTruncated {
				t.Fatalf("%s interior state %d released by %s", lag, k, a.Release)
			}
			worst = math.Max(worst, d)
		}
		t.Logf("lag %-4s: interior max |fixed-lag - full-track| = %.4f m over %d states", lag, worst, tail)
		if worst > previous+1e-9 {
			t.Fatalf("lag %s is further from full-track (%.4f m) than a shorter lag (%.4f m)", lag, worst, previous)
		}
		previous = worst
	}
	if previous > 0.01 {
		t.Fatalf("a 2 s lag is still %.4f m from full-track in the interior; the gain has not decayed", previous)
	}
}

// Manoeuvre preservation. A smoother that lowers residuals by flattening a
// braking event has failed (plan §10, §12.1, G-SMO-1 criterion 3), so the
// peak rates are measured with a stated bandwidth: differences exactly 0.5 s
// apart (five 10 Hz frames). Truth is the scene's own analytic path.
const manoeuvreBandwidthSecs = 0.5

type manoeuvreFigures struct {
	lag                   SmootherLag
	truth, online, smooth float64 // peak rate at the bandwidth
	magnitude             float64 // smoothed total change: Δspeed or lateral offset
}

func measureBraking(t *testing.T, cfg TrackerConfig) []manoeuvreFigures {
	_, arms := runScene(t, cfg, brakingPath, 10, 80, 0.1, 1, nil, comparatorLags...)
	var out []manoeuvreFigures
	for i, lag := range comparatorLags {
		st := soleTrackStates(t, arms.released[i])
		on, sm := seriesOf(st, false), seriesOf(st, true)
		tr := truthSeries(brakingPath, on.t)
		f := manoeuvreFigures{lag: lag}
		f.truth, _ = peakRate(tr.t, speeds(tr), manoeuvreBandwidthSecs, -1)
		f.online, _ = peakRate(on.t, speeds(on), manoeuvreBandwidthSecs, -1)
		f.smooth, _ = peakRate(sm.t, speeds(sm), manoeuvreBandwidthSecs, -1)
		sp := speeds(sm)
		f.magnitude = sp[0] - sp[len(sp)-1]
		out = append(out, f)
	}
	return out
}

func measureLaneChange(t *testing.T, cfg TrackerConfig) []manoeuvreFigures {
	_, arms := runScene(t, cfg, laneChangePath, 10, 100, 0.1, 2, nil, comparatorLags...)
	var out []manoeuvreFigures
	for i, lag := range comparatorLags {
		st := soleTrackStates(t, arms.released[i])
		on, sm := seriesOf(st, false), seriesOf(st, true)
		tr := truthSeries(laneChangePath, on.t)
		f := manoeuvreFigures{lag: lag}
		f.truth, _ = peakRate(tr.t, courses(tr), manoeuvreBandwidthSecs, 1)
		f.online, _ = peakRate(on.t, courses(on), manoeuvreBandwidthSecs, 1)
		f.smooth, _ = peakRate(sm.t, courses(sm), manoeuvreBandwidthSecs, 1)
		f.magnitude = sm.y[len(sm.y)-1] - sm.y[0]
		out = append(out, f)
	}
	return out
}

// TestComparatorHorizonPreservesManoeuvres: at the plan's comparator horizons
// (three frames, 0.5 s) and the shipped process noise, hard braking (15 m/s
// to rest at 7 m/s²) keeps at least 85% of its true peak deceleration, the
// lane change (3.5 m in 3 s at 15 m/s) keeps at least 90% of the peak course
// rate the online filter already showed, and both keep their total change:
// Δspeed within 2% and the lateral offset within 5 cm, at every horizon.
func TestComparatorHorizonPreservesManoeuvres(t *testing.T) {
	cfg := DefaultTrackerConfig()
	for _, f := range measureBraking(t, cfg) {
		t.Logf("braking   %-5s peak decel truth %.2f online %.2f smoothed %.2f m/s² (%.0f%% of truth); Δspeed %.2f m/s",
			f.lag, f.truth, f.online, f.smooth, 100*f.smooth/f.truth, f.magnitude)
		if math.Abs(f.magnitude-15) > 0.3 {
			t.Errorf("%s: braking Δspeed %.2f m/s, want 15 within 2%%", f.lag, f.magnitude)
		}
		if comparator(f.lag) && f.smooth < 0.85*f.truth {
			t.Errorf("%s: peak deceleration %.2f is below 85%% of the true %.2f", f.lag, f.smooth, f.truth)
		}
	}
	for _, f := range measureLaneChange(t, cfg) {
		t.Logf("lane change %-5s peak course rate truth %.3f online %.3f smoothed %.3f rad/s (%.0f%% of online); offset %.3f m",
			f.lag, f.truth, f.online, f.smooth, 100*f.smooth/f.online, f.magnitude)
		if math.Abs(f.magnitude-3.5) > 0.05 {
			t.Errorf("%s: lateral offset %.3f m, want 3.5 within 5 cm", f.lag, f.magnitude)
		}
		if comparator(f.lag) && f.smooth < 0.9*f.online {
			t.Errorf("%s: peak course rate %.3f is below 90%% of the online %.3f: the smoother flattened it", f.lag, f.smooth, f.online)
		}
	}
}

func comparator(lag SmootherLag) bool { return lag.Frames == 3 || lag.Secs == 0.5 }

// TestLongHorizonFlatteningBelongsToTheProcessNoise pins a measured finding
// that the horizon choice and G-SMO-1 depend on. RTS returns the most
// probable path under the filter's own model, and at the shipped process
// noise (0.2 m²/s³) that model finds a lane change improbable: the 2 s and
// full-track estimates keep under 85% of the peak course rate the online
// filter showed, which is itself under 85% of the truth. That fails G-SMO-1
// criterion 3 at those horizons. At a manoeuvre-scale noise (3 m²/s³) every
// horizon keeps at least 85% of the truth for both manoeuvres, so the
// flattening is the model's, not the smoother's. If this test starts failing,
// the finding has changed: update the criteria document before anything else.
func TestLongHorizonFlatteningBelongsToTheProcessNoise(t *testing.T) {
	shipped := DefaultTrackerConfig()
	for _, f := range measureLaneChange(t, shipped) {
		if f.lag.IsTrackEnd() || f.lag.Secs == 2 {
			if f.smooth >= 0.85*f.online {
				t.Errorf("%s at shipped noise kept %.0f%% of the online course rate; the documented flattening is gone",
					f.lag, 100*f.smooth/f.online)
			}
		}
		if f.online >= 0.85*f.truth {
			t.Errorf("online filter at shipped noise kept %.0f%% of the true course rate; the documented shortfall is gone",
				100*f.online/f.truth)
		}
	}
	manoeuvreScale := DefaultTrackerConfig()
	manoeuvreScale.ProcessNoiseVel = 3
	for _, f := range measureLaneChange(t, manoeuvreScale) {
		t.Logf("q=3 lane change %-5s %.0f%% of true peak course rate", f.lag, 100*f.smooth/f.truth)
		if f.smooth < 0.85*f.truth {
			t.Errorf("%s at q=3 kept %.0f%% of the true peak course rate", f.lag, 100*f.smooth/f.truth)
		}
	}
	for _, f := range measureBraking(t, manoeuvreScale) {
		t.Logf("q=3 braking     %-5s %.0f%% of true peak deceleration", f.lag, 100*f.smooth/f.truth)
		if f.smooth < 0.85*f.truth {
			t.Errorf("%s at q=3 kept %.0f%% of the true peak deceleration", f.lag, 100*f.smooth/f.truth)
		}
	}
}

// Phase 8 acceptance, from the smoother's side (plan §12): given a track with
// an injected single-frame impact, the stored record must let a person find
// the frame of impact and tell it from a measurement anomaly. The procedure
// below reads only what a SmoothedState stores: each state's own observation
// and innovation, its revision, and the evidence that justified it.

// impactPath: 15 m/s along X, then at 3.0 s an instantaneous change to
// (9, 3) m/s, a 6.7 m/s deflecting impact. At about 10 m/s of Δv the shipped
// association gate loses the object and a new track begins: that impact is an
// identity break, which a fixed-assignment smoother cannot see across, and it
// belongs to the reassociation work, not to this test.
func impactPath(t float64) (float64, float64) {
	const impact = 3.0
	if t < impact {
		return 15 * t, 10
	}
	return 15*impact + 9*(t-impact), 10 + 3*(t-impact)
}

// nisGate99 is chi-squared with two degrees of freedom at 0.99: the plan's
// per-frame anomaly threshold (§12.1, invariant 2).
const nisGate99 = 9.21

// onsetIndex is the first observed state whose innovation exceeds the NIS
// gate: where the stored record first says something surprising happened.
func onsetIndex(states []SmoothedState) int {
	for i, s := range states {
		if s.Observed && s.Observation.HasInnovation && s.Observation.NIS > nisGate99 {
			return i
		}
	}
	return -1
}

// eventStart walks back from a gate crossing through the observations whose
// innovation already pointed the same way, by at least a quarter of the
// crossing's length: the gate trips once the error is large, but the event
// began where the error started to build. A person reading the stored
// innovations does the same.
func eventStart(states []SmoothedState, onset int) int {
	o := states[onset].Observation
	norm := math.Hypot(float64(o.InnovationX), float64(o.InnovationY))
	i := onset
	for i > 0 && states[i-1].Observed && states[i-1].Observation.HasInnovation {
		in := states[i-1].Observation
		if float64(in.InnovationX*o.InnovationX+in.InnovationY*o.InnovationY)/norm < 0.25*norm {
			break
		}
		i--
	}
	return i
}

// signedInnovationSum is the CUSUM of plan §12.1 in its simplest form: the
// innovations of the onset state and the next n observed states, projected on
// the onset innovation's direction and divided by its length. A measurement
// anomaly reverses, so the sum returns towards zero; a real change of motion
// keeps adding same-signed innovations while the filter catches up.
func signedInnovationSum(states []SmoothedState, onset, n int) float64 {
	o := states[onset].Observation
	norm := math.Hypot(float64(o.InnovationX), float64(o.InnovationY))
	sum := 0.0
	for i, seen := onset, 0; i < len(states) && seen <= n; i++ {
		if !states[i].Observed {
			continue
		}
		in := states[i].Observation
		sum += float64(in.InnovationX*o.InnovationX+in.InnovationY*o.InnovationY) / norm / norm
		seen++
	}
	return sum
}

func maxNISIndex(states []SmoothedState) int {
	best := -1
	for i, s := range states {
		if s.Observed && s.Observation.HasInnovation && (best < 0 || s.Observation.NIS > states[best].Observation.NIS) {
			best = i
		}
	}
	return best
}

// velocityStep is the smoothed velocity change across index k over ±half
// seconds of capture time.
func velocityStep(states []SmoothedState, k int, half float64) float64 {
	lo, hi := k, k
	for lo > 0 && sceneSecs(states[k])-sceneSecs(states[lo]) < half-1e-6 {
		lo--
	}
	for hi < len(states)-1 && sceneSecs(states[hi])-sceneSecs(states[k]) < half-1e-6 {
		hi++
	}
	a, b := states[lo].Smoothed, states[hi].Smoothed
	return math.Hypot(float64(b.VX-a.VX), float64(b.VY-a.VY))
}

func TestImpactStaysIdentifiableInTheRevisionAudit(t *testing.T) {
	// The velocity changes at 3.0 s, where both paths share a position; the
	// 3.1 s frame carries the first observation of the new motion.
	const impactFrame = 31
	anomalyFrame := 30
	anomalyScene := func(t float64) (float64, float64) {
		x, y := straightPath(t)
		if math.Abs(t-float64(anomalyFrame)/10) < 1e-6 {
			y += 1.0 // one displaced cluster: a measurement anomaly
		}
		return x, y
	}
	trueDeltaV := math.Hypot(9-15, 3)

	for _, lag := range comparatorLags {
		_, impactArms := runScene(t, DefaultTrackerConfig(), impactPath, 10, 60, 0.05, 3, nil, lag)
		_, anomalyArms := runScene(t, DefaultTrackerConfig(), anomalyScene, 10, 60, 0.05, 3, nil, lag)
		impact := soleTrackStates(t, impactArms.released[0])
		anomaly := soleTrackStates(t, anomalyArms.released[0])

		frameOf := func(s SmoothedState) int { return int(math.Round(sceneSecs(s) * 10)) }

		// 1. Where did the stored record first say something happened? Both
		// events cross the per-frame NIS gate at their own frame.
		onset := onsetIndex(impact)
		if onset < 0 {
			t.Fatalf("%s: no impact innovation crossed the NIS gate", lag)
		}
		onset = eventStart(impact, onset)
		if frameOf(impact[onset]) != impactFrame {
			t.Fatalf("%s: impact identified at frame %d, want %d", lag, frameOf(impact[onset]), impactFrame)
		}
		peak := onsetIndex(anomaly)
		if peak < 0 || eventStart(anomaly, peak) != peak || frameOf(anomaly[peak]) != anomalyFrame || maxNISIndex(anomaly) != peak {
			t.Fatalf("%s: anomaly identified at index %d, want frame %d as the strongest innovation", lag, peak, anomalyFrame)
		}
		// 2. Which was it? The anomaly's next innovation reverses and its
		// signed sum over the following 0.5 s falls back; the impact's grows.
		next, now := anomaly[peak+1].Observation, anomaly[peak].Observation
		if next.InnovationX*now.InnovationX+next.InnovationY*now.InnovationY >= 0 {
			t.Fatalf("%s: the innovation after the anomaly did not reverse", lag)
		}
		impactSum, anomalySum := signedInnovationSum(impact, onset, 5), signedInnovationSum(anomaly, peak, 5)
		if impactSum < 2 || anomalySum > 0.5 {
			t.Fatalf("%s: signed innovation sums impact %.2f, anomaly %.2f: want the impact to accumulate (≥ 2) and the anomaly to return (≤ 0.5)",
				lag, impactSum, anomalySum)
		}

		// 3. The revision audit names the impact observation as the evidence
		// that moved the states before it: every pre-impact state whose window
		// reached the impact lists it, and its strongest evidence is at or
		// after the impact.
		for _, s := range impact {
			if frameOf(s) >= impactFrame || frameOf(s) < impactFrame-3 {
				continue
			}
			cites := false
			for _, e := range s.Revision.Evidence {
				if int(math.Round(float64(e.FrameUnixNanos-tdAt(1).UnixNano())/1e8)) == impactFrame {
					cites = true
				}
			}
			strongest, ok := s.Revision.StrongestEvidence()
			if s.LookaheadSteps > 0 && (!cites || !ok || strongest.FrameUnixNanos < tdAt(1+float64(impactFrame)/10).UnixNano()) {
				t.Fatalf("%s: pre-impact state at frame %d does not attribute its revision to the impact", lag, frameOf(s))
			}
		}

		// 4. The smoother kept the event. At the shipped process noise the CV
		// filter cannot resolve a step change inside ±0.5 s, so the bar is
		// relative there: the refined velocity change across the impact must
		// be at least what the online filter showed, at both ±0.5 s and ±1 s,
		// and at ±1 s at least 80% of the true change. Across the anomaly the
		// change is small beside the impact's.
		online := onlineSeries(impact)
		dvImpact, dvImpact1 := velocityStep(impact, onset, 0.5), velocityStep(impact, onset, 1.0)
		dvOnline, dvOnline1 := velocityStep(online, onset, 0.5), velocityStep(online, onset, 1.0)
		dvAnomaly := velocityStep(anomaly, peak, 0.5)
		t.Logf("%-5s impact Δv ±0.5 s %.2f (online %.2f), ±1 s %.2f (online %.2f) = %.0f%% of %.2f m/s; signed sum %.2f | anomaly Δv %.2f, signed sum %.2f",
			lag, dvImpact, dvOnline, dvImpact1, dvOnline1, 100*dvImpact1/trueDeltaV, trueDeltaV, impactSum, dvAnomaly, anomalySum)
		if dvImpact < dvOnline || dvImpact1 < dvOnline1 {
			t.Fatalf("%s: the refined estimate flattened the impact below the online filter's", lag)
		}
		if dvImpact1 < 0.8*trueDeltaV {
			t.Fatalf("%s: refined velocity change across the impact %.2f m/s is below 80%% of %.2f", lag, dvImpact1, trueDeltaV)
		}
		if dvAnomaly > 0.25*dvImpact {
			t.Fatalf("%s: the anomaly left a %.2f m/s velocity change, not distinguishable from the %.2f m/s impact", lag, dvAnomaly, dvImpact)
		}
	}
}

// onlineSeries substitutes each state's online moments for its smoothed
// ones, so the same measurement helpers read the causal estimate.
func onlineSeries(states []SmoothedState) []SmoothedState {
	out := append([]SmoothedState(nil), states...)
	for i := range out {
		out[i].Smoothed = out[i].Online
	}
	return out
}

// TestCoastedStatesStaySmoothedPredictions: a five-frame gap in the middle of
// a track. The coasted states are revised by the observation that ends the
// gap, cite it as evidence, and remain unobserved in every arm. The trailing
// coast before deletion has no later evidence, so it is not revised at all.
func TestCoastedStatesStaySmoothedPredictions(t *testing.T) {
	gap := func(k int) bool { return (k >= 40 && k < 45) || k >= 70 }
	tk, arms := runScene(t, DefaultTrackerConfig(), straightPath, 10, 90, 0.1, 5, gap, comparatorLags...)
	if tk.TimeDomainStats().ExpiredByMisses == 0 {
		t.Fatal("setup: the trailing gap should expire the track by misses")
	}
	for i, lag := range comparatorLags {
		states := soleTrackStates(t, arms.released[i])
		coasted, trailing := 0, 0
		for _, s := range states {
			frame := int(math.Round(sceneSecs(s) * 10))
			isGap := gap(frame)
			if s.Observed == isGap {
				t.Fatalf("%s frame %d: observed=%v in a scene where the gap is %v", lag, frame, s.Observed, isGap)
			}
			if frame >= 70 {
				trailing++
				if s.Revision.PositionMetres != 0 || s.Revision.VelocityMps != 0 || len(s.Revision.Evidence) != 0 {
					t.Fatalf("%s trailing coast at frame %d revised by %.3g m with %d evidence", lag, frame,
						s.Revision.PositionMetres, len(s.Revision.Evidence))
				}
				if s.Smoothed.X != s.Online.X || s.Smoothed.Y != s.Online.Y {
					t.Fatalf("%s trailing coast at frame %d moved", lag, frame)
				}
				continue
			}
			if !isGap {
				continue
			}
			coasted++
			if s.LookaheadSteps >= 45-frame {
				if s.Revision.PositionMetres == 0 || len(s.Revision.Evidence) == 0 {
					t.Fatalf("%s coasted frame %d was not revised by the observation ending the gap", lag, frame)
				}
				if s.Revision.Evidence[0].ClusterID != 46 { // runScene numbers clusters k+1
					t.Fatalf("%s coasted frame %d cites cluster %d first, want 46", lag, frame, s.Revision.Evidence[0].ClusterID)
				}
			}
		}
		if coasted != 5 || trailing == 0 {
			t.Fatalf("%s: %d coasted mid-track states and %d trailing, want 5 and some", lag, coasted, trailing)
		}
		if st := arms.smoothers[i].Stats(); st.RevisionsWithoutEvidence != 0 {
			t.Fatalf("%s: %d revisions without evidence", lag, st.RevisionsWithoutEvidence)
		}
	}
}

// TestSmootherMemoryIsBounded: many tracks, long lives. No window ever holds
// more than its cap, the steps held return to zero as chains end, and a
// full-track window that reaches its cap releases its oldest state early as
// fixed_lag rather than growing or claiming finality it does not have.
func TestSmootherMemoryIsBounded(t *testing.T) {
	cfg := DefaultTrackerConfig()
	tk := NewTracker(cfg)
	lags := []SmootherLag{LagFrames(3), LagSeconds(2)}
	arms := newSmootherArms(t, lags...)
	capped, err := NewFixedLagSmoother(SmootherConfig{Lag: LagTrackEnd(), MaxWindowSteps: 50})
	if err != nil {
		t.Fatal(err)
	}
	arms.smoothers = append(arms.smoothers, capped)
	arms.released = append(arms.released, nil)
	tk.SetFilterStepObserver(arms)
	const objects, frames = 12, 400
	for k := 0; k < frames; k++ {
		var clusters []WorldCluster
		for o := 0; o < objects; o++ {
			// Stationary objects 20 m apart: the scene is about memory, not motion.
			c := tdCluster(float32(20*o), float32(10+0.01*float64(k%3)), 0)
			c.ClusterID = int64(k*objects + o + 1)
			clusters = append(clusters, c)
		}
		tk.Update(clusters, tdAt(1+0.1*float64(k)))
	}
	for i, s := range arms.smoothers {
		st := s.Stats()
		cap := s.Config().WindowCap()
		if st.MaxWindowSteps > cap {
			t.Fatalf("arm %d held a %d-step window, cap %d", i, st.MaxWindowSteps, cap)
		}
		if st.PeakHeldSteps > objects*cap {
			t.Fatalf("arm %d held %d steps at once, bound %d", i, st.PeakHeldSteps, objects*cap)
		}
		if st.OpenWindows != objects {
			t.Fatalf("arm %d has %d open windows, want %d", i, st.OpenWindows, objects)
		}
	}
	arms.flush()
	for i, s := range arms.smoothers {
		st := s.Stats()
		if st.HeldSteps != 0 || st.OpenWindows != 0 {
			t.Fatalf("arm %d still holds %d steps in %d windows after flush", i, st.HeldSteps, st.OpenWindows)
		}
		if st.Released != st.Steps {
			t.Fatalf("arm %d released %d of %d steps: every state is released exactly once", i, st.Released, st.Steps)
		}
	}
	cappedStats := capped.Stats()
	if cappedStats.ReleasedByWindowCap == 0 {
		t.Fatal("the capped full-track arm never reached its cap")
	}
	for _, s := range arms.released[2] {
		if s.Release == ReleaseWindowCap && (s.Stage != RefinementFixedLag || !s.LookaheadTruncated) {
			t.Fatalf("a state released early by the cap claims stage %s, truncated=%v", s.Stage, s.LookaheadTruncated)
		}
		if s.Release == ReleaseChainEnd && s.Stage != RefinementFinal {
			t.Fatalf("a state released at the end of input is %s, want final", s.Stage)
		}
	}
	t.Logf("3f cap %d, 2 s cap %d, capped full-track %d; peak held steps %d / %d / %d",
		arms.smoothers[0].Config().WindowCap(), arms.smoothers[1].Config().WindowCap(), capped.Config().WindowCap(),
		arms.smoothers[0].Stats().PeakHeldSteps, arms.smoothers[1].Stats().PeakHeldSteps, cappedStats.PeakHeldSteps)
}

// TestSmootherIsDeterministic: the same input gives the same output, and a
// frame's step order does not matter to any track's result. Track IDs are
// random UUIDs, so tracks are compared by creation sequence.
func TestSmootherIsDeterministic(t *testing.T) {
	scene := func() *smootherArms {
		tk := NewTracker(DefaultTrackerConfig())
		arms := newSmootherArms(t, comparatorLags...)
		tk.SetFilterStepObserver(arms)
		for k := 0; k < 80; k++ {
			var clusters []WorldCluster
			for o := 0; o < 4; o++ {
				c := tdCluster(float32(30*o)+0.5*float32(k)*0.1, float32(8*o)+0.01*float32(k%5), 0)
				c.ClusterID = int64(k*4 + o + 1)
				clusters = append(clusters, c)
			}
			tk.Update(clusters, tdAt(1+0.1*float64(k)))
		}
		arms.flush()
		return arms
	}
	strip := func(states []SmoothedState) []SmoothedState {
		out := append([]SmoothedState(nil), states...)
		for i := range out {
			out[i].TrackID = ""
		}
		return out
	}
	first, second := scene(), scene()
	for i := range comparatorLags {
		if !reflect.DeepEqual(strip(first.released[i]), strip(second.released[i])) {
			t.Fatalf("%s: two runs of the same scene released different states", comparatorLags[i])
		}
	}

	// Reverse each frame's step order into a fresh smoother: per track, the
	// result must not change.
	for i, lag := range comparatorLags {
		s, _ := NewFixedLagSmoother(SmootherConfig{Lag: lag})
		var reversed []SmoothedState
		for _, frame := range first.frames {
			f := frame
			f.Steps = append([]FilterStep(nil), frame.Steps...)
			for a, b := 0, len(f.Steps)-1; a < b; a, b = a+1, b-1 {
				f.Steps[a], f.Steps[b] = f.Steps[b], f.Steps[a]
			}
			reversed = append(reversed, s.Observe(f)...)
		}
		reversed = append(reversed, s.Flush()...)
		if !reflect.DeepEqual(byTrack(reversed), byTrack(first.released[i])) {
			t.Fatalf("%s: per-track output depends on the order of steps within a frame", lag)
		}
	}
}

// TestFilterStepObserverDoesNotChangeTracking: attaching the recorder and a
// smoother must leave every track exactly as the unobserved tracker leaves
// it. The live pipeline never attaches one; this is the proof that an offline
// run that does is still measuring the shipped tracker.
func TestFilterStepObserverDoesNotChangeTracking(t *testing.T) {
	run := func(observe bool) map[int64]TrackedObject {
		tk := NewTracker(DefaultTrackerConfig())
		if observe {
			tk.SetFilterStepObserver(newSmootherArms(t, comparatorLags...))
		}
		for k := 0; k < 120; k++ {
			var clusters []WorldCluster
			for o := 0; o < 3; o++ {
				if (k+o)%7 == 0 {
					continue // misses, so coasting and inflation are exercised
				}
				c := tdCluster(float32(4*o)+0.8*float32(k)*0.1*float32(o+1), float32(10*o)+0.02*float32(k%4), 0)
				c.ClusterID = int64(k*3 + o + 1)
				clusters = append(clusters, c)
			}
			tk.Update(clusters, tdAt(1+0.1*float64(k)))
		}
		out := map[int64]TrackedObject{}
		for _, track := range tk.Tracks {
			snapshot := *track
			snapshot.TrackID = ""
			out[track.CreationSequence] = snapshot
		}
		return out
	}
	plain, observed := run(false), run(true)
	if len(plain) == 0 || len(plain) != len(observed) {
		t.Fatalf("%d tracks unobserved, %d observed", len(plain), len(observed))
	}
	for seq, want := range plain {
		got := observed[seq]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("track %d differs when observed:\n got X=%v Y=%v VX=%v VY=%v P=%v\nwant X=%v Y=%v VX=%v VY=%v P=%v",
				seq, got.X, got.Y, got.VX, got.VY, got.P, want.X, want.Y, want.VX, want.VY, want.P)
		}
	}
}

// TestFilterStepsRecordTheFilter checks the recorder against the filter it
// records: an observed step's prior is the prediction the innovation was
// measured against; the prediction interval is the one the filter used,
// including the MaxPredictDt clamp, which the smoother flags; and a deleted
// track's chain ends without a step for its deletion frame.
func TestFilterStepsRecordTheFilter(t *testing.T) {
	cfg := DefaultTrackerConfig()
	tk := NewTracker(cfg)
	arms := newSmootherArms(t, LagFrames(3))
	tk.SetFilterStepObserver(arms)
	times := []float64{1.0, 1.1, 1.2, 1.3, 2.3, 2.4, 2.5}
	for k, secs := range times {
		// 1 m/s: slow enough that the zero-velocity initial state keeps the
		// cluster inside the gate across the 1 s gap.
		c := tdCluster(float32(secs-1), 10, 0)
		c.ClusterID = int64(k + 1)
		tk.Update([]WorldCluster{c}, tdAt(secs))
	}
	var steps []FilterStep
	for _, f := range arms.frames {
		steps = append(steps, f.Steps...)
	}
	if len(steps) != len(times) || steps[0].Linked || !steps[0].Observed || steps[0].Observation.HasInnovation {
		t.Fatalf("the founding step must be observed, unlinked and without an innovation: %+v", steps[0])
	}
	for k := 1; k < len(steps); k++ {
		s := steps[k]
		if !s.Linked || !s.Observed || !s.Observation.HasInnovation {
			t.Fatalf("step %d: linked=%v observed=%v", k, s.Linked, s.Observed)
		}
		gap := times[k] - times[k-1]
		wantTau := math.Min(gap, float64(cfg.MaxPredictDt))
		if math.Abs(float64(s.PredictedSecs)-wantTau) > 1e-6 {
			t.Fatalf("step %d: PredictedSecs %.4f, want %.4f (gap %.1f s, max_predict_dt %.1f)", k, s.PredictedSecs, wantTau, gap, cfg.MaxPredictDt)
		}
		innov := [2]float32{s.Observation.X - s.Prior.X, s.Observation.Y - s.Prior.Y}
		if math.Abs(float64(innov[0]-s.Observation.InnovationX)) > 1e-5 || math.Abs(float64(innov[1]-s.Observation.InnovationY)) > 1e-5 {
			t.Fatalf("step %d: the recorded prior is not the one the innovation was measured against", k)
		}
		prev := steps[k-1].Posterior
		tau := s.PredictedSecs
		if math.Abs(float64(prev.X+prev.VX*tau-s.Prior.X)) > 1e-4 {
			t.Fatalf("step %d: prior X %.5f is not F(τ) applied to the previous posterior (%.5f)", k, s.Prior.X, prev.X+prev.VX*tau)
		}
	}
	released := arms.released[0]
	clamped := 0
	for _, s := range released {
		if s.ClampedPrediction {
			clamped++
		}
	}
	if clamped == 0 || arms.smoothers[0].Stats().ClampedTransitions != 1 {
		t.Fatalf("the 1 s gap clamped to max_predict_dt was not flagged: %d states, %d transitions", clamped,
			arms.smoothers[0].Stats().ClampedTransitions)
	}

	// Let the (confirmed) track expire: its chain ends, and no step is
	// recorded for the frame in which it was deleted.
	stepsBefore := len(steps)
	for k := 0; k < cfg.MaxMissesConfirmed+2; k++ {
		tk.Update(nil, tdAt(2.6+0.1*float64(k)))
	}
	var ended []FilterChainEnd
	after := 0
	for _, f := range arms.frames {
		ended = append(ended, f.Ended...)
		after += len(f.Steps)
	}
	if len(ended) != 1 || ended[0].Reason != ChainEndDeleted {
		t.Fatalf("chain ends %+v, want one track_deleted", ended)
	}
	if deleted := tk.Tracks[ended[0].TrackID]; deleted == nil || deleted.EndUnixNanos != ended[0].FrameUnixNanos {
		t.Fatal("the chain end is not stamped with the deletion frame")
	}
	if coasts := after - stepsBefore; coasts != cfg.MaxMissesConfirmed-1 {
		t.Fatalf("%d coasted steps recorded before deletion, want %d (the deletion frame is not a step)", coasts, cfg.MaxMissesConfirmed-1)
	}
}

// TestCaptureGapPredictionRecordsTheWholeInterval: with sub-stepped gap
// prediction the recorded interval is the whole gap, so nothing is flagged.
func TestCaptureGapPredictionRecordsTheWholeInterval(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.CaptureGapPrediction = true
	tk := NewTracker(cfg)
	arms := newSmootherArms(t, LagFrames(3))
	tk.SetFilterStepObserver(arms)
	for k, secs := range []float64{1.0, 1.1, 2.3, 2.4} {
		c := tdCluster(float32(2*(secs-1)), 10, 0)
		c.ClusterID = int64(k + 1)
		tk.Update([]WorldCluster{c}, tdAt(secs))
	}
	if got := arms.frames[2].Steps[0].PredictedSecs; math.Abs(float64(got)-1.2) > 1e-5 {
		t.Fatalf("PredictedSecs = %v across a 1.2 s gap, want the whole gap", got)
	}
	if n := arms.smoothers[0].Stats().ClampedTransitions; n != 0 {
		t.Fatalf("%d clamped transitions with capture-gap prediction on", n)
	}
}

// TestResetEndsEveryChain: Tracker.Reset releases everything the smoothers
// hold instead of leaving windows for tracks that no longer exist.
func TestResetEndsEveryChain(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	arms := newSmootherArms(t, LagSeconds(2))
	tk.SetFilterStepObserver(arms)
	for k := 0; k < 5; k++ {
		tk.Update([]WorldCluster{tdCluster(0, 10, 0), tdCluster(20, 10, 0)}, tdAt(1+0.1*float64(k)))
	}
	tk.Reset()
	last := arms.frames[len(arms.frames)-1]
	if len(last.Ended) != 2 || last.Ended[0].Reason != ChainEndReset {
		t.Fatalf("reset ended %+v, want two tracker_reset", last.Ended)
	}
	if st := arms.smoothers[0].Stats(); st.OpenWindows != 0 || st.Released != 10 {
		t.Fatalf("after reset: %d open windows, %d released of 10", st.OpenWindows, st.Released)
	}
}

// Numerical guards, on hand-built steps.

func manualStep(seq int64, secs float64, x, vx float32, observed, linked bool) FilterStep {
	p := [16]float32{0.1, 0, 0.05, 0, 0, 0.1, 0, 0.05, 0.05, 0, 0.5, 0, 0, 0.05, 0, 0.5}
	m := FilterMoments{X: x, Y: 10, VX: vx, P: p}
	prior := m
	prior.P[0] += 0.02
	prior.P[5] += 0.02
	return FilterStep{
		TrackID: "trk_manual", CreationSequence: seq,
		FrameUnixNanos: tdAt(secs).UnixNano(), StateUnixNanos: tdAt(secs).UnixNano(),
		PredictedSecs: 0.1, Linked: linked, Observed: observed, Prior: prior, Posterior: m,
		Observation: FilterObservation{ClusterID: int64(secs * 10), HasInnovation: observed, X: x, Y: 10},
	}
}

func TestSmootherGuardsNumerics(t *testing.T) {
	feed := func(s *FixedLagSmoother, steps ...FilterStep) []SmoothedState {
		var out []SmoothedState
		for _, st := range steps {
			out = append(out, s.Observe(FilterFrame{FrameUnixNanos: st.FrameUnixNanos, Steps: []FilterStep{st}})...)
		}
		return append(out, s.Flush()...)
	}
	checkFinite := func(t *testing.T, states []SmoothedState) {
		t.Helper()
		for _, s := range states {
			if !finiteMoments(s.Smoothed) || math.IsNaN(s.Revision.PositionMetres) {
				t.Fatalf("non-finite output: %+v", s.Smoothed)
			}
		}
	}

	t.Run("non-finite step is refused and splits the chain", func(t *testing.T) {
		s, _ := NewFixedLagSmoother(SmootherConfig{Lag: LagFrames(3)})
		bad := manualStep(1, 1.2, float32(math.NaN()), 1, true, true)
		out := feed(s, manualStep(1, 1.0, 0, 1, true, false), manualStep(1, 1.1, 0.1, 1, true, true), bad,
			manualStep(1, 1.3, 0.3, 1, true, true), manualStep(1, 1.4, 0.4, 1, true, true))
		checkFinite(t, out)
		st := s.Stats()
		if st.RefusedSteps != 1 || st.Barriers != 1 || st.ReleasedAtBarrier != 2 || st.Released != 4 {
			t.Fatalf("stats %+v", st)
		}
	})
	t.Run("indefinite prior is a barrier", func(t *testing.T) {
		s, _ := NewFixedLagSmoother(SmootherConfig{Lag: LagFrames(3)})
		indefinite := manualStep(1, 1.1, 0.1, 1, true, true)
		indefinite.Prior.P[2], indefinite.Prior.P[8] = 5, 5 // |cov(x,vx)| far beyond sqrt(var x · var vx)
		out := feed(s, manualStep(1, 1.0, 0, 1, true, false), indefinite, manualStep(1, 1.2, 0.2, 1, true, true))
		checkFinite(t, out)
		if st := s.Stats(); st.Barriers != 1 || st.ReleasedAtBarrier != 1 || !out[0].LookaheadTruncated {
			t.Fatalf("stats %+v, first state %+v", st, out[0])
		}
	})
	t.Run("capture time running backwards is a barrier", func(t *testing.T) {
		s, _ := NewFixedLagSmoother(SmootherConfig{Lag: LagFrames(3)})
		out := feed(s, manualStep(1, 1.0, 0, 1, true, false), manualStep(1, 1.1, 0.1, 1, true, true),
			manualStep(1, 1.05, 0.15, 1, true, true))
		checkFinite(t, out)
		if st := s.Stats(); st.TimeRegressions != 1 || st.Barriers != 1 {
			t.Fatalf("stats %+v", st)
		}
	})
	t.Run("an asymmetric posterior is symmetrised, not refused", func(t *testing.T) {
		s, _ := NewFixedLagSmoother(SmootherConfig{Lag: LagFrames(1)})
		skew := manualStep(1, 1.1, 0.1, 1, true, true)
		skew.Posterior.P[1] += 1e-4
		out := feed(s, manualStep(1, 1.0, 0, 1, true, false), skew)
		checkFinite(t, out)
		for _, st := range out {
			if asymmetry(st.Smoothed.P) != 0 {
				t.Fatalf("smoothed covariance is not symmetric: %v", st.Smoothed.P)
			}
		}
		if s.Stats().MaxCovarianceAsymmetry < 1e-5 || s.Stats().RefusedSteps != 0 {
			t.Fatalf("stats %+v", s.Stats())
		}
	})
}

func TestSmootherConfigValidation(t *testing.T) {
	for _, bad := range []SmootherConfig{
		{Lag: SmootherLag{Frames: 3, Secs: 1}},
		{Lag: SmootherLag{Frames: -1}},
		{Lag: SmootherLag{Secs: math.NaN()}},
		{Lag: SmootherLag{Secs: math.Inf(1)}},
		{Lag: LagFrames(3), MaxWindowSteps: 3},
		{Lag: LagTrackEnd(), MaxWindowSteps: 1},
	} {
		if _, err := NewFixedLagSmoother(bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	for cfg, want := range map[SmootherConfig]int{
		{Lag: LagFrames(3)}:                     4,
		{Lag: LagSeconds(0.5)}:                  11,
		{Lag: LagSeconds(2)}:                    41,
		{Lag: LagTrackEnd()}:                    DefaultTrackEndWindowSteps,
		{Lag: LagSeconds(1), MaxWindowSteps: 7}: 7,
	} {
		if got := cfg.WindowCap(); got != want {
			t.Errorf("%+v: WindowCap %d, want %d", cfg, got, want)
		}
	}
	if s := strings.Join([]string{LagFrames(3).String(), LagSeconds(0.5).String(), LagTrackEnd().String()}, ","); s != "3f,0.5s,track" {
		t.Errorf("lag labels %q", s)
	}
}
