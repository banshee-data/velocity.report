# velocity.report architecture

This document describes the system architecture, component relationships, data flow,
and integration points for the velocity.report traffic monitoring system.

For canonical numeric constants (ports, tuning defaults, and hard-coded thresholds),
see [MAGIC_NUMBERS.md](MAGIC_NUMBERS.md).

### How to read this document

- **Perception / ML engineers:** Start with [Perception pipeline](#perception-pipeline) and [Data & evaluation](#data--evaluation), then [Component status](#component-status) for the layer table.
- **Deploying the system:** Start with [Deployment architecture](#deployment-architecture), then [Quick start](README.md#quick-start).
- **Contributing code:** Start with [Components](#components) and [Integration points](#integration-points).

## Table of contents

- [System overview](#system-overview)
- [Sensor hardware](#sensor-hardware)
- [Architecture diagram](#architecture-diagram)
- [Components](#components)
- [Perception pipeline](#perception-pipeline)
- [Data flow](#data-flow)
- [Technology stack](#technology-stack)
- [Integration points](#integration-points)
- [Data & evaluation](#data--evaluation)
- [Deployment architecture](#deployment-architecture)
- [Security & privacy](#security--privacy)
- [Component status](#component-status)
- [Roadmap](#roadmap)
- [Mathematical references](#mathematical-references)

## System overview

**velocity.report** is a privacy-preserving traffic monitoring platform.
The core product is radar-based speed measurement:
a Doppler radar sensor captures vehicle speeds, the Go server stores and aggregates the data,
and produces professional reports ready for a city engineer's desk or a planning committee hearing.
(PDF generation is migrating from the legacy Python + LaTeX tool into the Go server.) No cameras,
no licence plates, no personally identifiable information: by architecture, not by policy.

The LiDAR pipeline extends the picture.
Where radar sees a vehicle's speed through a narrow field of view, LiDAR sees the full scene:
every object's shape, trajectory, and classification across the entire road.
A car that enters at 25 mph, slows for a pedestrian,
and exits at 35 mph is one speed reading to radar but a complete behavioural record to LiDAR.
The perception stack (DBSCAN clustering, Kalman-filtered tracking,
rule-based classification) runs on the same Raspberry Pi,
processing 70,000 points per frame at 10 Hz with no cloud dependency.

The two sensors are complementary. Radar provides Doppler-accurate speed.
LiDAR provides geometry, object identity, and track continuity.
Fusing them is the [v1.0 goal](docs/plans/lidar-l7-scene-plan.md).

| Component            | Language            | Purpose                                                                         |
| -------------------- | ------------------- | ------------------------------------------------------------------------------- |
| **Go server**        | Go                  | Sensor data collection, SQLite storage, HTTP + gRPC API                         |
| **PDF generator**    | Python + LaTeX      | Professional speed reports with charts, statistics, and formatting (deprecated) |
| **Web frontend**     | Svelte + TypeScript | Real-time data visualisation and interactive dashboards                         |
| **macOS visualiser** | Swift + Metal       | Native 3D LiDAR point cloud viewer with tracking and replay                     |

### Design principles

- **Privacy First**: No licence plates, no video, no PII
- **Simplicity**: SQLite as the only database, minimal dependencies
- **Offline-First**: Works without internet connectivity
- **Modular**: Each component operates independently
- **Well-Tested**: Comprehensive test coverage across all components

## Sensor hardware

| Sensor | Model                 | Measurement    | Interface           | Key Specifications                                                                     |
| ------ | --------------------- | -------------- | ------------------- | -------------------------------------------------------------------------------------- |
| Radar  | OmniPreSense OPS243-A | Doppler speed  | USB-Serial (RS-232) | K-band (24 GHz), ±0.1 mph accuracy, FFT-based, configurable speed/magnitude thresholds |
| LiDAR  | Hesai Pandar40P       | 3D point cloud | Ethernet/UDP (PoE)  | 40 beams, 200 m range, 10 Hz rotation, 0.2° azimuth resolution, ~70,000 points/frame   |

Radar delivers Doppler-accurate speed through a narrow field of view.
LiDAR delivers full-scene geometry: shape, trajectory, and classification across the entire road.
The two sensors run independently today;
sensor fusion via cross-sensor track handoff is the [v1.0 goal](docs/plans/lidar-l7-scene-plan.md).

For detailed sensor specifications, wiring, and calibration:
see [.github/knowledge/hardware.md](.github/knowledge/hardware.md).

## Architecture diagram

### Physical deployment

```
┌──────────────────────────────────────────────────────────────────────┐
│                        HARDWARE INFRASTRUCTURE                       │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌────────────────────┐                 ┌────────────────────┐       │
│  │  Radar Sensor      │                 │  LiDAR Sensor      │       │
│  │  ┌──────────────┐  │                 │  ┌──────────────┐  │       │
│  │  │ Omnipresense │  │                 │  │   Hesai P40  │  │       │
│  │  │   OPS243     │  │                 │  │    40-beam   │  │       │
│  │  └──────────────┘  │                 │  └──────────────┘  │       │
│  │   Serial Output    │                 │   Ethernet Output  │       │
│  │   (USB/RS-232)     │                 │   (RJ45/PoE)       │       │
│  └─────────┬──────────┘                 └──────────┬─────────┘       │
│            │                                       │                 │
│            │ USB-Serial                            │ Ethernet        │
│            │                                       │                 │
│            └───────────────────┬───────────────────┘                 │
│                                │                                     │
│  ┌─────────────────────────────┼──────────────────────────────────┐  │
│  │       Raspberry Pi 4 (ARM64 Linux)                             │  │
│  │                             │                                  │  │
│  │  Hardware:                  │                                  │  │
│  │  • 4GB RAM                  │                                  │  │
│  │  • 64GB SD Card             ↓                                  │  │
│  │  • USB Ports (Radar)   /dev/ttyUSB0                            │  │
│  │  • Ethernet Port (LiDAR + Network)                             │  │
│  │    - LiDAR network: 192.168.100.151/24 (listener)              │  │
│  │    - LiDAR sensor:  192.168.100.202 (UDP source)               │  │
│  │    - Local LAN:     192.168.1.x (API + gRPC server)            │  │
│  │                                                                │  │
│  │  Network: Dual configuration (LiDAR subnet + Local LAN)        │  │
│  │                                                                │  │
│  ├────────────────────────────────────────────────────────────────┤  │
│  │                 SOFTWARE STACK (on this Raspberry Pi)          │  │
│  ├────────────────────────────────────────────────────────────────┤  │
│  │                                                                │  │
│  │  ┌──────────────────────────────────────────────────────────┐  │  │
│  │  │         velocity.report Go Server                        │  │  │
│  │  │         (systemd service: velocity-report.service)       │  │  │
│  │  │                                                          │  │  │
│  │  │  ┌────────────────────────────────────────────────────┐  │  │  │
│  │  │  │  Sensor Input Handlers                             │  │  │  │
│  │  │  │                                                    │  │  │  │
│  │  │  │  ┌──────────────────┐  ┌──────────────────────┐    │  │  │  │
│  │  │  │  │ Radar Handler    │  │ LiDAR Handler        │    │  │  │  │
│  │  │  │  │ (Serial Port)    │  │ (Network/UDP)        │    │  │  │  │
│  │  │  │  │ internal/radar/  │  │ internal/lidar/      │    │  │  │  │
│  │  │  │  │                  │  │                      │    │  │  │  │
│  │  │  │  │ • Parse speed    │  │ • Decode UDP blocks  │    │  │  │  │
│  │  │  │  │ • JSON events    │  │ • FrameBuilder merge │    │  │  │  │
│  │  │  │  │                  │  │ • Background manager │    │  │  │  │
│  │  │  │  └───────┬──────────┘  └───────┬──────────────┘    │  │  │  │
│  │  │  │          │                     │                   │  │  │  │
│  │  │  │          │ radar_data          │ lidar_bg_snapshot │  │  │  │
│  │  │  │          │ (raw JSON)          │ (BLOB grid)       │  │  │  │
│  │  │  │          │                     │                   │  │  │  │
│  │  │  └──────────┼─────────────────────┼───────────────────┘  │  │  │
│  │  │             │                     │                      │  │  │
│  │  │  ┌──────────▼─────────────────────▼───────────────────┐  │  │  │
│  │  │  │  Sensor Pipelines → SQLite + gRPC                  │  │  │  │
│  │  │  │                                                    │  │  │  │
│  │  │  │  Radar Serial (/dev/ttyUSB0)                       │  │  │  │
│  │  │  │    → ops243 reader → JSON parse                    │  │  │  │
│  │  │  │    → INSERT radar_data, radar_objects              │  │  │  │
│  │  │  │                                                    │  │  │  │
│  │  │  │  LiDAR Ethernet (Hesai UDP 192.168.100.202)        │  │  │  │
│  │  │  │    → packet decoder → FrameBuilder rotations       │  │  │  │
│  │  │  │    → BackgroundManager EMA grid                    │  │  │  │
│  │  │  │    → persist lidar_bg_snapshot rows                │  │  │  │
│  │  │  │    → emit frame_stats → system_events              │  │  │  │
│  │  │  │    → gRPC stream → visualiser (port 50051)         │  │  │  │
│  │  │  └──────────┬──────────────────────┬──────────────────┘  │  │  │
│  │  │             │                      │                     │  │  │
│  │  └─────────────┼──────────────────────┼─────────────────────┘  │  │
│  │                │                      │                        │  │
│  │  ┌─────────────▼──────────────────────▼─────────────────────┐  │  │
│  │  │         SQLite Database (sensor_data.db)                 │  │  │
│  │  │         /var/lib/velocity-report/sensor_data.db          │  │  │
│  │  │                                                          │  │  │
│  │  │  Core Tables:                                            │  │  │
│  │  │  • radar_data (raw radar events, JSON)                   │  │  │
│  │  │  • lidar_bg_snapshot (background grid, BLOB)             │  │  │
│  │  │                                                          │  │  │
│  │  │  Transit/Object Tables (2 sources):                      │  │  │
│  │  │  • radar_objects (radar classifier detections)           │  │  │
│  │  │  • radar_data_transits (sessionized radar_data)          │  │  │
│  │  │                                                          │  │  │
│  │  │  Support Tables:                                         │  │  │
│  │  │  • radar_transit_links (radar_data ↔ transits)           │  │  │
│  │  │  • radar_commands / radar_command_log                    │  │  │
│  │  └─────────────┬────────────────────────────────────────────┘  │  │
│  │                │                                               │  │
│  │  ┌─────────────▼────────────────────────────────────────────┐  │  │
│  │  │         Background Workers                               │  │  │
│  │  │                                                          │  │  │
│  │  │  • Transit Worker: radar_data → radar_data_transits      │  │  │
│  │  │    (sessionises raw readings into vehicle transits)      │  │  │
│  │  │                                                          │  │  │
│  │  └─────────────┬────────────────────────────────────────────┘  │  │
│  │                │                                               │  │
│  │  ┌─────────────▼────────────────────────────────────────────┐  │  │
│  │  │         HTTP API Server (internal/api/)                  │  │  │
│  │  │         Listen: 0.0.0.0:8080                              │  │  │
│  │  │                                                          │  │  │
│  │  │  Endpoints:                                              │  │  │
│  │  │  • GET  /api/radar_stats (aggregated transit stats)      │  │  │
│  │  │  • GET  /api/config      (system config)                 │  │  │
│  │  │  • POST /command         (send radar command)            │  │  │
│  │  └──────────────────────────────────────────────────────────┘  │  │
│  │                                                                │  │
│  │  ┌──────────────────────────────────────────────────────────┐  │  │
│  │  │         gRPC Visualiser Server (internal/lidar/visual.)  │  │  │
│  │  │         Listen: 0.0.0.0:50051 (protobuf streaming)       │  │  │
│  │  │                                                          │  │  │
│  │  │  Modes:                                                  │  │  │
│  │  │  • Live: Stream real-time LiDAR frames                   │  │  │
│  │  │  • Replay: Stream recorded .vrlog files                  │  │  │
│  │  │  • Synthetic: Generate test data at configurable rate    │  │  │
│  │  │                                                          │  │  │
│  │  │  RPCs (VisualiserService):                               │  │  │
│  │  │  • StreamFrames   - Server-streaming FrameBundle         │  │  │
│  │  │  • Pause/Play     - Playback control (replay mode)       │  │  │
│  │  │  • Seek/SetRate   - Timeline navigation                  │  │  │
│  │  │  • Start/StopRecording - Record to .vrlog (live mode)    │  │  │
│  │  │  • GetCapabilities - Query server mode/features          │  │  │
│  │  └──────────────────────────────────────────────────────────┘  │  │
│  │                                                                │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
                                 │
           ┌─────────────────────┴─────────────────────────────┐
           │                                                   │
           │ HTTPS via nginx (port 443 → 8080)    gRPC (protobuf) │
           │                                          Port 50051 │
           │                                                   │
           ├───────────────────────┐                           │
           │                       │                           │
           ▼                       ▼                           ▼
┌─────────────────────┐ ┌─────────────────────┐ ┌─────────────────────┐
│    WEB PROJECT      │ │   PYTHON PROJECT    │ │  macOS VISUALISER   │
├─────────────────────┤ ├─────────────────────┤ ├─────────────────────┤
│  web/               │ │  tools/pdf-generator│ │  tools/visualiser-  │
│  Svelte Frontend    │ │  CLI Tools          │ │  macos/             │
│  • TypeScript       │ │  • create_config    │ │                     │
│  • Vite             │ │  • demo             │ │  Swift/SwiftUI App  │
│  • pnpm             │ │                     │ │  • Metal GPU render │
│                     │ │  Core Modules       │ │  • grpc-swift client│
│  API Client         │ │  • api_client       │ │                     │
│  • fetch/axios      │ │  • chart_builder    │ │  Features:          │
│                     │ │  • table_builders   │ │  • 3D point clouds  │
│                     │ │  • doc_builder      │ │  • Track box/trail  │
│                     │ │                     │ │  • Playback control │
│                     │ │  LaTeX Compiler     │ │  • Camera orbit/pan │
│                     │ │  • XeLaTeX          │ │  • Overlay toggles  │
│                     │ │  • matplotlib       │ │                     │
│                     │ │                     │ │  Modes:             │
│                     │ │  PDF Output         │ │  • Live streaming   │
│                     │ │  output/*.pdf       │ │  • Replay .vrlog    │
│                     │ │                     │ │  • Synthetic test   │
│  Runtime            │ │  Runtime            │ │                     │
│  • Dev: :5173       │ │  • CLI on demand    │ │  Runtime            │
│  • Prod: Go static  │ │  • Python 3.9+      │ │  • macOS 14+ (M1+)  │
│                     │ │  • Virtual env      │ │  • Metal GPU        │
└─────────────────────┘ └─────────────────────┘ └─────────────────────┘
```

## Components

### Go server

**Location**: `/cmd/`, `/internal/`

**Purpose**: Real-time data collection, storage, and API server

**Key Modules**:

- **[cmd/radar/](cmd/radar)** - Main server entry point
  - Sensor data collection (radar/LiDAR)
  - HTTP API server
  - Background task scheduler
  - Systemd service integration

- **[internal/api/](internal/api)** - HTTP API endpoints
  - `/api/radar_stats` - Statistical summaries and rollups
  - `/api/config` - Configuration retrieval
  - `/command` - Send radar commands
  - RESTful design with JSON responses

- **[internal/radar/](internal/radar)** - Radar sensor integration
  - Serial port communication
  - Data parsing and validation
  - Error handling and retry logic

- **[internal/lidar/](internal/lidar)** - LiDAR sensor integration
  - UDP packet listener and decoder (Hesai Pandar40P)
  - `FrameBuilder` accumulates complete 360° rotations with sequence checks
  - `BackgroundManager` maintains EMA grid (40 rings × 1800 azimuth bins)
  - Persists `lidar_bg_snapshot` rows and emits `frame_stats` into `system_events`
  - Tooling for ASC export, pose transforms, and background tuning APIs

- **[internal/lidar/sweep/](internal/lidar/sweep)** - Parameter sweep and tuning
  - `Runner`: runs combinatorial parameter sweeps (manual mode)
  - `AutoTuner`: iterative bounds-narrowing with proxy or ground truth scoring (auto mode)
  - `HINTTuner`: human-involved numerical tuning: creates reference runs, waits for human track labelling, then sweeps with ground truth scores (hint mode)

- **[internal/monitoring/](internal/monitoring)** - System monitoring
  - Health checks
  - Performance metrics
  - Error logging

- **[internal/units/](internal/units)** - Unit conversion
  - Speed conversions (MPH ↔ KPH)
  - Distance conversions
  - Timezone handling

**Runtime**: Deployed as systemd service on Raspberry Pi (ARM64 Linux)

**Communication**:

- **Input**:
  - Radar: Serial port data (/dev/ttyUSB0, USB connection)
  - LiDAR: Network/UDP packets (Ethernet connection, verified with LidarView/CloudCompare)
- **Output**:
  - HTTP API (JSON over port 8080, HTTPS via nginx on port 443)
  - SQLite database writes

### Python PDF generator (deprecated)

**Location**: `/tools/pdf-generator/`

**Purpose**: Generate professional PDF reports from sensor data.
Deprecated: PDF generation is moving into the Go server.

**Key Modules**:

- **`pdf_generator/cli/`** - Command-line tools
  - `create_config.py` - Interactive config generator
  - `demo.py` - Interactive demo/test tool

- **`pdf_generator/core/`** - Core functionality
  - `api_client.py` - HTTP API client
  - `chart_builder.py` - Matplotlib chart generation
  - `table_builders.py` - LaTeX table generation
  - `document_builder.py` - LaTeX document assembly
  - `date_parser.py` - Date/timezone parsing
  - `dependency_checker.py` - System requirements validation
  - `map_utils.py` - Map integration utilities

- **`pdf_generator/tests/`** - Test suite
  - Comprehensive test coverage
  - pytest + pytest-cov framework

**Runtime**: Command-line tool, invoked on-demand

**Communication**:

- **Input**: JSON config file, Go Server HTTP API
- **Output**: PDF files (via LaTeX → XeLaTeX → PDF)

**Dependencies**:

- Python 3.9+
- LaTeX distribution (XeLaTeX)
- matplotlib, PyLaTeX, requests

### Web frontend

**Location**: `/web/`

**Purpose**: Real-time data visualisation and interactive dashboards

**Key Technologies**:

- **Svelte** - Reactive UI framework
- **TypeScript** - Type-safe development
- **Vite** - Build tool and dev server
- **pnpm** - Package management

**Features**:

- Real-time chart updates
- Interactive data filtering
- Responsive design
- REST API integration

**Runtime**: Development server (Vite) or static build

**Communication**:

- **Input**: Go Server HTTP API (JSON)
- **Output**: HTML/CSS/JS served to browser

### macOS visualiser (swift/Metal)

**Location**: `/tools/visualiser-macos/`

**Purpose**:
Real-time 3D visualisation of LiDAR point clouds, object tracking, and debug overlays for M1+ Macs

**Technology Stack**:

- Swift 5.9+ with SwiftUI (macOS 14+)
- Metal for GPU-accelerated rendering
- grpc-swift for streaming communication
- XCTest for testing

**Completed Features (M0 + M1)**:

- ✅ SwiftUI app shell with window management
- ✅ Metal point cloud renderer (10,000+ points at 30fps)
- ✅ Instanced box renderer for tracks (AABB)
- ✅ Trail renderer with fading polylines
- ✅ gRPC client connecting to localhost:50051
- ✅ 3D camera controls (orbit, pan, zoom)
- ✅ Mouse/trackpad gesture support
- ✅ Pause/Play/Seek/SetRate playback controls
- ✅ Frame-by-frame navigation (step forward/back)
- ✅ Timeline scrubber with frame timestamps
- ✅ Playback rate adjustment (0.5x - 64x)
- ✅ Overlay toggles (show/hide tracks, trails, boxes)
- ✅ Deterministic replay of `.vrlog` recordings

**Go Backend** ([internal/lidar/l9endpoints/](internal/lidar/l9endpoints)):

- `grpc_server.go` - gRPC streaming server implementing VisualiserService
- `replay.go` - ReplayServer for streaming `.vrlog` files with seek/rate control
- `recorder/` - Record live frames to `.vrlog` format
- `synthetic.go` - Synthetic data generator for testing
- `adapter.go` - Convert pipeline data to FrameBundle proto
- `model.go` - Canonical FrameBundle data structures

**Swift Client**: [tools/visualiser-macos/VelocityVisualiser/](tools/visualiser-macos/VelocityVisualiser)

- `App/` - Application entry point and global state
- `gRPC/` - gRPC client wrapper and proto decoding
- `Rendering/` - Metal shaders and render pipeline
- `UI/` - SwiftUI views (playback controls, overlays, inspector)
- `Models/` - Swift data models (Track, Cluster, PointCloud)

**Command-Line Tools**:

- [cmd/tools/visualiser-server](cmd/tools/visualiser-server) - Multi-mode server (synthetic/replay/live)
- [cmd/tools/gen-vrlog](cmd/tools/gen-vrlog) - Generate sample `.vrlog` recordings

**Protocol Buffer Schema**: [proto/velocity_visualiser/v1/visualiser.proto](proto/velocity_visualiser/v1/visualiser.proto)

```protobuf
service VisualiserService {
  rpc StreamFrames(StreamRequest) returns (stream FrameBundle);
  rpc Pause(PauseRequest) returns (PlaybackStatus);
  rpc Play(PlayRequest) returns (PlaybackStatus);
  rpc Seek(SeekRequest) returns (PlaybackStatus);
  rpc SetRate(SetRateRequest) returns (PlaybackStatus);
  rpc SetOverlayModes(OverlayModeRequest) returns (OverlayModeResponse);
  rpc GetCapabilities(CapabilitiesRequest) returns (CapabilitiesResponse);
  rpc StartRecording(RecordingRequest) returns (RecordingStatus);
  rpc StopRecording(RecordingRequest) returns (RecordingStatus);
}
```

**Communication**:

- **Input**: gRPC streaming (localhost:50051)
- **Output**: User interactions, label annotations (planned)

**See**: [tools/visualiser-macos/README.md](tools/visualiser-macos/README.md) and
[docs/ui/](docs/ui/)

### Database layer

**Location**: `./sensor_data.db`, managed by [internal/db/](internal/db) <!-- link-ignore -->

**Database**: SQLite 3.51.2 (via `modernc.org/sqlite v1.44.3`)

**Schema Design**:

The database uses a **JSON-first approach** with generated columns for performance.
Raw sensor events are stored as JSON,
with frequently-queried fields extracted to indexed columns automatically.

**Example - `radar_data` table**:

```sql
CREATE TABLE radar_data (
    write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec')),
    raw_event JSON NOT NULL,
    -- Generated columns (automatically extracted from JSON)
    uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED,
    magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED,
    speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
);
```

When a sensor reading arrives, the Go server stores the entire event as JSON in `raw_event`,
and SQLite automatically populates the generated columns. This provides:

- **Flexibility**: Complete event data preserved for future analysis
- **Performance**: Fast indexed queries on common fields (speed, timestamp)
- **Schema evolution**: New fields can be added without migration

**Key Tables**:

- `radar_data` - Raw radar readings (JSON events with speed/magnitude)
- `radar_objects` - Classified transits from radar's onboard classifier
- `radar_data_transits` - Sessionized transits built by transit worker from `radar_data`
- `radar_transit_links` - Many-to-many links between transits and raw radar_data
- `lidar_bg_snapshot` - LiDAR background grid for motion detection (40×1800 range-image)
- `lidar_objects` - Track-extracted transits from LiDAR processing [PLANNED]
- `radar_commands` / `radar_command_log` - Command history and execution logs
- `site` - Site metadata (location, speed limits)
- `site_config_periods` - Time-based sensor configuration (cosine error angle history)

**Transit Sources** (3 independent object detection pipelines):

1. **radar_objects**: Hardware classifier in OPS243 radar sensor
2. **radar_data_transits**: Software sessionization of raw radar_data points
3. **lidar_objects**: Software tracking from LiDAR point clouds [PLANNED]

These three sources will be compared for initial reporting, with eventual goal of:

- FFT-based radar processing for improved object segmentation
- Sensor fusion using LiDAR data to assist radar object detection

**Key Features**:

- High-precision timestamps (DOUBLE for subsecond accuracy via `UNIXEPOCH('subsec')`)
- Sessionization via `radar_data_transits` (avoids expensive CTEs in queries)
- LiDAR background modeling for change detection (grid stored as BLOB)
- WAL mode enabled for concurrent readers/writers
- Indexes on timestamp columns for fast time-range queries
- Time-based site configuration via `site_config_periods` (Type 6 Slowly Changing Dimension)

**Site Configuration Periods**:

The `site_config_periods` table implements a Type 6 SCD pattern for tracking sensor configuration
changes over time. Key aspects:

- **Cosine error correction**: Radar mounted at an angle measures lower speeds than actual. Each period stores the mounting angle for automatic correction.
- **Non-overlapping periods**: Database triggers enforce that periods for the same site cannot overlap.
- **Active period tracking**: One period per site is marked `is_active = 1` for new data collection.
- **Retroactive corrections**: Changing a period's angle automatically affects all reports querying that time range.
- **Comparison report accuracy**: When comparing periods with different configurations, each period's data is corrected independently.

**Migrations**: Located in `/internal/db/migrations/`, managed by Go server

**Access Patterns**:

- **Go Server**: Read/Write
  - Real-time inserts to `radar_data` (raw radar events)
  - Hardware classifier → `radar_objects`
  - Transit worker → sessionize `radar_data` → `radar_data_transits`
  - LiDAR background grid → `lidar_bg_snapshot`
  - [PLANNED] LiDAR tracking → `lidar_objects`
- **Python PDF Generator** (deprecated): Read-only (via HTTP API)
  - Queries transit data from 3 sources for comparison reports
  - Aggregates statistics across detection methods
- **Web Frontend**: Read-only (via HTTP API)
  - Real-time dashboard showing all 3 transit sources

## Perception pipeline

The LiDAR perception stack runs layers L3 through L6 on every 10 Hz frame:
background subtraction, clustering, tracking, and classification.
Each layer is a separate Go package under [internal/lidar/](internal/lidar),
with its own parameters, tests, and maths reference. The pipeline aims to process
~70,000 points per frame on a Raspberry Pi 4 with no cloud dependency.

### L3: background model

The background model separates static scene (road surface, buildings,
vegetation) from moving objects.
It maintains a 40 × 3,600 polar grid (one row per LiDAR beam,
one column per 0.1° azimuth bin) where each cell tracks an exponentially weighted moving average of
range values and a Welford online variance estimate.

Cells are classified into three adaptive region types: **stable** (pavement, walls:
low variance, tight foreground threshold), **variable** (parked cars, street furniture:
moderate variance, relaxed threshold), and **volatile** (trees, reflective surfaces:
high variance, wide threshold).
The classification adapts per cell,
so a parking space that empties mid-session reclassifies automatically.
A point counts as foreground when its range deviates from the cell's background mean by more than a
threshold scaled to the cell's region type.

The grid settles over a configurable number of frames.
Until a cell has seen enough observations,
it remains unsettled and does not contribute to foreground extraction,
which prevents the first vehicle through the scene from becoming part of the background.

### L4: clustering and geometry

Foreground points are grouped into spatial clusters using DBSCAN with a grid-accelerated spatial
index. The index maps each point to a cell via a Szudzik pairing function on signed grid
coordinates, making neighbourhood queries O(1) per point instead of O(n).
Clusters are filtered by size, aspect ratio, and point count to reject noise and scene artefacts.

Each cluster gets an oriented bounding box (OBB) fitted via 2D PCA on its ground-plane projection.
PCA alone is ambiguous (the eigenvectors can flip 180° or swap axes between frames),
so the pipeline applies heading disambiguation guards:
aspect-ratio locking for near-square clusters,
90° jump rejection against the previous frame's heading,
and EMA temporal smoothing (α = 0.08) to absorb jitter without lagging real turns.

Ground-plane points within each cluster are removed using a local height threshold relative to the
cluster's lowest points.

### L5: multi-object tracking

Tracking follows the predict–associate–update loop.
Each track maintains a constant-velocity Kalman filter with state vector `[X, Y, VX, VY]` and a 4 ×
4 covariance matrix.
The motion model assumes constant velocity between frames,
simple enough to run at 10 Hz on constrained hardware,
accurate enough for urban traffic where vehicles rarely accelerate hard between 100 ms frames.

Association uses the Hungarian algorithm (Kuhn–Munkres) with Mahalanobis distance as the cost
metric. Three gating guards reject implausible assignments before they reach the solver:
a Euclidean position jump limit (5 m), an implied speed limit (30 m/s),
and a Mahalanobis distance² threshold (36).
Unmatched detections spawn tentative tracks;
unmatched tracks enter a coasting window where the Kalman prediction runs without measurement
updates.

Track lifecycle:
a new track is **tentative** until it accumulates 4 consecutive hits, then **confirmed**.
A confirmed track tolerates up to 15 consecutive misses (coasting through brief occlusions) before
deletion. Tentative tracks are deleted after 3 misses.
Covariance inflates progressively during coasting,
so a coasted track's association gate widens naturally:
it accepts a returning detection at greater distance but with lower confidence.

### L6: classification

Each confirmed track is classified using a rule-based system (v1.2) that evaluates spatial and
kinematic features: bounding box dimensions, aspect ratio, speed, point count, and height profile.
The classifier assigns one of eight object types:
car, truck, bus, pedestrian, cyclist, motorcyclist, bird, and dynamic (unclassified moving object),
with confidence levels at three tiers: high (0.85), medium (0.70), and low (0.50).

The rule set uses threshold ranges derived from measured vehicle dimensions and typical urban
speeds. Truck and motorcyclist labels are currently display-only:
visible in the visualiser and VRLOG replay but not selectable in the labelling UI,
pending wider validation data.
The `ClassDynamic` label catches moving objects that do not match any specific profile,
useful for flagging edge cases rather than forcing a wrong classification.

The classification is deliberately rule-based rather than learned.
With single-digit labelled sessions, a trained classifier would overfit;
the rule set is transparent, tuneable,
and correct enough to structure the data for future ML work when the ground truth corpus is larger.

## Data flow

### Real-time data collection

```
Radar (Serial):
1. OPS243 radar → USB-Serial (/dev/ttyUSB0)
2. internal/radar/ reader parses JSON speed/magnitude payloads
3. INSERT raw packets into `radar_data`; hardware detections → `radar_objects`

LiDAR (Network/UDP):
1. Hesai P40 → Ethernet/UDP (192.168.100.202 → 192.168.100.151 listener)
2. Packet decoder reconstructs blocks → `FrameBuilder` completes 360° rotations
3. `BackgroundManager` updates EMA background grid (40 × 1800 cells)
4. Persist snapshots → INSERT/UPSERT `lidar_bg_snapshot`
5. Emit frame statistics and performance metrics → `system_events`
6. [Optional] Stream FrameBundle → gRPC visualiser clients (port 50051)

Transit Worker (Background Process):
1. Query recent radar_data points → Sessionisation algorithm
2. Group readings into vehicle transits (time-gap based)
3. INSERT/UPDATE radar_data_transits
4. Link raw data → INSERT INTO radar_transit_links

Three Transit Sources:
• radar_objects        (radar hardware classifier)
• radar_data_transits  (software sessionisation)
• lidar_objects        (LiDAR tracking) [PLANNED]
```

### PDF report generation

```
1. User → Run create_config.py → Generate config.json
2. User → Run demo.py with config.json
3. demo.py → api_client.py → HTTP GET /api/radar_stats → Go Server
4. Go Server → Query SQLite → Return JSON
5. api_client.py → Parse JSON → Pass to chart_builder, table_builders
6. chart_builder → matplotlib → PNG charts
7. table_builders → PyLaTeX → LaTeX tables
8. document_builder → Combine → LaTeX document
9. XeLaTeX → Compile → PDF output
```

### Web visualisation

```
1. User → Open browser → Vite dev server (or static build)
2. Frontend → Fetch /api/radar_stats → Go Server
3. Go Server → Query SQLite → Return JSON
4. Frontend → Parse JSON → Render Svelte components
5. Frontend → Display charts/tables → Browser DOM
```

### macOS LiDAR visualisation

```
Live Mode:
1. LiDAR sensor → UDP packets → Go Server → FrameBuilder
2. FrameBuilder → Tracker → Adapter → FrameBundle proto
3. gRPC Server (port 50051) → StreamFrames RPC → Swift client
4. Swift client → Decode proto → Metal renderer
5. User ← 3D point clouds + tracks + trails

Replay Mode:
1. User → Select .vrlog file → Go ReplayServer
2. ReplayServer → Read frames from disk → gRPC stream
3. User → Pause/Play/Seek/SetRate → Control RPCs
4. ReplayServer → Adjust playback → Stream at selected rate
5. Swift client → Render frames → Frame-by-frame navigation

Synthetic Mode (Testing):
1. Go SyntheticGenerator → Generate rotating points + moving boxes
2. gRPC Server → StreamFrames → Swift client
3. Swift client → Render synthetic data → Validate pipeline
```

## Technology stack

### Go server

| Component  | Technology              | Version | Purpose                 |
| ---------- | ----------------------- | ------- | ----------------------- |
| Language   | Go                      | 1.25+   | High-performance server |
| Database   | SQLite                  | 3.51    | Data storage            |
| HTTP       | net/http (stdlib)       | -       | API server              |
| gRPC       | google.golang.org/grpc  | 1.60+   | Visualiser streaming    |
| Protobuf   | google.golang.org/proto | 1.32+   | Data serialisation      |
| Serial     | github.com/tarm/serial  | -       | Sensor communication    |
| Deployment | systemd                 | -       | Service management      |
| Build      | Make                    | -       | Build automation        |

### Python PDF generator (deprecated)

| Component      | Technology | Version | Purpose             |
| -------------- | ---------- | ------- | ------------------- |
| Language       | Python     | 3.11+   | Report generation   |
| Charts         | matplotlib | 3.9+    | Data visualisation  |
| LaTeX          | PyLaTeX    | 1.4+    | Document generation |
| HTTP           | requests   | 2.32+   | API client          |
| Testing        | pytest     | 8.4+    | Test framework      |
| Coverage       | pytest-cov | 7.0+    | Coverage reporting  |
| LaTeX Compiler | XeLaTeX    | -       | PDF compilation     |

### Web frontend

| Component       | Technology | Version | Purpose               |
| --------------- | ---------- | ------- | --------------------- |
| Framework       | Svelte     | 5.x     | Reactive UI           |
| Language        | TypeScript | 5.x     | Type safety           |
| Build Tool      | Vite       | 6.x     | Dev server & bundling |
| Package Manager | pnpm       | 9.x     | Dependency management |
| Linting         | ESLint     | 9.x     | Code quality          |

### macOS visualiser

| Component     | Technology | Version  | Purpose              |
| ------------- | ---------- | -------- | -------------------- |
| Language      | Swift      | 5.9+     | Native macOS app     |
| UI Framework  | SwiftUI    | macOS 14 | Declarative UI       |
| GPU Rendering | Metal      | 3.x      | 3D point clouds      |
| gRPC Client   | grpc-swift | 1.23+    | Server communication |
| Testing       | XCTest     | -        | Unit tests           |
| Build         | Xcode      | 15+      | IDE & build system   |

## Integration points

### Go server ↔ SQLite

**Interface**: Go database/sql with SQLite driver

**Operations**:

- INSERT `radar_data` (real-time writes with JSON events)
- INSERT `radar_objects` (classified detections)
- Background sessionisation: query `radar_data` → insert/update `radar_data_transits`
- LiDAR background modelling: update `lidar_bg_snapshot`
- SELECT for API queries (read optimised with generated columns)

**Performance Considerations**:

- JSON storage with generated columns for fast indexed queries
- Indexes on timestamp columns (`transit_start_unix`, `transit_end_unix`)
- Batched inserts for high-frequency sensors
- WAL mode for concurrent reads during writes
- Subsecond timestamp precision (DOUBLE type)

### Go server ↔ Python PDF generator (deprecated)

**Interface**: HTTP REST API (JSON)

**Endpoints**:

```
GET /api/radar_stats?start=<unix>&end=<unix>&group=<15m|1h|24h>&source=<radar_objects|radar_data_transits>
Response: {
  "metrics": [
    {
      "start_time": "2025-01-01T00:00:00Z",
      "count": 1234,
      "max_speed": 45.2,
      "p50_speed": 28.5,
      "p85_speed": 32.1,
      "p98_speed": 38.4
    },
    ...
  ],
  "histogram": {
    "0.00": 10,
    "5.00": 25,
    ...
  }
}

**Percentile policy**: `p50_speed`, `p85_speed`, and `p98_speed` are
**aggregate** percentiles computed over a population of vehicle max speeds
within each time bucket. Speed percentiles are never computed on a single
track's observations. The core high-speed metric is **p98**: the speed
exceeded by only 2% of observed vehicles, which requires at least 50
observations to be statistically meaningful.

GET /api/config
Response: {
  "units": "mph",
  "timezone": "America/Los_Angeles"
}

GET /events
Response: [
  {
    "uptime": 12345.67,
    "magnitude": 3456,
    "speed": 28.5,
    "direction": "inbound",
    "raw_json": "{...}"
  },
  ...
]
```

**Query Parameters**:

- `start`, `end`: Unix timestamps (seconds)
- `group`: Time bucket size (`15m`, `30m`, `1h`, `2h`, `3h`, `4h`, `6h`, `8h`, `12h`, `24h`, `all`, `2d`, `3d`, `7d`, `14d`, `28d`)
- `source`: Data source (`radar_objects` or `radar_data_transits`)
- `model_version`: Transit model version (when using `radar_data_transits`)
- `min_speed`: Minimum speed filter (in display units)
- `units`: Override units (`mph`, `kph`, `mps`)
- `timezone`: Override timezone
- `compute_histogram`: Enable histogram computation (`true`/`false`)
- `hist_bucket_size`: Histogram bucket size
- `hist_max`: Histogram maximum value

**Error Handling**:

- HTTP 200: Success
- HTTP 400: Invalid parameters
- HTTP 405: Method not allowed
- HTTP 500: Server error
- Python client retries with exponential backoff

### Go server ↔ web frontend

**Interface**: HTTP REST API (JSON) + Static file serving

**Same API as Python**, plus:

- Static file serving for Svelte build (`/app/*`)
- SPA routing with fallback to `index.html`
- Favicon serving
- Root redirect to `/app/`

### Go server ↔ macOS visualiser

**Interface**: gRPC streaming over protobuf (port 50051)

**Protocol**: `velocity_visualiser.v1.VisualiserService`

**Streaming RPC**:

```protobuf
// Server-streaming: continuous frame delivery
rpc StreamFrames(StreamRequest) returns (stream FrameBundle);
```

**Control RPCs** (replay mode):

```protobuf
rpc Pause(PauseRequest) returns (PlaybackStatus);
rpc Play(PlayRequest) returns (PlaybackStatus);
rpc Seek(SeekRequest) returns (PlaybackStatus);   // Seek to frame index
rpc SetRate(SetRateRequest) returns (PlaybackStatus);  // 0.5x - 64x
rpc GetCapabilities(CapabilitiesRequest) returns (CapabilitiesResponse);
```

**Recording RPCs** (live mode):

```protobuf
rpc StartRecording(RecordingRequest) returns (RecordingStatus);
rpc StopRecording(RecordingRequest) returns (RecordingStatus);
```

**FrameBundle Contents**:

- `frame_id` - Unique frame identifier
- `timestamp` - Frame timestamp (nanoseconds)
- `point_cloud` - XYZ points with intensity (up to 70,000 per frame)
- `tracks` - Active tracked objects with bounding boxes
- `clusters` - Raw cluster data (optional)
- `playback_info` - Current frame index, total frames, rate, paused state

**Server Modes**:

- **Live**: Stream real-time LiDAR data, record to `.vrlog`
- **Replay**: Stream recorded `.vrlog` files with playback control
- **Synthetic**: Generate test data for development

**Performance**:

- Frame rate: 10-20 Hz (configurable)
- Points per frame: Up to 70,000
- Tracks per frame: Up to 200
- Latency: < 50ms end-to-end

### Python ↔ LaTeX

**Interface**: PyLaTeX → XeLaTeX subprocess

**Process**:

1. PyLaTeX generates `.tex` file
2. `subprocess.run(['xelatex', ...])` compiles
3. Retry logic for LaTeX errors
4. Cleanup of intermediate files (`.aux`, `.log`)

## Data & evaluation

### VRLOG recording format

LiDAR sessions are recorded in the VRLOG format (v0.5),
a seekable binary container for point clouds, tracks, and metadata.
A recording is a directory containing chunked data files and a binary index.

Each index entry is 24 bytes:
an 8-byte frame ID, an 8-byte nanosecond timestamp, a 4-byte chunk ID,
and a 4-byte offset within the chunk.
The index is sorted by frame ID, enabling binary search for random access:
the visualiser can seek to any frame in a multi-hour recording without scanning the data files.
Chunks rotate at 1,000 frames or 150 MB, whichever comes first.

VRLOG recordings are the primary unit of reproducible work.
Every parameter sweep, every labelling session, and every evaluation run operates on a VRLOG file,
so results are deterministic and reviewable.

### Track labelling and ground truth

The labelling workflow produces ground truth for parameter evaluation.
A human reviewer watches a VRLOG replay in the macOS visualiser or Svelte frontend,
marks each track as correctly detected, fragmented, false positive, or missed,
and annotates the object type.
Labels are stored alongside the recording and versioned with the run that produced them.

The label vocabulary distinguishes selectable labels (what a reviewer can assign) from display
labels (what the classifier can output).
This separation lets the classifier report fine-grained types like truck and motorcyclist without
requiring reviewers to distinguish them reliably at LiDAR resolution,
an honest acknowledgement that some categories are easier to classify than to label.

### HINT parameter tuner

HINT (Human-Involved Numerical Tuning) closes the loop between perception quality and parameter
selection. The workflow:

1. **Reference run**: process a VRLOG recording with current parameters to produce tracks.
2. **Label**: a human labels the tracks (correct, fragmented, false positive, missed).
3. **Sweep**: the tuner replays the same recording across a grid of parameter combinations, scoring each against the labelled ground truth.
4. **Select**: the combination with the best composite score becomes the new parameter set.

The scoring function is a weighted linear combination of detection quality metrics:
acceptance rate, track fragmentation, false positive rate, heading jitter, speed jitter,
and foreground capture ratio, among others.
Weights are configurable;
the defaults penalise fragmentation heavily (a vehicle that splits into three tracks is worse than
one that is slightly misaligned) and reward detection rate.

HINT is not automated optimisation: the human labelling step is deliberate.
At the current data scale,
a reviewer who watches the replay catches failure modes that no metric captures:
a track that technically scores well but visually drifts through a wall,
or a cluster that fragments because the parameters are tuned for a different scene geometry.
The human stays in the loop until the ground truth corpus is large enough to trust automated
evaluation.

## Deployment architecture

### Production environment (Raspberry Pi)

```
┌────────────────────────────────────────────────┐
│         Raspberry Pi 4 (ARM64, Linux)          │
│                                                │
│  ┌──────────────────────────────────────────┐  │
│  │  systemd (velocity-report.service)       │  │
│  │  ↓                                       │  │
│  │  /usr/local/bin/velocity-report          │  │
│  │  --db-path /var/lib/velocity-report/...  │  │
│  │  (Go Server Binary)                      │  │
│  │                                          │  │
│  │  Configuration:                          │  │
│  │  • --listen :8080                         │  │
│  │  • --db-path (explicit SQLite location)  │  │
│  │  • WorkingDirectory=/var/lib/velocity... │  │
│  └──────────────────────────────────────────┘  │
│                                                │
│  ┌──────────────────────────────────────────┐  │
│  │  SQLite Database                         │  │
│  │  /var/lib/velocity-report/sensor_data.db │  │
│  └──────────────────────────────────────────┘  │
│                                                │
│  Sensor Connections:                           │
│  • /dev/ttyUSB0 (Radar - Serial)               │
│  • Network/UDP (LiDAR - Ethernet)              │
└────────────────────────────────────────────────┘
```

**Service Management**:

```sh
sudo systemctl status velocity-report.service
sudo systemctl restart velocity-report.service
sudo journalctl -u velocity-report.service -f
```

### Development environment

```
Developer Machine (macOS/Linux/Windows)

Go Development:
• ./app-local -dev (local build)
• Mock sensors or test data

Python Development:
• tools/pdf-generator/ (PYTHONPATH method)
• make pdf-test (532 tests)
• make pdf-demo (interactive testing)

Web Development:
• pnpm dev (Vite dev server)
• http://localhost:5173
• Hot module reloading
```

## Security & privacy

### Privacy guarantees

✅ **No licence plate recognition**

- Sensors measure speed only, no cameras

✅ **No video recording**

- Pure time-series data (timestamp + speed)

✅ **No personally identifiable information**

- No tracking of individual vehicles
- Aggregate statistics only

✅ **Local storage**

- All data stored locally on device
- No cloud uploads unless explicitly configured

### Security considerations

**API Access**:

- Currently no authentication (local network only)
- **TODO**: Add API key authentication for production deployments

**Database**:

- File permissions restrict access to service user
- No sensitive data stored (speed + timestamp only)

**Network**:

- Designed for local network operation
- Can be exposed via reverse proxy if needed
- HTTPS support via reverse proxy (nginx, caddy)

**Updates**:

- Manual deployment process
- Systemd service restart required
- **TODO**: Add automatic update mechanism

## Component status

### What ships today

| Capability                        | Status       | Component    |
| --------------------------------- | ------------ | ------------ |
| Radar vehicle detection (OPS243A) | Production   | Go server    |
| Real-time speed dashboard         | Production   | Svelte web   |
| Professional PDF reports          | Deprecated   | Python/LaTeX |
| Comparison reports (before/after) | Deprecated   | Go + Python  |
| Site configuration (SCD Type 6)   | Production   | Go + SQLite  |
| LiDAR background subtraction      | Experimental | Go server    |
| LiDAR foreground tracking         | Experimental | Go server    |
| Adaptive region segmentation      | Experimental | Go server    |
| Parameter sweep / auto-tune       | Experimental | Go server    |
| PCAP analysis mode                | Experimental | Go server    |
| macOS 3D visualiser (Metal)       | Experimental | Swift app    |
| Track labelling + VRLOG replay    | Experimental | Swift + Go   |

### LiDAR pipeline layers

The perception pipeline is organised as ten layers (L1–L10),
each a distinct Go package under [internal/lidar/](internal/lidar).
Layers L1–L6 form a complete stack from raw UDP frames to classified objects:
DBSCAN clustering, Kalman-filtered tracking with Hungarian assignment,
and rule-based classification, all tuneable and inspectable end to end.

| Layer | Package         | Capability                                                                      | Status         |
| ----- | --------------- | ------------------------------------------------------------------------------- | -------------- |
| L1    | `l1packets/`    | Sensor ingest (Hesai Pandar40P UDP, PCAP replay)                                | ✅ Implemented |
| L2    | `l2frames/`     | Frame assembly, rotation geometry, coordinate transforms                        | ✅ Implemented |
| L3    | `l3grid/`       | Background/foreground separation (EMA grid, Welford variance, adaptive regions) | ✅ Implemented |
| L4    | `l4perception/` | DBSCAN clustering, oriented bounding boxes via PCA, ground-plane removal        | ✅ Implemented |
| L5    | `l5tracks/`     | Kalman-filtered multi-object tracking, Hungarian assignment, occlusion coasting | ✅ Implemented |
| L6    | `l6objects/`    | Rule-based classification (8 object types), feature extraction                  | ✅ Implemented |
| L7    | `l7scene/`      | Persistent world model, multi-sensor fusion                                     | Planned (v1.0) |
| L8    | `l8analytics/`  | Run statistics, track labelling, sweep evaluation                               | ✅ Implemented |
| L9    | `l9endpoints/`  | gRPC frame streaming, HTTP API, chart rendering                                 | ✅ Implemented |
| L10   | Clients         | macOS visualiser (Metal), Svelte frontend, Python PDF (deprecated)              | ✅ Implemented |

Canonical layer reference:
[lidar-data-layer-model.md](docs/lidar/architecture/lidar-data-layer-model.md)

### LiDAR capability roadmap

| Capability                                               | Status         | Target | Plan                                                               |
| -------------------------------------------------------- | -------------- | ------ | ------------------------------------------------------------------ |
| Analysis run infrastructure (versioned runs, comparison) | ✅ Implemented | v0.5.0 | [plan](docs/plans/lidar-analysis-run-infrastructure-plan.md)       |
| Tracking upgrades (Hungarian, OBB, ground removal)       | ✅ Implemented | v0.5.0 | -                                                                  |
| Adaptive regions & HINT parameter tuner                  | ✅ Implemented | v0.5.0 | [plan](docs/plans/lidar-sweep-hint-mode-plan.md)                   |
| Track labelling (backend, API, Svelte + macOS UI)        | Experimental   | v0.5.2 | [plan](docs/plans/lidar-track-labelling-auto-aware-tuning-plan.md) |
| ML classifier benchmarking (optional research lane)      | Deferred       | v2.0   | [plan](docs/plans/lidar-ml-classifier-training-plan.md)            |
| Automated hyperparameter search                          | Planned        | v2.0   | [plan](docs/plans/lidar-parameter-tuning-optimisation-plan.md)     |
| Production LiDAR deployment                              | Planned        | -      | -                                                                  |

## Performance characteristics

### Go server

- **Throughput**: 1000+ readings/second (tested)
- **Memory**: ~50MB typical, ~100MB peak
- **CPU**: <5% on Raspberry Pi 4 (idle), <20% during aggregation
- **Storage**: ~1MB per 10,000 readings (compressed)

### Python PDF generator (deprecated)

- **Execution Time**:
  - Config generation: <1 second
  - PDF generation: 10-30 seconds (depends on data volume)
  - LaTeX compilation: 3-5 seconds
- **Memory**: ~200MB peak (matplotlib rendering)
- **Disk**: ~1MB per PDF, ~5MB temp files during generation

### Web frontend

- **Bundle Size**: ~150KB (gzipped)
- **Load Time**: <1 second (local network)
- **API Latency**: <100ms typical
- **Rendering**: 60fps on modern browsers

## Roadmap

Development is tracked in the [backlog](docs/BACKLOG.md), organised by version.
The versions most relevant to system architecture:

| Version | Theme                  | Key Capabilities                                                                                             |
| ------- | ---------------------- | ------------------------------------------------------------------------------------------------------------ |
| v0.5.2  | Data Contracts         | Track speed metric redesign, metric registry, data structure remediation, replay case terminology            |
| v0.5.3  | Replay Stabilisation   | VRLOG timestamp index, SSE backpressure, visualiser debug overlays, dynamic background segmentation          |
| v0.5.4  | Product Polish         | Serial port configuration UI, frontend theme compliance, metrics consolidation                               |
| v0.6.0  | Deployment & Packaging | Raspberry Pi image pipeline, single `velocity-report` binary, one-line installer, geometry-coherent tracking |
| v0.7.0  | United Frontend        | Svelte migration (retire Go-embedded dashboards), ECharts → LayerChart, track labelling UI in Swift          |
| v1.0    | Scene Layer            | L7 persistent world model, multi-sensor fusion (radar + LiDAR cross-sensor track handoff)                    |
| v2.0    | ML & Automation        | ML classifier benchmarking, automated hyperparameter search, velocity-coherent foreground extraction         |

The project ships incrementally.
Each version has a design document per work item; the backlog links to all of them.

## Mathematical references

The perception algorithms are documented in standalone mathematical references
under [data/maths/](data/maths). Each document covers the theory, parameter choices,
and implementation mapping for one pipeline stage.

| Document                                                                          | Pipeline Layer | Content                                                                                    |
| --------------------------------------------------------------------------------- | -------------- | ------------------------------------------------------------------------------------------ |
| [background-grid-settling-maths.md](data/maths/background-grid-settling-maths.md) | L3 Grid        | EMA/EWA update equations, settling state machine, freeze logic                             |
| [ground-plane-maths.md](data/maths/ground-plane-maths.md)                         | L3–L4          | Tile-based ground plane estimation, confidence scoring, curvature handling                 |
| [clustering-maths.md](data/maths/clustering-maths.md)                             | L4 Perception  | DBSCAN parameters, OBB via PCA, geometry extraction                                        |
| [tracking-maths.md](data/maths/tracking-maths.md)                                 | L5 Tracks      | Constant-velocity Kalman filter, Mahalanobis gating, Hungarian assignment, track stability |
| [classification-maths.md](data/maths/classification-maths.md)                     | L6 Objects     | Rule-based classifier thresholds, feature definitions                                      |
| [pipeline-review-open-questions.md](data/maths/pipeline-review-open-questions.md) | L1–L6          | Cross-layer dependency audit, open design questions                                        |

The parameter configuration reference ([config/README.maths.md](config/README.maths.md)) maps every
leaf path in `tuning.defaults.json` to the mathematical document that explains it.

Key references from the literature: Kalman (1960), Munkres (1957), Ester et al.
DBSCAN (1996), Bewley et al. SORT (2016), Weng et al. AB3DMOT (2020).
Full citations in [references.bib](data/maths/references.bib).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development workflows, testing requirements,
and contribution guidelines.

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.
