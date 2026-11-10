# The honest birth prior with the shipped gate, screened on kirk0 and six lot-drawn segments

<!-- ignore-style-length -->

l5est0009 gave a newborn track a birth velocity variance of (7.5 m/s)² and removed the first
second's along-track over-dispersion, but failed on speed, identity and noise. Its report put the
gain in the Kalman update and the cost in association: the wide variance also widened the
newborn's gate. This run keeps the wide variance for the update and gates and costs a tentative
track with the covariance it would hold under the shipped prior. It reads the form on kirk0 and on
l5est0009's six lot-drawn first segments, against l5est0009's rule unchanged, committed before the
option existed. It answers [l5est0010](../../../data/experiments/l5est/l5est0010.md).

- **Status:** Screen complete, 2026-11-10. Fails V3, V4 and V5; the falsifier holds; nothing
  adopted
- **Layers:** L5 (track initialisation and association), read by L8 analytics and the box rows
- **Related:** [honest velocity prior (l5est0009)](velocity-prior-init-2026-11.md),
  [G-EST-1 corpus read (l5est0007)](gest1-corpus-read-2026-11.md),
  [tracking maths §2.3](../../../data/maths/tracking-maths.md#honest-velocity-prior-at-birth)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) (`ab0c4035780d`) for both
  arms; `gated` adds the replay experiment `velocity_prior_init_7p5_gated` (replay parameter hash
  `e023e46b…`)

## Answer

**The gated form fails V3, V4 and V5, and the spec's falsifier holds: the cost is in the update,
not the gate.** The first second is as l5est0009 left it (along z RMS 2.24 to 0.79, V1 and V2
pass). Gating newborns as shipped removes part of the noise l5est0009 paid: brief tracks and
clutter now fall on five of six segments, where under `prior` they rose on four, and kirk0's switch
count is back at `a0`'s (88 against 90; `prior` 117). But duplicate pairs per hundred vehicle
tracks still rise on all six segments, the scored-row p98 still rises by more than 0.5 m/s on three,
and two segments still write rows over 15 m/s where `a0` writes none. A newborn that the shipped
gate admits still takes its speed from the first pairing once the update believes it.

| Clause                     | Deciding figures (`gated`; l5est0009's `prior` in brackets)                                                                                                                                                   | Result                     |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- |
| V1, the first second       | along z RMS at age under 1 s: `a0` 2.244, `gated` 0.787 (0.802), −64.9 % (−64.3 %); bootstrap 95th percentile 0.815 (0.835), under `a0`'s 2.244                                                               | **Pass**                   |
| V2, after the first second | along after 1 s 0.802 → 0.781 (0.764), −0.021; across after 1 s 0.367 → 0.367 (0.359), +0.000                                                                                                                 | **Pass**                   |
| V3, no invented speed      | p98 change over 0.5 m/s at 1st-mission +0.76 (+0.93), fulton-divisadero +0.70 (+2.66), franklin-mcallister +2.36 (+2.37); new rows over 15 m/s at columbus-broadway (34; 35) and franklin-mcallister (32; 92) | **Fail** on 4 of 6 (4)     |
| V4, identity               | kirk0 paired bootstrap: HOTA delta 5th percentile −0.0085 (−0.0171), IDF1 −0.0237 (−0.0383), bars −0.005; switch change 95th percentile +24 (+85), bar +10                                                    | **Fail** on all three bars |
| V5, noise                  | noise costs on 6 of 6 segments (6 of 6), bar two at most; the duplicate condition alone on every one                                                                                                          | **Fail**                   |
| Falsifier                  | duplicate pairs per hundred vehicle tracks rise on 6 of 6 segments (bar: more than two falsifies the gate as the cause)                                                                                       | **Holds**                  |

**Recommendation:** keep both options off. The gate explains the brief tracks, the clutter and
kirk0's switches; it does not explain the duplicates or the invented speed, which survive on the
same segments with the gate restored. The next form has to change what the first updates do with
an unknown velocity, not where a newborn may look.

## Rule

Committed in the spec (`2bb363e`) before the option existed: l5est0009's V1 to V5 unchanged, read
for `gated` against `a0`.

- **V1, the first second.** Pooled over the six segments, the medoid's one-step along-track z RMS
  at track age 1 s or less falls by 30 % or more from `a0`'s, with the whole-track bootstrap's
  95th percentile of the candidate's figure under `a0`'s point figure.
- **V2, after the first second.** The same figure after 1 s rises by no more than 0.05, and the
  across-track figure after 1 s moves by no more than 0.05.
- **V3, no invented speed.** On each segment's scored rows, the candidate's p98 speed is no more
  than 0.5 m/s above `a0`'s, and it writes no row over 15 m/s on a segment where `a0` writes none.
- **V4, identity.** On kirk0, against `a0`, the paired bootstrap's 5th percentiles of the HOTA and
  IDF1 deltas are above −0.005 and the 95th percentile of the switch change is +10 or less.
- **V5, noise.** On the six segments under l5id0019's R4 (moving by l5id0024's peak test), noise
  costs on no more than two.
- **Falsifier (l5est0009's).** If duplicate pairs per hundred vehicle tracks still rise on more
  than two of the six segments, the cost is in the update, not the gate.
- **Outcome.** A pass names `gated` for the second segments and label0003's spans. A fail names
  its clause. Nothing is adopted.

## Arms and the option

| Arm     | Parameter set            | Replay experiments              | Update's birth velocity variance | Tentative track's gate and cost read |
| ------- | ------------------------ | ------------------------------- | -------------------------------- | ------------------------------------ |
| `a0`    | defaults, `ab0c4035780d` | none                            | 1 m²/s² (shipped)                | the track's P                        |
| `gated` | defaults, `ab0c4035780d` | `velocity_prior_init_7p5_gated` | (7.5 m/s)², 56.25 m²/s²          | the shipped prior's shadow P         |

`TrackerConfig.VelocityPriorGated` (commit `c112218`) gives each new track a second 4x4
covariance, founded as the shipped prior founds P (position 10 m², velocity 1 m²/s², no
cross terms) and stepped by the same prediction, process noise, cap and coast inflation, and by
the same measurement update with its own gain and the same R. Since the covariance update does not
read the measurement, the shadow is exactly the covariance the track would hold under the shipped
prior after the same sequence of frames (`TestVelocityPriorGatedShadowIsTheShippedCovariance`).
While the track is tentative, the association gate (d² against `GatingDistanceSquared`), the
likelihood cost term when one is on, the pre-gate instrumentation and the near-edge gate read the
shadow; the Kalman update reads the track's own wide P. Once the track confirms, its gate reads its
own P. The replay experiment brings `VelocityPriorInit` with it and is refused with
`two_point_velocity_init`, whose second update the shadow does not follow; `velocity_prior_init_7p5`
is unchanged.

The inconsistency is deliberate and short: the gate is stated in a covariance the state does not
hold from a track's birth until its fourth observed frame (`hits_to_confirm` 4), with at most three
consecutive coasts (`max_misses` 3) between them. In the tests, the noise-free 6.5 m/s vehicle
converges by its fourth update as under `velocity_prior_init_7p5`
(`TestVelocityPriorGatedConvergesAsTheWidePrior`), and a cluster 2.5 m from a stationary newborn
after three hits, outside the shipped gate's 1.9 m reach but inside the wide gate's 3.1 m, is
associated under the wide prior and not under the gated form
(`TestVelocityPriorGatedRefusesWhatOnlyTheWideGateAdmits`).

## Method

l5est0009's method, its scripts and flags, at this commit's build:

- **Build.** Every tool built at `c112218` through the Makefile's LDFLAGS with the `pcap` tag;
  every replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `c1122188…`.
- **kirk0.** `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false
-sample-points 1024`, evidence per case, repeat-verified, the committed cohort manifest and a
  scratch copy of the run's use ledger per replay. Scored with `lidar-ground-truth-eval perframe`
  on the reconstructed kirk0 pack (`f0ed3414…`), split `kirk0-tuning`, `obj_788622b5` ignored,
  `-bootstrap 2000 -bootstrap-seed 20261108`, `gated` against `a0`.
- **Segments.** The first 300 s of each lot-drawn placement from `banshee-data/sf-street-speeds`,
  fetched again (l5est0009 had deleted its segments) and cut by the run's pcapng window filter.
  Every placement and segment hash equals l5est0009's.
- **What was replayed.** `gated` through both tools on all six segments; `a0` through
  `lidar-box-eval` on all six (the scripts need its evidence, which l5est0009 deleted) and through
  the gest1 replay on marina-broderick only. The other five segments' `a0` gest1 records are
  l5est0009's, which the spec allows once `a0` reproduces.
- **Does `a0` reproduce?** Yes. On kirk0 the tracking baseline is `79d9abff…` on both passes,
  byte for byte l5est0009's, and the scores are its 0.4284, 0.4728, 90 and 46. On marina-broderick
  the gest1 replay's tracking baseline (`0326930c…`) and its gest1 report are byte for byte
  l5est0009's; the track records differ only in their random track identifiers. On all six
  segments the box rows equal l5est0009's once the random track identifiers are dropped and the
  rows sorted, and every V3 and V5 figure of `a0` below equals l5est0009's.
- **The gest1 replay.** `lidar-state-estimation-baseline`, one case per run, `-warmup 40
-require-settled=false -sample-points 1024 -gest1-report`, repeat-verified (all seven runs
  `baseline_equal: true`), l5est0009's scratch cohort manifest and a fresh copy of the use ledger
  per replay. `lidar-gest1-pool` pooled the six cases; pooling l5est0009's `a0` records reproduces
  its pool exactly (1,368 tracks, along RMS 0.951).
- **V1 and V2.** l5est0009's scratch script over the per-case track records (whole-track bootstrap,
  2,000 draws, seed 20261108), reading all three arms; it reproduces l5est0009's `a0` and `prior`
  figures exactly.
- **The box rows, V3 and V5.** `lidar-box-eval -variants none -require-settled=false
-sample-points 1024 -warmup 40 -duration 120 -rows -evidence-db`; then `lidar_noise_screen.py` and
  `lidar_clutter_origin.py` on each arm's evidence, `lidar_figure_sensitivity.py` (default and
  `--moving peak`, rows and evidence with `--joins`) and `lidar_flip_cost.py --match footprint` on
  `gated` against `a0`. V5's R4 test as l5est0009 applied it: an arm costs noise on a segment when
  brief tracks rise by more than 3, clutter by more than 5 per hundred of `a0`'s moving tracks (peak
  test), or duplicate pairs per hundred vehicle tracks rise at all.

