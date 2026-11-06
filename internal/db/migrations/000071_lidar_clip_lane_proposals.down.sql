-- The lane proposal rows go, and the lane jobs with them, since the earlier
-- triggers do not admit their kind. Gerardus keeps the fields it stored.
     DROP TRIGGER lidar_capture_jobs_check_insert;

     DROP TRIGGER lidar_capture_jobs_check_update;

     DROP INDEX IF EXISTS idx_lidar_clip_lane_proposals_job;

     DROP INDEX IF EXISTS idx_lidar_clip_lane_proposals_clip;

     DROP TABLE IF EXISTS lidar_clip_lane_proposals;

   DELETE FROM lidar_capture_jobs
    WHERE kind = 'lane_proposals';

CREATE TRIGGER lidar_capture_jobs_check_insert BEFORE INSERT ON lidar_capture_jobs BEGIN
   SELECT RAISE (
          ABORT
        , 'a capture job is a motion_pass, a vrlog_record or a capture_export'
          )
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record', 'capture_export');

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

END;

CREATE TRIGGER lidar_capture_jobs_check_update BEFORE
   UPDATE OF kind
        , session_id ON lidar_capture_jobs BEGIN
   SELECT RAISE (
          ABORT
        , 'a capture job is a motion_pass, a vrlog_record or a capture_export'
          )
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record', 'capture_export');

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

END;
