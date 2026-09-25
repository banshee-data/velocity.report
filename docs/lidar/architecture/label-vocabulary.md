# Label vocabulary

Active plan: [label-vocabulary-consolidation-plan.md](../../plans/label-vocabulary-consolidation-plan.md)

**Status:** Phases 1–3.2 complete; phases 3.5–6 planned.

Canonical vocabulary of track classification labels used across the proto wire format, Go runtime, Swift visualiser, and Svelte frontend.

## Canonical vocabulary (proto3 enum authoritative)

| Value | Name         | User-Assignable | Notes                        |
| ----- | ------------ | --------------- | ---------------------------- |
| 0     | UNSPECIFIED  | -               | Default/unknown              |
| 1     | NOISE        | ✅              | Environmental noise          |
| 2     | DYNAMIC      | ✅              | Classifier fallback          |
| 3     | PEDESTRIAN   | ✅              | Foot traffic                 |
| 4     | CYCLIST      | ✅              | Pedal cyclists + motorcycles |
| 5     | BIRD         | ✅              | Airborne fauna               |
| 6     | BUS          | ✅              | Public transit               |
| 7     | CAR          | ✅              | Cars, vans, trucks           |
| 8     | TRUCK        | Reserved        | Proto value stable           |
| 9     | MOTORCYCLIST | Reserved        | Proto value stable           |

The system ships **7 user-assignable classes**. Truck and motorcyclist are
disabled in the classifier, hidden in UIs, and rejected by the label API.

This vocabulary is level 0 of the proposed
[vehicle taxonomy](vehicle-taxonomy.md), which extends beneath `CAR` with
range-dependent classes derived from what the sensor can resolve. That work
proposes no change to this enum: deeper classes live in a separate,
edition-versioned field.

## Wire protocol

- `visualiser.proto`: `ObjectClass` enum (field 26 on Track)
- Go → proto: `objectClassFromString()` in `grpc_server.go`
- Proto → Swift: `objectClassLabel()` in `VisualiserClient.swift`
- Internal domain model uses string labels; proto enums at boundaries only.

## Migration 000029

Converts legacy rows: `ped` → `pedestrian`, `other` → `dynamic`,
`impossible` → `noise`. Idempotent.

## VRLOG replay re-classification (phase 3.1)

Older recordings store empty `ObjectClass`. The gRPC server
`classifyOrConvert()` bridge re-classifies on-the-fly using
`ClassifyFeatures()`: a refactored classifier that accepts pre-built
features without a full `TrackedObject`.

## Keyboard shortcuts

Renumbered 1–7 (migration 029): car, bus, pedestrian, cyclist, bird, dynamic,
noise.

## Remaining work

### Phase 3.5: display vs selectable split (#381)

Split into `DisplayLabel` (9 classes: rendering, colour, inspector) and
`SelectableLabel` (7 classes: labelling UI, shortcuts, API validation).
Truck/motorcyclist visible when present in data but not user-selectable.

### Phase 4: taxonomy API

`GET /api/v1/lidar/taxonomy` returns canonical label list with metadata
(name, description, positive/negative, shortcut). Eliminates hardcoded
lists in frontends.

### Phase 5: frontend deduplication

Replace hardcoded label arrays in Go, TypeScript, Swift, and Svelte with
runtime imports from the taxonomy API.

### Phase 6: public API field alignment

Ensure REST API track responses use canonical string labels consistent
with the taxonomy API.

## Future reactivation path

When sufficient labelled data exists: uncomment truck/motorcyclist cascade
rules in `classification.go`, add labels back to `validUserLabels`, restore
UI entries. No proto or database migration needed: enum values already
allocated.

## Behaviour analytics vocabularies

Closed vocabularies for behaviour results, per
[lidar-behaviour-analytics-plan](../../plans/lidar-behaviour-analytics-plan.md) Sections 7 to 10.
They are defined in
[internal/lidar/l8behaviour/vocabulary.go](../../../internal/lidar/l8behaviour/vocabulary.go),
whose tests fail when a token there is missing here. Tokens are the wire format; numeric values are
not. `unspecified` is never a token: an unset field refuses to serialise rather than defaulting to
a plausible value. Metric visibility tokens live in the
[metrics registry](../../platform/architecture/metrics-registry.md).

