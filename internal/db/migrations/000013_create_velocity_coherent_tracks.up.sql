-- Migration 000013: Velocity-Coherent Tracking Tables
-- Creates parallel tables for velocity-coherent algorithm output
-- These exist alongside lidar_tracks for dual-source comparison
-- Velocity-coherent clustering results (6D DBSCAN output)
   CREATE TABLE IF NOT EXISTS lidar_velocity_coherent_clusters (
          cluster_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , centroid_x REAL
        , centroid_y REAL
        , centroid_z REAL
        , velocity_x REAL
        , velocity_y REAL
        , velocity_z REAL
        , velocity_confidence REAL
        , points_count INTEGER
        , bounding_box_length REAL
        , bounding_box_width REAL
        , bounding_box_height REAL
        , height_p95 REAL
        , intensity_mean REAL
          );

CREATE INDEX IF NOT EXISTS idx_vc_clusters_sensor_time ON lidar_velocity_coherent_clusters (sensor_id, ts_unix_nanos);

-- Velocity-coherent tracks (parallel to lidar_tracks)
-- This table stores tracks from the velocity-coherent algorithm for comparison with
-- background-subtraction tracks in lidar_tracks
   CREATE TABLE IF NOT EXISTS lidar_velocity_coherent_tracks (
          track_id TEXT PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , world_frame TEXT NOT NULL
        , track_state TEXT NOT NULL
        , start_unix_nanos INTEGER NOT NULL
        , end_unix_nanos INTEGER
        , observation_count INTEGER
        , hits INTEGER
        , misses INTEGER
        , avg_speed_mps REAL
        , peak_speed_mps REAL
        , p50_speed_mps REAL
        , p85_speed_mps REAL
        , p95_speed_mps REAL
        , avg_velocity_confidence REAL
        , velocity_consistency_score REAL
        , bounding_box_length_avg REAL
        , bounding_box_width_avg REAL
        , bounding_box_height_avg REAL
        , height_p95_max REAL
        , intensity_mean_avg REAL
        , min_points_observed INTEGER
        , sparse_frame_count INTEGER
        , object_class TEXT
        , object_confidence REAL
        , classification_model TEXT
          );

CREATE INDEX IF NOT EXISTS idx_vc_tracks_sensor ON lidar_velocity_coherent_tracks (sensor_id);

CREATE INDEX IF NOT EXISTS idx_vc_tracks_state ON lidar_velocity_coherent_tracks (track_state);

CREATE INDEX IF NOT EXISTS idx_vc_tracks_time ON lidar_velocity_coherent_tracks (start_unix_nanos, end_unix_nanos);

CREATE INDEX IF NOT EXISTS idx_vc_tracks_class ON lidar_velocity_coherent_tracks (object_class);

   CREATE TABLE IF NOT EXISTS lidar_velocity_coherent_track_obs (
          track_id TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , world_frame TEXT NOT NULL
        , x REAL
        , y REAL
        , z REAL
        , velocity_x REAL
        , velocity_y REAL
        , velocity_z REAL
        , velocity_confidence REAL
        , speed_mps REAL
        , heading_rad REAL
        , bounding_box_length REAL
        , bounding_box_width REAL
        , bounding_box_height REAL
        , height_p95 REAL
        , intensity_mean REAL
        , points_count INTEGER
        , PRIMARY KEY (track_id, ts_unix_nanos)
        , FOREIGN KEY (track_id) REFERENCES lidar_velocity_coherent_tracks (track_id) ON DELETE CASCADE
          );

CREATE INDEX IF NOT EXISTS idx_vc_track_obs_track ON lidar_velocity_coherent_track_obs (track_id);

CREATE INDEX IF NOT EXISTS idx_vc_track_obs_time ON lidar_velocity_coherent_track_obs (ts_unix_nanos);

   CREATE TABLE IF NOT EXISTS lidar_track_merges (
          merge_id INTEGER PRIMARY KEY
        , merged_at INTEGER NOT NULL
        , earlier_track_id TEXT NOT NULL
        , later_track_id TEXT NOT NULL
        , result_track_id TEXT NOT NULL
        , position_score REAL
        , velocity_score REAL
        , trajectory_score REAL
        , overall_score REAL
        , gap_seconds REAL
        , interpolated_points INTEGER
          );

CREATE INDEX IF NOT EXISTS idx_track_merges_result ON lidar_track_merges (result_track_id);

CREATE INDEX IF NOT EXISTS idx_track_merges_earlier ON lidar_track_merges (earlier_track_id);

CREATE INDEX IF NOT EXISTS idx_track_merges_later ON lidar_track_merges (later_track_id);

   CREATE TABLE IF NOT EXISTS lidar_algorithm_config_log (
          config_id INTEGER PRIMARY KEY
        , ts_unix_nanos INTEGER NOT NULL
        , algorithm TEXT NOT NULL
        , config_json TEXT NOT NULL
        , changed_by TEXT
          );

CREATE INDEX IF NOT EXISTS idx_algorithm_config_time ON lidar_algorithm_config_log (ts_unix_nanos);
