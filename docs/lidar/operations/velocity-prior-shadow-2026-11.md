# The gated birth prior refused in a confirmed track's shadow, screened on kirk0 and six lot-drawn segments

<!-- ignore-style-length -->

l5est0010 kept l5est0009's honest birth velocity variance in the update and gated a newborn as
shipped. The first second's along-track excess went, but invented speed (rows over 15 m/s at
columbus-broadway and franklin-mcallister) and duplicates stayed. l5est0006 traced invented speeds
under two-point initialisation to thin clusters on a nearer car's bearing beyond its range, and its
shadow test declines them. This run gives the births in a confirmed track's shadow the shipped
prior and every other birth the gated wide one. It reads the form on kirk0 and l5est0009's six
lot-drawn first segments, against l5est0009's rule and l5est0010's falsifier, committed before the
option existed. It answers [l5est0011](../../../data/experiments/l5est/l5est0011.md).

- **Status:** Screen complete, 2026-11-10. Fails V3 and V5; the falsifier does not hold; nothing
  adopted
- **Layers:** L5 (track initialisation and association), read by L8 analytics and the box rows
- **Related:** [gated birth prior (l5est0010)](velocity-prior-gated-2026-11.md),
  [honest velocity prior (l5est0009)](velocity-prior-init-2026-11.md),
  [two-point initialisation guards (l5est0006)](two-point-init-guards-2026-11.md),
  [tracking maths §2.3](../../../data/maths/tracking-maths.md#honest-velocity-prior-at-birth)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) (`ab0c4035780d`) for both
  arms; `shadow` adds the replay experiment `velocity_prior_init_7p5_gated_shadow` (replay
  parameter hash `c6dfefb6…`)

## Answer

**The shadow form fails V3 and V5, and the falsifier does not hold: columbus-broadway's invented
speed was a shadow birth, franklin-mcallister's was not.** kirk0 now passes V4 (HOTA delta 5th
percentile −0.0008, switch change 95th percentile 0), and columbus-broadway, 1st-mission and the
other two passing segments read as `a0` on speed. But the shadow test is far wider than reflections:
it touched 37 % to 71 % of each case's births, so the first second gives back half of l5est0010's
gain (along z RMS 2.244 to 1.533, −31.7 %, against a bar of 30 %). Franklin-mcallister still writes
33 rows over 15 m/s and its p98 rises by 2.97 m/s, fulton-divisadero's by 0.70, and duplicate pairs
per hundred vehicle tracks still rise on all six segments, by less than under `gated` on five.

| Clause                     | Deciding figures (`shadow`; l5est0010's `gated` and l5est0009's `prior` in brackets)                                                                                                                        | Result                     |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- |
| V1, the first second       | along z RMS at age under 1 s: `a0` 2.244, `shadow` 1.533 (0.787; 0.802), −31.7 % (−64.9 %; −64.3 %); bootstrap 95th percentile 1.585 (0.815; 0.835), under `a0`'s 2.244                                     | **Pass**, by 1.7 points    |
| V2, after the first second | along after 1 s 0.802 → 0.792 (0.781; 0.764), −0.010; across after 1 s 0.367 → 0.365 (0.367; 0.359), −0.002                                                                                                 | **Pass**                   |
| V3, no invented speed      | p98 change over 0.5 m/s at fulton-divisadero +0.70 (+0.70; +2.66) and franklin-mcallister +2.97 (+2.36; +2.37); new rows over 15 m/s at franklin-mcallister only, 33 (32; 92); columbus-broadway 0 (34; 35) | **Fail** on 2 of 6 (4; 4)  |
| V4, identity               | kirk0 paired bootstrap: HOTA delta 5th percentile −0.0008 (−0.0085; −0.0171), IDF1 −0.0009 (−0.0237; −0.0383), bars −0.005; switch change 95th percentile 0 (+24; +85), bar +10                             | **Pass** on all three bars |
| V5, noise                  | noise costs on 6 of 6 segments (6; 6), bar two at most; the duplicate condition alone on every one                                                                                                          | **Fail**                   |
| Falsifier                  | new rows over 15 m/s remain at franklin-mcallister (33) but not at columbus-broadway (0); the bar needs both                                                                                                | **Does not hold**          |

