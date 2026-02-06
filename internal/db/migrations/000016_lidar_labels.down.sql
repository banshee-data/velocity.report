-- Rollback lidar_labels table

DROP INDEX IF EXISTS idx_lidar_labels_class;
DROP INDEX IF EXISTS idx_lidar_labels_time;
DROP INDEX IF EXISTS idx_lidar_labels_track;
DROP TABLE IF EXISTS lidar_labels;
