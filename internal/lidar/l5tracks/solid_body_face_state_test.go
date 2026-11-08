package l5tracks

import (
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Tests for the face-state options (solid_body_face_state.go). What carries
// the weight: with ExtentState a body whose seeded length is short recovers
// it from an end face on approach and the other on departure, and its centre
// with it, where the shipped update stays half the shortfall out; the joint
// covariance stays symmetric positive semi-definite and a face adds exactly
// one rank of information; the censored extent does not narrow with frame
// count; the frame's span bounds the belief; the heading term is
// sigma_psi^2 t_eff^2; and the reference bias has the body's axes as its
// eigenvectors.

// passingBoxCluster is the returns a sensor at the origin sees from a box
// of the given length and width, axis +X, centred at (cx, cy) with cy > 0:
// its near side always, its +X end while the sensor is ahead of it and its
// -X end once the sensor is behind it, each return jittered by 2 cm.
func passingBoxCluster(rng *rand.Rand, cx, cy, length, width float64) WorldCluster {
	hl, hw := length/2, width/2
	var pts []l4perception.WorldPoint
	add := func(x, y float64) {
		pts = append(pts, l4perception.WorldPoint{X: x + 0.02*rng.NormFloat64(), Y: y + 0.02*rng.NormFloat64()})
	}
	for i := 0; i < 40; i++ {
		add(cx-hl+length*float64(i)/39, cy-hw)
	}
	for _, end := range []float64{cx + hl, cx - hl} {
		if (end == cx+hl && end < 0) || (end == cx-hl && end > 0) {
			for i := 0; i < 20; i++ {
				add(end, cy-hw+width*float64(i)/19)
			}
		}
	}
	var sx, sy float64
	for _, p := range pts {
		sx += p.X
		sy += p.Y
	}
	n := float64(len(pts))
	return WorldCluster{CentroidX: float32(sx / n), CentroidY: float32(sy / n), PointsCount: len(pts), Members: pts}
}

// boxPassBody is a confirmed body-centre solid body at (x, y) moving at vx
// along +X, its axis believed to within a degree.
func boxPassBody(x, y, vx float32) *TrackedObject {
	track := &TrackedObject{TrackID: "box", TrackState: TrackConfirmed, ObservationCount: 100}
	sb := &track.solidBody
	sb.seeded, sb.reference = true, ReferenceBodyCentre
	sb.estimation = EstimationGeometryConverging
	sb.state = [4]float32{x, y, vx, 0}
	sb.p = [16]float32{0.25, 0, 0, 0, 0, 0.25, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	sb.orientation = OrientationBelief{PsiRad: 0, VarianceRad2: 3e-4, Provenance: ProvenanceObserved}
	return track
}

// boxPassResult is one arm of the box pass.
type boxPassResult struct {
	departureErr float64 // mean |x - truth| over the departure end-face fixes
	approachErr  float64 // the same over the approach
	finalHalfLen float64 // hL at the end (ExtentState), or the belief's half
	worstAsym    float64 // largest |P - Pᵀ| over the run
	worstEigen   float64 // smallest eigenvalue of the joint covariance
	fixes        int
}

// runBoxPass drives the state machine over a 4.5 by 1.8 m box passing 5 m
// from the sensor at 5 m/s, from 20 m before it to 20 m after, frame by
// frame: predict, measure, step. Extent admission is not run, so the
// shipped arm's belief stays the short one it was given; that isolates the
// face update, which is what Section 3.1 changes.
func runBoxPass(t *testing.T, cfg TrackerConfig, setup func(*solidBodyTrack)) boxPassResult {
	t.Helper()
	const length, width, cy, speed, dt = 4.5, 1.8, 5.0, 5.0, 0.1
	tracker := NewTracker(cfg)
	start := -20.0
	track := boxPassBody(float32(start), cy, speed)
	sb := &track.solidBody
	setup(sb)
	prior := dimensionPriorFor(MotionRigidVehicle)
	rng := rand.New(rand.NewSource(1))
	r := boxPassResult{worstEigen: math.Inf(1)}
	var approach, departure []float64
	for k := 1; k <= 80; k++ {
		cx := start + speed*dt*float64(k)
		tracker.predictSolidBodyStep(track, dt)
		cluster := passingBoxCluster(rng, cx, cy, length, width)
		f := tracker.measureNearEdgeFrame(sb, cluster, prior, 100, false)
		m, outcome, _ := tracker.stepNearEdge(sb, cluster, prior, f)
		if !sb.seeded {
			t.Fatalf("frame %d: the body was dropped", k)
		}
		js := sb.jointOf(sb.state, sb.p)
		if !sb.ext.on {
			js = jointState{}
			for i := 0; i < 4; i++ {
				for j := 0; j < 4; j++ {
					js.p[i*jointStates+j] = float64(sb.p[i*4+j])
				}
			}
		}
		r.worstAsym = math.Max(r.worstAsym, maxAsymmetry(js.p[:], jointStates))
		r.worstEigen = math.Min(r.worstEigen, minEigenvalue(js.p, jointStates))
		if outcome != nearEdgeFix {
			continue
		}
		r.fixes++
		err := math.Abs(float64(sb.state[0]) - cx)
		switch {
		case m.Faces.Has(FaceFront):
			approach = append(approach, err)
		case m.Faces.Has(FaceRear):
			departure = append(departure, err)
		}
	}
	if len(approach) < 10 || len(departure) < 10 {
		t.Fatalf("%d approach and %d departure end-face fixes, want ten of each", len(approach), len(departure))
	}
	// Settled: the second half of each phase.
	r.approachErr = meanOf(approach[len(approach)/2:])
	r.departureErr = meanOf(departure[len(departure)/2:])
	if sb.ext.on {
		r.finalHalfLen = float64(sb.ext.h[0])
	} else {
		r.finalHalfLen = float64(sb.lengthBelief.Estimate()) / 2
	}
	return r
}

// shortLengthBelief gives a body a corroborated 3.125 m length and 1.875 m
// width: the F0 report's short belief, 1.375 m under the box.
func shortLengthBelief(sb *solidBodyTrack) {
	for i := 0; i < 10; i++ {
		sb.lengthBelief.Observe(3.0)
		sb.widthBelief.Observe(1.8)
	}
}

func TestExtentStateRecoversAShortLengthAndTheCentre(t *testing.T) {
	shipped := runBoxPass(t, solidBodyConfig(), shortLengthBelief)

	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	short := float32(3.125)
	state := runBoxPass(t, cfg, func(sb *solidBodyTrack) {
		// The same short length, seeded with the class prior's half-sigma
		// rather than the shrunk sigma the shipped belief would report.
		shortLengthBelief(sb)
		sigma := dimensionPriorFor(MotionRigidVehicle).sigmaMetres / 2
		sb.ext = solidBodyExtent{on: true, h: [2]float32{short / 2, 0.9375},
			phh: [4]float32{sigma * sigma, 0, 0, sigma * sigma}}
	})
	t.Logf("shipped: approach %.3f m, departure %.3f m, half-length %.3f m, %d fixes",
		shipped.approachErr, shipped.departureErr, shipped.finalHalfLen, shipped.fixes)
	t.Logf("extent state: approach %.3f m, departure %.3f m, half-length %.3f m (true 2.25), %d fixes, worst asymmetry %.2g, least eigenvalue %.3g",
		state.approachErr, state.departureErr, state.finalHalfLen, state.fixes, state.worstAsym, state.worstEigen)

	// The shipped update places the centre half the shortfall toward the
	// sensor, 0.69 m, on both ends.
	halfShortfall := (4.5 - float64(short)) / 2
	for name, e := range map[string]float64{"approach": shipped.approachErr, "departure": shipped.departureErr} {
		if math.Abs(e-halfShortfall) > 0.1 {
			t.Errorf("shipped %s error %.3f m, want half the shortfall, %.3f m", name, e, halfShortfall)
		}
	}
	// With the extent in the state, the second end face fixes the length,
	// and the centre comes with it.
	if math.Abs(state.finalHalfLen-2.25) > 0.15 {
		t.Errorf("half-length %.3f m after both ends, want 2.25 m within 0.15 m", state.finalHalfLen)
	}
	if state.departureErr > 0.15 || state.departureErr > shipped.departureErr/3 {
		t.Errorf("departure error %.3f m with the extent in the state, want under 0.15 m and a third of the shipped %.3f m",
			state.departureErr, shipped.departureErr)
	}
	if state.worstAsym != 0 {
		t.Errorf("the joint covariance was asymmetric by %.3g", state.worstAsym)
	}
	if state.worstEigen < -1e-6 {
		t.Errorf("the joint covariance had eigenvalue %.3g", state.worstEigen)
	}
}

// One face is one rank of information: in information form the update adds
// exactly h hᵀ / R, whatever the prior, so along any direction H does not
// see, the face says nothing the prior's correlation does not.
func TestAFaceAddsOneRankOfInformationToTheJointState(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var js jointState
	// A random symmetric positive-definite prior: A Aᵀ + I/10.
	var a [jointStates * jointStates]float64
	for i := range a {
		a[i] = rng.NormFloat64() * 0.5
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
			js.p[i*jointStates+j] = s
		}
	}
	prior := js.p
	nx, ny := math.Cos(0.4), math.Sin(0.4)
	h := [jointStates]float64{nx, ny, 0, 0, 1, 0}
	const r = 0.05
	if !js.scalarUpdate(h, 3, r) {
		t.Fatal("the update refused a positive innovation variance")
	}
	before, ok1 := invert(prior)
	after, ok2 := invert(js.p)
	if !ok1 || !ok2 {
		t.Fatal("singular covariance")
	}
	for i := 0; i < jointStates; i++ {
		for j := 0; j < jointStates; j++ {
			want := before[i*jointStates+j] + h[i]*h[j]/r
			if got := after[i*jointStates+j]; math.Abs(got-want) > 1e-6*math.Max(1, math.Abs(want)) {
				t.Fatalf("information[%d][%d] %.9g, want the prior's plus h hᵀ/R, %.9g", i, j, got, want)
			}
		}
	}

	// With n·c and hL uncorrelated and equally uncertain, the face measures
	// their sum and leaves their difference exactly as it was: a single face
	// cannot separate the centre from the half-extent.
	var flat jointState
	for i := 0; i < jointStates; i++ {
		flat.p[i*jointStates+i] = 0.5
	}
	h = [jointStates]float64{1, 0, 0, 0, 1, 0}
	diff := [jointStates]float64{1, 0, 0, 0, -1, 0}
	sumVar, diffVar := quadratic(flat.p, h), quadratic(flat.p, diff)
	flat.scalarUpdate(h, 1, r)
	if got := quadratic(flat.p, diff); math.Abs(got-diffVar) > 1e-12 {
		t.Errorf("var(n·c - hL) %.6g after one face, was %.6g", got, diffVar)
	}
	if got, want := quadratic(flat.p, h), sumVar*r/(sumVar+r); math.Abs(got-want) > 1e-12 {
		t.Errorf("var(n·c + hL) %.6g after one face, want the scalar posterior %.6g", got, want)
	}
}

// The joint update off, a face's update on the joint state with the extent
// block uncorrelated and the half-extent held fixed (zero variance) is the
// four-state update of the plane less the half-extent.
func TestTheJointFaceUpdateWithAFixedExtentIsTheShippedUpdate(t *testing.T) {
	var sb solidBodyTrack
	sb.ext = solidBodyExtent{on: true, h: [2]float32{2.25, 0.9}}
	state := [4]float32{0.1, -0.2, 5, 0.3}
	p := [16]float32{0.2, 0.05, 0.1, 0.02, 0.05, 0.12, 0.01, 0.06, 0.1, 0.01, 1, 0.1, 0.02, 0.06, 0.1, 0.8}
	face := EdgeMeasurement{Face: FaceFront, NormalX: 1, PlaneOffsetMetres: 2.6, HalfExtentMetres: 2.25,
		HalfExtentProvenance: ProvenanceAccumulated}
	tracker := NewTracker(solidBodyConfig())
	wantState, wantP, wantNIS, _, ok := tracker.applyEdgeMeasurements(state, p, []EdgeMeasurement{face}, nil, nil)
	if !ok {
		t.Fatal("shipped update refused")
	}
	js := sb.jointOf(state, p)
	nis, _, ok := tracker.applyEdgeMeasurementsJoint(&js, []EdgeMeasurement{face}, nil)
	if !ok {
		t.Fatal("joint update refused")
	}
	for i := 0; i < 4; i++ {
		if math.Abs(js.x[i]-float64(wantState[i])) > 1e-5 {
			t.Errorf("state[%d] %.6f, want %.6f", i, js.x[i], wantState[i])
		}
		for j := 0; j < 4; j++ {
			if math.Abs(js.p[i*jointStates+j]-float64(wantP[i*4+j])) > 1e-5 {
				t.Errorf("P[%d][%d] %.6f, want %.6f", i, j, js.p[i*jointStates+j], wantP[i*4+j])
			}
		}
	}
	if math.Abs(float64(nis-wantNIS)) > 1e-5 || js.x[4] != 2.25 {
		t.Errorf("NIS %.6f (want %.6f), hL %.6f (want it untouched)", nis, wantNIS, js.x[4])
	}
}

// Under ExtentState a class-prior half-extent is not a reason to drop a
// face: the prior's uncertainty is in P.
func TestExtentStateUsesAFaceWhoseExtentIsThePrior(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	tracker := NewTracker(cfg)
	track := boxPassBody(-10, 5, 5)
	sb := &track.solidBody
	prior := dimensionPriorFor(MotionRigidVehicle)
	seedSolidBodyExtent(sb, prior)
	cluster := passingBoxCluster(rand.New(rand.NewSource(3)), -10, 5, 4.5, 1.8)
	f := tracker.measureNearEdgeFrame(sb, cluster, prior, 100, false)
	if f.length.Provenance != ProvenanceClassPrior || f.length.Metres != prior.lengthMetres {
		t.Fatalf("seeded length %+v, want the class prior", f.length)
	}
	m, outcome, _ := tracker.stepNearEdge(sb, cluster, prior, f)
	if outcome != nearEdgeFix || m.Rank != 2 || m.InferredExtent {
		t.Fatalf("outcome %v rank %d inferred %v, want a rank-two fix on prior extents", outcome, m.Rank, m.InferredExtent)
	}
	if d := sb.extentDimension(0); d.Provenance != ProvenanceAccumulated || d.AdmissibleFrames != 1 {
		t.Fatalf("length after one face %+v, want accumulated over one frame", d)
	}
	// The shipped path refuses the same frame.
	plain := NewTracker(solidBodyConfig())
	track = boxPassBody(-10, 5, 5)
	f = plain.measureNearEdgeFrame(&track.solidBody, cluster, prior, 100, false)
	if m, _, _ := plain.stepNearEdge(&track.solidBody, cluster, prior, f); m.FallbackReason != "class_prior_extent" {
		t.Fatalf("shipped fallback %q, want class_prior_extent", m.FallbackReason)
	}
}

// Through the whole tracker the face-state options never feed back into
// the tracked state, and the joint covariance stays a covariance.
func TestFaceStateOptionsLeaveTheTrackedStateAlone(t *testing.T) {
	for _, pass := range []l4perception.SyntheticPass{l4perception.DefaultSyntheticPass(), endOnTruckPass()} {
		frames := syntheticPassFrames(t, pass)
		withBody := solidBodyConfig()
		withBody.SolidBody.CourseAlignedFaces = true
		withBody.SolidBody.Containment = true
		withBody.SolidBody.ExtentState = true
		withBody.SolidBody.CensoredExtent = true
		withBody.SolidBody.SpanBoundsBelief = true
		withBody.SolidBody.HeadingFaceNoise = true
		withBody.SolidBody.AnisotropicReferenceBias = true
		off, on := NewTracker(DefaultTrackerConfig()), NewTracker(withBody)
		fixes := 0
		for i, f := range frames {
			off.Update(f.clusters, f.at)
			on.Update(f.clusters, f.at)
			a, b := off.GetAllTracks(), on.GetAllTracks()
			if len(a) != len(b) {
				t.Fatalf("frame %d: %d tracks off, %d on", i, len(a), len(b))
			}
			bySeq := map[int64]*TrackedObject{}
			for _, tr := range b {
				bySeq[tr.CreationSequence] = tr
			}
			for _, tr := range a {
				other, ok := bySeq[tr.CreationSequence]
				if !ok || !reflect.DeepEqual(comparableTrack(tr), comparableTrack(other)) {
					t.Fatalf("frame %d: track %d differs with the face-state options on", i, tr.CreationSequence)
				}
			}
			for _, tr := range b {
				sb := &tr.solidBody
				if !sb.seeded {
					continue
				}
				if !sb.ext.on {
					t.Fatalf("frame %d: a shadow body without its extent block", i)
				}
				js := sb.jointOf(sb.state, sb.p)
				if asym := maxAsymmetry(js.p[:], jointStates); asym != 0 {
					t.Fatalf("frame %d: joint covariance asymmetric by %.3g", i, asym)
				}
				if e := minEigenvalue(js.p, jointStates); e < -1e-5 {
					t.Fatalf("frame %d: joint covariance eigenvalue %.3g", i, e)
				}
				if r, ok := tr.SolidBody(); ok && r.Measurement.Source == MeasurementNearEdgeCandidateV1 {
					fixes++
				}
			}
		}
		if fixes == 0 {
			t.Fatal("no near-edge fix with the face-state options on")
		}
	}
}

// Under NearEdgeTracking with ExtentState the tracked filter carries the
// extent block: every tracked body has one, the joint covariance stays a
// covariance through the whole tracker, and the published reading's joint
// covariance is the tracked filter's 4x4 with the body's extent blocks.
func TestTheTrackedPathCarriesExtentState(t *testing.T) {
	frames := syntheticPassFrames(t, endOnTruckPass())
	cfg := nearEdgeTrackingConfig()
	cfg.SolidBody.CourseAlignedFaces = true
	plain := solidBodyFingerprint(t, cfg, frames)
	cfg.SolidBody.ExtentState = true
	withExtent := solidBodyFingerprint(t, cfg, frames)
	if withExtent == plain {
		t.Fatal("ExtentState left the tracked path unchanged; the faces should measure n·c + h")
	}
	if again := solidBodyFingerprint(t, cfg, frames); again != withExtent {
		t.Fatalf("the tracked extent state is not deterministic: %s then %s", withExtent, again)
	}
	tracker := NewTracker(cfg)
	fixes := 0
	for i, f := range frames {
		tracker.Update(f.clusters, f.at)
		for _, tr := range tracker.GetAllTracks() {
			sb := &tr.solidBody
			if !sb.seeded || tr.TrackState == TrackDeleted {
				continue
			}
			if !sb.tracked || !sb.ext.on {
				t.Fatalf("frame %d: a tracked body without its extent block (tracked %v, ext %v)", i, sb.tracked, sb.ext.on)
			}
			js := sb.jointOf([4]float32{tr.X, tr.Y, tr.VX, tr.VY}, tr.P)
			if asym := maxAsymmetry(js.p[:], jointStates); asym != 0 {
				t.Fatalf("frame %d: tracked joint covariance asymmetric by %.3g", i, asym)
			}
			if e := minEigenvalue(js.p, jointStates); e < -1e-5 {
				t.Fatalf("frame %d: tracked joint covariance eigenvalue %.3g", i, e)
			}
			r, ok := tr.SolidBody()
			if !ok || !r.ExtentState || !r.Estimate.ExtentState {
				t.Fatalf("frame %d: reading without the extent state", i)
			}
			if r.Measurement.Source != MeasurementNearEdgeCandidateV1 {
				continue
			}
			// At a fix the reading was published after the tracked step, so
			// its blocks are the tracked filter's.
			fixes++
			joint, _ := r.JointCovariance()
			for k := range joint {
				if float64(joint[k]) != float64(float32(js.p[k])) {
					t.Fatalf("frame %d: published joint covariance [%d] %v, tracked %v", i, k, joint[k], js.p[k])
				}
			}
			if r.Estimate.PositionExtentCovariance != [4]float32{sb.ext.pxh[0], sb.ext.pxh[1], sb.ext.pxh[2], sb.ext.pxh[3]} ||
				r.Estimate.HalfExtentCovariance != sb.ext.phh {
				t.Fatalf("frame %d: the estimate's extent blocks are not the reading's", i)
			}
		}
	}
	if fixes == 0 {
		t.Fatal("no tracked near-edge fix with the extent state on")
	}
}

func TestCensoredExtentDoesNotNarrowWithFrameCount(t *testing.T) {
	prior := dimensionPriorFor(MotionRigidVehicle)
	var first DimensionBelief
	for i, n := range []int{1, 3, 10, 36, 200} {
		var b extentBelief
		for k := 0; k < n; k++ {
			b.Observe(5.6)
		}
		d := censoredDimension(b, prior.lengthMetres, prior.sigmaMetres, 0)
		shipped := dimensionFromBelief(b, prior.lengthMetres, prior.sigmaMetres)
		t.Logf("%3d frames at 5.6 m: censored %.3f ± %.3f m, shipped %.3f ± %.3f m",
			n, d.Metres, d.SigmaMetres, shipped.Metres, shipped.SigmaMetres)
		// The bound is the bin's lower edge less its noise.
		lower := float32(5.5 - censoredHistoryNoiseMetres)
		if d.Metres < lower || d.Provenance != ProvenanceAccumulated || d.AdmissibleFrames != b.Support {
			t.Fatalf("%d frames: %+v, want a mean at or above %.2f m, accumulated", n, d, lower)
		}
		if i == 0 {
			first = d
		} else if d.SigmaMetres != first.SigmaMetres || d.Metres != first.Metres {
			t.Fatalf("%d identical bounds gave %+v, one gave %+v", n, d, first)
		}
	}
	// A bound below the prior's mass (2.7 sigma under it) leaves the prior
	// within a few centimetres; one above it moves
	// the mean above the bound and narrows the sigma below the prior's.
	low := censoredDimension(extentBelief{}, prior.lengthMetres, prior.sigmaMetres, 0.5)
	if math.Abs(float64(low.Metres-prior.lengthMetres)) > 0.03 || math.Abs(float64(low.SigmaMetres-prior.sigmaMetres)) > 0.03 {
		t.Errorf("a 0.5 m bound moved the prior to %+v", low)
	}
	high := censoredDimension(extentBelief{}, prior.lengthMetres, prior.sigmaMetres, 9)
	if high.Metres < 9 || high.SigmaMetres >= prior.sigmaMetres || high.Provenance != ProvenanceClassPrior {
		t.Errorf("a 9 m bound gave %+v", high)
	}
	// Off, the shipped belief does narrow with frame count: the defect.
	var b extentBelief
	for k := 0; k < 36; k++ {
		b.Observe(5.6)
	}
	if s := extentBeliefSigma(b, prior.sigmaMetres); s > 0.26 {
		t.Errorf("the shipped sigma after 36 frames is %.3f m; the comparison above assumes it shrinks to the bin", s)
	}
}

func TestLowerTruncatedMomentsMatchTheTwoSidedFormAndTheTail(t *testing.T) {
	for _, lower := range []float64{-3, -0.5, 0, 1, 2.5, 4} {
		mean, variance := lowerTruncatedMoments(0, 1, lower)
		wantMean, wantVar, ok := truncatedNormalMoments(0, 1, lower, lower+40)
		if !ok || math.Abs(mean-wantMean) > 1e-9 || math.Abs(variance-wantVar) > 1e-7 {
			t.Errorf("lower %.1f: %.9f, %.9f; two-sided %.9f, %.9f", lower, mean, variance, wantMean, wantVar)
		}
	}
	// Far in the tail, the Mills-ratio asymptote: mean lower + 1/lower,
	// standard deviation 1/lower.
	mean, variance := lowerTruncatedMoments(0, 1, 50)
	if math.Abs(mean-50.02) > 1e-3 || math.Abs(math.Sqrt(variance)-0.02) > 1e-3 {
		t.Errorf("tail at 50 sigma: mean %.4f, sd %.4f", mean, math.Sqrt(variance))
	}
}

// Under ExtentState a bound truncates the half-extent once; repeating it
// narrows nothing, and a higher bound applies.
func TestExtentStateAppliesABoundOnceAndAHigherOneAgain(t *testing.T) {
	var sb solidBodyTrack
	sb.p = [16]float32{0.1, 0, 0, 0, 0, 0.1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	sb.ext = solidBodyExtent{on: true, h: [2]float32{2.25, 0.95}, phh: [4]float32{0.5625, 0, 0, 0.5625},
		pxh: [8]float32{0.05, 0, 0, 0, 0, 0, 0, 0}}
	bounds := [2]float32{6, 0}
	if touched := sb.applyExtentBounds(bounds); !touched[0] || touched[1] {
		t.Fatalf("touched %v, want the length alone", touched)
	}
	h, v := sb.ext.h[0], sb.ext.phh[0]
	if h < 3 || v >= 0.5625 {
		t.Fatalf("after a 6 m bound hL %.3f m with variance %.3f, want at least 3 m and narrower", h, v)
	}
	// The position moved by its covariance with hL.
	if sb.state[0] == 0 {
		t.Fatal("the bound did not move the correlated position")
	}
	for i := 0; i < 20; i++ {
		if touched := sb.applyExtentBounds(bounds); touched[0] {
			t.Fatalf("the same bound applied again on repeat %d", i)
		}
	}
	if sb.ext.h[0] != h || sb.ext.phh[0] != v {
		t.Fatalf("repeated bounds moved hL to %.4f with variance %.4f", sb.ext.h[0], sb.ext.phh[0])
	}
	if touched := sb.applyExtentBounds([2]float32{6.5, 0}); !touched[0] || sb.ext.h[0] <= h {
		t.Fatalf("a higher bound did not apply: %v, hL %.3f", touched, sb.ext.h[0])
	}
}

func TestSpanBoundsBeliefHoldsTheBeliefAtTheFramesSpan(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.SpanBoundsBelief = true
	tracker := NewTracker(cfg)
	prior := dimensionPriorFor(MotionRigidVehicle)
	track := boxPassBody(-10, 5, 5)
	sb := &track.solidBody
	for i := 0; i < 10; i++ {
		sb.lengthBelief.Observe(2.0)
		sb.widthBelief.Observe(1.0)
	}
	cluster := passingBoxCluster(rand.New(rand.NewSource(5)), -10, 5, 4.5, 1.8)
	f := tracker.measureNearEdgeFrame(sb, cluster, prior, 100, false)
	along, across := windowMinimumSpans(nearEdgePoints(cluster), 0)
	noise := float32(spanBoundNoise(math.Hypot(-10, 5)))
	if f.length.Metres < along-noise-1e-6 || f.width.Metres < across-noise-1e-6 {
		t.Fatalf("belief %.3f by %.3f m under the frame's spans less noise, %.3f by %.3f m",
			f.length.Metres, f.width.Metres, along-noise, across-noise)
	}
	if f.length.Metres > along || f.length.Provenance != ProvenanceAccumulated {
		t.Fatalf("length %+v: want the bound %.3f m, not more, keeping its provenance", f.length, along-noise)
	}
	ends := 0
	for _, e := range f.edges.Edges {
		if e.Face.IsLongitudinal() {
			ends++
			if e.HalfExtentMetres < (along-noise)/2-1e-6 {
				t.Fatalf("the end face measured with half-extent %.3f m, under half the bound", e.HalfExtentMetres)
			}
		}
	}
	if ends != 1 {
		t.Fatalf("%d end faces on an approaching box, want one", ends)
	}
	// Off, the same frame keeps the short belief.
	plain := NewTracker(solidBodyConfig())
	if g := plain.measureNearEdgeFrame(sb, cluster, prior, 100, false); g.length.Metres > 2.5 {
		t.Fatalf("without the option the length is %.3f m", g.length.Metres)
	}
	// A merge candidate's span is two bodies', not a bound on one.
	if g := tracker.measureNearEdgeFrame(sb, cluster, prior, 100, true); g.spanBound != [2]float32{} {
		t.Fatalf("a merge candidate gave bounds %v", g.spanBound)
	}
	// With CensoredExtent the bound joins the posterior: the mean is above it.
	cfg.SolidBody.CensoredExtent = true
	censored := NewTracker(cfg).measureNearEdgeFrame(sb, cluster, prior, 100, false)
	if censored.length.Metres < along-noise {
		t.Fatalf("censored length %.3f m under the bound %.3f m", censored.length.Metres, along-noise)
	}
}

func TestHeadingFaceNoiseAddsTheLeverArmsVariance(t *testing.T) {
	// A front face 20 returns wide at x = 2, from y = -0.9 to 0.9, seen from
	// the sensor ahead of it, and a body predicted at the origin.
	var points []l4perception.WorldPoint
	for i := 0; i < 20; i++ {
		points = append(points, l4perception.WorldPoint{X: 2, Y: -0.9 + 1.8*float64(i)/19})
	}
	for i := 0; i < 20; i++ {
		points = append(points, l4perception.WorldPoint{X: -2 + 4*float64(i)/19, Y: -0.9})
	}
	in := NearEdgeInput{
		Points: points, SensorX: 30, SensorY: 0, HeadingRad: 0,
		HalfLength: 2, HalfWidth: 0.9, LengthProvenance: ProvenanceAccumulated, WidthProvenance: ProvenanceAccumulated,
		TangentialLeverArms: true,
	}
	set := MeasureNearEdge(in)
	var front EdgeMeasurement
	for _, e := range set.Edges {
		if e.Face == FaceFront {
			front = e
		}
	}
	if front.SupportPoints == 0 {
		t.Fatal("no front face")
	}
	// The support is the returns within tolerance of the plane; their RMS
	// offset along the plane from the origin, by hand.
	var sum float64
	n := 0
	for _, p := range points {
		if float32(p.X) >= front.PlaneOffsetMetres-DefaultFaceToleranceMetres {
			sum += p.Y * p.Y
			n++
		}
	}
	want := math.Sqrt(sum / float64(n))
	if n != front.SupportPoints || math.Abs(float64(front.TangentialLeverArmMetres)-want) > 1e-6 {
		t.Fatalf("lever arm %.6f m over %d returns, want %.6f m over %d", front.TangentialLeverArmMetres, front.SupportPoints, want, n)
	}
	in.TangentialLeverArms = false
	if off := MeasureNearEdge(in); off.Edges[0].TangentialLeverArmMetres != 0 {
		t.Fatal("a lever arm without asking for one")
	}

	// The update with the heading term is the shipped update at R plus
	// sigma_psi^2 t_eff^2.
	const sigmaPsi2 = 0.03
	added := headingFaceVariance(true, []EdgeMeasurement{front}, sigmaPsi2, nil)
	if len(added) != 1 || math.Abs(added[0]-sigmaPsi2*want*want) > 1e-9 {
		t.Fatalf("added variance %v, want %.6g", added, sigmaPsi2*want*want)
	}
	if headingFaceVariance(false, []EdgeMeasurement{front}, sigmaPsi2, nil) != nil {
		t.Fatal("a heading term with the option off")
	}
	tracker := NewTracker(solidBodyConfig())
	state := [4]float32{0, 0, 1, 0}
	p := [16]float32{0.2, 0, 0, 0, 0, 0.2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	got, gotP, _, _, _ := tracker.applyEdgeMeasurements(state, p, []EdgeMeasurement{front}, nil, added)
	wantState, wantP := state, p
	scalarPositionUpdate(&wantState, &wantP, 1, 0, float64(front.ImpliedCentreOffset()), float64(tracker.Config.MeasurementNoise)+added[0])
	if got != wantState || gotP != wantP {
		t.Fatalf("update with the heading term %v, want %v", got, wantState)
	}
}

func TestAnisotropicReferenceBiasHasTheBodyAxesAsEigenvectors(t *testing.T) {
	prior := dimensionPriorFor(MotionRigidVehicle)
	a2 := float64(prior.lengthMetres/2) * float64(prior.lengthMetres/2)
	b2 := float64(prior.widthMetres/2) * float64(prior.widthMetres/2)
	for _, psi := range []float64{0, 0.3, math.Pi / 2, 2.2, -1} {
		m := anisotropicReferenceBias(prior, psi)
		xx, xy, yy := float64(m[0]), float64(m[1]), float64(m[2])
		u := [2]float64{math.Cos(psi), math.Sin(psi)}
		v := [2]float64{-u[1], u[0]}
		for _, c := range []struct {
			dir  [2]float64
			want float64
		}{{u, a2}, {v, b2}} {
			mx := xx*c.dir[0] + xy*c.dir[1]
			my := xy*c.dir[0] + yy*c.dir[1]
			if math.Abs(mx-c.want*c.dir[0]) > 1e-5 || math.Abs(my-c.want*c.dir[1]) > 1e-5 {
				t.Errorf("psi %.2f: M d = (%.5f, %.5f), want %.5f d = (%.5f, %.5f)", psi, mx, my, c.want, c.want*c.dir[0], c.want*c.dir[1])
			}
		}
	}

	// At a re-reference the fix's starting covariance gains the matrix; the
	// state machine is checked through the fix's prior, by a rank-one fix
	// along the axis whose gain reads the along-axis variance.
	cfg := solidBodyConfig()
	cfg.SolidBody.AnisotropicReferenceBias = true
	tracker := NewTracker(cfg)
	track := boxPassBody(-10, 5, 5)
	sb := &track.solidBody
	sb.reference = ReferenceClusterMedoid
	sb.p = [16]float32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	for i := 0; i < 10; i++ {
		sb.lengthBelief.Observe(4.5)
		sb.widthBelief.Observe(1.8)
	}
	face := EdgeMeasurement{Face: FaceFront, NormalX: 1, PlaneOffsetMetres: -10 + 2.25, HalfExtentMetres: 2.25,
		HalfExtentProvenance: ProvenanceAccumulated, SupportPoints: 20}
	f := nearEdgeFrame{edges: EdgeMeasurementSet{Rank: 1, Edges: []EdgeMeasurement{face}}, axis: 0,
		length: DimensionBelief{Metres: 4.5}, width: DimensionBelief{Metres: 1.8}}
	m, _, _ := tracker.stepNearEdge(sb, WorldCluster{CentroidX: -7.75, CentroidY: 4.1}, prior, f)
	if m.ReferenceChange != ReferenceToBodyCentre {
		t.Fatalf("no re-reference: %+v", m)
	}
	// Across the axis nothing measured it, so the variance is (W/2)^2; along
	// it, the posterior of (L/2)^2 against R.
	r := float64(cfg.MeasurementNoise)
	if got := float64(sb.p[5]); math.Abs(got-b2) > 1e-5 {
		t.Errorf("across-axis variance %.5f, want (W/2)^2 = %.5f", got, b2)
	}
	if got, want := float64(sb.p[0]), a2*r/(a2+r); math.Abs(got-want) > 1e-5 {
		t.Errorf("along-axis variance %.5f, want (L/2)^2 updated by R, %.5f", got, want)
	}

	// A lapse with an orientation is anisotropic; without one, and at the
	// seed, isotropic at (L/2)^2.
	tracker.lapseSolidBodyToMedoid(sb, WorldCluster{CentroidX: 1, CentroidY: 2}, prior)
	if math.Abs(float64(sb.p[0])-a2) > 1e-5 || math.Abs(float64(sb.p[5])-b2) > 1e-5 || sb.p[1] != 0 {
		t.Errorf("lapse along psi 0: %v", sb.p[:6])
	}
	sb.orientation.Provenance = ProvenanceNone
	tracker.lapseSolidBodyToMedoid(sb, WorldCluster{CentroidX: 1, CentroidY: 2}, prior)
	if math.Abs(float64(sb.p[0])-a2) > 1e-5 || math.Abs(float64(sb.p[5])-a2) > 1e-5 {
		t.Errorf("lapse without an axis: %v, want (L/2)^2 on both", sb.p[:6])
	}
	if got := isotropicReferenceBias(dimensionPriorFor(MotionPedestrian)); math.Abs(float64(got)-0.09) > 1e-6 {
		t.Errorf("pedestrian isotropic bias %.4f, want 0.09", got)
	}
}

// maxAsymmetry is the largest |P[i][j] - P[j][i]|.
func maxAsymmetry(p []float64, n int) float64 {
	worst := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			worst = math.Max(worst, math.Abs(p[i*n+j]-p[j*n+i]))
		}
	}
	return worst
}

// minEigenvalue is the smallest eigenvalue of a symmetric matrix, by cyclic
// Jacobi rotations.
func minEigenvalue(p [jointStates * jointStates]float64, n int) float64 {
	a := p
	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				off += a[i*jointStates+j] * a[i*jointStates+j]
			}
		}
		if off < 1e-24 {
			break
		}
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				aij := a[i*jointStates+j]
				if aij == 0 {
					continue
				}
				theta := (a[j*jointStates+j] - a[i*jointStates+i]) / (2 * aij)
				tt := math.Copysign(1, theta) / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				c := 1 / math.Sqrt(tt*tt+1)
				s := tt * c
				for k := 0; k < n; k++ {
					aki, akj := a[k*jointStates+i], a[k*jointStates+j]
					a[k*jointStates+i], a[k*jointStates+j] = c*aki-s*akj, s*aki+c*akj
				}
				for k := 0; k < n; k++ {
					aik, ajk := a[i*jointStates+k], a[j*jointStates+k]
					a[i*jointStates+k], a[j*jointStates+k] = c*aik-s*ajk, s*aik+c*ajk
				}
			}
		}
	}
	least := math.Inf(1)
	for i := 0; i < n; i++ {
		least = math.Min(least, a[i*jointStates+i])
	}
	return least
}

