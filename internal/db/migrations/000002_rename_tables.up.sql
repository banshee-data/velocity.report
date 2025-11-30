-- Migration: Rename tables to add radar_ prefix
-- Date: 2025-08-26
-- Description: Rename tables for clarity: data→radar_data, commands→radar_commands, log→radar_command_log
-- From commit b722fdd1
-- Rename data table to radar_data
    ALTER TABLE data
RENAME TO radar_data;

-- Rename commands table to radar_commands
    ALTER TABLE commands
RENAME TO radar_commands;

-- Rename log table to radar_command_log
    ALTER TABLE log
RENAME TO radar_command_log;

-- Also rename the 'length' column to 'length_m' in radar_objects for clarity
    ALTER TABLE radar_objects
   RENAME COLUMN length TO length_m;
