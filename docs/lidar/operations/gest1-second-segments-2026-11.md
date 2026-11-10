# G-EST-1 on the second segments: the first second of a track, held out

<!-- ignore-style-length -->

l5est0007 read G-EST-1's restated criteria on the corpus's 23 first segments, fell short of
criterion 1's count, and found after reading that the medoid's along-track over-dispersion sits in
each track's first second. This run states that split as a rule and reads it on the second
five-minute segment of the 22 sites of `d2-second-segment`, which no filter-consistency measure had
read, then pools both segment sets for the count. It answers
[l5est0008](../../../data/experiments/l5est/l5est0008.md).

- **Status:** Measurement complete, 2026-11-10. No default changed; no motion model ships
- **Layers:** L5 (the tracked filter, the shadow solid body), read by L8 analytics
- **Related:** [first-segment read (l5est0007)](gest1-corpus-read-2026-11.md),
  [state plan §7.3](../../plans/lidar-state-estimation-plan.md#73-recommendation-and-gate),
  [motion models maths](../../../data/maths/motion-models-maths.md#10-g-est-1-analyses)
- **Parameter set:** the [defaults](../../../config/tuning.defaults.json) (`ab0c4035780d`), read
  through one `face_state` replay per site (`solid_body`, `solid_body_face_state`)

## Answer

**N0 is met and N1 holds on the held-out segments: the along-track excess is a track-start
effect.** The pooled 45 segments hold 8,426 confirmed moving tracks with residuals (4,117 first,
4,309 second) against 5,000, so the readings decide. On the second segments alone the medoid's
one-step along z RMS is 2.42 at track age 1 s or less and 0.85 after it, each whole-track
bootstrap 5th to 95th interval clear of its bound. By the spec, G-EST-1's premise is not met after
the first second: CA and the IMM stay offline, and initialisation (the class-range velocity
prior) is the next increment, ahead of any motion model.

| Reading                    | Figure                                                                                                | Against the bar                      |
| -------------------------- | ----------------------------------------------------------------------------------------------------- | ------------------------------------ |
| N0, enough tracks          | 8,426 `a0` tracks with residuals over 45 segments                                                     | 5,000 or more: **met**               |
| N1, first second, held out | at most 1 s: along z RMS 2.419 (2.392 to 2.446); after 1 s: 0.846 (0.835 to 0.858)                    | above 1.5 and at most 1.0: **holds** |
| N2, acceleration after 1 s | medoid partial r 0.255, 5th percentile 0.244; one-step faces 0.089, 5th percentile 0.068              | faces under 0.1: does not hold       |
| N3, the pool (reported)    | along 1.064, 6.8 % beyond; `z_along²` 5th percentile 0.182 (medoid), 0.081 (faces); first second 2.47 | decides only through N0              |

N2 decides nothing here: its branch needs N1's after-1 s RMS above 1.0, and it is 0.85. Read
anyway, it fails on the faces, so neither route of the spec leads to a motion model.

Two findings sit beside the readings:

1. **The young excess is a signed lag.** At track age 1 s or less the medoid's along z has mean
   +1.89 against an RMS of 2.42: about 61 % of the mean square is bias, with the observation ahead
   of the prediction. That is the filter starting slower than the vehicle, which an initial
   velocity prior addresses and an acceleration model does not.
2. **Off the first second the medoid is under-dispersed on both axes** (along 0.85, across 0.39),
   as on the first segments and on kirk0. The across-track slack remains G-UNC-1's question.

**Recommendation:** ship nothing; specify the initialisation increment on the first second of
each moving track, with the young along z mean and RMS as its readings.

## Question and arms

Does the medoid's along-track over-dispersion live in a track's first second, read on segments no
filter-consistency measure has seen, and after it is there an excess for a motion model to
explain? One replay per site:

| Arm          | Parameter set            | Replay experiments                    | Residual source read | What it measures                                |
| ------------ | ------------------------ | ------------------------------------- | -------------------- | ----------------------------------------------- |
| `a0`         | defaults, `ab0c4035780d` | none                                  | `tracked_filter`     | The shipped CV filter on the cluster medoid     |
| `face_state` | defaults, `ab0c4035780d` | `solid_body`, `solid_body_face_state` | `solid_body_faces`   | The shadow solid body's one-step face residuals |

`a0`'s residuals are the `tracked_filter` section of the `face_state` replay, as in l5est0007.
That was checked on the first site, haight-divisadero's second segment: a separate `a0` replay
gave the byte-equal tracking baseline (`sha256:ae7c4935…`), the same `tracked_filter` report
section and the same 56 track records once each run's random track IDs are set aside. The
velocity arms were not rerun; l5est0007's M3 read them.

## Method

The rule (N0 to N3) was committed in the spec before any replay (`2c870f4`). The split was added
to the reading code and committed before the replays (`1851d50`); a coverage flag for the pooling
tool followed (`3df1dad`) while the first replays ran, before any second-segment figure was read.

- **The split.** `ReadGEST1Corpus` reads the one-step along and across dispersion, the
  `z_along²` partial correlation and criterion 4's offline tails separately over residuals at a
  track age of at most 1 s and after it, each with a whole-track bootstrap (2,000 seeded draws)
  over the tracks with a value on that side, on its own generator so every other figure is
  unchanged. Track records now carry each offline one-step value's age, so criterion 4 splits on
  the second segments; the first segments' records carry none.
- **N1's interval** is the bootstrap's 5th to 95th (`lidar-gest1-pool -level 0.9`); N3's
  intervals keep l5est0007's 95 %. N2's 5th percentile is the same at either level.
- **Replays.** `lidar-state-estimation-baseline`, one case per run, `-warmup 40
-require-settled=false -sample-points 1024 -gest1-report`, every run repeat-verified;
  `-experiment solid_body,solid_body_face_state` with `-evidence-dir`, `-evidence-per-case`,
  `-discard-evidence` and a source manifest. Population, one-step definition, the partial
  correlation's covariates (range, aspect, point count) and the jerk and turning checks are
  l5est0007's.
- **Segments.** 300 to 600 s from each placement's first packet, cut by the run's scratch pcapng
  window filter (l5id0019's method), one placement at a time, each deleted once cut and each
  segment once its summaries were written. haight-divisadero's was the run's shared copy;
  broadway-franklin is published as broadway-gough; ashbury-downey has one capture and is not in
  the corpus.
- **The pool.** The 23 first-segment track records are l5est0007's per-case outputs, reused
  rather than replayed, and pooled with the 22 new ones as 45 cases (each named by site and
  segment), with each segment left out.
- **Build check.** The first-segment records came from l5est0007's build `6cb4735`, the second
  from `1851d50`, whose changes on main since are default-off shadows. A `face_state` replay of
  haight-divisadero's first segment at `1851d50` gave l5est0007's tracking baseline
  (`sha256:c24f7d2a…`), identical report sections on both sources and identical track records
  apart from the new age fields. Re-pooled with the new tool, the 23 first segments give
  l5est0007's every figure exactly, and the split reproduces its post-hoc table (at most 1 s:
  2.518, r −0.023; after: 0.853, r 0.256): no residual falls at exactly 1 s.

## N0: enough tracks

| Segments | Cases | `a0` moving tracks | With scored residuals | Face moving tracks | With residuals |
| -------- | ----: | -----------------: | --------------------: | -----------------: | -------------: |
| First    |    23 |              4,459 |                 4,117 |              3,558 |          3,303 |
| Second   |    22 |              4,765 |                 4,309 |              3,638 |          3,300 |
| Pooled   |    45 |              9,224 |                 8,426 |              7,196 |          6,603 |

The count read is l5est0007's: `a0`'s confirmed moving tracks with at least one scored residual.
A first and a second segment of one site are different five-minute windows, so a vehicle can
appear in both only if it stays in view across the cut. **Met** (8,426 against 5,000); every
site ran.

## N1: the first second, held out

One-step along-track z on the 22 second segments, whole-track bootstrap 5th to 95th:

| Source       | Residuals    |       n | Tracks | z RMS | 5th to 95th    | z mean | Beyond 1.96 | Across RMS |
| ------------ | ------------ | ------: | -----: | ----: | -------------- | -----: | ----------: | ---------: |
| `a0`         | at most 1 s  |  23,871 |  3,942 | 2.419 | 2.392 to 2.446 | +1.888 |      44.8 % |      0.441 |
| `a0`         | after 1 s    | 284,474 |  4,148 | 0.846 | 0.835 to 0.858 | +0.085 |       3.5 % |      0.393 |
| `a0`         | all one-step | 308,345 |  4,285 | 1.055 | 1.042 to 1.068 | +0.225 |       6.7 % |      0.397 |
| `face_state` | at most 1 s  |   8,710 |  2,032 | 1.334 | 1.292 to 1.378 | +0.936 |      13.7 % |      0.248 |
| `face_state` | after 1 s    | 224,799 |  3,237 | 0.700 | 0.690 to 0.711 | +0.180 |       1.7 % |      0.285 |

**Reading.** At most 1 s the medoid's RMS is 2.42 with its 5th percentile at 2.39, above 1.5;
after 1 s it is 0.85 with its 95th percentile at 0.86, at most 1.0. **Both hold.** Residuals at
most 1 s into their track are 7.7 % of the one-step values and carry 51.9 % of those beyond 1.96.
With any one site left out the young RMS is at least 2.337 (lombard-laguna out) and the after-1 s
RMS at most 0.857 (bush-powell out). The first segments read 2.52 and 0.85 on the same split, so
the held-out read repeats the post-hoc one to within 0.1 young and 0.01 after.

The faces follow the same shape at a smaller scale (1.33 and 0.70); their young excess is under
1.5 and is not what the rule reads.

## N2: acceleration after the first second

Partial correlation of `z_along²` with the smoothed |longitudinal acceleration| (range, aspect and
point count controlled), second segments:

| Source       | Residuals   |       n | Tracks |      r | Bootstrap 5th percentile | Worst leave-one-out 5th percentile |
| ------------ | ----------- | ------: | -----: | -----: | -----------------------: | ---------------------------------: |
| `a0`         | after 1 s   | 247,656 |  3,742 |  0.255 |                    0.244 |           0.236 (marina-broderick) |
| `a0`         | at most 1 s |  14,360 |  3,414 | −0.020 |                   −0.046 |                                    |
| `face_state` | after 1 s   | 192,922 |  3,106 |  0.089 |                    0.068 |           0.060 (marina-broderick) |
| `face_state` | at most 1 s |   1,058 |    978 | −0.032 |                   −0.082 |                                    |

**Reading.** The medoid's 5th percentile is above 0.1; the faces' is not, so N2's condition, on
the medoid and on the one-step faces, does not hold. With N1's after-1 s RMS at most 1.0 the N2
branch is not reached in any case. On the medoid the correlation with acceleration is real and
lives after the first second, in residuals the filter already expects (RMS 0.85): lag the
covariance contains. In the first second the correlation is nil on both sources, as l5est0007's
post-hoc split found.

## N3: the pool of 45 segments (reported)

l5est0007's M1, M2 and M4 on the 23 first and 22 second segments pooled, 95 % whole-track
intervals:

| Reading            | `a0`                                                   | `face_state`                                           |
| ------------------ | ------------------------------------------------------ | ------------------------------------------------------ |
| M1, along one-step | RMS 1.064 (1.053 to 1.076), 6.8 % beyond, n 618,854    | RMS 0.738 (0.729 to 0.748), 2.1 % beyond, n 471,387    |
| M1, across         | RMS 0.394 (0.388 to 0.400)                             | RMS 0.280 (0.274 to 0.287)                             |
| M2, one-step       | r 0.189, 5th percentile 0.182, n 526,815, 7,888 tracks | r 0.102, 5th percentile 0.081, n 391,637, 6,266 tracks |
| M4, at most 1 s    | 2.47 (46.2 % beyond; 47,365, 7,728 tracks)             | 1.31 (13.3 %; 17,529, 4,102)                           |
| M4, after 1 s      | 0.85 (3.6 %; 571,489, 8,118)                           | 0.71 (1.7 %; 453,858, 6,485)                           |
| Young tail share   | 51.8 %                                                 | 23.1 %                                                 |

As l5est0007 wrote them: M1's condition holds on the medoid (RMS above 1, more than 5 % beyond);
M2's needs both 5th percentiles above 0.1 and the faces' is 0.081 in the pool (0.118 on the first
segments alone, 0.064 on the second); M4's trigger, a stratum over 1.5 while the pool is under 1,
is not met because the pool is 1.064. They decide only through N0, and N0 makes N1 the deciding
reading. Worst leave-one-out in the pool: along RMS 1.047 (lombard-broderick's first segment
out), medoid 5th percentile 0.178 (1st-mission's second out), faces 0.076 (marina-broderick's
first out).

The other strata repeat l5est0007's on the pool: along RMS rises with speed (0.78, 1.16 and 1.42
below 15 m/s) and range (0.81 under 10 m to 1.26 beyond 40 m) and falls with point count (1.20
under 50 points to 0.83 over 500). The table does not cross age with them.

## Reported, not decided

**Criterion 4 after the first second** (second segments; records carry the offline ages), p99 of
the one-step |innovation| and each model's ratio to CV's:

| Source       | Residuals   | n along | CV p99 along, m | CA ratio along | IMM ratio along | CA ratio across | IMM ratio across |
| ------------ | ----------- | ------: | --------------: | -------------: | --------------: | --------------: | ---------------: |
| `a0`         | after 1 s   | 275,705 |           0.956 |          1.051 |           0.970 |           1.010 |            0.920 |
| `a0`         | at most 1 s |  28,698 |           1.234 |          0.992 |           1.002 |           0.959 |            0.973 |
| `face_state` | after 1 s   | 214,803 |           3.420 |          0.795 |           0.833 |           0.751 |            0.733 |
| `face_state` | at most 1 s |   4,681 |           3.396 |          1.000 |           1.000 |           1.000 |            1.000 |

On the medoid neither model lowers the tail after the first second (IMM 0.970 against criterion 4's
0.8). On the faces after 1 s, CA's along ratio is 0.795 with across 0.751: at the bar, on a face
tail 3.4 m at p99, 3.6 times the medoid's, the same metre-scale face tail l5est0005 and l5est0007
found. The offline models are founded at each track's first scored observation, so their first
second is founding, where every model reads alike.

**Jerk and turning** (second segments; Punzo et al. 2011): share of jerk samples above 15 m/s³
and sign changes per second, live filter 10.1 % and 4.75, offline CV 9.9 % and 4.85, CA with the
coast rule 19.7 % and 4.53, IMM 18.0 % and 4.55 on the medoid; solid body 5.2 % and 4.26, CA
11.2 %, IMM 9.6 % on the faces. 9.5 % of the medoid's 218,180 track-frames with a yaw rate turn
faster than 0.1 rad/s and 4.7 % faster than 0.2 rad/s (faces 7.5 % and 3.4 %).

**Criterion 3** (three same-sign one-step along values beyond one sigma): 3,464 of 4,285 medoid
tracks, 80.8 %; 2,050 of 3,255 face tracks, 63.0 %.

## Per site

Second segments, one-step along z RMS at most 1 s (young) and after 1 s (mature) and the after-1 s
correlation's 5th percentile. The last four columns are the 22-segment read with that site left
out.

| Site                  | Tracks with residuals | Young | Mature | Mature r | Mature p05 | Face young | Face mature | Face mature p05 | Out: young | Out: mature | Out: p05 | Out: face p05 |
| --------------------- | --------------------: | ----: | -----: | -------: | ---------: | ---------: | ----------: | --------------: | ---------: | ----------: | -------: | ------------: |
| 1st-mission           |                   317 | 1.718 |  0.751 |    0.302 |      0.263 |      0.982 |       0.605 |           0.191 |      2.464 |       0.856 |    0.239 |         0.066 |
| 3rd-folsom            |                   270 | 2.665 |  0.851 |    0.286 |      0.231 |      1.988 |       0.734 |           0.101 |      2.401 |       0.846 |    0.241 |         0.071 |
| broadway-franklin     |                   162 | 2.278 |  1.042 |    0.156 |      0.116 |      1.343 |       0.929 |          −0.045 |      2.425 |       0.840 |    0.247 |         0.071 |
| bush-powell           |                   224 | 1.964 |  0.665 |    0.262 |      0.216 |      1.042 |       0.545 |           0.168 |      2.440 |       0.857 |    0.243 |         0.067 |
| california-leon-baker |                    43 | 3.166 |  0.653 |    0.139 |      0.029 |      1.412 |       0.891 |          −0.227 |      2.408 |       0.848 |    0.245 |         0.071 |
| columbus-broadway     |                   307 | 2.083 |  0.751 |    0.208 |      0.171 |      0.995 |       0.576 |           0.132 |      2.442 |       0.854 |    0.246 |         0.068 |
| columbus-north-point  |                   106 | 2.086 |  0.764 |    0.330 |      0.256 |      0.613 |       0.690 |          −0.041 |      2.426 |       0.848 |    0.243 |         0.119 |
| embarcadero-bay       |                   213 | 2.353 |  0.838 |    0.244 |      0.209 |      1.104 |       0.602 |           0.176 |      2.422 |       0.847 |    0.244 |         0.064 |
| embarcadero-broadway  |                   303 | 2.467 |  0.790 |    0.321 |      0.277 |      1.460 |       0.650 |           0.134 |      2.415 |       0.850 |    0.241 |         0.065 |
| embarcadero-bryant    |                   366 | 2.292 |  0.981 |    0.284 |      0.237 |      1.142 |       0.794 |           0.098 |      2.430 |       0.839 |    0.241 |         0.069 |
| franklin-mcallister   |                   311 | 1.545 |  0.844 |    0.179 |      0.145 |      0.846 |       0.697 |           0.068 |      2.468 |       0.847 |    0.249 |         0.066 |
| fulton-divisadero     |                   157 | 2.493 |  0.918 |    0.168 |      0.130 |      1.684 |       0.818 |          −0.017 |      2.416 |       0.844 |    0.247 |         0.072 |
| haight-divisadero     |                    53 | 1.916 |  0.777 |    0.340 |      0.250 |      0.831 |       0.683 |           0.023 |      2.424 |       0.847 |    0.244 |         0.069 |
| howard-6th            |                   212 | 2.253 |  0.942 |    0.233 |      0.191 |      1.680 |       0.747 |           0.028 |      2.427 |       0.842 |    0.244 |         0.070 |
| hyde-ofarrell         |                    85 | 2.097 |  0.729 |    0.255 |      0.182 |      0.931 |       0.639 |           0.122 |      2.425 |       0.849 |    0.244 |         0.068 |
| laguna-eddy           |                    77 | 2.052 |  0.933 |    0.178 |      0.110 |      0.905 |       0.761 |          −0.057 |      2.426 |       0.845 |    0.245 |         0.068 |
| lombard-broderick     |                   350 | 3.074 |  1.057 |    0.144 |      0.113 |      1.574 |       0.827 |           0.052 |      2.348 |       0.827 |    0.259 |         0.068 |
| lombard-laguna        |                   265 | 3.478 |  1.007 |    0.266 |      0.217 |      2.078 |       0.879 |           0.001 |      2.337 |       0.837 |    0.245 |         0.074 |
| marina-broderick      |                   129 | 2.407 |  0.844 |    0.388 |      0.329 |      0.958 |       0.728 |           0.184 |      2.419 |       0.846 |    0.236 |         0.060 |
| marina-webster-beach  |                    70 | 2.741 |  0.663 |    0.378 |      0.313 |      1.802 |       0.630 |           0.393 |      2.413 |       0.853 |    0.241 |         0.065 |
| pierce-haight         |                    76 | 1.670 |  0.738 |    0.252 |      0.190 |      0.742 |       0.602 |          −0.005 |      2.430 |       0.848 |    0.244 |         0.070 |
| van-ness-sacramento   |                   189 | 2.789 |  0.780 |    0.266 |      0.219 |      1.179 |       0.667 |           0.064 |      2.398 |       0.849 |    0.243 |         0.067 |

Worst leave-one-out: for N1, lombard-laguna out gives the lowest young RMS (2.337) and bush-powell
out the highest after-1 s RMS (0.857); for N2, marina-broderick out gives the lowest 5th
percentile on both sources (0.236 medoid, 0.060 faces). No single site decides N1 or N2. Site by
site, the young RMS is above 1.5 at all 22 sites (lowest franklin-mcallister, 1.545); the after-1 s
RMS is above 1.0 at three (lombard-broderick 1.057, broadway-franklin 1.042, lombard-laguna
1.007), among the most over-dispersed sites of the first segments too. The medoid's after-1 s
5th percentile is above 0.1 at 21 sites, the faces' at 9.

## Interpretation

**What was measured.** On the second five minutes of 22 sites, with the shipped tuning, the CV
filter's one-step along-track residuals on the cluster medoid read as l5est0007's did on the
first five: slightly over-dispersed in the pool (RMS 1.06, 6.7 % beyond 1.96), strongly
under-dispersed across (0.40). Split at 1 s of track age, as the rule fixed before the replays,
the first second reads 2.42 and the rest 0.85. The figures match the first segments' to within
0.1 on the young side and 0.01 on the mature side.

**Where the excess comes from.** In the first second the along z has mean +1.89: the vehicle is
ahead of where the filter predicted, consistently, so the filter starts slower than the traffic.
Bias carries about 61 % of the young mean square. That is the state at birth, an initial velocity
and covariance that do not yet describe the vehicle, which the class-range velocity prior the
November 10 review names is built to fix. It is not acceleration: in the first second `z_along²`
does not correlate with it on either source.

**After the first second.** The medoid's along z is under-dispersed (0.85) and its square still
rises with acceleration (partial r 0.26). The filter's covariance already contains that lag, so
the CV model is not failing there in the sense G-EST-1 asks about; offline CA does not lower the
medoid's tail at all (ratio 1.05) and the IMM by 3 %. On the faces the correlation is weak (0.09,
5th percentile 0.07), and CA's lower face tail sits on a metre-scale face residual that is a
measurement question first.

