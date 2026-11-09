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
// The axis. A span along the body's axis is a lower bound on the dimension,
// and the minimum over a window of axes is one too when the window contains
// the true axis (minimumAxisSpan). Main takes the frame's bounds over a fixed
// window of spanSearchHalfWindowDeg either side of the face axis, believed
// unless the course contradicts it at speed (spanAxisBelieved), which
// compares nothing below CourseAlignmentMinSpeedMps. Under this option the
// window is as wide as the axis is uncertain: the face axis or, where it is
// believed better, the course, each with its variance, and a half-window of
// twice the standard deviation, never under spanSearchHalfWindowDeg and never
// over a quarter-turn (boundWindow). At a quarter-turn the window holds every
// axis, and both bounds are the visible returns' least width, which no axis
// error can carry past the body's width. On kirk0 truck 2's face axis sat 12
// to 60 degrees from its course, with an orientation variance over 0.4 rad²,
// for the first 20 s of its track, and its bounds there were taken along an
// axis 40 to 50 degrees from the course: across that axis a whole 9.6 m by
// 2.8 m body spans over 8 m.
//
// The first version of this option refused a bound off an unsettled axis
// instead of widening its window (geom0017's first read). Without the width
// bounds the faces carried widths to the half-extent floor on twelve times
// as many rows, and without the length bounds truck 1 and car 8 read shorter.
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

// boundWindow is the axis a frame's span bounds are taken about under
// ExtentBoundAccrual and the half-window, in whole degrees, they are the
// minimum over (see the file comment). Of the face axis and, at speed, the
// course, the one with the smaller variance is taken; the face axis is not a
// candidate where the course contradicts it (spanAxisBelieved). An axis the
// faces took from the course, or that RectangleHeading or SharedOrientation
// believe, takes the fixed window.
func (t *Tracker) boundWindow(sb *solidBodyTrack, axis float32, axisIsCourse bool) (float32, int) {
	best, variance := axis, math.Inf(1)
	if axisIsCourse {
		variance = axisBelievedSigmaRad * axisBelievedSigmaRad
	} else if spanAxisBelieved(sb, axis, false) {
		variance = float64(faceAxisVariance(sb, axis))
	}
	if course, ok := courseOrientation(sb.state, sb.p); ok && float64(course.VarianceRad2) < variance {
		best, variance = course.PsiRad, float64(course.VarianceRad2)
	}
	half := spanSearchHalfWindowDeg
	if v := 2 * math.Sqrt(math.Max(variance, 0)) * 180 / math.Pi; v > float64(half) {
		half = int(math.Min(math.Ceil(v), 90))
	}
	return best, half
}

// boundWindowStepDeg is the angle step of a widened window's search: twice
// the fixed window's, so a quarter-turn window costs 91 angles on the
// floor's subset rather than 181. The span found can overstate the window's
// minimum by at most the change over half a step (windowSpans takes it off).
const boundWindowStepDeg = 2

// windowSpans is windowMinimumSpans over a half-window of half degrees about
// axis: exactly it at spanSearchHalfWindowDeg. Wider, the angle is searched
// at boundWindowStepDeg on the floor's subset and the span read on every
// point at the angle found, less the most a half-step can have saved: the
// perpendicular span times the half-step's sine, since a span's rate of
// change with angle is at most the extent across it.
func windowSpans(points []l4perception.WorldPoint, axis float64, half int) (along, across float32) {
	if half <= spanSearchHalfWindowDeg {
		return windowMinimumSpans(points, axis)
	}
	sub := floorPoints(points)
	scratch := make([]float64, len(points))
	slack := math.Sin(float64(boundWindowStepDeg) / 2 * math.Pi / 180)
	span := func(centre float64) float32 {
		best, bestAngle := math.Inf(1), centre
		for deg := -half; deg <= half; deg += boundWindowStepDeg {
			a := centre + float64(deg)*math.Pi/180
			if v, ok := trimmedSpan(sub, scratch[:len(sub)], math.Cos(a), math.Sin(a)); ok && v < best {
				best, bestAngle = v, a
			}
		}
		if math.IsInf(best, 0) {
			return 0
		}
		c, s := math.Cos(bestAngle), math.Sin(bestAngle)
		full, ok := trimmedSpan(points, scratch, c, s)
		if !ok {
			return 0
		}
		perpendicular, _ := trimmedSpan(points, scratch, -s, c)
		return float32(math.Max(full-perpendicular*slack, 0))
	}
	return span(axis), span(axis + math.Pi/2)
}

// accrualSpanBounds is frameSpanBounds under ExtentBoundAccrual: the frame's
// own bounds, over the axis window boundWindow gives, and the evidence it
// adds to the record (accrued). A merge candidate has no frame bound, and
// gives the record its length span when the cluster is one body wide. Nothing
// is taken while the lifecycle is initialising. Zero is no bound.
func (t *Tracker) accrualSpanBounds(sb *solidBodyTrack, cluster WorldCluster, prior classDimensionPrior, axis float32, axisIsCourse, mergeCandidate bool) (frame, accrued [2]float32) {
	if sb.estimation == EstimationInitialising {
		return frame, accrued
	}
	points := nearEdgePoints(cluster)
	if len(points) < DefaultMinFaceSupport {
		return frame, accrued
	}
	along, half := t.boundWindow(sb, axis, axisIsCourse)
	if mergeCandidate && !bodyWide(points, along, prior, sb.widthBelief) {
		return frame, accrued
	}
	l, w := windowSpans(points, float64(along), half)
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
