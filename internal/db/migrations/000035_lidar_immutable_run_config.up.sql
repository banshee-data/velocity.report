PRAGMA foreign_keys = OFF;

   CREATE TABLE lidar_param_sets (
          param_set_id TEXT PRIMARY KEY
        , params_hash TEXT NOT NULL UNIQUE
        , schema_version TEXT NOT NULL
        , param_set_type TEXT NOT NULL
        , params_json TEXT NOT NULL
        , created_at INTEGER NOT NULL
        , CHECK (param_set_type IN ('requested', 'effective', 'legacy'))
          );

   CREATE TABLE lidar_run_configs (
          run_config_id TEXT PRIMARY KEY
        , config_hash TEXT NOT NULL UNIQUE
        , param_set_id TEXT NOT NULL REFERENCES lidar_param_sets (param_set_id)
        , build_version TEXT NOT NULL
        , build_git_sha TEXT NOT NULL
        , created_at INTEGER NOT NULL
        , UNIQUE (param_set_id, build_version, build_git_sha)
          );

    ALTER TABLE lidar_run_records
      ADD COLUMN run_config_id TEXT REFERENCES lidar_run_configs (run_config_id) ON DELETE SET NULL;

    ALTER TABLE lidar_run_records
      ADD COLUMN requested_param_set_id TEXT REFERENCES lidar_param_sets (param_set_id) ON DELETE SET NULL;

    ALTER TABLE lidar_run_records
      ADD COLUMN replay_case_id TEXT REFERENCES lidar_replay_cases (replay_case_id) ON DELETE SET NULL;

    ALTER TABLE lidar_run_records
      ADD COLUMN completed_at INTEGER;

    ALTER TABLE lidar_run_records
      ADD COLUMN frame_start_ns INTEGER;

    ALTER TABLE lidar_run_records
      ADD COLUMN frame_end_ns INTEGER;

    ALTER TABLE lidar_replay_cases
      ADD COLUMN recommended_param_set_id TEXT REFERENCES lidar_param_sets (param_set_id) ON DELETE SET NULL;

CREATE INDEX idx_lidar_param_sets_params_hash ON lidar_param_sets (params_hash);

CREATE INDEX idx_lidar_run_configs_config_hash ON lidar_run_configs (config_hash);

CREATE INDEX idx_lidar_run_records_run_config ON lidar_run_records (run_config_id);

CREATE INDEX idx_lidar_run_records_requested_param_set ON lidar_run_records (requested_param_set_id);

CREATE INDEX idx_lidar_run_records_replay_case ON lidar_run_records (replay_case_id);

CREATE INDEX idx_lidar_replay_cases_recommended_param_set ON lidar_replay_cases (recommended_param_set_id);

PRAGMA foreign_keys = ON;
