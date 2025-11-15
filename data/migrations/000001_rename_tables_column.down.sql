-- Rollback: Restore original table and column names

ALTER TABLE radar_objects RENAME COLUMN length_m TO length;

ALTER TABLE radar_commands RENAME TO commands;

ALTER TABLE radar_command_log RENAME TO log;

ALTER TABLE radar_data RENAME TO data;
