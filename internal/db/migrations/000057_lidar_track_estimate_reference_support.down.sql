-- Rollback: point estimates no longer state their reference point and support
--
-- Every row is kept; only the two columns go. A reader from before 000057
-- infers the reference from measurement_source again, which is what a row
-- written by those writers says. A row whose state referred to a point its
-- measurement did not, such as a body centre updated from a medoid, loses
-- that statement and would be read as the medoid.
     DROP TRIGGER IF EXISTS lidar_track_estimates_state_reference_and_support;

    ALTER TABLE lidar_track_estimates
     DROP COLUMN support_instant;

    ALTER TABLE lidar_track_estimates
     DROP COLUMN reference_point;
