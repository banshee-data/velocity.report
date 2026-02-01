-- Add map-related fields to site table for storing bounding box coordinates,
-- map rotation, and SVG map data

-- Add bounding box coordinates (northeast and southwest corners)
ALTER TABLE site ADD COLUMN bbox_ne_lat REAL;
ALTER TABLE site ADD COLUMN bbox_ne_lng REAL;
ALTER TABLE site ADD COLUMN bbox_sw_lat REAL;
ALTER TABLE site ADD COLUMN bbox_sw_lng REAL;

-- Add map rotation (in degrees)
ALTER TABLE site ADD COLUMN map_rotation REAL DEFAULT 0;

-- Add map SVG data storage (BLOB for SVG content)
ALTER TABLE site ADD COLUMN map_svg_data BLOB;
