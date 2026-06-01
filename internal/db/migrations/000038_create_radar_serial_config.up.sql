-- Migration: Create radar_serial_config table for storing serial port configurations
-- Date: 2026-02-20
-- Description: Add radar_serial_config table to support database-driven serial configuration with multi-sensor support
-- Serial port configurations table
   CREATE TABLE IF NOT EXISTS radar_serial_config (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , port_path TEXT NOT NULL UNIQUE
        , baud_rate INTEGER NOT NULL DEFAULT 19200
        , data_bits INTEGER NOT NULL DEFAULT 8
        , stop_bits INTEGER NOT NULL DEFAULT 1
        , parity TEXT NOT NULL DEFAULT 'N'
        , enabled INTEGER NOT NULL DEFAULT 1
        , sensor_model TEXT NOT NULL DEFAULT 'ops243-a'
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          -- Note: Detailed sensor_model validation is performed in Go using the
          -- SupportedSensorModels registry; this CHECK only enforces a basic
          -- format to avoid requiring schema migrations when new models are added.

        , CHECK (sensor_model LIKE 'ops243-%')
          );

CREATE INDEX IF NOT EXISTS idx_radar_serial_config_enabled ON radar_serial_config (enabled);

-- +migrate StatementBegin
CREATE TRIGGER IF NOT EXISTS update_radar_serial_config_timestamp AFTER
   UPDATE ON radar_serial_config BEGIN
   UPDATE radar_serial_config
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

-- +migrate StatementEnd
-- Insert default configuration for HAT (Raspberry Pi header, SC16IS762)
   INSERT OR IGNORE INTO radar_serial_config (
          port_path
        , baud_rate
        , data_bits
        , stop_bits
        , parity
        , enabled
        , sensor_model
          )
   VALUES ('/dev/ttySC1', 19200, 8, 1, 'N', 1, 'ops243-a');
