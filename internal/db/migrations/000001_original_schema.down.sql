-- Rollback migration: Drop original schema tables
DROP TABLE IF EXISTS log;
DROP TABLE IF EXISTS commands;
DROP TABLE IF EXISTS data;
