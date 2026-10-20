-- Migration: lidar_track_observations keyed by track and frame
-- Date: 2026-10-20
-- Description: An observation was keyed by track and acquisition time
-- (ts_unix_nanos), and the server writes it with INSERT OR REPLACE. The
-- acquisition time is the cluster's first point's timestamp. When two
-- consecutive frames' clusters of one track both start with returns from the
-- laser firing at the frame seam, they share that time, and the later frame's
-- row silently replaced the earlier one: 1 row in 23,667 at one S2 site, and
-- one row in 4 of 69 corpus cases, measured 2026-10-20. The live server was
-- affected as well as replays.
--
-- The key is now (track_id, frame_unix_nanos). The tracker files at most one
-- observation of a track per frame, so the frame is what the row stands for.
-- frame_unix_nanos becomes NOT NULL. Rows written before it existed are filed
-- at their acquisition time, the value every reader already fell back to.
-- ts_unix_nanos keeps the acquisition time, and its index stays for the time
-- window queries.
--
-- SQLite cannot change a primary key in place, so the table is rebuilt:
-- create the new table, copy every row, drop the old one, rename the new one
-- and recreate both indexes. Two old rows of a track that would share a
-- frame cannot both be copied. The latest acquisition is kept, as INSERT OR
-- REPLACE kept the latest write, and each other row is kept whole in
-- lidar_migration_rejects, never dropped; the down migration puts it back.
-- The live writer files one row per track per frame, so none is expected.
--
-- A writer that names no frame has its row filed at its acquisition time by
-- a trigger, as this migration files the old rows, rather than refused.
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
        , frame_unix_nanos INTEGER NOT NULL
        , measurement_source TEXT NOT NULL DEFAULT 'legacy_centroid_v0'
        , box_x REAL
        , box_y REAL
        , box_heading_rad REAL
        , box_length REAL
        , box_width REAL
        , PRIMARY KEY (track_id, frame_unix_nanos)
        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id) ON DELETE CASCADE
          );

-- Each old row's frame, and its rank among the rows of its track at that
-- frame: rank 1 is the latest acquisition, the one copied.
   CREATE TEMP TABLE lidar_track_observation_frames AS
   SELECT rowid AS old_rowid
        , COALESCE(frame_unix_nanos, ts_unix_nanos) AS frame_key
        , ROW_NUMBER() OVER (
          PARTITION BY track_id
                  , COALESCE(frame_unix_nanos, ts_unix_nanos)
           ORDER BY ts_unix_nanos DESC
          ) AS frame_rank
     FROM lidar_track_observations;

   INSERT INTO lidar_migration_rejects (migration, source_table, source_key, reason, row_json, rejected_at_ns)
   SELECT 63
        , 'lidar_track_observations'
        , o.track_id || '@' || o.ts_unix_nanos
        , 'another row of the track at the same frame has a later acquisition time'
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
        , f.frame_key
        , o.measurement_source
        , o.box_x
        , o.box_y
        , o.box_heading_rad
        , o.box_length
        , o.box_width
     FROM lidar_track_observations o
     JOIN lidar_track_observation_frames f ON f.old_rowid = o.rowid
    WHERE f.frame_rank = 1;

-- Rows a rollback of this migration set aside fit this key again, and are
-- put back unless a row of their track at their frame was filed since.
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
        , COALESCE(
          JSON_EXTRACT(row_json, '$.frame_unix_nanos')
        , JSON_EXTRACT(row_json, '$.ts_unix_nanos')
          )
        , JSON_EXTRACT(row_json, '$.measurement_source')
        , JSON_EXTRACT(row_json, '$.box_x')
        , JSON_EXTRACT(row_json, '$.box_y')
        , JSON_EXTRACT(row_json, '$.box_heading_rad')
        , JSON_EXTRACT(row_json, '$.box_length')
        , JSON_EXTRACT(row_json, '$.box_width')
     FROM lidar_migration_rejects
    WHERE migration = 63
      AND source_table = 'lidar_track_observations'
      AND reason = 'another frame of the track has the same acquisition time and is later';

   DELETE FROM lidar_migration_rejects
    WHERE migration = 63
      AND source_table = 'lidar_track_observations'
      AND reason = 'another frame of the track has the same acquisition time and is later'
      AND EXISTS (
             SELECT 1
               FROM lidar_track_observations_new n
              WHERE n.track_id = JSON_EXTRACT(row_json, '$.track_id')
                AND n.ts_unix_nanos = JSON_EXTRACT(row_json, '$.ts_unix_nanos')
                AND n.frame_unix_nanos IS COALESCE(
                    JSON_EXTRACT(row_json, '$.frame_unix_nanos')
                  , JSON_EXTRACT(row_json, '$.ts_unix_nanos')
                    )
          );

     DROP TABLE lidar_track_observation_frames;

     DROP TABLE lidar_track_observations;

    ALTER TABLE lidar_track_observations_new
RENAME TO lidar_track_observations;

CREATE INDEX idx_lidar_track_observations_track ON lidar_track_observations (track_id);

CREATE INDEX idx_lidar_track_observations_time ON lidar_track_observations (ts_unix_nanos);

-- RAISE (IGNORE) drops the insert that named no frame after the trigger has
-- filed the same row at its acquisition time. The outer statement's conflict
-- clause applies to the trigger's insert. The trigger names every column, and
-- TestTrackObservationFrameTriggerNamesEveryColumn fails when a column is
-- added without it.
CREATE TRIGGER lidar_track_observations_frame_from_acquisition BEFORE INSERT ON lidar_track_observations WHEN NEW.frame_unix_nanos IS NULL BEGIN
   INSERT INTO lidar_track_observations (
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
   VALUES (
          NEW.track_id
        , NEW.ts_unix_nanos
        , NEW.frame_id
        , NEW.x
        , NEW.y
        , NEW.z
        , NEW.velocity_x
        , NEW.velocity_y
        , NEW.speed_mps
        , NEW.heading_rad
        , NEW.bounding_box_length
        , NEW.bounding_box_width
        , NEW.bounding_box_height
        , NEW.height_p95
        , NEW.intensity_mean
        , NEW.ts_unix_nanos
        , NEW.measurement_source
        , NEW.box_x
        , NEW.box_y
        , NEW.box_heading_rad
        , NEW.box_length
        , NEW.box_width
          );

   SELECT RAISE (IGNORE);

END;

PRAGMA foreign_keys = ON;
