-- Add map-related fields to site table for storing bounding box coordinates and SVG map data
-- Note: Radar location (latitude, longitude) and radar angle (map_angle) already exist in the site table

-- Add bounding box coordinates (northeast and southwest corners)
-- These define the area shown in the map for PDF reports
ALTER TABLE site ADD COLUMN bbox_ne_lat REAL;
ALTER TABLE site ADD COLUMN bbox_ne_lng REAL;
ALTER TABLE site ADD COLUMN bbox_sw_lat REAL;
ALTER TABLE site ADD COLUMN bbox_sw_lng REAL;

-- Add map SVG data storage (BLOB for base64-encoded SVG content)
ALTER TABLE site ADD COLUMN map_svg_data BLOB;
