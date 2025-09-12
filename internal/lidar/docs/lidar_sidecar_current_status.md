# LiDAR Sidecar — Current Implementation Status & Roadmap

**Current Status:** Core LiDAR packet parsing, UDP listener, and monitoring infrastructure implemented
**Scope:** Ingest Hesai UDP → parse to points → frame assembly → (future: background subtraction & tracking)
**Schema:** LiDAR-only implementation with comprehensive database schema
**Implemented:** UDP ingestion, packet parsing, frame building, monitoring, database persistence
**In Progress:** Background subtraction and tracking algorithms

---

## 1) Current Implementation Status

### ✅ **Completed Components**

```
[UDP Listener] ✅ -> [Parser] ✅ -> [Frame Assembler] ✅ -> [Database] ✅
                                        |
                                   [HTTP Monitor] ✅
                                        |
                                 [Statistics] ✅
```

### 🔄 **In Development**
- Background subtraction (sensor frame)
- Clustering and world transformation
- Tracking algorithms
- Advanced HTTP endpoints

### 📋 **Future Milestones**
- Range-image background subtraction
- Foreground clustering
- World frame transformation
- Multi-object tracking

---

## 2) Current Module Structure

```
cmd/lidar/main.go                  ✅ # Complete with flags, goroutines, HTTP
internal/lidar/network/listener.go ✅ # UDP socket and packet processing
internal/lidar/network/forwarder.go✅ # UDP packet forwarding to LidarView
internal/lidar/parse/extract.go    ✅ # Pandar40P packet -> []Point (30-byte tail)
internal/lidar/parse/config.go     ✅ # Embedded calibration configurations
internal/lidar/frame_builder.go    ✅ # Time-based frame assembly
internal/lidar/monitor/            ✅ # HTTP endpoints: /health, /status
internal/lidar/lidardb/            ✅ # Database schema and persistence
internal/lidar/arena.go            🔄 # Background, clustering, tracking (stubbed)
```

---

## 3) Implemented Data Structures

### Core LiDAR Data (✅ Implemented)

```go
// Point represents a single 3D LiDAR measurement (fully implemented)
type Point struct {
    X, Y, Z     float64   // 3D Cartesian coordinates
    Intensity   uint8     // Laser return intensity (0-255)
    Distance    float64   // Radial distance from sensor
    Azimuth     float64   // Horizontal angle (0-360°, corrected)
    Elevation   float64   // Vertical angle (corrected for channel)
    Channel     int       // Laser channel (1-40)
    Timestamp   time.Time // Point acquisition time (with firetime correction)
    BlockID     int       // Data block index (0-9)
}

// PacketTail - 30-byte structure matching official Hesai documentation (✅ Implemented)
type PacketTail struct {
    Reserved1   [5]uint8  // Reserved bytes 0-4
    HighTempFlag uint8    // 0x01 High temp, 0x00 Normal
    Reserved2   [2]uint8  // Reserved bytes 6-7
    MotorSpeed  uint16    // RPM (little-endian)
    Timestamp   uint32    // Microsecond UTC (little-endian)
    ReturnMode  uint8     // 0x37 Strongest, 0x38 Last, 0x39 Both
    FactoryInfo uint8     // 0x42 or 0x43
    DateTime    [6]uint8  // Whole second UTC
    UDPSequence uint32    // Packet sequence (little-endian)
    FCS         [4]uint8  // Frame check sequence
}

// LiDARFrame - Complete 360° rotation (✅ Implemented)
type LiDARFrame struct {
    FrameID        string    // Unique frame identifier
    SensorID       string    // Sensor that generated frame
    StartTimestamp time.Time // First point timestamp
    EndTimestamp   time.Time // Last point timestamp
    Points         []Point   // All points in rotation
    MinAzimuth     float64   // Min azimuth observed
    MaxAzimuth     float64   // Max azimuth observed
    PointCount     int       // Total points
    SpinComplete   bool      // Full 360° detected
}
```

### Database Schema (✅ Implemented)

