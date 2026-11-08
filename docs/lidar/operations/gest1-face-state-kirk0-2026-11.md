# G-EST-1 on the face-referenced measurement: a kirk0 screen

<!-- ignore-style-length -->

The state plan moves the tracker from constant velocity (CV) to constant acceleration (CA) only
behind G-EST-1, whose second criterion asks whether CV's NIS over-dispersion is correlated with
acceleration rather than with range, aspect or point count. The November maths review warned
that on the cluster medoid the test is likely to pass for the wrong reason: the medoid slides along
a long body as it enters and leaves view, and a CV filter reads the slide as acceleration. This
run reads G-EST-1's criteria 2 to 5 on kirk0 twice: on the shipped tracked filter's medoid
residuals and on the shadow solid body's face-referenced residuals. It answers
[l5est0005](../../../data/experiments/l5est/l5est0005.md).

- **Status:** Screen complete, 2026-11-08. No default changed; no motion model ships
- **Layers:** L5 (the tracked filter, the shadow solid body), read by L8 analytics
- **Related:** [state plan §7.3](../../plans/lidar-state-estimation-plan.md#73-recommendation-and-gate),
  [maths review §4.3](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md#43-why-ca-must-wait-for-the-measurement),
  [motion models maths](../../../data/maths/motion-models-maths.md#10-g-est-1-analyses),
  [figure0002 and l5est0003](cloud-run-kirk0-labelled-2026-11.md)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) for both arms,
  fingerprint `ab0c4035780d`; `face_state` adds the replay experiments `solid_body` and
  `solid_body_face_state`, which are not tuning keys

## Answer

**As declared, the screen passes: on kirk0 the face-referenced CV residuals' NIS is correlated
with acceleration, criterion 4 passes, and criterion 5 holds for CA with the coast rule and for the
IMM.** By l5est0005's rule that supports proposing a CA increment for the corpus read, and nothing
more. kirk0 holds 43 moving tracks against criterion 1's 5,000, so this is not a G-EST-1 decision.

Three findings qualify that pass, and they matter more than it does:

1. **There is no over-dispersion to explain.** Both arms' NIS is under-dispersed: mean NIS per
   degree of freedom 0.43 on the medoid and 0.24 to 0.33 on the faces, with 1.8 % and 0.2 to 0.8 %
   above the χ² 95 % point against a nominal 5 %. Along the track alone the normalised RMS is 0.91
   and 0.65, against 1. What correlates with acceleration is variation inside a
   conservative distribution, not an excess the filter fails to expect. G-EST-1's criterion 2 is
   worded for over-dispersion. On kirk0 its premise is not met.
2. **The medoid's correlation is not only the slide.** The tracked filter's NIS correlates with
   the face-referenced acceleration at the same track and time (r 0.161, track interval 0.070 to
   0.257). Section 4.3's prediction, that the medoid passes only because of the measurement, is
   not supported: the slide may inflate the medoid's r (0.216 against 0.172), but it does not
   explain all of it.
3. **Criterion 4's pass on the faces is a pass on noisier observations.** Offline CV's p99
   along-track innovation is 4.06 m on the face observations against 1.27 m on the medoid, for
   the same tracks (3.23 m against 0.97 m one-step). CA lowers the face tail by 38 % and the
   medoid's by 3 %. A model with more bandwidth follows observation jumps faster, so this pass is
   not by itself evidence of acceleration.

**Recommendation:** do not ship CA on this evidence. Restate criterion 2 for an under-dispersed
filter (Section [Recommendations](#recommendations)), and run it on the corpus with the
face-referenced source and the one-step reads before any CA arm.

## Question and arms

Is CV's over-dispersion acceleration-specific once the measurement is face-referenced? Both arms
replay kirk0 whole from `config/tuning.defaults.json`:

| Arm          | Replay experiments                    | Residual source read | What it measures                                                   |
| ------------ | ------------------------------------- | -------------------- | ------------------------------------------------------------------ |
| `a0`         | none                                  | `tracked_filter`     | The shipped CV filter on the cluster medoid                        |
| `face_state` | `solid_body`, `solid_body_face_state` | `solid_body_faces`   | The shadow solid body's near-edge fixes, half-extents in the state |

`solid_body_face_state` switches on the five face-state options of the maths review
(Sections 3.1 to 3.5): extents in the state, censored extent evidence, span bounds in the belief,
heading face noise and the anisotropic reference bias. The solid body is a shadow: it changes no
track. The two arms' `tracking_baseline.json` are byte-equal
(`sha256:79d9abff…`), and the `face_state` replay's `tracked_filter` section equals `a0`'s field for
field, so the arms read the same tracks and differ only in the measurement read.

## Method

The rule was committed in `4935e17` before any replay. The code is `5001c6c`; the post-hoc reads
below were added in `ac2e2b2` and `ab3fa87`, after the declared replay.

- **Residuals.** `a0`: each applied update's innovation against the tracked prior, with
  `S = P⁻ + R·I` and the live NIS at two degrees of freedom; recomputing `yᵀS⁻¹y` from the record
  matches the filter's NIS to 1.8 × 10⁻⁷. `face_state`: each near-edge fix's joint face
  innovations against the solid body's prior (`l5tracks.FaceInnovation`), rank-aware, with the live
  NIS at the fix's rank; the joint form matches the update's sequential NIS to 1.7 × 10⁻⁷. A
  two-face fix is rotated into along-track and across-track components; a one-face fix gives only
  its normal's component, attributed to the course axis the normal lies nearer.
- **Population.** Confirmed chains with at least 10 observations whose smoothed speed reaches
  2 m/s, and their residuals after the 6 s warm-up. Criterion 2's samples also need a course
  (smoothed speed at least 0.5 m/s), a smoothed acceleration and the associated cluster's point
  count, joined from the tracker's input.
- **Smoothed acceleration.** The same estimator on each arm's own observations: the offline CV
  filter, smoothed over the whole chain (RTS), and the centred difference of the smoothed velocity
  at the nearest observations at or beyond ±0.5 s, projected on the smoothed course. A face
  fix's observation is the centre it implies at the predicted half-extent, with the face's noise
  plus that half-extent's variance.
- **Statistics.** Criterion 2 is the partial correlation of NIS per degree of freedom with
  |longitudinal acceleration|, controlling for range, aspect, point count and dimension (dropped
  where constant), with its 95 % Fisher-z interval and a 95 % percentile interval over 2,000
  seeded resamples of whole tracks. Criteria 3 to 5 are as the state plan defines them; criterion 5
  uses the existing synthetic-occlusion scorer, with a gap ending only at a two-dimensional
  observation.

## Population

| Measure                                              |  `a0` | `face_state` |
| ---------------------------------------------------- | ----: | -----------: |
| Chains recorded                                      |   151 |           71 |
| Moving tracks (criterion 1 asks for 5,000)           |    43 |           28 |
| Scored residuals of moving tracks                    | 4,701 |        3,663 |
| Criterion 2 samples                                  | 3,656 |        2,918 |
| Tracks behind them                                   |    42 |           28 |
| Excluded: no course                                  |   589 |          335 |
| Excluded: no acceleration window                     |   456 |          329 |
| Excluded: no smoothed state (leading rank-one fixes) |     0 |           81 |
| Leading rank-one fixes kept out of the observations  |     0 |          370 |

The solid body made 5,497 near-edge fixes over 6,586 solid-body rows (83.5 %), with 79 lapses.
Of `face_state`'s criterion 2 samples, 2,675 are rank two and 243 rank one.

## NIS level

| Arm          | Dim |     n | Mean NIS per dim | Median NIS | p95 NIS | Share above χ² 95 % |
| ------------ | --: | ----: | ---------------: | ---------: | ------: | ------------------: |
| `a0`         |   2 | 3,656 |            0.427 |      0.357 |   2.987 |              1.75 % |
| `face_state` |   1 |   243 |            0.335 |      0.038 |   1.044 |              0.82 % |
| `face_state` |   2 | 2,675 |            0.244 |      0.265 |   1.561 |              0.19 % |

A consistent filter has mean NIS per degree of freedom 1 and 5 % above the χ² 95 % point. Both
arms are under-dispersed: S is larger than the residuals it describes.

## Criterion 2: partial correlation with acceleration

| Arm          | Read                                    |     n | Tracks |     r | Fisher 95 %    | Track bootstrap 95 % |
| ------------ | --------------------------------------- | ----: | -----: | ----: | -------------- | -------------------- |
| `a0`         | declared                                | 3,656 |     42 | 0.216 | 0.184 to 0.246 | 0.153 to 0.288       |
| `a0`         | interior (1.5 s from the chain's ends)  | 3,130 |     33 | 0.211 | 0.177 to 0.244 | not computed         |
| `a0`         | ranks                                   | 3,656 |     42 | 0.357 | 0.328 to 0.385 | not computed         |
| `face_state` | declared                                | 2,918 |     28 | 0.172 | 0.137 to 0.207 | 0.089 to 0.246       |
| `face_state` | rank one                                |   243 |     15 | 0.177 | 0.052 to 0.297 | not computed         |
| `face_state` | rank two                                | 2,675 |     28 | 0.154 | 0.116 to 0.191 | not computed         |
| `face_state` | interior                                | 2,524 |     27 | 0.191 | 0.153 to 0.228 | not computed         |
| `face_state` | ranks                                   | 2,918 |     28 | 0.274 | 0.240 to 0.307 | not computed         |
| cross-read   | `a0`'s NIS, `face_state`'s acceleration | 2,847 |     30 | 0.161 | 0.125 to 0.197 | 0.070 to 0.257       |

Every interval lies above zero. The track bootstrap is two to three times the Fisher interval's
width, as expected when samples within a track are not independent; it still excludes zero in
every row it was computed for. Mean |longitudinal acceleration| over the samples is 0.70 m/s² on
the medoid and 0.60 m/s² on the faces (rank two).

## Criterion 3: run-length signature

| Arm          | Moving tracks | Flagged |  Share |
| ------------ | ------------: | ------: | -----: |
| `a0`         |            43 |      36 | 83.7 % |
| `face_state` |            28 |      16 | 57.1 % |

Both are far above G-EST-1's 5 %. The rule reads this as context only: a slide also makes
same-sign runs, and the faces' share is lower.

## Criterion 4: CA's one-step residual tails

Offline CA with the coast rule against offline CV over each arm's observations, |innovation| in
metres:

| Arm          | n along | CV p99 along | CA p99 along | Ratio | CV p99 across | CA p99 across | Ratio | Pass |
| ------------ | ------: | -----------: | -----------: | ----: | ------------: | ------------: | ----: | ---- |
| `a0`         |   4,075 |        1.271 |        1.236 | 0.972 |         0.594 |         0.597 | 1.006 | no   |
| `face_state` |   3,050 |        4.063 |        2.500 | 0.615 |         2.031 |         1.728 | 0.851 | yes  |

The bar is an along ratio of at most 0.8 and an across ratio of at most 1.05.

## Criterion 5: reacquisition across synthetic gaps

Median and p95 reacquisition error in metres. Every row has at least 86 cases, so every row is
read.

`a0`, 1,227 cases:

| Horizon | Split        |   n | CV median | CV p95 | CA coast median | CA coast p95 | IMM median | IMM p95 | CA no coast median | CA no coast p95 |
| ------: | ------------ | --: | --------: | -----: | --------------: | -----------: | ---------: | ------: | -----------------: | --------------: |
|   0.5 s | all          | 434 |     0.468 |  1.372 |           0.341 |        1.510 |      0.367 |   1.350 |              0.351 |           1.683 |
|   0.5 s | decelerating | 133 |     0.537 |  1.358 |           0.451 |        1.885 |      0.431 |   1.597 |              0.451 |           1.954 |
|   1.0 s | all          | 417 |     0.957 |  2.716 |           0.800 |        3.034 |      0.772 |   2.561 |              0.828 |           3.546 |
|   1.0 s | decelerating | 120 |     1.070 |  2.466 |           0.932 |        3.536 |      0.906 |   3.143 |              0.945 |           3.918 |
|   2.5 s | all          | 376 |     2.978 |  7.834 |           2.597 |        9.380 |      2.492 |   7.239 |              3.259 |          12.441 |
|   2.5 s | decelerating | 109 |     3.298 |  8.521 |           2.656 |       10.181 |      2.659 |   7.415 |              3.452 |          15.693 |

`face_state`, 870 cases:

| Horizon | Split        |   n | CV median | CV p95 | CA coast median | CA coast p95 | IMM median | IMM p95 | CA no coast median | CA no coast p95 |
| ------: | ------------ | --: | --------: | -----: | --------------: | -----------: | ---------: | ------: | -----------------: | --------------: |
|   0.5 s | all          | 306 |     1.027 |  3.668 |           0.621 |        2.635 |      0.671 |   2.966 |              0.599 |           2.603 |
|   0.5 s | decelerating |  96 |     1.113 |  3.033 |           0.653 |        2.267 |      0.764 |   2.593 |              0.641 |           2.305 |
|   1.0 s | all          | 303 |     1.630 |  5.355 |           1.206 |        4.262 |      1.157 |   4.534 |              1.223 |           4.240 |
|   1.0 s | decelerating |  94 |     1.860 |  4.871 |           1.148 |        4.262 |      1.284 |   4.460 |              1.242 |           4.087 |
|   2.5 s | all          | 261 |     4.154 | 10.993 |           3.681 |       10.379 |      3.170 |  10.213 |              4.282 |          11.648 |
|   2.5 s | decelerating |  86 |     5.131 | 10.580 |           3.906 |        9.954 |      4.143 |  10.574 |              4.460 |          10.276 |

On the faces, CA with the coast rule and the IMM are no worse than CV in any row; CA without the
rule is worse at 2.5 s on all gaps. On the medoid, CA with the coast rule has a better median and
a worse p95 than CV at every horizon, and the IMM's p95 is worse on decelerating gaps at 0.5 and
1.0 s: on the medoid neither holds criterion 5.

## The rule's reading

| Clause                                          | `a0`                     | `face_state`             |
| ----------------------------------------------- | ------------------------ | ------------------------ |
| 1. Criterion 2 acceleration-specific            | yes (both intervals > 0) | yes (both intervals > 0) |
| 2. Criterion 3 share above 5 % (context)        | 83.7 %                   | 57.1 %                   |
| 3. Criterion 4 passes                           | no (0.97, 1.01)          | yes (0.62, 0.85)         |
| 4. Criterion 5 holds, CA with coast rule or IMM | no                       | yes, both                |
| 5. The CA increment is supported on kirk0       | not read                 | yes, as a screen         |

Clause 5's attribution reads: `a0` is acceleration-specific as well, and the cross-read is not
null, so the medoid's result is not attributed to the measurement alone.

## Post-hoc reads, not part of the rule

The declared replay showed three things the rule did not anticipate. The reads below were added
after it (commits `ac2e2b2` and `ab3fa87`) and replayed at `ab3fa87`. Every declared figure above
reproduced exactly at that build, and the tracking baseline was the same in all four replays.
These reads qualify the rule's reading; they do not replace it.

- **Along and across separately.** The total NIS mixes two axes, so each residual's along-track
  and across-track components are normalised by their own predicted sd.
- **One-step only.** G-EST-1 calls criteria 2 to 4 one-step tests. Residuals whose prior was
  predicted across more than 0.15 s since the filter's previous update are left out: after a miss
  for the tracked filter, after frames without a usable face for the solid body.
- **Face-referenced only.** A fix that re-references a medoid-referenced body carries the
  medoid's offset in its innovation. `solid_body_face_state` does not translate the reference
  first, so those 44 fixes are left out.

| Measure                                  |            `a0` |    `face_state` |
| ---------------------------------------- | --------------: | --------------: |
| One-step residuals                       |           4,375 |           3,362 |
| Multi-step residuals left out            |             326 |             257 |
| Re-referencing fixes left out            |               0 |              44 |
| Along z RMS, all (one-step)              |   0.896 (0.906) |   0.658 (0.655) |
| Along z mean, all (one-step)             | +0.153 (+0.146) | +0.097 (+0.094) |
| Along share beyond 1.96, all (one-step)  |   3.5 % (3.6 %) |   0.6 % (0.6 %) |
| Across z RMS, all (one-step)             |   0.364 (0.366) |   0.249 (0.234) |
| Across share beyond 1.96, all (one-step) |   0.3 % (0.3 %) |  0.03 % (0.0 %) |

| Read, one-step                    |           `a0` |   `face_state` |
| --------------------------------- | -------------: | -------------: |
| Criterion 2 samples (tracks)      |     3,420 (42) |     2,720 (27) |
| r                                 |          0.219 |          0.207 |
| Fisher 95 %                       | 0.187 to 0.251 | 0.171 to 0.243 |
| Track bootstrap 95 %              | 0.155 to 0.295 | 0.106 to 0.281 |
| Criterion 3 share                 |         81.4 % |         57.1 % |
| Criterion 4 CV p99 along (CA), m  |  0.967 (0.871) |  3.229 (2.292) |
| Criterion 4 along ratio           |          0.901 |          0.710 |
| Criterion 4 CV p99 across (CA), m |  0.426 (0.389) |  1.918 (1.453) |
| Criterion 4 across ratio          |          0.914 |          0.758 |
| Criterion 4 pass                  |             no |            yes |

Restricted to one-step, face-referenced residuals, nothing in the rule's reading changes sign or
crosses its bar.

## Interpretation

**What was measured.** On kirk0, with the shipped tuning, the CV filter's NIS rises with the
smoothed longitudinal acceleration estimated from its own observations. That holds on the
medoid (partial r 0.22) and on the face-referenced solid body (0.17, and 0.21 one-step), after
range, aspect, point count and dimension are controlled for. It survives resampling whole tracks
and a rank transform, and it holds on chain interiors. The tracked filter's NIS also rises with
the faces' acceleration. Both filters' residuals sit ahead of their predictions on average (along
z mean +0.15 and +0.09), and both have same-sign runs on most moving tracks. Neither is
over-dispersed: along-track RMS is 0.91 and 0.65, across-track 0.37 and 0.23, against 1 for a
consistent filter.

**What that supports.** The pattern is CV lag under acceleration: residuals that grow with
acceleration and lean ahead of the prediction. On kirk0, though, the filter's covariance is wide
enough to contain it. That is a weaker case for CA than G-EST-1 imagines. The criterion asks
whether the filter fails, measured as over-dispersion, because of acceleration. Here the filter
does not fail in that sense; it is conservative, and least conservative when the vehicle
accelerates. The across-track slack (RMS 0.23 to 0.37) shows S is set by more than acceleration.
That is G-UNC-1's question, and it should be answered first: a filter whose S is recalibrated may
become over-dispersed along the track exactly where this run shows the correlation.

**The slide.** Section 4.3 predicted that the medoid would pass for the wrong reason. The medoid's
correlation is somewhat higher than the faces' (0.22 against 0.17 declared, 0.22 against 0.21
one-step). The tracked filter's NIS still correlates with the face-referenced acceleration (0.16),
so the slide is not the whole of it. This run cannot apportion the difference; the intervals
overlap.

**Criterion 4.** On the faces CA cuts the p99 one-step along-track innovation by 29 % (38 % with
multi-step steps). On the medoid it cuts 10 % (3 %), short of the 20 % bar. But the faces' CV p99
is 3.2 m, three times the medoid's 0.97 m, on the same tracks. Those are metre-scale jumps in the
face-implied centre. A plausible cause, not shown here, is a face change with the half-extent
still wrong. CA's extra bandwidth follows such jumps faster. The faces' pass is therefore not, by
itself, evidence of acceleration. It is evidence that the face observation has a heavy tail that
the measurement work should remove first.

**Criterion 5.** On the faces, CA with the coast rule and the IMM reacquire no worse than CV at
every horizon and split, and better at the median (0.62 against 1.03 m at 0.5 s). On the medoid,
both improve the median and worsen the p95: CA with the coast rule at every horizon, the IMM on
decelerating gaps. Criterion 5's errors are scored against each arm's own next observation, so
the arms' absolute errors are not comparable: the faces' targets are noisier.

## Limitations

- **A screen on tuning data.** kirk0 is the tuning capture. It supplies 43 moving tracks (28 with
  face fixes) against criterion 1's 5,000 confirmed moving tracks. One truck can move any row.
- **No physical reference.** The acceleration is the pipeline's own smoothed estimate, not a
  measured acceleration, and no label was read. "Acceleration" here means the CV smoother's
  centred difference on each arm's observations, attenuated within about 1.5 s of a chain's ends;
  the interior read keeps it unattenuated and agrees.
- **The rule's own gap.** l5est0005's clause 1 read criterion 2 as a correlation off zero and did
  not require over-dispersion, which G-EST-1's wording does. The declared pass should be read
  with that omission in mind.
- **Post-hoc reads.** Everything under "Post-hoc reads" was designed after the declared numbers
  were seen. It is diagnostic.
- **The face observations for the offline models** convert each face to the centre it implies at
  the predicted half-extent, with the half-extent's variance added and its covariance with
  position dropped. Criteria 4 and 5 on `face_state` are therefore approximations of the solid
  body, not runs of it. The offline CA, coast and IMM parameters are the prototype defaults,
  not values G-EST-1 has selected.
- **The solid body ran on the retained sample** (1,024 points per cluster), not every member, and
  without reference translation, as `solid_body_face_state` defines the arm.
- **K9 is not reproduced.** The gap analysis found along-track innovation RMS 1.5 to 2.4 times the
  filter's expectation across the S2 sites. Here, normalised by `S = P⁻ + R`, kirk0's medoid reads
  0.91. This run does not establish whether the difference is the site, the normalisation or the
  population.

## Recommendations

1. **Ship nothing from this.** The screen proposes a corpus read; it decides no motion model.
2. **Restate criterion 2 before the corpus read.** Require along-track over-dispersion on moving
   tracks (one-step along z RMS above 1, or more than 5 % beyond 1.96) and the partial
   correlation off zero on the one-step, face-referenced residuals, with the track bootstrap. As
   worded, an under-dispersed filter can pass it.
3. **Settle calibration first.** The across-track slack says S is conservative independently of
   acceleration. Read G-UNC-1 on these residuals; a recalibrated S changes what criterion 2 can
   detect.
4. **Find the faces' metre-scale tail** before scoring a motion model on face observations: read
   the fixes with the largest along-track innovations by face transition and half-extent state.
5. **Run the corpus read** with `-gest1-report` and `-experiment solid_body,solid_body_face_state`
   across the S2 segments. It is the only route to criterion 1's population. Translate the
   reference at re-reference (`solid_body_reference_translation`) or keep the one-step,
   face-referenced read as the declared one.

## Provenance

| Item           | Value                                                                                                                                                                                                        |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Base           | main `99a9627`                                                                                                                                                                                               |
| Commits        | code `5001c6c`; rule `4935e17`, before any replay; post-hoc reads `ac2e2b2` and `ab3fa87`                                                                                                                    |
| Builds         | `lidar-state-estimation-baseline` with the Makefile's LDFLAGS and the `pcap` tag, `0.5.1-pre52`: declared replay at `4935e17` (binary `sha256:efa8fa55…`), post-hoc replay at `ab3fa87` (`sha256:f28c6097…`) |
| Parameter sets | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`; replay parameter hash `sha256:ab0c4035…` (`a0`) and `sha256:80e04c6e…` (`face_state`, experiments folded in)                                      |
| Capture        | kirk0 `sha256:2864ebde…` (Git LFS object, hash verified), 832 frames; source manifest `sha256:92a97f27…`                                                                                                     |
| Replay         | kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024 -gest1-report`, evidence per case, every run repeat-verified; tracking baseline `sha256:79d9abff…` in both arms at both builds            |
| Eligibility    | [cohort manifest](../../../config/cohorts/cohort-manifest.json) revision 1 (D-40), an empty annotation root, a scratch use ledger at migration 71                                                            |
| Host           | Claude Code cloud container, 4 cores, 15 GB, Go 1.27.1 linux/amd64; each arm about 1.5 to 2.5 minutes for both runs, two arms in parallel; raw output in the session's scratch, not kept                     |
