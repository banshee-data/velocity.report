# Truck membership and the extent state on kirk0: the join, its gap and track-conditioned membership

<!-- ignore-style-length -->

The [motion-supported join](w3-motion-fragment-joining-cloud-2026-11.md) (l4clus0008, #930) takes
kirk0's 20 to 35 m fragmented instants from 6.5 % to 5.7 %, and #970 gave the solid body an extent
state that carries the half-extents as states. This run asks whether the join's truck fragments
reach that extent state, scored on kirk0's reviewed labels with the extent state on in every arm,
and fixes the one gap the predeclared rule names. It answers
[l4clus0010](../../../data/experiments/l4clus/l4clus0010.md).

- **Status:** Measured, 2026-11-08. One Claude Code cloud session; no default changed
- **Spec:** [l4clus0010](../../../data/experiments/l4clus/l4clus0010.md)
- **Layers:** L4 joining (the footprint the join reads), L5 solid body (extent state), the
  per-frame and retention evaluators
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) and the
  [join set](../../../config/experiments/motion-fragment-join.json); the face-state options and the
  new `fragment_join_track_membership` are replay experiments
- **Related:** [kirk0's labels in a cloud run](cloud-run-kirk0-labelled-2026-11.md),
  [October W3 baseline](w3-retention-fragmentation-kirk0-2026-10.md),
  [F0 attribution](facet-f0-attribution-kirk0-report.md)

## Summary

- **The join does not reach the trucks.** With `solid_body` and `solid_body_face_state` on, the
  join takes fragmented truck instants from 198 to 182 of the 936 that reach a cluster (21.2 % to
  19.4 %), 1.7 points of the 5 the rule asks, and leaves the trucks' believed length where it was
  (median absolute error over the 15 referenced truck instants 0.82 m against 0.83 m). Its identity
  is non-inferior and its IDF1 gain keeps its sign over the bootstrap, as in the cloud run.
- **The rule names track-conditioned membership.** The selector it predeclared reads the published
  box, the footprint the join tests membership against, as more than 1 m under the solid body's
  believed length on 394 of 874 truck instants (45 %): not most, so the rule names fix (c), not (b).
  Post hoc, on the fragmented truck instants alone the share is 80 %: (b) remains a candidate.
- **Track-conditioned membership passes the screen.** `fragment_join_track_membership` widens each
  footprint by two standard deviations of the body's projected position along and across it. It
  takes fragmented truck instants to 145 of 936 (15.5 %, 5.7 points under `a0`), joins 79
  components against the join's 39, adds no reviewed merge, keeps pooled retention at 0.851, and
  with the option off the build reproduces both `a0`'s and `join`'s tracking baselines byte for
  byte. Its identity is non-inferior to `a0`, with an IDF1 gain that keeps its sign (+0.0004 to
  +0.0055). It is a candidate for the corpus read, not a default.
- **More membership is more extent evidence, but not enough.** Under the fix truck 1's believed
  length moves from 9.85 m to 10.15 m against its 10.63 m reference, and its median sigma over the
  pass from 0.50 m to 0.28 m; truck 2 stays 3.3 m short. Pooled over the 15 referenced instants
  the median absolute length error falls from 0.82 m to 0.50 m, short of the 0.5 m reduction J3
  asks, which is not the fix's clause.
- **A latent extent-state defect.** Under the join one track's extent-state length went to exactly
  0 m on three rows (a 4.6 m `unknown` body whose width doubled over the same frames); the
  evaluator refuses the arm on them. Nothing keeps the half-extent positive. Neither `a0` nor the
  fix arm writes such a row on kirk0.

## Question and rule

The question, from the review's fix 4: does joining a truck's fragments carry its extent evidence
to the extent state, and if not, where does the evidence stop? The rule was committed at
`1e60826` before any replay (one replay launched before that commit was stopped and its output
deleted unread). Its three clauses, each arm against `a0`:

- **J1, identity non-inferiority.** The paired bootstrap's 5th percentile of the HOTA and IDF1
  deltas at or above −0.02, its 95th percentile of the switch delta at or below +10.
- **J2, truck fragmentation.** The fragmented truck share at least 5 points under `a0`'s.
- **J3, truck extent.** The median absolute truck length error at least 0.5 m under `a0`'s.

On `join`: J2 holding and J3 failing names fix (a), a joined span admitted as length evidence on a
merge candidate; J2 failing names (b), the solid body's extent in the footprint, when the published
box is more than 1 m under the believed length on most truck frames, and (c), track-conditioned
membership, otherwise. The fix arm passes when it is J1 non-inferior, meets its gap's clause (J2
for (c)), adds no reviewed merge, keeps pooled retention at or above `a0`'s less 0.005, and the build
with the option off reproduces `a0`'s baseline. The `fix` arm was added to the spec at `0ac3ff8`,
before its replay, with the rule unchanged.

