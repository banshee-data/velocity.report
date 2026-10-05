# Span selection and balanced update timing, October 2026

- **Status:** Complete. Selecting a span's percentiles cuts the solid body's Tracker.Update p99 to 30 % to 40 % of the sort's, and selecting the near-edge percentile takes a further 8 % to 28 % off A2's (10 % off the shadow's), both with byte-identical output. A2 still breaches the campaign's 1.5× cost screen at every site, and would even with a free span search.
- **Scope:** B0, the shadow and A2 on main and on the selection build, at the October campaign's three timing sites, balanced order, four p99 measurements per condition, on the Mac. A CPU profile of B0 and A2 after selection at 3rd-folsom, and the near-edge percentile's selection timed at the same three sites.
- **Related:** [October near-edge campaign](near-edge-campaign-2026-10.md), [near-edge tracked-state plan](../../plans/lidar-near-edge-tracked-state-plan.md).

The October campaign kept B0 partly because A2's Tracker.Update p99 was 15 to 35 times B0's. A
profile of the same sites then put most of that cost, for the shadow and A2 alike, in the solid
body's extent admission: `minimumAxisSpan` tries 21 axes per visible face and `trimmedSpan` sorted
every member's projection at each, only to read the 1st and 99th percentiles. This change selects
those two values instead of sorting, and this record measures what that buys, against main, under
the campaign's balanced protocol.

## Answer

- **Byte-identical.** On kirk0 the shadow and A2 replays of both builds wrote identical tracking
  baselines (first and repeat), identical solid-body rows (all 1,951 and 1,924, every column but
  the random IDs and insert time) and identical summaries. A unit test compares every span with
  the sort's, bit for bit, on spread, tied, ordered, reversed, constant and NaN/Inf/signed-zero
  inputs.
- **Faster.** The solid body's p99 falls to 30 % to 40 % of main's at every site, for the shadow
  and A2 alike: 2.5 to 3.3 times faster where it is slowest. The p50 falls to 55 % to 60 % where
  the solid body runs on most frames.
- **Not enough for the screen.** A2's p99 is still 12 to 20 times B0's, against the campaign's 1.5.
  A profile after selection puts under half of A2's update in the span search; even without it,
  A2's update would cost three times B0's.
- **A second sort, also gone.** The near-edge measurement sorted every projection to read one
  percentile. Selecting it instead keeps the output byte-identical, takes 10 % off the shadow's
  p99 and 8 % to 28 % off A2's, and leaves A2 at 8 to 19 times B0's.

## Method

| Item      | Value                                                                                                                                                                                                                                       |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Builds    | main `acbe1834a` and the selection build `786bd0851` (main plus this change), each built from a clean checkout with its git SHA stamped; the parameter hash of each arm is the same on both builds                                          |
| Arms      | B0: no experiment. Shadow: `solid_body`, `solid_body_face_hysteresis`, `solid_body_course_faces`, `solid_body_full_members`. A2: the shadow's set plus `near_edge_track`. The campaign's definitions                                        |
| Runs      | 300 s scored after a 70 s warm-up, `-campaign-metrics`, no evidence database, captures read from the NAS. Tracker.Update wall time from `tracker_timing.json`, first replay and repeat                                                      |
| Order     | Per site, the six conditions ran in one order (B0 main, shadow selection, A2 main, A2 selection, shadow main, B0 selection) and then in reverse, so a linear drift in background load cancels. Two passes give four p99 values each         |
| Machine   | Apple M1 Pro, 16 GB, on AC power, under `caffeinate`. Nothing else of this work ran during the window. Each run waited up to 3 minutes for a one-minute load under 3.5; idle CPU at the end of a run ranged from 73 % to 89 % (median 82 %) |
| Statistic | The median of each condition's four p99 values, as the campaign's balanced timing used, and of its four p50 values. Ratios are to B0 pooled across both builds (eight values), since B0's code is the same on both                          |

B0 does not run the changed code, so its two builds should time alike. They differ by up to 25 %
at p99 (pierce-haight 0.085 ms on main, 0.106 ms on the selection build), which is the noise floor
of a 0.1 ms measurement on this machine; the pooled B0 is the fairer denominator.

## Findings

Milliseconds, medians of four. The range is the four p99 values' smallest and largest.

| Site               | Arm    | Build     |   p50 |   p99 | p99 range      | p99 to B0 | Selection to main |
| ------------------ | ------ | --------- | ----: | ----: | -------------- | --------: | ----------------: |
| 3rd-folsom         | B0     | main      | 0.031 | 0.118 | 0.097 to 0.125 |         1 |                   |
| 3rd-folsom         | B0     | selection | 0.032 | 0.112 | 0.090 to 0.121 |         1 |                   |
| 3rd-folsom         | Shadow | main      | 0.325 | 3.663 | 3.595 to 3.879 |      32.0 |                   |
| 3rd-folsom         | Shadow | selection | 0.184 | 1.377 | 1.340 to 1.453 |      12.0 |              0.38 |
| 3rd-folsom         | A2     | main      | 0.317 | 3.453 | 3.408 to 3.568 |      30.2 |                   |
| 3rd-folsom         | A2     | selection | 0.182 | 1.362 | 1.345 to 1.385 |      11.9 |              0.39 |
| pierce-haight      | B0     | main      | 0.028 | 0.085 | 0.079 to 0.091 |         1 |                   |
| pierce-haight      | B0     | selection | 0.028 | 0.106 | 0.087 to 0.131 |         1 |                   |
| pierce-haight      | Shadow | main      | 0.047 | 3.131 | 3.051 to 3.767 |      35.2 |                   |
| pierce-haight      | Shadow | selection | 0.048 | 1.154 | 1.041 to 1.196 |      13.0 |              0.37 |
| pierce-haight      | A2     | main      | 0.050 | 2.931 | 2.662 to 3.156 |      33.0 |                   |
| pierce-haight      | A2     | selection | 0.050 | 1.169 | 1.054 to 1.235 |      13.2 |              0.40 |
| embarcadero-bryant | B0     | main      | 0.031 | 0.129 | 0.114 to 0.142 |         1 |                   |
| embarcadero-bryant | B0     | selection | 0.031 | 0.149 | 0.125 to 0.154 |         1 |                   |
| embarcadero-bryant | Shadow | main      | 0.289 | 10.41 | 10.05 to 10.59 |      74.8 |                   |
| embarcadero-bryant | Shadow | selection | 0.170 | 3.084 | 2.808 to 3.115 |      22.2 |              0.30 |
| embarcadero-bryant | A2     | main      | 0.274 | 8.791 | 8.673 to 9.160 |      63.2 |                   |
| embarcadero-bryant | A2     | selection | 0.162 | 2.712 | 2.640 to 2.922 |      19.5 |              0.31 |

Each condition timed the same Tracker.Update calls (2,901, 2,635 and 2,805 per replay at the three
sites). The four p99 values of a solid-body condition lie within about 10 % of each other, so the
selection's gain is far outside the run-to-run spread.

- The shadow and A2 move together, as the profile predicted: the cost was the shared extent
  admission, not near-edge tracking.
- The gain is largest where the cost was: embarcadero-bryant, whose large vehicles gave the
  longest sorts, falls from 10.4 to 3.1 ms for the shadow.
- At pierce-haight the p50 is nearly B0's on both builds (1.7 to 1.8 times): the solid body's cost
  there is concentrated in a few frames with large clusters, and those set the p99.
- Against the campaign's balanced ratios on full cases (21.2, 15.7 and 35.4 times B0 for A2 on
  main), these 300 s windows give 30, 33 and 63. The windows differ and so does B0's tiny
  denominator, so the main-build ratios here are this session's own baseline, not a correction of
  the campaign's.

