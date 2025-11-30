# LiDAR Sidecar — Technical Implementation Overview

**Status:** Phase 3.5 completed (REST API Endpoints), UI visualization planned  
**Scope:** Hesai UDP → parse → frame assembly → background subtraction → foreground mask → clustering → tracking → classification → HTTP API  
**Current Phase:** UI Visualization (next)

---

## Implementation Status

### ✅ **Phase 1: Core Infrastructure (COMPLETED)**

- UDP packet ingestion with configurable parameters (4MB buffer, 2369 port)
- Hesai Pandar40P packet parsing (22-byte tail structure validated)
- Time-based frame assembly with motor speed adaptation (360° detection, 1s buffer)
- SQLite database persistence with comprehensive schema (738 lines)
- HTTP monitoring interface with real-time statistics
- Comprehensive test suite with real packet validation

### ✅ **Phase 2: Background & Clustering (COMPLETED)**

- ✅ Background grid infrastructure with EMA learning (implemented)
- ✅ Foreground/background classification with neighbor voting (implemented)
- ✅ Background model persistence to database (implemented)
- ✅ Enhanced HTTP endpoints for tuning and monitoring (implemented)
- ✅ Acceptance metrics for parameter tuning (implemented)
- ✅ PCAP file reading for parameter identification (implemented)
- ✅ Grid heatmap visualization API for spatial analysis (implemented)
- ✅ Comprehensive debug logging for diagnostics (implemented)

### ✅ **Phase 2.5: PCAP-Based Parameter Tuning (COMPLETED)**

- ✅ **Runtime Source Switching**: Live UDP ↔ PCAP toggled via API (no restart)
- ✅ **API-Controlled Replay**: POST to `/api/lidar/pcap/start?sensor_id=<id>` with file path
- ✅ **Safe Directory Restriction**: `--lidar-pcap-dir` limits file access to prevent path traversal attacks
- ✅ **BPF Filtering**: Filters PCAP by UDP port (supports multi-sensor captures)
- ✅ **Background Persistence**: Periodic flush every N seconds during replay
- ✅ **Sweep Tool Integration**: bg-sweep and bg-multisweep use PCAP API
- ✅ **No Server Restart**: Change PCAP files via API without restarting radar binary
- ✅ **Frame Builder Fix**: Fixed eviction bug that prevented frame callback delivery
- ✅ **Grid Visualization**: Spatial heatmap API for analyzing filled vs settled cells

### ✅ **Phase 2.9: Foreground Mask Generation (COMPLETED)**

- ✅ **`ProcessFramePolarWithMask()`**: Per-point foreground/background classification in polar coordinates
- ✅ **`ExtractForegroundPoints()`**: Helper to filter foreground points from mask
- ✅ **`ComputeFrameMetrics()`**: Frame-level statistics (total, foreground, background counts)
- ✅ **Unit Tests**: Comprehensive test coverage in `internal/lidar/foreground_test.go`
- ✅ **Location**: `internal/lidar/foreground.go`

### ✅ **Phase 3.0: Polar → World Transform (COMPLETED)**

- ✅ **`WorldPoint`** struct for world-frame Cartesian coordinates
- ✅ **`TransformToWorld()`**: Converts polar points to world frame using pose transform
- ✅ **`TransformPointsToWorld()`**: Convenience function for pre-computed Cartesian points
- ✅ **Identity transform fallback** when pose is nil
- ✅ **Unit Tests**: Transform accuracy validation in `internal/lidar/clustering_test.go`
- ✅ **Location**: `internal/lidar/clustering.go`

### ✅ **Phase 3.1: DBSCAN Clustering (COMPLETED)**

- ✅ **`SpatialIndex`**: Grid-based spatial indexing using Szudzik pairing with zigzag encoding
- ✅ **`DBSCAN()`**: Density-based clustering with configurable eps and minPts
- ✅ **`computeClusterMetrics()`**: Centroid, bounding box, height P95, intensity mean
- ✅ **`WorldCluster`** struct with all required features
- ✅ **Unit Tests**: Clustering validation in `internal/lidar/clustering_test.go`
- ✅ **Location**: `internal/lidar/clustering.go`

### ✅ **Phase 3.2: Kalman Tracking (COMPLETED)**

- ✅ **`TrackState`** lifecycle: Tentative → Confirmed → Deleted
- ✅ **`TrackedObject`**: Track state with Kalman filter and aggregated features
- ✅ **`Tracker`**: Multi-object tracker with configurable parameters
- ✅ **Mahalanobis distance gating** for cluster-to-track association
- ✅ **Kalman predict/update** with constant velocity model
- ✅ **Track lifecycle management**: hits/misses counting, promotion, deletion
- ✅ **Speed statistics**: Average, peak, and history for percentile computation
- ✅ **Unit Tests**: Comprehensive tracking tests in `internal/lidar/tracking_test.go`
- ✅ **Location**: `internal/lidar/tracking.go`

### ✅ **ML Training Data Support (COMPLETED)**

- ✅ **`ForegroundFrame`**: Export struct for foreground points with metadata
- ✅ **`EncodeForegroundBlob()`/`DecodeForegroundBlob()`**: Compact binary encoding (8 bytes/point)
- ✅ **`ValidatePose()`**: Pose quality assessment based on RMSE thresholds
- ✅ **`TransformToWorldWithValidation()`**: Transform with quality gating
- ✅ **`TrainingDataFilter`**: Filtering by pose quality for ML datasets
- ✅ **Unit Tests**: `internal/lidar/training_data_test.go`, `internal/lidar/pose_test.go`
- ✅ **Location**: `internal/lidar/training_data.go`, `internal/lidar/pose.go`

### ✅ **Phase 3.3: SQL Schema & Database Persistence (COMPLETED)**

