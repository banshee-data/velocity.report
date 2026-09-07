-- Migration: Remove the scene vantage column
-- Date: 2026-09-06
-- Description: Vantages now live in exactly one place, a vantages.json at the
-- scene root beside manifest.json, which is what the published viewer fetches.
-- This column was a second copy that no published page ever read, so an edit
-- made here changed nothing a visitor could see while looking authoritative.
    ALTER TABLE lidar_scenes
     DROP COLUMN vantages_json;
