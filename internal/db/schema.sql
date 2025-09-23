-- Enable Write-Ahead Logging for better concurrency
-- Allows readers and writers to operate simultaneously without blocking
PRAGMA journal_mode = WAL;

-- Use normal synchronous mode for balance of safety and performance
-- Reduces fsync calls while maintaining reasonable crash recovery
PRAGMA synchronous = NORMAL;

-- Store temporary tables and indices in memory for faster processing
-- Improves performance for complex queries and joins
PRAGMA temp_store = MEMORY;

-- Set busy timeout for handling concurrent access
-- Prevents immediate failures when database is locked by other processes
PRAGMA busy_timeout = 5000;

   CREATE TABLE IF NOT EXISTS radar_data (
          write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , raw_event JSON NOT NULL
        , uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED
        , magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED
        , speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
          );

   CREATE TABLE IF NOT EXISTS radar_objects (
          write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , raw_event JSON NOT NULL
        , classifier TEXT NOT NULL AS (JSON_EXTRACT(raw_event, '$.classifier')) STORED
        , start_time DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.start_time')) STORED
        , end_time DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.end_time')) STORED
        , delta_time_ms BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.delta_time_msec')) STORED
        , max_speed DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.max_speed_mps')) STORED
        , min_speed DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.min_speed_mps')) STORED
        , speed_change DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.speed_change')) STORED
        , max_magnitude BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.max_magnitude')) STORED
        , avg_magnitude BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.avg_magnitude')) STORED
        , total_frames BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.total_frames')) STORED
        , frames_per_mps DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.frames_per_mps')) STORED
        , length_m DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.length_m')) STORED
          );

   CREATE TABLE IF NOT EXISTS radar_commands (
          command_id BIGINT PRIMARY KEY
        , command TEXT
        , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
          );

   CREATE TABLE IF NOT EXISTS radar_command_log (
          log_id BIGINT PRIMARY KEY
        , command_id BIGINT
        , log_data TEXT
        , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , FOREIGN KEY (command_id) REFERENCES radar_commands (command_id)
          );

-- Append LiDAR schema (background snapshots, clusters, tracks) from internal/lidar/lidardb/schema.sql
-- This keeps a single unified schema for both radar and lidar features.
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

-- (Other lidar tables exist in internal/lidar/lidardb/schema.sql; they are omitted here to avoid duplicating large sections.
-- The critical table for snapshots is included above.)
CREATE INDEX IF NOT EXISTS idx_bg_snapshot_sensor_time ON lidar_bg_snapshot (sensor_id, taken_unix_nanos);
