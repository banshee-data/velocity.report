-- Migration: each point estimate states its reference point and support token
-- Date: 2026-09-29
-- Description: A lidar_track_estimates row said which geometry entered the
-- filter (measurement_source). It did not say which point its position refers
-- to or what the instant rested on. The behaviour adapter inferred both: the
-- reference from measurement_source, and observed support from the row's
-- existence. Under the near-edge tracked state (near-edge plan, invariant 7)
-- a medoid can enter a filter whose state refers to the body centre, so the
-- inference fails; each row now states both, as lidar_track_solid_bodies
-- already does. reference_point holds l5tracks.ReferencePoint's token and
-- support_instant the Section 7.3 support token, and the l5tracks parsers are
-- the only vocabulary: no CHECK repeats them.
--
-- Rows written before this migration are backfilled with exactly the mapping
-- the adapter used: obb_centre_v1 is visible_obb_centre, medoid_v0 and
-- medoid_fallback_v1 are cluster_medoid, and every row is observed, because
-- both writers wrote a row only at an accepted association. A row whose
-- measurement_source that mapping does not know keeps an empty
-- reference_point. The adapter refused such a row, and every reader now
-- refuses it; none is guessed and none is dropped.
--
-- A trigger refuses a new row that leaves either column empty.
    ALTER TABLE lidar_track_estimates
      ADD COLUMN reference_point TEXT NOT NULL DEFAULT '';

    ALTER TABLE lidar_track_estimates
      ADD COLUMN support_instant TEXT NOT NULL DEFAULT '';

   UPDATE lidar_track_estimates
      SET reference_point = CASE measurement_source
                    WHEN 'obb_centre_v1' THEN 'visible_obb_centre'
                    WHEN 'medoid_v0' THEN 'cluster_medoid'
                    WHEN 'medoid_fallback_v1' THEN 'cluster_medoid'
                    ELSE ''
          END
        , support_instant = 'observed';

CREATE TRIGGER lidar_track_estimates_state_reference_and_support BEFORE INSERT ON lidar_track_estimates WHEN NEW.reference_point = ''
       OR NEW.support_instant = '' BEGIN
             SELECT RAISE (
                    ABORT
                  , 'a track estimate states its reference point and support token'
                    );

END;
