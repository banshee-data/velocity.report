# Troubleshooting Guide

This guide covers common issues, error messages, and solutions for the velocity.report system across all components.

## Table of Contents

- [Quick Diagnosis](#quick-diagnosis)
- [Go Server Issues](#go-server-issues)
- [Python PDF Generator Issues](#python-pdf-generator-issues)
- [Web Frontend Issues](#web-frontend-issues)
- [Database Issues](#database-issues)
- [Sensor Hardware Issues](#sensor-hardware-issues)
- [Network and Connectivity Issues](#network-and-connectivity-issues)
- [Performance Issues](#performance-issues)
- [CI/CD Issues](#cicd-issues)
- [Getting Help](#getting-help)

---

## Quick Diagnosis

### System Health Check

Run these commands to quickly diagnose the system state:

```bash
# Check if Go server is running
ps aux | grep "velocity.report"

# Check database integrity
sqlite3 sensor_data.db "PRAGMA integrity_check;"

# Check Python environment
cd tools/pdf-generator
.venv/bin/python --version
.venv/bin/pip list | grep -E "pylatex|matplotlib|requests"

# Check web build
ls -lh web/build/

# Check sensor connections
ls -l /dev/ttyUSB* 2>/dev/null || echo "No USB-Serial devices found"
```

### Common Symptoms and Quick Fixes

| Symptom                | Likely Cause            | Quick Fix                                             |
| ---------------------- | ----------------------- | ----------------------------------------------------- |
| No data appearing      | Radar not connected     | Check `/dev/ttyUSB0`, verify power                    |
| PDF generation fails   | Missing LaTeX           | Install XeLaTeX: `sudo apt-get install texlive-xetex` |
| Web frontend blank     | Build not generated     | Run `cd web && pnpm run build`                        |
| API returns 500 errors | Database corruption     | Check database with `PRAGMA integrity_check`          |
| High CPU usage         | Background worker stuck | Restart Go server                                     |
| Cosine error warnings  | Missing config field    | Add `radar.cosine_error_angle` to config              |

---

## Go Server Issues

### Server Won't Start

**Error**: `listen tcp 0.0.0.0:8080: bind: address already in use`

**Cause**: Another process is using port 8080

**Solution**:

```bash
# Find the process using port 8080
sudo lsof -i :8080

# Kill it (replace PID with actual process ID)
kill -9 <PID>

# Or use a different port
./velocity-report-local -listen :8081
```

---

### Server Crashes on Startup

**Error**: `panic: unable to open database file: sensor_data.db: database is locked`

**Cause**: Database file is locked by another process or has incorrect permissions

**Solution**:

```bash
# Check if another process has the database open
lsof sensor_data.db

# Fix permissions
chmod 644 sensor_data.db
chown $USER:$USER sensor_data.db

# If database is corrupted, check integrity
sqlite3 sensor_data.db "PRAGMA integrity_check;"

# Last resort: backup and recreate
cp sensor_data.db sensor_data.db.backup
sqlite3 sensor_data.db < internal/db/schema.sql
```

---

### Serial Port Not Found

**Error**: `failed to open serial port: open /dev/ttyUSB0: no such file or directory`

**Cause**: Radar sensor not connected or USB-Serial driver not loaded

**Solution**:

```bash
# List available serial devices
ls -l /dev/tty*

# Check if device is recognized
dmesg | grep tty

# Add user to dialout group (required for serial access)
sudo usermod -a -G dialout $USER
# Log out and back in for group membership to take effect

# If using different port, specify it
./velocity-report-local -serial /dev/ttyUSB1
```

---

### No Data Being Collected

**Error**: Server runs but no data appears in database

**Cause**: Radar not sending data, incorrect baud rate, or serial misconfiguration

**Solution**:

```bash
# Test serial connection manually
screen /dev/ttyUSB0 115200
# Should see radar output. Press Ctrl-A, K to exit

# Verify radar configuration
echo "??" > /dev/ttyUSB0  # Query radar status

# Check database for recent data
sqlite3 sensor_data.db "SELECT COUNT(*) FROM radar_data WHERE timestamp > datetime('now', '-1 hour');"

# Enable debug logging
./velocity-report-local -debug
```

---

### LIDAR Connection Issues

**Error**: `failed to bind UDP socket: bind: address already in use`

**Cause**: Another process is using the LIDAR UDP port (2368)

**Solution**:

```bash
# Find process using port 2368
sudo netstat -tulpn | grep 2368

# Kill the process
sudo kill <PID>

# Verify LIDAR network configuration
ip addr show | grep 192.168.100

# If network interface missing, add it
sudo ip addr add 192.168.100.151/24 dev eth0
```

**Error**: `no LIDAR packets received`

**Cause**: LIDAR not configured to send to correct IP, network cable issue, or firewall blocking

**Solution**:

```bash
# Check if packets are arriving
sudo tcpdump -i eth0 udp port 2368

# Verify LIDAR is powered and LED is solid green
# Configure LIDAR to send to 192.168.100.151 using Hesai web interface at 192.168.100.202

# Disable firewall temporarily to test
sudo ufw disable
```

---

### API Returns Empty Data

**Error**: `GET /api/radar_stats` returns `{"metrics": []}`

**Cause**: No data in date range, incorrect query parameters, or timezone issues

**Solution**:

```bash
# Check what data exists in database
sqlite3 sensor_data.db "SELECT MIN(timestamp), MAX(timestamp), COUNT(*) FROM radar_data;"

# Check if radar_data_transits table has data
sqlite3 sensor_data.db "SELECT COUNT(*) FROM radar_data_transits;"

# Test API with broader date range
curl "http://localhost:8080/api/radar_stats?start=0&end=9999999999&group=all&source=radar_objects"

# Check timezone conversion
curl "http://localhost:8080/api/radar_stats?start=1717200000&end=1717300000&group=1h&timezone=UTC"
```

---

## Python PDF Generator Issues

### XeLaTeX Not Found

**Error**: `FileNotFoundError: [Errno 2] No such file or directory: 'xelatex'`

**Cause**: XeLaTeX not installed

**Solution**:

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install texlive-xetex texlive-fonts-extra

# macOS
brew install --cask mactex
# Or minimal version:
brew install basictex
sudo tlmgr update --self
sudo tlmgr install xetex

# Verify installation
which xelatex
xelatex --version
```

---

### Missing Python Dependencies

**Error**: `ModuleNotFoundError: No module named 'pylatex'`

**Cause**: Virtual environment not activated or dependencies not installed

**Solution**:

```bash
cd tools/pdf-generator

# Create virtual environment if missing
python3 -m venv .venv

# Activate virtual environment
source .venv/bin/activate  # Linux/macOS
# OR
.venv\Scripts\activate  # Windows

# Install dependencies
pip install -r requirements.txt

# Verify installation
pip list | grep -E "pylatex|matplotlib|requests"
```

---

### Config Validation Errors

**Error**: `radar.cosine_error_angle is required`

**Cause**: Config file missing required field

**Solution**:

```bash
# Generate new config with all fields
.venv/bin/python -m pdf_generator.cli.create_config --output my-config.json

# Or add missing field manually to existing config
# Edit config.json and add under "radar" section:
{
  "radar": {
    "cosine_error_angle": 21.0
  }
}

# Validate config
.venv/bin/python -c "
from pdf_generator.core.config_manager import load_config
config = load_config('my-config.json')
valid, errors = config.validate()
if not valid:
    print('Errors:', errors)
else:
    print('Config valid!')
"
```

---

### API Connection Failed

**Error**: `requests.exceptions.ConnectionError: Failed to establish a new connection`

**Cause**: Go server not running or wrong host/port

**Solution**:

```bash
# Check if server is running
curl http://localhost:8080/api/config

# If not running, start it
cd /path/to/velocity.report
./velocity-report-local

# If using different port, update config
# In config.json, you can't change API host (it's not configurable)
# But you can access via IP if running remotely:
curl http://192.168.1.100:8080/api/config

# For PDF generator, it uses localhost:8080 by default
# Check internal/report/query_data/api_client.py for API_BASE_URL
```

---

### LaTeX Compilation Errors

**Error**: `! LaTeX Error: File 'fontspec.sty' not found`

**Cause**: Missing LaTeX packages

**Solution**:

```bash
# Install missing packages
sudo tlmgr install fontspec
sudo tlmgr install geometry
sudo tlmgr install booktabs
sudo tlmgr install graphicx

# Or install full texlive
sudo apt-get install texlive-full  # Ubuntu/Debian

# Check what packages are available
tlmgr search --global fontspec
```

**Error**: `! Package xcolor Error: Undefined color 'linkcolor'`

**Cause**: Custom color not defined

**Solution**: Check LaTeX template in `document_builder.py` - ensure all colors are defined before use

---

### Chart Generation Fails

**Error**: `RuntimeError: Failed to create chart: no such column: speed_mph`

**Cause**: Database schema mismatch or API response format change

**Solution**:

```bash
# Check database schema
sqlite3 sensor_data.db ".schema radar_data"

# Verify API response format
curl "http://localhost:8080/api/radar_stats?start=0&end=9999999999&group=1h" | jq .

# Check for unit conversion issues - API returns data in requested units
curl "http://localhost:8080/api/radar_stats?start=0&end=9999999999&group=1h&units=mph" | jq '.metrics[0]'
```

---

### No Data Retrieved from API

**Error**: PDF generates but shows "No data available for period"

**Cause**: Date range has no data, wrong timezone, or source parameter mismatch

**Solution**:

```json
// Check config.json query section:
{
  "query": {
    "start_date": "2025-06-01", // Make sure this matches your data
    "end_date": "2025-06-07",
    "timezone": "US/Pacific", // Match your local timezone
    "source": "radar_data_transits" // Try "radar_objects" instead
  }
}
```

```bash
# Find actual data date range
sqlite3 sensor_data.db "SELECT DATE(MIN(timestamp)), DATE(MAX(timestamp)) FROM radar_data;"

# Test with that date range
.venv/bin/python internal/report/query_data/get_stats.py \
  --start-date 2025-06-01 \
  --end-date 2025-06-07 \
  --file-prefix test \
  --debug
```

---

## Web Frontend Issues

### Build Directory Missing

**Error**: `Error: ENOENT: no such file or directory, stat 'web/build'`

**Cause**: Frontend not built

**Solution**:

```bash
cd web

# Install dependencies
pnpm install

# Build for production
pnpm run build

# Verify build
ls -lh build/

# For development, use dev server instead
pnpm run dev
```

---

### Blank Page After Navigation

**Error**: Clicking links results in blank page or 404

**Cause**: SPA routing not configured correctly, missing prerendered routes

**Solution**:

```bash
# Check if build has HTML files for routes
ls web/build/*.html

# Verify server SPA fallback is working
curl http://localhost:8080/app/dashboard
# Should return index.html content

# In development, use hash router or ensure dev server has SPA fallback
# Check vite.config.ts for proper configuration
```

---

### API Calls Fail from Frontend

**Error**: `Failed to fetch` or CORS errors in browser console

**Cause**: API server not running, CORS misconfiguration, or wrong API URL

**Solution**:

```bash
# Check API is accessible
curl http://localhost:8080/api/config

# In development, verify proxy configuration in vite.config.ts:
# Should proxy /api/* requests to http://localhost:8080

# Check browser network tab for actual request URL
# Common issue: frontend built with wrong API URL

# For production, ensure API and frontend are served from same origin
# or configure CORS headers in Go server
```

---

### Chart Not Rendering

**Error**: Charts don't display or show "Loading..."

**Cause**: Missing chart library, data format mismatch, or API error

**Solution**:

```javascript
// Open browser console (F12) and check for errors
// Common issues:

// 1. Chart.js not installed
// Fix: cd web && pnpm install chart.js

// 2. Data format mismatch
// Check API response format matches chart expectations
console.log(
  await fetch("/api/radar_stats?start=0&end=9999999999&group=1h").then((r) =>
    r.json(),
  ),
);

// 3. Async data not loading
// Ensure component waits for data before rendering
```

---

## Database Issues

### Database Locked

**Error**: `database is locked`

**Cause**: Multiple processes accessing database, or previous process crashed without releasing lock

**Solution**:

```bash
# Check what processes have database open
lsof sensor_data.db

# Kill stale processes
kill -9 <PID>

# If persistent, restart in exclusive mode
sqlite3 sensor_data.db "PRAGMA locking_mode=EXCLUSIVE; SELECT 1;"

# Check for WAL files
ls -lh sensor_data.db*
# sensor_data.db-wal and sensor_data.db-shm should exist with WAL mode

# Force checkpoint to clear WAL
sqlite3 sensor_data.db "PRAGMA wal_checkpoint(TRUNCATE);"
```

---

### Database Corruption

**Error**: `database disk image is malformed`

**Cause**: Disk error, power loss during write, or filesystem issue

**Solution**:

```bash
# Try to recover
sqlite3 sensor_data.db ".recover" | sqlite3 recovered.db

# Or dump and restore
sqlite3 sensor_data.db .dump > backup.sql
mv sensor_data.db sensor_data.db.corrupt
sqlite3 sensor_data.db < backup.sql

# Verify integrity
sqlite3 sensor_data.db "PRAGMA integrity_check;"

# If unrecoverable, restore from backup
cp /path/to/backup/sensor_data.db.backup sensor_data.db
```

---

### Slow Query Performance

**Error**: Queries take too long, API timeouts

**Cause**: Missing indexes, large dataset, or inefficient query

**Solution**:

```bash
# Check query plan
sqlite3 sensor_data.db "EXPLAIN QUERY PLAN SELECT * FROM radar_data WHERE timestamp > datetime('now', '-1 day');"

# Add missing indexes
sqlite3 sensor_data.db "CREATE INDEX IF NOT EXISTS idx_radar_data_timestamp ON radar_data(timestamp);"

# Vacuum database to reclaim space and rebuild indexes
sqlite3 sensor_data.db "VACUUM;"

# Analyse tables for query optimisation
sqlite3 sensor_data.db "ANALYZE;"

# Check database size
ls -lh sensor_data.db
# If very large (>1GB), consider archiving old data
```

---

### Schema Migration Issues

**Error**: `no such column: elevation_fov`

**Cause**: Database schema out of date

**Solution**:

```bash
# Check current schema
sqlite3 sensor_data.db ".schema radar_data"

# Compare with expected schema
cat internal/db/schema.sql

# Run migrations
cd internal/db/migrations
ls -1 *.sql | sort | while read migration; do
  echo "Running $migration..."
  sqlite3 ../../sensor_data.db < "$migration"
done

# Or recreate database (CAUTION: loses data)
mv sensor_data.db sensor_data.db.old
sqlite3 sensor_data.db < internal/db/schema.sql
```

---

## Sensor Hardware Issues

### Radar Not Responding

**Symptoms**: No data, server logs show no speed readings

**Diagnosis**:

```bash
# Test serial connection directly
screen /dev/ttyUSB0 115200
# Should see JSON output like: {"speed":28.5,"direction":"inbound"}

# Query radar firmware
echo "??" > /dev/ttyUSB0
cat /dev/ttyUSB0

# Send initialization commands
echo "OT" > /dev/ttyUSB0  # Set output to JSON
echo "R0" > /dev/ttyUSB0  # Set to reporting mode
```

**Solutions**:

- Power cycle the radar (unplug, wait 10s, replug)
- Check USB cable (try different cable/port)
- Verify baud rate is 115200
- Update radar firmware if available

---

### LIDAR Not Producing Point Clouds

**Symptoms**: Server runs but no LIDAR data in database

**Diagnosis**:

```bash
# Check network connectivity
ping 192.168.100.202

# Verify packets arriving
sudo tcpdump -i eth0 -c 10 udp port 2368
# Should see packet captures

# Check LIDAR status via web interface
# Navigate to http://192.168.100.202 in browser
# Verify:
# - Destination IP is 192.168.100.151
# - Destination Port is 2368
# - Laser is ON
```

**Solutions**:

- Power cycle LIDAR
- Verify network cable connection
- Reset LIDAR to factory defaults via web interface
- Check that network interface has correct IP: `ip addr show`

---

### Cosine Error Correction Issues

**Error**: Speed readings seem consistently wrong by fixed percentage

**Cause**: Incorrect `cosine_error_angle` in config

**Diagnosis**:

```bash
# Check current angle in config
jq .radar.cosine_error_angle config.json

# Calculate expected factor: 1/cos(angle_degrees)
# For 21°: 1/cos(21°) = 1.071
# This means readings are multiplied by 1.071

# Test with known speed
# If radar shows 28 mph but actual is 30 mph:
# angle = acos(28/30) = 21.04°
```

**Solution**: Measure actual mounting angle with protractor or level, update config

---

## Network and Connectivity Issues

### Cannot Access Web Interface Remotely

**Error**: Web interface works on localhost but not from other devices

**Cause**: Server binding to localhost only, or firewall blocking

**Solution**:

```bash
# Check server binding
netstat -tlnp | grep 8080
# Should show 0.0.0.0:8080, not 127.0.0.1:8080

# Start server with explicit binding
./velocity-report-local -listen 0.0.0.0:8080

# Check firewall
sudo ufw status
sudo ufw allow 8080/tcp

# Test from remote machine
curl http://<raspberry-pi-ip>:8080/api/config
```

---

### Systemd Service Won't Start

**Error**: `systemctl status velocity-report` shows failed

**Diagnosis**:

```bash
# Check service status and logs
systemctl status velocity-report
journalctl -u velocity-report -n 50

# Check service file
cat /etc/systemd/system/velocity-report.service

# Test manual start
/usr/local/bin/velocity-report-local -db /var/lib/velocity-report/sensor_data.db
```

**Common Issues**:

- Binary path incorrect: verify `/usr/local/bin/velocity-report-local` exists
- Database path wrong: ensure `/var/lib/velocity-report/` exists and is writable
- Serial port permissions: add service user to `dialout` group
- Working directory: ensure `WorkingDirectory=` is set correctly

---

## Performance Issues

### High CPU Usage

**Symptoms**: CPU at 100%, system sluggish

**Diagnosis**:

```bash
# Check which process is using CPU
top
# Or more detailed:
htop

# Check Go server goroutines
curl http://localhost:8080/debug/pprof/goroutine?debug=1

# Check database activity
lsof sensor_data.db
```

**Solutions**:

- Restart Go server (background worker may be stuck)
- Reduce LIDAR frame rate if processing can't keep up
- Archive old data from database
- Check for runaway queries in logs

---

### High Memory Usage

**Symptoms**: Out of memory errors, system swapping

**Diagnosis**:

```bash
# Check memory usage
free -h

# Check Go server memory
ps aux | grep velocity-report-local

# Check for memory leaks
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

**Solutions**:

- Restart server periodically (add to cron)
- Reduce histogram bucket counts in API queries
- Limit query result sizes
- Add more swap space if needed

---

### Slow PDF Generation

**Symptoms**: PDF generation takes several minutes

**Diagnosis**:

```bash
# Enable debug mode to see timing
.venv/bin/python internal/report/query_data/get_stats.py --debug ...

# Check API response time
time curl "http://localhost:8080/api/radar_stats?start=0&end=9999999999&group=1h&compute_histogram=true"

# Check LaTeX compilation time
time xelatex test.tex
```

**Solutions**:

- Use smaller date ranges
- Disable histogram if not needed
- Use faster time grouping (24h instead of 15m)
- Generate charts at lower DPI
- Use faster LaTeX engine or fewer fonts

---

## Getting Help

### Before Asking for Help

Please gather this information:

```bash
# System information
uname -a
cat /etc/os-release

# Go server version
./velocity-report-local -version

# Python version and packages
.venv/bin/python --version
.venv/bin/pip list

# Database info
sqlite3 sensor_data.db "SELECT sqlite_version();"
ls -lh sensor_data.db

# Recent logs
journalctl -u velocity-report -n 100 > logs.txt

# Config (redact sensitive info)
cat config.json
```

### Support Channels

- **GitHub Issues**: https://github.com/banshee-data/velocity.report/issues
- **Documentation**: See `docs/README.md` for all documentation
- **Email**: david@banshee-data.com

### Useful Log Commands

```bash
# Tail Go server logs
journalctl -u velocity-report -f

# Search for errors
journalctl -u velocity-report | grep -i error

# Export logs for specific time period
journalctl -u velocity-report --since "2025-01-01" --until "2025-01-02" > debug.log

# Enable debug logging
./velocity-report-local -debug
```

---

## Common Error Messages Reference

| Error Message                             | Component     | Solution                              |
| ----------------------------------------- | ------------- | ------------------------------------- |
| `bind: address already in use`            | Go Server     | Kill process on port 8080             |
| `database is locked`                      | Database      | Check for stale processes with `lsof` |
| `xelatex: command not found`              | PDF Generator | Install texlive-xetex                 |
| `ModuleNotFoundError`                     | PDF Generator | Activate venv, install requirements   |
| `cosine_error_angle is required`          | PDF Generator | Add field to config                   |
| `Failed to fetch`                         | Web Frontend  | Check API server is running           |
| `no such file or directory: /dev/ttyUSB0` | Go Server     | Check radar connection                |
| `no LIDAR packets received`               | Go Server     | Verify LIDAR network config           |
| `PRAGMA integrity_check: failed`          | Database      | Restore from backup                   |
| `403 Forbidden`                           | Web Server    | Check file permissions                |

---

## CI/CD Issues

### Go CI Tests Fail with E2E Error

**Error**: `ModuleNotFoundError: No module named 'numpy'` during Go tests

**Cause**: PDF generation E2E tests require Python dependencies

**Solution**: API tests including E2E tests run in the CI `test-integration` job where Python dependencies are installed. For local development:

```bash
# Option 1: Skip E2E tests using environment variable (recommended)
SKIP_PDF_TESTS=1 go test ./internal/api/...

# Option 2: Install Python dependencies and run all tests
make install-python
go test ./internal/api/...
```

---

### Web CI Lint Failures

**Error**: Prettier or ESLint failures in web-ci workflow

**Cause**: Code formatting inconsistency

**Solution**:

```bash
# Auto-fix formatting
cd web
pnpm run format

# Or from repository root
make format-web
```

---

### GitHub Actions Cache Issues

**Error**: CI runs slower than expected or cache misses

**Cause**: Cache key mismatch or cache eviction

**Solution**:

```bash
# Clear and rebuild caches by pushing a commit that changes lock files
# Or wait for weekly cache eviction (7 days)

# Check cache usage in GitHub Actions UI:
# Repository → Actions → Caches
```

---

**Last Updated**: 2026-02-01
**Version**: 1.1
**Maintainer**: david@banshee-data.com
