# LiDAR config parameter tuning guide

How to tune the LiDAR processing pipeline's configuration parameters at runtime, covering foreground extraction, clustering, and tracking settings.

## 1. Tuning workflow

1. Start from baseline config ([config/tuning.defaults.json](../../../config/tuning.defaults.json) or your site baseline).
2. Change one parameter group at a time.
3. Validate with live metrics and/or PCAP replay.
4. Persist only parameter sets that improve quality without regressions.

## 2. Core parameter groups

| Group                 | Primary Keys                                                                                    | Typical Effect                                         |
| --------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| Foreground/background | `noise_relative`, `closeness_multiplier`, `neighbor_confirmation_count`, `safety_margin_meters` | Detection sensitivity, false positives, empty-box risk |
| Clustering            | `foreground_dbscan_eps`, `foreground_min_cluster_points`                                        | Fragmentation vs merge risk                            |
| Tracking/association  | `gating_distance_squared`, `measurement_noise`, `process_noise_pos`, `process_noise_vel`        | ID stability, jitter, velocity alignment               |
| Pipeline depth        | `l4.engine`, `l5.engine` set to `none`; `pipeline.frame_budget_ms`                              | Which layers run at all; the per-frame lag threshold   |

Setting a layer's `engine` to `none` switches that layer off, and the layers above
it must be `none` too. That gives three depths — `l3-only`, `detect` and the default
`full`. Tuning the background model is faster and quieter under `l3-only`, which runs
L3 and stops rather than paying for clustering on every frame. Ready-made configs are
in `config/profiles/`. See
[Performance regression testing](performance-regression-testing.md#profiles) for what
each depth runs and costs.

## 3. Apply runtime changes

```bash
curl -s -X POST "http://127.0.0.1:8080/api/lidar/params?sensor_id=hesai-pandar40p" \
  -H "Content-Type: application/json" \
  -d '{
    "noise_relative": 0.015,
    "closeness_multiplier": 2.0,
    "neighbor_confirmation_count": 1,
    "foreground_dbscan_eps": 0.7,
    "gating_distance_squared": 25.0,
    "measurement_noise": 0.15,
    "process_noise_vel": 0.3
  }' | jq .
```

Read back:

```bash
curl -s "http://127.0.0.1:8080/api/lidar/params?sensor_id=hesai-pandar40p" | jq .
```

To replay a capture on a whole parameter set rather than a patch, name a
file under the config directory when the replay starts. The server applies
it before the replay and echoes its fingerprint:

```bash
curl -s -X POST "http://127.0.0.1:8080/api/lidar/pcap/start?sensor_id=hesai-pandar40p" \
  -H "Content-Type: application/json" \
  -d '{"pcap_file": "kirk0.pcapng", "tuning_config": "experiments/geometry-convergence-candidate.json"}' | jq .
```

The replay form on the LiDAR Status page (`/app/lidar/status`) offers the same files in its Tuning
Config dropdown.

## 4. Validate and compare

1. Run a representative live session or replay golden PCAP.
2. Check jitter, fragmentation, misalignment, and empty-box behaviour.
3. Keep each parameter set you compare as a file under `config/experiments/`
   with a row naming what it changes, and record its fingerprint with the
   outcome. See
   [Experiment parameter sets](../../../config/CONFIG.md#experiment-parameter-sets).

For structured sweeps, use the Run a Sweep page (`/app/lidar/sweeps/run`) or
`velocity tune sweep`; see the [sweep tool guide](sweep-tool.md). To inspect the
background grid after a replay, open the LiDAR Debug page (`/app/lidar/debug`),
which shows the grid SVG from `GET /api/charts/grid`.

## 5. Rollback

If quality regresses, revert quickly:

```bash
curl -s -X POST "http://127.0.0.1:8080/api/lidar/params?sensor_id=hesai-pandar40p" \
  -H "Content-Type: application/json" \
  -d @config/tuning.defaults.json | jq .
```

## 6. Deep references

For a guided progression through all tuning tools, start with the
[Tuning Guide](tuning-guide.md).

- [Parameter Comparison](parameter-comparison.md): side-by-side evaluation of parameter sets
- [Pipeline Diagnosis](../troubleshooting/pipeline-diagnosis.md): root cause analysis and quick fixes
- [Sweep Tool](sweep-tool.md): automated grid search
- [Auto-Tuning](auto-tuning.md): multi-round iterative optimisation
- [HINT Mode](hint-sweep-mode.md): human-in-the-loop tuning
- [Performance regression testing](performance-regression-testing.md): pipeline profiles, the frame budget, and what makes two benchmark runs comparable
- [MATHS.md](../../../data/maths/MATHS.md) §Config Mapping: complete key-to-source mapping for algorithm researchers
