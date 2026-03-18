-- L8 tuning
DROP INDEX IF EXISTS idx_lidar_tuning_sweeps_sensor;
DROP INDEX IF EXISTS idx_lidar_tuning_sweeps_status;
ALTER TABLE lidar_tuning_sweeps RENAME TO lidar_sweeps;
CREATE INDEX idx_lidar_sweeps_sensor ON lidar_sweeps (sensor_id);
CREATE INDEX idx_lidar_sweeps_status ON lidar_sweeps (status);

-- L8 replay family
DROP INDEX IF EXISTS idx_replay_evaluations_pair;
DROP INDEX IF EXISTS idx_lidar_replay_cases_sensor;
DROP INDEX IF EXISTS idx_lidar_replay_cases_pcap;
ALTER TABLE lidar_replay_evaluations RENAME COLUMN replay_case_id TO scene_id;
ALTER TABLE lidar_replay_evaluations RENAME TO lidar_evaluations;
ALTER TABLE lidar_replay_cases RENAME COLUMN replay_case_id TO scene_id;
ALTER TABLE lidar_replay_cases RENAME TO lidar_scenes;
CREATE INDEX idx_lidar_scenes_sensor ON lidar_scenes (sensor_id);
CREATE INDEX idx_lidar_scenes_pcap ON lidar_scenes (pcap_file);
CREATE UNIQUE INDEX idx_evaluations_pair ON lidar_evaluations (reference_run_id, candidate_run_id);

-- L8 run family
DROP INDEX IF EXISTS idx_lidar_run_missed_regions_run_id;
ALTER TABLE lidar_run_missed_regions RENAME TO lidar_missed_regions;
ALTER TABLE lidar_run_records RENAME TO lidar_analysis_runs;
CREATE INDEX idx_missed_regions_run_id ON lidar_missed_regions (run_id);

-- L5 track children
DROP INDEX IF EXISTS idx_lidar_track_annotations_replay_case;
DROP INDEX IF EXISTS idx_lidar_track_annotations_track;
DROP INDEX IF EXISTS idx_lidar_track_annotations_time;
DROP INDEX IF EXISTS idx_lidar_track_annotations_class;
DROP INDEX IF EXISTS idx_lidar_track_observations_track;
DROP INDEX IF EXISTS idx_lidar_track_observations_time;
ALTER TABLE lidar_track_annotations RENAME COLUMN replay_case_id TO scene_id;
ALTER TABLE lidar_track_annotations RENAME TO lidar_labels;
CREATE INDEX idx_lidar_labels_scene ON lidar_labels (scene_id);
CREATE INDEX idx_lidar_labels_track ON lidar_labels (track_id);
CREATE INDEX idx_lidar_labels_time ON lidar_labels (start_timestamp_ns, end_timestamp_ns);
CREATE INDEX idx_lidar_labels_class ON lidar_labels (class_label);
ALTER TABLE lidar_track_observations RENAME TO lidar_track_obs;
CREATE INDEX idx_lidar_track_obs_track ON lidar_track_obs (track_id);
CREATE INDEX idx_lidar_track_obs_time ON lidar_track_obs (ts_unix_nanos);
