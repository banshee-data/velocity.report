-- Rollback: Remove lidar_bg_snapshot table
     DROP INDEX IF EXISTS idx_bg_snapshot_sensor_time;

     DROP TABLE IF EXISTS lidar_bg_snapshot;
