-- Phase 3.7: Analysis Run Infrastructure
-- Enables versioned parameter configurations and run comparison
-- Analysis runs with full parameter configuration
   CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
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

CREATE INDEX IF NOT EXISTS idx_lidar_runs_created ON lidar_analysis_runs (created_at);

CREATE INDEX IF NOT EXISTS idx_lidar_runs_source ON lidar_analysis_runs (source_path);

CREATE INDEX IF NOT EXISTS idx_lidar_runs_parent ON lidar_analysis_runs (parent_run_id);

CREATE INDEX IF NOT EXISTS idx_lidar_runs_status ON lidar_analysis_runs (status);

   CREATE TABLE IF NOT EXISTS lidar_run_tracks (
          run_id TEXT NOT NULL
        , track_id TEXT NOT NULL
        , sensor_id TEXT NOT NULL
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
        , user_label TEXT
        , label_confidence REAL
        , labeler_id TEXT
        , labeled_at INTEGER
        , is_split_candidate INTEGER DEFAULT 0
        , is_merge_candidate INTEGER DEFAULT 0
        , linked_track_ids TEXT
        , PRIMARY KEY (run_id, track_id)
        , FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs (run_id) ON DELETE CASCADE
          );

CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_run ON lidar_run_tracks (run_id);

CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_class ON lidar_run_tracks (object_class);

CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_label ON lidar_run_tracks (user_label);

CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_state ON lidar_run_tracks (track_state);
