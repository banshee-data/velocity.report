-- Migration: Convert legacy 'data' table to new 'radar_data' schema
-- Date: 2025-09-29
-- Description: Migrates legacy data table format to radar_data with JSON raw_event column

-- Create the target table (new schema) as radar_data_new
CREATE TABLE IF NOT EXISTS radar_data_new (
    write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec')),
    raw_event JSON NOT NULL,
    uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED,
    magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED,
    speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
);

-- Convert legacy data to new format with JSON raw_event
-- Handles both textual and numeric timestamps
INSERT INTO radar_data_new (write_timestamp, raw_event)
SELECT 
    CASE
        WHEN (TYPEOF(timestamp) = 'text') THEN CAST(STRFTIME('%s', timestamp) AS DOUBLE)
        WHEN (timestamp IS NULL) THEN UNIXEPOCH('subsec')
        ELSE timestamp
    END AS write_timestamp,
    JSON_OBJECT(
        'timestamp', COALESCE(timestamp, ''),
        'uptime', uptime,
        'magnitude', magnitude,
        'speed', speed
    ) AS raw_event
FROM radar_data;

-- Swap tables: keep backup as radar_data_old
ALTER TABLE radar_data RENAME TO radar_data_old;

ALTER TABLE radar_data_new RENAME TO radar_data;

-- Note: radar_data_old is kept for safety and can be manually dropped after verification
