-- Migration: convert legacy `data` table -> new `radar_data` schema
-- Filename: data/migrations/20250929_migrate_data_to_radar_data.sql
-- Usage:
--   cd <repo-root>
--   cp sensor_data.db sensor_data.db.bak
--   sqlite3 sensor_data.db < data/migrations/20250929_migrate_data_to_radar_data.sql
--
-- Notes:
-- * This migration assumes your legacy table is named `data` and that it
--   contains at least a `raw_event` (JSON) column and/or a `write_timestamp`
--   or `timestamp` column. The script will convert textual timestamps to
--   unix seconds where needed using `strftime('%s', ...)`.
-- * Always back up your database before running migrations.
-- * This SQL is written to be simple and explicit rather than fully
--   defensive; if your DB doesn't contain a `data` table you will get
--   errors. Inspect the file and adapt it to your environment if needed.
BEGIN TRANSACTION;

-- Create the target table (new schema) as radar_data_new
   CREATE TABLE IF NOT EXISTS radar_data_new (
          write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , raw_event JSON NOT NULL
        , uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED
        , magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED
        , speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
          );

-- IMPORTANT: this INSERT selects from the legacy `data` table. If your
-- legacy schema uses different column names adjust the SELECT below.
-- Typical legacy columns expected: write_timestamp (text or numeric), raw_event.
-- If `write_timestamp` is textual (ISO timestamp), we convert it to unix
-- seconds using strftime('%s', ...). If missing, we fall back to
-- UNIXEPOCH('subsec') default.
-- The legacy `data` table has columns: timestamp, uptime, magnitude, speed.
-- Build a JSON payload from these columns and convert textual timestamps
-- into unix seconds for write_timestamp.
   INSERT INTO radar_data_new (write_timestamp, raw_event)
   SELECT CASE
                    WHEN (TYPEOF(timestamp) = 'text') THEN CAST(STRFTIME('%s', timestamp) AS DOUBLE)
                    WHEN (timestamp IS NULL) THEN UNIXEPOCH('subsec')
                    ELSE timestamp
          END AS write_timestamp
        , JSON_OBJECT(
          'timestamp'
        , COALESCE(timestamp, '')
        , 'uptime'
        , uptime
        , 'magnitude'
        , magnitude
        , 'speed'
        , speed
          ) AS raw_event
     FROM radar_data;

-- Swap tables: keep an easy-to-remove backup as radar_data_old
    ALTER TABLE radar_data
RENAME TO radar_data_old;

    ALTER TABLE radar_data_new
RENAME TO radar_data;

-- Drop the legacy table backup after human verification (kept here for safety)
-- DROP TABLE radar_data_old;
COMMIT;

-- End migration
