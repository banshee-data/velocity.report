     DROP INDEX IF EXISTS idx_lidar_labels_scene;

-- NOTE: As of modernc.org/sqlite v1.44.3 (SQLite 3.51.2), ALTER TABLE DROP COLUMN
-- is now supported. Columns left in place as a design choice (additive, harmless).
-- New migrations needing column removal should use DROP COLUMN directly.
