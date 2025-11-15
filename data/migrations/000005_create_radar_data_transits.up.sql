-- Migration: Create radar_data_transits table
-- Date: 2025-11-15
-- Description: Add persisted sessionization table for radar_data transits

CREATE TABLE IF NOT EXISTS radar_data_transits (
    transit_id INTEGER PRIMARY KEY AUTOINCREMENT,
    transit_key TEXT NOT NULL UNIQUE,
    threshold_ms INTEGER NOT NULL,
    transit_start_unix DOUBLE NOT NULL,
    transit_end_unix DOUBLE NOT NULL,
    transit_max_speed DOUBLE NOT NULL,
    transit_min_speed DOUBLE,
    transit_max_magnitude BIGINT,
    transit_min_magnitude BIGINT,
    point_count INTEGER NOT NULL,
    model_version TEXT,
    created_at DOUBLE DEFAULT (UNIXEPOCH('subsec')),
    updated_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
);

CREATE INDEX IF NOT EXISTS idx_transits_time ON radar_data_transits (transit_start_unix, transit_end_unix);
