-- Rollback: Remove radar_data_transits table and index

DROP INDEX IF EXISTS idx_transits_time;
DROP TABLE IF EXISTS radar_data_transits;
