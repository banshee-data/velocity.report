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

   CREATE TABLE IF NOT EXISTS schema_migrations (version uint64, dirty bool);

CREATE UNIQUE INDEX IF NOT EXISTS version_unique ON schema_migrations (version);

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
        , FOREIGN KEY (command_id) REFERENCES "radar_commands" (command_id)
          );

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

   CREATE TABLE IF NOT EXISTS site (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , name TEXT NOT NULL UNIQUE
        , location TEXT NOT NULL
        , description TEXT
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

CREATE TRIGGER IF NOT EXISTS update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

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

CREATE INDEX IF NOT EXISTS idx_site_reports_site_id ON site_reports (site_id);

CREATE INDEX IF NOT EXISTS idx_site_reports_created_at ON site_reports (created_at DESC);

   CREATE TABLE IF NOT EXISTS site_config_periods (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , site_id INTEGER NOT NULL
        , site_variable_config_id INTEGER
        , effective_start_unix REAL NOT NULL
        , effective_end_unix REAL
        , is_active INTEGER DEFAULT 0
        , notes TEXT
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , FOREIGN KEY (site_id) REFERENCES site (id)
        , FOREIGN KEY (site_variable_config_id) REFERENCES site_variable_config (id)
          );

CREATE INDEX IF NOT EXISTS idx_site_config_periods_site ON site_config_periods (site_id);

CREATE INDEX IF NOT EXISTS idx_site_config_periods_variable_config ON site_config_periods (site_variable_config_id);

CREATE INDEX IF NOT EXISTS idx_site_config_periods_time ON site_config_periods (effective_start_unix, effective_end_unix);

CREATE INDEX IF NOT EXISTS idx_site_config_periods_active ON site_config_periods (is_active);

CREATE TRIGGER IF NOT EXISTS update_site_config_periods_timestamp AFTER
   UPDATE ON site_config_periods BEGIN
   UPDATE site_config_periods
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

CREATE TRIGGER IF NOT EXISTS enforce_single_active_period BEFORE INSERT ON site_config_periods WHEN NEW.is_active = 1 BEGIN
   UPDATE site_config_periods
      SET is_active = 0
    WHERE is_active = 1;

END;

CREATE TRIGGER IF NOT EXISTS enforce_single_active_period_update BEFORE
   UPDATE ON site_config_periods WHEN NEW.is_active = 1
      AND OLD.is_active = 0 BEGIN
             UPDATE site_config_periods
                SET is_active = 0
              WHERE is_active = 1
                AND id != NEW.id;

END;

   CREATE TABLE IF NOT EXISTS site_variable_config (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , cosine_error_angle REAL NOT NULL
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          );

CREATE TRIGGER IF NOT EXISTS update_site_variable_config_timestamp AFTER
   UPDATE ON site_variable_config BEGIN
   UPDATE site_variable_config
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

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
