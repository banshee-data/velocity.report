# LiDAR Data Layer Model (Six Layers)

## Purpose

A single concise layer model for LiDAR data in velocity.report. OSI is only a reference point; this model uses six practical layers that match current implementation and future AV compatibility work.

## The six layers

| Layer        | Label      | Scope                                              | Typical forms                                                                |
| ------------ | ---------- | -------------------------------------------------- | ---------------------------------------------------------------------------- |
| L1 (lowest)  | Packets    | Sensor-wire transport and capture                  | Hesai UDP payloads, PCAP packets                                             |
| L2           | Frames     | Time-coherent frame assembly and geometry exports  | `PointPolar`, `LiDARFrame`, Cartesian points, ASC/LidarView export feed      |
| L3           | Grid       | Background/foreground separation state             | `BackgroundGrid`, ring/azimuth bins, foreground mask                         |
| L4           | Perception | Per-frame object primitives and measurements       | Clusters and observations (`WorldCluster`, `TrackObservation`), ground plane surface model (`GroundSurface` interface) |
| L5           | Tracks     | Multi-frame identity and motion continuity         | `TrackedObject`, `TrackSet`                                                  |
| L6 (highest) | Objects    | Semantic object interpretation and dataset mapping | Local classes (`car`, `pedestrian`, `bird`, `other`) and AV taxonomy mapping |

## Artefact placement in this model

- Sensor packets → **L1 Packets**
- Frames and Cartesian representations → **L2 Frames**
- Background/foreground grid → **L3 Grid**
- Clusters and observations → **L4 Perception**
- Ground plane surface model → **L4 Perception** (non-point-based `GroundSurface` interface)
- Vector scene map → **L4 Perception** (polygon features for ground, structures, volumes; derived from ground plane; see `vector-scene-map.md`)
- Tracks → **L5 Tracks**
- Objects/classes → **L6 Objects**
- VRLOG recordings span **L2-L5** (frame bundles, perception outputs, tracks)
- LidarView exports primarily sit at **L2** (frame/geometry view)

## Current repository alignment

| ------------- | ------------------------------ | -------------------------------------------------------------------------------------------- |
| L1 Packets    | `internal/lidar/l1packets/`    | Facade over `network/` (UDP/PCAP) and `parse/` (Pandar40P)                                   |
| L2 Frames     | `internal/lidar/l2frames/`     | `frame_builder.go`, `export.go`, `geometry.go`                                               |
| L3 Grid       | `internal/lidar/l3grid/`       | `background.go`, `background_persistence.go`, `background_export.go`, `background_drift.go`, `foreground.go`, `config.go` |
| L4 Perception | `internal/lidar/l4perception/` | `cluster.go`, `dbscan_clusterer.go`, `ground.go`, `voxel.go`, `obb.go`, ground plane (planned) |
| L5 Tracks     | `internal/lidar/l5tracks/`     | `tracking.go`, `hungarian.go`, `tracker_interface.go`                                        |
| L6 Objects    | `internal/lidar/l6objects/`    | `classification.go`, `features.go`, `quality.go`, `comparison.go`                            |

Cross-cutting packages:

| Package                          | Purpose                                                                |
| -------------------------------- | ---------------------------------------------------------------------- |
| `internal/lidar/pipeline/`       | Orchestration (stage interfaces)                                       |
| `internal/lidar/storage/sqlite/` | DB repositories (scene, track, evaluation, sweep, analysis run stores) |
| `internal/lidar/adapters/`       | Transport and IO boundaries                                            |

Backward-compatible type aliases remain in the parent `internal/lidar/` package so existing callers continue to work.

## AV compatibility note

This six-layer model keeps traffic monitoring simple while preserving a clean upgrade path: AV 28-class compatibility is treated as an **L6 Objects** concern, without forcing AV complexity into packet/frame/grid/tracking layers.
