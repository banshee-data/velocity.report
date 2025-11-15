-- Rollback: Remove velocity.report_ prefix from report filenames
-- WARNING: This assumes all filenames have the velocity.report_ prefix

-- Remove velocity.report_ prefix from PDF filenames
UPDATE site_reports
SET filename = SUBSTR(filename, 18)  -- Remove 'velocity.report_' (17 chars + 1)
WHERE filename LIKE 'velocity.report_%'
  AND filename IS NOT NULL;

-- Remove velocity.report_ prefix from ZIP filenames
UPDATE site_reports
SET zip_filename = SUBSTR(zip_filename, 18)  -- Remove 'velocity.report_' (17 chars + 1)
WHERE zip_filename LIKE 'velocity.report_%'
  AND zip_filename IS NOT NULL;

-- Note: This rollback assumes filenames follow the standard pattern
-- Manual verification may be needed for edge cases