- ✅ **Migration File**: `internal/db/migrations/000009_create_lidar_tracks.up.sql`
- ✅ **`lidar_clusters` table**: DBSCAN cluster persistence with world-frame features
- ✅ **`lidar_tracks` table**: Track lifecycle, kinematics, classification fields
- ✅ **`lidar_track_obs` table**: Per-observation tracking data with foreign key to tracks
- ✅ **Persistence Functions**: `InsertCluster()`, `InsertTrack()`, `UpdateTrack()`, `InsertTrackObservation()`
- ✅ **Query Functions**: `GetActiveTracks()`, `GetTrackObservations()`, `GetRecentClusters()`
- ✅ **Unit Tests**: `internal/lidar/track_store_test.go`
- ✅ **Schema Updated**: `internal/db/schema.sql` includes all track tables
- ✅ **Location**: `internal/lidar/track_store.go`

### ✅ **Phase 3.4: Track Classification (COMPLETED)**

- ✅ **`TrackClassifier`**: Rule-based classification engine
- ✅ **Object Classes**: `pedestrian`, `car`, `bird`, `other`
- ✅ **Classification Features**: height, length, width, speed, duration, observation count
- ✅ **Confidence Scoring**: Per-class confidence based on feature match quality
- ✅ **Speed Percentiles**: `ComputeSpeedPercentiles()` for P50/P85/P95
- ✅ **Classification Integration**: `ClassifyAndUpdate()` for track field updates
- ✅ **Unit Tests**: `internal/lidar/classification_test.go`
- ✅ **Location**: `internal/lidar/classification.go`

### ✅ **Phase 3.5: REST API Endpoints (COMPLETED)**

- ✅ **TrackAPI**: HTTP handler struct for track/cluster queries
- ✅ **GET `/api/lidar/tracks`**: List tracks with optional state filter
- ✅ **GET `/api/lidar/tracks/active`**: Active tracks (real-time from memory or DB)
- ✅ **GET `/api/lidar/tracks/{track_id}`**: Get specific track details
- ✅ **PUT `/api/lidar/tracks/{track_id}`**: Update track metadata (class, confidence)
- ✅ **GET `/api/lidar/tracks/{track_id}/observations`**: Get track trajectory
- ✅ **GET `/api/lidar/tracks/summary`**: Aggregated statistics by class/state
- ✅ **GET `/api/lidar/clusters`**: Recent clusters by time range
- ✅ **Unit Tests**: `internal/lidar/monitor/track_api_test.go`
- ✅ **Location**: `internal/lidar/monitor/track_api.go`

### 📋 **Phase 4: Multi-Sensor & Production Optimization (PLANNED)**

- **Multi-Sensor Architecture**: Support multiple LiDAR sensors per machine
- **Local Persistence**: Each sensor stores data in local SQLite database
- **Database Unification**: Merge data from multiple local databases for analysis
- **World Frame Tracking**: Unified tracking across multiple intersections
- **Cross-Sensor Association**: Track objects as they move between sensor coverage areas
- **Distributed Storage**: Copy/consolidate data from edge nodes for whole-street analysis
- **Performance Profiling**: Optimize for multi-sensor concurrent processing
- **Memory Optimization**: Efficient handling of 100+ tracks across multiple sensors
- **Production Deployment**: Documentation for multi-node edge deployment
- **UI Visualization**: Track display components in web frontend

---

## Module Structure

```
cmd/radar/radar.go                 ✅ # LiDAR integration with --enable-lidar flag
cmd/bg-sweep/main.go               ✅ # Single-parameter sweep tool for tuning
cmd/bg-multisweep/main.go          ✅ # Multi-parameter grid search tool
internal/lidar/network/listener.go ✅ # UDP socket and packet processing
internal/lidar/network/forwarder.go✅ # UDP packet forwarding to LidarView
internal/lidar/network/pcap.go     ✅ # PCAP file reading with BPF filtering
internal/lidar/parse/extract.go    ✅ # Pandar40P packet -> []Point (22-byte tail)
internal/lidar/parse/config.go     ✅ # Embedded calibration configurations
internal/lidar/frame_builder.go    ✅ # Time-based frame assembly with motor speed
internal/lidar/monitor/            ✅ # HTTP endpoints: /health, /api/lidar/*
internal/lidar/monitor/track_api.go✅ # Track/cluster REST API handlers (Phase 3.5)
internal/lidar/background.go       ✅ # Background model & classification with persistence
internal/lidar/foreground.go       ✅ # Foreground mask generation and extraction (Phase 2.9)
internal/lidar/clustering.go       ✅ # World transform and DBSCAN clustering (Phase 3.0-3.1)
internal/lidar/tracking.go         ✅ # Kalman tracking with lifecycle management (Phase 3.2)
internal/lidar/track_store.go      ✅ # Database persistence for tracks/clusters (Phase 3.3)
internal/lidar/classification.go   ✅ # Rule-based track classification (Phase 3.4)
internal/lidar/training_data.go    ✅ # ML training data export and encoding
internal/lidar/pose.go             ✅ # Pose validation and quality assessment
internal/lidar/export.go           ✅ # ASC point cloud export
internal/lidar/arena.go            ✅ # Data structures for clustering and tracking
internal/db/db.go                  ✅ # Database schema and BgSnapshot persistence
internal/db/migrations/000009_*    ✅ # SQL migrations for lidar_clusters, lidar_tracks, lidar_track_obs
tools/grid-heatmap/                ✅ # Grid visualization and analysis tools
```

**Data Flow:**

```
[UDP:2369] → [Parse] → [Frame Builder] → [Background (sensor)] → [Foreground Mask]
                                                                        ↓
                                                               ProcessFramePolarWithMask()
                                                                        ↓
                                                           ExtractForegroundPoints()
                                                                        ↓
                                                             TransformToWorld()
                                                                        ↓
                                                                  DBSCAN()
                                                                        ↓
                                                              Tracker.Update()
                                                                        ↓
[HTTP API] ← [Database Persistence] ← [Confirmed Tracks] ← [Track Lifecycle]
```

---

## Core Algorithm Implementation

### UDP Ingestion & Parsing (✅ Complete)

- **UDP Listener**: Configurable port (default 2369), 4MB receive buffer
- **Packet Validation**: 1262-byte (standard) or 1266-byte (with sequence) packets
- **Tail Parsing**: Complete 30-byte structure per official Hesai documentation
- **Point Generation**: 40 channels × 10 blocks = up to 400 points per packet
- **Calibration**: Embedded per-channel angle and firetime corrections
- **Coordinate Transform**: Spherical → Cartesian with calibration applied

