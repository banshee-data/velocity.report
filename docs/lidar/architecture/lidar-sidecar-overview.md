# LiDAR Sidecar — Technical Implementation Overview

- **Status:** Phase 3.9 completed — All core features operational
- **Scope:** Hesai UDP → parse → frame assembly → background subtraction → foreground mask → clustering → tracking → classification → HTTP API → classification research data export → Analysis Runs → Sweep/Auto-Tune

Technical overview of the LiDAR sidecar: Hesai UDP ingestion through background subtraction, clustering, tracking, classification, and sweep-based parameter tuning.

---

## Implementation Status

Phases 1–3.9 complete: UDP ingestion, Hesai Pandar40P parsing, time-based frame assembly, background subtraction (EMA learning, neighbour voting, cell freezing), foreground mask extraction, polar→world transform, DBSCAN clustering, Kalman tracking (Hungarian assignment, OBB estimation, occlusion coasting, ground removal), rule-based classification (pedestrian/car/bird/other), SQL persistence, REST API endpoints, PCAP replay with runtime source switching, classification research data export, analysis run infrastructure, adaptive region segmentation, parameter sweep system with auto-tuner, and debug overlays via gRPC.

### Phase 4 (Planned)

- **Phase 4.0:** Track labelling UI, `lidar_transits` table for dashboard integration
- **Phase 4.0:** LiDAR transit table for dashboard integration
- **Phase 4.1:** Optional classification benchmarking — feature extraction, offline comparisons
- **Future:** Multi-sensor architecture, world-frame tracking, cross-sensor association, distributed storage
- ✅ **Track Visualisation UI**: SvelteKit components for track history playback (implemented)

> **See also:** [LiDAR Pipeline Reference](lidar-pipeline-reference.md) for Phase 4.0-4.3 plans ([labelling](../../plans/lidar-track-labelling-auto-aware-tuning-plan.md), [metrics-first data science](../../plans/platform-data-science-metrics-first-plan.md), [optional classification](../../plans/lidar-ml-classifier-training-plan.md), [parameter tuning](../../plans/lidar-parameter-tuning-optimisation-plan.md), production deployment)

---

## Module Structure

The LiDAR code is organised across `cmd/` (binaries: `radar`, `bg-sweep`, `bg-multisweep`, `pcap-analyze`) and `internal/lidar/` (core pipeline: `network/`, `parse/`, `frame_builder.go`, `background.go`, `foreground.go`, `clustering.go`, `tracking.go`, `classification.go`, `track_store.go`, `analysis_run.go`, `hungarian.go`, `ground.go`, `obb.go`, `debug/`, `sweep/`, `monitor/`, `training_data.go`, `export.go`, `arena.go`, `voxel.go`). Database migrations in `internal/db/migrations/000009_*` and `000010_*`. Web components in `web/src/lib/components/lidar/` and `web/src/routes/lidar/tracks/`.

**Data Flow:**

```
[UDP:2369] → [Parse] → [Frame Builder] → [Background (sensor)] → [Foreground Mask]
                                                                        ↓
                                                               ProcessFramePolarWithMask()
                                                                        ↓
                                                           ExtractForegroundPoints()
                                                                        ↓
                                                             TransformToWorld()
                                                                        ↓
                                                                  DBSCAN()
                                                                        ↓
                                                              Tracker.Update()
                                                                        ↓
[HTTP API] ← [Database Persistence] ← [Confirmed Tracks] ← [Track Lifecycle]
```

---

## Core Algorithm Implementation

### UDP Ingestion & Parsing (✅ Complete)

- **UDP Listener**: Configurable port (default 2369), 4MB receive buffer
- **Packet Validation**: 1262-byte (standard) or 1266-byte (with sequence) packets
- **Tail Parsing**: Complete 30-byte structure per official Hesai documentation
- **Point Generation**: 40 channels × 10 blocks = up to 400 points per packet
- **Calibration**: Embedded per-channel angle and firetime corrections
- **Coordinate Transform**: Spherical → Cartesian with calibration applied

### Frame Assembly (✅ Complete)

- **Hybrid Frame Detection**: Motor speed-adaptive timing + azimuth validation (prevents timing anomalies)
- **Time-based Primary**: Frame completion when duration exceeds expected time (RPM-based) + 10% tolerance
- **Azimuth Secondary**: Azimuth wrap detection (340° → 20°) respects timing constraints
- **Enhanced Wrap Detection**: Catches large negative azimuth jumps (>180°) like 289° → 61° transitions
- **Traditional Fallback**: Pure azimuth-based detection (350° → 10°) when motor speed unavailable
- **Late Packet Handling**: 1-second buffer for out-of-order packets before final callback
- **Frame Callback**: Configurable callback for frame completion
- **Buffer Management**: Fixed eviction bug - frames now properly finalized when evicted from buffer
- **Frame Buffer**: Holds up to 100 frames with 500ms timeout before cleanup
- **Cleanup Interval**: 250ms periodic sweep for frame finalization (configurable for PCAP mode)

