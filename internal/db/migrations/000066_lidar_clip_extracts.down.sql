-- Extract files on disk are left in place; only their records go. The
-- export jobs go too, since the earlier triggers do not admit their kind.
     DROP TRIGGER lidar_capture_jobs_check_insert;

     DROP TRIGGER lidar_capture_jobs_check_update;

   DELETE FROM lidar_capture_jobs
    WHERE kind = 'capture_export';

CREATE TRIGGER lidar_capture_jobs_check_insert BEFORE INSERT ON lidar_capture_jobs BEGIN
   SELECT RAISE (ABORT, 'a capture job is a motion_pass or a vrlog_record')
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record');

   SELECT RAISE (
          ABORT
        , 'a clip job names its segment in lidar_segment_clip_jobs, not in session_id'
          )
    WHERE NEW.kind = 'vrlog_record'
      AND NEW.session_id IS NOT NULL;

END;

CREATE TRIGGER lidar_capture_jobs_check_update BEFORE
   UPDATE OF kind
        , session_id ON lidar_capture_jobs BEGIN
   SELECT RAISE (ABORT, 'a capture job is a motion_pass or a vrlog_record')
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record');

   SELECT RAISE (
          ABORT
        , 'a clip job names its segment in lidar_segment_clip_jobs, not in session_id'
          )
    WHERE NEW.kind = 'vrlog_record'
      AND NEW.session_id IS NOT NULL;

END;

     DROP INDEX IF EXISTS idx_lidar_clip_extracts_clip;

     DROP TABLE IF EXISTS lidar_clip_extracts;

     DROP INDEX IF EXISTS idx_lidar_clips_dataset_id;

    ALTER TABLE lidar_clips
     DROP COLUMN storage;

    ALTER TABLE lidar_clips
     DROP COLUMN north_azimuth_deg;

    ALTER TABLE lidar_clips
     DROP COLUMN dataset_id;