### Frame Assembly (✅ Complete)

- **Hybrid Frame Detection**: Motor speed-adaptive timing + azimuth validation (prevents timing anomalies)
- **Time-based Primary**: Frame completion when duration exceeds expected time (RPM-based) + 10% tolerance
- **Azimuth Secondary**: Azimuth wrap detection (340° → 20°) respects timing constraints
- **Enhanced Wrap Detection**: Catches large negative azimuth jumps (>180°) like 289° → 61° transitions
- **Traditional Fallback**: Pure azimuth-based detection (350° → 10°) when motor speed unavailable
- **Late Packet Handling**: 1-second buffer for out-of-order packets before final callback
- **Frame Callback**: Configurable callback for frame completion
- **Buffer Management**: Fixed eviction bug - frames now properly finalized when evicted from buffer
- **Frame Buffer**: Holds up to 100 frames with 500ms timeout before cleanup
- **Cleanup Interval**: 250ms periodic sweep for frame finalization (configurable for PCAP mode)

### Database Persistence (✅ Complete)

- **SQLite with WAL**: High-performance concurrent access
- **Performance Optimized**: Prepared statements, batch inserts

### Background Model & Classification (✅ Complete)

**Current State:**

- The system implements background model learning and foreground/background classification for each observation.
- **Foreground mask extraction is now implemented** via `ProcessFramePolarWithMask()`.

**Algorithm (Implemented):**

```
closeness_threshold = closeness_multiplier * (range_spread + noise_relative * observation_mean + 0.01)
                    + safety_margin
cell_diff = abs(cell_average_range - observation_mean)
is_background = (cell_diff <= closeness_threshold) OR (neighbor_confirm >= required_neighbors)
```

**Implementation Details:**

- **Classification**: Each observation is classified as background or foreground
- **Foreground Mask**: `ProcessFramePolarWithMask()` returns per-point boolean mask
- **Foreground Extraction**: `ExtractForegroundPoints()` filters points using mask
- **Spatial filtering**: Same-ring neighbor vote (configurable via NeighborConfirmationCount)
- **Temporal filtering**: Cell freezing after large divergence (configurable via FreezeDurationNanos)
- **Learning**: EMA update of cell statistics when observation is background-like (BackgroundUpdateFraction)
- **Grid**: 40 rings × 1800 azimuth bins (0.2° resolution)
- **Persistence**: Automatic background snapshots to database with versioning
- **Noise Scaling**: Distance-adaptive noise threshold via NoiseRelativeFraction
- **Acceptance Metrics**: Range-bucketed tracking of foreground/background classification rates
- **Counters**: Real-time ForegroundCount and BackgroundCount telemetry

**What's Implemented:**

- ✅ Background model learning and updating
- ✅ Foreground/background classification per observation
- ✅ Neighbor confirmation voting
- ✅ Cell freezing on large divergence
- ✅ Acceptance metrics for parameter tuning
- ✅ **Foreground mask extraction** (`ProcessFramePolarWithMask()`)
- ✅ **Foreground point filtering** (`ExtractForegroundPoints()`)

### Polar → World Transform (✅ Complete)

- **Location**: `internal/lidar/clustering.go`
- **`TransformToWorld()`**: Converts polar points to world-frame Cartesian coordinates
- **Pose Support**: Uses 4x4 homogeneous transform matrix (sensor → world)
- **Identity Fallback**: Uses identity transform when pose is nil
- **`TransformPointsToWorld()`**: Convenience function for pre-computed Cartesian points

### Clustering (✅ Complete)

- **Location**: `internal/lidar/clustering.go`
- **Algorithm**: DBSCAN with required spatial index
- **Euclidean clustering**: eps = 0.6m (configurable), minPts = 12 (configurable)
- **`SpatialIndex`**: Grid-based indexing using Szudzik pairing with zigzag encoding for O(1) neighbor queries
- **Per-cluster metrics**: centroid, bounding box (length/width/height), height_p95, intensity_mean
- **`WorldCluster`** struct with all required features
- **2D Clustering**: Uses (x, y) for clustering, z for height features only

### Tracking (✅ Complete)

- **Location**: `internal/lidar/tracking.go`
- **State vector**: [x, y, velocity_x, velocity_y]
- **Constant-velocity Kalman filter** with configurable noise parameters
- **Association**: Mahalanobis distance gating for cluster-to-track association
- **`Tracker`**: Multi-object tracker with configurable parameters via `TrackerConfig`
- **`TrackedObject`**: Track state with Kalman filter, lifecycle counters, and aggregated features
- **Lifecycle States**: `Tentative` → `Confirmed` → `Deleted`
- **Track Management**: 
  - Birth from unmatched clusters
  - Promotion after N consecutive hits (default: 3)
  - Deletion after N consecutive misses (default: 3)
  - Grace period for deleted tracks before cleanup
- **Speed Statistics**: Average speed, peak speed, history for percentile computation
- **Aggregated Features**: Bounding box averages, height P95 max, intensity mean average

### ML Training Data (✅ Complete)

- **Location**: `internal/lidar/training_data.go`, `internal/lidar/pose.go`
- **`ForegroundFrame`**: Export struct for foreground points with metadata
- **Compact Encoding**: 8 bytes per point (vs ~40+ bytes for struct)
- **Pose Validation**: Quality assessment based on RMSE thresholds
  - Excellent: < 0.05m
  - Good: 0.05-0.15m (OK for training)
  - Fair: 0.15-0.30m (OK for tracking, exclude from training)
  - Poor: > 0.30m (requires recalibration)
- **`TransformToWorldWithValidation()`**: Transform with pose quality gating
- **`TrainingDataFilter`**: Filtering by pose quality for ML datasets
- **Storage Recommendation**: Store in polar (sensor) frame for pose independence

---

## Configuration

### ✅ Current Flags (Implemented)

The LiDAR functionality is integrated into the `cmd/radar/radar.go` binary and enabled via the `--enable-lidar` flag:

