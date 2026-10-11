-- Rollback: solid-body containment
    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN observed_span_across_m;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN observed_span_along_m;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN contained_points;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN containment_share;
