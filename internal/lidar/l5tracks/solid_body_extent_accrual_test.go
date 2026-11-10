package l5tracks

import (
	"math"
	"testing"
)

// Tests for ExtentBoundAccrual (solid_body_extent_accrual.go). What carries
// the weight: the record holds the largest bound three admitted frames
// reached and no single frame's; a bound's window of axes is as wide as the
// axis is uncertain, the course standing in for an unsettled face axis, and
// a quarter-turn window cannot pass the body's width; a merge
// candidate gives its length to the record, never its width and never as
// the frame's own bound, and only when it is one body wide; and through the
// state machine the record holds a half-extent after a face that would carry
// it below what the track has shown, where the frame's bound alone does not.

func TestExtentBoundRecordHoldsTheCorroboratedMaximum(t *testing.T) {
	var r extentBoundRecord
	r.observe([2]float32{9.0, 2.5})
	r.observe([2]float32{8.8, 0})
	if got := r.supported(); got != ([2]float32{}) {
		t.Fatalf("two frames support %v, want nothing until three", got)
	}
	r.observe([2]float32{9.1, 2.4})
	if got := r.supported(); got[0] != 8.8 || got[1] != 0 {
		t.Fatalf("after three length bounds the record supports %v, want the third largest, 8.8 m, and no width", got)
	}
	// Shorter spans, as an end-on view gives, do not lower it.
	for i := 0; i < 20; i++ {
		r.observe([2]float32{3.5, 2.4})
	}
	if got := r.supported(); got != ([2]float32{8.8, 2.4}) {
		t.Fatalf("after shorter spans the record supports %v, want 8.8 m and 2.4 m", got)
	}
	// One frame far above the rest, as a merged cluster gives, moves the
	// supported bound only to the next corroborated value.
	r.observe([2]float32{14, 0})
	if got := r.supported(); got[0] != 9.0 {
		t.Fatalf("after one 14 m frame the record supports %.2f m, want 9.0 m", got[0])
	}
	// admit takes the larger of the frame's bound and the record's.
	if got := r.admit([2]float32{}, [2]float32{9.5, 1.0}); got != ([2]float32{9.5, 2.4}) {
		t.Fatalf("admit gave %v, want the frame's length and the record's width", got)
	}
	// Non-finite and non-positive bounds are not evidence.
	before := r
	r.observe([2]float32{float32(math.Inf(1)), -1})
	r.observe([2]float32{float32(math.NaN()), 0})
	if r != before {
		t.Fatal("a non-finite or non-positive bound entered the record")
	}
}

// accrualTracker is a tracker with the face-state bounds and accrual on.
func accrualTracker() *Tracker {
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	cfg.SolidBody.CensoredExtent = true
	cfg.SolidBody.ExtentBoundAccrual = true
	return NewTracker(cfg)
}

// accrualBody is a body-centre extent-state body at (20, 5) moving along x at
// speed, with a velocity variance small enough that its course is believed,
// and a face axis turned and unsettled.
func accrualBody(speed, psi, psiVariance float32) *solidBodyTrack {
	track := boxPassBody(20, 5, speed)
	sb := &track.solidBody
	sb.p[10], sb.p[15] = 0.01, 0.01
	sb.orientation = OrientationBelief{PsiRad: psi, VarianceRad2: psiVariance, Provenance: ProvenanceObserved}
	seedSolidBodyExtent(sb, dimensionPriorFor(MotionRigidVehicle))
	return sb
}

func TestExtentAccrualWidensTheWindowAsTheAxisIsUncertain(t *testing.T) {
	tracker := accrualTracker()
	// A settled face axis the course agrees with: the face axis, at the fixed
	// window, as main takes it.
	settled := accrualBody(6, 0.05, 1e-4)
	if axis, half := tracker.boundWindow(settled, 0.05, false); axis != 0.05 || half != spanSearchHalfWindowDeg {
		t.Errorf("settled face axis: %.3f ±%d°, want 0.05 ±%d°", axis, half, spanSearchHalfWindowDeg)
	}
	// A face axis 40 degrees off at speed with a variance of 1 rad²: the
	// course, whose sigma is under a degree, at the fixed window.
	sb := accrualBody(6, 0.7, 1)
	if axis, half := tracker.boundWindow(sb, 0.7, false); math.Abs(float64(axis)) > 1e-6 || half != spanSearchHalfWindowDeg {
		t.Errorf("unsettled face axis at speed: %.3f ±%d°, want the course, 0, ±%d°", axis, half, spanSearchHalfWindowDeg)
	}
	// A settled face axis the course contradicts is not a candidate.
	contradicted := accrualBody(6, 0.5, 3e-4)
	if axis, _ := tracker.boundWindow(contradicted, 0.5, false); math.Abs(float64(axis)) > 1e-6 {
		t.Errorf("contradicted face axis: %.3f, want the course", axis)
	}
	// Below the course speed there is no course: the face axis, with twice
	// its sigma, 0.4 rad, as the half-window, and a quarter-turn at most.
	slow := accrualBody(1, 0.7, 0.04)
	if axis, half := tracker.boundWindow(slow, 0.7, false); axis != 0.7 || half != 23 {
		t.Errorf("slow body, sigma 0.2 rad: %.3f ±%d°, want 0.7 ±23°", axis, half)
	}
	slow.orientation.VarianceRad2 = 1
	if _, half := tracker.boundWindow(slow, 0.7, false); half != 90 {
		t.Errorf("slow body, sigma 1 rad: ±%d°, want ±90°", half)
	}
	// An axis the faces took from the course takes the fixed window.
	if axis, half := tracker.boundWindow(slow, 0.3, true); axis != 0.3 || half != spanSearchHalfWindowDeg {
		t.Errorf("course face axis: %.3f ±%d°, want 0.3 ±%d°", axis, half, spanSearchHalfWindowDeg)
	}
}

