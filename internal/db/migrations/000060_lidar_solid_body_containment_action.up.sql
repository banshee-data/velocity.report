-- Migration: solid-body containment action
-- Date: 2026-10-09
-- Description: Under the solid_body_containment experiment a row's reported
-- box can be held on its points two ways, and each row says which. extent_floor
-- names the reported dimensions the frame's window-minimum span raised
-- ('', 'length', 'width' or 'length,width'); the beliefs are untouched.
-- containment_shift_along_m and containment_shift_across_m are the signed
-- moves the position truncation made along the body axis and its left normal.
-- Rows written before this migration, and every row of an arm without the
-- experiment, carry the empty name and zero moves.
    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN extent_floor TEXT NOT NULL DEFAULT '';

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN containment_shift_along_m REAL NOT NULL DEFAULT 0;

    ALTER TABLE lidar_track_solid_bodies
      ADD COLUMN containment_shift_across_m REAL NOT NULL DEFAULT 0;
