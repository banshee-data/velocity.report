package l4perception

import "math"

// Rect is a rectangle in the plane: its centre, heading and extents along
// and across the heading.
type Rect struct {
	X, Y          float64
	HeadingRad    float64
	Length, Width float64
}

// Corners lists the rectangle's corners anticlockwise.
func (r Rect) Corners() [][2]float64 {
	c, s := math.Cos(r.HeadingRad), math.Sin(r.HeadingRad)
	out := make([][2]float64, 0, 4)
	for _, k := range [4][2]float64{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}} {
		a, n := k[0]*r.Length/2, k[1]*r.Width/2
		out = append(out, [2]float64{r.X + a*c - n*s, r.Y + a*s + n*c})
	}
	return out
}

// RectIntersectionArea is the area two rectangles share.
func RectIntersectionArea(p, q Rect) float64 {
	if math.Hypot(p.X-q.X, p.Y-q.Y) > (math.Hypot(p.Length, p.Width)+math.Hypot(q.Length, q.Width))/2 {
		return 0
	}
	return PolygonArea(ClipConvex(p.Corners(), q.Corners()))
}

// RectOverlapShare is the area two rectangles share over the smaller one's
// area, zero when either has none: one when the smaller lies wholly within
// the larger.
func RectOverlapShare(p, q Rect) float64 {
	small := math.Min(p.Length*p.Width, q.Length*q.Width)
	if small <= 0 {
		return 0
	}
	return RectIntersectionArea(p, q) / small
}

// ClipConvex clips the subject polygon by the convex clip polygon, both
// anticlockwise (Sutherland-Hodgman). An empty result means they do not
// overlap.
func ClipConvex(subject, clip [][2]float64) [][2]float64 {
	out := subject
	for i := range clip {
		a, b := clip[i], clip[(i+1)%len(clip)]
		inside := func(p [2]float64) bool { return (b[0]-a[0])*(p[1]-a[1])-(b[1]-a[1])*(p[0]-a[0]) >= 0 }
		in := out
		out = nil
		for j := range in {
			p, q := in[j], in[(j+1)%len(in)]
			pIn, qIn := inside(p), inside(q)
			if pIn {
				out = append(out, p)
			}
			if pIn != qIn {
				// The edge p-q crosses the clip line a-b.
				dx, dy := q[0]-p[0], q[1]-p[1]
				ex, ey := b[0]-a[0], b[1]-a[1]
				t := (ex*(p[1]-a[1]) - ey*(p[0]-a[0])) / (ey*dx - ex*dy)
				out = append(out, [2]float64{p[0] + t*dx, p[1] + t*dy})
			}
		}
		if len(out) == 0 {
			return nil
		}
	}
	return out
}

// PolygonArea is a simple polygon's area (the shoelace formula).
func PolygonArea(p [][2]float64) float64 {
	area := 0.0
	for i := range p {
		q := p[(i+1)%len(p)]
		area += p[i][0]*q[1] - q[0]*p[i][1]
	}
	return math.Abs(area) / 2
}
