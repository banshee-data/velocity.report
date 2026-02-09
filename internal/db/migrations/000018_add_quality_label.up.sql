-- Add quality_label column for track measurement quality assessment
-- Allowed values: perfect, good, truncated, noisy_velocity, stopped_recovered
ALTER TABLE lidar_run_tracks ADD COLUMN quality_label TEXT;

CREATE INDEX IF NOT EXISTS idx_lidar_run_tracks_quality_label ON lidar_run_tracks (quality_label);
