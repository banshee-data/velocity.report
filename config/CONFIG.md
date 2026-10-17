```
 ░▒▓██████▓▒░ ░▒▓██████▓▒░░▒▓███████▓▒░░▒▓████████▓▒░▒▓█▓▒░░▒▓██████▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░
░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░▒▓█▓▒░
░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓██████▓▒░ ░▒▓█▓▒░▒▓█▓▒▒▓███▓▒░
░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░
 ░▒▓██████▓▒░ ░▒▓██████▓▒░░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░░▒▓██████▓▒░
```

This directory holds the canonical LiDAR tuning configuration for
velocity.report.

For ports, thresholds, and fixed constants outside the tuning schema, see
[MAGIC_NUMBERS.md](../MAGIC_NUMBERS.md).

The segment selectors, which choose capture windows for annotation, live
beside it in [segment-selectors.defaults.json](segment-selectors.defaults.json)
and are described in [SELECTORS.md](SELECTORS.md).

## Schema

The runtime uses a versioned nested schema.

- `$.version`: must equal `2`
- `$.l1`: sensor identity and data-source selection
- `$.l3`, `$.l4`, `$.l5`: each carries an engine selector and exactly one
  engine block matching it
- `$.pipeline`: cross-cutting runtime settings

The runtime rejects:

- missing required fields
- unknown fields
- old flat root keys
- legacy spellings such as `neighbor_*` and `*_meters`

## Usage

The experimental heading path is selected by `l5.cv_kf_v1.obb_axis_coherence_enabled`
(or the corresponding `l5.imm_cv_ca_v2` field). It defaults to `false` in all shipped tuning
files. Existing version-2 files must add this required boolean; the strict loader does not
silently assume it. Runtime tuning accepts the same field and reports its effective value.
Switching paths clears the candidate's observed-support reference; use a fresh replay for A/B.
When enabled, published box dimensions are a conservative envelope of the fresh measured OBB
at the filtered centre and heading, not a reconstructed vehicle body. See the
[D2 experiment contract](../docs/plans/lidar-heading-d2-implementation-report.md).

```bash
./velocity-report --enable-lidar
./velocity-report --config config/tuning.example.json --enable-lidar

make config-validate TUNING_CONFIG=config/tuning.defaults.json
```

LiDAR networking is process-level CLI configuration:
`--lidar-udp-port`, `--lidar-udp-rcv-buf`, `--lidar-forward-port`, and
`--lidar-foreground-forward-port` are startup-only and do not appear in the
JSON tuning schema or the `/api/lidar/params` hot-reload flow.

## Canonical example

This block mirrors [tuning.example.json](tuning.example.json). CI validates
parity between them.

```json
{
  "version": 2,
  "l1": {
    "sensor": "hesai-pandar40p",
    "data_source": "live"
  },
  "l3": {
    "engine": "ema_baseline_v1",
    "ema_baseline_v1": {
      "background_update_fraction": 0.02,
      "closeness_multiplier": 3,
      "safety_margin_metres": 0.15,
      "noise_relative": 0.02,
      "neighbour_confirmation_count": 3,
      "seed_from_first": true,
      "warmup_duration_nanos": 30000000000,
      "warmup_min_frames": 100,
      "post_settle_update_fraction": 0,
      "enable_diagnostics": false,
      "freeze_duration": "5s",
      "freeze_threshold_multiplier": 3,
      "settling_period": "5m",
      "snapshot_interval": "2h",
      "change_threshold_snapshot": 100,
      "reacquisition_boost_multiplier": 5,
      "min_confidence_floor": 3,
      "locked_baseline_threshold": 50,
      "locked_baseline_multiplier": 4,
      "sensor_movement_foreground_threshold": 0.2,
      "sensor_movement_drift_ratio_threshold": 0.5,
      "background_drift_threshold_metres": 0.5,
      "background_drift_ratio_threshold": 0.1,
      "settling_min_coverage": 0.8,
      "settling_max_spread_delta": 0.001,
      "settling_min_region_stability": 0.95,
      "settling_min_confidence": 10
    }
  },
  "l4": {
    "engine": "dbscan_xy_v1",
    "dbscan_xy_v1": {
      "foreground_dbscan_eps": 0.8,
      "foreground_min_cluster_points": 5,
      "foreground_max_input_points": 8000,
      "max_sample_points": 0,
      "height_band_floor": -2.8,
      "height_band_ceiling": 1.5,
      "remove_ground": true,
      "max_cluster_diameter": 12,
      "min_cluster_diameter": 0.05,
      "max_cluster_aspect_ratio": 15
    }
  },
  "l5": {
    "engine": "cv_kf_v1",
    "cv_kf_v1": {
      "gating_distance_squared": 36,
      "process_noise_pos": 0.05,
      "process_noise_vel": 0.2,
      "measurement_noise": 0.05,
      "occlusion_cov_inflation": 0.5,
      "hits_to_confirm": 4,
      "max_misses": 3,
      "max_misses_confirmed": 15,
      "max_tracks": 100,
      "max_reasonable_speed_mps": 30,
      "max_position_jump_metres": 5,
      "max_predict_dt": 0.5,
      "max_covariance_diag": 100,
      "min_points_for_pca": 4,
      "obb_heading_smoothing_alpha": 0.08,
      "obb_aspect_ratio_lock_threshold": 0.25,
      "obb_heading_lock_max_rejections": 5,
      "obb_axis_coherence_enabled": false,
      "min_associable_extent_metres": 0.5,
      "association_extent_cost_weight": 0,
      "deleted_track_render_fade": "500ms",
      "max_track_history_length": 200,
      "max_speed_history_length": 100,
      "merge_size_ratio": 2.5,
      "split_size_ratio": 0.3,
      "deleted_track_grace_period": "5s",
      "min_observations_for_classification": 5,
      "box_model": {
        "enabled": true,
        "axis_source": "rectangle_fit",
        "course_min_speed_mps": 1.0,
        "max_curvature_per_metre": 0.2,
        "min_yaw_rate_deg_per_sec": 3,
        "unlabelled_yaw_rate_deg_per_sec": 45,
        "yaw_accel_sigma": 0.4,
        "axis_sigma_scale": 1.5,
        "centre_pull": 0.3,
        "vehicle_length_floor_metres": 3.8,
        "course_lead_sec": 0,
        "course_rate_tau_sec": 0,
        "displacement_label_metres": 0,
        "extent_window_spans": 50,
        "tall_height_metres": 0,
        "tall_length_floor_metres": 0,
        "exclusive_margin_metres": 0
      }
    }
  },
  "pipeline": {
    "buffer_timeout": "500ms",
    "min_frame_points": 1000,
    "flush_interval": "60s",
    "background_flush": false,
    "frame_budget_ms": 98
  }
}
```

