-- Rollback: lidar_track_observations keyed by track and acquisition time
-- The table is rebuilt with the old key, (track_id, ts_unix_nanos), and
-- frame_unix_nanos nullable again; rows keep the frame time this migration
-- filed. The trigger goes with the old table.
--
-- Two frames of a track whose clusters share an acquisition time, which the
-- new key keeps as two rows, cannot both be copied back. The later frame is
-- kept, as INSERT OR REPLACE kept the later write. Each earlier one is kept
-- whole in lidar_migration_rejects under migration 63 with its own reason
-- rather than dropped, and migrating up again puts it back. This happens
-- only if two frames of a track shared an acquisition time again after the
-- migration. Rows the up migration set aside are put back here.
PRAGMA foreign_keys = OFF;

   CREATE TABLE lidar_track_observations_new (
          track_id TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , frame_id TEXT NOT NULL
        , x REAL
        , y REAL
        , z REAL
        , velocity_x REAL
        , velocity_y REAL
        , speed_mps REAL
        , heading_rad REAL
        , bounding_box_length REAL
        , bounding_box_width REAL
        , bounding_box_height REAL
        , height_p95 REAL
        , intensity_mean REAL
        , frame_unix_nanos INTEGER
        , measurement_source TEXT NOT NULL DEFAULT 'legacy_centroid_v0'
        , box_x REAL
        , box_y REAL
        , box_heading_rad REAL
        , box_length REAL
        , box_width REAL
        , PRIMARY KEY (track_id, ts_unix_nanos)
        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id) ON DELETE CASCADE
          );

-- Each row's rank among the rows of its track at its acquisition time: rank
-- 1 is the latest frame, the one copied.
   CREATE TEMP TABLE lidar_track_observation_frames AS
   SELECT rowid AS old_rowid
        , ROW_NUMBER() OVER (
          PARTITION BY track_id
                  , ts_unix_nanos
           ORDER BY frame_unix_nanos DESC
          ) AS frame_rank
     FROM lidar_track_observations;

   INSERT INTO lidar_migration_rejects (migration, source_table, source_key, reason, row_json, rejected_at_ns)
   SELECT 63
        , 'lidar_track_observations'
        , o.track_id || '@' || o.frame_unix_nanos
        , 'another frame of the track has the same acquisition time and is later'
        , JSON_OBJECT(
          'track_id'
        , o.track_id
        , 'ts_unix_nanos'
        , o.ts_unix_nanos
        , 'frame_id'
        , o.frame_id
        , 'x'
        , o.x
        , 'y'
        , o.y
        , 'z'
        , o.z
        , 'velocity_x'
        , o.velocity_x
        , 'velocity_y'
        , o.velocity_y
        , 'speed_mps'
        , o.speed_mps
        , 'heading_rad'
        , o.heading_rad
        , 'bounding_box_length'
        , o.bounding_box_length
        , 'bounding_box_width'
        , o.bounding_box_width
        , 'bounding_box_height'
        , o.bounding_box_height
        , 'height_p95'
        , o.height_p95
        , 'intensity_mean'
        , o.intensity_mean
        , 'frame_unix_nanos'
        , o.frame_unix_nanos
        , 'measurement_source'
        , o.measurement_source
        , 'box_x'
        , o.box_x
        , 'box_y'
        , o.box_y
        , 'box_heading_rad'
        , o.box_heading_rad
        , 'box_length'
        , o.box_length
        , 'box_width'
        , o.box_width
          )
        , CAST(STRFTIME('%s', 'now') AS INTEGER) * 1000000000
     FROM lidar_track_observations o
     JOIN lidar_track_observation_frames f ON f.old_rowid = o.rowid
    WHERE f.frame_rank > 1;

   INSERT INTO lidar_track_observations_new (
          track_id
        , ts_unix_nanos
        , frame_id
        , x
        , y
        , z
        , velocity_x
        , velocity_y
        , speed_mps
        , heading_rad
        , bounding_box_length
        , bounding_box_width
        , bounding_box_height
        , height_p95
        , intensity_mean
        , frame_unix_nanos
        , measurement_source
        , box_x
        , box_y
        , box_heading_rad
        , box_length
        , box_width
          )
   SELECT o.track_id
        , o.ts_unix_nanos
        , o.frame_id
        , o.x
        , o.y
        , o.z
        , o.velocity_x
        , o.velocity_y
        , o.speed_mps
        , o.heading_rad
        , o.bounding_box_length
        , o.bounding_box_width
        , o.bounding_box_height
        , o.height_p95
        , o.intensity_mean
        , o.frame_unix_nanos
        , o.measurement_source
        , o.box_x
        , o.box_y
        , o.box_heading_rad
        , o.box_length
        , o.box_width
     FROM lidar_track_observations o
     JOIN lidar_track_observation_frames f ON f.old_rowid = o.rowid
    WHERE f.frame_rank = 1;

