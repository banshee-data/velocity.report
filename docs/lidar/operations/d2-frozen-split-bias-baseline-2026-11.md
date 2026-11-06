# D2 on kirk0's frozen split: identity, repeatability and the measurement-bias baseline

<!-- ignore-style-length -->

The October D2 re-run scored the three cluster positions against an unfrozen split draft and
against visible-mask centres only. This run binds the same question to kirk0's frozen tuning split
at the current build, keeps replay repeatability apart from track-linked accuracy, and adds the
measurement-bias baseline: how far each arm's measurement, estimate and published box sit from
the 22 reviewed body centres, by vehicle, range and aspect. It answers
[geom0015](../../../data/experiments/geom/geom0015.md).

- **Status:** Measurement complete, 2026-11-06. No default changed
- **Layers:** L4 (cluster position), L5 (the online estimate and the published box), scored
  against L10 physical references
- **Related:** [D2 current-solver re-run](d2-medoid-obb-current-solver-2026-10.md),
  [physical reference pilot](physical-reference-pilot-kirk0-2026-10.md),
  [centre against extent (W9)](geometry-centre-extent-decomposition-2026-10.md),
  [mean cluster centre](l4-cluster-mean-centre-2026-11.md),
  [labelling workspace and eligibility](labelling-workspace.md),
  [sprint plan S0](../../plans/lidar-052-mvp-sprint-plan.md#s0-make-the-next-experiment-trustworthy)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) (`a0` and
  `obb-centre`, fingerprint `ab0c4035780d`) and
  [foreground-refine](../../../config/experiments/foreground-refine.json) (`medoid`, fingerprint
  `42e3a582b803`)

## Answer

**The mean stays the production position and the OBB centre stays opt-in.** Neither alternative
beat `a0` on both HOTA and IDF1 with the whole 5th-to-95th bootstrap interval above zero, the
spec's bar for a candidate. The medoid is worse on both: HOTA delta −0.030, interval
[−0.053, −0.007]; IDF1 −0.054, [−0.096, −0.016]. The OBB centre ties on HOTA (−0.001,
[−0.036, +0.027]) and loses IDF1 (−0.045, [−0.087, −0.011]) with 125 more identity switches.
D5 and l4clus0006 stand. Nothing goes forward to label0001.

