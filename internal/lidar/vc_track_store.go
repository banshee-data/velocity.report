package lidar

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

// InsertVCTrack inserts or updates a velocity-coherent track in the database.
func InsertVCTrack(db *sql.DB, track *VelocityCoherentTrack, worldFrame string) error {
	if db == nil || track == nil {
		return nil
	}

	query := `
		INSERT INTO lidar_velocity_coherent_tracks (
			track_id, sensor_id, world_frame, track_state,
			start_unix_nanos, end_unix_nanos, observation_count, hits, misses,
			avg_speed_mps, peak_speed_mps,
			avg_velocity_confidence, velocity_consistency_score,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			min_points_observed, sparse_frame_count,
			object_class, object_confidence
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(track_id) DO UPDATE SET
			track_state = excluded.track_state,
			end_unix_nanos = excluded.end_unix_nanos,
			observation_count = excluded.observation_count,
			hits = excluded.hits,
			misses = excluded.misses,
			avg_speed_mps = excluded.avg_speed_mps,
			peak_speed_mps = excluded.peak_speed_mps,
			avg_velocity_confidence = excluded.avg_velocity_confidence,
			velocity_consistency_score = excluded.velocity_consistency_score,
			bounding_box_length_avg = excluded.bounding_box_length_avg,
			bounding_box_width_avg = excluded.bounding_box_width_avg,
			bounding_box_height_avg = excluded.bounding_box_height_avg,
			height_p95_max = excluded.height_p95_max,
			intensity_mean_avg = excluded.intensity_mean_avg,
			min_points_observed = excluded.min_points_observed,
			sparse_frame_count = excluded.sparse_frame_count,
			object_class = excluded.object_class,
			object_confidence = excluded.object_confidence
	`

	_, err := db.Exec(query,
		track.TrackID,
		track.SensorID,
		worldFrame,
		string(track.State),
		track.FirstUnixNanos,
		track.LastUnixNanos,
		track.ObservationCount,
		track.Hits,
		track.Misses,
		track.AvgSpeedMps,
		track.PeakSpeedMps,
		track.VelocityConfidence,
		track.VelocityConsistency,
		track.BoundingBoxLengthAvg,
		track.BoundingBoxWidthAvg,
		track.BoundingBoxHeightAvg,
		track.HeightP95Max,
		track.IntensityMeanAvg,
		track.MinPointsObserved,
		track.SparseFrameCount,
		track.ObjectClass,
		track.ObjectConfidence,
	)

	return err
}

// VCTrackObservation represents a single observation for a velocity-coherent track.
type VCTrackObservation struct {
	TrackID            string
	TSUnixNanos        int64
	WorldFrame         string
	X, Y, Z            float32
	VelocityX          float32
	VelocityY          float32
	VelocityZ          float32
	VelocityConfidence float32
	SpeedMps           float32
	HeadingRad         float32
	BoundingBoxLength  float32
	BoundingBoxWidth   float32
	BoundingBoxHeight  float32
	HeightP95          float32
	IntensityMean      float32
	PointsCount        int
}

// InsertVCTrackObservation inserts a velocity-coherent track observation.
func InsertVCTrackObservation(db *sql.DB, obs *VCTrackObservation) error {
	if db == nil || obs == nil {
		return nil
	}

	query := `
		INSERT INTO lidar_velocity_coherent_track_obs (
			track_id, ts_unix_nanos, world_frame,
			x, y, z,
			velocity_x, velocity_y, velocity_z, velocity_confidence,
			speed_mps, heading_rad,
			bounding_box_length, bounding_box_width, bounding_box_height,
			height_p95, intensity_mean, points_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(track_id, ts_unix_nanos) DO NOTHING
	`

	_, err := db.Exec(query,
		obs.TrackID,
		obs.TSUnixNanos,
		obs.WorldFrame,
		obs.X, obs.Y, obs.Z,
		obs.VelocityX, obs.VelocityY, obs.VelocityZ, obs.VelocityConfidence,
		obs.SpeedMps, obs.HeadingRad,
		obs.BoundingBoxLength, obs.BoundingBoxWidth, obs.BoundingBoxHeight,
		obs.HeightP95, obs.IntensityMean, obs.PointsCount,
	)

	return err
}

// InsertVCCluster inserts a velocity-coherent cluster.
func InsertVCCluster(db *sql.DB, cluster VelocityCoherentCluster) error {
	if db == nil {
		return nil
	}

	query := `
		INSERT INTO lidar_velocity_coherent_clusters (
			sensor_id, ts_unix_nanos,
			centroid_x, centroid_y, centroid_z,
			velocity_x, velocity_y, velocity_z, velocity_confidence,
			points_count,
			bounding_box_length, bounding_box_width, bounding_box_height,
			height_p95, intensity_mean
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query,
		cluster.SensorID,
		cluster.TSUnixNanos,
		cluster.CentroidX, cluster.CentroidY, cluster.CentroidZ,
		cluster.VelocityX, cluster.VelocityY, cluster.VelocityZ,
		cluster.VelocityConfidence,
		cluster.PointCount,
		cluster.BoundingBoxLength, cluster.BoundingBoxWidth, cluster.BoundingBoxHeight,
		cluster.HeightP95, cluster.IntensityMean,
	)

	return err
}

// InsertTrackMerge records a track merge operation.
func InsertTrackMerge(db *sql.DB, merge MergeCandidatePair, resultTrackID string) error {
	if db == nil {
		return nil
	}

	query := `
		INSERT INTO lidar_track_merges (
			merged_at, earlier_track_id, later_track_id, result_track_id,
			position_score, velocity_score, trajectory_score, overall_score,
			gap_seconds, interpolated_points
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query,
		time.Now().UnixNano(),
		merge.Earlier.Track.TrackID,
		merge.Later.Track.TrackID,
		resultTrackID,
		merge.PositionScore,
		merge.VelocityScore,
		merge.TrajectoryScore,
		merge.OverallScore,
		merge.GapSeconds,
		0, // interpolated_points - could be computed
	)

	return err
}

