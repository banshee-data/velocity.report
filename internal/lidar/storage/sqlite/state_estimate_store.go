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
	// CreationSequence is the tracker's deterministic per-run track ordinal
	// (l5tracks.TrackedObject.CreationSequence). Unlike TrackID, a random UUID
	// kept collision-free across resets and restarts, this is reproducible
	// across two replays of the same input and is what a semantic evidence
	// comparison should group tracks by.
	CreationSequence int64
	X, Y, VX, VY     float32
	Covariance       [16]float32
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

// FrameStateEstimate is one derived estimate/residual pair. It is the unit
// that joins an immutable observation to the online tracker output.
type FrameStateEstimate struct {
	Estimate TrackEstimate
	Residual TrackResidual
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
	return insertStateEstimate(exec, estimate, residual, time.Now().UnixNano())
}

func insertStateEstimate(exec Executor, estimate TrackEstimate, residual TrackResidual, insertedAtNanos int64) error {
	if err := validateStateEstimate(estimate, residual); err != nil {
		return err
	}
	covariance, err := json.Marshal(estimate.Covariance)
	if err != nil {
		return fmt.Errorf("marshal estimate covariance: %w", err)
	}
	_, err = exec.Exec(stateEstimateInsertSQL, stateEstimateInsertArgs(estimate, covariance, insertedAtNanos)...)
	if err != nil {
		return fmt.Errorf("insert track estimate %s: %w", estimate.EstimateID, err)
	}
	_, err = exec.Exec(stateResidualInsertSQL, stateResidualInsertArgs(residual, insertedAtNanos)...)
	if err != nil {
		return fmt.Errorf("insert track residual %s: %w", residual.EstimateID, err)
	}
	return nil
}

const stateEstimateInsertSQL = `INSERT OR REPLACE INTO lidar_track_estimates
		(estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos,
		 estimator_id, observation_model_id, param_hash, stage, measurement_source, creation_sequence, x, y, vx, vy, covariance_json, inserted_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const stateResidualInsertSQL = `INSERT OR REPLACE INTO lidar_track_residuals
		(estimate_id, observation_id, predicted_x, predicted_y, measurement_x, measurement_y, innovation_x, innovation_y,
		 nis, geometry_cov_xx, geometry_cov_xy, geometry_cov_yy, disposition, reason, inserted_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func stateEstimateInsertArgs(estimate TrackEstimate, covariance []byte, insertedAtNanos int64) []any {
	return []any{
		estimate.EstimateID, estimate.TrackID, estimate.ObservationID, estimate.SourceID, estimate.CalibrationID,
		estimate.FrameUnixNanos, estimate.MeasurementUnixNanos, estimate.EstimatorID, estimate.ObservationModelID,
		estimate.ParamHash, estimate.Stage, estimate.MeasurementSource, estimate.CreationSequence,
		estimate.X, estimate.Y, estimate.VX, estimate.VY, covariance, insertedAtNanos,
	}
}

func stateResidualInsertArgs(residual TrackResidual, insertedAtNanos int64) []any {
	return []any{
		residual.EstimateID, residual.ObservationID, residual.PredictedX, residual.PredictedY, residual.MeasurementX,
		residual.MeasurementY, residual.InnovationX, residual.InnovationY, residual.NIS, residual.GeometryCovXX,
		residual.GeometryCovXY, residual.GeometryCovYY, residual.Disposition, residual.Reason, insertedAtNanos,
	}
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

// ListBySource returns a source's estimates in deterministic track and frame
// order, so an offline analysis can reconstruct each track's path.
//
// Ordering is by CreationSequence rather than TrackID: the latter is a random
// UUID, deliberately, so it is not reproducible across two replays of the same
// input, whereas the creation sequence is.
func (s *StateEstimateStore) ListBySource(sourceID string) ([]TrackEstimate, error) {
	rows, err := s.db.Query(`
		SELECT estimate_id, track_id, observation_id, source_id, calibration_id
		     , frame_unix_nanos, measurement_unix_nanos, estimator_id
		     , observation_model_id, param_hash, stage, measurement_source
		     , creation_sequence, x, y, vx, vy, covariance_json
		  FROM lidar_track_estimates
		 WHERE source_id = ?
		 ORDER BY creation_sequence, frame_unix_nanos, estimate_id`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list track estimates for source %s: %w", sourceID, err)
	}
	defer rows.Close()

	estimates := []TrackEstimate{}
	for rows.Next() {
		var e TrackEstimate
		var covariance []byte
		if err := rows.Scan(
			&e.EstimateID, &e.TrackID, &e.ObservationID, &e.SourceID, &e.CalibrationID,
			&e.FrameUnixNanos, &e.MeasurementUnixNanos, &e.EstimatorID,
			&e.ObservationModelID, &e.ParamHash, &e.Stage, &e.MeasurementSource,
			&e.CreationSequence, &e.X, &e.Y, &e.VX, &e.VY, &covariance,
		); err != nil {
			return nil, fmt.Errorf("scan track estimate: %w", err)
		}
		if err := json.Unmarshal(covariance, &e.Covariance); err != nil {
			return nil, fmt.Errorf("unmarshal estimate covariance %s: %w", e.EstimateID, err)
		}
		estimates = append(estimates, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate track estimates: %w", err)
	}
	return estimates, nil
}
