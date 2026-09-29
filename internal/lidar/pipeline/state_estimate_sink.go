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
	pair, err := onlineStateEstimate(cfg, track, frameUnixNanos)
	if err != nil {
		return err
	}
	if err := cfg.StateEstimateSink.Insert(pair.Estimate, pair.Residual); err != nil {
		return err
	}
	if pair.SolidBody == nil {
		return nil
	}
	// A tracker populating solid bodies with a sink that cannot hold them is
	// a configuration fault, reported rather than resolved by dropping them.
	sink, ok := cfg.StateEstimateSink.(SolidBodySink)
	if !ok {
		return fmt.Errorf("state estimate sink cannot persist the solid-body estimate for track %s", track.TrackID)
	}
	return sink.InsertSolidBody(*pair.SolidBody)
}

// SolidBodySink is the optional extension of StateEstimateSink that writes
// solid-body estimates; sqlite.StateEstimateStore implements it. The offline
// frame path writes them through FrameEvidenceSink instead, in the frame's
// own transaction.
type SolidBodySink interface {
	InsertSolidBody(sqlite.TrackSolidBody) error
}

// onlineEstimateID is the online estimate's key. A refined estimate names the
// online estimate it revises by this same key; see refined_estimate_sink.go.
func onlineEstimateID(trackID, estimatorID, observationModelID string, frameUnixNanos int64) string {
	return fmt.Sprintf("estimate/%s/%s/%s/%d", trackID, estimatorID, observationModelID, frameUnixNanos)
}

func onlineStateEstimate(cfg *TrackingPipelineConfig, track *l5tracks.TrackedObject, frameUnixNanos int64) (sqlite.FrameStateEstimate, error) {
	if cfg.ObservationSourceID == "" || cfg.ObservationCalibrationID == "" || cfg.StateEstimatorID == "" || cfg.StateObservationModelID == "" || cfg.StateParameterHash == "" {
		return sqlite.FrameStateEstimate{}, fmt.Errorf("state estimate sink requires source, calibration, estimator, observation-model, and parameter identities")
	}
	observationID, err := l4bobserve.ObservationID(cfg.ObservationSourceID, cfg.ObservationCalibrationID, frameUnixNanos, track.LastClusterID)
	if err != nil {
		return sqlite.FrameStateEstimate{}, fmt.Errorf("derive estimate observation identity: %w", err)
	}
	estimateID := onlineEstimateID(track.TrackID, cfg.StateEstimatorID, cfg.StateObservationModelID, frameUnixNanos)
	residual := track.LastResidual
	estimate := sqlite.TrackEstimate{
		EstimateID: estimateID, TrackID: track.TrackID, ObservationID: observationID,
		SourceID: cfg.ObservationSourceID, CalibrationID: cfg.ObservationCalibrationID,
		FrameUnixNanos: frameUnixNanos, MeasurementUnixNanos: track.LastMeasurementUnixNanos,
		CreationSequence: track.CreationSequence,
		EstimatorID:      cfg.StateEstimatorID, ObservationModelID: cfg.StateObservationModelID,
		ParamHash: cfg.StateParameterHash, Stage: sqlite.EstimateStageOnline, MeasurementSource: string(track.LastMeasurementSource),
		X: track.X, Y: track.Y, VX: track.VX, VY: track.VY, Covariance: track.P,
	}
	pair := sqlite.FrameStateEstimate{Estimate: estimate, Residual: sqlite.TrackResidual{
		EstimateID: estimateID, ObservationID: observationID, PredictedX: residual.PredictedX, PredictedY: residual.PredictedY,
		MeasurementX: residual.Measurement.X, MeasurementY: residual.Measurement.Y,
		InnovationX: residual.InnovationX, InnovationY: residual.InnovationY, NIS: residual.NIS,
		GeometryCovXX: residual.GeometryCovariance.XX, GeometryCovXY: residual.GeometryCovariance.XY, GeometryCovYY: residual.GeometryCovariance.YY,
		Disposition: residualDisposition(residual), Reason: residualReason(residual),
	}}
	pair.SolidBody = onlineSolidBody(cfg, track, observationID, frameUnixNanos)
	return pair, nil
}

// onlineSolidBody files the track's solid body, when the tracker populates
// one, beside its point estimate: the same observation, estimator, parameter
// hash and stage, under the near-edge observation model that produced it. Its
// estimate ID has its own prefix, so the two rows never share a key.
func onlineSolidBody(cfg *TrackingPipelineConfig, track *l5tracks.TrackedObject, observationID string, frameUnixNanos int64) *sqlite.TrackSolidBody {
	reading, ok := track.SolidBody()
	if !ok {
		return nil
	}
	model := string(l5tracks.MeasurementNearEdgeCandidateV1)
	return &sqlite.TrackSolidBody{
		EstimateID:     fmt.Sprintf("solid_body/%s/%s/%s/%d", track.TrackID, cfg.StateEstimatorID, model, frameUnixNanos),
		TrackID:        track.TrackID,
		ObservationID:  observationID,
		SourceID:       cfg.ObservationSourceID,
		CalibrationID:  cfg.ObservationCalibrationID,
		FrameUnixNanos: frameUnixNanos, MeasurementUnixNanos: track.LastMeasurementUnixNanos,
		EstimatorID: cfg.StateEstimatorID, ObservationModelID: model,
		ParamHash: cfg.StateParameterHash, Stage: "online",
		CreationSequence: track.CreationSequence,
		Reading:          reading,
	}
}

// residualDisposition and residualReason are what became of the frame's
// measurement: an accepted update unless the tracker said otherwise.
func residualDisposition(r l5tracks.FilterResidual) string {
	if r.Disposition == "" {
		return "accepted"
	}
	return r.Disposition
}

func residualReason(r l5tracks.FilterResidual) string {
	if r.Disposition == "" {
		return "association_accepted"
	}
	return r.Reason
}
