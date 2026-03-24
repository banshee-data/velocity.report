-- P2 cleanup: drop legacy JSON columns now that the immutable run-config
-- asset model (migration 000035) is fully adopted.
--
-- Advisory: run the backfill tool (cmd/tools/backfill_lidar_run_config)
-- before applying this migration to ensure historical rows have been
-- normalised into run_config_id / recommended_param_set_id. The migration
-- itself does not enforce this — enforcement is the operator's responsibility.
--
-- lidar_run_records.params_json         → replaced by run_config_id
-- lidar_replay_evaluations.params_json  → derive from candidate_run_id
-- lidar_replay_cases.optimal_params_json → replaced by recommended_param_set_id
    ALTER TABLE lidar_run_records
     DROP COLUMN params_json;

    ALTER TABLE lidar_replay_evaluations
     DROP COLUMN params_json;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN optimal_params_json;
