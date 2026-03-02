-- 000030: Rename p95_speed_mps to p98_speed_mps
-- The codebase standardises on p98 (98th percentile) for the high speed metric.
-- Previously, some layers used p95 while others used p98; this migration
-- reconciles the database columns to match.
    ALTER TABLE lidar_run_tracks
   RENAME COLUMN p95_speed_mps TO p98_speed_mps;

    ALTER TABLE lidar_tracks
   RENAME COLUMN p95_speed_mps TO p98_speed_mps;
