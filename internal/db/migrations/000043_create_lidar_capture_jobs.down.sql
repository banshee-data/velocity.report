-- Rollback: Drop motion timelines and capture jobs
-- WARNING: This discards every motion/static timeline computed for a session.
-- The captures themselves are untouched; the timelines can be recomputed by
-- re-running the motion passes, which reads every file again.
     DROP INDEX IF EXISTS idx_lidar_capture_jobs_session;

     DROP INDEX IF EXISTS idx_lidar_capture_jobs_state;

     DROP TABLE IF EXISTS lidar_capture_jobs;

     DROP INDEX IF EXISTS idx_lidar_capture_motion_periods_type;

     DROP INDEX IF EXISTS idx_lidar_capture_motion_periods_session;

     DROP TABLE IF EXISTS lidar_capture_motion_periods;