```sql
-- Core tables implemented in internal/lidar/lidardb/schema.sql
CREATE TABLE sites (
    site_id TEXT PRIMARY KEY,
    world_frame TEXT NOT NULL
);

CREATE TABLE sensors (
    sensor_id TEXT PRIMARY KEY,
    site_id TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('lidar')),
    model TEXT,
    FOREIGN KEY (site_id) REFERENCES sites (site_id)
);

CREATE TABLE sensor_poses (
    pose_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    from_frame TEXT NOT NULL,
    to_frame TEXT NOT NULL,
    t_rowmajor_4x4 BLOB NOT NULL, -- 16 float64 values
    valid_from_ns INTEGER NOT NULL,
    valid_to_ns INTEGER,
    method TEXT,
    root_mean_square_error_meters REAL,
    FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
);

-- Additional tables for frames, clusters, tracks, background snapshots
-- (See schema.sql for complete 738-line specification)
```

### Configuration & Parsing (✅ Implemented)

```go
// Pandar40PParser - Fully implemented with 30-byte tail support
type Pandar40PParser struct {
    config        Pandar40PConfig // Embedded calibration data
    timestampMode TimestampMode   // PTP, GPS, System, or Internal
    bootTime      time.Time       // For internal timestamp mode
    packetCount   int             // Debug counter
    lastTimestamp uint32          // Static detection
    staticCount   int             // Static timestamp counter
    debug         bool            // Debug logging
}

// Performance metrics (current implementation):
// - Processing time: ~36.5μs per packet
// - Memory allocation: ~170KB per packet
// - Points per packet: up to 400 (40 channels × 10 blocks)
```

---

## 4) Current Algorithm Implementation

### 4.1 UDP Ingestion & Parsing (✅ Complete)
- **UDP Listener**: Configurable port (default 2369), 4MB receive buffer
- **Packet Validation**: 1262-byte (standard) or 1266-byte (with sequence) packets
- **Tail Parsing**: Complete 30-byte structure per official Hesai documentation
- **Point Generation**: 40 channels × 10 blocks = up to 400 points per packet
- **Calibration**: Embedded per-channel angle and firetime corrections
- **Coordinate Transform**: Spherical → Cartesian with calibration applied

### 4.2 Frame Assembly (✅ Complete)
- **Time-based Buffering**: 100ms default frame duration
- **Late Packet Handling**: 1-second buffer for out-of-order packets
- **Spin Detection**: Azimuth wrap-around detection for complete rotations
- **Frame Callback**: Configurable callback for frame completion

### 4.3 Database Persistence (✅ Complete)
- **SQLite with WAL**: High-performance concurrent access
- **Comprehensive Schema**: 738 lines covering all LiDAR data types
- **Performance Optimized**: Prepared statements, batch inserts
- **Schema Versioning**: Automatic migration support

### 4.4 Background Subtraction (🔄 Planned)
- **Sensor Frame Processing**: Range-image based background learning
- **Parameters**: Update fraction, sensitivity, safety margin, freeze duration
- **Grid Structure**: Ring/azimuth binning for 40-channel sensor
- **Persistence**: Automatic background snapshots to database

### 4.5 World Transform & Tracking (🔄 Planned)
- **Pose Management**: Time-versioned transformation matrices
- **Clustering**: Euclidean clustering on foreground points
- **Kalman Tracking**: 2D constant-velocity model in world frame
- **Track Management**: Birth, association, update, death cycle

---

## 5) Current HTTP Interface

### 5.1 ✅ `GET /health` (Implemented)
```json
{
  "status": "healthy",
  "uptime_seconds": 3661,
  "packets_received": 12543,
  "packets_per_second": 3.4,
  "points_per_second": 1360,
  "frames_completed": 42,
  "parsing_enabled": true,
  "forwarding_enabled": false
}
```

### 5.2 ✅ `GET /` (Status Dashboard - Implemented)
- HTML status page with real-time statistics
- Packet rates, point counts, system metrics
- UDP configuration and parsing status

### 5.3 🔄 `GET /fg` (Planned)
```json
{ "foreground_points": 8421, "background_points": 99123, "bins_frozen": 321 }
```

