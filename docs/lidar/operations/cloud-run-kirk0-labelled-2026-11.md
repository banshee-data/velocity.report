# kirk0's labels in a cloud run: the reconstructed pack, the bootstrap, the birth gate and the cap

<!-- ignore-style-length -->

The [November 5 review](experiment-review-2026-11-05.md) ranked eight experiments and noted that a
cloud container could run only kirk0. Two cloud runs that morning,
[#929](reacquisition-cloud-acceptance-2026-11.md) and
[#930](w3-motion-fragment-joining-cloud-2026-11.md), had left their labelled clauses at
"insufficient evidence" because the reviewed kirk0 labels' binary files could not be downloaded.
This run rebuilt those files from the capture, checked the rebuild against the published figures,
and then answered what kirk0 alone can answer: l5id0015 (how wide kirk0's identity figures are),
l4clus0009 (why more returns make more switches, and the radius graph beyond 50 m), the kirk0
clauses of l5id0014 (the birth gate) and l3bg0030 (the relative settling criterion), and the
labelled clauses #929 and #930 could not read.

- **Status:** Measured, 2026-11-05. One cloud run, 06:29 to 08:00 UTC; no default changed
- **Specs:** [l5id0015](../../../data/experiments/l5id/l5id0015.md),
  [l4clus0009](../../../data/experiments/l4clus/l4clus0009.md),
  [l5id0014](../../../data/experiments/l5id/l5id0014.md) (kirk0 only),
  [l3bg0030](../../../data/experiments/l3bg/l3bg0030.md) (kirk0 only),
  [figure0002](../../../data/experiments/figure/figure0002.md),
  [l5est0003](../../../data/experiments/l5est/l5est0003.md),
  [l4clus0008](../../../data/experiments/l4clus/l4clus0008.md) and
  [l5id0013](../../../data/experiments/l5id/l5id0013.md) (their labelled clauses)
- **Layers:** L3 settling, L4 input cap and joining, L5 identity, the per-frame evaluator
- **Related:** [noise and splits](pcl-noise-and-splits-2026-11.md),
  [the overlap end as default](overlap-end-default-cost-2026-11.md),
  [settling convergence](settling-convergence-corpus-2026-11.md)

## Summary

- **kirk0's labels can be scored in the cloud.** The new `lidar-pack-rebuild` rebuilds the pack's
  packet table and returns from the capture and the label set's text files, and both match the
  published digests exactly. The points are decoded by the current parser and their digest differs.
  On that pack the defaults score HOTA 0.4284, 90 identity switches and 46 fragmentations, against
  0.4283, 90 and 46 in [l5id0012](overlap-end-default-cost-2026-11.md), and W3 retention reads
  0.851 overall and 0.778 beyond 50 m, against 0.851 and 0.779 in the
  [noise and splits run](pcl-noise-and-splits-2026-11.md).
- **kirk0's identity figures are wide.** Drawing kirk0's 19 scored objects with replacement, the
  defaults' HOTA runs from 0.413 to 0.540 and their identity switches from
  4 to 242 (5th to 95th percentile). Of the round's claimed deltas only the overlap end's
  switches keep their sign (−231 to −2); its HOTA gain (+0.009 in the full comparison) and the
  radius graph's (+0.005) do not. The motion-supported join's IDF1 gain is the only other delta
  that does (+0.0004 to +0.0182).
- **The cap hides a tracker sensitivity.** Lifting the L4 input cap takes kirk0's switches from 90
  to 122; lifting it with tentative priority brings them to 95, within l4clus0009's bound, with
  retention 0.992 against 0.851.
- **The radius graph is retired by its rule, and the rule's figure moves without it.** Beyond 50 m
  it doubles fragmented instants (23 to 47), so l4clus0009 retires it. But the overlap end, which
  changes no L4 code, moves the same band from 23 to 45: the far band's fragmentation follows the
  tracker's feedback through the background, not only the joining rule.
- **The birth gate does almost nothing on kirk0.** It ended one track (88 tracks ended by the
  overlap end, against 89 without it), and every labelled figure equals the overlap end's. Of the
  overlap end's 14 brief tracks in the scored rows, 4 sit on another track's body, 7 beside one and
  3 alone: the re-birth inside a live body the review inferred is a minority on kirk0.
- **#930's motion-supported join fails its labelled target; #929's tentative arm passes its
  tuning clauses.** The join takes 20 to 35 m fragmented instants from 6.5 % to 5.7 % against the
  5 points required, with every other clause held. `body-older-tentative` passes l5id0004's limits
  on kirk0; `body-older` misses IDF1 by 0.0007.
- **A speed defect, found by the first independent speed check (figure0002).** Every
  `lidar_track_observations` row's `speed_mps` held the track's running mean speed since birth,
  not its speed at the frame. Over the spans of kirk0's three reviewed vehicles the rows read 0.38
  to 0.72 of the reference speeds, while the tracks' own displacement matched the references to
  within 0.42 m/s. The lane field's speeds and vehicle filter, the web live view's speed and peak
  and the noise screen's near-static rule all read that column; the published survey percentiles
  do not. The row now carries the instant speed. With it, the car's filter speed is within its
  bound and the two trucks' read 0.77 and 0.86 m/s (about 12 %) low: the filter lags them.
- **A looser velocity process noise is an identity lever (l5est0003).** It did not end the
  filter's lag on kirk0's trucks, its question; but doubling `process_noise_vel` took kirk0's
  switches from 90 to 43, with HOTA and IDF1 gains that keep their sign over the bootstrap.
  Proposed as l5est0004 for the corpus and the held-out window.
- **The relative settling criterion does not settle kirk0.** kirk0's spread delta over its mean
  spread never falls under 0.075 before the ceiling (lowest 0.076 at 27.9 s, median 0.104), so the
  ratio arm settles at 30 s, as the honest absolute one does, and scores the same. A quiet scene
  with a small spread reads a high ratio: the corpus read should look for that.

## The reconstructed pack

**Why.** The dataset's JSON and NDJSON files download from huggingface.co in a cloud container;
its `.bin` files are served from `us.aws.cdn.hf.co`, which the network policy refuses. #929 and
#930 stopped there.

**How.** Every binary in the pack and the label set is derived:

| File                    | Rebuilt from                                                                 | Check                                                                      |
| ----------------------- | ---------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| label set `packets.bin` | the capture's UDP 2369 packets, each key the first 64 bits of SHA-256        | `velocity lidar labels info` reads the head state `12b47778…`, as recorded |
| pack `packets.bin`      | the same table                                                               | 149,741 packets, `sha256:3ffeba76…d036`, equal to the manifest's           |
| pack `returns.bin`      | each window's domain returns (`fg-67842cb1b22b`) added to the window's start | `sha256:f0f9ca27…6986`, equal to the manifest's                            |
| pack `points.bin`       | each return decoded by this build's parser, classification 1 (foreground)    | `sha256:7860a86c…`, against the published `004a847e…`: not equal           |
| backgrounds             | not rebuilt; dropped from the manifest                                       |                                                                            |

The manifest records the rebuilt points' digest, the recomputed pack digest
(`sha256:f0ed3414…15dc`) and that the pack is reconstructed. Ten variants of intensity and
classification bytes did not reproduce the published points digest, so the difference is in the
coordinates; the parser's geometry is unchanged since the pack's build (`0b540677`), so its cause
is not identified here.

**What it changes.** Labels cite returns, not coordinates, and the returns are exact. Retention
matches points to the pack by exact float32 coordinates, and only 41.3 % of the evidence's retained
sample points matched, under the 50 % floor; it was run with `-min-domain-share 0.3`.

**The check against published figures.** The split was drafted with
`lidar-annotation-split-draft -from-seconds 6 -split kirk0-tuning` (20 objects, samples 61 to 831;
`obj_788622b5`, the parked car, ignored as before), on the defaults at main `9f13d2a`:

| Measure                                      |                                                                        This run | Published        | Source                                                    |
| -------------------------------------------- | ------------------------------------------------------------------------------: | ---------------- | --------------------------------------------------------- |
| HOTA                                         |                                                                          0.4284 | 0.4283           | [l5id0012](overlap-end-default-cost-2026-11.md)           |
| Identity switches                            |                                                                              90 | 90               | l5id0012                                                  |
| Fragmentations                               |                                                                              46 | 46               | l5id0012                                                  |
| IDF1                                         |                                                                          0.4728 | 0.4706           | l5id0012                                                  |
| Overlap end: HOTA, switches, fragmentations  |                                                                   0.4374, 5, 34 | 0.4365, 5, 34    | l5id0012                                                  |
| Radius graph: HOTA, switches, fragmentations |                                                                  0.4334, 93, 30 | 0.4337, 90, 30   | [noise and splits](pcl-noise-and-splits-2026-11.md#kirk0) |
| Retention, all and 50 m+                     |                                                                    0.851, 0.778 | 0.851, 0.779     | noise and splits                                          |
| Fragmented 20–35, 35–50, 50 m+               | 6.5, 32.6, 9.0 % (33 of 510, 142 of 435, 23 of 256 instants reaching a cluster) | 6.5, 32.2, 8.9 % | noise and splits                                          |

False negatives and false positives are not comparable: this split scores 2,472 reference points
and the defaults miss 265, where the published runs report 502. The deltas between arms are what
this report reads.

## l5id0015: how wide kirk0's figures are

**Method.** `lidar-ground-truth-eval perframe -bootstrap 2000 -bootstrap-seed 20261105`, new in
this run. Each hypothesis track is attributed once to the reviewed object it lies within the gate
of on the most frames; 2,000 draws of the split's 19 scored objects with replacement score both
arms on the same draws. Tracks attributed to no object are left out of every draw, so the point
figures below are the labelled-object figures (the defaults' HOTA 0.4798 against the full
comparison's 0.4284).

| Arm (B, against `a0`)  | B's point HOTA | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches, 5th / 50th / 95th | Δ fragmentations, 5th / 50th / 95th | Interval keeps its sign |
| ---------------------- | -------------: | --------------------------- | --------------------------- | ----------------------------- | ----------------------------------- | ----------------------- |
| `overlap-end`          |         0.4856 | −0.0085 / +0.0055 / +0.0175 | −0.0142 / +0.0182 / +0.0560 | −231 / −82 / −2               | −38 / −12 / +3                      | switches                |
| `birth-gate`           |         0.4856 | −0.0085 / +0.0056 / +0.0175 | −0.0141 / +0.0183 / +0.0561 | −231 / −82 / −2               | −38 / −12 / +3                      | switches                |
| `body-older-tentative` |         0.4856 | −0.0085 / +0.0055 / +0.0175 | −0.0142 / +0.0182 / +0.0560 | −231 / −82 / −2               | −38 / −12 / +3                      | switches                |
| `radius-graph`         |         0.4842 | −0.0016 / +0.0033 / +0.0095 | −0.0192 / −0.0062 / +0.0096 | −5 / +3 / +13                 | −39 / −16 / 0                       | none                    |
| `nocap-tentative`      |         0.4767 | −0.0066 / −0.0024 / +0.0031 | −0.0130 / −0.0046 / +0.0089 | −7 / +4 / +22                 | −39 / −15 / 0                       | none                    |
| `join`                 |         0.4819 | −0.0011 / +0.0019 / +0.0040 | +0.0004 / +0.0077 / +0.0182 | −3 / −1 / 0                   | −12 / −4 / 0                        | IDF1                    |

The defaults on their own: HOTA 0.4798 (5th to 95th percentile 0.4133 to 0.5398), IDF1 0.5582
(0.5083 to 0.6218), identity switches 90 (4 to 242), fragmentations 46 (16 to 83). The switch
interval is wide because two trucks carry most of them: a draw that takes truck 1 twice and no car
doubles the count. 34 of `a0`'s 79 tracks are attributed to a reviewed object and 45 to none.

**The `a0` anchor.** At main `9f13d2a` on the reconstructed pack and the drafted split, the
defaults read HOTA 0.4284, IDF1 0.4728, 90 switches and 46 fragmentations in the full comparison;
the half-widths of the labelled-object intervals are 0.063 (HOTA), 0.057 (IDF1) and 119 switches.
They are recorded in the [l5id README](../../../data/experiments/l5id/README.md). The full
comparison equals l5id0012's 0.4283, 90 and 46 at `84492f9e`; the noise and splits run's 0.4293
and 87 at `ef787825` differ by a tool commit neither report records, which this run cannot
reproduce.

**Against the rule.** Clause 1: the anchor is recorded. Clause 2: of the round's deltas, the
overlap end's switches stand (−231 to −2); its HOTA (+0.0055 at the median, −0.0085 to +0.0175)
and the radius graph's (+0.0033, −0.0016 to +0.0095) do not; the radius graph's 20 to 35 m
fragmentation is a retention figure, not a per-frame one, and was not bootstrapped. The spec
wrote "the 5th percentile on the same side as the median"; this report reads both ends, the
5th and 95th, on one side of zero, which for a negative delta is the clause's intent. Clause 3:
the HOTA half-width is 0.063, over 0.01, so the open l5id clauses written at 0.005 or 0.01 cannot
be told apart on kirk0 as written: l5id0008 (IDF1 less 0.005), l5id0010 (HOTA and IDF1 less 0.005),
l5id0011 (HOTA and IDF1 less 0.005) and l5id0014 (HOTA and IDF1 less 0.005); l4clus0009's HOTA
less 0.01 is the same. No clause is changed by this spec; the owner resets them.

## l4clus0009: the cap, the tracker and the far band

| Arm               | Parameter set     | Replay options       |   HOTA |   IDF1 | Switches | Fragmentations | Retention | Fragmented 20–35 m (share of the 510 reaching a cluster) | Fragmented 50 m+ (share of those reaching a cluster) | Missed 50 m+ (of 348) |
| ----------------- | ----------------- | -------------------- | -----: | -----: | -------: | -------------: | --------: | -------------------------------------------------------: | ---------------------------------------------------: | --------------------: |
| `a0`              | defaults          |                      | 0.4284 | 0.4728 |       90 |             46 |     0.851 |                                                33 (6.5%) |                                            23 (9.0%) |                    92 |
| `nocap`           | `l4-cap100k.json` |                      | 0.4238 | 0.4655 |      122 |             47 |     0.992 |                                                33 (6.5%) |                                            23 (9.0%) |                    92 |
| `nocap-tentative` | `l4-cap100k.json` | `tentative_priority` | 0.4261 | 0.4697 |       95 |             31 |     0.992 |                                                34 (6.7%) |                                           45 (17.6%) |                    92 |
| `radius-graph`    | defaults          | `radius_graph_xy`    | 0.4334 | 0.4693 |       93 |             30 |     0.851 |                                                23 (4.5%) |                                           47 (17.7%) |                    82 |
| `overlap-end`     | defaults          | the four identity    | 0.4374 | 0.4961 |        5 |             34 |     0.851 |                                                34 (6.7%) |                                           45 (17.6%) |                    92 |

**Reading 1, the cap and the tracker.** `nocap-tentative` holds 95 switches against `a0`'s 90 plus
10, retention 0.992 against 0.99, and HOTA 0.4261 against `a0` less 0.01: the clause holds. By the
rule, the cap is hiding an association sensitivity rather than saving identity, and tentative
priority is the option every cap change needs. The cap's own question is now the Pi's frame-time
tail (perf0001). `nocap` alone reads 122 switches here against 148 in the November run; the replay
build differs.

**Reading 2, the radius graph beyond 50 m.** Fragmented instants beyond 50 m rise from 23 to 47,
more than 10 %, while 10 more instants reach a cluster: by the rule the graph splits far objects
and is retired as an arm. The rule did not foresee what the control arms show: `overlap-end` and
`nocap-tentative`, neither of which changes the joining rule, move the same band from 23 to 45,
and both carry tentative priority. The far band's fragmentation moves with how the tracker feeds
the background (the track guard keeps a tracked body's returns in the foreground), so a joining
rule cannot be judged on it alone. The verdict stands as written; the next joining experiment
should hold the tracker fixed or read the band against an identity-only control.

## l5id0014 on kirk0: the birth gate

The birth gate (`birth_gate_body`, new in this run, default off) refuses confirmation to a
tentative track inside a live confirmed track's observed returns. Its corpus clauses and
label0003's held-out clause need data a cloud container cannot reach; kirk0 answers clause 7 and
says what the gate does.

| Arm           |   HOTA |   IDF1 | Switches | Fragmentations | Brief tracks (noise screen) | Duplicate pairs (`lidar-box-eval`) | Vehicle tracks | Observed vehicle frames | Ended overlapped | Ended birth-gated |
| ------------- | -----: | -----: | -------: | -------------: | --------------------------: | ---------------------------------: | -------------: | ----------------------: | ---------------: | ----------------: |
| `a0`          | 0.4284 | 0.4728 |       90 |             46 |                          12 |                                 10 |             44 |                   4,523 |                0 |                 0 |
| `overlap-end` | 0.4374 | 0.4961 |        5 |             34 |                          18 |                                  4 |             37 |                   4,563 |               89 |                 0 |
| `birth-gate`  | 0.4374 | 0.4961 |        5 |             34 |                          17 |                                  4 |             37 |                   4,563 |               88 |                 1 |

Clause 7 holds (HOTA and IDF1 at `a0`'s less 0.005 or above, switches within `overlap-end`'s plus
5); kirk1 was not read, as its capture is on the LiDAR volume. The gate fired once.

**What the overlap end's brief tracks are on kirk0.** A diagnostic written after this result, not
part of any rule: of the 14 confirmed tracks spanning under 1 s of `lidar-box-eval`'s scored rows
under `overlap-end`, 4 lie inside another track's drawn box grown 0.25 m on half or more of their
frames, 7 lie within 3 m of another track, and 3 are alone; under `a0` one track is brief, alone.
l6cls0004's classes read the same evidence: the overlap end's clutter tracks are 13 artefacts (6
under `a0`), 4 fragments of a mover (4), 5 stationary road users (4) and 2 unclassed (4). The
review's account, that the brief tracks are re-births inside the surviving body which a birth gate
would stop, explains a minority of them on kirk0. The corpus, where brief tracks rose ninefold, is
still the deciding read, through l5id0016's rescoring.

## The labelled clauses #929 and #930 could not read

| Spec       | Arm                    |   HOTA |   IDF1 | Switches | Fragmentations | Matches | False negatives | Fragmented 20–35 m | Verdict on its labelled clauses                                                             |
| ---------- | ---------------------- | -----: | -----: | -------: | -------------: | ------: | --------------: | -----------------: | ------------------------------------------------------------------------------------------- |
|            | `a0`                   | 0.4284 | 0.4728 |       90 |             46 |   2,207 |             265 |      33/510 (6.5%) |                                                                                             |
| l4clus0008 | `join`                 | 0.4307 | 0.4804 |       89 |             42 |   2,210 |             262 |      29/510 (5.7%) | Fails: −0.8 points of the 5 required; merges 0, retention level, HOTA, IDF1 and misses held |
| l5id0013   | `body-older`           | 0.4311 | 0.4671 |        9 |             38 |   2,211 |             261 |                    | Fails: IDF1 0.4671 against `a0`'s 0.4728 less 0.005 (0.4678); every other clause holds      |
| l5id0013   | `body-older-tentative` | 0.4374 | 0.4961 |        5 |             34 |   2,221 |             251 |                    | Passes the tuning clauses; against `body-older`, HOTA +0.0063, IDF1 +0.0290, switches −4    |

The join's bar asks for 5 points where the defaults now hold 6.5 %, the same outdated bar the
review found on the radius graph; whether to reset it is the owner's. Noise screen beside them:
`join` 11 brief tracks and 0.633 spurious clusters a frame against `a0`'s 12 and 0.695;
`body-older` 29 and 0.925; `body-older-tentative` 18 and 0.818, as #929 found. A tuning pass is
not acceptance: l5id0013's labelled acceptance step still needs label0001's held-out window.

## l3bg0030 on kirk0

Both arms carry `settling_honest`; `ratio` adds `settling_relative_0075`, new in this run.
The corpus clauses (18 of the 22 second segments before the ceiling, none at the first measured
check, the downstream spurious-cluster read) need the S2 captures; kirk0 answers clause 2.

| Arm       | Settles at     |   HOTA |   IDF1 | Switches | Fragmentations |
| --------- | -------------- | -----: | -----: | -------: | -------------: |
| `ceiling` | 30 s (ceiling) | 0.3921 | 0.4363 |       91 |             29 |
| `ratio`   | 30 s (ceiling) | 0.3921 | 0.4363 |       91 |             29 |

The ratio of spread delta to mean spread at each measured check from 6.9 s to 29.9 s runs from
0.121 to 0.076, median 0.104, with the mean spread rising from 0.043 to 0.075 m: kirk0 is a quiet
residential scene whose cells are tight, so a small delta is a large share of them. Clause 2 holds
(HOTA equal to the ceiling's); kirk1 was not read. The l3bg0022 corpus medians ran from 0.047 to
0.084, so kirk0 sits above every corpus site, which the held-out read should weigh: the ratio may
leave the quietest scenes waiting out the ceiling.

## figure0002: track speed against the physical references

**The rule** (committed before any speed was read): for each of kirk0's three referenced vehicles,
the anchor kind with the most reviewed, independent keyframes, differenced over its span, is the
reference speed, with the sum of the two position bounds over the span as its bound; the matched
track's mean row speed and its displacement speed over the span are compared with it. A
measurement; it decides no default.

| Vehicle                 | Anchor, keyframes, span | Reference (m/s) | Row speed, as it was | Row speed, fixed (the filter's) | Displacement speed |
| ----------------------- | ----------------------- | --------------: | -------------------: | ------------------------------: | -----------------: |
| `obj_35c661db`, truck 1 | rear face, 9, 3.0 s     |     6.48 ± 0.55 |         2.49 (−3.98) |                    5.71 (−0.77) |       6.39 (−0.09) |
| `obj_8c728991`, car 8   | body centre, 7, 1.8 s   |     4.50 ± 0.43 |         3.26 (−1.24) |                    4.21 (−0.29) |       4.28 (−0.22) |
| `obj_8e829a06`, truck 2 | front face, 6, 1.8 s    |     7.15 ± 0.40 |         3.53 (−3.62) |                    6.29 (−0.86) |       6.73 (−0.42) |

Errors in brackets; within the bound only where under the reference's ± figure. `a0` and
`body-older-tentative` match the same three tracks and agree to 0.01 m/s, except truck 2's row
speed under the identity arm (2.88 as it was, 6.28 fixed).

**What the as-was column held.** `trackObservation` in `internal/lidar/pipeline` wrote
`track.AvgSpeedMps`, the running mean since the track's birth, while its comment says the row
records per-frame values, not running averages, and while the same row carries the instant
velocity. A track born at rest reads a fraction of its speed for most of its pass. The fix, in this
run, writes `track.Speed()`, the magnitude of the row's own velocity; a replay at `d71b7b7`
confirms the rows then read the "fixed" column above and that tracking is unchanged (HOTA 0.4284,
90 switches, 46 fragmentations). Rows already in a database keep the old meaning.

**What read it.** The lane field (`l8analytics.BuildLaneField`, #919): each vehicle's lifetime
maximum (its 2 m/s vehicle filter), each cell's mean speed and the heading gate. The web app's live
scene (`liveSceneSource.ts`): the speed shown and the peak kept. The evidence scripts:
`lidar_noise_screen.py`'s near-static rule (p95 of the row speed under 0.5 m/s), whose kirk0 count
falls from 6 to 3 with the instant speed, so the November gates' near-static counts, read from
evidence databases, rested on the running mean; and `lidar_track_gate.py`. `lidar-box-eval`'s rows
compute the instant speed themselves, so l5id0012's speed clause is unaffected; the scene export's
published p85 and p98 read each track's `MaxSpeedMps`, the maximum of the instant speed, and are
unaffected.

**What it moves in a shipped figure.** kirk0's lane field built from each replay's observations
(`l8analytics.BuildLaneField`, whole capture, default options), before and after the fix:

| Lane field, vehicle group | Running mean (as it was) | Instant speed (fixed) |
| ------------------------- | -----------------------: | --------------------: |
| Tracks in the field       |                       30 |                    33 |
| Cells                     |                    2,227 |                 2,728 |
| Track-cells               |                      740 |                   916 |
| Heading votes             |                      636 |                   779 |
| Median cell speed         |                 3.25 m/s |              4.13 m/s |

Three vehicles the 2 m/s lifetime-maximum filter refused on the running mean are admitted, a fifth
more cells reach the field and the heading gate, and the median cell speed rises 27 %. The lane
field reports and the lanes proposed from them so far, such as
[Columbus at Broadway](lane-field-columbus-broadway-2026-11.md), were built on the running mean.

**What the filter's speed says.** Fixed, the car's speed is within its bound; the trucks' are 12 %
low over spans where their positions move at the reference speed, so the filter lags them, as an
accelerating long body pulled by a moving cluster centre would. A track's maximum speed, which the
published percentiles read, can only be as fast as that filter. Three slow vehicles of one capture
say nothing about a population's p85; the radar capture the review recommends is the check that
would.

## l5est0003: the filter's velocity on kirk0's trucks

**The rule** (committed before the replays): an arm passes when both trucks' mean filter speed over
their spans is within its reference bound and the car's stays within; non-inferior when the
bootstrap's 5th percentile of the HOTA and IDF1 deltas is above −0.02, the switches' 95th
percentile under +10, and brief tracks rise by no more than 3. Against `a0` at `d71b7b7` (the
observation speed fixed):

| Arm      | `process_noise_vel` | Truck 1 (6.48 ± 0.55) | Car 8 (4.50 ± 0.43) | Truck 2 (7.15 ± 0.40) |   HOTA |   IDF1 | Switches | Fragmentations | Brief tracks | Max-speed p85, p98 (m/s) |
| -------- | ------------------: | --------------------: | ------------------: | --------------------: | -----: | -----: | -------: | -------------: | -----------: | -----------------------: |
| `a0`     |                 0.2 |          5.71 (−0.77) |        4.21 (−0.29) |          6.29 (−0.86) | 0.4284 | 0.4728 |       90 |             46 |           12 |               7.06, 8.83 |
| `vel-x2` |                 0.4 |          5.88 (−0.60) |        4.22 (−0.28) |          6.41 (−0.74) | 0.4319 | 0.4825 |       43 |             36 |           11 |               7.15, 8.96 |
| `vel-x4` |                 0.8 |          6.01 (−0.47) |        4.22 (−0.28) |          6.48 (−0.67) | 0.4325 | 0.4896 |       24 |             35 |           11 |               7.09, 8.96 |

| Paired bootstrap against `a0` | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches      | Δ fragmentations |
| ----------------------------- | --------------------------- | --------------------------- | --------------- | ---------------- |
| `vel-x2`                      | +0.0002 / +0.0092 / +0.0216 | +0.0001 / +0.0194 / +0.0418 | −135 / −45 / +6 | −34 / −10 / +6   |
| `vel-x4`                      | −0.0011 / +0.0091 / +0.0216 | +0.0002 / +0.0270 / +0.0604 | −188 / −64 / +4 | −38 / −11 / +3   |

**Against the rule.** Neither arm passes the target: truck 1 comes within its bound at ×4, truck 2
stays 0.67 m/s low against 0.40. Both meet the non-inferiority clauses. The filter's lag on a long
accelerating body is not mostly its velocity process noise.

**What it found instead.** Doubling the velocity process noise halves kirk0's identity switches
and lifts HOTA and IDF1 by gains that keep their sign over the bootstrap, the only labelled
identity gain this run besides the overlap end's switches and the join's IDF1. The moving count
rises from 38 to 41 and the maximum-speed percentiles by 0.03 to 0.14 m/s. It was found by
looking, on the tuning capture, so it is proposed as
[l5est0004](../../../data/experiments/l5est/l5est0004.md) with its corpus and held-out rule written
now, not adopted.

## What this run does not establish

- Anything held out. kirk0 is a tuning split; every figure here is a screen, and the bootstrap's
  intervals are over the objects a reviewer chose on one capture.
- That the reconstructed pack is the published one. The returns are, and every published kirk0
  figure checked was reproduced; the coordinates are not, and a measure that matches points by
  exact coordinates (retention) needed its floor lowered.
- Anything on the corpus. l5id0014's site clauses, l3bg0030's held-out segments, l5id0016's
  rescoring, l6cls0004 and figure0001 need the S2 captures or the archived rows, which a cloud
  container cannot reach. Their scorers were built in this run and smoke-run on kirk0 only.
- kirk1, whose capture is on the LiDAR volume.
- Pi cost or live behaviour.

## Provenance

| Item           | Value                                                                                                                                                                                                                                                   |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base           | main `9f13d2a`                                                                                                                                                                                                                                          |
| Tools          | Built with the Makefile's LDFLAGS and the `pcap` tag: replays of `a0`, `overlap-end`, `radius-graph`, `nocap`, `nocap-tentative` at `9f13d2a`; `birth-gate`, `join`, `body-older*` at `b4f3d48`; settling arms at `6df9e1d`; the evaluator at `9202b4d` |
| Parameter sets | `tuning.defaults.json` `9ad7377f…`; `l4-cap100k.json` `b787ef31…`; `motion-fragment-join.json` `2e604040…`; `identity-body-older.json` `d4c0e4fc…`; `identity-body-older-tentative.json` `3571e018…`                                                    |
| Replay         | `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024`, evidence per case, every arm repeat-verified                                                                                                    |
| Capture        | kirk0 `sha256:2864ebde…`, by Git LFS                                                                                                                                                                                                                    |
| Labels         | label set `kirk0-reference` revision 2233 (state `12b47778…`), pack `ds_67842cb1b22b70e7` reconstructed (pack digest `f0ed3414…`), from dataset `banshee-data/sf-street-speeds` by `lidar-pack-rebuild`                                                 |
| Split          | drafted, `kirk0-tuning`, one episode, samples 61 to 831, `obj_788622b5` ignored, manifest `7194bb61…`                                                                                                                                                   |
| Host           | Claude Code cloud container, 4 cores, 15 GB; raw output in its scratch, not kept                                                                                                                                                                        |

## Recommendations

1. **Use the reconstructed pack for every cloud kirk0 score**, with the recipe in the
   [run-experiments skill](../../../.claude/skills/run-experiments/SKILL.md), and say so in the
   report. Find why the decoded points differ from the published ones before relying on a measure
   that matches by coordinates.
2. **Write kirk0 bounds against the bootstrap interval.** On kirk0 a HOTA or IDF1 delta under about 0.01 at the median
   cannot be told from the sample; a rule that needs one should be read on label0001's window, and
   every kirk0 clause should name the bootstrap interval it is judged against.
3. **Make tentative priority part of any cap change** (l4clus0009), and give the cap's question
   to perf0001's Pi timing.
4. **Read a joining rule against an identity-only control**, or hold the tracker fixed: the far
   band's fragmentation moves with tentative priority alone.
5. **Do not schedule the birth gate's corpus read for its own sake.** On kirk0 it changes nothing;
   run it beside l5id0016's rescoring, which says whether the corpus's brief tracks are the kind
   it can remove.
6. **Review the observation speed fix before it merges**, and decide whether rows already in
   deployed databases are rewritten from their velocity: the lane field and the live view read
   them as the instant speed.
7. **Run l5est0004 on the corpus next** to the overlap end's rescoring: a one-key change that
   halves kirk0's switches is cheaper to adopt than any identity option, if the corpus agrees.
8. **The owner decides two bars**: l4clus0008's and the radius graph's five points at 20 to 35 m,
   where the defaults hold 6.5 % on this reconstruction.
