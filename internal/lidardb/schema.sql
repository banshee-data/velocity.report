-- ---------------------------------------------------------------------------
-- Recommended pragmas
-- ---------------------------------------------------------------------------
PRAGMA journal_mode = WAL
;

PRAGMA synchronous = NORMAL
;

PRAGMA temp_store = MEMORY
;

PRAGMA busy_timeout = 5000
;

-- ---------------------------------------------------------------------------
-- Sites, sensors, poses (frames & transforms)
-- Units: meters, radians. Timestamps: unix nanoseconds (INTEGER).
-- ---------------------------------------------------------------------------
   CREATE TABLE sites (
          site_id TEXT PRIMARY KEY
        , world_frame TEXT NOT NULL /* e.g. "site/main-st-001" */
          )
;

   CREATE TABLE sensors (
          sensor_id TEXT PRIMARY KEY
        , site_id TEXT NOT NULL
        , type TEXT NOT NULL CHECK (type IN ('lidar', 'radar'))
        , model TEXT
        , UNIQUE (sensor_id)
        , FOREIGN KEY (site_id) REFERENCES sites (site_id)
          )
;

-- Pose history: transform from sensor frame -> site/world frame.
-- T_rowmajor_4x4 stores 16 float32/64 row-major values (binary blob).
   CREATE TABLE sensor_poses (
          pose_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , from_frame TEXT NOT NULL /* e.g. "sensor/hesai-01" */
        , to_frame TEXT NOT NULL /* e.g. "site/main-st-001" */
        , T_rowmajor_4x4 BLOB NOT NULL /* 16 floats, row-major */
        , valid_from_ns INTEGER NOT NULL
        , valid_to_ns INTEGER /* NULL = current */
        , method TEXT /* "tape+square","plane-fit",... */
        , rmse_m REAL
        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          )
;

CREATE INDEX idx_sensor_poses_sensor_time ON sensor_poses (sensor_id, valid_from_ns)
;

-- ---------------------------------------------------------------------------
-- LiDAR background (sensor frame) - periodic snapshots
-- Store the range-image background as a compressed blob (e.g., zstd).
-- Automatic persistence: after 5 min settling, then every 2 hours.
-- ---------------------------------------------------------------------------
   CREATE TABLE lidar_bg_snapshot (
          snapshot_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , taken_unix_nanos INTEGER NOT NULL
        , rings INTEGER NOT NULL
        , azimuth_bins INTEGER NOT NULL /* e.g., 1800 for 0.2° */
        , params_json TEXT NOT NULL /* dial settings used when taken */
        , grid_blob BLOB NOT NULL /* compressed []BackgroundCell */
        , changed_cells_count INTEGER
        , snapshot_reason TEXT /* 'settling_complete', 'periodic_update', 'manual' */
        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          )
;

CREATE INDEX idx_bg_snapshot_sensor_time ON lidar_bg_snapshot (sensor_id, taken_unix_nanos)
;

-- ---------------------------------------------------------------------------
-- LiDAR foreground clusters (WORLD frame)
-- One row per cluster per frame after BG subtraction + clustering.
-- ---------------------------------------------------------------------------
   CREATE TABLE lidar_clusters (
          lidar_cluster_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , pose_id INTEGER NOT NULL /* which transform was applied */
        , world_frame TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , centroid_x REAL
        , centroid_y REAL
        , centroid_z REAL
        , bbox_l REAL
        , bbox_w REAL
        , bbox_h REAL
        , points_count INTEGER
        , height_p95 REAL
        , intensity_mean REAL /* Optional sensor-frame hints for debugging */
        , sensor_ring_hint INTEGER
        , sensor_azimuth_deg_hint REAL
        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
        , FOREIGN KEY (pose_id) REFERENCES sensor_poses (pose_id)
          )
;

CREATE INDEX idx_lidar_clusters_time ON lidar_clusters (world_frame, ts_unix_nanos)
;

CREATE INDEX idx_lidar_clusters_sensor_time ON lidar_clusters (sensor_id, ts_unix_nanos)
;