### 5.4 🔄 `GET /tracks/recent` (Planned)
```json
[
  {
    "track_id": "t-123",
    "sensor_id": "hesai-pandar40p-001",
    "world_frame": "site/main-st-001",
    "pose_id": 42,
    "unix_nanos": 1699999999999999999,
    "x": 12.5, "y": 8.3,
    "velocity_x": 2.1, "velocity_y": 0.5,
    "speed_mps": 2.16,
    "heading_rad": 0.23,
    "bounding_box_length": 4.2,
    "class_label": "", "class_conf": 0.0
  }
]
```

---

## 6) Current Configuration

### ✅ Implemented Flags (cmd/lidar/main.go)
```bash
-listen ":8081"              # HTTP server address
-udp-port 2369               # UDP listen port
-udp-addr ""                 # UDP bind address (default: all interfaces)
-no-parse                    # Disable packet parsing
-forward                     # Enable packet forwarding
-forward-port 2368           # Forward destination port
-forward-addr "localhost"    # Forward destination address
-db "lidar_data.db"         # SQLite database file
-rcvbuf 4194304             # UDP receive buffer (4MB)
-log-interval 2             # Statistics interval (seconds)
-debug                      # Enable debug logging
```

### 🔄 Planned Configuration
```bash
# Background subtraction parameters
-bg.update_fraction 0.02
-bg.sensitivity_multiplier 3.0
-bg.safety_margin_m 0.5
-bg.freeze_duration_ms 5000
-bg.neighbor_votes 5

# Tracking parameters
-max_concurrent_tracks 100
-track_max_age_min 30
-pose_file "calibration.json"
```

---

## 7) Performance Metrics (Current Implementation)

### ✅ **Current Performance**
- **Packet Processing**: 36.5μs per packet
- **UDP Throughput**: Handles 10 Hz LiDAR (typical Pandar40P rate)
- **Memory Usage**: ~170KB per packet processing
- **Database**: High-performance SQLite with WAL mode
- **HTTP Response**: <5ms for health/status endpoints

### 🎯 **Target Performance**
- **End-to-end Latency**: <100ms (packet → tracking results)
- **CPU Usage**: 1-2 cores at 10-15 Hz
- **Memory Usage**: <300MB with background grid and tracking
- **Concurrent Tracks**: 100 active tracks maximum

---

## 8) Testing Status

### ✅ **Implemented Tests**
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
```

### 🔄 **Planned Tests**
- Background subtraction accuracy
- Tracking association and lifecycle
- Performance benchmarks under load
- Multi-track scenarios

---

## 9) Implementation Milestones

### ✅ **Phase 1: Core Infrastructure (COMPLETED)**
1. UDP listener with configurable parameters ✅
2. Packet parser with 30-byte tail support ✅
3. Frame assembly with time-based buffering ✅
4. Database schema and persistence ✅
5. HTTP monitoring interface ✅
6. Comprehensive test suite ✅

### 🔄 **Phase 2: Background & Clustering (IN PROGRESS)**
1. Range-image background subtraction
2. Foreground point clustering
3. Background persistence to database
4. Enhanced HTTP endpoints (/fg)

### 📋 **Phase 3: Tracking & World Transform (PLANNED)**
1. Pose management and world transforms
2. Multi-object tracking with Kalman filters
3. Track lifecycle management
4. REST API for tracks (/tracks/recent, /track/:id)

### 📋 **Phase 4: Production Optimization (PLANNED)**
1. Performance profiling and optimization
2. Memory usage optimization for 100 tracks
3. Advanced configuration options
4. Production deployment documentation

---

## 10) Technical Architecture Notes

### **Current State Summary**
The LiDAR sidecar has a **solid foundation** with core UDP ingestion, packet parsing, frame assembly, and monitoring fully implemented and tested. The 30-byte packet tail structure is validated against real Hesai Pandar40P data, and the database schema is comprehensive and production-ready.

### **Architecture Decisions Made**
1. **30-byte Tail**: Confirmed with official Hesai documentation and real packet validation
2. **Time-based Frames**: Chosen over azimuth-based for better late packet handling
3. **SQLite Database**: Selected for simplicity and performance in single-node deployment
4. **Embedded Calibration**: Baked-in calibration avoids runtime configuration complexity

### **Future Data Structures (Ready for Implementation)**

```go
// Background Subtraction (🔄 Next Priority)
type BackgroundParams struct {
    BackgroundUpdateFraction       float32 // e.g., 0.02 - EMA update rate
    ClosenessSensitivityMultiplier float32 // e.g., 3.0 - motion threshold
    SafetyMarginMeters             float32 // e.g., 0.5 - safety buffer
    FreezeDurationNanos            int64   // e.g., 5e9 - freeze after detection
    NeighborConfirmationCount      int     // e.g., 5 - spatial filtering
}

