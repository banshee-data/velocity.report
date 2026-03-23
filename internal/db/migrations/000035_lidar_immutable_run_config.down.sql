PRAGMA foreign_keys = OFF;

     DROP INDEX IF EXISTS idx_lidar_replay_cases_recommended_param_set;

     DROP INDEX IF EXISTS idx_lidar_run_records_replay_case;

     DROP INDEX IF EXISTS idx_lidar_run_records_requested_param_set;

     DROP INDEX IF EXISTS idx_lidar_run_records_run_config;

     DROP INDEX IF EXISTS idx_lidar_run_configs_config_hash;

     DROP INDEX IF EXISTS idx_lidar_param_sets_params_hash;

   CREATE TABLE lidar_run_records_old (
          run_id TEXT PRIMARY KEY
        , created_at INTEGER NOT NULL
        , source_type TEXT NOT NULL
        , source_path TEXT
        , sensor_id TEXT NOT NULL
        , params_json TEXT NOT NULL
        , duration_secs REAL
        , total_frames INTEGER
        , total_clusters INTEGER
        , total_tracks INTEGER
        , confirmed_tracks INTEGER
        , processing_time_ms INTEGER
        , status TEXT NOT NULL DEFAULT 'running'
        , error_message TEXT
        , parent_run_id TEXT
        , notes TEXT
        , statistics_json TEXT
        , vrlog_path TEXT
        , CHECK (source_type IN ('live', 'pcap'))
        , CHECK (status IN ('running', 'completed', 'failed'))
        , FOREIGN KEY (parent_run_id) REFERENCES lidar_run_records_old (run_id) ON DELETE SET NULL
          );

   INSERT INTO lidar_run_records_old (
          run_id
        , created_at
        , source_type
        , source_path
        , sensor_id
        , params_json
        , duration_secs
        , total_frames
        , total_clusters
        , total_tracks
        , confirmed_tracks
        , processing_time_ms
        , status
        , error_message
        , parent_run_id
        , notes
        , statistics_json
        , vrlog_path
          )
   SELECT run_id
        , created_at
        , source_type
        , source_path
        , sensor_id
        , params_json
        , duration_secs
        , total_frames
        , total_clusters
        , total_tracks
        , confirmed_tracks
        , processing_time_ms
        , status
        , error_message
        , parent_run_id
        , notes
        , statistics_json
        , vrlog_path
     FROM lidar_run_records;

     DROP TABLE lidar_run_records;

    ALTER TABLE lidar_run_records_old
RENAME TO lidar_run_records;

CREATE INDEX idx_lidar_runs_created ON lidar_run_records (created_at);

CREATE INDEX idx_lidar_runs_source ON lidar_run_records (source_path);

CREATE INDEX idx_lidar_runs_parent ON lidar_run_records (parent_run_id);

CREATE INDEX idx_lidar_runs_status ON lidar_run_records (status);

   CREATE TABLE lidar_replay_cases_old (
          replay_case_id TEXT PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , pcap_file TEXT NOT NULL
        , pcap_start_secs REAL
        , pcap_duration_secs REAL
        , description TEXT
        , reference_run_id TEXT
        , optimal_params_json TEXT
        , created_at_ns INTEGER NOT NULL
        , updated_at_ns INTEGER
        , CHECK (
          pcap_start_secs IS NULL
       OR pcap_start_secs >= 0
          )
        , CHECK (
          pcap_duration_secs IS NULL
       OR pcap_duration_secs >= 0
          )
        , FOREIGN KEY (reference_run_id) REFERENCES lidar_run_records (run_id) ON DELETE SET NULL
          );

   INSERT INTO lidar_replay_cases_old (
          replay_case_id
        , sensor_id
        , pcap_file
        , pcap_start_secs
        , pcap_duration_secs
        , description
        , reference_run_id
        , optimal_params_json
        , created_at_ns
        , updated_at_ns
          )
   SELECT replay_case_id
        , sensor_id
        , pcap_file
        , pcap_start_secs
        , pcap_duration_secs
        , description
        , reference_run_id
        , optimal_params_json
        , created_at_ns
        , updated_at_ns
     FROM lidar_replay_cases;

     DROP TABLE lidar_replay_cases;

    ALTER TABLE lidar_replay_cases_old
RENAME TO lidar_replay_cases;

CREATE INDEX idx_lidar_replay_cases_sensor ON lidar_replay_cases (sensor_id);

CREATE INDEX idx_lidar_replay_cases_pcap ON lidar_replay_cases (pcap_file);

     DROP TABLE IF EXISTS lidar_run_configs;

     DROP TABLE IF EXISTS lidar_param_sets;

PRAGMA foreign_keys = ON;