## kirk0 first: V4

| Measure                               |        `a0` |     `gated` |       Δ | `prior` (l5est0009) |
| ------------------------------------- | ----------: | ----------: | ------: | ------------------: |
| HOTA                                  |      0.4284 |      0.4256 | −0.0028 |              0.4208 |
| IDF1                                  |      0.4728 |      0.4666 | −0.0062 |              0.4578 |
| ID switches                           |          90 |          88 |      −2 |                 117 |
| Fragmentations                        |          46 |          31 |     −15 |                  34 |
| False negatives (of 2,472)            |         265 |         253 |     −12 |                 251 |
| False positives                       |       3,967 |       3,993 |     +26 |               3,988 |
| Labelled-object HOTA, bootstrap point |      0.4798 |      0.4812 |         |              0.4772 |
| Tracks born; confirmed                |     148; 76 |     146; 78 |         |             145; 87 |
| Tracking baseline (first = repeat)    | `79d9abff…` | `f99f269d…` |         |         `ae343438…` |

Paired bootstrap, `gated` less `a0` (2,000 draws, seed 20261108), with `prior`'s in brackets:

| Delta          |                p5 |               p50 |               p95 | Bar             | Holds |
| -------------- | ----------------: | ----------------: | ----------------: | --------------- | ----- |
| HOTA           | −0.0085 (−0.0171) | +0.0019 (−0.0029) | +0.0218 (+0.0180) | p5 above −0.005 | No    |
| IDF1           | −0.0237 (−0.0383) | +0.0015 (−0.0065) | +0.0379 (+0.0341) | p5 above −0.005 | No    |
| ID switches    |         −28 (−27) |          −2 (+25) |         +24 (+85) | p95 +10 or less | No    |
| Fragmentations |         −41 (−38) |         −15 (−12) |            0 (+2) | reported        |       |

