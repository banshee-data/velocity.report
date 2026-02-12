-- Add label_source column to lidar_run_tracks for provenance tracking.
-- Values: 'human_manual', 'carried_over', 'auto_suggested'
ALTER TABLE lidar_run_tracks ADD COLUMN label_source TEXT;
