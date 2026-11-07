-- Lane proposals per clip (docs/plans/lidar-lane-field-batch-plan.md): one
-- row per lane_proposals job or run-form call, created when the job is queued
-- and completed when its lane fields are in Gerardus and the proposals have
-- been derived. The row keeps what placed the fields and what built them, so
-- the clips view can say when a clip's lanes are stale. The fields themselves
-- are never stored here (D-39, ADR-0011): only Gerardus's ids for them.
   CREATE TABLE lidar_clip_lane_proposals (
          proposal_run_id TEXT PRIMARY KEY
        , clip_id TEXT
        , job_id TEXT
        , run_id TEXT
        , l16_token TEXT
        , sensor_id TEXT
        , record_id TEXT
        , gerardus_version TEXT
        , field_ids_json TEXT
        , window_start_ns INTEGER
        , window_end_ns INTEGER
        , track_count INTEGER
        , pipeline_build TEXT
        , tuning_fingerprint TEXT
        , high INTEGER
        , medium INTEGER
        , adopted INTEGER
        , rejected INTEGER
        , outstanding INTEGER
        , derive_secs REAL
        , created_at_ns INTEGER NOT NULL
        , completed_at_ns INTEGER
        , CHECK (
          clip_id IS NOT NULL
       OR run_id IS NOT NULL
          )
        , CHECK (
          field_ids_json IS NULL
       OR (
          JSON_VALID(field_ids_json)
      AND JSON_TYPE(field_ids_json) = 'array'
          )
          )
        , CHECK (
          completed_at_ns IS NULL
       OR (
          record_id IS NOT NULL
      AND field_ids_json IS NOT NULL
          )
          )
        , FOREIGN KEY (job_id) REFERENCES lidar_capture_jobs (job_id) ON DELETE SET NULL
        , FOREIGN KEY (clip_id) REFERENCES lidar_clips (clip_id) ON DELETE CASCADE
          );

CREATE INDEX idx_lidar_clip_lane_proposals_clip ON lidar_clip_lane_proposals (clip_id, created_at_ns);

CREATE UNIQUE INDEX idx_lidar_clip_lane_proposals_job ON lidar_clip_lane_proposals (job_id);

     DROP TRIGGER lidar_capture_jobs_check_insert;

     DROP TRIGGER lidar_capture_jobs_check_update;

CREATE TRIGGER lidar_capture_jobs_check_insert BEFORE INSERT ON lidar_capture_jobs BEGIN
   SELECT RAISE (
          ABORT
        , 'a capture job is a motion_pass, a vrlog_record, a capture_export or a lane_proposals'
          )
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record', 'capture_export', 'lane_proposals');

   SELECT RAISE (
          ABORT
        , 'a clip job names its segment in lidar_segment_clip_jobs, not in session_id'
          )
    WHERE NEW.kind = 'vrlog_record'
      AND NEW.session_id IS NOT NULL;

   SELECT RAISE (
          ABORT
        , 'an export job names its clip in lidar_clip_extracts, not in session_id'
          )
    WHERE NEW.kind = 'capture_export'
      AND NEW.session_id IS NOT NULL;

   SELECT RAISE (
          ABORT
        , 'a lane proposals job names its clip in lidar_clip_lane_proposals, not in session_id'
          )
    WHERE NEW.kind = 'lane_proposals'
      AND NEW.session_id IS NOT NULL;

END;

CREATE TRIGGER lidar_capture_jobs_check_update BEFORE
   UPDATE OF kind
        , session_id ON lidar_capture_jobs BEGIN
   SELECT RAISE (
          ABORT
        , 'a capture job is a motion_pass, a vrlog_record, a capture_export or a lane_proposals'
          )
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record', 'capture_export', 'lane_proposals');

   SELECT RAISE (
          ABORT
        , 'a clip job names its segment in lidar_segment_clip_jobs, not in session_id'
          )
    WHERE NEW.kind = 'vrlog_record'
      AND NEW.session_id IS NOT NULL;

   SELECT RAISE (
          ABORT
        , 'an export job names its clip in lidar_clip_extracts, not in session_id'
          )
    WHERE NEW.kind = 'capture_export'
      AND NEW.session_id IS NOT NULL;

   SELECT RAISE (
          ABORT
        , 'a lane proposals job names its clip in lidar_clip_lane_proposals, not in session_id'
          )
    WHERE NEW.kind = 'lane_proposals'
      AND NEW.session_id IS NOT NULL;

END;
