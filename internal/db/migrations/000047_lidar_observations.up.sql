-- Migration: Immutable L4 detection observations
-- Date: 2026-09-11
-- Description: Preserve track-independent sensor evidence as a write-once
-- replay unit. lidar_track_observations is deliberately untouched: its rows
-- are historical tracker output despite their misleading name.
   CREATE TABLE IF NOT EXISTS lidar_observations (
          observation_id TEXT PRIMARY KEY
        , schema_version INTEGER NOT NULL
        , source_id TEXT NOT NULL
        , calibration_id TEXT NOT NULL
        , sensor_id TEXT NOT NULL
        , frame_id TEXT NOT NULL
        , frame_unix_nanos INTEGER NOT NULL
        , cluster_unix_nanos INTEGER NOT NULL
        , cluster_id INTEGER NOT NULL
        , record_json BLOB NOT NULL
        , inserted_at_ns INTEGER NOT NULL
        , CHECK (schema_version = 1)
        , CHECK (LENGTH(record_json) > 0)
          );

CREATE INDEX IF NOT EXISTS idx_lidar_observations_source_time ON lidar_observations (source_id, frame_unix_nanos, observation_id);

CREATE INDEX IF NOT EXISTS idx_lidar_observations_calibration_time ON lidar_observations (calibration_id, frame_unix_nanos, observation_id);
