package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Read path for behaviour analysis over persisted estimates (the provisional
// headway run, internal/report/headway/fieldrun): one exact version of
// lidar_track_estimates or lidar_track_solid_bodies, any stage, with the
// sensor each row was observed by. Version selection is ListEstimateVersions'
// or ListSolidBodyVersions'; this reads what was selected.

// EstimateWithSensor is a persisted estimate and the sensor that observed
// it, read through the immutable observation the estimate names.
type EstimateWithSensor struct {
	TrackEstimate
	SensorID string
}

// ListVersionEstimates returns one version's estimates in track and frame
// order, each with the sensor_id of the lidar_observations row it links to.
// An estimate whose observation is not stored has lost its evidence, and is
// an error rather than a row with an empty sensor.
func (s *StateEstimateStore) ListVersionEstimates(key EstimateVersionKey) ([]EstimateWithSensor, error) {
	rows, err := s.db.Query(`
		SELECT e.estimate_id, e.track_id, e.observation_id, e.source_id, e.calibration_id
		     , e.frame_unix_nanos, e.measurement_unix_nanos, e.estimator_id
		     , e.observation_model_id, e.param_hash, e.stage, e.measurement_source
		     , e.creation_sequence, e.x, e.y, e.vx, e.vy, e.covariance_json
		     , o.sensor_id
		  FROM lidar_track_estimates e
		  LEFT JOIN lidar_observations o ON o.observation_id = e.observation_id
		 WHERE e.source_id = ? AND e.estimator_id = ? AND e.observation_model_id = ?
		   AND e.param_hash = ? AND e.stage = ?
		 ORDER BY e.track_id, e.frame_unix_nanos, e.estimate_id`,
		key.SourceID, key.EstimatorID, key.ObservationModelID, key.ParamHash, key.Stage)
	if err != nil {
		return nil, fmt.Errorf("list estimates %s/%s/%s: %w", key.SourceID, key.EstimatorID, key.Stage, err)
	}
	defer rows.Close()
	out := []EstimateWithSensor{}
	for rows.Next() {
		var r EstimateWithSensor
		e := &r.TrackEstimate
		var covariance []byte
		var sensor sql.NullString
		if err := rows.Scan(
			&e.EstimateID, &e.TrackID, &e.ObservationID, &e.SourceID, &e.CalibrationID,
			&e.FrameUnixNanos, &e.MeasurementUnixNanos, &e.EstimatorID,
			&e.ObservationModelID, &e.ParamHash, &e.Stage, &e.MeasurementSource,
			&e.CreationSequence, &e.X, &e.Y, &e.VX, &e.VY, &covariance, &sensor,
		); err != nil {
			return nil, fmt.Errorf("scan estimate: %w", err)
		}
		if !sensor.Valid || sensor.String == "" {
			return nil, fmt.Errorf("estimate %s names observation %s, which is not stored", e.EstimateID, e.ObservationID)
		}
		r.SensorID = sensor.String
		if err := json.Unmarshal(covariance, &e.Covariance); err != nil {
			return nil, fmt.Errorf("unmarshal estimate covariance %s: %w", e.EstimateID, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate estimates: %w", err)
	}
	return out, nil
}

// SolidBodyWithSensor is a persisted solid body and the sensor that observed
// it, read through the immutable observation the row names.
type SolidBodyWithSensor struct {
	TrackSolidBody
	SensorID string
}

// ListVersionSolidBodies returns one version's solid bodies in track and
// frame order, each with the sensor_id of the lidar_observations row it links
// to. As for ListVersionEstimates, a row whose observation is not stored is an
// error rather than a row with an empty sensor.
func (s *StateEstimateStore) ListVersionSolidBodies(key EstimateVersionKey) ([]SolidBodyWithSensor, error) {
	rows, err := s.db.Query(`SELECT `+solidBodyColumns+`
		     , (SELECT o.sensor_id FROM lidar_observations o WHERE o.observation_id = b.observation_id)
		  FROM lidar_track_solid_bodies b
		 WHERE source_id = ? AND estimator_id = ? AND observation_model_id = ?
		   AND param_hash = ? AND stage = ?
		 ORDER BY track_id, frame_unix_nanos, estimate_id`,
		key.SourceID, key.EstimatorID, key.ObservationModelID, key.ParamHash, key.Stage)
	if err != nil {
		return nil, fmt.Errorf("list solid bodies %s/%s/%s: %w", key.SourceID, key.EstimatorID, key.Stage, err)
	}
	defer rows.Close()
	out := []SolidBodyWithSensor{}
	for rows.Next() {
		var sensor sql.NullString
		sb, err := scanSolidBody(rows, &sensor)
		if err != nil {
			return nil, err
		}
		if !sensor.Valid || sensor.String == "" {
			return nil, fmt.Errorf("solid body %s names observation %s, which is not stored", sb.EstimateID, sb.ObservationID)
		}
		out = append(out, SolidBodyWithSensor{TrackSolidBody: sb, SensorID: sensor.String})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate solid bodies: %w", err)
	}
	return out, nil
}
