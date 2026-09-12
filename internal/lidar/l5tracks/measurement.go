package l5tracks

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// MeasurementSource identifies the geometry that supplied an L5 position.
// It is evidence about the estimate, rather than an object-class label: a
// valid OBB centre and a medoid can legitimately disagree for the same frame.
type MeasurementSource string

const (
	MeasurementOBBCentreV1      MeasurementSource = "obb_centre_v1"
	MeasurementMedoidFallbackV1 MeasurementSource = "medoid_fallback_v1"
)

// PositionMeasurement is the position and acquisition time supplied to the
// existing two-dimensional CV filter. It has no physical-vehicle claim: the
// OBB centre is a bounded visible-support stopgap, not a near-edge solution.
type PositionMeasurement struct {
	X, Y      float32
	UnixNanos int64
	Source    MeasurementSource
}

// measurementForCluster applies Decision D2 consistently to association,
// initialisation, and the Kalman update. A malformed or absent OBB fails
// visibly to the medoid; it never silently presents a zero-valued box centre.
func measurementForCluster(cluster l4perception.WorldCluster, frameUnixNanos int64) PositionMeasurement {
	timestamp := cluster.TSUnixNanos
	if timestamp <= 0 {
		timestamp = frameUnixNanos
	}
	if obb := cluster.OBB; obb != nil && finiteMeasurementCoordinate(obb.CenterX) && finiteMeasurementCoordinate(obb.CenterY) {
		return PositionMeasurement{X: obb.CenterX, Y: obb.CenterY, UnixNanos: timestamp, Source: MeasurementOBBCentreV1}
	}
	return PositionMeasurement{X: cluster.CentroidX, Y: cluster.CentroidY, UnixNanos: timestamp, Source: MeasurementMedoidFallbackV1}
}

func finiteMeasurementCoordinate(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
