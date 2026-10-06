# Near-edge update cost profile, October 2026

- **Status:** Complete. A2's update-cost breach comes from the solid body's extent admission, which the shadow runs too; no optimisation is made here.
- **Scope:** CPU profiles and Tracker.Update timing of B0, the shadow and A2 at the October campaign's three timing sites, 300 s scored per run, one invocation per arm.
- **Related:** [October near-edge campaign](near-edge-campaign-2026-10.md), [near-edge tracked-state plan](../../plans/lidar-near-edge-tracked-state-plan.md).

The [October campaign](near-edge-campaign-2026-10.md) found A2's Tracker.Update p99 15 to 35 times
B0's at its three timing sites and kept B0. Its first ranked follow-up asked for a separately
declared profiling build comparing B0, the shadow and A2 before anyone chose an optimisation,
because timing alone does not say which function is expensive. This is that profile.

## Answer

The cost is not near-edge tracking. The shadow, which never feeds the tracked state, costs as much
as A2 at every site. In both, 58 % to 88 % of Tracker.Update's CPU goes to admitting the solid
body's length and width (`admitSolidBodyExtents`), and nearly all of that is sorting.

For every matched solid-body track, on every frame, and for each visible face, `minimumAxisSpan`
tries 21 axes (±10° in 1° steps). At each axis `trimmedSpan` projects every member point and sorts
all of them to read two order statistics, the 1st and 99th percentiles. Under
`solid_body_full_members` the members are the whole cluster, so a large vehicle costs up to 42 full
sorts of its points per frame. Frames with large clusters carry the cost, which is why the p99
rises 20 to 80 times while the p50 at pierce-haight only goes from 0.028 to 0.050 ms.

## Method

| Item     | Value                                                                                                                                                                                                                                            |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Build    | `db5832133`: main `3ee42366c` plus `-cpuprofile-dir` on the corpus tool, built from a clean checkout with the git SHA stamped (`build_version` reads `dev`: the plan's `go build` recipe does not stamp the release version)                     |
| Command  | `lidar-state-estimation-baseline -pcap-subdir s2 -case <site> -duration 300 -warmup 70 -campaign-metrics -cpuprofile-dir <dir>`, captures read from the NAS (`arrow`) share; no evidence database                                                |
| Arms     | B0: no experiment. Shadow: `solid_body`, `solid_body_face_hysteresis`, `solid_body_course_faces`, `solid_body_full_members`. A2: the shadow's set plus `near_edge_track`. These are the campaign's definitions                                   |
| Profile  | Go `pprof` CPU profile of each run's repeat replay, which writes no evidence, so the profile is the pipeline's own cost. Timing is `tracker_timing.json` from `-campaign-metrics`: wall time inside Tracker.Update only, first and repeat replay |
| Machine  | Apple M1 Pro, 16 GB. Runs were sequential per site (B0, shadow, A2) but shared the machine with a coverage-survey replay and test runs: the one-minute load average ranged from 3.6 to 11.1                                                      |
| Coverage | One invocation per arm and site. The scored window is the first 300 s after a 70 s warm-up, not the whole case                                                                                                                                   |

## Tracker.Update timing

Milliseconds, first replay / repeat. The ratio is the arm's mean p99 over B0's.

| Site               | Arm    | Calls | p50           | p99           | p99 ratio to B0 |
| ------------------ | ------ | ----: | ------------- | ------------- | --------------: |
| 3rd-folsom         | B0     | 2,901 | 0.032 / 0.033 | 0.185 / 0.202 |               1 |
| 3rd-folsom         | Shadow | 2,901 | 0.328 / 0.343 | 3.738 / 4.162 |            20.4 |
| 3rd-folsom         | A2     | 2,901 | 0.323 / 0.331 | 3.611 / 3.549 |            18.5 |
| pierce-haight      | B0     | 2,635 | 0.028 / 0.028 | 0.169 / 0.115 |               1 |
| pierce-haight      | Shadow | 2,635 | 0.050 / 0.049 | 4.117 / 3.171 |            25.7 |
| pierce-haight      | A2     | 2,635 | 0.050 / 0.050 | 2.852 / 3.221 |            21.4 |
| embarcadero-bryant | B0     | 2,805 | 0.030 / 0.031 | 0.125 / 0.138 |               1 |
| embarcadero-bryant | Shadow | 2,805 | 0.291 / 0.291 | 11.24 / 10.75 |            83.6 |
| embarcadero-bryant | A2     | 2,805 | 0.270 / 0.273 | 8.585 / 10.19 |            71.4 |

The campaign's balanced ratios, from full cases, four measurements per arm and a balanced order,
were 21.2, 15.7 and 35.4. The direction agrees and the magnitudes do not transfer: these windows
are shorter, each arm ran once, the order was not balanced, and the machine was busy. The shadow's
p99 was not measured in the campaign at all, which is why the attribution could not be read from it.

## Where the CPU goes

CPU seconds in each repeat replay's profile. "Extent admission" is `admitSolidBodyExtents`, and the
sort inside it is almost all of it. "Frame callback" is the tracking pipeline's per-frame work (L3
to L5 and persistence hooks), the part a live sensor would pay.

