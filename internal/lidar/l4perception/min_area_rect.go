package l4perception

import (
	"math"
	"sort"
)

// MinAreaRect is the smallest-area rectangle enclosing a set of XY points.
type MinAreaRect struct {
	// AxisRad is the direction of one pair of sides, folded to [0, pi/2);
	// Span1 is the extent along it and Span2 across it.
	AxisRad      float64
	Span1, Span2 float64
	Area         float64
}

// MinAreaRectangle is the points' minimum-area enclosing rectangle, found by
// turning a side onto each edge of their convex hull: the optimum has a side
// on a hull edge. It is a cheaper axis than FitRectangle, O(n log n) for the
// hull and O(h²) for the edges, and it is blind where FitRectangle is not:
// the hull of an exact L is a right triangle, whose rectangle on the
// hypotenuse has the same area as the one on the legs.
func MinAreaRectangle(points []WorldPoint) (MinAreaRect, bool) {
	if len(points) < 3 {
		return MinAreaRect{}, false
	}
	pts := make([][2]float64, len(points))
	for i, p := range points {
		pts[i] = [2]float64{p.X, p.Y}
	}
	hull := convexHull2D(pts)
	if len(hull) < 3 {
		return MinAreaRect{}, false
	}
	best := MinAreaRect{Area: math.Inf(1)}
	for i := range hull {
		j := (i + 1) % len(hull)
		theta := math.Atan2(hull[j][1]-hull[i][1], hull[j][0]-hull[i][0])
		c, s := math.Cos(theta), math.Sin(theta)
		aLo, aHi := math.Inf(1), math.Inf(-1)
		nLo, nHi := math.Inf(1), math.Inf(-1)
		for _, p := range hull {
			a, n := p[0]*c+p[1]*s, -p[0]*s+p[1]*c
			aLo, aHi = math.Min(aLo, a), math.Max(aHi, a)
			nLo, nHi = math.Min(nLo, n), math.Max(nHi, n)
		}
		if area := (aHi - aLo) * (nHi - nLo); area < best.Area {
			best = MinAreaRect{AxisRad: theta, Span1: aHi - aLo, Span2: nHi - nLo, Area: area}
		}
	}
	if best.Area <= 0 || math.IsInf(best.Area, 0) {
		return MinAreaRect{}, false
	}
	// Fold to [0, pi/2), swapping the spans when the side turns by a quarter.
	a := math.Mod(best.AxisRad, math.Pi)
	if a < 0 {
		a += math.Pi
	}
	if a >= math.Pi/2 {
		a -= math.Pi / 2
		best.Span1, best.Span2 = best.Span2, best.Span1
	}
	best.AxisRad = a
	return best, true
}

// convexHull2D is Andrew's monotone chain, counter-clockwise, without
// collinear points. It sorts pts in place.
func convexHull2D(pts [][2]float64) [][2]float64 {
	sort.Slice(pts, func(i, j int) bool {
		if pts[i][0] != pts[j][0] {
			return pts[i][0] < pts[j][0]
		}
		return pts[i][1] < pts[j][1]
	})
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	hull := make([][2]float64, 0, 2*len(pts))
	for _, p := range pts {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	lower := len(hull) + 1
	for i := len(pts) - 2; i >= 0; i-- {
		p := pts[i]
		for len(hull) >= lower && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	return hull[:len(hull)-1]
}
