-- Create lidar_scenes table for managing PCAP-based evaluation scenes
-- A scene ties a PCAP file to a sensor, reference run, and optimal parameters
CREATE TABLE IF NOT EXISTS lidar_scenes (
    scene_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    pcap_file TEXT NOT NULL,
    pcap_start_secs REAL,
    pcap_duration_secs REAL,
    description TEXT,
    reference_run_id TEXT,
    optimal_params_json TEXT,
    created_at_ns INTEGER NOT NULL,
    updated_at_ns INTEGER,
    FOREIGN KEY (reference_run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_lidar_scenes_sensor ON lidar_scenes(sensor_id);
CREATE INDEX IF NOT EXISTS idx_lidar_scenes_pcap ON lidar_scenes(pcap_file);
