-- Rollback: Drop the capture index
-- WARNING: This discards the record of which capture files were seen on which
-- volume, and the sessions derived from them. The files themselves are untouched.
     DROP INDEX IF EXISTS idx_lidar_capture_sessions_root;

     DROP TABLE IF EXISTS lidar_capture_sessions;

     DROP INDEX IF EXISTS idx_lidar_capture_files_start;

     DROP INDEX IF EXISTS idx_lidar_capture_files_session;

     DROP INDEX IF EXISTS idx_lidar_capture_files_root;

     DROP TABLE IF EXISTS lidar_capture_files;

     DROP INDEX IF EXISTS idx_lidar_capture_roots_enabled;

     DROP TABLE IF EXISTS lidar_capture_roots;
