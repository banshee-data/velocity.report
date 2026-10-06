-- Rollback: versioned LiDAR track state records
     DROP INDEX IF EXISTS idx_lidar_track_estimates_observation;

     DROP TABLE IF EXISTS lidar_track_estimates;
