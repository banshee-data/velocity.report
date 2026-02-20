-- Rollback: Drop radar_serial_config table
-- WARNING: This will delete all serial port configurations

-- Drop the trigger first
DROP TRIGGER IF EXISTS update_radar_serial_config_timestamp;

-- Drop the index
DROP INDEX IF EXISTS idx_radar_serial_config_enabled;

-- Drop the table
DROP TABLE IF EXISTS radar_serial_config;
