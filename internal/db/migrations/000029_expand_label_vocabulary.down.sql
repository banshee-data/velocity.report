-- 000029: Revert expanded label vocabulary
-- Restore "pedestrian" → "ped" (short form).
   UPDATE lidar_run_tracks
      SET user_label = 'ped'
    WHERE user_label = 'pedestrian';

   UPDATE lidar_labels
      SET class_label = 'ped'
    WHERE class_label = 'pedestrian';

   UPDATE lidar_missed_regions
      SET expected_label = 'ped'
    WHERE expected_label = 'pedestrian';

-- Revert "dynamic" → "other" and restore "impossible" label
   UPDATE lidar_run_tracks
      SET user_label = 'other'
    WHERE user_label = 'dynamic';

   UPDATE lidar_labels
      SET class_label = 'other'
    WHERE class_label = 'dynamic';

   UPDATE lidar_missed_regions
      SET expected_label = 'other'
    WHERE expected_label = 'dynamic';

-- Note: "impossible" → "noise" mapping is lossy; cannot perfectly reverse.
-- Rows that were originally "impossible" are now "noise" and stay as "noise".
