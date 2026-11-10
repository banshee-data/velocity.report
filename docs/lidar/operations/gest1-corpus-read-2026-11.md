# G-EST-1 restated and read on the corpus's first segments

<!-- ignore-style-length -->

l5est0005's kirk0 screen passed G-EST-1's acceleration test on a filter that was not
over-dispersed, so the test could not say whether constant velocity (CV) fails. This run restates
criterion 2 so that it asks first for over-dispersion along the track, then for acceleration as
its cause, and reads it label-free on the first five-minute segment of the 23 tuning and screen
sites, with the velocity process noise at two further values. It answers
[l5est0007](../../../data/experiments/l5est/l5est0007.md).

- **Status:** Measurement complete, 2026-11-10. No default changed; no motion model ships
- **Layers:** L5 (the tracked filter, the shadow solid body), read by L8 analytics
- **Related:** [state plan §7.3](../../plans/lidar-state-estimation-plan.md#73-recommendation-and-gate),
  [kirk0 screen (l5est0005)](gest1-face-state-kirk0-2026-11.md),
  [motion models maths](../../../data/maths/motion-models-maths.md#10-g-est-1-analyses),
  [maths review §4.3](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md#43-why-ca-must-wait-for-the-measurement)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) (`ab0c4035780d`) for
  `a0` and `face_state`, [process-noise-vel-x2](../../../config/experiments/process-noise-vel-x2.json)
  (`deb48e282fa2`) and [process-noise-vel-x4](../../../config/experiments/process-noise-vel-x4.json)
  (`e026f818d678`)

## Answer

**M0 is not met, so by the spec's rule this read decides nothing.** The 23 first segments hold
4,117 confirmed moving tracks with scored residuals under `a0` (4,459 moving tracks in all),
against the 5,000 that G-EST-1's criterion 1 asks for. The second segments
(`d2-second-segment`, 22 sites) are named for the same read. Every reading below is reported with
its count, as the spec requires, and none of them is a decision.

What the readings would say, read against their bars:

| Reading                   | Figure                                                                                                                                    | Against the bar                                |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| M0, enough tracks         | 4,117 `a0` tracks with residuals                                                                                                          | under 5,000: not met                           |
| M1, over-dispersion along | `a0` one-step along z RMS 1.073 (track bootstrap 1.057 to 1.090), 7.0 % beyond 1.96                                                       | both halves hold                               |
| M2, is it acceleration    | `z_along²` partial r 0.188, bootstrap 5th percentile 0.177 (medoid); 0.137, 0.118 (one-step faces)                                        | above 0.1 on both                              |
| M3, a looser CV           | `vel-x2` along 0.987, across 0.355 against `a0`'s 0.392; `vel-x4` along 0.910, across 0.325, live jerk share 19.2 % against `a0`'s 10.4 % | neither passes; the IMM's along ratio is 0.977 |
| M4, the strata            | residuals under 1 s into their track: along z RMS 2.52, 47.6 % beyond, half of the pool's tail                                            | the pool is over 1, so the trigger is not met  |

Two findings matter more than the readings, and both would survive the count:

1. **The over-dispersion is the first second of each track.** Residuals less than 1 s after a
   track's first observation are 7.6 % of the one-step along values and carry 51.6 % of those
   beyond 1.96. Without them the pooled medoid reads RMS 0.85 and 3.7 % beyond, under-dispersed as
   on kirk0. A post-hoc split shows the partial correlation with acceleration is −0.02 on the young
   residuals and 0.26 on the rest: the excess is not acceleration, and where acceleration does
   track the residual, the filter already expects it.
2. **No process-noise value fixes both axes.** Doubling the velocity process noise brings along z
   RMS to 0.99 but pushes the across-track RMS, already 0.39, further from 1 and raises the live
   filter's jerk share from 10.4 % to 14.0 %. The across-track slack is the measurement noise
   (G-UNC-1), and the young-track excess is initialisation.

**Recommendation:** ship nothing. Read the same report on the first and second segments pooled
(M0's named route), with track age as a declared stratum, and look at the first second of a track
before any motion model: that is where the filter is wrong.

## Question and arms

Is the CV filter over-dispersed along the track at all, on the corpus, and if so does
acceleration explain it? The arms:

| Arm          | Parameter set                               | Replay experiments                    | Residual source read | What it measures                                |
| ------------ | ------------------------------------------- | ------------------------------------- | -------------------- | ----------------------------------------------- |
| `a0`         | defaults, `ab0c4035780d`                    | none                                  | `tracked_filter`     | The shipped CV filter on the cluster medoid     |
| `face_state` | defaults, `ab0c4035780d`                    | `solid_body`, `solid_body_face_state` | `solid_body_faces`   | The shadow solid body's one-step face residuals |
| `vel-x2`     | `process-noise-vel-x2.json`, `deb48e282fa2` | none                                  | `tracked_filter`     | `l5.cv_kf_v1.process_noise_vel` 0.4             |
| `vel-x4`     | `process-noise-vel-x4.json`, `e026f818d678` | none                                  | `tracked_filter`     | `l5.cv_kf_v1.process_noise_vel` 0.8             |

`a0` and `face_state` have byte-equal tracks (the solid body is a shadow), so one `face_state`
replay per site produced both residual records: its `tracked_filter` section is `a0`'s. That was
checked twice. On kirk0 both arms were replayed and their tracking baselines
(`sha256:79d9abff…`) and `tracked_filter` sections are identical. On haight-divisadero's first
segment a separate `a0` replay gave the same tracking baseline (`sha256:c24f7d2a…`), the same
report section, and the same track records once each run's random track IDs are set aside. The
offline CV, CA with the coast rule and IMM run inside `-gest1-report` at their prototype defaults
on every arm's record; none is live.

## Method

The rule (M0 to M4) was committed in the spec before any replay (`ab19e51`). The code is
`6cb4735`, committed before the replays.

- **Replays.** `lidar-state-estimation-baseline`, one case per run, `-warmup 40
-require-settled=false -sample-points 1024 -gest1-report`, every run repeat-verified;
  `face_state` with `-experiment solid_body,solid_body_face_state`, `-evidence-dir`,
  `-evidence-per-case`, `-discard-evidence` and a source manifest. kirk0 ran first, whole, at
  l5est0005's `-warmup 6`.
- **Population.** As l5est0005: confirmed chains with at least 10 observations whose smoothed speed
  reaches 2 m/s, their residuals in the scoring window after the warm-up.
- **One-step residuals.** Prior predicted across at most 0.15 s since the filter's previous update,
  and no re-reference (a face fix that re-referenced a medoid-referenced body is left out).
- **Restated criterion 2, dispersion.** `z = along / √(uᵀSu)`, pooled RMS and the share of `|z|`
  beyond 1.96, per track (tracks with at least five values) and pooled, with a 95 % interval over
  2,000 seeded resamples of whole tracks; across-track the same.
- **Restated criterion 2, cause.** The partial correlation of `z_along²` with the smoothed
  |longitudinal acceleration| (l5est0005's estimator: the RTS-smoothed offline CV, centred
  difference at ±0.5 s), controlling for range, aspect and point count, with the whole-track
  bootstrap's 5th percentile.
- **Strata.** One-step along z RMS by smoothed speed, range, aspect, point count and track age
  (time since the track's first tracked-filter observation, under or over 1 s).
- **Jerk** (Punzo et al. 2011). On each estimator's output (the live filter's posterior, or the
  solid body's own; offline CV, CA with the coast rule and the IMM over the scored observations):
  `a = Δ|v|/Δt` over consecutive intervals of at most 0.15 s at speeds of at least 0.5 m/s, `j` the
  difference of consecutive `a` over the mean interval. The checks are the share of jerk samples
  above 15 m/s³ and the jerk sign changes per second.
- **Turning.** The smoothed heading's centred rate over the acceleration's window where the
  smoothed speed reaches 2 m/s, as the share of track-frames above 0.1 and 0.2 rad/s.
- **Pooling.** Each case writes its moving tracks' records; `lidar-gest1-pool` reads the 23 cases
  pooled, each case alone, and the pool with each case left out. Definitions are in the
  [motion models maths](../../../data/maths/motion-models-maths.md#10-g-est-1-analyses).

## kirk0 first: does l5est0005 reproduce?

The tracked filter reproduces exactly. The face-referenced source does not, and the difference
is on main, not in this change.

| Figure                                             |                  l5est0005 (`ab3fa87`) | This build (`6cb4735`) |
| -------------------------------------------------- | -------------------------------------: | ---------------------: |
| `a0` moving tracks; criterion 2 samples            |                              43; 3,656 |              43; 3,656 |
| `a0` criterion 2 r (Fisher; track bootstrap)       | 0.216 (0.184 to 0.246; 0.153 to 0.288) |              identical |
| `a0` criterion 3; criterion 4 ratios               |                   83.7 %; 0.972, 1.006 |              identical |
| `a0` one-step along z RMS, beyond; across RMS      |                    0.906, 3.6 %; 0.366 |              identical |
| `a0` criterion 5 cases; tracking baseline          |                     1,227; `79d9abff…` |              identical |
| `face_state` moving tracks; scored residuals       |                              28; 3,663 |              28; 3,663 |
| `face_state` solid bodies, near-edge fixes, lapses |                       6,586, 5,497, 79 |              identical |
| `face_state` criterion 2 samples; r                |                           2,918; 0.172 |           2,902; 0.213 |
| `face_state` track bootstrap                       |                         0.089 to 0.246 |         0.140 to 0.282 |
| `face_state` criterion 3                           |                                 57.1 % |                 53.6 % |
| `face_state` criterion 4 ratios, pass              |                      0.615, 0.851, yes |       0.729, 1.108, no |
| `face_state` one-step along z RMS, beyond; across  |                    0.655, 0.6 %; 0.234 |    0.639, 0.5 %; 0.231 |

A replay at the spec's base `ab19e51`, without this change, gives the new `face_state` figures
exactly, so the face observation changed on main after l5est0005 (#987) and before `ab19e51`. It
was not bisected. #989 changed the face-state option's span bounds, and l5id0019 bisected a loss
in a truck's solid-body length to it, so it is the first candidate. kirk0's criterion 4 pass on
the faces no longer holds at today's build.

The restated read on kirk0: `a0` along z RMS 0.906 (bootstrap 0.827 to 1.005), 3.6 % beyond, not
over-dispersed; `z_along²` partial r 0.225, 5th percentile 0.167.

## Population

| Arm          | Moving tracks | With scored residuals | Scored residuals | One-step along | Tracks behind them | Complete one-step samples |
| ------------ | ------------: | --------------------: | ---------------: | -------------: | -----------------: | ------------------------: |
| `a0`         |         4,459 |                 4,117 |          447,527 |        310,509 |              4,101 |                   264,799 |
| `face_state` |         3,558 |                 3,303 |          360,959 |        237,878 |              3,263 |                   197,657 |
| `vel-x2`     |         4,453 |                 4,113 |          450,460 |        315,672 |              4,092 |                   269,537 |
| `vel-x4`     |         4,426 |                 4,085 |          452,509 |        319,994 |              4,070 |                   273,567 |

**M0.** The spec counts "confirmed moving tracks with residuals" in the pooled arms. The four arms
replay the same vehicles, so their records are the same tracks counted up to four times (15,618
records), and the count read is `a0`'s distinct tracks: 4,117, 82 % of 5,000. No single arm
reaches 5,000. **Not met.** Every site ran; none was left unread.

## M1: is there over-dispersion to explain

One-step residuals of the moving tracks:

| Arm          | Axis   |       n | z RMS | Track bootstrap 95 % | Beyond 1.96 | Track bootstrap 95 % | z mean | Per-track median RMS | Tracks with RMS > 1 | Over-dispersed |
| ------------ | ------ | ------: | ----: | -------------------- | ----------: | -------------------- | -----: | -------------------: | ------------------: | -------------- |
| `a0`         | along  | 310,509 | 1.073 | 1.057 to 1.090       |       7.0 % | 6.7 % to 7.2 %       | +0.213 |                1.138 |                62 % | yes            |
| `a0`         | across | 310,509 | 0.392 | 0.383 to 0.400       |       0.4 % | 0.4 % to 0.4 %       | −0.015 |                0.319 |                 2 % | no             |
| `face_state` | along  | 237,878 | 0.743 | 0.729 to 0.756       |       2.2 % | 2.0 % to 2.3 %       | +0.204 |                0.730 |                24 % | no             |
| `face_state` | across | 237,824 | 0.277 | 0.267 to 0.286       |       0.1 % | 0.1 % to 0.1 %       | −0.014 |                0.170 |                 0 % | no             |
| `vel-x2`     | along  | 315,672 | 0.987 | 0.972 to 1.003       |       5.9 % | 5.7 % to 6.2 %       | +0.179 |                1.046 |                54 % | yes            |
| `vel-x2`     | across | 315,672 | 0.355 | 0.348 to 0.362       |       0.3 % | 0.3 % to 0.3 %       | −0.012 |                0.299 |                 2 % | no             |
| `vel-x4`     | along  | 319,994 | 0.910 | 0.896 to 0.926       |       5.1 % | 4.9 % to 5.3 %       | +0.147 |                0.964 |                47 % | yes            |
| `vel-x4`     | across | 319,994 | 0.325 | 0.319 to 0.332       |       0.3 % | 0.2 % to 0.3 %       | −0.009 |                0.281 |                 1 % | no             |

Over every scored residual, one-step or not, `a0` reads 1.063 and 6.9 % along, 0.398 across.

**Reading.** Under `a0` the pooled along z RMS is above 1 and more than 5 % lie beyond 1.96, so
M1's condition holds on the figures. With any one site left out the pool stays over-dispersed:
the lowest leave-one-out RMS is 1.037 and share 6.4 % (lombard-broderick out). The faces are not
over-dispersed on either axis. With M0 unmet this reading decides nothing.

## M2: is it acceleration

| Arm          | Residuals |       n | Tracks |     r | Fisher 95 %    | Bootstrap 5th percentile | Bootstrap 95 % |
| ------------ | --------- | ------: | -----: | ----: | -------------- | -----------------------: | -------------- |
| `a0`         | one-step  | 264,799 |  3,864 | 0.188 | 0.184 to 0.191 |                    0.177 | 0.175 to 0.199 |
| `a0`         | all       | 279,654 |  3,913 | 0.185 | 0.182 to 0.189 |                    0.175 | 0.173 to 0.197 |
| `face_state` | one-step  | 197,657 |  3,144 | 0.137 | 0.132 to 0.141 |                    0.118 | 0.113 to 0.160 |
| `face_state` | all       | 207,775 |  3,183 | 0.106 | 0.102 to 0.111 |                    0.089 | 0.086 to 0.143 |
| `vel-x2`     | one-step  | 269,537 |  3,867 | 0.139 | 0.136 to 0.143 |                    0.130 | 0.128 to 0.150 |
| `vel-x4`     | one-step  | 273,567 |  3,850 | 0.106 | 0.102 to 0.110 |                    0.096 | 0.095 to 0.117 |

**Reading.** The bootstrap 5th percentile is above 0.1 on the medoid (0.177) and on the one-step
faces (0.118), so M2's condition holds on both, not on the medoid only. The worst leave-one-out
is 0.170 on the medoid (1st-mission out) and 0.106 on the faces (marina-broderick out), both
still above 0.1. Read on every face residual rather than the one-step ones, the faces' 5th
percentile is 0.089, under the bar: the multi-step and re-referencing fixes dilute it.

**Post-hoc, by track age** (designed after the stratum table was seen; a 500-draw track
bootstrap in a scratch script, not the tool):

| Source       | Residuals                  |       n | Tracks |      r | Bootstrap 5th percentile |
| ------------ | -------------------------- | ------: | -----: | -----: | -----------------------: |
| `a0`         | 1 s or more into the track | 250,617 |  3,634 |  0.256 |                    0.245 |
| `a0`         | under 1 s                  |  14,182 |  3,316 | −0.023 |                   −0.055 |
| `face_state` | 1 s or more into the track | 196,622 |  3,132 |  0.144 |                    0.125 |
| `face_state` | under 1 s                  |   1,035 |    952 | −0.075 |                   −0.118 |

The correlation with acceleration lives in the mature residuals; the young ones, which carry the
over-dispersion, do not correlate with acceleration at all.

## M3: does a looser CV do it

M3's bars: along z RMS within 0.9 to 1.1; across z RMS no further from 1 than `a0`'s (0.392, a
distance of 0.608); the live filter's jerk share above 15 m/s³ no more than 1.5 times `a0`'s
(10.35 %, so 15.5 %).

| Arm      | Along z RMS | In 0.9 to 1.1 | Across z RMS | Distance from 1 | Not further than `a0` | Live jerk share | At most 15.5 % | Passes |
| -------- | ----------: | ------------- | -----------: | --------------: | --------------------- | --------------: | -------------- | ------ |
| `a0`     |       1.073 | no            |        0.392 |           0.608 | (reference)           |         10.35 % | (reference)    |        |
| `vel-x2` |       0.987 | yes           |        0.355 |           0.645 | no                    |         13.97 % | yes            | no     |
| `vel-x4` |       0.910 | yes           |        0.325 |           0.675 | no                    |         19.24 % | no             | no     |

Neither passes. The IMM clause, on the medoid's one-step residuals (criterion 4 below): along p99
ratio 0.977 against 0.8, across 0.933 against 1.05. **Criterion 4 is not met**, so no branch of M3
fires: no process-noise value is a candidate and no live CV/CA IMM is specified.

The across-track clause is the one both fail, and a larger velocity process noise widens `S` on
both axes, so with `a0` already under 1 across no value of this key can pass it. That was the
rule's intent: a looser CV that only trades one axis's consistency for the other's is not a fix.

## M4: the strata

One-step along z RMS (share beyond 1.96; residuals, tracks):

| Factor     | Bin             | `a0`                             | `face_state`                 | `vel-x2`                         | `vel-x4`                         |
| ---------- | --------------- | -------------------------------- | ---------------------------- | -------------------------------- | -------------------------------- |
| Speed, m/s | 0 to 5          | 0.78 (2.8 %; 129,694, 2,321)     | 0.56 (0.6 %; 97,801, 1,463)  | 0.69 (2.3 %; 133,160, 2,328)     | 0.62 (1.9 %; 134,963, 2,302)     |
|            | 5 to 10         | 1.16 (9.2 %; 128,499, 2,770)     | 0.79 (2.3 %; 92,488, 2,063)  | 1.07 (7.8 %; 129,589, 2,757)     | 0.98 (6.7 %; 131,241, 2,751)     |
|            | 10 to 15        | 1.43 (11.9 %; 51,735, 1,272)     | 0.95 (5.2 %; 43,263, 1,067)  | 1.35 (10.7 %; 52,314, 1,298)     | 1.27 (9.5 %; 52,979, 1,316)      |
|            | 15 and over     | 0.91 (3.4 %; 581, 27)            | 1.36 (13.4 %; 725, 49)       | 0.77 (2.6 %; 609, 27)            | 0.86 (3.5 %; 811, 33)            |
| Range, m   | 0 to 10         | 0.82 (2.5 %; 33,243, 1,405)      | 0.78 (2.1 %; 28,783, 1,275)  | 0.71 (1.8 %; 33,678, 1,411)      | 0.62 (1.5 %; 34,138, 1,408)      |
|            | 10 to 20        | 0.98 (5.3 %; 75,327, 2,526)      | 0.79 (2.6 %; 72,080, 2,353)  | 0.87 (4.2 %; 76,270, 2,551)      | 0.78 (3.5 %; 78,446, 2,548)      |
|            | 20 to 40        | 1.08 (7.5 %; 129,941, 3,459)     | 0.73 (2.1 %; 108,363, 3,010) | 1.00 (6.3 %; 133,167, 3,472)     | 0.92 (5.3 %; 134,179, 3,457)     |
|            | 40 and over     | 1.24 (9.8 %; 71,998, 2,703)      | 0.64 (1.5 %; 25,051, 1,826)  | 1.18 (9.0 %; 72,557, 2,716)      | 1.11 (8.1 %; 73,231, 2,722)      |
| Aspect     | 0 to 30° end-on | 1.13 (7.7 %; 166,438, 3,337)     | 0.69 (1.4 %; 107,430, 2,589) | 1.04 (6.7 %; 169,157, 3,322)     | 0.96 (5.7 %; 172,166, 3,326)     |
|            | 30 to 60°       | 1.05 (6.7 %; 79,514, 3,272)      | 0.80 (2.8 %; 70,641, 2,888)  | 0.95 (5.4 %; 80,824, 3,295)      | 0.88 (4.7 %; 81,418, 3,295)      |
|            | 60 to 90°       | 1.00 (6.0 %; 50,973, 2,940)      | 0.83 (3.3 %; 46,452, 2,726)  | 0.91 (5.0 %; 52,090, 2,969)      | 0.84 (4.2 %; 52,706, 2,960)      |
| Points     | 1 to 49         | 1.19 (9.5 %; 109,342, 3,959)     | 0.53 (1.2 %; 41,357, 2,849)  | 1.13 (8.6 %; 112,019, 3,958)     | 1.07 (7.7 %; 111,529, 3,964)     |
|            | 50 to 199       | 1.11 (7.8 %; 90,264, 3,407)      | 0.75 (2.5 %; 83,965, 3,075)  | 1.02 (6.5 %; 91,558, 3,415)      | 0.94 (5.5 %; 94,166, 3,398)      |
|            | 200 to 499      | 0.97 (4.9 %; 54,752, 2,504)      | 0.82 (2.7 %; 55,714, 2,452)  | 0.86 (3.9 %; 55,440, 2,516)      | 0.76 (3.1 %; 56,723, 2,526)      |
|            | 500 and over    | 0.84 (2.7 %; 56,151, 1,858)      | 0.78 (1.9 %; 56,842, 1,833)  | 0.71 (1.8 %; 56,655, 1,863)      | 0.62 (1.4 %; 57,576, 1,860)      |
| Track age  | under 1 s       | **2.52** (47.6 %; 23,494, 3,786) | 1.29 (12.9 %; 8,819, 2,070)  | **2.45** (44.9 %; 23,496, 3,786) | **2.34** (41.3 %; 23,639, 3,781) |
|            | 1 s and over    | 0.85 (3.7 %; 287,015, 3,970)     | 0.71 (1.8 %; 229,059, 3,248) | 0.75 (2.8 %; 292,176, 3,973)     | 0.68 (2.2 %; 296,355, 3,964)     |

Residuals under 1 s into their track carry 51.6 % of `a0`'s one-step along values beyond 1.96
(22.0 % for the faces, 56.3 % and 59.7 % under `vel-x2` and `vel-x4`). 3,786 of `a0`'s 4,101
tracks with one-step along residuals have at least one under 1 s.

**Reading.** M4's trigger is a stratum over 1.5 while the pool is under 1. The pool is 1.073, so
the trigger is not met as written. The table is reported as M4 requires, and one stratum stands
apart: the first second of a track, at 2.52 under `a0` and 2.34 even at four times the process
noise. No other stratum reaches 1.5 (the nearest are 10 to 15 m/s at 1.43 and beyond 40 m at
1.24). Off the first second, `a0` reads 0.85 with 3.7 % beyond: under-dispersed. The table does
not cross age with the other factors, so how much of the speed and range gradient is the first
second is not read here.

## Jerk and turning

Punzo et al. 2011's checks on each estimator's output:

| Arm          | Estimator   | Tracks | Jerk samples | Share above 15 m/s³ | Sign changes per s |
| ------------ | ----------- | -----: | -----------: | ------------------: | -----------------: |
| `a0`         | live filter |  4,026 |      283,243 |             10.35 % |               4.73 |
| `a0`         | offline CV  |  4,076 |      288,391 |             10.13 % |               4.83 |
| `a0`         | CA, coast   |  4,079 |      288,097 |             20.32 % |               4.51 |
| `a0`         | IMM         |  4,077 |      285,772 |             18.48 % |               4.54 |
| `face_state` | solid body  |  3,212 |      227,788 |              5.38 % |               4.30 |
| `face_state` | offline CV  |  3,216 |      224,467 |              7.62 % |               3.74 |
| `face_state` | CA, coast   |  3,224 |      230,736 |             11.74 % |               3.65 |
| `face_state` | IMM         |  3,219 |      222,932 |              9.94 % |               3.63 |
| `vel-x2`     | live filter |  4,032 |      287,975 |             13.97 % |               4.83 |
| `vel-x4`     | live filter |  4,021 |      292,587 |             19.24 % |               4.94 |

The offline models under `vel-x2` and `vel-x4` read the same as under `a0` to within 0.1 point,
as they should: they run at their own defaults on nearly the same observations.

Turning: of `a0`'s 225,190 track-frames with a yaw rate, 8.6 % turn faster than 0.1 rad/s and
4.4 % faster than 0.2 rad/s (faces: 7.2 % and 3.5 % of 173,820). The process-noise arms read the
same.

## Criteria 3 to 5, as l5est0005 reported them

| Arm          | Criterion 3, one-step  | n along | CV p99 along, m | CA p99 | IMM p99 | CA along ratio | IMM along ratio | CA across ratio | IMM across ratio |
| ------------ | ---------------------- | ------: | --------------: | -----: | ------: | -------------: | --------------: | --------------: | ---------------: |
| `a0`         | 3,401 of 4,101, 82.9 % | 305,690 |           0.998 |  1.035 |   0.975 |          1.037 |           0.977 |           1.000 |            0.933 |
| `face_state` | 2,092 of 3,263, 64.1 % | 223,245 |           3.601 |  3.043 |   3.179 |          0.845 |           0.883 |           0.781 |            0.791 |
| `vel-x2`     | 3,305 of 4,092, 80.8 % | 310,378 |           1.005 |  1.036 |   0.974 |          1.030 |           0.969 |           0.995 |            0.927 |
| `vel-x4`     | 3,171 of 4,070, 77.9 % | 314,385 |           1.002 |  1.034 |   0.973 |          1.032 |           0.970 |           0.993 |            0.931 |

Criterion 4's bar is an along ratio of at most 0.8 with an across ratio of at most 1.05: no model
meets it on any arm. The faces' CV p99 along is 3.6 m, 3.6 times the medoid's, on the corpus as
on kirk0.

Criterion 5, median and p95 reacquisition error in metres:

`a0`, 114,186 cases:

| Horizon | Split        |      n | CV            | CA, coast      | IMM           | CA, no coast   |
| ------: | ------------ | -----: | ------------- | -------------- | ------------- | -------------- |
|   0.5 s | all          | 40,851 | 0.300 / 1.576 | 0.269 / 1.841  | 0.251 / 1.607 | 0.277 / 1.969  |
|   0.5 s | decelerating | 10,023 | 0.556 / 1.730 | 0.489 / 2.372  | 0.460 / 2.047 | 0.507 / 2.570  |
|   1.0 s | all          | 38,947 | 0.528 / 2.763 | 0.536 / 3.309  | 0.460 / 2.720 | 0.577 / 3.826  |
|   1.0 s | decelerating |  9,358 | 1.010 / 3.010 | 0.958 / 4.459  | 0.871 / 3.546 | 1.023 / 5.221  |
|   2.5 s | all          | 34,388 | 1.368 / 7.992 | 1.628 / 9.559  | 1.274 / 7.538 | 2.067 / 13.464 |
|   2.5 s | decelerating |  8,030 | 2.923 / 8.994 | 3.045 / 12.893 | 2.664 / 9.406 | 3.787 / 18.729 |

`face_state`, 76,769 cases:

| Horizon | Split        |      n | CV             | CA, coast      | IMM            | CA, no coast   |
| ------: | ------------ | -----: | -------------- | -------------- | -------------- | -------------- |
|   0.5 s | all          | 27,797 | 0.524 / 3.395  | 0.410 / 2.651  | 0.418 / 2.715  | 0.419 / 2.707  |
|   0.5 s | decelerating |  6,998 | 0.886 / 3.719  | 0.719 / 3.136  | 0.710 / 3.167  | 0.728 / 3.240  |
|   1.0 s | all          | 26,346 | 0.840 / 5.133  | 0.756 / 4.224  | 0.696 / 4.182  | 0.797 / 4.468  |
|   1.0 s | decelerating |  6,565 | 1.484 / 5.503  | 1.271 / 4.842  | 1.213 / 4.738  | 1.338 / 5.237  |
|   2.5 s | all          | 22,626 | 1.927 / 11.666 | 2.065 / 10.171 | 1.721 / 9.948  | 2.480 / 11.895 |
|   2.5 s | decelerating |  5,663 | 3.825 / 12.546 | 3.602 / 11.699 | 3.262 / 10.729 | 4.241 / 14.080 |

On the medoid, CA with the coast rule has a worse p95 than CV in every row, and the IMM a worse
p95 in four of six: neither holds criterion 5. On the faces the IMM is no worse than CV in any
row; CA with the coast rule is worse at the 2.5 s median on all gaps (2.065 against 1.927 m).

## Per site

`a0`'s and `face_state`'s one-step figures per site; `p05` is the `z_along²` correlation's
bootstrap 5th percentile. The last four columns are the 23-site pool with that site left out.

| Site                  | `a0` tracks | Along RMS | Beyond | Across RMS |     r |    p05 | Face tracks | Face along RMS | Face r | Face p05 | `vel-x2` along | `vel-x4` along | Left out: along RMS | Left out: beyond | Left out: `a0` p05 | Left out: face p05 |
| --------------------- | ----------: | --------: | -----: | ---------: | ----: | -----: | ----------: | -------------: | -----: | -------: | -------------: | -------------: | ------------------: | ---------------: | -----------------: | -----------------: |
| 1st-mission           |         318 |     0.739 |  3.0 % |      0.355 | 0.241 |  0.205 |         259 |          0.534 |  0.146 |    0.111 |          0.661 |          0.623 |               1.103 |            7.4 % |              0.170 |              0.113 |
| 3rd-folsom            |         275 |     1.088 |  7.1 % |      0.341 | 0.082 |  0.046 |         192 |          0.647 |  0.222 |    0.158 |          0.996 |          0.929 |               1.073 |            7.0 % |              0.182 |              0.116 |
| ashbury-downey        |          38 |     1.092 |  7.5 % |      0.412 | 0.043 | −0.003 |          35 |          0.723 |  0.105 |    0.005 |          1.010 |          0.918 |               1.073 |            7.0 % |              0.179 |              0.118 |
| broadway-franklin     |         180 |     1.213 | 10.8 % |      0.393 | 0.213 |  0.139 |         166 |          0.962 |  0.020 |   −0.044 |          1.117 |          1.029 |               1.069 |            6.9 % |              0.177 |              0.122 |
| bush-powell           |         164 |     0.950 |  5.1 % |      0.341 | 0.134 |  0.096 |         140 |          0.698 |  0.160 |    0.113 |          0.856 |          0.774 |               1.078 |            7.1 % |              0.178 |              0.116 |
| california-leon-baker |          58 |     1.339 | 10.7 % |      0.227 | 0.010 | −0.074 |          57 |          0.995 |  0.040 |   −0.071 |          1.273 |          1.213 |               1.071 |            6.9 % |              0.179 |              0.122 |
| columbus-broadway     |         324 |     1.032 |  6.7 % |      0.384 | 0.206 |  0.167 |         256 |          0.663 |  0.226 |    0.150 |          0.946 |          0.861 |               1.077 |            7.0 % |              0.175 |              0.113 |
| columbus-north-point  |         131 |     0.983 |  5.9 % |      0.380 | 0.218 |  0.168 |          71 |          0.658 |  0.185 |    0.069 |          0.895 |          0.776 |               1.076 |            7.0 % |              0.177 |              0.119 |
| embarcadero-bay       |         159 |     0.924 |  4.9 % |      0.399 | 0.201 |  0.156 |         123 |          0.668 |  0.361 |    0.274 |          0.828 |          0.760 |               1.082 |            7.1 % |              0.174 |              0.112 |
| embarcadero-broadway  |         262 |     0.975 |  5.7 % |      0.400 | 0.265 |  0.225 |         218 |          0.675 |  0.200 |    0.139 |          0.890 |          0.817 |               1.080 |            7.1 % |              0.173 |              0.116 |
| embarcadero-bryant    |         407 |     1.253 | 10.0 % |      0.479 | 0.220 |  0.175 |         250 |          0.860 |  0.129 |    0.025 |          1.187 |          1.118 |               1.061 |            6.8 % |              0.172 |              0.118 |
| franklin-mcallister   |         276 |     1.100 |  7.3 % |      0.388 | 0.150 |  0.115 |         233 |          0.772 |  0.121 |    0.068 |          1.013 |          0.925 |               1.072 |            7.0 % |              0.179 |              0.119 |
| fulton-divisadero     |         175 |     1.017 |  6.4 % |      0.407 | 0.137 |  0.104 |         158 |          0.724 |  0.083 |    0.027 |          0.936 |          0.854 |               1.076 |            7.0 % |              0.178 |              0.120 |
| haight-divisadero     |          56 |     1.023 |  6.1 % |      0.331 | 0.143 |  0.073 |          51 |          0.799 |  0.145 |   −0.027 |          0.932 |          0.849 |               1.074 |            7.0 % |              0.177 |              0.118 |
| howard-6th            |         195 |     1.131 |  7.7 % |      0.390 | 0.174 |  0.133 |         164 |          0.815 |  0.119 |    0.085 |          1.028 |          0.957 |               1.071 |            7.0 % |              0.177 |              0.121 |
| hyde-ofarrell         |         136 |     0.883 |  4.4 % |      0.409 | 0.199 |  0.143 |         122 |          0.681 |  0.159 |    0.118 |          0.810 |          0.732 |               1.080 |            7.1 % |              0.177 |              0.116 |
| laguna-eddy           |          87 |     1.121 |  7.4 % |      0.352 | 0.192 |  0.106 |          84 |          0.783 |  0.117 |    0.013 |          1.014 |          0.931 |               1.073 |            7.0 % |              0.177 |              0.117 |
| lombard-broderick     |         458 |     1.420 | 13.4 % |      0.495 | 0.124 |  0.098 |         340 |          0.962 |  0.061 |    0.024 |          1.330 |          1.229 |               1.037 |            6.4 % |              0.187 |              0.127 |
| lombard-laguna        |         251 |     1.330 | 10.6 % |      0.432 | 0.164 |  0.113 |         212 |          0.837 |  0.057 |    0.007 |          1.247 |          1.159 |               1.058 |            6.8 % |              0.180 |              0.122 |
| marina-broderick      |         116 |     0.954 |  4.1 % |      0.288 | 0.311 |  0.272 |         102 |          0.752 |  0.328 |    0.247 |          0.811 |          0.711 |               1.078 |            7.1 % |              0.173 |              0.106 |
| marina-webster-beach  |          72 |     0.832 |  3.0 % |      0.286 | 0.232 |  0.191 |          62 |          0.619 |  0.483 |    0.365 |          0.734 |          0.669 |               1.081 |            7.1 % |              0.175 |              0.114 |
| pierce-haight         |          59 |     1.132 |  8.6 % |      0.458 | 0.159 |  0.086 |          50 |          0.785 |  0.130 |   −0.028 |          1.068 |          0.984 |               1.073 |            7.0 % |              0.177 |              0.118 |
| van-ness-sacramento   |         262 |     1.044 |  6.2 % |      0.347 | 0.134 |  0.095 |         213 |          0.679 |  0.006 |   −0.028 |          0.988 |          0.904 |               1.076 |            7.0 % |              0.179 |              0.135 |

Worst leave-one-out: for M1, lombard-broderick out gives the lowest along RMS (1.037) and share
(6.4 %), still over-dispersed; for M2, 1st-mission out gives the medoid's lowest 5th percentile
(0.170) and marina-broderick out the faces' (0.106). No single site decides M1 or M2. Read site by
site, 18 of 23 are over-dispersed along under `a0` and 2 under the faces; the medoid's 5th
percentile is above 0.1 at 15 sites alone, the faces' at 9.

## Interpretation

**What was measured.** On the first five minutes of 23 sites, with the shipped tuning, the CV
filter's one-step along-track residuals on the cluster medoid are slightly over-dispersed in the
pool (RMS 1.07, 7.0 % beyond 1.96), and its across-track residuals are strongly under-dispersed
(RMS 0.39). The face-referenced residuals of the shadow solid body are under-dispersed on both
axes (0.74 and 0.28). On both measurements `z_along²` rises with the smoothed acceleration after
range, aspect and point count are controlled for (partial r 0.19 and 0.14), and the bound
survives resampling whole tracks and leaving out any site.

**Where the excess is.** Half of the along-track tail is in the first second of each track,
which is 7.6 % of the residuals. There the medoid's RMS is 2.5 and nearly half the values lie
beyond 1.96. After the first second the medoid reads 0.85, under-dispersed as on kirk0 (0.91).
So the pooled over-dispersion that M1 reads is, in the main, a track-start effect: the
filter's prior right after birth, from an initial velocity and covariance that do not yet
describe the vehicle. The post-hoc age split points the same way: in the first second `z_along²`
does not correlate with acceleration (−0.02), and after it the correlation is stronger (0.26).
The acceleration signal G-EST-1 looks for is there, but in residuals the filter already expects
to be large. That is lag the covariance contains, not a model failure. This is a post-hoc reading
of a stratum the rule declared, and it is diagnostic.

**The process noise.** A larger velocity process noise lowers along-track RMS (1.07, 0.99, 0.91
at 0.2, 0.4 and 0.8) and the share of the tail outside the first second, but it barely moves
the first second (2.52, 2.45, 2.34), drives across-track RMS further below 1 (0.39, 0.36, 0.33)
and makes the live output rougher (jerk share 10.4 %, 14.0 %, 19.2 %). One isotropic velocity
noise cannot set both axes. The across-track slack, present at every site and every stratum, is
S too wide where acceleration plays no part: that is the measurement noise, G-UNC-1's question.

**The models.** Offline CA does not lower the medoid's one-step p99 at all (ratio 1.04), and the
IMM lowers it by 2 %. On the faces CA lowers it by 16 % against the 20 % bar, on a face tail 3.6
times the medoid's in metres. CA's and the IMM's outputs are roughly twice as jerky as CV's by
Punzo's check (20 % and 18 % of samples above 15 m/s³ on the medoid). The live medoid filter is
itself at 10 %, with about 4.7 jerk sign changes a second, so the 10 Hz output is not a
plausible passenger-car trajectory by that check either; a smoother, not a model, is what reads
better on it.

**kirk0.** The tracked filter's figures reproduce exactly; the faces' do not, because main
changed the face-state solid body after l5est0005. kirk0's face criterion 4 pass (along ratio
0.62) reads 0.73 at this build, and the corpus's one-step ratio is 0.85.

## Limitations

- **The count.** 4,117 tracks with residuals against criterion 1's 5,000. By the spec's M0 the
  readings decide nothing, whatever their intervals say.
- **First segments.** Each segment starts at its capture's first packet, so it includes the
  site's background settling; the 40 s warm-up and `-require-settled=false` let unsettled
  background into the scoring window at some sites. These segments were read before for identity
  and noise, so this is a measurement, not a held-out score.
- **No reference.** No label was read and no speed reference exists off kirk0. The acceleration is
  the pipeline's own smoothed estimate from the same observations, so M2 correlates a residual
  with a function of the same residuals; the partial correlation and the track bootstrap bound
  that, they do not remove it. NIS and z read the filter's consistency with itself, not its
  distance from a vehicle. Nothing here says a speed is right.
- **Punzo's bound** is for passenger-car trajectories at 10 Hz and is a plausibility check on the
  estimators' outputs, not truth, and the corpus holds buses and trucks.
- **Track age** is the time since the track's first tracked-filter observation; a track that
  began before the scoring window is old in its first scored frame. The age split of M2 is post
  hoc.
- **The face observation** changed on main between l5est0005 and this build, unbisected. The
  faces' metre-scale tail by face kind and aspect, which the spec lists as reported, was not read:
  the track records carry no face kind.
- **The offline models** run at their prototype defaults; their criteria 4 and 5 on the faces
  approximate the solid body rather than run it, as in l5est0005.
- **Scratch eligibility.** The segments are not in the committed cohort manifest. The replays ran
  under a scratch manifest that assigns each segment, by hash, to its site as development data,
  and a scratch copy of the run's use ledger; embarcadero-folsom was not read.

## Recommendations

1. **Ship nothing.** No default changes, no CA, no live IMM, and no process-noise value goes to
   l5est0004 from this read: neither `vel-x2` nor `vel-x4` met M3's bars, and the across-track
   clause cannot be met by this key while across-track RMS is under 1.
2. **Read the same report on the first and second segments pooled**, M0's named route
   (`d2-second-segment`, 22 sites, held out from every first-segment screen). The first segments
   give about 180 tracks a site, so the pair should clear 5,000. Declare track age as a stratum
   and the over-dispersion read both with and without the first second, before any replay.
   - Falsified if: the second segments put the 1 s-and-over medoid along RMS above 1.
3. **Look at the first second of a track.** The along-track excess is at track start, under every
   process noise. The next question is the state at birth: the shipped tuning's initial velocity
   and its covariance against the residuals they produce (two-point initialisation is off by
   default, and [l5est0006](two-point-init-guards-2026-11.md) did not show its guarded form to
   help). Its population is the first second of each moving track.
4. **G-UNC-1 before G-EST-1.** The across-track RMS is 0.39 on the medoid and 0.28 on the faces
   at every site and stratum. Recalibrate S across the track first; a filter whose S is honest may
   then read over-dispersed along the track where acceleration is, which is the case G-EST-1 was
   written for.
5. **Bisect the face observation's change** between #987 and `ab19e51` on kirk0 before any
   face-referenced criterion is ruled on again.

## Provenance

| Item           | Value                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base           | `ab19e51` (spec approved, rule committed before any replay)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Commits        | code `6cb4735` (restated criterion 2, track records, `lidar-gest1-pool`), before the replays; report and spec in the commit after                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Builds         | built at `6cb4735` with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA` `6cb4735b…`) and the `pcap` tag; every replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `6cb4735b…`. `lidar-state-estimation-baseline` `sha256:713728b9…`, `lidar-gest1-pool` `sha256:9249d55f…`. The diagnostic kirk0 replay at `ab19e51` was a scratch build of that tree                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Parameter sets | `config/tuning.defaults.json` `ab0c4035780d` (`a0`; `face_state` replay hash `80e04c6e…` with its experiments folded in); `config/experiments/process-noise-vel-x2.json` `deb48e282fa2`; `config/experiments/process-noise-vel-x4.json` `e026f818d678`; each equals the replay's `params_hash`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Replay         | one case per run, `-warmup 40 -require-settled=false -sample-points 1024 -gest1-report`, repeat-verified; `face_state` with `-experiment solid_body,solid_body_face_state`, evidence per case, discarded after the summary; kirk0 whole at `-warmup 6`; two lanes, `GOMAXPROCS=2`, `nice -n 10`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Capture        | kirk0 `sha256:2864ebde…` (Git LFS)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Segments       | the first 300 s of each placement from `banshee-data/sf-street-speeds`, cut by the run's scratch pcapng window filter (l5id0019's method; its kept count includes non-2369 packets), each placement deleted once cut and each segment once its arms were written. SHA-256 prefixes: 1st-mission `83160e99`, 3rd-folsom `15b20307`, ashbury-downey `971e093e`, broadway-franklin (published as broadway-gough) `5623f2e2`, bush-powell `d52dbe06`, california-leon-baker `8c8d9048`, columbus-broadway `774c5434`, columbus-north-point `ceeb92ab`, embarcadero-bay `5a533b67`, embarcadero-broadway `4ae5de3a`, embarcadero-bryant `1fe9d972`, franklin-mcallister `ab9722de`, fulton-divisadero `eda80c97`, haight-divisadero `2afd5be4`, howard-6th `45542a7d`, hyde-ofarrell `ed2c936d`, laguna-eddy `3fad83b7`, lombard-broderick `a38d974e`, lombard-laguna `732d5572`, marina-broderick `c8141c2f`, marina-webster-beach `d4c46236`, pierce-haight `525df988`, van-ness-sacramento `8cadf5ab` |
| Sites          | all 23 of `d2-first-segment` ran every arm; embarcadero-folsom (held out) was not downloaded or read                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Host           | Claude Code cloud container, 4 cores shared with two other experiment agents, Go 1.27.1 linux/amd64; replays 05:45 to 10:12 UTC, median 508 s (`face_state`), 370 s and 356 s per segment; raw output in the session's scratch, not kept                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
