-- Migration: Create site_config_periods table for time-based radar configuration
-- Date: 2026-01-20
-- Description: Track cosine error angle changes over time with active period support
CREATE TABLE IF NOT EXISTS site_config_periods (
       id INTEGER PRIMARY KEY AUTOINCREMENT
     , site_id INTEGER NOT NULL
     , effective_start_unix DOUBLE NOT NULL
     , effective_end_unix DOUBLE
     , is_active INTEGER NOT NULL DEFAULT 0
     , notes TEXT
     , cosine_error_angle DOUBLE NOT NULL DEFAULT 0
     , created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
     , updated_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
     , FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE
       );

CREATE INDEX IF NOT EXISTS idx_site_config_periods_site_id ON site_config_periods (site_id);
CREATE INDEX IF NOT EXISTS idx_site_config_periods_effective ON site_config_periods (site_id, effective_start_unix, effective_end_unix);
CREATE INDEX IF NOT EXISTS idx_site_config_periods_active ON site_config_periods (site_id, is_active) WHERE is_active = 1;

-- Ensure only one active period per site
CREATE TRIGGER IF NOT EXISTS ensure_single_active_period_insert
BEFORE INSERT ON site_config_periods
WHEN NEW.is_active = 1
BEGIN
UPDATE site_config_periods
   SET is_active = 0
 WHERE site_id = NEW.site_id
   AND is_active = 1;
END;

CREATE TRIGGER IF NOT EXISTS ensure_single_active_period_update
BEFORE UPDATE OF is_active ON site_config_periods
WHEN NEW.is_active = 1
BEGIN
UPDATE site_config_periods
   SET is_active = 0
 WHERE site_id = NEW.site_id
   AND is_active = 1
   AND id != NEW.id;
END;

-- Update updated_at timestamp on changes
CREATE TRIGGER IF NOT EXISTS update_site_config_periods_timestamp AFTER
UPDATE ON site_config_periods BEGIN
UPDATE site_config_periods
   SET updated_at = UNIXEPOCH('subsec')
 WHERE id = NEW.id;
END;

-- Backfill existing site cosine_error_angle into an open-ended active period
INSERT INTO site_config_periods (
       site_id
     , effective_start_unix
     , effective_end_unix
     , is_active
     , notes
     , cosine_error_angle
     , created_at
     , updated_at
       )
SELECT
       site.id
     , 0
     , NULL
     , 1
     , 'Migrated from site.cosine_error_angle'
     , site.cosine_error_angle
     , UNIXEPOCH('subsec')
     , UNIXEPOCH('subsec')
  FROM site
 WHERE NOT EXISTS (
       SELECT 1
         FROM site_config_periods
        WHERE site_config_periods.site_id = site.id
       );