// invert is a 6x6 inverse by Gauss-Jordan elimination with partial pivoting.
func invert(p [jointStates * jointStates]float64) ([jointStates * jointStates]float64, bool) {
	const n = jointStates
	a := p
	var inv [n * n]float64
	for i := 0; i < n; i++ {
		inv[i*n+i] = 1
	}
	for c := 0; c < n; c++ {
		pivot := c
		for r := c + 1; r < n; r++ {
			if math.Abs(a[r*n+c]) > math.Abs(a[pivot*n+c]) {
				pivot = r
			}
		}
		if math.Abs(a[pivot*n+c]) < 1e-12 {
			return inv, false
		}
		for k := 0; k < n; k++ {
			a[c*n+k], a[pivot*n+k] = a[pivot*n+k], a[c*n+k]
			inv[c*n+k], inv[pivot*n+k] = inv[pivot*n+k], inv[c*n+k]
		}
		d := a[c*n+c]
		for k := 0; k < n; k++ {
			a[c*n+k] /= d
			inv[c*n+k] /= d
		}
		for r := 0; r < n; r++ {
			if r == c {
				continue
			}
			f := a[r*n+c]
			for k := 0; k < n; k++ {
				a[r*n+k] -= f * a[c*n+k]
				inv[r*n+k] -= f * inv[c*n+k]
			}
		}
	}
	return inv, true
}