**Recommendation:** keep all three birth-prior options off. The shadow test explains the one fast
track at columbus-broadway and buys kirk0's identity back, but it does so by refusing the wide prior
to most births, and it leaves franklin-mcallister's fast tracks and the duplicate rise where they
were. Franklin-mcallister's fast tracks need reading on their own before another birth rule.

## Rule

Committed in the spec (`cbebf2c`) before the option existed: l5est0009's V1 to V5 and l5est0010's
falsifier, unchanged, read for `shadow` against `a0`.

- **V1.** Pooled first-second medoid along-track z RMS falls by 30 % or more from `a0`'s, with the
  candidate's bootstrap 95th percentile under `a0`'s point figure.
- **V2.** After 1 s, along-track rises by no more than 0.05 and across-track moves by no more than
  0.05.
- **V3.** On each segment, p98 no more than 0.5 m/s above `a0`'s and no row over 15 m/s where `a0`
  writes none.
- **V4.** On kirk0, HOTA and IDF1 delta 5th percentiles above −0.005 and the switch change's 95th
  percentile +10 or less.
- **V5.** Noise costs under R4 (peak test) on no more than two of the six segments.
- **Falsifier.** If V3's new fast rows remain at both columbus-broadway and franklin-mcallister,
  the invented speed is not a shadow birth and the shadow test is not the lever.
- **Outcome.** A pass names `shadow` for the second segments and label0003's spans. Nothing is
  adopted.

## Arms and the option

| Arm      | Parameter set            | Replay experiments                     | Birth velocity variance in the update                       | Tentative track's gate and cost read                           |
| -------- | ------------------------ | -------------------------------------- | ----------------------------------------------------------- | -------------------------------------------------------------- |
| `a0`     | defaults, `ab0c4035780d` | none                                   | 1 m²/s² (shipped)                                           | the track's P                                                  |
| `shadow` | defaults, `ab0c4035780d` | `velocity_prior_init_7p5_gated_shadow` | 1 m²/s² in a confirmed track's shadow, (7.5 m/s)² elsewhere | the track's P in the shadow, the shipped prior's shadow P else |

`TrackerConfig.VelocityPriorShadowRefusal` (commit `29c98de`), with `VelocityPriorGated`, asks at
each birth whether the founding measurement lies in a confirmed track's shadow: `inConfirmedShadow`,
the test l5est0006's two-point guard reads, true when the point lies on a bearing between a
confirmed box's corner bearings and farther than its nearest corner. Such a birth keeps the shipped
founding covariance and carries no gate shadow, so it starts as `a0` starts it; every other birth is
founded as under `velocity_prior_init_7p5_gated`. The decision is made once, at birth, against the
confirmed boxes as they stand after that frame's updates. `ContinuityStats.ShadowPriorBirths`
counts the births it touched; it is omitted from JSON when zero, so other manifests are unchanged.
The replay experiment brings `VelocityPriorGated` and `VelocityPriorInit` with it and is refused
with `two_point_velocity_init`, as the gated form is. `velocity_prior_init_7p5` and
`velocity_prior_init_7p5_gated` are unchanged.

In the tests, a cluster on a confirmed car's bearing at twice its range is born with the shipped
founding covariance and counted
(`TestVelocityPriorShadowRefusalShadowBirthTakesTheShippedPrior`), a car in clear view is born with
the wide variance and the shipped-prior gate shadow
(`TestVelocityPriorShadowRefusalClearBirthTakesTheWidePrior`), and without the refusal a shadow
birth is widened as before (`TestVelocityPriorShadowRefusalIsOptIn`).

## Method

l5est0010's method, its scripts and flags, at this commit's build:

- **Build.** Every tool built at `29c98de` through the Makefile's LDFLAGS with the `pcap` tag;
  every replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `29c98de8…`.
- **kirk0.** `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false
-sample-points 1024`, evidence per case, repeat-verified, the committed cohort manifest and a
  scratch copy of the run's use ledger per replay. Scored with `lidar-ground-truth-eval perframe`
  on the reconstructed kirk0 pack (`f0ed3414…`), split `kirk0-tuning`, `obj_788622b5` ignored,
  `-bootstrap 2000 -bootstrap-seed 20261108`, `shadow` against `a0`. Both arms were replayed.
