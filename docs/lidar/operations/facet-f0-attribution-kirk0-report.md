# Facet F0 on kirk0: near-face attribution report

<!-- ignore-style-length -->

The first run of the [F0 attribution protocol](facet-f0-attribution-kirk0.md) on kirk0's reviewed
pack: four arms replayed from one build, scored against 3,151 labelled instants of 20 road users,
stratified and joined to face entries. Every number here is a tuning score on the capture the
shadow work was developed on.

- **Status:** Complete, 2026-10-08. Tuning split only; Step 6 not run (the pack has no physical
  references), so the attribution gate is bounded, not closed
- **Layers:** L4 geometric evidence, L5 estimation, L8 analytics, offline evaluation
- **Related:** [F0 protocol](facet-f0-attribution-kirk0.md), [facet registration plan](../../plans/lidar-facet-registration-experiment-plan.md), [near-edge tracked state](../../plans/lidar-near-edge-tracked-state-plan.md) (F9), [near-face evaluation](near-face-evaluation.md), [per-frame evaluation](per-frame-evaluation.md), [October campaign](near-edge-campaign-2026-10.md)
- **Code:** [lidar-near-face-eval](../../../cmd/tools/lidar-near-face-eval/main.go), [lidar-ground-truth-eval](../../../cmd/tools/lidar-ground-truth-eval/main.go), [lidar-annotation-split-draft](../../../cmd/tools/lidar-annotation-split-draft/main.go), [lidar-state-estimation-baseline](../../../cmd/tools/lidar-state-estimation-baseline/main.go)

## Summary

The believed body's visible faces sit well inside the returns a person labelled on them. Over all
scored instants the control's end face is a median 0.85 m short of the labelled surface (mean
absolute 1.62 m, p95 5.07 m) and its side face 0.50 m short (p95 3.43 m). That is not one error
spread evenly: two slow trucks at about 38 m supply half the scored instants and the largest
residuals, and for the ten scored cars the end face is a median 0.30 m short (p95 1.58 m).

A post-hoc check, not one of the protocol's readings, says what most of it is. At the scored
instants the believed length is shorter than the labelled returns' own span along the believed
heading in 83 % of car instants and 97 % of truck instants. A box shorter than the span leaves
half the shortfall outside it wherever its centre sits. Removing that forced part takes the
end-face median from −0.30 m to −0.02 m for cars and from −2.14 m to −0.02 m for trucks. On this
capture the along-path error where an end is visible is mostly an extent that is too short, and
the extents are "evidence" extents, accumulated from what the sensor saw.

The predeclared readings, in the protocol's order:

| Question                                 | Result                                                                                                     | Reading                                                                                                                                                      |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Does the extent prior own the end error? | Prior 1.20 m mean absolute (43 instants, 4 objects) against evidence 1.65 m (630, 16): ratio 0.73          | Under 1.25, so by the rule edge localisation is in play and arm D keeps its case. The rule's premise fails here; see [Section 4.1](#41-the-attribution-gate) |
| Is the end tangent a real error?         | Control end-tangent p95 absolute 2.03 m (280 instants); cars alone 1.10 m                                  | Over 0.3 m: the error a rank-one fix leaves is material                                                                                                      |
| Is the lateral-entry tail accuracy?      | End tangent worse at entry in all four arms (+0.16 to +0.62 m); interval excludes zero only for control    | Not settled. No arm is worse by more than its interval width                                                                                                 |
| Where does the sparse tail begin here?   | End exposure 53 % and 59 % below 50 m, 15 % at 50 to 80 m; 38 % of masks there under 8 returns             | Onset in the 50 to 80 m bin, consistent with the review's 60 to 80 m. The loss is fallback and non-association, not unseen ends                              |
| Is the synthetic sampling model usable?  | `returns / N_pred` median 1.71 below 20 m (337 instants, 10 cars), 0.31 at 20 to 50 m (13 instants, 1 car) | Falling with range, a factor of 5.5: the generator needs a dropout model before E1 trusts it at range. One car carries the far bin; this reading is weak     |
| Does T5 pull centres toward the sensor?  | `track_t5` minus `track`: end −0.059 m [−0.126, 0.000], side +0.009 m [−0.013, +0.033]                     | No shift beyond the interval: T5 is harmless on labelled faces and still not worth keeping                                                                   |
| Does A2's gate protect identity?         | `track_a1` minus `track`: ID switches +99 (100 to 199), fragmentations +5, IDF1 −0.053                     | More switches and fragmentations under A1 confirm the campaign's reading on labels                                                                           |
| Does the tracked arm beat the shadow?    | `track` minus control end-normal mean absolute −0.130 m [−0.219, −0.017]; side −0.041 m [−0.105, +0.030]   | A paired end-face improvement with an interval excluding zero: the first label-backed evidence for A2. No sparse-tail regression. Identity costs beside it   |

What this does not establish: anything held out, anything about the far side of a body or the
along-path error where no end is visible (Step 6 had no physical references to score), and
anything about a body's true length beyond the lower bound the labelled span gives.

## 1. Question

The facet plan's attribution gate asks how much of the along-path error is edge localisation and
how much is the extent prior. The maths review predicted the prior dominates and that patch
density cannot move it. This run measures, on labels, the three quantities the gate needs where a
face is visible (end normal, side normal, end tangent), stratified by extent provenance, range,
returns and aspect, and joins them to face entries to ask whether the October campaign's
lateral-face-entry tail is an accuracy error. Definitions of the residuals are in
[near-face evaluation](near-face-evaluation.md): a negative normal residual means the believed
face is short of the labelled returns, on the far side of them from the sensor.

## 2. What was run

| Arm        | `-experiment` beyond `BASE`                  | What it is                                                           |
| ---------- | -------------------------------------------- | -------------------------------------------------------------------- |
| `control`  | none                                         | The shadow body: T1 with T3 and full members, the facet plan's arm A |
| `track`    | `near_edge_track`                            | A2, the tracked near-edge candidate, default-off                     |
| `track_t5` | `near_edge_track,solid_body_rank_one_medoid` | The rank-one medoid remedy                                           |
| `track_a1` | `near_edge_track,near_edge_track_a1`         | Medoid gating                                                        |

