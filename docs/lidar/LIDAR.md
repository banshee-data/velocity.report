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
| Refinement criteria     | [operations/retrospective-refinement-criteria.md](operations/retrospective-refinement-criteria.md) |
| Uncertainty criteria    | [operations/adaptive-uncertainty-criteria.md](operations/adaptive-uncertainty-criteria.md)         |
| Analysis worker         | [operations/analysis-worker.md](operations/analysis-worker.md)                                     |
| Track labelling         | [operations/track-labelling-ui-implementation.md](operations/track-labelling-ui-implementation.md) |
| Point annotation        | [operations/point-annotation-tool.md](operations/point-annotation-tool.md)                         |
| Label sets              | [operations/label-sets.md](operations/label-sets.md)                                               |
| Label set migration     | [operations/annotation-label-set-migration.md](operations/annotation-label-set-migration.md)       |
| Per-frame evaluation    | [operations/per-frame-evaluation.md](operations/per-frame-evaluation.md)                           |
| Headway report oracle   | [operations/headway-report-oracle.md](operations/headway-report-oracle.md)                         |
| Near-edge frame cost    | [operations/near-edge-frame-cost-2026-10.md](operations/near-edge-frame-cost-2026-10.md)           |
| macOS visualiser        | [../ui/visualiser/architecture.md](../ui/visualiser/architecture.md)                               |
| Backlog                 | [../BACKLOG.md](../BACKLOG.md)                                                                     |

The [October near-edge campaign report](operations/near-edge-campaign-2026-10.md) preserves the
completed experiment's per-case findings, methods, uncertainty limits, and follow-up decisions.
Raw replay outputs remain local. The [update cost profile](operations/near-edge-update-cost-profile-2026-10.md) answers
its first follow-up: the cost breach is the solid body's extent admission, which the shadow shares. The [D2 re-run](operations/d2-medoid-obb-current-solver-2026-10.md)
refreshes the medoid against OBB-centre comparison on the current solver. The
[span selection timing](operations/near-edge-span-selection-timing-2026-10.md) measures the
solid body's update cost after its percentiles are selected rather than sorted. The
[physical-reference pilot](operations/physical-reference-pilot-kirk0-2026-10.md) is the first
frozen score against reviewed body-centre, size and following-gap references, on kirk0. The
[solid-body alignment](operations/solid-body-physical-alignment-kirk0-2026-10.md) follows it:
why the solid body misses both trucks, and opt-in experiments that correct their heading and
length, guarded on the 23-site corpus. The
[zero-code L3 arms report](operations/l3-zero-code-arms-kirk-2026-10.md) measures what
holding the region overrides off, and narrower base windows, do to labelled recall, false
foreground and stationary tracks on kirk0 and kirk1. The
[geometry convergence plan](../plans/lidar-tracker-geometry-convergence-plan.md) sets out what
replaces those patches: a heading the tracker observes every frame, and a box that holds its
points. Its [end-on truck window spike](operations/end-on-truck-window-spike-2026-10.md)
counts the second capture that plan needs: 33 clean windows in the corpus, an archive 6.7
times its size, and a selection rule in place of a search. The
[containment corpus screen](operations/solid-body-containment-corpus-2026-10.md) scores the
plan's first observation model, the box held to its points, on kirk0 and the 23 sites. The
[rectangle heading screen](operations/solid-body-rectangle-heading-2026-10.md) scores the
second, the heading observed from each frame's rectangle fit. The
[held-out replication](operations/geometry-convergence-heldout-2026-10.md) replays the
candidate on each site's second segment: the heading holds, the lateral margin shrinks. The
[region override attribution](operations/l3-region-override-attribution-kirk-2026-10.md)
follows the zero-code L3 arms: the volatile tercile's window is the one that absorbs road users,
switching the overrides off moves along the recall trade-off rather than across it, and an
overrides-on retune with the height band floor at road height reaches the recall at the shipped
false positives. The [region map](operations/l3-region-tercile-mechanism-2026-10.md) checks why:
the volatile tercile holds the overrides-off flood, but lanes are there for the surfaces behind
them, not traffic during settling; neither of the two [levers](operations/l3-region-levers-kirk-2026-10.md)
it suggests beats the retune. The [dwell policy](operations/l3-dwell-policy-arrival-2026-10.md)
hands an object stationary for 20 s to the background, which removes the retune's long stationary
tracks over a 39-minute capture; on the labelled kirk captures it
[costs no moving road user](operations/l3-dwell-policy-labelled-cost-2026-10.md) and absorbs the
parked car the reviewed pack counts. The [pose step](operations/l3-scene-change-pose-step-2026-10.md)
shows the capture's late scene change is a knock to the sensor, then a second change and the
pickup, and a [corpus sweep](operations/l3-corpus-pose-sweep-2026-10.md) finds unrecorded knocks at
six other placements, four of them with one inside a scored window. The [W3 report](operations/w3-retention-fragmentation-kirk0-2026-10.md)
measures how much of each reviewed object reaches a cluster and into how many it splits, and the
[range-adaptive eps](operations/l4-range-adaptive-eps-2026-10.md) does not change it. The
[return fate](operations/w3-return-fate-kirk0-2026-10.md) says which stage loses each reviewed
return: L3 under the shipped grid, the input cap, DBSCAN noise and the height band under CMR.
Lifting the [input cap](operations/l4-dbscan-input-cap-2026-10.md) recovers the returns but joins
no fragments.

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
