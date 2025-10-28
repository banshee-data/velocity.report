# LiDAR Sidecar — Technical Implementation Overview

**Status:** Phase 1 & 2 completed, PCAP parameter tuning in progress
**Scope:** Hesai UDP → parse → frame assembly → background subtraction → clustering → tracking → HTTP API
**Current Phase:** Phase 2.5 - PCAP-based parameter identification for background tuning

---

## Implementation Status

### ✅ **Phase 1: Core Infrastructure (COMPLETED)**

- UDP packet ingestion with configurable parameters (4MB buffer, 2369 port)
- Hesai Pandar40P packet parsing (22-byte tail structure validated)
- Time-based frame assembly with motor speed adaptation (360° detection, 1s buffer)
- SQLite database persistence with comprehensive schema (738 lines)
- HTTP monitoring interface with real-time statistics
- Comprehensive test suite with real packet validation

### 🔄 **Phase 2: Background & Clustering (IN PROGRESS)**

- ✅ Background grid infrastructure with EMA learning (implemented)
- ✅ Foreground/background classification with neighbor voting (implemented)
- ✅ Background model persistence to database (implemented)
- ✅ Enhanced HTTP endpoints for tuning and monitoring (implemented)
- ✅ Acceptance metrics for parameter tuning (implemented)
- 🔄 PCAP file reading for parameter identification (current focus)
- 📋 Foreground point extraction and clustering (planned)

### 📋 **Phase 2.5: PCAP-Based Parameter Tuning (CURRENT FOCUS)**

- PCAP file ingestion and frame replay
- Automated parameter sweep using bg-sweep and bg-multisweep tools
- Background settling analysis with real-world data (cars, pedestrians)
- Optimal threshold identification for noise, learning, filtering, and clustering
- Integration with existing sweep tools for iterative tuning

### 📋 **Phase 3: Tracking & World Transform (NEXT)**

- Pose management and coordinate transformations
- Multi-object Kalman filter tracking in world frame
- Track lifecycle management with configurable retention
- Complete REST API for tracking data

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

---

## Module Structure

```
cmd/radar/radar.go                 ✅ # LiDAR integration with -enable-lidar flag
cmd/bg-sweep/main.go               ✅ # Single-parameter sweep tool for tuning
cmd/bg-multisweep/main.go          ✅ # Multi-parameter grid search tool
internal/lidar/network/listener.go ✅ # UDP socket and packet processing
internal/lidar/network/forwarder.go✅ # UDP packet forwarding to LidarView
internal/lidar/parse/extract.go    ✅ # Pandar40P packet -> []Point (22-byte tail)
internal/lidar/parse/config.go     ✅ # Embedded calibration configurations
internal/lidar/frame_builder.go    ✅ # Time-based frame assembly with motor speed
internal/lidar/monitor/            ✅ # HTTP endpoints: /health, /api/lidar/*
internal/lidar/background.go       ✅ # Background model & classification with persistence
internal/lidar/export.go           ✅ # ASC point cloud export
internal/lidar/arena.go            🔄 # Clustering and tracking (stubbed)
internal/db/db.go                  ✅ # Database schema and BgSnapshot persistence
```

**Data Flow:**

```
[UDP:2369] → [Parse] → [Frame Builder] → [Background (sensor)] → [Foreground Mask]
                                                                        ↓
[HTTP API] ← [Tracking (world)] ← [Transform] ← [Clustering] ← [Foreground Points]
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
- **Traditional Fallback**: Pure azimuth-based detection (350° → 10°) when motor speed unavailable
- **Late Packet Handling**: 1-second buffer for out-of-order packets before final callback
- **Frame Callback**: Configurable callback for frame completion

### Database Persistence (✅ Complete)

- **SQLite with WAL**: High-performance concurrent access
- **Performance Optimized**: Prepared statements, batch inserts

### Background Model & Classification (✅ Implemented, Subtraction Not Yet)

**Current State:**

- The system implements background model learning and foreground/background classification for each observation.
- Actual foreground point extraction (subtraction) is **not yet implemented**; only counters and classification are tracked.

**Algorithm (Implemented):**

```
closeness_threshold = closeness_multiplier * (range_spread + noise_relative * observation_mean + 0.01)
                    + safety_margin
