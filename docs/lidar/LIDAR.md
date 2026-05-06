# LiDAR

Documentation for the velocity.report LiDAR subsystem (Hesai Pandar40P). The
canonical ten-layer processing architecture, package map, implementation
status, and literature alignment all live in
[architecture/LIDAR_ARCHITECTURE.md](architecture/LIDAR_ARCHITECTURE.md). This
file is just the index — start there for any layer detail.

## Folder structure

| Folder             | Scope                                                                  |
| ------------------ | ---------------------------------------------------------------------- |
| `architecture/`    | System design, layer specifications, and the canonical ten-layer model |
| `operations/`      | Runtime operations: data source switching, auto-tuning, debugging      |
| `troubleshooting/` | Resolved investigation notes for reference                             |

## Quick links

| Topic                   | Document                                                                                           |
| ----------------------- | -------------------------------------------------------------------------------------------------- |
| Ten-layer architecture  | [architecture/LIDAR_ARCHITECTURE.md](architecture/LIDAR_ARCHITECTURE.md)                           |
| Pipeline component map  | [architecture/lidar-pipeline-reference.md](architecture/lidar-pipeline-reference.md)               |
| Tracking implementation | [architecture/foreground-tracking.md](architecture/foreground-tracking.md)                         |
| Packet format           | [../../data/structures/HESAI_PACKET_FORMAT.md](../../data/structures/HESAI_PACKET_FORMAT.md)       |
| Auto-tuning             | [operations/auto-tuning.md](operations/auto-tuning.md)                                             |
| Track labelling         | [operations/track-labelling-ui-implementation.md](operations/track-labelling-ui-implementation.md) |
| macOS visualiser        | [../ui/visualiser/architecture.md](../ui/visualiser/architecture.md)                               |
| Backlog                 | [../BACKLOG.md](../BACKLOG.md)                                                                     |

## Terminology

Core domain terms for the LiDAR tracking system. Tuning-pipeline terms (sweep,
combo, round, auto-tune, HINT) live in
[operations/tuning-guide.md §Glossary](operations/tuning-guide.md#glossary).

| Term                   | Definition                                                                                                                                                   |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Point**              | A single 3D measurement from the LiDAR sensor (x, y, z, intensity, timestamp).                                                                               |
| **Cluster**            | A group of spatially-proximate foreground points identified by DBSCAN, representing a potential object.                                                      |
| **Track**              | A temporally-linked sequence of clusters representing a moving object across frames, maintained by the Kalman-filter tracker.                                |
| **Observation**        | A single cluster-to-track association at one point in time (one frame's measurement of a track).                                                             |
| **Scene**              | A named collection of reference ground-truth labels for a specific sensor environment (installation, angle, location). Used for evaluating tracking quality. |
| **Run** (Analysis Run) | A single processing pass over a data source (live or PCAP) with fixed parameters, producing tracks that can be compared against a scene's ground truth.      |

## Label taxonomy

Labels are applied by human reviewers to tracks within an analysis run. The same taxonomy is used across all platforms (Go backend, Svelte web frontend, macOS app).

### Detection labels (`user_label`)

Classify what object the track represents. Ships 7 active labels.

<!-- Canonical source: internal/api/lidar_labels.go → AllDetectionLabels -->

| Label        | Description                                              |
| ------------ | -------------------------------------------------------- |
| `car`        | Passenger car, SUV, van, or truck                        |
| `bus`        | Bus, coach, or large passenger vehicle (length > 7 m)    |
| `pedestrian` | Person walking, running, or using a mobility aid         |
| `cyclist`    | Person on a bicycle, e-scooter, or motorcycle            |
| `bird`       | Bird or other airborne fauna                             |
| `noise`      | Spurious track (sensor noise, rain, dust, or vegetation) |
| `dynamic`    | Ambiguous dynamic object or insufficient observations    |

### Quality flags (`quality_label`)

Rate the measurement quality of a track. Multi-select (comma-separated).

<!-- Canonical source: internal/api/lidar_labels.go → AllQualityFlags -->

| Flag              | Description                                                              |
| ----------------- | ------------------------------------------------------------------------ |
| `good`            | Clean, accurate track with correct speed and trajectory                  |
| `noisy`           | Track has noisy position or speed estimates                              |
| `jitter_velocity` | Speed estimates jitter significantly                                     |
| `jitter_heading`  | Heading estimates jitter significantly                                   |
| `merge`           | Two or more distinct objects incorrectly merged into one track           |
| `split`           | Single object incorrectly split into multiple tracks                     |
| `truncated`       | Track starts late or ends early relative to the object's true trajectory |
| `disconnected`    | Track was lost and recovered: identity may have changed                  |

### Canonical sources

- **Go backend:** [internal/api/lidar_labels.go](../../internal/api/lidar_labels.go); `validUserLabels`, `validQualityLabels`
- **Svelte frontend:** [web/src/lib/types/lidar.ts](../../web/src/lib/types/lidar.ts); `DetectionLabel`, `QualityLabel`
- **macOS app:** [tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift](../../tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift); `LabelPanelView`

> **Note:** `object_class` (e.g. `pedestrian`, `car`, `bird`, `dynamic`) is a _sensor-assigned_ classification from the tracker, not a human label. It is distinct from the `user_label` taxonomy above. Ships 7 classes: car, bus, pedestrian, cyclist, bird, dynamic, noise. Truck and motorcyclist are reserved for future use (proto enum values allocated but not user-assignable).
