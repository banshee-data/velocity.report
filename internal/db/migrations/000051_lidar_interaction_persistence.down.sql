-- Rollback: LiDAR behaviour interaction persistence
-- Derived rows only; they are regenerated from persisted estimates.
     DROP INDEX IF EXISTS idx_lidar_exposure_windows_track;

     DROP INDEX IF EXISTS idx_lidar_exposure_windows_version;

     DROP INDEX IF EXISTS idx_lidar_exposure_windows_event;

     DROP TABLE IF EXISTS lidar_exposure_windows;

     DROP TABLE IF EXISTS lidar_interaction_instants;

     DROP INDEX IF EXISTS idx_lidar_interaction_events_type;

     DROP INDEX IF EXISTS idx_lidar_interaction_events_secondary;

     DROP INDEX IF EXISTS idx_lidar_interaction_events_primary;

     DROP INDEX IF EXISTS idx_lidar_interaction_events_version;

     DROP TABLE IF EXISTS lidar_interaction_events;
