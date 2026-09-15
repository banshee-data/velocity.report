-- Rollback: deterministic per-track creation sequence on estimates
    ALTER TABLE lidar_track_estimates
     DROP COLUMN creation_sequence;
