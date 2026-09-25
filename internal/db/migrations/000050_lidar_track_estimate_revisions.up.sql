-- Migration: revision records for retrospectively refined LiDAR estimates
-- Date: 2026-09-25
-- Description: One row per fixed_lag or final estimate in
-- lidar_track_estimates, saying which online estimate it revised, by how much,
-- with which look-ahead, and on what evidence (state-estimation plan §10.1 and
-- G-SMO-1: a revision records its justifying evidence). The previous state is
-- copied, not only referenced, because online estimates are retained for 7
-- days and refined ones indefinitely (plan §11.1). Under fixed assignment the
-- evidence is exactly the track's observed steps between the two evidence
-- frame bounds; a revisable-association worker needs explicit lists in its
-- own record. Nothing here modifies an estimate, residual or observation.
   CREATE TABLE IF NOT EXISTS lidar_track_estimate_revisions (
          estimate_id TEXT PRIMARY KEY
        , revises_estimate_id TEXT NOT NULL
        , smoother_id TEXT NOT NULL
        , lag TEXT NOT NULL
        , lookahead_steps INTEGER NOT NULL
        , lookahead_secs REAL NOT NULL
        , released_at_unix_nanos INTEGER NOT NULL
        , release_reason TEXT NOT NULL
        , chain_end_reason TEXT NOT NULL
        , flags TEXT NOT NULL
        , previous_x REAL NOT NULL
        , previous_y REAL NOT NULL
        , previous_vx REAL NOT NULL
        , previous_vy REAL NOT NULL
        , revision_position_m REAL NOT NULL
        , revision_velocity_mps REAL NOT NULL
        , evidence_count INTEGER NOT NULL
        , evidence_first_frame_unix_nanos INTEGER NOT NULL
        , evidence_last_frame_unix_nanos INTEGER NOT NULL
        , strongest_evidence_observation_id TEXT NOT NULL
        , strongest_evidence_nis REAL NOT NULL
        , inserted_at_ns INTEGER NOT NULL
          );

CREATE INDEX IF NOT EXISTS idx_lidar_track_estimate_revisions_revises ON lidar_track_estimate_revisions (revises_estimate_id);
