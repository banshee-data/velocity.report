-- 000030 (down): Revert p98_speed_mps back to p95_speed_mps
    ALTER TABLE lidar_run_tracks
   RENAME COLUMN p98_speed_mps TO p95_speed_mps;

    ALTER TABLE lidar_tracks
   RENAME COLUMN p98_speed_mps TO p95_speed_mps;
