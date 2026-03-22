PRAGMA foreign_keys = OFF;

   CREATE TABLE lidar_replay_annotations (
          annotation_id TEXT PRIMARY KEY
        , replay_case_id TEXT
        , run_id TEXT
        , track_id TEXT
        , legacy_track_id TEXT
        , class_label TEXT NOT NULL
        , start_timestamp_ns INTEGER NOT NULL
        , end_timestamp_ns INTEGER
        , confidence REAL
        , created_by TEXT
        , created_at_ns INTEGER NOT NULL
        , updated_at_ns INTEGER
        , notes TEXT
        , source_file TEXT
        , CHECK (
          (
          run_id IS NULL
      AND track_id IS NULL
          )
       OR (
          run_id IS NOT NULL
      AND track_id IS NOT NULL
          )
          )
        , CHECK (
          end_timestamp_ns IS NULL
       OR end_timestamp_ns >= start_timestamp_ns
          )
        , CHECK (
          confidence IS NULL
       OR (
          confidence >= 0
      AND confidence <= 1
          )
          )
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
        , FOREIGN KEY (run_id, track_id) REFERENCES lidar_run_tracks (run_id, track_id) ON DELETE SET NULL
          );

   INSERT INTO lidar_replay_annotations (
          annotation_id
        , replay_case_id
        , run_id
        , track_id
        , legacy_track_id
        , class_label
        , start_timestamp_ns
        , end_timestamp_ns
        , confidence
        , created_by
        , created_at_ns
        , updated_at_ns
        , notes
        , source_file
          )
   SELECT label_id
        , CASE
                    WHEN replay_case_id IS NOT NULL
                          AND EXISTS (
                                 SELECT 1
                                   FROM lidar_replay_cases
                                  WHERE lidar_replay_cases.replay_case_id = lidar_track_annotations.replay_case_id
                              ) THEN replay_case_id
                              ELSE NULL
          END
        , NULL
        , NULL
        , track_id
        , class_label
        , start_timestamp_ns
        , end_timestamp_ns
        , confidence
        , created_by
        , created_at_ns
        , updated_at_ns
        , notes
        , source_file
     FROM lidar_track_annotations;

     DROP INDEX IF EXISTS idx_lidar_track_annotations_replay_case;

     DROP INDEX IF EXISTS idx_lidar_track_annotations_track;

     DROP INDEX IF EXISTS idx_lidar_track_annotations_time;

     DROP INDEX IF EXISTS idx_lidar_track_annotations_class;

     DROP TABLE IF EXISTS lidar_track_annotations;

CREATE INDEX idx_lidar_replay_annotations_replay_case ON lidar_replay_annotations (replay_case_id);

CREATE INDEX idx_lidar_replay_annotations_run_track ON lidar_replay_annotations (run_id, track_id);

CREATE INDEX idx_lidar_replay_annotations_track ON lidar_replay_annotations (track_id);

CREATE INDEX idx_lidar_replay_annotations_time ON lidar_replay_annotations (start_timestamp_ns, end_timestamp_ns);

CREATE INDEX idx_lidar_replay_annotations_class ON lidar_replay_annotations (class_label);

     DROP INDEX IF EXISTS idx_replay_evaluations_pair;

   CREATE TABLE lidar_replay_evaluations_new (
          evaluation_id TEXT PRIMARY KEY
        , replay_case_id TEXT NOT NULL
        , reference_run_id TEXT NOT NULL
        , candidate_run_id TEXT NOT NULL
        , detection_rate REAL
        , fragmentation REAL
        , false_positive_rate REAL
        , velocity_coverage REAL
        , quality_premium REAL
        , truncation_rate REAL
        , velocity_noise_rate REAL
        , stopped_recovery_rate REAL
        , composite_score REAL
        , matched_count INTEGER
        , reference_count INTEGER
        , candidate_count INTEGER
        , params_json TEXT
        , created_at INTEGER NOT NULL
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
        , FOREIGN KEY (reference_run_id) REFERENCES lidar_run_records (run_id) ON DELETE CASCADE
        , FOREIGN KEY (candidate_run_id) REFERENCES lidar_run_records (run_id) ON DELETE CASCADE
          );

   INSERT INTO lidar_replay_evaluations_new (
          evaluation_id
        , replay_case_id
        , reference_run_id
        , candidate_run_id
        , detection_rate
        , fragmentation
        , false_positive_rate
        , velocity_coverage
        , quality_premium
        , truncation_rate
        , velocity_noise_rate
        , stopped_recovery_rate
        , composite_score
        , matched_count
        , reference_count
        , candidate_count
        , params_json
        , created_at
          )
   SELECT evaluation_id
        , replay_case_id
        , reference_run_id
        , candidate_run_id
        , detection_rate
        , fragmentation
        , false_positive_rate
        , velocity_coverage
        , quality_premium
        , truncation_rate
        , velocity_noise_rate
        , stopped_recovery_rate
        , composite_score
        , matched_count
        , reference_count
        , candidate_count
        , params_json
        , created_at
     FROM lidar_replay_evaluations;

     DROP TABLE lidar_replay_evaluations;

    ALTER TABLE lidar_replay_evaluations_new
RENAME TO lidar_replay_evaluations;

CREATE UNIQUE INDEX idx_replay_evaluations_pair ON lidar_replay_evaluations (replay_case_id, reference_run_id, candidate_run_id);

PRAGMA foreign_keys = ON;