## Method

- **Replays.** `lidar-state-estimation-baseline` on kirk0 as a one-case corpus, the whole capture,
  `-warmup 6 -require-settled=false -sample-points 1024`, evidence per case, every arm
  repeat-verified (first and repeat tracking baselines equal), with the committed
  [cohort manifest](../../../config/cohorts/cohort-manifest.json), a scratch use ledger at migration
  71 and an empty annotation root, as the development eligibility guard now requires.
- **Arms.** Each runs `solid_body,solid_body_face_state`. `a0`: the defaults. `join`: the join
  set. `fix`: the join set with `fragment_join_track_membership`.
- **Labels.** The reviewed pack `ds_67842cb1b22b70e7` reconstructed in the container by
  `lidar-pack-rebuild`, as in the [cloud labelled run](cloud-run-kirk0-labelled-2026-11.md#the-reconstructed-pack):
  packets and returns match the manifest's digests, the points are decoded by this build's parser
  and their digest differs. The split was drafted with
  `lidar-annotation-split-draft -from-seconds 6 -split kirk0-tuning`; `obj_788622b5`, the parked
  car, is ignored by every score.
- **Identity (J1).** `perframe` on the online-stage tracked estimates as declared baselines (the
  replay writes no final stage), `-bootstrap 2000 -bootstrap-seed 20261108`, each arm against `a0`.
- **Truck fragmentation (J2).** `retention -min-domain-share 0.3`: of truck instants reaching a
  cluster, those in two or more. The denominators are the same in every arm: 936 truck instants,
  510 at 20 to 35 m, 435 at 35 to 50 m, 256 beyond 50 m.
- **Truck extent (J3).** `perframe -physical-reference` on the solid-body rows, online stage: each
  referenced truck instant's believed length against the reference length (truck 1 10.63 m,
  interval 10.40 to 10.87; truck 2 9.63 m). The pack's physical references pin the published pack
  digest, which a reconstructed pack cannot match; they were fitted to the reviewed returns, which
  the rebuild reproduces exactly, so a scratch copy re-pinned to the reconstructed digest was
  scored. Only instants a reference keyframe covers are scored: 15 truck instants of 22.