`BASE=solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members`, with
`-warmup 20 -sample-points 256` on kirk0 as a one-case corpus, exactly as the protocol's Step 1.
Steps 2 to 5 ran as written; Step 6 did not (no `physical-references.json`). Each replay took 18
to 19 s on the Mac rather than the minutes budgeted, and each scorer ran in seconds.

**Determinism.** All four arms wrote `baseline_equal: true` (first and repeat runs byte-identical,
631 frames each) from one source manifest. The protocol also asked that "the default replay
matched the committed kirk0 baseline". There is no such check to make: an experiment run has no
default replay, and the committed kirk0 artefacts are a timing baseline
(`internal/lidar/perf/baseline/`) and a coverage declaration, not an output baseline.

**Deviations and definitions the protocol left open.**

- The pack was scored from a local copy with byte-identical files, because the sidecar is live
  (revision 1974 was written the evening before). The split pins that revision.
- The evaluator writes scored instants only. For the exposure table the analysis script
  re-implements its matching (same footprint gate, frame tolerance, nearest-first one-to-one
  assignment) and was accepted only because it reproduces, for all four arms, the evaluator's
  matched, scored and unscored counts and every scored instant's track key. Exposure bins use the
  mask's footprint-centre range; residual strata use the instant's body-centre `range_m`.
- The end tangent's sign follows the heading, so its mean and median are taken over absolute
  values.
- A paired stratum takes its membership from the reference arm's instant.
- Face entries: a "frame" is the next solid-body row of the same track. A track's first row is
  not a change. `entered_lateral` is a row whose `visible_faces` gains `left` or `right` over the
  previous row; `entered_any` is any change of the face set. The entry groups are the entry row
  and the two after; stable is three or more rows since the later of the track's start and its
  last change. Medoid rows carry an empty face set and take part in change detection.
- "Worse by more than its interval width" is read two ways in Section 4.3, because the protocol
  does not say whether it means the full width or the half-width.
- The evaluator recorded no `too_few_returns` instants: masks under 8 returns are almost never
  matched. The tail-onset reading therefore reports the share of labelled masks under 8 returns
  beside the evaluator's zero.
- The tools were built with `go build` and an explicit `GitSHA` stamp, as the protocol writes, not
  through `make`; `Version` and `BuildTime` are unstamped in these four binaries.

## 3. Inputs and coverage

The split keeps 20 objects and 3,151 scored masks over samples 62 to 831 (6.2 s onward). It left
out four noise objects, one ground object, and one car with no mask in the window. Arms match
between 1,223 and 1,256 of the masks to a body; the control scores faces at 781.

| Class      | Labelled | Unmatched | Matched | Medoid-referenced | Face-scored |
| ---------- | -------: | --------: | ------: | ----------------: | ----------: |
| car        |     1914 |      1367 |     547 |               197 |         350 |
| truck      |      903 |       272 |     631 |               234 |         397 |
| pedestrian |      282 |       226 |      56 |                38 |          18 |
| bus        |       52 |        30 |      22 |                 6 |          16 |

Two facts shape every table below.

- **A parked car is a quarter of the labels.** `obj_788622b5` has 770 masks at a median 10 m and
  moved 1.3 m over 77 s. A stationary vehicle settles into the background and is not tracked, so
  almost all of its masks are unmatched; it contributes 43 scored instants.
- **Two trucks are half the scores.** `obj_35c661db` and `obj_8e829a06` pass at about 38 m and
  2.8 m/s and give 397 of the control's 781 scored instants, with the largest residuals. Any
  all-instant figure is close to a truck figure.

| Object         | Class      | Masks | Scored (control) | Median range (m) | Net displacement (m) | Mean speed (m/s) | Median returns |
| -------------- | ---------- | ----: | ---------------: | ---------------: | -------------------: | ---------------: | -------------: |
| `obj_788622b5` | car        |   770 |               43 |             10.1 |                  1.3 |             0.02 |             24 |
| `obj_35c661db` | truck      |   455 |              215 |             39.5 |                125.8 |             2.77 |             84 |
| `obj_8e829a06` | truck      |   412 |              182 |             37.1 |                116.0 |             2.82 |             52 |
| `obj_23e8160b` | car        |   207 |               18 |             34.1 |                102.5 |             4.49 |             15 |
| `obj_4929e754` | car        |   186 |               59 |             25.0 |                 90.8 |             4.91 |             92 |
| `obj_ad6f69ef` | car        |   177 |               41 |             17.2 |                 91.5 |             4.77 |            332 |
| `obj_835d059d` | car        |   146 |                3 |             10.8 |                 22.4 |             1.55 |            376 |
| `obj_a99f83f4` | pedestrian |   142 |                0 |             20.5 |                 15.1 |             0.99 |              9 |
| `obj_14ed7858` | car        |   119 |               75 |              5.9 |                 22.9 |             1.94 |            920 |
| `obj_8529ec5b` | car        |   109 |               55 |              7.3 |                 23.2 |             2.15 |           1164 |
| `obj_99987c9d` | pedestrian |    63 |                8 |             35.2 |                  8.8 |             1.41 |             12 |
| `obj_8c728991` | car        |    59 |               18 |              8.5 |                 25.9 |             4.47 |            373 |
| `obj_764efbef` | car        |    56 |               17 |             12.5 |                 30.4 |             5.53 |            270 |
| `obj_c3e7169b` | car        |    54 |               21 |             12.1 |                 31.3 |             5.91 |            710 |
| `obj_f6429947` | bus        |    52 |               16 |             32.9 |                 18.7 |             3.66 |            280 |
| `obj_7eb38c9c` | pedestrian |    40 |                6 |              8.9 |                  3.9 |             0.66 |             11 |
| `obj_bca3b9bb` | pedestrian |    37 |                4 |             20.6 |                  2.8 |             0.77 |             10 |
| `obj_53def904` | truck      |    36 |                0 |             28.8 |                 17.9 |             5.12 |             96 |
| `obj_4116ba22` | car        |    25 |                0 |             21.4 |                  3.3 |             1.36 |             49 |
| `obj_581a0d13` | car        |     6 |                0 |             11.3 |                  1.7 |             3.35 |            210 |

Speed is net displacement over the masked interval, a lower bound for anything that turned.

The frame check passed in every arm: the median distance from a body centre to its mask's
footprint centre was 0.69 to 0.79 m, against the protocol's "about a metre".

