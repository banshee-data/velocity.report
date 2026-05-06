# LiDAR foreground extraction and tracking — design rationale

This document records the **design decisions** behind the LiDAR foreground extraction and tracking pipeline: why background lives in polar coordinates, why clustering and tracking move to world coordinates, and where the locking and latency boundaries sit.

For the algorithm reference (EMA background, DBSCAN, Kalman + Hungarian, classification rules), see
[LIDAR_ARCHITECTURE.md](LIDAR_ARCHITECTURE.md). For the component inventory, see
[lidar-pipeline-reference.md](lidar-pipeline-reference.md). For the maths derivations, see
[`data/maths/`](../../../data/maths/).

---

## Architecture: polar vs world frame

The pipeline draws an explicit boundary between **sensor-centric polar
coordinates** (where the background grid and foreground classification live)
and **site-centric world coordinates** (where clustering, tracking, and
persistence live). Foreground points are extracted in polar, then transformed
once.

### Coordinate system boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│                    POLAR FRAME (Sensor-Centric)                 │
│                                                                 │
│  • Background Grid (40 rings × 1800 azimuth bins)               │
│  • EMA Learning (range, spread per cell)                        │
│  • Foreground/Background Classification                         │
│  • Neighbour Voting (same-ring only)                            │
│                                                                 │
│  Coordinates: (ring, azimuth_deg, range_m)                      │
│                                                                 │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         │ Single transform: foreground polar → world
                         │ Input: foreground polar points + sensor pose
                         │ Output: world Cartesian points
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                   WORLD FRAME (Site-Centric)                    │
│                                                                 │
│  • DBSCAN Clustering (Euclidean distance)                       │
│  • Kalman Tracking (position & velocity)                        │
│  • Track Classification (object type)                           │
│  • Database Persistence (clusters, tracks, observations)        │
│  • REST APIs (JSON responses)                                   │
│  • Web UI (visualisation)                                       │
│                                                                 │
│  Coordinates: (x, y, z) metres in site frame                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Key design decisions

1. **Background in polar.** Stable sensor geometry, efficient ring-based
   neighbour queries, and per-cell EMA semantics that match the physical
   measurement (one cell = one beam direction × one azimuth bin).
2. **Clustering in world.** Euclidean distance is consistent across the FOV;
   a cluster's spatial size shouldn't depend on its angular position relative
   to the sensor.
3. **Tracking in world.** Velocity estimation requires a fixed coordinate
   system. A track's `[X, Y, VX, VY]` state is meaningful only against an
   inertial site frame.
4. **No reverse transform.** World-frame components never convert back to
   polar. The boundary is one-directional.

The current production code path (`ProcessFramePolarWithMask()` →
`ExtractForegroundPolar()` → world transform → `Cluster()` → `Update()`)
follows this boundary exactly. The `AddPointsPolar()` materialiser used for
LiDARFrame, ASC, and LidarView export is a separate side-path and does not
feed the tracker.

---

## Performance and concurrency

### Locking boundaries

The background grid uses a `RWMutex` that is held only during polar-frame
classification and released before any world-frame work begins:

```
[Background Lock Held]
  - Classify points in polar space
  - Update EMA for background cells
  - Generate foreground mask

[Background Lock Released]
  - Extract foreground polar points
  - Transform polar → world
  - DBSCAN clustering
  - Kalman tracking
  - Database writes
  - API/UI updates
```

This separation matters because clustering and tracking are the expensive
stages; holding the background lock through them would serialise all
parameter reads and grid snapshots against the perception pipeline.

### Latency budget (per stage)

Target: **<100 ms end-to-end** at 10 Hz, with 10,000–20,000 points per frame.

| Stage                             | Target Latency | Notes                   |
| --------------------------------- | -------------- | ----------------------- |
| Background classification (polar) | <5 ms          | With background lock    |
| Foreground extraction             | <1 ms          | Simple mask application |
| Polar → World transform           | <3 ms          | Matrix multiplication   |
| DBSCAN clustering (world)         | <30 ms         | With spatial index      |
| Kalman tracking (world)           | <10 ms         | Association + update    |
| Database persistence              | <5 ms          | Async batch writes      |
| API/UI update                     | <5 ms          | Non-blocking            |
| **Total**                         | **<60 ms**     | Safety margin for 10 Hz |