```bash
# Radar binary with LiDAR integration
./radar [radar flags...] --enable-lidar [lidar flags...]

# LiDAR integration flags
--enable-lidar                        # Enable lidar components inside radar binary
--lidar-listen ":8081"                # HTTP listen address for lidar monitor
--lidar-udp-port 2369                 # UDP port to listen for lidar packets
--lidar-no-parse                      # Disable lidar packet parsing
--lidar-sensor "hesai-pandar40p"      # Sensor name identifier for lidar
--lidar-forward                       # Forward lidar UDP packets to another port
--lidar-forward-port 2368             # Port to forward lidar UDP packets to
--lidar-forward-addr "localhost"      # Address to forward lidar UDP packets to

# Background subtraction tuning (runtime-adjustable via HTTP API)
--lidar-bg-flush-interval 10s         # Interval to flush background grid to DB (PCAP mode)
--lidar-bg-noise-relative 0.315       # NoiseRelativeFraction: fraction of range treated as measurement noise
```

### PCAP Replay Workflow

PCAP replay now happens via runtime data source switching:

```bash
# Build with PCAP support (requires libpcap)
make radar-local              # macOS with PCAP support
make radar-linux              # Linux without PCAP (for Raspberry Pi cross-compile)
make radar-linux-pcap         # Linux with PCAP (requires ARM64 libpcap installed)

# Start radar with LiDAR enabled (no PCAP-only mode required)
./radar --enable-lidar --lidar-bg-flush-interval=5s --lidar-pcap-dir ../sensor-data/lidar

# Switch to PCAP replay
curl -X POST "http://localhost:8081/api/lidar/pcap/start?sensor_id=hesai-pandar40p" \
  -H "Content-Type: application/json" \
  -d '{"pcap_file": "cars.pcap"}'

# ...monitor status during replay
curl http://localhost:8081/api/lidar/status | jq .

# Switch back to live UDP data when finished
curl "http://localhost:8081/api/lidar/pcap/stop?sensor_id=hesai-pandar40p"

# Sweep tools continue to point at the API
./bg-sweep -pcap-file=/path/to/cars.pcap -start=0.01 -end=0.3 -step=0.01
./bg-multisweep -pcap-file=/path/to/pedestrians.pcap -closeness=2.0,3.0,4.0 -neighbors=1,2,3
```

**Build Notes:**

- PCAP support requires the `pcap` build tag and libpcap C library
- Safe directory restriction: `--lidar-pcap-dir` (default: `../sensor-data/lidar`) prevents path traversal attacks
- Only files within the safe directory can be replayed via the API
- Systemd service automatically creates the safe directory on startup

---

## Grid Analysis & Visualization

### Grid Heatmap API

The grid heatmap API aggregates the fine-grained background grid (40 rings × 1800 azimuth bins = 72,000 cells) into coarse spatial buckets for visualization and analysis.

**Endpoint**: `GET /api/lidar/grid_heatmap`

**Query Parameters**:

- `sensor_id` (required): Sensor identifier
- `azimuth_bucket_deg` (optional, default=3.0): Degrees per azimuth bucket
- `settled_threshold` (optional, default=5): Minimum TimesSeenCount for "settled" classification

**Response Structure**:

```json
{
  "sensor_id": "hesai-pandar40p",
  "timestamp": "2025-11-01T12:00:00Z",
  "grid_params": {
    "total_rings": 40,
    "total_azimuth_bins": 1800,
    "total_cells": 72000
  },
  "heatmap_params": {
    "azimuth_bucket_deg": 3.0,
    "azimuth_buckets": 120,
    "ring_buckets": 40,
    "settled_threshold": 5
  },
  "summary": {
    "total_filled": 58234,
    "total_settled": 52100,
    "fill_rate": 0.809,
    "settle_rate": 0.724
  },
  "buckets": [
    {
      "ring": 0,
      "azimuth_deg_start": 0.0,
      "azimuth_deg_end": 3.0,
      "total_cells": 15,
      "filled_cells": 14,
      "settled_cells": 12,
      "mean_times_seen": 8.5,
      "mean_range_meters": 25.3
    }
    // ... 4800 buckets total
  ]
}
```

### Visualization Tools

**Polar Heatmap**: Ring vs Azimuth visualization showing fill/settle rates

```bash
python3 tools/grid-heatmap/plot_grid_heatmap.py \
  --url http://localhost:8081 \
  --sensor hesai-pandar40p \
  --metric unsettled_ratio
```

**Cartesian Heatmap**: X-Y spatial visualization showing physical location patterns

```bash
python3 tools/grid-heatmap/plot_grid_heatmap.py \
  --url http://localhost:8081 \
  --sensor hesai-pandar40p \
  --cartesian \
  --metric fill_rate
```

**Full Dashboard**: Comprehensive 4K-optimized visualization with multiple views

```bash
# Single snapshot
python3 tools/grid-heatmap/plot_grid_heatmap.py \
  --url http://localhost:8081 \
  --output grid_dashboard.png

# PCAP replay with periodic snapshots
python3 tools/grid-heatmap/plot_grid_heatmap.py \
  --url http://localhost:8081 \
  --pcap /path/to/file.pcap \
  --interval 30 \
  --output-dir output/snapshots

# Live snapshot mode for continuous monitoring
python3 tools/grid-heatmap/plot_grid_heatmap.py \
  --url http://localhost:8081 \
  --live \
  --interval 10
```

**Dashboard Layout** (25.6×14.4 inches @ 150 DPI):

- Top 50%: Polar settle rate + Polar metric + Spatial XY distance
- Bottom 50%: 4 stacked metric panels (fill rate, settle rate, unsettled ratio, mean times seen)
- Layout optimizations: hspace=0.15, title repositioned to top right, settle rate chart on left

**Available Metrics**:

- `fill_rate`: Fraction of cells with TimesSeenCount > 0
- `settle_rate`: Fraction of cells meeting settled threshold
- `unsettled_ratio`: Filled but not settled cells (fill_rate - settle_rate)
- `mean_times_seen`: Average observation count per filled cell
- `frozen_cells`: Cells currently frozen due to dynamic obstacle detection

### Noise Analysis Scripts

**Noise Sweep Plotting**: Visualize acceptance rates vs noise parameters

```bash
python3 plot_noise_sweep.py sweep-results.csv
python3 plot_noise_buckets.py sweep-results.csv
```