## 4. Results

### 4.1 The attribution gate

**Predeclared reading.** The control's end-normal mean absolute is 1.20 m where an extent is a
class prior (43 instants, 4 objects) and 1.65 m where both extents are evidence (630 instants, 16
objects): a ratio of 0.73, under the protocol's 1.25. By the rule, edge localisation is in play
and arm D keeps its case.

**Why that reading should not be acted on.** The rule assumes an evidence extent is close to the
object's real extent, so that what remains in the evidence stratum is where the edge was placed.
On kirk0 that assumption fails, and the prior stratum is not comparable either.

- The prior stratum is 38 truck instants (19 from each truck), 3 car and 3 pedestrian, from four
  objects. The evidence stratum is 359 truck, 347 car, 16 bus and 15 pedestrian instants. The
  ratio compares the trucks' prior frames against a mixture.
- The evidence extents are short. The labelled returns' span along the believed heading is a lower
  bound on the object's length, given the heading. The believed length falls short of it in 83 % of
  car instants, 97 % of truck instants and every bus instant:

| Class      | Instants | Believed length median [IQR] | Labelled span along heading median [IQR] | Length short of span | Believed width median | Span across heading median | Width short of span |
| ---------- | -------: | ---------------------------- | ---------------------------------------- | -------------------: | --------------------: | -------------------------: | ------------------: |
| car        |      350 | 3.62 [3.12–4.12]             | 4.22 [3.57–4.60]                         |                  83% |                  1.62 |                       1.93 |                 83% |
| truck      |      397 | 3.62 [0.88–3.88]             | 8.83 [5.42–9.78]                         |                  97% |                  2.38 |                       3.68 |                 93% |
| bus        |       16 | 4.00 [2.56–6.12]             | 8.76 [5.35–11.19]                        |                 100% |                  1.88 |                       2.92 |                100% |
| pedestrian |       18 | 0.38 [0.20–0.38]             | 0.43 [0.33–0.49]                         |                  39% |                  0.20 |                       0.34 |                 67% |

A box shorter than the labelled span cannot contain it: half the shortfall is outside it at one
end or the other, wherever the centre sits. Adding that forced half back to each residual leaves
what placement contributes:

| Class      | End normal n · median · mean abs | Half length shortfall median · mean | End normal plus shortfall median · mean abs · p95 | Side normal n · median · mean abs | Half width shortfall median | Side normal plus shortfall median · mean abs · p95 |
| ---------- | -------------------------------- | ----------------------------------- | ------------------------------------------------- | --------------------------------- | --------------------------- | -------------------------------------------------- |
| car        | 271 · −0.30 · 0.62               | 0.25 · 0.43                         | −0.02 · 0.42 · 1.36                               | 350 · −0.38 · 0.54                | 0.18                        | −0.10 · 0.32 · 1.14                                |
| truck      | 376 · −2.14 · 2.41               | 2.39 · 2.27                         | −0.02 · 1.25 · 3.08                               | 396 · −0.93 · 1.51                | 0.94                        | −0.12 · 0.89 · 2.39                                |
| bus        | 9 · −0.69 · 0.71                 | 1.55 · 2.06                         | +1.08 · 1.49 · 3.01                               | 16 · −0.17 · 0.56                 | 0.52                        | +1.40 · 1.43 · 2.68                                |
| pedestrian | 17 · +0.08 · 0.54                | 0.00 · 0.06                         | +0.09 · 0.51 · 2.17                               | 18 · −0.03 · 0.03                 | 0.01                        | −0.01 · 0.06 · 0.14                                |

For cars and trucks the systematic part of the end residual is the shortfall: the medians fall to
−0.02 m once it is removed. The bus over-corrects (+1.08 m), so its centre or heading is off as
well as its length; with one bus that is not read further. What remains for cars, a
mean absolute of 0.42 m on the end and 0.32 m on the side, is placement of a box whose extent is
fixed: centre, heading, and the partial view.

**What this means for the gate.** The predeclared measure returns "arm D keeps its case", but the
error it attributes to localisation is, on kirk0, mostly an extent the estimator believes too
short, prior or not. This is post hoc and on one capture, so it is a reason to restate the gate,
not a reading of it: separate extent shortfall from placement in the measure itself, predeclare
it, and score it on a capture that is not kirk0 or against physical references. Until then the
evidence does not favour funding arm D, and it points at the extent belief (accumulation from
partial views, and extent memory) as the larger term.

### 4.2 Residuals by stratum

The control's full strata are in the [appendix](#appendix-control-strata). The selected strata
for every arm:

| Arm      | Stratum          | End normal n · median · mean abs · p95 | Side normal n · median · mean abs · p95 | End tangent n · mean abs · p95 |
| -------- | ---------------- | -------------------------------------- | --------------------------------------- | ------------------------------ |
| control  | All              | 673 · −0.85 · 1.62 · 5.07              | 780 · −0.50 · 1.02 · 3.43               | 280 · 0.66 · 2.03              |
| control  | Extent: prior    | 43 · −0.89 · 1.20 · 2.80               | 44 · −0.63 · 0.94 · 2.52                | 18 · 1.09 · 1.80               |
| control  | Extent: evidence | 630 · −0.84 · 1.65 · 5.08              | 736 · −0.49 · 1.03 · 3.45               | 262 · 0.63 · 2.02              |
| control  | Range < 20 m     | 407 · −0.48 · 1.30 · 4.24              | 507 · −0.40 · 0.66 · 2.10               | 212 · 0.45 · 1.56              |
| control  | Range 20–50 m    | 257 · −1.30 · 2.16 · 5.13              | 264 · −1.57 · 1.74 · 4.23               | 68 · 1.32 · 4.09               |
| control  | Range 50–80 m    | 9 · −0.81 · 0.61 · 0.84                | 9 · +0.04 · 0.10 · 0.24                 | 0                              |
| control  | Sparse tail      | 84 · −0.79 · 1.24 · 3.70               | 83 · −0.18 · 1.01 · 4.19                | 20 · 0.77 · 1.81               |
| track    | All              | 622 · −0.68 · 1.51 · 4.91              | 679 · −0.46 · 0.95 · 3.28               | 273 · 0.58 · 1.41              |
| track    | Extent: prior    | 42 · −0.96 · 1.24 · 3.02               | 47 · −0.64 · 0.98 · 2.22                | 18 · 1.19 · 2.65               |
| track    | Extent: evidence | 580 · −0.63 · 1.53 · 4.93              | 632 · −0.45 · 0.95 · 3.39               | 255 · 0.54 · 1.31              |
| track    | Range < 20 m     | 390 · −0.33 · 1.20 · 4.24              | 442 · −0.42 · 0.70 · 2.24               | 224 · 0.45 · 1.24              |
| track    | Range 20–50 m    | 225 · −1.30 · 2.08 · 4.96              | 230 · −0.95 · 1.46 · 3.51               | 49 · 1.14 · 4.38               |
| track    | Range 50–80 m    | 7 · −1.06 · 0.72 · 1.12                | 7 · −0.05 · 0.13 · 0.24                 | 0                              |
| track    | Sparse tail      | 80 · −0.71 · 1.28 · 3.65               | 80 · −0.15 · 0.66 · 2.97                | 17 · 0.83 · 2.65               |
| track_t5 | All              | 610 · −0.73 · 1.58 · 4.97              | 653 · −0.44 · 0.95 · 3.41               | 271 · 0.50 · 1.43              |
| track_t5 | Extent: prior    | 35 · −1.08 · 1.46 · 3.09               | 35 · −0.47 · 0.70 · 2.19                | 13 · 1.10 · 2.53               |
| track_t5 | Extent: evidence | 575 · −0.65 · 1.59 · 4.98              | 618 · −0.44 · 0.96 · 3.42               | 258 · 0.47 · 1.36              |
| track_t5 | Range < 20 m     | 379 · −0.34 · 1.26 · 4.20              | 422 · −0.41 · 0.68 · 2.22               | 215 · 0.38 · 1.15              |
| track_t5 | Range 20–50 m    | 224 · −1.33 · 2.15 · 5.01              | 224 · −1.09 · 1.49 · 3.48               | 56 · 0.93 · 3.29               |
| track_t5 | Range 50–80 m    | 7 · −1.06 · 0.72 · 1.12                | 7 · −0.05 · 0.14 · 0.24                 | 0                              |
| track_t5 | Sparse tail      | 107 · −0.61 · 1.10 · 3.69              | 99 · −0.19 · 0.97 · 3.74                | 29 · 0.59 · 2.44               |
| track_a1 | All              | 482 · −0.71 · 1.69 · 5.75              | 540 · −0.41 · 0.89 · 2.97               | 232 · 0.71 · 2.53              |
| track_a1 | Extent: prior    | 41 · −0.90 · 1.25 · 3.03               | 46 · −0.66 · 0.98 · 2.24                | 18 · 1.19 · 2.65               |
| track_a1 | Extent: evidence | 441 · −0.70 · 1.73 · 5.83              | 494 · −0.40 · 0.89 · 3.00               | 214 · 0.67 · 2.46              |
| track_a1 | Range < 20 m     | 315 · −0.40 · 1.37 · 5.12              | 366 · −0.37 · 0.63 · 2.45               | 182 · 0.50 · 1.39              |
| track_a1 | Range 20–50 m    | 160 · −1.30 · 2.36 · 5.98              | 167 · −0.62 · 1.51 · 6.10               | 50 · 1.46 · 5.62               |
| track_a1 | Range 50–80 m    | 7 · −1.06 · 0.72 · 1.12                | 7 · −0.05 · 0.13 · 0.24                 | 0                              |
| track_a1 | Sparse tail      | 98 · −0.88 · 2.32 · 6.00               | 92 · −0.19 · 0.90 · 3.09                | 24 · 0.83 · 2.61               |

Nothing was scored at 80 m and beyond, and the 50 to 80 m row is one truck. The 20 to 50 m rows
are the two trucks, the bus, one car and one pedestrian, and 227 of the control's 264 instants
there are the trucks'. By class (post hoc), the control's cars read 0.62 m mean
absolute on the end normal (p95 1.58 m), 0.54 m on the side (p95 1.52 m) and 0.32 m on the end
tangent (p95 1.10 m); its trucks 2.41, 1.51 and 1.02 m. Pedestrians' side residuals are 3 cm.

**End tangent.** The control's end-tangent p95 is 2.03 m over 280 instants, and 1.10 m for cars
alone. Both are over the protocol's 0.3 m, so the error a rank-one fix leaves along the end face
is material. Its prior stratum (18 instants) has a median of 1.25 m.

### 4.3 The lateral-entry tail

| Arm      | Group         | Instants (objects) | End normal n · mean abs · p95 | Side normal n · mean abs · p95 | End tangent n · mean abs · p95 | Rank 0/1/2 |
| -------- | ------------- | ------------------ | ----------------------------- | ------------------------------ | ------------------------------ | ---------- |
| control  | lateral entry | 164 (16)           | 122 · 1.52 · 4.88             | 163 · 1.01 · 4.10              | 39 · 1.06 · 5.54               | 6/55/103   |
| control  | any change    | 336 (16)           | 265 · 1.63 · 5.03             | 335 · 1.20 · 4.16              | 73 · 1.31 · 4.57               | 54/130/152 |
| control  | stable        | 445 (14)           | 408 · 1.62 · 5.09             | 445 · 0.89 · 3.26              | 207 · 0.43 · 1.35              | 0/72/373   |
| track    | lateral entry | 155 (16)           | 135 · 1.06 · 3.72             | 155 · 0.82 · 2.92              | 59 · 0.84 · 2.19               | 9/46/100   |
| track    | any change    | 262 (16)           | 228 · 1.44 · 4.85             | 262 · 0.97 · 2.89              | 68 · 0.84 · 2.67               | 32/102/128 |
| track    | stable        | 417 (15)           | 394 · 1.56 · 4.96             | 417 · 0.94 · 3.46              | 205 · 0.49 · 1.36              | 0/78/339   |
| track_t5 | lateral entry | 184 (16)           | 163 · 1.08 · 3.99             | 184 · 0.76 · 2.92              | 79 · 0.64 · 1.44               | 10/39/135  |
| track_t5 | any change    | 282 (16)           | 251 · 1.33 · 4.90             | 280 · 0.91 · 2.96              | 90 · 0.64 · 2.21               | 34/92/156  |
| track_t5 | stable        | 379 (15)           | 359 · 1.76 · 5.01             | 373 · 0.98 · 3.47              | 181 · 0.43 · 1.35              | 0/79/300   |
| track_a1 | lateral entry | 229 (16)           | 202 · 2.15 · 5.92             | 224 · 1.04 · 3.15              | 88 · 0.75 · 3.69               | 8/59/162   |
| track_a1 | any change    | 310 (16)           | 268 · 2.04 · 5.91             | 305 · 1.06 · 4.99              | 105 · 0.84 · 5.03              | 31/105/174 |
| track_a1 | stable        | 236 (15)           | 214 · 1.24 · 5.12             | 235 · 0.67 · 2.36              | 127 · 0.60 · 1.50              | 0/46/190   |

