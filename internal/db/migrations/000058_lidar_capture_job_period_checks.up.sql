-- Migration: capture jobs of a known kind, and motion periods that agree with their bounds
-- Date: 2026-09-28
-- Description: Two rules from the data model review
-- (data/structures/SCHEMA-REVIEW.md, findings 6 and 10).
--
-- lidar_capture_jobs holds two kinds of work: a motion pass over a capture
-- session, and a clip that records a chosen segment (vrlog_record). A job of
-- any other kind is refused. A clip job names its segment through
-- lidar_segment_clip_jobs, so its session_id stays empty. For a motion pass
-- session_id remains a note with no foreign key: sessions are re-derived from
-- the capture index, and a key would make a re-derive fail or erase the job.
--
-- A motion period ends no earlier than it starts, and duration_ns is
-- end_ns - start_ns. start_secs and end_secs count from the first frame the
-- motion pass analysed, which the row does not hold, so they are not checked.
--
-- Triggers, not table rebuilds: lidar_segment_clip_jobs references
-- lidar_capture_jobs, and no stored row has to change. The triggers look only
-- at new writes. A clip job with no link row, which an interrupted enqueue
-- could leave before migration 55, still names its segment in session_id:
-- migration 55 cleared the column only where it found a link.
CREATE TRIGGER lidar_capture_jobs_check_insert BEFORE INSERT ON lidar_capture_jobs BEGIN
   SELECT RAISE (ABORT, 'a capture job is a motion_pass or a vrlog_record')
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record');

   SELECT RAISE (
          ABORT
        , 'a clip job names its segment in lidar_segment_clip_jobs, not in session_id'
          )
    WHERE NEW.kind = 'vrlog_record'
      AND NEW.session_id IS NOT NULL;

END;

CREATE TRIGGER lidar_capture_jobs_check_update BEFORE
   UPDATE OF kind
        , session_id ON lidar_capture_jobs BEGIN
   SELECT RAISE (ABORT, 'a capture job is a motion_pass or a vrlog_record')
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record');

   SELECT RAISE (
          ABORT
        , 'a clip job names its segment in lidar_segment_clip_jobs, not in session_id'
          )
    WHERE NEW.kind = 'vrlog_record'
      AND NEW.session_id IS NOT NULL;

END;

CREATE TRIGGER lidar_capture_motion_periods_check_insert BEFORE INSERT ON lidar_capture_motion_periods BEGIN
   SELECT RAISE (ABORT, 'a motion period ends no earlier than it starts')
    WHERE NEW.end_ns < NEW.start_ns;

   SELECT RAISE (ABORT, 'a motion period lasts end_ns - start_ns')
    WHERE NEW.duration_ns != NEW.end_ns - NEW.start_ns;

END;

CREATE TRIGGER lidar_capture_motion_periods_check_update BEFORE
   UPDATE OF start_ns
        , end_ns
        , duration_ns ON lidar_capture_motion_periods BEGIN
   SELECT RAISE (ABORT, 'a motion period ends no earlier than it starts')
    WHERE NEW.end_ns < NEW.start_ns;

   SELECT RAISE (ABORT, 'a motion period lasts end_ns - start_ns')
    WHERE NEW.duration_ns != NEW.end_ns - NEW.start_ns;

END;
