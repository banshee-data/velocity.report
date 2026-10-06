-- Migration: a posted speed limit, its unit and its jurisdiction per site configuration period
-- Date: 2026-10-06
-- Description: No speed limit existed anywhere in the schema. The only one in
-- the codebase was a per-request report parameter (ReportConfig.SpeedLimit).
-- So no speed benchmark against the legal limit could be reproduced: a limit
-- has an effective date range, and site_config_periods already carries exactly
-- that pattern (behaviour analytics plan, Section 10.1).
--
-- speed_limit_kph is the posted limit in km/h, the canonical unit.
-- speed_limit_unit is the unit the sign shows, kph or mph, so a 25 mph limit
-- reads back as 25 mph rather than 40.2 km/h. It is set exactly when a limit
-- is. jurisdiction names whose rule the limit is, as free text (an ISO 3166-2
-- code such as US-CA where one fits). All three are optional: a period with no
-- recorded limit has none, and nothing is inferred for existing rows.
--
-- A CHECK passes when its expression is NULL, so the pairing compares IS NULL
-- results, which are never NULL; NULL IN (...) would let a limit without a
-- unit through. The unit's CHECK names speed_limit_kph, so the down migration
-- drops the unit first.
    ALTER TABLE site_config_periods
      ADD COLUMN speed_limit_kph DOUBLE CHECK (
speed_limit_kph IS NULL
       OR (
          speed_limit_kph > 0
      AND speed_limit_kph <= 200
          )
          );

    ALTER TABLE site_config_periods
      ADD COLUMN speed_limit_unit TEXT CHECK (
(speed_limit_unit IS NULL) = (speed_limit_kph IS NULL)
      AND (
          speed_limit_unit IS NULL
       OR speed_limit_unit IN ('kph', 'mph')
          )
          );

    ALTER TABLE site_config_periods
      ADD COLUMN jurisdiction TEXT CHECK (
jurisdiction IS NULL
       OR LENGTH(TRIM(jurisdiction)) BETWEEN 1 AND 100
          );