// quadratic is gᵀ P g.
func quadratic(p [jointStates * jointStates]float64, g [jointStates]float64) float64 {
	var s float64
	for i := 0; i < jointStates; i++ {
		for j := 0; j < jointStates; j++ {
			s += g[i] * p[i*jointStates+j] * g[j]
		}
	}
	return s
}

// The PSD checks above rest on minEigenvalue: a rotated diagonal with one
// negative entry must report it.
func TestMinEigenvalueFindsTheLeastOfARotatedDiagonal(t *testing.T) {
	d := [jointStates]float64{3, 1, 2, -0.25, 5, 0.5}
	c, s := math.Cos(0.7), math.Sin(0.7)
	// Q = a rotation in the (0, 3) and (1, 4) planes.
	var q [jointStates * jointStates]float64
	for i := 0; i < jointStates; i++ {
		q[i*jointStates+i] = 1
	}
	for _, pr := range [][2]int{{0, 3}, {1, 4}} {
		i, j := pr[0], pr[1]
		q[i*jointStates+i], q[j*jointStates+j] = c, c
		q[i*jointStates+j], q[j*jointStates+i] = -s, s
	}
	var m [jointStates * jointStates]float64
	for i := 0; i < jointStates; i++ {
		for j := 0; j < jointStates; j++ {
			for k := 0; k < jointStates; k++ {
				m[i*jointStates+j] += q[i*jointStates+k] * d[k] * q[j*jointStates+k]
			}
		}
	}
	if got := minEigenvalue(m, jointStates); math.Abs(got+0.25) > 1e-9 {
		t.Fatalf("least eigenvalue %.12f, want -0.25", got)
	}
}

