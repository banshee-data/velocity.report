-- Rollback: Restore legacy 'data' table format from radar_data
-- WARNING: This migration cannot fully restore the original format if radar_data_old was dropped

-- Check if radar_data_old exists and restore from it if available
-- Otherwise, convert back from JSON format (with potential data loss)

CREATE TABLE IF NOT EXISTS radar_data_legacy (
    timestamp TEXT,
    uptime DOUBLE,
    magnitude DOUBLE,
    speed DOUBLE
);

-- Restore from radar_data by extracting values from JSON
INSERT INTO radar_data_legacy (timestamp, uptime, magnitude, speed)
SELECT 
    JSON_EXTRACT(raw_event, '$.timestamp') AS timestamp,
    JSON_EXTRACT(raw_event, '$.uptime') AS uptime,
    JSON_EXTRACT(raw_event, '$.magnitude') AS magnitude,
    JSON_EXTRACT(raw_event, '$.speed') AS speed
FROM radar_data;

-- Swap back to original name
ALTER TABLE radar_data RENAME TO radar_data_new;

ALTER TABLE radar_data_legacy RENAME TO radar_data;

-- Note: radar_data_new is kept for safety
-- If radar_data_old still exists, you may want to use it instead:
-- DROP TABLE radar_data;
-- ALTER TABLE radar_data_old RENAME TO radar_data;
-- DROP TABLE radar_data_new;
