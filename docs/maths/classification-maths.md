# Classification Maths

**Status:** Implementation-aligned math note
**Layer:** L6 Objects (`internal/lidar/l6objects`)
**Related:** [Tracking Maths](tracking-maths.md)

## 1. Purpose

Classification assigns a semantic label to each tracked object using
rule-based thresholds on spatial and kinematic features extracted from
the track's observation history.

**Source of truth:** `internal/lidar/l6objects/classification.go`
**Model version:** `rule-based-v1.2`

## 2. Object Classes

| Class        | ObjectClass const | Description                                                |
| ------------ | ----------------- | ---------------------------------------------------------- |
| Bird         | `"bird"`          | Small flying object or ground-level animal                 |
| Bus          | `"bus"`           | Bus, coach, or large passenger vehicle (length ≥ 7 m)      |
| Truck        | `"truck"`         | Pickup truck, box truck, or freight vehicle                |
| Car          | `"car"`           | Passenger car, SUV, or van                                 |
| Motorcyclist | `"motorcyclist"`  | Person riding a motorcycle                                 |
| Cyclist      | `"cyclist"`       | Person on a bicycle or e-scooter                           |
| Pedestrian   | `"pedestrian"`    | Person walking, running, or using a mobility aid           |
| Other        | `"dynamic"`       | Unclassifiable dynamic object or insufficient observations |

### 2.1 Proto Enum

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
| 8     | `OBJECT_CLASS_TRUCK`        | Classifier + user |
| 9     | `OBJECT_CLASS_MOTORCYCLIST` | Classifier + user |

## 3. Feature Vector

Features are extracted per-track from the observation history:

| Feature           | Symbol            | Unit | Source                                    |
| ----------------- | ----------------- | ---- | ----------------------------------------- |
| Average height    | $\bar{h}$         | m    | `BoundingBoxHeightAvg`                    |
| Average length    | $\bar{l}$         | m    | `BoundingBoxLengthAvg`                    |
| Average width     | $\bar{w}$         | m    | `BoundingBoxWidthAvg`                     |
| P95 height        | $h_{95}$          | m    | `HeightP95Max`                            |
| Average speed     | $\bar{v}$         | m/s  | `AvgSpeedMps`                             |
| Peak speed        | $v_{\text{peak}}$ | m/s  | `PeakSpeedMps`                            |
| P50 speed         | $v_{50}$          | m/s  | Median of speed history                   |
| P85 speed         | $v_{85}$          | m/s  | 85th percentile                           |
| P95 speed         | $v_{95}$          | m/s  | 95th percentile                           |
| Observation count | $n$               | —    | `ObservationCount`                        |
| Duration          | $\Delta t$        | s    | `(LastUnixNanos - FirstUnixNanos) / 10^9` |

## 4. Decision Thresholds

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

## 5. Classification Rules (Priority Order)

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
\bigl(\bar{v} > V_{\text{veh}} \;\lor\; v_{\text{peak}} > 1.5 \, V_{\text{veh}} \;\lor\; \bar{h} > H_{\text{veh}}\bigr)
$$

Confidence increases with extreme length (>10 m), width (>2.5 m),
height (>2.5 m), and observation count (>20).

### 5.3 Truck (large vehicle, smaller than bus)

$$
\bar{l} > L_{\text{truck}}
\;\land\;
\bar{w} > W_{\text{truck}}
\;\land\;
\bar{h} > H_{\text{truck}}
\;\land\;
\bigl(\bar{v} > V_{\text{veh}} \;\lor\; v_{\text{peak}} > 1.5 \, V_{\text{veh}}\bigr)
$$

Bus-sized objects are excluded (detected in §5.2). Confidence
increases with length (>6 m), width (>2.2 m), height (>2.5 m),
speed (>8 m/s), and observation count (>20).

### 5.4 Car (medium vehicle)

$$
\bigl(\bar{l} > L_{\text{veh}} \;\lor\; \bar{w} > W_{\text{veh}}\bigr)
\;\land\;
\bigl(\bar{v} > V_{\text{veh}} \;\lor\; v_{\text{peak}} > 1.5 \, V_{\text{veh}}\bigr)
$$

