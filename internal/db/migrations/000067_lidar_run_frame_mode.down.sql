-- Full-frames runs keep their VRLOGs on disk; only the record of which frames
-- they hold goes.
    ALTER TABLE lidar_runs
     DROP COLUMN frame_mode;
