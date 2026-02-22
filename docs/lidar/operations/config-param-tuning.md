# LiDAR Config Parameter Tuning Guide

## 1. Tuning Workflow

1. Start from baseline config (`config/tuning.defaults.json` or your site baseline).
2. Change one parameter group at a time.
3. Validate with live metrics and/or PCAP replay.
4. Persist only parameter sets that improve quality without regressions.

## 2. Core Parameter Groups

| Group                 | Primary Keys                                                                                    | Typical Effect                                         |
| --------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| Foreground/background | `noise_relative`, `closeness_multiplier`, `neighbor_confirmation_count`, `safety_margin_meters` | Detection sensitivity, false positives, empty-box risk |
| Clustering            | `foreground_dbscan_eps`, `foreground_min_cluster_points`                                        | Fragmentation vs merge risk                            |
| Tracking/association  | `gating_distance_squared`, `measurement_noise`, `process_noise_pos`, `process_noise_vel`        | ID stability, jitter, velocity alignment               |

## 3. Apply Runtime Changes

```bash
curl -s -X POST "http://127.0.0.1:8081/api/lidar/params?sensor_id=hesai-pandar40p" \
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
curl -s "http://127.0.0.1:8081/api/lidar/params?sensor_id=hesai-pandar40p" | jq .
```

## 4. Validate and Compare

1. Run a representative live session or replay golden PCAP.
2. Check jitter, fragmentation, misalignment, and empty-box behavior.
3. Keep a changelog per parameter set and outcome.

For structured sweeps:

```bash
make stats-pcap PCAP=/absolute/path/to/capture.pcap
```

## 5. Rollback

If quality regresses, revert quickly:

```bash
curl -s -X POST "http://127.0.0.1:8081/api/lidar/params?sensor_id=hesai-pandar40p" \
  -H "Content-Type: application/json" \
  -d @config/tuning.defaults.json | jq .
```

## 6. Deep References

- `docs/lidar/operations/parameter-comparison.md`
- `docs/lidar/operations/quickstart-pipeline-fix.md`
- `docs/lidar/operations/sweep-tool.md`
- `docs/lidar/operations/auto-tuning.md`
- `docs/lidar/troubleshooting/pipeline-diagnosis.md`
