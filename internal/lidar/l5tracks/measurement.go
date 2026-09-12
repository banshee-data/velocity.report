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
	// MeasurementNearEdgeCandidateV1 describes a visible face selected for
	// offline E1 comparison. It is deliberately not accepted by the online
	// filter until the experiment establishes that the face is reliable.
	MeasurementNearEdgeCandidateV1 MeasurementSource = "near_edge_candidate_v1"
)

// PositionMeasurement is the position and acquisition time supplied to the
// existing two-dimensional CV filter. It has no physical-vehicle claim: the
// OBB centre is a bounded visible-support stopgap, not a near-edge solution.
type PositionMeasurement struct {
	X, Y      float32
	UnixNanos int64
	Source    MeasurementSource
}

// MeasurementCovariance is a two-dimensional covariance in the site frame.
// The current CV filter still consumes its tuned scalar variance. This
// covariance records the geometry-conditioned uncertainty needed to judge a
// future measurement model without retroactively changing D2's experiment.
type MeasurementCovariance struct {
	XX, XY, YY float32
}

// MeasurementInterpretation is a recomputable, track-conditioned reading of
// a frozen detection. Near edge describes what was visible to the sensor; it
// is not asserted to be the physical centre of an unseen vehicle.
type MeasurementInterpretation struct {
	Measurement PositionMeasurement
	Covariance  MeasurementCovariance

	PredictedX, PredictedY float32
	NearEdgeX, NearEdgeY   float32
	NearEdgeAvailable      bool
	FallbackReason         string
}

// FilterResidual is the accepted innovation against the prediction that
// produced the current state. It is copied out for versioned persistence; it
// never changes the frozen detection observation.
type FilterResidual struct {
	Valid                    bool
	PredictedX, PredictedY   float32
	Measurement              PositionMeasurement
	InnovationX, InnovationY float32
	NIS                      float32
	GeometryCovariance       MeasurementCovariance
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

// InterpretMeasurement records both D2's filter input and the near face that
// E1 must evaluate. sensorX/Y must be the calibrated sensor origin in the
// same site frame as the cluster. If that identity is unavailable, callers
// retain the OBB-centre measurement and record why no face was claimed.
func InterpretMeasurement(cluster l4perception.WorldCluster, frameUnixNanos int64, sensorX, sensorY, predictedX, predictedY, baseVariance float32) MeasurementInterpretation {
	measurement := measurementForCluster(cluster, frameUnixNanos)
	interpretation := MeasurementInterpretation{
		Measurement: measurement,
		Covariance:  covarianceForCluster(cluster, baseVariance),
		PredictedX:  predictedX,
		PredictedY:  predictedY,
	}
	obb := cluster.OBB
	if obb == nil || !finiteMeasurementCoordinate(obb.CenterX) || !finiteMeasurementCoordinate(obb.CenterY) ||
		!finiteMeasurementCoordinate(obb.Length) || !finiteMeasurementCoordinate(obb.Width) ||
		!finiteMeasurementCoordinate(obb.HeadingRad) || obb.Length <= 0 || obb.Width <= 0 {
		interpretation.FallbackReason = "missing_or_invalid_obb"
		return interpretation
	}
	if !finiteMeasurementCoordinate(sensorX) || !finiteMeasurementCoordinate(sensorY) {
		interpretation.FallbackReason = "missing_calibrated_sensor_origin"
		return interpretation
	}

	// The nearest OBB face is observable support, rather than an inferred body
	// centre. Choosing it only needs the calibrated view origin; prediction is
	// persisted above for later face/model analysis but cannot manufacture a
	// far-side measurement.
	dx, dy := sensorX-obb.CenterX, sensorY-obb.CenterY
	axisX, axisY := float32(math.Cos(float64(obb.HeadingRad))), float32(math.Sin(float64(obb.HeadingRad)))
	perpX, perpY := -axisY, axisX
	axisOffset := float32(math.Copysign(float64(obb.Length/2), float64(dx*axisX+dy*axisY)))
	perpOffset := float32(math.Copysign(float64(obb.Width/2), float64(dx*perpX+dy*perpY)))
	interpretation.NearEdgeX = obb.CenterX + axisOffset*axisX + perpOffset*perpX
	interpretation.NearEdgeY = obb.CenterY + axisOffset*axisY + perpOffset*perpY
	interpretation.NearEdgeAvailable = true
	return interpretation
}

// covarianceForCluster propagates only observable OBB support. It treats the
// fitted centre's uncertainty as anisotropic along the OBB axes and preserves
// the existing tuned variance as a floor. It is a recorded E1 input; D2 does
// not yet alter its Kalman gain with this unvalidated model.
func covarianceForCluster(cluster l4perception.WorldCluster, baseVariance float32) MeasurementCovariance {
	if !finiteMeasurementCoordinate(baseVariance) || baseVariance <= 0 {
		baseVariance = 1
	}
	obb := cluster.OBB
	if obb == nil || !finiteMeasurementCoordinate(obb.Length) || !finiteMeasurementCoordinate(obb.Width) ||
		!finiteMeasurementCoordinate(obb.HeadingRad) || obb.Length <= 0 || obb.Width <= 0 || cluster.PointsCount <= 0 {
		return MeasurementCovariance{XX: baseVariance, YY: baseVariance}
	}
	support := float32(math.Sqrt(float64(cluster.PointsCount)))
	// The fit cannot be more certain than its tuned floor. Capping the support
	// contribution prevents a fragmented, implausibly large box from turning a
	// bounded record into an unbounded numerical value.
	axisVariance := max(baseVariance, min(4, (obb.Length/support)*(obb.Length/support)))
	perpVariance := max(baseVariance, min(4, (obb.Width/support)*(obb.Width/support)))
	cos, sin := float32(math.Cos(float64(obb.HeadingRad))), float32(math.Sin(float64(obb.HeadingRad)))
	return MeasurementCovariance{
		XX: axisVariance*cos*cos + perpVariance*sin*sin,
		XY: (axisVariance - perpVariance) * sin * cos,
		YY: axisVariance*sin*sin + perpVariance*cos*cos,
	}
}

func finiteMeasurementCoordinate(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
