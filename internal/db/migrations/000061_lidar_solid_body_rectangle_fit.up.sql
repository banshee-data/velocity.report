-- Migration: solid-body rectangle fit
-- Date: 2026-10-09
-- Description: Under the solid_body_rectangle_fit experiment each associated
-- frame's rectangle orientation fit is recorded on the row as a diagnostic
-- (the geometry convergence plan's W1a): the axis modulo a quarter turn and
-- NULL when no fit was made; its sigma and the plateau width in radians;
-- the points' spans along and across it; and why the fit abstained (empty
-- when it did not). Nothing reads it yet. Rows written before this
-- migration and rows of arms without the experiment carry NULL and the
-- defaults.
    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN rectangle_axis_rad REAL;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN rectangle_sigma_rad REAL NOT NULL DEFAULT 0;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN rectangle_plateau_rad REAL NOT NULL DEFAULT 0;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN rectangle_span_1_m REAL NOT NULL DEFAULT 0;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN rectangle_span_2_m REAL NOT NULL DEFAULT 0;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN rectangle_abstain TEXT NOT NULL DEFAULT '';
