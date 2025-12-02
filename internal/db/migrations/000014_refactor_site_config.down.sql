-- Migration: Rollback site configuration refactoring
-- Date: 2025-11-19
-- Description: Restore cosine_error_angle to site table and remove site_variable_config/site_config_periods tables
-- Step 1: Recreate site table with cosine_error_angle
   CREATE TABLE site_new (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , name TEXT NOT NULL UNIQUE
        , location TEXT NOT NULL
        , description TEXT
        , cosine_error_angle REAL NOT NULL
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

-- Copy data back, retrieving cosine_error_angle from active config period
   INSERT INTO site_new
   SELECT s.id
        , s.name
        , s.location
        , s.description
        , COALESCE(vc.cosine_error_angle, 0.5) AS cosine_error_angle
        , s.speed_limit
        , s.surveyor
        , s.contact
        , s.address
        , s.latitude
        , s.longitude
        , s.map_angle
        , s.include_map
        , s.site_description
        , s.speed_limit_note
        , s.created_at
        , s.updated_at
     FROM site s
LEFT JOIN site_config_periods p ON p.site_id = s.id
      AND p.is_active = 1
LEFT JOIN site_variable_config vc ON vc.id = p.site_variable_config_id;

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

-- Step 2: Drop the new tables
     DROP TRIGGER IF EXISTS enforce_single_active_period_update;

     DROP TRIGGER IF EXISTS enforce_single_active_period;

     DROP TRIGGER IF EXISTS update_site_config_periods_timestamp;

     DROP INDEX IF EXISTS idx_site_config_periods_active;

     DROP INDEX IF EXISTS idx_site_config_periods_time;

     DROP INDEX IF EXISTS idx_site_config_periods_variable_config;

     DROP INDEX IF EXISTS idx_site_config_periods_site;

     DROP TABLE IF EXISTS site_config_periods;

     DROP TRIGGER IF EXISTS update_site_variable_config_timestamp;

     DROP TABLE IF EXISTS site_variable_config;
