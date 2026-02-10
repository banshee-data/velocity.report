-- Add scene_id and source_file columns to lidar_labels table
-- scene_id links a label to a specific scene for context
-- source_file records which PCAP file was being annotated
ALTER TABLE lidar_labels ADD COLUMN scene_id TEXT;
ALTER TABLE lidar_labels ADD COLUMN source_file TEXT;

CREATE INDEX IF NOT EXISTS idx_lidar_labels_scene ON lidar_labels(scene_id);
