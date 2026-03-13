# Architecture

Canonical reference for the velocity.report system architecture. For the full design document with diagrams, see [ARCHITECTURE.md](../../ARCHITECTURE.md).

## System Overview

**velocity.report** is a privacy-focused traffic monitoring system with four components:

| Component            | Language          | Location                  | Purpose                          |
| -------------------- | ----------------- | ------------------------- | -------------------------------- |
| Go server            | Go                | `cmd/`, `internal/`       | Sensor data collection, HTTP API |
| Python PDF generator | Python            | `tools/pdf-generator/`    | Professional report generation   |
| Web frontend         | Svelte/TypeScript | `web/`                    | Real-time visualisation          |
| macOS visualiser     | Swift/Metal       | `tools/visualiser-macos/` | 3D LiDAR point cloud rendering   |

## Technology Stack

| Layer      | Technology                        | Notes                             |
| ---------- | --------------------------------- | --------------------------------- |
| Server     | Go 1.25+                          | stdlib `net/http`, `database/sql` |
| Database   | SQLite 3.51.2 (modernc.org)       | Pure-Go driver, WAL mode          |
| Reports    | Python 3.11+, matplotlib, PyLaTeX | XeLaTeX compilation               |
| Frontend   | Svelte 5, TypeScript, Vite 7+     | pnpm, ESLint                      |
| Visualiser | Swift 5.9+, SwiftUI, Metal        | macOS 14+, grpc-swift             |
| Streaming  | gRPC + protobuf                   | Port 50051, server-streaming      |
| Docs site  | Eleventy                          | `public_html/`                    |
| Build      | Make                              | 101+ documented targets           |

## Deployment Target

- **Hardware:** Raspberry Pi 4 (ARM64 Linux, 4GB RAM, 64GB SD)
- **Service:** systemd (`velocity-report.service`)
- **Binary:** `/usr/local/bin/velocity-report`
- **Data:** `/var/lib/velocity-report/`
- **Database:** `/var/lib/velocity-report/sensor_data.db`
- **User:** `velocity:velocity`
- **Network:** Local only — no cloud, no external transmission

## Data Flow

```
Radar (Serial /dev/ttyUSB0)
  → internal/radar/ → JSON parse → INSERT radar_data, radar_objects

LIDAR (UDP 192.168.100.202 → 192.168.100.151)
  → packet decoder → FrameBuilder → BackgroundManager EMA grid
  → INSERT lidar_bg_snapshot, emit frame_stats
  → gRPC stream → macOS visualiser (port 50051)

Transit Worker (background)
  → sessionise radar_data → INSERT radar_data_transits + radar_transit_links
```

## Database Schema (Key Tables)

| Table                 | Purpose                                         | Status       |
| --------------------- | ----------------------------------------------- | ------------ |
| `radar_data`          | Raw radar events (JSON with generated columns)  | Production   |
| `radar_objects`       | Hardware classifier detections                  | Production   |
| `radar_data_transits` | Software-sessionised vehicle transits           | Production   |
| `radar_transit_links` | Many-to-many: transits ↔ raw data               | Production   |
| `lidar_bg_snapshot`   | Background grid (40×1800 BLOB)                  | Experimental |
| `lidar_objects`       | LIDAR track-extracted transits                  | Planned      |
| `site`                | Site metadata (location, speed limits)          | Production   |
| `site_config_periods` | Time-based sensor config (cosine error history) | Production   |

**Design:** JSON-first with generated columns — raw events stored as JSON, frequently-queried fields extracted to indexed columns automatically.

## Integration Points

| From → To              | Interface      | Port/Path      |
| ---------------------- | -------------- | -------------- |
| Radar → Go server      | Serial (USB)   | `/dev/ttyUSB0` |
| LIDAR → Go server      | UDP            | 192.168.100.x  |
| Go server → SQLite     | database/sql   | File I/O       |
| Go server → Web/Python | HTTP REST JSON | `:8080`        |
| Go server → Visualiser | gRPC streaming | `:50051`       |
| Python → PDF           | XeLaTeX        | subprocess     |

## API Endpoints

| Method | Path               | Purpose                       |
| ------ | ------------------ | ----------------------------- |
| GET    | `/api/radar_stats` | Aggregated transit statistics |
| GET    | `/api/config`      | System configuration          |
| POST   | `/command`         | Send radar commands           |
| GET    | `/events`          | Raw event stream              |

## Traffic Metrics

Standard traffic engineering percentiles:

- **p50** (median) — typical vehicle behaviour
- **p85** (85th percentile) — design speed standard
- **p98** (top 2%) — high-speed threshold detection

## Repository Structure

```
velocity.report/
├── cmd/                      # Go CLI applications
│   ├── deploy/               # Deployment management
│   ├── radar/                # Main server entry point
│   ├── sweep/                # Parameter sweep utilities
│   ├── tools/                # Go utility tools
│   └── transit-backfill/     # Transit data backfill
├── internal/                 # Go server internals
│   ├── api/                  # HTTP API endpoints
│   ├── config/               # Tuning configuration
│   ├── db/                   # SQLite + migrations
│   ├── lidar/                # LiDAR processing pipeline
│   ├── radar/                # Radar sensor logic
│   └── ...                   # See full tree in repo
├── web/                      # Svelte frontend
├── tools/                    # Python + native tooling
│   ├── grid-heatmap/         # Grid heatmap plotting
│   ├── pdf-generator/        # PDF report generation
│   └── visualiser-macos/     # macOS 3D visualiser
├── config/                   # LiDAR tuning configs
├── docs/                     # Internal documentation
├── public_html/              # Public docs site (Eleventy)
├── proto/                    # Protobuf definitions
├── data/                     # Test data and fixtures
└── scripts/                  # Utility scripts
```