## Runtime updates

`POST /api/lidar/params` accepts partial updates as nested JSON matching
the [tuning.defaults.json](tuning.defaults.json) structure. The server
normalises legacy dot-path keys, but nested format is preferred.

Example:

```json
{
  "l3": {
    "ema_baseline_v1": {
      "noise_relative": 0.03,
      "closeness_multiplier": 2.5
    }
  },
  "l5": {
    "cv_kf_v1": {
      "max_tracks": 120
    }
  }
}
```

Hot-reloadable parameters:

- L3 engine parameters (`l3.ema_baseline_v1.*`)
- L4 DBSCAN: `foreground_dbscan_eps`, `foreground_min_cluster_points`, `foreground_max_input_points`
- L5 tracker parameters (`l5.cv_kf_v1.*`), including the optional
  `l5.cv_kf_v1.solid_body` and `l5.cv_kf_v1.box_model` blocks. A block is
  applied whole: every key is required, and the block's own rules are
  checked before any of it reaches the tracker.

`l1.*`, `pipeline.*`, the engine selectors, and every L4 key except the
three DBSCAN keys above are startup-only.

A whole config file can be applied the same way, from the server's config
directory, when a PCAP replay starts: see
[Experiment parameter sets](#experiment-parameter-sets).

## Experiment parameter sets

A run's parameters should be a file someone can open later.
[experiments/](experiments/) holds a complete tuning config for each
reported experiment arm. Each is [tuning.defaults.json](tuning.defaults.json)
with that arm's changes, and its row in
[experiments/README.md](experiments/README.md) names every key it changes
and the report it serves. A test,
[TestExperimentConfigsDeclareEveryChangeFromTheDefaults](../internal/config/experiments_test.go),
loads each file under the strict loader and fails when a file changes a key
its row does not name. A stale copy of an older defaults file therefore
fails the test rather than running as something other than its arm.

| File                                                                                   | Arm                                                  |
| -------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| [solid-body-shadow.json](experiments/solid-body-shadow.json)                           | Solid body as a shadow, T1 + T3, full members        |
| [solid-body-a2.json](experiments/solid-body-a2.json)                                   | The shadow plus `near_edge_tracking` (A2)            |
| [alignment-candidate.json](experiments/alignment-candidate.json)                       | A2 with the alignment options and containment        |
| [geometry-convergence-candidate.json](experiments/geometry-convergence-candidate.json) | The current candidate: A2 with the rectangle heading |
| [geometry-convergence-sigma1.json](experiments/geometry-convergence-sigma1.json)       | The candidate with the rectangle fit at its own σ    |
| [height-floor-road.json](experiments/height-floor-road.json)                           | L4 height band floor at road height (2.3 m mount)    |

### Running a set

- **The :8081 status page.** The PCAP Replay form has a Tuning Config
  dropdown, filled on page load from every `.json` file under the server's
  config directory. A file that cannot be applied to the running server is
  listed but disabled, with the reason. "(keep the server's current
  parameters)" leaves the parameters alone.
