package l5tracks

import (
	"math"
	"math/rand"
	"testing"
)

// Tests for the extent state's inequality constraints: the frame's span
// bounds and the half-extents' floor (applyExtentBounds,
// holdHalfExtentsAbove). What carries the weight: on the box pass the span
// bounds no longer carry hL past what the faces say, on the shadow and the
// tracked filter; a bound moves the mean to the bound and no further, and a
// rising run of bounds leaves it where the highest alone would; no face,
// medoid update or bound drives a half-extent through zero; and the
// projection is the constrained minimum, with the covariance untouched.

// The box's near side is in view on every frame of the pass, so every
// frame's span covers the whole length and bounds it at 4.3 to 4.4 m. Those
// bounds must not carry hL past the faces' estimate: with them the
// half-length ends within 0.1 m of the truth and the departure centre is no
// worse than the extent state's alone. Before the projection, a moment-matched
// truncation ended at hL 2.457 m with a departure error of 0.190 m, against
// 2.214 m and 0.054 m without the bounds, on both paths.
func checkSpanBoundsHoldTheFaces(t *testing.T, arm string, alone, bounded boxPassResult) {
	t.Helper()
	t.Logf("%s, extent state: approach %.3f m, departure %.3f m, half-length %.3f m", arm,
		alone.approachErr, alone.departureErr, alone.finalHalfLen)
	t.Logf("%s, extent state with span bounds: approach %.3f m, departure %.3f m, half-length %.3f m, least eigenvalue %.3g", arm,
		bounded.approachErr, bounded.departureErr, bounded.finalHalfLen, bounded.worstEigen)
	if math.Abs(bounded.finalHalfLen-2.25) > 0.1 {
		t.Errorf("%s: half-length %.3f m with span bounds, want 2.25 m within 0.1 m", arm, bounded.finalHalfLen)
	}
	if bounded.departureErr > alone.departureErr+1e-3 {
		t.Errorf("%s: departure error %.3f m with span bounds, worse than the extent state's alone, %.3f m",
			arm, bounded.departureErr, alone.departureErr)
	}
	if bounded.approachErr > alone.approachErr+1e-3 {
		t.Errorf("%s: approach error %.3f m with span bounds, worse than the extent state's alone, %.3f m",
			arm, bounded.approachErr, alone.approachErr)
	}
	if bounded.worstAsym != 0 || bounded.worstEigen < -1e-6 {
		t.Errorf("%s: with span bounds the joint covariance had asymmetry %.3g and eigenvalue %.3g",
			arm, bounded.worstAsym, bounded.worstEigen)
	}
}

func TestSpanBoundsDoNotCarryTheExtentStatePastTheFaces(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	alone := runBoxPass(t, cfg, shortExtentState)
	cfg.SolidBody.SpanBoundsBelief = true
	bounded := runBoxPass(t, cfg, shortExtentState)
	checkSpanBoundsHoldTheFaces(t, "shadow", alone, bounded)

	tracked := nearEdgeTrackingConfig()
	tracked.SolidBody.ExtentState = true
	trackedAlone := runTrackedBoxPass(t, tracked, shortExtentState)
	tracked.SolidBody.SpanBoundsBelief = true
	trackedBounded := runTrackedBoxPass(t, tracked, shortExtentState)
	checkSpanBoundsHoldTheFaces(t, "tracked", trackedAlone.boxPassResult, trackedBounded.boxPassResult)
}

