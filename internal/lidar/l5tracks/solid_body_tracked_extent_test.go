package l5tracks

import (
	"math"
	"math/rand"
	"testing"
)

// Tests for the tracked extent state (NearEdgeTracking with ExtentState,
// near_edge_tracking.go) and the joint covariance the reading publishes.
// What carries the weight: on the tracked filter a body seeded short recovers
// its half-length from the rear face and then the front, and its centre with
// it, where the tracked filter without the option stays half the shortfall
// out; the joint covariance stays exactly symmetric and positive
// semi-definite through prediction, faces and the medoid update; the tracked
// medoid update and the consistent speed clamp are the joint updates; and a
// projected surface's variance is uᵀP_cc u + Var(h) + 2 dᵀP_ch.

// trackedBoxPassBody is boxPassBody on the tracked filter: a confirmed track
// whose solid body is the tracked state, referenced to its centre, moving at
// vx along +X with a velocity-resolved heading along +X.
func trackedBoxPassBody(x, y, vx float32) *TrackedObject {
	track := &TrackedObject{
		TrackID: "box", TrackState: TrackConfirmed, ObservationCount: 100,
		X: x, Y: y, VX: vx,
		P:             [16]float32{0.25, 0, 0, 0, 0, 0.25, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		HeadingSource: HeadingSourceVelocity,
	}
	sb := &track.solidBody
	sb.seeded, sb.tracked, sb.reference = true, true, ReferenceBodyCentre
	sb.estimation = EstimationGeometryConverging
	sb.orientation = OrientationBelief{PsiRad: 0, VarianceRad2: 3e-4, Provenance: ProvenanceObserved}
	sb.syncFromTrack(track)
	return track
}

// trackedBoxPassResult is one arm of the tracked box pass.
type trackedBoxPassResult struct {
	boxPassResult
	// lastFront and lastRear are the readings at the last fix by the front
	// face alone and by the rear face alone.
	lastFront, lastRear SolidBodyReading
	// readingDisagreements counts fixes whose published joint covariance was
	// not the tracked filter's.
	readingDisagreements int
}

// runTrackedBoxPass is runBoxPass through the tracked filter: each frame is
// the tracker's own prediction, its update (the near-edge step on the tracked
// state) and the solid body's completion, for a 4.5 by 1.8 m box passing
// 5 m from the sensor at 5 m/s from 20 m before it to 20 m after. The sensor
// sees the box's front (+X) face while the box approaches and its rear face
// once it has passed.
func runTrackedBoxPass(t *testing.T, cfg TrackerConfig, setup func(*solidBodyTrack)) trackedBoxPassResult {
	t.Helper()
	const length, width, cy, speed, dt = 4.5, 1.8, 5.0, 5.0, 0.1
	tracker := NewTracker(cfg)
	start := -20.0
	track := trackedBoxPassBody(float32(start), cy, speed)
	sb := &track.solidBody
	setup(sb)
	rng := rand.New(rand.NewSource(1))
	r := trackedBoxPassResult{boxPassResult: boxPassResult{worstEigen: math.Inf(1)}}
	var approach, departure []float64
	for k := 1; k <= 80; k++ {
		cx := start + speed*dt*float64(k)
		tracker.predict(track, dt)
		cluster := passingBoxCluster(rng, cx, cy, length, width)
		tracker.update(track, cluster, int64(k)*1e8)
		tracker.updateSolidBody(track, cluster)
		if track.TrackState == TrackDeleted || !sb.seeded {
			t.Fatalf("frame %d: the track was dropped", k)
		}
		state := [4]float32{track.X, track.Y, track.VX, track.VY}
		js := sb.jointOf(state, track.P)
		if !sb.ext.on {
			js = jointState{}
			for i := 0; i < 4; i++ {
				for j := 0; j < 4; j++ {
					js.p[i*jointStates+j] = float64(track.P[i*4+j])
				}
			}
		}
		r.worstAsym = math.Max(r.worstAsym, maxAsymmetry(js.p[:], jointStates))
		r.worstEigen = math.Min(r.worstEigen, minEigenvalue(js.p, jointStates))
		reading, ok := track.SolidBody()
		if !ok || reading.Measurement.Source != MeasurementNearEdgeCandidateV1 {
			continue
		}
		r.fixes++
		if sb.ext.on {
			joint, _ := reading.JointCovariance()
			for i := range joint {
				if float64(joint[i]) != float64(float32(js.p[i])) {
					r.readingDisagreements++
					break
				}
			}
		}
		err := math.Abs(float64(track.X) - cx)
		faces := reading.Measurement.Faces
		switch {
		case faces.Has(FaceFront):
			approach = append(approach, err)
			if !faces.Has(FaceRear) {
				r.lastFront = reading
			}
		case faces.Has(FaceRear):
			departure = append(departure, err)
			r.lastRear = reading
		}
	}
	if len(approach) < 10 || len(departure) < 10 {
		t.Fatalf("%d approach and %d departure end-face fixes, want ten of each", len(approach), len(departure))
	}
	r.approachErr = meanOf(approach[len(approach)/2:])
	r.departureErr = meanOf(departure[len(departure)/2:])
	if sb.ext.on {
		r.finalHalfLen = float64(sb.ext.h[0])
	} else {
		r.finalHalfLen = float64(sb.lengthBelief.Estimate()) / 2
	}
	return r
}

// shortExtentState seeds the extent block at the short belief's length, with
// the class prior's half-sigma, uncorrelated with the position.
func shortExtentState(sb *solidBodyTrack) {
	shortLengthBelief(sb)
	sigma := dimensionPriorFor(MotionRigidVehicle).sigmaMetres / 2
	sb.ext = solidBodyExtent{on: true, h: [2]float32{3.125 / 2, 0.9375},
		phh: [4]float32{sigma * sigma, 0, 0, sigma * sigma}}
}

// On the tracked path the box's side is in view throughout, so span admission
// corrects a fixed-extent belief by itself: the tracked filter without the
// option is the control, not a strawman. The extent state alone cannot use
// that span (only CensoredExtent or SpanBoundsBelief turn it into a bound on
// hL), so on approach one end face leaves n·c and hL confounded and the
// centre carries part of the shortfall; the second end face separates them,
// and the error falls.
func TestTrackedExtentStateRecoversAShortLengthAndTheCentre(t *testing.T) {
	fixed := runTrackedBoxPass(t, nearEdgeTrackingConfig(), shortLengthBelief)
	cfg := nearEdgeTrackingConfig()
	cfg.SolidBody.ExtentState = true
	state := runTrackedBoxPass(t, cfg, shortExtentState)
	cfg.SolidBody.SpanBoundsBelief = true
	bounded := runTrackedBoxPass(t, cfg, shortExtentState)
	t.Logf("tracked, extents fixed (span admission on): approach %.3f m, departure %.3f m, half-length %.3f m, %d fixes",
		fixed.approachErr, fixed.departureErr, fixed.finalHalfLen, fixed.fixes)
	t.Logf("tracked, extent state: approach %.3f m, departure %.3f m, half-length %.3f m (true 2.25), %d fixes, worst asymmetry %.2g, least eigenvalue %.3g",
		state.approachErr, state.departureErr, state.finalHalfLen, state.fixes, state.worstAsym, state.worstEigen)
	t.Logf("tracked, extent state with span bounds: approach %.3f m, departure %.3f m, half-length %.3f m, least eigenvalue %.3g",
		bounded.approachErr, bounded.departureErr, bounded.finalHalfLen, bounded.worstEigen)

	// The second end face fixes the length and the tracked centre comes with
	// it: below the approach's error, and no worse than the fixed extent's.
	if math.Abs(state.finalHalfLen-2.25) > 0.15 {
		t.Errorf("half-length %.3f m after both ends, want 2.25 m within 0.15 m", state.finalHalfLen)
	}
	if state.departureErr > 0.15 || state.departureErr >= state.approachErr/2 {
		t.Errorf("departure error %.3f m with the extent state, want under 0.15 m and under half the approach's %.3f m",
			state.departureErr, state.approachErr)
	}
	if state.departureErr > fixed.departureErr {
		t.Errorf("departure error %.3f m with the extent state, worse than the fixed extent's %.3f m", state.departureErr, fixed.departureErr)
	}
	// On this pass nothing but the faces moves the state, and the tracked
	// filter's prediction is the shadow's, so the tracked arm is the shadow
	// arm (runBoxPass) to float precision: the state machine is the same
	// joint filter whichever covariance it runs on.
	shadowCfg := solidBodyConfig()
	shadowCfg.SolidBody.ExtentState = true
	shadow := runBoxPass(t, shadowCfg, shortExtentState)
	for name, d := range map[string]float64{
		"approach":    state.approachErr - shadow.approachErr,
		"departure":   state.departureErr - shadow.departureErr,
		"half-length": state.finalHalfLen - shadow.finalHalfLen,
	} {
		if math.Abs(d) > 1e-4 {
			t.Errorf("tracked %s differs from the shadow's by %.3g", name, d)
		}
	}
	// The span bound is logged, not asserted: on this pass its truncation
	// overshoots hL on the shadow exactly as here (#970's behaviour, not the
	// tracked path's). Its covariance must still be a covariance.
	if bounded.worstAsym != 0 || bounded.worstEigen < -1e-6 {
		t.Errorf("with span bounds the joint covariance had asymmetry %.3g and eigenvalue %.3g", bounded.worstAsym, bounded.worstEigen)
	}
	if state.worstAsym != 0 {
		t.Errorf("the tracked joint covariance was asymmetric by %.3g", state.worstAsym)
	}
	if state.worstEigen < -1e-6 {
		t.Errorf("the tracked joint covariance had eigenvalue %.3g", state.worstEigen)
	}
	if state.readingDisagreements != 0 {
		t.Errorf("%d fixes published a joint covariance other than the tracked filter's", state.readingDisagreements)
	}

	// A face measures its surface, d·c + hL, better than either part, so
	// the parts are negatively correlated along d and the joint bound on the
	// surface just seen is tighter than the independent sum. The tracked
	// heading's variance here is the jitter prior's, which dominates both
	// bounds, so it is held out to compare the terms this change is about.
	for _, c := range []struct {
		name    string
		reading SolidBodyReading
		surface BodySurface
	}{{"front", state.lastFront, SurfaceFront}, {"rear", state.lastRear, SurfaceRear}} {
		e := c.reading.Estimate
		e.Orientation.VarianceRad2 = 0
		joint, ok := e.ProjectSurface(c.surface)
		if !ok || !joint.JointBound {
			t.Fatalf("%s: no joint bound at the last %s-face fix (ok %v, estimation %s)", c.name, c.name, ok, e.Estimation)
		}
		independent := e
		independent.ExtentState = false
		apart, ok := independent.ProjectSurface(c.surface)
		if !ok || apart.JointBound {
			t.Fatalf("%s: the independent bound is unavailable or claims to be joint", c.name)
		}
		t.Logf("%s surface after its own face: joint sigma %.3f m, independent sum %.3f m", c.name, joint.SigmaMetres, apart.SigmaMetres)
		if !(joint.SigmaMetres < apart.SigmaMetres) {
			t.Errorf("%s: joint sigma %.3f m is not below the independent %.3f m after the face measured it",
				c.name, joint.SigmaMetres, apart.SigmaMetres)
		}
	}
}

// randomJoint is a random symmetric positive-definite 6x6, A Aᵀ + I/10.
func randomJoint(rng *rand.Rand, scale float64) [jointStates * jointStates]float64 {
	var a, p [jointStates * jointStates]float64
	for i := range a {
		a[i] = rng.NormFloat64() * scale
	}
	for i := 0; i < jointStates; i++ {
		for j := 0; j < jointStates; j++ {
			var s float64
			for k := 0; k < jointStates; k++ {
				s += a[i*jointStates+k] * a[j*jointStates+k]
			}
			if i == j {
				s += 0.1
			}
			p[i*jointStates+j] = s
		}
	}
	return p
}

// setTrackedJoint makes a tracked body's tracked state and extent block one
// joint state.
func setTrackedJoint(track *TrackedObject, js jointState) {
	sb := &track.solidBody
	sb.ext.on = true
	sb.setJoint(js)
	sb.writeToTrack(track)
}

// The tracked medoid update, before the first fix, is the four-state update
// with the extent rows and columns of the same joint update: two scalar
// updates on the joint state, x then y, with the isotropic noise.
func TestTrackedMedoidUpdateIsTheJointUpdate(t *testing.T) {
	cfg := nearEdgeTrackingConfig()
	cfg.SolidBody.ExtentState = true
	for _, joseph := range []bool{false, true} {
		cfg.JosephCovarianceUpdate = joseph
		tracker := NewTracker(cfg)
		rng := rand.New(rand.NewSource(11))
		track := trackedBoxPassBody(3, 4, 2)
		track.ObservationCount = 1 // the initialisation window: the medoid update applies
		sb := &track.solidBody
		sb.reference = ReferenceClusterMedoid
		var js jointState
		js.x = [jointStates]float64{3, 4, 2, 0.5, 2.2, 0.9}
		js.p = randomJoint(rng, 0.3)
		setTrackedJoint(track, js)
		before := sb.jointOf([4]float32{track.X, track.Y, track.VX, track.VY}, track.P)

		cluster := passingBoxCluster(rng, 3.4, 4, 4.5, 1.8)
		tracker.update(track, cluster, 1e8)
		if !track.LastResidual.Valid || sb.pending.outcome != nearEdgeMedoid {
			t.Fatalf("joseph %v: not a medoid update (residual %v, outcome %v)", joseph, track.LastResidual.Valid, sb.pending.outcome)
		}
		z := track.LastResidual.Measurement
		want := before
		r := float64(cfg.MeasurementNoise)
		if !want.scalarUpdate([jointStates]float64{1}, float64(z.X), r) ||
			!want.scalarUpdate([jointStates]float64{0, 1}, float64(z.Y), r) {
			t.Fatal("reference joint update refused")
		}
		got := sb.jointOf([4]float32{track.X, track.Y, track.VX, track.VY}, track.P)
		for i := 0; i < jointStates; i++ {
			if d := math.Abs(got.x[i] - want.x[i]); d > 1e-5*math.Max(1, math.Abs(want.x[i])) {
				t.Errorf("joseph %v: state[%d] %.7g, want the joint update's %.7g", joseph, i, got.x[i], want.x[i])
			}
			for j := 0; j < jointStates; j++ {
				if d := math.Abs(got.p[i*jointStates+j] - want.p[i*jointStates+j]); d > 2e-5 {
					t.Errorf("joseph %v: P[%d][%d] %.7g, want %.7g", joseph, i, j, got.p[i*jointStates+j], want.p[i*jointStates+j])
				}
			}
		}
		if asym := maxAsymmetry(got.p[:], jointStates); asym != 0 {
			t.Errorf("joseph %v: joint covariance asymmetric by %.3g after the medoid update", joseph, asym)
		}
	}
}

// The tracked prediction propagates the cross block by F and adds the
// extents' process noise: the joint F P Fᵀ + Q, with F the constant-velocity
// transition on [x, y, vx, vy] and the identity on the extents.
func TestTrackedPredictionIsTheJointPrediction(t *testing.T) {
	cfg := nearEdgeTrackingConfig()
	cfg.SolidBody.ExtentState = true
	tracker := NewTracker(cfg)
	rng := rand.New(rand.NewSource(5))
	track := trackedBoxPassBody(0, 0, 1)
	var js jointState
	js.x = [jointStates]float64{0, 0, 1, 0, 2, 1}
	js.p = randomJoint(rng, 0.2)
	setTrackedJoint(track, js)
	before := track.solidBody.jointOf([4]float32{track.X, track.Y, track.VX, track.VY}, track.P)
	const dt = 0.1
	tracker.predict(track, dt)

	// F P Fᵀ over the six states, then the four-state process noise the
	// tracked filter adds and the extents' own.
	var f [jointStates * jointStates]float64
	for i := 0; i < jointStates; i++ {
		f[i*jointStates+i] = 1
	}
	f[0*jointStates+2], f[1*jointStates+3] = dt, dt
	var fp, want [jointStates * jointStates]float64
	for i := 0; i < jointStates; i++ {
		for j := 0; j < jointStates; j++ {
			for k := 0; k < jointStates; k++ {
				fp[i*jointStates+j] += f[i*jointStates+k] * before.p[k*jointStates+j]
			}
		}
	}
	for i := 0; i < jointStates; i++ {
		for j := 0; j < jointStates; j++ {
			for k := 0; k < jointStates; k++ {
				want[i*jointStates+j] += fp[i*jointStates+k] * f[j*jointStates+k]
			}
		}
	}
	var q [16]float32
	addProcessNoise(&q, cfg.ProcessNoisePos, cfg.ProcessNoiseVel, dt, cfg.CoupledProcessNoise)
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			want[i*jointStates+j] += float64(q[i*4+j])
		}
	}
	want[4*jointStates+4] += extentStateProcessNoise * dt
	want[5*jointStates+5] += extentStateProcessNoise * dt
	got := track.solidBody.jointOf([4]float32{track.X, track.Y, track.VX, track.VY}, track.P)
	for i := range want {
		if d := math.Abs(got.p[i] - want[i]); d > 1e-5 {
			t.Errorf("P[%d][%d] %.7g after the tracked prediction, want the joint F P Fᵀ + Q %.7g", i/jointStates, i%jointStates, got.p[i], want[i])
		}
	}
	if got.x[4] != js.x[4] || got.x[5] != js.x[5] {
		t.Errorf("the prediction moved the half-extents: %v %v", got.x[4], got.x[5])
	}
	if asym := maxAsymmetry(got.p[:], jointStates); asym != 0 {
		t.Errorf("joint covariance asymmetric by %.3g after the tracked prediction", asym)
	}
}

