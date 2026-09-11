-- Rollback: Remove geographic identity from replay cases
-- WARNING: This discards where each located capture was taken. The positions
-- cannot be recovered from the remaining columns; the S2 tokens were derived
-- from them and go with them.
     DROP INDEX IF EXISTS idx_lidar_replay_cases_s2_l16;

     DROP INDEX IF EXISTS idx_lidar_replay_cases_s2_l13;

     DROP INDEX IF EXISTS idx_lidar_replay_cases_s2_l10;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN geographic_status;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN geographic_source;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN s2_l16_token;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN s2_l13_token;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN s2_l10_token;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN origin_lon;

    ALTER TABLE lidar_replay_cases
     DROP COLUMN origin_lat;