**What this does not establish.** Nothing here measures a speed against a reference. That the
young excess is a lag is read from the sign of a self-consistency statistic, not from a vehicle's
known position. The first segments set the boundary, so this read tests a hypothesis they
suggested on later data from the same placements; it is not independent traffic.

## Limitations

- **The boundary and bounds** come from l5est0007's post-hoc read of the first segments. The
  second segments follow the first in time at every site (300 to 600 s after the first packet),
  so their traffic, scene and sensor pose resemble the first's. The held-out read guards against
  an accident of the first five minutes, not against a site-level artefact.
- **Track age** is the filter's: a fragment reborn mid-pass is young. Part of the young excess may
  be re-acquisition of a vehicle the tracker lost rather than its first sight. Not split here.
- **No reference.** No label was read and no speed reference exists off kirk0. The acceleration is
  the pipeline's own smoothed estimate from the same observations; z reads the filter's
  consistency with itself.
- **Earlier reads of these segments.** They had been read for identity and noise and, today, for
  settling (l3bg0030); never for filter consistency.
- **Two builds in the pool.** The first segments' records are l5est0007's at `6cb4735`; the
  reproduction on haight-divisadero's first segment is one site. The changes between the builds
  are default-off shadows.
- **The coverage flag** for N1's 5th to 95th interval was added after the first replays started
  and before any second-segment figure was read. At the 95 % level the same intervals are 2.387 to
  2.451 and 0.832 to 0.860, and N1 holds there too.
