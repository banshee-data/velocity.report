-- Migration rollback: Add back cosine_error_angle, speed_limit, and speed_limit_note to site table
-- Date: 2026-01-20
-- SQLite doesn't support ADD COLUMN with NOT NULL and no default easily
-- So we recreate the table with the original columns
   CREATE TABLE site_new (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , name TEXT NOT NULL UNIQUE
        , location TEXT NOT NULL
        , description TEXT
        , cosine_error_angle REAL
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

-- Copy data back, using defaults for the restored columns
   INSERT INTO site_new (
          id
        , name
        , location
        , description
        , cosine_error_angle
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
          )
   SELECT id
        , name
        , location
        , description
        , NULL
        , 25
        , surveyor
        , contact
        , address
        , latitude
        , longitude
        , map_angle
        , include_map
        , site_description
        , NULL
        , created_at
        , updated_at
     FROM site;

     DROP TABLE site;

    ALTER TABLE site_new
RENAME TO site;

CREATE INDEX idx_site_name ON site (name);

CREATE TRIGGER update_site_timestamp AFTER
   UPDATE ON site BEGIN
   UPDATE site
      SET updated_at = STRFTIME('%s', 'now')
    WHERE id = NEW.id;

END;
