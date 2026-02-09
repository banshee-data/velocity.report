-- 000021: Create lidar_missed_regions table for marking areas where objects
-- should have been tracked but were not detected by the tracker.
   CREATE TABLE IF NOT EXISTS lidar_missed_regions (
          region_id TEXT PRIMARY KEY
        , run_id TEXT NOT NULL
        , center_x REAL NOT NULL
        , center_y REAL NOT NULL
        , radius_m REAL NOT NULL DEFAULT 3.0
        , time_start_ns INTEGER NOT NULL
        , time_end_ns INTEGER NOT NULL
        , expected_label TEXT NOT NULL DEFAULT 'good_vehicle'
        , labeler_id TEXT
        , labeled_at INTEGER
        , notes TEXT
        , FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs (run_id) ON DELETE CASCADE
          );

CREATE INDEX IF NOT EXISTS idx_missed_regions_run_id ON lidar_missed_regions (run_id);
