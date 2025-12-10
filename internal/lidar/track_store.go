package lidar

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
)

// TrackStore defines the interface for track persistence operations.
type TrackStore interface {
	InsertCluster(cluster *WorldCluster) (int64, error)
	InsertTrack(track *TrackedObject, worldFrame string) error
	UpdateTrack(track *TrackedObject, worldFrame string) error
	InsertTrackObservation(obs *TrackObservation) error
	GetTrack(trackID string) (*TrackedObject, error)
	GetActiveTracks(sensorID string, state string) ([]*TrackedObject, error)
	GetTracksInRange(sensorID string, state string, startNanos, endNanos int64, limit int) ([]*TrackedObject, error)
	GetTrackObservations(trackID string, limit int) ([]*TrackObservation, error)
	GetRecentClusters(sensorID string, startNanos, endNanos int64, limit int) ([]*WorldCluster, error)
}

// TrackObservation represents a single observation of a track at a point in time.
type TrackObservation struct {
	TrackID     string
	TSUnixNanos int64
	WorldFrame  string

	// Position (world frame)
	X, Y, Z float32

	// Velocity (world frame)
	VelocityX, VelocityY float32
	SpeedMps             float32
	HeadingRad           float32

	// Shape
	BoundingBoxLength float32
	BoundingBoxWidth  float32
	BoundingBoxHeight float32
	HeightP95         float32
	IntensityMean     float32
}

