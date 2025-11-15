-- Rollback: Remove site table and related objects

DROP TRIGGER IF EXISTS update_site_timestamp;

DROP INDEX IF EXISTS idx_site_name;

DROP TABLE IF EXISTS site;
