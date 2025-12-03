-- Drop triggers
     DROP TRIGGER IF EXISTS update_angle_presets_timestamp;

     DROP TRIGGER IF EXISTS prevent_system_preset_deletion;

-- Drop indexes
     DROP INDEX IF EXISTS idx_angle_presets_is_system;

     DROP INDEX IF EXISTS idx_angle_presets_angle;

-- Drop table
     DROP TABLE IF EXISTS angle_presets;
