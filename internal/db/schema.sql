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

CREATE INDEX IF NOT EXISTS idx_bg_snapshot_sensor_time ON lidar_bg_snapshot (sensor_id, taken_unix_nanos);

-- Phase 3.3: LIDAR Tracks Schema
-- Clusters detected via DBSCAN (world frame)
CREATE TABLE IF NOT EXISTS lidar_clusters (
    lidar_cluster_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    
    -- World frame position (meters)
    centroid_x REAL,
    centroid_y REAL,
    centroid_z REAL,
    
    -- Bounding box (world frame, meters)
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL,
    
    -- Cluster features
    points_count INTEGER,
    height_p95 REAL,
    intensity_mean REAL
);

CREATE INDEX IF NOT EXISTS idx_lidar_clusters_sensor_time ON lidar_clusters(sensor_id, ts_unix_nanos);

-- Tracks (world frame)
CREATE TABLE IF NOT EXISTS lidar_tracks (
    track_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    track_state TEXT NOT NULL, -- 'tentative', 'confirmed', 'deleted'
    
    -- Lifecycle
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,
    
    -- Kinematics (world frame)
    avg_speed_mps REAL,
    peak_speed_mps REAL,
    p50_speed_mps REAL,  -- Median speed
    p85_speed_mps REAL,  -- 85th percentile
    p95_speed_mps REAL,  -- 95th percentile
    
    -- Shape features (world frame averages)
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,
    intensity_mean_avg REAL,
    
    -- Classification (Phase 3.4)
    object_class TEXT,           -- 'pedestrian', 'car', 'bird', 'other'
    object_confidence REAL,
    classification_model TEXT    -- Model version used for classification
);

CREATE INDEX IF NOT EXISTS idx_lidar_tracks_sensor ON lidar_tracks(sensor_id);
CREATE INDEX IF NOT EXISTS idx_lidar_tracks_state ON lidar_tracks(track_state);
CREATE INDEX IF NOT EXISTS idx_lidar_tracks_time ON lidar_tracks(start_unix_nanos, end_unix_nanos);
CREATE INDEX IF NOT EXISTS idx_lidar_tracks_class ON lidar_tracks(object_class);

-- Track observations (world frame)
CREATE TABLE IF NOT EXISTS lidar_track_obs (
    track_id TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    world_frame TEXT NOT NULL,
    
    -- Position (world frame, meters)
    x REAL,
    y REAL,
    z REAL,
    
    -- Velocity (world frame, m/s)
    velocity_x REAL,
    velocity_y REAL,
    speed_mps REAL,
    heading_rad REAL,
    
    -- Shape (world frame)
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL,
    height_p95 REAL,
    intensity_mean REAL,
    
    PRIMARY KEY (track_id, ts_unix_nanos),
    FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_lidar_track_obs_track ON lidar_track_obs(track_id);
CREATE INDEX IF NOT EXISTS idx_lidar_track_obs_time ON lidar_track_obs(ts_unix_nanos);

-- Phase 3.7: Analysis Run Infrastructure
-- Enables versioned parameter configurations and run comparison

-- Analysis runs with full parameter configuration
CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
    run_id TEXT PRIMARY KEY,              -- UUID or timestamp-based ID
    created_at INTEGER NOT NULL,          -- Unix nanoseconds
    source_type TEXT NOT NULL,            -- 'pcap' or 'live'
    source_path TEXT,                     -- PCAP file path (if applicable)
    sensor_id TEXT NOT NULL,
    
    -- Full parameter configuration as JSON (all LIDAR params in single blob)
    params_json TEXT NOT NULL,
    
    -- Run statistics
    duration_secs REAL,
    total_frames INTEGER,
    total_clusters INTEGER,
    total_tracks INTEGER,
    confirmed_tracks INTEGER,
    
    -- Processing metadata
    processing_time_ms INTEGER,
    status TEXT DEFAULT 'running',        -- 'running', 'completed', 'failed'
    error_message TEXT,
    
    -- Comparison metadata
    parent_run_id TEXT,                   -- For parameter tuning comparisons
    notes TEXT                            -- User notes about this run
);

CREATE INDEX IF NOT EXISTS idx_lidar_runs_created ON lidar_analysis_runs(created_at);
CREATE INDEX IF NOT EXISTS idx_lidar_runs_source ON lidar_analysis_runs(source_path);
CREATE INDEX IF NOT EXISTS idx_lidar_runs_parent ON lidar_analysis_runs(parent_run_id);
CREATE INDEX IF NOT EXISTS idx_lidar_runs_status ON lidar_analysis_runs(status);

-- Track results per run (extends lidar_tracks with run_id and user labels)
CREATE TABLE IF NOT EXISTS lidar_run_tracks (
    run_id TEXT NOT NULL,
    track_id TEXT NOT NULL,
    
    -- All track fields from lidar_tracks
    sensor_id TEXT NOT NULL,
    track_state TEXT NOT NULL,
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,
    avg_speed_mps REAL,
    peak_speed_mps REAL,
    p50_speed_mps REAL,
    p85_speed_mps REAL,
    p95_speed_mps REAL,
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,
    intensity_mean_avg REAL,
    
    -- Classification (rule-based or ML)
    object_class TEXT,
    object_confidence REAL,
    classification_model TEXT,
    
    -- User labels (for ML training)
    user_label TEXT,                      -- Human-assigned label
    label_confidence REAL,                -- Annotator confidence
    labeler_id TEXT,                      -- Who labeled this
    labeled_at INTEGER,                   -- When labeled (unix nanos)
    
    -- Track quality flags
    is_split_candidate INTEGER DEFAULT 0,   -- Suspected split
    is_merge_candidate INTEGER DEFAULT 0,   -- Suspected merge
    linked_track_ids TEXT,                  -- JSON array of related track IDs
    
    PRIMARY KEY (run_id, track_id),
    FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_run ON lidar_run_tracks(run_id);
CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_class ON lidar_run_tracks(object_class);
CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_label ON lidar_run_tracks(user_label);
CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_state ON lidar_run_tracks(track_state);
