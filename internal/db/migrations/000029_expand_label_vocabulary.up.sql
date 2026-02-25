-- 000029: Expand label vocabulary
-- Revert "ped" → "pedestrian" (full word) and validate truck/motorcyclist classes.
-- The classifier now produces: car, truck, bus, pedestrian, cyclist, motorcyclist, bird, dynamic.
-- User-only label: noise.
-- Update existing "ped" labels in lidar_run_tracks
   UPDATE lidar_run_tracks
      SET user_label = 'pedestrian'
    WHERE user_label = 'ped';

-- Update existing "ped" labels in lidar_labels
   UPDATE lidar_labels
      SET class_label = 'pedestrian'
    WHERE class_label = 'ped';

-- Update missed regions
   UPDATE lidar_missed_regions
      SET expected_label = 'pedestrian'
    WHERE expected_label = 'ped';

-- Rename "other" → "dynamic" and remove "impossible" label
-- Aligns DB label vocabulary with proto ObjectClass enum
-- "impossible" is remapped to "noise" (both are non-detection user labels).
-- Rename "other" → "dynamic" in all label tables
   UPDATE lidar_run_tracks
      SET user_label = 'dynamic'
    WHERE user_label = 'other';

   UPDATE lidar_labels
      SET class_label = 'dynamic'
    WHERE class_label = 'other';

   UPDATE lidar_missed_regions
      SET expected_label = 'dynamic'
    WHERE expected_label = 'other';

-- Remap "impossible" → "noise" in all label tables
   UPDATE lidar_run_tracks
      SET user_label = 'noise'
    WHERE user_label = 'impossible';

   UPDATE lidar_labels
      SET class_label = 'noise'
    WHERE class_label = 'impossible';

   UPDATE lidar_missed_regions
      SET expected_label = 'noise'
    WHERE expected_label = 'impossible';
