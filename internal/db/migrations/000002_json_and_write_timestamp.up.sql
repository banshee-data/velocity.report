-- Migration: Migrate from timestamp to write_timestamp and add JSON storage
-- Date: 2024 (commit 57182957)
-- Description: Transforms the original schema with timestamp column to use:
--   1. write_timestamp (DOUBLE with unix epoch) instead of timestamp (TIMESTAMP)
--   2. raw_event JSON column for storing raw data
--   3. Generated columns for extracting values from JSON
--
-- This migration transforms existing data to the new format.
-- Migrate data table
    ALTER TABLE data
RENAME TO data_old;

   CREATE TABLE data (
          write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , raw_event JSON NOT NULL
        , uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED
        , magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED
        , speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
          );

-- Migrate existing data: convert timestamp to write_timestamp and create JSON
   INSERT INTO data (write_timestamp, raw_event)
   SELECT CAST(STRFTIME('%s', timestamp) AS DOUBLE) + (
          CAST(STRFTIME('%f', timestamp) AS DOUBLE) - CAST(STRFTIME('%S', timestamp) AS DOUBLE)
          )
        , JSON_OBJECT('uptime', uptime, 'magnitude', magnitude, 'speed', speed)
     FROM data_old;

     DROP TABLE data_old;

-- Create radar_objects table (new in this migration)
   CREATE TABLE IF NOT EXISTS radar_objects (
          write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , raw_event JSON NOT NULL
        , classifier TEXT NOT NULL AS (JSON_EXTRACT(raw_event, '$.classifier')) STORED
        , start_time DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.start_time')) STORED
        , end_time DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.end_time')) STORED
        , delta_time_ms BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.delta_time_msec')) STORED
        , max_speed DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.max_speed_mps')) STORED
        , min_speed DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.min_speed_mps')) STORED
        , speed_change DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.speed_change')) STORED
        , max_magnitude BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.max_magnitude')) STORED
        , avg_magnitude BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.avg_magnitude')) STORED
        , total_frames BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.total_frames')) STORED
        , frames_per_mps DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.frames_per_mps')) STORED
        , LENGTH DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.length_m')) STORED
          );

-- Migrate commands table
    ALTER TABLE commands
RENAME TO commands_old;

   CREATE TABLE commands (
          command_id BIGINT PRIMARY KEY
        , command TEXT
        , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
          );

   INSERT INTO commands (command_id, command, write_timestamp)
   SELECT command_id
        , command
        , CAST(STRFTIME('%s', timestamp) AS DOUBLE) + (
          CAST(STRFTIME('%f', timestamp) AS DOUBLE) - CAST(STRFTIME('%S', timestamp) AS DOUBLE)
          )
     FROM commands_old;

     DROP TABLE commands_old;

-- Migrate log table
    ALTER TABLE log
RENAME TO log_old;

   CREATE TABLE log(
          log_id BIGINT PRIMARY KEY
        , command_id BIGINT
        , log_data TEXT
        , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , FOREIGN KEY (command_id) REFERENCES commands (command_id)
          );

   INSERT INTO log(log_id, command_id, log_data, write_timestamp)
   SELECT log_id
        , command_id
        , log_data
        , CAST(STRFTIME('%s', timestamp) AS DOUBLE) + (
          CAST(STRFTIME('%f', timestamp) AS DOUBLE) - CAST(STRFTIME('%S', timestamp) AS DOUBLE)
          )
     FROM log_old;

     DROP TABLE log_old;
