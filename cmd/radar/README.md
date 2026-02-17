# Radar binary (single-binary model)

This project uses a single `radar` binary which runs the radar serial monitor, HTTP API server, and (optionally) in-process LiDAR components when `--enable-lidar` is passed. All LiDAR functionality is integrated behind flags.

## Build

```bash
# Build the radar binary (includes optional LiDAR components)
go build ./cmd/radar
```

## Build

```bash
# Build radar
go build ./cmd/radar

# Build lidar (standalone)
go build ./cmd/lidar
```

## Run

```bash
# Run the radar server (serve DB and HTTP UI, talk to serial port):
./radar

# Run without hardware (serve DB/UI only):
./radar --disable-radar

# Run in debug mode with a mocked serial port (useful for development):
./radar --debug

# Force precompiled PDF LaTeX flow (minimal TeX tree):
./radar --disable-radar --pdf-latex-flow precompiled --pdf-tex-root /opt/velocity-report/texlive-minimal

# Force full system LaTeX flow (unset VELOCITY_TEX_ROOT):
./radar --disable-radar --pdf-latex-flow full

# Enable in-process LiDAR components (UDP listener + forwarder):
./radar --enable-lidar --lidar-udp-port 2369 --lidar-listen :8081
```

## Command-line flags

The radar binary exposes several CLI flags (see `cmd/radar/radar.go` for exact defaults). Key options are listed below.

- `--fixture` (bool) — Load fixture data into the local DB instead of opening a serial port.
- `--debug` (bool) — Run in debug mode (uses a mock serial mux and enables extra debug logging).
- `--db-path` (string) — Path to SQLite DB file (default: `sensor_data.db`). Use this when your DB file lives outside the current working directory (for example, systemd services).
- `--listen` (string) — HTTP listen address for the central API (default: `:8080`).
  - `--port` (string) — Serial device path for the radar (default: `/dev/ttySC1`). Ignored in `--debug` or `--disable-radar`.
- `--units` (string) — Display units (mps, mph, kmph). Default: `mph`.
- `--timezone` (string) — Timezone for display (default: `UTC`).
- `--disable-radar` (bool) — Disable radar serial I/O; useful when running without radar hardware. The HTTP server and DB remain active.
- `--pdf-latex-flow` (string) — PDF LaTeX mode: `inherit` (default), `precompiled`, or `full`.
  - `inherit`: leave `VELOCITY_TEX_ROOT` unchanged unless `--pdf-tex-root` is provided.
  - `precompiled`: validate and set `VELOCITY_TEX_ROOT` to the minimal TeX tree.
  - `full`: unset `VELOCITY_TEX_ROOT` and force full system TeX.
- `--pdf-tex-root` (string) — TeX root directory used by `precompiled` flow (expects `bin/xelatex` inside). In `inherit` mode, this can be used as an explicit override.
- `--listen` and `--port` must be set sensibly; the binary validates units/timezone on startup.

LiDAR integration flags (only relevant when `--enable-lidar` is supplied):

- `--enable-lidar` (bool) — Enable in-process LiDAR components inside the radar binary (UDP listener, parser, monitor).
- `--lidar-listen` (string) — HTTP listen address for the LiDAR monitor webserver (default: `:8081`).
- `--lidar-udp-port` (int) — UDP port to listen for LiDAR packets (default: `2369`).
- `--lidar-no-parse` (bool) — Disable LiDAR packet parsing (useful when only forwarding packets).
- `--lidar-sensor` (string) — Sensor identifier (used for BackgroundManager registration and DB records). Default: `hesai-pandar40p`.
- `--lidar-forward` (bool) — Forward incoming LiDAR packets to another port (useful for LidarView).
- `--lidar-forward-port` (int) — Forward destination port (default: `2368`).
- `--lidar-forward-addr` (string) — Forward destination address (default: `localhost`).
- `--lidar-pcap-dir` (string) — Safe directory for PCAP files (default: `../sensor-data/lidar`). Only files within this directory can be replayed via the API. This prevents path traversal attacks.

## PCAP Replay Setup

Runtime switching lets you replay captures without special startup flags:

1. **Create the PCAP directory** (default: `../sensor-data/lidar`):

   ```bash
   mkdir -p ../sensor-data/lidar
   ```

2. **Place your PCAP files** in this directory:

   ```bash
   cp /path/to/your/file.pcap ../sensor-data/lidar/
   ```

3. **Start the server** with LiDAR enabled (no dedicated PCAP mode required):

   ```bash
   ./radar --enable-lidar --lidar-pcap-dir ../sensor-data/lidar
   ```

4. **Switch to PCAP** via the API:

   ```bash
   curl -X POST "http://localhost:8081/api/lidar/pcap/start?sensor_id=hesai-pandar40p" \
     -H "Content-Type: application/json" \
     -d '{"pcap_file": "file.pcap"}'
   ```

   The server stops the live UDP listener, resets the background grid, and starts replaying the requested PCAP. If a replay is already running the endpoint returns `409 Conflict`; wait for completion (check `/api/lidar/data_source`) and retry.

