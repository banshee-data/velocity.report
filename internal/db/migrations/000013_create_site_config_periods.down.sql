DROP TRIGGER IF EXISTS update_site_config_periods_timestamp;
DROP TRIGGER IF EXISTS ensure_single_active_period_update;
DROP TRIGGER IF EXISTS ensure_single_active_period_insert;

DROP INDEX IF EXISTS idx_site_config_periods_active;
DROP INDEX IF EXISTS idx_site_config_periods_effective;
DROP INDEX IF EXISTS idx_site_config_periods_site_id;

DROP TABLE IF EXISTS site_config_periods;
