DROP INDEX IF EXISTS idx_lidar_run_tracks_quality_label;
-- SQLite does not support DROP COLUMN before 3.35.0; recreating the table
-- is complex and this column is additive, so we leave it in place.