## Where A2's update goes after selection

One CPU profile each of B0 and A2 on the selection build (`28d713f2e`), at 3rd-folsom, 300 s scored, with the
captures read from the internal SSD; the profile is of the repeat replay. Profiles sample at
100 Hz, so A2's Tracker.Update is 33 samples: a coarse split, not a two-figure measurement.

| Measure                                                        |    B0 |    A2 |
| -------------------------------------------------------------- | ----: | ----: |
| Tracker.Update p50 (ms)                                        | 0.032 | 0.181 |
| Tracker.Update p99 (ms)                                        | 0.148 | 1.335 |
| Tracker.Update CPU (s)                                         |  0.06 |  0.33 |
| of which extent admission, the span search                     |       |  0.15 |
| of which the near-edge measurement in association's gate       |       |  0.06 |
| of which sorting inside that measurement (`LateralPercentile`) |       |  0.05 |
| Frame callback CPU (s)                                         | 13.14 | 13.03 |

- **Extent admission is now under half of A2's update.** Were it free, A2's update would still
  cost about 0.18 s, three times B0's whole update.
- **The near-edge measurement costs as much as B0's whole update.** A2's association measures the
  near edge for every candidate pair it gates, and most of that was another sort:
  `LateralPercentile` sorted every member's projection to read the 5th percentile.
