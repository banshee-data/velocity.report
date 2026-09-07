-- Reverse: drop the published-scene index.
     DROP TRIGGER IF EXISTS update_lidar_scenes_timestamp;

     DROP INDEX IF EXISTS idx_lidar_scenes_published;

     DROP INDEX IF EXISTS idx_lidar_scenes_captured;

     DROP INDEX IF EXISTS idx_lidar_scenes_site;

     DROP TABLE IF EXISTS lidar_scenes;
