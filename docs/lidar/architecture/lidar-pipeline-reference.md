# LiDAR pipeline reference

Complete reference for the velocity.report LiDAR processing pipeline: data flow, component inventory, production deployment architecture, and the metrics-first data science boundaries around tuning and future classification work.

---

## Current data flow

```
PCAP/Live UDP → Parse → Frame → Background → Foreground → Cluster → Track → Classify → API
                                                                                 ↓
                                                                  JSON/CSV Export + Labelled Runs
                                                                                 ↓
                                                                    Scorecards / Replay Benchmarks
```

## Existing components

| Component             | Location                                                                                                | Status      |
| --------------------- | ------------------------------------------------------------------------------------------------------- | ----------- |
| PCAP Reader           | [internal/lidar/l1packets/network/pcap.go](../../../internal/lidar/l1packets/network/pcap.go)           | ✅ Complete |
| Frame Builder         | [internal/lidar/l2frames/frame_builder.go](../../../internal/lidar/l2frames/frame_builder.go)           | ✅ Complete |
| Background Manager    | [internal/lidar/l3grid/background.go](../../../internal/lidar/l3grid/background.go)                     | ✅ Complete |
| Foreground Extraction | [internal/lidar/l3grid/foreground.go](../../../internal/lidar/l3grid/foreground.go)                     | ✅ Complete |
| DBSCAN Clustering     | [internal/lidar/l4perception/cluster.go](../../../internal/lidar/l4perception/cluster.go)               | ✅ Complete |
| Kalman Tracking       | [internal/lidar/l5tracks/tracking.go](../../../internal/lidar/l5tracks/tracking.go)                     | ✅ Complete |
| Rule-Based Classifier | [internal/lidar/l6objects/classification.go](../../../internal/lidar/l6objects/classification.go)       | ✅ Complete |
| Track Store           | [internal/lidar/storage/sqlite/track_store.go](../../../internal/lidar/storage/sqlite/track_store.go)   | ✅ Complete |
| REST API              | `internal/lidar/monitor/track_api.go`                                                                   | ✅ Complete |
| Pipeline benchmark    | [internal/lidar/lidarbench/lidarbench.go](../../../internal/lidar/lidarbench/lidarbench.go)             | ✅ Complete |
| Research Data Export  | [internal/lidar/adapters/training_data.go](../../../internal/lidar/adapters/training_data.go)           | ✅ Complete |
| Analysis Run Store    | [internal/lidar/storage/sqlite/analysis_run.go](../../../internal/lidar/storage/sqlite/analysis_run.go) | ✅ Complete |
| Sweep Runner          | [internal/lidar/sweep/runner.go](../../../internal/lidar/sweep/runner.go)                               | ✅ Complete |
| Auto-Tuner            | [internal/lidar/sweep/auto.go](../../../internal/lidar/sweep/auto.go)                                   | ✅ Complete |
| Sweep Scoring         | `internal/lidar/sweep/scoring.go`                                                                       | ✅ Complete |
| Sweep Dashboard       | `internal/lidar/monitor/html/sweep_dashboard.html`                                                      | ✅ Complete |
| Hungarian Solver      | [internal/lidar/l5tracks/hungarian.go](../../../internal/lidar/l5tracks/hungarian.go)                   | ✅ Complete |
| Ground Removal        | [internal/lidar/l4perception/ground.go](../../../internal/lidar/l4perception/ground.go)                 | ✅ Complete |
| OBB Estimation        | [internal/lidar/l4perception/obb.go](../../../internal/lidar/l4perception/obb.go)                       | ✅ Complete |
| Debug Collector       | [internal/lidar/debug/collector.go](../../../internal/lidar/debug/collector.go)                         | ✅ Complete |
| Behaviour Following   | [internal/lidar/l8behaviour/doc.go](../../../internal/lidar/l8behaviour/doc.go)                         | Gated       |

Behaviour following carries the following-metric contracts and equations, the local following
path, leader choice, encounter exposure and a held-out scoring harness, validated on analytic
scenarios only. Encounters persist to `lidar_interaction_events`, `lidar_interaction_instants` and
`lidar_exposure_windows` through
[interaction_store.go](../../../internal/lidar/storage/sqlite/interaction_store.go), write-once
per version. Reporting and its API are not built, and production emission waits for G-SMO-1 and
the held-out metric gate; see the
[behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md).

### Behaviour following methods

Each method id versions its rules and the meaning of its parameters; a change to either moves the
id. No parameter has a default: the caller states every bound, and the bounds enter the geometry
id or the encounter's recorded method id. The analytic scenarios' values are in
`EncounterScenarioParams`; they are fixture values, not calibrated ones.

