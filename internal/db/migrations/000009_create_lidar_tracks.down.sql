-- Rollback Phase 3.3: LIDAR Tracks Schema
DROP INDEX IF EXISTS idx_lidar_track_obs_time;
DROP INDEX IF EXISTS idx_lidar_track_obs_track;
DROP TABLE IF EXISTS lidar_track_obs;

DROP INDEX IF EXISTS idx_lidar_tracks_class;
DROP INDEX IF EXISTS idx_lidar_tracks_time;
DROP INDEX IF EXISTS idx_lidar_tracks_state;
DROP INDEX IF EXISTS idx_lidar_tracks_sensor;
DROP TABLE IF EXISTS lidar_tracks;

DROP INDEX IF EXISTS idx_lidar_clusters_sensor_time;
DROP TABLE IF EXISTS lidar_clusters;
