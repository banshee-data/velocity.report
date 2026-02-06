# LiDAR Documentation

Documentation for the velocity.report LiDAR subsystem (Hesai Pandar40P integration).

## Folder Structure

### `architecture/`

Core system design and implementation specifications.

- **Technical overview** of the LiDAR processing pipeline
- **Foreground tracking pipeline** implementation (Phases 2.9–3.7)
- **Background grid standards** comparison with industry formats
- **AV range image format** design for dual-return support (future)

### `operations/`

Runtime operations and debugging.

- **Data source switching** between live UDP and PCAP replay
- **PCAP analysis mode** for background characterisation
- **Performance regression testing** for pipeline benchmarking
- **Tracking status** and known issues

### `reference/`

Protocol specifications and data formats.

- **Packet structure** for Hesai Pandar40P UDP format
- **Database schema** for LiDAR tables

### `roadmap/`

Development progress and future planning.

- **ML pipeline roadmap** (Phases 4.0–4.3: labeling UI, training, tuning)
- **Development log** with implementation notes

### `future/`

Deferred features for specialised use cases.

- **AV integration plan** (28-class taxonomy, Parquet ingestion)
- **Motion capture architecture** (moving sensors)
- **Velocity-coherent extraction** (alternative algorithm)
- **PCAP split tool** for motion/static segmentation
- **Static pose alignment** (7DOF tracking)

These are **not required** for static roadside deployments.

### `troubleshooting/`

Resolved investigation notes for reference.

- **Warmup trails fix** (January 2026)

### `noise_investigation/`

Historical analysis artifacts from background parameter tuning.

### `visualiser/`

Design documentation for the macOS-native 3D visualiser.

- **Problem statement** and user workflows
- **API contracts** (protobuf schema, gRPC service)
- **Architecture** (Track A visualiser, Track B pipeline refactor)
- **Implementation plan** (incremental milestones)

### `refactor/`

Tracking pipeline refactor and upgrade proposals.

- **Tracking upgrades** (ground removal, OBB, association, debug overlays)

## Quick Links

| Topic                   | Document                                                                                         |
| ----------------------- | ------------------------------------------------------------------------------------------------ |
| System overview         | [architecture/lidar_sidecar_overview.md](architecture/lidar_sidecar_overview.md)                 |
| Tracking implementation | [architecture/foreground_tracking_plan.md](architecture/foreground_tracking_plan.md)             |
| Current status          | [operations/lidar-foreground-tracking-status.md](operations/lidar-foreground-tracking-status.md) |
| ML pipeline             | [roadmap/ml_pipeline_roadmap.md](roadmap/ml_pipeline_roadmap.md)                                 |
| Packet format           | [reference/packet_analysis_results.md](reference/packet_analysis_results.md)                     |
| **macOS Visualiser**    | [visualiser/01-problem-and-user-workflows.md](visualiser/01-problem-and-user-workflows.md)       |
| **API Contracts**       | [visualiser/02-api-contracts.md](visualiser/02-api-contracts.md)                                 |
| **Tracking Upgrades**   | [refactor/01-tracking-upgrades.md](refactor/01-tracking-upgrades.md)                             |

## Implementation Status

**Completed through Phase 3.7:**

- ✅ UDP packet ingestion (Hesai Pandar40P)
- ✅ Frame assembly (360° rotations)
- ✅ Background learning (EMA-based polar grid)
- ✅ Foreground/background classification with warmup scaling
- ✅ Adaptive region parameters (variance-based segmentation)
- ✅ Region persistence & restoration (scene hash-based, skips settling on subsequent runs)
- ✅ DBSCAN clustering (world frame)
- ✅ Kalman tracking (constant velocity model)
- ✅ Rule-based classification (pedestrian, car, bird, other)
- ✅ REST API endpoints for tracks/clusters
- ✅ PCAP analysis tool for batch processing
- ✅ Analysis run infrastructure (params JSON, run comparison)
- ✅ Port 2370 foreground streaming

**Resolved Issues (January 2026):**

- ✅ Warmup trails (sensitivity scaling fix)
- ✅ Port 2370 packet corruption (RawBlockAzimuth preservation)
- ✅ recFg accumulation during freeze (reset on thaw)

**Planned (Phase 4.0+):**

- Track labeling UI (SvelteKit)
- ML classifier training pipeline
- Parameter optimisation with grid search
- Parameter tuning with split/merge metrics
- Production edge deployment

See `roadmap/ml_pipeline_roadmap.md` for detailed Phase 4.0–4.3 planning.