### Database Persistence (✅ Complete)

- **SQLite with WAL**: High-performance concurrent access
- **Performance Optimised**: Prepared statements, batch inserts

### Background Model & Classification (✅ Complete)

The system implements EMA-based background model learning with foreground/background classification per observation. Key features: closeness threshold with distance-adaptive noise scaling, same-ring neighbour voting, cell freezing after large divergence, foreground mask extraction via `ProcessFramePolarWithMask()`, and foreground point filtering via `ExtractForegroundPoints()`.

> **Source:** Algorithm details and parameters in `internal/lidar/l3grid/` (background model) and `internal/lidar/foreground.go`. Grid: 40 rings × 1800 azimuth bins (0.2° resolution).

### Polar → World Transform (✅ Complete)

- **Location**: `internal/lidar/clustering.go`
- **`TransformToWorld()`**: Converts polar points to world-frame Cartesian coordinates
- **Identity Transform**: Currently uses identity transform (sensor frame = world frame)
- **`TransformPointsToWorld()`**: Convenience function for pre-computed Cartesian points

> **Future Work:** Pose-based transformations using 4x4 homogeneous matrices are planned for a future phase.

### Clustering (✅ Complete)

- **Location**: `internal/lidar/clustering.go`
- **Algorithm**: DBSCAN with required spatial index
- **Euclidean clustering**: eps = 0.6m (configurable), minPts = 12 (configurable)
- **`SpatialIndex`**: Grid-based indexing using Szudzik pairing with zigzag encoding for O(1) neighbour queries
- **Per-cluster metrics**: centroid, bounding box (length/width/height), height_p95, intensity_mean
- **`WorldCluster`** struct with all required features
- **2D Clustering**: Uses (x, y) for clustering, z for height features only

### Tracking (✅ Complete)

- **Location**: `internal/lidar/tracking.go`
- **State vector**: [x, y, velocity_x, velocity_y]
- **Constant-velocity Kalman filter** with configurable noise parameters
- **Association**: Mahalanobis distance gating for cluster-to-track association
- **`Tracker`**: Multi-object tracker with configurable parameters via `TrackerConfig`
- **`TrackedObject`**: Track state with Kalman filter, lifecycle counters, and aggregated features
- **Lifecycle States**: `Tentative` → `Confirmed` → `Deleted`
- **Track Management**:
  - Birth from unmatched clusters
  - Promotion after N consecutive hits (default: 3)
  - Deletion after N consecutive misses (default: 3)
  - Grace period for deleted tracks before cleanup
- **Speed Statistics**: Average speed, max speed, history for percentile computation
- **Aggregated Features**: Bounding box averages, height P95 max, intensity mean average

### Classification Research Data (✅ Complete)

- **Location**: `internal/lidar/training_data.go`
- **`ForegroundFrame`**: Export struct for foreground points with metadata
- **Compact Encoding**: 8 bytes per point (vs ~40+ bytes for struct)
- **`TrainingDataFilter`**: Filtering exported research frames by sensor, sequence, and foreground count
- **Storage Format**: Polar (sensor) frame for pose independence

> **Future Work:** Pose validation and quality-based filtering for classification research datasets are planned for a future phase.

---

## Configuration

LiDAR is integrated into the `cmd/radar/radar.go` binary and enabled via `--enable-lidar`. See `velocity-report --help` for all flags including sensor/network settings, background subtraction tuning, PCAP replay, and HTTP listen address. Sensor and network parameters are configured via the tuning config file (`l1.*` keys). Background subtraction parameters are runtime-adjustable via the HTTP API (`/api/lidar/params`).

> **Source:** `BackgroundParams` fields and defaults in `internal/lidar/l3grid/`. Clustering and tracking parameters in `internal/lidar/tracking.go`.

### PCAP Replay Workflow

PCAP replay uses runtime data source switching via the HTTP API:

```bash
# Start PCAP replay (resets grid, switches from live UDP)
curl -X POST "http://localhost:8081/api/lidar/pcap/start?sensor_id=hesai-pandar40p" \
  -H "Content-Type: application/json" \
  -d '{"pcap_file": "cars.pcap"}'

# Return to live UDP
curl -X POST "http://localhost:8081/api/lidar/pcap/stop?sensor_id=hesai-pandar40p"
```

PCAP support requires the `pcap` build tag and libpcap. Safe directory restriction via `--lidar-pcap-dir` prevents path traversal. Sweep tools (`bg-sweep`, `bg-multisweep`) use the PCAP API automatically.

---

## Grid Analysis & Visualisation

### Grid Heatmap API