-- ---------------------------------------------------------------------------
-- LiDAR tracks (episodes) in WORLD frame + their time series
-- ---------------------------------------------------------------------------
   CREATE TABLE lidar_tracks (
          track_id TEXT PRIMARY KEY /* stable ID per episode */
        , sensor_id TEXT NOT NULL
        , pose_id INTEGER NOT NULL
        , world_frame TEXT NOT NULL
        , start_unix_nanos INTEGER NOT NULL
        , end_unix_nanos INTEGER /* NULL if active */
        , class_label TEXT /* "", "car","ped","bird","other" */
        , class_conf REAL
        , peak_speed_mps REAL
        , avg_speed_mps REAL
        , height_p95_max REAL
        , intensity_mean_avg REAL
        , obs_count INTEGER
        , bbox_l_avg REAL
        , bbox_w_avg REAL
        , bbox_h_avg REAL
        , source_mask INTEGER DEFAULT 1 /* bit0=lidar, bit1=radar (later) */
        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
        , FOREIGN KEY (pose_id) REFERENCES sensor_poses (pose_id)
          )
;

-- Enhanced for 100 concurrent tracks with better performance indices
CREATE INDEX idx_lidar_tracks_site_time ON lidar_tracks (world_frame, start_unix_nanos)
;

CREATE INDEX idx_lidar_tracks_sensor_time ON lidar_tracks (sensor_id, start_unix_nanos)
;

CREATE INDEX idx_lidar_tracks_active ON lidar_tracks (world_frame, end_unix_nanos)
    WHERE end_unix_nanos IS NULL
;

   CREATE TABLE lidar_track_obs (
          track_id TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , world_frame TEXT NOT NULL
        , pose_id INTEGER NOT NULL
        , x REAL
        , y REAL
        , z REAL
        , vx REAL
        , vy REAL
        , vz REAL
        , speed_mps REAL
        , heading_rad REAL
        , bbox_l REAL
        , bbox_w REAL
        , bbox_h REAL
        , height_p95 REAL
        , intensity_mean REAL
        , PRIMARY KEY (track_id, ts_unix_nanos)
        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id)
        , FOREIGN KEY (pose_id) REFERENCES sensor_poses (pose_id)
          )
;

-- Optimized for efficient track state queries (100 concurrent tracks)
CREATE INDEX idx_track_obs_time ON lidar_track_obs (ts_unix_nanos)
;

CREATE INDEX idx_track_obs_track ON lidar_track_obs (track_id)
;

CREATE INDEX idx_track_obs_track_time ON lidar_track_obs (track_id, ts_unix_nanos DESC)
;

-- Denormalized per-track feature vectors for training/export.
   CREATE TABLE lidar_track_features (
          track_id TEXT PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , duration_s REAL
        , distance_m REAL
        , peak_speed_mps REAL
        , avg_speed_mps REAL
        , accel_p95_mps2 REAL
        , jerk_p95_mps3 REAL
        , bbox_l_avg REAL
        , bbox_w_avg REAL
        , bbox_h_avg REAL
        , height_p50 REAL
        , height_p95 REAL
        , intensity_mean_avg REAL
        , intensity_std REAL
        , lateral_oscillation_m REAL
        , near_ground_ratio REAL
        , far_range_ratio REAL
        , label TEXT /* human label if provided */
        , label_source TEXT /* 'human','rule','model' */
        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id)
          )
;

-- Optional human labeling log
   CREATE TABLE labels (
          label_id INTEGER PRIMARY KEY
        , track_id TEXT NOT NULL
        , labeled_unix_nanos INTEGER NOT NULL
        , label TEXT NOT NULL /* car/ped/bird/other/ignore */
        , who TEXT
        , reason TEXT
        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id)
          )
;

CREATE INDEX idx_labels_track ON labels (track_id)
;

-- Helpful training view
   CREATE VIEW v_training AS
   SELECT               f.*
        , t.class_label AS label_final
     FROM lidar_track_features f
     JOIN lidar_tracks t USING (track_id)
    WHERE COALESCE(t.class_label, '') != ''
      AND t.class_label != 'ignore'
;

