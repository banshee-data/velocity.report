-- The lane snapshot (docs/plans/lidar-lane-snapshot-and-attribution-plan.md):
-- an append-only copy of the street-use features Gerardus holds for one
-- site and sensor, with the accepted pose that places them, written by the
-- painter after each save and by `velocity lidar lanes pull`. The union of
-- a deployment's snapshots is the version history of its lanes, so a limit
-- that changes over time is attributed by measurement time.
   CREATE TABLE lidar_lane_snapshots (
          snapshot_id TEXT PRIMARY KEY
        , l16_token TEXT NOT NULL
        , sensor_id TEXT NOT NULL
        , writer TEXT NOT NULL
        , request_id TEXT NOT NULL
        , record_id TEXT NOT NULL
        , features_digest TEXT NOT NULL
        , fetched_at_ns INTEGER NOT NULL
        , supersedes TEXT REFERENCES lidar_lane_snapshots (snapshot_id)
        , snapshot_json TEXT NOT NULL
        , request_json TEXT NOT NULL
        , CHECK (writer IN ('painter', 'pull'))
        , CHECK (JSON_VALID(snapshot_json))
        , CHECK (JSON_VALID(request_json))
        , UNIQUE (l16_token, request_id)
          );

CREATE INDEX idx_lidar_lane_snapshots_site ON lidar_lane_snapshots (l16_token, sensor_id, fetched_at_ns);

-- One row per lane per snapshot: the lane as derived, in both frames with
-- its tags and limit, the limit's effective dates indexed.
   CREATE TABLE lidar_lane_snapshot_lanes (
          snapshot_id TEXT NOT NULL REFERENCES lidar_lane_snapshots (snapshot_id)
        , feature_id TEXT NOT NULL
        , row_id TEXT NOT NULL
        , lane_json TEXT NOT NULL
        , speed_limit INTEGER GENERATED ALWAYS AS (JSON_EXTRACT(lane_json, '$.limit.value')) VIRTUAL
        , speed_limit_unit TEXT GENERATED ALWAYS AS (JSON_EXTRACT(lane_json, '$.limit.unit')) VIRTUAL
        , effective_from TEXT GENERATED ALWAYS AS (JSON_EXTRACT(lane_json, '$.limit.effective_from')) VIRTUAL
        , effective_to TEXT GENERATED ALWAYS AS (JSON_EXTRACT(lane_json, '$.limit.effective_to')) VIRTUAL
        , direction_id TEXT GENERATED ALWAYS AS (JSON_EXTRACT(lane_json, '$.direction')) VIRTUAL
        , PRIMARY KEY (snapshot_id, feature_id)
        , CHECK (JSON_VALID(lane_json))
          );

CREATE INDEX idx_lidar_lane_snapshot_lanes_feature ON lidar_lane_snapshot_lanes (feature_id, row_id);

CREATE TRIGGER lidar_lane_snapshots_append_only_update BEFORE
   UPDATE ON lidar_lane_snapshots BEGIN
   SELECT RAISE (ABORT, 'lane snapshots are append-only');

END;

CREATE TRIGGER lidar_lane_snapshots_append_only_delete BEFORE DELETE ON lidar_lane_snapshots BEGIN
   SELECT RAISE (ABORT, 'lane snapshots are append-only');

END;

CREATE TRIGGER lidar_lane_snapshot_lanes_append_only_update BEFORE
   UPDATE ON lidar_lane_snapshot_lanes BEGIN
   SELECT RAISE (ABORT, 'lane snapshots are append-only');

END;

CREATE TRIGGER lidar_lane_snapshot_lanes_append_only_delete BEFORE DELETE ON lidar_lane_snapshot_lanes BEGIN
   SELECT RAISE (ABORT, 'lane snapshots are append-only');

END;