// A 9 m by 2.6 m side seen along an axis 40 degrees off: over the fixed
// window its across-span is over 5.5 m, which as a width bound is the ratchet;
// over a quarter-turn window both bounds are at most the true width, and
// within a few centimetres of it.
func TestExtentAccrualWideWindowBoundsCannotPassTheWidth(t *testing.T) {
	points := strip(15.5, 9, 3.7, 2.6, 40)
	_, fixed := windowSpans(points, 0.7, spanSearchHalfWindowDeg)
	if !(fixed > 5.5) {
		t.Fatalf("the fixed window 40 degrees off gave an across-span of %.2f m; the case needs one past 5.5 m", fixed)
	}
	along, across := windowSpans(points, 0.7, 90)
	t.Logf("40 degrees off: fixed-window across-span %.2f m; quarter-turn window %.2f m along, %.2f m across", fixed, along, across)
	for _, v := range []float32{along, across} {
		if v > 2.6+1e-3 || v < 2.4 {
			t.Errorf("quarter-turn window span %.3f m, want at most the 2.6 m width and within 0.2 m of it", v)
		}
	}
	// At the fixed window it is exactly main's computation.
	l, w := windowSpans(points, 0.05, spanSearchHalfWindowDeg)
	ml, mw := windowMinimumSpans(points, 0.05)
	if l != ml || w != mw {
		t.Errorf("fixed window %.4f, %.4f against windowMinimumSpans' %.4f, %.4f", l, w, ml, mw)
	}
}

func TestExtentAccrualMergeCandidateGivesItsLengthToTheRecordOnly(t *testing.T) {
	tracker := accrualTracker()
	prior := dimensionPriorFor(MotionRigidVehicle)
	sb := accrualBody(6, 0.7, 1)
	// A truck's side in view, 9 m along the course and 2.6 m across.
	longer := WorldCluster{RetainedPoints: strip(15.5, 9, 3.7, 2.6, 40)}
	// Two vehicles side by side: 4.5 m along and 5 m across.
	wider := WorldCluster{RetainedPoints: strip(17.75, 4.5, 2.5, 5, 40)}

	frame, accrued := tracker.accrualSpanBounds(sb, longer, prior, 0.7, false, false)
	if !(frame[0] > 8.5) || !(frame[1] > 2.2) || frame != accrued {
		t.Fatalf("a plain frame: bound %v, accrued %v; want about 9 m by 2.6 m, both the same", frame, accrued)
	}
	frame, accrued = tracker.accrualSpanBounds(sb, longer, prior, 0.7, false, true)
	if frame != ([2]float32{}) || !(accrued[0] > 8.5) || accrued[1] != 0 {
		t.Fatalf("a merge candidate one body wide: bound %v, accrued %v; want no frame bound and the length alone accrued", frame, accrued)
	}
	if frame, accrued = tracker.accrualSpanBounds(sb, wider, prior, 0.7, false, true); frame != ([2]float32{}) || accrued != ([2]float32{}) {
		t.Fatalf("a merge candidate two bodies wide: bound %v, accrued %v; want nothing", frame, accrued)
	}
	sb.estimation = EstimationInitialising
	if frame, accrued = tracker.accrualSpanBounds(sb, longer, prior, 0.7, false, false); frame != ([2]float32{}) || accrued != ([2]float32{}) {
		t.Fatalf("while initialising: bound %v, accrued %v; want nothing", frame, accrued)
	}
	// Below the course speed with an unsettled axis, both bounds are the
	// least width.
	slow := accrualBody(1, 0.7, 1)
	if frame, _ = tracker.accrualSpanBounds(slow, longer, prior, 0.7, false, false); frame[0] > 2.6 || frame[1] > 2.6 || !(frame[1] > 2.0) {
		t.Fatalf("on an unsettled axis: bound %v, want both at most the 2.6 m width, the width's near it", frame)
	}
}

