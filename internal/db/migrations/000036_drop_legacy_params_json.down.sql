-- Re-add the legacy JSON columns dropped by migration 000036.
    ALTER TABLE lidar_run_records
      ADD COLUMN params_json TEXT NOT NULL DEFAULT '{}';

    ALTER TABLE lidar_replay_evaluations
      ADD COLUMN params_json TEXT;

    ALTER TABLE lidar_replay_cases
      ADD COLUMN optimal_params_json TEXT;
