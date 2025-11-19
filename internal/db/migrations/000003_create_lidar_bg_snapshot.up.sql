-- Migration: Add lidar_bg_snapshot table
-- Date: 2025-09-23
-- Description: Add LiDAR background snapshot table for storing spatial background grids
-- From commit adaca5c7
   CREATE TABLE IF NOT EXISTS lidar_bg_snapshot (
          snapshot_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , taken_unix_nanos INTEGER NOT NULL
        , rings INTEGER NOT NULL
        , azimuth_bins INTEGER NOT NULL
        , params_json TEXT NOT NULL
        , ring_elevations_json TEXT
        , grid_blob BLOB NOT NULL
        , changed_cells_count INTEGER
        , snapshot_reason TEXT
          );

CREATE INDEX IF NOT EXISTS idx_bg_snapshot_sensor_time ON lidar_bg_snapshot (sensor_id, taken_unix_nanos);