**V4 fails on all three bars**, by less than `prior` did. At the point the gated form keeps `a0`'s
switch count (88 against 90) and fragments less (31 against 46); `prior`'s 27 extra switches were
the gate's. The bootstrap's spread is what fails: resampled over 19 labelled objects, the switch
change reaches +24 and the HOTA and IDF1 deltas reach below −0.005, though their medians are now
positive.

## V1 and V2: the first second and after

One-step along-track and across-track z of the moving tracks' medoid residuals, pooled over the
six segments, with the whole-track bootstrap's 95 % interval:

| Arm     | Age       | Axis   |       n | Tracks | z RMS | Bootstrap 95 % | Beyond 1.96 |
| ------- | --------- | ------ | ------: | -----: | ----: | -------------- | ----------: |
| `a0`    | under 1 s | along  |   6,863 |  1,128 | 2.244 | 2.187 to 2.303 |      41.0 % |
| `prior` | under 1 s | along  |   9,064 |  1,339 | 0.802 | 0.765 to 0.840 |       3.3 % |
| `gated` | under 1 s | along  |   8,628 |  1,270 | 0.787 | 0.755 to 0.822 |       3.2 % |
| `a0`    | 1 s on    | along  | 107,970 |  1,209 | 0.802 | 0.778 to 0.826 |       3.0 % |
| `prior` | 1 s on    | along  | 105,805 |  1,376 | 0.764 | 0.740 to 0.788 |       2.6 % |
| `gated` | 1 s on    | along  | 108,938 |  1,313 | 0.781 | 0.756 to 0.806 |       2.8 % |
| `a0`    | under 1 s | across |   6,863 |  1,128 | 0.446 | 0.425 to 0.473 |       0.6 % |
| `gated` | under 1 s | across |   8,628 |  1,270 | 0.397 | 0.372 to 0.419 |       0.5 % |
| `a0`    | 1 s on    | across | 107,970 |  1,209 | 0.367 | 0.353 to 0.381 |       0.3 % |
| `prior` | 1 s on    | across | 105,805 |  1,376 | 0.359 | 0.346 to 0.373 |       0.3 % |
| `gated` | 1 s on    | across | 108,938 |  1,313 | 0.367 | 0.354 to 0.381 |       0.3 % |
| `a0`    | all       | along  | 114,833 |  1,240 | 0.951 | 0.925 to 0.978 |       5.3 % |
| `gated` | all       | along  | 117,566 |  1,351 | 0.781 | 0.759 to 0.804 |       2.8 % |

