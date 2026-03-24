-- P2 cleanup: drop legacy JSON columns now that the immutable run-config
-- asset model (migration 000035) is fully adopted.
--
-- lidar_run_records.params_json   → replaced by run_config_id
-- lidar_replay_evaluations.params_json → derive from candidate_run_id
-- lidar_replay_cases.optimal_params_json → replaced by recommended_param_set_id
    ALTER TABLE lidar_run_records
     DROP COLUMN params_json;

    ALTER TABLE lidar_replay_evaluations
     DROP COLUMN params_json;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN optimal_params_json;
