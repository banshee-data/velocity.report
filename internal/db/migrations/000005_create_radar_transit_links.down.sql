-- Rollback: Remove radar_transit_links table
     DROP INDEX IF EXISTS idx_transit_links_data;

     DROP INDEX IF EXISTS idx_transit_links_transit;

     DROP TABLE IF EXISTS radar_transit_links;