**V1 passes.** The first second falls by 64.9 %, against a bar of 30 %, and `gated`'s bootstrap
95th percentile (0.815) is far under `a0`'s 2.244. **V2 passes.** After the first second the along
figure falls by 0.021 and the across figure moves by +0.0002. The young residuals' share of the
along tail beyond 1.96 is 8.4 % (`a0` 46.6 %, `prior` 10.0 %).

Per site (along z RMS under 1 s, with n, and from 1 s; across from 1 s):

| Site                | `a0` under 1 s (n) | `prior` under 1 s | `gated` under 1 s (n) | `a0` 1 s on | `gated` 1 s on | `a0` across 1 s on | `gated` across 1 s on |
| ------------------- | -----------------: | ----------------: | --------------------: | ----------: | -------------: | -----------------: | --------------------: |
| columbus-broadway   |      2.131 (1,648) |             0.906 |         0.861 (2,032) |       0.890 |          0.869 |              0.375 |                 0.383 |
| embarcadero-bay     |        2.619 (868) |             0.717 |         0.689 (1,033) |       0.730 |          0.710 |              0.396 |                 0.403 |
| marina-broderick    |        2.336 (605) |             0.645 |           0.651 (706) |       0.832 |          0.829 |              0.285 |                 0.283 |
| 1st-mission         |      1.615 (1,473) |             0.718 |         0.701 (1,948) |       0.662 |          0.650 |              0.353 |                 0.347 |
| fulton-divisadero   |        2.187 (877) |             0.825 |         0.842 (1,142) |       0.846 |          0.809 |              0.408 |                 0.412 |
| franklin-mcallister |      2.651 (1,392) |             0.841 |         0.853 (1,767) |       0.892 |          0.847 |              0.379 |                 0.370 |

