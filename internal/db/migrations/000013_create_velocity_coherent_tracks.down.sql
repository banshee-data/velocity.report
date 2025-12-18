-- Migration 000013 down: Remove velocity-coherent tracking tables
     DROP INDEX IF EXISTS idx_algorithm_config_time;

     DROP TABLE IF EXISTS lidar_algorithm_config_log;

     DROP INDEX IF EXISTS idx_track_merges_later;

     DROP INDEX IF EXISTS idx_track_merges_earlier;

     DROP INDEX IF EXISTS idx_track_merges_result;

     DROP TABLE IF EXISTS lidar_track_merges;

     DROP INDEX IF EXISTS idx_vc_track_obs_time;

     DROP INDEX IF EXISTS idx_vc_track_obs_track;

     DROP TABLE IF EXISTS lidar_velocity_coherent_track_obs;

     DROP INDEX IF EXISTS idx_vc_tracks_class;

     DROP INDEX IF EXISTS idx_vc_tracks_time;

     DROP INDEX IF EXISTS idx_vc_tracks_state;

     DROP INDEX IF EXISTS idx_vc_tracks_sensor;

     DROP TABLE IF EXISTS lidar_velocity_coherent_tracks;

     DROP INDEX IF EXISTS idx_vc_clusters_sensor_time;

     DROP TABLE IF EXISTS lidar_velocity_coherent_clusters;