- **The API.** `POST /api/lidar/pcap/start` takes `tuning_config`, a path
  relative to the config directory, as a JSON or form field. See
  [pcap-analysis-mode.md](../docs/lidar/operations/pcap-analysis-mode.md#start-pcap-replay-analysis-mode).
  The server applies the file before it stops the current source. A file
  that fails to load or changes a startup-only key refuses the start before
  any parameter changes. The
  response and `GET /api/lidar/data_source` echo `tuning_config` and
  `tuning_fingerprint`. `GET /api/lidar/configs` lists the files with their
  fingerprints and whether each can be applied.
- **Offline replay.** The replay tools take a set without a server, for
  example `lidar-state-estimation-baseline -tuning <file>`.
- **Startup.** `--config config/experiments/<file>.json` runs a set from the
  first frame. Use this for a set that changes a startup-only key.
- **As web scenes.** `make scene-variants SET=<set>.json` replays one
  window of a site under each file of a set and exports every run as a web
  scene the player opens unchanged, with a manifest naming each file and
  fingerprint. See
  [Scene variant exports](../tools/s2-archive/README.md#scene-variant-exports)
  and the [plan](../docs/plans/lidar-scene-variant-exports-plan.md).

The config directory is `--lidar-config-dir`, by default the directory of
`--config`. A file can be applied at a replay start when its engine
selectors, `pipeline` block and L4 keys other than DBSCAN's three match the
running server's. `l1` is not compared, because the replay sets the source.
Apply such a file and the server runs every key in it; any other file needs
a restart with `--config`.

### Building a new set

When an experiment needs parameters that no file here holds, build its set
before the runs, and commit it with the code and the report:

1. Copy [tuning.defaults.json](tuning.defaults.json) to
   `config/experiments/<arm>.json`. Start from the defaults even when the arm
   builds on another set, so the file's changes are the arm's changes.
2. Change only the keys the arm changes. For solid-body options, add the
   `l5.cv_kf_v1.solid_body` block with every key present (see the field
   reference below). Use the mapping below to translate replay experiment
   names into keys.
3. Add a row to [experiments/README.md](experiments/README.md) that names
   each changed key, says what the arm is, and links the report.
4. Validate it, and run the anti-drift test:

   ```bash
   make config-validate TUNING_CONFIG=config/experiments/<arm>.json
   go test ./internal/config -run Experiment
   ```

5. Run the arm from the file, and record the file's path and fingerprint in
   the report's provenance beside the commit.

When the defaults move, the test fails for every set until each is
regenerated from the new defaults. A regenerated set has a new fingerprint.
The commit a report cites still holds the file as it ran.

### Replay experiment names

The offline replay tools name default-off options as experiments
(`-experiment solid_body,near_edge_track,...`). Each solid-body name has a
key in the `l5.cv_kf_v1.solid_body` block. A replay that names experiments
combines them with the block it loaded: a name turns its option on, and a
named scale replaces the block's.

| Replay experiment                         | Block key                                              |
| ----------------------------------------- | ------------------------------------------------------ |
| `solid_body`                              | `enabled`                                              |
| `solid_body_full_members`                 | `full_members`                                         |
| `near_edge_track`                         | `near_edge_tracking`                                   |
| `near_edge_track_a1`                      | `near_edge_medoid_gate`                                |
| `solid_body_face_hysteresis`              | `face_hysteresis`                                      |
| `solid_body_face_consider`                | `face_entry_consider`                                  |
| `solid_body_course_faces`                 | `course_aligned_faces`                                 |
| `solid_body_reference_translation`        | `reference_translation`                                |
| `solid_body_rank_one_medoid`              | `rank_one_medoid_scale`: `1`                           |
| `solid_body_rank_one_medoid_tight`        | `rank_one_medoid_scale`: `0.25`                        |
| `solid_body_course_heading`               | `course_heading`                                       |
| `solid_body_extent_prior_floor`           | `extent_prior_floor`                                   |
| `solid_body_face_plane_spans`             | `face_plane_spans`                                     |
| `solid_body_extent_growth`                | `extent_growth_admission`                              |
| `solid_body_vehicle_extent_floor`         | `vehicle_extent_floor`                                 |
| `solid_body_end_face_centring`            | `end_face_centring`                                    |
| `solid_body_end_face_centring_open_prior` | `end_face_centring` and `end_face_centring_open_prior` |
| `solid_body_containment`                  | `containment`                                          |
| `solid_body_rectangle_fit`                | `rectangle_fit`                                        |
| `solid_body_rectangle_heading`            | `rectangle_heading`                                    |
| `solid_body_rectangle_course_fusion`      | `rectangle_course_fusion`                              |
| `solid_body_rectangle_sigma_mid`          | `rectangle_sigma_scale`: `1.5`                         |
| `solid_body_rectangle_sigma_wide`         | `rectangle_sigma_scale`: `2`                           |

The other replay experiments have no config key yet, so a set cannot carry
them, and the server cannot run them: `adaptive_uncertainty`,
`likelihood_cost`, `cascade`, `density_cap`, `range_adaptive_eps`,
`flip_rule`, `no_region_overrides`, `region_overrides_volatile_only`,
`region_overrides_quiet_only`, `region_overrides_cell_tercile`,
`volatile_window_cap_1m`, `volatile_window_cap_2m`, `support_filter`,
`dwell_policy`, `dwell_policy_t40`, `dwell_policy_t60`,
`dwell_policy_t40_drift2m`, `dwell_policy_t40_median`, `measurement_time`,
`capture_gap_predict`, `coast_support`, `coast_time_inflation`,
`class_coast_bounds`, `reacquisition_guard`, `occlusion_continuity`,
`contested_rejoin` (with the reacquisition guard it needs), `tentative_priority`,
`class_identity` and `fixed_lag_rts`. An arm that uses one runs through the offline replay tools,
and its report names the experiment beside the set.

The L3 background experiments among them (`no_region_overrides`, the
`region_overrides_*` and `volatile_window_cap_*` names, and `support_filter`)
also run in `lidar-bench -experiment`. A replay names at most one of
`no_region_overrides` and the two scopes, and at most one window cap. It may
not name `region_overrides_cell_tercile` with `no_region_overrides`, or a
window cap with `no_region_overrides` or `region_overrides_quiet_only`: under
those no cell runs the override the option changes, so the arm would run as
another and be reported under its name.

`dwell_policy` absorbs a track that has stood still for 20 s: at most
0.5 m/s, and within 1 m of its estimate when the stationary run began.
`dwell_policy_t40` and `dwell_policy_t60` are the same policy at 40 s and
60 s, with the same speed, drift rule and box margin.
`dwell_policy_t40_drift2m` and `dwell_policy_t40_median` keep 40 s and change
the drift rule: 2 m from the run's first estimate, or 1 m from the running
median of the run's estimates over its last 400 observed frames. A replay
names at most one of the five, and the manifest's `dwell_policy` section
records the time used as `absorb_after_seconds` and the drift rule as
`stationary_drift_metres` and `stationary_anchor` (`first_estimate` or
`running_median`, with `stationary_median_window_frames` for the median). The
policy ends tracks, so `lidar-bench` refuses all five.

## Key order

Canonical ordering is derived from `config/tuning.defaults.json`.

```bash
make check-config-order
make sync-config-order
make check-config-maths
```

## Field reference

Schema sources: [tuning.defaults.json](tuning.defaults.json),
[internal/config/tuning.go](../internal/config/tuning.go).

### Root

| Path       | Type   | Primary consumer                                  | Notes                            |
| ---------- | ------ | ------------------------------------------------- | -------------------------------- |
| `version`  | int    | [Validate](../internal/config/tuning_validate.go) | Must equal `2`                   |
| `l1`       | object | [LoadTuningConfig](../internal/config/tuning.go)  | Sensor identity and data source  |
| `l3`       | object | [LoadTuningConfig](../internal/config/tuning.go)  | Background/foreground extraction |
| `l4`       | object | [LoadTuningConfig](../internal/config/tuning.go)  | Clustering and ground filtering  |
| `l5`       | object | [LoadTuningConfig](../internal/config/tuning.go)  | Tracking                         |
| `pipeline` | object | [LoadTuningConfig](../internal/config/tuning.go)  | Cross-cutting runtime settings   |

### L1

| Path             | Type   | Primary consumer                                        | Notes                                  |
| ---------------- | ------ | ------------------------------------------------------- | -------------------------------------- |
| `l1.sensor`      | string | [GetSensor](../internal/config/tuning_accessors.go)     | Sensor identifier                      |
| `l1.data_source` | string | [GetDataSource](../internal/config/tuning_accessors.go) | One of `live`, `pcap`, `pcap_analysis` |

### L3

Maths: [background-grid-settling-maths.md](../data/maths/background-grid-settling-maths.md),
[20260219-unify-l3-l4-settling.md](../data/maths/proposals/20260219-unify-l3-l4-settling.md)

| Path                                                       | Type    | Primary consumer                                                               | Notes                                                                          |
| ---------------------------------------------------------- | ------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------ |
| `l3.engine`                                                | string  | [(\*L3Config).ActiveCommon](../internal/config/tuning_accessors.go)            | Active L3 engine.                                                              |
| `l3.ema_baseline_v1.background_update_fraction`            | float64 | [GetBackgroundUpdateFraction](../internal/config/tuning_accessors.go)          | Background EMA alpha.                                                          |
| `l3.ema_baseline_v1.closeness_multiplier`                  | float64 | [GetClosenessMultiplier](../internal/config/tuning_accessors.go)               | Background acceptance multiplier.                                              |
| `l3.ema_baseline_v1.safety_margin_metres`                  | float64 | [GetSafetyMarginMetres](../internal/config/tuning_accessors.go)                | Additive safety margin.                                                        |
| `l3.ema_baseline_v1.noise_relative`                        | float64 | [GetNoiseRelative](../internal/config/tuning_accessors.go)                     | Range-relative noise model.                                                    |
| `l3.ema_baseline_v1.neighbour_confirmation_count`          | int     | [GetNeighbourConfirmationCount](../internal/config/tuning_accessors.go)        | Spatial confirmation threshold.                                                |
| `l3.ema_baseline_v1.seed_from_first`                       | bool    | [GetSeedFromFirst](../internal/config/tuning_accessors.go)                     | Seed cells from first observation.                                             |
| `l3.ema_baseline_v1.warmup_duration_nanos`                 | int64   | [GetWarmupDurationNanos](../internal/config/tuning_accessors.go)               | Warmup duration.                                                               |
| `l3.ema_baseline_v1.warmup_min_frames`                     | int     | [GetWarmupMinFrames](../internal/config/tuning_accessors.go)                   | Minimum warmup frames.                                                         |
| `l3.ema_baseline_v1.post_settle_update_fraction`           | float64 | [GetPostSettleUpdateFraction](../internal/config/tuning_accessors.go)          | Background alpha after settling.                                               |
| `l3.ema_baseline_v1.enable_diagnostics`                    | bool    | [GetEnableDiagnostics](../internal/config/tuning_accessors.go)                 | Verbose background diagnostics.                                                |
| `l3.ema_baseline_v1.freeze_duration`                       | string  | [GetFreezeDuration](../internal/config/tuning_accessors.go)                    | Freeze duration after foreground.                                              |
| `l3.ema_baseline_v1.freeze_threshold_multiplier`           | float64 | [GetFreezeThresholdMultiplier](../internal/config/tuning_accessors.go)         | Freeze trigger multiplier.                                                     |
| `l3.ema_baseline_v1.settling_period`                       | string  | [GetSettlingPeriod](../internal/config/tuning_accessors.go)                    | Settling period before persistence.                                            |
| `l3.ema_baseline_v1.snapshot_interval`                     | string  | [GetSnapshotInterval](../internal/config/tuning_accessors.go)                  | Snapshot cadence.                                                              |
| `l3.ema_baseline_v1.change_threshold_snapshot`             | int     | [GetChangeThresholdSnapshot](../internal/config/tuning_accessors.go)           | Minimum changed cells before snapshot.                                         |
| `l3.ema_baseline_v1.reacquisition_boost_multiplier`        | float64 | [GetReacquisitionBoostMultiplier](../internal/config/tuning_accessors.go)      | Fast background reacquisition multiplier.                                      |
| `l3.ema_baseline_v1.min_confidence_floor`                  | int     | [GetMinConfidenceFloor](../internal/config/tuning_accessors.go)                | Minimum confidence preserved during foreground.                                |
| `l3.ema_baseline_v1.locked_baseline_threshold`             | int     | [GetLockedBaselineThreshold](../internal/config/tuning_accessors.go)           | Observation count needed before baseline lock.                                 |
| `l3.ema_baseline_v1.locked_baseline_multiplier`            | float64 | [GetLockedBaselineMultiplier](../internal/config/tuning_accessors.go)          | Locked-baseline spread multiplier.                                             |
| `l3.ema_baseline_v1.sensor_movement_foreground_threshold`  | float64 | [GetSensorMovementForegroundThreshold](../internal/config/tuning_accessors.go) | Sensor movement detection ratio.                                               |
| `l3.ema_baseline_v1.sensor_movement_drift_ratio_threshold` | float64 | [GetSensorMovementDriftRatioThreshold](../internal/config/tuning_accessors.go) | Sustained-motion drift ratio: fraction of settled cells shifted from baseline. |
| `l3.ema_baseline_v1.background_drift_threshold_metres`     | float64 | [GetBackgroundDriftThresholdMetres](../internal/config/tuning_accessors.go)    | Drift distance threshold.                                                      |
| `l3.ema_baseline_v1.background_drift_ratio_threshold`      | float64 | [GetBackgroundDriftRatioThreshold](../internal/config/tuning_accessors.go)     | Drift ratio threshold.                                                         |
| `l3.ema_baseline_v1.settling_min_coverage`                 | float64 | [GetSettlingMinCoverage](../internal/config/tuning_accessors.go)               | Minimum coverage for convergence.                                              |
| `l3.ema_baseline_v1.settling_max_spread_delta`             | float64 | [GetSettlingMaxSpreadDelta](../internal/config/tuning_accessors.go)            | Maximum spread delta for convergence.                                          |
| `l3.ema_baseline_v1.settling_min_region_stability`         | float64 | [GetSettlingMinRegionStability](../internal/config/tuning_accessors.go)        | Minimum region stability for convergence.                                      |
| `l3.ema_baseline_v1.settling_min_confidence`               | float64 | [GetSettlingMinConfidence](../internal/config/tuning_accessors.go)             | Minimum confidence for convergence.                                            |

### L4

Maths: [clustering-maths.md](../data/maths/clustering-maths.md),
[ground-plane-maths.md](../data/maths/ground-plane-maths.md)

| Path                                            | Type    | Primary consumer                                                        | Notes                                                                                                          |
| ----------------------------------------------- | ------- | ----------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `l4.engine`                                     | string  | [(\*L4Config).ActiveCommon](../internal/config/tuning_accessors.go)     | Active L4 engine.                                                                                              |
| `l4.dbscan_xy_v1.foreground_dbscan_eps`         | float64 | [GetForegroundDBSCANEps](../internal/config/tuning_accessors.go)        | DBSCAN epsilon.                                                                                                |
| `l4.dbscan_xy_v1.foreground_min_cluster_points` | int     | [GetForegroundMinClusterPoints](../internal/config/tuning_accessors.go) | DBSCAN min points.                                                                                             |
| `l4.dbscan_xy_v1.foreground_max_input_points`   | int     | [GetForegroundMaxInputPoints](../internal/config/tuning_accessors.go)   | DBSCAN input cap.                                                                                              |
| `l4.dbscan_xy_v1.max_sample_points`             | int     | [DBSCANParamsFromTuning](../internal/lidar/l4perception/cluster.go)     | Offline replay evidence cap per cluster, 0–1024; default 0 disables retention. Not yet a live runtime setting. |
| `l4.dbscan_xy_v1.height_band_floor`             | float64 | [GetHeightBandFloor](../internal/config/tuning_accessors.go)            | Lower Z filter bound in sensor frame; the road is near −2.3 m at the deployed ~2.3 m mount                     |
| `l4.dbscan_xy_v1.height_band_ceiling`           | float64 | [GetHeightBandCeiling](../internal/config/tuning_accessors.go)          | Upper Z filter bound.                                                                                          |
| `l4.dbscan_xy_v1.remove_ground`                 | bool    | [GetRemoveGround](../internal/config/tuning_accessors.go)               | Ground filter master switch.                                                                                   |
| `l4.dbscan_xy_v1.max_cluster_diameter`          | float64 | [GetMaxClusterDiameter](../internal/config/tuning_accessors.go)         | Maximum accepted cluster diameter.                                                                             |
| `l4.dbscan_xy_v1.min_cluster_diameter`          | float64 | [GetMinClusterDiameter](../internal/config/tuning_accessors.go)         | Minimum accepted cluster diameter.                                                                             |
| `l4.dbscan_xy_v1.max_cluster_aspect_ratio`      | float64 | [GetMaxClusterAspectRatio](../internal/config/tuning_accessors.go)      | Maximum accepted cluster aspect ratio.                                                                         |

### L5

Maths: [tracking-maths.md](../data/maths/tracking-maths.md)

The heading experiment's additional keys remain explicit, including disabled candidates:

| Path                                          | Type    | Primary consumer                                                  | Notes                                                    |
| --------------------------------------------- | ------- | ----------------------------------------------------------------- | -------------------------------------------------------- |
| `l5.cv_kf_v1.obb_axis_coherence_enabled`      | bool    | [Heading axis](../internal/lidar/l5tracks/heading_axis.go)        | Default-off axial interpretation candidate.              |
| `l5.cv_kf_v1.obb_heading_lock_max_rejections` | int     | [Heading update](../internal/lidar/l5tracks/tracking_update.go)   | Bounds heading lock rejection streaks.                   |
| `l5.cv_kf_v1.min_associable_extent_metres`    | float64 | [Association](../internal/lidar/l5tracks/tracking_association.go) | Minimum usable extent for association.                   |
| `l5.cv_kf_v1.association_extent_cost_weight`  | float64 | [Association](../internal/lidar/l5tracks/tracking_association.go) | Default-zero extent penalty; experimental.               |
| `l5.cv_kf_v1.deleted_track_render_fade`       | string  | [Tracker config](../internal/lidar/l5tracks/tracking.go)          | Duration for deleted-track rendering, not a measurement. |

| Path                                              | Type    | Primary consumer                                                              | Notes                                                                                        |
| ------------------------------------------------- | ------- | ----------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `l5.engine`                                       | string  | [(\*L5Config).ActiveCommon](../internal/config/tuning_accessors.go)           | Active L5 engine.                                                                            |
| `l5.cv_kf_v1.gating_distance_squared`             | float64 | [GetGatingDistanceSquared](../internal/config/tuning_accessors.go)            | Mahalanobis association gate.                                                                |
| `l5.cv_kf_v1.process_noise_pos`                   | float64 | [GetProcessNoisePos](../internal/config/tuning_accessors.go)                  | Position process noise.                                                                      |
| `l5.cv_kf_v1.process_noise_vel`                   | float64 | [GetProcessNoiseVel](../internal/config/tuning_accessors.go)                  | Velocity process noise.                                                                      |
| `l5.cv_kf_v1.measurement_noise`                   | float64 | [GetMeasurementNoise](../internal/config/tuning_accessors.go)                 | Measurement noise.                                                                           |
| `l5.cv_kf_v1.occlusion_cov_inflation`             | float64 | [GetOcclusionCovInflation](../internal/config/tuning_accessors.go)            | Coast-mode covariance inflation.                                                             |
| `l5.cv_kf_v1.hits_to_confirm`                     | int     | [GetHitsToConfirm](../internal/config/tuning_accessors.go)                    | Hits required for confirmation.                                                              |
| `l5.cv_kf_v1.max_misses`                          | int     | [GetMaxMisses](../internal/config/tuning_accessors.go)                        | Tentative-track miss budget.                                                                 |
| `l5.cv_kf_v1.max_misses_confirmed`                | int     | [GetMaxMissesConfirmed](../internal/config/tuning_accessors.go)               | Confirmed-track miss budget.                                                                 |
| `l5.cv_kf_v1.max_tracks`                          | int     | [GetMaxTracks](../internal/config/tuning_accessors.go)                        | Tracker capacity, validated in `[1,1000]`.                                                   |
| `l5.cv_kf_v1.max_reasonable_speed_mps`            | float64 | [GetMaxReasonableSpeedMps](../internal/config/tuning_accessors.go)            | Speed sanity limit.                                                                          |
| `l5.cv_kf_v1.max_position_jump_metres`            | float64 | [GetMaxPositionJumpMetres](../internal/config/tuning_accessors.go)            | Max plausible position jump.                                                                 |
| `l5.cv_kf_v1.max_predict_dt`                      | float64 | [GetMaxPredictDt](../internal/config/tuning_accessors.go)                     | Maximum prediction horizon.                                                                  |
| `l5.cv_kf_v1.max_covariance_diag`                 | float64 | [GetMaxCovarianceDiag](../internal/config/tuning_accessors.go)                | Covariance clamp.                                                                            |
| `l5.cv_kf_v1.min_points_for_pca`                  | int     | [GetMinPointsForPCA](../internal/config/tuning_accessors.go)                  | Minimum points for OBB PCA.                                                                  |
| `l5.cv_kf_v1.obb_heading_smoothing_alpha`         | float64 | [GetOBBHeadingSmoothingAlpha](../internal/config/tuning_accessors.go)         | Heading smoothing factor.                                                                    |
| `l5.cv_kf_v1.obb_aspect_ratio_lock_threshold`     | float64 | [GetOBBAspectRatioLockThreshold](../internal/config/tuning_accessors.go)      | Heading lock threshold; default `0.25`, with `0.15` / `0.10` reserved for replay validation. |
| `l5.cv_kf_v1.max_track_history_length`            | int     | [GetMaxTrackHistoryLength](../internal/config/tuning_accessors.go)            | Track history capacity.                                                                      |
| `l5.cv_kf_v1.max_speed_history_length`            | int     | [GetMaxSpeedHistoryLength](../internal/config/tuning_accessors.go)            | Speed history capacity.                                                                      |
| `l5.cv_kf_v1.merge_size_ratio`                    | float64 | [GetMergeSizeRatio](../internal/config/tuning_accessors.go)                   | Merge heuristic ratio.                                                                       |
| `l5.cv_kf_v1.split_size_ratio`                    | float64 | [GetSplitSizeRatio](../internal/config/tuning_accessors.go)                   | Split heuristic ratio.                                                                       |
| `l5.cv_kf_v1.deleted_track_grace_period`          | string  | [GetDeletedTrackGracePeriod](../internal/config/tuning_accessors.go)          | Deleted-track reuse window.                                                                  |
| `l5.cv_kf_v1.min_observations_for_classification` | int     | [GetMinObservationsForClassification](../internal/config/tuning_accessors.go) | Minimum observations before classification.                                                  |

### L5 solid body

Optional. The block is absent from the defaults, and a config without it
runs with the solid body off. When the block is present, every key is
required. Schema: [L5SolidBody](../internal/config/tuning.go). Mapping onto
the tracker: [SolidBodyOptionsFromTuning](../internal/lidar/l5tracks/tracking_config.go).
Plans: [near-edge tracked state](../docs/plans/lidar-near-edge-tracked-state-plan.md),
[geometry convergence](../docs/plans/lidar-tracker-geometry-convergence-plan.md).

Rules: no option may be set while `enabled` is false, `enabled` requires
`full_members`, `near_edge_medoid_gate` requires `near_edge_tracking`,
`end_face_centring_open_prior` requires `end_face_centring`, and scales are
not negative.

| Path (under `l5.cv_kf_v1.solid_body.`) | Type    | Notes                                                                                                                        |
| -------------------------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `enabled`                              | bool    | Runs the solid-body estimator beside the tracked filter. Alone it is a shadow: the tracks are unchanged.                     |
| `full_members`                         | bool    | Hands the tracker every cluster member each frame. Required by `enabled`: the live pipeline keeps no point sample otherwise. |
| `near_edge_tracking`                   | bool    | Runs the body's state machine on the tracked filter with A2 face-residual association. Changes the tracks.                   |
| `near_edge_medoid_gate`                | bool    | A2's ablation A1: body-centre tracks gated on the medoid against the predicted centre.                                       |
| `face_hysteresis`                      | bool    | Face-transition remedy T1.                                                                                                   |
| `face_entry_consider`                  | bool    | Face-transition remedy T2.                                                                                                   |
| `course_aligned_faces`                 | bool    | Remedy T3: faces and spans along the body's own course while it moves.                                                       |
| `reference_translation`                | bool    | The first fix translates the medoid-referenced position to the body centre.                                                  |
| `rank_one_medoid_scale`                | float64 | Remedy T5's half-extent scale for a fix by an end face alone; `0` off, `1` loose, `0.25` tight.                              |
| `course_heading`                       | bool    | The orientation is the course while the body moves.                                                                          |
| `extent_prior_floor`                   | bool    | A dimension shorter than the class prior keeps the prior.                                                                    |
| `face_plane_spans`                     | bool    | A face gives the span along its own plane, not its depth.                                                                    |
| `extent_growth_admission`              | bool    | A merge candidate no wider than the body still gives extents.                                                                |
| `vehicle_extent_floor`                 | bool    | A rigid vehicle's length and width are at least the smallest road car's.                                                     |
| `end_face_centring`                    | bool    | At a fix by an end face alone, the position across the body also updates from the face's midpoint.                           |
| `end_face_centring_open_prior`         | bool    | End-face centring that refuses a narrow face only against a measured width, not a class prior.                               |
| `containment`                          | bool    | Holds the reported box on the frame's points: extents floored at observed spans, position held to what the points allow.     |
| `rectangle_fit`                        | bool    | Records each frame's rectangle orientation fit as a diagnostic; nothing reads it.                                            |
| `rectangle_heading`                    | bool    | The rectangle fit is the body's heading observation (W1b), filtered modulo 90° and labelled by the course.                   |
| `rectangle_course_fusion`              | bool    | With the rectangle heading, the course is a second observation of the axis at speed.                                         |
| `rectangle_sigma_scale`                | float64 | Multiplies the rectangle fit's standard deviation in the axis filter; `0` means one. The candidate uses `1.5`.               |

### L5 box model

On by default since geom0006. With `enabled`, the tracker publishes a box whose centre, heading
and extents hold the cluster's points and whose turn rate is bounded by speed; the tracks
themselves are unchanged. A config without the block, or with `enabled` false
([box-model-off.json](experiments/box-model-off.json)), publishes the legacy box: the tracked
position, the smoothed OBB heading and the frame's PCA extents. When the block is present, every
key is required. Schema: [L5BoxModel](../internal/config/tuning.go). Model:
[box_model.go](../internal/lidar/l5tracks/box_model.go). Reports:
[published box rotation](../docs/lidar/operations/published-box-rotation-2026-10.md),
[refinement](../docs/lidar/operations/published-box-refinement-2026-10.md).

Rules: `axis_source` is `rectangle_fit`, `min_area` or `pca`; every number is finite and not
negative; `yaw_accel_sigma` is positive and `centre_pull` at most 1. `rectangle_fit` and
`min_area` keep every cluster member for the frame, in memory only.

| Path (under `l5.cv_kf_v1.box_model.`) | Type    | Notes                                                                                                                                                                |
| ------------------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`                             | bool    | Publishes the box model's box in place of the legacy OBB fields, with its own centre.                                                                                |
| `axis_source`                         | string  | `rectangle_fit`: the closeness fit on the members. `min_area`: the members' minimum-area rectangle, cheaper and worse on L views. `pca`: the cluster box's axis.     |
| `course_min_speed_mps`                | float64 | Speed from which the direction of travel steers the heading and labels which side is the length. `1.0`.                                                              |
| `max_curvature_per_metre`             | float64 | κ in the turn-rate bound κ·v + ω₀: the tightest turn a vehicle makes, `0.2` (a 5 m radius).                                                                          |
| `min_yaw_rate_deg_per_sec`            | float64 | ω₀ for a labelled or vehicle-sized box, in degrees a second: how fast it may turn at rest. `3`.                                                                      |
| `unlabelled_yaw_rate_deg_per_sec`     | float64 | ω₀ for a box in its first second or smaller than a vehicle, which may be a person turning on the spot. `45`.                                                         |
| `yaw_accel_sigma`                     | float64 | The heading filter's white yaw acceleration, rad/s². `0.4`.                                                                                                          |
| `axis_sigma_scale`                    | float64 | Multiplies the axis observation's standard deviation. `1.5`.                                                                                                         |
| `centre_pull`                         | float64 | Share of the way to the sensor-anchored placement the centre moves each frame, inside the interval that holds the points. `0.3`.                                     |
| `vehicle_length_floor_metres`         | float64 | A labelled box of a vehicle's believed width (1.5 m or more) is drawn at least this long, the unseen part away from the sensor. `3.8`.                               |
| `course_lead_sec`                     | float64 | Leads the course by its own rate times this many seconds, against the Kalman velocity's lag in a turn. `0` (geom0006: 0.4 s cut the turn lag by 0.3°).               |
| `course_rate_tau_sec`                 | float64 | Adds (tau·rate)² to a turning course's variance, so it weighs less against the shape. `0` (no gain in geom0006).                                                     |
| `displacement_label_metres`           | float64 | Labels a box from the track's straight displacement over up to two seconds once it is this long. `0` (geom0006: did not hold out of sample).                         |
| `extent_window_spans`                 | int     | How many of the latest admitted spans the length and width are the third largest of; `0` keeps the whole life's. `50` (five seconds of a vehicle in view).           |
| `tall_height_metres`                  | float64 | The corroborated vertical extent from which a labelled box of a vehicle's width takes `tall_length_floor_metres` instead of the vehicle floor. `0` is off.           |
| `tall_length_floor_metres`            | float64 | The length floor for a tall body: a van, truck or bus seen end-on. `0`.                                                                                              |
| `exclusive_margin_metres`             | float64 | Stops the part of the box beyond its own returns this far short of another cluster's return in its path, and on the sensor's side at its nearest return. `0` is off. |

### Pipeline

| Path                        | Type   | Primary consumer                                             | Notes                                                                            |
| --------------------------- | ------ | ------------------------------------------------------------ | -------------------------------------------------------------------------------- |
| `pipeline.buffer_timeout`   | string | [GetBufferTimeout](../internal/config/tuning_accessors.go)   | Frame assembly timeout.                                                          |
| `pipeline.min_frame_points` | int    | [GetMinFramePoints](../internal/config/tuning_accessors.go)  | Minimum points required to process a frame.                                      |
| `pipeline.flush_interval`   | string | [GetFlushInterval](../internal/config/tuning_accessors.go)   | Background snapshot cadence.                                                     |
| `pipeline.background_flush` | bool   | [GetBackgroundFlush](../internal/config/tuning_accessors.go) | Background snapshot master switch.                                               |
| `pipeline.frame_budget_ms`  | float  | [GetFrameBudgetMs](../internal/config/profile.go)            | Per-frame wall-clock ceiling; frames beyond it count as over budget. Default 98. |
