-- Reverse migration: Remove map-related fields from site table
-- Note: SQLite doesn't support DROP COLUMN directly in older versions,
-- so we would need to recreate the table. For simplicity, we'll use
-- the newer SQLite 3.35.0+ syntax which supports DROP COLUMN.

ALTER TABLE site DROP COLUMN bbox_ne_lat;
ALTER TABLE site DROP COLUMN bbox_ne_lng;
ALTER TABLE site DROP COLUMN bbox_sw_lat;
ALTER TABLE site DROP COLUMN bbox_sw_lng;
ALTER TABLE site DROP COLUMN map_rotation;
ALTER TABLE site DROP COLUMN map_svg_data;
