package l5tracks

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The geometry convergence plan's W1b: the solid body's heading from an
// observation of its shape rather than from the tracked heading.
//
// Each associated frame's rectangle fit (l4perception.FitRectangle) observes
// the body's axis modulo 90 degrees, with a standard deviation from its edges
// and an abstention when the cluster has no edges to speak of. The axis is a
// scalar Kalman filter on that quarter-turn circle: a random walk with
// axisProcessNoiseRad2PerSecond, updated by each fit that does not abstain,
// the innovation taken on the circle so a fit at 89 degrees and a state at 1
// are two degrees apart. A fit beyond axisGateSigmas of the prediction is
// refused; axisReinitialiseAfter refusals in a row replace the state, so a
// first fit that was wrong cannot hold the axis for the rest of the track.
//
// The axis says which way the body's sides run, not which is its length or
// its front. The label picks one of the four headings axis + k·π/2: the one
// nearest the course at CourseAlignmentMinSpeedMps or more, which takes the
// length along the direction of travel and the front forward, resolving both;
// below it, the one nearest the last heading, carrying its ambiguity; and with
// no last heading, the one nearest the tracked heading, or along the fit's
// longer span, unresolved. A body that has never moved fast enough therefore
// keeps whatever label it started with, which is the plan's low-speed case.

// axisProcessNoiseRad2PerSecond is the axis's random-walk variance per second,
// (0.1 rad)² per second: about 1.8 degrees of standard deviation per 0.1 s
// frame, so a turn at 30 degrees a second (3 degrees a frame) sits inside the
// gate while a three-degree fit is still weighted about half. Not tuned.
const axisProcessNoiseRad2PerSecond = 0.01

// axisGateSigmas is the gate on a fit's innovation, in standard deviations
// of the innovation.
const axisGateSigmas = 3

// axisReinitialiseAfter is the number of consecutive refused fits after which
// the axis is restarted from the fit.
const axisReinitialiseAfter = 3

// axisMaxVarianceRad2 caps the axis variance at a quarter-turn's uniform
// variance, (π/2)²/12: beyond it the state says nothing about the axis.
const axisMaxVarianceRad2 = math.Pi * math.Pi / 48

// axisBelievedSigmaRad is the axis standard deviation within which spans are
// admitted along the labelled heading without comparing it with the course:
// half the span search's half-window.
const axisBelievedSigmaRad = spanSearchHalfWindowDeg / 2 * math.Pi / 180

// solidBodyAxis is the filtered body axis, in [0, π/2).
type solidBodyAxis struct {
	known      bool
	rad        float64
	varRad2    float64
	lastNanos  int64
	refused    int
	updates    int
	lastSource string
}

// frameRectangleFit is the rectangle fit of the frame at nanos.
type frameRectangleFit struct {
	nanos int64
	done  bool
	fit   l4perception.RectangleFit
}

// rectangleFitFor is this frame's rectangle fit, computed once per frame.
func (sb *solidBodyTrack) rectangleFitFor(nanos int64, cluster WorldCluster) l4perception.RectangleFit {
	if sb.fit.done && sb.fit.nanos == nanos {
		return sb.fit.fit
	}
	sb.fit = frameRectangleFit{nanos: nanos, done: true, fit: l4perception.FitRectangle(nearEdgePoints(cluster))}
	return sb.fit.fit
}

// observeAxis predicts the axis to this frame and updates it with the frame's
// rectangle fit, under RectangleHeading.
func (t *Tracker) observeAxis(track *TrackedObject, sb *solidBodyTrack, cluster WorldCluster) {
	if !t.Config.SolidBody.RectangleHeading {
		return
	}
	now := track.LastMeasurementUnixNanos
	fit := sb.rectangleFitFor(now, cluster)
	sb.axis.observe(fit, now)
}

