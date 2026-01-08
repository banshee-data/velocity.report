-- Migration 000013: Add algorithm comparison tables
-- These tables support A/B algorithm evaluation and comparison

-- Algorithm comparison runs (metadata for a comparison session)
CREATE TABLE IF NOT EXISTS lidar_algorithm_runs (
    run_id TEXT PRIMARY KEY,
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    
    -- Configuration
    algorithms_json TEXT,         -- JSON array of algorithm names
    params_json TEXT,             -- JSON object of per-algorithm params
    pcap_file TEXT,               -- Source PCAP if replay (NULL for live)
    
    -- Aggregate metrics
    total_frames INTEGER DEFAULT 0,
    total_processing_time_us INTEGER DEFAULT 0,
    
    -- Summary statistics (JSON)
    summary_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_algorithm_runs_start ON lidar_algorithm_runs(start_unix_nanos);

-- Per-frame comparison results
CREATE TABLE IF NOT EXISTS lidar_algorithm_frame_results (
    run_id TEXT NOT NULL,
    frame_unix_nanos INTEGER NOT NULL,
    algorithm_name TEXT NOT NULL,
    
    -- Metrics
    foreground_count INTEGER,
    background_count INTEGER,
    cluster_count INTEGER,
    processing_time_us INTEGER,
    
    -- Optional: precision/recall if ground truth available
    precision REAL,
    recall REAL,
    
    -- Algorithm-specific metrics (JSON)
    extra_json TEXT,
    
    PRIMARY KEY (run_id, frame_unix_nanos, algorithm_name),
    FOREIGN KEY (run_id) REFERENCES lidar_algorithm_runs(run_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_frame_results_run ON lidar_algorithm_frame_results(run_id);
CREATE INDEX IF NOT EXISTS idx_frame_results_algo ON lidar_algorithm_frame_results(algorithm_name);
CREATE INDEX IF NOT EXISTS idx_frame_results_time ON lidar_algorithm_frame_results(frame_unix_nanos);