- **Segments.** The first 300 s of each lot-drawn placement from `banshee-data/sf-street-speeds`,
  fetched again (l5est0010 had deleted its segments) and cut by the run's pcapng window filter.
  Every placement and segment hash equals l5est0009's and l5est0010's.
- **What was replayed.** `shadow` through both tools on all six segments; `a0` through
  `lidar-box-eval` on all six (the scripts need its evidence, which l5est0010 deleted) and through
  the gest1 replay on marina-broderick only. The other five segments' `a0` gest1 records are
  l5est0009's, as in l5est0010.
- **Does `a0` reproduce?** Yes. On kirk0 the tracking baseline is `79d9abff…` on both passes, byte
  for byte l5est0009's and l5est0010's, and the scores are their 0.4284, 0.4728, 90 and 46. On
  marina-broderick the gest1 replay's tracking baseline (`0326930c…`) is byte for byte
  l5est0009's; its gest1 report equals l5est0009's once the `age_split` section is removed, which
  l5est0008's merge (`1851d50`) added to the report after l5est0010 ran. On all six segments the
  box rows equal l5est0010's once the random track identifiers are dropped and the rows sorted, and
  every V3 and V5 figure of `a0` below equals l5est0010's.
- **The gest1 replay.** `lidar-state-estimation-baseline`, one case per run, `-warmup 40
-require-settled=false -sample-points 1024 -gest1-report`, repeat-verified (all seven runs
  `baseline_equal: true`), the scratch cohort manifest and a fresh copy of the use ledger per
  replay; `lidar-gest1-pool` over the six cases.
- **V1 and V2.** l5est0010's scratch script over the per-case track records (whole-track bootstrap,
  2,000 draws, seed 20261108), reading all four arms; it reproduces l5est0009's `a0` and `prior`
  and l5est0010's `gated` figures exactly.
- **The box rows, V3 and V5.** `lidar-box-eval -variants none -require-settled=false
-sample-points 1024 -warmup 40 -duration 120 -rows -evidence-db`; then `lidar_noise_screen.py` and
  `lidar_clutter_origin.py` on each arm's evidence, `lidar_figure_sensitivity.py` (default and
  `--moving peak`, rows and evidence with `--joins`) and `lidar_flip_cost.py --match footprint` on
  `shadow` against `a0`. V5's R4 test as l5est0009 applied it: an arm costs noise on a segment when
  brief tracks rise by more than 3, clutter by more than 5 per hundred of `a0`'s moving tracks (peak
  test), or duplicate pairs per hundred vehicle tracks rise at all.
- **Births touched.** `shadow_prior_births` and `tracks_born` from each `shadow` replay manifest:
  kirk0's scoring window and each segment's gest1 window (40 s warm-up excluded).

## How many births the shadow refusal touched

| Case                |  Born | Given the shipped prior |  Share | Confirmed (share of born) |
| ------------------- | ----: | ----------------------: | -----: | ------------------------- |
| kirk0               |   147 |                      96 | 65.3 % | 76 (51.7 %)               |
| columbus-broadway   | 1,305 |                     925 | 70.9 % | 570 (43.7 %)              |
| embarcadero-bay     |   583 |                     248 | 42.5 % | 246 (42.2 %)              |
| marina-broderick    |   325 |                     191 | 58.8 % | 141 (43.4 %)              |
| 1st-mission         | 1,500 |                     880 | 58.7 % | 809 (53.9 %)              |
| fulton-divisadero   |   537 |                     196 | 36.5 % | 274 (51.0 %)              |
| franklin-mcallister |   947 |                     637 | 67.3 % | 396 (41.8 %)              |
| six segments        | 5,197 |                   3,077 | 59.2 % | 2,436 (46.9 %)            |

The test touched most births, not a few reflections. A near confirmed car's box spans tens of
degrees of bearing, and anything born beyond it on those bearings, a real road user on the far lane
included, is in its shadow; the spec named this risk. The confirmation share sits between `a0`'s
(45.7 % on the six segments) and `gated`'s (47.4 %).

## kirk0 first: V4

