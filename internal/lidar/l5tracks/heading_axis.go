package l5tracks

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"math"
)

// These are bounded experiment scales, not sensor noise or posterior covariance.
const axisLogExtentScale = 0.35
const axisMinimumGap = 2.0
const axisMaximumCost = 16.0
const axisMinimumAspect = 0.10

// axisResidual compares unlabelled axes modulo pi, including across the wrap.
func axisResidual(a, b float64) float64 {
	return .5 * math.Atan2(math.Sin(2*(a-b)), math.Cos(2*(a-b)))
}

func finiteOBB(b *l4perception.OrientedBoundingBox) bool {
	if b == nil {
		return false
	}
	for _, v := range []float32{b.CenterX, b.CenterY, b.CenterZ, b.Length, b.Width, b.Height, b.HeadingRad} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return b.Length > 0 && b.Width > 0 && b.Height >= 0
}

// updateAxisHeading uses the PREVIOUS accepted support descriptor. The raw
// running averages have already consumed this frame and are deliberately unused.
// It never treats partial extents as physical dimensions or course as body yaw.
//
// Every abstention records why it abstained. A near-square observation, an
// observation that fits neither interpretation, and two interpretations that
// score equally are three different problems with three different fixes, and
// one shared "ambiguous" label cannot tell them apart afterwards.
func (t *Tracker) updateAxisHeading(track *TrackedObject, cluster WorldCluster) {
	b := cluster.OBB
	track.AxisScoreGap = 0
	track.HeadingSource = HeadingSourceInsufficient
	if !finiteOBB(b) {
		track.AxisAbstentionRun++
		track.RecordHeadingSource(track.HeadingSource)
		return
	}
	// Best-scoring assignment, retained for the release test below. It is only
	// meaningful when the cost test actually ran.
	var bestL, bestW float32
	var bestH float64
	if cluster.PointsCount >= t.Config.MinPointsForPCA {
		aspect := math.Abs(float64(b.Length-b.Width)) / math.Max(float64(b.Length), float64(b.Width))
		switch {
		case aspect < axisMinimumAspect:
			track.HeadingSource = HeadingSourceAxisSquare
		case track.axisReferenceL <= 0 || track.axisReferenceW <= 0:
			// First supported observation seeds an observed-shape reference.
			// There is no evidence here for which end of the object is its front.
			track.axisReferenceL, track.axisReferenceW = b.Length, b.Width
			track.OBBHeadingRad = b.HeadingRad
			track.HeadingSource = HeadingSourceAxis
		default:
			cost := func(l, w float32, h float64) float64 {
				x := math.Log(float64(l/track.axisReferenceL)) / axisLogExtentScale
				y := math.Log(float64(w/track.axisReferenceW)) / axisLogExtentScale
				a := axisResidual(h, float64(track.OBBHeadingRad)) / (math.Pi / 4)
				return x*x + y*y + .25*a*a
			}
			h, l, w := float64(b.HeadingRad), b.Length, b.Width
			c0, c1 := cost(l, w, h), cost(w, l, h+math.Pi/2)
			track.AxisScoreGap = float32(math.Abs(c0 - c1))
			best := c0
			if c1 < c0 {
				best, h, l, w = c1, h+math.Pi/2, w, l
			}
			bestL, bestW, bestH = l, w, h
			switch {
			case best > axisMaximumCost:
				track.HeadingSource = HeadingSourceAxisNoFit
			case float64(track.AxisScoreGap) < axisMinimumGap:
				track.HeadingSource = HeadingSourceAmbiguous
			default:
				delta := axisResidual(h, float64(track.OBBHeadingRad))
				// Guard 3 is diagnostic only on this path: a cost winner is not
				// rejected again merely because the old smoother has fallen behind.
				track.HeadingJitterSumSq += delta * delta
				track.HeadingJitterCount++
				updated := float64(track.OBBHeadingRad) + float64(t.Config.OBBHeadingSmoothingAlpha)*delta
				track.OBBHeadingRad = float32(math.Atan2(math.Sin(updated), math.Cos(updated)))
				track.HeadingSource = HeadingSourceAxis
				// Only comparable support may revise the descriptor. Gross
				// fragments cannot revise it; gradual support drift remains possible.
				if l >= .85*track.axisReferenceL && w >= .85*track.axisReferenceW && l <= 1.15*track.axisReferenceL && w <= 1.15*track.axisReferenceW {
					track.axisReferenceL = .9*track.axisReferenceL + .1*l
					track.axisReferenceW = .9*track.axisReferenceW + .1*w
				}
			}
		}
	}
	t.releaseStuckAxisReference(track, bestL, bestW, bestH)
	track.HeadingRejectionRun = 0
	track.RecordHeadingSource(track.HeadingSource)
	// D1.4: an observed envelope centred on the published filtered position.
	// Project the measured OBB, never the previous envelope, so uncertainty in
	// orientation does not recursively inflate dimensions. This is conservative
	// containment of that OBB, NOT reconstructed physical vehicle geometry.
	projectObservedEnvelope(track, b)
	track.SampleCourseAlignment()
}

