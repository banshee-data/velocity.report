---
id: ground0004
title: "The published box on the ground: its base from the surface and its height from a held belief, not the frame's cluster"
status: proposed
owner: david
created: 2026-10-18
asks:
  - docs/plans/lidar-state-estimation-plan.md#54-what-is-in-the-state-and-what-is-not
  - docs/lidar/operations/published-box-refinement-2026-10.md#recommendations
  - docs/BACKLOG.md
depends_on: [ground0002]
report: docs/lidar/operations/published-box-ground-2026-10.md
requires:
  capabilities: [pcap:kirk, pcap:s2, labels:kirk, pcap-build]
  disk_gb: 20
arms:
  - name: a0
    tuning: config/tuning.defaults.json
  - name: surface
    tuning: config/tuning.defaults.json
    replay: [surface_ground]
  - name: box-ground
    tuning: config/tuning.defaults.json
    replay: [surface_ground, box_ground]
cases:
  corpus: d2-first-segment
  count: 23
steps:
  - id: implement
    kind: manual:implement
    estimate: { human: 2h }
    note: >-
      box_model.ground_anchor: the published box's base at the surface's height under its centre
      (the cluster's lowest return where no surface is fitted) and its height a window belief
      like the extents', the frame's LatestZ and OBBHeight kept as diagnostics; lidar-box-eval's
      dx-ground diagnosis, the drawn box's frame-to-frame change of base and of height as an RMS
      on confirmed vehicle tracks, and a check that x, y, heading and extents are unchanged row
      for row against the arm without it.
  - id: boxes
    kind: box_eval
    over: [arms, cases]
    estimate: { each: 1m, worker: m1pro }
  - id: kirk
    kind: state_estimation_baseline
    over: [arms]
    estimate: { each: 3m, worker: m1pro }
    note: kirk0, for the physical references' heights
  - id: score
    kind: ground_truth_eval
    over: [arms]
    estimate: { each: 2m, worker: m1pro }
    params: { split_role: tuning, physical_reference: true }
  - id: write
    kind: manual:write
    estimate: { human: 1h }
---

# ground0004: the published box on the ground

**Question.** The box model made the published box's centre, heading and extents agree with each
other and with the returns (geom0005 to geom0010); its vertical placement it left as it was:
`LatestZ` is the associated cluster's base in the frame it was seen, `OBBHeight` the frame's
cluster height. Both change with which returns the clip and the frame kept, so the drawn box bobs
and breathes while its footprint holds still. The state plan puts vertical position outside the
state as a belief, the ground contact's z plus the surface it was read from. With the surface
fitted (`surface` is the control), should the box stand on it, with a height it has learned,
rather than on each frame's cluster?

**Measures.** The drawn box's frame-to-frame change of base z and of height, as an RMS over
confirmed vehicle tracks, per site and pooled (`dx-ground`); the box's x, y, heading and extents
against `surface`'s, row for row; kirk0's reviewed heights against the 22 references, where the
fit recorded one; the frame time.

**Rule, written before any replay (2026-10-18).** `box-ground` passes when its base and height
RMS each fall by half or more against `surface`, pooled and on every graded site; its x, y,
heading and extents are identical to `surface`'s on every row, since an option that touches only
the vertical must not move the plane; kirk0's reviewed height error median is no higher than
`surface`'s; and the frame time is unchanged within the bench's spread. A failure of the second
clause is a defect in the implementation, not a result about the ground.

**What this is for.** The viewers and the published scene assets draw this box, and a box whose
base follows a surface the sensor settled on is what a mesh would give it: with the cell model
the base follows the cell, with one plane the plane, and the difference between the two on a
site is ground0001's departure table at the box's positions. Nothing on the following path reads
z, so the following gap must be unchanged by construction; if it moves, the implementation
leaked into the plane.

**What it does not establish.** Anything about speed, identity or the footprint. Whether a
viewer wants the base or the centre reported: the option publishes the base and the height, from
which either is drawn.