// BenchmarkSolidBodyFaceState times the end-on truck pass with the
// face-state options off and on: the joint state adds a 6x6 Joseph update per
// face, and the span bounds a window-minimum search per frame.
func BenchmarkSolidBodyFaceState(b *testing.B) {
	frames := syntheticPassFrames(&testing.T{}, endOnTruckPass())
	for _, on := range []bool{false, true} {
		name := "off"
		if on {
			name = "on"
		}
		b.Run(name, func(b *testing.B) {
			cfg := solidBodyConfig()
			cfg.SolidBody.CourseAlignedFaces = true
			cfg.SolidBody.ExtentState = on
			cfg.SolidBody.CensoredExtent = on
			cfg.SolidBody.SpanBoundsBelief = on
			cfg.SolidBody.HeadingFaceNoise = on
			cfg.SolidBody.AnisotropicReferenceBias = on
			for range b.N {
				tracker := NewTracker(cfg)
				for _, f := range frames {
					tracker.Update(f.clusters, f.at)
				}
			}
		})
	}
}

// Under ExtentState the joint cap scales the capped rows and columns of the
// whole 6x6 covariance, so ConsistentCovarianceCap must not scale the 4x4
// block a second time, in the prediction or in a coast. Both settings must
// give the same joint covariance, and it must stay positive semi-definite
// with every correlation, the position-extent ones included, at most one.
func TestExtentStateWithConsistentCovarianceCapScalesOnce(t *testing.T) {
	run := func(consistent bool) jointState {
		cfg := DefaultTrackerConfig()
		cfg.CoupledProcessNoise = true
		cfg.MaxCovarianceDiag = 2
		cfg.ConsistentCovarianceCap = consistent
		tr := NewTracker(cfg)
		track := &TrackedObject{TrackID: "trk_joint_cap"}
		sb := &track.solidBody
		sb.seeded = true
		sb.state = [4]float32{0, 0, 5, 0}
		sb.p = [16]float32{
			1.5, 0, 0.6, 0,
			0, 1.5, 0, 0.6,
			0.6, 0, 0.5, 0,
			0, 0.6, 0, 0.5,
		}
		sb.ext = solidBodyExtent{
			on:  true,
			h:   [2]float32{2.25, 0.9},
			phh: [4]float32{0.5, 0, 0, 0.2},
			// x with hL, and y with hW: correlations of 0.92 and 0.73.
			pxh: [8]float32{0.8, 0, 0, 0.4, 0, 0, 0, 0},
		}
		for i := 0; i < 40; i++ {
			tr.predictSolidBodyStep(track, 0.1)
			tr.coastSolidBody(track, 0.5)
			if !sb.seeded {
				t.Fatalf("consistent=%v: the solid body was dropped at step %d", consistent, i)
			}
			js := sb.jointOf(sb.state, sb.p)
			if e := minEigenvalue(js.p, jointStates); e < -1e-5 {
				t.Fatalf("consistent=%v step %d: joint covariance not PSD, least eigenvalue %v", consistent, i, e)
			}
			for a := 0; a < jointStates; a++ {
				for b := a + 1; b < jointStates; b++ {
					paa, pbb := js.p[a*jointStates+a], js.p[b*jointStates+b]
					if paa <= 0 || pbb <= 0 {
						continue
					}
					if c := math.Abs(js.p[a*jointStates+b]) / math.Sqrt(paa*pbb); c > 1+1e-5 {
						t.Fatalf("consistent=%v step %d: correlation (%d,%d) = %v above one", consistent, i, a, b, c)
					}
				}
			}
		}
		return sb.jointOf(sb.state, sb.p)
	}
	off, on := run(false), run(true)
	if off.p != on.p {
		t.Errorf("ConsistentCovarianceCap changed the joint covariance under ExtentState, so it scaled twice:\noff %v\non  %v", off.p, on.p)
	}
	if on.p[0] > 2+1e-6 {
		t.Errorf("position variance %v above the cap 2", on.p[0])
	}
}
