```
  _
 / | _  /_    _  _  ._  _
/_.'/_'/_//_//_//_/// //_/
             _/ _/     _/
```

Debugging is the subtle art of persuading a very literal machine to confess what it has been
doing all along.

This guide is your map, torch, and occasional pep talk: real errors, likely causes, and practical
fixes for the velocity.report system across all components.

## Debugging table of contents

- [Quick diagnosis](#quick-diagnosis)
- [Go server issues](#go-server-issues)
- [Web frontend issues](#web-frontend-issues)
- [Database issues](#database-issues)
- [Sensor hardware issues](#sensor-hardware-issues)
- [Network and connectivity issues](#network-and-connectivity-issues)
- [Deployment and upgrade issues](#deployment-and-upgrade-issues)
- [Pi system issues](#pi-system-issues)
- [Performance issues](#performance-issues)
- [Getting help](#getting-help)
- [Known fixed issues](#known-fixed-issues)
- [macOS visualiser issues](#macos-visualiser-issues)
- [Common error messages reference](#common-error-messages-reference)
- [CI/CD issues](#cicd-issues)

---

## Quick diagnosis

### System health check

Run this quick sweep to see what is alive, and what might be broken:

```bash
# Check if Go server is running
ps aux | grep "velocity.report"

# Check database integrity
sqlite3 sensor_data.db "PRAGMA integrity_check;"

# Check web build
ls -lh web/build/

# Check sensor connections (Pi HAT default: ttySC1; USB-serial dongle: ttyUSB0)
ls -l /dev/ttySC* /dev/ttyUSB* 2>/dev/null || echo "No serial devices found"
```

### Common symptoms and quick fixes (short version)

| Symptom                | Likely Cause            | Quick Fix                                                    |
| ---------------------- | ----------------------- | ------------------------------------------------------------ |
| No data appearing      | Radar not connected     | Check `/dev/ttySC1` (Pi HAT) or `/dev/ttyUSB0`, verify power |
| PDF generation fails   | Missing LaTeX           | Install XeLaTeX: `sudo apt-get install texlive-xetex`        |
| Web frontend blank     | Build not generated     | Run `cd web && pnpm run build`                               |
| API returns 500 errors | Database corruption     | Check database with `PRAGMA integrity_check`                 |
| High CPU usage         | Background worker stuck | Restart Go server                                            |
| Cosine error warnings  | Missing config field    | Add `radar.cosine_error_angle` to config                     |

---

## Go server issues

### Server won't start

**Error**: `listen tcp 0.0.0.0:8080: bind: address already in use`

**Likely cause**: Another process is using port 8080

**Do this**:

```bash
# Find the process using port 8080
sudo lsof -i :8080

# Prefer stopping the service cleanly if it's ours
sudo systemctl stop velocity-report

# Otherwise verify the PID is what you think, then ask it to exit
ps -p <PID> -o comm=
kill -TERM <PID>   # escalate to -KILL only if -TERM fails

# Or use a different port
./velocity-report-local --listen :8081
```

---

### Server crashes on startup

**Error**: `panic: unable to open database file: sensor_data.db: database is locked`

**Likely cause**: Database file is locked by another process or has incorrect permissions

**Do this**:

```bash
# Stop the service before touching the file
sudo systemctl stop velocity-report

# Check if anything else still holds the database open
lsof sensor_data.db

# Fix permissions to match the service account (not world-readable: the DB holds transit data)
sudo chown velocity:velocity sensor_data.db
sudo chmod 600 sensor_data.db

# If database is corrupted, check integrity
sqlite3 sensor_data.db "PRAGMA integrity_check;"

# Last resort: move the file aside and let the binary re-create + migrate on next start.
# Migrations are embedded in the binary (golang-migrate); do NOT pipe schema.sql into sqlite3 —
# it is an auto-generated reference and leaves `schema_migrations` unpopulated.
sudo mv sensor_data.db sensor_data.db.$(date +%s).bak
sudo systemctl start velocity-report
```

---

### Serial port not found

**Error**: `failed to open serial port: open /dev/ttyUSB0: no such file or directory`

**Likely cause**: Radar sensor not connected or USB-Serial driver not loaded

**Do this**:

```bash
# List available serial devices
ls -l /dev/tty*

# Check if device is recognized
dmesg | grep tty

# Add user to dialout group (required for serial access).
# Production: grant access to the systemd service account.
sudo usermod -a -G dialout velocity
# Development only: if you run the server manually from your shell, grant access to
# your interactive account as well.
sudo usermod -a -G dialout "$USER"
# Log out and back in for group membership to take effect

# If using a different device, specify it. Pi HAT default is /dev/ttySC1;
# USB-serial dongles typically appear as /dev/ttyUSB0.
./velocity-report-local --port /dev/ttyUSB1
```

---

### No data being collected

**Error**: Server runs but no data appears in database

**Likely cause**: Radar not sending data, incorrect baud rate, or serial misconfiguration

**Do this**:

```bash
# Test serial connection manually. OPS243-A ships at 19200 baud (NOT 115200).
# On a Pi with the SC16IS HAT, the device is /dev/ttySC1.
screen /dev/ttySC1 19200
# Should see radar output. Press Ctrl-A, K to exit

# Verify radar configuration
echo "??" > /dev/ttySC1  # Query radar status

# Check database for recent data
sqlite3 sensor_data.db "SELECT COUNT(*) FROM radar_data WHERE timestamp > datetime('now', '-1 hour');"

# Enable debug logging
./velocity-report-local --debug
```

---

### LiDAR connection issues

**Error**: `failed to bind UDP socket: bind: address already in use`

**Likely cause**: Another process is using the LiDAR UDP ingest port (2369). Port 2368 is the
raw-forward port used only when `--lidar-forward` is set; do not confuse the two.

**Do this**:

```bash
# Find process using port 2369
sudo lsof -i UDP:2369
# or: sudo ss -lunp 'sport = :2369'

# Verify the PID is what you think, then stop it cleanly
ps -p <PID> -o comm=
sudo systemctl stop velocity-report   # if it is ours

# Verify LiDAR network configuration
ip addr show | grep 192.168.100

# If a static address on the LiDAR VLAN is missing, configure it persistently via
# netplan or systemd-networkd. Ad-hoc `ip addr add` does not survive reboot and can
# conflict with an active DHCP lease.
```

**Error**: `no LiDAR packets received`

**Likely cause**: LiDAR not configured to send to correct IP, network cable issue, or firewall blocking

**Do this**:

```bash
# Check if packets are arriving on the ingest port
sudo tcpdump -i eth0 udp port 2369

# Verify LiDAR is powered and LED is solid green
# Configure LiDAR to send to 192.168.100.151:2369 using Hesai web interface at 192.168.100.202

# If a firewall is involved, open the ingest port narrowly rather than disabling ufw:
sudo ufw allow from 192.168.100.202 to any port 2369 proto udp
```

### LiDAR emitting on an unknown IP, subnet, or port

The factory defaults (`192.168.100.202` → `192.168.100.151:2369`) are only a starting
point: any previous operator may have changed the source address, destination address,
destination port, or subnet mask. A sensor that has left the factory network is invisible
to the Pi until you find out what it is actually transmitting.

The cleanest way to answer "what is this sensor doing?" is to isolate it on a dedicated
interface and watch traffic directly. Use a USB 3.0 gigabit Ethernet adapter as a second
NIC, connect the LiDAR to it with a short patch cable (no switch in between), and confirm
the adapter came up — for example, `eth1`:

```bash
# Find the new interface after plugging in the adapter
ip -br link show
# Expect a new entry like: eth1  UP  ...

# Bring the interface up without assigning an IP — we only want to listen
sudo ip link set eth1 up
```

Now passively observe. The `-Q in` filter drops anything the Pi itself transmits, and
`-q -nn` strip the output down to source → destination so the LiDAR's real IP and port
are obvious:

```bash
# tcpdump: one line per packet, unique src → dst pairs
sudo tcpdump -i eth1 -Q in -nn -q -c 200 udp 2>/dev/null \
  | awk '{print $3, "->", $5}' \
  | sed 's/:$//' \
  | sort -u
```

`tshark` is friendlier if you want a clean table and port frequencies:

```bash
# 10-second capture; columns: src, dst, src-port, dst-port; sorted by frequency
sudo tshark -i eth1 -a duration:10 -n -Q \
  -Y 'udp and !mdns and !stp and !lldp' \
  -T fields -e ip.src -e ip.dst -e udp.srcport -e udp.dstport 2>/dev/null \
  | sort | uniq -c | sort -rn
```

Interpret the output:

- **Source IP** is the LiDAR's current address. If it is on an unexpected subnet (say
  `10.5.0.x`), either the LiDAR was reconfigured or it is still carrying another site's
  settings — plan to reset it from the Hesai web UI once you can reach it.
- **Destination IP** tells you whether the LiDAR is unicasting to a host that no longer
  exists (common after a Pi swap) or broadcasting (`255.255.255.255` or a subnet
  broadcast). Either way, change it to the Pi's LiDAR-facing address.
- **Destination port** confirms whether the sensor is pointed at `2369` (our ingest) or
  something else.

To reach the LiDAR's web UI once its address is known, put the monitoring interface on
the same subnet — for example, if the sensor is on `192.168.50.x`:

```bash
sudo ip addr add 192.168.50.10/24 dev eth1
# Then browse to the address shown as the packet source
```

Revert any ad-hoc `ip addr add` once the sensor is reconfigured onto the production
subnet, or make the change persistent via netplan / systemd-networkd.

---

### API returns empty data

**Error**: `GET /api/radar_stats` returns `{"metrics": []}`

**Likely cause**: No data in date range, incorrect query parameters, or timezone issues

**Do this**:

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

## Web frontend issues

### Build directory missing

**Error**: `Error: ENOENT: no such file or directory, stat 'web/build'`

**Likely cause**: Frontend not built

**Do this**:

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

### Blank page after navigation

**Error**: Clicking links results in blank page or 404

**Likely cause**: SPA routing not configured correctly, missing prerendered routes

**Do this**:

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

### API calls fail from frontend

**Error**: `Failed to fetch` or CORS errors in browser console

**Likely cause**: API server not running, CORS misconfiguration, or wrong API URL

**Do this**:

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

### Chart not rendering

**Error**: Charts don't display or show "Loading..."

**Likely cause**: Missing chart library, data format mismatch, or API error

**Do this**:

```javascript
// Open browser console (F12) and check for errors
// Common issues:

// 1. Chart.js not installed
// Fix: cd web && pnpm install chart.js

// 2. Data format mismatch
// Check API response format matches chart expectations
console.log(await fetch("/api/radar_stats?start=0&end=9999999999&group=1h").then((r) => r.json()));

// 3. Async data not loading
// Ensure component waits for data before rendering
```

---

## Database issues

### Database locked

**Error**: `database is locked`

**Likely cause**: Multiple processes accessing database, or previous process crashed without releasing lock

**Do this**:

```bash
# Check what processes have the database open
lsof sensor_data.db

# Stop the service cleanly. `kill -9` skips SQLite's cleanup and is itself a common cause
# of locked databases and corruption — reach for it only after -TERM has been refused.
sudo systemctl stop velocity-report
ps -p <PID> -o comm=   # only if an unmanaged process remains
kill -TERM <PID>

# Check for WAL files
ls -lh sensor_data.db*
# sensor_data.db-wal and sensor_data.db-shm should exist with WAL mode

# Force checkpoint to clear WAL
sqlite3 sensor_data.db "PRAGMA wal_checkpoint(TRUNCATE);"
```

---

### Database corruption

**Error**: `database disk image is malformed`

**Likely cause**: Disk error, power loss during write, or filesystem issue

**Do this**:

```bash
# Always stop the service before touching a damaged DB — concurrent writes guarantee
# further damage — and operate on a copy, not the live file.
sudo systemctl stop velocity-report
cp sensor_data.db sensor_data.db.corrupt

# Try to recover from the copy
sqlite3 sensor_data.db.corrupt ".recover" | sqlite3 recovered.db

# Or dump and restore
sqlite3 sensor_data.db.corrupt .dump > backup.sql
mv sensor_data.db sensor_data.db.old
sqlite3 sensor_data.db < backup.sql

# Verify integrity
sqlite3 sensor_data.db "PRAGMA integrity_check;"

# If unrecoverable, restore from backup
cp /path/to/backup/sensor_data.db.backup sensor_data.db
sudo systemctl start velocity-report
```

---

### Slow query performance

**Error**: Queries take too long, API timeouts

**Likely cause**: Missing indexes, large dataset, or inefficient query

**Do this**:

```bash
# Check query plan
sqlite3 sensor_data.db "EXPLAIN QUERY PLAN SELECT * FROM radar_data WHERE timestamp > datetime('now', '-1 day');"

# Add missing indexes
sqlite3 sensor_data.db "CREATE INDEX IF NOT EXISTS idx_radar_data_timestamp ON radar_data(timestamp);"

# Vacuum database to reclaim space and rebuild indexes
sqlite3 sensor_data.db "VACUUM;"

# Analyse tables for query optimisation (ANALYZE is the SQL keyword — keep the spelling)
sqlite3 sensor_data.db "ANALYZE;"

# Check database size
ls -lh sensor_data.db
# If very large (>1GB), consider archiving old data
```

---

### Schema migration issues

**Error**: `no such column: elevation_fov`

**Likely cause**: Database schema out of date

**Do this**:

Migrations are embedded in the binary via `golang-migrate` and applied on startup: the
server is the source of truth, not the raw `.sql` files. Do not shell `sqlite3 < migration.sql`
by hand — that bypasses the `schema_migrations` table and leaves the DB in a dirty state
the next startup will refuse to repair. Likewise, `internal/db/schema.sql` is an
auto-generated snapshot for reference only, not an installer.

```bash
# Stop the service, take a backup, then let the binary migrate on the next start
sudo systemctl stop velocity-report
sudo cp /var/lib/velocity-report/sensor_data.db /var/lib/velocity-report/sensor_data.db.pre-migrate
sudo systemctl start velocity-report
sudo journalctl -u velocity-report -n 100    # confirm migrations applied cleanly

# To inspect the current schema for comparison
sqlite3 sensor_data.db ".schema radar_data"
```

---

## Sensor hardware issues

### Radar not responding

**What you see**: No data, server logs show no speed readings

**Quick check**:

```bash
# Test serial connection directly. OPS243-A defaults to 19200 baud. On a Pi HAT the
# device is /dev/ttySC1; on a USB-serial dongle it is typically /dev/ttyUSB0.
screen /dev/ttySC1 19200
# Should see JSON output like: {"speed":28.5,"direction":"inbound"}

# Query radar firmware
echo "??" > /dev/ttySC1
cat /dev/ttySC1

# Send initialisation commands
echo "OT" > /dev/ttySC1  # Set output to JSON
echo "R0" > /dev/ttySC1  # Set to reporting mode
```

**Do this**:

- Power cycle the radar (unplug, wait 10s, replug)
- Check cable (try different cable/port)
- Verify baud rate is 19200
- Update radar firmware if available

---

### LiDAR not producing point clouds

**What you see**: Server runs but no LiDAR data in database

**Quick check**:

```bash
# Check network connectivity
ping 192.168.100.202

# Verify packets arriving on the ingest port (2369, NOT 2368 — 2368 is the raw-forward port)
sudo tcpdump -i eth0 -c 10 udp port 2369
# Should see packet captures

# Check LiDAR status via web interface
# Navigate to http://192.168.100.202 in browser
# Verify:
# - Destination IP is 192.168.100.151
# - Destination Port is 2369
# - Laser is ON
```

**Do this**:

- Power cycle LiDAR
- Verify network cable connection
- Reset LiDAR to factory defaults via web interface
- Check that network interface has correct IP: `ip addr show`

---

### Cosine error correction issues

**Error**: Speed readings seem consistently wrong by fixed percentage

**Likely cause**: Incorrect `cosine_error_angle` in config

**Quick check**:

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

**Do this**: Measure actual mounting angle with protractor or level, update config

---

## Network and connectivity issues

### Cannot access web interface remotely

**Error**: Web interface works on localhost but not from other devices

**Likely cause**: Server binding to localhost only, or firewall blocking

**Do this**:

```bash
# Check server binding
netstat -tlnp | grep 8080
# Should show 0.0.0.0:8080, not 127.0.0.1:8080

# The HTTP API ships without authentication. Binding to 0.0.0.0 publishes transit data
# and site coordinates to every network the Pi can reach. Prefer binding to a specific
# LAN interface and gating access with ufw from a trusted CIDR.
./velocity-report-local --listen 192.168.1.10:8080

# If you must bind widely, restrict by source address
sudo ufw allow from 192.168.1.0/24 to any port 8080 proto tcp

# Test from remote machine
curl http://<raspberry-pi-ip>:8080/api/config
```

---

### Systemd service won't start

**Error**: `systemctl status velocity-report` shows failed

**Quick check**:

```bash
# Check service status and logs
systemctl status velocity-report
journalctl -u velocity-report -n 50

# Check service file
cat /etc/systemd/system/velocity-report.service

# Test manual start. The production binary is installed as `velocity-report` (no -local
# suffix); `velocity-report-local` is the dev-build artefact produced by `make build-radar-local`.
/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db
```

**Common Issues**:

- Binary path incorrect: verify `/usr/local/bin/velocity-report` exists
- Database path wrong: ensure `/var/lib/velocity-report/` exists and is writable
- Serial port permissions: add the `velocity` service user to `dialout` (`sudo usermod -a -G dialout velocity`)
- Working directory: ensure `WorkingDirectory=` is set correctly

---

## Deployment and upgrade issues

`velocity-ctl` is the on-device management tool. It runs as root and wraps upgrade,
rollback, backup, and status operations. Subcommands: `upgrade`, `rollback`, `backup`,
`status`, `version`. Schema migrations are owned by the `velocity-report` binary, not
`velocity-ctl`. Common migration operations use `velocity-report migrate up`,
`velocity-report migrate down`, `velocity-report migrate status`,
`velocity-report migrate version`, and `velocity-report migrate force`;
`baseline`, `detect`, and `help` subcommands are also available for recovery
and diagnostics.

### Upgrade check fails or stalls

**Error**: `velocity-ctl upgrade` times out, reports a network error, or exits with
`SHA-256 mismatch`.

**Likely cause**: Release metadata is fetched from `https://velocity.report/release.json`. Network
failure, TLS error, a stale DNS cache, or a corrupted download will stop the upgrade before
anything on disk changes.

**Do this**:

```bash
# Verify the release feed is reachable and well-formed
curl -sfL https://velocity.report/release.json | jq .

# Re-run with explicit check (no side effects)
sudo velocity-ctl upgrade --check

# If the feed is reachable but a SHA-256 mismatch is reported, retry the upgrade —
# a partial download is the most common cause. Do not move the binary by hand.
sudo velocity-ctl upgrade
```

### Upgrade left the service in a bad state

**What you see**: `velocity-ctl status` reports the service failed after upgrade; logs show a
migration error or `schema_migrations` dirty version.

**Do this**:

```bash
# Check the migration state
sudo velocity-report migrate --db-path /var/lib/velocity-report/sensor_data.db status

# If a migration is marked dirty, inspect the target version, resolve the underlying error,
# then force the version back to a known-good number before re-running `migrate up`.
# `force` is a recovery escape hatch — it marks the version without executing the migration.
sudo velocity-report migrate --db-path /var/lib/velocity-report/sensor_data.db force <N>
sudo velocity-report migrate --db-path /var/lib/velocity-report/sensor_data.db up

# If the failure is not recoverable, roll back to the previous snapshot
sudo velocity-ctl rollback
```

### `velocity-ctl rollback` reports no backup found

**Likely cause**: `/var/lib/velocity-report/backups/` is empty — either this is the first install,
or an earlier manual operation removed it.

**Do this**:

```bash
ls -la /var/lib/velocity-report/backups/

# Take a manual snapshot before the next upgrade so a rollback target always exists
sudo velocity-ctl backup
```

### First boot: no web UI, service disabled, or TLS not ready

**What you see**: Pi is up and reachable but `https://velocity.local/` returns a connection
error, or nginx is running but serves a certificate error.

**Quick check**:

```bash
# Confirm the two first-boot services are enabled and have run
systemctl is-enabled velocity-report.service velocity-generate-tls.service
systemctl status velocity-generate-tls.service
systemctl status velocity-report.service

# Inspect TLS generation output — it is a oneshot unit that runs before nginx
journalctl -u velocity-generate-tls.service -n 50 --no-pager

# Confirm the cert files landed
ls -la /var/lib/velocity-report/tls/     # expect ca.crt, ca.key, server.crt, server.key
```

**Do this**:

```bash
# Re-enable services if missing (e.g. image-build hiccup)
sudo systemctl enable --now velocity-generate-tls.service velocity-report.service

# The TLS script is idempotent and preserves the CA across regeneration, so operators
# do not need to re-trust the CA in their browser after a renewal:
sudo /usr/local/bin/velocity-generate-tls.sh
sudo systemctl reload nginx
```

### Certificate expired or about to expire

**Likely cause**: The server certificate is issued for ~825 days and auto-renews on boot when
within 24 hours of expiry. The CA is valid for ten years and is regenerated only if
missing or expired. If the Pi is powered off for long periods, certs can lapse.

**Do this**:

```bash
# Inspect validity
openssl x509 -in /var/lib/velocity-report/tls/server.crt -noout -dates

# Regenerate (reuses the existing CA — browsers that already trust it keep trusting)
sudo systemctl restart velocity-generate-tls.service
sudo systemctl reload nginx
```

---

## Pi system issues

### Clock drift / NTP not syncing

**What you see**: Transit timestamps jump or land in the future; PDF reports list the wrong
date; radar/LiDAR frames are occasionally rejected by ingest checks that assume a
monotonic clock.

**Likely cause**: The Pi image ships with `systemd-timesyncd`, not chrony. On a network with no
outbound UDP/123, or on first boot before DNS resolves, the clock can be hours or days off.

**Quick check**:

```bash
timedatectl status
# Look for: "System clock synchronized: yes" and "NTP service: active"

journalctl -u systemd-timesyncd.service -n 50 --no-pager
```

**Do this**:

```bash
# Enable NTP if disabled
sudo timedatectl set-ntp true

# If outbound 123/udp is blocked, point timesyncd at a reachable server
sudo mkdir -p /etc/systemd/timesyncd.conf.d
printf '[Time]\nNTP=time.cloudflare.com\n' | \
  sudo tee /etc/systemd/timesyncd.conf.d/local.conf
sudo systemctl restart systemd-timesyncd

# After the clock resyncs, restart the service so it stops carrying stale timestamps
sudo systemctl restart velocity-report
```

### SD card errors or filesystem remounted read-only

**What you see**: The Go server logs `readonly database` or `disk I/O error`; writes to the
DB fail; `velocity-ctl backup` reports `permission denied` despite running as root.

**Likely cause**: Raspberry Pi OS uses a writable ext4 root on the SD card. Power loss during
write and end-of-life wear both trigger ext4 to remount read-only. Once that happens,
every writer — server, migrations, backup — fails in confusing ways.

**Quick check**:

```bash
# Is the root filesystem read-only?
mount | awk '$3=="/" {print}'
findmnt -no OPTIONS /          # look for "ro" in the options list

# Kernel messages usually name the failing block device
sudo dmesg | grep -Ei 'ext4-fs error|remount.*read-only|I/O error' | tail -20
```

**Do this**:

```bash
# Stop the service before touching the filesystem
sudo systemctl stop velocity-report

# A clean remount is only a probe — if the kernel went read-only, it had a reason.
# Capture the current backup elsewhere, then plan for an SD-card replacement.
sudo cp -a /var/lib/velocity-report/sensor_data.db /media/usb/pre-replace.db

# fsck can only run on an unmounted or read-only root — reboot into rescue or shut
# down, pull the card, and fsck it from another host:
#   sudo fsck.ext4 -fy /dev/sdX2
# Do not run `fsck -y` on a mounted rw filesystem; it will damage more than it fixes.

# After replacing the card, re-flash the velocity image and restore the copied DB into
# /var/lib/velocity-report/ before starting the service for the first time.
```

---

## Performance issues

### High CPU usage

**What you see**: CPU at 100%, system sluggish

**Quick check**:

```bash
# Check which process is using CPU
top
# Or more detailed:
htop

# Check Go server goroutines. Pprof is mounted on the main listen port; access may be
# gated to loopback/tailscale by `tsweb.AllowDebugAccess` — a 403 here is expected from a
# remote host, run the curl from the Pi itself.
curl http://localhost:8080/debug/pprof/goroutine?debug=1

# Check database activity
lsof sensor_data.db
```

**Do this**:

- Restart Go server (background worker may be stuck)
- Reduce LiDAR frame rate if processing can't keep up
- Archive old data from database
- Check for runaway queries in logs

---

### High memory usage

**What you see**: Out of memory errors, system swapping

**Quick check**:

```bash
# Check memory usage
free -h

# Check Go server memory
ps aux | grep velocity-report-local

# Check for memory leaks
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

**Do this**:

- Reduce histogram bucket counts in API queries
- Limit query result sizes
- Add more swap space if needed
- If memory grows unboundedly, capture a heap profile (`curl /debug/pprof/heap`) and
  file an issue rather than papering over the leak with a cron restart

---

### Slow PDF generation

**What you see**: PDF generation takes several minutes

**Quick check**:

```bash
# Check API response time
time curl "http://localhost:8080/api/radar_stats?start=0&end=9999999999&group=1h&compute_histogram=true"

# Check LaTeX compilation time
time xelatex test.tex
```

**Do this**:

- Use smaller date ranges
- Disable histogram if not needed
- Use faster time grouping (24h instead of 15m)
- Generate charts at lower DPI
- Use faster LaTeX engine or fewer fonts

---

## Getting help

### Before asking for help

Please gather this information:

```bash
# System information
uname -a
cat /etc/os-release

# Go server version
./velocity-report-local --version

# Python version and packages
.venv/bin/python --version
.venv/bin/pip list

# Database info
sqlite3 sensor_data.db "SELECT sqlite_version();"
ls -lh sensor_data.db

# Recent logs. Logs can contain request paths with query parameters and client IPs; mask
# them before sharing outside the team:
journalctl -u velocity-report -n 100 \
  | sed -E 's/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/x.x.x.x/g' > logs.txt

# Config with sensitive fields stripped. config.json holds site coordinates, install
# address, and any API keys; share the redacted version, never the raw file:
jq 'del(
      .site.latitude, .site.longitude, .site.address,
      .site.installer, .site.contact_email
    )
    | del(.. | objects | .api_key?, .auth_token?, .password?)' \
  config.json > config.redacted.json
```

### Support channels

- **GitHub Issues**: https://github.com/banshee-data/velocity.report/issues
- **Documentation**: See [docs/README.md](docs/README.md) for all documentation
- **Email**: david@banshee-data.com

### Useful log commands

```bash
# Tail Go server logs
journalctl -u velocity-report -f

# Search for errors
journalctl -u velocity-report | grep -i error

# Export logs for specific time period
journalctl -u velocity-report --since "2025-01-01" --until "2025-01-02" > debug.log

# Enable debug logging
./velocity-report-local --debug
```

---

## Known fixed issues

Issues that were previously reported and have been resolved. Listed here
so operators on older versions can recognise the symptom and upgrade.

### LiDAR background grid: warmup trails (fixed January 2026)

**Symptom:** False positive foreground points ("trails") appearing on walls
and static surfaces for ~30 seconds after grid reset or service restart.

**Cause:** When a cell was reset (`TimesSeenCount=0`), `RangeSpreadMeters`
started at 0. The EMA took ~50–100 observations to learn true variance,
during which normal surface noise exceeded the threshold.

**Fix:** Warmup sensitivity scaling in `ProcessFramePolarWithMask()`: the
closeness threshold ramps from ~4× normal at count 0 down to 1× at count 100.
Vehicles (>1 m deviation) are still detected during warmup.

**Files:** [internal/lidar/l3grid/foreground.go](internal/lidar/l3grid/foreground.go), [internal/lidar/l3grid/foreground_warmup_test.go](internal/lidar/l3grid/foreground_warmup_test.go).

---

## macOS visualiser issues

### Build errors

#### "Unable to find module dependency: 'GRPCCore'"

Swift Package dependencies not resolved.

1. Open `VelocityVisualiser.xcodeproj` in Xcode
2. Wait for package resolution (may take several minutes on first run)
3. If packages don't resolve automatically:
   - File → Packages → Resolve Package Versions
   - File → Packages → Reset Package Caches
4. Clean build folder (⇧⌘K) and rebuild (⌘B)

#### "No such module 'SwiftProtobuf'"

1. In Xcode: File → Packages → Reset Package Caches
2. File → Packages → Resolve Package Versions
3. Clean build folder (⇧⌘K)
4. Build (⌘B)

#### Build succeeds but app crashes on launch

Metal device not available or shader compilation failure.

1. Ensure running on Apple Silicon or Intel Mac with Metal support
2. Check Console.app for `MetalRenderer` error messages
3. Try running from Xcode to see detailed crash logs

### Connection issues

#### "Server unreachable" or connection timeout

Go gRPC server not running or wrong address.

1. Start the server:
   ```bash
   go run ./cmd/tools/visualiser-server -addr localhost:50051
   ```
2. Verify the address in the app matches the server's `-addr` flag
3. Check for firewall blocking localhost connections

#### Connection succeeds but no frames received

1. Check server logs for "StreamFrames started" message
2. Verify the app is requesting the correct sensor ID
3. Try restarting both server and client

### Rendering issues

| Symptom                        | Cause                                        | Solution                                          |
| ------------------------------ | -------------------------------------------- | ------------------------------------------------- |
| Points not visible             | Points toggle disabled or point buffer empty | Enable "P" toggle; check point count in stats     |
| Boxes not visible              | Boxes toggle disabled or no tracked objects  | Enable "B" toggle; check server sends tracks      |
| Trails appear corrupted        | Bug in older builds                          | Rebuild with `make build-mac`                     |
| SwiftUI "AttributeGraph cycle" | High-frequency frame updates through SwiftUI | Informational only; does not affect functionality |

### Regenerating protobuf stubs

Generated files are gitignored. On a fresh clone, or after changing
`proto/velocity_visualiser/v1/visualiser.proto`, regenerate them:

```bash
# One-time: install pinned toolchain (protoc, swift-protobuf, grpc-swift-2)
./scripts/install-proto-tooling.sh

# Generate stubs for all languages
make proto-gen

# Or individual targets
make proto-gen-go    # Go stubs → internal/lidar/l9endpoints/pb/
make proto-gen-swift # Swift stubs → tools/visualiser-macos/VelocityVisualiser/gRPC/Generated/
```

`make build-mac` and `make test-mac` call `make proto-gen-swift` automatically.

### Missing `Velocity_Visualiser_V1_*` symbols in Xcode

If Xcode reports errors like `cannot find type 'Velocity_Visualiser_V1_Frame'`, the
generated Swift stubs are absent:

```bash
./scripts/install-proto-tooling.sh
make proto-gen-swift
```

---

## Common error messages reference

| Error Message                             | Component     | Solution                              |
| ----------------------------------------- | ------------- | ------------------------------------- |
| `bind: address already in use`            | Go Server     | Kill process on port 8080             |
| `database is locked`                      | Database      | Check for stale processes with `lsof` |
| `xelatex: command not found`              | PDF Generator | Install texlive-xetex                 |
| `ModuleNotFoundError`                     | PDF Generator | Activate venv, install requirements   |
| `cosine_error_angle is required`          | PDF Generator | Add field to config                   |
| `Failed to fetch`                         | Web Frontend  | Check API server is running           |
| `no such file or directory: /dev/ttyUSB0` | Go Server     | Check radar connection                |
| `no LiDAR packets received`               | Go Server     | Verify LiDAR network config           |
| `PRAGMA integrity_check: failed`          | Database      | Restore from backup                   |
| `403 Forbidden`                           | Web Server    | Check file permissions                |

---

## CI/CD issues

### Go CI tests fail with e2e error

**Error**: `ModuleNotFoundError: No module named 'numpy'` during Go tests

**Likely cause**: PDF generation E2E tests require Python dependencies

**Do this**:
API tests including E2E tests run in the CI `test-integration` job where Python dependencies are
installed. For local development:

```bash
# Option 1: Skip E2E tests using environment variable (recommended)
SKIP_PDF_TESTS=1 go test ./internal/api/...

# Option 2: Install Python dependencies and run all tests
make install-python
go test ./internal/api/...
```

---

### Web CI lint failures

**Error**: Prettier or ESLint failures in web-ci workflow

**Likely cause**: Code formatting inconsistency

**Do this**:

```bash
# Auto-fix formatting
cd web
pnpm run format

# Or from repository root
make format-web
```

---

### GitHub actions cache issues

**Error**: CI runs slower than expected or cache misses

**Likely cause**: Cache key mismatch or cache eviction

**Do this**:

```bash
# Clear and rebuild caches by pushing a commit that changes lock files
# Or wait for weekly cache eviction (7 days)

# Check cache usage in GitHub Actions UI:
# Repository → Actions → Caches
```
