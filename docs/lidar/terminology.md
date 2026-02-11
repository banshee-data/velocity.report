# LiDAR Terminology

Core terms used across the LiDAR tracking system.

| Term                   | Definition                                                                                                                                                   |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Point**              | A single 3D measurement from the LiDAR sensor (x, y, z, intensity, timestamp).                                                                               |
| **Cluster**            | A group of spatially-proximate foreground points identified by DBSCAN, representing a potential object.                                                      |
| **Track**              | A temporally-linked sequence of clusters representing a moving object across frames, maintained by the Kalman-filter tracker.                                |
| **Observation**        | A single cluster-to-track association at one point in time (one frame's measurement of a track).                                                             |
| **Scene**              | A named collection of reference ground-truth labels for a specific sensor environment (installation, angle, location). Used for evaluating tracking quality. |
| **Run** (Analysis Run) | A single processing pass over a data source (live or PCAP) with fixed parameters, producing tracks that can be compared against a scene's ground truth.      |
| **Sweep**              | A batch execution that varies parameter combinations, running one analysis per combination and collecting metrics for comparison.                            |
| **Auto-Tune**          | An iterative sweep that narrows parameter bounds across rounds, converging on optimal parameters via objective scoring.                                      |

## Label Taxonomy

Labels are applied by human reviewers to tracks within an analysis run. The same taxonomy is used across all platforms (Go backend, Svelte web frontend, macOS app).

### Detection Labels (`user_label`)

Classify whether the tracker correctly detected and tracked a real-world object.

| Label             | Description                                                                  |
| ----------------- | ---------------------------------------------------------------------------- |
| `good_vehicle`    | Correctly tracked vehicle (car, van, lorry, motorcycle, etc.)                |
| `good_pedestrian` | Correctly tracked pedestrian                                                 |
| `good_other`      | Correctly tracked non-vehicle, non-pedestrian object (e.g. wheelchair, pram) |
| `noise`           | False positive — no real object (sensor artefact, multipath reflection)      |
| `noise_flora`     | False positive caused by vegetation (tree, bush, hedge movement)             |
| `split`           | Single real object incorrectly split into multiple tracks                    |
| `merge`           | Multiple real objects incorrectly merged into a single track                 |
| `missed`          | Real object that was not tracked (false negative)                            |

### Quality Labels (`quality_label`)

Rate the measurement quality of a correctly-detected track.

| Label               | Description                                                              |
| ------------------- | ------------------------------------------------------------------------ |
| `perfect`           | Clean track throughout — accurate velocity, stable bounding box          |
| `good`              | Minor imperfections that do not affect usability                         |
| `truncated`         | Track starts late or ends early relative to the object's true trajectory |
| `noisy_velocity`    | Velocity estimate is unstable or inaccurate                              |
| `stopped_recovered` | Track was temporarily lost and re-acquired after the object stopped      |

### Canonical Sources

- **Go backend:** `internal/api/lidar_labels.go` — `validUserLabels`, `validQualityLabels`
- **Svelte frontend:** `web/src/lib/types/lidar.ts` — `DetectionLabel`, `QualityLabel`
- **macOS app:** `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift` — `LabelPanelView`

> **Note:** `object_class` (e.g. `pedestrian`, `car`, `bird`, `other`) is a _sensor-assigned_ classification from the tracker, not a human label. It is distinct from the `user_label` taxonomy above.
