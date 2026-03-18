-- lidar_tracks: restore percentile columns
ALTER TABLE lidar_tracks ADD COLUMN p50_speed_mps REAL;
ALTER TABLE lidar_tracks ADD COLUMN p85_speed_mps REAL;
ALTER TABLE lidar_tracks ADD COLUMN p95_speed_mps REAL;

-- lidar_tracks: restore peak name
ALTER TABLE lidar_tracks RENAME COLUMN max_speed_mps TO peak_speed_mps;

-- lidar_run_tracks: restore percentile columns
ALTER TABLE lidar_run_tracks ADD COLUMN p50_speed_mps REAL;
ALTER TABLE lidar_run_tracks ADD COLUMN p85_speed_mps REAL;
ALTER TABLE lidar_run_tracks ADD COLUMN p95_speed_mps REAL;

-- lidar_run_tracks: restore peak name
ALTER TABLE lidar_run_tracks RENAME COLUMN max_speed_mps TO peak_speed_mps;

-- restore world_frame
ALTER TABLE lidar_clusters RENAME COLUMN frame_id TO world_frame;
ALTER TABLE lidar_tracks RENAME COLUMN frame_id TO world_frame;
ALTER TABLE lidar_track_obs RENAME COLUMN frame_id TO world_frame;

-- restore scene_hash
ALTER TABLE lidar_bg_regions RENAME COLUMN grid_hash TO scene_hash;
DROP INDEX IF EXISTS idx_bg_regions_grid_hash;
CREATE INDEX idx_bg_regions_scene_hash ON lidar_bg_regions (scene_hash);
