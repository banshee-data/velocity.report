-- Migration: Create lidar_scenes, the index of published LiDAR scenes
-- Date: 2026-09-05
-- Description: One row per publishable scene, carrying the geographic and
-- temporal metadata a catalogue needs plus the provenance tying a published
-- asset to its recording. NULL latitude/longitude means inherit from site_id.
-- Capture times are Unix nanoseconds to match the recording; the API exposes
-- them as RFC 3339 for editing.
   CREATE TABLE IF NOT EXISTS lidar_scenes (
          scene_id TEXT PRIMARY KEY
        , site_id INTEGER
        , title TEXT NOT NULL
        , description TEXT
        , latitude REAL
        , longitude REAL
        , captured_start_ns INTEGER
        , captured_end_ns INTEGER
        , duration_secs REAL
        , source_capture TEXT
        , source_vrlog_sha256 TEXT
        , frame_count INTEGER
        , frame_stride INTEGER
        , asset_path TEXT
        , published INTEGER NOT NULL DEFAULT 0
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , CHECK (published IN (0, 1))
        , CHECK (
          latitude IS NULL
       OR (latitude BETWEEN -90 AND 90)
          )
        , CHECK (
          longitude IS NULL
       OR (longitude BETWEEN -180 AND 180)
          )
        , CHECK (
          captured_start_ns IS NULL
       OR captured_end_ns IS NULL
       OR captured_end_ns >= captured_start_ns
          )
        , FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE SET NULL
          );

CREATE INDEX IF NOT EXISTS idx_lidar_scenes_site ON lidar_scenes (site_id);

CREATE INDEX IF NOT EXISTS idx_lidar_scenes_captured ON lidar_scenes (captured_start_ns);

CREATE INDEX IF NOT EXISTS idx_lidar_scenes_published ON lidar_scenes (published);

CREATE TRIGGER IF NOT EXISTS update_lidar_scenes_timestamp AFTER
   UPDATE ON lidar_scenes BEGIN
   UPDATE lidar_scenes
      SET updated_at = STRFTIME('%s', 'now')
    WHERE scene_id = NEW.scene_id;

END;
