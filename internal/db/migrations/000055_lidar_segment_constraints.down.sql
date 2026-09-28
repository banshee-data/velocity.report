-- Rollback: restore the segment tables of migration 000054.
--
-- Every selection returns with the role, finder and run it was made with;
-- source stands in for a run that has since been deleted, because 000054
-- required one. Rows this migration set aside are restored as they were.
--
-- A pack link does not return as it was: 000054 stored an absolute path and
-- this migration kept none. The relative path is restored, which the 000054
-- worker does not recognise, so it cuts the pack again if the job is retried.
PRAGMA foreign_keys = OFF;

   CREATE TABLE lidar_segment_selections_old (
          segment_id TEXT PRIMARY KEY
        , run_id TEXT NOT NULL
        , replay_case_id TEXT NOT NULL UNIQUE
        , role TEXT NOT NULL CHECK (role IN ('tuning', 'held_out'))
        , finder TEXT NOT NULL
        , parameters_json TEXT NOT NULL
        , window_json TEXT NOT NULL
        , created_at_ns INTEGER NOT NULL
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
          );

   INSERT INTO lidar_segment_selections_old (
          segment_id
        , run_id
        , replay_case_id
        , role
        , finder
        , parameters_json
        , window_json
        , created_at_ns
          )
   SELECT segment_id
        , source
        , replay_case_id
        , role
        , finder
        , parameters_json
        , window_json
        , created_at_ns
     FROM lidar_segment_selections;

-- A rejected row goes back only where 000054 itself would have taken it:
-- its role in the vocabulary and its keys not already in use.
   INSERT OR IGNORE INTO lidar_segment_selections_old (
          segment_id
        , run_id
        , replay_case_id
        , role
        , finder
        , parameters_json
        , window_json
        , created_at_ns
          )
   SELECT JSON_EXTRACT(row_json, '$.segment_id')
        , JSON_EXTRACT(row_json, '$.run_id')
        , JSON_EXTRACT(row_json, '$.replay_case_id')
        , JSON_EXTRACT(row_json, '$.role')
        , JSON_EXTRACT(row_json, '$.finder')
        , JSON_EXTRACT(row_json, '$.parameters_json')
        , JSON_EXTRACT(row_json, '$.window_json')
        , JSON_EXTRACT(row_json, '$.created_at_ns')
     FROM lidar_migration_rejects
    WHERE migration = 55
      AND source_table = 'lidar_segment_selections'
      AND JSON_EXTRACT(row_json, '$.role') IN ('tuning', 'held_out')
      AND JSON_EXTRACT(row_json, '$.run_id') IS NOT NULL
      AND JSON_EXTRACT(row_json, '$.replay_case_id') IS NOT NULL
      AND JSON_EXTRACT(row_json, '$.finder') IS NOT NULL
      AND JSON_EXTRACT(row_json, '$.parameters_json') IS NOT NULL
      AND JSON_EXTRACT(row_json, '$.window_json') IS NOT NULL
      AND JSON_EXTRACT(row_json, '$.created_at_ns') IS NOT NULL;

   CREATE TABLE lidar_segment_clip_jobs_old (
          job_id TEXT PRIMARY KEY
        , segment_id TEXT NOT NULL
        , replay_case_id TEXT NOT NULL
        , pack_dir TEXT NOT NULL DEFAULT ''
        , FOREIGN KEY (job_id) REFERENCES lidar_capture_jobs (job_id) ON DELETE CASCADE
        , FOREIGN KEY (segment_id) REFERENCES lidar_segment_selections (segment_id) ON DELETE CASCADE
        , FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases (replay_case_id) ON DELETE CASCADE
          );

   INSERT INTO lidar_segment_clip_jobs_old (job_id, segment_id, replay_case_id, pack_dir)
   SELECT j.job_id
        , j.segment_id
        , s.replay_case_id
        , COALESCE(j.pack_dir, '')
     FROM lidar_segment_clip_jobs j
     JOIN lidar_segment_selections s ON s.segment_id = j.segment_id;

   INSERT OR IGNORE INTO lidar_segment_clip_jobs_old (job_id, segment_id, replay_case_id, pack_dir)
   SELECT JSON_EXTRACT(row_json, '$.job_id')
        , JSON_EXTRACT(row_json, '$.segment_id')
        , JSON_EXTRACT(row_json, '$.replay_case_id')
        , COALESCE(JSON_EXTRACT(row_json, '$.pack_dir'), '')
     FROM lidar_migration_rejects
    WHERE migration = 55
      AND source_table = 'lidar_segment_clip_jobs'
      AND JSON_EXTRACT(row_json, '$.job_id') IS NOT NULL
      AND JSON_EXTRACT(row_json, '$.segment_id') IS NOT NULL
      AND JSON_EXTRACT(row_json, '$.replay_case_id') IS NOT NULL;

-- The queue's session column carried the segment under 000054, and its
-- duplicate check reads it.
   UPDATE lidar_capture_jobs
      SET session_id = (
             SELECT j.segment_id
               FROM lidar_segment_clip_jobs_old j
              WHERE j.job_id = lidar_capture_jobs.job_id
          )
    WHERE kind = 'vrlog_record'
      AND job_id IN (
             SELECT job_id
               FROM lidar_segment_clip_jobs_old
          );

     DROP INDEX IF EXISTS idx_lidar_segment_clip_jobs_segment;

     DROP INDEX IF EXISTS idx_lidar_segment_selections_run;

     DROP INDEX IF EXISTS idx_lidar_segment_selections_guard;

     DROP TABLE lidar_segment_clip_jobs;

     DROP TABLE lidar_segment_selections;

    ALTER TABLE lidar_segment_selections_old
RENAME TO lidar_segment_selections;

    ALTER TABLE lidar_segment_clip_jobs_old
RENAME TO lidar_segment_clip_jobs;

     DROP INDEX IF EXISTS idx_lidar_migration_rejects_source;

     DROP TABLE IF EXISTS lidar_migration_rejects;

PRAGMA foreign_keys = ON;
