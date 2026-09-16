-- Migration: versioned LiDAR track state and residual records
-- Date: 2026-09-12
-- Description: Derived estimator output references immutable L4 evidence;
-- it never replaces lidar_observations or legacy lidar_track_observations.
   CREATE TABLE IF NOT EXISTS lidar_track_estimates (
          estimate_id TEXT PRIMARY KEY
        , track_id TEXT NOT NULL
        , observation_id TEXT NOT NULL
        , source_id TEXT NOT NULL
        , calibration_id TEXT NOT NULL
        , frame_unix_nanos INTEGER NOT NULL
        , measurement_unix_nanos INTEGER NOT NULL
        , estimator_id TEXT NOT NULL
        , observation_model_id TEXT NOT NULL
        , param_hash TEXT NOT NULL
        , stage TEXT NOT NULL
        , measurement_source TEXT NOT NULL
        , x REAL NOT NULL
        , y REAL NOT NULL
        , vx REAL NOT NULL
        , vy REAL NOT NULL
        , covariance_json BLOB NOT NULL
        , inserted_at_ns INTEGER NOT NULL
        , UNIQUE (
          track_id
        , estimator_id
        , observation_model_id
        , param_hash
        , stage
        , frame_unix_nanos
          )
          );

CREATE INDEX IF NOT EXISTS idx_lidar_track_estimates_observation ON lidar_track_estimates (observation_id, estimator_id, stage);

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