**Every replay repeats exactly.** Each arm's two replays wrote identical observation, estimate and
residual rows. One premise of the rule did not hold: `a0` and `obb-centre` were expected to write
identical observations, since only L5's measurement differs, and they do not (7,024 against 7,113
rows). This is a finding about the pipeline, not a repeatability failure (see
[Repeatability](#repeatability)).

**The published box is the reading to quote, and it barely depends on the cluster position.** On
the 22 reviewed poses, every arm's box matches all 22 within 3 m at a median centre error of 0.18
to 0.19 m. The cluster positions do worse: the mean and medoid measurements leave 7 and 8 truck
poses beyond 3 m, and the matched ones sit about 2.9 m off on truck 1. The OBB-centre measurement
matches all 22 at 0.56 m. The car is where position bias reads cleanly: there the mean
measurement sits 0.35 m off, 0.27 m toward the sensor, and the box 0.05 m off.

## Method

Spec, rule and arms are in [geom0015](../../../data/experiments/geom/geom0015.md). The rule was
committed in `9cb7b3804` before any replay. Each arm replayed kirk0 whole twice with
`lidar-state-estimation-baseline` (`-warmup 6 -require-settled=false -sample-points 1024`, evidence
per case). `obb-centre` is the defaults with `-measurement-mode obb_centre_v1`, which no parameter
set can name. Each replay took about 2.4 minutes on the M1 Pro. The development eligibility guard
(#944) admitted every replay and every score against the committed
[cohort manifest](../../../config/cohorts/cohort-manifest.json) (Kirkham at 9th in development,
D-40), and recorded each use in a scratch use ledger.

Scoring is `lidar-ground-truth-eval perframe` against the frozen split
`kirk0-reference-tuning-r1.json`:

- **Identity:** `whole-pack`, default reference policy, footprint gate 1 m, `obj_788622b5` (a
  parked car) ignored, online estimates as declared baselines, 2,000 bootstrap draws over objects
  with seed 20261105. The same comparison was rerun without the ignore, and with the D2 recipe's
  `point_mean` reference, as sensitivity.
- **Physical:** episode `following-472-530`, gate 3 m, each arm's three readings: the measurement
  its L5 update consumed (`-a-measurements`), the online estimate, and the published box
  (`-a-boxes`). `centre_by_view` gives each matched pose's range, aspect and offset toward the
  sensor. Range bands are under 10, 10 to 20 and 20 m or more. Aspect is 0° end on to 90°
  broadside.

**A scorer fix during the run.** The first scoring pass counted the episode's 6 unreviewed poses
as reference poses and listed them as unmatched, giving 28 poses where 22 are reviewed. Commit
`50fb74690` counts them as `unreviewed_poses` instead. Every figure here is from the re-score at
that build. The identity scores came out identical in both passes, and the matched poses and their
errors did not change. Only the denominators and unmatched counts moved.

## Identity

Whole reviewed pack, all hypotheses counted (the comparison's totals):

| Comparison | Variant              | Arm        |   HOTA |   IDF1 | IDSW | Frag | Recall |
| ---------- | -------------------- | ---------- | -----: | -----: | ---: | ---: | -----: |
| medoid     | obj_788622b5 ignored | a0         |  0.422 |  0.467 |   87 |   47 |  0.815 |
|            |                      | medoid     |  0.392 |  0.413 |  117 |   48 |  0.802 |
|            |                      | delta      | −0.030 | −0.054 |  +30 |   +1 | −0.012 |
|            | no ignore            | delta      | −0.026 | −0.045 |   −4 |   +2 | −0.012 |
|            | point_mean reference | delta      | −0.036 | −0.050 |  +27 |   +3 | −0.010 |
| obb-centre | obj_788622b5 ignored | a0         |  0.422 |  0.467 |   87 |   47 |  0.815 |
|            |                      | obb-centre |  0.421 |  0.421 |  212 |   31 |  0.812 |
|            |                      | delta      | −0.001 | −0.045 | +125 |  −16 | −0.002 |
|            | no ignore            | delta      | +0.003 | −0.038 | +109 |  −15 | −0.001 |
|            | point_mean reference | delta      | −0.072 | −0.055 | +129 |  −14 | −0.002 |

The bootstrap resamples the 20 reviewed objects and counts only hypotheses attributed to them, so
its point values are higher than the totals above (`a0` HOTA 0.471, IDF1 0.549). Its paired deltas,
5th to 95th percentile:

| Arm against a0 | HOTA delta       | IDF1 delta       | IDSW delta  | Frag delta |
| -------------- | ---------------- | ---------------- | ----------- | ---------- |
| medoid         | [−0.053, −0.007] | [−0.096, −0.016] | [+4, +65]   | [−8, +12]  |
| obb-centre     | [−0.036, +0.027] | [−0.087, −0.011] | [+21, +279] | [−44, +8]  |

The medoid's HOTA and IDF1 intervals lie wholly below zero. The OBB centre fragments less (16 fewer
fragmentations) but switches identity far more; its HOTA interval straddles zero and its IDF1
interval is wholly below. This matches the October re-run's direction on an unfrozen draft, now
bound to the frozen split.

## Repeatability

| Arm        | Observations | Observation digest | Estimates | Estimate digest    | Residual digest    |
| ---------- | -----------: | ------------------ | --------: | ------------------ | ------------------ |
| a0         |        7,024 | `bce28ddf1efda235` |     6,581 | `af40704678152a8f` | `760fb85bf228b47f` |
| medoid     |        7,013 | `170dad81ce7cbf0d` |     6,567 | `03ffaf31d9e3b925` | `c518a1d63f1d6869` |
| obb-centre |        7,113 | `e521d2823db55ee1` |     6,651 | `baa64e8a6be223f6` | `772e21383d220fe4` |

Each row holds for both replays of its arm; estimates are hashed without the random `track_id`.
So no arm has its accuracy withheld under rule 2.

`a0` and `obb-centre` diverge in their observations from frame 133 of 773, and 159 frames differ.
The rule assumed only L5 differs between them. Since the L3 track guard became a default, L5's
confirmed tracks feed back into L3's background update, so a different L5 measurement changes later
foreground and L4 clusters. That is the likely mechanism; this run did not test it by replaying with
the guard off. Later specs should not assume an L5-only change leaves L4's observations alone.

## The measurement-bias baseline

All three vehicles are in one lane on reviewed straight passes: truck 1 (`obj_35c661db`, 9 reviewed
poses), car 8 (`obj_8c728991`, 7) and truck 2 (`obj_8e829a06`, 6). The episode has 6 more
keyframes whose references are unreviewed; they are counted apart and not scored.

| Arm        | Reading     | Matched | Median error (m) | Toward sensor (m) | Along (m) | Across (m) | Position           |
| ---------- | ----------- | ------: | ---------------: | ----------------: | --------: | ---------: | ------------------ |
| a0         | measurement | 15 / 22 |             2.47 |              0.35 |      2.04 |       0.55 | cluster mean       |
| a0         | estimate    | 14 / 22 |             1.68 |              0.34 |      1.38 |       0.45 | cluster mean       |
| a0         | box         | 22 / 22 |             0.19 |              0.07 |      0.14 |       0.07 | body centre        |
| medoid     | measurement | 14 / 22 |             1.69 |              0.48 |      1.25 |       0.45 | cluster medoid     |
| medoid     | estimate    | 13 / 22 |             0.80 |              0.39 |      0.38 |       0.46 | cluster medoid     |
| medoid     | box         | 22 / 22 |             0.18 |              0.07 |      0.14 |       0.08 | body centre        |
| obb-centre | measurement | 22 / 22 |             0.56 |              0.25 |      0.15 |       0.48 | visible OBB centre |
| obb-centre | estimate    | 22 / 22 |             0.51 |              0.33 |      0.19 |       0.41 | visible OBB centre |
| obb-centre | box         | 22 / 22 |             0.19 |              0.06 |      0.12 |       0.06 | body centre        |

Medians are over matched poses only, and the matched sets differ. The mean and medoid arms lose
their worst truck poses beyond the 3 m gate (7 to 9 of 22, all trucks: truck 1 3 to 5, truck 2 4),
so their overall medians flatter them. Read the matched count first.

**By vehicle** (matched poses: median error / median offset toward the sensor, m):

| Arm        | Reading     | Truck 1        | Car 8           | Truck 2         |
| ---------- | ----------- | -------------- | --------------- | --------------- |
| a0         | measurement | 6: 2.92 / 2.86 | 7: 0.35 / 0.27  | 2: 2.58 / −1.92 |
| a0         | estimate    | 5: 2.92 / 2.87 | 7: 0.37 / 0.27  | 2: 2.75 / −2.12 |
| a0         | box         | 9: 0.19 / 0.15 | 7: 0.05 / −0.05 | 6: 1.49 / 0.25  |
| medoid     | measurement | 5: 2.75 / 2.71 | 7: 0.47 / 0.41  | 2: 2.60 / −1.82 |
| medoid     | estimate    | 4: 2.96 / 2.88 | 7: 0.45 / 0.34  | 2: 2.84 / −2.09 |
| medoid     | box         | 9: 0.19 / 0.16 | 7: 0.03 / −0.01 | 6: 1.54 / 0.26  |
| obb-centre | measurement | 9: 0.56 / 0.27 | 7: 0.06 / 0.01  | 6: 1.73 / 0.46  |
| obb-centre | estimate    | 9: 0.54 / 0.35 | 7: 0.10 / −0.07 | 6: 1.81 / 0.67  |
| obb-centre | box         | 9: 0.18 / 0.12 | 7: 0.03 / −0.02 | 6: 1.54 / 0.25  |

**By range band** (same cells):

| Arm        | Reading     | Under 10 m      | 10 to 20 m     | 20 m or more    |
| ---------- | ----------- | --------------- | -------------- | --------------- |
| a0         | measurement | 8: 0.35 / 0.29  | 5: 2.97 / 2.90 | 2: 2.58 / −1.92 |
| a0         | estimate    | 8: 0.38 / 0.31  | 3: 2.92 / 2.87 | 3: 2.78 / −2.09 |
| a0         | box         | 8: 0.05 / −0.05 | 5: 0.19 / 0.07 | 9: 0.33 / 0.24  |
| medoid     | measurement | 8: 0.49 / 0.41  | 3: 2.75 / 2.71 | 3: 2.61 / −1.80 |
| medoid     | estimate    | 8: 0.49 / 0.36  | 2: 2.94 / 2.92 | 3: 2.86 / −2.03 |
| medoid     | box         | 8: 0.03 / −0.01 | 5: 0.16 / 0.08 | 9: 0.35 / 0.23  |
| obb-centre | measurement | 8: 0.07 / 0.02  | 5: 0.55 / 0.27 | 9: 0.81 / 0.41  |
| obb-centre | estimate    | 8: 0.11 / −0.06 | 5: 0.54 / 0.35 | 9: 0.84 / 0.45  |
| obb-centre | box         | 8: 0.03 / −0.01 | 5: 0.18 / 0.07 | 9: 0.35 / 0.23  |

**By aspect bin** (same cells):

| Arm        | Reading     | 0 to 30° (end on) | 30 to 60°      | 60 to 90° (broadside) |
| ---------- | ----------- | ----------------- | -------------- | --------------------- |
| a0         | measurement | 7: 2.87 / 2.83    | 3: 0.67 / 0.67 | 5: 0.34 / 0.26        |
| a0         | estimate    | 6: 2.87 / 2.82    | 3: 0.63 / 0.62 | 5: 0.35 / 0.27        |
| a0         | box         | 14: 0.26 / 0.16   | 3: 0.18 / 0.01 | 5: 0.05 / −0.05       |
| medoid     | measurement | 6: 2.69 / 2.64    | 3: 0.78 / 0.61 | 5: 0.47 / 0.37        |
| medoid     | estimate    | 5: 2.88 / 2.85    | 3: 0.80 / 0.63 | 5: 0.43 / 0.27        |
| medoid     | box         | 14: 0.25 / 0.17   | 3: 0.18 / 0.01 | 5: 0.03 / −0.03       |
| obb-centre | measurement | 14: 0.67 / 0.36   | 3: 0.23 / 0.06 | 5: 0.04 / 0.01        |
| obb-centre | estimate    | 14: 0.63 / 0.38   | 3: 0.27 / 0.03 | 5: 0.10 / −0.08       |
| obb-centre | box         | 14: 0.26 / 0.16   | 3: 0.12 / 0.01 | 5: 0.03 / −0.03       |

Range and aspect are confounded with the vehicle, as the spec warned. The poses under 10 m are car
8's seven and one of truck 1's, and every end-on pose is a truck. A cell says which vehicle it is as much as which view.

### The stated predictions

The spec stated three predictions before the replays. They are readings, not gates.

1. **Measurement and estimate on the sensor's side of the reference on both trucks, beyond the
   reference bound:** not borne out as stated. On truck 1 the mean and medoid sit 2.7 to 2.9 m
   toward the sensor, beyond the bound on 5 of 8 and 5 of 7 matched truck poses. On truck 2 the
   matched mean and medoid poses sit about 2 m on the far side (negative offsets), and the OBB
   centre is beyond the bound on only 2 (measurement) and 4 (estimate) of 15 truck poses. The
   truck errors are dominated by length, as W9 found, and their sign is consistent with which end
   is seen rather than with the sensor.
2. **The box centre nearer the reference than the measurement on car 8:** borne out under every
   arm (box 0.03 to 0.05 m against measurements of 0.35, 0.47 and 0.06 m).
3. **Across arms, the measurement's offset toward the sensor largest for the medoid and smallest
   for the OBB centre:** borne out (0.48, 0.35, 0.25 m). The OBB centre's median is over all 22
   poses while the others drop their worst trucks, so the ordering is weaker than the numbers
   suggest.

## Interpretation

On the car, the one vehicle whose rows measure position bias cleanly, the mean measurement sits
0.35 m from the body centre, 0.27 m of it toward the sensor. That is the visible-face pull a
cluster position must have, because the far side returns nothing. The medoid pulls further
(0.47 m), and the OBB centre removes most of it (0.06 m). The published box, which places the body
from the solid-body state, sits within 0.05 m on the car under every arm and within 0.2 m on
truck 1. It is not sensitive to which cluster position feeds the filter: its error moves by at most
0.05 m between arms in any cell.

On truck 2 every reading, the box included, is about 1.5 m off at the median. That is the length
the box model still cannot see, the W3 and W9 finding. It is not position bias, and no choice of
cluster position fixes it.

So the identity result and the bias baseline point the same way. Changing the cluster position
trades a fraction of a metre of car bias for identity switches, while the body-level error that
matters for the following gap sits in the box and the truck extents. Later work on the body's
position (geom0012, the near-edge and box levers) should quote its kirk0 centre errors against the
box rows here, at this split, episode and gate.

## Limitations

- **Tuning data only.** kirk0 is a development capture, the split is its tuning role, and the
  OBB-centre and medoid questions were developed on it. Nothing here is held out.
- **Three vehicles in one lane, 22 poses.** Two of the vehicles are trucks whose length every arm
  underestimates. The car's seven rows carry the position-bias reading, and per-cell counts are a
  few poses each, not rates.
- **References are fits to the reviewed returns,** not surveyed bodies; errors inside a reference's
  bound are not resolved. External references are the RTK campaign's job (D-40).
- **Survivorship at the 3 m gate.** The mean and medoid arms' truck medians are over the poses that
  stayed within 3 m; their unmatched counts are part of the result.
- **The L5-to-L3 feedback** that separates `a0`'s and `obb-centre`'s observations is inferred from
  where they diverge, not tested by switching the track guard off.
- **The vertical** is not measured.

## Recommendations

1. Keep the mean as production and the OBB centre opt-in. Close geom0015 without a candidate for
   label0001.
2. Quote kirk0 body-centre errors against the box rows of this table (median 0.19 m over 22 poses;
   car 8 0.05 m, truck 1 0.19 m, truck 2 1.49 m under `a0`). A change that moves the tuning
   fingerprint re-runs this spec's physical step rather than comparing across builds.
3. Drop the "only L5 differs" assumption from future spec rules, or state the track guard's setting
   when an arm changes L5 alone.
4. Spend the next position effort on the truck extents (W3's retention and fragmentation fix), not
   the cluster position.

## Provenance

| Item        | Value                                                                                                                                                                                                                                                                                                             |
| ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Spec        | [geom0015](../../../data/experiments/geom/geom0015.md), rule committed `9cb7b3804` before any replay                                                                                                                                                                                                              |
| Replays     | `lidar-state-estimation-baseline` built at `dd8ef3d41` with the Makefile's LDFLAGS (binary SHA-256 `06f1335c7331921e…`); the tool's and the pipeline's sources are unchanged to `50fb74690`                                                                                                                       |
| Scoring     | `lidar-ground-truth-eval` built at `50fb74690` with the Makefile's LDFLAGS (binary SHA-256 `dbfe2d7827b70f92…`)                                                                                                                                                                                                   |
| Capture     | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`, source `source/v1/72e36168…` (a0), `source/v1/4d0a5d79…` (medoid)                                                                                                                                                               |
| Arms        | `a0`: `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377fa5d356ba…`; `medoid`: `config/experiments/foreground-refine.json`, fingerprint `42e3a582b803`, file SHA-256 `cb37e11e7d96c1ad…`; `obb-centre`: the defaults, fingerprint `ab0c4035780d`, `-measurement-mode obb_centre_v1` |
| Split       | `kirk0-reference-tuning-r1.json`, split digest `sha256:74ddd659346d28a3158db009d3cd6e73437a5d7deee913ccfd243a5e0fad5617`, revision 2, manifest digest `sha256:5df49517…`                                                                                                                                          |
| References  | pack `sha256:67842cb1…`, label revision 2232, physical revision 76 (`sha256:c9b7ca00…`), pinned                                                                                                                                                                                                                   |
| Eligibility | [cohort manifest](../../../config/cohorts/cohort-manifest.json) revision 1 (D-40); a scratch use ledger at migration 70                                                                                                                                                                                           |
| Evidence    | Six evidence databases (about 375 MB each), scores, bundles and the run script under `~/vr-scratch/d2-refresh-20261105/` on the M1 Pro; not in the repository                                                                                                                                                     |
