# The published box takes only measured length from the extent state: kirk0

<!-- ignore-style-length -->

`box_extent_solid_body` (#984) widened car 8's published box on kirk0 from about 1.8 m to 2.3 to
2.6 m, against a 1.73 m reference. A sensor beside the road sees one side of a passing car, so the
far side of that width came from the class prior and from one-sided bounds. The new qualifier
`box_extent_measured_length` lets the box take a dimension from the extent state only once the
state has measured it. In practice that means the length. This run scores it on geom0015's 22
reviewed poses and on kirk0's identity labels. It answers
[geom0017](../../../data/experiments/geom/geom0017.md).

- **Status:** Measured on kirk0, 2026-11-09. G1 fails, by 0.005 m. G2 and G3 pass. Nothing
  adopted, no default changed
- **Spec:** [geom0017](../../../data/experiments/geom/geom0017.md), rule committed at `d2dd11ad`,
  before any replay
- **Layers:** L5 (the published box), and L3 and L4 through the track guard. Scored against L10
  physical references and the reviewed identity labels
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`, for all three arms. The options are replay experiments, not tuning keys
- **Related:** [geometry convergence plan §4.4](../../plans/lidar-tracker-geometry-convergence-plan.md#44-published-box-from-the-solid-bodys-extent-state-default-off),
  [geom0015's bias baseline](d2-frozen-split-bias-baseline-2026-11.md),
  [kirk0's labels in a cloud run](cloud-run-kirk0-labelled-2026-11.md)

## Findings

- **The qualifier removes #984's car 8 fault completely.** With it, car 8's seven poses score
  exactly as `a0`'s do, pose for pose: median centre error 0.04 m and widths 1.78 to 1.90 m.
  Under `box-extent` the same poses read 0.29 m and 2.32 to 2.56 m.
- **The overall median is 0.195 m against `a0`'s 0.190 m, so G1 fails.** One truck 1 pose
  carries the whole difference: sample 495 goes from 0.189 m to 0.205 m and becomes a median pose.
  `box-extent` shows the same change on truck 1, pose for pose, so the qualifier did not cause it.
  It comes from the length the box now takes from the state, and from the L3 track guard carrying
  the changed box into the next frames' clusters. l5id0019's R3 bar for moving the box is 0.05 m.
- **Identity does not move against `box-extent`.** HOTA 0.4297, IDF1 0.4740 and 88 switches, the
  same figures as `box-extent`. Its paired-bootstrap point figures sit inside `a0`'s intervals,
  so G3 passes.
- **Truck lengths do not move, as predicted.** The median absolute truck length error is 0.15 m
  in all three arms. Truck 2 still reads 6.59 m against 9.63 m: the extent state has no longer
  length to hand over until tracking fix 3.

## Method

- **Build.** Tools built at `d2dd11ad` with the Makefile's LDFLAGS and the `pcap` tag. Every
  replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `d2dd11ad…`. Base: main
  `704b8e46`, which differs from l5id0019's `fe6549b3` only in `docs/DEVLOG.md`.
- **Replays.** `lidar-state-estimation-baseline` on kirk0 whole as a one-case corpus:
  `-warmup 6 -require-settled=false -sample-points 1024`, evidence per case, the committed cohort
  manifest and a scratch use ledger at migration 71. This is l5id0019's replay step without
  `-campaign-metrics`. Each arm's repeat pass wrote an identical tracking baseline.
- **Scoring.** `lidar-ground-truth-eval perframe` on the reconstructed reviewed pack, with the
  physical references re-pinned to its digest as l5id0019 scored them. The split was redrafted
  with `-from-seconds 6 -split kirk0-tuning`, and its manifest digest, `7194bb61…`, equals
  l5id0019's. `obj_788622b5` was ignored, and online estimates were scored as declared
  baselines, each arm against `a0`. Identity used `-bootstrap 2000 -bootstrap-seed 20261108`. The
  box rows used `-physical-reference -a-boxes -b-boxes` with a 3 m gate. Then
  `lidar_noise_screen.py` and `lidar_flip_cost.py --match footprint` ran on the evidence.
- **Box counters.** From the kirk0 wiring test `TestBoxExtentMeasuredLengthOnKirk0`. It replays
  a different window (from 20 s, 20 s warm-up, 256 sample points), so its counts describe the
  option's reach, not the scored replay.

**The anchor (rule 1).** `a0`'s tracking baseline digest is `79d9abff…`, as l5id0019 recorded,
and its box rows equal l5id0019's: 22 matched, median 0.190 m, car 8 0.04 m. `box-extent`'s
parameter hash, `7c8a88e85dc1`, equals l5id0019's arm of that name. Its box rows equal
l5id0019's too: median 0.281 m, car 8 0.29 m.

## Geometry: the published box on the 22 reviewed poses

Gate 3 m. Each vehicle cell gives the poses matched, then the median centre error and the median
offset toward the sensor (m), then the median published length and the range of published widths.

| Arm               | Matched | Median (m) | Truck 1 (10.63 × 2.91)                | Car 8 (4.26 × 1.73)                  | Truck 2 (9.63 × 2.84)                | IoU   | Yaw  |
| ----------------- | ------: | ---------: | ------------------------------------- | ------------------------------------ | ------------------------------------ | ----- | ---- |
| `a0`              | 22 / 22 |      0.190 | 9: 0.19 / +0.15; L 10.60; W 2.84–3.11 | 7: 0.04 / −0.03; L 4.40; W 1.78–1.90 | 6: 1.49 / +0.25; L 6.59; W 1.09–2.92 | 0.887 | 1.2° |
| `box-extent`      | 22 / 22 |      0.281 | 9: 0.19 / +0.13; L 10.62; W 2.75–3.11 | 7: 0.29 / −0.29; L 4.40; W 2.32–2.56 | as `a0`                              | 0.802 | 0.8° |
| `measured-length` | 22 / 22 |      0.195 | as `box-extent`                       | as `a0`, pose for pose               | as `a0`                              | 0.887 | 0.8° |

Truck 1 changes the same way under both solid-body arms, on four of its nine poses: sample 479
0.161 to 0.133 m, 494 0.269 to 0.277 m, 495 0.189 to 0.205 m and 504 0.332 to 0.339 m. The poses
either side of the median were 0.1891 and 0.1906 m under `a0`, and are 0.1906 and 0.1987 m under
`measured-length`, so the 0.016 m rise at sample 495 alone carries the median from 0.1899 to
0.1947 m. Truck 1's published widths at those poses fall, not rise (2.90 to 2.75 m at sample
495), and its yaw error falls (1.2° to 0.1°). This is the box's own width belief along a changed
heading, not the extent state's width.

## Identity

Full comparison over all hypotheses, then the paired bootstrap against `a0`: 19 scored objects,
2,000 draws.

| Arm               |   HOTA |   IDF1 | Switches | Frag |  FN | Tracks | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches  | Bootstrap point HOTA, IDF1, switches |
| ----------------- | -----: | -----: | -------: | ---: | --: | -----: | --------------------------- | --------------------------- | ----------- | ------------------------------------ |
| `a0`              | 0.4284 | 0.4728 |       90 |   46 | 265 |     79 |                             |                             |             | 0.4798, 0.5582, 90                   |
| `box-extent`      | 0.4297 | 0.4740 |       88 |   46 | 265 |     78 | +0.0001 / +0.0016 / +0.0032 | +0.0002 / +0.0015 / +0.0029 | −6 / −2 / 0 | 0.4815, 0.5598, 88                   |
| `measured-length` | 0.4297 | 0.4740 |       88 |   46 | 265 |     78 | +0.0001 / +0.0016 / +0.0032 | +0.0002 / +0.0015 / +0.0029 | −6 / −2 / 0 | 0.4815, 0.5598, 88                   |

`a0`'s 5th-to-95th intervals: HOTA 0.4111 to 0.5415, IDF1 0.5107 to 0.6232, switches 4 to 242.
The two solid-body arms write different tracking baselines (`186d30d4…` and `ff6e460d…`), but
those differences do not reach a scored object. Read by l5id0019's R1, both arms gain a little
on HOTA and IDF1 and are non-inferior.

## Label-free and the box counters

`lidar_noise_screen.py` gives 12 brief tracks and 0.7025 spurious clusters a frame for both
solid-body arms, against 12 and 0.6947 for `a0`. `lidar_flip_cost.py` against `a0`: 38 moving
tracks matched, none lost, shortened or added, and a median absolute p85 speed difference of
0.0 m/s.

From the kirk0 wiring test, associated frames under `box_extent_solid_body` are 990 that took the
state (551 of them the width), 1,180 with no state to read, 3,459 refused on axis and 1,185
unsettled. With the qualifier, 730 took the state (24 the width), and 534 kept a dimension the
state had settled from one side. The median width at each vehicle track's longest box falls from
2.53 m to 2.01 m. The 24 remaining width frames are frames on which both side faces had updated
the width, with opposed normals. Which vehicles they belong to was not read.

## Reading against the rule

| Gate                                                | Result                             | Outcome  |
| --------------------------------------------------- | ---------------------------------- | -------- |
| G1: median centre error at most 0.190 m, 22 matched | 0.195 m, 22 matched                | **Fail** |
| G2: car 8 within 0.05 m of `a0`'s 0.04 m            | 0.04 m, every pose equal to `a0`'s | Pass     |
| G3: bootstrap HOTA, IDF1 and switches inside `a0`'s | 0.4815, 0.5598, 88                 | Pass     |

Under rule 4 the failing gate is G1, so `box-extent` and the qualifier both stay off and neither is
named for label0003's held-out window. The rule was written to the third decimal, and the
measured miss is one truck pose changing which pose sits at the median, a change that
`box-extent` makes as well. G1 has been read as written, not restated.

## Interpretation

Measured: the qualifier does what it was built to do. Car 8's box is `a0`'s again, and every
figure that differs between `box-extent` and `measured-length` moves toward `a0`. Overall median
0.281 to 0.195 m, car 8 0.29 to 0.04 m, IoU 0.802 to 0.887, identity unchanged. Inferred: what
is left of the difference from `a0` is the effect of the box's length from the state through
the track guard. On truck 1 it shortens the box's width and improves its yaw, and it nudges the
centre either way by up to 0.03 m. Since `box_extent_solid_body` is to be read at all,
`measured-length` is the form to read it in.

## What this does not establish

- **Anything off kirk0 or held out.** kirk0 is one tuning capture, and the 22 poses are three
  vehicles. Car 8 is the only car.
- **That the length from the state helps.** On these poses the frame's span floor already
  exceeds the state's truck lengths, so the length it takes changes little. Its value waits on
  tracking fix 3, which should lengthen the state.
- **Where the 24 width frames come from.** A vehicle turning past the sensor, or a heading error
  that the 135° normal test does not catch, would both produce them. They were not traced.
- **Whether an end-on span should count as measuring the width.** That waits on gating the width
  bounds on a converged heading.
- **Pi cost.** No timings were taken.

## Recommendations

1. Keep both options off. Under geom0017 neither goes to the held-out read.
2. Score `box_extent_solid_body` with `box_extent_measured_length` from now on, never alone:
   #984's width is a known fault, and the qualifier removes it without costing identity.
3. Re-read the pair after tracking fix 3, in a new spec, with G1 at l5id0019's R3 bar (0.05 m)
   rather than at the third decimal, and with truck 2's length as the measure that decides.
4. Trace the 24 remaining width frames before any width is taken from the state.

## Provenance

| Item            | Value                                                                                                                                                                                                           |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base            | main `704b8e46`; code at `5d7c5016`; spec committed at `d2dd11ad` (`d2dd11ad9506900b7f5c2c0228bdc579e20e1391`), which changes no tool source                                                                    |
| Tools           | Built at `d2dd11ad` with the Makefile's LDFLAGS and the `pcap` tag: `lidar-state-estimation-baseline` `sha256:1b62df29…`, `lidar-ground-truth-eval` `sha256:0aeeb923…`                                          |
| Parameter set   | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`                                                                                                                                                       |
| Arm hashes      | `a0` `ab0c4035780d`; `box-extent` `7c8a88e85dc1`; `measured-length` `edebbd41584e` (each folds the replay experiments into the tuning hash)                                                                     |
| Tracking digest | `a0` `79d9abff…`, `box-extent` `186d30d4…`, `measured-length` `ff6e460d…`; each arm's repeat equal                                                                                                              |
| Capture         | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`, equal to the repository's LFS pointer                                                                                         |
| Labels          | label set `kirk0-reference` revision 2233; pack `ds_67842cb1b22b70e7` reconstructed (pack digest `sha256:f0ed3414…`); physical references revision 77, content `sha256:189ecf65…`, re-pinned copy as l5id0019's |
| Split           | drafted, `kirk0-tuning`, episode `kirk0-tuning-from-61`, `obj_788622b5` ignored, manifest `7194bb61…`, reference digest `sha256:a784e69d…`                                                                      |
| Eligibility     | committed cohort manifest; a scratch use ledger at migration 71; actor `appius-geom0017`                                                                                                                        |
| Host            | Claude Code cloud container, 4 cores shared with seven other sessions; raw output in the session scratch, deleted once summarised                                                                               |