// LogAlgorithmConfig logs an algorithm configuration change.
func LogAlgorithmConfig(db *sql.DB, algorithm TrackingAlgorithm, configJSON string, changedBy string) error {
	if db == nil {
		return nil
	}

	query := `
		INSERT INTO lidar_algorithm_config_log (ts_unix_nanos, algorithm, config_json, changed_by)
		VALUES (?, ?, ?, ?)
	`

	_, err := db.Exec(query, time.Now().UnixNano(), string(algorithm), configJSON, changedBy)
	return err
}

// QueryVCTracks queries velocity-coherent tracks from the database.
func QueryVCTracks(db *sql.DB, sensorID string, start, end time.Time, limit int) ([]*VelocityCoherentTrack, error) {
	if db == nil {
		return nil, nil
	}

	query := `
		SELECT
			track_id, sensor_id, track_state,
			start_unix_nanos, end_unix_nanos, observation_count, hits, misses,
			avg_speed_mps, peak_speed_mps,
			avg_velocity_confidence, velocity_consistency_score,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			min_points_observed, sparse_frame_count,
			object_class, object_confidence
		FROM lidar_velocity_coherent_tracks
		WHERE sensor_id = ?
		AND start_unix_nanos >= ? AND start_unix_nanos <= ?
		ORDER BY start_unix_nanos DESC
		LIMIT ?
	`

	rows, err := db.Query(query, sensorID, start.UnixNano(), end.UnixNano(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []*VelocityCoherentTrack
	for rows.Next() {
		var track VelocityCoherentTrack
		var stateStr string
		var objectClass, objectConfidence sql.NullString

		err := rows.Scan(
			&track.TrackID, &track.SensorID, &stateStr,
			&track.FirstUnixNanos, &track.LastUnixNanos, &track.ObservationCount, &track.Hits, &track.Misses,
			&track.AvgSpeedMps, &track.PeakSpeedMps,
			&track.VelocityConfidence, &track.VelocityConsistency,
			&track.BoundingBoxLengthAvg, &track.BoundingBoxWidthAvg, &track.BoundingBoxHeightAvg,
			&track.HeightP95Max, &track.IntensityMeanAvg,
			&track.MinPointsObserved, &track.SparseFrameCount,
			&objectClass, &objectConfidence,
		)
		if err != nil {
			return nil, err
		}

		track.State = TrackStateVC(stateStr)
		if objectClass.Valid {
			track.ObjectClass = objectClass.String
		}
		if objectConfidence.Valid {
			// Parse confidence from string if needed
		}

		tracks = append(tracks, &track)
	}

	return tracks, rows.Err()
}

// ClearVCTracks deletes all VC tracks and observations for a sensor.
func ClearVCTracks(db *sql.DB, sensorID string) error {
	if db == nil {
		return nil
	}

	// Delete observations first (foreign key)
	_, err := db.Exec(`
		DELETE FROM lidar_velocity_coherent_track_obs
		WHERE track_id IN (
			SELECT track_id FROM lidar_velocity_coherent_tracks WHERE sensor_id = ?
		)
	`, sensorID)
	if err != nil {
		return fmt.Errorf("delete VC track observations: %w", err)
	}

	// Delete tracks
	_, err = db.Exec(`DELETE FROM lidar_velocity_coherent_tracks WHERE sensor_id = ?`, sensorID)
	if err != nil {
		return fmt.Errorf("delete VC tracks: %w", err)
	}

	// Delete clusters
	_, err = db.Exec(`DELETE FROM lidar_velocity_coherent_clusters WHERE sensor_id = ?`, sensorID)
	if err != nil {
		return fmt.Errorf("delete VC clusters: %w", err)
	}

	return nil
}

// VCTrackFromCluster creates a VCTrackObservation from a cluster.
func VCTrackObsFromCluster(trackID string, cluster VelocityCoherentCluster, worldFrame string) *VCTrackObservation {
	speed := float32(math.Sqrt(cluster.VelocityX*cluster.VelocityX + cluster.VelocityY*cluster.VelocityY))
	heading := float32(math.Atan2(cluster.VelocityY, cluster.VelocityX))

	return &VCTrackObservation{
		TrackID:            trackID,
		TSUnixNanos:        cluster.TSUnixNanos,
		WorldFrame:         worldFrame,
		X:                  float32(cluster.CentroidX),
		Y:                  float32(cluster.CentroidY),
		Z:                  float32(cluster.CentroidZ),
		VelocityX:          float32(cluster.VelocityX),
		VelocityY:          float32(cluster.VelocityY),
		VelocityZ:          float32(cluster.VelocityZ),
		VelocityConfidence: cluster.VelocityConfidence,
		SpeedMps:           speed,
		HeadingRad:         heading,
		BoundingBoxLength:  cluster.BoundingBoxLength,
		BoundingBoxWidth:   cluster.BoundingBoxWidth,
		BoundingBoxHeight:  cluster.BoundingBoxHeight,
		HeightP95:          cluster.HeightP95,
		IntensityMean:      cluster.IntensityMean,
		PointsCount:        cluster.PointCount,
	}
}
