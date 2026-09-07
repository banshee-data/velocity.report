package l5tracks

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"math"
)

// These are bounded experiment scales, not sensor noise or posterior covariance.
const (
	// axisAspectScale is the log-aspect disagreement counted as one unit of
	// cost. Aspect is what chooses between the two interpretations.
	axisAspectScale = 0.5
	// axisExcessScale is the log-overshoot beyond the believed dimension
	// counted as one unit. Tighter than the aspect scale, because exceeding a
	// lower-bound belief needs the belief to be wrong, not merely occluded.
	axisExcessScale = 0.30
	// axisMinVisibleSupport is the fraction of the believed long dimension a
	// view must show before it may orient the box at all.
	axisMinVisibleSupport = 0.30

	axisMinimumGap    = 2.0
	axisMaximumCost   = 16.0
	axisMinimumAspect = 0.10
)

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

// updateAxisHeading resolves the observation against the track's extent
// beliefs, which are read before this frame revises them.
//
// Every abstention records why it abstained. A view too small to orient, a
// near-square view, a view that fits neither interpretation, and two
// interpretations that tie are four different problems with four different
// fixes, and one shared label cannot tell them apart afterwards.
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
		bestL, bestW, bestH = t.selectAxis(track, b)
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

// selectAxis chooses between the aligned and swapped interpretations and, on a
// confident choice, admits the observation as lower-bound extent evidence.
//
// The cost separates two questions the first version conflated:
//
//   - Aspect agreement decides WHICH axis. An elongated observation belongs
//     along the believed long axis. The two interpretations differ by twice the
//     observation's own log-aspect, so a square view carries no signal and an
//     elongated one carries plenty, which is the correct behaviour in both
//     cases.
//   - Excess decides WHETHER the observation is admissible. A span longer than
//     the belief requires the belief to be wrong; a span shorter than it is
//     just occlusion, and costs nothing, because occlusion removes points and
//     never adds them.
//
// Conflating those two is what rejected legitimate partial views: a symmetric
// log-extent cost punished a half-visible car as hard as an impossible one, and
// 92% of abstentions on the reference capture were that mistake.
func (t *Tracker) selectAxis(track *TrackedObject, b *l4perception.OrientedBoundingBox) (float32, float32, float64) {
	beliefL, beliefW := track.lengthBelief.Estimate(), track.widthBelief.Estimate()
	aspect := math.Abs(float64(b.Length-b.Width)) / math.Max(float64(b.Length), float64(b.Width))

	if beliefL <= 0 || beliefW <= 0 {
		if aspect < axisMinimumAspect {
			track.HeadingSource = HeadingSourceAxisSquare
			return 0, 0, 0
		}
		// First supported observation seeds the beliefs. There is no evidence
		// here for which end of the object is its front.
		track.lengthBelief.Observe(b.Length)
		track.widthBelief.Observe(b.Width)
		track.OBBHeadingRad = b.HeadingRad
		track.HeadingSource = HeadingSourceAxis
		return b.Length, b.Width, float64(b.HeadingRad)
	}

	// A scrap showing a fraction of the believed long axis cannot orient the
	// box, however elongated the scrap itself happens to be. Extent alone
	// cannot reject it — a short span is consistent with any longer object —
	// so the visible-support floor is what refuses it.
	if math.Max(float64(b.Length), float64(b.Width)) < axisMinVisibleSupport*float64(beliefL) {
		track.HeadingSource = HeadingSourceAxisLowSupport
		return 0, 0, 0
	}
	if aspect < axisMinimumAspect {
		track.HeadingSource = HeadingSourceAxisSquare
		return 0, 0, 0
	}

	current := float64(track.OBBHeadingRad)
	h := float64(b.HeadingRad)
	c0 := axisCost(b.Length, b.Width, h, beliefL, beliefW, current)
	c1 := axisCost(b.Width, b.Length, h+math.Pi/2, beliefL, beliefW, current)
	best, l, w := c0, b.Length, b.Width
	if c1 < c0 {
		best, l, w, h = c1, b.Width, b.Length, h+math.Pi/2
	}
	track.AxisScoreGap = float32(math.Abs(c0 - c1))

	switch {
	case best > axisMaximumCost:
		track.HeadingSource = HeadingSourceAxisNoFit
		return l, w, h
	case float64(track.AxisScoreGap) < axisMinimumGap:
		track.HeadingSource = HeadingSourceAmbiguous
		return l, w, h
	}

	delta := axisResidual(h, current)
	// Guard 3 is diagnostic only on this path: a cost winner is not rejected
	// again merely because the old smoother has fallen behind.
	track.HeadingJitterSumSq += delta * delta
	track.HeadingJitterCount++
	updated := current + float64(t.Config.OBBHeadingSmoothingAlpha)*delta
	track.OBBHeadingRad = float32(math.Atan2(math.Sin(updated), math.Cos(updated)))
	track.HeadingSource = HeadingSourceAxis

	// Membership acceptance is not dimension-update acceptance. Only a
	// confidently assigned observation revises a dimension, and it revises it
	// as lower-bound evidence rather than by averaging.
	//
	// A cluster already flagged as a probable merge is excluded. The flag is
	// computed a frame late, which is the right lag here: a merge lasting long
	// enough to corroborate itself is exactly the one that would otherwise
	// resize the object, and a single-frame merge cannot corroborate anyway.
	if !track.MergeCandidate {
		track.lengthBelief.Observe(l)
		track.widthBelief.Observe(w)
	}
	return l, w, h
}

// axisCost scores one interpretation of the observation.
func axisCost(l, w float32, heading float64, beliefL, beliefW float32, current float64) float64 {
	aspect := (math.Log(float64(l/w)) - math.Log(float64(beliefL/beliefW))) / axisAspectScale
	angular := axisResidual(heading, current) / (math.Pi / 4)
	return aspect*aspect + excessCost(l, beliefL) + excessCost(w, beliefW) + .25*angular*angular
}

// excessCost charges only for observing more than the belief. Observing less is
// what occlusion does, and costs nothing.
func excessCost(observed, belief float32) float64 {
	if observed <= belief {
		return 0
	}
	r := math.Log(float64(observed/belief)) / axisExcessScale
	return r * r
}

// releaseStuckAxisReference bounds how long a track may abstain.
//
// Without it this path has no escape at all. Beliefs grow on accepted evidence,
// but a belief seeded far below the truth rejects the very observations that
// would correct it, and nothing else here can unseat it. That is the ratchet
// the rejection release was added to break, reappearing in a new form.
//
// An under-seeded belief and a fragment give the same signal — a long run of
// observations that fit nothing — and are separated by direction, because
// occlusion removes points and never adds them. An observation larger than the
// belief on both axes is evidence the belief was the partial one; a smaller one
// is a partial view or a scrap and may not redefine the object.
//
// Two abstentions deliberately cannot fire this. A square or low-support view
// carries no axis to seed from, and a tie means both interpretations fit, so
// choosing one on a timer manufactures a decision the evidence does not
// support.
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
	if obsL <= track.lengthBelief.Estimate() || obsW <= track.widthBelief.Estimate() {
		return
	}
	// Discarding the belief removes the only basis for preferring the swapped
	// interpretation, so seed from the best-scoring assignment as measured.
	track.lengthBelief.Reseed(obsL)
	track.widthBelief.Reseed(obsW)
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
