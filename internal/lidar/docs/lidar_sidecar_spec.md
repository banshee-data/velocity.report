# LiDAR Sidecar — Implementation Specification

**Status:** Core infrastructure completed, background subtraction & tracking in development
**Scope:** Hesai UDP → parse → frame assembly → background subtraction → clustering → tracking → HTTP API
**Current Phase:** Phase 2 - Background subtraction and clustering

---

## Implementation Status

### ✅ **Phase 1: Core Infrastructure (COMPLETED)**
- UDP packet ingestion with configurable parameters (4MB buffer, 2369 port)
- Hesai Pandar40P packet parsing (30-byte tail structure validated)
- Time-based frame assembly with late packet handling (100ms frames, 1s buffer)
- SQLite database persistence with comprehensive schema (738 lines)
- HTTP monitoring interface with real-time statistics
- Comprehensive test suite with real packet validation

### 🔄 **Phase 2: Background & Clustering (CURRENT FOCUS)**
- Range-image background subtraction in sensor frame
- Foreground point clustering with configurable parameters
- Background model persistence to database
- Enhanced HTTP endpoints for tuning and monitoring

### 📋 **Phase 3: Tracking & World Transform (NEXT)**
- Pose management and coordinate transformations
- Multi-object Kalman filter tracking in world frame
- Track lifecycle management with configurable retention
- Complete REST API for tracking data

---

## Module Structure

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

**Data Flow:**
```
[UDP:2369] → [Parse] → [Frame Builder] → [Background (sensor)] → [Foreground Mask]
                                                                        ↓
[HTTP API] ← [Tracking (world)] ← [Transform] ← [Clustering] ← [Foreground Points]
```

---

## Core Algorithms

### Background Subtraction (Sensor Frame)
```
motion_threshold = average_range
                 - closeness_sensitivity_multiplier * range_spread
                 - safety_margin
is_foreground = (current_range < motion_threshold)
```
- **Spatial filtering**: 3×3 neighbor vote
- **Temporal filtering**: Freeze updates after foreground detection
- **Learning**: Slow EMA update when not frozen
- **Grid**: 40 rings × 1800 azimuth bins (0.2° resolution)

### Clustering (World Frame)
- **Euclidean clustering**: eps ≈ 0.6m, minPts ≈ 12
- **Per-cluster metrics**: centroid, PCA bbox, height_p95, intensity_mean

### Tracking (World Frame)
- **State vector**: [x, y, velocity_x, velocity_y]
- **Constant-velocity Kalman filter** with configurable noise parameters
- **Association**: Mahalanobis distance on position
- **Lifecycle**: Birth from unmatched clusters, death after consecutive misses

---

## Configuration

### ✅ Current Flags (Implemented)
```bash
-listen ":8081"              # HTTP server address
-udp-port 2369               # UDP listen port
-db "lidar_data.db"         # SQLite database file
-rcvbuf 4194304             # UDP receive buffer (4MB)
-debug                      # Enable debug logging
-forward                    # Enable packet forwarding
```

### 🔄 Planned Flags (Background & Tracking)
```bash
-bg.update_fraction 0.02         # EMA learning rate
-bg.sensitivity_multiplier 3.0   # Motion threshold
-bg.safety_margin_m 0.5         # Safety buffer
-bg.freeze_duration_ms 5000     # Freeze after detection
-max_concurrent_tracks 100      # Memory management
-pose_file "calibration.json"   # Sensor calibration
```

---

## HTTP Interface

### ✅ Current Endpoints
- `GET /health` - System status and packet statistics
- `GET /` - HTML dashboard with real-time metrics

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
- **HTTP Response**: <5ms for health/status endpoints

### 🎯 Target Performance (Complete System)
- **End-to-end Latency**: <100ms (packet → track update)
- **CPU Usage**: 1-2 cores at 10-15 Hz LiDAR rate
- **Memory Usage**: <300MB with 100 concurrent tracks
- **Track Capacity**: 100 active tracks with 1000 observations each

---

## Testing Status

### ✅ Implemented Tests
```bash
go test ./internal/lidar/parse -v   # Packet parsing validation
go test ./internal/lidar/network -v # UDP forwarding tests
go test ./internal/lidar/monitor -v # Statistics & web server
```

Key test coverage:
- Real Hesai packet validation with 30-byte tail structure
- Point generation with embedded calibration
- Frame assembly with time-based buffering
- HTTP endpoint functionality

---

## Development Workflow

### Next Implementation Steps (Phase 2)
1. **Background Grid**: Range-image binning (40 rings × 1800 azimuth bins)
2. **Motion Detection**: Per-cell background learning with EMA updates
3. **Spatial Filtering**: 3×3 neighbor voting for noise reduction
4. **Persistence**: Automatic background snapshot saving to database
5. **HTTP Interface**: Add `/fg` endpoint for background tuning

### Database Schema
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
- **Time-based Frames**: 100ms default duration with 1s late packet buffer
- **30-byte Tail**: Confirmed with official Hesai documentation and real packet validation
- **SQLite Database**: Selected for simplicity and performance in single-node deployment

### Future Extensions
- **Radar Integration**: Modular architecture allows future radar fusion
- **Multi-sensor**: Support for multiple LiDAR units with pose management
- **Production Optimization**: Memory pooling and advanced configuration options

Architecture for modular sensor deployment with independent HTTP interfaces:

```
┌─────────────────┐    gRPC     ┌──────────────────┐
│   cmd/radar     │ ───────────▶│   cmd/lidar      │
│                 │             │                  │
│ • Serial listen │             │ • UDP listen     │
│ • Parse radar   │             │ • Parse lidar    │
│ • HTTP endpoints│             │ • HTTP endpoints │
│ • Standalone OK │             │ • Tracking       │
└─────────────────┘             │ • Fusion logic   │
         │                      └──────────────────┘
         │                               │
         ▼                               ▼
    ┌─────────────────────────────────────────┐
    │           web/ (Svelte/Vite)            │
    │                                         │
    │ • Proxy radar HTTP (if available)       │
    │ • Proxy lidar HTTP (if available)       │
    │ • Aggregate sensor data for UI          │
    │ • Handle radar-only or lidar-only       │
    └─────────────────────────────────────────┘
```

---

## Implementation Summary

The LiDAR sidecar has a **solid foundation** with core UDP ingestion, packet parsing, frame assembly, and monitoring fully implemented and tested. The 30-byte packet tail structure is validated against real Hesai Pandar40P data, and the database schema is comprehensive and production-ready.

**Current Focus**: Implementing background subtraction and clustering algorithms to complete the perception pipeline before adding tracking capabilities.

**Architecture**: Modular design with clear separation between UDP ingestion, parsing, frame assembly, background processing, and tracking - ready for production deployment.
