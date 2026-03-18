-- L8 tuning
ALTER TABLE lidar_tuning_sweeps RENAME TO lidar_sweeps;

-- L8 replay family
ALTER TABLE lidar_replay_evaluations RENAME COLUMN replay_case_id TO scene_id;
ALTER TABLE lidar_replay_evaluations RENAME TO lidar_evaluations;
ALTER TABLE lidar_replay_cases RENAME COLUMN replay_case_id TO scene_id;
ALTER TABLE lidar_replay_cases RENAME TO lidar_scenes;

-- L8 run family
ALTER TABLE lidar_run_missed_regions RENAME TO lidar_missed_regions;
ALTER TABLE lidar_run_records RENAME TO lidar_analysis_runs;

-- L5 track children
DROP INDEX IF EXISTS idx_lidar_track_annotations_replay_case;
ALTER TABLE lidar_track_annotations RENAME COLUMN replay_case_id TO scene_id;
ALTER TABLE lidar_track_annotations RENAME TO lidar_labels;
CREATE INDEX idx_lidar_labels_scene ON lidar_labels (scene_id);
ALTER TABLE lidar_track_observations RENAME TO lidar_track_obs;
