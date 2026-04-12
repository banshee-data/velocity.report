# Classification maths

- **Status:** Implementation-aligned math note
- **Layers:** L6 Objects ([l6objects](../../internal/lidar/l6objects))
- **Related:** [Tracking Maths](tracking-maths.md)

## 1. Purpose

Classification assigns a semantic label to each tracked object using
rule-based thresholds on spatial and kinematic features extracted from
the track's observation history.

**Source of truth:** [l6objects/classification.go](../../internal/lidar/l6objects/classification.go)
**Model version:** `rule-based-v1.2`

## 2. Object classes

| Class      | ObjectClass const | Description                                                |
| ---------- | ----------------- | ---------------------------------------------------------- |
| Pedestrian | `"pedestrian"`    | Person walking, running, or using a mobility aid           |
| Cyclist    | `"cyclist"`       | Person on a bicycle, e-scooter, or motorcycle              |
| Bird       | `"bird"`          | Small flying object or ground-level animal                 |
| Bus        | `"bus"`           | Bus, coach, or large passenger vehicle (length ≥ 7 m)      |
| Car        | `"car"`           | Passenger car, SUV, van, or truck                          |
| Noise      | `"noise"`         | Spurious track (sensor noise, rain, dust, vegetation)      |
| Other      | `"dynamic"`       | Unclassifiable dynamic object or insufficient observations |

### 2.1 Proto enum

The wire protocol uses a proto3 enum (`ObjectClass`) for forward
compatibility and self-documentation:

| Value | Name                        | Scope             |
| ----- | --------------------------- | ----------------- |
| 0     | `OBJECT_CLASS_UNSPECIFIED`  | Default / unset   |
| 1     | `OBJECT_CLASS_NOISE`        | User-only         |
| 2     | `OBJECT_CLASS_DYNAMIC`      | Classifier + user |
| 3     | `OBJECT_CLASS_PEDESTRIAN`   | Classifier + user |
| 4     | `OBJECT_CLASS_CYCLIST`      | Classifier + user |
| 5     | `OBJECT_CLASS_BIRD`         | Classifier + user |
| 6     | `OBJECT_CLASS_BUS`          | Classifier + user |
| 7     | `OBJECT_CLASS_CAR`          | Classifier + user |
| 8     | `OBJECT_CLASS_TRUCK`        | Reserved (v0.6+)  |
| 9     | `OBJECT_CLASS_MOTORCYCLIST` | Reserved (v0.6+)  |

## 3. Feature vector

Features are extracted per-track from the observation history:

| Feature           | Symbol           | Unit | Source                                    |
| ----------------- | ---------------- | ---- | ----------------------------------------- |
| Average height    | $\bar{h}$        | m    | `BoundingBoxHeightAvg`                    |
| Average length    | $\bar{l}$        | m    | `BoundingBoxLengthAvg`                    |
| Average width     | $\bar{w}$        | m    | `BoundingBoxWidthAvg`                     |
| P95 height        | $h_{95}$         | m    | `HeightP95Max`                            |
| Average speed     | $\bar{v}$        | m/s  | `AvgSpeedMps`                             |
| Max speed         | $v_{\text{max}}$ | m/s  | `MaxSpeedMps`                             |
| P50 speed         | $v_{50}$         | m/s  | Median of speed history                   |
| P85 speed         | $v_{85}$         | m/s  | 85th percentile                           |
| P95 speed         | $v_{95}$         | m/s  | 95th percentile                           |
| Observation count | $n$              | -    | `ObservationCount`                        |
| Duration          | $\Delta t$       | s    | `(LastUnixNanos - FirstUnixNanos) / 10^9` |

## 4. Decision thresholds

