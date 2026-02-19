CREATE TABLE IF NOT EXISTS lidar_evaluations (
    evaluation_id       TEXT PRIMARY KEY,
    scene_id            TEXT NOT NULL,
    reference_run_id    TEXT NOT NULL,
    candidate_run_id    TEXT NOT NULL,
    detection_rate      REAL,
    fragmentation       REAL,
    false_positive_rate REAL,
    velocity_coverage   REAL,
    quality_premium     REAL,
    truncation_rate     REAL,
    velocity_noise_rate REAL,
    stopped_recovery_rate REAL,
    composite_score     REAL,
    matched_count       INTEGER,
    reference_count     INTEGER,
    candidate_count     INTEGER,
    params_json         TEXT,
    created_at          INTEGER NOT NULL,
    FOREIGN KEY (scene_id) REFERENCES lidar_scenes(scene_id) ON DELETE CASCADE,
    FOREIGN KEY (reference_run_id) REFERENCES lidar_analysis_runs(run_id),
    FOREIGN KEY (candidate_run_id) REFERENCES lidar_analysis_runs(run_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_evaluations_pair ON lidar_evaluations(reference_run_id, candidate_run_id);
