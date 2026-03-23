-- AUTO-GENERATED — do not edit by hand.
-- Regenerate with: ./scripts/sync-schema.sh
--
   CREATE TABLE lidar_bg_regions (
          region_set_id INTEGER PRIMARY KEY AUTOINCREMENT
        , snapshot_id INTEGER REFERENCES lidar_bg_snapshot (snapshot_id)
        , sensor_id TEXT NOT NULL
        , created_unix_nanos INTEGER NOT NULL
        , region_count INTEGER NOT NULL
        , regions_json TEXT NOT NULL
        , variance_data_json TEXT
        , settling_frames INTEGER NOT NULL
        , grid_hash TEXT NOT NULL
        , source_path TEXT
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

   CREATE TABLE lidar_clusters (
          lidar_cluster_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , frame_id TEXT NOT NULL
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
        , noise_points_count INTEGER DEFAULT 0
        , cluster_density REAL
        , aspect_ratio REAL
          );

   CREATE TABLE lidar_param_sets (
          param_set_id TEXT PRIMARY KEY
        , params_hash TEXT NOT NULL UNIQUE
        , schema_version TEXT NOT NULL
        , param_set_type TEXT NOT NULL
        , params_json TEXT NOT NULL
        , created_at INTEGER NOT NULL
        , CHECK (param_set_type IN ('requested', 'effective', 'legacy'))
          );

   CREATE TABLE lidar_run_configs (
          run_config_id TEXT PRIMARY KEY
        , config_hash TEXT NOT NULL UNIQUE
        , param_set_id TEXT NOT NULL REFERENCES lidar_param_sets (param_set_id)
        , build_version TEXT NOT NULL
        , build_git_sha TEXT NOT NULL
        , created_at INTEGER NOT NULL
        , UNIQUE (param_set_id, build_version, build_git_sha)
          );

   CREATE TABLE IF NOT EXISTS "lidar_run_records" (
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
        , status TEXT NOT NULL DEFAULT 'running'
        , error_message TEXT
        , parent_run_id TEXT
        , notes TEXT
        , statistics_json TEXT
        , vrlog_path TEXT
        , run_config_id TEXT REFERENCES lidar_run_configs (run_config_id) ON DELETE SET NULL
        , requested_param_set_id TEXT REFERENCES lidar_param_sets (param_set_id) ON DELETE SET NULL
        , replay_case_id TEXT REFERENCES lidar_replay_cases (replay_case_id) ON DELETE SET NULL
        , completed_at INTEGER
        , frame_start_ns INTEGER
        , frame_end_ns INTEGER
        , CHECK (source_type IN ('live', 'pcap'))
        , CHECK (status IN ('running', 'completed', 'failed'))
        , FOREIGN KEY (parent_run_id) REFERENCES "lidar_run_records" (run_id) ON DELETE SET NULL
          );

   CREATE TABLE IF NOT EXISTS "lidar_replay_cases" (
          replay_case_id TEXT PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , pcap_file TEXT NOT NULL
        , pcap_start_secs REAL
        , pcap_duration_secs REAL
        , description TEXT
        , reference_run_id TEXT
        , optimal_params_json TEXT
        , created_at_ns INTEGER NOT NULL
        , updated_at_ns INTEGER
        , recommended_param_set_id TEXT REFERENCES lidar_param_sets (param_set_id) ON DELETE SET NULL
        , CHECK (
          pcap_start_secs IS NULL
       OR pcap_start_secs >= 0
          )
        , CHECK (
          pcap_duration_secs IS NULL
       OR pcap_duration_secs >= 0
          )
        , FOREIGN KEY (reference_run_id) REFERENCES lidar_run_records (run_id) ON DELETE SET NULL
          );

   CREATE TABLE IF NOT EXISTS "lidar_replay_evaluations" (
          evaluation_id TEXT PRIMARY KEY
        , replay_case_id TEXT NOT NULL
        , reference_run_id TEXT NOT NULL
        , candidate_run_id TEXT NOT NULL
        , detection_rate REAL
        , fragmentation REAL
        , false_positive_rate REAL
        , velocity_coverage REAL
        , quality_premium REAL
        , truncation_rate REAL
        , velocity_noise_rate REAL
        , stopped_recovery_rate REAL
        , composite_score REAL
        , matched_count INTEGER
        , reference_count INTEGER
        , candidate_count INTEGER
        , params_json TEXT
        , created_at INTEGER NOT NULL
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
        , FOREIGN KEY (reference_run_id) REFERENCES lidar_run_records (run_id) ON DELETE CASCADE
        , FOREIGN KEY (candidate_run_id) REFERENCES lidar_run_records (run_id) ON DELETE CASCADE
          );

   CREATE TABLE IF NOT EXISTS "lidar_run_missed_regions" (
          region_id TEXT PRIMARY KEY
        , run_id TEXT NOT NULL
        , center_x REAL NOT NULL
        , center_y REAL NOT NULL
        , radius_m REAL NOT NULL DEFAULT 3.0
        , time_start_ns INTEGER NOT NULL
        , time_end_ns INTEGER NOT NULL
        , expected_label TEXT NOT NULL DEFAULT 'car'
        , labeler_id TEXT
        , labeled_at INTEGER
        , notes TEXT
        , CHECK (radius_m > 0)
        , CHECK (time_end_ns >= time_start_ns)
        , CHECK (
          expected_label IN ('car', 'bus', 'pedestrian', 'cyclist', 'bird', 'noise', 'dynamic')
          )
        , FOREIGN KEY (run_id) REFERENCES lidar_run_records (run_id) ON DELETE CASCADE
          );

   CREATE TABLE IF NOT EXISTS "lidar_run_tracks" (
          run_id TEXT NOT NULL
        , track_id TEXT NOT NULL
        , sensor_id TEXT NOT NULL
        , track_state TEXT NOT NULL
        , start_unix_nanos INTEGER NOT NULL
        , end_unix_nanos INTEGER
        , observation_count INTEGER
        , avg_speed_mps REAL
        , max_speed_mps REAL
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
        , quality_label TEXT
        , label_source TEXT
        , PRIMARY KEY (run_id, track_id)
        , CHECK (track_state IN ('tentative', 'confirmed', 'deleted'))
        , CHECK (
          end_unix_nanos IS NULL
       OR end_unix_nanos >= start_unix_nanos
          )
        , CHECK (
          object_confidence IS NULL
       OR (
          object_confidence >= 0
      AND object_confidence <= 1
          )
          )
        , CHECK (
          label_confidence IS NULL
       OR (
          label_confidence >= 0
      AND label_confidence <= 1
          )
          )
        , CHECK (is_split_candidate IN (0, 1))
        , CHECK (is_merge_candidate IN (0, 1))
        , FOREIGN KEY (run_id) REFERENCES lidar_run_records (run_id) ON DELETE CASCADE
          );

   CREATE TABLE lidar_replay_annotations (
          annotation_id TEXT PRIMARY KEY
        , replay_case_id TEXT NOT NULL
        , run_id TEXT
        , track_id TEXT
        , class_label TEXT NOT NULL
        , start_timestamp_ns INTEGER NOT NULL
        , end_timestamp_ns INTEGER
        , confidence REAL
        , created_by TEXT
        , created_at_ns INTEGER NOT NULL
        , updated_at_ns INTEGER
        , notes TEXT
        , source_file TEXT
        , CHECK (
          (
          run_id IS NULL
      AND track_id IS NULL
          )
       OR (
          run_id IS NOT NULL
      AND track_id IS NOT NULL
          )
          )
        , CHECK (
          end_timestamp_ns IS NULL
       OR end_timestamp_ns >= start_timestamp_ns
          )
        , CHECK (
          confidence IS NULL
       OR (
          confidence >= 0
      AND confidence <= 1
          )
          )
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
        , FOREIGN KEY (run_id, track_id) REFERENCES lidar_run_tracks (run_id, track_id) ON DELETE SET NULL
          );

   CREATE TABLE IF NOT EXISTS "lidar_tracks" (
          track_id TEXT PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , frame_id TEXT NOT NULL
        , track_state TEXT NOT NULL
        , start_unix_nanos INTEGER NOT NULL
        , end_unix_nanos INTEGER
        , observation_count INTEGER
        , avg_speed_mps REAL
        , max_speed_mps REAL
        , bounding_box_length_avg REAL
        , bounding_box_width_avg REAL
        , bounding_box_height_avg REAL
        , height_p95_max REAL
        , intensity_mean_avg REAL
        , object_class TEXT
        , object_confidence REAL
        , classification_model TEXT
        , track_length_meters REAL
        , track_duration_secs REAL
        , occlusion_count INTEGER DEFAULT 0
        , max_occlusion_frames INTEGER DEFAULT 0
        , spatial_coverage REAL
        , noise_point_ratio REAL
        , CHECK (track_state IN ('tentative', 'confirmed', 'deleted'))
        , CHECK (
          end_unix_nanos IS NULL
       OR end_unix_nanos >= start_unix_nanos
          )
        , CHECK (
          object_confidence IS NULL
       OR (
          object_confidence >= 0
      AND object_confidence <= 1
          )
          )
          );

   CREATE TABLE IF NOT EXISTS "lidar_track_observations" (
          track_id TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , frame_id TEXT NOT NULL
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

   CREATE TABLE IF NOT EXISTS "lidar_tuning_sweeps" (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , sweep_id TEXT NOT NULL UNIQUE
        , sensor_id TEXT NOT NULL
        , mode TEXT NOT NULL DEFAULT 'sweep'
        , status TEXT NOT NULL DEFAULT 'running'
        , request TEXT NOT NULL
        , results TEXT
        , charts TEXT
        , recommendation TEXT
        , round_results TEXT
        , error TEXT
        , started_at DATETIME NOT NULL
        , completed_at DATETIME
        , created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        , objective_name TEXT
        , objective_version TEXT
        , transform_pipeline_name TEXT
        , transform_pipeline_version TEXT
        , score_components_json TEXT
        , recommendation_explanation_json TEXT
        , label_provenance_summary_json TEXT
        , checkpoint_round INTEGER
        , checkpoint_bounds TEXT
        , checkpoint_results TEXT
        , checkpoint_request TEXT
        , CHECK (mode IN ('manual', 'sweep', 'auto', 'auto-tune', 'hint'))
        , CHECK (status IN ('running', 'completed', 'failed', 'suspended'))
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

   CREATE TABLE IF NOT EXISTS "radar_data" (
          data_id INTEGER PRIMARY KEY AUTOINCREMENT
        , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , raw_event JSON NOT NULL
        , uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED
        , magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED
        , speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
          );

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

   CREATE TABLE IF NOT EXISTS "radar_transit_links" (
          link_id INTEGER PRIMARY KEY AUTOINCREMENT
        , transit_id INTEGER NOT NULL REFERENCES radar_data_transits (transit_id) ON DELETE CASCADE
        , data_rowid INTEGER NOT NULL REFERENCES radar_data (data_id) ON DELETE CASCADE
        , link_score DOUBLE
        , created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , UNIQUE (transit_id, data_rowid)
          );

   CREATE TABLE schema_migrations (version uint64, dirty bool);

   CREATE TABLE IF NOT EXISTS "site" (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , name TEXT NOT NULL UNIQUE
        , location TEXT NOT NULL
        , description TEXT
        , surveyor TEXT NOT NULL
        , contact TEXT NOT NULL
        , address TEXT
        , latitude REAL
        , longitude REAL
        , map_angle REAL
        , include_map INTEGER DEFAULT 0
        , site_description TEXT
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , bbox_ne_lat REAL
        , bbox_ne_lng REAL
        , bbox_sw_lat REAL
        , bbox_sw_lng REAL
        , map_svg_data BLOB
        , CHECK (include_map IN (0, 1))
          );

   CREATE TABLE IF NOT EXISTS "site_config_periods" (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , site_id INTEGER NOT NULL
        , effective_start_unix DOUBLE NOT NULL
        , effective_end_unix DOUBLE
        , is_active INTEGER NOT NULL DEFAULT 0
        , notes TEXT
        , cosine_error_angle DOUBLE NOT NULL DEFAULT 0
        , created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , updated_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , CHECK (is_active IN (0, 1))
        , CHECK (
          effective_end_unix IS NULL
       OR effective_end_unix >= effective_start_unix
          )
        , FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE
          );

   CREATE TABLE IF NOT EXISTS "site_reports" (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , site_id INTEGER
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
        , FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE SET NULL
          );

CREATE UNIQUE INDEX version_unique ON schema_migrations (version);

CREATE INDEX idx_bg_snapshot_sensor_time ON lidar_bg_snapshot (sensor_id, taken_unix_nanos);

CREATE INDEX idx_transits_time ON radar_data_transits (transit_start_unix, transit_end_unix);

CREATE INDEX idx_lidar_clusters_sensor_time ON lidar_clusters (sensor_id, ts_unix_nanos);

CREATE INDEX idx_bg_regions_sensor ON lidar_bg_regions (sensor_id);

CREATE INDEX idx_bg_regions_source_path ON lidar_bg_regions (source_path);

CREATE INDEX idx_bg_regions_grid_hash ON lidar_bg_regions (grid_hash);

CREATE INDEX idx_lidar_track_observations_track ON lidar_track_observations (track_id);

CREATE INDEX idx_lidar_track_observations_time ON lidar_track_observations (ts_unix_nanos);

CREATE INDEX idx_lidar_replay_annotations_replay_case ON lidar_replay_annotations (replay_case_id);

CREATE INDEX idx_lidar_replay_annotations_run_track ON lidar_replay_annotations (run_id, track_id);

CREATE INDEX idx_lidar_replay_annotations_track ON lidar_replay_annotations (track_id);

CREATE INDEX idx_lidar_replay_annotations_time ON lidar_replay_annotations (start_timestamp_ns, end_timestamp_ns);

CREATE INDEX idx_lidar_replay_annotations_class ON lidar_replay_annotations (class_label);

CREATE UNIQUE INDEX idx_replay_evaluations_pair ON lidar_replay_evaluations (replay_case_id, reference_run_id, candidate_run_id);

CREATE INDEX idx_lidar_runs_created ON lidar_run_records (created_at);

CREATE INDEX idx_lidar_runs_source ON lidar_run_records (source_path);

CREATE INDEX idx_lidar_runs_parent ON lidar_run_records (parent_run_id);

CREATE INDEX idx_lidar_runs_status ON lidar_run_records (status);

CREATE INDEX idx_lidar_run_tracks_run ON lidar_run_tracks (run_id);

CREATE INDEX idx_lidar_run_tracks_class ON lidar_run_tracks (object_class);

CREATE INDEX idx_lidar_run_tracks_label ON lidar_run_tracks (user_label);

CREATE INDEX idx_lidar_run_tracks_state ON lidar_run_tracks (track_state);

CREATE INDEX idx_lidar_run_tracks_quality_label ON lidar_run_tracks (quality_label);

CREATE INDEX idx_lidar_tracks_sensor ON lidar_tracks (sensor_id);

CREATE INDEX idx_lidar_tracks_state ON lidar_tracks (track_state);

CREATE INDEX idx_lidar_tracks_time ON lidar_tracks (start_unix_nanos, end_unix_nanos);

CREATE INDEX idx_lidar_tracks_class ON lidar_tracks (object_class);

CREATE INDEX idx_lidar_tracks_quality ON lidar_tracks (track_length_meters, occlusion_count);

CREATE INDEX idx_lidar_run_missed_regions_run_id ON lidar_run_missed_regions (run_id);

CREATE INDEX idx_lidar_replay_cases_sensor ON lidar_replay_cases (sensor_id);

CREATE INDEX idx_lidar_replay_cases_pcap ON lidar_replay_cases (pcap_file);

CREATE INDEX idx_lidar_replay_evaluations_replay_case_created_at ON lidar_replay_evaluations (replay_case_id, created_at DESC);

CREATE INDEX idx_lidar_tuning_sweeps_sensor ON lidar_tuning_sweeps (sensor_id);

CREATE INDEX idx_lidar_tuning_sweeps_status ON lidar_tuning_sweeps (status);

CREATE INDEX idx_transit_links_transit ON radar_transit_links (transit_id);

CREATE INDEX idx_transit_links_data ON radar_transit_links (data_rowid);

CREATE INDEX idx_site_reports_site_id ON site_reports (site_id);

CREATE INDEX idx_site_reports_created_at ON site_reports (created_at DESC);

CREATE INDEX idx_site_name ON site (name);

CREATE INDEX idx_site_config_periods_site_id ON site_config_periods (site_id);

CREATE INDEX idx_site_config_periods_effective ON site_config_periods (site_id, effective_start_unix, effective_end_unix);

CREATE INDEX idx_site_config_periods_active ON site_config_periods (site_id, is_active)
    WHERE is_active = 1;

CREATE TRIGGER ensure_single_active_period_insert BEFORE INSERT ON site_config_periods WHEN NEW.is_active = 1 BEGIN
   UPDATE site_config_periods
      SET is_active = 0
    WHERE site_id = NEW.site_id
      AND is_active = 1;

END;

CREATE TRIGGER ensure_single_active_period_update BEFORE
   UPDATE OF is_active ON site_config_periods WHEN NEW.is_active = 1 BEGIN
   UPDATE site_config_periods
      SET is_active = 0
    WHERE site_id = NEW.site_id
      AND is_active = 1
      AND id != NEW.id;

END;

CREATE TRIGGER update_site_config_periods_timestamp AFTER
   UPDATE ON site_config_periods BEGIN
   UPDATE site_config_periods
      SET updated_at = UNIXEPOCH('subsec')
    WHERE id = NEW.id;

END;

CREATE TRIGGER update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

   CREATE VIEW lidar_all_tracks AS
   SELECT track_id
        , NULL AS run_id
        , sensor_id
        , track_state
        , start_unix_nanos
        , end_unix_nanos
        , observation_count
        , avg_speed_mps
        , max_speed_mps
        , bounding_box_length_avg
        , bounding_box_width_avg
        , bounding_box_height_avg
        , height_p95_max
        , intensity_mean_avg
        , object_class
        , object_confidence
        , classification_model
        , NULL AS user_label
        , NULL AS quality_label
     FROM lidar_tracks
UNION ALL
   SELECT track_id
        , run_id
        , sensor_id
        , track_state
        , start_unix_nanos
        , end_unix_nanos
        , observation_count
        , avg_speed_mps
        , max_speed_mps
        , bounding_box_length_avg
        , bounding_box_width_avg
        , bounding_box_height_avg
        , height_p95_max
        , intensity_mean_avg
        , object_class
        , object_confidence
        , classification_model
        , user_label
        , quality_label
     FROM lidar_run_tracks;

CREATE INDEX idx_lidar_param_sets_params_hash ON lidar_param_sets (params_hash);

CREATE INDEX idx_lidar_run_configs_config_hash ON lidar_run_configs (config_hash);

CREATE INDEX idx_lidar_run_records_run_config ON lidar_run_records (run_config_id);

CREATE INDEX idx_lidar_run_records_requested_param_set ON lidar_run_records (requested_param_set_id);

CREATE INDEX idx_lidar_run_records_replay_case ON lidar_run_records (replay_case_id);

CREATE INDEX idx_lidar_replay_cases_recommended_param_set ON lidar_replay_cases (recommended_param_set_id);
