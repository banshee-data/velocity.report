-- Rollback: solid-body containment action
    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN containment_shift_across_m;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN containment_shift_along_m;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN extent_floor;