-- ---------------------------------------------------------------------------
-- Radar (WORLD frame coordinates available for spatial join)
-- Store both native polar measurements and derived XY in world frame.
-- Enhanced for fusion with processing latency tracking.
-- ---------------------------------------------------------------------------
   CREATE TABLE radar_observations (
          radar_obs_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , pose_id INTEGER NOT NULL
        , world_frame TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL /* native polar */
        , range_m REAL NOT NULL
        , azimuth_deg REAL NOT NULL
        , radial_speed_mps REAL
        , snr REAL /* derived (projected to road plane in world frame) */
        , x REAL
        , y REAL
        , quality INTEGER
        , received_unix_nanos INTEGER NOT NULL /* when radar process received it */
        , processed_unix_nanos INTEGER /* when lidar process handled it */
        , processing_latency_us INTEGER /* receive to process time */
        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
        , FOREIGN KEY (pose_id) REFERENCES sensor_poses (pose_id)
          )
;

CREATE INDEX idx_radar_time ON radar_observations (world_frame, ts_unix_nanos)
;

CREATE INDEX idx_radar_sensor_time ON radar_observations (sensor_id, ts_unix_nanos)
;

-- Raw radar lines (optional, if you capture them)
   CREATE TABLE radar_lines (
          radar_line_id INTEGER PRIMARY KEY
        , sensor_id TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL
        , angle_deg REAL
        , range_m REAL
        , intensity REAL
        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          )
;

CREATE INDEX idx_radar_lines_sensor_time ON radar_lines (sensor_id, ts_unix_nanos)
;

-- ---------------------------------------------------------------------------
-- Associations & fusion ledger (WORLD frame)
-- Unified table for all sensor association events (radar-lidar, future sensors)
-- ---------------------------------------------------------------------------
   CREATE TABLE sensor_associations (
          assoc_id INTEGER PRIMARY KEY
        , world_frame TEXT NOT NULL
        , ts_unix_nanos INTEGER NOT NULL /* association time */
        , track_id TEXT
        , radar_obs_id INTEGER
        , lidar_cluster_id INTEGER
        , association_method TEXT /* 'mahalanobis', 'nearest', 'kalman' */
        , cost REAL /* e.g., Mahalanobis distance */
        , association_quality TEXT CHECK (association_quality IN ('high', 'medium', 'low'))
        , fused_x REAL
        , fused_y REAL
        , fused_vx REAL
        , fused_vy REAL
        , fused_speed_mps REAL
        , fused_cov_blob BLOB /* 16 floats row-major (optional) */
        , source_mask INTEGER DEFAULT 3 /* bit0=lidar, bit1=radar, bit2=future */
        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id)
        , FOREIGN KEY (radar_obs_id) REFERENCES radar_observations (radar_obs_id)
        , FOREIGN KEY (lidar_cluster_id) REFERENCES lidar_clusters (lidar_cluster_id)
          )
;

CREATE INDEX idx_sensor_assoc_time ON sensor_associations (world_frame, ts_unix_nanos)
;

CREATE INDEX idx_sensor_assoc_track ON sensor_associations (track_id)
;

CREATE INDEX idx_sensor_assoc_radar ON sensor_associations (radar_obs_id)
;

-- ---------------------------------------------------------------------------
-- System monitoring and events
-- Unified table for performance metrics, track events, and system health
-- ---------------------------------------------------------------------------
   CREATE TABLE system_events (
          event_id INTEGER PRIMARY KEY
        , sensor_id TEXT /* NULL for system-wide events */
        , ts_unix_nanos INTEGER NOT NULL
        , event_type TEXT NOT NULL CHECK (
          event_type IN (
          'performance'
        , 'track_birth'
        , 'track_death'
        , 'track_merge'
        , 'track_split'
        , 'system_start'
        , 'system_stop'
        , 'background_snapshot'
          )
          )
        , event_data JSON /* flexible storage for different event types */
        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          )
;

CREATE INDEX idx_system_events_time ON system_events (ts_unix_nanos)
;

CREATE INDEX idx_system_events_type ON system_events (event_type, ts_unix_nanos)
;

CREATE INDEX idx_system_events_sensor ON system_events (sensor_id, ts_unix_nanos)
;

