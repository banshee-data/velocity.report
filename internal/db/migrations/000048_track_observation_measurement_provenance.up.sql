-- Migration: track-observation measurement provenance
-- Date: 2026-09-12
-- Description: legacy lidar_track_observations remains a tracker-output
-- table, but each new row must say which geometry entered the filter and
-- distinguish the L2 frame boundary from the cluster acquisition time.
    ALTER TABLE lidar_track_observations
      ADD COLUMN frame_unix_nanos INTEGER;

    ALTER TABLE lidar_track_observations
      ADD COLUMN measurement_source TEXT NOT NULL DEFAULT 'legacy_centroid_v0';
