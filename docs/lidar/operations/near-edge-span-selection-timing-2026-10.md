# Span selection and balanced update timing, October 2026

- **Status:** Complete. Selecting a span's percentiles cuts the solid body's Tracker.Update p99 to 30 % to 40 % of the sort's, with byte-identical output; A2 still breaches the campaign's 1.5× cost screen at all three sites.
- **Scope:** B0, the shadow and A2 on main and on the selection build, at the October campaign's three timing sites, balanced order, four p99 measurements per condition, on the Mac.
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
  The cost that remains is the axis search itself: 21 projections of every member per face.

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

## Interpretation

Selecting rather than sorting is a pure gain: the same spans, two to three times faster where the
solid body is slowest. It does not change the campaign's decision. A2 at 12 to 20 times B0's p99
still breaches the observed cost screen, so the screen remains a reason not to promote A2.

What remains is the axis search. For each visible face, `minimumAxisSpan` projects every member
onto 21 axes, and with `solid_body_full_members` that is the whole cluster. Reducing it changes the
spans, so it is a tuning change rather than an implementation detail:

- a coarse-to-fine search (for example 5° steps, then 1° around the best), or
- measuring the span on a capped sample of the members rather than all of them, or
- a smaller search window once the heading has converged.

Any of these needs the corpus screen again, because the minimum over the window is what keeps a
span a lower bound (see the comment on `minimumAxisSpan`).

## Limitations

- One machine, one session, 300 s windows, three sites. Absolute milliseconds are Mac figures,
  not Pi ones, and the timing is Tracker.Update wall time only, not end-to-end latency.
- The machine was not idle: one-minute load stayed near 3 to 4 because of macOS background
  processes, and one burst (load 11, from `logd`) came at the end of a B0 run at embarcadero-bryant. The
  balanced order and medians absorb drift; they do not make the absolute values quiet-machine
  minimums.
- Byte-identity was checked on kirk0 with full replays and on synthetic inputs by test; the three
  timing sites were not compared row by row.

## Provenance

| Item           | Value                                                                                                                                                                        |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Builds         | main `acbe1834ace66be9f5a953aaaf9a405627527fa1`; selection `786bd085187e57372cf702eaa291ee40e2da7ff2`; both stamped                                                          |
| Parameter hash | B0 `fd35b0b28fc1…`, shadow `2faa2a0dd8e6…`, A2 `5c7fd88acaa3…` (`params_sha256`, the same on both builds and at every site)                                                  |
| Source digests | 3rd-folsom `1d19feceed93…` (4 captures), pierce-haight `c9b3b0f8d588…` (6), embarcadero-bryant `3c55a2292c59…` (5)                                                           |
| Raw outputs    | Run script, per-run logs and `timing-summary.json` on the LiDAR volume under `velocity-campaign/quiet-timing-20261005/`; replay outputs on the profiling Mac's internal disk |