Entry minus stable, in mean absolute residual, with object-resampled 95 % intervals:

| Arm      | Contrast               | End normal Δ [interval] | Side normal Δ [interval] | End tangent Δ [interval] (width) |
| -------- | ---------------------- | ----------------------- | ------------------------ | -------------------------------- |
| control  | lateral entry − stable | −0.09 [−0.68, +0.52]    | +0.12 [−0.49, +0.59]     | +0.62 [+0.02, +1.52] (1.50)      |
| control  | any change − stable    | +0.02 [−0.39, +0.36]    | +0.31 [−0.22, +0.71]     | +0.88 [+0.17, +1.52] (1.34)      |
| track    | lateral entry − stable | −0.50 [−1.22, +0.49]    | −0.12 [−0.70, +0.31]     | +0.35 [−0.13, +1.04] (1.17)      |
| track    | any change − stable    | −0.12 [−0.56, +0.45]    | +0.03 [−0.48, +0.39]     | +0.34 [−0.06, +0.94] (1.00)      |
| track_t5 | lateral entry − stable | −0.68 [−1.24, +0.49]    | −0.22 [−0.66, +0.27]     | +0.21 [−0.17, +0.95] (1.12)      |
| track_t5 | any change − stable    | −0.42 [−0.83, +0.45]    | −0.06 [−0.40, +0.29]     | +0.22 [−0.15, +0.80] (0.95)      |
| track_a1 | lateral entry − stable | +0.90 [−0.20, +1.43]    | +0.36 [+0.05, +0.58]     | +0.16 [−0.25, +0.87] (1.12)      |
| track_a1 | any change − stable    | +0.80 [−0.19, +1.26]    | +0.39 [−0.06, +0.79]     | +0.24 [−0.16, +0.72] (0.88)      |

**Reading.** The end tangent is worse at a lateral entry than in stable frames in every arm, by
0.16 to 0.62 m, and the any-change contrast agrees. Read as "the interval excludes zero", the
control's end tangent (+0.62 m, lower bound +0.02 m) and `track_a1`'s side normal (+0.36 m) meet
it; read as "worse by more than the full interval width", nothing does. With 15 or 16 objects the
intervals are about a metre wide, so the reading is not settled either way: the direction says
the tail is at least partly an accuracy error in the end tangent, and the size of the effect
cannot be pinned on this capture. The entry groups carry more rank-0 and rank-1 fixes and the
fallbacks (control any-change: 41 `no_face_reached_minimum_support`, 12 `face_hysteresis`, 1
`class_prior_extent`); stable frames carry none, so a difference is read against a different
mix of fixes as well as a face change.

### 4.4 Sparse-tail onset

Every labelled mask, binned by the range of its footprint centre, control arm:

| Range   | Labelled | Unmatched | Matched | Medoid-referenced | Face-scored | End scored | End exposure of matched | End scored of face-scored | Masks under 8 returns |
| ------- | -------: | --------: | ------: | ----------------: | ----------: | ---------: | ----------------------: | ------------------------: | --------------------- |
| < 20 m  |     1884 |      1126 |     758 |               254 |         504 |        403 |                     53% |                       80% | 41 (2%)               |
| 20–50 m |     1013 |       567 |     446 |               177 |         269 |        262 |                     59% |                       97% | 128 (13%)             |
| 50–80 m |      243 |       191 |      52 |                44 |           8 |          8 |                     15% |                      100% | 93 (38%)              |
| ≥ 80 m  |       11 |        11 |       0 |                 0 |           0 |          0 |                       – |                         – | 11 (100%)             |

**Reading.** Exposure falls under half, and the share of masks under 8 returns rises over a fifth,
in the same bin: 50 to 80 m. That is kirk0's tail onset at this bin resolution, and it agrees
with the review's 60 to 80 m. The table also says why. Where a body-centre estimate exists, an end
face is nearly always visible (97 % and 100 % above 20 m). The along-path exposure is lost earlier
in the chain: at 50 to 80 m, 191 of 243 masks match no body, and 44 of the 52 that do match a
medoid-referenced row. Below 20 m the parked car alone is 770 of the 1,884 masks, and no arm
tracks it.

### 4.5 Synthetic sampling model

Cars only, control instants, `N_pred(r, a) = rings_on_body(r) * (L sin a + W cos a) / (r * 0.2°)`
with L = 4.5 m, W = 1.8 m, the Pandar40P's 40-channel elevation table, and a 1.5 m body under a
3 m mount. No mount height is recorded for kirk0; 3 m is the protocol's default, and the pack's
height band (floor 2.8 m below the sensor) is consistent with it.

| Range   | Instants (objects) | Rings on a 1.5 m body at bin centre | `returns / N_pred` median | IQR       |
| ------- | -----------------: | ----------------------------------: | ------------------------: | --------- |
| < 20 m  |           337 (10) |                                   6 |                      1.71 | 0.97–2.15 |
| 20–50 m |             13 (1) |                                   7 |                      0.31 | 0.30–0.31 |
| 50–80 m |              0 (0) |                                   4 |                         – | –         |
| ≥ 80 m  |              0 (0) |                                   3 |                         – | –         |

**Reading.** The median falls by a factor of 5.5 from the near bin to the next, so by the rule the
generator needs a dropout model before E1 trusts it at range. The far bin is one car
(`obj_4929e754`, 13 instants) and there are no scored cars beyond 50 m, so the evidence for
"falling" is one object. The near bin is the firmer finding, and it is the opposite of a dropout:
close cars return more than the model predicts (median 1.7), which a lower mount than 3 m, taller
bodies, or the model's ring count at short range would each produce. Recording kirk0's mount
height would remove the first.

