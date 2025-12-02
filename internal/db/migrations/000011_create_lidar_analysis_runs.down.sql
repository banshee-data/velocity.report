-- Rollback Phase 3.7: Analysis Run Infrastructure
     DROP INDEX IF EXISTS idx_lidar_run_tracks_state;

     DROP INDEX IF EXISTS idx_lidar_run_tracks_label;

     DROP INDEX IF EXISTS idx_lidar_run_tracks_class;

     DROP INDEX IF EXISTS idx_lidar_run_tracks_run;

     DROP TABLE IF EXISTS lidar_run_tracks;

     DROP INDEX IF EXISTS idx_lidar_runs_status;

     DROP INDEX IF EXISTS idx_lidar_runs_parent;

     DROP INDEX IF EXISTS idx_lidar_runs_source;

     DROP INDEX IF EXISTS idx_lidar_runs_created;

     DROP TABLE IF EXISTS lidar_analysis_runs;
