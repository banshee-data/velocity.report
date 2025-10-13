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

**velocity.report** is a distributed system for neighborhood traffic monitoring with three main components:

1. **Go Server** - Real-time data collection and HTTP API
2. **Python PDF Generator** - Professional report generation with LaTeX
3. **Web Frontend** - Real-time visualization (Svelte/TypeScript)

All components share a common SQLite database as the single source of truth.

### Design Principles

- **Privacy First**: No license plates, no video, no PII
- **Simplicity**: SQLite as the only database, minimal dependencies
- **Offline-First**: Works without internet connectivity
- **Modular**: Each component operates independently
- **Well-Tested**: Comprehensive test coverage across all components

## Architecture Diagram

### Physical Deployment

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        HARDWARE INFRASTRUCTURE                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌────────────────────┐                    ┌────────────────────┐       │
│  │  Radar Sensor      │                    │  LIDAR Sensor      │       │
│  │  ┌──────────────┐  │                    │  ┌──────────────┐  │       │
│  │  │ Omnipresense │  │                    │  │   Hesai P40  │  │       │
│  │  │   OPS243     │  │                    │  │    40-beam   │  │       │
│  │  └──────────────┘  │                    │  └──────────────┘  │       │
│  │   Serial Output    │                    │   Ethernet Output  │       │
│  │   (USB/RS-232)     │                    │   (RJ45/PoE)       │       │
│  └─────────┬──────────┘                    └──────────┬─────────┘       │
│            │                                          │                 │
│            │ USB-Serial                               │ Ethernet        │
│            │                                          │                 │
│            └───────────────────┬──────────────────────┘                 │
│                                │                                        │
│  ┌─────────────────────────────┼─────────────────────────────────────┐  │
│  │       Raspberry Pi 4 (ARM64 Linux)                                │  │
│  │                             │                                     │  │
│  │  Hardware:                  │                                     │  │
│  │  • 4GB RAM                  │                                     │  │
│  │  • 64GB SD Card             ↓                                     │  │
│  │  • USB Ports (Radar)   /dev/ttyUSB0                               │  │
│  │  • Ethernet Port (LIDAR + Network)                                │  │
│  │                                                                   │  │
│  │  Network: Local LAN (192.168.1.x)                                 │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
                                 │
                                 │ Local Network
                                 │
┌────────────────────────────────┼──────────────────────────────────────┐
│                      SOFTWARE STACK (Raspberry Pi)                    │
├────────────────────────────────┼──────────────────────────────────────┤
│                                │                                      │
│  ┌─────────────────────────────┼──────────────────────────────────┐   │
│  │             velocity.report Go Server                          │   │
│  │             (systemd service: go-sensor.service)               │   │
│  │                            │                                   │   │
│  │  ┌─────────────────────────┼──────────────────────────────┐    │   │
│  │  │  cmd/radar/             │                              │    │   │
│  │  │  ┌──────────────────────▼───────────────────────────┐  │    │   │
│  │  │  │         Sensor Input Handlers                    │  │    │   │
│  │  │  │                                                   │ │   │   │
│  │  │  │  ┌────────────────┐      ┌────────────────────┐ │ │   │   │
│  │  │  │  │ Radar Handler  │      │  LIDAR Handler     │ │ │   │   │
│  │  │  │  │ (Serial Port)  │      │  (Network/UDP)     │ │ │   │   │
│  │  │  │  │ internal/radar/│      │  internal/lidar/   │ │ │   │   │
│  │  │  │  └───────┬────────┘      └─────────┬──────────┘ │ │   │   │
│  │  │  │          │                         │            │ │   │   │
│  │  │  │          └────────┬────────────────┘            │ │   │   │
│  │  │  │                   │ Speed + Timestamp           │ │   │   │
│  │  │  └───────────────────┼─────────────────────────────┘ │   │   │
│  │  │                      │                               │   │   │
│  │  │  ┌───────────────────▼─────────────────────────────┐ │   │   │
│  │  │  │         Database Layer (internal/db/)           │ │   │   │
│  │  │  │  • Connection pooling                           │ │   │   │
│  │  │  │  • Transaction management                       │ │   │   │
│  │  │  │  • Query optimization                           │ │   │   │
│  │  │  └───────────────────┬─────────────────────────────┘ │   │   │
│  │  └────────────────────────────────────────────────────────┘   │   │
│  │                         │                                      │   │
│  │  ┌──────────────────────▼─────────────────────────────────┐  │   │
│  │  │         SQLite Database (sensor_data.db)               │  │   │
│  │  │         /var/lib/velocity/sensor_data.db               │  │   │
│  │  │                                                         │  │   │
│  │  │  Tables:                                               │  │   │
│  │  │  • radar_readings (raw sensor data)                   │  │   │
│  │  │  • aggregated_stats (hourly summaries)                │  │   │
│  │  │  • config (system settings)                           │  │   │
│  │  └──────────────────────┬─────────────────────────────────┘  │   │
│  │                         │                                     │   │
│  │  ┌──────────────────────▼─────────────────────────────────┐  │   │
│  │  │         HTTP API Server (internal/api/)                │  │   │
│  │  │         Listen: 0.0.0.0:8080                           │  │   │
│  │  │                                                         │  │   │
│  │  │  Endpoints:                                            │  │   │
│  │  │  • GET  /api/stats      (time-series data)            │  │   │
│  │  │  • GET  /api/readings   (raw sensor data)             │  │   │
│  │  │  • GET  /api/config     (system config)               │  │   │
│  │  │  • POST /api/config     (update config)               │  │   │
│  │  └─────────────────────────────────────────────────────────┘  │   │
│  │                                                                │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
                                 │
                                 │ HTTP API (JSON)
                                 │ Local Network (192.168.1.x:8080)
                                 │
        ┌────────────────────────┼───────────────────────┐
        │                        │                       │
        │                        │                       │
