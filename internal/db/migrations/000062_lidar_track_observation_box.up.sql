-- Migration: the published box on each track observation
-- Date: 2026-10-17
-- Description: The tracker's published box (the box model's, in the defaults
-- since #765) is recorded on every observation so the web app's live view
-- draws the box the visualiser and the scene assets draw: its own centre and
-- heading and extents, which hold every return. NULL when the track had no
-- published box (the box model off or not yet seeded) and on rows written
-- before this migration; a viewer then draws the legacy bounding box at the
-- tracked position.
    ALTER TABLE lidar_track_observations
      ADD COLUMN box_x REAL;

    ALTER TABLE lidar_track_observations
      ADD COLUMN box_y REAL;

    ALTER TABLE lidar_track_observations
      ADD COLUMN box_heading_rad REAL;

    ALTER TABLE lidar_track_observations
      ADD COLUMN box_length REAL;

    ALTER TABLE lidar_track_observations
      ADD COLUMN box_width REAL;
