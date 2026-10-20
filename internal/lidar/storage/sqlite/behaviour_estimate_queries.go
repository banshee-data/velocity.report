package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Read path for behaviour analysis over persisted estimates (the provisional
// headway run, internal/report/headway/fieldrun): one exact version of
// lidar_track_estimates or lidar_track_solid_bodies, any stage, with the
// sensor each row was observed by, or one online version of the estimates
// with the published box filed beside each. Version selection is
// ListEstimateVersions', ListSolidBodyVersions' or ListBoxVersions'; this
// reads what was selected.

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
		     , e.reference_point, e.support_instant, o.sensor_id
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
		var reference, support string
		var sensor sql.NullString
		if err := rows.Scan(
			&e.EstimateID, &e.TrackID, &e.ObservationID, &e.SourceID, &e.CalibrationID,
			&e.FrameUnixNanos, &e.MeasurementUnixNanos, &e.EstimatorID,
			&e.ObservationModelID, &e.ParamHash, &e.Stage, &e.MeasurementSource,
			&e.CreationSequence, &e.X, &e.Y, &e.VX, &e.VY, &covariance,
			&reference, &support, &sensor,
		); err != nil {
			return nil, fmt.Errorf("scan estimate: %w", err)
		}
		if !sensor.Valid || sensor.String == "" {
			return nil, fmt.Errorf("estimate %s names observation %s, which is not stored", e.EstimateID, e.ObservationID)
		}
		if err := readEstimateStatement(e, reference, support); err != nil {
			return nil, err
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

// BoxWithEstimate is a published box and the online estimate filed with it:
// the lidar_track_observations row (migration 000062's box columns) at the
// estimate's track and frame, the track's label from lidar_tracks, and the
// sensor of the immutable observation the estimate names.
//
// An observation row carries no version of its own. It is tied to the
// estimate by track and frame: the track id is a UUID the tracker assigns
// within one run, and FrameEvidenceStore writes the row in the estimate's own
// transaction, so the two were filed together.
type BoxWithEstimate struct {
	TrackEstimate
	SensorID string
	// Filed is false when no lidar_track_observations row is filed at the
	// estimate's frame. While the table keyed a row by track and acquisition
	// time (until a migration re-keyed it by frame), two consecutive frames'
	// clusters of a track could carry one acquisition time (a cluster takes
	// its first point's time, and returns of one firing can fall on both
	// sides of a frame seam), so the later frame's row replaced the earlier
	// one and evidence written then has estimates without a row. HasBox is
	// then false too.
	Filed bool
	// HasBox is false when no published box is filed at this frame: no
	// observation row (see Filed), or one recording no box (the box model
	// off, or not yet seeded). The box fields are then zero.
	HasBox                                         bool
	BoxX, BoxY, BoxHeadingRad, BoxLength, BoxWidth float32
	// ClassLabel is lidar_tracks.object_class, the classifier's label as last
	// filed for the track, empty when it never labelled it; ClassConfidence
	// is its confidence, nil with no label.
	ClassLabel      string
	ClassConfidence *float32
}

// boxJoin ties each estimate to its frame's observation row and its track's
// row. Only the online stage is read: the published box is the online
// tracker's, and a refined stage revises the filter's state, not the box.
const boxJoin = `
		  FROM lidar_track_estimates e
		  LEFT JOIN lidar_track_observations b
		    ON b.track_id = e.track_id AND b.frame_unix_nanos = e.frame_unix_nanos
		  LEFT JOIN lidar_tracks t ON t.track_id = e.track_id`

// ListBoxVersions lists the online estimate versions with at least one
// published box filed beside them, counting the boxes and their tracks.
func (s *StateEstimateStore) ListBoxVersions() ([]EstimateVersion, error) {
	rows, err := s.db.Query(`
		SELECT e.source_id, e.estimator_id, e.observation_model_id, e.param_hash, e.stage
		     , COUNT(*), COUNT(DISTINCT e.track_id)`+boxJoin+`
		 WHERE e.stage = ? AND b.box_length IS NOT NULL
		 GROUP BY e.source_id, e.estimator_id, e.observation_model_id, e.param_hash, e.stage
		 ORDER BY e.source_id, e.estimator_id, e.observation_model_id, e.param_hash, e.stage`, EstimateStageOnline)
	if err != nil {
		return nil, fmt.Errorf("list box versions: %w", err)
	}
	defer rows.Close()
	out := []EstimateVersion{}
	for rows.Next() {
		var v EstimateVersion
		if err := rows.Scan(&v.SourceID, &v.EstimatorID, &v.ObservationModelID, &v.ParamHash, &v.Stage,
			&v.Estimates, &v.Tracks); err != nil {
			return nil, fmt.Errorf("scan box version: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate box versions: %w", err)
	}
	return out, nil
}

// ListVersionBoxes returns one online version's estimates in track and frame
// order, each with the published box filed beside it. An estimate has at
// most one observation row at its frame, and one without is returned with
// Filed false (see BoxWithEstimate.Filed). A version with no observation row
// at all is refused rather than read as having no boxes: its replay did not
// file them (it was written before FrameEvidenceStore did). So is an
// estimate whose observation is not stored or whose box is only partly
// recorded.
func (s *StateEstimateStore) ListVersionBoxes(key EstimateVersionKey) ([]BoxWithEstimate, error) {
	if key.Stage != EstimateStageOnline {
		return nil, fmt.Errorf("a published box is filed beside online estimates only, not %s ones", key.Stage)
	}
	rows, err := s.db.Query(`
		SELECT e.estimate_id, e.track_id, e.observation_id, e.source_id, e.calibration_id
		     , e.frame_unix_nanos, e.measurement_unix_nanos, e.estimator_id
		     , e.observation_model_id, e.param_hash, e.stage, e.measurement_source
		     , e.creation_sequence, e.x, e.y, e.vx, e.vy, e.covariance_json
		     , e.reference_point, e.support_instant
		     , (SELECT o.sensor_id FROM lidar_observations o WHERE o.observation_id = e.observation_id)
		     , b.track_id IS NOT NULL, b.box_x, b.box_y, b.box_heading_rad, b.box_length, b.box_width
		     , t.object_class, t.object_confidence`+boxJoin+`
		 WHERE e.source_id = ? AND e.estimator_id = ? AND e.observation_model_id = ?
		   AND e.param_hash = ? AND e.stage = ?
		 ORDER BY e.track_id, e.frame_unix_nanos, e.estimate_id`,
		key.SourceID, key.EstimatorID, key.ObservationModelID, key.ParamHash, key.Stage)
	if err != nil {
		return nil, fmt.Errorf("list boxes %s/%s/%s: %w", key.SourceID, key.EstimatorID, key.Stage, err)
	}
	defer rows.Close()
	out := []BoxWithEstimate{}
	for rows.Next() {
		var r BoxWithEstimate
		e := &r.TrackEstimate
		var covariance []byte
		var reference, support string
		var sensor, label sql.NullString
		var recorded bool
		var box [5]sql.NullFloat64
		var confidence sql.NullFloat64
		if err := rows.Scan(
			&e.EstimateID, &e.TrackID, &e.ObservationID, &e.SourceID, &e.CalibrationID,
			&e.FrameUnixNanos, &e.MeasurementUnixNanos, &e.EstimatorID,
			&e.ObservationModelID, &e.ParamHash, &e.Stage, &e.MeasurementSource,
			&e.CreationSequence, &e.X, &e.Y, &e.VX, &e.VY, &covariance,
			&reference, &support, &sensor,
			&recorded, &box[0], &box[1], &box[2], &box[3], &box[4],
			&label, &confidence,
		); err != nil {
			return nil, fmt.Errorf("scan box: %w", err)
		}
		if !sensor.Valid || sensor.String == "" {
			return nil, fmt.Errorf("estimate %s names observation %s, which is not stored", e.EstimateID, e.ObservationID)
		}
		r.Filed = recorded
		if n := len(out); n > 0 && out[n-1].EstimateID == e.EstimateID {
			return nil, fmt.Errorf("estimate %s has more than one lidar_track_observations row at its frame", e.EstimateID)
		}
		present := 0
		for _, v := range box {
			if v.Valid {
				present++
			}
		}
		switch present {
		case len(box):
			r.HasBox = true
			r.BoxX, r.BoxY, r.BoxHeadingRad = float32(box[0].Float64), float32(box[1].Float64), float32(box[2].Float64)
			r.BoxLength, r.BoxWidth = float32(box[3].Float64), float32(box[4].Float64)
		case 0:
		default:
			return nil, fmt.Errorf("estimate %s: the observation at its frame records %d of the box's 5 columns", e.EstimateID, present)
		}
		if label.Valid && label.String != "" {
			r.ClassLabel = label.String
			if confidence.Valid {
				c := float32(confidence.Float64)
				r.ClassConfidence = &c
			}
		}
		if err := readEstimateStatement(e, reference, support); err != nil {
			return nil, err
		}
		r.SensorID = sensor.String
		if err := json.Unmarshal(covariance, &e.Covariance); err != nil {
			return nil, fmt.Errorf("unmarshal estimate covariance %s: %w", e.EstimateID, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate boxes: %w", err)
	}
	filed := false
	for _, r := range out {
		filed = filed || r.Filed
	}
	if len(out) > 0 && !filed {
		return nil, fmt.Errorf("no estimate of %s/%s/%s/%s has a lidar_track_observations row at its frame, so no "+
			"published box was filed with them; the database was written by a replay that did not record the box",
			key.SourceID, key.EstimatorID, key.ObservationModelID, key.ParamHash)
	}
	return out, nil
}
