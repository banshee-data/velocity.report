-- Add checkpoint columns for suspend/resume of auto-tune sweeps.
    ALTER TABLE lidar_sweeps
      ADD COLUMN checkpoint_round INTEGER;

    ALTER TABLE lidar_sweeps
      ADD COLUMN checkpoint_bounds TEXT;

    ALTER TABLE lidar_sweeps
      ADD COLUMN checkpoint_results TEXT;

    ALTER TABLE lidar_sweeps
      ADD COLUMN checkpoint_request TEXT;
