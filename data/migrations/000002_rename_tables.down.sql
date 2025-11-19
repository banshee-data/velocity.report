-- Rollback: Revert table and column renames
-- Revert column rename in radar_objects
    ALTER TABLE radar_objects
   RENAME COLUMN length_m TO LENGTH;

-- Revert table renames
    ALTER TABLE radar_command_log
RENAME TO log;

    ALTER TABLE radar_commands
RENAME TO commands;

    ALTER TABLE radar_data
RENAME TO data;
