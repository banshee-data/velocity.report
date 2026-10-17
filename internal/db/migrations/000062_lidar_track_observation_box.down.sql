-- Rollback: the published box on each track observation
    ALTER TABLE lidar_track_observations
     DROP COLUMN box_width;

    ALTER TABLE lidar_track_observations
     DROP COLUMN box_length;

    ALTER TABLE lidar_track_observations
     DROP COLUMN box_heading_rad;

    ALTER TABLE lidar_track_observations
     DROP COLUMN box_y;

    ALTER TABLE lidar_track_observations
     DROP COLUMN box_x;
