-- Migration: Create radar_serial_config table for storing serial port configurations
-- Date: 2025-11-06
-- Description: Add radar_serial_config table to support database-driven serial configuration with multi-sensor support

-- Serial port configurations table
CREATE TABLE IF NOT EXISTS radar_serial_config (
       id INTEGER PRIMARY KEY AUTOINCREMENT
     , name TEXT NOT NULL UNIQUE
     , port_path TEXT NOT NULL
     , baud_rate INTEGER NOT NULL DEFAULT 19200
     , data_bits INTEGER NOT NULL DEFAULT 8
     , stop_bits INTEGER NOT NULL DEFAULT 1
     , parity TEXT NOT NULL DEFAULT 'N'
     , enabled INTEGER NOT NULL DEFAULT 1
     , description TEXT
     , sensor_model TEXT NOT NULL DEFAULT 'ops243-a'
     , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
     , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
     , CHECK (sensor_model IN ('ops243-a', 'ops243-c'))
     );

CREATE INDEX IF NOT EXISTS idx_radar_serial_config_enabled 
    ON radar_serial_config (enabled);

CREATE TRIGGER IF NOT EXISTS update_radar_serial_config_timestamp 
    AFTER UPDATE ON radar_serial_config 
BEGIN
    UPDATE radar_serial_config 
    SET updated_at = STRFTIME('%s', 'now') 
    WHERE id = NEW.id;
END;

-- Insert default configuration for HAT (Raspberry Pi header)
INSERT OR IGNORE INTO radar_serial_config (
       name
     , port_path
     , baud_rate
     , data_bits
     , stop_bits
     , parity
     , enabled
     , description
     , sensor_model
     )
VALUES (
       'Default HAT'
     , '/dev/ttySC1'
     , 19200
     , 8
     , 1
     , 'N'
     , 1
     , 'Default serial configuration for Raspberry Pi HAT (SC16IS762)'
     , 'ops243-a'
     );
