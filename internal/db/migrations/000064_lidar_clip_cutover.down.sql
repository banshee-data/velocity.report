PRAGMA foreign_keys = OFF;

PRAGMA legacy_alter_table = OFF;

     DROP INDEX idx_lidar_replay_annotations_clip;

     DROP INDEX idx_clip_evaluations_pair;

     DROP INDEX idx_lidar_clips_sensor;

     DROP INDEX idx_lidar_clip_evaluations_clip_created_at;

     DROP INDEX idx_lidar_runs_run_config;

     DROP INDEX idx_lidar_runs_requested_param_set;

     DROP INDEX idx_lidar_runs_clip;

     DROP INDEX idx_lidar_clips_recommended_param_set;

     DROP INDEX idx_lidar_clip_captures_case;

     DROP INDEX idx_lidar_clip_captures_capture;

     DROP INDEX idx_lidar_clips_session;

     DROP INDEX idx_lidar_clips_s2_l10;

     DROP INDEX idx_lidar_clips_s2_l13;

     DROP INDEX idx_lidar_clips_s2_l16;

     DROP INDEX idx_lidar_clips_site_id;

    ALTER TABLE lidar_runs
RENAME TO lidar_run_records;

    ALTER TABLE lidar_clip_evaluations
RENAME TO lidar_replay_evaluations;

    ALTER TABLE lidar_clip_captures
RENAME TO lidar_replay_case_files;

    ALTER TABLE lidar_clips
RENAME TO lidar_replay_cases;

    ALTER TABLE lidar_run_records
   RENAME COLUMN clip_id TO replay_case_id;

    ALTER TABLE lidar_replay_cases
   RENAME COLUMN clip_id TO replay_case_id;

    ALTER TABLE lidar_replay_case_files
   RENAME COLUMN clip_id TO replay_case_id;

    ALTER TABLE lidar_replay_evaluations
   RENAME COLUMN clip_id TO replay_case_id;

    ALTER TABLE lidar_replay_annotations
   RENAME COLUMN clip_id TO replay_case_id;

    ALTER TABLE lidar_segment_selections
   RENAME COLUMN clip_id TO replay_case_id;

PRAGMA legacy_alter_table = ON;

   CREATE TABLE lidar_replay_cases_restored (
          replay_case_id TEXT PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , pcap_file TEXT NOT NULL
        , pcap_start_secs REAL
        , pcap_duration_secs REAL
        , description TEXT
        , reference_run_id TEXT
        , created_at_ns INTEGER NOT NULL
        , updated_at_ns INTEGER
        , recommended_param_set_id TEXT REFERENCES lidar_param_sets (param_set_id) ON DELETE SET NULL
        , session_id TEXT
        , source_period_id TEXT
        , origin_lat REAL
        , origin_lon REAL
        , s2_l10_token TEXT
        , s2_l13_token TEXT
        , s2_l16_token TEXT
        , geographic_source TEXT
        , geographic_status TEXT NOT NULL DEFAULT 'unavailable'
        , site_id TEXT REFERENCES lidar_sites (site_id) ON DELETE SET NULL
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

   INSERT INTO lidar_replay_cases_restored (
          replay_case_id
        , sensor_id
        , pcap_file
        , pcap_start_secs
        , pcap_duration_secs
        , description
        , reference_run_id
        , created_at_ns
        , updated_at_ns
        , recommended_param_set_id
        , session_id
        , source_period_id
        , origin_lat
        , origin_lon
        , s2_l10_token
        , s2_l13_token
        , s2_l16_token
        , geographic_source
        , geographic_status
        , site_id
          )
   SELECT c.replay_case_id
        , c.sensor_id
        , COALESCE(
          (
             SELECT pcap_file
               FROM lidar_replay_case_files f
              WHERE f.replay_case_id = c.replay_case_id
           ORDER BY ordinal
              LIMIT 1
          )
        , ''
          )
        , c.pcap_start_secs
        , c.pcap_duration_secs
        , c.description
        , c.reference_run_id
        , c.created_at_ns
        , c.updated_at_ns
        , c.recommended_param_set_id
        , c.session_id
        , c.source_period_id
        , c.origin_lat
        , c.origin_lon
        , c.s2_l10_token
        , c.s2_l13_token
        , c.s2_l16_token
        , c.geographic_source
        , c.geographic_status
        , c.site_id
     FROM lidar_replay_cases c;

     DROP TABLE lidar_replay_cases;

    ALTER TABLE lidar_replay_cases_restored
RENAME TO lidar_replay_cases;

CREATE INDEX idx_lidar_replay_annotations_replay_case ON lidar_replay_annotations (replay_case_id);

CREATE UNIQUE INDEX idx_replay_evaluations_pair ON lidar_replay_evaluations (replay_case_id, reference_run_id, candidate_run_id);

CREATE INDEX idx_lidar_replay_cases_sensor ON lidar_replay_cases (sensor_id);

CREATE INDEX idx_lidar_replay_cases_pcap ON lidar_replay_cases (pcap_file);

CREATE INDEX idx_lidar_replay_evaluations_replay_case_created_at ON lidar_replay_evaluations (replay_case_id, created_at DESC);

CREATE INDEX idx_lidar_run_records_run_config ON lidar_run_records (run_config_id);

CREATE INDEX idx_lidar_run_records_requested_param_set ON lidar_run_records (requested_param_set_id);

CREATE INDEX idx_lidar_run_records_replay_case ON lidar_run_records (replay_case_id);

CREATE INDEX idx_lidar_replay_cases_recommended_param_set ON lidar_replay_cases (recommended_param_set_id);

CREATE INDEX idx_lidar_replay_case_files_case ON lidar_replay_case_files (replay_case_id, ordinal);

CREATE INDEX idx_lidar_replay_case_files_capture ON lidar_replay_case_files (capture_file_id);

CREATE INDEX idx_lidar_replay_cases_session ON lidar_replay_cases (session_id);

CREATE INDEX idx_lidar_replay_cases_s2_l10 ON lidar_replay_cases (s2_l10_token);

CREATE INDEX idx_lidar_replay_cases_s2_l13 ON lidar_replay_cases (s2_l13_token);

CREATE INDEX idx_lidar_replay_cases_s2_l16 ON lidar_replay_cases (s2_l16_token);

CREATE INDEX idx_lidar_replay_cases_site_id ON lidar_replay_cases (site_id);

   UPDATE lidar_tuning_sweeps
      SET request = JSON_REMOVE(
          JSON_SET(request, '$.replay_case_id', JSON_EXTRACT(request, '$.clip_id'))
        , '$.clip_id'
          )
    WHERE JSON_VALID(request)
      AND JSON_TYPE(request, '$.clip_id') IS NOT NULL;

   UPDATE lidar_tuning_sweeps
      SET request = JSON_SET(request, '$.data_source', 'pcap')
    WHERE JSON_VALID(request)
      AND JSON_EXTRACT(request, '$.data_source') = 'capture';

PRAGMA legacy_alter_table = OFF;

PRAGMA foreign_keys = ON;
