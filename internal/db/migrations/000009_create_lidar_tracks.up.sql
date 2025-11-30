-- Phase 3.3: LIDAR Tracks Schema
-- Clusters detected via DBSCAN (world frame)
CREATE TABLE IF NOT EXISTS lidar_clusters (
    lidar_cluster_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    pose_id INTEGER NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    
    -- World frame position (meters)
    centroid_x REAL,
    centroid_y REAL,
    centroid_z REAL,
    
    -- Bounding box (world frame, meters)
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL,
    
    -- Cluster features
    points_count INTEGER,
    height_p95 REAL,
    intensity_mean REAL
);

CREATE INDEX IF NOT EXISTS idx_lidar_clusters_sensor_time ON lidar_clusters(sensor_id, ts_unix_nanos);

-- Tracks (world frame)
CREATE TABLE IF NOT EXISTS lidar_tracks (
    track_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    pose_id INTEGER NOT NULL,
    track_state TEXT NOT NULL, -- 'tentative', 'confirmed', 'deleted'
    
    -- Lifecycle
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,
    
    -- Kinematics (world frame)
    avg_speed_mps REAL,
    peak_speed_mps REAL,
    p50_speed_mps REAL,  -- Median speed
    p85_speed_mps REAL,  -- 85th percentile
    p95_speed_mps REAL,  -- 95th percentile
    
    -- Shape features (world frame averages)
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,
    intensity_mean_avg REAL,
    
    -- Classification (Phase 3.4)
    object_class TEXT,           -- 'pedestrian', 'car', 'bird', 'other'
    object_confidence REAL,
    classification_model TEXT    -- Model version used for classification
);

CREATE INDEX IF NOT EXISTS idx_lidar_tracks_sensor ON lidar_tracks(sensor_id);
CREATE INDEX IF NOT EXISTS idx_lidar_tracks_state ON lidar_tracks(track_state);
CREATE INDEX IF NOT EXISTS idx_lidar_tracks_time ON lidar_tracks(start_unix_nanos, end_unix_nanos);
CREATE INDEX IF NOT EXISTS idx_lidar_tracks_class ON lidar_tracks(object_class);

-- Track observations (world frame)
CREATE TABLE IF NOT EXISTS lidar_track_obs (
    track_id TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    world_frame TEXT NOT NULL,
    pose_id INTEGER NOT NULL,
    
    -- Position (world frame, meters)
    x REAL,
    y REAL,
    z REAL,
    
    -- Velocity (world frame, m/s)
    velocity_x REAL,
    velocity_y REAL,
    speed_mps REAL,
    heading_rad REAL,
    
    -- Shape (world frame)
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL,
    height_p95 REAL,
    intensity_mean REAL,
    
    PRIMARY KEY (track_id, ts_unix_nanos),
    FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_lidar_track_obs_track ON lidar_track_obs(track_id);
CREATE INDEX IF NOT EXISTS idx_lidar_track_obs_time ON lidar_track_obs(ts_unix_nanos);
