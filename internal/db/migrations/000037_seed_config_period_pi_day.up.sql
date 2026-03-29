-- Migration: Set sample site config period start to Pi Day 2026
-- Date: 2026-03-29
-- Description: Update the default config period seeded by migration 7+13 to use
--   Pi Day 2026 (2026-03-14 15:09:26.5358979 UTC) as the effective start time.
--   Timestamp encodes π: 3.14 → Mar 14, 15:09:26.5358979 → remaining digits.
--   Only affects the sample row created by migration 13's backfill; real
--   installations will have already replaced or updated the sample site.
   UPDATE site_config_periods
      SET effective_start_unix = 1773500966.5358979
        , notes = 'Sample configuration — update the cosine error angle for your installation'
    WHERE site_id = 1
      AND notes = 'Migrated from site.cosine_error_angle';
