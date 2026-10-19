---
id: ground0003
title: "The planar state in the ground's frame: x and y of the fitted surface instead of the sensor's horizontal plane"
status: proposed
owner: david
created: 2026-10-18
asks:
  - docs/plans/lidar-state-estimation-plan.md#54-what-is-in-the-state-and-what-is-not
  - docs/lidar/operations/brief/research-briefing-2025-01-to-2026-10.md#schäfer-alrifaee-and-hashemi-advanced-intelligent-systems-2026
  - docs/BACKLOG.md
depends_on: [ground0001, ground0002]
report: docs/lidar/operations/ground-frame-tracking-2026-10.md
requires:
  capabilities: [pcap:kirk, pcap:s2, labels:kirk, pcap-build]
  disk_gb: 20
arms:
  - name: a0
    tuning: config/tuning.defaults.json
  - name: surface
    tuning: config/tuning.defaults.json
    replay: [surface_ground]
  - name: ground-frame
    tuning: config/tuning.defaults.json
    replay: [surface_ground, ground_frame]
  - name: ground-frame-local
    tuning: config/tuning.defaults.json
    replay: [surface_ground, ground_frame_local]
cases:
  corpus: d2-first-segment
  count: 3
  sites: [marina-webster-beach, columbus-broadway, embarcadero-bay]
steps:
  - id: implement
    kind: manual:implement
    estimate: { human: 4h }
    note: >-
      ground_frame: the roll and pitch of the global plane's normal applied as the static pose to
      every world point before the clip and L4, so L5's x and y are the surface's; every published
      position, box and solid body rotated back to the sensor frame, so the labels, the references
      and the viewers read the one frame they always have. ground_frame_local: the same, with the
      speed also reported along the local cell's surface, the planar speed divided by the cosine
      of the cell's grade along the velocity, beside the planar speed and never in its place.
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
  - id: replay
    kind: state_estimation_baseline
    over: [arms, cases]
    estimate: { each: 3m, worker: m1pro }
    note: >-
      the two graded first segments and one level control; embarcadero-bay stands in until
      ground0001's table names the flattest first segment, and is replaced before approval if it
      is not within 0.3 % grade
  - id: write
    kind: manual:write
    estimate: { human: 2h }
---

# ground0003: the planar state in the ground's frame

**Question.** The state is `[x, y, vx, vy]` and the measurement the box centre's x and y, both in
the sensor frame at identity pose. Constraining motion to the ground, as every fixed roadside node
in the literature does, is therefore not a change of state here: it is the question of which
plane x and y are in. Today it is the sensor's horizontal plane; the ground's plane is the one
the P11 surface fits. With the surface clip held the same (`surface` is the control), does
tracking in the surface's own x and y give steadier tracks, and what is the planar speed worth
against the speed along the surface?

**What the geometry predicts, written first.** For a global gradient `g` the planar speed in
the sensor frame is `cos(atan g)` of the speed along the surface: 0.09 % low at Marina's 4.35 %,
0.004 % at Columbus. A return at height `h` above the ground moves `h sin(atan g)` in x and y
under the rotation: 0.11 m for a 2.5 m roof at Marina, 0.07 m for a car's. Both are below what
the measures resolve (the lateral fit's 0.5 m excursion, the references' 3.7° yaw and 0.25 m
centre). The expected result is therefore no measurable difference between `ground-frame` and
`surface` at these grades. The experiment exists to measure that bound rather than assume it,
and to build the correction a pitched mount needs: at 20° down-pitch, which Schäfer's node runs,
the planar speed is 6 % low and a roof moves 0.85 m, and a corpus whose mounts are level cannot
say so. The corpus pose sweep found the mounts' tilt changing by under 0.1° within a placement;
what the absolute pitch is, only the surface says.

**Measures.** As ground0002's: lateral p99, speed and heading jitter, course alignment per site;
kirk0's per-frame score and the 22 reviewed poses; and, new, the ratio of `ground-frame`'s
planar speed to `surface`'s on tracks matched between arms, per site, against the predicted
`cos(atan g)`, and `ground-frame-local`'s along-surface speed against both.

**Rule, written before any replay (2026-10-18).** `ground-frame` passes when, against
`surface`, the lateral p99 and the speed jitter fall by a tenth or more on marina-webster-beach
and nothing of ground0002's clause 2 is lost on kirk0. It is found moot, the expected outcome,
when every difference from `surface` is inside the measures' resolution and the speed ratio
agrees with the prediction within 0.05 %. It has a defect when a difference exceeds what the
geometry allows, since nothing but the frame changed, and the report says where. Moot is a
result: the frame then closes for level mounts, the rotation is kept as the correction for a
pitched one, and ground0001's gradient table is the record that says which a site has.
`ground-frame-local` is read for one thing: that its along-surface speed exceeds the planar
speed by the cell's `1/cos` and by nothing else. Where a cell's grade differs from the global
plane's by more than 1 %, the local speed is the one a mesh would report, and the difference is
the mesh's whole effect on speed.

**What could mislead.** Scoring in the wrong frame: the labels and references are in the sensor
frame, so an arm that reports in the surface's frame is compared on its frame, not its
tracking; the implement step rotates every output back, and the first check is that
`ground-frame`'s box rows on the level site equal `surface`'s to the rotation. The L3 grid is
polar in the sensor frame and is not rotated: the background and foreground are the same
returns, only L4 and L5 see them turned, so a change in cluster counts between the arms is a
defect, not an effect. The clip is shared by construction; an arm without `surface_ground` would
mix the frame with the clip.

**What it does not establish.** A speed truth: the planar speed's scale is read against the
geometry, not a reference; D-29's RTK probe is what would measure it. Anything on a pitched
mount, which no capture has. Anything held out.