### 4.6 Paired differences and identity

Arm minus reference, mean absolute residual, on instants both scored, object-resampled 95 %
intervals (2,000 replicates, seed 20261008):

| Comparison         | Stratum          | End normal: n (objects), Δ mean abs [interval] | Side normal                       | End tangent                       |
| ------------------ | ---------------- | ---------------------------------------------- | --------------------------------- | --------------------------------- |
| track − control    | All              | 555 (16), −0.130 [−0.219, −0.017]              | 613 (16), −0.041 [−0.105, +0.030] | 204 (16), +0.045 [−0.017, +0.103] |
| track − control    | Extent: prior    | 43 (4), −0.002 [−0.315, +0.061]                | 43 (4), −0.062 [−0.305, −0.000]   | 16 (3), −0.030 [−0.099, +0.001]   |
| track − control    | Extent: evidence | 512 (16), −0.141 [−0.248, −0.007]              | 570 (16), −0.039 [−0.106, +0.036] | 188 (16), +0.051 [−0.017, +0.125] |
| track − control    | Range < 20 m     | 333 (14), −0.116 [−0.224, +0.001]              | 386 (14), −0.011 [−0.050, +0.016] | 158 (14), +0.047 [−0.022, +0.165] |
| track − control    | Range 20–50 m    | 219 (5), −0.154 [−0.228, +0.099]               | 224 (5), −0.093 [−0.352, +0.082]  | 46 (4), +0.035 [−0.169, +0.152]   |
| track − control    | Sparse tail      | 71 (6), −0.258 [−0.622, −0.008]                | 70 (6), −0.462 [−0.939, +0.013]   | 14 (4), +0.015 [−0.002, +0.023]   |
| track − control    | Not sparse tail  | 484 (14), −0.112 [−0.167, −0.015]              | 543 (14), +0.014 [−0.080, +0.071] | 190 (14), +0.047 [−0.021, +0.107] |
| track − control    | End-on < 20°     | 34 (4), −0.753 [−1.380, −0.001]                | 33 (4), −0.975 [−1.550, +0.005]   | 4 (2), +0.005 [+0.003, +0.007]    |
| track − control    | Oblique 20–69°   | 424 (15), −0.081 [−0.151, −0.009]              | 438 (15), −0.016 [−0.099, +0.026] | 169 (13), +0.039 [−0.020, +0.112] |
| track − control    | Broadside ≥ 70°  | 97 (12), −0.128 [−0.240, +0.041]               | 142 (13), +0.100 [−0.014, +0.213] | 31 (10), +0.078 [−0.031, +0.182]  |
| track_t5 − control | All              | 542 (16), −0.102 [−0.138, −0.016]              | 590 (16), −0.062 [−0.169, +0.048] | 204 (16), −0.089 [−0.205, +0.019] |
| track_t5 − control | Sparse tail      | 70 (6), −0.140 [−0.299, −0.008]                | 69 (6), −0.216 [−0.587, +0.037]   | 15 (5), −0.077 [−0.583, +0.021]   |
| track_a1 − control | All              | 434 (16), +0.193 [−0.064, +0.377]              | 494 (16), −0.080 [−0.226, +0.040] | 176 (15), +0.067 [−0.020, +0.173] |
| track_a1 − control | Range 20–50 m    | 155 (5), +0.242 [+0.074, +0.376]               | 161 (5), −0.137 [−0.624, +0.454]  | 40 (4), +0.436 [−0.151, +0.922]   |
| track_a1 − track   | All              | 427 (16), +0.332 [+0.009, +0.528]              | 480 (16), +0.014 [−0.214, +0.237] | 177 (15), +0.109 [−0.006, +0.249] |
| track_a1 − track   | Range < 20 m     | 272 (14), +0.310 [+0.008, +0.607]              | 320 (14), +0.038 [−0.044, +0.182] | 138 (13), +0.013 [−0.021, +0.060] |
| track_a1 − track   | Range 20–50 m    | 148 (5), +0.389 [+0.016, +0.530]               | 153 (5), −0.036 [−0.760, +0.790]  | 39 (4), +0.449 [+0.000, +0.848]   |

The p95 differences over all instants have intervals that include zero for every comparison
(`track` − control end normal −0.16 m [−0.22, +0.05]). One p95 regression excludes zero: `track`'s
end tangent at broadside, +0.40 m [+0.002, +0.96] over 31 instants from 10 objects.

`track_t5` minus `track`, signed, positive meaning toward the sensor:

| Stratum         | End normal n (objects), mean shift [interval] | Side normal                       |
| --------------- | --------------------------------------------- | --------------------------------- |
| All             | 596 (16), −0.059 [−0.126, +0.000]             | 636 (16), +0.009 [−0.013, +0.033] |
| Range < 20 m    | 372 (14), −0.083 [−0.234, +0.030]             | 412 (14), +0.006 [−0.054, +0.053] |
| Range 20–50 m   | 217 (5), −0.020 [−0.077, +0.050]              | 217 (5), +0.014 [−0.008, +0.033]  |
| Sparse tail     | 79 (6), +0.027 [+0.000, +0.035]               | 77 (6), +0.032 [+0.000, +0.064]   |
| Not sparse tail | 517 (14), −0.073 [−0.156, +0.000]             | 559 (14), +0.005 [−0.023, +0.036] |

Identity, per frame on the same labels (online-stage track estimates, footprint gate):

| Arm      |  MOTA | MOTP (m) | ID switches | Fragmentations |   FN |  FP |  HOTA |  AssA |  IDF1 |
| -------- | ----: | -------: | ----------: | -------------: | ---: | --: | ----: | ----: | ----: |
| control  | 0.163 |    1.221 |         100 |             47 | 1895 | 643 | 0.387 | 0.662 | 0.355 |
| track    | 0.162 |    1.245 |         100 |             50 | 1909 | 630 | 0.325 | 0.506 | 0.306 |
| track_t5 | 0.159 |    1.223 |         111 |             48 | 1905 | 635 | 0.319 | 0.487 | 0.287 |
| track_a1 | 0.127 |    1.178 |         199 |             55 | 1928 | 624 | 0.312 | 0.467 | 0.253 |

