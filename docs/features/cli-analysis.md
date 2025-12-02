# CLI Architecture Analysis

**Date:** 2025-12-02  
**Purpose:** Analyze all CLI flags, subcommands, and HTTP methods for velocity.report applications

## Executive Summary

velocity.report currently consists of multiple CLI applications with overlapping concerns:
- **1 main application** (radar) - production service with optional lidar integration
- **4 utility applications** - deployment, sweep testing, backfill, and tools
- **59 Makefile targets** - build, test, deploy, and development tasks
- **Multiple HTTP APIs** - radar API (`:8080`), lidar monitor (`:8081`), admin routes (`/debug/`)

This document categorizes the current structure and proposes a more coherent, structured approach while maintaining the single executable model.

---

## Current CLI Applications

### 1. Radar Binary (`cmd/radar`)

**Description:** Main production service that runs radar serial monitoring, HTTP API server, and optional lidar components.

**Mode:** Long-running service

**CLI Flags:**

#### Core Service Flags
- `--listen` (string, default: `:8080`) - HTTP listen address for API server
- `--db-path` (string, default: `sensor_data.db`) - Path to SQLite database file
- `--debug` (bool) - Run in debug mode (mock serial mux, extra logging)
- `--fixture` (bool) - Load fixture data instead of real hardware
- `--version`, `-v` (bool) - Print version information and exit

#### Radar Hardware Flags
- `--port` (string, default: `/dev/ttySC1`) - Serial device path for radar sensor
- `--disable-radar` (bool) - Disable radar serial I/O (serve DB/HTTP only)
- `--units` (string, default: `mph`) - Display units (`mps`, `mph`, `kmph`)
- `--timezone` (string, default: `UTC`) - Timezone for display

#### LiDAR Integration Flags (when `--enable-lidar`)
- `--enable-lidar` (bool) - Enable in-process lidar components
- `--lidar-listen` (string, default: `:8081`) - HTTP listen address for lidar monitor
- `--lidar-udp-port` (int, default: `2369`) - UDP port for lidar packets
- `--lidar-no-parse` (bool) - Disable packet parsing (forwarding only)
- `--lidar-sensor` (string, default: `hesai-pandar40p`) - Sensor identifier
- `--lidar-forward` (bool) - Forward UDP packets to another port
- `--lidar-forward-port` (int, default: `2368`) - Forwarding destination port
- `--lidar-forward-addr` (string, default: `localhost`) - Forwarding destination address
- `--lidar-pcap-dir` (string, default: `../sensor_data/lidar`) - Safe directory for PCAP files

#### LiDAR Background Tuning Flags
- `--lidar-bg-flush-interval` (duration, default: `60s`) - Background grid flush interval
- `--lidar-bg-noise-relative` (float, default: `0.315`) - Background noise relative fraction
- `--lidar-frame-buffer-timeout` (duration, default: `500ms`) - Frame buffer timeout
- `--lidar-min-frame-points` (int, default: `1000`) - Minimum points for valid frame
- `--lidar-seed-from-first` (bool, default: `true`) - Seed background from first observation

**Subcommands:**
- `version` - Print version information
- `migrate <action>` - Database migration operations (delegates to internal/db)
  - Actions: `up`, `down`, `status`, `detect`, `version`, `force`, `baseline`
  - Accepts `--db-path` flag

**HTTP Endpoints Served:**

*Radar API (`:8080` by default):*
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
- `GET /app/` - Web frontend (SPA)
- `GET /` - Redirect to `/app/`

*Admin Routes (`:8080/debug/`):*
- `/debug/tailsql/` - SQL debugging interface (tsweb)
- `/debug/backup` - Create and download database backup
- `/debug/send-command` - Send command to serial port (HTML UI)
- `/debug/send-command-api` - Send command to serial port (API)
- `/debug/live-tail` - Live tail of serial port output
- `/debug/serial-disabled` - Status page when radar disabled

*LiDAR Monitor (`:8081` when `--enable-lidar`):*
- `GET /health` - Health check
- `GET /` - Status page (HTML dashboard)
- `GET /api/lidar/status` - LiDAR system status
- `POST /api/lidar/persist` - Manually trigger background persistence
- `POST /api/lidar/snapshot` - Trigger background snapshot
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

---

### 2. Sweep Binary (`cmd/sweep`)

**Description:** Parameter sweep utility for testing lidar background model with different configurations.

