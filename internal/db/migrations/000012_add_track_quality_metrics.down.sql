-- Rollback Phase 1: Track Quality Metrics
-- Drop index
     DROP INDEX IF EXISTS idx_lidar_tracks_quality;

-- Note: SQLite doesn't support DROP COLUMN, so we would need to recreate tables
-- For development purposes, we'll document the columns that were added:
-- lidar_tracks: track_length_meters, track_duration_secs, occlusion_count
--               max_occlusion_frames, spatial_coverage, noise_point_ratio
-- lidar_clusters: noise_points_count, cluster_density, aspect_ratio
-- lidar_analysis_runs: statistics_json
-- In production, migration down would require table recreation with data preservation
