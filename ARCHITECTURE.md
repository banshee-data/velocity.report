# velocity.report Architecture

This document describes the system architecture, component relationships, data flow, and integration points for the velocity.report traffic monitoring system.

## Table of Contents

- [System Overview](#system-overview)
- [Architecture Diagram](#architecture-diagram)
- [Components](#components)
- [Data Flow](#data-flow)
- [Technology Stack](#technology-stack)
- [Integration Points](#integration-points)
- [Deployment Architecture](#deployment-architecture)
- [Security & Privacy](#security--privacy)

## System Overview

**velocity.report** is a distributed system for neighborhood traffic monitoring with four main components:

1. **Go Server** - Real-time data collection and HTTP API
2. **Python PDF Generator** - Professional report generation with LaTeX
3. **Web Frontend** - Real-time visualisation (Svelte/TypeScript)
4. **macOS Visualiser** - Native 3D LiDAR visualisation (Swift/Metal)

All components share a common SQLite database as the single source of truth, with the macOS visualiser receiving real-time data via gRPC streaming.

### Design Principles

- **Privacy First**: No license plates, no video, no PII
- **Simplicity**: SQLite as the only database, minimal dependencies
- **Offline-First**: Works without internet connectivity
- **Modular**: Each component operates independently
- **Well-Tested**: Comprehensive test coverage across all components

## Architecture Diagram

### Physical Deployment

```
┌──────────────────────────────────────────────────────────────────────┐
│                        HARDWARE INFRASTRUCTURE                       │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌────────────────────┐                 ┌────────────────────┐       │
│  │  Radar Sensor      │                 │  LIDAR Sensor      │       │
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
│  │  • Ethernet Port (LIDAR + Network)                             │  │
│  │    - LIDAR network: 192.168.100.151/24 (listener)              │  │
│  │    - LIDAR sensor:  192.168.100.202 (UDP source)               │  │
│  │    - Local LAN:     192.168.1.x (API server)                   │  │
│  │                                                                │  │
│  │  Network: Dual configuration (LIDAR subnet + Local LAN)        │  │
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
│  │  │  │  │ Radar Handler    │  │ LIDAR Handler        │    │  │  │  │
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
│  │  │  │  Sensor Pipelines → SQLite                         │  │  │  │
│  │  │  │                                                    │  │  │  │
│  │  │  │  Radar Serial (/dev/ttyUSB0)                       │  │  │  │
│  │  │  │    → ops243 reader → JSON parse                    │  │  │  │
│  │  │  │    → INSERT radar_data, radar_objects              │  │  │  │
│  │  │  │                                                    │  │  │  │
│  │  │  │  LIDAR Ethernet (Hesai UDP 192.168.100.202)        │  │  │  │
│  │  │  │    → packet decoder → FrameBuilder rotations       │  │  │  │
│  │  │  │    → BackgroundManager EMA grid                    │  │  │  │
│  │  │  │    → persist lidar_bg_snapshot rows                │  │  │  │
│  │  │  │    → emit frame_stats → system_events              │  │  │  │
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
│  │  │    (sessionizes raw readings into vehicle transits)      │  │  │
│  │  │                                                          │  │  │
│  │  └─────────────┬────────────────────────────────────────────┘  │  │
│  │                │                                               │  │
│  │  ┌─────────────▼────────────────────────────────────────────┐  │  │
│  │  │         HTTP API Server (internal/api/)                  │  │  │
│  │  │         Listen: 0.0.0.0:8080                             │  │  │
│  │  │                                                          │  │  │
│  │  │  Endpoints:                                              │  │  │
│  │  │  • GET  /api/radar_stats (aggregated transit stats)      │  │  │
│  │  │  • GET  /api/config      (system config)                 │  │  │
│  │  │  • POST /command         (send radar command)            │  │  │
│  │  └──────────────────────────────────────────────────────────┘  │  │
│  │                                                                │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
                                 │
                                 │ HTTP API (JSON)
                                 │ Local Network (192.168.1.x:8080)
                                 │
        ┌────────────────────────┴─────────────┐
        │                                      │
        │                                      │
┌───────▼───────────────────────┐      ┌───────▼───────────────────────┐
│         WEB PROJECT           │      │         PYTHON PROJECT        │
├───────────────────────────────┤      ├───────────────────────────────┤
│  web/                         │      │  tools/pdf-generator          │
│  Svelte Frontend              │      │  CLI Tools                    │
│  • TypeScript                 │      │  • create_config              │
│  • Vite                       │      │  • demo                       │
│  • pnpm                       │      │                               │
│                               │      │  Core Modules                 │
│  API Client                   │      │  • api_client                 │
│  • fetch/axios                │      │  • chart_builder              │
│                               │      │  • table_builders             │
│                               │      │  • doc_builder                │
│                               │      │                               │
│                               │      │  LaTeX Compiler               │
│                               │      │  • XeLaTeX                    │
│                               │      │  • matplotlib                 │
│                               │      │                               │
│                               │      │  PDF Output                   │
│                               │      │  output/*.pdf                 │
│                               │      │                               │
│  Runtime                      │      │  Runtime                      │
│  • Dev: localhost:5173        │      │  • CLI on demand              │
│  • Prod: Go-served static     │      │  • Python 3.9+                │
│                               │      │  • Virtual env                │
└───────────────────────────────┘      └───────────────────────────────┘
```

## Components

### Go Server

**Location**: `/cmd/`, `/internal/`

**Purpose**: Real-time data collection, storage, and API server

**Key Modules**:

- **`cmd/radar/`** - Main server entry point
  - Sensor data collection (radar/LIDAR)
  - HTTP API server
  - Background task scheduler
  - Systemd service integration

- **`internal/api/`** - HTTP API endpoints
  - `/api/radar_stats` - Statistical summaries and rollups
  - `/api/config` - Configuration retrieval
  - `/command` - Send radar commands
  - RESTful design with JSON responses

- **`internal/radar/`** - Radar sensor integration
  - Serial port communication
  - Data parsing and validation
  - Error handling and retry logic

- **`internal/lidar/`** - LIDAR sensor integration
  - UDP packet listener and decoder (Hesai Pandar40P)
  - `FrameBuilder` accumulates complete 360° rotations with sequence checks
  - `BackgroundManager` maintains EMA grid (40 rings × 1800 azimuth bins)
  - Persists `lidar_bg_snapshot` rows and emits `frame_stats` into `system_events`
  - Tooling for ASC export, pose transforms, and background tuning APIs

- **`internal/monitoring/`** - System monitoring
  - Health checks
  - Performance metrics
  - Error logging

- **`internal/units/`** - Unit conversion
  - Speed conversions (MPH ↔ KPH)
  - Distance conversions
  - Timezone handling

**Runtime**: Deployed as systemd service on Raspberry Pi (ARM64 Linux)

**Communication**:

- **Input**:
  - Radar: Serial port data (/dev/ttyUSB0, USB connection)
  - LIDAR: Network/UDP packets (Ethernet connection, verified with LidarView/CloudCompare)
- **Output**:
  - HTTP API (JSON over port 8080)
  - SQLite database writes

### Python PDF Generator

**Location**: `/tools/pdf-generator/`

**Purpose**: Generate professional PDF reports from sensor data

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

### Web Frontend

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

### macOS Visualiser (Swift/Metal)

**Location**: `/tools/visualiser-macos/`

**Purpose**: Real-time 3D visualisation of LiDAR point clouds, object tracking, and debug overlays for M1+ Macs

**Technology Stack**:

- Swift 5.9+ with SwiftUI (macOS 14+)
- Metal for GPU-accelerated rendering
- gRPC for streaming communication
- XCTest for testing

**M1 Features (Milestone 1 - Recorder/Replayer - 91.9% Go Coverage)**:

- ✅ Deterministic replay of `.vrlog` recordings
- ✅ Pause/Play/Seek/SetRate playback controls via gRPC
- ✅ Frame-by-frame navigation and timeline scrubbing
- 🚧 3D camera controls (Swift implementation in progress)

**Go Backend** (`internal/lidar/visualiser/`):

- `replay.go` - ReplayServer for streaming `.vrlog` files
- `recorder/` - Record/replay with deterministic frame sequences
- `grpc_server.go` - Streaming RPCs and playback control
- `synthetic.go` - Test data generator

**Command-Line Tools**:

- `cmd/tools/visualiser-server` - Synthetic data generator
- `cmd/tools/replay-server` - Replay recordings via gRPC

**Communication**:

- **Input**: gRPC streaming (localhost:50051)
- **Output**: User interactions, label annotations

**See**: [tools/visualiser-macos/README.md](tools/visualiser-macos/README.md) and [docs/lidar/visualiser/](docs/lidar/visualiser/)

### Database Layer

**Location**: `/data/`, managed by `internal/db/`

**Database**: SQLite 3.x

**Schema Design**:

The database uses a **JSON-first approach** with generated columns for performance. Raw sensor events are stored as JSON, with frequently-queried fields extracted to indexed columns automatically.

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

When a sensor reading arrives, the Go server stores the entire event as JSON in `raw_event`, and SQLite automatically populates the generated columns. This provides:

- **Flexibility**: Complete event data preserved for future analysis
- **Performance**: Fast indexed queries on common fields (speed, timestamp)
- **Schema evolution**: New fields can be added without migration

**Key Tables**:

- `radar_data` - Raw radar readings (JSON events with speed/magnitude)
- `radar_objects` - Classified transits from radar's onboard classifier
- `radar_data_transits` - Sessionized transits built by transit worker from `radar_data`
- `radar_transit_links` - Many-to-many links between transits and raw radar_data
- `lidar_bg_snapshot` - LIDAR background grid for motion detection (40×1800 range-image)
- `lidar_objects` - Track-extracted transits from LIDAR processing [PLANNED]
- `radar_commands` / `radar_command_log` - Command history and execution logs
- `site` - Site metadata (location, speed limits)
- `site_config_periods` - Time-based sensor configuration (cosine error angle history)

**Transit Sources** (3 independent object detection pipelines):

1. **radar_objects**: Hardware classifier in OPS243 radar sensor
2. **radar_data_transits**: Software sessionization of raw radar_data points
3. **lidar_objects**: Software tracking from LIDAR point clouds [PLANNED]

These three sources will be compared for initial reporting, with eventual goal of:

- FFT-based radar processing for improved object segmentation
- Sensor fusion using LIDAR data to assist radar object detection

**Key Features**:

- High-precision timestamps (DOUBLE for subsecond accuracy via `UNIXEPOCH('subsec')`)
- Sessionization via `radar_data_transits` (avoids expensive CTEs in queries)
- LIDAR background modeling for change detection (grid stored as BLOB)
- WAL mode enabled for concurrent readers/writers
- Indexes on timestamp columns for fast time-range queries
- Time-based site configuration via `site_config_periods` (Type 6 Slowly Changing Dimension)

**Site Configuration Periods**:

The `site_config_periods` table implements a Type 6 SCD pattern for tracking sensor configuration changes over time. Key aspects:

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
  - LIDAR background grid → `lidar_bg_snapshot`
  - [PLANNED] LIDAR tracking → `lidar_objects`
- **Python PDF Generator**: Read-only (via HTTP API)
  - Queries transit data from 3 sources for comparison reports
  - Aggregates statistics across detection methods
- **Web Frontend**: Read-only (via HTTP API)
  - Real-time dashboard showing all 3 transit sources

## Data Flow

### Real-Time Data Collection

```
Radar (Serial):
1. OPS243 radar → USB-Serial (/dev/ttyUSB0)
2. internal/radar/ reader parses JSON speed/magnitude payloads
3. INSERT raw packets into `radar_data`; hardware detections → `radar_objects`

LIDAR (Network/UDP):
1. Hesai P40 → Ethernet/UDP (192.168.100.202 → 192.168.100.151 listener)
2. Packet decoder reconstructs blocks → `FrameBuilder` completes 360° rotations
3. `BackgroundManager` updates EMA background grid (40 × 1800 cells)
4. Persist snapshots → INSERT/UPSERT `lidar_bg_snapshot`
5. Emit frame statistics and performance metrics → `system_events`

Transit Worker (Background Process):
1. Query recent radar_data points → Sessionization algorithm
2. Group readings into vehicle transits (time-gap based)
3. INSERT/UPDATE radar_data_transits
4. Link raw data → INSERT INTO radar_transit_links

Three Transit Sources:
• radar_objects        (radar hardware classifier)
• radar_data_transits  (software sessionization)
• lidar_objects        (LIDAR tracking) [PLANNED]
```

### PDF Report Generation

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

### Web Visualisation

```
1. User → Open browser → Vite dev server (or static build)
2. Frontend → Fetch /api/radar_stats → Go Server
3. Go Server → Query SQLite → Return JSON
4. Frontend → Parse JSON → Render Svelte components
5. Frontend → Display charts/tables → Browser DOM
```

## Technology Stack

### Go Server

| Component  | Technology             | Version | Purpose                 |
| ---------- | ---------------------- | ------- | ----------------------- |
| Language   | Go                     | 1.21+   | High-performance server |
| Database   | SQLite                 | 3.x     | Data storage            |
| HTTP       | net/http (stdlib)      | -       | API server              |
| Serial     | github.com/tarm/serial | -       | Sensor communication    |
| Deployment | systemd                | -       | Service management      |
| Build      | Make                   | -       | Build automation        |

### Python PDF Generator

| Component      | Technology | Version | Purpose             |
| -------------- | ---------- | ------- | ------------------- |
| Language       | Python     | 3.9+    | Report generation   |
| Charts         | matplotlib | 3.9+    | Data visualisation  |
| LaTeX          | PyLaTeX    | 1.4+    | Document generation |
| HTTP           | requests   | 2.32+   | API client          |
| Testing        | pytest     | 8.4+    | Test framework      |
| Coverage       | pytest-cov | 7.0+    | Coverage reporting  |
| LaTeX Compiler | XeLaTeX    | -       | PDF compilation     |

### Web Frontend

| Component       | Technology | Version | Purpose               |
| --------------- | ---------- | ------- | --------------------- |
| Framework       | Svelte     | 5.x     | Reactive UI           |
| Language        | TypeScript | 5.x     | Type safety           |
| Build Tool      | Vite       | 6.x     | Dev server & bundling |
| Package Manager | pnpm       | 9.x     | Dependency management |
| Linting         | ESLint     | 9.x     | Code quality          |

## Integration Points

### Go Server ↔ SQLite

**Interface**: Go database/sql with SQLite driver

**Operations**:

- INSERT `radar_data` (real-time writes with JSON events)
- INSERT `radar_objects` (classified detections)
- Background sessionization: query `radar_data` → insert/update `radar_data_transits`
- LIDAR background modeling: update `lidar_bg_snapshot`
- SELECT for API queries (read optimised with generated columns)

**Performance Considerations**:

- JSON storage with generated columns for fast indexed queries
- Indexes on timestamp columns (`transit_start_unix`, `transit_end_unix`)
- Batched inserts for high-frequency sensors
- WAL mode for concurrent reads during writes
- Subsecond timestamp precision (DOUBLE type)

### Go Server ↔ Python PDF Generator

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

### Go Server ↔ Web Frontend

**Interface**: HTTP REST API (JSON) + Static file serving

**Same API as Python**, plus:

- Static file serving for Svelte build (`/app/*`)
- SPA routing with fallback to `index.html`
- Favicon serving
- Root redirect to `/app/`

### Python ↔ LaTeX

**Interface**: PyLaTeX → XeLaTeX subprocess

**Process**:

1. PyLaTeX generates `.tex` file
2. `subprocess.run(['xelatex', ...])` compiles
3. Retry logic for LaTeX errors
4. Cleanup of intermediate files (`.aux`, `.log`)

## Deployment Architecture

### Production Environment (Raspberry Pi)

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
│  │  • --listen :8080                        │  │
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
│  • Network/UDP (LIDAR - Ethernet)              │
└────────────────────────────────────────────────┘
```

**Service Management**:

```sh
sudo systemctl status velocity-report.service
sudo systemctl restart velocity-report.service
sudo journalctl -u velocity-report.service -f
```

### Development Environment

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

## Security & Privacy

### Privacy Guarantees

✅ **No License Plate Recognition**

- Sensors measure speed only, no cameras

✅ **No Video Recording**

- Pure time-series data (timestamp + speed)

✅ **No Personally Identifiable Information**

- No tracking of individual vehicles
- Aggregate statistics only

✅ **Local Storage**

- All data stored locally on device
- No cloud uploads unless explicitly configured

### Security Considerations

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

## Performance Characteristics

### Go Server

- **Throughput**: 1000+ readings/second (tested)
- **Memory**: ~50MB typical, ~100MB peak
- **CPU**: <5% on Raspberry Pi 4 (idle), <20% during aggregation
- **Storage**: ~1MB per 10,000 readings (compressed)

### Python PDF Generator

- **Execution Time**:
  - Config generation: <1 second
  - PDF generation: 10-30 seconds (depends on data volume)
  - LaTeX compilation: 3-5 seconds
- **Memory**: ~200MB peak (matplotlib rendering)
- **Disk**: ~1MB per PDF, ~5MB temp files during generation

### Web Frontend

- **Bundle Size**: ~150KB (gzipped)
- **Load Time**: <1 second (local network)
- **API Latency**: <100ms typical
- **Rendering**: 60fps on modern browsers

## Future Improvements

### Planned Enhancements

1. **Real-time WebSocket Updates** (Web Frontend)
   - Push new readings to browser
   - Live chart updates without polling

2. **Multi-Location Support** (All Components)
   - Support multiple sensor deployments
   - Aggregate across locations

3. **Advanced Analytics** (Python + Go)
   - Traffic pattern detection
   - Anomaly detection (speeding clusters)
   - Predictive modeling

4. **Mobile App** (New Component)
   - iOS/Android apps
   - Push notifications for events
   - Mobile-optimised dashboards

5. **Export Formats** (Python)
   - CSV exports
   - Excel reports
   - JSON data dumps

6. **Authentication & Authorization** (Go Server)
   - API key authentication
   - Role-based access control
   - Audit logging

### Architectural Considerations

**Scalability**:

- Current design supports single deployment
- For multi-location, consider:
  - PostgreSQL instead of SQLite
  - Message queue for sensor data
  - Distributed tracing

**Reliability**:

- Add health check endpoints
- Implement circuit breakers for external dependencies
- Add retry logic with exponential backoff

**Observability**:

- Structured logging (JSON)
- Prometheus metrics export
- Distributed tracing (OpenTelemetry)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development workflows, testing requirements, and contribution guidelines.

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.