5. **Switch back to live data** when finished:

   ```bash
   curl -X POST "http://localhost:8081/api/lidar/pcap/stop?sensor_id=hesai-pandar40p"
   ```

**Security Note**: The `--lidar-pcap-dir` flag restricts file access to prevent path traversal attacks. Only files within the specified directory (or its subdirectories) can be accessed. Attempting to access files outside this directory (e.g., using `../../../etc/passwd`) will be rejected with a 403 Forbidden error.

You can provide either a bare filename (`"pcap_file": "file.pcap"`) or a relative path within the safe directory (`"pcap_file": "subfolder/file.pcap"`). Absolute paths are automatically resolved relative to the safe directory.

## Architecture

The application is organized into separate components under `internal/` for maintainability:

- `internal/serialmux` — Serial port abstraction and event handlers (real, mock, disabled implementations).
- `internal/api` — Central HTTP server, static assets, and admin endpoints.
- `internal/db` — SQLite helpers and schema management (single DB file `sensor_data.db` by default). Use `--db-path` to point the binary at a different file (for example: `--db-path /var/lib/velocity-report/sensor_data.db`).
- `internal/lidar` — LiDAR parsers, frame builders, background model, monitor webserver, and DB persistence.

### LiDAR core components

- `forwarder.go` — Asynchronous packet forwarding with drop counting.
- `listener.go` — UDP packet reception and listener orchestration.
- `webserver.go` / monitor — HTTP UI for LiDAR stats and manual snapshot triggers.
- `parser.go` — Pandar40P LiDAR packet parsing.
- `stats.go` — Thread-safe packet/statistics tracking.

## Features & Performance

- High-performance UDP packet reception with configurable buffer sizes.
- Optional built-in parsing for Pandar40P into 3D points.
- Packet forwarding for integration with LidarView or other tools.
- Background model snapshotting: BackgroundManager serializes grid blobs (gob+gzip) and persists to `lidar_bg_snapshot` when a DB-backed store is present.

Performance notes (observed in production):

- **Packet Rate**: ~1800 packets/sec sustained throughput
- **Data Rate**: ~2.17 MB/sec with ~700k points/sec when parsing enabled
- **Point Processing**: ~700,000 3D points/sec from parsed Pandar40P packets
- **Forwarding Latency**: Microsecond-level with dedicated forwarding goroutine
- **Memory Usage**: Optimized buffering with 1000-packet forwarding buffer and configurable UDP receive buffer
- **CPU Usage**: Minimal overhead with direct packet processing (no per-packet goroutines)
- **Statistics Logging**: Configurable intervals (1-60 seconds) with colored output for errors

## Embedded Configuration

Both binaries include embedded Pandar40P sensor configuration files (angle and firetime corrections), so parsing works without external files.

## LidarView Integration

To visualize incoming LiDAR data, forward packets to LidarView's listening port (2368 by default) or configure a custom port and use the `--lidar-forward` flag.

## Command & DB notes

- The radar binary uses a single SQLite DB file by default (`sensor_data.db`) and exposes admin routes via the HTTP API. You can override the DB location by passing `--db-path /path/to/your.db` when starting the binary (this is how production systemd units should point the service at `/var/lib/velocity-report/sensor_data.db`).
- When `--disable-radar` is set, a `DisabledSerialMux` keeps the HTTP/DB APIs available while disabling serial I/O.
- When `--enable-lidar` is used, the LiDAR components reuse the same DB instance for snapshot persistence and event storage. A `BackgroundManager` is created per sensor and will persist background snapshots into the `lidar_bg_snapshot` table if the manager was constructed with a DB-backed `BgStore`.

## Systemd Service Setup

When running as a systemd service, the included `velocity-report.service` file:

- Automatically creates the PCAP safe directory (`/home/david/sensor-data/lidar`) on service start
- Passes the `--lidar-pcap-dir` flag to restrict PCAP file access to this directory
- Ensures proper permissions and directory structure before the service starts

To install and enable the service:

```bash
sudo cp velocity-report.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable velocity-report
sudo systemctl start velocity-report
```

## Troubleshooting

- If the radar device path is incorrect, use `--disable-radar` to run the server without hardware while you debug.
- If LiDAR snapshot persistence is not appearing in the DB, confirm `--enable-lidar` is used and that the `BackgroundManager` was created with a DB-backed `BgStore` (the binary wires the main DB by default when LiDAR is enabled).
- Port conflicts: use `lsof -i UDP:<port>` to find processes binding the UDP or HTTP ports.
- If LiDAR snapshots do not appear in the DB, confirm `--enable-lidar` is used and the `BackgroundManager` was created with a DB-backed store.
- No packets received: check firewall, UDP bind address, and network interface; use `netstat -un` to verify listeners.

### Port Conflicts

- **Error**: "bind: address already in use"
- **Solution**: Check for other processes using the UDP port with `lsof -i UDP:2369`

### LidarView Socket Errors

- **Error**: "Error while opening socket!" in LidarView
- **Solution**: Use different forwarding ports (avoid 2368 if LidarView binds to it)

### No Packets Received

- **Check**: Firewall settings, UDP port configuration, network interface binding
- **Debug**: Use `netstat -un` to verify UDP listener is active
