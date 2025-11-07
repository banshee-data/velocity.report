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

-- Persisted sessionization table for radar_data transits.
-- Populated by a periodic worker (Go process) that scans recent radar_data
-- and inserts/updates transit records. Keeping this as a table avoids expensive
-- repeated CTEs and allows linking to radar_objects without modifying raw data.
   CREATE TABLE IF NOT EXISTS radar_data_transits (
          transit_id INTEGER PRIMARY KEY AUTOINCREMENT
        , transit_key TEXT NOT NULL UNIQUE
        , threshold_ms INTEGER NOT NULL
        , transit_start_unix DOUBLE NOT NULL
        , transit_end_unix DOUBLE NOT NULL
        , transit_max_speed DOUBLE NOT NULL
        , transit_min_speed DOUBLE
        , transit_max_magnitude BIGINT
        , transit_min_magnitude BIGINT
        , point_count INTEGER NOT NULL
        , model_version TEXT
        , created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , updated_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
          );

CREATE INDEX IF NOT EXISTS idx_transits_time ON radar_data_transits (transit_start_unix, transit_end_unix);

-- Join table linking radar_data_transits to radar_data (many-to-many).
-- Each link associates a transit (an aggregated session) with the underlying
-- radar_data row (an individual raw reading). This enables inspection of which
-- raw samples contributed to a transit.
   CREATE TABLE IF NOT EXISTS radar_transit_links (
          link_id INTEGER PRIMARY KEY AUTOINCREMENT
        , transit_id INTEGER NOT NULL REFERENCES radar_data_transits (transit_id) ON DELETE CASCADE
        , data_rowid INTEGER NOT NULL REFERENCES radar_data (rowid) ON DELETE CASCADE
        , link_score DOUBLE
        , created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , UNIQUE (transit_id, data_rowid)
          );

CREATE INDEX IF NOT EXISTS idx_transit_links_transit ON radar_transit_links (transit_id);

CREATE INDEX IF NOT EXISTS idx_transit_links_data ON radar_transit_links (data_rowid);

-- Site configuration table for storing location information, radar configuration, and report settings
   CREATE TABLE IF NOT EXISTS site (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , name TEXT NOT NULL UNIQUE
        , location TEXT NOT NULL
        , description TEXT
        , cosine_error_angle REAL NOT NULL
        , speed_limit INTEGER DEFAULT 25
        , surveyor TEXT NOT NULL
        , contact TEXT NOT NULL
        , address TEXT
        , latitude REAL
        , longitude REAL
        , map_angle REAL
        , include_map INTEGER DEFAULT 0
        , site_description TEXT
        , speed_limit_note TEXT
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          );

CREATE INDEX IF NOT EXISTS idx_site_name ON site (name);

-- Create trigger to update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

-- Insert a default site for existing installations
   INSERT OR IGNORE INTO site (
          name
        , location
        , description
        , cosine_error_angle
        , speed_limit
        , surveyor
        , contact
        , site_description
          )
   VALUES (
          'Default Location'
        , 'Survey Location'
        , 'Default site configuration'
        , 0.5
        , 25
        , 'Sir Veyor'
        , 'example@velocity.report'
        , 'Default site for radar velocity surveys'
          );

-- Speed limit schedule table for storing time-based speed limits per site
-- Supports different speed limits by day of week and time of day in 5-minute increments
   CREATE TABLE IF NOT EXISTS speed_limit_schedule (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , site_id INTEGER NOT NULL
        , day_of_week INTEGER NOT NULL -- 0=Sunday, 1=Monday, ..., 6=Saturday
        , start_time TEXT NOT NULL -- HH:MM format (e.g., "06:00")
        , end_time TEXT NOT NULL -- HH:MM format (e.g., "07:05")
        , speed_limit INTEGER NOT NULL -- Speed limit for this time block
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE
          );

CREATE INDEX IF NOT EXISTS idx_speed_limit_schedule_site ON speed_limit_schedule (site_id);

-- Create trigger to update updated_at timestamp for speed_limit_schedule
CREATE TRIGGER IF NOT EXISTS update_speed_limit_schedule_timestamp AFTER
   UPDATE ON speed_limit_schedule BEGIN
   UPDATE speed_limit_schedule
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

-- Create site_reports table to track generated PDF reports
   CREATE TABLE IF NOT EXISTS site_reports (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , site_id INTEGER NOT NULL DEFAULT 0
        , start_date TEXT NOT NULL
        , end_date TEXT NOT NULL
        , filepath TEXT NOT NULL
        , filename TEXT NOT NULL
        , zip_filepath TEXT
        , zip_filename TEXT
        , run_id TEXT NOT NULL
        , timezone TEXT NOT NULL
        , units TEXT NOT NULL
        , source TEXT NOT NULL
        , created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        , FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE
          );

-- Index for fast lookups by site
CREATE INDEX IF NOT EXISTS idx_site_reports_site_id ON site_reports (site_id);

-- Index for ordering by creation time
CREATE INDEX IF NOT EXISTS idx_site_reports_created_at ON site_reports (created_at DESC);

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