// The consistent speed clamp on the joint state leaves the four-state
// arithmetic bit for bit as it was and regresses the extent block on the
// same truncation: P' = P + (s² − σ²) k kᵀ over all six states.
func TestConsistentSpeedClampOnTheJointState(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	var js jointState
	js.x = [jointStates]float64{1, 2, 30, 10, 2.2, 0.9}
	js.p = randomJoint(rng, 0.4)
	var sb solidBodyTrack
	sb.ext.on = true
	sb.setJoint(js)
	before := sb.jointOf(sb.state, sb.p)
	const vmax = 25

	plainState, plainP := sb.state, sb.p
	constrainSpeedConsistent(&plainState, &plainP, vmax)
	jointStateX, jointP := sb.state, sb.p
	constrainSpeedConsistentJoint(&jointStateX, &jointP, &sb.ext, vmax)
	if plainState != jointStateX || plainP != jointP {
		t.Fatal("the joint clamp changed the four-state arithmetic")
	}

	vx, vy := before.x[2], before.x[3]
	speed := math.Hypot(vx, vy)
	g := [jointStates]float64{0, 0, vx / speed, vy / speed}
	var pg [jointStates]float64
	var sigma2 float64
	for i := 0; i < jointStates; i++ {
		for j := 0; j < jointStates; j++ {
			pg[i] += before.p[i*jointStates+j] * g[j]
		}
		sigma2 += g[i] * pg[i]
	}
	mean, variance := upperTruncatedNormalMoments(speed, math.Sqrt(sigma2), vmax)
	variance = math.Min(sigma2, math.Max(variance, speedClampVarianceFloor))
	d := variance - sigma2
	got := sb.jointOf(jointStateX, jointP)
	for i := 0; i < jointStates; i++ {
		k := pg[i] / sigma2
		if i >= 4 {
			if want := before.x[i] + k*(mean-speed); math.Abs(got.x[i]-want) > 1e-5 {
				t.Errorf("h[%d] %.7g, want %.7g", i-4, got.x[i], want)
			}
		}
		for j := 0; j < jointStates; j++ {
			want := before.p[i*jointStates+j] + d*k*pg[j]/sigma2
			if math.Abs(got.p[i*jointStates+j]-want) > 1e-5 {
				t.Errorf("P[%d][%d] %.7g, want %.7g", i, j, got.p[i*jointStates+j], want)
			}
		}
	}
	if e := minEigenvalue(got.p, jointStates); e < -1e-6 {
		t.Errorf("joint covariance eigenvalue %.3g after the clamp", e)
	}
}