Per-frame latency is collected by the `PipelineMetrics` struct in
[`internal/lidar/l8analytics/`](../../../internal/lidar/l8analytics/).

---

## Current operational status

### Working features

| Feature                     | Status     | Notes                                            |
| --------------------------- | ---------- | ------------------------------------------------ |
| Foreground Feed (Port 2370) | ✅ Working | Foreground points visible in LidarView           |
| Real-time Parameter Tuning  | ✅ Working | Edit params via JSON textarea without restart    |
| Background Subtraction      | ✅ Working | Points correctly masked as foreground/background |
| Warmup Sensitivity Scaling  | ✅ Working | Eliminates initialisation trails                 |
| PCAP Analysis Mode          | ✅ Working | Grid preserved for analysis workflows            |

### Resolved issues

**Packet corruption on port 2370.** The forwarder used to reconstruct
packets with incorrect azimuth values. Fixed by rewriting
`ForegroundForwarder` to preserve `RawBlockAzimuth` and `UDPSequence`.

**Foreground "trails" after object pass.** Points lingered as foreground for
~30 seconds. Two root causes: (1) warmup variance underestimation, fixed
with sensitivity scaling in `ProcessFramePolarWithMask()` (4× → 1× over 100
observations); (2) `recFg` accumulation during freeze, fixed by not
incrementing during freeze and resetting to 0 on thaw. See
[DEBUGGING.md §Known Fixed Issues](../../../DEBUGGING.md#lidar-background-grid--warmup-trails-fixed-january-2026).

**Real-time parameter tuning.** POST to `/api/lidar/params` with JSON body;
changes apply immediately without restart.

### Known limitations

- **CPU performance.** Foreground processing CPU usage is higher than
  expected on M1 hardware. Investigate with `go tool pprof` — likely
  per-frame allocations, lock contention, or packet encoding overhead.
- **Runtime tuning schema parity.** `/api/lidar/params` supports core
  background/tracker keys but not full canonical tuning parity for all
  runtime keys. `max_tracks` POST support is wired.

### Configuration reference

| Parameter                        | Default | Description                                |
| -------------------------------- | ------- | ------------------------------------------ |
| `BackgroundUpdateFraction`       | 0.02    | EMA alpha for background learning          |
| `ClosenessSensitivityMultiplier` | 3.0     | Threshold multiplier for classification    |
| `SafetyMarginMeters`             | 0.1     | Fixed margin added to threshold            |
| `NoiseRelativeFraction`          | 0.01    | Distance-proportional noise allowance      |
| `NeighborConfirmationCount`      | 3       | Neighbours needed to confirm background    |
| `FreezeDurationNanos`            | 5e9     | Cell freeze duration after large deviation |
| `SeedFromFirstObservation`       | true    | Initialise cells from first observation    |

Warmup sensitivity: cells with `TimesSeenCount < 100` have their threshold
multiplied by `1.0 + 3.0 × (100 − count) / 100` (4× at count 0, 1× at 100+).

### API endpoints

| Endpoint                 | Method   | Description                       |
| ------------------------ | -------- | --------------------------------- |
| `/api/lidar/status`      | GET      | Current pipeline status           |
| `/api/lidar/params`      | GET/POST | View/update background parameters |
| `/api/lidar/grid_status` | GET      | Background grid statistics        |
| `/api/lidar/grid_reset`  | GET      | Reset background grid             |
| `/api/lidar/pcap/start`  | POST     | Start PCAP replay                 |
| `/api/lidar/pcap/stop`   | POST     | Stop PCAP replay                  |
| `/api/lidar/data_source` | GET      | Current data source (live/pcap)   |

---

## Related documentation

- **[LiDAR Architecture](LIDAR_ARCHITECTURE.md)**: Canonical layer model, package map, and per-layer literature alignment
- **[LiDAR Pipeline Reference](lidar-pipeline-reference.md)**: Component inventory, data flow, and deployment topology
- **[Velocity-Coherent Foreground Extraction](../../plans/lidar-velocity-coherent-foreground-extraction-plan.md)**: Alternative algorithm design for sparse-point tracking with velocity coherence
- **[Development Log](../../DEVLOG.md)**: Chronological implementation history
