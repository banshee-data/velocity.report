-- Rollback: remove corrected-measurement provenance from legacy tracker rows.
    ALTER TABLE lidar_track_observations
     DROP COLUMN measurement_source;

    ALTER TABLE lidar_track_observations
     DROP COLUMN frame_unix_nanos;
