-- L5 track children
ALTER TABLE lidar_track_obs RENAME TO lidar_track_observations;
ALTER TABLE lidar_labels RENAME TO lidar_track_annotations;
ALTER TABLE lidar_track_annotations RENAME COLUMN scene_id TO replay_case_id;
DROP INDEX IF EXISTS idx_lidar_labels_scene;
CREATE INDEX idx_lidar_track_annotations_replay_case ON lidar_track_annotations (replay_case_id);

-- L8 run family
ALTER TABLE lidar_analysis_runs RENAME TO lidar_run_records;
ALTER TABLE lidar_missed_regions RENAME TO lidar_run_missed_regions;

-- L8 replay family
ALTER TABLE lidar_scenes RENAME TO lidar_replay_cases;
ALTER TABLE lidar_replay_cases RENAME COLUMN scene_id TO replay_case_id;
ALTER TABLE lidar_evaluations RENAME TO lidar_replay_evaluations;
ALTER TABLE lidar_replay_evaluations RENAME COLUMN scene_id TO replay_case_id;

-- L8 tuning
ALTER TABLE lidar_sweeps RENAME TO lidar_tuning_sweeps;
