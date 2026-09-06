-- Restore the scene vantage column.
-- Any values it held were lost when it was dropped; a scene's vantages are
-- read from its vantages.json.
    ALTER TABLE lidar_scenes
      ADD COLUMN vantages_json TEXT;