- **At the frame, A2's update is small.** It is 2.5 % of the frame callback's CPU, against B0's
  0.5 %. The frame callback cost 13.14 s under B0 and 13.03 s under A2, one run each, so the 2 %
  that A2's update adds is smaller than what two single runs can tell apart.

So the span search was not all that remained: no change to it alone can bring A2 within the
screen's 1.5 times B0's Tracker.Update p99. B0's update is so small a part of the frame that a
ratio to it is a strict test.

## Selecting the near-edge percentile

`LateralPercentile` now selects its one order statistic with the same quickselect, which moved to
`l4perception` as `NthFloat64` so both callers share it (`c33ed5d8a`). The near-edge offset is
the sort's: a test compares every percentile with the sort's on the same input shapes as the span
test, and bit for bit on cluster-like points. The only value the two could order differently is
the sign of an exact zero, which no sort promises. Selection is 2.8 times faster than the sort at
256 points, 3.3 times at 1,024 and 5 times at 4,096. kirk0 replays of the shadow and A2 on
`28d713f2e` and `c33ed5d8a` wrote identical tracking baselines (first and repeat) and identical
observation, estimate and solid-body rows (2,423, 1,951 and 1,924; every column but random IDs
and insert time).

Timing used the same protocol as above, with the captures read from the internal SSD: per site,
B0, the shadow on both builds and A2 on both builds, in one order and then reversed, four p99
values per condition. Runs waited for the one-minute load to stay under 3.5 for a minute, and a
run that ended with the load at 8 or more was set aside and repeated.

Milliseconds, medians of four. "Spans" is the span selection alone (`28d713f2e`); "both" adds the
near-edge percentile (`c33ed5d8a`). B0 ran on the second build only, as it runs neither change.

| Site               | Arm    | Build |   p50 |   p99 | p99 range      | p99 to B0 | Both to spans |
| ------------------ | ------ | ----- | ----: | ----: | -------------- | --------: | ------------: |
| 3rd-folsom         | B0     | both  | 0.031 | 0.113 | 0.101 to 0.116 |         1 |               |
| 3rd-folsom         | Shadow | spans | 0.182 | 1.377 | 1.329 to 1.418 |      12.1 |               |
| 3rd-folsom         | Shadow | both  | 0.168 | 1.244 | 1.213 to 1.269 |      11.0 |          0.90 |
| 3rd-folsom         | A2     | spans | 0.180 | 1.341 | 1.336 to 1.361 |      11.8 |               |
| 3rd-folsom         | A2     | both  | 0.166 | 1.178 | 1.144 to 1.210 |      10.4 |          0.88 |
| embarcadero-bryant | B0     | both  | 0.031 | 0.132 | 0.121 to 0.147 |         1 |               |
| embarcadero-bryant | Shadow | spans | 0.167 | 2.968 | 2.932 to 3.206 |      22.5 |               |
| embarcadero-bryant | Shadow | both  | 0.155 | 2.661 | 2.637 to 2.687 |      20.2 |          0.90 |
| embarcadero-bryant | A2     | spans | 0.161 | 2.651 | 2.622 to 2.895 |      20.1 |               |
| embarcadero-bryant | A2     | both  | 0.143 | 2.436 | 2.419 to 2.466 |      18.5 |          0.92 |
| pierce-haight      | B0     | both  | 0.028 | 0.107 | 0.102 to 0.114 |         1 |               |
| pierce-haight      | Shadow | spans | 0.048 | 1.138 | 1.086 to 1.185 |      10.7 |               |
| pierce-haight      | Shadow | both  | 0.044 | 1.024 | 0.963 to 1.092 |       9.6 |          0.90 |
| pierce-haight      | A2     | spans | 0.050 | 1.200 | 1.169 to 1.256 |      11.3 |               |
| pierce-haight      | A2     | both  | 0.045 | 0.858 | 0.791 to 0.961 |       8.1 |          0.72 |

