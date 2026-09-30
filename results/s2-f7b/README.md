# S2 F7b: the corrected T5 on the tuning pair, then the screen

F7b runs test F7 of the S2 near-edge plan from the corrected T5 build (the rank-one medoid). It
supersedes the earlier F7 run, which built an older T5.

## Host

- hydra (Apple M1 Pro, 16 GB, macOS 15.7.7), on AC power, go1.26.5 darwin/arm64.
- Captures are read in place from the NAS (`/Volumes/banshee-captures/lidar`, `-pcap-subdir s2`).
- Kept outputs go to the USB lidar drive (`/Volumes/lidar/evidence/s2-f7b`). Evidence databases go
  to the internal disk and are deleted after each invocation.
- The F6s screen run (one `baseline` process) ran alongside, so wall times are not clean.

## Source

- Branch `claude/upbeat-galileo-4xbaat-s2-t5` (PR #649) at `4e57ede7`: "S2 T5: act only at
  end-face fixes with no side face found".
- Binary: `go build -tags=pcap ./cmd/tools/lidar-state-estimation-baseline`, with `GitSHA` set to
  that commit.

## Arms

`BASE = solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members`

| Arm              | Experiments                                             |
| ---------------- | ------------------------------------------------------- |
| `track_t5`       | `BASE,near_edge_track,solid_body_rank_one_medoid`       |
| `track_t5_tight` | `BASE,near_edge_track,solid_body_rank_one_medoid_tight` |
| `track`          | `BASE,near_edge_track`                                  |
| `shadow_t5`      | `BASE,solid_body_rank_one_medoid`                       |
| `control`        | `BASE`                                                  |

Each arm runs once on the tuning pair (`marina-webster-beach,columbus-broadway`) with
`-sample-points 256 -evidence-per-case -discard-evidence`.

## Rule for the screen arm

A T5 arm qualifies if its steady p99 (`solid_body.anchor_solid_bodies_steady_runs.p99_m`) is
below `track`'s on both tuning cases, and its columbus-broadway lapses are no more than 1.05
times `track`'s. Of the qualifying arms, the one with the lower mean steady p99 over the two cases
runs on the 21 screen sites, one site per invocation. If neither qualifies, no screen arm runs.

## Files

- `<arm>/all/phase0-summary.json` and `<arm>/all/run.txt` (the last 200 lines of the run log).
- `progress.md`: the tuning-pair table, regenerated after each arm.
- `screen_<arm>/<case>/…` and `screen-progress.md`, if a screen arm runs.
