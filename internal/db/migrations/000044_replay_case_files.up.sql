-- Migration: A replay case references an ordered set of capture files
-- Date: 2026-09-07
-- Description: A static period is bounded by when the sensor stopped and started
-- moving, not by when the capture tool rolled its output file, so a replay case
-- routinely spans several captures. The scalar pcap_file column cannot say that.
-- One row per file in a case, ordered. capture_file_id links to the index when
-- the file is known to it; pcap_file always carries the path, so a case survives
-- the index being rebuilt or the file being indexed under a different root.
   CREATE TABLE IF NOT EXISTS lidar_replay_case_files (
          replay_case_id TEXT NOT NULL
        , ordinal INTEGER NOT NULL
        , capture_file_id TEXT
        , pcap_file TEXT NOT NULL
        , PRIMARY KEY (replay_case_id, ordinal)
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
          );

CREATE INDEX IF NOT EXISTS idx_lidar_replay_case_files_case ON lidar_replay_case_files (replay_case_id, ordinal);

CREATE INDEX IF NOT EXISTS idx_lidar_replay_case_files_capture ON lidar_replay_case_files (capture_file_id);

-- Link a case to the session it was cut from, and to the motion period it came
-- from when the split engine created it. Both are advisory: a case remains
-- valid after its session is re-derived away.
    ALTER TABLE lidar_replay_cases
      ADD COLUMN session_id TEXT;

    ALTER TABLE lidar_replay_cases
      ADD COLUMN source_period_id TEXT;

CREATE INDEX IF NOT EXISTS idx_lidar_replay_cases_session ON lidar_replay_cases (session_id);

-- Backfill: every existing case becomes a one-file case, so nothing that
-- referenced pcap_file changes meaning.
--
-- pcap_file stays as the read-only projection of ordinal 0 for one release, so
-- existing API clients and the sweep integration keep working. It is removed in
-- v0.6.1 once those have moved to the file list.
   INSERT OR IGNORE INTO lidar_replay_case_files (replay_case_id, ordinal, capture_file_id, pcap_file)
   SELECT replay_case_id
        , 0
        , NULL
        , pcap_file
     FROM lidar_replay_cases
    WHERE pcap_file IS NOT NULL
      AND pcap_file <> '';