| Threshold               | Symbol                | Value    | Purpose                          |
| ----------------------- | --------------------- | -------- | -------------------------------- |
| Bird height max         | $H_{\text{bird}}$     | 0.5 m    | Birds are small                  |
| Bird speed max          | $V_{\text{bird}}$     | 1.0 m/s  | Birds detected at low speeds     |
| Bus length min          | $L_{\text{bus}}$      | 7.0 m    | Buses are long                   |
| Bus width min           | $W_{\text{bus}}$      | 2.3 m    | Buses are wide                   |
| Truck length min        | $L_{\text{truck}}$    | 5.5 m    | Trucks are longer than cars      |
| Truck width min         | $W_{\text{truck}}$    | 2.0 m    | Trucks are wider than cars       |
| Truck height min        | $H_{\text{truck}}$    | 2.0 m    | Trucks are taller than cars      |
| Motorcyclist speed min  | $V_{\text{moto,min}}$ | 5.0 m/s  | Faster than cyclists             |
| Motorcyclist speed max  | $V_{\text{moto,max}}$ | 30.0 m/s | Upper bound (~108 km/h)          |
| Motorcyclist width max  | $W_{\text{moto}}$     | 1.2 m    | Narrow two-wheel profile         |
| Motorcyclist length min | $L_{\text{moto,min}}$ | 1.5 m    | Longer than a bicycle            |
| Motorcyclist length max | $L_{\text{moto,max}}$ | 3.0 m    | Shorter than a car               |
| Cyclist height min      | $H_{\text{cyc,min}}$  | 1.0 m    | Seated cyclist minimum           |
| Cyclist height max      | $H_{\text{cyc,max}}$  | 2.0 m    | Cyclist maximum                  |
| Cyclist speed min       | $V_{\text{cyc,min}}$  | 2.0 m/s  | Faster than walking              |
| Cyclist speed max       | $V_{\text{cyc,max}}$  | 10.0 m/s | Slower than motor vehicles       |
| Cyclist width max       | $W_{\text{cyc}}$      | 1.2 m    | Narrow profile                   |
| Cyclist length max      | $L_{\text{cyc}}$      | 2.5 m    | Bike length                      |
| Pedestrian height min   | $H_{\text{ped,min}}$  | 1.0 m    | Minimum human height             |
| Pedestrian height max   | $H_{\text{ped,max}}$  | 2.2 m    | Maximum human height             |
| Pedestrian speed max    | $V_{\text{ped}}$      | 3.0 m/s  | ~10.8 km/h upper bound           |
| Vehicle height min      | $H_{\text{veh}}$      | 1.2 m    | Minimum vehicle height           |
| Vehicle length min      | $L_{\text{veh}}$      | 3.0 m    | Minimum vehicle length           |
| Vehicle width min       | $W_{\text{veh}}$      | 1.5 m    | Minimum vehicle width            |
| Vehicle speed min       | $V_{\text{veh}}$      | 5.0 m/s  | Minimum vehicle speed            |
| Stationary max          | $V_{\text{stat}}$     | 0.5 m/s  | Below this, object is stationary |

## 5. Classification rules (priority order)

Classification follows a strict priority cascade. The first matching
rule wins.

### 5.1 Bird

$$
\bar{h} < H_{\text{bird}} \;\land\; \bar{v} < V_{\text{bird}} \;\land\; \bar{l} < 1\,\text{m} \;\land\; \bar{w} < 1\,\text{m}
$$

Confidence is boosted for very small objects ($\bar{h} < 0.3$) and
penalised for near-stationary objects ($\bar{v} < 0.1$) which may be
noise.

### 5.2 Bus (very large vehicle)

$$
\bar{l} > L_{\text{bus}}
\;\land\;
\bar{w} > W_{\text{bus}}
\;\land\;
\bigl(\bar{v} > V_{\text{veh}} \;\lor\; v_{\text{max}} > 1.5 \, V_{\text{veh}} \;\lor\; \bar{h} > H_{\text{veh}}\bigr)
$$

Confidence increases with extreme length (>10 m), width (>2.5 m),
height (>2.5 m), and observation count (>20).

### 5.3 Truck: _reserved (v0.6+)_

> **Disabled in v0.5.0.** Truck-like objects fall through to §5.4 (Car).
> Thresholds are preserved for future reactivation.

$$
\bar{l} > L_{\text{truck}}
\;\land\;
\bar{w} > W_{\text{truck}}
\;\land\;
\bar{h} > H_{\text{truck}}
\;\land\;
\bigl(\bar{v} > V_{\text{veh}} \;\lor\; v_{\text{max}} > 1.5 \, V_{\text{veh}}\bigr)
$$

### 5.4 Car (medium vehicle)

$$
\bigl(\bar{l} > L_{\text{veh}} \;\lor\; \bar{w} > W_{\text{veh}}\bigr)
\;\land\;
\bigl(\bar{v} > V_{\text{veh}} \;\lor\; v_{\text{max}} > 1.5 \, V_{\text{veh}}\bigr)
$$

**OR** large + tall:

$$
\bigl(\bar{l} > L_{\text{veh}} \;\lor\; \bar{w} > W_{\text{veh}}\bigr)
\;\land\;
\bar{h} > H_{\text{veh}}
$$

