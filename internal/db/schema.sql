   CREATE TABLE schema_migrations (version uint64, dirty bool);

CREATE UNIQUE INDEX version_unique ON schema_migrations (version);

   CREATE TABLE IF NOT EXISTS "radar_data" (
          write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , raw_event JSON NOT NULL
        , uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED
        , magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED
        , speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
          );

   CREATE TABLE radar_objects (
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

   CREATE TABLE IF NOT EXISTS "radar_commands" (
          command_id BIGINT PRIMARY KEY
        , command TEXT
        , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
          );

   CREATE TABLE IF NOT EXISTS "radar_command_log" (
          log_id BIGINT PRIMARY KEY
        , command_id BIGINT
        , log_data TEXT
        , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , FOREIGN KEY (command_id) REFERENCES "radar_commands" (command_id)
          );

   CREATE TABLE lidar_bg_snapshot (
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

CREATE INDEX idx_bg_snapshot_sensor_time ON lidar_bg_snapshot (sensor_id, taken_unix_nanos);

   CREATE TABLE radar_data_transits (
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

CREATE INDEX idx_transits_time ON radar_data_transits (transit_start_unix, transit_end_unix);

   CREATE TABLE radar_transit_links (
          link_id INTEGER PRIMARY KEY AUTOINCREMENT
        , transit_id INTEGER NOT NULL REFERENCES radar_data_transits (transit_id) ON DELETE CASCADE
        , data_rowid INTEGER NOT NULL REFERENCES radar_data (rowid) ON DELETE CASCADE
        , link_score DOUBLE
        , created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , UNIQUE (transit_id, data_rowid)
          );

CREATE INDEX idx_transit_links_transit ON radar_transit_links (transit_id);

CREATE INDEX idx_transit_links_data ON radar_transit_links (data_rowid);

   CREATE TABLE site_reports (
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

CREATE INDEX idx_site_reports_site_id ON site_reports (site_id);

CREATE INDEX idx_site_reports_created_at ON site_reports (created_at DESC);

   CREATE TABLE lidar_clusters (
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

CREATE INDEX idx_lidar_clusters_sensor_time ON lidar_clusters (sensor_id, ts_unix_nanos);

   CREATE TABLE lidar_tracks (
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

CREATE INDEX idx_lidar_tracks_sensor ON lidar_tracks (sensor_id);

CREATE INDEX idx_lidar_tracks_state ON lidar_tracks (track_state);

CREATE INDEX idx_lidar_tracks_time ON lidar_tracks (start_unix_nanos, end_unix_nanos);

CREATE INDEX idx_lidar_tracks_class ON lidar_tracks (object_class);

   CREATE TABLE lidar_track_obs (
          track_id TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , world_frame TEXT NOT NULL
        , x REAL
        , y REAL
        , z REAL
        , velocity_x REAL
        , velocity_y REAL
        , speed_mps REAL
        , heading_rad REAL
        , bounding_box_length REAL
        , bounding_box_width REAL
        , bounding_box_height REAL
        , height_p95 REAL
        , intensity_mean REAL
        , PRIMARY KEY (track_id, ts_unix_nanos)
        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id) ON DELETE CASCADE
          );

CREATE INDEX idx_lidar_track_obs_track ON lidar_track_obs (track_id);

CREATE INDEX idx_lidar_track_obs_time ON lidar_track_obs (ts_unix_nanos);

   CREATE TABLE lidar_analysis_runs (
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

CREATE INDEX idx_lidar_runs_created ON lidar_analysis_runs (created_at);

CREATE INDEX idx_lidar_runs_source ON lidar_analysis_runs (source_path);

CREATE INDEX idx_lidar_runs_parent ON lidar_analysis_runs (parent_run_id);

CREATE INDEX idx_lidar_runs_status ON lidar_analysis_runs (status);

   CREATE TABLE lidar_run_tracks (
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

CREATE INDEX idx_lidar_run_tracks_run ON lidar_run_tracks (run_id);

CREATE INDEX idx_lidar_run_tracks_class ON lidar_run_tracks (object_class);

CREATE INDEX idx_lidar_run_tracks_label ON lidar_run_tracks (user_label);

CREATE INDEX idx_lidar_run_tracks_state ON lidar_run_tracks (track_state);

   CREATE TABLE angle_presets (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , angle REAL NOT NULL UNIQUE
        , color_hex TEXT NOT NULL
        , is_system INTEGER NOT NULL DEFAULT 0
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          );

CREATE INDEX idx_angle_presets_angle ON angle_presets (angle);

CREATE INDEX idx_angle_presets_is_system ON angle_presets (is_system);

CREATE TRIGGER prevent_system_preset_deletion BEFORE DELETE ON angle_presets FOR EACH ROW WHEN OLD.is_system = 1 BEGIN
   SELECT RAISE(ABORT, 'Cannot delete system preset');

END;

CREATE TRIGGER update_angle_presets_timestamp AFTER
   UPDATE ON angle_presets BEGIN
   UPDATE angle_presets
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

   CREATE TABLE site_variable_config (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , cosine_error_angle REAL NOT NULL
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          );

CREATE TRIGGER update_site_variable_config_timestamp AFTER
   UPDATE ON site_variable_config BEGIN
   UPDATE site_variable_config
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

   CREATE TABLE site_config_periods (
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

CREATE INDEX idx_site_config_periods_site ON site_config_periods (site_id);

CREATE INDEX idx_site_config_periods_variable_config ON site_config_periods (site_variable_config_id);

CREATE INDEX idx_site_config_periods_time ON site_config_periods (effective_start_unix, effective_end_unix);

CREATE INDEX idx_site_config_periods_active ON site_config_periods (is_active);

CREATE TRIGGER update_site_config_periods_timestamp AFTER
   UPDATE ON site_config_periods BEGIN
   UPDATE site_config_periods
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

CREATE TRIGGER enforce_single_active_period BEFORE INSERT ON site_config_periods WHEN NEW.is_active = 1 BEGIN
   UPDATE site_config_periods
      SET is_active = 0
    WHERE is_active = 1
      AND site_id = NEW.site_id;

END;

CREATE TRIGGER enforce_single_active_period_update BEFORE
   UPDATE ON site_config_periods WHEN NEW.is_active = 1
      AND OLD.is_active = 0 BEGIN
             UPDATE site_config_periods
                SET is_active = 0
              WHERE is_active = 1
                AND site_id = NEW.site_id
                AND id != NEW.id;

END;

   CREATE TABLE IF NOT EXISTS "site" (
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

CREATE INDEX idx_site_name ON site (name);

CREATE TRIGGER update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;