- **Criterion 4's split** is on the second segments only: the first segments' records carry no
  offline ages. The offline models run at their prototype defaults.
- **Scratch eligibility.** The segments are not in the committed cohort manifest; the replays ran
  under a scratch manifest assigning each segment, by hash, to its site as development data, and
  a scratch copy of the use ledger. embarcadero-folsom was not read.

## Recommendations

1. **Ship nothing.** No default changes. CA and the IMM stay offline; no CA-with-coast spec follows
   from G-EST-1 on this evidence.
2. **Specify the initialisation increment next**, on the first second of each moving track: the
   class-range velocity prior against the shipped initial velocity and covariance, read by the
   young along z mean and RMS (here +1.89 and 2.42 on the medoid) and the after-1 s figures as the
   guard that nothing else moves. Split young residuals into first sight and re-acquisition before
   ruling.
   - Falsified if: a prior that brings the young along z mean near zero leaves the young RMS above
     1.5.
3. **G-UNC-1 before G-EST-1 again.** Across-track RMS is 0.39 on the medoid and 0.28 on the faces
   on both segment sets. A recalibrated S may make the after-1 s along read over-dispersed where
   acceleration is, and G-EST-1 should be read again then, on the 45-segment pool.
4. **The face tail.** CA's face along ratio after 1 s (0.795) is at criterion 4's bar on a 3.4 m
   p99 tail. Bisect the face observation's change (l5est0007's recommendation 5) before any face
   criterion is ruled on.

