# LiDAR Documentation

Documentation for the velocity.report LiDAR subsystem (Hesai Pandar40P integration).

## Folder Structure

### `architecture/`

Core system design and implementation specifications.

- **Technical overview** of the LiDAR sidecar architecture
- **Foreground tracking pipeline** implementation plan (Phases 2.9–3.7)
- **Background grid standards** comparison with industry formats
- **Waymo format alignment** design for dual-return and range image support

Start here to understand how the system works.

### `operations/`

Runtime operations, debugging, and active development.

- **Data source switching** between live UDP and PCAP replay
- **PCAP analysis mode** for background characterization
- **Active issues** and debugging investigations (foreground trails, performance)

Consult when troubleshooting or operating the system.

### `reference/`

Protocol specifications and data formats.

- **Packet structure** analysis for Hesai Pandar40P UDP format
- **Database schema** for LiDAR tables (SQL and ERD)

Use for protocol-level debugging or schema reference.

### `roadmap/`

Development progress and future planning.

- **ML pipeline roadmap** (Phases 4.0–4.3: labeling UI, training, tuning)
- **Integration status** of tracking components
- **Development log** with chronological implementation notes

Check for implementation history and upcoming features.

### `future/`

Deferred features not needed for current traffic monitoring deployments.

- **AV integration plan** (28-class taxonomy, Parquet ingestion)
- **7DOF pose alignment** for AV dataset compatibility
- **Motion capture architecture** for moving sensors (vehicle/drone-mounted)
- **Velocity-coherent extraction** algorithm design
- **PCAP split tool** for motion/static segmentation

These are **not required** for static roadside sensor deployments. Implement when pursuing AV research or mobile sensor platforms.

### `noise_investigation/`

Historical analysis artifacts from background parameter tuning. Contains sweep results, acceptance rate plots, and CSV data from noise characterization experiments.

## Quick Links

| Topic                   | Document                                         |
| ----------------------- | ------------------------------------------------ |
| System overview         | `architecture/lidar_sidecar_overview.md`         |
| Tracking implementation | `architecture/foreground_tracking_plan.md`       |
| Waymo format alignment  | `architecture/waymo-format-alignment.md`         |
| Current issues          | `operations/lidar-foreground-tracking-status.md` |
| ML pipeline             | `roadmap/ml_pipeline_roadmap.md`                 |
| Packet format           | `reference/packet_analysis_results.md`           |

## Implementation Status

**Completed through Phase 3.7:**

- ✅ UDP packet ingestion (Hesai Pandar40P)
- ✅ Frame assembly (360° rotations)
- ✅ Background learning (EMA-based polar grid)
- ✅ Foreground/background classification
- ✅ DBSCAN clustering (world frame)
- ✅ Kalman tracking (constant velocity model)
- ✅ Rule-based classification (pedestrian, car, bird, other)
- ✅ REST API endpoints for tracks/clusters
- ✅ PCAP analysis tool for batch processing
- ✅ Analysis run infrastructure (params JSON, run comparison)

**Active Development:**

- 🚧 Foreground "trail" artifacts after objects pass
- 🚧 M1 Mac CPU usage optimization
- 🚧 Port 2370 foreground streaming performance

**Planned (Phase 4.0+):**

- Track labeling UI (SvelteKit)
- ML classifier training pipeline
- Parameter tuning with split/merge metrics
- Production edge deployment

See `roadmap/ml_pipeline_roadmap.md` for detailed Phase 4.0–4.3 planning.
