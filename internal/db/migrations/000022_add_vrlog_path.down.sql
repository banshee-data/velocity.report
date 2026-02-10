-- Migration 000022: Remove vrlog_path column
-- Rollback for Phase 1.2 of track-labeling-ui-plan.md

-- SQLite doesn't support DROP COLUMN directly, need to recreate table
-- For simplicity, just mark the column as unused (SQLite will handle it)
-- In production, a full table recreation would be needed

-- Note: SQLite 3.35.0+ supports ALTER TABLE DROP COLUMN
-- For older SQLite versions, this migration may need adjustment
ALTER TABLE lidar_analysis_runs DROP COLUMN vrlog_path;