// releaseStuckAxisReference bounds how long a track may abstain.
//
// Without it this path has no escape at all. A reference seeded from a bad
// partial view fails the cost test against every later observation, and
// nothing else here can unseat it, so the track carries a stale heading for
// the rest of its life. That is the ratchet the rejection release was added
// to break, reappearing in a new form.
//
// The reference is what is stuck, so the reference is what gets reset. The
// difficulty is that an under-seeded reference and a fragment produce the
// same signal: a long run of observations that fit nothing. They are told
// apart by direction, because occlusion only ever removes points. A partial
// view under-measures an object and can never over-measure it, so an
// observation LARGER than the reference on both axes is evidence the
// reference was the partial one, while a smaller observation is a partial
// view or a fragment and must not be allowed to redefine the object. Growing
// only is what keeps a 0.11 m scrap from becoming a car's shape.
//
// Two abstentions deliberately cannot fire this. A square or invalid
// observation still advances the run but carries no axis, so re-seeding from
// it would swap a stale reference for a meaningless one. A tie means both
// interpretations fit; picking one on a timer manufactures a decision the
// evidence does not support.
func (t *Tracker) releaseStuckAxisReference(track *TrackedObject, obsL, obsW float32, obsHeading float64) {
	if !track.HeadingSource.IsLocked() {
		track.AxisAbstentionRun = 0
		return
	}
	track.AxisAbstentionRun++
	maxRun := t.Config.OBBHeadingLockMaxRejections
	if maxRun <= 0 || track.AxisAbstentionRun < maxRun || track.HeadingSource != HeadingSourceAxisNoFit {
		return
	}
	if obsL <= track.axisReferenceL || obsW <= track.axisReferenceW {
		return
	}
	// Discarding the reference removes the only basis for preferring the
	// swapped interpretation, so seed from the best-scoring assignment as
	// measured. This is still an observed-support heuristic, not a physical
	// dimension estimate.
	track.axisReferenceL, track.axisReferenceW = obsL, obsW
	track.OBBHeadingRad = float32(math.Atan2(math.Sin(obsHeading), math.Cos(obsHeading)))
	track.HeadingSource = HeadingSourceAxisReleased
	track.AxisAbstentionRun = 0
	track.HeadingLockReleases++
}

func projectObservedEnvelope(track *TrackedObject, b *l4perception.OrientedBoundingBox) {
	h := float64(track.OBBHeadingRad)
	c, s := math.Cos(h), math.Sin(h)
	dx, dy := float64(b.CenterX-track.X), float64(b.CenterY-track.Y)
	delta := float64(b.HeadingRad) - h
	a, z := math.Abs(math.Cos(delta)), math.Abs(math.Sin(delta))
	track.OBBLength = float32(a*float64(b.Length) + z*float64(b.Width) + 2*math.Abs(c*dx+s*dy))
	track.OBBWidth = float32(z*float64(b.Length) + a*float64(b.Width) + 2*math.Abs(-s*dx+c*dy))
	track.OBBHeight, track.LatestZ = b.Height, b.CenterZ
}
