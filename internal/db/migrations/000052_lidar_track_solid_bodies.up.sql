-- Migration: solid-body track estimates behind the state_model discriminator
-- Date: 2026-09-25
-- Description: A solid-body estimate (state-estimation plan Section 5.4) is a
-- different claim from the point estimate in lidar_track_estimates. Its
-- position is a named place on the body, and it carries orientation and
-- extent beliefs with provenance, the estimation lifecycle and the support
-- state. It gets its own table so that no reader of point estimates can mix
-- the two. Both tables carry state_model: the dynamic state layout the
-- covariance blob is written in, so that a reader never infers dimensionality
-- from the length of the blob. Every existing lidar_track_estimates row is
-- the four-state CV layout cv_cartesian_v1 and the default records that.
-- Derived estimator output references immutable L4 evidence and never
-- replaces lidar_observations.
    ALTER TABLE lidar_track_estimates
      ADD COLUMN state_model TEXT NOT NULL DEFAULT 'cv_cartesian_v1';

   CREATE TABLE IF NOT EXISTS lidar_track_solid_bodies (
          estimate_id TEXT PRIMARY KEY
        , track_id TEXT NOT NULL
        , creation_sequence INTEGER NOT NULL
        , observation_id TEXT NOT NULL
        , source_id TEXT NOT NULL
        , calibration_id TEXT NOT NULL
        , frame_unix_nanos INTEGER NOT NULL
        , measurement_unix_nanos INTEGER NOT NULL
        , estimator_id TEXT NOT NULL
        , observation_model_id TEXT NOT NULL
        , param_hash TEXT NOT NULL
        , stage TEXT NOT NULL
        , state_model TEXT NOT NULL
        , reference_point TEXT NOT NULL
        , x REAL NOT NULL
        , y REAL NOT NULL
        , vx REAL NOT NULL
        , vy REAL NOT NULL
        , covariance_json BLOB NOT NULL
        , heading_rad REAL NOT NULL
        , heading_variance_rad2 REAL NOT NULL
        , heading_ambiguous_weight REAL NOT NULL
        , heading_provenance TEXT NOT NULL
        , length_m REAL NOT NULL
        , length_sigma_m REAL NOT NULL
        , length_frames INTEGER NOT NULL
        , length_provenance TEXT NOT NULL
        , width_m REAL NOT NULL
        , width_sigma_m REAL NOT NULL
        , width_frames INTEGER NOT NULL
        , width_provenance TEXT NOT NULL
        , height_m REAL NOT NULL
        , height_sigma_m REAL NOT NULL
        , height_frames INTEGER NOT NULL
        , height_provenance TEXT NOT NULL
        , ground_z REAL NOT NULL
        , ground_surface_model TEXT NOT NULL
        , motion_class TEXT NOT NULL
        , motion_posterior REAL NOT NULL
        , estimation_state TEXT NOT NULL
        , last_observed_unix_nanos INTEGER NOT NULL
        , support_points INTEGER NOT NULL
        , coasted_frames INTEGER NOT NULL
        , measurement_source TEXT NOT NULL
        , measurement_rank INTEGER NOT NULL
        , visible_faces TEXT NOT NULL
        , inferred_extent INTEGER NOT NULL
        , aspect_rad REAL
        , nis REAL NOT NULL
        , fallback_reason TEXT NOT NULL
        , inserted_at_ns INTEGER NOT NULL
        , UNIQUE (
          track_id
        , estimator_id
        , observation_model_id
        , param_hash
        , stage
        , frame_unix_nanos
          )
        , CHECK (measurement_rank BETWEEN 0 AND 2)
          );

CREATE INDEX IF NOT EXISTS idx_lidar_track_solid_bodies_observation ON lidar_track_solid_bodies (observation_id, estimator_id, stage);
