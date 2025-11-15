-- Rollback: Remove all initial schema tables

DROP TABLE IF EXISTS radar_command_log;
DROP TABLE IF EXISTS radar_commands;
DROP TABLE IF EXISTS radar_objects;
DROP TABLE IF EXISTS radar_data;
