-- Migration: constraints for annotation segment selections and clip jobs
-- Date: 2026-09-27
-- Description: Migration 000054 stored a selection's role, finder and window
-- twice: once as columns and once inside window_json, with nothing to make
-- the two agree. Its run had no foreign key, its JSON was unchecked text, its
-- held-out guard read a field out of every row's JSON, and a clip job named a
-- replay case of its own beside the one its selection already had.
-- (data/structures/SCHEMA-REVIEW.md, findings 1 to 5 and 7.)
--
-- lidar_segment_selections: window_json is the record of what the finder
-- chose, and the facts that queries select on are generated from it, so a
-- column cannot say one thing and the document another. CHECKs hold the role
-- and finder vocabularies, the rule that a held-out window is never chosen
-- by a finder that seeks tracker failure, and the window's own bounds.
--
-- run_id is a foreign key that is cleared when the run is deleted. The
-- selection, its replay case and its pack outlive the run, and source keeps
-- the identity the segment ID was computed from.
--
-- lidar_segment_clip_jobs: the replay case is reached through the selection.
-- A pack is named by a path relative to the pack directory and by its
-- digest, both or neither. '' no longer means "no pack".
--
-- Rows written under 000054 are copied when they satisfy the new rules. A
-- finder column left empty by the request default is repaired from the
-- document. Any other row is kept whole in lidar_migration_rejects with the
-- reason, never dropped. A pack link is cleared, not converted: the path was
-- absolute and the digest unknown to SQL, so the server finds the pack again
-- by its job when it starts.
PRAGMA foreign_keys = OFF;

   CREATE TABLE IF NOT EXISTS lidar_migration_rejects (
          reject_id INTEGER PRIMARY KEY
        , migration INTEGER NOT NULL
        , source_table TEXT NOT NULL
        , source_key TEXT NOT NULL
        , reason TEXT NOT NULL
        , row_json TEXT NOT NULL
        , rejected_at_ns INTEGER NOT NULL
        , CHECK (JSON_VALID(row_json))
          );

CREATE INDEX IF NOT EXISTS idx_lidar_migration_rejects_source ON lidar_migration_rejects (source_table, source_key);

-- One verdict per old row, worked out once and used for both destinations.
-- CASE stops at the first rule that fails, so a later rule never reads a
-- field out of a document an earlier rule found to be malformed.
   CREATE TEMP TABLE lidar_segment_selection_verdicts AS
   SELECT s.segment_id
        , CASE
                    WHEN NOT JSON_VALID(s.window_json) THEN 'window_json is not valid JSON'
                    WHEN JSON_TYPE(s.window_json) != 'object' THEN 'window_json is not an object'
                    WHEN NOT JSON_VALID(s.parameters_json) THEN 'parameters_json is not valid JSON'
                    WHEN JSON_TYPE(s.parameters_json) != 'object' THEN 'parameters_json is not an object'
                    WHEN JSON_EXTRACT(s.window_json, '$.id') IS NOT s.segment_id THEN 'window_json names another segment'
                    WHEN JSON_EXTRACT(s.window_json, '$.source') IS NOT s.run_id THEN 'window_json names another source than run_id'
                    WHEN JSON_EXTRACT(s.window_json, '$.role') IS NOT s.role THEN 'role disagrees with window_json'
                    WHEN s.finder != ''
                          AND JSON_EXTRACT(s.window_json, '$.finder') IS NOT s.finder THEN 'finder disagrees with window_json'
                              WHEN JSON_EXTRACT(s.window_json, '$.role') NOT IN ('tuning', 'held_out') THEN 'role is not tuning or held_out'
                              WHEN JSON_EXTRACT(s.window_json, '$.finder') IS NULL
                           OR JSON_EXTRACT(s.window_json, '$.finder') NOT IN (
                              'following'
                            , 'leader_changes'
                            , 'lateral_jump'
                            , 'split_flags'
                            , 'exposure'
                            , 'random'
                              ) THEN 'finder is not in the catalogue'
                              WHEN JSON_EXTRACT(s.window_json, '$.role') = 'held_out'
                          AND JSON_EXTRACT(s.window_json, '$.finder') NOT IN ('following', 'exposure', 'random') THEN 'held-out window chosen by a failure-seeking finder'
                              WHEN TYPEOF(JSON_EXTRACT(s.window_json, '$.version')) != 'integer'
                           OR JSON_EXTRACT(s.window_json, '$.version') < 1 THEN 'finder version is missing'
                              WHEN TYPEOF(JSON_EXTRACT(s.window_json, '$.window_start_unix_nanos')) != 'integer'
                           OR TYPEOF(JSON_EXTRACT(s.window_json, '$.window_end_unix_nanos')) != 'integer'
                           OR JSON_EXTRACT(s.window_json, '$.window_end_unix_nanos') <= JSON_EXTRACT(s.window_json, '$.window_start_unix_nanos') THEN 'window bounds are missing or inverted'
                              WHEN TYPEOF(JSON_EXTRACT(s.window_json, '$.capture')) != 'text'
                           OR JSON_EXTRACT(s.window_json, '$.capture') = '' THEN 'window has no capture'
                              WHEN NOT EXISTS (
                                 SELECT 1
                                   FROM lidar_replay_cases c
                                  WHERE c.replay_case_id = s.replay_case_id
                              ) THEN 'replay case does not exist'
          END AS reason
     FROM lidar_segment_selections s;

   INSERT INTO lidar_migration_rejects (migration, source_table, source_key, reason, row_json, rejected_at_ns)
   SELECT 55
        , 'lidar_segment_selections'
        , s.segment_id
        , v.reason
        , JSON_OBJECT(
          'segment_id'
        , s.segment_id
        , 'run_id'
        , s.run_id
        , 'replay_case_id'
        , s.replay_case_id
        , 'role'
        , s.role
        , 'finder'
        , s.finder
        , 'parameters_json'
        , s.parameters_json
        , 'window_json'
        , s.window_json
        , 'created_at_ns'
        , s.created_at_ns
          )
        , CAST(STRFTIME('%s', 'now') AS INTEGER) * 1000000000
     FROM lidar_segment_selections s
     JOIN lidar_segment_selection_verdicts v ON v.segment_id = s.segment_id
    WHERE v.reason IS NOT NULL;

