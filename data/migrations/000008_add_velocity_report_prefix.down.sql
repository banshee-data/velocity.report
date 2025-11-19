-- Rollback: Remove velocity.report_ prefix from report filenames
-- WARNING: This rollback is lossy - cannot distinguish between filenames that
-- originally had the prefix vs. those added by the migration
-- Remove velocity.report_ prefix from PDF filenames
   UPDATE site_reports
      SET filename = SUBSTR(filename, 17)
    WHERE filename LIKE 'velocity.report_%';

-- Remove velocity.report_ prefix from ZIP filenames
   UPDATE site_reports
      SET zip_filename = SUBSTR(zip_filename, 17)
    WHERE zip_filename LIKE 'velocity.report_%';
