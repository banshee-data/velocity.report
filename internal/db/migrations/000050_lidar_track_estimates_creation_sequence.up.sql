-- Migration: deterministic per-track creation sequence on estimates
-- Date: 2026-09-15
-- Description: track_id is a random UUID (internal/lidar/l5tracks/tracking.go)
-- assigned that way deliberately so it stays collision-free across tracker
-- resets server restarts and long-running deployments. That makes it
-- unusable as a content key for verifying two replays produced the same
-- evidence: an identical run assigns different random track IDs each time.
-- creation_sequence is the tracker's existing deterministic per-run ordinal
-- (already computed never persisted); recording it lets a semantic
-- comparison group estimates by track identity without depending on the
-- random label.
    ALTER TABLE lidar_track_estimates
      ADD COLUMN creation_sequence INTEGER NOT NULL DEFAULT 0;