The consistency gain is the update's: restoring the shipped gate leaves it within 0.05 of
`prior`'s at every site.

## V3: invented speed

Scored rows (40 s warm-up, 120 s scored). p98 and p85 of the moving tracks' maximum speeds at the
default moving test, in m/s; the peak test's p98 in brackets. `a0`'s figures equal l5est0009's.

| Site                | `a0` moving | `gated` moving | `a0` p85 | `gated` p85 | `a0` p98      | `gated` p98   | p98 Δ (`prior`)   | `a0` fastest; rows over 15 | `gated` fastest; rows over 15 (`prior`) | V3   |
| ------------------- | ----------: | -------------: | -------: | ----------: | ------------- | ------------- | ----------------- | -------------------------- | --------------------------------------- | ---- |
| columbus-broadway   |         159 |            171 |     9.94 |       10.57 | 13.45 (13.45) | 13.84 (13.84) | +0.39 (+0.49)     | 14.49; 0                   | 20.47; **34** (35)                      | Fail |
| embarcadero-bay     |          72 |             77 |    11.20 |       11.30 | 15.59 (15.59) | 15.97 (15.97) | +0.38 (+0.38)     | 15.84; 122                 | 16.47; 216 (216)                        | Pass |
| marina-broderick    |          81 |             80 |    11.13 |       11.33 | 13.53 (13.53) | 13.53 (13.53) | 0.00 (0.00)       | 13.80; 0                   | 13.80; 0 (0)                            | Pass |
| 1st-mission         |         227 |            249 |     7.38 |        7.45 | 9.74 (9.74)   | 10.50 (10.37) | **+0.76** (+0.93) | 11.10; 0                   | 11.83; 0 (0)                            | Fail |
| fulton-divisadero   |          82 |             86 |     8.68 |        8.68 | 11.84 (11.84) | 12.54 (12.54) | **+0.70** (+2.66) | 12.33; 0                   | 14.52; 0 (25)                           | Fail |
| franklin-mcallister |         124 |            144 |    10.82 |       11.68 | 12.50 (12.50) | 14.85 (14.85) | **+2.36** (+2.37) | 14.89; 0                   | 17.63; **32** (92)                      | Fail |

**V3 fails on four of six segments, the same four as `prior`.** The gate removes fulton-divisadero's
fast track (25 rows under `prior`, none here) and most of franklin-mcallister's (92 to 32 rows),
but not columbus-broadway's: its 34 rows are one track at up to 20.47 m/s, the same peak as under
`prior`, on clusters of at most 45 returns at 20 m, observed in two of its fast rows and coasting
for the rest. That newborn's pairings fall inside the shipped gate too; what makes it fast is the
update's gain on an unknown velocity. Franklin-mcallister's 32 rows are four tracks at 32 to 48 m,
most of their fast rows observed; no reference says what moved there.

## V5: noise and the falsifier

Each segment's figures under `gated` (`a0`'s are l5est0009's, reproduced exactly), then the R4
test. Clutter classes are artefact / stationary road user / fragment of a mover / unclassed.

