# CLI reference guide

Complete reference for the unified `velocity` command-line interface as currently implemented: namespaces, flags, subcommands, HTTP endpoints, and Makefile targets. `velocity` is a single multi-call binary; `velocity-report` is retained only as a server-oriented compatibility alias (it dispatches straight to the `serve` surface).

---

## Table of contents

1. [Overview](#overview)
2. [Current State Inventory](#current-state-inventory)
3. [Quick Reference](#quick-reference)

---

## Overview

velocity.report ships **one multi-call binary**, `velocity`, dispatched on `os.Args[0]` and the first argument into namespaces:

| Namespace          | Purpose                                                                           | Backed by                                        |
| ------------------ | --------------------------------------------------------------------------------- | ------------------------------------------------ |
| `velocity serve`   | Run the radar/LiDAR server (HTTP API, transit worker)                             | [internal/cmd/server](../../internal/cmd/server) |
| `velocity device`  | On-device lifecycle: check, upgrade, rollback, backup, status, tailscale, install | [internal/cmd/device](../../internal/cmd/device) |
| `velocity data`    | Database operations: `migrate`, `transits`, `sql`                                 | [internal/cmd/server](../../internal/cmd/server) |
| `velocity report`  | Generate PDF reports: `pdf`                                                       | [internal/cmd/server](../../internal/cmd/server) |
| `velocity tune`    | Parameter tuning: `sweep`                                                         | [internal/cmd/tune](../../internal/cmd/tune)     |
| `velocity version` | Print version information                                                         | [internal/version](../../internal/version)       |
| `velocity help`    | Top-level usage                                                                   | [internal/cmd/root](../../internal/cmd/root)     |

Compatibility alias: `velocity-report [serve flags]` (and suffixed dev/release artefact names like `velocity-report-local`) routes to the server surface. The former separate binaries — `velocity-ctl`, `sweep`, and the `velocity-update` redirect stub — no longer exist; their functions are namespaces above.

- **100+ Makefile targets**: build, test, and development tasks
- **Multiple HTTP APIs**: radar API (`:8080`), LiDAR monitor (`:8081`), admin routes (`/debug/`)

This document covers what exists and works today.

---

## Current state inventory

### 1. Server namespace — `velocity serve` ([internal/cmd/server](../../internal/cmd/server))

**Description:** Main production service that runs radar serial monitoring, HTTP API server, and optional lidar components.

**Mode:** Long-running service

**Location:** [internal/cmd/server](../../internal/cmd/server)

#### Quick start examples

```bash
# Production mode
velocity serve --db-path /var/lib/velocity-report/sensor_data.db

# Development (no hardware)
velocity serve --disable-radar --debug

# With lidar enabled
velocity serve --enable-lidar --lidar-listen :8081
```

> The systemd unit and the `velocity-report` compatibility alias invoke this
> surface directly: `velocity-report --db-path …` is equivalent to
> `velocity serve --db-path …`.

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

#### Data & report subcommands

The server binary also backs the `data` and `report` namespaces (the dispatcher
forwards `velocity data …` and `velocity report …` here):

- `velocity data migrate <action>` - Database migration operations (delegates to internal/db)
  - Actions: `up`, `down`, `status`, `detect`, `version`, `force`, `baseline`
  - Accepts `--db-path` flag
- `velocity data transits <action>` - Transit session management
  - `analyse` - Analyse transit sessions for a time range
  - `delete` - Delete transit sessions
  - `migrate` - Backfill transits from historical radar data
- `velocity data sql [--db-path <file>] [--limit N] "<SQL>"` - Run a single
  **read-only** query for operator inspection (replaces the dropped `sqlite3`
  package). Read-only is enforced (`mode=ro`); `--read-only=false` is rejected.
  Defaults: `--limit 100`, `--db-path` resolves the runtime database.
- `velocity report pdf --config report.json --db sensor_data.db --output ./reports` - Generate a PDF report
- `velocity version` - Print version information

#### HTTP endpoints served

**Radar API (`:8080` by default):**

- `GET /api/events` - List radar detection events
- `POST /admin/radar/command` - Send command to serial port (mutating device control; admin namespace)
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
- `GET /api/capabilities` - Get named `radar`/`lidar` capability maps. Radar is reported as the built-in `default` sensor; LiDAR is `{}` when `--enable-lidar` is off and currently reports `lidar.default.status = "starting"` when enabled until ready/error lifecycle callbacks are wired.
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
- `POST /api/lidar/replay/stop` - Stop whatever is replaying (PCAP or VRLOG), return to live
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

### 2. Tune namespace — `velocity tune sweep` ([internal/cmd/tune](../../internal/cmd/tune))

**Description:** Parameter sweep utility for testing lidar background model with different configurations.

**Mode:** Batch job (runs sweep, writes CSV, exits)

**Location:** [internal/cmd/tune](../../internal/cmd/tune)

#### Quick start examples

```bash
# Multi-parameter sweep
velocity tune sweep --mode multi --output results.csv

# Noise-only sweep
velocity tune sweep --mode noise --noise-start 0.005 --noise-end 0.03

# PCAP replay mode
velocity tune sweep --pcap recording.pcap --pcap-settle 20s
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

### 3. Device namespace — `velocity device` ([internal/cmd/device](../../internal/cmd/device))

> **Note:** this namespace replaces the former standalone `velocity-ctl` binary
> (itself the successor to the deleted `velocity-deploy`). There is no
> `velocity-ctl` shim — invoke `velocity device …` directly.

**Description:** On-device management for velocity.report installations. Handles upgrade checks, upgrades, rollback, backup, status, the Tailscale toggle, and writing embedded deploy files; no SSH, no remote targets. Always runs as root on-device.

**Mode:** Interactive CLI tool (subcommand-based)

**Location:** [internal/cmd/device](../../internal/cmd/device)

#### Quick start examples

```bash
# Is a newer release available? (read-only, no download)
sudo velocity device check

# Upgrade to latest release (versioned install + atomic symlink swap)
sudo velocity device upgrade

# Rollback to the previous version (one atomic swap)
sudo velocity device rollback

# Show service status
sudo velocity device status
```

#### Subcommands

**`check`**: Report whether a newer release is available (read-only; no download). Equivalent to `upgrade --check`.

**`upgrade`**: Check for and apply new releases. Stages the new binary under `/opt/velocity-report/versions/<v>/`, backs up the database (consistent `VACUUM INTO` snapshot), migrates with the new binary, then swaps the `current` symlink atomically (`renameat2`) and restarts the service. Flags: `--check` (compare only), `--binary <file>` (apply a local binary for an offline upgrade).

**`rollback`**: Revert to the previous version via a single atomic symlink swap. Does **not** down-migrate the database — restore the matching backup if the current version applied a forward-incompatible migration.

**`backup`**: Snapshot binary + database. `--output <dir>` sets the backup directory.

**`status`**: Show service status and version info.

**`tailscale <enable-tailscaled|disable-tailscaled>`**: Manage the `tailscaled` lifecycle. Invoked via a narrow sudoers grant by the web UI toggle, not usually by hand.

**`install <network|udev|wifi>`**: Write an embedded deploy file to its canonical system path (`/etc/network/interfaces.d/lidar`, `/etc/udev/rules.d/99-velocity-report.rules`, or `/etc/wpa_supplicant/wpa_supplicant.conf`). Idempotent; used by the image build stages.

**`version`**: Print version information.

---

### 4. Transit backfill (removed)

> Transit backfill functionality is now part of the main binary via the `velocity data transits migrate` subcommand. The standalone `transit-backfill` binary has been deleted.

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

**Build Targets:**

- `build-velocity` - Build the single multi-call `velocity` binary (→ `./velocity`)
- `build-velocity-linux` - Host cross-compile compatibility target for ARM64 Linux
- `build-velocity-mac` / `build-velocity-mac-intel` - Build `velocity` for macOS ARM64 / Intel
- `build-radar-local` - Local development build with pcap (→ `./velocity-report-local`)
- `build-radar-linux` / `build-radar-mac` / `build-radar-mac-intel` - Compatibility aliases for the `build-velocity-*` targets
- `build-radar-static-arm64` - Static Raspberry Pi image/release-candidate build using Docker, zig/musl, and vendored libpcap
- `build-radar-linux-docker` - Stage the static ARM64 image binary under `image/velocity-binaries/`
- `build-ctl` / `build-ctl-linux` / `build-tools` - Compatibility aliases that build the same `velocity` binary (no separate `velocity-ctl`)
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

- `test` - Run aggregate tests (Go + Web + macOS)
- `test-go` - Go unit tests only
- `test-python` - Python script/tool tests
- `test-python-cov` - Python script/tool tests with coverage
- `test-web` - Web tests

**Code Quality Targets (8):**

- `format` - Format all code (Go + Python tools + Web)
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

> **Deployment targets removed:** `deploy-install`, `deploy-upgrade`,
> `deploy-status`, `deploy-health`, and `setup-radar` (and the `cmd/deploy`
> tool) were deleted in v0.5.1. On-device lifecycle is now `velocity device …`
> (run on the Pi); there is no host-to-device push tool.

**Installation Targets (4):**

- `install-python` - Install Python dependencies
- `install-web` - Install web dependencies
- `install-docs` - Install docs dependencies
- `ensure-python-tools` - Ensure Python formatting tools

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
# Build the static Raspberry Pi artifact used by image/release packaging
make build-radar-static-arm64

# On the Pi: upgrade using the device namespace
sudo velocity device upgrade

# Check status
sudo velocity device status
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

```

#### Parameter sweep testing

```bash
# Multi-parameter sweep
velocity tune sweep \
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

**Development:**

- `GOARCH` - Target architecture (e.g., `arm64`)
- `GOOS` - Target OS (e.g., `linux`)

### Tips & best practices

**Database Management:**

- **Production:** Use `--db-path /var/lib/velocity-report/sensor_data.db`
- **Development:** Default `./sensor_data.db` is fine <!-- link-ignore -->
- **Backup:** Use `/debug/backup` endpoint or `velocity device backup` command
- **Migrations:** Always run `velocity data migrate status` before upgrading

**Port Usage:**

- **8080** - Main HTTP API and web UI
- **8081** - LiDAR monitor (when enabled)
- **2369** - LiDAR UDP packets (incoming)
- **2368** - LiDAR forwarding (to LidarView)

**Performance Tuning:**

LiDAR background subtraction parameters (frame buffer timeout, flush interval, noise threshold) are configured via the [tuning config file](../../config/CONFIG.md), adjustable at runtime via the HTTP API.