- **The selector.** For each truck instant reaching a cluster, the cluster holding most of it, the
  solid-body row that observation fed, and the same track's published box at that frame
  (`lidar_track_observations.box_length`, the `BoxLength` the join's footprint reads), read from
  the evidence database with a scratch script.

## Findings

| Arm    | Replay options               |   HOTA |   IDF1 | Switches | Fragmentations | False negatives | Components joined (frames) | Truck instants fragmented (of 936) | 20–35 m (of 510) | 35–50 m (of 435) | 50 m+ (of 256) | Pooled retention | Reviewed merges | Truck length, median abs error (15 instants) |
| ------ | ---------------------------- | -----: | -----: | -------: | -------------: | --------------: | -------------------------: | ---------------------------------: | ---------------: | ---------------: | -------------: | ---------------: | --------------: | -------------------------------------------: |
| `a0`   | face state                   | 0.4284 | 0.4728 |       90 |             46 |             265 |                          0 |                        198 (21.2%) |        33 (6.5%) |      142 (32.6%) |      23 (9.0%) |            0.851 |               0 |                                       0.82 m |
| `join` | face state                   | 0.4307 | 0.4804 |       89 |             42 |             262 |                    39 (39) |                        182 (19.4%) |        29 (5.7%) |      132 (30.3%) |      23 (9.0%) |            0.851 |               0 |                                     0.83 m ¹ |
| `fix`  | face state, track membership | 0.4323 | 0.4769 |       90 |             42 |             262 |                    79 (78) |                        145 (15.5%) |        22 (4.3%) |      103 (23.7%) |      22 (8.6%) |            0.851 |               0 |                                       0.50 m |

¹ Scored without the three zero-length rows the evaluator refuses (one non-truck track; see below).

The solid body runs as a shadow, so `a0` and `join` read the cloud run's identity figures exactly
(0.4284, 0.4728, 90, 46; 0.4307, 0.4804, 89, 42).

**Identity against `a0`, paired bootstrap** (19 scored objects, 2,000 draws, seed 20261108):

| Arm    | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches, 5th / 50th / 95th | Δ fragmentations, 5th / 50th / 95th | Interval keeps its sign |
| ------ | --------------------------- | --------------------------- | ----------------------------- | ----------------------------------- | ----------------------- |
| `join` | −0.0013 / +0.0018 / +0.0040 | +0.0004 / +0.0077 / +0.0181 | −3 / −1 / 0                   | −12 / −4 / 0                        | IDF1                    |
| `fix`  | −0.0002 / +0.0037 / +0.0081 | +0.0004 / +0.0032 / +0.0055 | 0 / 0 / 0                     | −12 / −4 / 0                        | IDF1                    |

The labelled-object point figures (tracks attributed to no reviewed object left out): `a0` HOTA
0.4798 and IDF1 0.5582, `join` 0.4819 and 0.5662, `fix` 0.4835 and 0.5615.

**By truck**, instants fragmented of those reaching a cluster:

| Arm    | Truck 1 (`obj_35c661db`, of 488) | Truck 2 (`obj_8e829a06`, of 412) |
| ------ | -------------------------------: | -------------------------------: |
| `a0`   |                      130 (26.6%) |                       68 (16.5%) |
| `join` |                      114 (23.4%) |                       68 (16.5%) |
| `fix`  |                       81 (16.6%) |                       64 (15.5%) |

**Extent.** The referenced truck instants, believed length (median) and its error:

| Arm    | Truck 1, 9 instants (10.63 m) | Truck 2, 6 instants (9.63 m) | Below the reference interval |
| ------ | ----------------------------- | ---------------------------- | ---------------------------: |
| `a0`   | 9.85 m (−0.78)                | 6.24 m (−3.40)               |                     15 of 15 |
| `join` | 9.84 m (−0.79)                | 5.77 m (−3.86)               |                     15 of 15 |
| `fix`  | 10.15 m (−0.48)               | 6.33 m (−3.30)               |                     15 of 15 |

Over every truck instant reaching a cluster (the selector's population, not a rule measure), the
believed length's median is, for truck 1, 9.90 m (sigma 0.50 m) under `a0`, 9.89 m under `join` and
10.16 m (0.28 m) under `fix`; for truck 2, 5.69 m (1.03 m), 5.77 m and 6.16 m (0.94 m). Over all 22
referenced instants, cars included, the solid body's mean absolute length error is 1.513 m, 1.646 m
and 1.359 m, its centre error 1.094 m, 1.169 m and 1.221 m, and its box IoU 0.452, 0.436 and 0.424:
the fix's longer truck moves its centre further from the reference on these instants, a placement
cost the next extent work should read.

**The selector.** On the trucks' instants with a solid-body row and a published box, the box is
more than 1 m under the believed length:

| Arm    | All truck instants | Fragmented truck instants | Box median, truck 1 / 2 | Belief median, truck 1 / 2 |
| ------ | -----------------: | ------------------------: | ----------------------- | -------------------------- |
| `a0`   |   404 of 875 (46%) |          148 of 190 (78%) | 9.51 / 4.05 m           | 9.90 / 5.69 m              |
| `join` |   394 of 874 (45%) |          138 of 173 (80%) | 9.70 / 4.05 m           | 9.89 / 5.77 m              |

**The zero-length rows.** Under `join`, track `trk_96ac28bb…` (class `unknown`, 11 rows) carried an
extent-state length of 4.2 to 4.6 m with sigma about 1.9 m while its width rose from 1.2 m to 2.6 m,
then wrote 0.0 m with sigma 0.94 m on its last three rows, two of them near-edge fixes. The
evaluator refuses a solid body whose length is not positive; the J3 row above scores a scratch copy
of the database without those three rows. A face measures n·c + h in the extent state's update and
nothing bounds h below, so a face whose plane jumps, as a cluster that has absorbed a neighbour's
returns can make it, moves h by its gain. This is the extent state's defect, exposed by the join's
feedback; it is not the fix's, which writes no such row.

## Against the rule

- **`join`.** J1 holds (5th percentiles −0.0013 and +0.0004, switch 95th percentile 0). J2 fails
  (−1.7 points of 5). J3 fails (+0.01 m). The selector reads 45 %, not most: fix (c).
- **`fix`.** J1 holds (5th percentiles −0.0002 and +0.0004, switch 95th percentile 0), and its
  IDF1 gain keeps its sign. J2 holds: 5.7 points under `a0` (198 to 145 of 936). No reviewed merge in
  any arm. Pooled retention 0.851 in every arm, at `a0`'s. Off-path identity holds: built at
  `9bf60fb`, `a0` and `join` without the option write `tracking_baseline.json` byte-identical to the
  base build's (`79d9abff…` and `9dc26717…`) with the join's counters unchanged (39 in 39 frames).
  The VRLOG frame chunks differ between any two runs, the first and repeat of one build included,
  so they are not the comparison. **The fix arm passes the screen.**

## Why (c) works where the join did not, and what it costs

The join tests membership against the published box, which for truck 2 is a 4 m box on a 9.6 m
truck, and asks every member of a component to lie inside it. Under the fix a component whose
members lie within two standard deviations of the predicted position beyond the box is the body's.
On kirk0 that doubles the joins (79 against 39), and fragmentation falls where it was worst: truck
1's fragmented instants from 130 to 81, and the 35 to 50 m band from 142 to 103. The joins are
counted per frame, not attributed to objects, so this is the end-to-end figure. Truck 2 barely moves
(68 to 64), which suggests its fragments lie metres, not sigmas, outside its 4 m box. That is the
case fix (b) is for.

The burden: the widened footprint is up to twice `max_position_stddev_metres` longer at each end
(1.5 m at the registered 0.75 m) and is still refused when it touches a second eligible body, so
the cost falls on refusals near other vehicles rather than on merges; kirk0's trucks never come
within reach of each other, so this capture does not test that. It adds two square roots per hint
per frame; no Pi timing was taken.

## What this does not establish

- Anything held out or on the corpus. kirk0 is a tuning capture, and its two trucks are all of J2
  and J3. The rule's J3 rests on 15 referenced instants.
- That the reconstructed pack is the published one: its returns are, its points are not, so
  retention ran with its floor lowered, as in the cloud run. The physical references were re-pinned
  to the reconstructed digest in scratch.
- Merges between vehicles close together. No kirk0 cluster holds two reviewed objects in any arm.
- Whether the extent state is right for cars, or whether its truck length is now good enough for
  the published box. Truck 2 is still 3.3 m short.
- Pi cost or live behaviour.

## Recommendations

1. **Read `fragment_join_track_membership` on the corpus** beside the join alone, with a merge
   reference that has vehicles side by side, before any default.
2. **Bound the extent state's half-extents below** (a truncation at a small positive floor, or at
   the frame's span bound) and add the zero-length case to its tests: it is a defect in the merged
   #970 state that any cluster jump can reach.
3. **Score fix (b) next on truck 2's case**: on 80 % of fragmented truck instants the published
   box is more than 1 m under the solid body's believed length, and covariance cannot cover metres.
4. **Read placement with extent.** A longer believed truck moved the solid body's centre error up on
   the referenced instants; the next extent arm should report both.

## Provenance

| Item           | Value                                                                                                                                                                                                                                                                                                    |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base           | main `99a9627`; rule committed at `1e60826`, fix at `9bf60fb`, fix arm in the spec at `0ac3ff8`                                                                                                                                                                                                          |
| Tools          | Built with the Makefile's LDFLAGS and the `pcap` tag. Replays of `a0` and `join` and their scores at `99a9627` (replay executable `sha256:49596e84…`, evaluator `fd1f2a30…`); `fix` and the off-path controls at `9bf60fb` (replay `8d5e23b5…`, evaluator `5b9bee1c…`); `lidar-pack-rebuild` `8d8b39da…` |
| Parameter sets | `tuning.defaults.json` fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`; `motion-fragment-join.json` fingerprint `719d291ed197`, file SHA-256 `2e604040…`                                                                                                                                            |
| Replay         | `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024`, evidence per case, every arm repeat-verified; replay experiments `solid_body,solid_body_face_state` (and `fragment_join_track_membership` for `fix`)                                             |
| Capture        | kirk0 `sha256:2864ebde…`, copied from another worktree's Git LFS checkout and checked against the pointer                                                                                                                                                                                                |
| Labels         | label set `kirk0-reference` revision 2233, pack `ds_67842cb1b22b70e7` reconstructed (pack digest `sha256:f0ed3414…`, packets `3ffeba76…`, returns `f0f9ca27…`, points `7860a86c…`), physical references revision 77 re-pinned in scratch; dataset `banshee-data/sf-street-speeds`                        |
| Split          | drafted, `kirk0-tuning`, one episode, samples 61 to 831, `obj_788622b5` ignored, manifest `7194bb61…`, split digest `sha256:a784e69d…`                                                                                                                                                                   |
| Eligibility    | committed cohort manifest; a scratch use ledger; actor `appius-l4clus0010`                                                                                                                                                                                                                               |
| Host           | Claude Code cloud container, 4 cores, 15 GB; raw output in the session scratch, not kept                                                                                                                                                                                                                 |
