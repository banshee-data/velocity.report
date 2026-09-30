# S2 F6s: near-edge tracked update on the screen sites

Test F6s of [the near-edge tracked-state plan](../../docs/plans/lidar-near-edge-tracked-state-plan.md):
the identity screen (gate 3) on the 21 screen sites, which are never tuned on. It repeats F6's two
arms (F6 ran them on the two tuning sites; results on branch
`claude/upbeat-galileo-4xbaat-s2-f6-results`). A sensor coverage survey of all 24 corpus sites
follows the screen run.

- Host: hydra (Apple M1 Pro), unattended run.
- Source commit: `122535f8` (origin/main; contains `0b540677`).
- Tool: `cmd/tools/lidar-state-estimation-baseline`, built with `-tags=pcap` and the commit stamped.
- Captures: read in place from the NAS archive, `-pcap-subdir s2`, `-sample-points 256`.

## Arms

`BASE = solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members`

| Arm     | `-experiment`          |
| ------- | ---------------------- |
| control | `BASE` (T1+T3 shadow)  |
| track   | `BASE,near_edge_track` |

## Cases (run order)

ashbury-downey, haight-divisadero, pierce-haight, fulton-divisadero, california-leon-baker,
broadway-gough, laguna-eddy, lombard-broderick, marina-broderick, lombard-laguna,
columbus-north-point, embarcadero-bay, van-ness-sacramento, embarcadero-broadway,
franklin-mcallister, hyde-ofarrell, bush-powell, 1st-mission, embarcadero-bryant, 3rd-folsom,
howard-6th.

Survey (no `-experiment`, `-sample-points 1`): marina-webster-beach, columbus-broadway,
embarcadero-folsom, then the 21 above.

## Layout

- `<arm>/<case>/phase0-summary.json`: the tool's summary.
- `<arm>/<case>/run.txt`: last 200 lines of the run log.
- `progress.md`: table regenerated after each case.
- `survey/continuity-coverage.json` and `.survey.json`: the survey declaration set so far.

Per-case evidence databases were written to the internal disk and discarded
(`-discard-evidence`) once each summary was written; they are not kept.