┌───────▼─────────────┐  ┌───────▼─────────────┐  ┌──────▼──────────────┐
│  PYTHON PROJECT     │  │  WEB PROJECT        │  │  DEVELOPER MACHINE  │
│  tools/pdf-generator│  │  web/               │  │  (Any OS)           │
├─────────────────────┤  ├─────────────────────┤  ├─────────────────────┤
│                     │  │                     │  │                     │
│ ┌─────────────────┐ │  │ ┌─────────────────┐ │  │ ┌─────────────────┐ │
│ │ CLI Tools       │ │  │ │ Svelte Frontend │ │  │ │ Web Browser     │ │
│ │ • create_config │ │  │ │ • TypeScript    │ │  │ │ • Chrome/Firefox│ │
│ │ • demo          │ │  │ │ • Vite          │ │  │ │                 │ │
│ └────────┬────────┘ │  │ │ • pnpm          │ │  │ └────────┬────────┘ │
│          │          │  │ └────────┬────────┘ │  │          │          │
│          │          │  │          │ Build    │  │          │          │
│ ┌────────▼────────┐ │  │          │          │  │          │          │
│ │ Core Modules    │ │  │ ┌────────▼────────┐ │  │ ┌────────▼────────┐ │
│ │ • api_client    │ │  │ │ API Client      │ │  │ │ HTTP Requests   │ │
│ │ • chart_builder │ │  │ │ (fetch/axios)   │ │  │ │ to :8080        │ │
│ │ • table_builders│ │  │ └─────────────────┘ │  │ └─────────────────┘ │
│ │ • doc_builder   │ │  │                     │  │                     │
│ └────────┬────────┘ │  │ Runtime:            │  │                     │
│          │          │  │ • Dev: localhost:   │  │                     │
│          │ Charts & │  │   5173 (Vite)       │  │                     │
│          │ Tables   │  │ • Prod: Static      │  │                     │
│          │          │  │   files served by   │  │                     │
│ ┌────────▼────────┐ │  │   Go server         │  │                     │
│ │ LaTeX Compiler  │ │  │                     │  │                     │
│ │ • XeLaTeX       │ │  │                     │  │                     │
│ │ • matplotlib    │ │  │                     │  │                     │
│ └────────┬────────┘ │  │                     │  │                     │
│          │          │  │                     │  │                     │
│ ┌────────▼────────┐ │  │                     │  │                     │
│ │ PDF Output      │ │  │                     │  │                     │
│ │ output/*.pdf    │ │  │                     │  │                     │
│ └─────────────────┘ │  │                     │  │                     │
│                     │  │                     │  │                     │
│ Runtime:            │  │                     │  │                     │
│ • CLI on demand     │  │                     │  │                     │
│ • Python 3.9+       │  │                     │  │                     │
│ • Virtual env       │  │                     │  │                     │
└─────────────────────┘  └─────────────────────┘  └─────────────────────┘

    Developer Machine        Developer Machine        Any Device on LAN
    (macOS/Linux/Windows)    (macOS/Linux/Windows)    (Browser)
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
  - `/api/stats` - Statistical summaries
  - `/api/readings` - Raw sensor readings
  - `/api/config` - Configuration management
  - RESTful design with JSON responses

- **`internal/db/`** - Database layer
  - SQLite connection management
  - Schema migrations
  - Query builders
  - Transaction handling

- **`internal/radar/`** - Radar sensor integration
  - Serial port communication
  - Data parsing and validation
  - Error handling and retry logic

- **`internal/lidar/`** - LIDAR sensor integration
  - Network/Ethernet communication
  - Point cloud processing
  - Data normalization
  - Externally verified in LidarView and CloudCompare

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

**Purpose**: Real-time data visualization and interactive dashboards

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

### Database Layer

**Location**: `/data/`, managed by `internal/db/`

**Database**: SQLite 3.x

**Schema**:

```sql
-- Raw sensor readings
CREATE TABLE radar_readings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,        -- Unix timestamp
    speed_mph REAL NOT NULL,           -- Vehicle speed
    sensor_type TEXT NOT NULL,         -- 'radar' or 'lidar'
    direction TEXT,                    -- 'approaching' or 'departing'
    created_at INTEGER DEFAULT (strftime('%s', 'now'))
);

-- Aggregated statistics (hourly)
CREATE TABLE aggregated_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hour_start INTEGER NOT NULL,       -- Unix timestamp (hour boundary)
    count INTEGER NOT NULL,            -- Number of readings
    avg_speed REAL NOT NULL,           -- Average speed
    max_speed REAL NOT NULL,           -- Maximum speed
    min_speed REAL NOT NULL,           -- Minimum speed
    percentile_85 REAL                 -- 85th percentile speed
);

-- System configuration
CREATE TABLE config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER DEFAULT (strftime('%s', 'now'))
);

-- Schema migrations
CREATE TABLE migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER DEFAULT (strftime('%s', 'now'))
);
```

**Migrations**: Located in `/data/migrations/`, managed by Go server

**Access Patterns**:
- **Go Server**: Read/Write (real-time inserts, aggregations)
- **Python PDF Generator**: Read-only (via HTTP API)
- **Web Frontend**: Read-only (via HTTP API)

## Data Flow

### Real-Time Data Collection

```
Radar (Serial):
1. Radar Sensor → USB-Serial (/dev/ttyUSB0) → internal/radar/ handler
2. Parse speed data → Validate → internal/db
3. INSERT INTO radar_readings → SQLite

LIDAR (Network):
1. LIDAR Sensor → Ethernet/UDP → internal/lidar/ handler
2. Process point cloud → Extract speed → Validate → internal/db
3. INSERT INTO radar_readings → SQLite

Background Processing:
4. Hourly aggregation task → Query radar_readings
5. Compute statistics (avg, max, p85) → INSERT INTO aggregated_stats
```

### PDF Report Generation

```
1. User → Run create_config.py → Generate config.json
2. User → Run demo.py with config.json
3. demo.py → api_client.py → HTTP GET /api/stats → Go Server
4. Go Server → Query SQLite → Return JSON
5. api_client.py → Parse JSON → Pass to chart_builder, table_builders
6. chart_builder → matplotlib → PNG charts
7. table_builders → PyLaTeX → LaTeX tables
8. document_builder → Combine → LaTeX document
9. XeLaTeX → Compile → PDF output
```

### Web Visualization

```
1. User → Open browser → Vite dev server (or static build)
2. Frontend → Fetch /api/stats → Go Server
3. Go Server → Query SQLite → Return JSON
4. Frontend → Parse JSON → Render Svelte components
5. Frontend → Display charts/tables → Browser DOM
6. (Optional) Frontend → Poll for updates → Real-time refresh
```

## Technology Stack

### Go Server

| Component | Technology | Version | Purpose |
|-----------|-----------|---------|---------|
| Language | Go | 1.21+ | High-performance server |
| Database | SQLite | 3.x | Data storage |
| HTTP | net/http (stdlib) | - | API server |
| Serial | github.com/tarm/serial | - | Sensor communication |
| Deployment | systemd | - | Service management |
| Build | Make | - | Build automation |

### Python PDF Generator

| Component | Technology | Version | Purpose |
|-----------|-----------|---------|---------|
| Language | Python | 3.9+ | Report generation |
| Charts | matplotlib | 3.9+ | Data visualization |
| LaTeX | PyLaTeX | 1.4+ | Document generation |
| HTTP | requests | 2.32+ | API client |
| Testing | pytest | 8.4+ | Test framework |
| Coverage | pytest-cov | 7.0+ | Coverage reporting |
| LaTeX Compiler | XeLaTeX | - | PDF compilation |

### Web Frontend

| Component | Technology | Version | Purpose |
|-----------|-----------|---------|---------|
| Framework | Svelte | 5.x | Reactive UI |
| Language | TypeScript | 5.x | Type safety |
| Build Tool | Vite | 6.x | Dev server & bundling |
| Package Manager | pnpm | 9.x | Dependency management |
| Linting | ESLint | 9.x | Code quality |

## Integration Points

### Go Server ↔ SQLite

**Interface**: Go database/sql with SQLite driver

**Operations**:
- INSERT radar_readings (real-time writes)
- SELECT for aggregations (hourly background task)
- INSERT aggregated_stats (computed summaries)
- SELECT for API queries (read optimized)

**Performance Considerations**:
- Indexes on timestamp columns
- Batched inserts for high-frequency sensors
- WAL mode for concurrent reads during writes

### Go Server ↔ Python PDF Generator

**Interface**: HTTP REST API (JSON)

**Endpoints**:

```
GET /api/stats?start=<unix>&end=<unix>&group_by=<hour|day>
Response: {
  "readings": [...],
  "summary": {
    "total_count": 1234,
    "avg_speed": 28.5,
    "max_speed": 45.2,
    "percentile_85": 32.1
  }
}

GET /api/config
Response: {
  "timezone": "America/Los_Angeles",
  "speed_limit": 25,
  "units": "mph"
}
```

**Error Handling**:
- HTTP 200: Success
- HTTP 400: Invalid parameters
- HTTP 500: Server error
- Python client retries with exponential backoff

### Go Server ↔ Web Frontend

**Interface**: HTTP REST API (JSON) + Static file serving

**Same API as Python**, plus:
- Static file serving for Svelte build
- CORS headers for development
- WebSocket support (planned for real-time updates)

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
┌─────────────────────────────────────────────────┐
│         Raspberry Pi 4 (ARM64, Linux)           │
│                                                  │
│  ┌──────────────────────────────────────────┐  │
│  │  systemd (go-sensor.service)             │  │
│  │  ↓                                        │  │
│  │  /usr/local/bin/velocity-server          │  │
│  │  (Go Server Binary)                      │  │
│  │                                            │  │
│  │  Environment:                             │  │
│  │  • PORT=8080                              │  │
│  │  • DB_PATH=/var/lib/velocity/sensor.db   │  │
│  │  • LOG_LEVEL=info                         │  │
│  └──────────────────────────────────────────┘  │
│                                                  │
│  ┌──────────────────────────────────────────┐  │
│  │  SQLite Database                         │  │
│  │  /var/lib/velocity/sensor_data.db        │  │
│  └──────────────────────────────────────────┘  │
│                                                  │
│  Sensor Connections:                            │
│  • /dev/ttyUSB0 (Radar - Serial)               │
│  • Network/UDP (LIDAR - Ethernet)              │
└─────────────────────────────────────────────────┘
```

**Service Management**:
```sh
sudo systemctl status go-sensor.service
sudo systemctl restart go-sensor.service
sudo journalctl -u go-sensor.service -f
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
   - Mobile-optimized dashboards

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