**Readings.**

- **A2 against the shadow.** `track` lowers the end-normal mean absolute by 0.130 m, with an
  interval that excludes zero, and by 0.258 m in the sparse tail. That is the first label-backed
  evidence for A2, on end-face placement. It is 8 % of a 1.62 m residual whose larger part is the
  extent (Section 4.1). The side normal does not move beyond its interval. Beside it: the
  identity cost, AssA −0.156, HOTA −0.062, IDF1 −0.050 and 3 more fragmentations, with no change
  in ID switches. No stratum shows a sparse-tail regression.
- **T5.** Over all instants T5 shifts neither face toward the sensor beyond its interval. In the
  sparse tail both shifts are positive, 3 cm, with intervals that reach zero: at the sensor's
  range-noise floor and on six objects. T5 is harmless on labelled faces. It adds 11 ID switches
  against `track`, which is a reason not to keep it.
- **A1.** Medoid gating doubles the ID switches (100 to 199), adds 5 fragmentations against
  `track`, and worsens the end normal by 0.33 m [+0.01, +0.53]. That confirms the campaign's
  reading of A1 on labels.

## 5. Limitations

- kirk0 is a tuning split, and the shadow work was developed on it. Nothing here is a held-out
  score.
- The near-face residuals see only faces the sensor sees. Where only a side is visible the
  along-path error is unmeasured, and with no physical references Step 6 could not reach it. The
  gate is bounded, not closed.
- The face is the outermost labelled return less a trimmed share. A partly seen face's extreme is
  a lower bound on where it ends, and range noise floors every residual at a few centimetres.
- Twenty objects on one capture, one placement, one day. Two trucks are half the face scores and
  one parked car is a quarter of the labels. Intervals resample objects and are wide; a stratum
  with one to six objects is reported with its count and should be read as anecdote.
- The extent diagnostic in Section 4.1 is post hoc. It rests on the believed heading: a wrong
  heading inflates the span across it, which is what the bus shows. It gives a lower bound on the
  shortfall, not the object's length.
- The return-budget comparison assumes a 3 m mount and a 1.5 m body, and its far bin is one car.
- Update cost and the label-free five-point residual are not measured here; the campaign's
  figures stand.
- Identity scores online-stage track estimates, as the protocol specified, not solid bodies.

## 6. Recommendations

1. Restate the attribution gate before it is used to fund arm D. Its measure should separate an
   extent shortfall (the believed box is shorter than the labelled span) from placement, and be
   predeclared again. On kirk0 the shortfall, not edge localisation, is the larger part of the
   end error.
2. Look at extent accumulation. Accumulated "evidence" extents are short of the visible span in
   83 % of car instants and almost all truck instants: an extent built from partial views, and a
   truck length of 3.6 m against at least 8.8 m labelled. That is the facet plan's extent-memory
   and interval-output question, and it may be cheaper than any facet arm.
3. Score the same arms against physical references when kirk0 or another reviewed pack has them;
   that is the only route to the along-path error when no end is visible.
4. Record kirk0's mount height, and repeat the return-budget comparison on a capture with cars
   beyond 50 m before E1 relies on the synthetic generator at range.
5. Treat the A2 face-placement gain as real but small, and weigh it with its identity cost, which
   this run confirms on labels.

## 7. Provenance