| Site                | Tracks | Brief | Clutter (art / stat / frag / uncl) | Moving, peak | p85, p98 peak (m/s) | Vehicle tracks | Dup pairs (per 100) | Pair-frames | Confirmed |
| ------------------- | -----: | ----: | ---------------------------------- | -----------: | ------------------- | -------------: | ------------------- | ----------: | --------: |
| columbus-broadway   |    340 |    54 | 66 (32 / 6 / 7 / 21)               |          172 | 10.20, 13.67        |            139 | 42 (30.2)           |       1,366 |       282 |
| embarcadero-bay     |    174 |    19 | 30 (19 / 3 / 1 / 7)                |          103 | 11.62, 13.53        |             76 | 12 (15.8)           |         827 |       133 |
| marina-broderick    |     96 |     8 | 10 (4 / 1 / 1 / 4)                 |           74 | 11.33, 13.63        |             67 | 16 (23.9)           |         488 |        92 |
| 1st-mission         |    510 |    53 | 79 (46 / 4 / 13 / 16)              |          265 | 7.88, 10.66         |            192 | 70 (36.5)           |       2,424 |       429 |
| fulton-divisadero   |    158 |    12 | 18 (14 / 0 / 1 / 3)                |           98 | 9.71, 12.26         |             71 | 23 (32.4)           |         655 |       132 |
| franklin-mcallister |    258 |    26 | 45 (23 / 7 / 1 / 14)               |          151 | 12.14, 15.40        |            141 | 45 (31.9)           |       1,411 |       212 |

| Site                | Brief Δ (`prior`) | Clutter Δ per 100 (`prior`) | Dup pairs per 100: `a0` → `gated` (`prior`) | Costs noise |
| ------------------- | ----------------: | --------------------------: | ------------------------------------------- | ----------- |
| columbus-broadway   |           −4 (+4) |                 −4.1 (+0.7) | **23.4 → 30.2** (31.7)                      | Yes         |
| embarcadero-bay     |           −3 (+4) |                 −6.6 (+2.2) | **6.2 → 15.8** (17.3)                       | Yes         |
| marina-broderick    |           −4 (+1) |                 −5.8 (+1.4) | **21.0 → 23.9** (27.0)                      | Yes         |
| 1st-mission         |           −6 (+2) |                 −7.8 (−4.3) | **32.4 → 36.5** (35.4)                      | Yes         |
| fulton-divisadero   |           −9 (−6) |               −16.1 (−12.9) | **31.0 → 32.4** (31.6)                      | Yes         |
| franklin-mcallister |           −1 (+8) |                 +2.2 (+6.6) | **24.6 → 31.9** (35.9)                      | Yes         |

**V5 fails: noise costs on six of six segments, on the duplicate condition alone.** Brief tracks
fall on every segment and clutter on five, where `prior` raised brief tracks on five and clutter on
four: those were the gate's. **The falsifier holds:** duplicate pairs per hundred vehicle tracks
rise on all six segments, by 1.4 to 9.6, close to `prior`'s rises (0.6 to 11.3) and above them at
1st-mission and fulton-divisadero. By the spec's reading, the duplicates come from the update.
Footprint matching loses one `a0` moving track at columbus-broadway and none elsewhere (shortened
0 to 4, added 0 to 5).

## Reported, not decided

**Counts under the peak test.** The moving count rises on every segment, by less than under
`prior` on each: columbus-broadway +17.8 % (credited for joins +20.5 %; `prior` +28.8 %),
embarcadero-bay +13.2 % (+15.4 %), marina-broderick +7.2 % (+13.0 %), 1st-mission +14.2 %
(+19.4 %), fulton-divisadero +5.4 % (+12.9 %), franklin-mcallister +10.2 % (+13.9 %). p85 moves by
−0.30 to +0.55 m/s.

**Confirmation.** The share of born tracks that confirm sits between `a0`'s and `prior`'s:

