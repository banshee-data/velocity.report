-- Rollback: Remove sites
-- WARNING: This discards every site's canonical pose and label. They cannot
-- be recovered from the remaining columns — a case's own origin_lat/lon is
-- the sensor's pose on one visit, not the site's canonical pose.
     DROP INDEX IF EXISTS idx_lidar_replay_cases_site_id;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN site_id;

     DROP INDEX IF EXISTS idx_lidar_sites_l10;

     DROP INDEX IF EXISTS idx_lidar_sites_l13;

     DROP TABLE IF EXISTS lidar_sites;