-- A job whose selection is rejected, or that replayed another case than its
-- selection's, cannot stand for that selection's pack.
   INSERT INTO lidar_migration_rejects (migration, source_table, source_key, reason, row_json, rejected_at_ns)
   SELECT 55
        , 'lidar_segment_clip_jobs'
        , j.job_id
        , CASE
                    WHEN s.segment_id IS NULL THEN 'selection does not exist'
                    WHEN v.reason IS NOT NULL THEN 'selection was rejected'
                    WHEN j.replay_case_id IS NOT s.replay_case_id THEN 'job replays another case than its selection'
                    ELSE 'capture job does not exist'
          END
        , JSON_OBJECT(
          'job_id'
        , j.job_id
        , 'segment_id'
        , j.segment_id
        , 'replay_case_id'
        , j.replay_case_id
        , 'pack_dir'
        , j.pack_dir
          )
        , CAST(STRFTIME('%s', 'now') AS INTEGER) * 1000000000
     FROM lidar_segment_clip_jobs j
LEFT JOIN lidar_segment_selections s ON s.segment_id = j.segment_id
LEFT JOIN lidar_segment_selection_verdicts v ON v.segment_id = j.segment_id
    WHERE s.segment_id IS NULL
       OR v.reason IS NOT NULL
       OR j.replay_case_id IS NOT s.replay_case_id
       OR NOT EXISTS (
             SELECT 1
               FROM lidar_capture_jobs q
              WHERE q.job_id = j.job_id
          );

   CREATE TABLE lidar_segment_selections_new (
          segment_id TEXT PRIMARY KEY
        , run_id TEXT
        , replay_case_id TEXT NOT NULL UNIQUE
        , document_version INTEGER NOT NULL DEFAULT 1
        , parameters_json TEXT NOT NULL
        , window_json TEXT NOT NULL
        , source TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.source')) STORED
        , role TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.role')) STORED
        , finder TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.finder')) STORED
        , finder_version INTEGER NOT NULL AS (JSON_EXTRACT(window_json, '$.version')) STORED
        , capture TEXT NOT NULL AS (JSON_EXTRACT(window_json, '$.capture')) STORED
        , window_start_ns INTEGER NOT NULL AS (JSON_EXTRACT(window_json, '$.window_start_unix_nanos')) STORED
        , window_end_ns INTEGER NOT NULL AS (JSON_EXTRACT(window_json, '$.window_end_unix_nanos')) STORED
        , created_at_ns INTEGER NOT NULL
        , CHECK (document_version = 1)
        , CHECK (
          JSON_VALID(parameters_json)
      AND JSON_TYPE(parameters_json) = 'object'
          )
        , CHECK (
          JSON_VALID(window_json)
      AND JSON_TYPE(window_json) = 'object'
          )
        , CHECK (segment_id = JSON_EXTRACT(window_json, '$.id'))
        , CHECK (
          run_id IS NULL
       OR run_id = source
          )
        , CHECK (role IN ('tuning', 'held_out'))
        , CHECK (
          finder IN (
          'following'
        , 'leader_changes'
        , 'lateral_jump'
        , 'split_flags'
        , 'exposure'
        , 'random'
          )
          )
        , CHECK (
          role = 'tuning'
       OR finder IN ('following', 'exposure', 'random')
          )
        , CHECK (
          TYPEOF(finder_version) = 'integer'
      AND finder_version >= 1
          )
        , CHECK (
          TYPEOF(window_start_ns) = 'integer'
      AND TYPEOF(window_end_ns) = 'integer'
      AND window_end_ns > window_start_ns
          )
        , CHECK (
          TYPEOF(capture) = 'text'
      AND capture != ''
          )
        , FOREIGN KEY (run_id) REFERENCES lidar_run_records (run_id) ON DELETE SET NULL
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
          );

   INSERT INTO lidar_segment_selections_new (
          segment_id
        , run_id
        , replay_case_id
        , parameters_json
        , window_json
        , created_at_ns
          )
   SELECT s.segment_id
        , (
             SELECT r.run_id
               FROM lidar_run_records r
              WHERE r.run_id = s.run_id
          )
        , s.replay_case_id
        , s.parameters_json
        , s.window_json
        , s.created_at_ns
     FROM lidar_segment_selections s
     JOIN lidar_segment_selection_verdicts v ON v.segment_id = s.segment_id
    WHERE v.reason IS NULL;

   CREATE TABLE lidar_segment_clip_jobs_new (
          job_id TEXT PRIMARY KEY
        , segment_id TEXT NOT NULL
        , pack_dir TEXT
        , pack_digest TEXT
        , CHECK ((pack_dir IS NULL) = (pack_digest IS NULL))
        , CHECK (
          pack_dir IS NULL
       OR (
          pack_dir != ''
      AND SUBSTR(pack_dir, 1, 1) != '/'
      AND INSTR(pack_dir, '..') = 0
      AND INSTR(pack_dir, CHAR(92)) = 0
          )
          )
        , CHECK (
          pack_digest IS NULL
       OR pack_digest LIKE 'sha256:_%'
          )
        , FOREIGN KEY (job_id) REFERENCES lidar_capture_jobs (job_id) ON DELETE CASCADE
        , FOREIGN KEY (segment_id) REFERENCES lidar_segment_selections (segment_id) ON DELETE CASCADE
          );

   INSERT INTO lidar_segment_clip_jobs_new (job_id, segment_id)
   SELECT j.job_id
        , j.segment_id
     FROM lidar_segment_clip_jobs j
     JOIN lidar_segment_selections s ON s.segment_id = j.segment_id
     JOIN lidar_segment_selection_verdicts v ON v.segment_id = j.segment_id
    WHERE v.reason IS NULL
      AND j.replay_case_id IS s.replay_case_id
      AND EXISTS (
             SELECT 1
               FROM lidar_capture_jobs q
              WHERE q.job_id = j.job_id
          );

-- A clip job no longer borrows the capture queue's session column for its
-- segment: the link table says which segment a job cuts.
   UPDATE lidar_capture_jobs
      SET session_id = NULL
    WHERE kind = 'vrlog_record'
      AND job_id IN (
             SELECT job_id
               FROM lidar_segment_clip_jobs
          );

     DROP TABLE lidar_segment_selection_verdicts;

     DROP TABLE lidar_segment_clip_jobs;

     DROP TABLE lidar_segment_selections;

    ALTER TABLE lidar_segment_selections_new
RENAME TO lidar_segment_selections;

    ALTER TABLE lidar_segment_clip_jobs_new
RENAME TO lidar_segment_clip_jobs;

-- The held-out guard asks whether a capture already has a random held-out
-- window. With this index it is a lookup, not a read of every selection.
CREATE INDEX idx_lidar_segment_selections_guard ON lidar_segment_selections (role, finder, capture);

CREATE INDEX idx_lidar_segment_selections_run ON lidar_segment_selections (run_id);

CREATE INDEX idx_lidar_segment_clip_jobs_segment ON lidar_segment_clip_jobs (segment_id);

PRAGMA foreign_keys = ON;