### Suppression reasons

A suppressed metric carries one of these and no value, never a zero. Rows are in reporting
precedence: when several apply, the earliest is stored and the rest are retained for review. Stage
is last, so a run over non-final estimates still shows the physical reasons beneath it.

| Reason                            | Meaning                                                                                        | Source           |
| --------------------------------- | ---------------------------------------------------------------------------------------------- | ---------------- |
| `class_not_supported`             | The metric is not defined for this motion class                                                | 7.2              |
| `metric_not_observable`           | Structurally unobservable at this sample rate, for example jerk on a short passage             | 7.2              |
| `interaction_type_uncertain`      | Interaction classification confidence is below the metric's bound                              | 7.2, 7.5         |
| `model_degraded`                  | The estimator reported `model_invalid` or `temporarily_degraded` for a contributing track      | 7.2              |
| `insufficient_observation`        | Too few observed frames or too short a passage, or the pose is not yet believed                | 7.2              |
| `not_observed`                    | A contributing track, or a surface the metric needs, was not directly observed at this instant | 9.1, 9.2         |
| `no_common_path`                  | The pair does not share the path, lateral corridor or direction the metric assumes             | 7.2              |
| `ambiguous_leader`                | More than one credible leader, or leader/follower order is not stable                          | 8.3              |
| `road_geometry_unavailable`       | No road-surface model for the traversed region                                                 | 7.2              |
| `lane_geometry_unavailable`       | No lane centreline or edges                                                                    | 7.2              |
| `planar_fallback_insufficient`    | Computed under a planar assumption on a graded site, where the grade error dominates           | 7.2              |
| `orientation_unresolved`          | The body's front/rear direction is unresolved, so no physical endpoint can be named            | 8.3              |
| `extent_not_converged`            | A required dimension belief is absent, short of its admissibility count, or a class prior      | 7.2, 9.1         |
| `trajectory_uncertainty_too_high` | Propagated uncertainty exceeds the metric's usable bound                                       | 7.2              |
| `non_positive_gap`                | The endpoint gap is zero or negative; requires overlap/geometry review, not a collision claim  | 8.3              |
| `below_speed_floor`               | Follower speed below the calibrated floor: net time gap is undefined at rest, not infinite     | 8.3, 9.1         |
| `estimate_not_final`              | Derived from an online or fixed-lag estimate; production reads final estimates only            | 2, 2.1 (G-SMO-1) |

### Observation support

Every sampled instant carries one support state (Section 7.3). Only observed time enters an
exposure or opportunity denominator. An interaction records the worst support of either party over
its interval (Section 10.2); worst is ranked by evidence, not by row order: less direct evidence is
worse, and among absences an unexplained one is worse than an explained one.

| State               | Meaning                                                     | Counts toward exposure           | Worst-support rank |
| ------------------- | ----------------------------------------------------------- | -------------------------------- | ------------------ |
| `observed`          | A detection was associated at this instant                  | Yes                              | 1 (best)           |
| `coasted`           | The estimator propagated without a measurement              | No                               | 6                  |
| `occluded_inferred` | Missing, and another object's geometry explains the absence | No; recorded as expected-missing | 4                  |
| `missed_unknown`    | Missing with no explanation, including gaps in the record   | No; a detector defect signal     | 7 (worst)          |
| `cluster_merged`    | Present but merged with another object                      | No                               | 3                  |
| `cluster_split`     | Present but fragmented across clusters                      | No                               | 2                  |
| `out_of_fov`        | Geometrically outside the sensor's coverage                 | No; not a failure                | 5                  |

### Estimate stage and estimation state

Estimate stage tokens match the persisted `lidar_track_estimates.stage` column: `online`
(association and the live view), `fixed_lag` (persisted comparison and provisional inspection) and
`final` (production behaviour and reports). Only `final` may reach a production surface. An
in-memory tracker estimate maps to `online`, or `fixed_lag` when smoothed; `final` is never
inferred.

Estimation state tokens are the l5tracks lifecycle's: `initialising`, `geometry_converging`,
`established`, `temporarily_degraded` and `model_invalid`. Production emission requires
`established`.

### Endpoint source

Each projected body endpoint records its evidence (Section 8.3). The source is derived from face
visibility and extent provenance, never declared.

