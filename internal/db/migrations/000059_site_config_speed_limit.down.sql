-- Rollback: remove the posted speed limit, its unit and its jurisdiction.
-- speed_limit_unit's CHECK names speed_limit_kph, so it goes first.
    ALTER TABLE site_config_periods
     DROP COLUMN jurisdiction;

    ALTER TABLE site_config_periods
     DROP COLUMN speed_limit_unit;

    ALTER TABLE site_config_periods
     DROP COLUMN speed_limit_kph;
