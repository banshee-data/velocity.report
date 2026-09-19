-- Rollback: LiDAR track filter residual records
     DROP INDEX IF EXISTS idx_lidar_track_residuals_observation;

     DROP TABLE IF EXISTS lidar_track_residuals;
