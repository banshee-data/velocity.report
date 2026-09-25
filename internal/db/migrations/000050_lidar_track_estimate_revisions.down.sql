-- Rollback: revision records for retrospectively refined LiDAR estimates
-- The refined estimates themselves remain in lidar_track_estimates under their
-- own stage and parameter hash; only their audit records are dropped.
     DROP INDEX IF EXISTS idx_lidar_track_estimate_revisions_revises;

     DROP TABLE IF EXISTS lidar_track_estimate_revisions;
