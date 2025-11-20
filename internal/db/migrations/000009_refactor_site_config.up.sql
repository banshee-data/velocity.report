-- Migration: Refactor site configuration to support time-varying settings
-- Date: 2025-11-19
-- Description: Move cosine_error_angle from site table to separate site_variable_config table
--              Add site_config_periods table to track configuration changes over time (Type 6 SCD)
-- Step 1: Create site_variable_config table for time-varying configuration
   CREATE TABLE IF NOT EXISTS site_variable_config (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , cosine_error_angle REAL NOT NULL
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          );

-- Trigger to update timestamp on site_variable_config
CREATE TRIGGER IF NOT EXISTS update_site_variable_config_timestamp AFTER
   UPDATE ON site_variable_config BEGIN
   UPDATE site_variable_config
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

-- Step 2: Create site_config_periods table (Type 6 SCD)
   CREATE TABLE IF NOT EXISTS site_config_periods (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , site_id INTEGER NOT NULL
        , site_variable_config_id INTEGER
        , effective_start_unix REAL NOT NULL
        , effective_end_unix REAL
        , is_active INTEGER DEFAULT 0
        , notes TEXT
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , FOREIGN KEY (site_id) REFERENCES site (id)
        , FOREIGN KEY (site_variable_config_id) REFERENCES site_variable_config (id)
          );

-- Indexes for site_config_periods
CREATE INDEX IF NOT EXISTS idx_site_config_periods_site ON site_config_periods (site_id);

CREATE INDEX IF NOT EXISTS idx_site_config_periods_variable_config ON site_config_periods (site_variable_config_id);

CREATE INDEX IF NOT EXISTS idx_site_config_periods_time ON site_config_periods (effective_start_unix, effective_end_unix);

CREATE INDEX IF NOT EXISTS idx_site_config_periods_active ON site_config_periods (is_active);

-- Trigger to update timestamp on site_config_periods
CREATE TRIGGER IF NOT EXISTS update_site_config_periods_timestamp AFTER
   UPDATE ON site_config_periods BEGIN
   UPDATE site_config_periods
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

-- Trigger to enforce single active period
CREATE TRIGGER IF NOT EXISTS enforce_single_active_period BEFORE INSERT ON site_config_periods WHEN NEW.is_active = 1 BEGIN
   UPDATE site_config_periods
      SET is_active = 0
    WHERE is_active = 1;

END;

CREATE TRIGGER IF NOT EXISTS enforce_single_active_period_update BEFORE
   UPDATE ON site_config_periods WHEN NEW.is_active = 1
      AND OLD.is_active = 0 BEGIN
             UPDATE site_config_periods
                SET is_active = 0
              WHERE is_active = 1
                AND id != NEW.id;

END;

-- Step 3: Migrate existing cosine_error_angle data
-- For each site, create a corresponding variable config and config period
-- Using ROW_NUMBER to ensure correct pairing even if IDs don't match
   INSERT INTO site_variable_config (cosine_error_angle)
   SELECT cosine_error_angle
     FROM site
    ORDER BY id;

   INSERT INTO site_config_periods (
          site_id
        , site_variable_config_id
        , effective_start_unix
        , effective_end_unix
        , is_active
          )
   SELECT s.id AS site_id
        , vc.id AS site_variable_config_id
        , 0.0 AS effective_start_unix
        , NULL AS effective_end_unix
        , CASE WHEN s.id = (SELECT MIN(id) FROM site) THEN 1 ELSE 0 END AS is_active
     FROM (
         SELECT id, cosine_error_angle, ROW_NUMBER() OVER (ORDER BY id) AS rn FROM site
     ) s
     JOIN (
         SELECT id, ROW_NUMBER() OVER (ORDER BY id) AS rn FROM site_variable_config
     ) vc
     ON s.rn = vc.rn;

-- Step 4: Remove cosine_error_angle from site table
-- SQLite doesn't support DROP COLUMN, so we need to recreate the table
   CREATE TABLE site_new (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , name TEXT NOT NULL UNIQUE
        , location TEXT NOT NULL
        , description TEXT
        , speed_limit INTEGER DEFAULT 25
        , surveyor TEXT NOT NULL
        , contact TEXT NOT NULL
        , address TEXT
        , latitude REAL
        , longitude REAL
        , map_angle REAL
        , include_map INTEGER DEFAULT 0
        , site_description TEXT
        , speed_limit_note TEXT
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          );

-- Copy data (excluding cosine_error_angle)
   INSERT INTO site_new
   SELECT id
        , name
        , location
        , description
        , speed_limit
        , surveyor
        , contact
        , address
        , latitude
        , longitude
        , map_angle
        , include_map
        , site_description
        , speed_limit_note
        , created_at
        , updated_at
     FROM site;

-- Drop old table and rename
     DROP TABLE site;

    ALTER TABLE site_new
RENAME TO site;

-- Recreate indexes and triggers
CREATE INDEX IF NOT EXISTS idx_site_name ON site (name);

CREATE TRIGGER IF NOT EXISTS update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;
