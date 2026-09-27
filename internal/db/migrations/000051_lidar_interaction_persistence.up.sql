-- Migration: LiDAR behaviour interaction persistence
-- Date: 2026-09-25
-- Description: Following encounters as three derived, versioned tables, per
-- Section 10.3 of docs/plans/lidar-behaviour-analytics-plan.md. Each row is
-- JSON-first: the *_json column holds the authoritative
-- internal/lidar/l8behaviour record, and the generated columns index what
-- queries select on. Metric values are keyed inside the JSON by registry
-- metric id and suppressions by reason token; no column names a metric.
--
-- lidar_interaction_events: one row per pairwise encounter per version,
-- indexed on both track ids and on interaction_type. event_id digests the
-- source, type, pair and every version axis (estimate stage, estimator,
-- observation model, behaviour method with its parameter hash, geometry and
-- estimator parameter hash), so regeneration under a new version writes new
-- rows beside the old and never overwrites them; a reader selects one
-- version through the version index.
--
-- lidar_interaction_instants: an event's per-instant suppression history,
-- physical endpoints and series values. basis separates instants where both
-- parties were observed from predicted-only ones, and a CHECK refuses a
-- valid predicted-only instant.
--
-- lidar_exposure_windows: opportunity denominators, kept apart so a rate can
-- be recomputed without re-running the pipeline. Only basis 'observed' is
-- opportunity; a 'predicted_only' window records time a prediction stood in
-- for observation and never enters a rate.
--
-- Every row is derived from persisted estimates and can be regenerated;
-- rolling back loses no evidence.
   CREATE TABLE IF NOT EXISTS lidar_interaction_events (
          event_id TEXT PRIMARY KEY
        , event_json JSON NOT NULL
        , source_id TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.source_id')) STORED
        , interaction_type TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.interaction_type')) STORED
        , primary_track_id TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.primary_track_id')) STORED
        , secondary_track_id TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.secondary_track_id')) STORED
        , start_unix_nanos INTEGER NOT NULL AS (JSON_EXTRACT(event_json, '$.start_unix_nanos')) STORED
        , end_unix_nanos INTEGER NOT NULL AS (JSON_EXTRACT(event_json, '$.end_unix_nanos')) STORED
        , estimate_stage TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.version.estimate_stage')) STORED
        , estimator_id TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.version.estimator_id')) STORED
        , obs_model_id TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.version.obs_model_id')) STORED
        , method_id TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.version.method_id')) STORED
        , param_hash TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.version.param_hash')) STORED
        , worst_support TEXT NOT NULL AS (JSON_EXTRACT(event_json, '$.worst_support')) STORED
        , inserted_at_ns INTEGER NOT NULL
        , CHECK (event_id = JSON_EXTRACT(event_json, '$.event_id'))
          );

CREATE INDEX IF NOT EXISTS idx_lidar_interaction_events_version ON lidar_interaction_events (source_id, estimate_stage, estimator_id, obs_model_id, method_id, param_hash);

CREATE INDEX IF NOT EXISTS idx_lidar_interaction_events_primary ON lidar_interaction_events (primary_track_id, interaction_type);

CREATE INDEX IF NOT EXISTS idx_lidar_interaction_events_secondary ON lidar_interaction_events (secondary_track_id, interaction_type);

CREATE INDEX IF NOT EXISTS idx_lidar_interaction_events_type ON lidar_interaction_events (interaction_type, start_unix_nanos);

   CREATE TABLE IF NOT EXISTS lidar_interaction_instants (
          event_id TEXT NOT NULL REFERENCES lidar_interaction_events (event_id) ON DELETE CASCADE
        , capture_unix_nanos INTEGER NOT NULL
        , instant_json JSON NOT NULL
        , basis TEXT NOT NULL AS (JSON_EXTRACT(instant_json, '$.basis')) STORED
        , valid INTEGER NOT NULL AS (JSON_EXTRACT(instant_json, '$.valid')) STORED
        , reason TEXT AS (JSON_EXTRACT(instant_json, '$.reason')) STORED
        , PRIMARY KEY (event_id, capture_unix_nanos)
        , CHECK (
          event_id = JSON_EXTRACT(instant_json, '$.event_id')
          AND capture_unix_nanos = JSON_EXTRACT(instant_json, '$.capture_unix_nanos')
          )
        , CHECK (
          basis = 'observed'
          OR valid = 0
          )
          );

   CREATE TABLE IF NOT EXISTS lidar_exposure_windows (
          window_id TEXT PRIMARY KEY
        , event_id TEXT NOT NULL REFERENCES lidar_interaction_events (event_id) ON DELETE CASCADE
        , window_json JSON NOT NULL
        , source_id TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.source_id')) STORED
        , kind TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.kind')) STORED
        , basis TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.basis')) STORED
        , track_id TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.track_id')) STORED
        , counterpart_track_id TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.counterpart_track_id')) STORED
        , start_unix_nanos INTEGER NOT NULL AS (JSON_EXTRACT(window_json, '$.start_unix_nanos')) STORED
        , end_unix_nanos INTEGER NOT NULL AS (JSON_EXTRACT(window_json, '$.end_unix_nanos')) STORED
        , duration_nanos INTEGER NOT NULL AS (JSON_EXTRACT(window_json, '$.duration_nanos')) STORED
        , estimate_stage TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.version.estimate_stage')) STORED
        , estimator_id TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.version.estimator_id')) STORED
        , obs_model_id TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.version.obs_model_id')) STORED
        , method_id TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.version.method_id')) STORED
        , param_hash TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.version.param_hash')) STORED
        , inserted_at_ns INTEGER NOT NULL
        , CHECK (
          window_id = JSON_EXTRACT(window_json, '$.window_id')
          AND event_id = JSON_EXTRACT(window_json, '$.event_id')
          )
        , CHECK (
          duration_nanos > 0
          AND end_unix_nanos - start_unix_nanos = duration_nanos
          )
          );

CREATE INDEX IF NOT EXISTS idx_lidar_exposure_windows_event ON lidar_exposure_windows (event_id, start_unix_nanos);

CREATE INDEX IF NOT EXISTS idx_lidar_exposure_windows_version ON lidar_exposure_windows (source_id, kind, basis, estimate_stage, estimator_id, obs_model_id, method_id, param_hash);

CREATE INDEX IF NOT EXISTS idx_lidar_exposure_windows_track ON lidar_exposure_windows (track_id, kind);
