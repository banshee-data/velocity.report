-- Migration: solid-body support detail
-- Date: 2026-09-27
-- Description: A solid-body row kept only its support point count and
-- coasted frames. An explained absence or a fragmented or truncated cluster
-- therefore read back as a bare observed or coasted instant. support_instant
-- stores the tracker's Section 7.3 support token. support_fragmented and
-- support_truncated store the two flags that refuse a cluster's extent as
-- dimension evidence. Rows written before this migration carry the empty
-- token: their support was not recorded and a reader must not invent it.
    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN support_instant TEXT NOT NULL DEFAULT '';

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN support_fragmented INTEGER NOT NULL DEFAULT 0;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN support_truncated INTEGER NOT NULL DEFAULT 0;
