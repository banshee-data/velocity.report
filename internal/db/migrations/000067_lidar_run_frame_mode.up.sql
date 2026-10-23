-- What a run's VRLOG frames hold: foreground points only (split streaming), or
-- full frames with background and foreground, chosen when the replay starts
-- (POST /api/lidar/pcap/start full_frames). Every VRLOG recorded before this
-- was written after background was stripped, so existing runs are foreground.
    ALTER TABLE lidar_runs
      ADD COLUMN frame_mode TEXT NOT NULL DEFAULT 'foreground' CHECK (frame_mode IN ('foreground', 'full'));
