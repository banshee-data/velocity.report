-- Migration: Motion/static timelines and the jobs that produce them
-- Date: 2026-09-07
-- Description: Records the motion/static periods found in a capture session and
-- the background jobs that find them.
-- Periods belong to a session, not to a single file, and that is deliberate.
-- The background model needs tens of seconds to settle, so classifying one file
-- at a time restarts it at every boundary and reports the settling as motion.
-- Measured on s2_sf_3 files 2-9: per-file analysis invented a ~14 second motion
-- period at the head of two files and missed a real 25 second one inside a third
-- that it called wholly static. The joined stream produces neither error, and a
-- static period that spans a file boundary — the case replay cases exist for —
-- can only be expressed here.
   CREATE TABLE IF NOT EXISTS lidar_capture_motion_periods (
          period_id TEXT PRIMARY KEY
        , session_id TEXT NOT NULL
        , ordinal INTEGER NOT NULL
        , period_type TEXT NOT NULL
        , label TEXT NOT NULL DEFAULT ''
        , start_ns INTEGER NOT NULL
        , end_ns INTEGER NOT NULL
        , duration_ns INTEGER NOT NULL
        , start_secs REAL NOT NULL
        , end_secs REAL NOT NULL
        , start_frame INTEGER
        , end_frame INTEGER
        , created_at_ns INTEGER NOT NULL
        , CHECK (period_type IN ('motion', 'static'))
        , FOREIGN KEY (session_id) REFERENCES lidar_capture_sessions (session_id) ON DELETE CASCADE
        , UNIQUE (session_id, ordinal)
          );

CREATE INDEX IF NOT EXISTS idx_lidar_capture_motion_periods_session ON lidar_capture_motion_periods (session_id, start_ns);

CREATE INDEX IF NOT EXISTS idx_lidar_capture_motion_periods_type ON lidar_capture_motion_periods (period_type);

-- Background work over the capture index. A motion pass reads every byte of
-- every file in a session, so it cannot run inside a request.
   CREATE TABLE IF NOT EXISTS lidar_capture_jobs (
          job_id TEXT PRIMARY KEY
        , kind TEXT NOT NULL
        , session_id TEXT
        , root_id TEXT
        , state TEXT NOT NULL DEFAULT 'queued'
        , progress_current INTEGER NOT NULL DEFAULT 0
        , progress_total INTEGER NOT NULL DEFAULT 0
        , detail TEXT NOT NULL DEFAULT ''
        , error TEXT NOT NULL DEFAULT ''
        , queued_at_ns INTEGER NOT NULL
        , started_at_ns INTEGER
        , finished_at_ns INTEGER
        , CHECK (state IN ('queued', 'running', 'completed', 'failed', 'cancelled'))
          );

CREATE INDEX IF NOT EXISTS idx_lidar_capture_jobs_state ON lidar_capture_jobs (state, queued_at_ns);

CREATE INDEX IF NOT EXISTS idx_lidar_capture_jobs_session ON lidar_capture_jobs (session_id, queued_at_ns);