**Convergence Analysis**: Analyze neighbor/closeness parameter impact

```bash
# Data analysis scripts for parameter tuning
tools/data-analysis/*.py
```

### Use Cases

1. **Spatial Pattern Analysis**: Identify regions not filling or settling properly
2. **Parameter Tuning**: Visualize impact of noise/closeness/neighbor parameters
3. **Diagnostic Visualization**: Create heatmaps for filled vs settled cells
4. **Anomaly Detection**: Find unexpected patterns in grid population
5. **Temporal Analysis**: Track grid settlement progress during warmup
6. **PCAP Snapshot Mode**: Periodic captures with configurable interval/duration, auto-numbered output directories
7. **Metadata Tracking**: JSON metadata for snapshot sessions

---

## Debugging & Diagnostics

### Critical Bug Fixes

**FrameBuilder Eviction Bug** (Fixed):

- **Issue**: `evictOldestBufferedFrame()` deleted frames without calling `finalizeFrame()`
- **Impact**: Frames accumulated but callback never fired, preventing background population
- **Fix**: Added `fb.finalizeFrame(oldestFrame)` to eviction path
- **Location**: `internal/lidar/frame_builder.go:~436`

### Debug Logging Strategy

The system includes comprehensive debug logging for diagnosing issues with grid reset, acceptance rates, and frame delivery.

**Enable Debug Mode**:

```bash
# Via CLI flag
./radar --enable-lidar --debug

# Via API (runtime)
curl -X POST 'http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p' \
  -H 'Content-Type: application/json' \
  -d '{"enable_diagnostics": true}'
```

**Key Log Patterns**:

1. **Grid Reset Timing**:

   - `[ResetGrid]`: Shows before/after nonzero cell counts
   - `[API:grid_reset]`: API call timing and duration
   - `[API:params]`: Parameter change timing

2. **Grid Population**:

   - `[ProcessFramePolar]`: Frame-by-frame grid growth
   - Rate-limited logging every 100 frames or at significant thresholds

3. **Acceptance Decisions**:

   - `[ProcessFramePolar:decision]`: Per-cell acceptance/rejection details
   - `[ProcessFramePolar:summary]`: Frame-level acceptance rates
   - Includes: cell state, closeness threshold, neighbor confirmation

4. **Frame Delivery**:
   - `[FrameBuilder:finalize]`: Frame completion events
   - `[FrameBuilder:evict]`: Buffer eviction events
   - `[FrameBuilder:callback]`: Callback invocation
   - `[BackgroundManager]`: Frame processing and snapshot persistence

**Common Diagnostic Scenarios**:

**Grid Reset Race Condition**:

- Symptom: `nonzero_cells` stays high after reset
- Diagnosis: Check timing between reset API call and first ProcessFramePolar log
- Expected: Grid grows from 0 to 60k+ within 1-2 seconds during live operation
- Root cause: Between `POST /api/lidar/grid_reset` and grid sampling, incoming frames continuously repopulate cells
- Solution: For testing, use shorter settle times and understand that grid repopulation is normal during live operation

**Low Acceptance Rates**:

- Symptom: Seeing <99% acceptance when expected higher (e.g., 98% instead of 99%+)
- Diagnosis: Enable diagnostics and examine decision logs
- Common causes:
  - Cold start rejection: Empty cells (TimesSeenCount=0) reject observations until seeded
  - Empty cells rejecting before seeding (check `SeedFromFirstObservation` via `--lidar-seed-from-first` flag)
  - Tight thresholds at long range (check `NoiseRelativeFraction`)
  - Strict neighbor confirmation (check `NeighborConfirmationCount` - neighbor=2 requires 2 of 2 neighbors)
  - NoiseRelativeFraction too strict (0.01 = 1% may be too tight for real sensor noise at long ranges)
- Analysis: After settling period, rates typically converge to 99.8%+

**Frames Not Finalizing**:

- Symptom: Points added but no frame completion logs
- Diagnosis: Check frame buffer size and cleanup timing
- Common causes:
  - `minFramePoints` threshold too high for sparse data (default: 1000 points)
  - Buffer timeout too long for fast PCAP replay (bufferTimeout: 500ms)
  - Eviction bug (now fixed - frames were deleted without callback)
- Buffer behavior: Frames wait for `bufferTimeout` before cleanup timer finalizes them
- Fast PCAP replay: At 5k+ pkt/s, buffer may fill before cleanup timer fires (consider reducing CleanupInterval to 50ms)

**PCAP Replay Issues**:

- Symptom: PCAP reads but background stays empty (nonzero_cells=0)
- Root cause findings:
  - Azimuth wrap detection initially only checked 350°→10°, missing wraps like 289°→61°
  - Enhanced detection now catches large negative jumps (>180°)
  - FrameBuilder eviction bug: frames deleted from buffer without invoking callback
  - Fix applied: `evictOldestBufferedFrame()` now calls `finalizeFrame(oldestFrame)`
- Verification: Check logs for frame completion, callback invocation, and background snapshot persistence

### Performance Tuning

**PCAP Replay Optimization**:

```bash
# Lower minFramePoints for sparse data
--lidar-min-frame-points 100

# Faster cleanup for rapid replay
# (modify CleanupInterval in code to 50ms for PCAP mode)

# Frequent background snapshots
--lidar-bg-flush-interval 5s

# Enable seeding from first observation (PCAP mode)
--lidar-seed-from-first

# Settle time for grid stabilization after parameter changes
--lidar-settle-time 5s
```

**Runtime Parameter Adjustment**:

```bash
# Permissive settings for debugging (via API)
curl -X POST 'http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p' \
  -H 'Content-Type: application/json' \
  -d '{
    "noise_relative_fraction": 1.0,
    "closeness_sensitivity_multiplier": 10.0,
    "neighbor_confirmation_count": 1,
    "enable_diagnostics": true
  }'
```

**Grid Analysis**:

```bash
# Quick status check
curl "http://localhost:8081/api/lidar/grid_status?sensor_id=hesai-pandar40p" | jq

# Detailed heatmap for analysis
curl "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p" | \
  jq '.summary'

# Find buckets with low settlement
curl -s "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p" | \
  jq '.buckets[] | select(.filled_cells > 0 and .settled_cells == 0) | {ring, azimuth_deg_start, filled_cells}'

# Extract summary with custom thresholds
curl "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p&azimuth_bucket_deg=6.0&settled_threshold=10" | \
  jq '.summary'
```

