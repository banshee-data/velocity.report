-- Rollback: remove the posted speed limit, its unit and its jurisdiction.
-- This loses data: every recorded limit, unit and jurisdiction is dropped and
-- does not come back on a later up migration.
-- speed_limit_unit's CHECK names speed_limit_kph, so it goes first.
    ALTER TABLE site_config_periods
     DROP COLUMN jurisdiction;

    ALTER TABLE site_config_periods
     DROP COLUMN speed_limit_unit;

    ALTER TABLE site_config_periods
     DROP COLUMN speed_limit_kph;
