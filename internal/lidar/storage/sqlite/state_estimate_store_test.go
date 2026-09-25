package sqlite

import (
	"errors"
	"testing"
)

func TestStateEstimateStoreWritesVersionedEstimateAndResidual(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	estimate := TrackEstimate{
		EstimateID: "estimate/v1/one", TrackID: "track/one", ObservationID: "observation/v1/one",
		SourceID: "source/v1/test", CalibrationID: "calibration/v1/test", FrameUnixNanos: 100,
		MeasurementUnixNanos: 101, EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1",
		ParamHash: "params/test", Stage: "online", MeasurementSource: "obb_centre_v1", X: 1, Y: 2, VX: 3, VY: 4,
		Covariance: [16]float32{1, 2, 3, 4},
	}
	residual := TrackResidual{
		EstimateID: estimate.EstimateID, ObservationID: estimate.ObservationID, PredictedX: 0.5, PredictedY: 1.5,
		MeasurementX: 1, MeasurementY: 2, InnovationX: 0.5, InnovationY: 0.5, NIS: 1.25,
		GeometryCovXX: 0.2, GeometryCovXY: 0.01, GeometryCovYY: 0.3, Disposition: "accepted", Reason: "association_accepted",
	}
	if err := store.Insert(estimate, residual); err != nil {
		t.Fatal(err)
	}
	var gotTrack, gotObservation, gotSource, gotCalibration, gotModel string
	var gotTime int64
	if err := database.QueryRow(`SELECT track_id, observation_id, source_id, calibration_id, observation_model_id, measurement_unix_nanos FROM lidar_track_estimates WHERE estimate_id = ?`, estimate.EstimateID).Scan(&gotTrack, &gotObservation, &gotSource, &gotCalibration, &gotModel, &gotTime); err != nil {
		t.Fatal(err)
	}
	if gotTrack != estimate.TrackID || gotObservation != estimate.ObservationID || gotSource != estimate.SourceID || gotCalibration != estimate.CalibrationID || gotModel != estimate.ObservationModelID || gotTime != estimate.MeasurementUnixNanos {
		t.Fatalf("estimate identity lost: %q %q %q %q %q %d", gotTrack, gotObservation, gotSource, gotCalibration, gotModel, gotTime)
	}
	var disposition, reason string
	if err := database.QueryRow(`SELECT disposition, reason FROM lidar_track_residuals WHERE estimate_id = ?`, estimate.EstimateID).Scan(&disposition, &reason); err != nil {
		t.Fatal(err)
	}
	if disposition != "accepted" || reason != "association_accepted" {
		t.Fatalf("residual decision = %q %q", disposition, reason)
	}
}

func TestStateEstimateStoreRejectsMismatchedResidualIdentity(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()
	estimate := TrackEstimate{EstimateID: "e", TrackID: "t", ObservationID: "o", SourceID: "s", CalibrationID: "c", EstimatorID: "e", ObservationModelID: "m", ParamHash: "p", Stage: "online", MeasurementSource: "obb"}
	err := validateStateEstimate(estimate, TrackResidual{EstimateID: "other", ObservationID: "o", Disposition: "accepted", Reason: "ok"})
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("mismatched residual identity error = %v", err)
	}
}
