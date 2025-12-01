-- Rollback: Revert to original schema with timestamp column
-- This reverses the JSON and write_timestamp changes
-- Revert data table
    ALTER TABLE data
RENAME TO data_new;

   CREATE TABLE data (
          uptime DOUBLE
        , magnitude DOUBLE
        , speed DOUBLE
        , timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
          );

   INSERT INTO data (uptime, magnitude, speed, timestamp)
   SELECT uptime
        , magnitude
        , speed
        , DATETIME(write_timestamp, 'unixepoch')
     FROM data_new;

     DROP TABLE data_new;

-- Drop radar_objects (didn't exist in original schema)
     DROP TABLE IF EXISTS radar_objects;

-- Revert commands table
    ALTER TABLE commands
RENAME TO commands_new;

   CREATE TABLE commands (
          command_id BIGINT PRIMARY KEY
        , command TEXT
        , timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
          );

   INSERT INTO commands (command_id, command, timestamp)
   SELECT command_id
        , command
        , DATETIME(write_timestamp, 'unixepoch')
     FROM commands_new;

     DROP TABLE commands_new;

-- Revert log table
    ALTER TABLE log
RENAME TO log_new;

   CREATE TABLE log(
          log_id BIGINT PRIMARY KEY
        , command_id BIGINT
        , log_data TEXT
        , timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        , FOREIGN KEY (command_id) REFERENCES commands (command_id)
          );

   INSERT INTO log(log_id, command_id, log_data, timestamp)
   SELECT log_id
        , command_id
        , log_data
        , DATETIME(write_timestamp, 'unixepoch')
     FROM log_new;

     DROP TABLE log_new;