-- ---------------------------------------------------------------------------
-- Optional: spatial acceleration with R*Tree (requires SQLite RTREE)
-- Keeps approximate XY bounding boxes for quick neighborhood search.
-- Uncomment if your build supports it.
-- ---------------------------------------------------------------------------
-- CREATE VIRTUAL TABLE rtree_radar_obs USING rtree(
--   radar_obs_id, minX, maxX, minY, maxY
-- );
-- CREATE VIRTUAL TABLE rtree_track_obs USING rtree(
--   rowid, minX, maxX, minY, maxY
-- );
-- (Maintain via triggers that mirror inserts/updates into *_obs tables.)
-- ---------------------------------------------------------------------------
-- Convenience views
-- ---------------------------------------------------------------------------
-- Latest observation per active track (handy for UI)
   CREATE VIEW v_tracks_latest AS
     WITH last_ts AS (
             SELECT                    track_id
                  , MAX(ts_unix_nanos) AS ts
               FROM lidar_track_obs
           GROUP BY track_id
          )
   SELECT                  t.track_id
        , t.sensor_id
        , t.world_frame
        , t.class_label
        , t.class_conf
        , o.ts_unix_nanos
        , o.x
        , o.y
        , o.z
        , o.speed_mps
        , o.heading_rad
        , t.peak_speed_mps
        , t.avg_speed_mps
     FROM lidar_tracks t
     JOIN last_ts lt USING (track_id)
     JOIN lidar_track_obs o ON o.track_id = lt.track_id
      AND o.ts_unix_nanos = lt.ts
;

-- Tracks with any radar confirmation
   CREATE VIEW v_tracks_with_radar AS
   SELECT DISTINCT      t.track_id
        , t.world_frame
     FROM lidar_tracks t
     JOIN sensor_associations a ON a.track_id = t.track_id
      AND a.radar_obs_id IS NOT NULL
;

-- System performance summary for monitoring 100-track capacity
   CREATE VIEW v_system_performance AS
     WITH recent_performance AS (
             SELECT JSON_EXTRACT(event_data, '$.metric_name')                     AS metric_name
                  , AVG(CAST(JSON_EXTRACT(event_data, '$.metric_value') AS REAL)) AS avg_value
                  , MAX(CAST(JSON_EXTRACT(event_data, '$.metric_value') AS REAL)) AS max_value
                  , MIN(CAST(JSON_EXTRACT(event_data, '$.metric_value') AS REAL)) AS min_value
               FROM system_events
              WHERE event_type = 'performance'
                AND ts_unix_nanos > (STRFTIME('%s', 'now') - 300) * 1000000000 /* last 5 minutes */
           GROUP BY JSON_EXTRACT(event_data, '$.metric_name')
          )
   SELECT   rp.*
        , (
             SELECT COUNT(*)
               FROM lidar_tracks
              WHERE end_unix_nanos IS NULL
          ) AS active_tracks
        , (
             SELECT COUNT(*)
               FROM lidar_bg_snapshot
              WHERE taken_unix_nanos > (STRFTIME('%s', 'now') - 86400) * 1000000000
          ) AS bg_snapshots_24h
     FROM recent_performance rp
;

-- Track activity summary
   CREATE VIEW v_track_activity AS
   SELECT          t.world_frame
        , COUNT(*) AS total_tracks
        , COUNT(
          CASE
                    WHEN t.end_unix_nanos IS NULL THEN 1
          END
          ) AS active_tracks
        , AVG(t.peak_speed_mps) AS avg_peak_speed
        , AVG((t.end_unix_nanos - t.start_unix_nanos) / 1e9) AS avg_duration_s
     FROM lidar_tracks t
    WHERE t.start_unix_nanos > (STRFTIME('%s', 'now') - 86400) * 1000000000 /* last 24 hours */
 GROUP BY t.world_frame
;

-- Track lifecycle events summary
   CREATE VIEW v_track_lifecycle AS
   SELECT DATE(ts_unix_nanos / 1000000000, 'unixepoch') AS event_date
        , event_type
        , COUNT(*)                                      AS event_count
     FROM system_events
    WHERE event_type IN ('track_birth', 'track_death', 'track_merge', 'track_split')
      AND ts_unix_nanos > (STRFTIME('%s', 'now') - 86400) * 1000000000 /* last 24 hours */
 GROUP BY event_date
        , event_type
 ORDER BY event_date DESC
        , event_type
;