// boundBody is a body-centre solid body whose half-length is believed at
// 1.5625 m (sigma 0.75 m), correlated with x.
func boundBody() solidBodyTrack {
	var sb solidBodyTrack
	sb.p = [16]float32{0.1, 0, 0, 0, 0, 0.1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	sb.ext = solidBodyExtent{on: true, h: [2]float32{1.5625, 0.95}, phh: [4]float32{0.5625, 0, 0, 0.04},
		pxh: [8]float32{0.05, 0, 0, 0, 0, 0, 0, 0}}
	return sb
}

// A bound is an inequality constraint, imposed by projection: one above the
// mean moves hL to half of it and the correlated position by its regression
// on hL, and leaves the covariance as it was. A slowly rising run of bounds,
// as consecutive frames' spans of the same body give, leaves hL where the
// highest alone puts it. A moment-matched truncation does neither: its mean
// lands past the bound by sigma times the inverse Mills ratio, and each bound
// a millimetre higher truncates the returned Gaussian again as if it were new
// evidence.
func TestExtentStateBoundMovesTheMeanToTheBoundAndNoFurther(t *testing.T) {
	sb := boundBody()
	p0, pxh0, phh0 := sb.p, sb.ext.pxh, sb.ext.phh
	const first = float32(4.372)
	if touched := sb.applyExtentBounds([2]float32{first, 0}); !touched[0] || touched[1] {
		t.Fatalf("touched %v, want the length alone", touched)
	}
	if got, want := sb.ext.h[0], first/2; got != want {
		t.Fatalf("after a %.3f m bound hL %.4f m, want half the bound, %.4f m", first, got, want)
	}
	wantX := 0.05 / 0.5625 * (float64(first)/2 - 1.5625)
	if math.Abs(float64(sb.state[0])-wantX) > 1e-6 {
		t.Fatalf("x %.6f after the bound, want its regression on hL, %.6f", sb.state[0], wantX)
	}
	if sb.p != p0 || sb.ext.pxh != pxh0 || sb.ext.phh != phh0 {
		t.Fatal("the bound changed the covariance")
	}

	// Twenty frames' bounds, each a millimetre above the last.
	highest := first
	for k := 1; k <= 20; k++ {
		highest = first + 0.001*float32(k)
		sb.applyExtentBounds([2]float32{highest, 0})
	}
	once := boundBody()
	once.applyExtentBounds([2]float32{highest, 0})
	if math.Abs(float64(sb.ext.h[0]-highest/2)) > 1e-6 || math.Abs(float64(sb.state[0]-once.state[0])) > 1e-6 {
		t.Fatalf("after a rising run to %.3f m: hL %.5f m and x %.6f, the highest bound alone gives %.5f m and %.6f",
			highest, sb.ext.h[0], sb.state[0], once.ext.h[0], once.state[0])
	}
	if sb.p != p0 || sb.ext.pxh != pxh0 || sb.ext.phh != phh0 {
		t.Fatal("the run of bounds changed the covariance")
	}

	// A bound at or below the mean does nothing, and is not counted.
	h, x := sb.ext.h, sb.state
	if touched := sb.applyExtentBounds([2]float32{4, 1.8}); touched != [2]bool{} || sb.ext.h != h || sb.state != x {
		t.Fatalf("bounds under the mean moved the state: touched %v, hL %.4f, hW %.4f", touched, sb.ext.h[0], sb.ext.h[1])
	}
}

// Nothing in the joint update keeps a half-extent positive: a face measures
// n·c + h, so a face whose plane lies far inboard of the prediction (a
// cluster that has shed its far returns, or a face of the wrong identity)
// moves h by its gain, through zero. kirk0's fragment-join arm wrote a
// 0 m length on three rows this way. The floor holds each half-extent at
// extentStateMinHalfMetres by the projection the bounds use, so the
// covariance is the face update's.
func TestAFaceCannotDriveAHalfExtentThroughZero(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	tracker := NewTracker(cfg)
	prior := dimensionPriorFor(MotionRigidVehicle)
	track := boxPassBody(-10, 5, 5)
	sb := &track.solidBody
	sb.p = [16]float32{0.01, 0, 0, 0, 0, 0.01, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	sb.ext = solidBodyExtent{on: true, h: [2]float32{0.3, 0.9}, phh: [4]float32{0.5, 0, 0, 0.04}}
	// The front face's plane 1.2 m inboard of where the state puts it.
	face := EdgeMeasurement{Face: FaceFront, NormalX: 1, PlaneOffsetMetres: -10 + 0.3 - 1.2,
		HalfExtentMetres: 0.3, HalfExtentProvenance: ProvenanceAccumulated, SupportPoints: 20}

	// Unconstrained, the face's update takes hL below zero.
	free := sb.jointOf(sb.state, sb.p)
	if _, _, ok := tracker.applyEdgeMeasurementsJoint(&free, []EdgeMeasurement{face}, nil); !ok {
		t.Fatal("the face update refused")
	}
	if free.x[4] >= 0 {
		t.Fatalf("the unconstrained update leaves hL at %.3f m; the case needs one through zero", free.x[4])
	}

	cluster := passingBoxCluster(rand.New(rand.NewSource(13)), -10, 5, 4.5, 1.8)
	f := nearEdgeFrame{axis: 0, edges: EdgeMeasurementSet{Edges: []EdgeMeasurement{face}, Rank: 1},
		length: sb.extentDimension(0), width: sb.extentDimension(1)}
	if _, outcome, _ := tracker.stepNearEdge(sb, cluster, prior, f); outcome != nearEdgeFix {
		t.Fatalf("outcome %v, want a fix", outcome)
	}
	if sb.ext.h[0] != float32(extentStateMinHalfMetres) || !(sb.ext.h[1] > 0) {
		t.Fatalf("after the face hL %.4f m and hW %.4f m, want hL at the %.2f m floor and hW positive",
			sb.ext.h[0], sb.ext.h[1], extentStateMinHalfMetres)
	}
	got := sb.jointOf(sb.state, sb.p)
	checkJointCovariance(t, "after the face", got)
	// The covariance is the face update's, and every state moved from the
	// unconstrained update by its regression on hL.
	shift := extentStateMinHalfMetres - free.x[4]
	for i := 0; i < jointStates; i++ {
		want := free.x[i] + free.p[i*jointStates+4]/free.p[4*jointStates+4]*shift
		if math.Abs(got.x[i]-want) > 1e-5 {
			t.Errorf("state[%d] %.6f, want the unconstrained %.6f moved by its regression on hL, %.6f", i, got.x[i], free.x[i], want)
		}
		for j := 0; j < jointStates; j++ {
			if math.Abs(got.p[i*jointStates+j]-free.p[i*jointStates+j]) > 1e-6 {
				t.Errorf("P[%d][%d] %.6g, want the face update's %.6g", i, j, got.p[i*jointStates+j], free.p[i*jointStates+j])
			}
		}
	}
}

// On the tracked filter the medoid update moves the half-extents by their
// covariance with the position (applyMedoidGain): a medoid far behind a body
// whose x is strongly correlated with hL takes hL through zero without the
// floor.
func TestTheTrackedMedoidUpdateCannotDriveAHalfExtentThroughZero(t *testing.T) {
	cfg := nearEdgeTrackingConfig()
	cfg.SolidBody.ExtentState = true
	tracker := NewTracker(cfg)
	track := trackedBoxPassBody(3, 4, 2)
	track.ObservationCount = 1 // the initialisation window: the medoid update applies
	sb := &track.solidBody
	sb.reference = ReferenceClusterMedoid
	var js jointState
	js.x = [jointStates]float64{3, 4, 2, 0, 0.4, 0.9}
	for i, v := range []float64{1, 1, 1, 1, 0.5, 0.04} {
		js.p[i*jointStates+i] = v
	}
	// x with hL: a correlation of 0.85.
	js.p[0*jointStates+4], js.p[4*jointStates+0] = 0.6, 0.6
	setTrackedJoint(track, js)
	checkJointCovariance(t, "before", sb.jointOf([4]float32{track.X, track.Y, track.VX, track.VY}, track.P))

	// The medoid 2 m behind the state along x.
	cluster := passingBoxCluster(rand.New(rand.NewSource(17)), 1, 4, 4.5, 1.8)
	tracker.update(track, cluster, 1e8)
	if !track.LastResidual.Valid || sb.pending.outcome != nearEdgeMedoid {
		t.Fatalf("not a medoid update (residual %v, outcome %v)", track.LastResidual.Valid, sb.pending.outcome)
	}
	if y := track.LastResidual.InnovationX; y > -1.5 {
		t.Fatalf("innovation along x %.3f m; the case needs the medoid well behind", y)
	}
	if sb.ext.h[0] != float32(extentStateMinHalfMetres) || !(sb.ext.h[1] > 0) {
		t.Fatalf("after the medoid hL %.4f m and hW %.4f m, want hL at the %.2f m floor and hW positive",
			sb.ext.h[0], sb.ext.h[1], extentStateMinHalfMetres)
	}
	checkJointCovariance(t, "after the medoid", sb.jointOf([4]float32{track.X, track.Y, track.VX, track.VY}, track.P))
}

// checkJointCovariance fails unless the joint covariance is exactly
// symmetric and positive semi-definite to float precision.
func checkJointCovariance(t *testing.T, when string, js jointState) {
	t.Helper()
	if asym := maxAsymmetry(js.p[:], jointStates); asym != 0 {
		t.Errorf("%s: joint covariance asymmetric by %.3g", when, asym)
	}
	if e := minEigenvalue(js.p, jointStates); e < -1e-6 {
		t.Errorf("%s: joint covariance eigenvalue %.3g", when, e)
	}
}

// The projection is the constrained minimum: for random joint covariances
// and limits about the half-extents, the result is feasible, an active
// constraint is met exactly, no feasible point on a grid around the result
// costs less in the P_hh⁻¹ metric, every state moved by
// P_xh P_hh⁻¹ (h' − ĥ), and P is untouched.
func TestHalfExtentProjectionIsTheConstrainedMinimum(t *testing.T) {
	rng := rand.New(rand.NewSource(23))
	const n = jointStates
	bothActive := 0
	for trial := 0; trial < 200; trial++ {
		var js jointState
		js.p = randomJoint(rng, 0.4)
		for i := range js.x {
			js.x[i] = rng.NormFloat64()
		}
		lower := [2]float64{js.x[4] + rng.Float64() - 0.3, js.x[5] + rng.Float64() - 0.3}
		before := js
		active := js.holdHalfExtentsAbove(lower)
		if js.p != before.p {
			t.Fatalf("trial %d: the projection changed P", trial)
		}
		if active == [2]bool{true, true} {
			bothActive++
		}
		h := [2]float64{js.x[4], js.x[5]}
		for j := 0; j < 2; j++ {
			if h[j] < lower[j] || (active[j] && h[j] != lower[j]) {
				t.Fatalf("trial %d: h %v against limits %v, active %v", trial, h, lower, active)
			}
		}
		s00, s01, s11 := before.p[4*n+4], before.p[4*n+5], before.p[5*n+5]
		det := s00*s11 - s01*s01
		cost := func(a, b float64) float64 {
			da, db := a-before.x[4], b-before.x[5]
			return (s11*da*da - 2*s01*da*db + s00*db*db) / det
		}
		got := cost(h[0], h[1])
		const step = 0.005
		for a := -100; a <= 100; a++ {
			for b := -100; b <= 100; b++ {
				ha, hb := h[0]+step*float64(a), h[1]+step*float64(b)
				if ha < lower[0] || hb < lower[1] {
					continue
				}
				if c := cost(ha, hb); c < got-1e-9 {
					t.Fatalf("trial %d: (%.4f, %.4f) costs %.6g, under the projection's (%.4f, %.4f) at %.6g",
						trial, ha, hb, c, h[0], h[1], got)
				}
			}
		}
		// Every state's move is its regression on the half-extents' move.
		dh := [2]float64{h[0] - before.x[4], h[1] - before.x[5]}
		w := [2]float64{(s11*dh[0] - s01*dh[1]) / det, (s00*dh[1] - s01*dh[0]) / det}
		for i := 0; i < 4; i++ {
			want := before.x[i] + before.p[i*n+4]*w[0] + before.p[i*n+5]*w[1]
			if math.Abs(js.x[i]-want) > 1e-9 {
				t.Fatalf("trial %d: state[%d] %.9f, want its regression on the half-extents, %.9f", trial, i, js.x[i], want)
			}
		}
	}
	if bothActive == 0 {
		t.Fatal("no trial had both constraints active; the two-constraint branch is untested")
	}
	// Nothing below its limit: nothing moves.
	var js jointState
	js.p = randomJoint(rng, 0.4)
	js.x = [jointStates]float64{1, 2, 3, 4, 2, 1}
	before := js
	if active := js.holdHalfExtentsAbove([2]float64{1.9, 0.9}); active != ([2]bool{}) || js != before {
		t.Fatalf("limits under the mean: active %v, state %v", active, js.x)
	}
}
