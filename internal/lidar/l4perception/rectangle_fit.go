package l4perception

import (
	"math"
	"math/rand"
)

// Rectangle orientation from a cluster's returns: the geometry convergence
// plan's W1a (docs/plans/lidar-tracker-geometry-convergence-plan.md, 4.1).
//
// A vehicle's visible returns lie on one or two of its faces, and the box
// axis is observable from them every frame: an end face is a strip whose
// edge runs across the body, a corner view is an L whose two edges run along
// and across it. PCA does not read that: the principal axis of a strip is the
// strip's long direction, of an L the diagonal. The closeness criterion of
// Zhang, Xu, Dong and Dolan (IEEE IV 2017, section III; `Zhang2017` in the
// references) searches the orientation that puts the most points nearest a
// rectangle edge, which is the axis modulo 90 degrees. Which of the two axes
// is the length, and which end is the front, are labels the fit does not
// give; they are the solid body's to resolve from its extent belief and its
// course, which is W1b.

const (
	// RectangleFitCoarseStepDeg and RectangleFitFineStepDeg are the search
	// steps: the whole quadrant at the coarse step, then the fine step over
	// one coarse step either side of the best.
	RectangleFitCoarseStepDeg = 1.0
	RectangleFitFineStepDeg   = 0.25
	// RectangleFitMaxPoints caps the points the search reads: a cluster's
	// full membership is subsampled at a stride, since the axis of a few
	// hundred returns is as sharp as the axis of thousands and the search
	// costs their product.
	RectangleFitMaxPoints = 256
	// RectangleFitMinPoints is the fewest points a fit is attempted on.
	RectangleFitMinPoints = 8
	// rectangleClosenessFloorMetres bounds a point's distance to its nearest
	// edge from below in the criterion, so a point on the edge does not
	// dominate the sum. A floor above the returns' noise flattens the score
	// over every orientation that keeps the points within it, which is why
	// the axis is the plateau's centre and not the argmax; a floor below the
	// noise lets single returns steer it.
	rectangleClosenessFloorMetres = 0.03
	// RectangleEdgeToleranceMetres is how near an edge a point must lie to
	// support it.
	RectangleEdgeToleranceMetres = 0.15
	// RectanglePlateauFraction is the share of the best score within which
	// an orientation counts as part of the plateau: the set the axis is the
	// circular mean of, and whose circular spread is the per-frame width.
	RectanglePlateauFraction = 0.05
	// RectangleRangeNoiseMetres is the return's noise perpendicular to an
	// edge that the noise-only axis variance is built from, and
	// RectangleSigmaFloorRad the shape floor under it: what a vehicle is not
	// (bumpers, mirrors, a bonnet seen from above, the across-face band from
	// ring spacing), 3 degrees. On kirk0's 21 reviewed poses the fit's axis
	// error is 2.6 degrees at the median and 4.7 at p90, the error over sigma
	// 1.0 at the median and 1.9 at p90 with this floor, which is the floor
	// on every one of them: the noise-only term never reaches it on a
	// cluster the near-edge model reads.
	RectangleRangeNoiseMetres = 0.05
	RectangleSigmaFloorRad    = 3 * math.Pi / 180
	// RectangleSigmaScale multiplies the noise-only variance; one, since on
	// kirk0 the floor decided every reviewed pose and the scale could not be
	// read.
	RectangleSigmaScale = 1.0
	// RectangleAbstainSigmaRad, RectangleAbstainPlateauRad and
	// RectangleAbstainSupportShare are the axis abstention bounds: a sigma
	// above 10 degrees, a plateau wider than 15 degrees, or edges that
	// fewer than three points in ten lie on (a filled blob, not a box) is no
	// axis.
	RectangleAbstainSigmaRad     = 10 * math.Pi / 180
	RectangleAbstainPlateauRad   = 15 * math.Pi / 180
	RectangleAbstainSupportShare = 0.15
)

