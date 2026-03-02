ALTER TABLE lidar_run_tracks
      ADD COLUMN avg_speed_mps REAL DEFAULT 0;

    ALTER TABLE lidar_tracks
      ADD COLUMN avg_speed_mps REAL DEFAULT 0;
