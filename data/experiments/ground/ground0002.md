---
id: ground0002
title: "The vertical clip follows the ground: the surface clip against the height band, on jitter, the labelled score and cost"
status: proposed
owner: david
created: 2026-10-18
asks:
  - docs/BACKLOG.md
  - docs/plans/lidar-state-estimation-plan.md#21-p11-is-a-correctness-problem-not-a-future-refinement
  - data/maths/paper-implementation-gap-analysis.md#8-patchwork-lim-et-al-2022
  - data/experiments/try/l5-tracking-noise-parameter-sweep.md
report: docs/lidar/operations/ground-surface-clip-2026-10.md
requires:
  capabilities: [pcap:kirk, pcap:s2, labels:kirk, pcap-build]
  disk_gb: 40
arms:
  - name: a0
    tuning: config/tuning.defaults.json
  - name: floor-road
    tuning: config/experiments/height-floor-road.json
  - name: surface
    tuning: config/tuning.defaults.json
    replay: [surface_ground]
  - name: surface-global
    tuning: config/tuning.defaults.json
    replay: [surface_ground_global]
  - name: surface-5m
    tuning: config/tuning.defaults.json
    replay: [surface_ground_5m]
cases:
  corpus: d2-first-segment
  count: 23
steps:
  - id: implement
    kind: manual:implement
    estimate: { human: 3h }
    note: >-
      The replay-only P11 clip as an l4.dbscan_xy_v1.surface_ground block (enabled, floor and
      ceiling above the surface, cell metres), off by default so the fingerprint stands, behind
      the live path's ground removal as well as replay's; the replay names surface_ground (10 m
      cells, floor 0.2 m, ceiling 4.5 m), surface_ground_global (one plane for the capture) and
      surface_ground_5m mapped onto it in CONFIG.md; and the tracker's heading jitter, speed
      jitter and course alignment carried into the baseline summary, which has none of them.
  - id: replay
    kind: state_estimation_baseline
    over: [arms, cases]
    estimate: { each: 3m, worker: m1pro }
    note: marina-webster-beach and columbus-broadway first, the graded sites; the rest after
  - id: kirk
    kind: state_estimation_baseline
    over: [arms]
    estimate: { each: 3m, worker: m1pro }
    note: kirk0, for the per-frame evaluator and the physical references
  - id: score
    kind: ground_truth_eval
    over: [arms]
    estimate: { each: 2m, worker: m1pro }
    params: { split_role: tuning, physical_reference: true }
  - id: bench
    kind: benchmark
    over: [arms]
    estimate: { each: 2m, worker: m1pro }
    params: { profile: full, repeats: 5 }
    note: the clip's cost on kirk0; the Pi timing is owed, as perf0001's
  - id: write
    kind: manual:write
    estimate: { human: 3h }
---

# ground0002: the vertical clip follows the ground

**Question.** L4 removes ground with a band on absolute sensor Z, floor −2.8 m and ceiling
+1.5 m, chosen for a 3 m mount. At the deployed 2.3 m the floor is 0.5 m below the road, so
ground returns are admitted on level sites; on a grade the band sits at a different height above
the road at every range, clipping the lower body where the road falls away and admitting ground
where it rises (the state plan's P11). Each changes which returns a cluster has, so its centre,
extents and heading move with the body's position: a jitter source the tracker then smooths. The
P11 remedy, a clip by height above a surface fitted from the settled background, has existed in
replay since September and has never been screened. Does it give steadier tracks, and does it
lose anything the labels can see?

**Arms.** `a0` is the band as shipped. `floor-road` is the band with its floor 0.3 m above the
road (−2.0 m), the control that separates "the floor was too low" from "the floor should follow
the road". `surface` is the regional clip, 10 m cells with a global fallback, 0.2 m above the
surface to 4.5 m. `surface-global` is one plane for the whole capture: the control for the cell
model, the mesh in waiting. `surface-5m` is the cell model at half the cell, for the trend.

**Measures.**

- _Steadiness_, label-free, per site and pooled: the anchor-point lateral p99 over scored
  windows (`five_point_time_xy_fit_v1`, the state plan's G-GEO-1 measure); speed jitter (RMS
  frame-to-frame change of the Kalman speed) and heading jitter (RMS raw-to-smoothed OBB
  innovation) from the tracker's metrics, pooled over confirmed tracks; the course alignment p50
  and p90.
- _The labels_, on kirk0's tuning split: matches, HOTA, IDF1, identity switches and
  fragmentations from the per-frame evaluator; the reviewed centre, heading and extent errors
  against the 22 physical references.
- _The vehicles_, label-free on the corpus: confirmed vehicle-sized tracks and observed vehicle
  frames per site.
- _Cost_: the full profile's wall time on kirk0 against `a0`.

**Rule, written before any replay (2026-10-18).** Only the three sites' gradients and `a0`'s
shipped behaviour were known. A surface arm passes on the tuning set when:

1. its target improves: on marina-webster-beach and columbus-broadway the anchor-point lateral
   p99 falls by a tenth or more against `a0` and the speed jitter falls; on the sites ground0001
   puts under 0.5 % grade neither rises by more than 5 %, and the heading jitter rises nowhere
   by more than 5 %;
2. it loses nothing on kirk0's tuning split: matches at least `a0`'s × 0.98, HOTA and IDF1 each
   no lower than `a0`'s less 0.005, identity switches no more than `a0`'s, fragmentations no
   more than `a0`'s × 1.1 + 1, and the reviewed centre error's median no higher than `a0`'s plus
   0.05 m;
3. it keeps the vehicles: confirmed vehicle-sized tracks and observed vehicle frames at least
   `a0`'s × 0.98 at every site (a correct floor removes ground that merged into bodies, so
   extents may fall; tracks may not);
4. its cost is inside the gate: full-profile wall time no more than 1.05 × `a0`'s.

`floor-road` is read against the same clauses: where it passes and the surface arms add nothing
to it, the floor was the defect and the surface is not needed on these sites. Between the surface
arms, `surface` or `surface-5m` must beat `surface-global` on clause 1 at a site before the cell
model is credited there; where neither does, one plane is enough for the clip and the mesh
question closes for it, whatever ground0001's departure table predicted. A candidate that passes
is promising, not adopted: the default needs the second segments read for the first time, the
perf baselines recaptured for the moved fingerprint, and the Pi's frame time, as the backlog's
"slope-aware ground surface as the default" item says.

**What could mislead.** The lateral fit is non-causal and rewards smoothing (state plan §0.2),
which is why clause 2 stands beside clause 1. The E1 record found the lateral artefact as present
at Embarcadero (0.14 %) as at Marina (4.35 %), so the clip is not expected to remove it, only the
grade's share of it; a surface arm that moves the lateral p99 on the level sites has moved
something else. kirk0 is close to level (the background puts the road at −2.2 to −2.3 m within
15 m), so its labelled score guards against loss and does not measure the gain. The physical
references were fitted to reviewed returns before any clip and stay as they are. The surface
ceiling at 4.5 m is 0.7 m higher above the road than the band's at the deployed mount, so a
change in tall vehicles' extents between `a0` and the surface arms may be the ceiling's;
`floor-road`, which keeps the band's ceiling, separates it.

**What it does not establish.** Anything held out: the second segments and embarcadero-folsom
are unread. The ground's truth: the surface is the sensor's own returns. A live refit when the
settled background changes, which the implement step leaves to the backlog item.
