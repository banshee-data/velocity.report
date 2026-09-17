-- Rollback: measurement provenance from legacy tracker rows.
    ALTER TABLE lidar_track_observations
     DROP COLUMN measurement_source;

    ALTER TABLE lidar_track_observations
     DROP COLUMN frame_unix_nanos;

     DROP INDEX IF EXISTS idx_lidar_observations_calibration_time;

     DROP INDEX IF EXISTS idx_lidar_observations_source_time;

     DROP TABLE IF EXISTS lidar_observations;