| Measure                               |        `a0` |    `shadow` |       Δ | `gated` (l5est0010) | `prior` (l5est0009) |
| ------------------------------------- | ----------: | ----------: | ------: | ------------------: | ------------------: |
| HOTA                                  |      0.4284 |      0.4285 | +0.0002 |              0.4256 |              0.4208 |
| IDF1                                  |      0.4728 |      0.4734 | +0.0005 |              0.4666 |              0.4578 |
| ID switches                           |          90 |          86 |      −4 |                  88 |                 117 |
| Fragmentations                        |          46 |          45 |      −1 |                  31 |                  34 |
| False negatives (of 2,472)            |         265 |         267 |      +2 |                 253 |                 251 |
| False positives                       |       3,967 |       3,959 |      −8 |               3,993 |               3,988 |
| Labelled-object HOTA, bootstrap point |      0.4798 |      0.4802 |         |              0.4812 |              0.4772 |
| Tracks born; confirmed                |     148; 76 |     147; 76 |         |             146; 78 |             145; 87 |
| Tracking baseline (first = repeat)    | `79d9abff…` | `038ad47d…` |         |         `f99f269d…` |         `ae343438…` |

Paired bootstrap, `shadow` less `a0` (2,000 draws, seed 20261108), with `gated`'s and `prior`'s in
brackets:

| Delta          |                         p5 |                        p50 |                        p95 | Bar             | Holds |
| -------------- | -------------------------: | -------------------------: | -------------------------: | --------------- | ----- |
| HOTA           | −0.0008 (−0.0085; −0.0171) | +0.0005 (+0.0019; −0.0029) | +0.0061 (+0.0218; +0.0180) | p5 above −0.005 | Yes   |
| IDF1           | −0.0009 (−0.0237; −0.0383) | +0.0010 (+0.0015; −0.0065) | +0.0024 (+0.0379; +0.0341) | p5 above −0.005 | Yes   |
| ID switches    |             −12 (−28; −27) |               −4 (−2; +25) |               0 (+24; +85) | p95 +10 or less | Yes   |
| Fragmentations |              −3 (−41; −38) |              −1 (−15; −12) |                  0 (0; +2) | reported        |       |

**V4 passes on all three bars.** Two thirds of kirk0's births took the shipped prior, so the run
sits close to `a0` and its bootstrap is narrow: no resample loses HOTA or IDF1 by more than 0.001
or gains a switch. It also loses `gated`'s fragmentation gain (45 against 31).

## V1 and V2: the first second and after

One-step along-track and across-track z of the moving tracks' medoid residuals, pooled over the
six segments, with the whole-track bootstrap's 95 % interval:

| Arm      | Age       | Axis   |       n | Tracks | z RMS | Bootstrap 95 % | Beyond 1.96 |
| -------- | --------- | ------ | ------: | -----: | ----: | -------------- | ----------: |
| `a0`     | under 1 s | along  |   6,863 |  1,128 | 2.244 | 2.187 to 2.303 |      41.0 % |
| `prior`  | under 1 s | along  |   9,064 |  1,339 | 0.802 | 0.765 to 0.840 |       3.3 % |
| `gated`  | under 1 s | along  |   8,628 |  1,270 | 0.787 | 0.755 to 0.822 |       3.2 % |
| `shadow` | under 1 s | along  |   7,650 |  1,177 | 1.533 | 1.468 to 1.594 |      18.5 % |
| `a0`     | 1 s on    | along  | 107,970 |  1,209 | 0.802 | 0.778 to 0.826 |       3.0 % |
| `gated`  | 1 s on    | along  | 108,938 |  1,313 | 0.781 | 0.756 to 0.806 |       2.8 % |
| `shadow` | 1 s on    | along  | 109,788 |  1,242 | 0.792 | 0.767 to 0.816 |       2.9 % |
| `a0`     | under 1 s | across |   6,863 |  1,128 | 0.446 | 0.425 to 0.473 |       0.6 % |
| `shadow` | under 1 s | across |   7,650 |  1,177 | 0.412 | 0.391 to 0.436 |       0.5 % |
| `a0`     | 1 s on    | across | 107,970 |  1,209 | 0.367 | 0.353 to 0.381 |       0.3 % |
| `gated`  | 1 s on    | across | 108,938 |  1,313 | 0.367 | 0.354 to 0.381 |       0.3 % |
| `shadow` | 1 s on    | across | 109,788 |  1,242 | 0.365 | 0.352 to 0.378 |       0.3 % |
| `a0`     | all       | along  | 114,833 |  1,240 | 0.951 | 0.925 to 0.978 |       5.3 % |
| `shadow` | all       | along  | 117,438 |  1,286 | 0.860 | 0.834 to 0.886 |       3.9 % |