-- Rows this migration set aside on the way up fit the old key again, and
-- are put back unless a row of their track at their acquisition time was
-- filed since.
   INSERT OR IGNORE INTO lidar_track_observations_new (
          track_id
        , ts_unix_nanos
        , frame_id
        , x
        , y
        , z
        , velocity_x
        , velocity_y
        , speed_mps
        , heading_rad
        , bounding_box_length
        , bounding_box_width
        , bounding_box_height
        , height_p95
        , intensity_mean
        , frame_unix_nanos
        , measurement_source
        , box_x
        , box_y
        , box_heading_rad
        , box_length
        , box_width
          )
   SELECT JSON_EXTRACT(row_json, '$.track_id')
        , JSON_EXTRACT(row_json, '$.ts_unix_nanos')
        , JSON_EXTRACT(row_json, '$.frame_id')
        , JSON_EXTRACT(row_json, '$.x')
        , JSON_EXTRACT(row_json, '$.y')
        , JSON_EXTRACT(row_json, '$.z')
        , JSON_EXTRACT(row_json, '$.velocity_x')
        , JSON_EXTRACT(row_json, '$.velocity_y')
        , JSON_EXTRACT(row_json, '$.speed_mps')
        , JSON_EXTRACT(row_json, '$.heading_rad')
        , JSON_EXTRACT(row_json, '$.bounding_box_length')
        , JSON_EXTRACT(row_json, '$.bounding_box_width')
        , JSON_EXTRACT(row_json, '$.bounding_box_height')
        , JSON_EXTRACT(row_json, '$.height_p95')
        , JSON_EXTRACT(row_json, '$.intensity_mean')
        , JSON_EXTRACT(row_json, '$.frame_unix_nanos')
        , JSON_EXTRACT(row_json, '$.measurement_source')
        , JSON_EXTRACT(row_json, '$.box_x')
        , JSON_EXTRACT(row_json, '$.box_y')
        , JSON_EXTRACT(row_json, '$.box_heading_rad')
        , JSON_EXTRACT(row_json, '$.box_length')
        , JSON_EXTRACT(row_json, '$.box_width')
     FROM lidar_migration_rejects
    WHERE migration = 63
      AND source_table = 'lidar_track_observations'
      AND reason = 'another row of the track at the same frame has a later acquisition time';

   DELETE FROM lidar_migration_rejects
    WHERE migration = 63
      AND source_table = 'lidar_track_observations'
      AND reason = 'another row of the track at the same frame has a later acquisition time'
      AND EXISTS (
             SELECT 1
               FROM lidar_track_observations_new n
              WHERE n.track_id = JSON_EXTRACT(row_json, '$.track_id')
                AND n.ts_unix_nanos = JSON_EXTRACT(row_json, '$.ts_unix_nanos')
                AND n.frame_unix_nanos IS JSON_EXTRACT(row_json, '$.frame_unix_nanos')
          );

     DROP TABLE lidar_track_observation_frames;

     DROP TABLE lidar_track_observations;

    ALTER TABLE lidar_track_observations_new
RENAME TO lidar_track_observations;

CREATE INDEX idx_lidar_track_observations_track ON lidar_track_observations (track_id);

CREATE INDEX idx_lidar_track_observations_time ON lidar_track_observations (ts_unix_nanos);

PRAGMA foreign_keys = ON;
