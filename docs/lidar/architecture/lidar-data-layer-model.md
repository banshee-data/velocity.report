# LiDAR Data Layer Model (Six Layers)

## Purpose

A single concise layer model for LiDAR data in velocity.report. OSI is only a reference point; this model uses six practical layers that match current implementation and future AV compatibility work.

## The six layers

| Layer        | Label      | Scope                                              | Typical forms                                                                |
| ------------ | ---------- | -------------------------------------------------- | ---------------------------------------------------------------------------- |
| L1 (lowest)  | Packets    | Sensor-wire transport and capture                  | Hesai UDP payloads, PCAP packets                                             |
| L2           | Frames     | Time-coherent frame assembly and geometry exports  | `PointPolar`, `LiDARFrame`, Cartesian points, ASC/LidarView export feed      |
| L3           | Grid       | Background/foreground separation state             | `BackgroundGrid`, ring/azimuth bins, foreground mask                         |
| L4           | Perception | Per-frame object primitives and measurements       | Clusters and observations (`WorldCluster`, `TrackObservation`)               |
| L5           | Tracks     | Multi-frame identity and motion continuity         | `TrackedObject`, `TrackSet`                                                  |
| L6 (highest) | Objects    | Semantic object interpretation and dataset mapping | Local classes (`car`, `pedestrian`, `bird`, `other`) and AV taxonomy mapping |

## Artefact placement in this model

- Sensor packets → **L1 Packets**
- Frames and Cartesian representations → **L2 Frames**
- Background/foreground grid → **L3 Grid**
- Clusters and observations → **L4 Perception**
- Tracks → **L5 Tracks**
- Objects/classes → **L6 Objects**
- VRLOG recordings span **L2-L5** (frame bundles, perception outputs, tracks)
- LidarView exports primarily sit at **L2** (frame/geometry view)

## Current repository alignment

| Layer         | Primary anchors in repo                                                                                   |
| ------------- | --------------------------------------------------------------------------------------------------------- |
| L1 Packets    | `internal/lidar/network/listener.go`, `internal/lidar/network/pcap.go`, `internal/lidar/parse/extract.go` |
| L2 Frames     | `internal/lidar/frame_builder.go`, `internal/lidar/visualiser/lidarview_adapter.go`                       |
| L3 Grid       | `internal/lidar/background.go`                                                                            |
| L4 Perception | `internal/lidar/clustering.go`, `internal/lidar/track_store.go` (`TrackObservation`)                      |
| L5 Tracks     | `internal/lidar/tracking.go`, `internal/lidar/visualiser/model.go` (`TrackSet`)                           |
| L6 Objects    | `internal/lidar/classification.go`, `docs/lidar/future/av-lidar-integration-plan.md`                      |

## AV compatibility note

This six-layer model keeps traffic monitoring simple while preserving a clean upgrade path: AV 28-class compatibility is treated as an **L6 Objects** concern, without forcing AV complexity into packet/frame/grid/tracking layers.
