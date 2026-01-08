-- Migration 000013: Remove algorithm comparison tables
     DROP TABLE IF EXISTS lidar_algorithm_frame_results;

     DROP TABLE IF EXISTS lidar_algorithm_runs;