**Mode:** Batch job (runs sweep, writes CSV, exits)

**CLI Flags:**

#### Core Configuration
- `--monitor` (string, default: `http://localhost:8081`) - Base URL for lidar monitor API
- `--sensor` (string, default: `hesai-pandar40p`) - Sensor ID
- `--output` (string) - Output CSV filename (defaults to `sweep-<mode>-<timestamp>.csv`)

#### PCAP Support
- `--pcap` (string) - PCAP file to replay (enables PCAP mode)
- `--pcap-settle` (duration, default: `20s`) - Wait time after PCAP replay before sampling

#### Sweep Mode Selection
- `--mode` (string, default: `multi`) - Sweep mode: `multi`, `noise`, `closeness`, `neighbor`

#### Parameter Ranges for Multi-Sweep
- `--noise` (string) - Comma-separated noise values or range `start:end:step`
- `--closeness` (string) - Comma-separated closeness values or range
- `--neighbors` (string) - Comma-separated neighbor values

#### Single-Variable Sweep Ranges
- `--noise-start`, `--noise-end`, `--noise-step` (float) - Noise sweep parameters
- `--closeness-start`, `--closeness-end`, `--closeness-step` (float) - Closeness sweep
- `--neighbor-start`, `--neighbor-end`, `--neighbor-step` (int) - Neighbor sweep

#### Fixed Values (for single-variable sweeps)
- `--fixed-noise` (float, default: `0.01`)
- `--fixed-closeness` (float, default: `2.0`)
- `--fixed-neighbor` (int, default: `1`)

#### Sampling Configuration
- `--iterations` (int, default: `30`) - Samples per parameter combination
- `--interval` (duration, default: `2s`) - Interval between samples
- `--settle-time` (duration, default: `5s`) - Time to wait for grid to settle after applying params

#### Seed Control
- `--seed` (string, default: `true`) - Seed behavior: `true`, `false`, or `toggle`

**Subcommands:** None

**HTTP Endpoints Called:**
- `POST /api/lidar/pcap/start` - Start PCAP replay
- `GET /api/lidar/acceptance` - Fetch acceptance metrics
- `POST /api/lidar/grid_reset` - Reset background grid
- `POST /api/lidar/params` - Set background parameters
- `POST /api/lidar/acceptance/reset` - Reset acceptance counters
- `GET /api/lidar/grid_status` - Check grid status

---

### 3. Deploy Binary (`cmd/deploy`)

**Description:** Deployment manager for velocity.report service on remote hosts.

**Mode:** Interactive CLI tool (subcommand-based)

**CLI Flags (Common):**
- `--target` (string, default: `localhost`) - Target host (hostname, IP, or SSH config alias)
- `--ssh-user` (string) - SSH user (defaults to `~/.ssh/config` or current user)
- `--ssh-key` (string) - SSH private key path (defaults to `~/.ssh/config`)
- `--debug` (bool) - Enable debug logging
- `--dry-run` (bool) - Show what would be done without executing (some commands)

**Subcommands:**

#### `install` - Install velocity.report service
- `--binary` (string, **required**) - Path to velocity-report binary
- `--db-path` (string) - Path to existing database to migrate
- `--dry-run` (bool)

#### `upgrade` - Upgrade to newer version
- `--binary` (string, **required**) - Path to new binary
- `--no-backup` (bool) - Skip backup before upgrade
- `--dry-run` (bool)

#### `fix` - Diagnose and repair broken installation
- `--binary` (string) - Path to binary (optional, for fixing missing binary)
- `--repo-url` (string, default: `https://github.com/banshee-data/velocity.report`) - Git repo URL
- `--build-from-source` (bool) - Build binary from source on server (requires Go)
- `--dry-run` (bool)

#### `status` - Check service status
- `--api-port` (int, default: `8080`) - API server port
- `--timeout` (int, default: `30`) - Timeout in seconds
- `--scan` (bool) - Perform detailed disk scan

#### `health` - Perform health check
- `--api-port` (int, default: `8080`) - API server port

#### `rollback` - Rollback to previous version
- `--dry-run` (bool)

#### `backup` - Backup database and configuration
- `--output` (string, default: `.`) - Output directory for backup

#### `config` - Manage deployment configuration
- `--show` (bool) - Show current configuration
- `--edit` (bool) - Edit configuration

#### `version` - Show velocity-deploy version

#### `help` - Show help message

