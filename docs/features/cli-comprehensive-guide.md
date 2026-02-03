# CLI Comprehensive Guide & Implementation Plan

**Date:** 2025-12-02
**Purpose:** Complete reference and restructuring plan for velocity.report CLI interfaces

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Current State Inventory](#current-state-inventory)
3. [Quick Reference](#quick-reference)
4. [Problems & Opportunities](#problems--opportunities)
5. [Proposed Solution](#proposed-solution)
6. [Implementation Plan](#implementation-plan)
7. [Configuration File Support](#configuration-file-support)

---

## Executive Summary

velocity.report currently consists of multiple CLI applications with overlapping concerns:

- **1 main application** (radar) - production service with optional lidar integration
- **4 utility applications** - deployment, sweep testing, backfill, and tools
- **59 Makefile targets** - build, test, deploy, and development tasks
- **Multiple HTTP APIs** - radar API (`:8080`), lidar monitor (`:8081`), admin routes (`/debug/`)

This document provides both a complete reference for the current CLI structure and a detailed plan for improving it while maintaining the single executable model and backward compatibility.

---

## Current State Inventory

### 1. Radar Binary (`cmd/radar`)

**Description:** Main production service that runs radar serial monitoring, HTTP API server, and optional lidar components.

**Mode:** Long-running service

**Location:** `cmd/radar`

#### Quick Start Examples

```bash
# Production mode
velocity-report --db-path /var/lib/velocity-report/sensor_data.db

# Development (no hardware)
velocity-report --disable-radar --debug

# With lidar enabled
velocity-report --enable-lidar --lidar-listen :8081
```

#### CLI Flags

**Core Service Flags (5):**

- `--listen :8080` - HTTP listen address for API server
- `--db-path sensor_data.db` - Path to SQLite database file
- `--debug` - Run in debug mode (mock serial mux, extra logging)
- `--fixture` - Load fixture data instead of real hardware
- `--version`, `-v` - Print version information and exit

**Radar Hardware Flags (4):**

- `--port /dev/ttySC1` - Serial device path for radar sensor
- `--disable-radar` - Disable radar serial I/O (serve DB/HTTP only)
- `--units mph` - Display units (`mps`, `mph`, `kmph`)
- `--timezone UTC` - Timezone for display

**LiDAR Integration Flags (9):**

- `--enable-lidar` - Enable in-process lidar components
- `--lidar-listen :8081` - HTTP listen address for lidar monitor
- `--lidar-udp-port 2369` - UDP port for lidar packets
- `--lidar-no-parse` - Disable packet parsing (forwarding only)
- `--lidar-sensor hesai-pandar40p` - Sensor identifier
- `--lidar-forward` - Forward UDP packets to another port
- `--lidar-forward-port 2368` - Forwarding destination port
- `--lidar-forward-addr localhost` - Forwarding destination address
- `--lidar-pcap-dir ../sensor_data/lidar` - Safe directory for PCAP files

**LiDAR Background Tuning Flags (5):**

- `--lidar-bg-flush-interval 60s` - Background grid flush interval
- `--lidar-bg-noise-relative 0.315` - Background noise relative fraction
- `--lidar-frame-buffer-timeout 500ms` - Frame buffer timeout
- `--lidar-min-frame-points 1000` - Minimum points for valid frame
- `--lidar-seed-from-first true` - Seed background from first observation

**Total:** 30+ flags

#### Subcommands

- `version` - Print version information
- `migrate <action>` - Database migration operations (delegates to internal/db)
  - Actions: `up`, `down`, `status`, `detect`, `version`, `force`, `baseline`
  - Accepts `--db-path` flag

#### HTTP Endpoints Served

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
- `GET /app/` - Web frontend (SPA)
- `GET /` - Redirect to `/app/`

**Admin Routes (`:8080/debug/`):**

- `/debug/tailsql/` - SQL debugging interface (tsweb)
- `/debug/backup` - Create and download database backup
- `/debug/send-command` - Send command to serial port (HTML UI)
- `/debug/send-command-api` - Send command to serial port (API)
- `/debug/live-tail` - Live tail of serial port output
- `/debug/serial-disabled` - Status page when radar disabled

**LiDAR Monitor (`:8081` when `--enable-lidar`):**

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

**Location:** `cmd/sweep`

#### Quick Start Examples

```bash
# Multi-parameter sweep
sweep --mode multi --output results.csv

# Noise-only sweep
sweep --mode noise --noise-start 0.005 --noise-end 0.03

# PCAP replay mode
sweep --pcap recording.pcap --pcap-settle 20s
```

#### CLI Flags

**Core Configuration:**

- `--monitor http://localhost:8081` - Base URL for lidar monitor API
- `--sensor hesai-pandar40p` - Sensor ID
- `--output <file>` - Output CSV filename (defaults to `sweep-<mode>-<timestamp>.csv`)

**PCAP Support:**

- `--pcap <file>` - PCAP file to replay (enables PCAP mode)
- `--pcap-settle 20s` - Wait time after PCAP replay before sampling

**Sweep Mode Selection:**

- `--mode multi` - Sweep mode: `multi`, `noise`, `closeness`, `neighbor`

**Parameter Ranges for Multi-Sweep:**

- `--noise <values>` - Comma-separated noise values or range `start:end:step`
- `--closeness <values>` - Comma-separated closeness values or range
- `--neighbors <values>` - Comma-separated neighbour values

**Single-Variable Sweep Ranges:**

- `--noise-start`, `--noise-end`, `--noise-step` - Noise sweep parameters
- `--closeness-start`, `--closeness-end`, `--closeness-step` - Closeness sweep
- `--neighbor-start`, `--neighbor-end`, `--neighbor-step` - Neighbor sweep

**Fixed Values (for single-variable sweeps):**

- `--fixed-noise 0.01`
- `--fixed-closeness 2.0`
- `--fixed-neighbor 1`

**Sampling Configuration:**

- `--iterations 30` - Samples per parameter combination
- `--interval 2s` - Interval between samples
- `--settle-time 5s` - Time to wait for grid to settle after applying params

**Seed Control:**

- `--seed true` - Seed behavior: `true`, `false`, or `toggle`

**Total:** 20+ flags

---

### 3. Deploy Binary (`cmd/deploy`)

**Description:** Deployment manager for velocity.report service on remote hosts.

**Mode:** Interactive CLI tool (subcommand-based)

**Location:** `cmd/deploy`

#### Quick Start Examples

```bash
# Install on remote Pi
deploy install --target mypi --binary ./velocity-report-linux-arm64

# Check status
deploy status --target mypi

# Upgrade
deploy upgrade --target mypi --binary ./new-binary
```

#### CLI Flags (Common)

- `--target localhost` - Target host (hostname, IP, or SSH config alias)
- `--ssh-user <user>` - SSH user (defaults to `~/.ssh/config` or current user)
- `--ssh-key <path>` - SSH private key path (defaults to `~/.ssh/config`)
- `--debug` - Enable debug logging
- `--dry-run` - Show what would be done without executing (some commands)

#### Subcommands

**`install`** - Install velocity.report service

- `--binary` (required) - Path to velocity-report binary
- `--db-path` - Path to existing database to migrate
- `--dry-run`

**`upgrade`** - Upgrade to newer version

- `--binary` (required) - Path to new binary
- `--no-backup` - Skip backup before upgrade
- `--dry-run`

**`fix`** - Diagnose and repair broken installation

- `--binary` - Path to binary (optional, for fixing missing binary)
- `--repo-url` - Git repo URL
- `--build-from-source` - Build binary from source on server (requires Go)
- `--dry-run`

**`status`** - Check service status

- `--api-port 8080` - API server port
- `--timeout 30` - Timeout in seconds
- `--scan` - Perform detailed disk scan

**`health`** - Perform health check

- `--api-port 8080` - API server port

**`rollback`** - Rollback to previous version

- `--dry-run`

**`backup`** - Backup database and configuration

- `--output .` - Output directory for backup

**`config`** - Manage deployment configuration

- `--show` - Show current configuration
- `--edit` - Edit configuration

**`version`** - Show velocity-deploy version

**`help`** - Show help message

**SSH Config Support:**

- Reads `~/.ssh/config` automatically
- Uses HostName, User, IdentityFile from config
- Command-line flags override SSH config values

---

### 4. Transit Backfill Binary (`cmd/transit-backfill`)

**Description:** Backfill radar_data_transits table from historical radar_data events.

**Mode:** Batch job (processes time range, exits)

**Location:** `cmd/transit-backfill`

#### Quick Start Examples

```bash
transit-backfill \
  --db sensor_data.db \
  --start 2024-01-01T00:00:00Z \
  --end 2024-01-31T23:59:59Z
```

#### CLI Flags

- `--db sensor_data.db` - Path to SQLite database
- `--start <RFC3339>` - Start time (required)
- `--end <RFC3339>` - End time (required)
- `--gap 1` - Session gap in seconds
- `--model manual-backfill` - Model version string for transits

---

### 5. Backfill Ring Elevations Binary (`cmd/tools/backfill_ring_elevations`)

**Description:** Backfill ring elevation data for lidar background snapshots using embedded parser config.

**Mode:** Batch job (updates DB, exits)

**Location:** `cmd/tools/backfill_ring_elevations`

#### Quick Start Examples

```bash
# Dry run (preview)
backfill_ring_elevations --db sensor_data.db --dry-run

# Apply changes
backfill_ring_elevations --db sensor_data.db
```

#### CLI Flags

- `--db sensor_data.db` - Path to SQLite database
- `--dry-run` - Don't write changes; just report

---

### Makefile Targets (59 total)

**Build Targets (15):**

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

**Development Targets (7):**

- `dev-go` - Run Go server (radar disabled)
- `dev-go-lidar` - Run Go server with lidar enabled
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

## Quick Reference

### Common Workflows

#### Development Setup

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

#### Remote Deployment

```bash
# Build for Raspberry Pi
make build-radar-linux

# Deploy to remote Pi (using SSH config)
deploy install --target mypi --binary ./velocity-report-linux-arm64

# Check status
deploy status --target mypi
```

#### PDF Report Generation

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

#### Parameter Sweep Testing

```bash
# Multi-parameter sweep
sweep \
  --mode multi \
  --noise 0.01,0.02,0.03 \
  --closeness 1.5,2.0,2.5 \
  --neighbors 0,1,2 \
  --iterations 30 \
  --output sweep-results.csv

# Analyse results
make plot-multisweep INPUT=sweep-results.csv
```

### Environment Variables

**PDF Generator:**

- `PDF_GENERATOR_DIR` - PDF generator directory (default: auto-detect)
- `PDF_GENERATOR_PYTHON` - Python binary path (default: auto-detect)

**Development:**

- `GOARCH` - Target architecture (e.g., `arm64`)
- `GOOS` - Target OS (e.g., `linux`)

### Tips & Best Practices

**Database Management:**

- **Production:** Use `--db-path /var/lib/velocity-report/sensor_data.db`
- **Development:** Default `./sensor_data.db` is fine
- **Backup:** Use `/admin/db/backup` endpoint or `deploy backup` command
- **Migrations:** Always run `migrate status` before upgrading

**Port Usage:**

- **8080** - Main HTTP API and web UI
- **8081** - LiDAR monitor (when enabled)
- **2369** - LiDAR UDP packets (incoming)
- **2368** - LiDAR forwarding (to LidarView)

**SSH Deployment:**

- Store host config in `~/.ssh/config` for convenience
- Use SSH key authentication (not passwords)
- Test connection: `ssh mypi 'systemctl status velocity-report'`

**Performance Tuning:**

- **LiDAR frame buffer:** Adjust `--lidar-frame-buffer-timeout` for capture rate
- **Background flush:** Set `--lidar-bg-flush-interval` based on disk I/O capacity
- **Noise threshold:** Tune `--lidar-bg-noise-relative` for environment conditions

---

## Problems & Opportunities

### Current Architecture Observations

**Strengths:**

1. **Single Executable Model** - `radar` binary handles both radar and lidar, reducing deployment complexity
2. **Clear Separation** - Different binaries for different concerns (production service vs utilities)
3. **HTTP API Design** - RESTful patterns, clear resource paths
4. **Admin Routes Separation** - `/debug/` prefix for administrative/diagnostic endpoints
5. **Flexible Configuration** - Extensive CLI flags allow fine-tuning without code changes
6. **SSH Config Integration** - Deploy tool reads `~/.ssh/config` for convenience

**Areas for Improvement:**

1. **Flag Organization** (Priority: High)
   - **Issue:** 30+ flags with no logical grouping, making discovery difficult
   - **Impact:** Users struggle to find relevant flags, documentation is scattered

2. **Subcommand Gaps** (Priority: High)
   - **Issue:** Only 2 subcommands, most functionality via flags
   - **Impact:** Flat command structure, no clear action hierarchy

3. **Utility Fragmentation** (Priority: Medium)
   - **Issue:** 4 separate binaries for related utility tasks
   - **Impact:** Users must learn multiple CLIs, inconsistent patterns

4. **HTTP API Inconsistency** (Priority: Medium)
   - **Issue:** Mix of REST patterns, query params vs path params vs body
   - **Impact:** API consumers face unpredictable interfaces

5. **Makefile Target Naming** (Priority: Low)
   - **Issue:** 59 targets with no consistent naming convention
   - **Impact:** Users can't predict target names, poor discoverability

6. **Configuration Complexity** (Priority: Low)
   - **Issue:** No config file support, all configuration via flags
   - **Impact:** Complex deployments require unwieldy command lines

---

## Proposed Solution

### Core Principles

1. **Single Executable Model** - Maintain unified production binary
2. **Backward Compatibility** - Phased migration, no breaking changes initially
3. **Subcommand Hierarchy** - Actions before options (verb-noun pattern)
4. **Consistent Naming** - Predictable patterns across CLI, API, and Makefile
5. **Progressive Enhancement** - Add new patterns alongside old ones
6. **Clear Deprecation** - Explicit warnings with migration paths

### Target CLI Structure

```bash
# Production binary
velocity-report [global-flags] <command> [command-flags]

Commands:
  serve          Run production server (default if no command)
  migrate        Database migration operations
  version        Show version information
  help           Show help message

# Utility binary
velocity-tools <command> [flags]

Commands:
  sweep                    Run parameter sweep tests
  backfill-transits        Backfill transit sessions
  backfill-elevations      Backfill ring elevations
  deploy                   Deployment operations
  help                     Show help message
```

### Target HTTP API Structure

```bash
# Production API - Versioned
/api/v1/events           # New versioned endpoints
/api/v1/sites
/api/v1/reports
/api/v1/config

# Legacy API - Preserved
/api/radar_stats         # Keep for backward compatibility
/api/sites
/api/reports

# Admin API - Consistent prefix
/admin/db/backup
/admin/db/sql
/admin/radar/command
/admin/radar/status

# LiDAR Monitor API - Grouped by resource
/api/background/params
/api/background/grid
/api/acceptance
/api/snapshots
/api/datasource
```

### Target Makefile Structure

```makefile
# Pattern: <action>-<component>[-<variant>]

# Build targets
build-server              # Main server binary
build-server-linux        # Cross-compile for Linux
build-tools               # Utility tools
build-web                 # Web frontend
build-all                 # Everything

# Development targets
dev-server                # Run dev server
dev-server-lidar          # With lidar enabled
dev-web                   # Web dev server
dev-kill                  # Kill dev servers

# Test targets
test                      # All tests
test-go                   # Go tests
test-python               # Python tests
test-web                  # Web tests

# Code quality targets
format                    # Format all
format-go                 # Format Go
lint                      # Lint all
lint-go                   # Lint Go

# Database targets
db-migrate-up             # Run migrations
db-migrate-status         # Check status
db-backup                 # Backup database
```

### Before & After Examples

**Current (Before):**

```bash
# Too many flags, unclear structure
velocity-report --listen :8080 --db-path /var/lib/velocity-report/sensor_data.db --units mph --timezone US/Pacific --enable-lidar --lidar-listen :8081 --lidar-udp-port 2369

# Separate binaries for utilities
sweep --mode multi --output results.csv
deploy install --target mypi --binary ./velocity-report-linux-arm64
transit-backfill --db sensor_data.db --start 2024-01-01T00:00:00Z --end 2024-01-31T23:59:59Z
```

**Proposed (After):**

```bash
# With config file (cleaner)
velocity-report --config /etc/velocity-report.toml serve

# Or explicit flags (organized by subcommand)
velocity-report serve --listen :8080 --db-path /var/lib/velocity-report/sensor_data.db

# Unified tools binary
velocity-tools sweep --mode multi --output results.csv
velocity-tools deploy install --target mypi --binary ./velocity-report-linux-arm64
velocity-tools backfill-transits --start 2024-01-01T00:00:00Z --end 2024-01-31T23:59:59Z
```

---

## Long-Term Stable Architecture

This section outlines the **ideal future state** for velocity.report CLI and API design, optimised for long-term stability, consistency, and ease of use. This goes beyond incremental improvements to define a cohesive architecture.

### Design Philosophy

**Core Tenets:**

1. **Command-Data Separation** - Commands expressed as verbs, data as structured objects
2. **CLI-HTTP Alignment** - CLI commands mirror HTTP API structure
3. **JSON for Complex Parameters** - Use JSON objects for multi-field configurations
4. **Declarative Configuration** - What to achieve, not how to achieve it
5. **Single Source of Truth** - Configuration file as canonical state
6. **Composable Operations** - Small, focused commands that combine well

### Unified Command Structure

```bash
velocity-report <command> [subcommand] [options]
```

**Global Options (available for all commands):**

- `--config <file>` - Configuration file (TOML/JSON/YAML)
- `--db <path>` - Database path
- `--log-level <level>` - Logging verbosity (error/warn/info/debug)
- `--format <format>` - Output format (text/json/yaml)
- `--dry-run` - Show what would be done without executing

### Core Commands

#### 1. `serve` - Run Production Server (Default)

**Purpose:** Start the main production service with radar and optional lidar.

```bash
# Start with config file (preferred)
velocity-report serve --config /etc/velocity-report.toml

# Start with inline parameters (for testing)
velocity-report serve \
  --radar.enabled=true \
  --radar.port=/dev/ttySC1 \
  --lidar.enabled=true \
  --lidar.port=:8081

# Start with JSON config (advanced)
velocity-report serve --params '{"radar":{"enabled":true},"lidar":{"enabled":true}}'
```

**Subcommands:**

- None (single-purpose command)

**Configuration Structure:**

```toml
[server]
listen = ":8080"
db_path = "/var/lib/velocity-report/sensor_data.db"

[radar]
enabled = true
port = "/dev/ttySC1"
units = "mph"
timezone = "US/Pacific"

[lidar]
enabled = true
listen = ":8081"
udp_port = 2369
sensor_id = "hesai-pandar40p"

[lidar.background]
flush_interval = "60s"
noise_relative = 0.315

[lidar.forwarding]
enabled = false
address = "localhost"
port = 2368
```

---

#### 2. `sensor` - Sensor Operations

**Purpose:** Manage and configure radar and lidar sensors.

**Subcommands:**

**`sensor radar`** - Radar sensor operations

```bash
# Get current radar status
velocity-report sensor radar status

# Send command to radar
velocity-report sensor radar command --cmd "P?"

# Configure radar parameters
velocity-report sensor radar configure --params '{"units":"mph","sample_rate":10}'

# Test radar connection
velocity-report sensor radar test --port /dev/ttySC1
```

**`sensor lidar`** - LiDAR sensor operations

```bash
# Get lidar status
velocity-report sensor lidar status

# Configure background model (JSON for complex params)
velocity-report sensor lidar configure --params '{
  "background": {
    "noise_relative": 0.01,
    "closeness_multiplier": 2.0,
    "neighbor_confirmation_count": 1
  }
}'

# Trigger snapshot
velocity-report sensor lidar snapshot --persist

# Export data
velocity-report sensor lidar export --format asc --output /tmp/frame.asc

# Control data source
velocity-report sensor lidar source --mode live
velocity-report sensor lidar source --mode pcap --file recording.pcap
```

**Why JSON for LiDAR config?**

- **Consistency:** Matches HTTP API `POST /api/lidar/params` which accepts JSON
- **Structure:** Complex nested parameters (background, forwarding, frame) map naturally to JSON
- **Validation:** Schema validation easier with structured data
- **Extensibility:** Easy to add new parameters without flag explosion
- **Readability:** Clear hierarchy vs. long flat flags like `--lidar-bg-noise-relative`

---

#### 3. `data` - Data Management

**Purpose:** Query, export, and manage collected data.

**Subcommands:**

**`data query`** - Query sensor data

```bash
# Query radar events
velocity-report data query events \
  --start 2024-01-01T00:00:00Z \
  --end 2024-01-31T23:59:59Z \
  --format json

# Query transits (sessionized)
velocity-report data query transits \
  --site-id abc123 \
  --min-speed 25 \
  --format csv --output transits.csv

# Query lidar snapshots
velocity-report data query snapshots \
  --sensor hesai-pandar40p \
  --limit 10
```

**`data export`** - Export data

```bash
# Export to CSV
velocity-report data export --format csv \
  --start 2024-01-01 --end 2024-01-31 \
  --output /tmp/export.csv

# Export to JSON (for API consumers)
velocity-report data export --format json \
  --query '{"site_id":"abc123","min_speed":25}' \
  --output /tmp/export.json
```

**`data backfill`** - Backfill operations

```bash
# Backfill transits
velocity-report data backfill transits \
  --start 2024-01-01T00:00:00Z \
  --end 2024-01-31T23:59:59Z \
  --gap-seconds 1

# Backfill lidar elevations
velocity-report data backfill elevations \
  --dry-run
```

**`data stats`** - Statistics

```bash
# Get statistics for date range
velocity-report data stats \
  --start 2024-01-01 --end 2024-01-31 \
  --grouping hourly \
  --format json

# Real-time stats
velocity-report data stats --live --interval 5s
```

---

#### 4. `site` - Site Management

**Purpose:** Manage monitoring sites (locations).

**Subcommands:**

**`site list`** - List all sites

```bash
velocity-report site list --format json
```

**`site create`** - Create new site

```bash
# Interactive
velocity-report site create --interactive

# With parameters
velocity-report site create --params '{
  "name": "Main Street",
  "location": "123 Main St",
  "speed_limit": 25,
  "timezone": "US/Pacific"
}'
```

**`site get`** - Get site details

```bash
velocity-report site get <site-id> --format json
```

**`site update`** - Update site

```bash
velocity-report site update <site-id> --params '{
  "speed_limit": 30,
  "description": "Updated limit"
}'
```

**`site delete`** - Delete site

```bash
velocity-report site delete <site-id> --confirm
```

---

#### 5. `report` - Report Generation

**Purpose:** Generate PDF reports from collected data.

**Subcommands:**

**`report generate`** - Generate new report

```bash
# From config file
velocity-report report generate --config report-config.json

# With inline parameters
velocity-report report generate --params '{
  "site_id": "abc123",
  "start_date": "2024-01-01",
  "end_date": "2024-01-31",
  "timezone": "US/Pacific",
  "units": "mph"
}'

# From template
velocity-report report generate --template monthly --site abc123 --month 2024-01
```

**`report list`** - List reports

```bash
velocity-report report list --site abc123 --format json
```

**`report get`** - Get report metadata

```bash
velocity-report report get <report-id> --format json
```

**`report download`** - Download report

```bash
velocity-report report download <report-id> --output /tmp/report.pdf
```

**`report delete`** - Delete report

```bash
velocity-report report delete <report-id> --confirm
```

---

#### 6. `db` - Database Operations

**Purpose:** Database administration and maintenance.

**Subcommands:**

**`db migrate`** - Run migrations

```bash
# Check status
velocity-report db migrate status

# Run pending migrations
velocity-report db migrate up

# Rollback
velocity-report db migrate down --steps 1

# Force to version
velocity-report db migrate force --version 20240101
```

**`db backup`** - Backup database

```bash
# Create backup
velocity-report db backup --output /tmp/backup.db

# Automated backup with timestamp
velocity-report db backup --output /backups/db-$(date +%Y%m%d).db
```

**`db restore`** - Restore from backup

```bash
velocity-report db restore --input /tmp/backup.db --confirm
```

**`db stats`** - Database statistics

```bash
# Show size, record counts
velocity-report db stats --format json

# Detailed table stats
velocity-report db stats --detailed
```

**`db vacuum`** - Optimize database

```bash
velocity-report db vacuum
```

---

#### 7. `config` - Configuration Management

**Purpose:** Manage configuration files and validate settings.

**Subcommands:**

**`config init`** - Initialise config

```bash
# Create default config
velocity-report config init --output /etc/velocity-report.toml

# Interactive wizard
velocity-report config init --interactive

# From template
velocity-report config init --template production --output config.toml
```

**`config validate`** - Validate config

```bash
velocity-report config validate --file /etc/velocity-report.toml
```

**`config show`** - Show current config

```bash
# Show effective configuration
velocity-report config show --format json

# Show with defaults filled in
velocity-report config show --with-defaults

# Show specific section
velocity-report config show --section lidar
```

**`config set`** - Update config value

```bash
velocity-report config set server.listen :9090 --file config.toml
velocity-report config set lidar.enabled true --file config.toml
```

**`config get`** - Get config value

```bash
velocity-report config get server.listen --file config.toml
```

---

#### 8. `deploy` - Deployment Operations

**Purpose:** Deploy and manage remote installations.

**Subcommands:**

**`deploy install`** - Install on remote host

```bash
velocity-report deploy install \
  --target mypi \
  --binary velocity-report-linux-arm64 \
  --config prod-config.toml
```

**`deploy upgrade`** - Upgrade remote installation

```bash
velocity-report deploy upgrade \
  --target mypi \
  --binary velocity-report-linux-arm64 \
  --backup
```

**`deploy status`** - Check deployment status

```bash
velocity-report deploy status --target mypi --format json
```

**`deploy rollback`** - Rollback deployment

```bash
velocity-report deploy rollback --target mypi --confirm
```

**`deploy config`** - Manage remote config

```bash
# Push config
velocity-report deploy config push --target mypi --file config.toml

# Pull config
velocity-report deploy config pull --target mypi --output config.toml

# Edit remote config
velocity-report deploy config edit --target mypi
```

---

#### 9. `test` - Testing and Diagnostics

**Purpose:** Test system components and diagnose issues.

**Subcommands:**

**`test radar`** - Test radar sensor

```bash
velocity-report test radar --port /dev/ttySC1 --duration 10s
```

**`test lidar`** - Test lidar sensor

```bash
velocity-report test lidar --port :2369 --duration 30s
```

**`test api`** - Test HTTP API

```bash
velocity-report test api --url http://localhost:8080 --full
```

**`test sweep`** - Run parameter sweep

```bash
# Parameter sweep with JSON config
velocity-report test sweep --params '{
  "mode": "multi",
  "noise": [0.01, 0.02, 0.03],
  "closeness": [1.5, 2.0, 2.5],
  "neighbors": [0, 1, 2],
  "iterations": 30
}' --output results.csv

# PCAP-based sweep
velocity-report test sweep --pcap recording.pcap --params sweep-config.json
```

**`test health`** - Health check

```bash
velocity-report test health --comprehensive
```

---

#### 10. `admin` - Administrative Operations

**Purpose:** Administrative and maintenance tasks.

**Subcommands:**

**`admin logs`** - View logs

```bash
# Tail logs
velocity-report admin logs --tail --lines 100

# Filter by level
velocity-report admin logs --level error --since 1h

# Export logs
velocity-report admin logs --since 24h --output /tmp/logs.txt
```

**`admin users`** - Manage users (future auth)

```bash
velocity-report admin users list
velocity-report admin users create --username admin --role admin
velocity-report admin users delete <user-id>
```

**`admin tokens`** - Manage API tokens (future auth)

```bash
velocity-report admin tokens create --name "monitoring" --scopes read:events,read:sites
velocity-report admin tokens revoke <token-id>
```

**`admin maintenance`** - Maintenance mode

```bash
# Enable maintenance mode
velocity-report admin maintenance enable --message "System upgrade in progress"

# Disable
velocity-report admin maintenance disable
```

---

### JSON Parameter Pattern

**Philosophy:** Use JSON for any parameter group with 3+ related fields or nested structure.

**Examples:**

**Simple parameters (flags):**

```bash
velocity-report serve --listen :8080 --debug
```

**Complex parameters (JSON):**

```bash
velocity-report sensor lidar configure --params '{
  "background": {
    "noise_relative": 0.01,
    "closeness_multiplier": 2.0,
    "neighbor_confirmation_count": 1,
    "seed_from_first_frame": true
  },
  "frame": {
    "buffer_timeout": "500ms",
    "min_points": 1000
  }
}'
```

**From file (for reusability):**

```bash
velocity-report sensor lidar configure --params @lidar-config.json
```

**Advantages:**

1. **Consistency:** Matches HTTP API patterns exactly
2. **Validation:** JSON schema validation
3. **Documentation:** Self-documenting with field names
4. **Tooling:** Easy to generate, parse, version control
5. **Extensibility:** Add fields without breaking existing usage

---

### HTTP API Alignment

**Principle:** CLI commands should map directly to HTTP endpoints.

| CLI Command                               | HTTP Endpoint                      | Method |
| ----------------------------------------- | ---------------------------------- | ------ |
| `site list`                               | `GET /api/v1/sites`                | GET    |
| `site create --params '{...}'`            | `POST /api/v1/sites`               | POST   |
| `site get <id>`                           | `GET /api/v1/sites/{id}`           | GET    |
| `site update <id> --params '{...}'`       | `PUT /api/v1/sites/{id}`           | PUT    |
| `site delete <id>`                        | `DELETE /api/v1/sites/{id}`        | DELETE |
| `sensor lidar configure --params '{...}'` | `POST /api/v1/lidar/params`        | POST   |
| `sensor lidar status`                     | `GET /api/v1/lidar/status`         | GET    |
| `data query events --start X --end Y`     | `GET /api/v1/events?start=X&end=Y` | GET    |

**Benefits:**

- Users familiar with API can predict CLI commands
- Documentation reusable across CLI and API
- Testing simplified (same operations, different interface)
- Code sharing (CLI can call HTTP client internally)

---

### Configuration Priority

**Order (highest to lowest):**

1. **Command-line flags/parameters** - `--listen :9090`
2. **Environment variables** - `VELOCITY_REPORT_LISTEN=:9090`
3. **Configuration file** - `config.toml: listen = ":9090"`
4. **Default values** - Hard-coded in application

**Example:**

```bash
# Config file has: listen = ":8080"
# Command overrides: --listen :9090
# Result: Server listens on :9090

velocity-report serve --config config.toml --listen :9090
```

---

### Output Formats

**Support multiple output formats for machine and human consumption:**

**Text (default):**

```bash
velocity-report site list
# Main Street    | 123 Main St | 25 mph
# Oak Avenue     | 456 Oak Ave | 30 mph
```

**JSON (for programmatic use):**

```bash
velocity-report site list --format json
# [{"id":"abc","name":"Main Street","speed_limit":25}, ...]
```

**YAML (for readability):**

```bash
velocity-report site list --format yaml
# - id: abc
#   name: Main Street
#   speed_limit: 25
```

**CSV (for export):**

```bash
velocity-report site list --format csv --output sites.csv
```

---

### Security Architecture

**Authentication & Authorization (Future):**

1. **API Tokens** - Long-lived tokens for CLI and automation
2. **Session-based** - Web UI authentication
3. **Role-based Access Control** - admin, operator, viewer roles
4. **Audit Logging** - Track all administrative operations

**CLI Token Usage:**

```bash
# Set token in environment
export VELOCITY_REPORT_TOKEN="vrt_abc123..."

# Or via flag
velocity-report site create --token "vrt_abc123..." --params '{...}'

# Or in config file
[auth]
token = "vrt_abc123..."
```

**Sensitive Endpoints (require auth):**

- All POST/PUT/DELETE operations
- LiDAR configuration changes
- Database backups/restores
- Export operations
- Deployment commands

---

### Migration Path Summary

**Current → Long-Term:**

| Current                          | Long-Term                                 | Benefit                       |
| -------------------------------- | ----------------------------------------- | ----------------------------- |
| 30+ flat flags                   | Subcommands + JSON params                 | Organization, discoverability |
| `--lidar-bg-noise-relative 0.01` | `sensor lidar configure --params '{...}'` | Consistency with HTTP         |
| 4 separate binaries              | Single binary with commands               | Unified interface             |
| Multiple HTTP patterns           | Versioned API with consistent structure   | Predictability                |
| No config validation             | `config validate` command                 | Catch errors early            |
| No testing commands              | `test` subcommands                        | Built-in diagnostics          |

---

## Implementation Plan

**Goal:** Document current state and design future structure
**Duration:** Completed
**Risk:** None

**Tasks:**

- ✅ Inventory all CLI flags, subcommands, HTTP endpoints
- ✅ Categorize and document current patterns
- ✅ Design target structure with backward compatibility
- ✅ Create comprehensive guide and implementation plan

**Deliverables:**

- ✅ This comprehensive guide document

---

### Phase 2: Non-Breaking Improvements (4-6 weeks)

**Goal:** Add new patterns alongside existing ones
**Duration:** 4-6 weeks
**Risk:** Low (additive only, no removals)

#### Step 1: Add Subcommand Structure (Week 1-2)

**Tasks:**

1. Add `serve` subcommand to radar binary
   - Make `serve` the default action when no subcommand provided
   - Keep all existing flags working with and without `serve` prefix
   - Update help text to show subcommand structure
   - Test: `velocity-report` === `velocity-report serve`

2. Enhance `migrate` subcommand
   - Accept `--db-path` flag in consistent position
   - Improve help text and error messages
   - Test all migration operations with new structure

3. Add explicit `help` subcommand
   - Show organized help by command and category
   - Include flag groupings (core, radar, lidar, tuning)
   - Test help output for clarity

**Acceptance Criteria:**

- [ ] `velocity-report` works identically to `velocity-report serve`
- [ ] All existing flags work with `serve` subcommand
- [ ] Help text clearly shows command structure
- [ ] No breaking changes to existing scripts/deployments

**Testing:**

```bash
# All of these should work identically
velocity-report --disable-radar
velocity-report serve --disable-radar

# Subcommands should work
velocity-report version
velocity-report migrate status
velocity-report help
```

#### Step 2: Version HTTP APIs (Week 2-3)

**Tasks:**

1. Add `/api/v1/` endpoints (keeping legacy)
   - Implement v1 versions of: events, sites, reports, config
   - Route both old and new paths to same handlers initially
   - Add API version header to responses

2. Update OpenAPI/Swagger docs
   - Document both v1 and legacy endpoints
   - Mark legacy endpoints as deprecated
   - Include migration examples

3. Add deprecation warnings
   - Log warnings when legacy endpoints used
   - Include `X-API-Version` response header
   - Document recommended migration path

**Acceptance Criteria:**

- [ ] New `/api/v1/` endpoints work identically to legacy
- [ ] Legacy endpoints continue to work without errors
- [ ] Deprecation warnings logged (but not user-visible errors)
- [ ] API documentation shows both versions

**Testing:**

```bash
# Both should work
curl http://localhost:8080/api/sites
curl http://localhost:8080/api/v1/sites

# Version header should be present
curl -I http://localhost:8080/api/v1/sites | grep X-API-Version
```

#### Step 3: Reorganize Makefile (Week 3-4)

**Tasks:**

1. Add new target names following pattern
   - Create new targets: `build-server`, `dev-server`, `db-migrate-up`
   - Keep old targets as aliases: `build-radar-local` → `build-server`
   - Document new naming convention in Makefile

2. Update documentation
   - Update README with new target names
   - Mark old names as deprecated (but still working)
   - Provide migration table in docs

3. Add `make help` improvements
   - Group targets by category
   - Show both old and new names
   - Indicate deprecation status

**Acceptance Criteria:**

- [ ] All old targets still work via aliases
- [ ] New targets work identically to old ones
- [ ] `make help` shows organized, clear output
- [ ] Documentation updated with migration path

**Testing:**

```bash
# Both should work
make build-radar-local
make build-server

# Help should show new structure
make help | grep "Build Targets"
```

#### Step 4: Configuration File Support (Week 4-6)

**Tasks:**

1. Implement TOML config parsing
   - Add dependency: `github.com/BurntSushi/toml`
   - Create config struct matching flag structure
   - Implement config file loader

2. Add `--config` global flag
   - Load config file before parsing CLI flags
   - CLI flags override config file values
   - Validate config file syntax and values

3. Create example config files
   - Development config example
   - Production config example
   - Document all available options

4. Update documentation
   - Add config file reference
   - Show priority: defaults → config → CLI flags
   - Provide migration examples

**Acceptance Criteria:**

- [ ] Config file loaded when `--config` provided
- [ ] CLI flags override config values
- [ ] Invalid config files show helpful errors
- [ ] Example configs provided and documented

**Testing:**

```bash
# Config file usage
velocity-report --config /etc/velocity-report.toml serve

# CLI override
velocity-report --config config.toml serve --listen :9090

# Validate config
velocity-report --config bad.toml serve  # Should show clear error
```

**Phase 2 Deliverables:**

- [ ] Enhanced radar binary with subcommands (backward compatible)
- [ ] Versioned HTTP API (`/api/v1/`) alongside legacy
- [ ] Reorganized Makefile with consistent naming (aliases preserved)
- [ ] Configuration file support (optional, additive)
- [ ] Updated documentation showing new patterns

---

### Phase 3: Consolidation (6-8 weeks)

**Goal:** Create unified `velocity-tools` binary for all utilities
**Duration:** 6-8 weeks
**Risk:** Medium (new binary, but old ones still work)

**Tasks:**

1. Create new binary structure
   - New directory: `cmd/velocity-tools/`
   - Implement subcommand router
   - Add global flags: `--db-path`, `--debug`, `--version`

2. Migrate sweep functionality
   - Move sweep logic to `internal/tools/sweep/`
   - Implement `velocity-tools sweep` subcommand
   - Preserve all existing flags and behavior
   - Test against existing sweep scripts

3. Migrate backfill utilities
   - Add `backfill-transits` subcommand
   - Add `backfill-elevations` subcommand
   - Preserve all flags and behavior

4. Integrate deploy functionality
   - Move deploy logic to `internal/tools/deploy/`
   - Implement `velocity-tools deploy` with all subcommands
   - Preserve SSH config integration

5. Add deprecation warnings to old binaries
   - Print notice on startup
   - Direct users to velocity-tools
   - Log migration path

**Acceptance Criteria:**

- [ ] `velocity-tools` binary with all utility functionality
- [ ] Old binaries still work with deprecation notices
- [ ] All commands work identically to old binaries
- [ ] Updated build system and documentation

**Phase 3 Deliverables:**

- [ ] velocity-tools binary with all utility functionality
- [ ] Old binaries still work with deprecation notices
- [ ] Updated build system and documentation
- [ ] User communication about migration path

---

### Phase 4: Breaking Changes (Major Version)

**Goal:** Remove deprecated patterns, finalize new structure
**Duration:** 8-12 weeks (after 2-3 release cycles)
**Risk:** Medium (breaking changes, requires user migration)

**Tasks:**

1. **Grace Period** (First 2-3 releases)
   - Monitor deprecation warnings
   - Track usage of deprecated endpoints
   - User support and documentation

2. **Remove Old Binaries**
   - Stop building sweep, deploy, backfill binaries
   - Keep velocity-tools only
   - Update package managers

3. **Remove Legacy API Routes**
   - Keep only `/api/v1/` routes
   - Remove unversioned endpoints
   - Update all clients

4. **Remove Makefile Aliases**
   - Keep only new naming pattern
   - Remove build-radar-\* aliases
   - Clean up help text

5. **Final Documentation**
   - Remove references to old structure
   - Show only new patterns
   - Archive migration guides
   - Publish v2.0 release

**Phase 4 Deliverables:**

- [ ] Clean codebase with only new patterns
- [ ] Single velocity-report and velocity-tools binaries
- [ ] Versioned API only
- [ ] Consistent Makefile
- [ ] Final documentation and v2.0 release

---

## Configuration File Support

### Proposed Format

**Format:** TOML (primary), YAML, or JSON support

**Priority:** CLI flags override config file values

**Example Configuration:**

```toml
# velocity.report configuration file
# Save as: /etc/velocity-report.toml or ~/.velocity-report.toml

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

### Usage Examples

```bash
# Use config file
velocity-report --config /etc/velocity-report.toml serve

# Config file + CLI override
velocity-report --config config.toml serve --listen :9090

# Multiple config files (last one wins)
velocity-report --config base.toml --config local.toml serve
```

### Priority Order

1. **Default values** - Hard-coded in application
2. **Configuration file** - From `--config` flag or default locations
3. **Environment variables** - (Future enhancement)
4. **Command-line flags** - Highest priority, overrides all

---

## Summary

This comprehensive guide provides a complete reference for velocity.report's current CLI structure and a detailed plan for improving it. The key principles are:

1. **Single Executable Model** - Maintain for production (`velocity-report`)
2. **Utility Consolidation** - Group utilities under `velocity-tools`
3. **Subcommand Organization** - Clear command hierarchy
4. **Consistent Naming** - Predictable patterns for flags, endpoints, and targets
5. **Backward Compatibility** - Phased migration with deprecation warnings
6. **Configuration Files** - Reduce flag clutter for complex deployments
7. **API Versioning** - Enable evolution without breaking changes

The proposed changes balance immediate improvements with long-term maintainability while respecting existing deployments.

**Total Timeline:** 18-26 weeks from Phase 2 start to v2.0 release

---

_Last updated: 2025-12-02_
