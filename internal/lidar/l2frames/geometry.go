package l2frames

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Type aliases and function re-exports for coordinate geometry.

// Point represents a point in sensor Cartesian coordinates.
type Point = l4perception.Point

// PointPolar represents a LiDAR return in polar coordinates.
type PointPolar = l4perception.PointPolar

// Function re-exports.

// SphericalToCartesian converts spherical coordinates (distance, azimuth,
// elevation in degrees) to Cartesian (x, y, z).
var SphericalToCartesian = l4perception.SphericalToCartesian

// ApplyPose applies a 4×4 homogeneous transform matrix to a 3D point.
var ApplyPose = l4perception.ApplyPose
