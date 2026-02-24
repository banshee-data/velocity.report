-- Phase 1: Track Quality Metrics
-- Add quality metrics columns to existing tables
-- Track quality metrics
    ALTER TABLE lidar_tracks
      ADD COLUMN track_length_meters REAL;

    ALTER TABLE lidar_tracks
      ADD COLUMN track_duration_secs REAL;

    ALTER TABLE lidar_tracks
      ADD COLUMN occlusion_count INTEGER DEFAULT 0;

    ALTER TABLE lidar_tracks
      ADD COLUMN max_occlusion_frames INTEGER DEFAULT 0;

    ALTER TABLE lidar_tracks
      ADD COLUMN spatial_coverage REAL;

    ALTER TABLE lidar_tracks
      ADD COLUMN noise_point_ratio REAL;

-- Cluster quality metrics
    ALTER TABLE lidar_clusters
      ADD COLUMN noise_points_count INTEGER DEFAULT 0;

    ALTER TABLE lidar_clusters
      ADD COLUMN cluster_density REAL;

    ALTER TABLE lidar_clusters
      ADD COLUMN aspect_ratio REAL;

-- Analysis run aggregate statistics (JSON field for extensibility)
    ALTER TABLE lidar_analysis_runs
      ADD COLUMN statistics_json TEXT;

-- Create index on quality metrics for filtering high-quality tracks
CREATE INDEX IF NOT EXISTS idx_lidar_tracks_quality ON lidar_tracks (track_length_meters, occlusion_count);
