package sqlite

import (
	"encoding/json"
	"fmt"
	"time"
)

// TrackEstimate is a versioned estimator output. Its observation identity is
// required: estimates can be regenerated, evidence cannot.
type TrackEstimate struct {
	EstimateID           string
	TrackID              string
	ObservationID        string
	SourceID             string
	CalibrationID        string
	FrameUnixNanos       int64
	MeasurementUnixNanos int64
	EstimatorID          string
	ObservationModelID   string
	ParamHash            string
	Stage                string
	MeasurementSource    string
	X, Y, VX, VY         float32
	Covariance           [16]float32
}

// TrackResidual is the innovation that led to one estimate. The geometry
// covariance is retained even while D2's CV gain uses its historic scalar R.
type TrackResidual struct {
	EstimateID                                  string
	ObservationID                               string
	PredictedX, PredictedY                      float32
	MeasurementX, MeasurementY                  float32
	InnovationX, InnovationY                    float32
	NIS                                         float32
	GeometryCovXX, GeometryCovXY, GeometryCovYY float32
	Disposition                                 string
	Reason                                      string
}

// StateEstimateStore owns derived, versioned records. It intentionally has no
// method that mutates an observation payload.
type StateEstimateStore struct{ db DBClient }

func NewStateEstimateStore(db DBClient) *StateEstimateStore { return &StateEstimateStore{db: db} }

// Insert writes the estimate and residual as one SQLite transaction when the
// store owns a *sql.DB. Pipeline callers that already batch writes may use
// InsertWithExecutor instead.
func (s *StateEstimateStore) Insert(estimate TrackEstimate, residual TrackResidual) error {
	return InsertStateEstimate(s.db, estimate, residual)
}

// InsertStateEstimate writes derived state through either a database or a
// caller-owned transaction. Replacing a derived online row is allowed only for
// its exact versioned key; the immutable observation remains untouched.
func InsertStateEstimate(exec Executor, estimate TrackEstimate, residual TrackResidual) error {
	if err := validateStateEstimate(estimate, residual); err != nil {
		return err
	}
	covariance, err := json.Marshal(estimate.Covariance)
	if err != nil {
		return fmt.Errorf("marshal estimate covariance: %w", err)
	}
	now := time.Now().UnixNano()
	_, err = exec.Exec(`INSERT OR REPLACE INTO lidar_track_estimates
		(estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos,
		 estimator_id, observation_model_id, param_hash, stage, measurement_source, x, y, vx, vy, covariance_json, inserted_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		estimate.EstimateID, estimate.TrackID, estimate.ObservationID, estimate.SourceID, estimate.CalibrationID,
		estimate.FrameUnixNanos, estimate.MeasurementUnixNanos, estimate.EstimatorID, estimate.ObservationModelID,
		estimate.ParamHash, estimate.Stage, estimate.MeasurementSource, estimate.X, estimate.Y, estimate.VX, estimate.VY,
		covariance, now)
	if err != nil {
		return fmt.Errorf("insert track estimate %s: %w", estimate.EstimateID, err)
	}
	_, err = exec.Exec(`INSERT OR REPLACE INTO lidar_track_residuals
		(estimate_id, observation_id, predicted_x, predicted_y, measurement_x, measurement_y, innovation_x, innovation_y,
		 nis, geometry_cov_xx, geometry_cov_xy, geometry_cov_yy, disposition, reason, inserted_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		residual.EstimateID, residual.ObservationID, residual.PredictedX, residual.PredictedY, residual.MeasurementX,
		residual.MeasurementY, residual.InnovationX, residual.InnovationY, residual.NIS, residual.GeometryCovXX,
		residual.GeometryCovXY, residual.GeometryCovYY, residual.Disposition, residual.Reason, now)
	if err != nil {
		return fmt.Errorf("insert track residual %s: %w", residual.EstimateID, err)
	}
	return nil
}

func validateStateEstimate(estimate TrackEstimate, residual TrackResidual) error {
	if estimate.EstimateID == "" || estimate.TrackID == "" || estimate.ObservationID == "" || estimate.SourceID == "" ||
		estimate.CalibrationID == "" || estimate.EstimatorID == "" || estimate.ObservationModelID == "" || estimate.ParamHash == "" ||
		estimate.Stage == "" || estimate.MeasurementSource == "" {
		return fmt.Errorf("state estimate requires identity, model and source fields")
	}
	if residual.EstimateID != estimate.EstimateID || residual.ObservationID != estimate.ObservationID || residual.Disposition == "" || residual.Reason == "" {
		return fmt.Errorf("residual must identify the same estimate and observation with a disposition and reason")
	}
	return nil
}
