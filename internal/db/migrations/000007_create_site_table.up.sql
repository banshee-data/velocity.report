-- Migration: Create site table for location and configuration
-- Date: 2025-10-14
-- Description: Add site table for storing location information, radar configuration, and report settings
-- From commit e53d50da
   CREATE TABLE IF NOT EXISTS site (
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

CREATE INDEX IF NOT EXISTS idx_site_name ON site (name);

-- Create trigger to update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

-- Insert a default site for existing installations
   INSERT OR IGNORE INTO site (
          name
        , location
        , description
        , cosine_error_angle
        , speed_limit
        , surveyor
        , contact
        , site_description
          )
   VALUES (
          'Default Location'
        , 'Survey Location'
        , 'Default site configuration'
        , 0.5
        , 25
        , 'Sir Veyor'
        , 'example@velocity.report'
        , 'Default site for radar velocity surveys'
          );