**V1 passes, narrowly.** The first second falls by 31.7 %, against a bar of 30 %, and `shadow`'s
bootstrap 95th percentile (1.585) is under `a0`'s 2.244. It keeps under half of `gated`'s fall
(64.9 %): the births given the shipped prior keep the shipped first second. **V2 passes.** After
the first second the along figure falls by 0.010 and the across figure by 0.002.
`lidar-gest1-pool` over the six `shadow` records reads 1,426 tracks, along RMS 0.860, across 0.368.

Per site (along z RMS under 1 s, with n, and from 1 s; across from 1 s):

| Site                | `a0` under 1 s (n) | `gated` under 1 s | `shadow` under 1 s (n) | `a0` 1 s on | `shadow` 1 s on | `a0` across 1 s on | `shadow` across 1 s on |
| ------------------- | -----------------: | ----------------: | ---------------------: | ----------: | --------------: | -----------------: | ---------------------: |
| columbus-broadway   |      2.131 (1,648) |             0.861 |          1.727 (1,769) |       0.890 |           0.872 |              0.375 |                  0.376 |
| embarcadero-bay     |        2.619 (868) |             0.689 |            1.350 (973) |       0.730 |           0.730 |              0.396 |                  0.408 |
| marina-broderick    |        2.336 (605) |             0.651 |            1.374 (647) |       0.832 |           0.831 |              0.285 |                  0.284 |
| 1st-mission         |      1.615 (1,473) |             0.701 |          1.408 (1,626) |       0.662 |           0.652 |              0.353 |                  0.344 |
| fulton-divisadero   |        2.187 (877) |             0.842 |          1.212 (1,080) |       0.846 |           0.829 |              0.408 |                  0.404 |
| franklin-mcallister |      2.651 (1,392) |             0.853 |          1.773 (1,555) |       0.892 |           0.881 |              0.379 |                  0.368 |

The first-second gain falls most where the shadow share is highest (columbus-broadway 71 %,
franklin-mcallister 67 %) and least where it is lowest (fulton-divisadero 37 %).

## V3: invented speed

Scored rows (40 s warm-up, 120 s scored). p98 and p85 of the moving tracks' maximum speeds at the
default moving test, in m/s; the peak test's p98 in brackets. `a0`'s figures equal l5est0010's.

| Site                | `a0` moving | `shadow` moving | `a0` p85 | `shadow` p85 | `a0` p98      | `shadow` p98  | p98 Δ (`gated`; `prior`) | `a0` fastest; rows over 15 | `shadow` fastest; rows over 15 (`gated`; `prior`) | V3   |
| ------------------- | ----------: | --------------: | -------: | -----------: | ------------- | ------------- | ------------------------ | -------------------------- | ------------------------------------------------- | ---- |
| columbus-broadway   |         159 |             156 |     9.94 |         9.93 | 13.45 (13.45) | 13.45 (13.45) | 0.00 (+0.39; +0.49)      | 14.49; 0                   | 14.49; 0 (34; 35)                                 | Pass |
| embarcadero-bay     |          72 |              77 |    11.20 |        11.20 | 15.59 (15.59) | 15.97 (15.97) | +0.38 (+0.38; +0.38)     | 15.84; 122                 | 16.44; 215 (216; 216)                             | Pass |
| marina-broderick    |          81 |              80 |    11.13 |        11.33 | 13.53 (13.53) | 13.53 (13.53) | 0.00 (0.00; 0.00)        | 13.80; 0                   | 13.80; 0 (0; 0)                                   | Pass |
| 1st-mission         |         227 |             232 |     7.38 |         7.40 | 9.74 (9.74)   | 9.74 (9.74)   | 0.00 (+0.76; +0.93)      | 11.10; 0                   | 11.10; 0 (0; 0)                                   | Pass |
| fulton-divisadero   |          82 |              88 |     8.68 |         8.80 | 11.84 (11.84) | 12.54 (12.54) | **+0.70** (+0.70; +2.66) | 12.33; 0                   | 14.50; 0 (0; 25)                                  | Fail |
| franklin-mcallister |         124 |             132 |    10.82 |        11.63 | 12.50 (12.50) | 15.47 (16.04) | **+2.97** (+2.36; +2.37) | 14.89; 0                   | 17.63; **33** (32; 92)                            | Fail |