// InsertCluster inserts a cluster into the database and returns its ID.
func InsertCluster(db *sql.DB, cluster *WorldCluster) (int64, error) {
	query := `
		INSERT INTO lidar_clusters (
			sensor_id, world_frame, ts_unix_nanos,
			centroid_x, centroid_y, centroid_z,
			bounding_box_length, bounding_box_width, bounding_box_height,
			points_count, height_p95, intensity_mean
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.Exec(query,
		cluster.SensorID,
		cluster.WorldFrame,
		cluster.TSUnixNanos,
		cluster.CentroidX,
		cluster.CentroidY,
		cluster.CentroidZ,
		cluster.BoundingBoxLength,
		cluster.BoundingBoxWidth,
		cluster.BoundingBoxHeight,
		cluster.PointsCount,
		cluster.HeightP95,
		cluster.IntensityMean,
	)
	if err != nil {
		return 0, fmt.Errorf("insert cluster: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get cluster insert ID: %w", err)
	}

	return id, nil
}

// InsertTrack inserts a new track into the database.
func InsertTrack(db *sql.DB, track *TrackedObject, worldFrame string) error {
	// Compute speed percentiles
	p50, p85, p95 := ComputeSpeedPercentiles(track.speedHistory)

	query := `
		INSERT INTO lidar_tracks (
			track_id, sensor_id, world_frame, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Always set end_unix_nanos to LastUnixNanos for all track states
	// This allows accurate time range queries for track history visualization
	endNanos := track.LastUnixNanos

	_, err := db.Exec(query,
		track.TrackID,
		track.SensorID,
		worldFrame,
		string(track.State),
		track.FirstUnixNanos,
		endNanos,
		track.ObservationCount,
		track.AvgSpeedMps,
		track.PeakSpeedMps,
		p50, p85, p95,
		track.BoundingBoxLengthAvg,
		track.BoundingBoxWidthAvg,
		track.BoundingBoxHeightAvg,
		track.HeightP95Max,
		track.IntensityMeanAvg,
		nullString(track.ObjectClass),
		nullFloat32(track.ObjectConfidence),
		nullString(track.ClassificationModel),
	)
	if err != nil {
		return fmt.Errorf("insert track: %w", err)
	}

	return nil
}

// UpdateTrack updates an existing track in the database.
func UpdateTrack(db *sql.DB, track *TrackedObject, worldFrame string) error {
	// Compute speed percentiles
	p50, p85, p95 := ComputeSpeedPercentiles(track.speedHistory)

	query := `
		UPDATE lidar_tracks SET
			track_state = ?,
			end_unix_nanos = ?,
			observation_count = ?,
			avg_speed_mps = ?,
			peak_speed_mps = ?,
			p50_speed_mps = ?,
			p85_speed_mps = ?,
			p95_speed_mps = ?,
			bounding_box_length_avg = ?,
			bounding_box_width_avg = ?,
			bounding_box_height_avg = ?,
			height_p95_max = ?,
			intensity_mean_avg = ?,
			object_class = ?,
			object_confidence = ?,
			classification_model = ?
		WHERE track_id = ?
	`

	// Always set end_unix_nanos to LastUnixNanos for all track states
	// This allows accurate time range queries for track history visualization
	endNanos := track.LastUnixNanos

	_, err := db.Exec(query,
		string(track.State),
		endNanos,
		track.ObservationCount,
		track.AvgSpeedMps,
		track.PeakSpeedMps,
		p50, p85, p95,
		track.BoundingBoxLengthAvg,
		track.BoundingBoxWidthAvg,
		track.BoundingBoxHeightAvg,
		track.HeightP95Max,
		track.IntensityMeanAvg,
		nullString(track.ObjectClass),
		nullFloat32(track.ObjectConfidence),
		nullString(track.ClassificationModel),
		track.TrackID,
	)
	if err != nil {
		return fmt.Errorf("update track: %w", err)
	}

	return nil
}

// InsertTrackObservation inserts a track observation into the database.
func InsertTrackObservation(db *sql.DB, obs *TrackObservation) error {
	query := `
		INSERT INTO lidar_track_obs (
			track_id, ts_unix_nanos, world_frame,
			x, y, z,
			velocity_x, velocity_y, speed_mps, heading_rad,
			bounding_box_length, bounding_box_width, bounding_box_height,
			height_p95, intensity_mean
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query,
		obs.TrackID,
		obs.TSUnixNanos,
		obs.WorldFrame,
		obs.X, obs.Y, obs.Z,
		obs.VelocityX, obs.VelocityY, obs.SpeedMps, obs.HeadingRad,
		obs.BoundingBoxLength, obs.BoundingBoxWidth, obs.BoundingBoxHeight,
		obs.HeightP95, obs.IntensityMean,
	)
	if err != nil {
		return fmt.Errorf("insert track observation: %w", err)
	}

	return nil
}

// GetActiveTracks retrieves active tracks from the database.
// If state is empty, returns all non-deleted tracks.
func GetActiveTracks(db *sql.DB, sensorID string, state string) ([]*TrackedObject, error) {
	var query string
	var args []interface{}

	if state != "" {
		query = `
			SELECT track_id, sensor_id, track_state,
				start_unix_nanos, end_unix_nanos, observation_count,
				avg_speed_mps, peak_speed_mps,
				bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
				height_p95_max, intensity_mean_avg,
				object_class, object_confidence, classification_model
			FROM lidar_tracks
			WHERE sensor_id = ? AND track_state = ?
			ORDER BY start_unix_nanos DESC
		`
		args = []interface{}{sensorID, state}
	} else {
		query = `
			SELECT track_id, sensor_id, track_state,
				start_unix_nanos, end_unix_nanos, observation_count,
				avg_speed_mps, peak_speed_mps,
				bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
				height_p95_max, intensity_mean_avg,
				object_class, object_confidence, classification_model
			FROM lidar_tracks
			WHERE sensor_id = ? AND track_state != 'deleted'
			ORDER BY start_unix_nanos DESC
		`
		args = []interface{}{sensorID}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query active tracks: %w", err)
	}
	defer rows.Close()

	var tracks []*TrackedObject
	for rows.Next() {
		track := &TrackedObject{}
		var stateStr string
		var endNanos sql.NullInt64
		var objectClass sql.NullString
		var objectConfidence sql.NullFloat64
		var classificationModel sql.NullString

		err := rows.Scan(
			&track.TrackID,
			&track.SensorID,
			&stateStr,
			&track.FirstUnixNanos,
			&endNanos,
			&track.ObservationCount,
			&track.AvgSpeedMps,
			&track.PeakSpeedMps,
			&track.BoundingBoxLengthAvg,
			&track.BoundingBoxWidthAvg,
			&track.BoundingBoxHeightAvg,
			&track.HeightP95Max,
			&track.IntensityMeanAvg,
			&objectClass,
			&objectConfidence,
			&classificationModel,
		)
		if err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}

		track.State = TrackState(stateStr)
		if endNanos.Valid {
			track.LastUnixNanos = endNanos.Int64
		}
		if objectClass.Valid {
			track.ObjectClass = objectClass.String
		}
		if objectConfidence.Valid {
			track.ObjectConfidence = float32(objectConfidence.Float64)
		}
		if classificationModel.Valid {
			track.ClassificationModel = classificationModel.String
		}

		tracks = append(tracks, track)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tracks: %w", err)
	}

	// Populate history for each track
	for _, track := range tracks {
		// Fetch recent observations (limit 1000 to capture full history for typical tracks)
		obs, err := GetTrackObservations(db, track.TrackID, 1000)
		if err != nil {
			// Log error but continue, returning track without history is better than failing
			continue
		}

		// Convert observations to TrackPoint history
		// GetTrackObservations returns DESC (newest first), so we prepend or reverse
		// Pre-allocate history slice
		track.History = make([]TrackPoint, len(obs))
		for i, o := range obs {
			// Store in reverse order (oldest first) for chronological history
			idx := len(obs) - 1 - i
			track.History[idx] = TrackPoint{
				X:         o.X,
				Y:         o.Y,
				Timestamp: o.TSUnixNanos,
			}
		}
	}

	return tracks, nil
}

// GetTracksInRange retrieves tracks whose lifespan overlaps the given time window (nanoseconds).
// A track is included if its start is on/before endNanos and its end (or start when end is NULL) is on/after startNanos.
// Deleted tracks are excluded by default unless state explicitly requests them.
func GetTracksInRange(db *sql.DB, sensorID string, state string, startNanos, endNanos int64, limit int) ([]*TrackedObject, error) {
	if limit <= 0 {
		limit = 100
	}

	var query strings.Builder
	var args []interface{}

	query.WriteString(`
		SELECT track_id, sensor_id, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, peak_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model
		FROM lidar_tracks
		WHERE sensor_id = ?
	`)
	args = append(args, sensorID)

	if state != "" {
		query.WriteString(" AND track_state = ?")
		args = append(args, state)
	} else {
		query.WriteString(" AND track_state != 'deleted'")
	}

	query.WriteString(`
		AND start_unix_nanos <= ?
		AND COALESCE(end_unix_nanos, start_unix_nanos) >= ?
		ORDER BY start_unix_nanos ASC
		LIMIT ?
	`)
	args = append(args, endNanos, startNanos, limit)

	rows, err := db.Query(query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query tracks in range: %w", err)
	}
	defer rows.Close()

	var tracks []*TrackedObject
	for rows.Next() {
		track := &TrackedObject{}
		var stateStr string
		var end sql.NullInt64
		var objectClass sql.NullString
		var objectConfidence sql.NullFloat64
		var classificationModel sql.NullString

		err := rows.Scan(
			&track.TrackID,
			&track.SensorID,
			&stateStr,
			&track.FirstUnixNanos,
			&end,
			&track.ObservationCount,
			&track.AvgSpeedMps,
			&track.PeakSpeedMps,
			&track.BoundingBoxLengthAvg,
			&track.BoundingBoxWidthAvg,
			&track.BoundingBoxHeightAvg,
			&track.HeightP95Max,
			&track.IntensityMeanAvg,
			&objectClass,
			&objectConfidence,
			&classificationModel,
		)
		if err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}

		track.State = TrackState(stateStr)
		if end.Valid {
			track.LastUnixNanos = end.Int64
		}
		if objectClass.Valid {
			track.ObjectClass = objectClass.String
		}
		if objectConfidence.Valid {
			track.ObjectConfidence = float32(objectConfidence.Float64)
		}
		if classificationModel.Valid {
			track.ClassificationModel = classificationModel.String
		}

		tracks = append(tracks, track)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tracks: %w", err)
	}

	for _, track := range tracks {
		obs, err := GetTrackObservations(db, track.TrackID, 1000)
		if err != nil {
			continue
		}

		track.History = make([]TrackPoint, len(obs))
		for i, o := range obs {
			idx := len(obs) - 1 - i
			track.History[idx] = TrackPoint{
				X:         o.X,
				Y:         o.Y,
				Timestamp: o.TSUnixNanos,
			}
		}
	}

	return tracks, nil
}

// GetTrackObservations retrieves observations for a track.
func GetTrackObservations(db *sql.DB, trackID string, limit int) ([]*TrackObservation, error) {
	query := `
		SELECT track_id, ts_unix_nanos, world_frame,
			x, y, z,
			velocity_x, velocity_y, speed_mps, heading_rad,
			bounding_box_length, bounding_box_width, bounding_box_height,
			height_p95, intensity_mean
		FROM lidar_track_obs
		WHERE track_id = ?
		ORDER BY ts_unix_nanos DESC
		LIMIT ?
	`

	rows, err := db.Query(query, trackID, limit)
	if err != nil {
		return nil, fmt.Errorf("query track observations: %w", err)
	}
	defer rows.Close()

	var observations []*TrackObservation
	for rows.Next() {
		obs := &TrackObservation{}
		err := rows.Scan(
			&obs.TrackID,
			&obs.TSUnixNanos,
			&obs.WorldFrame,
			&obs.X, &obs.Y, &obs.Z,
			&obs.VelocityX, &obs.VelocityY, &obs.SpeedMps, &obs.HeadingRad,
			&obs.BoundingBoxLength, &obs.BoundingBoxWidth, &obs.BoundingBoxHeight,
			&obs.HeightP95, &obs.IntensityMean,
		)
		if err != nil {
			return nil, fmt.Errorf("scan observation: %w", err)
		}
		observations = append(observations, obs)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observations: %w", err)
	}

	return observations, nil
}

// GetRecentClusters retrieves recent clusters from the database.
func GetRecentClusters(db *sql.DB, sensorID string, startNanos, endNanos int64, limit int) ([]*WorldCluster, error) {
	query := `
		SELECT lidar_cluster_id, sensor_id, world_frame, ts_unix_nanos,
			centroid_x, centroid_y, centroid_z,
			bounding_box_length, bounding_box_width, bounding_box_height,
			points_count, height_p95, intensity_mean
		FROM lidar_clusters
		WHERE sensor_id = ? AND ts_unix_nanos >= ? AND ts_unix_nanos <= ?
		ORDER BY ts_unix_nanos DESC
		LIMIT ?
	`

	rows, err := db.Query(query, sensorID, startNanos, endNanos, limit)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	var clusters []*WorldCluster
	for rows.Next() {
		c := &WorldCluster{}
		err := rows.Scan(
			&c.ClusterID,
			&c.SensorID,
			&c.WorldFrame,
			&c.TSUnixNanos,
			&c.CentroidX, &c.CentroidY, &c.CentroidZ,
			&c.BoundingBoxLength, &c.BoundingBoxWidth, &c.BoundingBoxHeight,
			&c.PointsCount, &c.HeightP95, &c.IntensityMean,
		)
		if err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		clusters = append(clusters, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clusters: %w", err)
	}

	return clusters, nil
}

// Helper functions for nullable values

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullFloat32(f float32) interface{} {
	if math.IsNaN(float64(f)) {
		return nil
	}
	return f
}
