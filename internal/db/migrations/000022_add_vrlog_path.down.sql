-- Migration 000022: Remove vrlog_path column
-- Rollback for Phase 1.2 of track-labeling-ui-plan.md

-- This project uses modernc.org/sqlite v1.44.3 (SQLite 3.45+), which fully
-- supports ALTER TABLE DROP COLUMN (available since SQLite 3.35.0).
-- If this migration is ever applied with an older SQLite runtime, the table
-- would need to be recreated manually to remove the vrlog_path column.
ALTER TABLE lidar_analysis_runs DROP COLUMN vrlog_path;
