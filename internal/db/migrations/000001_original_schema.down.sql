-- Rollback migration: Drop original schema tables (except data, which is in migration 000000)
     DROP TABLE IF EXISTS LOG;

     DROP TABLE IF EXISTS commands;