**V3 fails on two of six segments, against four under `gated` and `prior`.** Columbus-broadway's
fast track (34 rows up to 20.47 m/s under `gated`) is gone, and its p98 is `a0`'s; 1st-mission's
p98 rise is gone too. Fulton-divisadero's p98 rise is `gated`'s exactly. Franklin-mcallister's
33 rows over 15 m/s are five tracks at up to 17.63 m/s, 7 to 48 m from the sensor, against
`gated`'s four at 32 to 48 m, and its p98 rise grows by 0.61 m/s: those births were not in a
confirmed shadow, or the shadow refusal changed which tracks the frame's clusters went to.

## V5: noise and the falsifier

Each segment's figures under `shadow` (`a0`'s are l5est0010's, reproduced exactly), then the R4
test. Clutter classes are artefact / stationary road user / fragment of a mover / unclassed.

| Site                | Tracks | Brief | Clutter (art / stat / frag / uncl) | Moving, peak | p85, p98 peak (m/s) | Vehicle tracks | Dup pairs (per 100) | Pair-frames | Confirmed |
| ------------------- | -----: | ----: | ---------------------------------- | -----------: | ------------------- | -------------: | ------------------- | ----------: | --------: |
| columbus-broadway   |    333 |    55 | 68 (29 / 7 / 6 / 26)               |          153 | 10.09, 13.45        |            140 | 33 (23.6)           |       1,194 |       276 |
| embarcadero-bay     |    174 |    18 | 32 (17 / 5 / 2 / 8)                |          101 | 11.21, 13.53        |             70 | 10 (14.3)           |         370 |       133 |
| marina-broderick    |     98 |    11 | 13 (7 / 1 / 1 / 4)                 |           70 | 11.63, 13.63        |             63 | 14 (22.2)           |         722 |        94 |
| 1st-mission         |    510 |    59 | 89 (54 / 5 / 13 / 17)              |          235 | 7.81, 10.59         |            182 | 62 (34.1)           |       3,013 |       425 |
| fulton-divisadero   |    166 |    16 | 25 (16 / 1 / 1 / 7)                |           98 | 9.58, 12.26         |             71 | 24 (33.8)           |         511 |       136 |
| franklin-mcallister |    249 |    26 | 44 (23 / 6 / 2 / 13)               |          143 | 12.03, 16.04        |            135 | 38 (28.1)           |         954 |       205 |

| Site                | Brief Δ (`gated`; `prior`) | Clutter Δ per 100 (`gated`; `prior`) | Dup pairs per 100: `a0` → `shadow` (`gated`; `prior`) | Costs noise |
| ------------------- | -------------------------: | -----------------------------------: | ----------------------------------------------------- | ----------- |
| columbus-broadway   |                −3 (−4; +4) |                    −2.7 (−4.1; +0.7) | **23.4 → 23.6** (30.2; 31.7)                          | Yes         |
| embarcadero-bay     |                −4 (−3; +4) |                    −4.4 (−6.6; +2.2) | **6.2 → 14.3** (15.8; 17.3)                           | Yes         |
| marina-broderick    |                −1 (−4; +1) |                    −1.4 (−5.8; +1.4) | **21.0 → 22.2** (23.9; 27.0)                          | Yes         |
| 1st-mission         |                 0 (−6; +2) |                    −3.4 (−7.8; −4.3) | **32.4 → 34.1** (36.5; 35.4)                          | Yes         |
| fulton-divisadero   |                −5 (−9; −6) |                  −8.6 (−16.1; −12.9) | **31.0 → 33.8** (32.4; 31.6)                          | Yes         |
| franklin-mcallister |                −1 (−1; +8) |                    +1.5 (+2.2; +6.6) | **24.6 → 28.1** (31.9; 35.9)                          | Yes         |