// RectangleFit is the orientation of the rectangle that best explains a
// cluster's XY returns.
type RectangleFit struct {
	// AxisRad is the fitted axis in [0, pi/2): the direction of one pair of
	// the rectangle's edges, unlabelled; the other pair lies at
	// AxisRad + pi/2.
	AxisRad float64
	// PlateauRad is the width of the orientations whose score is within
	// RectanglePlateauFraction of the best: twice their circular standard
	// deviation, so a flat top reads as its width and two separate peaks
	// as their distance, which a count of them would not show. SigmaRad is
	// the axis's standard deviation: the largest of the noise-only line-fit
	// variance summed over the supported edges, scaled; half the plateau;
	// and the shape floor.
	SigmaRad   float64
	PlateauRad float64
	// Span1 and Span2 are the points' trimmed extents along AxisRad and
	// across it; Support1 and Support2 count the points within
	// RectangleEdgeToleranceMetres of the better-supported edge of each pair.
	Span1, Span2       float64
	Support1, Support2 int
	// Score is the closeness criterion per point at AxisRad.
	Score float64
	// Points is how many points the search read, after the cap.
	Points int
	// Abstain names why the axis is not usable, or is empty: too_few_points,
	// wide_sigma, wide_plateau or weak_edges. The fit's fields are still
	// filled when it was attempted.
	Abstain string
}

// Known reports whether the fit gave a usable axis.
func (f RectangleFit) Known() bool { return f.Abstain == "" }

// FitRectangle fits the rectangle orientation to the points' XY positions.
func FitRectangle(points []WorldPoint) RectangleFit {
	pts := rectanglePoints(points)
	fit := RectangleFit{Points: len(pts)}
	if len(pts) < RectangleFitMinPoints {
		fit.Abstain = "too_few_points"
		return fit
	}
	var sumX, sumY float64
	for _, p := range pts {
		sumX += p[0]
		sumY += p[1]
	}
	mx, my := sumX/float64(len(pts)), sumY/float64(len(pts))
	for i := range pts {
		pts[i][0] -= mx
		pts[i][1] -= my
	}
	c1 := make([]float64, len(pts))
	c2 := make([]float64, len(pts))

	// Coarse search over the quadrant.
	coarse := int(90 / RectangleFitCoarseStepDeg)
	scores := make([]float64, coarse)
	bestScore := math.Inf(-1)
	for i := 0; i < coarse; i++ {
		theta := float64(i) * RectangleFitCoarseStepDeg * math.Pi / 180
		scores[i] = rectangleCloseness(pts, c1, c2, theta)
		bestScore = math.Max(bestScore, scores[i])
	}
	// The plateau: coarse orientations within the fraction of the best,
	// counted around the quadrant; the axis is their circular mean, since a
	// flat score top is symmetric about the true axis and its argmax is not.
	// Angles are taken times four so that the quadrant is a full turn.
	threshold := bestScore * (1 - RectanglePlateauFraction)
	var sx, sy, sw float64
	for i, s := range scores {
		if s >= threshold {
			theta := float64(i) * RectangleFitCoarseStepDeg * math.Pi / 180
			sx += s * math.Cos(4*theta)
			sy += s * math.Sin(4*theta)
			sw += s
		}
	}
	// The circular spread of the plateau, on the quadruple angle: a mean
	// resultant length r gives a circular standard deviation of
	// sqrt(-2 ln r), divided back by four.
	r := math.Hypot(sx, sy) / sw
	spread := 0.0
	if r < 1 {
		spread = math.Sqrt(-2*math.Log(math.Max(r, 1e-12))) / 4
	}
	fit.PlateauRad = 2 * spread
	axis := math.Atan2(sy, sx) / 4
	// Refine over one coarse step either side at the fine step, the same way.
	fineScore := bestScore
	sx, sy = 0, 0
	for d := -RectangleFitCoarseStepDeg; d <= RectangleFitCoarseStepDeg+1e-9; d += RectangleFitFineStepDeg {
		theta := axis + d*math.Pi/180
		s := rectangleCloseness(pts, c1, c2, theta)
		if s > fineScore {
			fineScore = s
		}
		if s >= threshold {
			sx += s * math.Cos(4*theta)
			sy += s * math.Sin(4*theta)
		}
	}
	if sx != 0 || sy != 0 {
		axis = math.Atan2(sy, sx) / 4
	}
	axis = math.Mod(axis, math.Pi/2)
	if axis < 0 {
		axis += math.Pi / 2
	}
	fit.AxisRad, fit.Score = axis, fineScore/float64(len(pts))

	// Spans and edge support along each axis of the fit.
	ct, st := math.Cos(axis), math.Sin(axis)
	for i, p := range pts {
		c1[i] = p[0]*ct + p[1]*st
		c2[i] = -p[0]*st + p[1]*ct
	}
	fit.Span1, fit.Support1 = rectangleEdge(c1)
	fit.Span2, fit.Support2 = rectangleEdge(c2)

	// The noise-only line-fit variance, summed over the edges, under the
	// shape floor. An edge with no support adds nothing.
	information := 0.0
	for _, e := range [2]struct {
		n int
		s float64
	}{{fit.Support1, fit.Span1}, {fit.Support2, fit.Span2}} {
		if e.n >= 2 && e.s > 0 {
			information += float64(e.n) * e.s * e.s
		}
	}
	sigma2 := math.Inf(1)
	if information > 0 {
		sigma2 = RectangleSigmaScale * 12 * RectangleRangeNoiseMetres * RectangleRangeNoiseMetres / information
	}
	fit.SigmaRad = math.Sqrt(math.Max(math.Max(sigma2, RectangleSigmaFloorRad*RectangleSigmaFloorRad), (fit.PlateauRad/2)*(fit.PlateauRad/2)))
	switch {
	case fit.SigmaRad > RectangleAbstainSigmaRad:
		fit.Abstain = "wide_sigma"
	case fit.PlateauRad > RectangleAbstainPlateauRad:
		fit.Abstain = "wide_plateau"
	case float64(fit.Support1+fit.Support2) < RectangleAbstainSupportShare*float64(len(pts)):
		fit.Abstain = "weak_edges"
	}
	return fit
}

