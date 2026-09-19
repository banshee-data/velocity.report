package l4bobserve

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

const (
	primitiveCandidateLimit = 64
	edgeCandidateLimit      = 24
	primitiveTolerance      = 0.02 // metres; the Phase 1 synthetic acceptance bound
)

// ExtractPrimitives derives deliberately track-independent plane and edge
// candidates from the retained sample. It never assigns a face direction:
// near/far and leading/trailing depend on a predicted track pose and remain a
// Phase 2 interpretation.
func ExtractPrimitives(cluster l4perception.WorldCluster) Primitives {
	points := cluster.RetainedPoints
	if len(points) < 2 {
		return Primitives{}
	}
	candidates := evenlySpacedPoints(points, primitiveCandidateLimit)
	a, b, ok := farthestPair(candidates)
	if !ok {
		return Primitives{}
	}
	result := Primitives{}
	if edge, ok := strongestEdge(evenlySpacedPoints(points, edgeCandidateLimit), points); ok {
		result.Edges = []Edge{edge}
	}
	if c, ok := farthestFromLine(a, b, candidates); ok {
		if plane, ok := planeFromPoints(a, b, c, points); ok {
			result.Planes = []Plane{plane}
		}
	}
	return result
}

func evenlySpacedPoints(points []l4perception.WorldPoint, limit int) []l4perception.WorldPoint {
	if len(points) <= limit {
		return points
	}
	result := make([]l4perception.WorldPoint, limit)
	for i := range result {
		result[i] = points[i*(len(points)-1)/(limit-1)]
	}
	return result
}

func farthestPair(points []l4perception.WorldPoint) (l4perception.WorldPoint, l4perception.WorldPoint, bool) {
	var a, b l4perception.WorldPoint
	best := 0.0
	for i := range points {
		for j := i + 1; j < len(points); j++ {
			if d := squaredDistance(points[i], points[j]); d > best {
				best, a, b = d, points[i], points[j]
			}
		}
	}
	return a, b, best > 0
}

func farthestFromLine(a, b l4perception.WorldPoint, points []l4perception.WorldPoint) (l4perception.WorldPoint, bool) {
	var candidate l4perception.WorldPoint
	best := 0.0
	for _, point := range points {
		if d := squaredDistanceToLine(point, a, b); d > best {
			best, candidate = d, point
		}
	}
	return candidate, best > 0
}

func planeFromPoints(a, b, c l4perception.WorldPoint, all []l4perception.WorldPoint) (Plane, bool) {
	u := subtract(b, a)
	v := subtract(c, a)
	normal := cross(u, v)
	length := norm(normal)
	if length == 0 {
		return Plane{}, false
	}
	for i := range normal {
		normal[i] /= length
	}
	// A normal and its inverse are the same plane. Give it a stable sign so
	// equivalent replays serialise identically.
	for _, value := range normal {
		if math.Abs(value) < 1e-12 {
			continue
		}
		if value < 0 {
			for i := range normal {
				normal[i] = -normal[i]
			}
		}
		break
	}
	offset := -dot(normal, pointArray(a))
	support := 0
	for _, point := range all {
		if math.Abs(dot(normal, pointArray(point))+offset) <= primitiveTolerance {
			support++
		}
	}
	return Plane{Normal: normal, Offset: offset, Support: support}, true
}

func strongestEdge(candidates, all []l4perception.WorldPoint) (Edge, bool) {
	var best Edge
	bestLength := 0.0
	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			length := squaredDistance(candidates[i], candidates[j])
			if length == 0 {
				continue
			}
			support := 0
			for _, point := range all {
				if squaredDistanceToSegment(point, candidates[i], candidates[j]) <= primitiveTolerance*primitiveTolerance {
					support++
				}
			}
			if support > best.Support || (support == best.Support && length > bestLength) {
				best = Edge{Start: pointArray(candidates[i]), End: pointArray(candidates[j]), Support: support}
				bestLength = length
			}
		}
	}
	// Two points define every arbitrary chord. Three retained points make the
	// candidate independently supported and avoid calling a box diagonal an edge.
	return best, best.Support >= 3
}

func pointArray(point l4perception.WorldPoint) [3]float64 {
	return [3]float64{point.X, point.Y, point.Z}
}
func subtract(a, b l4perception.WorldPoint) [3]float64 {
	return [3]float64{a.X - b.X, a.Y - b.Y, a.Z - b.Z}
}
func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func dot(a, b [3]float64) float64                          { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func norm(a [3]float64) float64                            { return math.Sqrt(dot(a, a)) }
func squaredDistance(a, b l4perception.WorldPoint) float64 { d := subtract(a, b); return dot(d, d) }

func squaredDistanceToLine(point, a, b l4perception.WorldPoint) float64 {
	ab := subtract(b, a)
	ap := subtract(point, a)
	denominator := dot(ab, ab)
	if denominator == 0 {
		return dot(ap, ap)
	}
	projection := dot(ap, ab) / denominator
	delta := [3]float64{ap[0] - projection*ab[0], ap[1] - projection*ab[1], ap[2] - projection*ab[2]}
	return dot(delta, delta)
}

func squaredDistanceToSegment(point, a, b l4perception.WorldPoint) float64 {
	ab := subtract(b, a)
	ap := subtract(point, a)
	denominator := dot(ab, ab)
	if denominator == 0 {
		return dot(ap, ap)
	}
	projection := dot(ap, ab) / denominator
	if projection < 0 {
		projection = 0
	} else if projection > 1 {
		projection = 1
	}
	delta := [3]float64{ap[0] - projection*ab[0], ap[1] - projection*ab[1], ap[2] - projection*ab[2]}
	return dot(delta, delta)
}