**V5 fails: noise costs on six of six segments, on the duplicate condition alone.** Brief tracks
and clutter fall or hold on every segment but franklin-mcallister's clutter (+1.5 per hundred,
under the bar). Duplicate pairs per hundred vehicle tracks rise on all six, by 0.2 to 8.1, less
than under `gated` on five; at columbus-broadway the pair count is `a0`'s (33) and the rise is one
vehicle track fewer. Footprint matching loses no `a0` moving track (shortened 0 to 5, added 0
to 3).

**The falsifier does not hold.** Its bar needs new rows over 15 m/s at both columbus-broadway and
franklin-mcallister; columbus-broadway writes none. By the spec's reading, columbus-broadway's
invented speed was a shadow birth and the shadow test was its lever. Franklin-mcallister's was
not removed by it.

## Reported, not decided

**Counts under the peak test.** The moving count rises on every segment, by less than under
`gated` on five: columbus-broadway +4.8 % (credited for joins +8.2 %; `gated` +17.8 %),
embarcadero-bay +11.0 % (+12.1 %), marina-broderick +1.4 % (+2.9 %), 1st-mission +1.3 % (+6.9 %),
fulton-divisadero +5.4 % (+8.6 %; `gated` +5.4 %), franklin-mcallister +4.4 % (+6.6 %). p85 moves
by −0.30 to +0.44 m/s.

**Criterion 2's partial correlation after 1 s.** `a0` r 0.273 (5th percentile 0.255), `gated`
0.285 (0.265), `shadow` 0.288 (0.267).

**kirk0's fragmentations.** 46 → 45 (bootstrap delta 5th to 95th percentile −3 to 0), against
`gated`'s 31.

## Interpretation

**What was measured.** The shadow refusal moves the gated form most of the way back to `a0`. That
is what it was asked to do for reflections and fragments behind a nearer body, but the test, read
at birth against every confirmed box, applies to 37 % to 71 % of births. Where it applied, it
removed the one fast track at columbus-broadway, 1st-mission's p98 rise and kirk0's identity
spread, and it took back half the first-second gain. Where the remaining births took the wide
prior, franklin-mcallister's fast tracks and the duplicate rise remained, a little smaller.

**What this says about the cost.** The invented speed is not one mechanism. At columbus-broadway
it was a birth in a confirmed track's shadow, as l5est0006 found under two-point initialisation.
At franklin-mcallister it was not: the fast rows survive with the shadow births refused, and the
fast tracks now include one 7 to 30 m from the sensor. The duplicates rise with the clear-view
births too: the rise is smallest at columbus-broadway (+0.2, 71 % of births refused) and largest at
embarcadero-bay (+8.1, 42 %), though fulton-divisadero (+2.8, 37 %) breaks the order. That is
loosely what a cost spread over every wide-prior birth would look like; six segments cannot say
more.

**What the spec warned of.** "The shadow test reads the published box, which for a near car spans
tens of degrees, so real births behind a nearer body also take the shipped prior." It did, for most
births, so V4's pass and V1's narrow pass are partly a measure of how little of the gated form is
left, not evidence that the remaining wide-prior births are sound.

## Limitations

- **A screen read three times.** The six segments and kirk0 were read for this question by
  l5est0009 and l5est0010, and this form was chosen after their failures. Its passes and fails say
  where the cost sits on this data, not how the form reads elsewhere.
- **The shadow share is a count, not a classification.** `shadow_prior_births` counts births in a
  confirmed box's bearing span beyond its nearest corner; how many were reflections, fragments or
  real far-lane road users was not measured.
- **The falsifier reads two sites.** Its not holding says columbus-broadway's fast track was a
  shadow birth; it does not say franklin-mcallister's are invented, which only a reference there
  could show.
- **No speed reference off kirk0.** z RMS reads the filter's consistency with itself; the rows over
  15 m/s are inferred to be invented from their clusters and observed rows, as in l5est0009.
- **kirk0's labels** are the reconstructed `kirk0-reference` pack (`f0ed3414…`), on the tuning
  split, not held out; 19 objects, so the bootstrap is wide.
- **`a0` partly reused.** Five segments' `a0` gest1 records are l5est0009's, reused after `a0`
  reproduced on kirk0 and marina-broderick and every box row reproduced on all six.