// projectionEstimate is an estimate ProjectSurface accepts: established, at
// the body centre, axis at 0.3 rad resolved, length accumulated.
func projectionEstimate() SolidBodyEstimate {
	return SolidBodyEstimate{
		StateModel: StateModelCVCartesianV1,
		Reference:  ReferenceBodyCentre,
		X:          10, Y: 5,
		PositionCovariance: [4]float32{0.04, 0.01, 0.01, 0.09},
		Orientation:        OrientationBelief{PsiRad: 0.3, VarianceRad2: 1e-3, Provenance: ProvenanceObserved},
		Length:             DimensionBelief{Metres: 4.5, SigmaMetres: 0.4, AdmissibleFrames: 10, Provenance: ProvenanceAccumulated},
		Width:              DimensionBelief{Metres: 1.8, SigmaMetres: 0.2, AdmissibleFrames: 10, Provenance: ProvenanceAccumulated},
		Estimation:         EstimationEstablished,
	}
}

// The joint bound is uᵀP_cc u + Var(hL) + 2 dᵀP_ch, with d = +u for the front
// and -u for the rear, plus the orientation term: hand-computed here. A
// covariance kappa between u·c and hL raises the front's bound and lowers the
// rear's when positive, and the reverse when negative, against the
// independent sum the shipped formula takes.
func TestProjectSurfaceJointVarianceIsTheHandComputedForm(t *testing.T) {
	const psi = 0.3
	ux, uy := math.Cos(psi), math.Sin(psi)
	base := projectionEstimate()
	cc := ux*ux*0.04 + 2*ux*uy*0.01 + uy*uy*0.09
	const varH = 0.04 // (0.4 / 2)^2, the shipped dimVar
	ang := 2.25 * 2.25 * 1e-3
	independent := math.Sqrt(cc + varH + ang)

	off, ok := base.ProjectSurface(SurfaceFront)
	if !ok || off.JointBound || math.Abs(float64(off.SigmaMetres)-independent) > 1e-6 {
		t.Fatalf("without the extent state: sigma %.6f joint %v, want the independent %.6f and not joint", off.SigmaMetres, off.JointBound, independent)
	}
	if _, ok := base.ProjectSurfaceJoint(SurfaceFront); ok {
		t.Fatal("ProjectSurfaceJoint answered without the extent state")
	}

	for _, kappa := range []float64{0.015, -0.015} {
		e := base
		e.ExtentState = true
		// P_ch = kappa·u, so uᵀP_ch = kappa: x·hL and y·hL.
		e.PositionExtentCovariance = [4]float32{float32(kappa * ux), 0.002, float32(kappa * uy), -0.001}
		e.HalfExtentCovariance = [4]float32{varH, 0.003, 0.003, 0.01}
		for _, c := range []struct {
			s    BodySurface
			sign float64
		}{{SurfaceFront, 1}, {SurfaceRear, -1}} {
			got, ok := e.ProjectSurface(c.s)
			if !ok || !got.JointBound {
				t.Fatalf("kappa %v %s: unavailable or not joint", kappa, c.s)
			}
			want := math.Sqrt(cc + varH + 2*c.sign*kappa + ang)
			if math.Abs(float64(got.SigmaMetres)-want) > 1e-6 {
				t.Errorf("kappa %v %s: sigma %.6f, want the hand-computed %.6f", kappa, c.s, got.SigmaMetres, want)
			}
			wider := c.sign*kappa > 0
			if wider != (float64(got.SigmaMetres) > independent) {
				t.Errorf("kappa %v %s: joint %.6f against independent %.6f, want it %s", kappa, c.s, got.SigmaMetres, independent,
					map[bool]string{true: "wider", false: "narrower"}[wider])
			}
			if j, ok := e.ProjectSurfaceJoint(c.s); !ok || j != got {
				t.Errorf("kappa %v %s: ProjectSurfaceJoint %+v, want %+v", kappa, c.s, j, got)
			}
			// The surface's position does not depend on the bound.
			if want, _ := base.ProjectSurface(c.s); got.X != want.X || got.Y != want.Y {
				t.Errorf("kappa %v %s: the joint form moved the surface", kappa, c.s)
			}
		}
	}

	// A joint variance that is not positive is no bound at all.
	bad := base
	bad.ExtentState = true
	bad.HalfExtentCovariance = [4]float32{0.04, 0, 0, 0.01}
	bad.PositionExtentCovariance = [4]float32{float32(-ux), 0, float32(-uy), 0}
	if _, ok := bad.ProjectSurface(SurfaceFront); ok {
		t.Error("a negative joint variance was reported as a bound")
	}
}

