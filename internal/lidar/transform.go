package lidar

import "math"

// SphericalToCartesian converts distance (meters), azimuth (degrees) and
// elevation (degrees) into Cartesian sensor-frame coordinates.
// Coordinate convention: X=right, Y=forward, Z=up (matches existing code).
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