- **A tenth off the p99, more where few frames set it.** Selecting the percentile cuts the
  shadow's p99 by 10 % at every site and A2's by 12 % and 8 % at 3rd-folsom and
  embarcadero-bryant, which matches the profile's sixth of A2's update CPU. At pierce-haight A2's
  p99 falls by 28 %: there the solid body's cost sits in a few frames with large clusters, and
  A2's gate measures the near edge for every candidate pair in them. The p50 falls by 7 % to 11 %
  everywhere. The four values of each condition do not overlap the other build's.
- **The screen still fails.** A2 is 10.4, 18.5 and 8.1 times B0's p99 at the three sites.
- **The two sessions agree.** The span-selection build reproduced its own earlier p99 within 4 %
  at every site (at 3rd-folsom 1.377 against 1.377 for the shadow, 1.341 against 1.362 for A2),
  though the earlier session read its captures from the NAS.
- Two runs were disturbed and repeated. During an A2 run on the span-selection build at
  embarcadero-bryant a desktop application started and the one-minute load reached 36: its repeat
  replay's p99 was 4.28 ms where its first replay's was 2.60. A B0 run at pierce-haight ended at
  load 9.5. Both were set aside and their conditions run again once the load had settled.

## Interpretation

Selecting rather than sorting is a pure gain: the same spans, two to three times faster where the
solid body is slowest. It does not change the campaign's decision. A2 at 12 to 20 times B0's p99
still breaches the observed cost screen, so the screen remains a reason not to promote A2.

What remains is in three parts:

- **The near-edge measurement in association.** Its sort is now a selection too (see
  [Selecting the near-edge percentile](#selecting-the-near-edge-percentile)), which took 8 % to
  28 % off A2's p99; what is left is one measurement per gated pair, which is A2's design.
- **The span search.** For each visible face, `minimumAxisSpan` projects every member onto 21
  axes, and with `solid_body_full_members` that is the whole cluster. Reducing it changes the
  spans, so it is a tuning change: a coarse-to-fine search (for example 5° steps, then 1° around
  the best), a capped sample of the members, or a smaller window once the heading has converged.
  Any of these needs the corpus screen again, because the minimum over the window is what keeps a
  span a lower bound (see the comment on `minimumAxisSpan`).
- **The screen itself.** It compares Tracker.Update alone, where B0 spends 0.5 % of the frame.
  Whether A2 should instead be held to the frame's cost, of which its update is about 2.5 %, is a
  decision for the campaign, not for this record.

## Limitations

- One machine, two sessions, 300 s windows, three sites. Absolute milliseconds are Mac figures,
  not Pi ones, and the timing is Tracker.Update wall time only, not end-to-end latency.
- The machine was not idle: one-minute load stayed near 3 to 4 because of macOS background
  processes, and one burst (load 11, from `logd`) came at the end of a B0 run at embarcadero-bryant. The
  balanced order and medians absorb drift; they do not make the absolute values quiet-machine
  minimums.
- Byte-identity was checked on kirk0 with full replays and on synthetic inputs by test; the three
  timing sites were not compared row by row.
- The profile after selection is one run per arm at one site, and A2's update is 33 of its
  samples.

## Provenance

| Item           | Value                                                                                                                                                                                                           |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Builds         | main `acbe1834ace66be9f5a953aaaf9a405627527fa1`; selection `786bd085187e57372cf702eaa291ee40e2da7ff2` (the selection change on that main, later rebased onto #676 unchanged); both stamped                      |
| Parameter hash | B0 `fd35b0b28fc1…`, shadow `2faa2a0dd8e6…`, A2 `5c7fd88acaa3…` (`params_sha256`, the same on both builds and at every site)                                                                                     |
| Source digests | 3rd-folsom `1d19feceed93…` (4 captures), pierce-haight `c9b3b0f8d588…` (6), embarcadero-bryant `3c55a2292c59…` (5)                                                                                              |
| Raw outputs    | Run script, per-run logs and `timing-summary.json` on the LiDAR volume under `velocity-campaign/quiet-timing-20261005/`, and the replay outputs under its `local-full/`                                         |
| Second session | Span selection at `28d713f2efe3dbf92eb9464533e94e029e369469` and with the near-edge percentile at `c33ed5d8acddaa7d57cbae900258f19bddeb904a`, both stamped; captures from the internal SSD, digests as above    |
| Its outputs    | Profiles, kirk0 byte-identity replays, timing runs, `timing-summary.json` and the set-aside runs under `velocity-campaign/a2-profile-20261005/` on the LiDAR volume, pierce-haight's under its `pierce-haight/` |
