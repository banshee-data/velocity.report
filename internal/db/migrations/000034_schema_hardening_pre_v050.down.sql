PRAGMA foreign_keys = OFF;

     DROP VIEW IF EXISTS lidar_all_tracks;

     DROP TRIGGER IF EXISTS ensure_single_active_period_insert;

     DROP TRIGGER IF EXISTS ensure_single_active_period_update;

     DROP TRIGGER IF EXISTS update_site_config_periods_timestamp;

     DROP TRIGGER IF EXISTS update_site_timestamp;

     DROP INDEX IF EXISTS idx_lidar_replay_evaluations_replay_case_created_at;

     DROP INDEX IF EXISTS idx_lidar_runs_parent;

     DROP INDEX IF EXISTS idx_lidar_runs_status;

     DROP INDEX IF EXISTS idx_lidar_runs_source;

     DROP INDEX IF EXISTS idx_lidar_runs_created;

     DROP INDEX IF EXISTS idx_lidar_run_tracks_run;

     DROP INDEX IF EXISTS idx_lidar_run_tracks_class;

     DROP INDEX IF EXISTS idx_lidar_run_tracks_label;

     DROP INDEX IF EXISTS idx_lidar_run_tracks_state;

     DROP INDEX IF EXISTS idx_lidar_run_tracks_quality_label;

     DROP INDEX IF EXISTS idx_lidar_tracks_sensor;

     DROP INDEX IF EXISTS idx_lidar_tracks_state;

     DROP INDEX IF EXISTS idx_lidar_tracks_time;

     DROP INDEX IF EXISTS idx_lidar_tracks_class;

     DROP INDEX IF EXISTS idx_lidar_tracks_quality;

     DROP INDEX IF EXISTS idx_lidar_run_missed_regions_run_id;

     DROP INDEX IF EXISTS idx_lidar_replay_cases_sensor;

     DROP INDEX IF EXISTS idx_lidar_replay_cases_pcap;

     DROP INDEX IF EXISTS idx_lidar_tuning_sweeps_sensor;

     DROP INDEX IF EXISTS idx_lidar_tuning_sweeps_status;

     DROP INDEX IF EXISTS idx_transit_links_transit;

     DROP INDEX IF EXISTS idx_transit_links_data;

     DROP INDEX IF EXISTS idx_site_reports_site_id;

     DROP INDEX IF EXISTS idx_site_reports_created_at;

     DROP INDEX IF EXISTS idx_site_name;

     DROP INDEX IF EXISTS idx_site_config_periods_site_id;

     DROP INDEX IF EXISTS idx_site_config_periods_effective;

     DROP INDEX IF EXISTS idx_site_config_periods_active;

   CREATE TABLE lidar_run_records_old (
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
        , statistics_json TEXT
        , vrlog_path TEXT
          );

   INSERT INTO lidar_run_records_old
   SELECT run_id
        , created_at
        , source_type
        , source_path
        , sensor_id
        , params_json
        , duration_secs
        , total_frames
        , total_clusters
        , total_tracks
        , confirmed_tracks
        , processing_time_ms
        , status
        , error_message
        , parent_run_id
        , notes
        , statistics_json
        , vrlog_path
     FROM lidar_run_records;

     DROP TABLE lidar_run_records;

    ALTER TABLE lidar_run_records_old
RENAME TO lidar_run_records;

   CREATE TABLE lidar_replay_cases_old (
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
        , FOREIGN KEY (reference_run_id) REFERENCES lidar_run_records (run_id) ON DELETE SET NULL
          );

   INSERT INTO lidar_replay_cases_old
   SELECT replay_case_id
        , sensor_id
        , pcap_file
        , pcap_start_secs
        , pcap_duration_secs
        , description
        , reference_run_id
        , optimal_params_json
        , created_at_ns
        , updated_at_ns
     FROM lidar_replay_cases;

     DROP TABLE lidar_replay_cases;

    ALTER TABLE lidar_replay_cases_old
RENAME TO lidar_replay_cases;

   CREATE TABLE lidar_run_tracks_old (
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
        , FOREIGN KEY (run_id) REFERENCES lidar_run_records (run_id) ON DELETE CASCADE
          );

   INSERT INTO lidar_run_tracks_old
   SELECT run_id
        , track_id
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
        , label_confidence
        , labeler_id
        , labeled_at
        , is_split_candidate
        , is_merge_candidate
        , linked_track_ids
        , quality_label
        , label_source
     FROM lidar_run_tracks;

     DROP TABLE lidar_run_tracks;

    ALTER TABLE lidar_run_tracks_old
RENAME TO lidar_run_tracks;

   CREATE TABLE lidar_tracks_old (
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
          );

   INSERT INTO lidar_tracks_old
   SELECT track_id
        , sensor_id
        , frame_id
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
        , track_length_meters
        , track_duration_secs
        , occlusion_count
        , max_occlusion_frames
        , spatial_coverage
        , noise_point_ratio
     FROM lidar_tracks;

     DROP TABLE lidar_tracks;

    ALTER TABLE lidar_tracks_old
