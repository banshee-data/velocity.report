-- 000026 down: Revert label taxonomy changes
-- Revert user labels in lidar_run_tracks
   UPDATE lidar_run_tracks
      SET user_label = 'good_vehicle'
    WHERE user_label = 'car';

   UPDATE lidar_run_tracks
      SET user_label = 'good_pedestrian'
    WHERE user_label = 'ped';

-- Note: cannot distinguish noise_flora from good_other; reverting all to good_other
   UPDATE lidar_run_tracks
      SET user_label = 'good_other'
    WHERE user_label = 'noise';

-- Revert quality labels in lidar_run_tracks
   UPDATE lidar_run_tracks
      SET quality_label = 'perfect'
    WHERE quality_label = 'good';

   UPDATE lidar_run_tracks
      SET quality_label = 'noisy_velocity'
    WHERE quality_label = 'jitter_velocity';

   UPDATE lidar_run_tracks
      SET quality_label = 'stopped_recovered'
    WHERE quality_label = 'disconnected';

-- Revert class labels in lidar_labels
   UPDATE lidar_labels
      SET class_label = 'good_vehicle'
    WHERE class_label = 'car';

   UPDATE lidar_labels
      SET class_label = 'good_pedestrian'
    WHERE class_label = 'ped';

   UPDATE lidar_labels
      SET class_label = 'good_other'
    WHERE class_label = 'noise';

-- Revert missed regions
   UPDATE lidar_missed_regions
      SET expected_label = 'good_vehicle'
    WHERE expected_label = 'car';