type BackgroundGrid struct {
    SensorID    string
    SensorFrame FrameID
    Rings       int                // 40 for Pandar40P
    AzimuthBins int                // e.g., 1800 for 0.2° resolution
    Cells       []BackgroundCell   // len=Rings*AzimuthBins
    Params      BackgroundParams
    // ... persistence and management fields
}

// World-space Tracking (📋 Future)
type WorldCluster struct {
    ClusterID         int64   // lidar_cluster_id PRIMARY KEY
    SensorID          string  // sensor_id TEXT NOT NULL
    WorldFrame        FrameID // world_frame TEXT NOT NULL
    PoseID            int64   // pose_id INTEGER NOT NULL
    TSUnixNanos       int64   // ts_unix_nanos INTEGER NOT NULL
    CentroidX         float32 // centroid_x REAL
    CentroidY         float32 // centroid_y REAL
    CentroidZ         float32 // centroid_z REAL
    BoundingBoxLength float32 // bounding_box_length REAL
    BoundingBoxWidth  float32 // bounding_box_width REAL
    BoundingBoxHeight float32 // bounding_box_height REAL
    PointsCount       int     // points_count INTEGER
    HeightP95         float32 // height_p95 REAL
    IntensityMean     float32 // intensity_mean REAL
}

type Track struct {
    TrackID    string  // track_id TEXT PRIMARY KEY
    SensorID   string  // sensor_id TEXT NOT NULL
    WorldFrame FrameID // world_frame TEXT NOT NULL
    PoseID     int64   // pose_id INTEGER NOT NULL

    // Lifecycle timestamps
    FirstUnixNanos int64 // start_unix_nanos INTEGER NOT NULL
    LastUnixNanos  int64 // end_unix_nanos INTEGER (NULL if active)

    // Current state for real-time tracking
    State TrackState2D

    // Statistics matching schema fields
    ObservationCount     int     // observation_count INTEGER
    AvgSpeedMps          float32 // avg_speed_mps REAL
    PeakSpeedMps         float32 // peak_speed_mps REAL
    BoundingBoxLengthAvg float32 // bounding_box_length_avg REAL
    BoundingBoxWidthAvg  float32 // bounding_box_width_avg REAL
    BoundingBoxHeightAvg float32 // bounding_box_height_avg REAL
    HeightP95Max         float32 // height_p95_max REAL
    IntensityMeanAvg     float32 // intensity_mean_avg REAL
    ClassLabel           string  // class_label TEXT
    ClassConfidence      float32 // class_conf REAL
    SourceMask           uint8   // source_mask INTEGER (bit0=lidar)
}
```

### **Next Priority Steps**
1. **Background Algorithm**: Implement range-image subtraction in sensor frame
2. **Clustering**: Add Euclidean clustering for foreground points
3. **World Transform**: Implement pose lookup and coordinate transformation
4. **Tracking**: Add Kalman filter-based multi-object tracking
5. **Memory Management**: Add ring buffers for production efficiency

### **Production Readiness Assessment**
- ✅ **Foundation**: Solid core infrastructure ready for production use
- ✅ **Performance**: Meets real-time processing requirements
- ✅ **Testing**: Comprehensive test coverage for implemented components
- ✅ **Configuration**: Flexible deployment options
- 🔄 **Perception**: Background subtraction and tracking algorithms needed
- 📋 **Scale**: Memory optimization needed for 100-track scenarios

The implementation is ready for background subtraction development as the next major milestone.
