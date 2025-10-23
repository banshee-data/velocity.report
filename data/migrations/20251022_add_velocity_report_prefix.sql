-- Add velocity.report_ prefix to existing report filenames
-- This migration updates filenames in site_reports table to include the new standard prefix
-- Update PDF filenames (add velocity.report_ prefix if not already present)
   UPDATE site_reports
      SET filename = 'velocity.report_' || filename
    WHERE filename NOT LIKE 'velocity.report_%'
      AND filename IS NOT NULL
      AND filename != '';

-- Update ZIP filenames (add velocity.report_ prefix if not already present)
   UPDATE site_reports
      SET zip_filename = 'velocity.report_' || zip_filename
    WHERE zip_filename NOT LIKE 'velocity.report_%'
      AND zip_filename IS NOT NULL
      AND zip_filename != '';

-- Note: This migration updates database records but does NOT rename files on disk.
-- Old report files on disk should be regenerated or manually renamed to match the new format.