| Item                 | Value                                                                                                                                                                 |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Run                  | 2026-10-08, on the Mac (8 cores), Go 1.27.1 darwin/arm64                                                                                                              |
| Commit               | `e49051335cde9b5f01bb2caa793caae2191dc343` (this branch, with main through #709)                                                                                      |
| `baseline` binary    | `897eaf72aac1664423c3595e8ee86fab913acfd51d69db328be094f1f846995f`                                                                                                    |
| `split-draft` binary | `dd43d3da4e7548ebab3add07d0edca0b82dde7401e1ae1940afd01bc9d413e1f`                                                                                                    |
| `near-face` binary   | `8246d7cdc71f0e0e23f58739cd236b17eb4e6e5aa8bd316129abfdc0a311c64f`                                                                                                    |
| `gt-eval` binary     | `48f3dbb71799c54d018c910094bf682d2239bfaf568f2674b023f56d47133b0a`                                                                                                    |
| Capture              | `kirk0.pcapng`, SHA-256 `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`                                                                            |
| Source manifest      | `sha256:15e3ed3fb5c32f628e26be5743a87df9e7178cc1bda20d141beb3741b9226a79`, all four arms                                                                              |
| Pack                 | `ad8b9438-…-20260929-184718.156560000`, dataset `ds_95be6fb22498b44a`, digest `sha256:95be6fb22498b44a49821b98043e830130c74a0c4968d9806a984aeefa3849d6`, sensor frame |
| Sidecar              | revision 1974 (2026-10-07T23:56:38Z), `annotations.json` SHA-256 `c2426a32c6e442af87f81050c5a8885612a3341e8abbe13108a48febd5b614c8`                                   |
| Split manifest       | `kirk0-tuning.json`, SHA-256 `eb53dca8fe568b2f1309aa652d87237b32f52b2a8b0de02fadec0f9db54d48ec`, episode `tuning-from-62`                                             |
| Analysis script      | SHA-256 `3fd04bbbdd82b9e258d924ffce218872de4a1c7416e7a3e7a1e4f23f489e22e7` (Python 3.14, NumPy 2.5.3)                                                                 |
| Analysis output      | SHA-256 `443a6625e53abb03aabe7afe91c84cbf8fb34987b4ab5ba9da44c729d7181f7a`                                                                                            |
| Arm parameter hashes | control `2faa2a0d…`, `track` `5c7fd88a…`, `track_t5` `edeb3ad9…`, `track_a1` `03c15760…` (solid bodies, `cv_kf_v1`, online)                                           |

Raw outputs (evidence databases, scorer JSON, logs, the analysis script and its output) are kept
outside the repository and are not part of this report.

## Appendix: control strata

| Stratum          | Objects | Residual          |   n |  Mean | Median | Mean abs | p95 abs | Over 10 cm |
| ---------------- | ------: | ----------------- | --: | ----: | -----: | -------: | ------: | ---------: |
| All              |      16 | End normal        | 673 | −1.50 |  −0.85 |     1.62 |    5.07 |        90% |
| All              |      16 | Side normal       | 780 | −0.98 |  −0.50 |     1.02 |    3.43 |        86% |
| All              |      16 | End tangent (abs) | 280 |  0.66 |   0.32 |     0.66 |    2.03 |        83% |
| Extent: prior    |       4 | End normal        |  43 | −0.75 |  −0.89 |     1.20 |    2.80 |        93% |
| Extent: prior    |       4 | Side normal       |  44 | −0.92 |  −0.63 |     0.94 |    2.52 |        91% |
| Extent: prior    |       4 | End tangent (abs) |  18 |  1.09 |   1.25 |     1.09 |    1.80 |        83% |
| Extent: evidence |      16 | End normal        | 630 | −1.56 |  −0.84 |     1.65 |    5.08 |        89% |
| Extent: evidence |      16 | Side normal       | 736 | −0.98 |  −0.49 |     1.03 |    3.45 |        85% |
| Extent: evidence |      16 | End tangent (abs) | 262 |  0.63 |   0.31 |     0.63 |    2.02 |        83% |
| Range < 20 m     |      14 | End normal        | 407 | −1.23 |  −0.48 |     1.30 |    4.24 |        86% |
| Range < 20 m     |      14 | Side normal       | 507 | −0.61 |  −0.40 |     0.66 |    2.10 |        83% |
| Range < 20 m     |      14 | End tangent (abs) | 212 |  0.45 |   0.29 |     0.45 |    1.56 |        81% |
| Range 20–50 m    |       5 | End normal        | 257 | −1.96 |  −1.30 |     2.16 |    5.13 |        95% |
| Range 20–50 m    |       5 | Side normal       | 264 | −1.71 |  −1.57 |     1.74 |    4.23 |        92% |
| Range 20–50 m    |       5 | End tangent (abs) |  68 |  1.32 |   0.67 |     1.32 |    4.09 |        90% |
| Range 50–80 m    |       1 | End normal        |   9 | −0.61 |  −0.81 |     0.61 |    0.84 |        89% |
| Range 50–80 m    |       1 | Side normal       |   9 | +0.03 |  +0.04 |     0.10 |    0.24 |        44% |
| Range 50–80 m    |       1 | End tangent (abs) |   0 |     – |      – |        – |       – |          – |
| Range ≥ 80 m     |       0 | all               |   0 |     – |      – |        – |       – |          – |
| Returns < 30     |       3 | End normal        |  37 | −0.19 |  −0.33 |     0.63 |    2.09 |        78% |
| Returns < 30     |       3 | Side normal       |  37 | −0.05 |  −0.04 |     0.09 |    0.22 |        30% |
| Returns < 30     |       3 | End tangent (abs) |  10 |  0.06 |   0.05 |     0.06 |    0.10 |        10% |
| Returns 30–59    |       1 | End normal        |  17 | −0.89 |  −0.93 |     0.90 |    1.11 |        94% |
| Returns 30–59    |       1 | Side normal       |  17 | −0.20 |  −0.17 |     0.20 |    0.32 |       100% |
| Returns 30–59    |       1 | End tangent (abs) |   0 |     – |      – |        – |       – |          – |
| Returns 60–119   |       7 | End normal        |  75 | −1.26 |  −1.38 |     1.69 |    3.19 |        96% |
| Returns 60–119   |       7 | Side normal       |  82 | −1.84 |  −0.97 |     1.85 |    5.10 |        90% |
| Returns 60–119   |       7 | End tangent (abs) |  14 |  2.25 |   2.23 |     2.25 |    4.05 |        93% |
| Returns ≥ 120    |      13 | End normal        | 544 | −1.65 |  −0.66 |     1.70 |    5.10 |        89% |
| Returns ≥ 120    |      13 | Side normal       | 644 | −0.94 |  −0.50 |     0.99 |    3.26 |        88% |
| Returns ≥ 120    |      13 | End tangent (abs) | 256 |  0.60 |   0.31 |     0.60 |    1.84 |        86% |
| End-on < 20°     |       4 | End normal        |  34 | −1.90 |  −0.87 |     2.03 |    6.49 |        97% |
| End-on < 20°     |       4 | Side normal       |  33 | −2.36 |  −2.30 |     2.36 |    5.17 |       100% |
| End-on < 20°     |       4 | End tangent (abs) |  10 |  1.49 |   1.56 |     1.49 |    1.82 |       100% |
| Oblique 20–69°   |      15 | End normal        | 498 | −1.65 |  −0.97 |     1.72 |    5.08 |        89% |
| Oblique 20–69°   |      15 | Side normal       | 505 | −0.98 |  −0.59 |     1.00 |    3.26 |        88% |
| Oblique 20–69°   |      15 | End tangent (abs) | 228 |  0.51 |   0.30 |     0.51 |    1.86 |        83% |
| Broadside ≥ 70°  |      15 | End normal        | 141 | −0.88 |  −0.80 |     1.20 |    3.07 |        90% |
| Broadside ≥ 70°  |      15 | Side normal       | 242 | −0.78 |  −0.24 |     0.88 |    4.06 |        80% |
| Broadside ≥ 70°  |      15 | End tangent (abs) |  42 |  1.29 |   0.76 |     1.29 |    3.97 |        81% |
| Sparse tail      |       6 | End normal        |  84 | −0.99 |  −0.79 |     1.24 |    3.70 |        88% |
| Sparse tail      |       6 | Side normal       |  83 | −0.99 |  −0.18 |     1.01 |    4.19 |        69% |
| Sparse tail      |       6 | End tangent (abs) |  20 |  0.77 |   0.60 |     0.77 |    1.81 |        55% |
| Not sparse tail  |      14 | End normal        | 589 | −1.58 |  −0.99 |     1.68 |    5.07 |        90% |
| Not sparse tail  |      14 | Side normal       | 697 | −0.97 |  −0.53 |     1.02 |    3.37 |        88% |
| Not sparse tail  |      14 | End tangent (abs) | 260 |  0.65 |   0.32 |     0.65 |    2.25 |        85% |

The sparse tail is any of: range 50 m or more, under 60 returns, or an end-on aspect under 20°.