**OR** large + tall:

$$
\bigl(\bar{l} > L_{\text{veh}} \;\lor\; \bar{w} > W_{\text{veh}}\bigr)
\;\land\;
\bar{h} > H_{\text{veh}}
$$

Bus- and truck-sized objects are excluded (detected in §5.2–5.3).
Confidence increases with length (>4 m), width (>2 m), speed (>10 m/s),
peak speed (>15 m/s), and observation count (>20).

### 5.5 Motorcyclist

$$
V_{\text{moto,min}} \le \bar{v} \le V_{\text{moto,max}}
\;\land\;
\bar{w} \le W_{\text{moto}}
\;\land\;
L_{\text{moto,min}} \le \bar{l} \le L_{\text{moto,max}}
$$

Confidence increases in the typical motorcycle speed range
($8 \le \bar{v} \le 25$ m/s), for narrow profiles ($\bar{w} \le 0.9$),
for longer platforms ($\bar{l} \ge 2.0$), and when peak speed exceeds
12 m/s (distinguishing from cyclists).

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

## 6. Confidence Calibration

| Level  | Value | Meaning                        |
| ------ | ----- | ------------------------------ |
| High   | 0.85  | Strong match to class features |
| Medium | 0.70  | Baseline for matched rules     |
| Low    | 0.50  | Weak match or fallback         |

Confidence is always clamped to $[0, 1]$.

## 7. Minimum Observation Requirement

Classification is deferred until the track accumulates
$n \ge n_{\text{min}}$ observations (configurable via
`min_observations_for_classification` in tuning.json). Below
this threshold, the result is `"dynamic"` with very low confidence
($0.25$).

## 8. Unified Label Vocabulary

As of v1.2, the classifier output and user-label vocabularies are
aligned. Both use canonical full-word strings. The wire protocol uses
a proto3 enum (`ObjectClass`) for forward compatibility.

| Label            | Classifier output | User-assignable | Positive (real detection) |
| ---------------- | ----------------- | --------------- | ------------------------- |
| `"car"`          | ✓                 | ✓               | ✓                         |
| `"truck"`        | ✓                 | ✓               | ✓                         |
| `"bus"`          | ✓                 | ✓               | ✓                         |
| `"pedestrian"`   | ✓                 | ✓               | ✓                         |
| `"cyclist"`      | ✓                 | ✓               | ✓                         |
| `"motorcyclist"` | ✓                 | ✓               | ✓                         |
| `"bird"`         | ✓                 | ✓               | —                         |
| `"noise"`        | —                 | ✓               | —                         |
| `"dynamic"`      | ✓                 | ✓               | —                         |

**Canonical locations:**

- Proto enum definition: `proto/velocity_visualiser/v1/visualiser.proto`
- Go classifier constants: `internal/lidar/l6objects/classification.go`
- Go feature-based API: `ClassifyFeatures()` in `internal/lidar/l6objects/classification.go`
- Go user-label validation: `internal/api/lidar_labels.go`
- Go VRLOG replay bridge: `classifyOrConvert()` in `internal/lidar/visualiser/grpc_server.go`
- TypeScript types: `web/src/lib/types/lidar.ts`
- Swift labels: `ContentView.swift` → `LabelPanelView.classificationLabels`

## 9. VRLOG Replay Re-classification

VRLOG recordings made before classification was added contain tracks
with empty `ObjectClass` strings. When these recordings are replayed,
the gRPC server re-classifies tracks on-the-fly using
`classifyOrConvert()` in `grpc_server.go`.

The function builds a `ClassificationFeatures` struct from the Track's
available per-frame metrics (bounding box dimensions, speed, height,
observation count, duration) and delegates to
`TrackClassifier.ClassifyFeatures()`. This uses the same rule-based
classifier as the live pipeline, but operates on aggregate metrics
rather than requiring a full `TrackedObject` with observation history.

Tracks that already carry a non-empty `ObjectClass` (e.g. from newer
recordings or live data) are converted directly without re-classification.

## 10. Future Work

- Replace rule-based classifier with ML model (feature vector is
  designed to be export-compatible)
- Expose label taxonomy via API endpoint for frontend consumption
