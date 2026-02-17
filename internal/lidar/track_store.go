package lidar

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// TrackStore defines the interface for track persistence operations.
type TrackStore interface {
	InsertCluster(cluster *WorldCluster) (int64, error)
	InsertTrack(track *TrackedObject, worldFrame string) error
	UpdateTrack(track *TrackedObject, worldFrame string) error
	InsertTrackObservation(obs *TrackObservation) error
	ClearTracks(sensorID string) error
	GetTrack(trackID string) (*TrackedObject, error)
	GetActiveTracks(sensorID string, state string) ([]*TrackedObject, error)
	GetTracksInRange(sensorID string, state string, startNanos, endNanos int64, limit int) ([]*TrackedObject, error)
	GetTrackObservations(trackID string, limit int) ([]*TrackObservation, error)
	GetTrackObservationsInRange(sensorID string, startNanos, endNanos int64, limit int, trackID string) ([]*TrackObservation, error)
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

	// Use ON CONFLICT DO UPDATE to avoid cascade deleting observations
	// (INSERT OR REPLACE would delete the row first, triggering cascade delete on lidar_track_obs)
	query := `
		INSERT INTO lidar_tracks (
			track_id, sensor_id, world_frame, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(track_id) DO UPDATE SET
			sensor_id = excluded.sensor_id,
			world_frame = excluded.world_frame,
			track_state = excluded.track_state,
			start_unix_nanos = excluded.start_unix_nanos,
			end_unix_nanos = excluded.end_unix_nanos,
			observation_count = excluded.observation_count,
			avg_speed_mps = excluded.avg_speed_mps,
			peak_speed_mps = excluded.peak_speed_mps,
			p50_speed_mps = excluded.p50_speed_mps,
			p85_speed_mps = excluded.p85_speed_mps,
			p95_speed_mps = excluded.p95_speed_mps,
			bounding_box_length_avg = excluded.bounding_box_length_avg,
			bounding_box_width_avg = excluded.bounding_box_width_avg,
			bounding_box_height_avg = excluded.bounding_box_height_avg,
			height_p95_max = excluded.height_p95_max,
			intensity_mean_avg = excluded.intensity_mean_avg,
			object_class = excluded.object_class,
			object_confidence = excluded.object_confidence,
			classification_model = excluded.classification_model
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
		INSERT OR REPLACE INTO lidar_track_obs (
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

// PruneDeletedTracks removes tracks in the 'deleted' state (and their
// observations) whose last update is older than the supplied TTL. This
// prevents the database from growing unboundedly as the tracker
// continuously creates and deletes short-lived spurious tracks.
// Returns the number of tracks pruned and any error encountered.
func PruneDeletedTracks(db *sql.DB, sensorID string, ttl time.Duration) (int64, error) {
	if sensorID == "" {
		return 0, fmt.Errorf("sensorID is required to prune deleted tracks")
	}

	cutoffNanos := time.Now().Add(-ttl).UnixNano()

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin prune tx: %w", err)
	}

	// Delete orphaned observations first (foreign-key safe).
	_, err = tx.Exec(`
		DELETE FROM lidar_track_obs
		WHERE track_id IN (
			SELECT track_id FROM lidar_tracks
			WHERE sensor_id = ? AND state = 'deleted'
			  AND COALESCE(end_unix_nanos, start_unix_nanos) < ?
		)`, sensorID, cutoffNanos)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("prune observations: %w", err)
	}

	// Delete the track rows themselves.
	res, err := tx.Exec(`
		DELETE FROM lidar_tracks
		WHERE sensor_id = ? AND state = 'deleted'
		  AND COALESCE(end_unix_nanos, start_unix_nanos) < ?`,
		sensorID, cutoffNanos)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("prune tracks: %w", err)
	}

	pruned, _ := res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prune tx: %w", err)
	}

	return pruned, nil
}

// ClearTracks removes all tracks, observations, and clusters for a sensor.
// This is intended for development/debug resets and should not be exposed in production without auth.
func ClearTracks(db *sql.DB, sensorID string) error {
	if sensorID == "" {
		return fmt.Errorf("sensorID is required to clear tracks")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin clear tracks tx: %w", err)
	}

	steps := []struct {
		query string
	}{
		{query: `DELETE FROM lidar_track_obs WHERE track_id IN (SELECT track_id FROM lidar_tracks WHERE sensor_id = ?)`},
		{query: `DELETE FROM lidar_tracks WHERE sensor_id = ?`},
		{query: `DELETE FROM lidar_clusters WHERE sensor_id = ?`},
	}

	for _, step := range steps {
		if _, err := tx.Exec(step.query, sensorID); err != nil {
			tx.Rollback()
			return fmt.Errorf("clear tracks step failed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit clear tracks tx: %w", err)
	}

	return nil
}

// ClearRuns removes all analysis runs and their associated run tracks for a sensor.
// This is intended for development/debug resets and should not be exposed in production without auth.
// The CASCADE foreign key on lidar_run_tracks will automatically delete associated run track records.
func ClearRuns(db *sql.DB, sensorID string) error {
	if sensorID == "" {
		return fmt.Errorf("sensorID is required to clear runs")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin clear runs tx: %w", err)
	}

	// Delete runs for this sensor (CASCADE will delete lidar_run_tracks)
	query := `DELETE FROM lidar_analysis_runs WHERE sensor_id = ?`
	if _, err := tx.Exec(query, sensorID); err != nil {
		tx.Rollback()
		return fmt.Errorf("clear runs failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit clear runs tx: %w", err)
	}

	return nil
}

// DeleteRun removes a specific analysis run and its associated run tracks.
// The CASCADE foreign key on lidar_run_tracks will automatically delete associated run track records.
func DeleteRun(db *sql.DB, runID string) error {
	if runID == "" {
		return fmt.Errorf("runID is required to delete run")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete run tx: %w", err)
	}

	// Delete the run (CASCADE will delete lidar_run_tracks)
	query := `DELETE FROM lidar_analysis_runs WHERE run_id = ?`
	result, err := tx.Exec(query, runID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("delete run failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		tx.Rollback()
		return fmt.Errorf("run not found: %s", runID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete run tx: %w", err)
	}

	return nil
}

// GetTrackObservationsInRange returns observations for a sensor within a time window (inclusive).
// Joins against tracks to scope by sensor.
func GetTrackObservationsInRange(db *sql.DB, sensorID string, startNanos, endNanos int64, limit int, trackID string) ([]*TrackObservation, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT o.track_id, o.ts_unix_nanos, o.world_frame,
			o.x, o.y, o.z,
			o.velocity_x, o.velocity_y, o.speed_mps, o.heading_rad,
			o.bounding_box_length, o.bounding_box_width, o.bounding_box_height,
			o.height_p95, o.intensity_mean
		FROM lidar_track_obs o
		JOIN lidar_tracks t ON o.track_id = t.track_id
		WHERE t.sensor_id = ? AND o.ts_unix_nanos BETWEEN ? AND ?
	`
	args := []interface{}{sensorID, startNanos, endNanos}

	if trackID != "" {
		query += " AND o.track_id = ?"
		args = append(args, trackID)
	}

	query += " ORDER BY o.ts_unix_nanos ASC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query track observations in range: %w", err)
	}
	defer rows.Close()

	var observations []*TrackObservation
	for rows.Next() {
		obs := &TrackObservation{}
		if err := rows.Scan(
			&obs.TrackID,
			&obs.TSUnixNanos,
			&obs.WorldFrame,
			&obs.X, &obs.Y, &obs.Z,
			&obs.VelocityX, &obs.VelocityY, &obs.SpeedMps, &obs.HeadingRad,
			&obs.BoundingBoxLength, &obs.BoundingBoxWidth, &obs.BoundingBoxHeight,
			&obs.HeightP95, &obs.IntensityMean,
		); err != nil {
			return nil, fmt.Errorf("scan track observation: %w", err)
		}
		observations = append(observations, obs)
	}

	return observations, nil
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

	// Populate history for each track, scoped to a recency window to avoid
	// pulling observations from unrelated sessions with reused track IDs.
	nowNanos := time.Now().UnixNano()
	recencyWindow := int64(60 * time.Second) // 60 s of recent history
	for _, track := range tracks {
		obsStart := nowNanos - recencyWindow
		// Use the track's own lifetime if it falls within the recency window
		if track.FirstUnixNanos > obsStart {
			obsStart = track.FirstUnixNanos
		}
		obs, err := GetTrackObservationsInRange(db, sensorID, obsStart, nowNanos, 1000, track.TrackID)
		if err != nil {
			// Log error but continue, returning track without history is better than failing
			continue
		}

		// GetTrackObservationsInRange returns ASC (oldest first)
		track.History = make([]TrackPoint, len(obs))
		for i, o := range obs {
			track.History[i] = TrackPoint{
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
		// Use time-scoped observation query to ensure history stays within
		// the requested window. This prevents cross-session contamination
		// when track IDs have been reused across resets.
		obs, err := GetTrackObservationsInRange(db, sensorID, startNanos, endNanos, 1000, track.TrackID)
		if err != nil {
			continue
		}

		// GetTrackObservationsInRange returns ASC (oldest first)
		track.History = make([]TrackPoint, len(obs))
		for i, o := range obs {
			track.History[i] = TrackPoint{
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
