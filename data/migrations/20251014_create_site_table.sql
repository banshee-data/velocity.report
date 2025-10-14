-- Migration: Create site table for storing site-specific configuration
-- Date: 2025-10-14
-- Description: Add site table to store location information, radar configuration, and report settings
   CREATE TABLE IF NOT EXISTS site (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , name TEXT NOT NULL UNIQUE
        , location TEXT NOT NULL
        , description TEXT, -- Radar configuration
          cosine_error_angle REAL NOT NULL, -- Required: mounting angle in degrees
          speed_limit INTEGER DEFAULT 25, -- Contact information
          surveyor TEXT NOT NULL
        , contact TEXT NOT NULL, -- Address and map information
          address TEXT
        , latitude REAL
        , longitude REAL
        , map_angle REAL
        , include_map INTEGER DEFAULT 1, -- boolean: 1 = yes, 0 = no
          -- Report content
          site_description TEXT
        , speed_limit_note TEXT, -- Timestamps
          created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          );

-- Create index on name for faster lookups
CREATE INDEX IF NOT EXISTS idx_site_name ON site (name);

-- Create trigger to update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;

-- Insert a default site for existing installations
   INSERT INTO site (
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
          'Default Site'
        , 'Survey Location'
        , 'Default site configuration'
        , 21.0
        , 25
        , 'Surveyor'
        , 'contact@example.com'
        , 'Default site for radar velocity surveys'
          );
