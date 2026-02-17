package l2frames

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases and function re-exports for coordinate geometry.

// PointPolar represents a LiDAR return in polar coordinates.
type PointPolar = lidar.PointPolar

// Function re-exports.

// SphericalToCartesian converts spherical coordinates (distance, azimuth,
// elevation in degrees) to Cartesian (x, y, z).
var SphericalToCartesian = lidar.SphericalToCartesian

// ApplyPose applies a 4×4 homogeneous transform matrix to a 3D point.
var ApplyPose = lidar.ApplyPose
