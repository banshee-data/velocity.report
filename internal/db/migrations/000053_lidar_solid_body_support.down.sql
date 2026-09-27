-- Rollback: solid-body support detail
    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN support_truncated;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN support_fragmented;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN support_instant;
