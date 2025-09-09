-- ---------------------------------------------------------------------------
-- SQLite PRAGMA configuration for high-performance LiDAR tracking system
--
-- These settings optimize SQLite for real-time sensor data processing with:
-- - High write throughput (track observations, cluster detections)
-- - Concurrent read access (UI queries, background processing)
-- - Memory efficiency for 100 concurrent tracks
-- - Robust crash recovery for 24/7 operation
-- ---------------------------------------------------------------------------
-- Enable Write-Ahead Logging for better concurrency
-- Allows readers and writers to operate simultaneously without blocking
PRAGMA journal_mode = WAL
;

-- Use normal synchronous mode for balance of safety and performance
-- Reduces fsync calls while maintaining reasonable crash recovery
PRAGMA synchronous = NORMAL
;

-- Store temporary tables and indices in memory for faster processing
-- Improves performance for complex queries and joins
PRAGMA temp_store = MEMORY
;

-- Set busy timeout for handling concurrent access
-- Prevents immediate failures when database is locked by other processes
PRAGMA busy_timeout = 5000
;

-- ---------------------------------------------------------------------------
-- Sites, sensors, poses (frames & transforms)
-- Units: meters, radians. Timestamps: unix nanoseconds (INTEGER).
-- ---------------------------------------------------------------------------
/*
 * Sites table: Physical deployment locations for sensor networks
 *
 * This table defines the top-level physical sites where sensors are deployed.
 * Each site represents a real-world location (e.g., intersection, highway segment)
 * with its own coordinate system and reference frame.
 */
   CREATE TABLE sites (
          -- Unique identifier for the physical site (e.g., "main-st-001", "highway-i95-mm123")
          site_id TEXT PRIMARY KEY
          -- The world/global coordinate frame name for this site. This defines the reference
          -- coordinate system that all measurements at this site will be transformed into.
          -- Typically follows naming convention like "site/main-st-001" to match site_id.

        , world_frame TEXT NOT NULL /* e.g. "site/main-st-001" */
          )
;

/*
 * Sensors table: Individual sensor hardware units deployed at sites
 *
 * This table catalogs all sensor hardware (LiDAR, radar, etc.) deployed across
 * sites. Each sensor has a unique identity and is associated with a specific site.
 * Sensors may be recalibrated, moved, or replaced over time.
 */
   CREATE TABLE sensors (
          -- Unique identifier for the sensor hardware unit (e.g., "hesai-pandar64-001")
          sensor_id TEXT PRIMARY KEY
          -- Reference to the site where this sensor is deployed

        , site_id TEXT NOT NULL
          -- Type of sensor technology: 'lidar' for LiDAR units, 'radar' for radar units
          -- Additional types may be added in future (e.g., 'camera', 'thermal')

        , type TEXT NOT NULL CHECK (type IN ('lidar', 'radar'))
          -- Manufacturer model/part number (e.g., "Pandar64", "AWR2944"). Optional for
          -- cases where sensor model is unknown or mixed within same sensor_id.

        , model TEXT
          -- Redundant unique constraint to ensure sensor_id uniqueness across sites

        , UNIQUE (sensor_id)
          -- Foreign key ensuring sensor is deployed at a valid site

        , FOREIGN KEY (site_id) REFERENCES sites (site_id)
          )
;

