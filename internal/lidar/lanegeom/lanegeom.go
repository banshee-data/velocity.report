// Package lanegeom is the geometry of directed lanes in a plane: a lane's
// centreline in its direction of travel and its corridor, the projection of
// a point onto it (arc length, lateral offset, tangent), membership of a
// position and heading with hysteresis, and route distance along a lane.
//
// It is neutral: it imports no layer, so the lane snapshot's attribution
// (docs/plans/lidar-lane-snapshot-and-attribution-plan.md) and lane-constrained
// stitching (docs/plans/lidar-lane-stitching-plan.md) share one definition.
// Angles are radians counter-clockwise from +X, as the tracker's are; a
// caller holding compass bearings converts at its edge.
package lanegeom

import (
	"errors"
	"math"
)

// Point is a position in the plane, metres.
type Point struct{ X, Y float64 }

// Lane is a directed centreline: travel runs from its first vertex to its
// last. HalfWidth is the corridor's half-width either side of it.
type Lane struct {
	ID        string
	Points    []Point
	HalfWidth float64
}

// Membership defaults (snapshot plan, Attribution): the corridor is half the
// lane's width plus 0.5 m, and a heading must lie within 45° of the tangent.
const (
	CorridorMarginM      = 0.5
	MaxHeadingErrorRad   = math.Pi / 4
	HysteresisMarginM    = 0.3
	HysteresisHeadingRad = math.Pi / 36
)

// ErrDegenerate is returned for a lane with fewer than two distinct vertices.
var ErrDegenerate = errors.New("lanegeom: a lane needs two distinct vertices")

// Corridor is the half-width a lane of the given width is matched within.
func Corridor(widthM float64) float64 {
	if widthM <= 0 || math.IsNaN(widthM) || math.IsInf(widthM, 0) {
		widthM = 3.0
	}
	return widthM/2 + CorridorMarginM
}

// Projection is a point projected onto a lane.
type Projection struct {
	// S is the arc length from the lane's first vertex to the foot.
	S float64
	// Lateral is the signed offset, positive to the left of travel.
	Lateral float64
	// Tangent is the direction of travel at the foot.
	Tangent float64
	// Foot is the nearest point on the centreline.
	Foot Point
	// Length is the lane's whole length.
	Length float64
	// Beyond is true when the foot is an end and the point lies past it
	// along the lane's direction (before the start or after the end).
	Beyond bool
}

// Length is the lane's arc length.
func (l Lane) Length() float64 {
	total := 0.0
	for i := 1; i < len(l.Points); i++ {
		total += dist(l.Points[i-1], l.Points[i])
	}
	return total
}

// Project returns the projection of p onto the lane: the nearest point of
// its centreline.
func (l Lane) Project(p Point) (Projection, error) {
	best := Projection{}
	bestD := math.Inf(1)
	s0 := 0.0
	found := false
	length := l.Length()
	for i := 1; i < len(l.Points); i++ {
		a, b := l.Points[i-1], l.Points[i]
		dx, dy := b.X-a.X, b.Y-a.Y
		seg := math.Hypot(dx, dy)
		if seg < 1e-9 {
			continue
		}
		ux, uy := dx/seg, dy/seg
		t := (p.X-a.X)*ux + (p.Y-a.Y)*uy
		beyond := (i == 1 && t < 0) || (i == len(l.Points)-1 && t > seg)
		tc := math.Max(0, math.Min(seg, t))
		foot := Point{a.X + tc*ux, a.Y + tc*uy}
		d := dist(p, foot)
		if d < bestD-1e-12 {
			found = true
			bestD = d
			best = Projection{
				S:       s0 + tc,
				Lateral: -(p.X-a.X)*uy + (p.Y-a.Y)*ux,
				Tangent: math.Atan2(uy, ux),
				Foot:    foot,
				Length:  length,
				Beyond:  beyond,
			}
		}
		s0 += seg
	}
	if !found {
		return Projection{}, ErrDegenerate
	}
	return best, nil
}

// HeadingError is the absolute difference between a heading and a tangent,
// in [0, π].
func HeadingError(heading, tangent float64) float64 {
	d := math.Mod(heading-tangent, 2*math.Pi)
	if d < -math.Pi {
		d += 2 * math.Pi
	} else if d > math.Pi {
		d -= 2 * math.Pi
	}
	return math.Abs(d)
}

// Reason says why a position is in no lane.
type Reason string

// The reasons membership finds no lane.
const (
	ReasonNone      Reason = ""
	ReasonOutside   Reason = "outside_corridor"
	ReasonOpposed   Reason = "wrong_way"
	ReasonNoLanes   Reason = "no_lanes"
	ReasonAmbiguous Reason = "ambiguous"
)

// Match is the lane a position and heading belong to, or the reason none.
type Match struct {
	Lane       int // index into the lanes; -1 when none
	Projection Projection
	Reason     Reason
}

// Member finds the lane holding a position moving at a heading: the lane
// whose corridor holds it and whose tangent lies within MaxHeadingErrorRad
// of the heading, nearest by lateral offset. A point past either end of a
// lane is not in it. With `previous` the lane the track held at its last
// instant (or -1), that lane is kept while the position stays within its
// corridor widened by HysteresisMarginM and its heading within the limit
// widened by HysteresisHeadingRad, so a vehicle straddling two lanes does
// not flicker between them.
func Member(lanes []Lane, p Point, heading float64, previous int) Match {
	if len(lanes) == 0 {
		return Match{Lane: -1, Reason: ReasonNoLanes}
	}
	if previous >= 0 && previous < len(lanes) {
		l := lanes[previous]
		if pr, err := l.Project(p); err == nil && !pr.Beyond &&
			math.Abs(pr.Lateral) <= l.HalfWidth+HysteresisMarginM &&
			HeadingError(heading, pr.Tangent) <= MaxHeadingErrorRad+HysteresisHeadingRad {
			return Match{Lane: previous, Projection: pr}
		}
	}
	best := Match{Lane: -1, Reason: ReasonOutside}
	bestLateral := math.Inf(1)
	inCorridor := false
	for i, l := range lanes {
		pr, err := l.Project(p)
		if err != nil || pr.Beyond || math.Abs(pr.Lateral) > l.HalfWidth {
			continue
		}
		inCorridor = true
		if HeadingError(heading, pr.Tangent) > MaxHeadingErrorRad {
			continue
		}
		if math.Abs(pr.Lateral) < bestLateral {
			bestLateral = math.Abs(pr.Lateral)
			best = Match{Lane: i, Projection: pr}
		}
	}
	if best.Lane < 0 && inCorridor {
		best.Reason = ReasonOpposed
	}
	return best
}

// RouteDistance is the distance along a lane from arc length s0 to s1,
// negative when s1 lies behind s0.
func RouteDistance(s0, s1 float64) float64 { return s1 - s0 }

// BearingToHeading turns a compass bearing (degrees clockwise from north) into
// a heading (radians counter-clockwise from +X, with +Y north).
func BearingToHeading(bearingDeg float64) float64 {
	return math.Pi/2 - bearingDeg*math.Pi/180
}

// HeadingToBearing is the inverse, in [0, 360).
func HeadingToBearing(heading float64) float64 {
	b := math.Mod(90-heading*180/math.Pi, 360)
	if b < 0 {
		b += 360
	}
	return b
}

func dist(a, b Point) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }
