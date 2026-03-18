# Data Structures & Wire Formats

This directory documents the binary and on-disk data formats used by the
velocity.report system. Each format has a dedicated specification file.

## Documented Formats

| Format            | Document                                         | Layer | Description                                        |
| ----------------- | ------------------------------------------------ | ----- | -------------------------------------------------- |
| Hesai UDP packets | [HESAI_PACKET_FORMAT.md](HESAI_PACKET_FORMAT.md) | L1    | Hesai Pandar40P LiDAR UDP payload structure        |
| VRLOG recording   | [VRLOG_FORMAT.md](VRLOG_FORMAT.md)               | L2–L5 | Directory-based LiDAR frame recording format       |
| Surface matrix    | [MATRIX.md](MATRIX.md)                           | L4–L8 | Backend-computed data vs web/PDF/macOS consumption |

## Canonical Source Definitions

These structures are defined in code rather than standalone docs. Links below
point to the authoritative source files.

### SQLite Database

| Definition     | File                                                       | Description                                 |
| -------------- | ---------------------------------------------------------- | ------------------------------------------- |
| Current schema | [`internal/db/schema.sql`](../../internal/db/schema.sql)   | 18 tables — radar, LiDAR, labelling, sweeps |
| Migrations     | [`internal/db/migrations/`](../../internal/db/migrations/) | Incremental schema evolution                |

### Configuration

| Definition        | File                                                               | Description                                  |
| ----------------- | ------------------------------------------------------------------ | -------------------------------------------- |
| Tuning parameters | [`config/README.md`](../../config/README.md)                       | ~40 tuning knobs with types and defaults     |
| Default values    | [`config/tuning.defaults.json`](../../config/tuning.defaults.json) | Canonical defaults                           |
| Tuning Go struct  | [`internal/config/tuning.go`](../../internal/config/tuning.go)     | `TuningConfig` with JSON tags and validation |

### Protobuf / gRPC

| Definition          | File                                                                                                   | Description                                                |
| ------------------- | ------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------- |
| Visualiser gRPC API | [`proto/velocity_visualiser/v1/visualiser.proto`](../../proto/velocity_visualiser/v1/visualiser.proto) | FrameBundle, PointCloudFrame, TrackSet, PlaybackInfo, etc. |

### Internal Models

| Definition               | File                                                                                                           | Description                               |
| ------------------------ | -------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| Data layer model         | [`docs/lidar/architecture/lidar-data-layer-model.md`](../../docs/lidar/architecture/lidar-data-layer-model.md) | Six-layer model (L1 Packets → L6 Objects) |
| FrameBundle (Go)         | [`internal/lidar/visualiser/model.go`](../../internal/lidar/visualiser/model.go)                               | Canonical internal model for LiDAR frames |
| Recorder / Replayer (Go) | [`internal/lidar/visualiser/recorder/recorder.go`](../../internal/lidar/visualiser/recorder/recorder.go)       | VRLOG read/write logic                    |

## TODO — Candidates for Dedicated Documentation

The following data structures would benefit from standalone format specifications.
Contributions welcome.

- [ ] **SQLite schema reference** — table-by-table documentation of all 18 tables,
      column semantics, computed columns, triggers, and index rationale.
      Source: [`internal/db/schema.sql`](../../internal/db/schema.sql)

- [ ] **Radar JSON event format** — structure of raw radar events stored in
      `radar_data.data_json` (computed columns: speed, magnitude, uptime).
      Source: [`internal/db/schema.sql`](../../internal/db/schema.sql) (`radar_data` table)

- [ ] **Transit record format** — how vehicle transits are computed and stored
      in `radar_data_transits` (speed stats, duration, gap analysis).
      Source: [`internal/db/schema.sql`](../../internal/db/schema.sql) (`radar_data_transits` table)

- [ ] **PDF report data contract** — inputs consumed by the Python PDF generator
      (API response shapes, CSV exports, chart specifications).
      Source: [`tools/pdf-generator/`](../../tools/pdf-generator/)

- [ ] **Sweep configuration format** — JSON schema for parameter sweep definitions
      (sweep-overnight, velocity-jitter, quality-tuning).
      Source: [`config/sweep-*.json`](../../config/)

- [ ] **Background grid persistence format** — `lidar_bg_snapshot` and
      `lidar_bg_regions` JSON blobs (variance data, region geometry).
      Source: [`internal/db/schema.sql`](../../internal/db/schema.sql)

- [ ] **Label / evaluation format** — `lidar_labels`, `lidar_scenes`,
      `lidar_evaluations` table semantics and JSON fields.
      Source: [`internal/db/schema.sql`](../../internal/db/schema.sql)
