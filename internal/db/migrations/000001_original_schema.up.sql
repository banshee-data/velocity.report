-- Migration: Create original database schema (pre-JSON, timestamp column)
-- Date: 2024 (reconstructed from commit f5ade674)
-- Description: The very first database schema with timestamp column (not write_timestamp)
-- This schema predates the switch to JSON raw_event storage and write_timestamp naming.
-- Note: The data table is created in migration 000000_bootstrap.
   CREATE TABLE IF NOT EXISTS commands (
          command_id BIGINT PRIMARY KEY
        , command TEXT
        , timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
          );

   CREATE TABLE IF NOT EXISTS LOG(
          log_id BIGINT PRIMARY KEY
        , command_id BIGINT
        , log_data TEXT
        , timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        , FOREIGN KEY (command_id) REFERENCES commands (command_id)
          );
