-- Create lidar_transits table for polished transit data
-- Analogous to radar_data_transits but for LiDAR-tracked objects
CREATE TABLE IF NOT EXISTS lidar_transits (
    transit_id INTEGER PRIMARY KEY AUTOINCREMENT,
    track_id TEXT NOT NULL UNIQUE,
    sensor_id TEXT NOT NULL,
    transit_start_unix DOUBLE NOT NULL,
    transit_end_unix DOUBLE NOT NULL,
    max_speed_mps REAL,
    min_speed_mps REAL,
    avg_speed_mps REAL,
    p50_speed_mps REAL,
    p85_speed_mps REAL,
    p95_speed_mps REAL,
    track_length_m REAL,
    observation_count INTEGER,
    object_class TEXT,
    classification_confidence REAL,
    quality_score REAL,
    bbox_length_avg REAL,
    bbox_width_avg REAL,
    bbox_height_avg REAL,
    created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
);

CREATE INDEX IF NOT EXISTS idx_lidar_transits_time ON lidar_transits(transit_start_unix, transit_end_unix);
CREATE INDEX IF NOT EXISTS idx_lidar_transits_sensor ON lidar_transits(sensor_id);
CREATE INDEX IF NOT EXISTS idx_lidar_transits_class ON lidar_transits(object_class);
CREATE INDEX IF NOT EXISTS idx_lidar_transits_speed ON lidar_transits(p85_speed_mps);