## Provenance

| Item          | Value                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base          | `2c870f4` (spec approved, rule committed before any replay)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Commits       | `1851d50` (age split, offline ages, `LABEL=PATH`), before the replays; `3df1dad` (`-level`), before any second-segment figure was read; report and spec in the commit after                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Builds        | the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52) and the `pcap` tag. `lidar-state-estimation-baseline` at `1851d50`, `sha256:46241d4b…`; every replay manifest reads `build_version` 0.5.1-pre52, `build_git_sha` `1851d509…`, stamped. `lidar-gest1-pool` at `3df1dad`, `sha256:d44900f7…`. First-segment records: l5est0007's build `6cb4735`                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Parameter set | `config/tuning.defaults.json` `ab0c4035780d` (the `a0` check replay's `params_hash`); `face_state` replays `sha256:80e04c6e…` with their experiments folded in                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Replay        | one case per run, `-warmup 40 -require-settled=false -sample-points 1024 -gest1-report`, repeat-verified; `-experiment solid_body,solid_body_face_state`, evidence per case, discarded; two lanes, `GOMAXPROCS=2`, `nice -n 5`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Pools         | second segments at `-level 0.9` (N1, N2) and 0.95; 45 segments at 0.95 (N3); 2,000 draws, seed 20261110                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Segments      | 300 to 600 s of each placement from `banshee-data/sf-street-speeds`. SHA-256 prefixes: 1st-mission `e2341d3b`, 3rd-folsom `6a696d2e`, broadway-franklin (published as broadway-gough) `8f39f1d6`, bush-powell `22df7d52`, california-leon-baker `bf45bc27`, columbus-broadway `9f60b5dc`, columbus-north-point `7dc18457`, embarcadero-bay `506fba05`, embarcadero-broadway `36ced09e`, embarcadero-bryant `a9601871`, franklin-mcallister `8296eb12`, fulton-divisadero `eaf7b74e`, haight-divisadero `5b4d6b8e` (shared copy), howard-6th `522e0d05`, hyde-ofarrell `ad7948a4`, laguna-eddy `ac24fc4d`, lombard-broderick `667e2358`, lombard-laguna `2133fa1a`, marina-broderick `0a289b67`, marina-webster-beach `9f0a01d6`, pierce-haight `5c582efe`, van-ness-sacramento `cda2b730` |
| Checks        | `a0` against `face_state` on haight-divisadero's second segment: tracking baseline `sha256:ae7c4935…` both; haight-divisadero's first segment at `1851d50`: `sha256:c24f7d2a…`, l5est0007's                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Sites         | all 22 of `d2-second-segment`; ashbury-downey (one capture) is not in it; embarcadero-folsom (held out) was not downloaded or read                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Host          | Claude Code cloud container, 4 cores shared with one other experiment agent, Go 1.27.1 linux/amd64; replays 10:27 to 12:31 UTC, 353 to 991 s each; raw output in the session's scratch, not kept                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
