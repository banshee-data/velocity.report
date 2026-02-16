# LiDAR Data Layer Model (OSI-Style)

## Purpose

Define an OSI-style multi-layer model for LiDAR data, formats, and structures in velocity.report, while remaining compatible with future AV dataset integration (`docs/lidar/future/av-lidar-integration-plan.md`).

Required artefacts in scope:

- Network packets from the LiDAR sensor (lowest level)
- LidarView exports
- Cartesian grid / Cartesian point representations
- VRLOG recordings
- Clusters
- Observations
- Tracks

---

## Rubric A: Representation stack (recommended)

This rubric groups by **what the data is** at each abstraction level.

| Layer        | Name       | Canonical forms in this repo                                                            | Why it exists                                            |
| ------------ | ---------- | --------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| A1 (lowest)  | Transport  | Raw Hesai UDP payloads, PCAP packets                                                    | Capture and replay exact sensor wire format              |
| A2           | Decoding   | Parsed `PointPolar` streams with timing/calibration                                     | Convert bytes into physically meaningful returns         |
| A3           | Geometry   | Cartesian `Point`/`PointCloudFrame`, polar `BackgroundGrid` metadata, ASC export points | Represent spatial geometry for processing and tooling    |
| A4           | Logging    | `.vrlog` chunks + index + header                                                        | Deterministic replay of higher-level frame bundles       |
| A5           | Primitives | `WorldCluster`, `TrackObservation`                                                      | Per-frame object primitives (spatial + temporal samples) |
| A6           | Tracking   | `TrackedObject`/visualiser `TrackSet`                                                   | Multi-frame identity, motion state, lifecycle            |
| A7 (highest) | Semantics  | Local classes (`car`, `pedestrian`, `bird`, `other`) and AV 28-class target taxonomy    | Human/ML semantics and future dataset interoperability   |

### Where required artefacts fit (Rubric A)

- **Network packets:** A1
- **LidarView exports:** A3 (geometry export path for visual tools)
- **Cartesian grid/representation:** A3
- **VRLOG:** A4
- **Clusters:** A5
- **Observations:** A5
- **Tracks:** A6

---

## Rubric B: Pipeline lifecycle stack

This rubric groups by **when** data appears in the processing lifecycle.

| Layer | Name                            | Included artefacts                                             |
| ----- | ------------------------------- | -------------------------------------------------------------- |
| B1    | Ingest                          | Sensor UDP packets, PCAP reads                                 |
| B2    | Decode + calibration            | Parsed packet blocks/tails to `PointPolar`                     |
| B3    | Frame assembly + geometry       | Frame builder, Cartesian conversion, ASC/LidarView export feed |
| B4    | Background/foreground modelling | Polar grid and foreground extraction                           |
| B5    | Perception                      | Clustering, observations, track association                    |
| B6    | Persistence + replay            | SQLite track/cluster/observation rows, VRLOG runs              |
| B7    | Semantics + evaluation          | Object classes, labels, AV taxonomy alignment                  |

---

## Rubric C: Storage and query contract stack

This rubric groups by **how data is persisted and queried**.

| Layer | Name                   | Primary contract                                                         |
| ----- | ---------------------- | ------------------------------------------------------------------------ |
| C1    | Wire contract          | UDP packet bytes and packet sequencing                                   |
| C2    | Compute contract       | In-memory structs used in hot path (`PointPolar`, `Point`, frame bundle) |
| C3    | Durable event contract | VRLOG chunk/index/header and analysis-run references                     |
| C4    | Relational contract    | SQLite tables for clusters, tracks, observations                         |
| C5    | Interop contract       | LidarView-compatible forwarding, ASC exports, AV taxonomy mapping        |

---

## Rubric comparison matrix (pros/cons)

| Rubric                    | Pros                                                                                                                                                             | Cons                                                                                        | Best use                                      |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- | --------------------------------------------- |
| A: Representation stack   | Clear OSI-style abstraction boundaries; easiest to explain to mixed engineering/product audiences; directly maps required artefacts (packets → tracks/semantics) | Slightly less explicit about runtime order                                                  | Architecture, documentation, roadmap planning |
| B: Pipeline lifecycle     | Closest to runtime flow and debugging sequence; useful for incident triage                                                                                       | Blurs durable format concerns (e.g., VRLOG vs DB); less stable as pipeline internals evolve | Operations, troubleshooting, runbooks         |
| C: Storage/query contract | Strong for API/DB/export ownership; clarifies compatibility surfaces                                                                                             | Weak at describing algorithmic progression; less intuitive for non-storage discussions      | Data governance, schema/API change reviews    |

## Opinionated recommendation

**Use Rubric A as the primary model.**

Why:

1. It is the closest analogue to the OSI model (layered by representation/abstraction).
2. It cleanly places every required artefact in one layer hierarchy without mixing concerns.
3. It remains stable even if implementation details of the pipeline change.

Use Rubric B as a secondary “runtime view” and Rubric C as a “data contract view”.

---

## Alignment with current repository implementation

| Artefact                                  | Layer(s)        | Current implementation anchors                                                                                                                     |
| ----------------------------------------- | --------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Sensor packets                            | A1 / B1 / C1    | `internal/lidar/network/listener.go`, `internal/lidar/network/pcap.go`, `internal/lidar/parse/extract.go`                                          |
| Packet decode (`PointPolar`)              | A2 / B2 / C2    | `internal/lidar/parse/extract.go` (`Pandar40PParser`, packet tail/block parsing)                                                                   |
| Cartesian point representation and export | A3 / B3 / C2+C5 | `internal/lidar/frame_builder.go` (`SphericalToCartesian`, `exportFrameToASC`)                                                                     |
| LidarView-compatible forwarding/export    | A3 / B3 / C5    | `internal/lidar/network/foreground_forwarder.go`, `internal/lidar/visualiser/lidarview_adapter.go`                                                 |
| Cartesian/polar background grid context   | A3 / B4 / C2    | `internal/lidar/background.go` (`BackgroundGrid`, ring/azimuth grid metadata)                                                                      |
| VRLOG recording and replay                | A4 / B6 / C3    | `internal/lidar/visualiser/recorder/recorder.go`, `internal/lidar/monitor/webserver.go` (`/api/lidar/vrlog/*`)                                     |
| Clusters                                  | A5 / B5 / C4    | `internal/lidar/clustering.go`, `internal/lidar/track_store.go` (`InsertCluster`)                                                                  |
| Observations                              | A5 / B5 / C4    | `internal/lidar/track_store.go` (`TrackObservation`, observation queries), `internal/lidar/monitor/track_api.go`                                   |
| Tracks                                    | A6 / B5 / C4    | `internal/lidar/tracking.go` (`TrackedObject`), `internal/lidar/visualiser/model.go` (`TrackSet`)                                                  |
| Semantic classes and AV compatibility     | A7 / B7 / C5    | Current local classes in `internal/lidar/classification.go`; AV 28-class target and categories in `docs/lidar/future/av-lidar-integration-plan.md` |

---

## AV dataset compatibility notes (from integration plan)

To keep current traffic monitoring simple while enabling AV interoperability later:

- Keep A1-A6 minimal and traffic-focused (current approach).
- Treat AV 28-class taxonomy as an **A7 semantic extension**, not a prerequisite for packet/geometry/tracking layers.
- Maintain a mapping path from current local classes (`car`, `pedestrian`, `bird`, `other`) to AV high-level categories (`Vehicle`, `Pedestrian`, `Cyclist`, `Animal`, `Static`, etc.) defined in the AV integration plan.

This preserves current operational simplicity and creates a clear upgrade path to AV dataset ingestion.
