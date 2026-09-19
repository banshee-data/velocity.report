-- Migration: LiDAR track filter residual records
-- Date: 2026-09-12
-- Description: Per-update innovation and NIS evidence, keyed to the estimate
-- it was computed for. Derived estimator output references immutable L4
-- evidence; it never replaces lidar_observations or legacy
-- lidar_track_observations.
   CREATE TABLE IF NOT EXISTS lidar_track_residuals (
          estimate_id TEXT PRIMARY KEY
        , observation_id TEXT NOT NULL
        , predicted_x REAL NOT NULL
        , predicted_y REAL NOT NULL
        , measurement_x REAL NOT NULL
        , measurement_y REAL NOT NULL
        , innovation_x REAL NOT NULL
        , innovation_y REAL NOT NULL
        , nis REAL NOT NULL
        , geometry_cov_xx REAL NOT NULL
        , geometry_cov_xy REAL NOT NULL
        , geometry_cov_yy REAL NOT NULL
        , disposition TEXT NOT NULL
        , reason TEXT NOT NULL
        , inserted_at_ns INTEGER NOT NULL
          );

CREATE INDEX IF NOT EXISTS idx_lidar_track_residuals_observation ON lidar_track_residuals (observation_id, estimate_id);