// Three frames bound the length at 8.9 m and no frame after does. A face
// whose plane lies a metre inboard of the state's then pulls hL down by its
// gain. With the frame's bound alone nothing restores it; with the record
// the step ends with hL at half the corroborated bound, and the covariance is
// the face update's.
func TestExtentAccrualHoldsTheLengthAfterTheSpanShrinks(t *testing.T) {
	prior := dimensionPriorFor(MotionRigidVehicle)
	const bound = float32(8.9)
	run := func(accrual bool) (*solidBodyTrack, [16]float32, [4]float32) {
		cfg := solidBodyConfig()
		cfg.SolidBody.ExtentState = true
		cfg.SolidBody.CensoredExtent = true
		cfg.SolidBody.ExtentBoundAccrual = accrual
		tracker := NewTracker(cfg)
		sb := accrualBody(6, 0, 3e-4)
		cluster := WorldCluster{RetainedPoints: strip(15.5, 9, 3.7, 2.6, 40)}
		for k := 0; k < 3; k++ {
			f := nearEdgeFrame{axis: 0, spanBound: [2]float32{bound, 0}, accrued: [2]float32{bound, 0},
				edges: EdgeMeasurementSet{FallbackReason: "no_face_reached_minimum_support"}}
			tracker.stepNearEdge(sb, cluster, prior, f)
		}
		if got := sb.ext.h[0]; math.Abs(float64(got-bound/2)) > 1e-5 {
			t.Fatalf("accrual %v: after three bounds hL %.4f m, want %.4f m", accrual, got, bound/2)
		}
		// The rear face, a metre inboard of where the state puts it.
		face := EdgeMeasurement{Face: FaceRear, NormalX: -1, PlaneOffsetMetres: -sb.state[0] + sb.ext.h[0] - 1,
			HalfExtentMetres: sb.ext.h[0], HalfExtentProvenance: ProvenanceAccumulated, SupportPoints: 20}
		f := nearEdgeFrame{axis: 0, edges: EdgeMeasurementSet{Edges: []EdgeMeasurement{face}, Rank: 1},
			length: sb.extentDimension(0), width: sb.extentDimension(1)}
		if _, outcome, _ := tracker.stepNearEdge(sb, cluster, prior, f); outcome != nearEdgeFix {
			t.Fatalf("accrual %v: outcome %v, want a fix", accrual, outcome)
		}
		return sb, sb.p, sb.ext.phh
	}
	plain, plainP, plainPhh := run(false)
	held, heldP, heldPhh := run(true)
	t.Logf("after the inboard face: hL %.3f m with the frame's bound alone, %.3f m with the record", plain.ext.h[0], held.ext.h[0])
	if !(plain.ext.h[0] < bound/2-0.05) {
		t.Fatalf("the face left hL at %.3f m without the record; the case needs one that pulls it down", plain.ext.h[0])
	}
	if math.Abs(float64(held.ext.h[0]-bound/2)) > 1e-5 {
		t.Errorf("with the record hL %.4f m after the face, want %.4f m", held.ext.h[0], bound/2)
	}
	if heldP != plainP || heldPhh != plainPhh {
		t.Error("the record changed the covariance the face update left")
	}
	if held.ext.updates != plain.ext.updates {
		t.Errorf("the record's hold after the face counted as evidence: updates %v against %v", held.ext.updates, plain.ext.updates)
	}
}

// On the box pass every frame's span covers the whole length, along an axis
// that is settled and agrees with the course, so the record adds nothing the
// frame's bound does not: the pass ends as it does with the bounds alone, on
// the shadow and the tracked filter.
func TestExtentAccrualLeavesTheBoxPassToTheFaces(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	alone := runBoxPass(t, cfg, shortExtentState)
	cfg.SolidBody.SpanBoundsBelief = true
	cfg.SolidBody.ExtentBoundAccrual = true
	accrued := runBoxPass(t, cfg, shortExtentState)
	checkSpanBoundsHoldTheFaces(t, "shadow with accrual", alone, accrued)

	tracked := nearEdgeTrackingConfig()
	tracked.SolidBody.ExtentState = true
	trackedAlone := runTrackedBoxPass(t, tracked, shortExtentState)
	tracked.SolidBody.SpanBoundsBelief = true
	tracked.SolidBody.ExtentBoundAccrual = true
	trackedAccrued := runTrackedBoxPass(t, tracked, shortExtentState)
	checkSpanBoundsHoldTheFaces(t, "tracked with accrual", trackedAlone.boxPassResult, trackedAccrued.boxPassResult)
}
