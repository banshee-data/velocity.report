-- D-31 terminology cutover: preserve keys, values and relationships.
   INSERT INTO lidar_replay_case_files (replay_case_id, ordinal, pcap_file)
   SELECT replay_case_id
        , 0
        , pcap_file
     FROM lidar_replay_cases c
    WHERE NOT EXISTS (
             SELECT 1
               FROM lidar_replay_case_files f
              WHERE f.replay_case_id = c.replay_case_id
          );

    ALTER TABLE lidar_run_records
   RENAME COLUMN replay_case_id TO clip_id;

    ALTER TABLE lidar_replay_cases
   RENAME COLUMN replay_case_id TO clip_id;

    ALTER TABLE lidar_replay_case_files
   RENAME COLUMN replay_case_id TO clip_id;

    ALTER TABLE lidar_replay_evaluations
   RENAME COLUMN replay_case_id TO clip_id;

    ALTER TABLE lidar_replay_annotations
   RENAME COLUMN replay_case_id TO clip_id;

    ALTER TABLE lidar_segment_selections
   RENAME COLUMN replay_case_id TO clip_id;

    ALTER TABLE lidar_replay_cases
RENAME TO lidar_clips;

    ALTER TABLE lidar_replay_case_files
RENAME TO lidar_clip_captures;

    ALTER TABLE lidar_replay_evaluations
RENAME TO lidar_clip_evaluations;

    ALTER TABLE lidar_run_records
RENAME TO lidar_runs;

     DROP INDEX idx_lidar_replay_annotations_replay_case;

CREATE INDEX idx_lidar_replay_annotations_clip ON lidar_replay_annotations (clip_id);

     DROP INDEX idx_replay_evaluations_pair;

CREATE UNIQUE INDEX idx_clip_evaluations_pair ON lidar_clip_evaluations (clip_id, reference_run_id, candidate_run_id);

     DROP INDEX idx_lidar_replay_cases_sensor;

CREATE INDEX idx_lidar_clips_sensor ON lidar_clips (sensor_id);

     DROP INDEX idx_lidar_replay_cases_pcap;

     DROP INDEX idx_lidar_replay_evaluations_replay_case_created_at;

CREATE INDEX idx_lidar_clip_evaluations_clip_created_at ON lidar_clip_evaluations (clip_id, created_at DESC);

     DROP INDEX idx_lidar_run_records_run_config;

CREATE INDEX idx_lidar_runs_run_config ON lidar_runs (run_config_id);

     DROP INDEX idx_lidar_run_records_requested_param_set;

CREATE INDEX idx_lidar_runs_requested_param_set ON lidar_runs (requested_param_set_id);

     DROP INDEX idx_lidar_run_records_replay_case;

CREATE INDEX idx_lidar_runs_clip ON lidar_runs (clip_id);

     DROP INDEX idx_lidar_replay_cases_recommended_param_set;

CREATE INDEX idx_lidar_clips_recommended_param_set ON lidar_clips (recommended_param_set_id);

     DROP INDEX idx_lidar_replay_case_files_case;

CREATE INDEX idx_lidar_clip_captures_case ON lidar_clip_captures (clip_id, ordinal);

     DROP INDEX idx_lidar_replay_case_files_capture;

CREATE INDEX idx_lidar_clip_captures_capture ON lidar_clip_captures (capture_file_id);

     DROP INDEX idx_lidar_replay_cases_session;

CREATE INDEX idx_lidar_clips_session ON lidar_clips (session_id);

     DROP INDEX idx_lidar_replay_cases_s2_l10;

CREATE INDEX idx_lidar_clips_s2_l10 ON lidar_clips (s2_l10_token);

     DROP INDEX idx_lidar_replay_cases_s2_l13;

CREATE INDEX idx_lidar_clips_s2_l13 ON lidar_clips (s2_l13_token);

     DROP INDEX idx_lidar_replay_cases_s2_l16;

CREATE INDEX idx_lidar_clips_s2_l16 ON lidar_clips (s2_l16_token);

     DROP INDEX idx_lidar_replay_cases_site_id;

CREATE INDEX idx_lidar_clips_site_id ON lidar_clips (site_id);

    ALTER TABLE lidar_clips
     DROP COLUMN pcap_file;

-- Stored sweep requests carry the same clip identity and source under new keys.
   UPDATE lidar_tuning_sweeps
      SET request = JSON_REMOVE(
          JSON_SET(
          request
        , '$.clip_id'
        , COALESCE(
          NULLIF(JSON_EXTRACT(request, '$.clip_id'), '')
        , NULLIF(JSON_EXTRACT(request, '$.replay_case_id'), '')
        , NULLIF(JSON_EXTRACT(request, '$.scene_id'), '')
          )
          )
        , '$.replay_case_id'
        , '$.scene_id'
          )
    WHERE JSON_VALID(request)
      AND JSON_TYPE(request) = 'object'
      AND (
          JSON_TYPE(request, '$.replay_case_id') IS NOT NULL
       OR JSON_TYPE(request, '$.scene_id') IS NOT NULL
          );

   UPDATE lidar_tuning_sweeps
      SET request = JSON_SET(request, '$.data_source', 'capture')
    WHERE JSON_VALID(request)
      AND JSON_EXTRACT(request, '$.data_source') = 'pcap';
