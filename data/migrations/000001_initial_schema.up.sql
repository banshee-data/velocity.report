-- Migration: Create initial database schema
-- Date: 2025-08-25
-- Description: Initial schema with 4 core tables: radar_data, radar_objects, radar_commands, radar_command_log
-- This represents the baseline schema before any migrations
-- Note: PRAGMA statements are executed by the application, not in migrations

CREATE TABLE IF NOT EXISTS radar_data (
    write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec')),
    raw_event JSON NOT NULL,
    uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED,
    magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED,
    speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
);

CREATE TABLE IF NOT EXISTS radar_objects (
    write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec')),
    raw_event JSON NOT NULL,
    classifier TEXT NOT NULL AS (JSON_EXTRACT(raw_event, '$.classifier')) STORED,
    start_time DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.start_time')) STORED,
    end_time DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.end_time')) STORED,
    delta_time_ms BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.delta_time_msec')) STORED,
    max_speed DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.max_speed_mps')) STORED,
    min_speed DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.min_speed_mps')) STORED,
    speed_change DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.speed_change')) STORED,
    max_magnitude BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.max_magnitude')) STORED,
    avg_magnitude BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.avg_magnitude')) STORED,
    total_frames BIGINT NOT NULL AS (JSON_EXTRACT(raw_event, '$.total_frames')) STORED,
    frames_per_mps DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.frames_per_mps')) STORED,
    length_m DOUBLE NOT NULL AS (JSON_EXTRACT(raw_event, '$.length_m')) STORED
);

CREATE TABLE IF NOT EXISTS radar_commands (
    command_id BIGINT PRIMARY KEY,
    command TEXT,
    write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
);

CREATE TABLE IF NOT EXISTS radar_command_log (
    log_id BIGINT PRIMARY KEY,
    command_id BIGINT,
    log_data TEXT,
    write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec')),
    FOREIGN KEY (command_id) REFERENCES radar_commands (command_id)
);
