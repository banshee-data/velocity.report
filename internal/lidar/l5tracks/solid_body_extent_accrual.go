package l5tracks

// ExtentBoundAccrual (SolidBodyOptions.ExtentBoundAccrual): the extent
// state's span bounds accrue over the track's life. Section 9.2.4 of
// docs/plans/lidar-state-estimation-plan.md; geom0017 scores it on kirk0.
//
// Under ExtentState a frame's span bound holds hL or hW at or above half the
// bound by projection, and leaves P unchanged (applyExtentBounds). The bound
// is the current frame's alone. Nothing remembers a higher bound an earlier
// frame admitted, so once the visible span shrinks (an end-on view, a
// cluster that sheds its far returns) a face can carry the half-extent back
// below what the track has already shown, and with P unchanged by bounds the
// face's gain on h is the prior's. Length seen at different times does not
// accumulate, which is the observability case of Section 3.1 of the
// face-state maths review.
//
// The record. Each half-extent keeps the extentBeliefCorroboration largest
// bounds admitted over the track's life (extentBoundRecord). The supported
// bound is the smallest of them: the largest value that that many admitted
// frames reached. It is the corroborated maximum the extent belief uses
// (extentBelief.Estimate, Section 9.2.3), without the belief's bins: a single
// merged frame, which looks exactly like one unusually good view, cannot set
// it, while a handful of good views among many partial ones does. Each frame
// the half-extent is held at or above half of the larger of the frame's bound
// and the supported one, by the same projection, so the record cannot narrow
// P either and a run of equal bounds moves the mean once.
//
// The axis. A span is a lower bound on a dimension only along the body's own
// axis. The face axis is believed when it is the course (axisIsCourse), or
// when its orientation variance is within axisBelievedSigmaRad and, at speed,
// the course does not contradict it (spanAxisBelieved). Otherwise the course
// is used when its own variance is within the same bound: at speed a road
// vehicle moves along its axis, and the window-minimum span covers a course
// within spanSearchHalfWindowDeg of the axis. Otherwise no bound is taken. On
// kirk0 truck 2's face axis sat 12 to 60 degrees from its course, with an
// orientation variance over 0.4 rad², for the first 20 s of its track, and
// spanAxisBelieved, which compares nothing below CourseAlignmentMinSpeedMps,
// admitted bounds there along an axis 40 to 50 degrees from the course.
//
// Merges. Section 9.2.1 refuses a merge candidate's spans. The tracker's
// merge flag is an area ratio against the track's running box (tracking.go),
// so a truck whose view grows from its end to its side raises it on every
// frame the view grows. Under this option a merge candidate's length span
// counts toward the record, and only toward it, when the cluster is no wider
// across the axis than one body (bodyWide, growingNotMerging's test). Its
// width span is never taken, and its length is never the frame's own bound,
// so it holds only once extentBeliefCorroboration frames reach it.
//
// Revisability. Section 9.2.3 asks that a frame later found to be a merge
// can be removed. The record keeps only its largest bounds, not which frames
// gave them, so it cannot be revised; corroboration and the lateral test are
// what stand in. That is a known debt of this option.

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// extentBoundRecord is, per half-extent's dimension, the largest bounds the
// track's admitted frames gave, in decreasing order; zero is an empty slot.
type extentBoundRecord struct {
	top [2][extentBeliefCorroboration]float32
}

// observe adds one frame's bounds (zero for none) to the record.
func (r *extentBoundRecord) observe(bounds [2]float32) {
	for i, b := range bounds {
		if !(b > 0) || math.IsInf(float64(b), 0) {
			continue
		}
		top := &r.top[i]
		for j := range top {
			if b > top[j] {
				copy(top[j+1:], top[j:len(top)-1])
				top[j] = b
				break
			}
		}
	}
}

