package pipeline

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// persistOnlineStateEstimate writes a derived online result only when it can
// name the immutable detection that supplied it. A missing identity is an
// error, never a reason to fabricate source provenance from the sensor name.
func persistOnlineStateEstimate(cfg *TrackingPipelineConfig, track *l5tracks.TrackedObject, frameUnixNanos int64) error {
	if cfg.StateEstimateSink == nil || !track.LastResidual.Valid {
		return nil
	}
	if cfg.ObservationSourceID == "" || cfg.ObservationCalibrationID == "" || cfg.StateEstimatorID == "" || cfg.StateObservationModelID == "" || cfg.StateParameterHash == "" {
		return fmt.Errorf("state estimate sink requires source, calibration, estimator, observation-model, and parameter identities")
	}
	observationID, err := l4bobserve.ObservationID(cfg.ObservationSourceID, cfg.ObservationCalibrationID, frameUnixNanos, track.LastClusterID)
	if err != nil {
		return fmt.Errorf("derive estimate observation identity: %w", err)
	}
	estimateID := fmt.Sprintf("estimate/%s/%s/%s/%d", track.TrackID, cfg.StateEstimatorID, cfg.StateObservationModelID, frameUnixNanos)
	residual := track.LastResidual
	estimate := sqlite.TrackEstimate{
		EstimateID: estimateID, TrackID: track.TrackID, ObservationID: observationID,
		SourceID: cfg.ObservationSourceID, CalibrationID: cfg.ObservationCalibrationID,
		FrameUnixNanos: frameUnixNanos, MeasurementUnixNanos: track.LastMeasurementUnixNanos,
		EstimatorID: cfg.StateEstimatorID, ObservationModelID: cfg.StateObservationModelID,
		ParamHash: cfg.StateParameterHash, Stage: "online", MeasurementSource: string(track.LastMeasurementSource),
		X: track.X, Y: track.Y, VX: track.VX, VY: track.VY, Covariance: track.P,
	}
	return cfg.StateEstimateSink.Insert(estimate, sqlite.TrackResidual{
		EstimateID: estimateID, ObservationID: observationID, PredictedX: residual.PredictedX, PredictedY: residual.PredictedY,
		MeasurementX: residual.Measurement.X, MeasurementY: residual.Measurement.Y,
		InnovationX: residual.InnovationX, InnovationY: residual.InnovationY, NIS: residual.NIS,
		GeometryCovXX: residual.GeometryCovariance.XX, GeometryCovXY: residual.GeometryCovariance.XY, GeometryCovYY: residual.GeometryCovariance.YY,
		Disposition: "accepted", Reason: "association_accepted",
	})
}
