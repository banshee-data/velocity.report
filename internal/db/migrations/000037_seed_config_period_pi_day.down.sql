-- Revert: restore migration 13's original backfill values
   UPDATE site_config_periods
      SET effective_start_unix = 0
        , notes = 'Migrated from site.cosine_error_angle'
    WHERE site_id = 1
      AND notes = 'Sample configuration: the cosine error angle is a guess. Measure yours and replace it.'
      AND effective_end_unix IS NULL;
