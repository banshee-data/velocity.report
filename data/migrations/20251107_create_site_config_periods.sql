-- Migration: Create site_config_periods table for Type 6 SCD
-- Date: 2025-11-07
-- Description: Add site_config_periods table to track time-based site configuration effective periods.
-- This is a Type 6 Slowly Changing Dimension that allows:
-- 1. Tracking which site configuration was active during different time periods
-- 2. Associating radar data with the correct site configuration based on timestamp
-- 3. Changing cosine angles after-the-fact without recomputing stored data
-- 4. Displaying unconfigured time periods in timeline views
-- 5. Supporting multiple site configurations within a single report period

CREATE TABLE IF NOT EXISTS site_config_periods (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL,
    effective_start_unix DOUBLE NOT NULL,
    effective_end_unix DOUBLE, -- NULL means currently active/open-ended
    is_active INTEGER NOT NULL DEFAULT 0, -- 1 if this is the current active period for new data
    notes TEXT, -- Optional notes about why configuration changed
    created_at DOUBLE NOT NULL DEFAULT (UNIXEPOCH('subsec')),
    updated_at DOUBLE NOT NULL DEFAULT (UNIXEPOCH('subsec')),
    FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE,
    -- Ensure no overlapping periods for the same site
    CHECK (effective_end_unix IS NULL OR effective_end_unix > effective_start_unix)
);

-- Index for efficient time-range queries (joining radar_data to site config periods)
CREATE INDEX IF NOT EXISTS idx_site_config_periods_time 
    ON site_config_periods (effective_start_unix, effective_end_unix);

-- Index for finding the active site period
CREATE INDEX IF NOT EXISTS idx_site_config_periods_active 
    ON site_config_periods (is_active) WHERE is_active = 1;

-- Index for site-specific queries
CREATE INDEX IF NOT EXISTS idx_site_config_periods_site 
    ON site_config_periods (site_id, effective_start_unix);

-- Trigger to update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_site_config_periods_timestamp 
AFTER UPDATE ON site_config_periods 
BEGIN
    UPDATE site_config_periods
    SET updated_at = UNIXEPOCH('subsec')
    WHERE id = NEW.id;
END;

-- Trigger to ensure only one period is marked as active at a time
CREATE TRIGGER IF NOT EXISTS enforce_single_active_period
BEFORE INSERT ON site_config_periods
WHEN NEW.is_active = 1
BEGIN
    UPDATE site_config_periods SET is_active = 0 WHERE is_active = 1;
END;

-- Trigger to ensure only one period is marked as active at a time (on update)
CREATE TRIGGER IF NOT EXISTS enforce_single_active_period_update
BEFORE UPDATE ON site_config_periods
WHEN NEW.is_active = 1
BEGIN
    UPDATE site_config_periods SET is_active = 0 WHERE is_active = 1 AND id != NEW.id;
END;

-- Create an initial active period for the default site (id=1) if it exists
-- This period starts from epoch 0 and is open-ended (NULL end date)
-- This ensures backward compatibility with existing radar data
INSERT OR IGNORE INTO site_config_periods (
    site_id,
    effective_start_unix,
    effective_end_unix,
    is_active,
    notes
)
SELECT 
    id,
    0.0, -- Start from epoch (will match all historical data)
    NULL, -- Open-ended (currently active)
    1, -- Mark as active
    'Initial default period created during migration'
FROM site
WHERE id = 1
LIMIT 1;