| Case                | `a0` born; confirmed (share) | `prior` born; confirmed (share) | `gated` born; confirmed (share) |
| ------------------- | ---------------------------- | ------------------------------- | ------------------------------- |
| kirk0               | 148; 76 (51.4 %)             | 145; 87 (60.0 %)                | 146; 78 (53.4 %)                |
| columbus-broadway   | 1,285; 569 (44.3 %)          | 1,263; 627 (49.6 %)             | 1,282; 576 (44.9 %)             |
| embarcadero-bay     | 625; 245 (39.2 %)            | 581; 269 (46.3 %)               | 588; 245 (41.7 %)               |
| marina-broderick    | 338; 147 (43.5 %)            | 319; 151 (47.3 %)               | 323; 140 (43.3 %)               |
| 1st-mission         | 1,514; 794 (52.4 %)          | 1,475; 842 (57.1 %)             | 1,503; 814 (54.2 %)             |
| fulton-divisadero   | 557; 271 (48.7 %)            | 517; 301 (58.2 %)               | 519; 268 (51.6 %)               |
| franklin-mcallister | 965; 391 (40.5 %)            | 921; 440 (47.8 %)               | 952; 406 (42.6 %)               |
| six segments        | 5,284; 2,417 (45.7 %)        | 5,076; 2,630 (51.8 %)           | 5,167; 2,449 (47.4 %)           |

Confirmation time was not read, as in l5est0009.

**Criterion 2's partial correlation after 1 s.** `a0` r 0.273 (5th percentile 0.255), `prior`
0.287 (0.269), `gated` 0.285 (0.265). Over all one-step residuals the pool reads 0.278 (0.259)
under `gated`.

**kirk0's fragmentations.** 46 → 31 (bootstrap delta 5th to 95th percentile −41 to 0).

**Live jerk.** The live filter's share of jerk samples above 15 m/s³ is 8.4 % under `a0` and 8.8 %
under `gated` on the six segments.

## Interpretation

**What was measured.** Restoring the shipped gate for tentative tracks keeps every consistency
figure l5est0009 gained (V1, V2) and removes a measurable part of its cost: brief tracks, clutter,
the confirmation share, the moving-count rise and kirk0's switch point estimate move back towards
`a0` or beyond it. Those were the widened gate's. What the gate does not remove is the duplicate
rise, which is as large as under `prior`, and most of the invented speed: the gated tracks at
columbus-broadway and franklin-mcallister were admitted by the shipped gate and then took a fast
velocity from one or two updates whose gain on the velocity is the wide prior's.

**Why the duplicates survive.** A plausible reading, not tested here: under the wide prior a
newborn's second update moves its velocity most of the way to the displacement between its first
two clusters, so a fragment or a second hypothesis on the same vehicle acquires the vehicle's speed
within a frame and keeps pace with it instead of falling behind and dying tentative. The gate
decides which cluster it takes, not how fast it then moves; two tracks that both move at the
vehicle's speed overlap for longer, and the pair-frames rise with them (1,136 to 1,366 at
columbus-broadway, 817 to 1,411 at franklin-mcallister).

**What the spec warned of.** "Gating with a covariance the state does not hold is a deliberate
inconsistency for a tentative track's first frames." It applies from birth to the fourth observed
frame. The figures show no harm from it (the young residuals are as consistent as under `prior`),
and no gain on the clause it was built for.

## Limitations

- **A screen read twice.** The six segments and kirk0 were read for this question by l5est0009,
  and this form was chosen after its failure. Its passes and fails say where the cost sits on this
  data; they do not say how the form reads elsewhere.
- **The falsifier is a reading of one figure.** Duplicates rising with the gate restored put the
  cost outside the gate; the mechanism above is an inference from the pair-frames and the fast
  tracks, not a measured one.
- **No speed reference off kirk0.** z RMS reads the filter's consistency with itself; the rows
  over 15 m/s are inferred to be invented from their clusters and observed rows, as in l5est0009.
- **kirk0's labels** are the reconstructed `kirk0-reference` pack (`f0ed3414…`), on the tuning
  split, not held out; 19 objects, so the bootstrap is wide.
- **`a0` partly reused.** Five segments' `a0` gest1 records are l5est0009's, reused after `a0`
  reproduced on kirk0 and marina-broderick and every box row reproduced on all six. The track
  records' random identifiers differ between runs.
- **The shadow covers the four-state filter.** Under the solid body's tracked extent state, or the
  two-point update (refused), it would not follow every step; neither is on at the defaults.