Bus- and truck-sized objects are excluded (detected in §5.2–5.3).
Confidence increases with length (>4 m), width (>2 m), speed (>10 m/s),
max speed (>15 m/s), and observation count (>20).

### 5.5 Motorcyclist: _reserved (v0.6+)_

> **Disabled in v0.5.0.** Motorcycle-speed objects that don't match
> cyclist thresholds fall through to §5.8 (Dynamic).
> Thresholds are preserved for future reactivation.

$$
V_{\text{moto,min}} \le \bar{v} \le V_{\text{moto,max}}
\;\land\;
\bar{w} \le W_{\text{moto}}
\;\land\;
L_{\text{moto,min}} \le \bar{l} \le L_{\text{moto,max}}
$$

### 5.6 Cyclist

$$
H_{\text{cyc,min}} \le \bar{h} \le H_{\text{cyc,max}}
\;\land\;
V_{\text{cyc,min}} \le \bar{v} \le V_{\text{cyc,max}}
\;\land\;
\bar{w} < W_{\text{cyc}}
\;\land\;
\bar{l} < L_{\text{cyc}}
$$

Confidence increases in the typical cycling speed range
($3 \le \bar{v} \le 8$ m/s), for narrow profiles ($\bar{w} < 0.8$),
and typical seated height ($1.2 \le \bar{h} \le 1.8$).

### 5.7 Pedestrian

$$
H_{\text{ped,min}} \le \bar{h} \le H_{\text{ped,max}}
\;\land\;
\bar{v} \le V_{\text{ped}}
\;\land\;
\bar{l} < L_{\text{veh}}
\;\land\;
\bar{w} < W_{\text{veh}}
$$

Confidence increases when height is in the typical adult range
($1.5 \le \bar{h} \le 1.9$), speed is in the walking range
($0.5 \le \bar{v} \le 2.0$), and the bounding box is
narrow ($\bar{w} < 0.8$).

### 5.8 Dynamic (fallback)

If no rule matches, the object is classified as `"dynamic"` with low
confidence (0.50).

## 6. Confidence calibration

| Level  | Value | Meaning                        |
| ------ | ----- | ------------------------------ |
| High   | 0.85  | Strong match to class features |
| Medium | 0.70  | Baseline for matched rules     |
| Low    | 0.50  | Weak match or fallback         |

Confidence is always clamped to $[0, 1]$.

## 7. Minimum observation requirement

Classification is deferred until the track accumulates
$n \ge n_{\text{min}}$ observations (configurable via
`min_observations_for_classification` in [tuning.example.json](../../config/tuning.example.json)). Below
this threshold, the result is `"dynamic"` with very low confidence
($0.25$).

## 8. Label vocabulary

The canonical class list and proto enum are in §2. All seven active
labels are both classifier outputs and user-assignable; `noise` is
user-assignable only. Reserved labels will be activated in future
releases. See §11 for the full AV taxonomy alignment.

**Canonical locations:**

- Proto enum: [proto/velocity_visualiser/v1/visualiser.proto](../../proto/velocity_visualiser/v1/visualiser.proto)
- Go constants: [l6objects/classification.go](../../internal/lidar/l6objects/classification.go)
- Go classifier: `ClassifyFeatures()` in [classification.go](../../internal/lidar/l6objects/classification.go)
- Go user-label validation: [internal/api/lidar_labels.go](../../internal/api/lidar_labels.go)
- Go VRLOG replay bridge: `classifyOrConvert()` in [l9endpoints/grpc_frames.go](../../internal/lidar/l9endpoints/grpc_frames.go)
- TypeScript: [web/src/lib/types/lidar.ts](../../web/src/lib/types/lidar.ts)
- Swift: [ContentView.swift](../../tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift) → `LabelPanelView.classificationLabels`

## 9. VRLOG replay re-classification

VRLOG recordings made before classification was added contain tracks
with empty `ObjectClass` strings. When these recordings are replayed,
the gRPC server re-classifies tracks on-the-fly using
`classifyOrConvert()` in [grpc_frames.go](../../internal/lidar/l9endpoints/grpc_frames.go).

The function builds a `ClassificationFeatures` struct from the Track's
available per-frame metrics (bounding box dimensions, speed, height,
observation count, duration) and delegates to
`TrackClassifier.ClassifyFeatures()`. This uses the same rule-based
classifier as the live pipeline, but operates on aggregate metrics
rather than requiring a full `TrackedObject` with observation history.

Tracks that already carry a non-empty `ObjectClass` (e.g. from newer
recordings or live data) are converted directly without re-classification.