- **Scratch eligibility,** as l5est0009 and l5est0010: a scratch cohort manifest that assigns each
  segment by hash to its site as development data, a fresh copy of the use ledger per replay.
  embarcadero-folsom was not downloaded or read.

## Recommendations

1. **Keep `velocity_prior_init_7p5_gated_shadow`, `velocity_prior_init_7p5_gated` and
   `velocity_prior_init_7p5` off.** The shadow form fails V3 and V5; it is not named for the second
   segments or label0003's spans, and no default changes.
2. **Read franklin-mcallister's fast tracks before another birth rule.** Five tracks under
   `shadow`, four under `gated`, 32 to 92 rows over 15 m/s under all three wide-prior forms: a
   per-track look at their clusters, their first pairings and whether `a0` has the same road users
   slower would say whether they are invented or real.
   - Falsified if: those tracks match `a0` tracks of the same road users at under 15 m/s on their
     observed rows, in which case the speed is invented and its cause is outside the shadow.
3. **Do not narrow the shadow test to rescue the first second.** A narrower test would give back
   the columbus-broadway fast track it removed; the first-second gain and the speed cost come from
   the same births.

## Provenance

| Item           | Value                                                                                                                                                                                                                                                                                                                                                                                       |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base           | `cbebf2c` (spec approved, rule and falsifier committed before the option existed)                                                                                                                                                                                                                                                                                                           |
| Commits        | code `29c98de` (`VelocityPriorShadowRefusal`, `velocity_prior_init_7p5_gated_shadow`, `ShadowPriorBirths`, tests), before the replays; report, spec answer, DEVLOG and maths note in the commit after                                                                                                                                                                                       |
| Builds         | built at `29c98de` with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA` `29c98de8…`) and the `pcap` tag; every replay header reads `build_version` 0.5.1-pre52, `build_git_sha` `29c98de8…`. `lidar-state-estimation-baseline` `sha256:a601bb5c…`, `lidar-box-eval` `sha256:ab9fb785…`, `lidar-ground-truth-eval` `sha256:99f4ffbb…`, `lidar-gest1-pool` `sha256:a8b9a68e…` |
| Parameter sets | `config/tuning.defaults.json` (`sha256:9ad7377f…`), fingerprint `ab0c4035780d`, unchanged, for both arms; replay parameter hashes `ab0c4035780d…` (`a0`) and `c6dfefb6…` (`shadow`, with `velocity_prior_init_7p5_gated_shadow` folded in)                                                                                                                                                  |
| kirk0          | capture `sha256:2864ebde…` (Git LFS); reconstructed pack `ds_67842cb1b22b70e7` rebuilt to `sha256:f0ed3414…`, split `kirk0-tuning` (`7194bb61…`), `obj_788622b5` ignored; tracking baselines `79d9abff…` (`a0`, equal to l5est0009's and l5est0010's) and `038ad47d…` (`shadow`), first equal to repeat                                                                                     |
| The lot        | l5est0009's: `random.Random(202611101).sample` over the sorted names of the 21 corpus sites other than ashbury-downey, haight-divisadero and embarcadero-folsom: columbus-broadway, embarcadero-bay, marina-broderick, 1st-mission, fulton-divisadero, franklin-mcallister                                                                                                                  |
| Segments       | fetched again; placement and segment hashes equal l5est0009's: columbus-broadway `aa69875b` → `774c5434`, embarcadero-bay `50ccf70b` → `5a533b67`, marina-broderick `1d7d876b` → `c8141c2f`, 1st-mission `cef1cf1b` → `83160e99`, fulton-divisadero `ae6c6f06` → `eda80c97`, franklin-mcallister `e6601525` → `ab9722de`                                                                    |
| Sites          | all six ran `shadow` through both tools and `a0` through the box-eval; embarcadero-folsom (held out) was not downloaded or read                                                                                                                                                                                                                                                             |
| Host           | Claude Code cloud container, 4 cores, the only experiment agent, `GOMAXPROCS=3`, `nice -n 5`, three lanes; replays 13:23 to 13:51 UTC (kirk0 126 to 127 s, gest1 253 to 398 s, box-eval 112 to 282 s per segment); raw output in the session's scratch, not kept                                                                                                                            |
