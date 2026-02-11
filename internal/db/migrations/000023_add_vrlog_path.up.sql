-- Migration 000023: Add vrlog_path column for VRLOG replay support
-- Phase 1.2 of track-labeling-ui-plan.md

ALTER TABLE lidar_analysis_runs ADD COLUMN vrlog_path TEXT;