**SSH Config Support:**
- Reads `~/.ssh/config` automatically
- Uses HostName, User, IdentityFile from config
- Command-line flags override SSH config values

---

### 4. Transit Backfill Binary (`cmd/transit-backfill`)

**Description:** Backfill radar_data_transits table from historical radar_data events.

**Mode:** Batch job (processes time range, exits)

**CLI Flags:**
- `--db` (string, default: `sensor_data.db`) - Path to SQLite database
- `--start` (string, **required**) - Start time (RFC3339 format)
- `--end` (string, **required**) - End time (RFC3339 format)
- `--gap` (int, default: `1`) - Session gap in seconds
- `--model` (string, default: `manual-backfill`) - Model version string for transits

**Subcommands:** None

**Operation:**
- Processes data in 20-minute windows
- Uses `internal/db.TransitWorker` to sessionize radar events
- Writes radar_data_transits records with specified model version

---

### 5. Backfill Ring Elevations Binary (`cmd/tools/backfill_ring_elevations`)

**Description:** Backfill ring elevation data for lidar background snapshots using embedded parser config.

**Mode:** Batch job (updates DB, exits)

**CLI Flags:**
- `--db` (string, default: `sensor_data.db`) - Path to SQLite database
- `--dry-run` (bool) - Don't write changes; just report

**Subcommands:** None

**Operation:**
- Loads embedded Pandar40P configuration
- Extracts ring elevations
- Updates `lidar_bg_snapshot` table records with elevation data
- Reports: total records processed, updated count, skipped count

---

## Makefile Targets

### Build Targets
- `build-radar-linux` - Cross-compile for ARM64 Linux (Raspberry Pi)
- `build-radar-linux-pcap` - Linux build with libpcap support
- `build-radar-mac` - Build for macOS ARM64
- `build-radar-mac-intel` - Build for macOS Intel
- `build-radar-local` - Local development build with pcap
- `build-tools` - Build utility tools
- `build-deploy` - Build deploy binary
- `build-deploy-linux` - Cross-compile deploy for Linux
- `build-web` - Build Svelte web frontend
- `build-docs` - Build documentation site

### Development Targets
- `dev-go` - Run Go server (radar disabled)
- `dev-go-lidar` - Run Go server with lidar enabled
- `dev-go-kill-server` - Kill running dev server
- `dev-web` - Run web dev server
- `dev-docs` - Run docs dev server

### Testing Targets
- `test` - Run all tests (Go + Python + Web)
- `test-go` - Go unit tests only
- `test-python` - Python tests
- `test-python-cov` - Python tests with coverage
- `test-web` - Web tests

### Database Migration Targets
- `migrate-up` - Run pending migrations
- `migrate-down` - Rollback last migration
- `migrate-status` - Show migration status
- `migrate-detect` - Detect schema drift
- `migrate-version` - Show current version
- `migrate-force` - Force migration to specific version
- `migrate-baseline` - Set baseline for existing DB

### Code Quality Targets
- `format` - Format all code (Go + Python + Web)
- `format-go` - Format Go code
- `format-python` - Format Python code
- `format-web` - Format web code
- `lint` - Lint all code
- `lint-go` - Lint Go code
- `lint-python` - Lint Python code
- `lint-web` - Lint web code

### Python/PDF Targets
- `install-python` - Install Python dependencies
- `pdf-report` - Generate PDF report
- `pdf-config` - Create PDF config template
- `pdf-test` - Test PDF generation
- `pdf-demo` - Generate demo report
- `pdf` - Alias for pdf-report
- `clean-python` - Clean Python artifacts

### Installation Targets
- `install-web` - Install web dependencies
- `install-docs` - Install docs dependencies
- `ensure-python-tools` - Ensure Python formatting tools

### Deployment Targets
- `deploy-install` - Install on remote host
- `deploy-upgrade` - Upgrade remote installation
- `deploy-status` - Check remote status
- `deploy-health` - Check remote health
- `deploy-install-latex` - Install LaTeX on deploy target
- `deploy-update-deps` - Update Python deps on remote
- `setup-radar` - Setup radar hardware

### Monitoring/Logging Targets
- `log-go-tail` - Tail Go server logs
- `log-go-cat` - Cat Go server logs
- `stats-live` - Show live statistics
- `stats-pcap` - Show PCAP statistics

### Plotting/Analysis Targets
- `plot-noise-sweep` - Plot noise sweep results
- `plot-multisweep` - Plot multi-parameter sweep
- `plot-noise-buckets` - Plot noise bucket analysis

