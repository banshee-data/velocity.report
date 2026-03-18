-- lidar_tracks: drop dead percentile columns
ALTER TABLE lidar_tracks DROP COLUMN p50_speed_mps;
ALTER TABLE lidar_tracks DROP COLUMN p85_speed_mps;
ALTER TABLE lidar_tracks DROP COLUMN p95_speed_mps;

-- lidar_tracks: rename peak → max
ALTER TABLE lidar_tracks RENAME COLUMN peak_speed_mps TO max_speed_mps;

-- lidar_run_tracks: drop percentile columns
ALTER TABLE lidar_run_tracks DROP COLUMN p50_speed_mps;
ALTER TABLE lidar_run_tracks DROP COLUMN p85_speed_mps;
ALTER TABLE lidar_run_tracks DROP COLUMN p95_speed_mps;

-- lidar_run_tracks: rename peak → max
ALTER TABLE lidar_run_tracks RENAME COLUMN peak_speed_mps TO max_speed_mps;

-- rename world_frame → frame_id on three tables
ALTER TABLE lidar_clusters RENAME COLUMN world_frame TO frame_id;
ALTER TABLE lidar_tracks RENAME COLUMN world_frame TO frame_id;
ALTER TABLE lidar_track_obs RENAME COLUMN world_frame TO frame_id;

-- rename scene_hash → grid_hash on lidar_bg_regions
ALTER TABLE lidar_bg_regions RENAME COLUMN scene_hash TO grid_hash;
DROP INDEX IF EXISTS idx_bg_regions_scene_hash;
CREATE INDEX idx_bg_regions_grid_hash ON lidar_bg_regions (grid_hash);
