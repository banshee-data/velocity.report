-- Migration: Rename legacy tables and columns to new naming convention
-- Date: 2025-08-26
-- Description: Rename 'data' to 'radar_data', 'log' to 'radar_command_log', 
--              'commands' to 'radar_commands', and rename 'length' column to 'length_m'

ALTER TABLE data RENAME TO radar_data;

ALTER TABLE log RENAME TO radar_command_log;

ALTER TABLE commands RENAME TO radar_commands;

ALTER TABLE radar_objects RENAME COLUMN length TO length_m;
