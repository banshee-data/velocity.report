-- Migration: solid-body containment
-- Date: 2026-10-09
-- Description: A solid-body row reported a box and said nothing about the
-- points it was measured from. The geometry convergence plan's W0 adds the
-- measurement of the box against those points: containment_share is the
-- share of the frame's cluster points inside the reported box widened by the
-- face tolerance, contained_points how many were counted, and the two
-- observed spans are the cluster's trimmed spans along and across the
-- reported orientation. containment_share is NULL where the tracker had no
-- points or no orientation (a seed or a coast), and for every row written
-- before this migration; a reader must not read NULL as zero.
    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN containment_share REAL;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN contained_points INTEGER NOT NULL DEFAULT 0;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN observed_span_along_m REAL NOT NULL DEFAULT 0;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN observed_span_across_m REAL NOT NULL DEFAULT 0;
