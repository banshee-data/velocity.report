# A birth rule for the robust likelihood cost's fragments, on kirk0

<!-- ignore-style-length -->

- **Status:** Measured on kirk0 on 2026-11-09. The rule passes. Nothing is adopted and no default
  changes
- **Spec:** [l5id0021](../../../data/experiments/l5id/l5id0021.md). Its rule was committed and
  pushed at `52626db7`, before any scored replay
- **Option:** `TrackerConfig.FragmentBirthGate`, replay experiment `robust_birth_gate` (commit
  `54dc1b7e`). It is default-off, bit-identical when off, and refused without
  `robust_likelihood_cost`
- **Parameter set:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`. Every arm differs only by replay experiments
- **Related:** the S3 section of the
  [gap analysis](../../../data/maths/paper-implementation-gap-analysis.md#computed-s3), and the
  November benchmark (PR #994, branch `claude/lucid-bardeen-ux375z-benchmark-nov8`, spec
  l5id0019), which ranks this fix second among the tracking fixes

## Summary

- **What robust's extra brief tracks are.** On kirk0 the robust likelihood cost raises brief
  confirmed tracks from 12 to 16 and clutter from 15 to 19. Twelve of the 16 brief tracks have a
  twin in `a0`. Of the other four, two are `a0` identity errors that robust corrects. One is a
  contest between a newborn and a confirmed track on the same body. One is a fragment that the
  fragment guard refuses to a coasting vehicle track. Under `a0` a duplicate track used to take
  that fragment; robust removes the duplicate, so the fragment seeds a track of its own.
- **The rule.** `robust_birth_gate` applies to an unassociated cluster that the fragment guard
  refuses to some confirmed track, when the cluster lies inside that track's drawn box. The box
  is grown by 0.25 m plus the track's position sigma, at most 1 m. Such a cluster seeds no track.
- **It passes l5id0021's rule on kirk0.** Identity keeps robust's gain over `a0`: HOTA +0.0126
  (5th percentile +0.0032), IDF1 +0.0183 (+0.0055), switches 90 to 6. Brief tracks are 14 and
  clutter 16, within l5id0019's bounds of `a0`'s 12 and 15. Duplicate pairs stay at robust's 8,
  and no moving track is lost.
- **It removes exactly the predicted tracks.** Against robust it removes three clutter tracks and
  adds none. The diagnosis named all three: two brief fragments and a 3.4 s static artefact
  inside a car's box. Identity does not move against robust: no paired interval excludes zero.
- **It does not reach `a0`'s level strictly.** Two brief tracks remain above `a0`'s. One is a
  real mover that robust tracks correctly for 0.96 s. The other is the contest, which is an
  assignment between two tracks, not a birth.
- **Plain `likelihood_cost` does almost all of robust's identity work on kirk0.** It gives HOTA
  0.4360 against robust's 0.4362, the same 6 switches, and 14 brief tracks. On this capture the
  `ln det S` term acts; the Student-t tail adds little.

## Question and rule

The robust cost (#983) is the strongest identity lever the November benchmark measured. It
raised brief tracks and clutter at four of five cases, so that benchmark did not name it for a
held-out read. l5id0021 asks what those tracks are born from. It then asks whether a birth rule
aimed at that population keeps the identity gain and brings brief tracks and clutter back within
`a0`'s.

The rule, in short ([spec](../../../data/experiments/l5id/l5id0021.md)):

- **J1, identity kept.** `robust-birth` is compared with `a0` by paired bootstrap, 2,000 draws,
  seed 20261108. The 5th percentiles of the HOTA and IDF1 deltas must both be above 0.
- **J2, noise within `a0`'s.** Brief tracks may rise by 3 at most, and clutter by 5 per hundred of
  `a0`'s moving tracks at most. These are l5id0019's R4 bounds. Duplicate pairs per hundred
  vehicle tracks may not rise above `a0`'s.
- **J3, falsified.** The rule is falsified if duplicates rise above robust's, or if a lasting
  moving track is lost against robust.

## The diagnosis, before the rule

A scratch tracker tap, which was not committed, replayed `a0` and `robust` on kirk0 whole with
l5id0019's settings. It recorded every cluster, every pairing that reached the cost matrix with
its `d²` and `ln det S`, and every track on every frame. Robust's brief count reproduced
l5id0019's 16. Each brief track was matched to `a0` by birth frame (within two) and birth cluster
(within 1.5 m). Every one of `a0`'s 12 has a twin in robust. Robust's four without a twin:

| Case          | Birth cluster                                          | Under `a0`                                                                                            | Under robust                                                                                   | Verdict                    |
| ------------- | ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | -------------------------- |
| Static return | 0.44 × 0.28 m, 8 points, 11.7 m out                    | Born too. After 18 coasted frames its track took a live car's 1.47 × 1.07 m cluster; it lasted 10.4 s | The car keeps its cluster; the artefact's track dies at 0.55 s                                 | Robust is right            |
| Mover, 1 s    | 3.9 × 0.75 m, 90 points                                | Born too, but an older coasting track (`ln det S` 0.22) took its clusters; it never confirmed         | The young track (`ln det S` −4.57) keeps them and confirms; the body then leaves               | Robust is right            |
| Contest       | A split of a confirmed track's body                    | Born too; the confirmed track won the later contest by `d²`                                           | The newborn won the body's last two clusters, so the confirmed track lasted 0.87 s, not 1.07 s | An assignment, not a birth |
| Fragment      | 0.27 × 0.19 m, 0.22 m from a 2.45 m track's prediction | The fragment guard refuses it to that track; a duplicate on the same body took it                     | No duplicate, so it seeds a track: 4 hits, confirmed, 0.31 s                                   | The target                 |

Two broader birth rules were screened on the same taps and rejected before the spec:

- **No birth inside any confirmed track's grown box.** On robust this would remove 45 births, 11
  of them lasting tracks. Among those are vehicles born inside a long-coasting track's box
  (position sigma up to 3.7 m) and two vehicles of 190 and 271 points beside a truck.
- **No birth from a cluster another track's gate admitted.** This would remove 14 births on
  robust, 3 of them lasting tracks. All three move, by 16, 23 and 26 m. The spec says "two of them
  movers"; by displacement it is all three.

One population fitted: births from a fragment (longest extent under the guard's 0.5 m) inside the
grown box of a confirmed track that believes itself 2 m or more. Robust had 8 such births and
confirmed 3 of them: two brief, and one static artefact of 3.36 s that is near-static clutter.
`a0` had 6 and confirmed 1, a brief one. None became a lasting moving track in either arm.

## Method

- **Replays.** `lidar-state-estimation-baseline` on kirk0 whole as a one-case corpus, with
  `-warmup 6 -require-settled=false -sample-points 1024`, evidence per case, the committed
  cohort manifest and a scratch use ledger at migration 71. Every arm's repeat pass reproduced
  its first pass's tracking baseline byte for byte.
- **Boxes.** `lidar-box-eval -variants none -rows -warmup 6 -duration 0 -require-settled=false`,
  for duplicate pairs over 771 scored frames.
- **Identity.** `lidar-ground-truth-eval perframe` on the reconstructed reviewed pack,
  `kirk0-tuning` split, `obj_788622b5` ignored, online estimates as declared baselines. Each arm
  was compared with `a0`, and `robust-birth` also with `robust`, using `-bootstrap 2000