## 10. Future work

- Activate reserved labels: `truck`, `motorcyclist`, `pole`,
  `sign`, and `static` (future; see §11.1 master table)
- Replace rule-based classifier with ML model (feature vector is
  designed to be export-compatible)
- Expose label taxonomy via API endpoint for frontend consumption

## 11. AV dataset taxonomy alignment

velocity.report's label vocabulary (§2) is derived from the Waymo Open
Dataset [`CameraSegmentation.Type`][waymo-proto] enum (28 named
classes; Mei et al. 2022). Waymo naming is adopted directly where a
v.r class exists. The one deliberate broadening: v.r's `cyclist`
covers bicycle riders **and** e-scooter riders (UK road-safety
framing), whereas Waymo's `TYPE_CYCLIST` covers bicycle riders only.

Static scene elements (types 19–26) are removed by L3 background
subtraction before clustering. They never receive an L6 label but are
included in the table below for completeness. Their geometric
representation as scene features (Ground, Structure, Volume) is
specified in
[vector-scene-map.md §2.4](../../docs/lidar/architecture/vector-scene-map.md#24-waymo-scene-type-alignment).

[waymo-proto]: https://github.com/waymo-research/waymo-open-dataset/blob/99a4cb3ff07e2fe06c2ce73da001f850f628e45a/src/waymo_open_dataset/protos/camera_segmentation.proto#L42

### 11.1 Waymo → v.r master mapping

| #   | Waymo `CameraSegmentation.Type` | v.r class      | Status         | SemanticKITTI     |
| --- | ------------------------------- | -------------- | -------------- | ----------------- |
| 1   | `TYPE_EGO_VEHICLE`              | —              | n/a            | —                 |
| 2   | `TYPE_CAR`                      | `car`          | ✅ active      | car               |
| 3   | `TYPE_TRUCK`                    | `truck`        | ⚠️ reserved    | truck             |
| 4   | `TYPE_BUS`                      | `bus`          | ✅ active      | —                 |
| 5   | `TYPE_OTHER_LARGE_VEHICLE`      | —              | ❌ not covered | other-vehicle     |
| 6   | `TYPE_BICYCLE`                  | —              | ❌ not covered | bicycle           |
| 7   | `TYPE_MOTORCYCLE`               | —              | ❌ not covered | motorcycle        |
| 8   | `TYPE_TRAILER`                  | —              | ❌ not covered | —                 |
| 9   | `TYPE_PEDESTRIAN`               | `pedestrian`   | ✅ active      | person            |
| 10  | `TYPE_CYCLIST`                  | `cyclist`      | ✅ active      | bicyclist         |
| 11  | `TYPE_MOTORCYCLIST`             | `motorcyclist` | ⚠️ reserved    | motorcyclist      |
| 12  | `TYPE_BIRD`                     | `bird`         | ✅ active      | —                 |
| 13  | `TYPE_GROUND_ANIMAL`            | —              | ❌ not covered | —                 |
| 14  | `TYPE_CONSTRUCTION_CONE_POLE`   | —              | ❌ not covered | —                 |
| 15  | `TYPE_POLE`                     | `pole`         | ⚠️ reserved    | pole              |
| 16  | `TYPE_PEDESTRIAN_OBJECT`        | —              | ❌ not covered | —                 |
| 17  | `TYPE_SIGN`                     | `sign`         | ⚠️ reserved    | traffic sign      |
| 18  | `TYPE_TRAFFIC_LIGHT`            | —              | ❌ not covered | —                 |
| 19  | `TYPE_BUILDING`                 | —              | BG subtraction | building          |
| 20  | `TYPE_ROAD`                     | —              | BG subtraction | road              |
| 21  | `TYPE_LANE_MARKER`              | —              | BG subtraction | —                 |
| 22  | `TYPE_ROAD_MARKER`              | —              | BG subtraction | —                 |
| 23  | `TYPE_SIDEWALK`                 | —              | BG subtraction | sidewalk          |
| 24  | `TYPE_VEGETATION`               | —              | BG subtraction | vegetation, trunk |
| 25  | `TYPE_SKY`                      | —              | BG subtraction | —                 |
| 26  | `TYPE_GROUND`                   | —              | BG subtraction | parking, terrain  |
| 27  | `TYPE_DYNAMIC`                  | `dynamic`      | ✅ active      | —                 |
| 28  | `TYPE_STATIC`                   | `static`       | ⚠️ reserved    | other-object      |

`noise` is a v.r-only class (user-assignable label for sensor
artefacts). It has no Waymo equivalent; the nearest analogues are
nuScenes `noise → void` and SemanticKITTI `outlier`.

**Coverage:** 6 active, 5 reserved (`truck`, `motorcyclist`, `pole`,
`sign`, `static`), 8 uncovered foreground, 8 scene/BG, 1 n/a
(ego_vehicle). v.r maps **11 of 28** Waymo classes (6 active +
5 reserved). The 8 uncovered foreground classes are candidates for
Phase 3 of the AV integration plan.

Three SemanticKITTI classes have no direct Waymo equivalent:
`other-structure` and `fence` fold into `TYPE_BUILDING`; `outlier`
maps to v.r's `noise`.

### 11.2 nuScenes mapping

nuScenes (Caesar et al. 2020; Fong et al. 2021) uses 32 hierarchical
categories that merge into 16 evaluation classes. The many-to-one
relationship does not map cleanly to a single Waymo/v.r row, so a
separate table is warranted.

| nuScenes category                                                             | Eval class (idx)         | v.r class            |
| ----------------------------------------------------------------------------- | ------------------------ | -------------------- |
| vehicle.car                                                                   | car (4)                  | `car`                |
| vehicle.truck                                                                 | truck (10)               | `truck` _(reserved)_ |
| vehicle.bus.bendy, vehicle.bus.rigid                                          | bus (3)                  | `bus`                |
| vehicle.construction                                                          | construction_vehicle (5) | —                    |
| vehicle.trailer                                                               | trailer (9)              | —                    |
| vehicle.bicycle                                                               | bicycle (2)              | —                    |
| vehicle.motorcycle                                                            | motorcycle (6)           | —                    |
| vehicle.emergency.ambulance, vehicle.emergency.police, vehicle.ego            | void (0)                 | —                    |
| human.pedestrian.{adult, child, construction_worker, police_officer}          | pedestrian (7)           | `pedestrian`         |
| human.pedestrian.{personal_mobility, stroller, wheelchair}                    | void (0)                 | —                    |
| animal                                                                        | void (0)                 | —                    |
| movable_object.trafficcone                                                    | traffic_cone (8)         | —                    |
| movable_object.barrier                                                        | barrier (1)              | —                    |
| movable_object.{pushable_pullable, debris}, static_object.bicycle_rack, noise | void (0)                 | `noise` (noise only) |
| flat.driveable_surface, flat.sidewalk, flat.terrain, flat.other               | (11–14)                  | BG subtraction       |
| static.manmade, static.vegetation, static.other                               | (15–16, void)            | BG subtraction       |

### 11.3 The "28-class" source

The 28 named semantic classes are defined in the Waymo Open Dataset
[`CameraSegmentation.Type`][waymo-proto] enum (Mei et al. 2022). The
LiDAR-specific `Segmentation.Type` in
[`segmentation.proto`](https://github.com/waymo-research/waymo-open-dataset/blob/master/src/waymo_open_dataset/protos/segmentation.proto)
defines a 22-type subset (it omits ego_vehicle, trailer,
pedestrian_object, sky, road_marker, and lane_marker). Earlier
velocity.report documentation attributed "28 classes" to SemanticKITTI
(Behley 2019), which actually defines 19 evaluation classes (28 raw
label IDs including `moving-*` variants). The attribution has been
corrected in `references.bib`.

## 12. References

| Reference            | BibTeX key                 | Relevance                                                                                                         |
| -------------------- | -------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| Geiger et al. (2012) | `Geiger2012`               | KITTI 3D object detection benchmark; class definitions used in threshold calibration                              |
| Behley et al. (2019) | `Behley2019`               | SemanticKITTI: 19 eval-class point-level semantic segmentation for outdoor LiDAR                                  |
| Caesar et al. (2020) | `Caesar2020`               | nuScenes: 32-class AV taxonomy; candidate for future learned classifier in L6                                     |
| Fong et al. (2021)   | `Fong2021PanopticNuScenes` | Panoptic nuScenes: 32 raw → 16 eval LiDAR panoptic segmentation and tracking                                      |
| Mei et al. (2022)    | `Mei2022Waymo`             | Waymo panoptic: 28 classes in `CameraSegmentation.Type` (north star); 22-type LiDAR subset in `Segmentation.Type` |
| Lang et al. (2019)   | `Lang2019`                 | PointPillars: learned detector occupying the same architectural slot as our rule-based L6                         |

Full BibTeX entries: [data/maths/references.bib](references.bib)
