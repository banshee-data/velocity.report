-- Migration 0: Bootstrap migration for golang-migrate
-- Creates the data table (the very first table in the schema)
-- This migration exists so golang-migrate can start from version 0.
   CREATE TABLE IF NOT EXISTS data (
          uptime DOUBLE
        , magnitude DOUBLE
        , speed DOUBLE
        , timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
          );
