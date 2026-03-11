package l2frames

import (
	"math"
	"time"
)

// PointPolar is a compact representation of a LiDAR return in polar terms.
// It can be used where sensor-frame operations are preferred (background model).
type PointPolar struct {
	Channel         int
	Azimuth         float64
	Elevation       float64
	Distance        float64
	Intensity       uint8
	Timestamp       int64 // unix nanos if needed; keep small to avoid heavy time usage
	BlockID         int
	UDPSequence     uint32
	RawBlockAzimuth uint16 // Original block azimuth from packet (0.01 deg units)
}

// Point represents a point in sensor Cartesian coordinates.
type Point struct {
	// 3D Cartesian coordinates (computed from spherical measurements)
	X float64 // X coordinate in meters (forward direction from sensor)
	Y float64 // Y coordinate in meters (right direction from sensor)
	Z float64 // Z coordinate in meters (upward direction from sensor)

	// Measurement metadata
	Intensity uint8     // Laser return intensity/reflectivity (0-255)
	Distance  float64   // Radial distance from sensor in meters
	Azimuth   float64   // Horizontal angle in degrees (0-360, corrected)
	Elevation float64   // Vertical angle in degrees (corrected for channel)
	Channel   int       // Laser channel number (1-40)
	Timestamp time.Time // Point acquisition time (with firetime correction)
	BlockID   int       // Data block index within packet (0-9)

	// Packet tracking for completeness validation
	UDPSequence     uint32 // UDP sequence number for gap detection
	RawBlockAzimuth uint16 // Original block azimuth from packet (0.01 deg units)
}

// SphericalToCartesian converts spherical coordinates to Cartesian.
// Uses the sensor coordinate system (X=forward, Y=right, Z=up).
func SphericalToCartesian(distance, azimuthDeg, elevationDeg float64) (x, y, z float64) {
	azimuthRad := azimuthDeg * math.Pi / 180.0
	elevationRad := elevationDeg * math.Pi / 180.0

	cosElevation := math.Cos(elevationRad)
	sinElevation := math.Sin(elevationRad)
	cosAzimuth := math.Cos(azimuthRad)
	sinAzimuth := math.Sin(azimuthRad)

	x = distance * cosElevation * sinAzimuth
	y = distance * cosElevation * cosAzimuth
	z = distance * sinElevation
	return
}

// ApplyPose applies a 4x4 row-major transform T to point (x,y,z).
// T is expected as [16]float64 row-major: m00,m01,m02,m03, m10,...
func ApplyPose(x, y, z float64, T [16]float64) (wx, wy, wz float64) {
	wx = T[0]*x + T[1]*y + T[2]*z + T[3]
	wy = T[4]*x + T[5]*y + T[6]*z + T[7]
	wz = T[8]*x + T[9]*y + T[10]*z + T[11]
	return
}
