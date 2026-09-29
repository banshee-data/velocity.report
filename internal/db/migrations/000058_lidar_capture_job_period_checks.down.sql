-- Rollback: capture jobs of any kind, and motion periods with any bounds, are
-- accepted again. No stored row changes in either direction.
     DROP TRIGGER IF EXISTS lidar_capture_motion_periods_check_update;

     DROP TRIGGER IF EXISTS lidar_capture_motion_periods_check_insert;

     DROP TRIGGER IF EXISTS lidar_capture_jobs_check_update;

     DROP TRIGGER IF EXISTS lidar_capture_jobs_check_insert;
