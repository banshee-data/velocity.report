-- Migration: Sites — the persisted place a located replay case is at
-- Date: 2026-09-07
-- Description: A site is one L16 cell: a junction and its approaches, per
-- docs/lidar/architecture/geographic-indexing.md. It is created the first
-- time a case is located there (see SetCaseLocation) and its identity is the
-- L16 token itself, consistent with that guide's rule that only the
-- canonical token is an identifier.
--
-- This separates two things migration 45 conflated in one lat/lon pair on the
-- case: the sensor pose (origin_lat/origin_lon on lidar_replay_cases — where
-- the car was for one visit, which moves visit to visit) and a site's own
-- canonical pose (here — a fixed point such as the midpoint of the
-- intersection, set once and deliberately by a surveyor or an operator, never
-- derived from any one visit). Both stay nullable: a site with several
-- visits and no canonical pose yet is the ordinary case, not a defect.
   CREATE TABLE IF NOT EXISTS "lidar_sites" (
          site_id TEXT PRIMARY KEY
        , s2_l13_token TEXT NOT NULL
        , s2_l10_token TEXT NOT NULL
        , label TEXT
        , canonical_lat REAL
        , canonical_lon REAL
          -- How the canonical pose was set: surveyed or operator. Never a fix —
          -- a canonical pose is by definition not one sensor's reading taken on
          -- one visit.

        , canonical_source TEXT
        , created_at_ns INTEGER NOT NULL
        , updated_at_ns INTEGER
          );

CREATE INDEX IF NOT EXISTS idx_lidar_sites_l13 ON lidar_sites (s2_l13_token);

CREATE INDEX IF NOT EXISTS idx_lidar_sites_l10 ON lidar_sites (s2_l10_token);

-- A scene should be at a site: linking a case to the site it visited, so many
-- visits to one junction share one row rather than each repeating its own
-- copy of where "here" is. Nullable like the rest of a case's geography: an
-- unlocated case names no site.
    ALTER TABLE lidar_replay_cases
      ADD COLUMN site_id TEXT REFERENCES lidar_sites (site_id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_lidar_replay_cases_site_id ON lidar_replay_cases (site_id);

-- Backfill: a case located before this migration already names the site its
-- own L16 token identifies. The site row may not exist yet, so it is created
-- here with no canonical pose — exactly what a fresh site created by
-- SetCaseLocation looks like.
   INSERT INTO lidar_sites (site_id, s2_l13_token, s2_l10_token, created_at_ns)
   SELECT DISTINCT s2_l16_token
        , s2_l13_token
        , s2_l10_token
        , STRFTIME('%s', 'now') * 1000000000
     FROM lidar_replay_cases
    WHERE geographic_status = 'located'
      AND s2_l16_token IS NOT NULL
       ON CONFLICT (site_id) DO NOTHING;

   UPDATE lidar_replay_cases
      SET site_id = s2_l16_token
    WHERE geographic_status = 'located'
      AND s2_l16_token IS NOT NULL;