- **Scratch eligibility,** as l5est0009: a scratch cohort manifest that assigns each segment by
  hash to its site as development data, a fresh copy of the use ledger per replay.
  embarcadero-folsom was not downloaded or read.

## Recommendations

1. **Keep `velocity_prior_init_7p5_gated` and `velocity_prior_init_7p5` off.** The gated form fails
   V3, V4 and V5; it is not named for the second segments or label0003's spans, and no default
   changes.
2. **Look at the update, not the gate, for the remaining cost.** The falsifier holds: the
   duplicates and most of the invented speed survive the shipped gate. A next form would keep the
   honest variance but limit what a tentative track's first updates may believe of its velocity,
   for instance by holding a newborn's published speed until the velocity variance falls below a
   bound, or by keeping the wide variance only for tracks whose clusters are vehicle-sized.
   - Falsified if: under such a form, duplicate pairs per hundred vehicle tracks still rise on
     more than two of the six segments.
3. **Keep the gate's half as evidence.** Brief tracks, clutter and kirk0's switches were the
   widened gate's; any later wide-prior form should gate tentative tracks as shipped, and the
   option here is the way to do it.

## Provenance

| Item           | Value                                                                                                                                                                                                                                                                                                                                                                                       |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base           | `2bb363e` (spec approved, rule and falsifier committed before the option existed)                                                                                                                                                                                                                                                                                                           |
| Commits        | code `c112218` (`VelocityPriorGated`, `velocity_prior_init_7p5_gated`, tests), before the replays; report, spec answer, DEVLOG and maths note in the commit after                                                                                                                                                                                                                           |
| Builds         | built at `c112218` with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA` `c1122188…`) and the `pcap` tag; every replay header reads `build_version` 0.5.1-pre52, `build_git_sha` `c1122188…`. `lidar-state-estimation-baseline` `sha256:899eba8c…`, `lidar-box-eval` `sha256:805d34ec…`, `lidar-ground-truth-eval` `sha256:8fa1d0c4…`, `lidar-gest1-pool` `sha256:86828d53…` |
| Parameter sets | `config/tuning.defaults.json` (`sha256:9ad7377f…`), fingerprint `ab0c4035780d`, unchanged, for both arms; replay parameter hashes `ab0c4035780d…` (`a0`) and `e023e46b…` (`gated`, with `velocity_prior_init_7p5_gated` folded in)                                                                                                                                                          |
| kirk0          | capture `sha256:2864ebde…` (Git LFS); reconstructed pack `ds_67842cb1b22b70e7` rebuilt to `sha256:f0ed3414…`, split `kirk0-tuning` (`7194bb61…`), `obj_788622b5` ignored; tracking baselines `79d9abff…` (`a0`, equal to l5est0009's) and `f99f269d…` (`gated`), first equal to repeat                                                                                                      |
| The lot        | l5est0009's: `random.Random(202611101).sample` over the sorted names of the 21 corpus sites other than ashbury-downey, haight-divisadero and embarcadero-folsom: columbus-broadway, embarcadero-bay, marina-broderick, 1st-mission, fulton-divisadero, franklin-mcallister                                                                                                                  |
| Segments       | fetched again; placement and segment hashes equal l5est0009's: columbus-broadway `aa69875b` → `774c5434`, embarcadero-bay `50ccf70b` → `5a533b67`, marina-broderick `1d7d876b` → `c8141c2f`, 1st-mission `cef1cf1b` → `83160e99`, fulton-divisadero `ae6c6f06` → `eda80c97`, franklin-mcallister `e6601525` → `ab9722de`                                                                    |
| Sites          | all six ran `gated` through both tools and `a0` through the box-eval; embarcadero-folsom (held out) was not downloaded or read                                                                                                                                                                                                                                                              |
| Host           | Claude Code cloud container, 4 cores shared with another experiment agent, `GOMAXPROCS=2`, `nice -n 5`, two lanes; replays 12:34 to 13:07 UTC (gest1 166 to 346 s, box-eval 79 to 248 s per segment); raw output in the session's scratch, not kept                                                                                                                                         |