cell_diff = abs(cell_average_range - observation_mean)
is_background = (cell_diff <= closeness_threshold) OR (neighbor_confirm >= required_neighbors)
```

**Implementation Details:**

- **Classification**: Each observation is classified as background or foreground, but foreground points are not yet extracted for further processing.
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

**What's Not Yet Implemented:**

- ❌ Foreground point extraction/subtraction (actual filtering of points for clustering)
- ❌ Clustering of foreground points into objects

### Clustering (🔄 Planned)

- **Euclidean clustering**: eps ≈ 0.6m, minPts ≈ 12
- **Per-cluster metrics**: centroid, PCA bbox, height_p95, intensity_mean
- **World Frame Processing**: Transform from sensor to world coordinates

### Tracking (🔄 Planned)

- **State vector**: [x, y, velocity_x, velocity_y]
- **Constant-velocity Kalman filter** with configurable noise parameters
- **Association**: Mahalanobis distance on position
- **Lifecycle**: Birth from unmatched clusters, death after consecutive misses
- **Track Management**: Birth, association, update, death cycle

---

## Configuration

### ✅ Current Flags (Implemented)

The LiDAR functionality is integrated into the `cmd/radar/radar.go` binary and enabled via the `-enable-lidar` flag:

```bash
# Radar binary with LiDAR integration
./radar [radar flags...] -enable-lidar [lidar flags...]

# LiDAR integration flags
-enable-lidar                        # Enable lidar components inside radar binary
-lidar-listen ":8081"                # HTTP listen address for lidar monitor
-lidar-udp-port 2369                 # UDP port to listen for lidar packets
-lidar-no-parse                      # Disable lidar packet parsing
-lidar-sensor "hesai-pandar40p"      # Sensor name identifier for lidar
-lidar-forward                       # Forward lidar UDP packets to another port
-lidar-forward-port 2368             # Port to forward lidar UDP packets to
-lidar-forward-addr "localhost"      # Address to forward lidar UDP packets to

# Background subtraction tuning (runtime-adjustable via HTTP API)
-bg-noise-relative 0.315             # NoiseRelativeFraction: fraction of range treated as measurement noise
```

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
```

### 🔄 PCAP Reading Flags (Planned)

```bash
-pcap-file "capture.pcap"            # PCAP file to read instead of live UDP
-pcap-loop                           # Loop PCAP playback for continuous testing
-pcap-speed 1.0                      # Playback speed multiplier (1.0 = realtime)
```

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
- `POST /api/lidar/params?sensor_id=<id>` - Update background parameters
- `GET /api/lidar/acceptance?sensor_id=<id>` - Get acceptance metrics by range bucket
- `POST /api/lidar/acceptance/reset?sensor_id=<id>` - Reset acceptance counters
- `POST /api/lidar/grid_reset?sensor_id=<id>` - Reset background grid (for testing/sweeps)
- `GET /api/lidar/grid/status?sensor_id=<id>` - Get grid statistics and settling status
- `GET /api/lidar/grid/export_asc?sensor_id=<id>` - Export background grid as ASC point cloud

### 🔄 Planned Endpoints

- `GET /fg` - Foreground/background statistics
- `GET /tracks/recent` - Recent track states
- `GET /track/:id` - Full track history
- `GET /clusters/recent` - Recent cluster detections

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

The LiDAR sidecar has **completed Phase 1 (core infrastructure) and the background classification portion of Phase 2**. Background learning, foreground/background classification, and parameter tuning infrastructure are fully operational. The system is now ready for **Phase 2.5 (PCAP-based parameter identification)** before implementing foreground point extraction and clustering.

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

### 🔄 **In Development (Phase 2.5)**

- 🔄 **PCAP Reading**: File-based replay for parameter identification
- 🔄 **Parameter Optimization**: Use real-world PCAP data to tune thresholds

### 📋 **Future Work**

- 📋 **Foreground Extraction**: Extract foreground-classified points from frames
- 📋 **Clustering**: Euclidean clustering on foreground points
- 📋 **Tracking**: Kalman filter-based multi-object tracking in world frame
- 📋 **Multi-Sensor**: Support multiple sensors per machine with local databases
- 📋 **Database Unification**: Consolidate data from distributed edge nodes
- 📋 **Cross-Sensor Tracking**: Track objects across multiple sensor coverage areas
- 📋 **Scale**: Memory optimization for 100+ tracks across multiple sensors

**Current Focus**: Implementing PCAP file reading to enable parameter tuning with real-world data (cars, pedestrians) before implementing foreground extraction and clustering algorithms.

**Architecture**: Modular design with clear separation between UDP ingestion, parsing, frame assembly, background classification, and (future) clustering/tracking. Background classification is production-ready; foreground extraction, clustering, and tracking await parameter identification via PCAP analysis.

**Multi-Sensor Vision (Phase 4)**: The architecture supports a distributed edge deployment model where each machine runs multiple LiDAR sensors, storing data locally in SQLite. Data from multiple edge nodes can be consolidated later for whole-street analysis and cross-intersection tracking in world frame coordinates.

The implementation is ready for PCAP reader development and parameter sweep execution as the next major milestone.