-bootstrap-seed 20261108`.
- **Label-free measures.** `lidar_noise_screen.py`, `lidar_clutter_origin.py`, and
  `lidar_flip_cost.py --match footprint` and `lidar_figure_sensitivity.py` against the reference
  arm, all on the evidence.
- **The anchor.** `a0` reproduces l5id0019 in every figure checked. Tracking baseline
  `79d9abff…`, HOTA 0.4284, IDF1 0.4728, 90 switches; 79 tracks, 12 brief, 0.6947 spurious
  clusters a frame, 537 unowned; 44 vehicle tracks, 10 duplicate pairs, 452 pair-frames.
  `robust` reproduces l5id0019's robust row in every figure too: HOTA 0.4362, 16 brief, clutter
  19 (8 / 1 / 3 / 7), 8 pairs, 246 pair-frames.

## Identity on kirk0's labels

These are full-comparison figures, then the paired bootstrap (19 scored objects, 2,000 draws).

| Arm            |   HOTA |   IDF1 | Switches | Frag |  FN | Tracks | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches      | Δ frag         |
| -------------- | -----: | -----: | -------: | ---: | --: | -----: | --------------------------- | --------------------------- | --------------- | -------------- |
| `a0`           | 0.4284 | 0.4728 |       90 |   46 | 265 |     79 |                             |                             |                 |                |
| `robust`       | 0.4362 | 0.4827 |        6 |   30 | 247 |     81 | +0.0023 / +0.0115 / +0.0222 | +0.0032 / +0.0166 / +0.0338 | −226 / −80 / −4 | −42 / −16 / −2 |
| `robust-birth` | 0.4370 | 0.4841 |        6 |   30 | 247 |     78 | +0.0032 / +0.0126 / +0.0228 | +0.0055 / +0.0183 / +0.0337 | −226 / −80 / −4 | −42 / −16 / −2 |
| `likelihood`   | 0.4360 | 0.4823 |        6 |   30 | 247 |     80 | +0.0021 / +0.0114 / +0.0220 | +0.0026 / +0.0161 / +0.0333 | −226 / −80 / −4 | −42 / −16 / −2 |

`robust-birth` against `robust`: HOTA +0.0010 (−0.0003 to +0.0026), IDF1 +0.0018 (−0.0004 to
+0.0043), switches and fragmentations 0 in every draw. No interval excludes zero. The gate costs
no identity on kirk0 and adds none it can measure. The bootstrap's labelled-object HOTA is 0.4927
for `robust-birth`, against 0.4917 for robust and 0.4798 for `a0`.

## Noise, duplicates and counts

| Arm            | Tracks | Brief | Near-static | Clutter (artefact / stationary / fragment / unclassed) | Spurious a frame | Unowned | Moving (lost / short / added vs `a0`) | p85, p98 (m/s) | Vehicle tracks | Dup pairs (per 100) | Pair-frames | Births absorbed |
| -------------- | -----: | ----: | ----------: | ------------------------------------------------------ | ---------------: | ------: | ------------------------------------- | -------------- | -------------: | ------------------- | ----------: | --------------: |
| `a0`           |     79 |    12 |           3 | 15 (6 / 2 / 3 / 4)                                     |           0.6947 |     537 | 38                                    | 7.06, 8.83     |             44 | 10 (22.7)           |         452 |               — |
| `robust`       |     81 |    16 |           3 | 19 (8 / 1 / 3 / 7)                                     |           0.6856 |     529 | 41 (0 / 0 / 0)                        | 6.99, 8.83     |             43 | 8 (18.6)            |         246 |               — |
| `robust-birth` |     78 |    14 |           2 | 16 (7 / 1 / 2 / 6)                                     |           0.7206 |     556 | 41 (0 / 0 / 0)                        | 7.01, 8.83     |             43 | 8 (18.6)            |         246 |              43 |
| `likelihood`   |     80 |    14 |           3 | 17 (8 / 1 / 3 / 5)                                     |           0.6831 |     527 | 41 (0 / 0 / 0)                        | 7.01, 8.83     |             43 | 8 (18.6)            |         246 |               — |

The births absorbed figure is `fragment_births_absorbed` in the run's continuity. It counts
frames, not fragments: a recurring scrap is absorbed again on each frame it appears.

**Against `robust`, track by track.** Three of robust's clutter tracks have no counterpart under
`robust-birth`, matched by start within 0.25 s and first row within 1.5 m:

- a brief track at 7.0 s, 0.30 × 0.07 m;
- a near-static artefact at 34.5 s, 0.19 × 0.13 m, lasting 3.36 s;
- a brief track at 56.4 s, 0.29 × 0.21 m.

These are the three tracks the diagnosis predicted. `robust-birth` has no clutter track that
robust lacks. Against `a0`, the one clutter track `robust-birth` adds is the 0.96 s mover (3.2 ×
1.2 m).

**Spurious clusters rise.** The figure goes from 0.6856 to 0.7206 a frame, and unowned clusters
from 529 to 556. This follows from the rule: an absorbed fragment is still a cluster, but no track
now takes it as a measurement, so the screen counts it as spurious. The figure measures what no
track explains, and it now includes the scraps the rule chose not to explain with a track.

## Against the rule

| Clause               | Reading                                                                          | Holds?               |
| -------------------- | -------------------------------------------------------------------------------- | -------------------- |
| J1, identity         | HOTA 5th +0.0032, IDF1 5th +0.0055                                               | Yes                  |
| J2, brief            | 14 against 12: +2, within +3                                                     | Yes                  |
| J2, clutter          | 16 against 15: +1, which is 2.6 per hundred of `a0`'s 38 moving tracks, within 5 | Yes                  |
| J2, duplicates       | 8 pairs, 18.6 per hundred, against `a0`'s 10 and 22.7                            | Yes                  |
| J3, duplicates back  | 8 pairs and 246 pair-frames, equal to robust's                                   | Not falsified        |
| J3, lost movers      | 0 lost, 0 shortened against robust by footprint                                  | Not falsified        |
| Strict, not deciding | Brief 14 > 12 and clutter 16 > 15                                                | No: two tracks above |

`robust-birth` passes. By the spec, a pass names it for the placements' label-free read and for
label0003's held-out window, in robust's place.

The counts clause of l5id0019's R4 is not part of this rule, but it should be stated. The moving
count is 41 against `a0`'s 38, as under robust: 7.9 %, over R4's 5 %. No moving track is lost,
shortened or added by footprint. Robust carries the same vehicles in more pieces, and the gate
does not change that.

## Interpretation

Each reading is marked measured or inferred.

- **Robust's brief tracks are mostly not a birth defect.** Measured: two of the four tracks
  without an `a0` twin are robust being right. In one, `a0`'s coasting artefact took a car's
  cluster. In the other, an older coasting track took a young mover's clusters. Both are the S3
  discount, and robust removes it. The tracks are brief because the objects were seen briefly. A
  birth rule that removed them would remove a correct track.
- **The defect is a fragment-guard interaction that robust exposes.** Measured: under `a0` the
  scrap the guard refuses to a vehicle track was taken by a duplicate track on the same body.
  Robust removes the duplicate, so the scrap seeds a track. Inferred: the guard's design note
  ("so the fragment stays unassociated and can seed its own track") assumed something would take
  the scrap. Under `d²` that was often a duplicate.
- **Absorption is a narrow rule, and kirk0 does not show a cost for it.** Measured: no identity
  change against robust and no lost mover. Inferred: the guard's own condition (a cluster under
  0.5 m beside a track believed to be 2 m or more) keeps pedestrians out of it, except one partly
  hidden by a vehicle. kirk0's labels have no such case.
- **The contest is an assignment problem.** Inferred: a newborn with `d²` 0.03 and `ln det S` 4.72
  beat a confirmed track with `d²` 19 and `ln det S` −4.86 on one body. That is S2, not S3.
  Tentative priority, or the cascade, under robust is the zero-code control to try; it was not an
  arm here.
- **Which half of the robust cost acts.** Measured on kirk0: plain `likelihood_cost` gives the
  same switches and duplicates and nearly the same HOTA. It has 14 brief tracks against robust's 16. Inferred: the `ln det S` term does the identity work here, and the Student-t tail mainly
  changes which contested young tracks confirm. This is one capture. The benchmark's placements
  should be read with both costs before choosing between them.

## Recommendations

1. **Read `robust-birth` on the placements** as l5id0019 read them: the first two five-minute
   segments of ashbury-downey and haight-divisadero, under R4. The benchmark found robust's
   clutter over its bound at four of five cases. The question is whether this gate brings it
   within the bound off kirk0, where the population was not seen before the rule.
2. **Read `likelihood_cost` with the gate as well.** The gate only reads the fragment guard and a
   box, so it applies to either cost. It is registered against robust because that is where the
   population was found. If the placements confirm that the Gaussian likelihood carries the
   identity gain, extending `robust_birth_gate`'s prerequisite to `likelihood_cost` is a
   one-line change with its own test.
3. **Try tentative priority under robust for the contest** before writing anything new.
4. **Keep it default-off.** Nothing here is held out, and kirk0 is the capture the rule was shaped
   on.

## What this does not establish

- **Anything off kirk0.** The population, the one-metre sigma bound and the rule all come from
  kirk0's taps. A pass here is in-sample.
- **Physical truth for the absorbed scraps.** They are 0.2 to 0.3 m returns inside a vehicle's
  box. Nothing here shows what each one was.
- **A cost on pedestrians.** kirk0's labels have no pedestrian partly hidden by a vehicle. J1 can
  see neither that cost nor its absence.
- **The robust cost's own case.** The robust cost still costs counts on kirk0 (+3 moving tracks).
  The two cases where robust corrects `a0` are read from the taps, not scored against labels
  track by track.
- **Pi cost.** The gate is one pass over the tracks for each unassociated cluster, and only when
  the option is on. It was not timed.

## Provenance

| Item          | Value                                                                                                                                                                                                                                                                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Base          | main `704b8e46`; option `54dc1b7e`; spec `52626db7`, pushed before the scored replays                                                                                                                                                                                                |
| Off digests   | `robust_birth_gate_off_test.go`, ten association digests taken on `704b8e46` before the option existed                                                                                                                                                                               |
| Tools         | Built at `52626db7` with the Makefile's LDFLAGS and the `pcap` tag. Every replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `52626db7…`. `lidar-state-estimation-baseline` `sha256:5b948e40…`, `lidar-box-eval` `bb8d123b…`, `lidar-ground-truth-eval` `7697afb1…` |
| Parameter set | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`                                                                                                                                                                                                  |
| Arm hashes    | `a0` `ab0c4035780d`; `robust` `0fc4dbd7c131`; `robust-birth` `e9db12b214d1`; `likelihood` `a492dc0b48b0` (each folds its replay experiments into the tuning hash)                                                                                                                    |
| Baselines     | Tracking baseline digests, first and repeat equal: `a0` `79d9abff…`, `robust` `e81d487e…`, `robust-birth` `dee8eb94…`, `likelihood` `2158da09…`                                                                                                                                      |
| Capture       | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`, equal to the repository's LFS pointer                                                                                                                                                              |
| Labels        | The l5id0019 reconstruction: label set `kirk0-reference` revision 2233, pack `ds_67842cb1b22b70e7` rebuilt by `lidar-pack-rebuild`; split `kirk0-tuning`, `obj_788622b5` ignored                                                                                                     |
| Raw output    | Scratch only. Evidence databases, rows and tap dumps were deleted once these tables were written                                                                                                                                                                                     |