// supported is the largest bound extentBeliefCorroboration admitted frames
// reached, per dimension: the smallest of the kept bounds, and zero until the
// record holds that many.
func (r *extentBoundRecord) supported() [2]float32 {
	var out [2]float32
	for i := range out {
		out[i] = r.top[i][extentBeliefCorroboration-1]
	}
	return out
}

// admit adds a frame's accrued evidence to the record and returns the bounds
// the frame applies: per dimension, the larger of the frame's own bound and
// the supported one.
func (r *extentBoundRecord) admit(accrued, frame [2]float32) [2]float32 {
	r.observe(accrued)
	out := r.supported()
	for i := range out {
		if frame[i] > out[i] {
			out[i] = frame[i]
		}
	}
	return out
}

// accruesExtentBounds says whether this body's span bounds accrue: under
// ExtentBoundAccrual, on a body carrying the extent state, with the span
// bounds taken (takesSpanBounds).
func (t *Tracker) accruesExtentBounds(sb *solidBodyTrack) bool {
	return t.Config.SolidBody.ExtentBoundAccrual && sb.ext.on && t.takesSpanBounds(sb)
}

// boundAxis is the axis a frame's span bounds are taken along under
// ExtentBoundAccrual, and false when no axis is believed (see the file
// comment).
func (t *Tracker) boundAxis(sb *solidBodyTrack, axis float32, axisIsCourse bool) (float32, bool) {
	const believed = axisBelievedSigmaRad * axisBelievedSigmaRad
	if axisIsCourse {
		return axis, true
	}
	if faceAxisVariance(sb, axis) <= believed && spanAxisBelieved(sb, axis, false) {
		return axis, true
	}
	if course, ok := courseOrientation(sb.state, sb.p); ok && course.VarianceRad2 <= believed {
		return course.PsiRad, true
	}
	return 0, false
}

// accrualSpanBounds is frameSpanBounds under ExtentBoundAccrual: the frame's
// own bounds, along the believed axis (boundAxis), and the evidence it adds
// to the record (accrued). A merge candidate has no frame bound, and gives
// the record its length span when the cluster is one body wide. Nothing is
// taken while the lifecycle is initialising. Zero is no bound.
func (t *Tracker) accrualSpanBounds(sb *solidBodyTrack, cluster WorldCluster, prior classDimensionPrior, axis float32, axisIsCourse, mergeCandidate bool) (frame, accrued [2]float32) {
	if sb.estimation == EstimationInitialising {
		return frame, accrued
	}
	along, ok := t.boundAxis(sb, axis, axisIsCourse)
	if !ok {
		return frame, accrued
	}
	points := nearEdgePoints(cluster)
	if len(points) < DefaultMinFaceSupport {
		return frame, accrued
	}
	if mergeCandidate && !bodyWide(points, along, prior, sb.widthBelief) {
		return frame, accrued
	}
	l, w := windowMinimumSpans(points, float64(along))
	noise := spanBoundNoise(math.Hypot(float64(sb.state[0]-t.Config.SolidBody.SensorX), float64(sb.state[1]-t.Config.SolidBody.SensorY)))
	for i, span := range [2]float32{l, w} {
		if b := float64(span) - noise; b > 0 {
			accrued[i] = float32(b)
		}
	}
	if mergeCandidate {
		// The length only, and only through the record.
		accrued[1] = 0
		return frame, accrued
	}
	return accrued, accrued
}

// bodyWide is growingNotMerging's lateral test without its option: the
// points' trimmed span straight across axis is no more than the believed
// width, or the class prior's if wider, plus extentGrowthLateralMarginMetres.
func bodyWide(points []l4perception.WorldPoint, axis float32, prior classDimensionPrior, widthBelief extentBelief) bool {
	across, ok := trimmedSpan(points, make([]float64, len(points)), -math.Sin(float64(axis)), math.Cos(float64(axis)))
	if !ok {
		return false
	}
	width := prior.widthMetres
	if w := widthBelief.Estimate(); w > width {
		width = w
	}
	return across <= float64(width)+extentGrowthLateralMarginMetres
}
