-- Rollback: solid-body rectangle fit
    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN rectangle_abstain;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN rectangle_span_2_m;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN rectangle_span_1_m;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN rectangle_plateau_rad;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN rectangle_sigma_rad;

    ALTER TABLE lidar_track_solid_bodies
     DROP COLUMN rectangle_axis_rad;