/*
 * Sensor poses table: Time-versioned 3D transformations for sensor calibration
 *
 * This table stores the spatial transformation matrices that convert measurements
 * from each sensor's local coordinate frame to the site's world coordinate frame.
 * Multiple poses per sensor support recalibration events and temporal calibration drift.
 * Transform from sensor frame -> site/world frame.
 * T_rowmajor_4x4 stores 16 float32/64 row-major values (binary blob).
 */
   CREATE TABLE sensor_poses (
          -- Auto-incrementing unique identifier for this pose record
          pose_id INTEGER PRIMARY KEY
          -- Reference to the sensor this pose applies to

        , sensor_id TEXT NOT NULL
          -- Source coordinate frame name (e.g., "sensor/hesai-01") - the sensor's local frame

        , from_frame TEXT NOT NULL /* e.g. "sensor/hesai-01" */
          -- Target coordinate frame name (e.g., "site/main-st-001") - the site's world frame

        , to_frame TEXT NOT NULL /* e.g. "site/main-st-001" */
          -- 4x4 transformation matrix stored as binary blob of 16 float values in row-major order.
          -- This matrix transforms homogeneous coordinates from from_frame to to_frame.
          -- Typically obtained through calibration procedures using known targets.

        , T_rowmajor_4x4 BLOB NOT NULL /* 16 floats, row-major */
          -- Unix nanoseconds when this pose calibration becomes valid/active

        , valid_from_ns INTEGER NOT NULL
          -- Unix nanoseconds when this pose expires. NULL means currently active.
          -- Allows for temporal calibration updates without losing historical data.

        , valid_to_ns INTEGER /* NULL = current */
          -- Calibration method used to determine this pose (e.g., "tape+square", "plane-fit"
          -- "checkerboard", "manual"). Helps track calibration quality and repeatability.

        , method TEXT /* "tape+square","plane-fit",... */
          -- Root mean square error in meters from calibration procedure. Lower values
          -- indicate higher precision calibration. Used for pose quality assessment.

        , rmse_m REAL
          -- Foreign key ensuring pose belongs to a valid sensor

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
/*
 * LiDAR background snapshot table: Persistent storage for environmental background models
 *
 * This table stores compressed snapshots of the static background environment as seen
 * by each LiDAR sensor. The background model is used for foreground/background separation
 * to detect moving objects. Snapshots are taken automatically after system settling
 * and then periodically to account for environmental changes (weather, construction, etc.).
 * Data is stored in sensor frame coordinates for computational efficiency.
 */
   CREATE TABLE lidar_bg_snapshot (
          -- Auto-incrementing unique identifier for this background snapshot
          snapshot_id INTEGER PRIMARY KEY
          -- Reference to the LiDAR sensor that captured this background

        , sensor_id TEXT NOT NULL
          -- Unix nanoseconds timestamp when this background snapshot was captured

        , taken_unix_nanos INTEGER NOT NULL
          -- Number of vertical rings/channels in the LiDAR sensor (e.g., 64 for Pandar64)

        , rings INTEGER NOT NULL
          -- Number of horizontal azimuth discretization bins (e.g., 1800 for 0.2° resolution)
          -- Determines angular resolution of the background grid

        , azimuth_bins INTEGER NOT NULL /* e.g., 1800 for 0.2° */
          -- JSON string storing the dial/parameter settings active when this snapshot was taken.
          -- Includes background subtraction thresholds, update rates, sensitivity settings, etc.
          -- Allows correlation of background quality with specific parameter configurations.

        , params_json TEXT NOT NULL /* dial settings used when taken */
          -- Compressed binary blob storing the background model grid. Contains an array of
          -- BackgroundCell structures with statistics per (ring, azimuth) bin.
          -- Compression (e.g., zstd) reduces storage overhead for large background models.

        , grid_blob BLOB NOT NULL /* compressed []BackgroundCell */
          -- Number of grid cells that changed since the previous snapshot. Used to track
          -- environmental stability and determine when new snapshots are needed.

        , changed_cells_count INTEGER
          -- Reason why this snapshot was taken: 'settling_complete' (initial 5min settling)
          -- 'periodic_update' (scheduled 2hr updates), 'manual' (user-initiated)

        , snapshot_reason TEXT /* 'settling_complete', 'periodic_update', 'manual' */
          -- Foreign key ensuring snapshot belongs to a valid LiDAR sensor

        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          )
;

CREATE INDEX idx_bg_snapshot_sensor_time ON lidar_bg_snapshot (sensor_id, taken_unix_nanos)
;

-- ---------------------------------------------------------------------------
-- LiDAR foreground clusters (WORLD frame)
-- One row per cluster per frame after BG subtraction + clustering.
-- ---------------------------------------------------------------------------
/*
 * LiDAR clusters table: Detected foreground object clusters in world coordinates
 *
 * This table stores individual clusters of LiDAR points detected as foreground objects
 * after background subtraction and spatial clustering. Each cluster represents a potential
 * moving object (vehicle, pedestrian, etc.). Clusters are transformed to world frame
 * coordinates to enable fusion with other sensors and consistent spatial analysis.
 * One row per cluster per LiDAR frame.
 */
   CREATE TABLE lidar_clusters (
          -- Auto-incrementing unique identifier for this cluster detection
          lidar_cluster_id INTEGER PRIMARY KEY
          -- Reference to the LiDAR sensor that detected this cluster

        , sensor_id TEXT NOT NULL
          -- Reference to the sensor pose used to transform this cluster to world frame

        , pose_id INTEGER NOT NULL /* which transform was applied */
          -- The world coordinate frame this cluster was transformed into

        , world_frame TEXT NOT NULL
          -- Unix nanoseconds timestamp when this cluster was detected

        , ts_unix_nanos INTEGER NOT NULL
          -- Geometric center X coordinate of the cluster in world frame (meters)

        , centroid_x REAL
          -- Geometric center Y coordinate of the cluster in world frame (meters)

        , centroid_y REAL
          -- Geometric center Z coordinate of the cluster in world frame (meters)

        , centroid_z REAL
          -- Bounding box length (longitudinal extent) in meters

        , bbox_l REAL
          -- Bounding box width (lateral extent) in meters

        , bbox_w REAL
          -- Bounding box height (vertical extent) in meters

        , bbox_h REAL
          -- Total number of LiDAR points contributing to this cluster

        , points_count INTEGER
          -- 95th percentile height of points in this cluster (meters). Robust height metric
          -- less sensitive to ground points or outliers than max height.

        , height_p95 REAL
          -- Mean intensity/reflectivity value of points in this cluster. Sensor-dependent
          -- units but useful for material classification (retroreflectors, metal, etc.)

        , intensity_mean REAL /* Optional sensor-frame hints for debugging */
          -- Hint: predominant LiDAR ring/channel that detected this cluster. Useful for
          -- debugging sensor coverage and cluster quality. Not used for tracking logic.

        , sensor_ring_hint INTEGER
          -- Hint: approximate azimuth angle (degrees) in sensor frame where cluster was detected.
          -- Useful for debugging and validating coordinate transformations.

        , sensor_azimuth_deg_hint REAL
          -- Foreign key ensuring cluster belongs to a valid LiDAR sensor

        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          -- Foreign key ensuring pose used for transformation is valid

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
/*
 * LiDAR tracks table: Temporal sequences of object detections (track episodes)
 *
 * This table stores track episodes - sequences of clusters that have been associated
 * over time to represent individual moving objects. Each track has a stable identifier
 * and spans from track birth to track death. Summary statistics are computed across
 * the entire track lifetime. System is optimized for 100 concurrent active tracks.
 * All coordinates are in world frame for consistent spatial analysis.
 */
   CREATE TABLE lidar_tracks (
          -- Stable unique identifier for this track episode (e.g., "track_20250909_001234")
          -- Remains constant throughout the track's lifetime for consistent data association
          track_id TEXT PRIMARY KEY /* stable ID per episode */
          -- Reference to the primary LiDAR sensor that initiated/owns this track

        , sensor_id TEXT NOT NULL
          -- Reference to the sensor pose active during track initiation

        , pose_id INTEGER NOT NULL
          -- The world coordinate frame this track exists in

        , world_frame TEXT NOT NULL
          -- Unix nanoseconds when this track was first detected/born

        , start_unix_nanos INTEGER NOT NULL
          -- Unix nanoseconds when this track ended/died. NULL indicates track is still active.
          -- Used to distinguish between active tracks (in memory) and completed tracks (archived).

        , end_unix_nanos INTEGER /* NULL if active */
          -- Classification label assigned to this track: "", "car", "ped" (pedestrian)
          -- "bird", "other". Empty string indicates unclassified. May be assigned by
          -- rules engine, ML model, or human annotation.

        , class_label TEXT /* "", "car","ped","bird","other" */
          -- Confidence score (0.0-1.0) for the assigned classification label.
          -- Higher values indicate more confident classification.

        , class_conf REAL
          -- Maximum instantaneous speed observed during this track's lifetime (m/s).
          -- Key feature for distinguishing vehicle types and validating track quality.

        , peak_speed_mps REAL
          -- Average speed computed across all track observations (m/s).
          -- More robust than peak speed for speed-based classification.

        , avg_speed_mps REAL
          -- Maximum 95th percentile height observed during track lifetime (meters).
          -- Robust height metric for distinguishing cars, trucks, pedestrians.

        , height_p95_max REAL
          -- Average intensity/reflectivity across all observations in this track.
          -- Useful for material-based classification (vehicles vs. people).

        , intensity_mean_avg REAL
          -- Total number of cluster observations contributing to this track.
          -- Higher counts indicate longer-duration, higher-quality tracks.

        , obs_count INTEGER
          -- Average bounding box length across all track observations (meters)

        , bbox_l_avg REAL
          -- Average bounding box width across all track observations (meters)

        , bbox_w_avg REAL
          -- Average bounding box height across all track observations (meters)

        , bbox_h_avg REAL
          -- Bitmask indicating which sensor types contributed to this track:
          -- bit0=LiDAR, bit1=radar, bit2=future sensors. Default 1 = LiDAR only.
          -- Enables tracking of multi-sensor fusion contributions.

        , source_mask INTEGER DEFAULT 1 /* bit0=lidar, bit1=radar (later) */
          -- Foreign key ensuring track belongs to a valid LiDAR sensor

        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          -- Foreign key ensuring pose used for track initiation is valid

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

/*
 * LiDAR track observations table: Time-series state estimates for each track
 *
 * This table stores the detailed time-series of state estimates (position, velocity, etc.)
 * for each track. Each row represents one temporal observation/update of a track's state.
 * Data enables trajectory analysis, smoothing, and detailed movement pattern analysis.
 * Optimized for efficient queries across 100 concurrent tracks with high temporal resolution.
 */
   CREATE TABLE lidar_track_obs (
          -- Reference to the parent track this observation belongs to
          track_id TEXT NOT NULL
          -- Unix nanoseconds timestamp for this state observation

        , ts_unix_nanos INTEGER NOT NULL
          -- World coordinate frame this observation is expressed in

        , world_frame TEXT NOT NULL
          -- Reference to sensor pose used for coordinate transformation

        , pose_id INTEGER NOT NULL
          -- Position X coordinate in world frame (meters)

        , x REAL
          -- Position Y coordinate in world frame (meters)

        , y REAL
          -- Position Z coordinate in world frame (meters)

        , z REAL
          -- Velocity X component in world frame (m/s)

        , vx REAL
          -- Velocity Y component in world frame (m/s)

        , vy REAL
          -- Velocity Z component in world frame (m/s)

        , vz REAL
          -- Instantaneous speed magnitude computed from velocity components (m/s)

        , speed_mps REAL
          -- Heading/bearing angle in radians (world frame). Typically yaw angle around Z-axis.

        , heading_rad REAL
          -- Bounding box length at this time instant (meters)

        , bbox_l REAL
          -- Bounding box width at this time instant (meters)

        , bbox_w REAL
          -- Bounding box height at this time instant (meters)

        , bbox_h REAL
          -- 95th percentile height of cluster points at this time instant (meters)

        , height_p95 REAL
          -- Mean intensity/reflectivity of cluster points at this time instant

        , intensity_mean REAL
          -- Composite primary key: unique observation per track per timestamp

        , PRIMARY KEY (track_id, ts_unix_nanos)
          -- Foreign key ensuring observation belongs to a valid track

        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id)
          -- Foreign key ensuring pose used for transformation is valid

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

/*
 * LiDAR track features table: Denormalized feature vectors for ML training and export
 *
 * This table stores pre-computed feature vectors extracted from completed tracks.
 * Features are designed for machine learning classification and training datasets.
 * Each track gets one feature vector computed after track completion, combining
 * spatial, temporal, and kinematic characteristics into a fixed-length vector
 * suitable for training classifiers or exporting to external ML pipelines.
 */
   CREATE TABLE lidar_track_features (
          -- Reference to the track these features were extracted from
          track_id TEXT PRIMARY KEY
          -- Reference to the sensor that generated this track

        , sensor_id TEXT NOT NULL
          -- Total track duration in seconds (end_time - start_time)

        , duration_s REAL
          -- Total distance traveled by the track in meters (path length)

        , distance_m REAL
          -- Maximum instantaneous speed observed during track lifetime (m/s)

        , peak_speed_mps REAL
          -- Average speed across entire track lifetime (m/s)

        , avg_speed_mps REAL
          -- 95th percentile acceleration magnitude during track (m/s²).
          -- Robust metric for distinguishing aggressive vs. smooth motion.

        , accel_p95_mps2 REAL
          -- 95th percentile jerk (acceleration derivative) during track (m/s³).
          -- Indicates smoothness of motion - vehicles typically have lower jerk than pedestrians.

        , jerk_p95_mps3 REAL
          -- Average bounding box length across all track observations (meters)

        , bbox_l_avg REAL
          -- Average bounding box width across all track observations (meters)

        , bbox_w_avg REAL
          -- Average bounding box height across all track observations (meters)

        , bbox_h_avg REAL
          -- Median (50th percentile) height across all track observations (meters)

        , height_p50 REAL
          -- 95th percentile height across all track observations (meters)

        , height_p95 REAL
          -- Average intensity/reflectivity across all track observations

        , intensity_mean_avg REAL
          -- Standard deviation of intensity across track observations.
          -- High std may indicate mixed materials or varying sensor angles.

        , intensity_std REAL
          -- Lateral oscillation amplitude in meters. Measures side-to-side movement
          -- relative to primary direction of travel. High values may indicate
          -- pedestrians, animals, or erratic vehicle behavior.

        , lateral_oscillation_m REAL
          -- Fraction of track observations where object was near ground level.
          -- Useful for distinguishing ground vehicles from flying objects/birds.

        , near_ground_ratio REAL
          -- Fraction of track observations at far range from sensor.
          -- May indicate track quality and classification confidence degradation.

        , far_range_ratio REAL
          -- Human-assigned label if available ("car", "ped", "bird", "other").
          -- Ground truth for supervised learning when human annotation exists.

        , label TEXT /* human label if provided */
          -- Source of the label: 'human' (manual annotation), 'rule' (rule-based classifier)
          -- 'model' (ML model prediction). Tracks label provenance for training quality.

        , label_source TEXT /* 'human','rule','model' */
          -- Foreign key ensuring features belong to a valid completed track

        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id)
          )
;

/*
 * Labels table: Human annotation log for ground truth generation
 *
 * This table records human labeling activities for creating ground truth datasets.
 * Multiple labels can be assigned to the same track over time as analysts review
 * and refine classifications. The labeling history is preserved for quality control
 * and inter-annotator agreement analysis. Used primarily for supervised ML training.
 */
   CREATE TABLE labels (
          -- Auto-incrementing unique identifier for this labeling event
          label_id INTEGER PRIMARY KEY
          -- Reference to the track being labeled

        , track_id TEXT NOT NULL
          -- Unix nanoseconds timestamp when this label was assigned

        , labeled_unix_nanos INTEGER NOT NULL
          -- Classification label assigned: "car", "ped" (pedestrian), "bird", "other", "ignore".
          -- "ignore" indicates track should be excluded from training due to poor quality
          -- ambiguity, or other issues identified by the human annotator.

        , label TEXT NOT NULL /* car/ped/bird/other/ignore */
          -- Identifier of the person who assigned this label (username, initials, etc.).
          -- Enables tracking of annotator performance and inter-annotator agreement.

        , who TEXT
          -- Free-text reason or justification for this labeling decision. Useful for
          -- understanding difficult cases and improving annotation guidelines.

        , reason TEXT
          -- Foreign key ensuring label is assigned to a valid track

        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id)
          )
;

CREATE INDEX idx_labels_track ON labels (track_id)
;

/*
 * Training view: Curated feature vectors with ground truth labels for ML training
 *
 * This view combines track features with finalized classification labels to create
 * a clean training dataset. Only includes tracks with valid, non-empty labels
 * and excludes tracks marked as 'ignore'. The result is a ready-to-use dataset
 * for supervised machine learning with features as input and label_final as target.
 * Automatically maintained as new tracks and features are added to the system.
 */
   CREATE VIEW v_training AS
   SELECT f.* /* All feature columns from lidar_track_features            */
        , t.class_label AS label_final /* Ground truth label for training    */
     FROM lidar_track_features f
     JOIN lidar_tracks t USING (track_id)
    WHERE COALESCE(t.class_label, '') != '' /* Exclude unlabeled tracks */
      AND t.class_label != 'ignore' /* Exclude tracks marked to ignore */
;

-- ---------------------------------------------------------------------------
-- Radar (WORLD frame coordinates available for spatial join)
-- Store both native polar measurements and derived XY in world frame.
-- Enhanced for fusion with processing latency tracking.
-- ---------------------------------------------------------------------------
/*
 * Radar observations table: Processed radar detections in world coordinates
 *
 * This table stores individual radar detections transformed to world frame coordinates
 * for spatial fusion with LiDAR data. Both native polar measurements (range, azimuth
 * radial velocity) and derived Cartesian coordinates are stored. Processing latency
 * metrics enable real-time performance monitoring and fusion quality assessment.
 * Each detection represents a potential moving object detected by radar.
 */
   CREATE TABLE radar_observations (
          -- Auto-incrementing unique identifier for this radar detection
          radar_obs_id INTEGER PRIMARY KEY
          -- Reference to the radar sensor that generated this detection

        , sensor_id TEXT NOT NULL
          -- Reference to sensor pose used to transform this detection to world frame

        , pose_id INTEGER NOT NULL
          -- World coordinate frame this detection was transformed into

        , world_frame TEXT NOT NULL
          -- Unix nanoseconds timestamp of the original radar measurement

        , ts_unix_nanos INTEGER NOT NULL /* native polar */
          -- Range/distance measurement from radar sensor to detected object (meters)

        , range_m REAL NOT NULL
          -- Azimuth/bearing angle from radar sensor to detected object (degrees)
          -- Typically measured clockwise from sensor's forward axis

        , azimuth_deg REAL NOT NULL
          -- Radial velocity component: positive = moving away from sensor
          -- negative = moving toward sensor (m/s). Doppler-derived measurement.

        , radial_speed_mps REAL
          -- Signal-to-noise ratio of this detection (dB). Higher values indicate
          -- stronger, more reliable detections. Used for quality filtering.

        , snr REAL /* derived (projected to road plane in world frame) */
          -- X coordinate in world frame after coordinate transformation (meters)

        , x REAL
          -- Y coordinate in world frame after coordinate transformation (meters)

        , y REAL
          -- Quality score/confidence for this detection (0-100 or similar scale).
          -- May incorporate SNR, range, multi-frame consistency, etc.

        , quality INTEGER
          -- Unix nanoseconds when the radar process first received this detection.
          -- Used for measuring radar processing pipeline latency.

        , received_unix_nanos INTEGER NOT NULL /* when radar process received it */
          -- Unix nanoseconds when the LiDAR process handled/processed this detection.
          -- Used for measuring cross-sensor fusion latency in radar->LiDAR data flow.

        , processed_unix_nanos INTEGER /* when lidar process handled it */
          -- Processing latency from radar reception to LiDAR processing (microseconds).
          -- Key performance metric for real-time fusion quality assessment.

        , processing_latency_us INTEGER /* receive to process time */
          -- Foreign key ensuring detection belongs to a valid radar sensor

        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          -- Foreign key ensuring pose used for transformation is valid

        , FOREIGN KEY (pose_id) REFERENCES sensor_poses (pose_id)
          )
;

CREATE INDEX idx_radar_time ON radar_observations (world_frame, ts_unix_nanos)
;

CREATE INDEX idx_radar_sensor_time ON radar_observations (sensor_id, ts_unix_nanos)
;

/*
 * Radar lines table: Raw radar scan line data (optional storage)
 *
 * This table optionally stores raw radar scan line data for detailed analysis
 * and debugging. Each row represents one angular measurement from a radar sweep.
 * This low-level data can be used for custom processing algorithms, interference
 * analysis, or detailed post-processing but generates high data volumes.
 * Most production systems will skip this table and work directly with radar_observations.
 */
   CREATE TABLE radar_lines (
          -- Auto-incrementing unique identifier for this radar scan line
          radar_line_id INTEGER PRIMARY KEY
          -- Reference to the radar sensor that captured this scan line

        , sensor_id TEXT NOT NULL
          -- Unix nanoseconds timestamp when this scan line was captured

        , ts_unix_nanos INTEGER NOT NULL
          -- Angular position of this scan line within the radar sweep (degrees)

        , angle_deg REAL
          -- Range/distance measurement for this scan line (meters)

        , range_m REAL
          -- Raw intensity/amplitude measurement for this scan line.
          -- Sensor-specific units, used for SNR calculation and detection processing.

        , intensity REAL
          -- Foreign key ensuring scan line belongs to a valid radar sensor

        , FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
          )
;

CREATE INDEX idx_radar_lines_sensor_time ON radar_lines (sensor_id, ts_unix_nanos)
;

-- ---------------------------------------------------------------------------
-- Associations & fusion ledger (WORLD frame)
-- Unified table for all sensor association events (radar-lidar, future sensors)
-- ---------------------------------------------------------------------------
/*
 * Sensor associations table: Multi-sensor data fusion and association ledger
 *
 * This table records all attempts to associate/fuse measurements from different sensors
 * (primarily radar-LiDAR fusion). Each row represents one association event where
 * measurements from different sensors are determined to come from the same real-world object.
 * The table stores both the input measurements and the resulting fused state estimates.
 * Supports extensibility for future additional sensor types through flexible foreign keys.
 */
   CREATE TABLE sensor_associations (
          -- Auto-incrementing unique identifier for this association event
          assoc_id INTEGER PRIMARY KEY
          -- World coordinate frame where this association was performed

        , world_frame TEXT NOT NULL
          -- Unix nanoseconds timestamp when this association was computed

        , ts_unix_nanos INTEGER NOT NULL /* association time */
          -- Reference to LiDAR track involved in this association (may be NULL)

        , track_id TEXT
          -- Reference to radar observation involved in this association (may be NULL)

        , radar_obs_id INTEGER
          -- Reference to LiDAR cluster involved in this association (may be NULL)

        , lidar_cluster_id INTEGER
          -- Algorithm used for this association: 'mahalanobis' (statistical distance)
          -- 'nearest' (Euclidean nearest neighbor), 'kalman' (Kalman filter innovation)

        , association_method TEXT /* 'mahalanobis', 'nearest', 'kalman' */
          -- Numerical cost/distance metric from the association algorithm.
          -- For Mahalanobis: statistical distance. For nearest: Euclidean distance.
          -- Lower values indicate better associations.

        , cost REAL /* e.g., Mahalanobis distance */
          -- Qualitative assessment of association reliability: 'high', 'medium', 'low'.
          -- Based on cost thresholds, sensor quality, temporal consistency, etc.

        , association_quality TEXT CHECK (association_quality IN ('high', 'medium', 'low'))
          -- Fused X position estimate in world frame (meters) after sensor combination

        , fused_x REAL
          -- Fused Y position estimate in world frame (meters) after sensor combination

        , fused_y REAL
          -- Fused X velocity estimate in world frame (m/s) after sensor combination

        , fused_vx REAL
          -- Fused Y velocity estimate in world frame (m/s) after sensor combination

        , fused_vy REAL
          -- Fused speed magnitude estimate (m/s) computed from velocity components

        , fused_speed_mps REAL
          -- Optional: fused covariance matrix stored as binary blob of 16 floats (4x4 matrix).
          -- Represents uncertainty in the fused state estimate. Row-major order.

        , fused_cov_blob BLOB /* 16 floats row-major (optional) */
          -- Bitmask indicating which sensors contributed to this association:
          -- bit0=LiDAR, bit1=radar, bit2=future sensors. Default 3 = LiDAR+radar.

        , source_mask INTEGER DEFAULT 3 /* bit0=lidar, bit1=radar, bit2=future */
          -- Foreign key ensuring association references a valid LiDAR track

        , FOREIGN KEY (track_id) REFERENCES lidar_tracks (track_id)
          -- Foreign key ensuring association references a valid radar observation

        , FOREIGN KEY (radar_obs_id) REFERENCES radar_observations (radar_obs_id)
          -- Foreign key ensuring association references a valid LiDAR cluster

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
/*
 * System events table: Unified logging for performance monitoring and system events
 *
 * This table provides a unified event log for all system monitoring, performance metrics
 * and significant system events. Different event types store their specific data in the
 * flexible JSON event_data field. This design enables efficient querying while supporting
 * diverse event schemas. Critical for monitoring 100-track system performance and debugging
 * fusion pipeline issues. Replaces multiple separate logging tables with a unified approach.
 */
   CREATE TABLE system_events (
          -- Auto-incrementing unique identifier for this system event
          event_id INTEGER PRIMARY KEY
          -- Reference to specific sensor (NULL for system-wide events that don't relate to a single sensor)

        , sensor_id TEXT /* NULL for system-wide events */
          -- Unix nanoseconds timestamp when this event occurred or was recorded

        , ts_unix_nanos INTEGER NOT NULL
          -- Type/category of event being recorded. Constrained to specific valid types:
          -- 'performance': CPU usage, memory consumption, processing latencies, throughput metrics
          -- 'track_birth': New track initialization events with initial state
          -- 'track_death': Track termination events with final statistics
          -- 'track_merge': Multiple tracks combined into single track (association correction)
          -- 'track_split': Single track divided into multiple tracks (tracking error correction)
          -- 'system_start': System initialization and startup events
          -- 'system_stop': System shutdown and cleanup events
          -- 'background_snapshot': LiDAR background model snapshot completion events

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
          -- Flexible JSON storage for event-specific data. Schema varies by event_type:
          -- 'performance': {"metric_name": "cpu_usage_pct", "metric_value": 23.5, "sensor_id": "lidar01"}
          -- 'track_birth': {"track_id": "track_001", "initial_position": {"x": 10.5, "y": 5.2}}
          -- 'track_death': {"track_id": "track_001", "final_stats": {"duration_s": 15.3, "distance_m": 45.2}}
          -- 'background_snapshot': {"snapshot_id": 123, "changed_cells": 1502, "reason": "periodic_update"}

        , event_data JSON /* flexible storage for different event types */
          -- Foreign key ensuring event belongs to a valid sensor (when sensor_id is not NULL)

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
/*
 * Latest observation view: Most recent state of all tracks for UI display
 *
 * This view provides the most recent observation (position, velocity, etc.) for each track.
 * Particularly useful for real-time UI displays showing current track positions and states.
 * Combines track metadata (classification, summary stats) with the latest temporal observation.
 * Automatically updates as new track observations are added to the system.
 */
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

/*
 * Radar-confirmed tracks view: Tracks with multi-sensor validation
 *
 * This view identifies tracks that have been confirmed/validated by radar observations
 * through the sensor fusion pipeline. These tracks have higher confidence since they
 * are detected by multiple independent sensors. Useful for filtering high-quality
 * tracks for analysis and reducing false positive detections from single-sensor artifacts.
 */
   CREATE VIEW v_tracks_with_radar AS
   SELECT DISTINCT      t.track_id
        , t.world_frame
     FROM lidar_tracks t
     JOIN sensor_associations a ON a.track_id = t.track_id
      AND a.radar_obs_id IS NOT NULL
;

/*
 * System performance summary view: Real-time monitoring dashboard for 100-track capacity
 *
 * This view aggregates recent performance metrics from the system_events table to provide
 * a real-time system health dashboard. Includes performance metrics (CPU, memory, latency)
 * averaged over the last 5 minutes, current active track count, and recent background
 * snapshot activity. Critical for monitoring system performance under the target load
 * of 100 concurrent tracks and identifying performance bottlenecks or degradation.
 */
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

/*
 * Track activity summary view: Site-level traffic analysis and statistics
 *
 * This view provides aggregated traffic statistics per site/world frame for the last 24 hours.
 * Includes total track count, currently active tracks, average peak speeds, and average
 * track durations. Useful for traffic flow analysis, site comparison, and identifying
 * busy vs. quiet periods. Helps validate system performance and understand traffic patterns
 * across different deployment sites.
 */
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

/*
 * Track lifecycle events summary view: Daily tracking system health metrics
 *
 * This view summarizes track lifecycle events (birth, death, merge, split) by date
 * for the last 24 hours. Provides insight into tracking system stability and performance.
 * High merge/split counts may indicate tracking algorithm issues or challenging conditions.
 * Birth/death balance indicates overall traffic flow. Used for system health monitoring
 * and identifying periods requiring algorithm tuning or manual review.
 */
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