---

## Current Architecture Observations

### Strengths
1. **Single Executable Model** - `radar` binary handles both radar and lidar, reducing deployment complexity
2. **Clear Separation** - Different binaries for different concerns (production service vs utilities)
3. **HTTP API Design** - RESTful patterns, clear resource paths
4. **Admin Routes Separation** - `/debug/` prefix for administrative/diagnostic endpoints
5. **Flexible Configuration** - Extensive CLI flags allow fine-tuning without code changes
6. **SSH Config Integration** - Deploy tool reads `~/.ssh/config` for convenience

### Areas for Improvement
1. **Flag Organization** - 30+ flags on radar binary with no clear grouping
2. **Subcommand Inconsistency** - Only `radar` has subcommands (`version`, `migrate`)
3. **Utility Tool Fragmentation** - 4 separate binaries for utilities (sweep, deploy, backfill, tools)
4. **HTTP Method Inconsistency** - Mix of patterns (query params vs path params vs body)
5. **Documentation Gaps** - No single reference for all CLI flags and HTTP endpoints
6. **Make Target Naming** - 59 targets without consistent naming scheme
7. **Port Configuration** - Hard-coded defaults (8080, 8081) not consistent across tools

---

## Proposed Structure

### Goal
Create a coherent, structured CLI approach while maintaining the single executable model for production, with clear organization for utilities.

### Proposed Changes

#### 1. Enhanced Subcommand Structure for `velocity-report`

```
velocity-report [global-flags] <command> [command-flags]

Global Flags:
  --config <file>          Configuration file (TOML/YAML/JSON)
  --db-path <path>         Database path (default: sensor_data.db)
  --debug                  Enable debug mode
  --version, -v            Show version

Commands:
  serve                    Run production server (default)
  migrate                  Database migrations
  version                  Show version information
  help                     Show help
```

**`serve` command (default if no subcommand):**
```
velocity-report serve [flags]

Radar Flags:
  --port <device>          Serial port (default: /dev/ttySC1)
  --disable-radar          Disable radar hardware
  --units <unit>           Display units: mps|mph|kmph (default: mph)
  --timezone <tz>          Timezone (default: UTC)

HTTP Flags:
  --listen <addr>          HTTP listen address (default: :8080)

LiDAR Flags:
  --enable-lidar           Enable lidar components
  --lidar-listen <addr>    LiDAR monitor address (default: :8081)
  --lidar-udp-port <port>  UDP port for lidar (default: 2369)
  --lidar-sensor <name>    Sensor identifier
  --lidar-forward          Enable packet forwarding
  --lidar-forward-addr     Forward address
  --lidar-forward-port     Forward port
  --lidar-pcap-dir <dir>   PCAP safe directory

LiDAR Tuning Flags:
  --lidar-bg-flush-interval      Background flush interval
  --lidar-bg-noise-relative      Noise relative fraction
  --lidar-frame-buffer-timeout   Frame buffer timeout
  --lidar-min-frame-points       Minimum frame points
  --lidar-seed-from-first        Seed from first observation

Development Flags:
  --fixture                Load fixture data
```

**`migrate` command:**
```
velocity-report migrate <action> [flags]

Actions:
  up                       Run pending migrations
  down                     Rollback last migration
  status                   Show migration status
  detect                   Detect schema drift
  version                  Show current version
  force <version>          Force to specific version
  baseline                 Set baseline for existing DB

Flags:
  --db-path <path>         Database path
```

#### 2. Consolidated Utilities Binary: `velocity-tools`

```
velocity-tools <command> [flags]

Commands:
  sweep                    Run parameter sweep tests
  backfill-transits        Backfill transit sessions
  backfill-elevations      Backfill ring elevations
  deploy                   Deployment operations
  help                     Show help

Global Flags:
  --db-path <path>         Database path (default: sensor_data.db)
  --debug                  Enable debug logging
  --version, -v            Show version
```

**`sweep` command:**
```
velocity-tools sweep [flags]

Monitor Configuration:
  --monitor <url>          Monitor URL (default: http://localhost:8081)
  --sensor <id>            Sensor ID (default: hesai-pandar40p)
  --output <file>          Output CSV filename

Mode Selection:
  --mode <mode>            Sweep mode: multi|noise|closeness|neighbor

Parameter Ranges:
  --noise <values>         Noise values (comma-separated or range)
  --closeness <values>     Closeness values
  --neighbors <values>     Neighbor values

Sampling:
  --iterations <n>         Samples per combination (default: 30)
  --interval <duration>    Sample interval (default: 2s)
  --settle-time <duration> Settle time (default: 5s)

PCAP:
  --pcap <file>            PCAP file to replay
  --pcap-settle <duration> PCAP settle time (default: 20s)

Seed:
  --seed <mode>            Seed mode: true|false|toggle
```

