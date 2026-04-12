# Schema simplification — migrations 000030 + 000031

- **Status:** Complete — migrations 000030 and 000031 written, Go code, web frontend, and tests updated in PR #400.

## What changed

Two coordinated migrations standardise the LiDAR schema before v0.5.0:

### Migration 000030 — column cleanup

- **Dropped** `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` from
  `lidar_tracks` and `lidar_run_tracks` (dead/wrong-abstraction columns).
- **Renamed** `peak_speed_mps` → `max_speed_mps` on both tables.
- **Renamed** `world_frame` → `frame_id` on `lidar_clusters`,
  `lidar_tracks`, `lidar_track_obs`.
- **Renamed** `scene_hash` → `grid_hash` on `lidar_bg_regions`.

### Migration 000031 — table naming standardisation

7 tables renamed into coherent family groups:

| Old Name               | New Name                   | Family |
| ---------------------- | -------------------------- | ------ |
| `lidar_track_obs`      | `lidar_track_observations` | Track  |
| `lidar_labels`         | `lidar_track_annotations`  | Track  |
| `lidar_analysis_runs`  | `lidar_run_records`        | Run    |
| `lidar_missed_regions` | `lidar_run_missed_regions` | Run    |
| `lidar_scenes`         | `lidar_replay_cases`       | Replay |
| `lidar_evaluations`    | `lidar_replay_evaluations` | Replay |
| `lidar_sweeps`         | `lidar_tuning_sweeps`      | Tuning |

Column `scene_id` → `replay_case_id` on `lidar_replay_cases`,
`lidar_replay_evaluations`, and `lidar_track_annotations`.

## Post-Migration schema (target)

```
lidar_bg_regions          lidar_run_records
lidar_bg_snapshot         lidar_run_tracks
                          lidar_run_missed_regions
lidar_clusters
                          lidar_replay_cases
lidar_tracks              lidar_replay_evaluations
lidar_track_observations
lidar_track_annotations   lidar_tuning_sweeps
```

## Design rules

1. Full words by default; `bg` is an allowed entrenched short form.
2. Group tables by conceptual owner: `bg_*`, `track_*`, `run_*`,
   `replay_*`, `tuning_*`.
3. Reserve `scene` for future L7 canonical scene work.
4. Prefer plural entity names for tables.
5. Keep already-good anchor names (`lidar_tracks`, `lidar_clusters`,
   `lidar_run_tracks`) to avoid unnecessary FK churn.

## Tables kept unchanged

`lidar_bg_regions`, `lidar_bg_snapshot`, `lidar_tracks`, `lidar_clusters`,
`lidar_run_tracks` — already clear and well-established.

## Quality columns (not dropped)

The 6 quality columns on `lidar_tracks` (`track_length_meters`,
`track_duration_secs`, `occlusion_count`, `max_occlusion_frames`,
`spatial_coverage`, `noise_point_ratio`) exist in the schema but are not
yet written. Wiring them to `InsertTrack()`/`UpdateTrack()` is tracked
separately. They must **not** be dropped.

## Non-Goals

- Dropping `height_p95`/`height_p95_max` (spatial filters, not population
  stats).
- Touching `lidar_track_obs` beyond the `world_frame` rename.
- Radar table names (LiDAR-only plan).
- Merging live and analysis track tables (separate plan).

## v0.5.x Follow-Through

> **Wire-by and wire-or-delete checklists:** tracked in the v0.5.x follow-through backlog.