// rectanglePoints is the XY of the points, cut to the cap by a deterministic
// random subset. A stride would alias the order the points arrive in (by
// ring, or by face in a synthetic cloud) and could keep one face and drop
// the other.
func rectanglePoints(points []WorldPoint) [][2]float64 {
	n := len(points)
	if n <= RectangleFitMaxPoints {
		out := make([][2]float64, n)
		for i, p := range points {
			out[i] = [2]float64{p.X, p.Y}
		}
		return out
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	rng := rand.New(rand.NewSource(int64(n)))
	out := make([][2]float64, RectangleFitMaxPoints)
	for i := 0; i < RectangleFitMaxPoints; i++ {
		j := i + rng.Intn(n-i)
		idx[i], idx[j] = idx[j], idx[i]
		out[i] = [2]float64{points[idx[i]].X, points[idx[i]].Y}
	}
	return out
}

// rectangleCloseness is the criterion at one orientation: the sum over the
// points of the reciprocal of each point's distance to the nearer of its two
// candidate edges, floored. c1 and c2 are scratch.
func rectangleCloseness(pts [][2]float64, c1, c2 []float64, theta float64) float64 {
	ct, st := math.Cos(theta), math.Sin(theta)
	min1, max1 := math.Inf(1), math.Inf(-1)
	min2, max2 := math.Inf(1), math.Inf(-1)
	for i, p := range pts {
		a, b := p[0]*ct+p[1]*st, -p[0]*st+p[1]*ct
		c1[i], c2[i] = a, b
		min1, max1 = math.Min(min1, a), math.Max(max1, a)
		min2, max2 = math.Min(min2, b), math.Max(max2, b)
	}
	score := 0.0
	for i := range pts {
		d1 := math.Min(max1-c1[i], c1[i]-min1)
		d2 := math.Min(max2-c2[i], c2[i]-min2)
		score += 1 / math.Max(math.Min(d1, d2), rectangleClosenessFloorMetres)
	}
	return score
}

// rectangleEdge is the trimmed span of projections and the support of the
// better-supported of the two edges they bound: the points within the edge
// tolerance of the minimum or of the maximum.
func rectangleEdge(c []float64) (span float64, support int) {
	n := len(c)
	if n == 0 {
		return 0, 0
	}
	scratch := append([]float64(nil), c...)
	trim := n / 100
	hi := NthFloat64(scratch, n-1-trim)
	lo := hi
	if trim < n-1-trim {
		lo = NthFloat64(scratch[:n-1-trim], trim)
	}
	nearLo, nearHi := 0, 0
	for _, v := range c {
		if v-lo <= RectangleEdgeToleranceMetres {
			nearLo++
		}
		if hi-v <= RectangleEdgeToleranceMetres {
			nearHi++
		}
	}
	if nearLo > nearHi {
		return hi - lo, nearLo
	}
	return hi - lo, nearHi
}

// FoldAxisRad folds the difference between two axes to [0, pi/2), the
// error between unlabelled axes.
func FoldAxisRad(a, b float64) float64 {
	d := math.Mod(a-b, math.Pi/2)
	if d < 0 {
		d += math.Pi / 2
	}
	if d > math.Pi/4 {
		d = math.Pi/2 - d
	}
	return d
}