| Method id                       | Decides                                                                                                                                                                                                                              | Parameters                                                                                                                                                                     |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `following_pointwise_v1`        | Bumper endpoints, spatial gap and net time gap at one instant, with linearised sigma and precedence-ordered suppression                                                                                                              | `speed_floor_mps`, `corridor_half_width_m`, `max_relative_heading_rad`                                                                                                         |
| `following_local_path_v1`       | One follower's directed centreline over its own passage, from observed moving evidence; its group of same-path tracks; refusal with a path condition for weak support, reversal, crossing, fork or merge, or lateral incompatibility | `knot_spacing_m`, `group_lateral_m`, `max_tangent_rad`, `min_speed_mps`, `min_track_evidence`, `min_overlap_knots`, `min_samples_per_knot`, `max_bridge_knots`, `min_extent_m` |
| `following_pairing_v1`          | The nearest credible leader at each follower instant, `ambiguous_leader` when a second body is not separable, `no_common_path` when the nearest body is not on the path, and a disposition for every other body                      | `max_leader_range_m`, `separation_sigmas`, `unresolved_separation_m`, and the corridor half-width                                                                              |
| `following_sync_frame_exact_v1` | Pairs are evaluated on samples at the follower's exact capture time; a leader without a row there is interpolated for ordering and accounted as `not_observed`                                                                       | none                                                                                                                                                                           |
| `following_exposure_v1`         | Encounter accounting, valid following time, band durations and rates, minimum and median with Monte Carlo intervals, and the minimum-opportunity, class and stage guards                                                             | `min_opportunity_seconds`, `max_interval_nanos`, `interval_coverage`, `monte_carlo_samples`, `common_mode_fraction`                                                            |
| `following_encounter_v1`        | The composition of the five above, recorded on every encounter measurement with the parameters' hash appended                                                                                                                        | all of the above                                                                                                                                                               |
| `following_heldout_scoring_v1`  | Endpoint, gap and interval-coverage scoring against independent references, stratified by class, range, face aspect and support, against bounds pinned by hash before scoring                                                        | `nominal_coverage`, `range_edges_m`, `aspect_edges_rad`, and the acceptance bounds                                                                                             |

The separability rule, in full: a body further ahead competes with the nearest unless its trailing
extreme clears the nearest's leading extreme by more than `separation_sigmas` combined one-sigmas,
or, when either extreme cannot be projected, unless the centres are more than
`unresolved_separation_m` apart. A candidate must lie within the corridor half-width of both the
path and the follower. The Monte Carlo interval draws each instant's error as
`sqrt(rho) z_common + sqrt(1 - rho) z_instant` times its one-sigma, with `rho` the common-mode
fraction, from a seed derived from the method, parameters, geometry and pair.

## Production deployment architecture (phase 4.3)

```
┌─────────────────────────────────────────────────────────────────┐
│                      Edge Node (Raspberry Pi)                   │
│                                                                 │
│  [UDP:2369] → [LIDAR Pipeline] → [Local SQLite] → [REST API]    │
│                      ↓                   ↓                      │
│        [Rule-Based + Tunable L6]  [Runs / Labels / Metrics]     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                                 ↓
                   [Replay Packs / Consolidated Analysis]
                                 ↓
┌─────────────────────────────────────────────────────────────────┐
│               Offline Analysis / Research Workstation           │
│                                                                 │
│  [Reference Runs] → [Scorecards / Threshold Studies]            │
│                           ↓                                     │
│             [Optional Classification Research]                  │
│                           ↓                                     │
│     [Deploy Only If It Beats Transparent Baseline]              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Metrics and Threshold Update Flow:**

1. Collect labelled tracks from labelling UI
2. Re-run the same scenes with explicit parameter bundles
3. Compare scorecards: detection, fragmentation, false positives, velocity
   coverage, and stability
4. Version the winning thresholds/params and document the metric deltas
5. Monitor production metrics → collect new edge cases → repeat

**Optional classification research** may use the same labelled runs and exported
features, but it is not on the critical path. Any candidate model must beat the
current rule-based baseline on fixed replay packs before deployment is even
considered.

## Metrics-First data science and optional classification flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                   METRICS-FIRST DATA SCIENCE WORKFLOW                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────┐    ┌───────────┐    ┌──────────┐    ┌──────────────────┐   │
│  │  PCAP   │───→│   Parse   │───→│  Frame   │───→│    Background    │   │
│  │  Live   │    │           │    │ Builder  │    │    Subtraction   │   │
│  └─────────┘    └───────────┘    └──────────┘    └────────┬─────────┘   │
│                                                           │             │
│                                                           ▼             │
│  ┌─────────────────┐    ┌──────────────────┐    ┌──────────────────┐    │
│  │   Foreground    │───→│     DBSCAN       │───→│     Tracker      │    │
│  │     Points      │    │   Clustering     │    │    Update()      │    │
│  └─────────────────┘    └──────────────────┘    └────────┬─────────┘    │
│                                                          │              │
│                                                          ▼              │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │              Rule-Based Classify (L6)                            │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                     ANALYSIS RUN                                │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐  │    │
│  │   │  params_json│    │  lidar_run_    │    │ Split/Merge     │  │    │
│  │   │   (all cfg) │    │    tracks      │    │   Detection     │  │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    LABELLING UI                                 │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐  │    │
│  │   │   Track     │    │    Label       │    │   Quality       │  │    │
│  │   │  Browser    │───→│   Assignment   │───→│   Marking       │  │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │               SCORECARDS / REPLAY BENCHMARKS                    │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐  │    │
│  │   │  Threshold  │    │   Parameter    │    │  Report Metric  │  │    │
│  │   │   Studies   │    │    Tuning      │    │   Validation    │  │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │           OPTIONAL CLASSIFICATION RESEARCH                      │    │
│  │                                                                 │    │
│  │   Deploy only if benchmark wins are reproducible & explainable  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```
