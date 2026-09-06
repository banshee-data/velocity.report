-- Migration: Add named viewpoints to lidar_scenes
-- Date: 2026-09-05
-- Description: Stores the vantage list a scene publishes, as a JSON array of
-- {id,label,azimuth_deg,polar_deg,zoom,offset_x,offset_y}. Geometry is relative
-- to the scene's own framing, so a saved viewpoint survives the export being
-- regenerated. NULL means the recording's own list, or compass defaults.
    ALTER TABLE lidar_scenes
      ADD COLUMN vantages_json TEXT;
