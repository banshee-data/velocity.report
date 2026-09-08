-- Migration: Geographic identity for located replay cases
-- Date: 2026-09-07
-- Description: Records where a static capture was taken, as a WGS84 position and
-- as the S2 cells derived from it.
-- The levels and token forms are fixed by
-- docs/lidar/architecture/geographic-indexing.md, which is normative. Two of its
-- rules are enforced here by what the schema does and does not hold:
--
--   Only the canonical token is an identifier, so that is what is stored and
--   indexed. The family display (80858-1 and the like) is presentation derived
--   from the token and is never a column.
--
--   The three tokens are one cell family: L16.Parent(13) = L13 and
--   L13.Parent(10) = L10. They are derived together with Parent and written
--   together, so a row cannot hold a family that disagrees with itself.
--
-- Measured at San Francisco's latitude one cell spans roughly: L10 11.6 km by
-- 8.0 km — a district; L13 1.4 km by 1.0 km — a neighbourhood; L16 181 m by
-- 125 m — a junction and its approaches. So L16 is the site, L13 groups the
-- sites in a neighbourhood, and L10 is the archive-scale roll-up the style
-- guide uses for filesystem areas.
--
-- Everything here is nullable. A capture without accepted WGS84 provenance is
-- the ordinary case, not a defect: it keeps its sensor-local artefacts and
-- records geographic_status 'unavailable'.
    ALTER TABLE lidar_replay_cases
      ADD COLUMN origin_lat REAL;

    ALTER TABLE lidar_replay_cases
      ADD COLUMN origin_lon REAL;

    ALTER TABLE lidar_replay_cases
      ADD COLUMN s2_l10_token TEXT;

    ALTER TABLE lidar_replay_cases
      ADD COLUMN s2_l13_token TEXT;

    ALTER TABLE lidar_replay_cases
      ADD COLUMN s2_l16_token TEXT;

-- How the position was established. A surveyed value comes from a configured
-- site origin. An operator value was entered by hand. A fix was taken from the
-- capture itself.
    ALTER TABLE lidar_replay_cases
      ADD COLUMN geographic_source TEXT;

    ALTER TABLE lidar_replay_cases
      ADD COLUMN geographic_status TEXT NOT NULL DEFAULT 'unavailable';

-- The site index. Grouping every located case by its L10 cell is what the scene
-- map is: one entry per junction, however many visits it holds.
CREATE INDEX IF NOT EXISTS idx_lidar_replay_cases_s2_l10 ON lidar_replay_cases (s2_l10_token);

CREATE INDEX IF NOT EXISTS idx_lidar_replay_cases_s2_l13 ON lidar_replay_cases (s2_l13_token);

CREATE INDEX IF NOT EXISTS idx_lidar_replay_cases_s2_l16 ON lidar_replay_cases (s2_l16_token);