**`backfill-transits` command:**
```
velocity-tools backfill-transits [flags]

Required:
  --start <time>           Start time (RFC3339)
  --end <time>             End time (RFC3339)

Optional:
  --gap <seconds>          Session gap (default: 1)
  --model <version>        Model version (default: manual-backfill)
```

**`backfill-elevations` command:**
```
velocity-tools backfill-elevations [flags]

Optional:
  --dry-run                Don't write changes
```

**`deploy` command:**
```
velocity-tools deploy <action> [flags]

Actions:
  install                  Install service on host
  upgrade                  Upgrade to newer version
  fix                      Diagnose and repair
  status                   Check service status
  health                   Health check
  rollback                 Rollback to previous
  backup                   Backup database
  config                   Manage configuration

Connection:
  --target <host>          Target host (default: localhost)
  --ssh-user <user>        SSH user
  --ssh-key <path>         SSH key path

Common:
  --dry-run                Show what would be done
  --debug                  Enable debug logging
```

#### 3. HTTP API Reorganization

**Principle:** Group by resource, use consistent HTTP methods, separate admin from production API.

**Production API (`:8080`):**

```
# Radar Data
GET    /api/v1/events                    List radar events
GET    /api/v1/stats                     Radar statistics (with grouping)

# Sites (Monitoring Locations)
GET    /api/v1/sites                     List sites
POST   /api/v1/sites                     Create site
GET    /api/v1/sites/{id}                Get site
PUT    /api/v1/sites/{id}                Update site
DELETE /api/v1/sites/{id}                Delete site

# Reports
GET    /api/v1/reports                   List all reports
POST   /api/v1/reports                   Generate report
GET    /api/v1/reports/{id}              Get report metadata
DELETE /api/v1/reports/{id}              Delete report
GET    /api/v1/reports/{id}/pdf          Download PDF
GET    /api/v1/reports/{id}/sources      Download sources ZIP
GET    /api/v1/sites/{id}/reports        List reports for site

# Configuration
GET    /api/v1/config                    Get server config

# Web Frontend
GET    /app/                             Web UI
GET    /                                 Redirect to /app/
```

**Admin API (`:8080/admin`):**

```
# Database
GET    /admin/db/backup                  Download database backup
GET    /admin/db/sql                     SQL debugging interface

# Hardware
POST   /admin/radar/command              Send radar command
GET    /admin/radar/status               Radar status
GET    /admin/radar/tail                 Live tail serial output
```

**LiDAR Monitor API (`:8081`):**

```
# Status & Health
GET    /health                           Health check
GET    /                                 Status dashboard (HTML)
GET    /api/status                       System status (JSON)

# Background Model
GET    /api/background/params            Get parameters
POST   /api/background/params            Update parameters
GET    /api/background/grid              Grid status
POST   /api/background/grid/reset        Reset grid
GET    /api/background/grid/heatmap      Grid heatmap

# Acceptance Metrics
GET    /api/acceptance                   Get metrics
POST   /api/acceptance/reset             Reset counters

# Snapshots
POST   /api/snapshots                    Trigger snapshot
GET    /api/snapshots                    List snapshots
GET    /api/snapshots/{id}/persist       Manually persist
GET    /api/snapshots/{id}/export        Export as ASC

# Frames
GET    /api/frames/next                  Export next frame as ASC

# Data Source
GET    /api/datasource                   Get current source
POST   /api/datasource/pcap/start        Start PCAP replay
POST   /api/datasource/pcap/stop         Stop PCAP, return to live
```

#### 4. Makefile Target Reorganization

**Principle:** Consistent naming `<action>-<component>[-<variant>]`