// The reading's joint covariance round-trips through its blocks, refuses a
// matrix that is not symmetric across the blocks, and drops cleanly.
func TestReadingJointCovarianceRoundTrips(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	p64 := randomJoint(rng, 0.5)
	var p [JointCovarianceSize]float32
	for i := range p {
		p[i] = float32(p64[i])
	}
	for i := 0; i < jointStates; i++ {
		for j := i + 1; j < jointStates; j++ {
			p[j*jointStates+i] = p[i*jointStates+j]
		}
	}
	var r SolidBodyReading
	r.Estimate.StateModel = StateModelCVCartesianV1
	if _, ok := r.JointCovariance(); ok || r.CovarianceStateModel() != StateModelCVCartesianV1 {
		t.Fatal("a reading without the extent state has a joint covariance")
	}
	r, err := r.WithJointCovariance(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := r.JointCovariance(); !ok || got != p {
		t.Fatalf("joint covariance did not round-trip:\n%v\n%v", got, p)
	}
	if r.CovarianceStateModel() != StateModelCVCartesianExtentV1 || r.Estimate.StateModel != StateModelCVCartesianV1 {
		t.Errorf("state models: persisted %q, estimate %q", r.CovarianceStateModel(), r.Estimate.StateModel)
	}
	e := r.Estimate
	if !e.ExtentState || e.PositionCovariance != [4]float32{p[0], p[1], p[6], p[7]} ||
		e.PositionExtentCovariance != [4]float32{p[4], p[5], p[10], p[11]} ||
		e.HalfExtentCovariance != [4]float32{p[28], p[29], p[34], p[35]} {
		t.Errorf("estimate blocks are not the joint matrix's: %+v", e)
	}
	asym := p
	asym[1*jointStates+4] += 1e-3
	if _, err := r.WithJointCovariance(asym); err == nil {
		t.Error("an asymmetric joint covariance was accepted")
	}
	dropped := r.WithoutExtentCovariance()
	if dropped.ExtentState || dropped.Estimate.ExtentState || dropped.ExtentCrossCovariance != [8]float32{} ||
		dropped.Estimate.PositionExtentCovariance != [4]float32{} || dropped.CovarianceStateModel() != StateModelCVCartesianV1 ||
		dropped.Covariance != r.Covariance {
		t.Errorf("dropping the extent covariance left %+v", dropped)
	}
}
