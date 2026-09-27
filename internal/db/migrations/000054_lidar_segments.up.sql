CREATE TABLE lidar_segment_selections (
          segment_id TEXT PRIMARY KEY
        , run_id TEXT NOT NULL
        , replay_case_id TEXT NOT NULL UNIQUE
        , role TEXT NOT NULL CHECK (role IN ('tuning', 'held_out'))
        , finder TEXT NOT NULL
        , parameters_json TEXT NOT NULL
        , window_json TEXT NOT NULL
        , created_at_ns INTEGER NOT NULL
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
          );

   CREATE TABLE lidar_segment_clip_jobs (
          job_id TEXT PRIMARY KEY
        , segment_id TEXT NOT NULL
        , replay_case_id TEXT NOT NULL
        , pack_dir TEXT NOT NULL DEFAULT ''
        , FOREIGN KEY (job_id) REFERENCES lidar_capture_jobs (job_id) ON DELETE CASCADE
        , FOREIGN KEY (segment_id) REFERENCES lidar_segment_selections (segment_id) ON DELETE CASCADE
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
          );
