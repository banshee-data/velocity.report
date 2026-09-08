-- Migration: Capture index for multi-file replay cases
-- Date: 2026-09-06
-- Description: Records the capture volumes an operator has configured, the PCAP
-- files found on them, and the contiguous sessions those files form. A replay
-- case whose window spans a file boundary needs to know which files abut, and
-- that needs the files' packet-time extents kept somewhere.
-- Capture roots are operator-configured directories. They are deliberately not
-- creatable from the web API: the safe-directory boundary that keeps replay from
-- reading arbitrary paths is only a boundary if the set of roots comes from the
-- process configuration.
   CREATE TABLE IF NOT EXISTS lidar_capture_roots (
          root_id TEXT PRIMARY KEY
        , path TEXT NOT NULL UNIQUE
        , label TEXT NOT NULL DEFAULT ''
        , enabled INTEGER NOT NULL DEFAULT 1
        , last_scan_at_ns INTEGER
        , last_scan_state TEXT NOT NULL DEFAULT 'never'
        , last_scan_error TEXT NOT NULL DEFAULT ''
        , created_at_ns INTEGER NOT NULL
        , updated_at_ns INTEGER NOT NULL
        , CHECK (last_scan_state IN ('never', 'ok', 'unreachable', 'error'))
          );

CREATE INDEX IF NOT EXISTS idx_lidar_capture_roots_enabled ON lidar_capture_roots (enabled);

-- One row per capture file seen on a root.
--
-- size_bytes, modified_at_ns and content_tag are cheap and recorded on every
-- scan. The packet extents are not: probing them reads the whole file, so they
-- stay NULL until a probe runs and probe_state says whether one has.
--
-- present is how a file that has left the disk is remembered rather than
-- deleted: a case referencing it must still be able to say what it lost.
   CREATE TABLE IF NOT EXISTS lidar_capture_files (
          capture_file_id TEXT PRIMARY KEY
        , root_id TEXT NOT NULL
        , rel_path TEXT NOT NULL
        , size_bytes INTEGER NOT NULL
        , modified_at_ns INTEGER NOT NULL
        , content_tag TEXT NOT NULL DEFAULT ''
        , first_packet_ns INTEGER
        , last_packet_ns INTEGER
        , packet_count INTEGER
        , udp_port INTEGER
        , probe_state TEXT NOT NULL DEFAULT 'pending'
        , probe_error TEXT NOT NULL DEFAULT ''
        , probed_at_ns INTEGER
        , present INTEGER NOT NULL DEFAULT 1
        , first_seen_at_ns INTEGER NOT NULL
        , last_seen_at_ns INTEGER NOT NULL
        , session_id TEXT
        , CHECK (probe_state IN ('pending', 'ok', 'failed'))
        , FOREIGN KEY (root_id) REFERENCES lidar_capture_roots (root_id) ON DELETE CASCADE
        , UNIQUE (root_id, rel_path)
          );

CREATE INDEX IF NOT EXISTS idx_lidar_capture_files_root ON lidar_capture_files (root_id, present);

CREATE INDEX IF NOT EXISTS idx_lidar_capture_files_session ON lidar_capture_files (session_id);

CREATE INDEX IF NOT EXISTS idx_lidar_capture_files_start ON lidar_capture_files (first_packet_ns);

-- Sessions are derived from packet-time adjacency, not authored. Re-deriving
-- replaces them wholesale, so nothing an operator typed may live here except
-- label, which survives by session_id.
   CREATE TABLE IF NOT EXISTS lidar_capture_sessions (
          session_id TEXT PRIMARY KEY
        , root_id TEXT NOT NULL
        , label TEXT NOT NULL DEFAULT ''
        , sensor_id TEXT NOT NULL DEFAULT ''
        , file_count INTEGER NOT NULL DEFAULT 0
        , start_ns INTEGER NOT NULL
        , end_ns INTEGER NOT NULL
        , covered_ns INTEGER NOT NULL DEFAULT 0
        , lost_ns INTEGER NOT NULL DEFAULT 0
        , worst_seam TEXT NOT NULL DEFAULT 'seamless'
        , size_bytes INTEGER NOT NULL DEFAULT 0
        , derived_at_ns INTEGER NOT NULL
        , FOREIGN KEY (root_id) REFERENCES lidar_capture_roots (root_id) ON DELETE CASCADE
          );

CREATE INDEX IF NOT EXISTS idx_lidar_capture_sessions_root ON lidar_capture_sessions (root_id, start_ns);
