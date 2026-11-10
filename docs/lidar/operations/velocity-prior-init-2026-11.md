# An honest velocity prior at birth, screened on kirk0 and six lot-drawn segments

<!-- ignore-style-length -->

l5est0007 found the CV filter's along-track over-dispersion concentrated in each track's first
second, under a birth prior of zero velocity with a variance of 1 m²/s². This run gives the
newborn track an honest prior instead: the velocity still zero, its variance (7.5 m/s)² per axis.
It reads the candidate on kirk0 for identity and on the first five-minute segments of six sites
drawn by lot, against a rule committed before the option existed. It answers
[l5est0009](../../../data/experiments/l5est/l5est0009.md).

- **Status:** Screen complete, 2026-11-10. Fails V3, V4 and V5; nothing adopted
- **Layers:** L5 (track initialisation), read by L8 analytics and the box rows
- **Related:** [G-EST-1 corpus read (l5est0007)](gest1-corpus-read-2026-11.md),
  [two-point initialisation guards (l5est0006)](two-point-init-guards-2026-11.md),
  [likelihood birth gate on the lot (l5id0024)](likelihood-birth-gate-lot-2026-11.md),
  [tracking maths §2.3](../../../data/maths/tracking-maths.md#honest-velocity-prior-at-birth),
  [maths review §4.2](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md#42-velocity-initialisation)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) (`ab0c4035780d`) for both
  arms; `prior` adds the replay experiment `velocity_prior_init_7p5` (replay parameter hash
  `48106d7e…`)

## Answer

**The candidate fails V3, V4 and V5, so it is not named for the second segments or label0003's
spans.** It does what it was built for: the first second's along-track z RMS falls from 2.24 to
0.80 (−64 %), and after the first second neither axis moves by more than 0.04 (V1 and V2 pass).
The cost is at birth itself. A newborn track whose velocity variance is 56 m²/s² can take a
cluster anywhere within its widened gate, so fragments and sparse clutter that the shipped prior
lets die confirm instead. Duplicate pairs per hundred vehicle tracks rise on all six segments,
four segments write rows over 15 m/s, three where `a0` writes none, and kirk0 gains 27 switches.

| Clause                     | Deciding figures                                                                                                                                                                                     | Result                     |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- |
| V1, the first second       | along z RMS at age under 1 s: `a0` 2.244, `prior` 0.802 (−64.3 %); `prior`'s whole-track bootstrap 95th percentile 0.835, under `a0`'s 2.244                                                         | **Pass**                   |
| V2, after the first second | along after 1 s 0.802 → 0.764 (−0.038); across after 1 s 0.367 → 0.359 (−0.008)                                                                                                                      | **Pass**                   |
| V3, no invented speed      | scored-row p98 change over 0.5 m/s at 1st-mission (+0.93), fulton-divisadero (+2.66), franklin-mcallister (+2.37); new rows over 15 m/s at columbus-broadway, fulton-divisadero, franklin-mcallister | **Fail** on 4 of 6         |
| V4, identity               | kirk0 paired bootstrap: HOTA delta 5th percentile −0.0171, IDF1 −0.0383 (bars −0.005); switch change 95th percentile +85 (bar +10)                                                                   | **Fail** on all three bars |
| V5, noise                  | noise costs on 6 of 6 segments (bar: two at most); duplicate pairs per hundred vehicle tracks rise on all six                                                                                        | **Fail**                   |

**Recommendation:** keep the option off. The first-second finding stands and now has a measured
cause: the prior, not acceleration and not the process noise. The next form should keep the
honest velocity variance for the filter's update but not for the newborn's gate (or gate newborns
on position alone), so that a wider prior cannot widen what a tentative track may take.

## Rule

Committed in the spec (`53d1eeb`) before the option existed; quoted in short:

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
- **Outcome.** A pass on V1 to V5 names `prior` for the corpus's second segments and label0003's
  spans; it is not adopted on a screen. A fail names its clause.

## Arms and the option

| Arm     | Parameter set            | Replay experiments        | Birth velocity variance per axis |
| ------- | ------------------------ | ------------------------- | -------------------------------- |
| `a0`    | defaults, `ab0c4035780d` | none                      | 1 m²/s² (shipped)                |
| `prior` | defaults, `ab0c4035780d` | `velocity_prior_init_7p5` | (7.5 m/s)², 56.25 m²/s²          |

`TrackerConfig.VelocityPriorInit` (commit `eed21db`) sets the two velocity diagonals of the
founding covariance in `initTrack` and nothing else: the velocity stays zero, the position
variance stays 10 m², and the position-velocity covariance stays zero. The covariance cap (100 m²
at the defaults) is above the prior, and the prediction caps it as it caps the shipped one. With
the option off, every pinned two-point and hygiene scene keeps its shipped digest. On a noise-free
6.5 m/s vehicle the velocity is within 10 % of the truth from the fourth update under the option
and from the eleventh under the shipped prior (`TestVelocityPriorInitConvergesByTheFourthUpdate`).
The tuning file is unchanged, so both arms run under the same fingerprint.

## Method

- **Build.** Every tool built at `eed21db` through the Makefile's LDFLAGS with the `pcap` tag;
  every replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `eed21db8…`.
- **kirk0.** `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false
-sample-points 1024`, evidence per case, repeat-verified, the committed cohort manifest and a
  scratch copy of the run's use ledger per replay. Scored with `lidar-ground-truth-eval perframe`
  on the reconstructed kirk0 pack (`f0ed3414…`), split `kirk0-tuning`, `obj_788622b5` ignored,
  `-bootstrap 2000 -bootstrap-seed 20261108`, `prior` against `a0`.
- **Segments.** The first 300 s of each lot-drawn placement from `banshee-data/sf-street-speeds`,
  cut by the run's pcapng window filter, one placement at a time, each placement deleted once cut
  and each segment once both arms' tools had run. Each segment's hash equals l5est0007's.
- **The gest1 replay.** `lidar-state-estimation-baseline`, one case per run, `-warmup 40
-require-settled=false -sample-points 1024 -gest1-report`, repeat-verified (all 12 runs
  `baseline_equal: true`), a scratch cohort manifest that assigns each segment by hash to its
  site as development data, and a fresh copy of the use ledger per replay. `lidar-gest1-pool`
  pooled the six cases per arm.
- **V1 and V2.** The pool's age stratum (residuals under 1 s since the track's first
  tracked-filter observation) is the clause's "1 s or less": no one-step residual sits at exactly
  1 s, so the two readings are equal. The whole-track bootstrap (2,000 draws, seed 20261108) and
  the across-track figure by age are a scratch script over the same per-case track records; it
  reproduces the pool's stratum figures exactly (columbus-broadway 2.131 and 0.890 under `a0`).
- **The box rows.** `lidar-box-eval -variants none -require-settled=false -sample-points 1024
-warmup 40 -duration 120 -rows -evidence-db`, one run per arm per segment; the gest1 replay does
  not write rows or duplicates, so two tools ran. Then `lidar_noise_screen.py` and
  `lidar_clutter_origin.py` on each arm's evidence, and `lidar_figure_sensitivity.py --moving peak
--joins` and `lidar_flip_cost.py --match footprint` on `prior` against `a0`.
- **V3.** The scored rows' p98 is `lidar_figure_sensitivity.py` on the two rows CSVs at its default
  moving test, as l5est0006 read it; the peak test's figure is reported beside it. The fastest row
  and the count over 15 m/s are read from the rows directly.
- **V5.** l5id0019's R4 noise half: an arm costs noise on a segment when brief tracks rise by more
  than 3, clutter by more than 5 per hundred of `a0`'s moving tracks (peak test), or duplicate
  pairs per hundred vehicle tracks rise at all.

## kirk0 first: does `a0` reproduce, and V4

`a0` reproduces the anchor exactly: HOTA 0.4284, IDF1 0.4728, 90 switches, 46 fragmentations,
tracking baseline `79d9abff…` on both passes.

| Measure                               |        `a0` |     `prior` |       Δ |
| ------------------------------------- | ----------: | ----------: | ------: |
| HOTA                                  |      0.4284 |      0.4208 | −0.0075 |
| IDF1                                  |      0.4728 |      0.4578 | −0.0150 |
| ID switches                           |          90 |         117 |     +27 |
| Fragmentations                        |          46 |          34 |     −12 |
| False negatives (of 2,472)            |         265 |         251 |     −14 |
| Labelled-object HOTA, bootstrap point |      0.4798 |      0.4772 |         |
| Tracks born; confirmed                |     148; 76 |     145; 87 |         |
| Tracking baseline (first = repeat)    | `79d9abff…` | `ae343438…` |         |

Paired bootstrap, `prior` less `a0` (2,000 draws, seed 20261108):

| Delta          |      p5 |     p50 |     p95 | Bar             | Holds |
| -------------- | ------: | ------: | ------: | --------------- | ----- |
| HOTA           | −0.0171 | −0.0029 | +0.0180 | p5 above −0.005 | No    |
| IDF1           | −0.0383 | −0.0065 | +0.0341 | p5 above −0.005 | No    |
| ID switches    |     −27 |     +25 |     +85 | p95 +10 or less | No    |
| Fragmentations |     −38 |     −12 |      +2 | reported        |       |

**V4 fails on all three bars.** The candidate fragments less (34 against 46) and misses fewer
reference points, but switches more: tracks that confirm sooner and hold a wider gate take the
wrong cluster more often.

## V1 and V2: the first second and after

One-step along-track and across-track z of the moving tracks' medoid residuals, pooled over the
six segments, with the whole-track bootstrap's 95 % interval:

| Arm     | Age       | Axis   |       n | Tracks | z RMS | Bootstrap 95 % | Beyond 1.96 |
| ------- | --------- | ------ | ------: | -----: | ----: | -------------- | ----------: |
| `a0`    | under 1 s | along  |   6,863 |  1,128 | 2.244 | 2.187 to 2.303 |      41.0 % |
| `prior` | under 1 s | along  |   9,064 |  1,339 | 0.802 | 0.765 to 0.840 |       3.3 % |
| `a0`    | 1 s on    | along  | 107,970 |  1,209 | 0.802 | 0.778 to 0.827 |       3.0 % |
| `prior` | 1 s on    | along  | 105,805 |  1,376 | 0.764 | 0.740 to 0.788 |       2.6 % |
| `a0`    | under 1 s | across |   6,863 |  1,128 | 0.447 | 0.425 to 0.473 |       0.6 % |
| `prior` | under 1 s | across |   9,064 |  1,339 | 0.387 | 0.366 to 0.411 |       0.4 % |
| `a0`    | 1 s on    | across | 107,970 |  1,209 | 0.367 | 0.353 to 0.381 |       0.3 % |
| `prior` | 1 s on    | across | 105,805 |  1,376 | 0.359 | 0.346 to 0.373 |       0.3 % |
| `a0`    | all       | along  | 114,833 |  1,240 | 0.951 | 0.925 to 0.978 |       5.3 % |
| `prior` | all       | along  | 114,869 |  1,414 | 0.767 | 0.744 to 0.790 |       2.6 % |

**V1 passes.** The first second falls by 64.3 %, against a bar of 30 %, and `prior`'s bootstrap
95th percentile (0.835) is far under `a0`'s 2.244. **V2 passes.** After the first second the along
figure falls by 0.038 and the across figure by 0.008, both within 0.05.

The young residuals' share of the along tail beyond 1.96 falls from 46.6 % to 10.0 %. The pooled
along RMS of `a0` on these six sites is 0.951, under 1, so this subset is not over-dispersed as the
23-site pool was (1.073); its first second (2.24) is as l5est0007 read it (2.52 on 23 sites).

Per site (along z RMS under 1 s and from 1 s; across from 1 s):

| Site                | `a0` under 1 s (n) | `prior` under 1 s (n) | `a0` 1 s on | `prior` 1 s on | `a0` across 1 s on | `prior` across 1 s on |
| ------------------- | -----------------: | --------------------: | ----------: | -------------: | -----------------: | --------------------: |
| columbus-broadway   |      2.131 (1,648) |         0.906 (2,226) |       0.890 |          0.829 |              0.375 |                 0.368 |
| embarcadero-bay     |        2.619 (868) |         0.717 (1,082) |       0.730 |          0.692 |              0.396 |                 0.390 |
| marina-broderick    |        2.336 (605) |           0.645 (752) |       0.832 |          0.814 |              0.285 |                 0.281 |
| 1st-mission         |      1.615 (1,473) |         0.718 (1,939) |       0.662 |          0.648 |              0.353 |                 0.347 |
| fulton-divisadero   |        2.187 (877) |         0.825 (1,226) |       0.846 |          0.797 |              0.408 |                 0.403 |
| franklin-mcallister |      2.651 (1,392) |         0.841 (1,839) |       0.892 |          0.838 |              0.379 |                 0.359 |

Every site moves the same way. `a0`'s whole-track per-site figures equal l5est0007's (for example
1st-mission 0.739, columbus-broadway 1.032).

## V3: invented speed

Scored rows (40 s warm-up, 120 s scored). p98 and p85 of the moving tracks' maximum speeds at the
default moving test, in m/s; the peak test's p98 in brackets.

| Site                | `a0` moving | `prior` moving | `a0` p85 | `prior` p85 | `a0` p98      | `prior` p98   | p98 Δ     | `a0` fastest; rows over 15 | `prior` fastest; rows over 15 | V3   |
| ------------------- | ----------: | -------------: | -------: | ----------: | ------------- | ------------- | --------- | -------------------------- | ----------------------------- | ---- |
| columbus-broadway   |         159 |            193 |     9.94 |       10.73 | 13.45 (13.45) | 13.94 (13.84) | +0.49     | 14.49; 0                   | 20.47; **35**                 | Fail |
| embarcadero-bay     |          72 |             83 |    11.20 |       11.62 | 15.59 (15.59) | 15.97 (15.97) | +0.38     | 15.84; 122                 | 16.46; 216                    | Pass |
| marina-broderick    |          81 |             88 |    11.13 |       11.33 | 13.53 (13.53) | 13.53 (13.53) | 0.00      | 13.80; 0                   | 13.80; 0                      | Pass |
| 1st-mission         |         227 |            266 |     7.38 |        7.65 | 9.74 (9.74)   | 10.66 (10.66) | **+0.93** | 11.10; 0                   | 13.68; 0                      | Fail |
| fulton-divisadero   |          82 |             94 |     8.68 |        8.59 | 11.84 (11.84) | 14.50 (12.54) | **+2.66** | 12.33; 0                   | 17.66; **25**                 | Fail |
| franklin-mcallister |         124 |            156 |    10.82 |       11.88 | 12.50 (12.50) | 14.87 (16.07) | **+2.37** | 14.89; 0                   | 19.42; **92**                 | Fail |

**V3 fails on four of six segments.** The fast rows come from tracks born on small or sparse
clusters. At columbus-broadway the 35 rows are one track born on a 0.4 by 0.2 m cluster of 15
returns at 20 m range, observed in two fast rows and coasting at up to 20.5 m/s for the rest. At
fulton-divisadero one track of 11 returns coasts 25 rows at up to 17.7 m/s, observed in one.
Franklin-mcallister's 92 rows are eight tracks, most a single fast observed row on a cluster of
11 to 54 returns, and one 53-row track that coasts at up to 19.4 m/s 7 m from the sensor. At
embarcadero-bay both arms write rows over 15 m/s (`a0` on two whole vehicles); under `prior` they
are two tracks on 10- and 11-return clusters at 63 and 94 m. Under the shipped prior such a
newborn is held near zero velocity and its next cluster falls outside the gate; under the honest
prior one association gives it a speed and it can confirm and coast on it.

## V5: noise

Each segment's figures, then the R4 noise test (candidate less `a0`). Clutter classes are
artefact / stationary road user / fragment of a mover / unclassed.

| Site                | Arm     | Tracks | Brief | Clutter (art / stat / frag / uncl) | Moving, peak | p85, p98 peak (m/s) | Vehicle tracks | Dup pairs (per 100) | Pair-frames | Confirmed |
| ------------------- | ------- | -----: | ----: | ---------------------------------- | -----------: | ------------------- | -------------: | ------------------- | ----------: | --------: |
| columbus-broadway   | `a0`    |    338 |    58 | 72 (29 / 8 / 4 / 31)               |          146 | 10.39, 13.69        |            141 | 33 (23.4)           |       1,136 |       277 |
|                     | `prior` |    370 |    62 | 73 (31 / 6 / 5 / 31)               |          188 | 10.57, 13.76        |            164 | 52 (31.7)           |       1,978 |       303 |
| embarcadero-bay     | `a0`    |    176 |    22 | 36 (17 / 5 / 2 / 12)               |           91 | 11.21, 15.59        |             65 | 4 (6.2)             |         224 |       132 |
|                     | `prior` |    186 |    26 | 38 (21 / 4 / 1 / 12)               |          105 | 11.76, 13.53        |             81 | 14 (17.3)           |         903 |       140 |
| marina-broderick    | `a0`    |     99 |    12 | 14 (8 / 1 / 1 / 4)                 |           69 | 11.63, 13.63        |             62 | 13 (21.0)           |         638 |        94 |
|                     | `prior` |    107 |    13 | 15 (5 / 1 / 2 / 7)                 |           78 | 11.67, 13.63        |             74 | 20 (27.0)           |         602 |       101 |
| 1st-mission         | `a0`    |    506 |    59 | 97 (59 / 6 / 13 / 19)              |          232 | 7.81, 10.39         |            179 | 58 (32.4)           |       3,234 |       424 |
|                     | `prior` |    530 |    61 | 87 (49 / 4 / 14 / 20)              |          277 | 7.92, 11.10         |            198 | 70 (35.4)           |       2,282 |       447 |
| fulton-divisadero   | `a0`    |    168 |    21 | 33 (18 / 3 / 2 / 10)               |           93 | 9.41, 12.08         |             71 | 22 (31.0)           |         479 |       135 |
|                     | `prior` |    173 |    15 | 21 (16 / 0 / 2 / 3)                |          105 | 9.77, 11.84         |             76 | 24 (31.6)           |         753 |       140 |
| franklin-mcallister | `a0`    |    239 |    27 | 42 (22 / 4 / 3 / 13)               |          137 | 11.59, 12.95        |            134 | 33 (24.6)           |         817 |       198 |
|                     | `prior` |    273 |    35 | 51 (21 / 5 / 3 / 22)               |          156 | 12.14, 16.07        |            153 | 55 (35.9)           |       1,221 |       224 |

| Site                | Brief Δ | Clutter Δ per 100 | Dup pairs per 100 | Costs noise |
| ------------------- | ------: | ----------------: | ----------------- | ----------- |
| columbus-broadway   |  **+4** |              +0.7 | **23.4 → 31.7**   | Yes         |
| embarcadero-bay     |  **+4** |              +2.2 | **6.2 → 17.3**    | Yes         |
| marina-broderick    |      +1 |              +1.4 | **21.0 → 27.0**   | Yes         |
| 1st-mission         |      +2 |              −4.3 | **32.4 → 35.4**   | Yes         |
| fulton-divisadero   |      −6 |             −12.9 | **31.0 → 31.6**   | Yes         |
| franklin-mcallister |  **+8** |          **+6.6** | **24.6 → 35.9**   | Yes         |

**V5 fails: noise costs on six of six segments**, against two allowed. The duplicate condition
alone costs on every segment, by 0.6 to 11.3 per hundred. No `a0` moving track is lost by
footprint on any segment (shortened 0 to 7, added 0 to 6).

## Reported, not decided

**Counts under the peak test.** The moving count rises on every segment: columbus-broadway
+28.8 % (credited for joins +31.5 %), embarcadero-bay +15.4 %, marina-broderick +13.0 %,
1st-mission +19.4 %, fulton-divisadero +12.9 %, franklin-mcallister +13.9 %. p85 moves by +0.04
to +0.55 m/s. These are rises, not losses: more vehicles are carried in two moving tracks, and
more short movers confirm.

**Confirmation.** The share of born tracks that confirm rises at every site, and on kirk0:

| Case                | `a0` born; confirmed (share) | `prior` born; confirmed (share) |
| ------------------- | ---------------------------- | ------------------------------- |
| kirk0               | 148; 76 (51.4 %)             | 145; 87 (60.0 %)                |
| columbus-broadway   | 1,285; 569 (44.3 %)          | 1,263; 627 (49.6 %)             |
| embarcadero-bay     | 625; 245 (39.2 %)            | 581; 269 (46.3 %)               |
| marina-broderick    | 338; 147 (43.5 %)            | 319; 151 (47.3 %)               |
| 1st-mission         | 1,514; 794 (52.4 %)          | 1,475; 842 (57.1 %)             |
| fulton-divisadero   | 557; 271 (48.7 %)            | 517; 301 (58.2 %)               |
| franklin-mcallister | 965; 391 (40.5 %)            | 921; 440 (47.8 %)               |
| six segments        | 5,284; 2,417 (45.7 %)        | 5,076; 2,630 (51.8 %)           |

Fewer tracks are born (a newborn holds its vehicle rather than being re-born) and more of them
confirm. Confirmation time was not read: no tool records the frame a track confirms on, and
reading it would need instrumentation this spec did not declare.

**Criterion 2's partial correlation after 1 s.** `z_along²` against the smoothed |longitudinal
acceleration|, controlling for range, aspect and point count, on one-step residuals 1 s or more
into the track: `a0` r 0.273 (500-draw track bootstrap 5th percentile 0.255), `prior` 0.287
(0.269). Over all one-step residuals the pool reads 0.219 (5th percentile 0.203) and 0.278
(0.258). The acceleration signal is unchanged after the first second, and over the whole track it
strengthens once the young residuals stop diluting it.

**kirk0's fragmentations.** 46 → 34 (bootstrap delta 5th to 95th percentile −38 to +2).

**Live jerk.** The live filter's share of jerk samples above 15 m/s³ is 8.4 % under `a0` and 8.8 %
under `prior` on the six segments.

## Interpretation

**What was measured.** On six lot-drawn first segments, the honest prior removes the first
second's over-dispersion almost entirely: young residuals read 0.80 against 2.24, the same as the
mature ones (0.76), and nothing after the first second gets worse. The excess l5est0007 found was
the birth prior, as the maths review predicted. That is a measurement of filter consistency with
itself, not of speed against a reference.

**What it costs.** The prior's variance is also the newborn's gate. Under the shipped prior a
tentative track's predicted position barely moves and its innovation covariance is about the
position's, so a cluster that is not near where the track began is not associated, and a fragment
or a scrap of clutter dies tentative. Under (7.5 m/s)² the innovation covariance one frame after
birth is about 0.56 m² wider per axis, so the second cluster can be taken from further away, the
velocity then jumps to whatever the pair implies, and the track confirms on it. The figures read
that way: more tracks confirm (45.7 % to 51.8 % of those born), duplicate pairs rise everywhere,
the moving count rises 13 to 29 %, and the fast rows are tracks born on sparse clusters that
coast at the speed one association gave them. It is the defect two-point initialisation showed
in l5est0006 by a different route: the speed of a newborn is believed after one pair. On kirk0 the
same mechanism trades 12 fewer fragmentations for 27 more switches.

**What the spec warned of.** "A wide prior also widens the young track's gate, so a newborn can
take a neighbour's cluster: V4 and V5 are there for that." They caught it.

**The subset.** `a0`'s pooled along RMS on these six sites is 0.951, under 1, where l5est0007's 23
sites read 1.073; the six are not the over-dispersed half of the corpus. The first second is as
over-dispersed here (2.24) as there (2.52), so V1 reads the effect the spec was written for.

## Limitations

- **A screen, not a held-out read.** l5est0007 read these segments under `a0`; the first second
  was named as the place to look after that read. V1 tests the candidate there, not the finding.
- **No speed reference off kirk0.** z RMS reads the filter's consistency with itself. The rows
  over 15 m/s are inferred to be invented from their clusters' size and from observed rows being
  a few of each fast track's rows; no reference says what moved there.
- **kirk0's labels** are the reconstructed `kirk0-reference` pack (`f0ed3414…`), on the tuning
  split, not held out.
- **One value.** 7.5 m/s is a stated urban prior, not fitted. A smaller value would trade some of
  V1 for less gate widening; that is not read here.
- **Two tools per segment.** The gest1 replay scores 260 s and the box-eval 120 s of the same
  segment in separate replays; their figures are not track-for-track the same tracks.
- **V1's bootstrap and the age-split across figures** come from a scratch script over the pool's
  own track records; it reproduces the pool's stratum figures exactly. The partial correlation
  after 1 s uses a 500-draw bootstrap in the same script.
- **Confirmation time** was not read (see above).
- **Scratch eligibility.** The segments are not in the committed cohort manifest; they ran under
  a scratch manifest that assigns each by hash to its site as development data, with a fresh copy
  of the use ledger per replay. embarcadero-folsom was not downloaded or read.

## Recommendations

1. **Keep `velocity_prior_init_7p5` off.** It fails V3, V4 and V5; it is not named for the second
   segments or label0003's spans, and no default changes.
2. **Separate the newborn's gate from its velocity prior.** The consistency gain lives in the
   update; the costs come from association. A next form: keep the honest variance for the Kalman
   update, but gate and cost a tentative track's second association with the shipped prior's
   innovation covariance (or on position alone), so a newborn cannot reach further because its
   velocity is unknown. The rule should carry V1 to V5 unchanged, so the two forms read alike.
   - Falsified if: under that form duplicate pairs per hundred still rise on more than two of
     the six segments, which would put the cost in the update, not the gate.
3. **Read the young residuals again under any initialisation change.** The first-second stratum
   now has a measured cause and a reproducible figure (2.24 on these six, 2.52 on 23); it is the
   cheapest read of whether a birth change does what it claims.
4. **Treat a newborn's first fast pair as suspect** in any form: every invented fast track here,
   as in l5est0006, was a sparse cluster whose speed came from one association.

## Provenance

| Item           | Value                                                                                                                                                                                                                                                                                                                                                                                       |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base           | `53d1eeb` (spec approved, rule V1 to V5 committed before the option existed)                                                                                                                                                                                                                                                                                                                |
| Commits        | code `eed21db` (`VelocityPriorInit`, `velocity_prior_init_7p5`, tests), before the replays; report, spec answer, DEVLOG and maths note in the commit after                                                                                                                                                                                                                                  |
| Builds         | built at `eed21db` with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA` `eed21db8…`) and the `pcap` tag; every replay header reads `build_version` 0.5.1-pre52, `build_git_sha` `eed21db8…`. `lidar-state-estimation-baseline` `sha256:72b4d261…`, `lidar-box-eval` `sha256:c69a6334…`, `lidar-ground-truth-eval` `sha256:c514339b…`, `lidar-gest1-pool` `sha256:c094be70…` |
| Parameter sets | `config/tuning.defaults.json` (`sha256:9ad7377f…`), fingerprint `ab0c4035780d`, unchanged, for both arms; replay parameter hashes `ab0c4035780d…` (`a0`) and `48106d7e…` (`prior`, with `velocity_prior_init_7p5` folded in)                                                                                                                                                                |
| kirk0          | capture `sha256:2864ebde…` (Git LFS); reconstructed pack `ds_67842cb1b22b70e7` rebuilt to `sha256:f0ed3414…`, split `kirk0-tuning` (`7194bb61…`), `obj_788622b5` ignored; tracking baselines `79d9abff…` (`a0`) and `ae343438…` (`prior`), first equal to repeat                                                                                                                            |
| The lot        | `random.Random(202611101).sample` over the sorted names of the 21 corpus sites other than ashbury-downey, haight-divisadero and embarcadero-folsom, six drawn, run in the drawn order: columbus-broadway, embarcadero-bay, marina-broderick, 1st-mission, fulton-divisadero, franklin-mcallister (reproduced with broadway-franklin under either of its names)                              |
| Segments       | first 300 s of each placement, SHA-256 prefixes (placement → segment): columbus-broadway `aa69875b` → `774c5434`, embarcadero-bay `50ccf70b` → `5a533b67`, marina-broderick `1d7d876b` → `c8141c2f`, 1st-mission `cef1cf1b` → `83160e99`, fulton-divisadero `ae6c6f06` → `eda80c97`, franklin-mcallister `e6601525` → `ab9722de`; each equals l5est0007's                                   |
| Sites          | all six ran both arms through both tools; embarcadero-folsom (held out) was not downloaded or read                                                                                                                                                                                                                                                                                          |
| Host           | Claude Code cloud container, 4 cores shared with another experiment agent, `GOMAXPROCS=2`, `nice -n 5`, two lanes; replays 11:03 to 12:17 UTC (gest1 354 to 573 s, box-eval 135 to 354 s per segment); raw output in the session's scratch, not kept                                                                                                                                        |
