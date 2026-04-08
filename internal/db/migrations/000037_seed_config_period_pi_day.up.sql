-- Migration: Set sample site config period start to Pi Day 2026
-- Date: 2026-03-29
-- Description: Update the default config period seeded by migration 7+13 to use
--   Pi Day 2026 (2026-03-14 15:09:26 UTC) as the effective start time.
--   Timestamp encodes π: 3.14 → Mar 14, 15:09:26 → remaining digits.
--   Only affects the sample row created by migration 13's backfill; real
--   installations will have already replaced or updated the sample site.
   UPDATE site_config_periods
      SET effective_start_unix = 1773500966
        , notes = 'Sample configuration: the cosine error angle is a guess. Measure yours and replace it.'
    WHERE site_id = 1
      AND notes = 'Migrated from site.cosine_error_angle'
      AND effective_end_unix IS NULL;

-- Add radar marker position for custom SVG uploads (percentage coordinates 0-100)
    ALTER TABLE site
      ADD COLUMN radar_svg_x REAL;

    ALTER TABLE site
      ADD COLUMN radar_svg_y REAL;