```makefile
# Help
help                     Show all targets

# Build Targets
build-server             Build main server binary
build-server-linux       Cross-compile for Linux ARM64
build-server-mac         Build for macOS
build-tools              Build utility tools
build-web                Build web frontend
build-docs               Build documentation
build-all                Build everything

# Development Targets
dev-server               Run dev server (radar disabled)
dev-server-lidar         Run dev server with lidar
dev-web                  Run web dev server
dev-docs                 Run docs dev server
dev-kill                 Kill running dev servers

# Test Targets
test                     Run all tests
test-go                  Go tests
test-python              Python tests
test-web                 Web tests

# Code Quality Targets
format                   Format all code
format-go                Format Go
format-python            Format Python
format-web               Format Web
lint                     Lint all code
lint-go                  Lint Go
lint-python              Lint Python
lint-web                 Lint Web

# Database Targets
db-migrate-up            Apply migrations
db-migrate-down          Rollback migration
db-migrate-status        Show status
db-migrate-detect        Detect drift
db-backup                Backup database

# Deployment Targets
deploy-install           Install on remote
deploy-upgrade           Upgrade remote
deploy-status            Check remote status
deploy-health            Check remote health

# PDF/Report Targets
pdf-generate             Generate PDF report
pdf-config               Create config template
pdf-test                 Test PDF generation

# Analysis Targets
analysis-sweep           Run parameter sweep
analysis-stats-live      Live statistics
analysis-stats-pcap      PCAP statistics
analysis-plot-sweep      Plot sweep results

# Installation Targets
install-python           Install Python deps
install-web              Install web deps
install-docs             Install docs deps
install-all              Install all deps

# Cleanup Targets
clean-python             Clean Python artifacts
clean-web                Clean web build
clean-all                Clean everything
```

---

## Implementation Recommendations

### Phase 1: Documentation & Analysis (Current)
- [x] Complete CLI analysis document
- [ ] Review with team
- [ ] Finalize proposed structure

### Phase 2: Non-Breaking Improvements
1. **Add subcommands to radar binary** (backward compatible)
   - Add `serve` as default subcommand
   - Keep existing flags working without subcommand
   - Add `velocity-report serve` as explicit option

2. **Version HTTP APIs** 
   - Add `/api/v1/` prefix to new endpoints
   - Keep existing endpoints for backward compatibility
   - Document deprecation timeline

3. **Reorganize Makefile**
   - Add new consistent target names
   - Keep old targets as aliases initially
   - Document new naming scheme

### Phase 3: Consolidation
1. **Create `velocity-tools` binary**
   - Consolidate sweep, backfill utilities
   - Move deploy functionality
   - Maintain backward compatibility with old binaries initially

2. **Deprecate old binaries**
   - Mark as deprecated in documentation
   - Continue building for 2-3 releases
   - Remove in major version bump

### Phase 4: Breaking Changes (Major Version)
1. **Remove deprecated endpoints**
2. **Remove old binaries**
3. **Clean up Makefile aliases**
4. **Finalize CLI structure**

---

## Configuration File Support (Future Enhancement)

**Proposed:** Add support for configuration files to reduce CLI flag clutter.

**Format:** TOML (primary), YAML, or JSON

**Example:** `velocity.toml`

```toml
# velocity.report configuration file

[server]
listen = ":8080"
db_path = "/var/lib/velocity-report/sensor_data.db"
units = "mph"
timezone = "US/Pacific"
debug = false

[radar]
enabled = true
port = "/dev/ttySC1"

[lidar]
enabled = true
listen = ":8081"
udp_port = 2369
sensor = "hesai-pandar40p"
pcap_dir = "/var/lib/velocity-report/lidar"

[lidar.forwarding]
enabled = false
address = "localhost"
port = 2368

[lidar.background]
flush_interval = "60s"
noise_relative = 0.315
seed_from_first = true

[lidar.frame]
buffer_timeout = "500ms"
min_points = 1000
```

**Priority:** CLI flags override config file values

**Usage:**
```bash
velocity-report --config /etc/velocity-report.toml serve
```

---

## Summary

This analysis provides a comprehensive view of the current CLI structure and proposes a coherent path forward. The key principles of the proposed structure are:

1. **Single Executable Model** - Maintain for production (`velocity-report`)
2. **Utility Consolidation** - Group utilities under `velocity-tools`
3. **Subcommand Organization** - Clear command hierarchy
4. **Consistent Naming** - Predictable patterns for flags, endpoints, and targets
5. **Backward Compatibility** - Phased migration with deprecation warnings
6. **Configuration Files** - Reduce flag clutter for complex deployments
7. **API Versioning** - Enable evolution without breaking changes

The proposed changes balance immediate improvements with long-term maintainability while respecting existing deployments.
