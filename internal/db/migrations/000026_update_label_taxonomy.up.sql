-- 000026: Update label taxonomy
-- Classification labels: car, ped, noise (replaces good_vehicle, good_pedestrian, etc.)
-- Quality flags: good, noisy, jitter_velocity, merge, split, truncated, disconnected
-- Quality flags are stored as comma-separated values for multi-select.
-- Update existing user labels in lidar_run_tracks to new taxonomy
   UPDATE lidar_run_tracks
      SET user_label = 'car'
    WHERE user_label = 'good_vehicle';

   UPDATE lidar_run_tracks
      SET user_label = 'ped'
    WHERE user_label = 'good_pedestrian';

   UPDATE lidar_run_tracks
      SET user_label = 'noise'
    WHERE user_label IN ('good_other', 'noise_flora');

-- Update existing quality labels in lidar_run_tracks to new taxonomy
   UPDATE lidar_run_tracks
      SET quality_label = 'good'
    WHERE quality_label = 'perfect';

   UPDATE lidar_run_tracks
      SET quality_label = 'jitter_velocity'
    WHERE quality_label = 'noisy_velocity';

   UPDATE lidar_run_tracks
      SET quality_label = 'disconnected'
    WHERE quality_label = 'stopped_recovered';

-- Update existing class labels in lidar_labels to new taxonomy
   UPDATE lidar_labels
      SET class_label = 'car'
    WHERE class_label = 'good_vehicle';

   UPDATE lidar_labels
      SET class_label = 'ped'
    WHERE class_label = 'good_pedestrian';

   UPDATE lidar_labels
      SET class_label = 'noise'
    WHERE class_label IN ('good_other', 'noise_flora');

-- Update missed regions default values
   UPDATE lidar_missed_regions
      SET expected_label = 'car'
    WHERE expected_label = 'good_vehicle';
