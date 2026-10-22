-- A clip can be extracted to one standalone capture and published as a survey.
-- dataset_id is the published survey's id: the site-index id and the manifest
-- site_slug. It is unique because one published capture answers to one id.
    ALTER TABLE lidar_clips
      ADD COLUMN dataset_id TEXT;

CREATE UNIQUE INDEX idx_lidar_clips_dataset_id ON lidar_clips (dataset_id)
    WHERE dataset_id IS NOT NULL;

-- The scene viewer's measured north azimuth, as site-index.json records it.
-- It is not the bearing of the sensor's +Y; see gerardus-alignment.md.
    ALTER TABLE lidar_clips
      ADD COLUMN north_azimuth_deg REAL CHECK (
north_azimuth_deg IS NULL
       OR (
          north_azimuth_deg >= 0
      AND north_azimuth_deg < 360
          )
          );

-- What a clip replays: its source captures, or its latest completed extract.
    ALTER TABLE lidar_clips
      ADD COLUMN storage TEXT NOT NULL DEFAULT 'sources' CHECK (storage IN ('sources', 'extract'));

-- One row per capture_export job; the file fields are filled when it completes.
   CREATE TABLE lidar_clip_extracts (
          job_id TEXT PRIMARY KEY
        , clip_id TEXT NOT NULL
        , rel_path TEXT
        , sha256 TEXT
        , size_bytes INTEGER
        , packet_count INTEGER
        , lidar_packet_count INTEGER
        , frame_count INTEGER
        , first_packet_ns INTEGER
        , last_packet_ns INTEGER
        , closed_by TEXT
        , rubric_version INTEGER NOT NULL
        , sidecar_json TEXT
        , created_at_ns INTEGER NOT NULL
        , completed_at_ns INTEGER
        , CHECK (
          rel_path IS NULL
       OR (
          rel_path != ''
      AND SUBSTR(rel_path, 1, 1) != '/'
      AND INSTR(rel_path, '..') = 0
      AND INSTR(rel_path, CHAR(92)) = 0
          )
          )
        , CHECK (
          sha256 IS NULL
       OR (
          LENGTH(sha256) = 71
      AND SUBSTR(sha256, 1, 7) = 'sha256:'
      AND SUBSTR(sha256, 8) NOT GLOB '*[^0-9a-f]*'
          )
          )
        , CHECK ((rel_path IS NULL) = (sha256 IS NULL))
        , CHECK (
          closed_by IS NULL
       OR closed_by IN ('window', 'end')
          )
        , CHECK (
          sidecar_json IS NULL
       OR (
          JSON_VALID(sidecar_json)
      AND JSON_TYPE(sidecar_json) = 'object'
          )
          )
        , FOREIGN KEY (job_id) REFERENCES lidar_capture_jobs (job_id) ON DELETE CASCADE
        , FOREIGN KEY (clip_id) REFERENCES lidar_clips (clip_id) ON DELETE CASCADE
          );

CREATE INDEX idx_lidar_clip_extracts_clip ON lidar_clip_extracts (clip_id, created_at_ns);

-- A capture_export job names its clip in lidar_clip_extracts, as a clip job
-- names its segment in lidar_segment_clip_jobs.
     DROP TRIGGER lidar_capture_jobs_check_insert;

     DROP TRIGGER lidar_capture_jobs_check_update;

CREATE TRIGGER lidar_capture_jobs_check_insert BEFORE INSERT ON lidar_capture_jobs BEGIN
   SELECT RAISE (
          ABORT
        , 'a capture job is a motion_pass, a vrlog_record or a capture_export'
          )
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record', 'capture_export');

   SELECT RAISE (
          ABORT
        , 'a clip job names its segment in lidar_segment_clip_jobs, not in session_id'
          )
    WHERE NEW.kind = 'vrlog_record'
      AND NEW.session_id IS NOT NULL;

   SELECT RAISE (
          ABORT
        , 'an export job names its clip in lidar_clip_extracts, not in session_id'
          )
    WHERE NEW.kind = 'capture_export'
      AND NEW.session_id IS NOT NULL;

END;

CREATE TRIGGER lidar_capture_jobs_check_update BEFORE
   UPDATE OF kind
        , session_id ON lidar_capture_jobs BEGIN
   SELECT RAISE (
          ABORT
        , 'a capture job is a motion_pass, a vrlog_record or a capture_export'
          )
    WHERE NEW.kind NOT IN ('motion_pass', 'vrlog_record', 'capture_export');

   SELECT RAISE (
          ABORT
        , 'a clip job names its segment in lidar_segment_clip_jobs, not in session_id'
          )
    WHERE NEW.kind = 'vrlog_record'
      AND NEW.session_id IS NOT NULL;

   SELECT RAISE (
          ABORT
        , 'an export job names its clip in lidar_clip_extracts, not in session_id'
          )
    WHERE NEW.kind = 'capture_export'
      AND NEW.session_id IS NOT NULL;

END;
