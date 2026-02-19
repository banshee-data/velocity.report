# LiDAR Architecture

Current architecture documentation for the velocity.report LiDAR subsystem.

## Six-Layer Model

The LiDAR pipeline uses a six-layer model. Each layer has a canonical Go package under `internal/lidar/`:

| Layer | Package | Responsibility |
|-------|---------|---------------|
| L1 Packets | `l1packets/` | Sensor-wire transport: UDP ingestion, PCAP replay, Hesai packet parsing |
| L2 Frames | `l2frames/` | Time-coherent frame assembly, polar/Cartesian geometry, ASC export |
| L3 Grid | `l3grid/` | Background/foreground separation: EMA grid, regions, persistence, drift |
| L4 Perception | `l4perception/` | Per-frame object primitives: DBSCAN clustering, OBB, ground removal |
| L5 Tracks | `l5tracks/` | Multi-frame identity: Kalman tracking, Hungarian assignment, lifecycle |
| L6 Objects | `l6objects/` | Semantic interpretation: classification, quality scoring, comparison |

Cross-cutting packages:

| Package | Purpose |
|---------|---------|
| `pipeline/` | Orchestration via stage interfaces (`ForegroundStage`, `PerceptionStage`, etc.) |
| `storage/sqlite/` | DB repositories: scene, track, evaluation, sweep, analysis run stores |
| `adapters/` | Transport and IO: export, training data, ground truth evaluation |
| `sweep/` | Parameter sweep runner and auto-tuner (interface-only layer coupling) |
| `monitor/` | HTTP server, API handlers, ECharts dashboards, data source management |

**Dependency rule**: `L(n)` may depend on `L(n-1)` and below, never upward. SQL/DB code lives in `storage/`, not in domain layers.

For the full layer model specification, see [lidar-data-layer-model.md](lidar-data-layer-model.md).

## Architecture Documents

### Current (active)

| Document | Scope |
|----------|-------|
| [lidar-data-layer-model.md](lidar-data-layer-model.md) | Canonical six-layer model with package mapping |
| [lidar-layer-alignment-refactor-review-20260217.md](lidar-layer-alignment-refactor-review-20260217.md) | Layer alignment review: completed migration, complexity analysis, file splits |
| [lidar-logging-stream-split-and-rubric-design-20260217.md](lidar-logging-stream-split-and-rubric-design-20260217.md) | Planned: ops/debug/trace logging streams |
| [foreground_tracking.md](foreground_tracking.md) | Foreground extraction and tracking pipeline design |
| [lidar-background-grid-standards.md](lidar-background-grid-standards.md) | Background grid format comparison with industry standards |
| [hesai_packet_structure.md](hesai_packet_structure.md) | Hesai Pandar40P UDP packet format reference |
| [lidar_sidecar_overview.md](lidar_sidecar_overview.md) | System-level overview of the LiDAR sidecar architecture |

### Historical (completed designs)

| Document | Status |
|----------|--------|
| [arena-go-deprecation-and-layered-type-layout-design-20260217.md](arena-go-deprecation-and-layered-type-layout-design-20260217.md) | ✅ Complete — arena.go removed, types migrated to layer packages |

### Future / Research

| Document | Scope |
|----------|-------|
| [av-range-image-format-alignment.md](av-range-image-format-alignment.md) | AV dual-return range image format (deferred) |
| [dynamic-algorithm-selection.md](dynamic-algorithm-selection.md) | Runtime algorithm switching (deferred) |

## Implementation Status

The layer alignment migration is **complete** (items 1–12, 14 in the review doc). Remaining:

- **Item 13**: Frontend decomposition (tracksStore, runsStore, missedRegionStore) — see [BACKLOG.md](/BACKLOG.md)

Post-migration file sizes:

| File | Lines | Notes |
|------|-------|-------|
| `l3grid/background.go` | 1,628 | Core grid processing (split from 2,610) |
| `l3grid/background_persistence.go` | 450 | Snapshot serialisation, DB restore/persist |
| `l3grid/background_export.go` | 350 | Heatmaps, ASC export, region debug info |
| `l3grid/background_drift.go` | 245 | M3.5 sensor movement and drift detection |
| `monitor/webserver.go` | 2,749 | Server init, routes, handlers (split from 4,067) |
| `monitor/datasource_handlers.go` | 682 | UDP/PCAP data source management |
| `monitor/playback_handlers.go` | 589 | PCAP/VRLOG playback controls |
| `storage/sqlite/analysis_run.go` | 1,325 | Run CRUD (domain logic moved to l6objects) |
| `l6objects/comparison.go` | 81 | ComputeTemporalIoU, comparison types |
