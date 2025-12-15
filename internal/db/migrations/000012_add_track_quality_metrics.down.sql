-- Rollback Phase 1: Track Quality Metrics (transactional)
-- Removes quality metric columns from `lidar_tracks`, `lidar_clusters`
-- and `lidar_analysis_runs` while preserving existing data.
PRAGMA foreign_keys = OFF;

BEGIN TRANSACTION;

     DROP INDEX IF EXISTS idx_lidar_tracks_quality;

   CREATE TABLE IF NOT EXISTS lidar_tracks_new (
          track_id TEXT PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , world_frame TEXT NOT NULL
        , track_state TEXT NOT NULL
        , start_unix_nanos INTEGER NOT NULL
        , end_unix_nanos INTEGER
        , observation_count INTEGER
        , avg_speed_mps REAL
        , peak_speed_mps REAL
        , p50_speed_mps REAL
        , p85_speed_mps REAL
        , p95_speed_mps REAL
        , bounding_box_length_avg REAL
        , bounding_box_width_avg REAL
        , bounding_box_height_avg REAL
        , height_p95_max REAL
        , intensity_mean_avg REAL
        , object_class TEXT
        , object_confidence REAL
        , classification_model TEXT
          );

   INSERT INTO lidar_tracks_new (
          track_id
        , sensor_id
        , world_frame
        , track_state
        , start_unix_nanos
        , end_unix_nanos
        , observation_count
        , avg_speed_mps
        , peak_speed_mps
        , p50_speed_mps
        , p85_speed_mps
        , p95_speed_mps
        , bounding_box_length_avg
        , bounding_box_width_avg
        , bounding_box_height_avg
        , height_p95_max
        , intensity_mean_avg
        , object_class
        , object_confidence
        , classification_model
          )
   SELECT track_id
        , sensor_id
        , world_frame
        , track_state
        , start_unix_nanos
        , end_unix_nanos
        , observation_count
        , avg_speed_mps
        , peak_speed_mps
        , p50_speed_mps
        , p85_speed_mps
        , p95_speed_mps
        , bounding_box_length_avg
        , bounding_box_width_avg
        , bounding_box_height_avg
        , height_p95_max
        , intensity_mean_avg
        , object_class
        , object_confidence
        , classification_model
     FROM lidar_tracks;

     DROP TABLE IF EXISTS lidar_tracks;

    ALTER TABLE lidar_tracks_new
RENAME TO lidar_tracks;

CREATE INDEX IF NOT EXISTS idx_lidar_tracks_sensor ON lidar_tracks (sensor_id);

CREATE INDEX IF NOT EXISTS idx_lidar_tracks_state ON lidar_tracks (track_state);

CREATE INDEX IF NOT EXISTS idx_lidar_tracks_time ON lidar_tracks (start_unix_nanos, end_unix_nanos);

CREATE INDEX IF NOT EXISTS idx_lidar_tracks_class ON lidar_tracks (object_class);

   CREATE TABLE IF NOT EXISTS lidar_clusters_new (
          lidar_cluster_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , world_frame TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , centroid_x REAL
        , centroid_y REAL
        , centroid_z REAL
        , bounding_box_length REAL
        , bounding_box_width REAL
        , bounding_box_height REAL
        , points_count INTEGER
        , height_p95 REAL
        , intensity_mean REAL
          );

   INSERT INTO lidar_clusters_new (
          lidar_cluster_id
        , sensor_id
        , world_frame
        , ts_unix_nanos
        , centroid_x
        , centroid_y
        , centroid_z
        , bounding_box_length
        , bounding_box_width
        , bounding_box_height
        , points_count
        , height_p95
        , intensity_mean
          )
   SELECT lidar_cluster_id
        , sensor_id
        , world_frame
        , ts_unix_nanos
        , centroid_x
        , centroid_y
        , centroid_z
        , bounding_box_length
        , bounding_box_width
        , bounding_box_height
        , points_count
        , height_p95
        , intensity_mean
     FROM lidar_clusters;

     DROP TABLE IF EXISTS lidar_clusters;

    ALTER TABLE lidar_clusters_new
RENAME TO lidar_clusters;

CREATE INDEX IF NOT EXISTS idx_lidar_clusters_sensor_time ON lidar_clusters (sensor_id, ts_unix_nanos);

   CREATE TABLE IF NOT EXISTS lidar_analysis_runs_new (
          run_id TEXT PRIMARY KEY
        , created_at INTEGER NOT NULL
        , source_type TEXT NOT NULL
        , source_path TEXT
        , sensor_id TEXT NOT NULL
        , params_json TEXT NOT NULL
        , duration_secs REAL
        , total_frames INTEGER
        , total_clusters INTEGER
        , total_tracks INTEGER
        , confirmed_tracks INTEGER
        , processing_time_ms INTEGER
        , status TEXT DEFAULT 'running'
        , error_message TEXT
        , parent_run_id TEXT
        , notes TEXT
          );

   INSERT INTO lidar_analysis_runs_new (
          run_id
        , created_at
        , source_type
        , source_path
        , sensor_id
        , params_json
        , duration_secs
        , total_frames
        , total_clusters
        , total_tracks
        , confirmed_tracks
        , processing_time_ms
        , status
        , error_message
        , parent_run_id
        , notes
          )
   SELECT run_id
        , created_at
        , source_type
        , source_path
        , sensor_id
        , params_json
        , duration_secs
        , total_frames
        , total_clusters
        , total_tracks
        , confirmed_tracks
        , processing_time_ms
        , status
        , error_message
        , parent_run_id
        , notes
     FROM lidar_analysis_runs;

     DROP TABLE IF EXISTS lidar_analysis_runs;

    ALTER TABLE lidar_analysis_runs_new
RENAME TO lidar_analysis_runs;

CREATE INDEX IF NOT EXISTS idx_lidar_runs_created ON lidar_analysis_runs (created_at);

CREATE INDEX IF NOT EXISTS idx_lidar_runs_source ON lidar_analysis_runs (source_path);

CREATE INDEX IF NOT EXISTS idx_lidar_runs_parent ON lidar_analysis_runs (parent_run_id);

CREATE INDEX IF NOT EXISTS idx_lidar_runs_status ON lidar_analysis_runs (status);

PRAGMA foreign_keys = ON;

COMMIT;