**Background Parameter Sweep**:

```bash
# Single parameter sweep (noise_relative)
./bg-sweep -pcap-file=/path/to/cars.pcap -start=0.01 -end=0.3 -step=0.01

# Multi-parameter grid search
./bg-multisweep -pcap-file=/path/to/pedestrians.pcap \
  -closeness=2.0,3.0,4.0 \
  -neighbors=1,2,3 \
  -noise-start=0.01 -noise-end=0.5 -noise-step=0.05

# Fetch live nonzero counts (avoids DB timing races)
# Sweep tools use grid_status API for real-time metrics
```

**Makefile Targets**:

```bash
# Development workflow
make dev-go          # Build and run with auto-restart
make dev-go-pcap     # PCAP mode development
make log-go-tail     # Tail application logs
make log-go-cat      # View full logs

# Visualization
make plot-grid-heatmap URL=http://localhost:8081 SENSOR=hesai-pandar40p
```

---

## Security

### PCAP File Access Restriction

PCAP file access is restricted to a designated safe directory to prevent path traversal attacks.

**Configuration**:

- CLI flag: `--lidar-pcap-dir <path>` (default: `../sensor-data/lidar`)
- Only files within this directory tree can be accessed
- Absolute paths are converted to be relative to safe directory

**Security Features**:

- Path sanitization with `filepath.Clean()`
- Absolute path requirement verification
- Safe directory prefix validation
- Regular file type enforcement (no directories/symlinks/devices)
- File extension whitelist (`.pcap`, `.pcapng`)

**Usage Examples**:

```bash
# Valid: filename only
{"pcap_file": "cars.pcap"}

# Valid: relative path within safe dir
{"pcap_file": "subfolder/pedestrians.pcap"}

# Rejected: path traversal attempt
{"pcap_file": "../../../etc/passwd"}
# Returns: 403 Forbidden
```

**Systemd Integration**:
The service file automatically creates the safe directory on startup:

```ini
ExecStartPre=/bin/mkdir -p /home/david/sensor-data/lidar
ExecStart=/home/david/code/velocity.report/radar --lidar-pcap-dir /home/david/sensor-data/lidar
```

- Raspberry Pi builds (`radar-linux`) omit PCAP support by default to avoid cross-compile complexity
- If PCAP API is called without PCAP support, returns error: "PCAP support not enabled: rebuild with -tags=pcap"

**Benefits:**

- No server restart needed to change PCAP files
- BPF filtering by UDP port (supports multi-sensor PCAP files)
- Periodic background grid persistence for parameter evolution tracking
- Sweep tools automatically trigger PCAP replay before parameter testing

````

### ✅ BackgroundParams (All Fields)

These parameters are configured at startup and can be adjusted at runtime via the HTTP API (`/api/lidar/params`):

```go
BackgroundUpdateFraction       float32  // EMA learning rate (default: 0.02)
ClosenessSensitivityMultiplier float32  // Motion threshold multiplier (default: 3.0)
SafetyMarginMeters             float32  // Safety buffer in meters (default: 0.5)
FreezeDurationNanos            int64    // Freeze after detection (default: 5s)
NeighborConfirmationCount      int      // Spatial filtering votes (default: 3)
NoiseRelativeFraction          float32  // Distance-adaptive noise (default: 0.315)
SettlingPeriodNanos            int64    // Time before first snapshot (default: 5 minutes)
SnapshotIntervalNanos          int64    // Time between snapshots (default: 2 hours)
ChangeThresholdForSnapshot     int      // Min changed cells to trigger snapshot (default: 100)
````

### 🔄 Planned Configuration (Clustering & Tracking)

```bash
# Clustering parameters (future)
-cluster-eps 0.6                     # Euclidean clustering distance threshold
-cluster-min-points 12               # Minimum points per cluster

# Tracking parameters (future)
-max_concurrent_tracks 100      # Memory management
-track_max_age_min 30          # Track retention
-pose_file "calibration.json"   # Sensor calibration
```

---

## HTTP Interface

### ✅ Current Endpoints

- `GET /health` - System status and packet statistics
- `GET /` - HTML dashboard with real-time metrics
- `GET /api/lidar/params?sensor_id=<id>` - Get current background parameters
- `POST /api/lidar/params?sensor_id=<id>` - Update background parameters (JSON body)
- `GET /api/lidar/acceptance?sensor_id=<id>` - Get acceptance metrics by range bucket
  - Optional: `?debug=true` for per-bucket details with active parameter context
- `POST /api/lidar/acceptance/reset?sensor_id=<id>` - Reset acceptance counters
- `POST /api/lidar/grid_reset?sensor_id=<id>` - Reset background grid (for testing/sweeps)
- `GET /api/lidar/grid_status?sensor_id=<id>` - Get grid statistics and settling status
- `GET /api/lidar/grid_heatmap?sensor_id=<id>` - Get spatial bucket aggregation (40 rings × 120 azimuth buckets)
- `GET /api/lidar/grid/export_asc?sensor_id=<id>` - Export background grid as ASC point cloud
- `POST /api/lidar/pcap/start?sensor_id=<id>` - Start PCAP replay (resets grid, stops UDP listener)
  - JSON body: `{"pcap_file": "filename.pcap"}` or `{"pcap_file": "subfolder/file.pcap"}`
- `GET /api/lidar/pcap/stop?sensor_id=<id>` - Stop replay and return to live UDP packets
- `GET /api/lidar/data_source` - Current data source, PCAP file, and replay status
- `POST /api/lidar/snapshot/persist?sensor_id=<id>` - Force immediate background snapshot to database
- `GET /api/lidar/snapshot?sensor_id=<id>` - Retrieve latest background snapshot from database

### ✅ Track API Endpoints (Phase 3.5 - Complete)

- `GET /api/lidar/tracks` - List tracks with optional state/sensor filter
- `GET /api/lidar/tracks/active` - Active tracks (real-time from memory or DB)
- `GET /api/lidar/tracks/{track_id}` - Get specific track details
- `PUT /api/lidar/tracks/{track_id}` - Update track metadata (class, confidence, model)
- `GET /api/lidar/tracks/{track_id}/observations` - Get track trajectory (observation history)
- `GET /api/lidar/tracks/summary` - Aggregated statistics by class and state
- `GET /api/lidar/clusters` - Recent clusters by sensor and time range

---

## Performance Metrics

### ✅ Current Performance

- **Packet Processing**: 36.5μs per packet
- **UDP Throughput**: Handles 10 Hz LiDAR (typical Pandar40P rate)
- **Memory Usage**: ~50MB baseline + 170KB per packet burst
- **Database**: High-performance SQLite with WAL mode
- **HTTP Response**: <5ms for health/status endpoints

### 🎯 Target Performance (Complete System)

- **End-to-end Latency**: <100ms (packet → track update)
- **CPU Usage**: 1-2 cores at 10-15 Hz LiDAR rate
- **Memory Usage**: <300MB with 100 concurrent tracks
- **Track Capacity**: 100 active tracks with 1000 observations each
- **Concurrent Tracks**: 100 active tracks maximum

---

## Testing Status

### ✅ Implemented Tests

```bash
# Packet parsing validation
go test ./internal/lidar/parse -v
=== RUN   TestSamplePacketTailParsing     ✅ Real packet validation
=== RUN   TestPacketTailParsing           ✅ 30-byte structure
=== RUN   TestLoadEmbeddedPandar40PConfig ✅ Calibration loading
=== RUN   TestPacketParsing               ✅ Point generation