| Source                | Meaning                                                                              |
| --------------------- | ------------------------------------------------------------------------------------ |
| `directly_observed`   | Returns from the face carrying the endpoint were associated at this instant          |
| `temporally_inferred` | The face was not seen now; extent evidence about this object from other frames       |
| `prior_dominated`     | The face was not seen and the extent is a class prior, not evidence about the object |

### Path condition

The detail behind a `no_common_path` suppression from the local following path (Section 8.3). The
first five say why a follower's path was refused, or why another track is not on it; the last two
say why one follower instant could not be ordered along a path that was built. Rows are in
reporting precedence.

| Condition              | Meaning                                                                                |
| ---------------------- | -------------------------------------------------------------------------------------- |
| `weak_support`         | Too little observed, moving evidence to fit a path or to place a track on one          |
| `direction_reversal`   | Motion both ways along the corridor: a member that reverses, or opposing traffic       |
| `crossing`             | A body crosses the corridor at an angle no following relation allows                   |
| `fork_or_merge`        | Tracks share part of the corridor and diverge elsewhere, so the path branches          |
| `lateral_incompatible` | Tracks that overlap along the path are laterally apart: more than one path             |
| `outside_extent`       | The follower lies outside its path's supported extent at this instant                  |
| `unestablished_body`   | The nearest body ahead in the corridor is not on the path, for no more specific reason |

### Candidate disposition

What leader choice decided about each other body present at a follower instant, so a chosen
leader or a suppression can be explained from the record.

| Disposition        | Meaning                                                                       |
| ------------------ | ----------------------------------------------------------------------------- |
| `leader`           | The nearest credible leader, chosen                                           |
| `competing`        | Ahead in the corridor and not separable from the nearest: `ambiguous_leader`  |
| `unestablished`    | The nearest body ahead in the corridor, but not on the path: `no_common_path` |
| `blocked`          | Ahead in the corridor, separably beyond the nearest                           |
| `beyond_range`     | Ahead in the corridor, beyond the search range                                |
| `behind`           | In the corridor, not ahead of the follower                                    |
| `outside_corridor` | Laterally outside the corridor, measured from the path or from the follower   |
| `off_path`         | Does not project onto the path's supported extent                             |

### Interaction type, exposure kind and observation basis

The persisted interaction records (Section 10.3) carry three more vocabularies. Only tokens a
method produces are registered; the plan's other interaction types (`crossing`, `merging`,
`overtaking`, `opposing`) and exposure kinds (`free_flow`, `yielding_opportunity`, `overtaking`)
are reserved until one does, and can be added without migrating stored rows. An interaction's
primary and secondary tracks are geometric roles fixed by its type, never fault.

| Vocabulary        | Token             | Meaning                                                                         |
| ----------------- | ----------------- | ------------------------------------------------------------------------------- |
| Interaction type  | `following`       | Leader/follower on a shared path: primary is the follower, secondary the leader |
| Exposure kind     | `valid_following` | Time behind a leader on a shared path: every following rate's denominator       |
| Observation basis | `observed`        | Both parties observed at the instant: the only basis a denominator counts       |
| Observation basis | `predicted_only`  | A party not observed: any value is a review-only prediction, never opportunity  |

### Motion class and other tokens

Motion class tokens are l5tracks': `rigid_vehicle` (car, truck, bus), `two_wheeler` (cyclist,
motorcyclist), `pedestrian` and `unknown` (dynamic or unclassified). Following metrics apply to
`rigid_vehicle` pairs only.

| Vocabulary         | Tokens                                                                                                   |
| ------------------ | -------------------------------------------------------------------------------------------------------- |
| Reference point    | `body_centre`, `near_face_centre`, `cluster_medoid`                                                      |
| Belief provenance  | `class_prior`, `accumulated`, `observed`                                                                 |
| Path extremity     | `leading`, `trailing`                                                                                    |
| Uncertainty kind   | `none`, `sigma`, `interval`, `bounds`                                                                    |
| Propagation method | `analytic`, `linearised`, `sigma_point`, `monte_carlo`                                                   |
| Benchmark kind     | `legal`, `research_threshold`, `external_distribution`, `local_distribution`, `no_established_threshold` |
