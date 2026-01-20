-- Migration: Remove cosine_error_angle, speed_limit, and speed_limit_note from site table
-- Date: 2026-01-20
-- Description: These fields are now managed exclusively through site_config_periods (SCD)
--              speed_limit and speed_limit_note will be reintroduced later
-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
-- Step 1: Create new table without the columns we want to remove
   CREATE TABLE site_new (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , name TEXT NOT NULL UNIQUE
        , location TEXT NOT NULL
        , description TEXT
        , surveyor TEXT NOT NULL
        , contact TEXT NOT NULL
        , address TEXT
        , latitude REAL
        , longitude REAL
        , map_angle REAL
        , include_map INTEGER DEFAULT 0
        , site_description TEXT
        , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
        , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
          );

-- Step 2: Copy data from old table to new table (excluding removed columns)
   INSERT INTO site_new (
          id
        , name
        , location
        , description
        , surveyor
        , contact
        , address
        , latitude
        , longitude
        , map_angle
        , include_map
        , site_description
        , created_at
        , updated_at
          )
   SELECT id
        , name
        , location
        , description
        , surveyor
        , contact
        , address
        , latitude
        , longitude
        , map_angle
        , include_map
        , site_description
        , created_at
        , updated_at
     FROM site;

-- Step 3: Drop old table
     DROP TABLE site;

-- Step 4: Rename new table to site
    ALTER TABLE site_new
RENAME TO site;

-- Step 5: Recreate indexes
CREATE INDEX idx_site_name ON site (name);

-- Step 6: Recreate trigger
CREATE TRIGGER update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;
