# CLI Quick Reference

**Quick reference for velocity.report command-line interfaces**

For complete analysis and proposed restructuring, see [cli-analysis.md](./cli-analysis.md)

---

## Current Binaries

### `velocity-report` (Production Service)
**Location:** `cmd/radar`  
**Purpose:** Main production server with radar + optional lidar

**Quick Start:**
```bash
# Production mode
velocity-report --db-path /var/lib/velocity-report/sensor_data.db

# Development (no hardware)
velocity-report --disable-radar --debug

# With lidar enabled
velocity-report --enable-lidar --lidar-listen :8081
```

**Key Flags:**
- `--listen :8080` - HTTP API server address
- `--db-path sensor_data.db` - Database location
- `--disable-radar` - Disable hardware (DB/API only)
- `--enable-lidar` - Enable lidar components
- `--units mph` - Display units (mps/mph/kmph)
- `--timezone UTC` - Display timezone

**Subcommands:**
- `version` - Show version
- `migrate up|down|status` - Database migrations

**HTTP APIs:**
- `:8080` - Radar API, sites, reports, web UI
- `:8080/debug/` - Admin routes (SQL, backup, commands)
- `:8081` - LiDAR monitor (when `--enable-lidar`)

---

### `velocity-tools sweep` (Parameter Testing)
**Location:** `cmd/sweep`  
**Purpose:** Test lidar background model parameters

**Quick Start:**
```bash
# Multi-parameter sweep
velocity-tools sweep --mode multi --output results.csv

# Noise-only sweep
velocity-tools sweep --mode noise --noise-start 0.005 --noise-end 0.03

# PCAP replay mode
velocity-tools sweep --pcap recording.pcap --pcap-settle 20s
```

**Key Flags:**
- `--monitor http://localhost:8081` - Lidar monitor URL
- `--mode multi|noise|closeness|neighbor` - Sweep mode
- `--iterations 30` - Samples per combination
- `--output <file>` - Output CSV filename

---

### `velocity-tools deploy` (Deployment Manager)
**Location:** `cmd/deploy`  
**Purpose:** Install/upgrade/manage remote installations

**Quick Start:**
```bash
# Install on remote Pi
velocity-tools deploy install --target mypi --binary ./velocity-report-linux-arm64

# Check status
velocity-tools deploy status --target mypi

# Upgrade
velocity-tools deploy upgrade --target mypi --binary ./new-binary
```

**Key Commands:**
- `install` - Install service
- `upgrade` - Upgrade to new version
- `status` - Check service status
- `health` - Health check
- `backup` - Backup database
- `rollback` - Rollback to previous version

**Key Flags:**
- `--target localhost` - Target host (SSH config alias supported)
- `--ssh-user <user>` - SSH user
- `--ssh-key <path>` - SSH key path
- `--dry-run` - Preview without executing

---

### `velocity-tools backfill-transits` (Data Processing)
**Location:** `cmd/transit-backfill`  
**Purpose:** Backfill radar_data_transits from historical data

**Quick Start:**
```bash
velocity-tools backfill-transits \
  --db sensor_data.db \
  --start 2024-01-01T00:00:00Z \
  --end 2024-01-31T23:59:59Z
```

**Key Flags:**
- `--db sensor_data.db` - Database path
- `--start <RFC3339>` - Start time (required)
- `--end <RFC3339>` - End time (required)
- `--gap 1` - Session gap in seconds
- `--model manual-backfill` - Model version string

---

### `velocity-tools backfill-elevations` (Lidar Maintenance)
**Location:** `cmd/tools/backfill_ring_elevations`  
**Purpose:** Backfill elevation data in lidar snapshots

**Quick Start:**
```bash
# Dry run (preview)
velocity-tools backfill-elevations --db sensor_data.db --dry-run

# Apply changes
velocity-tools backfill-elevations --db sensor_data.db
```

---

## HTTP API Endpoints

### Radar API (`:8080`)

#### Events & Statistics
```
GET  /events              - List radar events
GET  /api/radar_stats     - Radar statistics (with grouping)
GET  /api/config          - Server configuration
```

#### Sites (Monitoring Locations)
```
GET    /api/sites         - List sites
POST   /api/sites         - Create site
GET    /api/sites/{id}    - Get site
PUT    /api/sites/{id}    - Update site
DELETE /api/sites/{id}    - Delete site
```

#### Reports
```
GET    /api/reports                - List all reports
POST   /api/generate_report        - Generate new report
GET    /api/reports/{id}           - Get report metadata
DELETE /api/reports/{id}           - Delete report
GET    /api/reports/{id}/download  - Download PDF or ZIP
GET    /api/reports/site/{id}      - Reports for specific site
```

#### Web UI
```
GET  /app/                - Web frontend (SPA)
GET  /                    - Redirect to /app/
```

### Admin API (`:8080/debug/`)

```
GET  /debug/tailsql/      - SQL debugging interface
GET  /debug/backup        - Download database backup
GET  /debug/send-command  - Send radar command (HTML)
POST /debug/send-command-api - Send radar command (API)
GET  /debug/live-tail     - Live serial port output
```