# Network layer tests
go test ./internal/lidar/network -v        ✅ UDP forwarding
go test ./internal/lidar/monitor -v        ✅ Statistics & web server

# Frame builder tests
go test ./internal/lidar/ -v                        ✅ Complete test suite with integration
=== RUN   TestFrameBuilder_HybridDetection               ✅ Time-based + azimuth validation
=== RUN   TestFrameBuilder_AzimuthWrapWithTimeBased      ✅ Azimuth wrap in time-based mode
=== RUN   TestFrameBuilder_TraditionalAzimuthOnly        ✅ Traditional azimuth-only detection
=== RUN   TestHesaiLiDAR_PCAPIntegration                 ✅ End-to-end PCAP→parsing→framing

# Background subtraction tests
go test ./internal/lidar -run TestBackground            ✅ Background grid operations
go test ./internal/lidar -run TestStress                ✅ Concurrent load testing
go test ./internal/lidar -run TestExport                ✅ ASC export functionality
```

Key test coverage:

- Real Hesai packet validation with 22-byte tail structure
- Point generation with embedded calibration
- Time-based frame assembly with motor speed adaptation
- HTTP endpoint functionality
- Comprehensive frame builder testing with production-level data volumes (60,000 points)
- Both traditional azimuth-based and hybrid time-based frame detection modes
- End-to-end integration testing with real PCAP data (76,934 points → 56,929 frame points)
- Background grid learning and foreground detection
- Concurrent stress testing with race detection
- ASC point cloud export with elevation corrections

### 🔄 Planned Tests

- PCAP file reading and replay
- Parameter sweep automation
- Background settling with real-world data
- Clustering accuracy with known ground truth
- Tracking association and lifecycle
- Performance benchmarks under load
- Multi-track scenarios

---

## Development Workflow

### Next Implementation Steps (Phase 2.5 - PCAP Parameter Tuning)

**Goal**: Use existing PCAP captures (cars, pedestrians) to identify optimal background subtraction parameters before implementing clustering.

1. **PCAP Reader Implementation**:

   - Add PCAP file reading capability to UDP listener
   - Support both live UDP and PCAP file modes
   - Implement frame replay with configurable speed
   - Add loop mode for continuous parameter testing

2. **Parameter Sweep Integration**:

   - Use `bg-sweep` tool for single-parameter sweeps (noise_relative)
   - Use `bg-multisweep` tool for multi-parameter sweeps (noise, closeness, neighbors)
   - Analyze acceptance metrics to identify optimal thresholds
   - Document settling behavior with real-world data

3. **Threshold Identification**:

   - Analyze cars PCAP for vehicle detection thresholds
   - Analyze pedestrians PCAP for human detection thresholds
   - Identify optimal NoiseRelativeFraction values
   - Tune ClosenessSensitivityMultiplier for best separation
   - Optimize NeighborConfirmationCount for noise reduction

4. **Validation & Documentation**:
   - Validate identified parameters with both PCAP files
   - Document acceptance rates and foreground/background separation
   - Prepare parameter recommendations for production deployment
   - Update sweep tools with findings for future tuning

### Next Implementation Steps (Phase 3 - Clustering)

1. **Foreground Extraction**: Extract points classified as foreground from ProcessFramePolar
2. **Point Collection**: Build frame-level collection of foreground points
3. **Euclidean Clustering**: DBSCAN-style clustering with tuned parameters (eps, minPts)
4. **Cluster Metrics**: Compute centroid, PCA bbox, height_p95, intensity_mean
5. **World Frame Transform**: Convert clusters from sensor frame to world coordinates
6. **Database Integration**: Persist clusters to lidar_clusters table

### Development Tools

**Background Parameter Sweep Tools:**

- `cmd/bg-sweep/main.go` - Single-parameter sweeps with acceptance metrics

  - Supports noise_relative sweeps
  - Multiple modes: standard, settle, incremental
  - Outputs CSV with acceptance rates by distance bucket

- `cmd/bg-multisweep/main.go` - Multi-parameter grid search
  - Sweeps noise_relative × closeness_multiplier × neighbor_confirmation_count
  - Statistical analysis with mean/stddev per parameter combination
  - Raw and summary CSV outputs for analysis

**Available PCAP Test Data:**

The project has real-world PCAP captures for parameter validation:

- **Cars PCAP**: Vehicle traffic data for tuning vehicle detection thresholds
- **Pedestrians PCAP**: Pedestrian movement data for tuning human detection sensitivity

These PCAP files will be used to:

1. Identify optimal NoiseRelativeFraction values for distance-adaptive noise handling
2. Tune ClosenessSensitivityMultiplier for best foreground/background separation
3. Optimize NeighborConfirmationCount for spatial filtering effectiveness
4. Analyze background settling behavior with real-world motion patterns
5. Validate parameter choices across different target types (vehicles vs. pedestrians)

### Database Schema Overview

The system uses a comprehensive SQLite schema with 738 lines covering:

- **Sites & Sensors**: Physical deployment topology
- **Poses**: Time-versioned sensor calibration matrices
- **Frames**: LiDAR rotation data with metadata
- **Clusters**: Object detection results with rich features
- **Tracks**: Multi-object tracking with lifecycle management
- **Background**: Learned background models with automatic persistence

---

## Architecture Notes

### Design Decisions

- **Sensor vs World Frame**: Background subtraction in sensor frame (stable geometry), tracking in world frame (stable coordinates)
- **Hybrid Frame Detection**: Time-based primary trigger + azimuth validation prevents timing anomalies
- **22-byte Tail**: Confirmed with official Hesai documentation and real packet validation
- **SQLite Database**: Selected for simplicity and performance in single-node deployment
- **Embedded Calibration**: Baked-in calibration avoids runtime configuration complexity

### Future Extensions

- **Multi-Sensor Deployment**: Multiple LiDAR units per machine with local storage
- **Database Consolidation**: Merge SQLite databases from multiple edge nodes
- **World Frame Tracking**: Unified tracking across sensor coverage areas
- **Cross-Intersection Analysis**: Track objects moving between multiple intersections
- **Radar Integration**: Modular architecture allows future radar fusion
- **Production Optimization**: Memory pooling and advanced configuration options

---

## Production Readiness Assessment

### ✅ **Current State Summary**

The LiDAR sidecar has **completed Phases 1-2 (core infrastructure, background classification), Phase 2.5 (PCAP-based parameter tuning), and Phases 2.9-3.2 (foreground tracking pipeline)**. The complete pipeline from UDP packets to tracked objects is implemented and tested. The system is now ready for **Phase 3.3 (SQL Schema & REST APIs)** to enable database persistence and API access.

### ✅ **Completed Components**

- ✅ **Foundation**: Solid core infrastructure ready for production use
- ✅ **Performance**: Meets real-time processing requirements
- ✅ **Testing**: Comprehensive test coverage for implemented components
- ✅ **Configuration**: Flexible deployment options
- ✅ **Background Classification**: Distance-adaptive foreground/background classification with neighbor voting
- ✅ **Background Learning**: EMA-based background model updates with cell freezing
- ✅ **Persistence**: Background grid snapshots with versioning
- ✅ **Parameter Tuning**: Runtime-adjustable parameters via HTTP API
- ✅ **Monitoring**: Acceptance metrics and grid statistics for tuning
- ✅ **Sweep Tools**: Automated parameter sweep utilities for optimization
- ✅ **Foreground Mask Generation** (Phase 2.9): `ProcessFramePolarWithMask()`, `ExtractForegroundPoints()`
- ✅ **World Transform** (Phase 3.0): `TransformToWorld()` with pose support
- ✅ **DBSCAN Clustering** (Phase 3.1): `SpatialIndex`, `DBSCAN()`, `WorldCluster`
- ✅ **Kalman Tracking** (Phase 3.2): `Tracker`, `TrackedObject`, lifecycle management
- ✅ **ML Training Data Support**: `ForegroundFrame`, pose validation, compact encoding

### ✅ **Completed (Phase 2.5, 2.9, 3.0, 3.1, 3.2, 3.3, 3.4, 3.5)**

- ✅ **PCAP Reading**: File-based replay with BPF filtering (Phase 2.5)
- ✅ **Parameter Optimization**: Runtime-adjustable via HTTP API (Phase 2.5)
- ✅ **Foreground Extraction**: `ProcessFramePolarWithMask()` and `ExtractForegroundPoints()` (Phase 2.9)
- ✅ **World Transform**: `TransformToWorld()` with pose support (Phase 3.0)
- ✅ **Clustering**: `DBSCAN()` with `SpatialIndex` for efficient neighbor queries (Phase 3.1)
- ✅ **Tracking**: `Tracker` with Kalman filter and lifecycle management (Phase 3.2)
- ✅ **ML Training Data**: `ForegroundFrame` export and pose validation
- ✅ **SQL Schema**: `lidar_clusters`, `lidar_tracks`, `lidar_track_obs` tables (Phase 3.3)
- ✅ **Track Persistence**: `InsertCluster()`, `InsertTrack()`, `UpdateTrack()` functions (Phase 3.3)
- ✅ **Classification**: `TrackClassifier` for pedestrian/car/bird/other labels (Phase 3.4)
- ✅ **REST API Endpoints**: `TrackAPI` HTTP handlers for track/cluster queries (Phase 3.5)

### 📋 **Future Work (Phase 4)**

- 📋 **UI Visualization**: Track display components in web frontend
- 📋 **Multi-Sensor (Phase 4)**: Support multiple sensors per machine with local databases
- 📋 **Database Unification**: Consolidate data from distributed edge nodes
- 📋 **Cross-Sensor Tracking**: Track objects across multiple sensor coverage areas
- 📋 **Scale**: Memory optimization for 100+ tracks across multiple sensors

**Current Focus**: UI visualization for track display. REST API endpoints (Phase 3.5) are complete.

**Architecture**: Modular design with clear separation between:
- UDP ingestion and parsing
- Frame assembly  
- Background classification (polar frame)
- Foreground extraction (polar frame)
- World transform (polar → world)
- Clustering (world frame)
- Tracking (world frame)
- Classification (world frame)
- Database persistence (complete)
- REST APIs (complete)

**Pipeline Status**: The complete foreground tracking pipeline from UDP packets to tracked objects is implemented and tested. REST API endpoints are ready for UI integration.

**Multi-Sensor Vision (Phase 4)**: The architecture supports a distributed edge deployment model where each machine runs multiple LiDAR sensors, storing data locally in SQLite. Data from multiple edge nodes can be consolidated later for whole-street analysis and cross-intersection tracking in world frame coordinates.

The implementation is ready for Phase 3.3 (SQL Schema & REST APIs) development.
