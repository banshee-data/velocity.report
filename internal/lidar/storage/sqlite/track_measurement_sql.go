package sqlite

import "database/sql"

// trackMeasurementColumns lists the 15 SQL columns for TrackMeasurement fields,
// shared between lidar_tracks and lidar_run_tracks tables.
const trackMeasurementColumns = `sensor_id, track_state,
	start_unix_nanos, end_unix_nanos, observation_count,
	avg_speed_mps, max_speed_mps,
	bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
	height_p95_max, intensity_mean_avg,
	object_class, object_confidence, classification_model`

// trackMeasurementUpsertSet is the SET clause for ON CONFLICT DO UPDATE,
// covering all 15 TrackMeasurement columns.
const trackMeasurementUpsertSet = `
			sensor_id = excluded.sensor_id,
			track_state = excluded.track_state,
			start_unix_nanos = excluded.start_unix_nanos,
			end_unix_nanos = excluded.end_unix_nanos,
			observation_count = excluded.observation_count,
			avg_speed_mps = excluded.avg_speed_mps,
			max_speed_mps = excluded.max_speed_mps,
			bounding_box_length_avg = excluded.bounding_box_length_avg,
			bounding_box_width_avg = excluded.bounding_box_width_avg,
			bounding_box_height_avg = excluded.bounding_box_height_avg,
			height_p95_max = excluded.height_p95_max,
			intensity_mean_avg = excluded.intensity_mean_avg,
			object_class = excluded.object_class,
			object_confidence = excluded.object_confidence,
			classification_model = excluded.classification_model`

// trackMeasurementUpdateSet is the SET clause for UPDATE statements.
// Excludes sensor_id and start_unix_nanos which are immutable after creation.
const trackMeasurementUpdateSet = `
		track_state = ?,
		end_unix_nanos = ?,
		observation_count = ?,
		avg_speed_mps = ?,
		max_speed_mps = ?,
		bounding_box_length_avg = ?,
		bounding_box_width_avg = ?,
		bounding_box_height_avg = ?,
		height_p95_max = ?,
		intensity_mean_avg = ?,
		object_class = ?,
		object_confidence = ?,
		classification_model = ?`

// trackMeasurementInsertArgs returns the 15 values for inserting or upserting
// TrackMeasurement fields. Nullable columns (end_unix_nanos, object_class,
// object_confidence, classification_model) are wrapped for SQL NULL handling.
func trackMeasurementInsertArgs(m *TrackMeasurement) []any {
	var endNanos any
	if m.EndUnixNanos > 0 {
		endNanos = m.EndUnixNanos
	}
	return []any{
		m.SensorID,
		string(m.TrackState),
		m.StartUnixNanos,
		endNanos,
		m.ObservationCount,
		m.AvgSpeedMps,
		m.MaxSpeedMps,
		m.BoundingBoxLengthAvg,
		m.BoundingBoxWidthAvg,
		m.BoundingBoxHeightAvg,
		m.HeightP95Max,
		m.IntensityMeanAvg,
		nullString(m.ObjectClass),
		nullFloat32(m.ObjectConfidence),
		nullString(m.ClassificationModel),
	}
}

// trackMeasurementUpdateArgs returns the 13 values for updating
// TrackMeasurement fields. Excludes sensor_id and start_unix_nanos
// which are immutable after track creation.
func trackMeasurementUpdateArgs(m *TrackMeasurement) []any {
	var endNanos any
	if m.EndUnixNanos > 0 {
		endNanos = m.EndUnixNanos
	}
	return []any{
		string(m.TrackState),
		endNanos,
		m.ObservationCount,
		m.AvgSpeedMps,
		m.MaxSpeedMps,
		m.BoundingBoxLengthAvg,
		m.BoundingBoxWidthAvg,
		m.BoundingBoxHeightAvg,
		m.HeightP95Max,
		m.IntensityMeanAvg,
		nullString(m.ObjectClass),
		nullFloat32(m.ObjectConfidence),
		nullString(m.ClassificationModel),
	}
}

// scanTrackMeasurementDests returns scan destinations for the 15
// TrackMeasurement columns and a finalise function that must be called
// after a successful Scan to apply nullable values. The caller assembles
// the full destination list by prepending/appending type-specific fields.
func scanTrackMeasurementDests(m *TrackMeasurement) (dests []any, apply func()) {
	var endNanos sql.NullInt64
	var objectClass sql.NullString
	var objectConfidence sql.NullFloat64
	var classificationModel sql.NullString

	dests = []any{
		&m.SensorID,
		&m.TrackState,
		&m.StartUnixNanos,
		&endNanos,
		&m.ObservationCount,
		&m.AvgSpeedMps,
		&m.MaxSpeedMps,
		&m.BoundingBoxLengthAvg,
		&m.BoundingBoxWidthAvg,
		&m.BoundingBoxHeightAvg,
		&m.HeightP95Max,
		&m.IntensityMeanAvg,
		&objectClass,
		&objectConfidence,
		&classificationModel,
	}

	apply = func() {
		if endNanos.Valid {
			m.EndUnixNanos = endNanos.Int64
		}
		if objectClass.Valid {
			m.ObjectClass = objectClass.String
		}
		if objectConfidence.Valid {
			m.ObjectConfidence = float32(objectConfidence.Float64)
		}
		if classificationModel.Valid {
			m.ClassificationModel = classificationModel.String
		}
	}

	return dests, apply
}