### LiDAR Monitor API (`:8081`)

#### Status
```
GET  /health              - Health check
GET  /                    - Status dashboard (HTML)
GET  /api/lidar/status    - System status (JSON)
```

#### Background Model
```
GET  /api/lidar/params           - Get background parameters
POST /api/lidar/params           - Update parameters
GET  /api/lidar/grid_status      - Grid status
POST /api/lidar/grid_reset       - Reset grid
GET  /api/lidar/grid_heatmap     - Grid heatmap data
```

#### Acceptance Metrics
```
GET  /api/lidar/acceptance       - Get metrics
POST /api/lidar/acceptance/reset - Reset counters
```

#### Snapshots & Export
```
POST /api/lidar/persist              - Trigger persistence
POST /api/lidar/snapshot             - Trigger snapshot
GET  /api/lidar/snapshots            - List snapshots
GET  /api/lidar/export_snapshot      - Export as ASC
GET  /api/lidar/export_next_frame    - Export next frame
```

#### Data Source Control
```
GET  /api/lidar/data_source    - Current source (live/PCAP)
POST /api/lidar/pcap/start     - Start PCAP replay
POST /api/lidar/pcap/stop      - Stop PCAP, return to live
```

---

## Common Makefile Targets

### Building
```bash
make build-radar-linux      # Cross-compile for ARM64 Linux
make build-radar-local      # Local build with pcap
make build-tools            # Build utility tools
make build-web              # Build web frontend
```

### Development
```bash
make dev-go                 # Run dev server (radar disabled)
make dev-go-lidar           # Run dev server with lidar
make dev-web                # Run web dev server
```

### Testing
```bash
make test                   # Run all tests
make test-go                # Go tests only
make test-python            # Python tests only
make lint                   # Lint all code
make format                 # Format all code
```

### Database
```bash
make migrate-up             # Apply migrations
make migrate-status         # Show migration status
```

### Deployment
```bash
make deploy-install         # Install on remote
make deploy-upgrade         # Upgrade remote
make deploy-status          # Check remote status
```

### Reports
```bash
make pdf-report CONFIG=config.json   # Generate PDF report
make pdf-config                      # Create config template
```

---

## Environment Variables

### PDF Generator
- `PDF_GENERATOR_DIR` - PDF generator directory (default: auto-detect)
- `PDF_GENERATOR_PYTHON` - Python binary path (default: auto-detect)

### Development
- `GOARCH` - Target architecture (e.g., `arm64`)
- `GOOS` - Target OS (e.g., `linux`)

---

## Configuration Files (Proposed)

**Location:** `/etc/velocity-report.toml` or `~/.velocity-report.toml`

**Example:**
```toml
[server]
listen = ":8080"
db_path = "/var/lib/velocity-report/sensor_data.db"
units = "mph"
timezone = "US/Pacific"

[radar]
enabled = true
port = "/dev/ttySC1"

[lidar]
enabled = true
listen = ":8081"
udp_port = 2369
```

**Usage:**
```bash
velocity-report --config /etc/velocity-report.toml serve
```

---

## Common Workflows

### Development Setup
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

### Remote Deployment
```bash
# Build for Raspberry Pi
make build-radar-linux

# Deploy to remote Pi (using SSH config)
velocity-tools deploy install --target mypi --binary ./velocity-report-linux-arm64

# Check status
velocity-tools deploy status --target mypi
```

### PDF Report Generation
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

### Parameter Sweep Testing
```bash
# Multi-parameter sweep
velocity-tools sweep \
  --mode multi \
  --noise 0.01,0.02,0.03 \
  --closeness 1.5,2.0,2.5 \
  --neighbors 0,1,2 \
  --iterations 30 \
  --output sweep-results.csv

# Analyze results
make plot-multisweep INPUT=sweep-results.csv
```

---

## Tips & Best Practices

### Database Management
- **Production:** Use `--db-path /var/lib/velocity-report/sensor_data.db`
- **Development:** Default `./sensor_data.db` is fine
- **Backup:** Use `/admin/db/backup` endpoint or `deploy backup` command
- **Migrations:** Always run `migrate status` before upgrading

### Port Usage
- **8080** - Main HTTP API and web UI
- **8081** - LiDAR monitor (when enabled)
- **2369** - LiDAR UDP packets (incoming)
- **2368** - LiDAR forwarding (to LidarView)

### SSH Deployment
- Store host config in `~/.ssh/config` for convenience
- Use SSH key authentication (not passwords)
- Test connection: `ssh mypi 'systemctl status velocity-report'`

### Performance Tuning
- **LiDAR frame buffer:** Adjust `--lidar-frame-buffer-timeout` for capture rate
- **Background flush:** Set `--lidar-bg-flush-interval` based on disk I/O capacity
- **Noise threshold:** Tune `--lidar-bg-noise-relative` for environment conditions

---

For detailed analysis and proposed improvements, see [cli-analysis.md](./cli-analysis.md)
