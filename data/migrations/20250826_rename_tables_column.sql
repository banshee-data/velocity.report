-- To apply this migration, run:
--   sqlite3 sensor_data.db < data/migrations/20250826_rename_tables_column.sql
    ALTER TABLE data
RENAME TO radar_data
;

    ALTER TABLE log
RENAME TO radar_command_log
;

    ALTER TABLE commands
RENAME TO radar_commands
;

    ALTER TABLE radar_objects
   RENAME COLUMN length TO length_m
;