// observe is one frame of the axis filter: the prediction to nanos, then the
// fit's update, refusal or restart.
func (a *solidBodyAxis) observe(fit l4perception.RectangleFit, nanos int64) {
	if a.known {
		if dt := float64(nanos-a.lastNanos) / 1e9; dt > 0 {
			a.varRad2 = math.Min(a.varRad2+axisProcessNoiseRad2PerSecond*dt, axisMaxVarianceRad2)
		}
	}
	a.lastNanos = nanos
	if !fit.Known() {
		a.lastSource = "abstained"
		return
	}
	r := fit.SigmaRad * fit.SigmaRad
	if !a.known {
		a.known, a.rad, a.varRad2, a.refused, a.updates = true, foldQuarterTurn(fit.AxisRad), r, 0, 1
		a.lastSource = "initialised"
		return
	}
	innovation := math.Remainder(fit.AxisRad-a.rad, math.Pi/2)
	s := a.varRad2 + r
	if innovation*innovation > axisGateSigmas*axisGateSigmas*s {
		a.refused++
		a.lastSource = "refused"
		if a.refused >= axisReinitialiseAfter {
			a.rad, a.varRad2, a.refused = foldQuarterTurn(fit.AxisRad), r, 0
			a.lastSource = "restarted"
		}
		return
	}
	k := a.varRad2 / s
	a.rad = foldQuarterTurn(a.rad + k*innovation)
	a.varRad2 = (1 - k) * a.varRad2
	a.refused = 0
	a.updates++
	a.lastSource = "updated"
}

// foldQuarterTurn is x modulo π/2, in [0, π/2).
func foldQuarterTurn(x float64) float64 {
	x = math.Mod(x, math.Pi/2)
	if x < 0 {
		x += math.Pi / 2
	}
	if x >= math.Pi/2 {
		x = 0
	}
	return x
}

// axisOrientation labels the filtered axis into a heading, and false while
// the axis has not been observed.
func (t *Tracker) axisOrientation(track *TrackedObject, sb *solidBodyTrack) (OrientationBelief, bool) {
	a := sb.axis
	if !t.Config.SolidBody.RectangleHeading || !a.known {
		return OrientationBelief{}, false
	}
	o := OrientationBelief{VarianceRad2: float32(a.varRad2), Provenance: ProvenanceObserved}
	var reference float64
	switch course, ok := courseOrientation(sb.state, sb.p); {
	case ok:
		reference, o.AmbiguousModeWeight = float64(course.PsiRad), 0
	case sb.orientation.Provenance != ProvenanceNone:
		reference, o.AmbiguousModeWeight = float64(sb.orientation.PsiRad), sb.orientation.AmbiguousModeWeight
	default:
		o.AmbiguousModeWeight = 0.5
		if tracked, ok := trackOrientation(track); ok {
			reference = float64(tracked.PsiRad)
		} else {
			// The length along the fit's longer span; the front either way.
			reference = a.rad
			if f := sb.fit.fit; sb.fit.done && f.Span2 > f.Span1 {
				reference += math.Pi / 2
			}
		}
	}
	o.PsiRad = float32(labelAxis(a.rad, reference))
	return o, true
}

// labelAxis is the heading among axis + k·π/2 nearest reference, in (-π, π].
func labelAxis(axis, reference float64) float64 {
	best, bestCos := axis, math.Inf(-1)
	for k := 0; k < 4; k++ {
		psi := axis + float64(k)*math.Pi/2
		if c := math.Cos(psi - reference); c > bestCos {
			best, bestCos = psi, c
		}
	}
	return math.Remainder(best, 2*math.Pi)
}

// axisBelieved says the labelled axis is precise enough for spans to be
// admitted along it without the course check: under RectangleHeading, an
// observed axis within axisBelievedSigmaRad.
func (t *Tracker) axisBelieved(sb *solidBodyTrack) bool {
	return t.Config.SolidBody.RectangleHeading && sb.axis.known &&
		sb.axis.varRad2 <= axisBelievedSigmaRad*axisBelievedSigmaRad
}
