-- Raw packet storage table (commented out - use wireshark/pcaps instead)
/*
CREATE TABLE IF NOT EXISTS lidar_packets (
id INTEGER PRIMARY KEY AUTOINCREMENT
, write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
, source_address TEXT NOT NULL
, packet_size INTEGER NOT NULL
, packet_data BLOB NOT NULL
, packet_hex TEXT NOT NULL
);
 */
   CREATE TABLE IF NOT EXISTS lidar_points (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , packet_id INTEGER NOT NULL
        , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , x REAL
        , y REAL
        , z REAL intensity INTEGER
        , timestamp_ns BIGINT
        , azimuth REAL
        , distance REAL
          -- FOREIGN KEY (packet_id) REFERENCES lidar_packets (id) -- Commented out since lidar_packets table is disabled
          )
;

   CREATE TABLE IF NOT EXISTS lidar_sessions (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , start_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
        , end_timestamp DOUBLE
        , source_address TEXT NOT NULL
        , packet_count INTEGER DEFAULT 0
        , points_count INTEGER DEFAULT 0
        , session_notes TEXT
          )
;

-- Indexes for better query performance
-- Raw packet indexes (commented out since lidar_packets table is disabled)
/*
CREATE INDEX IF NOT EXISTS idx_lidar_packets_timestamp ON lidar_packets(write_timestamp);
CREATE INDEX IF NOT EXISTS idx_lidar_packets_source ON lidar_packets(source_address);
 */
CREATE INDEX IF NOT EXISTS idx_lidar_points_packet_id ON lidar_points (packet_id)
;

CREATE INDEX IF NOT EXISTS idx_lidar_points_timestamp ON lidar_points (write_timestamp)
;

CREATE INDEX IF NOT EXISTS idx_lidar_sessions_timestamp ON lidar_sessions (start_timestamp)
;
