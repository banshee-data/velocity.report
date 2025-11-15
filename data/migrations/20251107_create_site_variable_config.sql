-- Migration: Create site_variable_config table and refactor cosine_error_angle
-- Date: 2025-11-07
-- Description: Move time-varying configuration (cosine_error_angle) from site table
-- to a separate site_variable_config table. Multiple site_config_periods can reference
-- the same site_variable_config (many-to-one relationship), allowing configuration reuse.

-- Create site_variable_config table to hold time-varying configuration
-- Multiple periods can reference the same config
CREATE TABLE IF NOT EXISTS site_variable_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cosine_error_angle REAL NOT NULL,
    created_at DOUBLE NOT NULL DEFAULT (UNIXEPOCH('subsec')),
    updated_at DOUBLE NOT NULL DEFAULT (UNIXEPOCH('subsec'))
);

-- Trigger to update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_site_variable_config_timestamp 
AFTER UPDATE ON site_variable_config 
BEGIN
    UPDATE site_variable_config
    SET updated_at = UNIXEPOCH('subsec')
    WHERE id = NEW.id;
END;

-- Add site_variable_config_id column to site_config_periods
-- Many periods can point to one config
ALTER TABLE site_config_periods ADD COLUMN site_variable_config_id INTEGER REFERENCES site_variable_config(id);

-- Create index for efficient lookups
CREATE INDEX IF NOT EXISTS idx_site_config_periods_variable_config 
    ON site_config_periods (site_variable_config_id);

-- Create default site_variable_config with 0.5 degrees (from original default site)
INSERT INTO site_variable_config (cosine_error_angle) VALUES (0.5);

-- Update existing default period to reference the default config
UPDATE site_config_periods 
SET site_variable_config_id = (SELECT id FROM site_variable_config WHERE cosine_error_angle = 0.5 LIMIT 1)
WHERE site_id = 1 AND effective_start_unix = 0.0;