RENAME TO lidar_tracks;

   CREATE TABLE lidar_run_missed_regions_old (
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
        , FOREIGN KEY (run_id) REFERENCES lidar_run_records (run_id) ON DELETE CASCADE
          );

   INSERT INTO lidar_run_missed_regions_old (
          region_id
        , run_id
        , center_x
        , center_y
        , radius_m
        , time_start_ns
        , time_end_ns
        , expected_label
        , labeler_id
        , labeled_at
        , notes
          )
   SELECT region_id
        , run_id
        , center_x
        , center_y
        , radius_m
        , time_start_ns
        , time_end_ns
        , expected_label
        , labeler_id
        , labeled_at
        , notes
     FROM lidar_run_missed_regions;

     DROP TABLE lidar_run_missed_regions;

    ALTER TABLE lidar_run_missed_regions_old
RENAME TO lidar_run_missed_regions;

   CREATE TABLE lidar_tuning_sweeps_old (
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
          );

   INSERT INTO lidar_tuning_sweeps_old
   SELECT id
        , sweep_id
        , sensor_id
        , mode
        , status
        , request
        , results
        , charts
        , recommendation
        , round_results
        , error
        , started_at
        , completed_at
        , created_at
        , objective_name
        , objective_version
        , transform_pipeline_name
        , transform_pipeline_version
        , score_components_json
        , recommendation_explanation_json
        , label_provenance_summary_json
        , checkpoint_round
        , checkpoint_bounds
        , checkpoint_results
        , checkpoint_request
     FROM lidar_tuning_sweeps;

     DROP TABLE lidar_tuning_sweeps;

    ALTER TABLE lidar_tuning_sweeps_old
RENAME TO lidar_tuning_sweeps;

   CREATE TABLE site_old (
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
          );

   INSERT INTO site_old
   SELECT id
        , name
        , location
        , description
        , surveyor
        , contact
        , address
        , latitude
        , longitude
        , map_angle
        , include_map
        , site_description
        , created_at
        , updated_at
        , bbox_ne_lat
        , bbox_ne_lng
        , bbox_sw_lat
        , bbox_sw_lng
        , map_svg_data
     FROM site;

     DROP TABLE site;

    ALTER TABLE site_old
RENAME TO site;

   CREATE TABLE site_config_periods_old (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , site_id INTEGER NOT NULL
        , effective_start_unix DOUBLE NOT NULL
        , effective_end_unix DOUBLE
        , is_active INTEGER NOT NULL DEFAULT 0
        , notes TEXT
        , cosine_error_angle DOUBLE NOT NULL DEFAULT 0
        , created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , updated_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE
          );

   INSERT INTO site_config_periods_old
   SELECT id
        , site_id
        , effective_start_unix
        , effective_end_unix
        , is_active
        , notes
        , cosine_error_angle
        , created_at
        , updated_at
     FROM site_config_periods;

     DROP TABLE site_config_periods;

    ALTER TABLE site_config_periods_old
RENAME TO site_config_periods;

   CREATE TABLE site_reports_old (
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

   INSERT INTO site_reports_old (
          id
        , site_id
        , start_date
        , end_date
        , filepath
        , filename
        , zip_filepath
        , zip_filename
        , run_id
        , timezone
        , units
        , source
        , created_at
          )
   SELECT id
        , COALESCE(site_id, 0)
        , start_date
        , end_date
        , filepath
        , filename
        , zip_filepath
        , zip_filename
        , run_id
        , timezone
        , units
        , source
        , created_at
     FROM site_reports;

     DROP TABLE site_reports;

    ALTER TABLE site_reports_old
RENAME TO site_reports;

   CREATE TABLE radar_data_old (
          write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , raw_event JSON NOT NULL
        , uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED
        , magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED
        , speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
          );

   INSERT INTO radar_data_old (rowid, write_timestamp, raw_event)
   SELECT data_id
        , write_timestamp
        , raw_event
     FROM radar_data;

     DROP TABLE radar_data;

    ALTER TABLE radar_data_old
RENAME TO radar_data;

   CREATE TABLE radar_transit_links_old (
          link_id INTEGER PRIMARY KEY AUTOINCREMENT
        , transit_id INTEGER NOT NULL REFERENCES radar_data_transits (transit_id) ON DELETE CASCADE
        , data_rowid INTEGER NOT NULL
        , link_score DOUBLE
        , created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , UNIQUE (transit_id, data_rowid)
          );

   INSERT INTO radar_transit_links_old (link_id, transit_id, data_rowid, link_score, created_at)
   SELECT link_id
        , transit_id
        , data_rowid
        , link_score
        , created_at
     FROM radar_transit_links;

     DROP TABLE radar_transit_links;

    ALTER TABLE radar_transit_links_old
RENAME TO radar_transit_links;

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

   CREATE VIEW IF NOT EXISTS lidar_all_tracks AS
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

PRAGMA foreign_keys = ON;
