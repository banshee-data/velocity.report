-- L5 track children
ALTER TABLE lidar_track_obs RENAME TO lidar_track_observations;
ALTER TABLE lidar_labels RENAME TO lidar_track_annotations;
ALTER TABLE lidar_track_annotations RENAME COLUMN scene_id TO replay_case_id;
DROP INDEX IF EXISTS idx_lidar_labels_scene;
CREATE INDEX idx_lidar_track_annotations_replay_case ON lidar_track_annotations (replay_case_id);
DROP INDEX IF EXISTS idx_lidar_track_obs_track;
CREATE INDEX idx_lidar_track_observations_track ON lidar_track_observations (track_id);
DROP INDEX IF EXISTS idx_lidar_track_obs_time;
CREATE INDEX idx_lidar_track_observations_time ON lidar_track_observations (ts_unix_nanos);
DROP INDEX IF EXISTS idx_lidar_labels_track;
CREATE INDEX idx_lidar_track_annotations_track ON lidar_track_annotations (track_id);
DROP INDEX IF EXISTS idx_lidar_labels_time;
CREATE INDEX idx_lidar_track_annotations_time ON lidar_track_annotations (start_timestamp_ns, end_timestamp_ns);
DROP INDEX IF EXISTS idx_lidar_labels_class;
CREATE INDEX idx_lidar_track_annotations_class ON lidar_track_annotations (class_label);

-- L8 run family
ALTER TABLE lidar_analysis_runs RENAME TO lidar_run_records;
ALTER TABLE lidar_missed_regions RENAME TO lidar_run_missed_regions;
DROP INDEX IF EXISTS idx_missed_regions_run_id;
CREATE INDEX idx_lidar_run_missed_regions_run_id ON lidar_run_missed_regions (run_id);

-- L8 replay family
ALTER TABLE lidar_scenes RENAME TO lidar_replay_cases;
ALTER TABLE lidar_replay_cases RENAME COLUMN scene_id TO replay_case_id;
ALTER TABLE lidar_evaluations RENAME TO lidar_replay_evaluations;
ALTER TABLE lidar_replay_evaluations RENAME COLUMN scene_id TO replay_case_id;
DROP INDEX IF EXISTS idx_lidar_scenes_sensor;
CREATE INDEX idx_lidar_replay_cases_sensor ON lidar_replay_cases (sensor_id);
DROP INDEX IF EXISTS idx_lidar_scenes_pcap;
CREATE INDEX idx_lidar_replay_cases_pcap ON lidar_replay_cases (pcap_file);
DROP INDEX IF EXISTS idx_evaluations_pair;
CREATE UNIQUE INDEX idx_replay_evaluations_pair ON lidar_replay_evaluations (reference_run_id, candidate_run_id);

-- L8 tuning
ALTER TABLE lidar_sweeps RENAME TO lidar_tuning_sweeps;
DROP INDEX IF EXISTS idx_lidar_sweeps_sensor;
CREATE INDEX idx_lidar_tuning_sweeps_sensor ON lidar_tuning_sweeps (sensor_id);
DROP INDEX IF EXISTS idx_lidar_sweeps_status;
CREATE INDEX idx_lidar_tuning_sweeps_status ON lidar_tuning_sweeps (status);
