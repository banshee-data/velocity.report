-- Rollback: Return replay cases to a single PCAP file
-- WARNING: A case referencing more than one capture loses every file after the
-- first. pcap_file already holds that first file, so single-file cases are
-- unaffected; multi-file cases are not recoverable from the remaining columns.
     DROP INDEX IF EXISTS idx_lidar_replay_cases_session;

     DROP INDEX IF EXISTS idx_lidar_replay_case_files_capture;

     DROP INDEX IF EXISTS idx_lidar_replay_case_files_case;

     DROP TABLE IF EXISTS lidar_replay_case_files;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN source_period_id;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN session_id;
