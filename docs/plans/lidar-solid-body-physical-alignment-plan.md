# Solid body against physical references: extent and heading from one face

The [kirk0 physical-reference pilot](../lidar/operations/physical-reference-pilot-kirk0-2026-10.md)
scored the solid body against reviewed body references for the first time. A car is tracked
within 0.25 m; neither truck is. This plan fixes the two mechanisms behind that, as opt-in
experiments, and measures them against the same frozen split, with guards that keep three
vehicles on one capture from becoming the thing the tracker is fitted to.

- **Status:** Run 2026-10-09 on branch `claude/physical-align`: the [report](../lidar/operations/solid-body-physical-alignment-kirk0-2026-10.md) keeps the course heading, growth admission, end-face centring (open prior) and the vehicle floor as one opt-in candidate, which aligns the trucks' heading and truck 1's length and holds the corpus's lateral tail; it records the prior floor as harmful and face-plane spans as null, and finds the rest of the trucks' error upstream of the solid body. The [geometry convergence plan](lidar-tracker-geometry-convergence-plan.md) takes the two structural gaps the run exposed, no heading observation and no containment of the points, as its brief
- **Target:** v0.5.2, Sprint 0.5.2.1, the backlog's "Solid-body extent accumulation" item
- **Layers:** L5 tracker (solid body), offline evaluation
- **Canonical:** [LiDAR pipeline reference](../lidar/architecture/lidar-pipeline-reference.md)
- **Related:** [kirk0 pilot](../lidar/operations/physical-reference-pilot-kirk0-2026-10.md), [near-edge tracked state](lidar-near-edge-tracked-state-plan.md) (extent admission "is the lever"), [state estimation](lidar-state-estimation-plan.md) §9.2.1–9.2.2, [F0 report](../lidar/operations/facet-f0-attribution-kirk0-report.md) §4.1, [span selection timing](../lidar/operations/near-edge-span-selection-timing-2026-10.md)

## What the pilot measured

At 22 reviewed poses on samples 472 to 530 (pinned split `kirk0-ad8b9438-tuning-r1`, physical
revision 76), the solid body's persisted lengths are exact bins of its extent histogram: truck 2,
approaching end on, at 0.625 m against 9.63 m in both arms; truck 1 at 0.375 m (control) and
5.875 m (near-edge) against 10.63 m. Truck 2's heading is 76° to 126° off, the box turned across
the truck. Every gap with truck 2 as follower is 1.6 to 2.2 m long.

## Why, in the code

1. **A single face's depth replaces the prior.** `admitSolidBodyExtents`
   (`l5tracks/solid_body_nearedge.go`) admits, for a front or rear face, the cluster's span along
   the axis as a length, and for a side face its span across the axis as a width. That is the span
   along the face's normal, which one face sees only as deep as the returns behind it reach. The
   span along the face's own plane, which it does see whole, is not admitted. `dimensionFromBelief`
   then uses the accumulated value from the first observation, with no floor at the prior, so the
   first front-face frame of an approaching truck turns 4.5 m into 0.625 m, and the corroborated
   maximum only grows when three frames see longer, which an approach never supplies. The state
   estimation plan already says a single face is an uncertain lower bound and estimation starts
   from the prior; the code does neither.
2. **A face-only cluster's heading runs across the vehicle.** The reported heading is the tracked
   PCA axis seeded at birth. A front face is a strip about 2.8 m across and 0.6 m deep; its axis
   passes the aspect lock, is seeded across the truck, and the 60°–120° jump guard never fires
   against a heading that started wrong. Velocity only flips it by π. Below 2 m/s the front face is
   then named lateral and the length and width are transposed.

## Experiments

Each is a new default-off `-experiment` name, folded into the parameter hash; nothing changes
production B0, the shadow or A2 unless named.

| Name                            | Change                                                                                                                                                                                                                                                            |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `solid_body_face_plane_spans`   | A face admits the span along its own plane: an end face a width, a side face a length. The span along its normal is admitted only when an adjacent face is also visible, a corner view, where the cluster does reach both ends of that dimension.                 |
| `solid_body_extent_prior_floor` | An accumulated extent below the class prior is a lower bound, not a value: the dimension stays at the prior, with its prior sigma, until a span at or above it is admitted.                                                                                       |
| `solid_body_course_heading`     | Above the existing course speed (2 m/s) the solid body's orientation is the course, with the PCA axis used only to resolve its sign when they agree within the existing 10° window. Hooks: `MotionClass.HeadingFollowsVelocity`, `OrientationBelief.ResolveWith`. |

Constants are the existing ones (the class priors, `CourseAlignmentMinSpeedMps`, the 10° span
window). None is fitted to the pilot's poses.

The run added a fourth mechanism and four experiments, found by reading the rows and the corpus:
`solid_body_extent_growth` (a merge candidate no wider than the body still gives its length,
since the merge test refused an approaching truck's length for 22 frames as its side came into
view), `solid_body_vehicle_extent_floor` (a rigid vehicle, or a body of unknown class already a
car's length, is at least the smallest road car) and `solid_body_end_face_centring` (at a fix by
an end face alone, the position across the body comes from that face's own returns, since the
corpus showed earlier lengths starting end-face-only fixes that drift across the body), with
`solid_body_end_face_centring_open_prior` as its second setting (a narrow face is refused only
against a measured width, not the class prior). The
course heading as built turns from the axis toward the course as their disagreement and the
speed grow, after taking the course outright cost an accelerating car two degrees and switching
at the window edge turned the box by the whole window in one frame.

## Method

- **Loop.** One kirk0 replay per arm (`-warmup 20`, about 18 s) and a score against the frozen
  split: physical references on `following-472-530` scored alone (the split's episodes overlap),
  identity on `whole-pack`. Scripts and every result in `~/vr-scratch/physical-align/`, one ledger
  row per arm. Unit tests in the same commit as each experiment: an end-on synthetic approach, a
  receding pass and a corner view.
- **Order.** Diagnose first (per-frame lengths, faces and headings of the three vehicles against
  their poses), then each experiment alone on the shadow, then together, then with
  `near_edge_track`.
- **Guards.**
  - Car 8 does not get worse: median centre within 0.30 m, heading within 8°.
  - The near-edge arms' whole-pack HOTA and IDF1 stay within 0.01 of their baseline, or the change
    is explained. The shadow cannot move them, since it does not feed the track.
  - The 23-site first-segment corpus, label-free, for the best combination with `near_edge_track`
    against A2: track counts, lifetimes and the solid-body summary, read from the local volume.
  - kirk0 `Tracker.Update` p99 within 10 % of its arm's baseline: extent admission is already most
    of A2's cost.
- **Reading.** Primary: truck 2's heading and gaps, the trucks' lengths and seen bumpers, box
  overlap. Secondary: centres, which an end-on truck of unknown length cannot place without its
  length, so a prior floor may move truck 2's centre either way.

## What this cannot establish

Three vehicles on one tuning capture. Any improvement is tuning evidence for a mechanism the code
and its plans already say is wrong, not a validated rate; physical scoring still refuses a held-out
split. No default changes and no promotion follow from this plan; a report records every arm,
including those that make things worse.
