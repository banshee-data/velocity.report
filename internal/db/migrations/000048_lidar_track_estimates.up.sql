-- Migration: versioned LiDAR track state records
-- Date: 2026-09-12
-- Description: Derived estimator output references immutable L4 evidence; it
-- never replaces lidar_observations or legacy lidar_track_observations.
-- creation_sequence is the tracker's existing deterministic per-run ordinal
-- (already computed, never persisted before this table existed). track_id is
-- a random UUID (internal/lidar/l5tracks/tracking.go) assigned that way
-- deliberately so it stays collision-free across tracker resets, server
-- restarts, and long-running deployments, which makes it unusable as a
-- content key for verifying two replays produced the same evidence: an
-- identical run assigns different random track IDs each time.
-- creation_sequence lets a semantic comparison group estimates by track
-- identity without depending on that random label.
   CREATE TABLE IF NOT EXISTS lidar_track_estimates (
          estimate_id TEXT PRIMARY KEY
        , track_id TEXT NOT NULL
        , creation_sequence INTEGER NOT NULL DEFAULT 0
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
