package l4perception

import "time"

// WorldPoint represents a point in Cartesian world coordinates (site frame).
// This is the canonical definition; internal/lidar aliases it for backward compatibility.
type WorldPoint struct {
	X, Y, Z   float64   // World frame position (meters)
	Intensity uint8     // Laser return intensity
	Timestamp time.Time // Acquisition time
	SensorID  string    // Source sensor
}
