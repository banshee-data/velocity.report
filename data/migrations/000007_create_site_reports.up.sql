-- Migration: Create site_reports table to track generated reports
-- Date: 2025-10-21
-- Description: Add site_reports table to track generated PDF reports
-- From commit 611b4cf6
   CREATE TABLE IF NOT EXISTS site_reports (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , site_id INTEGER NOT NULL DEFAULT 0
        , start_date TEXT NOT NULL
        , end_date TEXT NOT NULL
        , filepath TEXT NOT NULL
        , filename TEXT NOT NULL
        , zip_filepath TEXT
        , zip_filename TEXT
        , run_id TEXT NOT NULL
        , timezone TEXT NOT NULL
        , units TEXT NOT NULL
        , source TEXT NOT NULL
        , created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        , FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE
          );

-- Index for fast lookups by site
CREATE INDEX IF NOT EXISTS idx_site_reports_site_id ON site_reports (site_id);

-- Index for ordering by creation time
CREATE INDEX IF NOT EXISTS idx_site_reports_created_at ON site_reports (created_at DESC);