| Site               | Arm    | Samples | Frame callback | Tracker.Update | Extent admission | Admission share of Update | Update share of callback | L3 background | L4 DBSCAN |
| ------------------ | ------ | ------: | -------------: | -------------: | ---------------: | ------------------------: | -----------------------: | ------------: | --------: |
| 3rd-folsom         | B0     |    98.6 |          15.00 |           0.11 |                0 |                         0 |                    0.7 % |          6.85 |      4.43 |
| 3rd-folsom         | Shadow |    99.7 |          15.93 |           0.84 |             0.66 |                      79 % |                    5.3 % |          6.66 |      4.21 |
| 3rd-folsom         | A2     |    98.1 |          15.94 |           0.82 |             0.58 |                      71 % |                    5.1 % |          6.21 |      4.51 |
| pierce-haight      | B0     |    99.3 |          13.07 |           0.10 |                0 |                         0 |                    0.8 % |          6.49 |      2.43 |
| pierce-haight      | Shadow |    99.1 |          13.51 |           0.65 |             0.38 |                      58 % |                    4.8 % |          5.88 |      2.69 |
| pierce-haight      | A2     |    98.4 |          13.09 |           0.65 |             0.38 |                      58 % |                    5.0 % |          5.84 |      2.51 |
| embarcadero-bryant | B0     |    92.8 |          15.44 |           0.16 |                0 |                         0 |                    1.0 % |          5.09 |      6.15 |
| embarcadero-bryant | Shadow |    93.5 |          19.17 |           2.65 |             2.33 |                      88 % |                   13.8 % |          5.24 |      6.66 |
| embarcadero-bryant | A2     |    94.6 |          18.56 |           2.46 |             2.08 |                      85 % |                   13.3 % |          5.71 |      6.28 |

Inside A2's Tracker.Update at 3rd-folsom, sorting (`slices.pdqsortOrdered` on float64, reached
from `trimmedSpan`) takes 0.51 of the 0.82 s. Near-edge association, face updates and
re-referencing appear only as small fractions of what is left. Outside the tracker, the L3
background model and L4 clustering each cost more than A2's whole Update at two of the three
sites.

Most of each profile's samples sit outside the frame callback: PCAP reading, frame assembly, and
Go runtime scheduling (`pthread_cond_signal`, `usleep` and `pthread_cond_wait` take 47 % of B0's
samples at 3rd-folsom). That is the replay harness's handoff between goroutines. It does not
affect the attribution inside Tracker.Update, and it is a separate question whether the live path
pays the same.

## Interpretation

- The campaign's update-cost breach belongs to the solid body's extent admission as implemented,
  which the shadow and A2 share. It is not evidence against feeding near-edge measurements into
  tracked state.
- The breach is real for any arm that runs the solid body with full members, the shadow included.
  A future shadow-only measurement would breach the same screen.
- In whole-pipeline terms the tracker is small: even A2's Update is 5 % of the per-frame callback
  at two sites and 13 % at the third. What this means on a Raspberry Pi is not measured.

## Recommendation

1. Compute the two trimmed order statistics by selection rather than a full sort. The values are
   the same, so every output stays byte-identical, and the cost falls from O(n log n) to O(n) per
   axis. Verify byte-identity with a shadow and an A2 replay of kirk0, as the S2 gates require.
2. Then re-time B0, the shadow and A2 with the campaign's balanced protocol (alternating order,
   four measurements per arm) on a quiet machine before saying anything about the cost screen.
3. Reducing the 21-axis search (coarse to fine, or rotating calipers on the hull) would cut the cost
   further but changes the spans, so it is a tuning change that needs the corpus screen again.

## Limitations

- One machine (Mac), one invocation per arm, 300 s windows, and a busy machine. Absolute
  milliseconds are not Pi figures and the ratios carry the contention.
- CPU profiles sample at 100 Hz and attribute CPU time, not wall time; Tracker.Update's timing and
  its profile agree on the shape, not the scale.
- Only the repeat replay was profiled, so evidence persistence costs are not included.

## Provenance

| Item           | Value                                                                                                                                                                      |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Build          | git `db5832133582a95463c89afccfc2c1782fc3f038`                                                                                                                             |
| Parameter hash | B0 `fd35b0b28fc1…`, shadow `2faa2a0dd8e6…`, A2 `5c7fd88acaa3…` (`params_sha256`, same at every site)                                                                       |
| Source digests | 3rd-folsom `1d19feceed93…` (4 captures), pierce-haight `c9b3b0f8d588…` (6), embarcadero-bryant `3c55a2292c59…` (5)                                                         |
| Raw outputs    | Profiles, logs and the run script on the LiDAR volume under `velocity-campaign/near-edge-followups-20261005/profile/`; replay outputs on the profiling Mac's internal disk |
