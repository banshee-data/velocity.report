# LiDAR documentation

Documentation for the velocity.report LiDAR subsystem (Hesai Pandar40P integration).

For the canonical ten-layer processing stack and per-block implementation status,
see [architecture/lidar-data-layer-model.md](architecture/lidar-data-layer-model.md).

## Folder structure

### `architecture/`

Core system design and implementation specifications. See [architecture/README.md](architecture/README.md) for the consolidated architecture overview, and [architecture/lidar-data-layer-model.md](architecture/lidar-data-layer-model.md) for the canonical ten-layer stack reference plus the detailed concept/algorithm status chart.

### `operations/`

Runtime operations and debugging.

- **Data source switching** between live UDP and PCAP replay
- **PCAP analysis mode** for background characterisation
- **Performance regression testing** for pipeline benchmarking
- **Auto-tuning** parameter sweep and iterative grid narrowing
- **Settling time optimisation** for efficient parameter sweeps
- **Tracking status** and known issues

### `reference/`

Protocol specifications and data formats.

- **Packet structure** for Hesai Pandar40P UDP format
- **Database schema** for LiDAR tables

### `roadmap/`

Development progress and future planning.

- **Metrics-first data science roadmap** (Phases 4.0–4.3: labelling UI, scorecards, optional classification research, tuning)
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
- **Labelling + QC enhancements** (quality scoring, event timeline, repairs, queueing, dashboard)

### `refactor/`

Tracking pipeline refactor and upgrade proposals.

- **Tracking upgrades** (ground removal, OBB, association, debug overlays)

## Quick links

| Topic                   | Document                                                                                                     |
| ----------------------- | ------------------------------------------------------------------------------------------------------------ |
| Layer stack + status    | [architecture/lidar-data-layer-model.md](architecture/lidar-data-layer-model.md)                             |
| System overview         | [architecture/lidar-sidecar-overview.md](architecture/lidar-sidecar-overview.md)                             |
| Tracking implementation | [architecture/foreground-tracking.md](architecture/foreground-tracking.md)                                   |
| Packet format           | [../../data/structures/HESAI_PACKET_FORMAT.md](../../data/structures/HESAI_PACKET_FORMAT.md)                 |
| Auto-tuning             | [operations/auto-tuning.md](operations/auto-tuning.md)                                                       |
| Track labelling         | [operations/track-labelling-ui-implementation.md](operations/track-labelling-ui-implementation.md)           |
| macOS visualiser        | [../ui/visualiser/architecture.md](../ui/visualiser/architecture.md)                                         |
| API contracts           | [../ui/visualiser/api-contracts.md](../ui/visualiser/api-contracts.md)                                       |
| Data science plan       | [../plans/platform-data-science-metrics-first-plan.md](../plans/platform-data-science-metrics-first-plan.md) |
| Backlog                 | [../BACKLOG.md](../BACKLOG.md)                                                                               |

## Implementation status

L1–L6 are fully implemented and in production. L7 (scene) is planned. L8–L10
are structurally present and being formalised.

For the complete per-layer and per-algorithm status, see the ten-layer model:
[architecture/lidar-data-layer-model.md](architecture/lidar-data-layer-model.md).
