# CLI reference guide

Complete reference for the `velocity-report` and `velocity-ctl` command-line interfaces as currently implemented: all flags, subcommands, HTTP endpoints, and Makefile targets.

For the proposed CLI restructuring plan (subcommand hierarchy, API versioning, config file support), see [cli-restructuring-plan.md](../plans/cli-restructuring-plan.md).

---

## Table of contents

1. [Overview](#overview)
2. [Current State Inventory](#current-state-inventory)
3. [Quick Reference](#quick-reference)

---

## Overview

velocity.report consists of multiple CLI applications:

- **1 main application** (`velocity-report`): production service with radar, optional LiDAR integration, HTTP API, and transit worker
- **3 utility applications**: device management (`velocity-ctl`), parameter sweep testing (`sweep`), and elevation backfill
- **100+ Makefile targets**: build, test, deploy, and development tasks
- **Multiple HTTP APIs**: radar API (`:8080`), LiDAR monitor (`:8081`), admin routes (`/debug/`)

This document covers what exists and works today.

---

## Current state inventory

### 1. Radar binary ([cmd/radar](../../cmd/radar))

**Description:** Main production service that runs radar serial monitoring, HTTP API server, and optional lidar components.

**Mode:** Long-running service

**Location:** [cmd/radar](../../cmd/radar)

#### Quick start examples

```bash
# Production mode
velocity-report --db-path /var/lib/velocity-report/sensor_data.db

# Development (no hardware)
velocity-report --disable-radar --debug

# With lidar enabled
velocity-report --enable-lidar --lidar-listen :8081
```

#### CLI flags

**Core Service Flags:**

- `--listen :8080` - HTTP listen address for API server
- `--db-path sensor_data.db` - Path to SQLite database file
- `--debug` - Run in debug mode (mock serial mux, extra logging)
- `--fixture` - Load fixture data instead of real hardware
- `--version`, `-v` - Print version information and exit

**Radar Hardware Flags:**

- `--port /dev/ttySC1` - Serial device path for radar sensor
- `--disable-radar` - Disable radar serial I/O (serve DB/HTTP only)
- `--units mph` - Display units (`mps`, `mph`, `kmph`)
- `--timezone UTC` - Timezone for display

**LiDAR Integration Flags:**

- `--enable-lidar` - Enable in-process lidar components
- `--lidar-listen :8081` - HTTP listen address for lidar monitor
- `--lidar-no-parse` - Disable packet parsing (forwarding only)
- `--lidar-forward` - Forward UDP packets to another port
- `--lidar-forward-addr localhost` - Forwarding destination address
- `--lidar-forward-mode lidarview` - Forward mode: lidarview, grpc, or both
- `--lidar-foreground-forward` - Forward foreground-only packets
- `--lidar-foreground-forward-addr localhost` - Foreground forwarding address
- `--lidar-grpc-listen localhost:50051` - gRPC server listen address
- `--lidar-pcap-dir ../sensor_data/lidar` - Safe directory for PCAP files

**Sensor/network settings** are now configured via the
[tuning config file](../../config/CONFIG.md) (`l1.sensor`, `l1.udp_port`,
`l1.forward_port`, `l1.foreground_forward_port`), not CLI flags.

**PDF Report Flags:**

- `--pdf-latex-flow` - Use LaTeX flow for PDF generation
- `--pdf-tex-root` - TeX installation root directory

**Logging Flags:**

- `--log-level ops` - Log level (`ops`, `diag`, `trace`)

**LiDAR Network Flags:**

- `--lidar-udp-port 2369` - UDP listen port for LiDAR packets
- `--lidar-udp-rcv-buf` - UDP receive buffer size
- `--lidar-forward-port 2368` - Port for raw packet forwarding
- `--lidar-foreground-forward-port 2370` - Port for foreground packet forwarding

**Transit Worker Flags:**

- `--enable-transit-worker` - Enable background transit sessionisation
- `--transit-worker-interval` - Processing interval
- `--transit-worker-window` - Lookback window
- `--transit-worker-threshold` - Speed threshold
- `--transit-worker-model` - Model version string

**Tuning Config:**

- `--config tuning.json` - Path to JSON tuning config file (see [config/CONFIG.md](../../config/CONFIG.md))

Background subtraction parameters (flush interval, noise threshold, frame buffer timeout, min frame points, seed behaviour) are configured via the [tuning config file](../../config/CONFIG.md), not CLI flags.

#### Subcommands

- `version` - Print version information
- `migrate <action>` - Database migration operations (delegates to internal/db)
  - Actions: `up`, `down`, `status`, `detect`, `version`, `force`, `baseline`
  - Accepts `--db-path` flag
- `transits <action>` - Transit session management
  - `analyse` - Analyse transit sessions for a time range
  - `delete` - Delete transit sessions
  - `migrate` - Backfill transits from historical radar data

#### HTTP endpoints served

**Radar API (`:8080` by default):**

- `GET /events` - List radar detection events
- `POST /command` - Send command to serial port
- `GET /api/radar_stats` - Get radar statistics with grouping
- `GET /api/config` - Get server configuration (units, timezone)
- `POST /api/generate_report` - Generate PDF report
- `GET /api/sites`, `POST /api/sites` - List/create monitoring sites
- `GET /api/sites/{id}`, `PUT /api/sites/{id}`, `DELETE /api/sites/{id}` - Site CRUD
- `GET /api/reports` - List all recent reports
- `GET /api/reports/{id}` - Get report metadata
- `GET /api/reports/{id}/download[/filename]` - Download report (PDF or ZIP)
- `DELETE /api/reports/{id}` - Delete report
- `GET /api/reports/site/{siteID}` - List reports for specific site
- `GET /api/capabilities` - Get server capabilities
- `GET /api/site_config_periods` - Get site configuration periods
- `GET /api/timeline` - Get data timeline
- `GET /api/transit_worker` - Transit worker status
- `GET /api/db_stats` - Database statistics
- `GET /app/` - Web frontend (SPA)
- `GET /` - Redirect to `/app/`

**Admin Routes (`:8080/debug/`):**

- `/debug/tailsql/` - SQL debugging interface (tsweb)
- `/debug/backup` - Create and download database backup
- `/debug/send-command` - Send command to serial port (HTML UI)
- `/debug/send-command-api` - Send command to serial port (API)
- `/debug/tail` - Live tail of serial port output
- `/debug/serial-disabled` - Status page when radar disabled

**LiDAR Monitor (`:8081` when `--enable-lidar`):**

- `GET /health` - Health check
- `GET /` - Status page (HTML dashboard)
- `GET /api/lidar/status` - LiDAR system status
- `POST /api/lidar/persist` - Manually trigger background persistence
- `GET /api/lidar/snapshot` - Retrieve latest background snapshot
- `GET /api/lidar/snapshots` - List background snapshots
- `GET /api/lidar/export_snapshot` - Export snapshot as ASC file
- `GET /api/lidar/export_next_frame` - Export next complete frame as ASC
- `GET /api/lidar/acceptance` - Get acceptance metrics
- `POST /api/lidar/acceptance/reset` - Reset acceptance counters
- `GET /api/lidar/params` - Get background parameters
- `POST /api/lidar/params` - Update background parameters
- `GET /api/lidar/grid_status` - Get grid status
- `POST /api/lidar/grid_reset` - Reset background grid
- `GET /api/lidar/grid_heatmap` - Get grid heatmap data
- `GET /api/lidar/data_source` - Get current data source (live/PCAP)
- `POST /api/lidar/pcap/start` - Start PCAP replay
- `POST /api/lidar/pcap/stop` - Stop PCAP replay, return to live
- `POST /api/lidar/pcap/resume_live` - Resume live UDP after PCAP
- `GET /api/lidar/pcap/files` - List available PCAP files
- `POST /api/lidar/snapshots/cleanup` - Clean up old snapshots
- `GET /api/lidar/export_frame_sequence` - Export frame sequence
- `GET /api/lidar/export_foreground` - Export foreground points
- `GET /api/lidar/traffic` - Traffic statistics
- `GET /api/lidar/settling_eval` - Settling evaluation metrics
- `GET /api/lidar/background/grid` - Background grid data

**Track API:**

- `GET /api/lidar/tracks` - List tracks (optional state/sensor filter)
- `GET /api/lidar/tracks/active` - Active tracks (real-time)
- `GET /api/lidar/tracks/{track_id}` - Track details
- `PUT /api/lidar/tracks/{track_id}` - Update track metadata
- `GET /api/lidar/tracks/{track_id}/observations` - Track trajectory
- `GET /api/lidar/tracks/summary` - Aggregated track statistics
- `GET /api/lidar/clusters` - Recent clusters by sensor and time range

**Sweep & Auto-Tune API:**

- `POST /api/lidar/sweep/start` - Start parameter sweep
- `GET /api/lidar/sweep/status` - Sweep progress
- `GET /api/lidar/sweep/results` - Sweep results
- `POST /api/lidar/sweep/stop` - Stop sweep
- Auto-tune and HINT tuner endpoints under `/api/lidar/sweep/`

**Chart & Visualisation API:**

- `GET /api/lidar/chart/*` - JSON chart data endpoints (acceptance, grid, tracks, etc.)

**Playback & VRLOG API:**

- `/api/lidar/playback/*` - Playback control endpoints
- `/api/lidar/vrlog/*` - VRLOG replay endpoints

**Run & Scene API:**

- `/api/lidar/runs/*` - Run management
- `/api/lidar/scenes/*` - Scene management

**Debug Dashboard (`:8081/debug/`):**

- `/debug/lidar/*` - LiDAR debug dashboard and diagnostic views

---

### 2. Sweep binary ([cmd/sweep](../../cmd/sweep))

**Description:** Parameter sweep utility for testing lidar background model with different configurations.

**Mode:** Batch job (runs sweep, writes CSV, exits)

**Location:** [cmd/sweep](../../cmd/sweep)

#### Quick start examples

```bash
# Multi-parameter sweep
sweep --mode multi --output results.csv

# Noise-only sweep
sweep --mode noise --noise-start 0.005 --noise-end 0.03

# PCAP replay mode
sweep --pcap recording.pcap --pcap-settle 20s
```

#### CLI flags

**Core Configuration:**

- `--monitor http://localhost:8081` - Base URL for lidar monitor API
- `--sensor hesai-pandar40p` - Sensor ID
- `--output <file>` - Output CSV filename (defaults to `sweep-<mode>-<timestamp>.csv`)

**PCAP Support:**

- `--pcap <file>` - PCAP file to replay (enables PCAP mode)
- `--pcap-settle 20s` - Wait time after PCAP replay before sampling

**Sweep Mode Selection:**

- `--mode multi` - Sweep mode: `multi`, `noise`, `closeness`, `neighbour`, `tracking`

**Parameter Ranges for Multi-Sweep:**

- `--noise <values>` - Comma-separated noise values or range `start:end:step`
- `--closeness <values>` - Comma-separated closeness values or range
- `--neighbours <values>` - Comma-separated neighbour values

**Single-Variable Sweep Ranges:**

- `--noise-start`, `--noise-end`, `--noise-step` - Noise sweep parameters
- `--closeness-start`, `--closeness-end`, `--closeness-step` - Closeness sweep
- `--neighbour-start`, `--neighbour-end`, `--neighbour-step` - Neighbour sweep

**Fixed Values (for single-variable sweeps):**

- `--fixed-noise 0.01`
- `--fixed-closeness 2.0`
- `--fixed-neighbour 1`

**Sampling Configuration:**

- `--iterations 30` - Samples per parameter combination
- `--interval 2s` - Interval between samples
- `--settle-time 5s` - Time to wait for grid to settle after applying params

**Seed Control:**

- `--seed true` - Seed behaviour: `true`, `false`, or `toggle`

**Total:** 20+ flags

---

### 3. Device management binary ([cmd/velocity-ctl](../../cmd/velocity-ctl))

> **Note:** `velocity-ctl` replaces the deleted `velocity-deploy` binary.
> See [deploy-rpi-imager-fork-plan.md §8](../plans/deploy-rpi-imager-fork-plan.md) for rationale.

**Description:** On-device management tool for velocity.report installations. Handles upgrades, rollback, backup, and status; no SSH, no remote targets.

**Mode:** Interactive CLI tool (subcommand-based)

**Location:** [cmd/velocity-ctl](../../cmd/velocity-ctl)

#### Quick start examples

```bash
# Check status
sudo velocity-ctl status

# Upgrade to latest release
sudo velocity-ctl upgrade

# Rollback to previous version
sudo velocity-ctl rollback
```

#### Subcommands

**`upgrade`**: Download and install latest release from GitHub

**`rollback`**: Revert to previous binary version

**`backup`**: Back up database and configuration

**`--output .`**: Output directory for backup

**`status`**: Show service status and version info

**`version`**: Show velocity-ctl version

---

### 4. Transit backfill (removed)

> Transit backfill functionality is now part of the main binary via the `velocity-report transits migrate` subcommand. The standalone `transit-backfill` binary has been deleted.

---

### 5. Backfill ring elevations binary ([cmd/tools/backfill_ring_elevations](../../cmd/tools/backfill_ring_elevations))

**Description:** Backfill ring elevation data for lidar background snapshots using embedded parser config.

**Mode:** Batch job (updates DB, exits)

**Location:** [cmd/tools/backfill_ring_elevations](../../cmd/tools/backfill_ring_elevations)

#### Quick start examples

```bash
# Dry run (preview)
backfill_ring_elevations --db sensor_data.db --dry-run

# Apply changes
backfill_ring_elevations --db sensor_data.db
```

#### CLI flags

- `--db sensor_data.db` - Path to SQLite database
- `--dry-run` - Don't write changes; just report

---

### Makefile targets (101 total)

**Build Targets (15):**

- `build-radar-linux` - Cross-compile for ARM64 Linux (Raspberry Pi) with pcap
- `build-radar-mac` - Build for macOS ARM64
- `build-radar-mac-intel` - Build for macOS Intel
- `build-radar-local` - Local development build with pcap
- `build-tools` - Build utility tools
- `build-ctl` - Build velocity-ctl binary
- `build-ctl-linux` - Cross-compile velocity-ctl for Linux ARM64
- `build-web` - Build Svelte web frontend
- `build-docs` - Build documentation site

**Development Targets (8):**

- `dev-go` - Run Go server (radar disabled)
- `dev-go-lidar` - Run Go server with lidar enabled (gRPC mode)
- `dev-go-lidar-both` - Run Go server with lidar (both gRPC and 2370 forward)
- `dev-go-kill-server` - Kill running dev server
- `dev-web` - Run web dev server
- `dev-docs` - Run docs dev server

**Testing Targets (5):**

- `test` - Run all tests (Go + Python + Web)
- `test-go` - Go unit tests only
- `test-python` - Python tests
- `test-python-cov` - Python tests with coverage
- `test-web` - Web tests

**Code Quality Targets (8):**

- `format` - Format all code (Go + Python + Web)
- `format-go` - Format Go code
- `format-python` - Format Python code
- `format-web` - Format web code
- `lint` - Lint all code
- `lint-go` - Lint Go code
- `lint-python` - Lint Python code
- `lint-web` - Lint web code

**Database Migration Targets (7):**

- `migrate-up` - Run pending migrations
- `migrate-down` - Rollback last migration
- `migrate-status` - Show migration status
- `migrate-detect` - Detect schema drift
- `migrate-version` - Show current version
- `migrate-force` - Force migration to specific version
- `migrate-baseline` - Set baseline for existing DB

**Deployment Targets (5):**

- `deploy-install` - Install on remote host
- `deploy-upgrade` - Upgrade remote installation
- `deploy-status` - Check remote status
- `deploy-health` - Check remote health
- `setup-radar` - Setup radar hardware

**PDF/Report Targets (5):**

- `pdf-report` - Generate PDF report
- `pdf-config` - Create PDF config template
- `pdf-test` - Test PDF generation
- `pdf-demo` - Generate demo report
- `pdf` - Alias for pdf-report

**Installation Targets (4):**

- `install-python` - Install Python dependencies
- `install-web` - Install web dependencies
- `install-docs` - Install docs dependencies
- `ensure-python-tools` - Ensure Python formatting tools

**Cleanup Targets (1):**

- `clean-python` - Clean Python artifacts

**Monitoring/Logging Targets (2):**

- `log-go-tail` - Tail Go server logs
- `log-go-cat` - Cat Go server logs

---

## Quick reference

### Common workflows

#### Development setup

```bash
# Clone and build
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report
make build-radar-local

# Run without hardware
./velocity-report-local --disable-radar

# Visit web UI
open http://localhost:8080/app/
```

#### Remote deployment

```bash
# Build for Raspberry Pi
make build-radar-linux

# On the Pi: upgrade using velocity-ctl
sudo velocity-ctl upgrade

# Check status
sudo velocity-ctl status
```

#### PDF report generation

```bash
# Via HTTP API
curl -X POST http://localhost:8080/api/generate_report \
  -H "Content-Type: application/json" \
  -d '{
    "start_date": "2024-01-01",
    "end_date": "2024-01-31",
    "timezone": "US/Pacific",
    "units": "mph"
  }'

# Via command line (using Python tools)
cd tools/pdf-generator
make pdf-report CONFIG=my-config.json
```

#### Parameter sweep testing

```bash
# Multi-parameter sweep
sweep \
  --mode multi \
  --noise 0.01,0.02,0.03 \
  --closeness 1.5,2.0,2.5 \
  --neighbours 0,1,2 \
  --iterations 30 \
  --output sweep-results.csv

# Analyse results
make plot-multisweep INPUT=sweep-results.csv
```

### Environment variables

**PDF Generator:**

- `PDF_GENERATOR_DIR` - PDF generator directory (default: auto-detect)
- `PDF_GENERATOR_PYTHON` - Python binary path (default: auto-detect)

**Development:**

- `GOARCH` - Target architecture (e.g., `arm64`)
- `GOOS` - Target OS (e.g., `linux`)

### Tips & best practices

**Database Management:**

- **Production:** Use `--db-path /var/lib/velocity-report/sensor_data.db`
- **Development:** Default `./sensor_data.db` is fine <!-- link-ignore -->
- **Backup:** Use `/debug/backup` endpoint or `velocity-ctl backup` command
- **Migrations:** Always run `migrate status` before upgrading

**Port Usage:**

- **8080** - Main HTTP API and web UI
- **8081** - LiDAR monitor (when enabled)
- **2369** - LiDAR UDP packets (incoming)
- **2368** - LiDAR forwarding (to LidarView)

**Performance Tuning:**

LiDAR background subtraction parameters (frame buffer timeout, flush interval, noise threshold) are configured via the [tuning config file](../../config/CONFIG.md), adjustable at runtime via the HTTP API.
