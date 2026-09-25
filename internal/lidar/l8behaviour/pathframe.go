package l8behaviour

// The seam between following metrics and the shared path they are measured
// along. Section 8.3 needs only a short, directed, local path: arc length to
// order the pair, a tangent to project each body's extent, and a lateral
// offset to tell a same-path leader from a lane-adjacent distractor.
//
// Building that path from final trajectories is the next increment. This file
// fixes the interface it must satisfy, and provides the one path that needs no
// construction: a straight line, which is what the analytic fixtures use.

import (
	"fmt"
	"math"
)

// PathLocation is a planar point expressed in a path's frame.
type PathLocation struct {
	// ArcM is the arc length of the point's projection along the path.
	ArcM float64 `json:"arc_m"`
	// LateralM is the signed offset from the path, positive to the left of
	// the path's direction.
	LateralM float64 `json:"lateral_m"`
	// TangentRad is the path's direction at the projection.
	TangentRad float64 `json:"tangent_rad"`
}

// PathFrame is a directed shared path at one instant.
//
// Locate returns false when the point does not project onto the path's
// supported extent; a caller suppresses with no_common_path rather than
// extrapolating. GeometryID names the path version for provenance, so a result
// can be regenerated against the geometry that produced it.
type PathFrame interface {
	GeometryID() string
	Locate(x, y float64) (PathLocation, bool)
}

// StraightPath is a directed line segment from an origin, of finite length.
// Arc length runs from zero at the origin to LengthM.
type StraightPath struct {
	ID         string  `json:"id"`
	OriginX    float64 `json:"origin_x"`
	OriginY    float64 `json:"origin_y"`
	HeadingRad float64 `json:"heading_rad"`
	LengthM    float64 `json:"length_m"`
}

// Validate requires an identity, finite geometry and a positive length.
func (p StraightPath) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("straight path requires an id")
	}
	if !finite(p.OriginX) || !finite(p.OriginY) || !finite(p.HeadingRad) || !(p.LengthM > 0) || !finite(p.LengthM) {
		return fmt.Errorf("straight path %s requires finite geometry and a positive length", p.ID)
	}
	return nil
}

// GeometryID names the path for provenance.
func (p StraightPath) GeometryID() string { return p.ID }

// Locate projects a point onto the line. Points whose projection falls before
// the origin or beyond the end are off the path.
func (p StraightPath) Locate(x, y float64) (PathLocation, bool) {
	c, s := math.Cos(p.HeadingRad), math.Sin(p.HeadingRad)
	dx, dy := x-p.OriginX, y-p.OriginY
	arc := dx*c + dy*s
	if !(arc >= 0 && arc <= p.LengthM) {
		return PathLocation{}, false
	}
	return PathLocation{ArcM: arc, LateralM: -dx*s + dy*c, TangentRad: p.HeadingRad}, true
}

// PointAt returns the planar point at an arc length and lateral offset. It is
// the inverse of Locate, used to place fixture bodies exactly on the path.
func (p StraightPath) PointAt(arcM, lateralM float64) (x, y float64) {
	c, s := math.Cos(p.HeadingRad), math.Sin(p.HeadingRad)
	return p.OriginX + arcM*c - lateralM*s, p.OriginY + arcM*s + lateralM*c
}