`GET /api/lidar/grid_heatmap` — aggregates the background grid (40 rings × 1800 azimuth bins) into coarse spatial buckets. Parameters: `sensor_id` (required), `azimuth_bucket_deg` (default 3.0), `settled_threshold` (default 5). Returns summary (total filled/settled, fill/settle rates) and per-bucket data.

### Visualisation Tools

```bash
# Polar or cartesian heatmap
python3 tools/grid-heatmap/plot_grid_heatmap.py \
  --url http://localhost:8081 --sensor hesai-pandar40p --metric unsettled_ratio

# Full 4K dashboard (polar + spatial + stacked metrics)
python3 tools/grid-heatmap/plot_grid_heatmap.py \
  --url http://localhost:8081 --output grid_dashboard.png
```

Available metrics: `fill_rate`, `settle_rate`, `unsettled_ratio`, `mean_times_seen`, `frozen_cells`. Supports live snapshot mode (`--live`), PCAP replay with periodic snapshots (`--pcap`), and noise sweep plotting (`plot_noise_sweep.py`, `plot_noise_buckets.py`).

---

## Debugging & Diagnostics

Enable debug mode via `--debug` flag or runtime API (`POST /api/lidar/params` with `enable_diagnostics: true`). Key log patterns: `[ResetGrid]`, `[ProcessFramePolar]`, `[FrameBuilder:finalize]`, `[BackgroundManager]`.

**Known issues (resolved):** FrameBuilder eviction bug — frames deleted without callback, now fixed with `finalizeFrame()` in eviction path. Enhanced azimuth wrap detection catches large negative jumps (>180°).

**Common diagnostics:** Low acceptance rates typically caused by cold start, tight noise thresholds at long range, or strict neighbour confirmation — rates converge to 99.8%+ after settling. For PCAP replay, reduce `CleanupInterval` to 50ms for fast packet rates.

**Runtime Parameter Adjustment**:

```bash
# Permissive settings for debugging (via API)
curl -X POST 'http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p' \
  -H 'Content-Type: application/json' \
  -d '{
    "noise_relative_fraction": 1.0,
    "closeness_sensitivity_multiplier": 10.0,
    "neighbor_confirmation_count": 1,
    "enable_diagnostics": true
  }'
```

**Grid Analysis**:

```bash
# Quick status check
curl "http://localhost:8081/api/lidar/grid_status?sensor_id=hesai-pandar40p" | jq

# Detailed heatmap for analysis
curl "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p" | \
  jq '.summary'

# Find buckets with low settlement
curl -s "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p" | \
  jq '.buckets[] | select(.filled_cells > 0 and .settled_cells == 0) | {ring, azimuth_deg_start, filled_cells}'

# Extract summary with custom thresholds
curl "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p&azimuth_bucket_deg=6.0&settled_threshold=10" | \
  jq '.summary'
```

**Background Parameter Sweep**:

```bash
# Single parameter sweep (noise_relative)
./bg-sweep -pcap-file=/path/to/cars.pcap -start=0.01 -end=0.3 -step=0.01

# Multi-parameter grid search
./bg-multisweep -pcap-file=/path/to/pedestrians.pcap \
  -closeness=2.0,3.0,4.0 \
  -neighbors=1,2,3 \
  -noise-start=0.01 -noise-end=0.5 -noise-step=0.05

# Fetch live nonzero counts (avoids DB timing races)
# Sweep tools use grid_status API for real-time metrics
```

**Makefile Targets**:

```bash
# Development workflow
make dev-go          # Build and run with auto-restart
make dev-go-pcap     # PCAP mode development
make log-go-tail     # Tail application logs
make log-go-cat      # View full logs

# Visualization
make plot-grid-heatmap URL=http://localhost:8081 SENSOR=hesai-pandar40p
```

---

## Security

### PCAP File Access Restriction

PCAP file access is restricted to a designated safe directory to prevent path traversal attacks.

**Configuration**:

- CLI flag: `--lidar-pcap-dir <path>` (default: `../sensor-data/lidar`)
- Only files within this directory tree can be accessed
- Absolute paths are converted to be relative to safe directory

**Security Features**:

- Path sanitization with `filepath.Clean()`
- Absolute path requirement verification
- Safe directory prefix validation
- Regular file type enforcement (no directories/symlinks/devices)
- File extension whitelist (`.pcap`, `.pcapng`)

**Usage Examples**:

```bash
# Valid: filename only
{"pcap_file": "cars.pcap"}

# Valid: relative path within safe dir
{"pcap_file": "subfolder/pedestrians.pcap"}

# Rejected: path traversal attempt
{"pcap_file": "../../../etc/passwd"}
# Returns: 403 Forbidden
```

**Systemd Integration**:
The service file automatically creates the safe directory on startup:

```ini
ExecStartPre=/bin/mkdir -p /home/david/sensor-data/lidar
ExecStart=/home/david/code/velocity.report/radar --lidar-pcap-dir /home/david/sensor-data/lidar
```

- Raspberry Pi builds (`radar-linux`) omit PCAP support by default to avoid cross-compile complexity
- If PCAP API is called without PCAP support, returns error: "PCAP support not enabled: rebuild with -tags=pcap"

**Benefits:**

- No server restart needed to change PCAP files
- BPF filtering by UDP port (supports multi-sensor PCAP files)
- Periodic background grid persistence for parameter evolution tracking
- Sweep tools automatically trigger PCAP replay before parameter testing

### BackgroundParams

All background parameters are runtime-adjustable via `POST /api/lidar/params`. See `internal/lidar/l3grid/` for the canonical `BackgroundParams` struct and defaults. Clustering and tracking parameters (`--cluster-eps`, `--cluster-min-points`, `--max-concurrent-tracks`) are configured via CLI flags — see `velocity-report --help`.

---

## HTTP Interface

Full endpoint list available via `GET /` dashboard on the running server. Key groups:

- **Health/status**: `GET /health`, `GET /`
- **Background params**: `GET/POST /api/lidar/params`, `POST /api/lidar/grid_reset`, `GET /api/lidar/grid_status`, `GET /api/lidar/grid_heatmap`, `GET /api/lidar/grid/export_asc`
- **Acceptance metrics**: `GET /api/lidar/acceptance` (with `?debug=true`), `POST /api/lidar/acceptance/reset`
- **PCAP replay**: `POST /api/lidar/pcap/start`, `POST /api/lidar/pcap/stop`, `GET /api/lidar/data_source`
- **Persistence**: `POST /api/lidar/persist`, `GET /api/lidar/snapshot`
- **Tracks** (Phase 3.5): `GET /api/lidar/tracks[/active|/summary|/{id}|/{id}/observations]`, `PUT /api/lidar/tracks/{id}`, `GET /api/lidar/clusters`

---

## Performance Metrics

- **Packet processing**: 36.5μs per packet, handles 10 Hz Pandar40P rate
- **Memory**: ~50MB baseline + 170KB per packet burst; target <300MB with 100 concurrent tracks
- **End-to-end latency target**: <100ms (packet → track update) on 1–2 cores

---

## Testing Status

Test packages: `internal/lidar/parse`, `internal/lidar/network`, `internal/lidar/monitor`, `internal/lidar/` (frame builder, background, export, integration). Run with `go test ./internal/lidar/... -v`. Coverage includes real Hesai packet validation, time-based frame assembly, end-to-end PCAP integration (76,934 points → 56,929 frame points), background grid learning, concurrent stress testing with race detection, and ASC export.

---

## Development Workflow

PCAP-based parameter tuning (Phase 2.5) and the full clustering/tracking pipeline (Phase 3) are complete. Sweep tools (`cmd/bg-sweep/`, `cmd/bg-multisweep/`) automate parameter exploration against real PCAP captures (cars, pedestrians). Next focus: Phase 4.0 — track labelling UI with ground truth evaluation.

---

## Architecture Notes

### Design Decisions

- **Sensor vs World Frame**: Background subtraction in sensor frame (stable geometry), tracking in world frame (stable coordinates)
- **Hybrid Frame Detection**: Time-based primary trigger + azimuth validation prevents timing anomalies
- **22-byte Tail**: Confirmed with official Hesai documentation and real packet validation
- **SQLite Database**: Selected for simplicity and performance in single-node deployment
- **Embedded Calibration**: Baked-in calibration avoids runtime configuration complexity

### Future Extensions

- **Multi-Sensor Deployment**: Multiple LiDAR units per machine with local storage
- **Database Consolidation**: Merge SQLite databases from multiple edge nodes
- **World Frame Tracking**: Unified tracking across sensor coverage areas
- **Cross-Intersection Analysis**: Track objects moving between multiple intersections
- **Radar Integration**: Modular architecture allows future radar fusion
- **Production Optimisation**: Memory pooling and advanced configuration options

---

## Production Readiness Assessment

Phases 1–3.9 complete. The full pipeline from UDP packets to classified, tracked objects is implemented, tested, and persisted to SQLite. REST API endpoints are ready for UI integration.

**Current focus**: Phase 4.0 — Track labelling UI with ground truth evaluation. Label API routes need wiring (`internal/api/lidar_labels.go` handlers exist, routes not registered in WebServer). See `docs/plans/lidar-track-labelling-auto-aware-tuning-plan.md` for the detailed 8-phase design.

**Future phases**: Pose-based world transform, multi-sensor deployment, database unification across edge nodes, cross-sensor tracking, and memory optimisation for 100+ concurrent tracks.
