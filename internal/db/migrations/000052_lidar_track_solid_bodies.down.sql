-- Rollback: solid-body track estimates behind the state_model discriminator
     DROP INDEX IF EXISTS idx_lidar_track_solid_bodies_observation;

     DROP TABLE IF EXISTS lidar_track_solid_bodies;

    ALTER TABLE lidar_track_estimates
     DROP COLUMN state_model;
