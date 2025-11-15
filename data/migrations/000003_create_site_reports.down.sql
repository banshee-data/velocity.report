-- Rollback: Remove site_reports table and related indexes

DROP INDEX IF EXISTS idx_site_reports_created_at;

DROP INDEX IF EXISTS idx_site_reports_site_id;

DROP TABLE IF EXISTS site_reports;
