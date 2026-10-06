# D2 medoid against OBB centre under the current solver, October 2026

- **Status:** Complete. `medoid_v0` stays the production position; `obb_centre_v1` stays opt-in. Quote these figures, not the pre-#600 OBB ones.
- **Scope:** kirk0 only (one 83.5 s capture, tuning data), build `db5832133` against the 24 September run that used the padded solver. Three replays, three scorers, two annotation packs.
- **Related:** [State-estimation plan, D2 outcome](../../plans/lidar-state-estimation-plan.md#d2-outcome-2026-09-24), [per-frame evaluation](per-frame-evaluation.md), [gap analysis H1](../../../data/maths/paper-implementation-gap-analysis.md#measured-outcome-h1), [annotation-scored tuning](annotation-scored-tuning.md).

On 24 September the D2 A/B scored the OBB centre against the medoid on kirk0. Recall was level,
but the OBB centre switched identity 146 times against the medoid's 92, and fragmented more. The
medoid stayed production (plan decision D5). That run used the padded Hungarian solver, which
#600 replaced with an exact one. The plan and the backlog item "Current-solver evidence refresh"
ask for the A/B to be re-run before any pre-#600 figure is reused. This is that re-run.

Raw outputs stay on the LiDAR volume (see [Provenance](#provenance)). This report is meant to be
read without them.

## Answer

The medoid barely moves under the current build. The OBB centre improves. Against the
24 September truth, with the 24 September scorer, gate and window:

- Identity switches: OBB centre 128 (was 146), medoid 92 (was 92). The gap falls from 54 to 36.
- Fragmentations: OBB centre 63 (was 64), medoid 57 (was 55).
- IDF1: OBB centre 0.370 (was 0.366), medoid 0.378 (was 0.380).
- Recall: OBB centre 0.533 (was 0.534), medoid 0.526 (was 0.527). Still level.

The OBB centre still switches more at five of six gates. It fragments more and has lower IDF1 at
all six. On the newer, fully reviewed pack the switch gap nearly closes: 101 against 99 with the
in-repo per-frame evaluator, and it changes sign with the reference position. Fragmentation and
IDF1 still favour the medoid there in every scoring.

So D5 stands, on weaker grounds than on 24 September. The determinism check passed: two OBB
replays wrote byte-identical estimates.

## The question

Does D5 still hold under the exact assignment solver? D5 says the OBB centre tracked identity
worse at equal recall, so the medoid stays production. And which 24 September figures may still be
quoted?

## What differs from 24 September

| Item                 | 24 September                            | This run                                       |
| -------------------- | --------------------------------------- | ---------------------------------------------- |
| Build                | Unstamped (`dev`, git SHA `unknown`)    | `db5832133`, stamped                           |
| Assignment solver    | Padded Hungarian, 1e18 sentinel         | Exact rectangular (#600)                       |
| Parameter hash       | `sha256:01aecfff…`                      | `sha256:fd35b0b2…`                             |
| Capture read from    | `internal/lidar/perf/pcap/kirk0.pcapng` | `/Volumes/banshee-captures/lidar/kirk0.pcapng` |
| Capture SHA-256      | `sha256:2864ebde…`                      | Same                                           |
| Warm-up and duration | 6 s, whole capture                      | Same                                           |
| Evidence sample size | 256 points per observation              | Same                                           |

The two execution configurations differ in one key: L3 `sensor_movement_drift_ratio_threshold`
moved from 0.35 to 0.5. It did not change the detections. The L4 observations are identical in
content between the builds: 2,423 clusters in 706 frames, the same records apart from the source
and calibration IDs they carry. Every difference below is therefore on the tracker side.

It is not #600 alone. Other default-on tracker changes landed between the builds, including #597
(capture-time boundary for prediction, coast age and expiry). Each arm's estimates are identical
across the builds up to sample 282 (28.2 s) and diverge after it. This report measures the
current build. It does not attribute the change to one pull request.

## Arms and runs

Each replay ran from the root of a clean checkout of `db5832133`, one at a time:

```bash
bin/baseline-db5832133 \
  -corpus cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-corpus.json \
  -index cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-index.json \
  -pcap-root /Volumes/banshee-captures -pcap-subdir lidar \
  -existing-source-manifest d2/source-manifest.json -case kirk0 \
  -warmup 6 -duration 0 -measurement-mode <mode> \
  -out ~/near-edge-followups/d2/<arm>/out -evidence-dir d2/<arm>/ev
```

| Arm         | `-measurement-mode` | Estimates | Tracks | Purpose                    |
| ----------- | ------------------- | --------: | -----: | -------------------------- |
| `medoid`    | `medoid_v0`         |     1,951 |     59 | Production position        |
| `obb`       | `obb_centre_v1`     |     1,973 |     62 | D2 candidate               |
| `obb_again` | `obb_centre_v1`     |     1,973 |     62 | Determinism check of `obb` |

Everything else is the tool's default: embedded tuning, no experiments, `-sample-points 256`,
`-require-settled` on. On 24 September the arms held 1,955 and 1,979 estimates on 59 and 64
tracks. All estimates are `online` stage from `cv_kf_v1`.

**The warm-up.** The per-frame evaluator scores one episode from sample 60 (6.0 s) to the last,
sample 831. The settings must put that episode inside the evidence. A 6 s warm-up does, and is
what 24 September used:

- 5 s and 5.5 s were refused: "background not settled at scoring boundary".
- L3 emits no foreground until it settles, at sample 59. Evidence starts there whatever the
  warm-up. The refused 5 s and 5.5 s runs and a 20 s check run wrote estimates byte-identical to
  the 6 s run.
- The first estimate is at sample 62, because a track needs four hits to confirm. Samples 60 and
  61 carry no estimate in any arm, build or warm-up. Both arms read them as misses.

## Scorers and references

| Scorer                                    | Reference                                   | Objects, scored units      | Window                | Like for like with 24 September     |
| ----------------------------------------- | ------------------------------------------- | -------------------------- | --------------------- | ----------------------------------- |
| `score_truth.py`, the 24 September scorer | Pack 8e422582 revision 204                  | 30, 2,579 object-frames    | Samples 61 to 831     | Yes: scorer, truth, gate and window |
| `score_truth.py`                          | Pack 8e422582 revision 402, the pack's head | 32, 3,286 object-frames    | Samples 61 to 831     | Scorer only; newer labels           |
| `score_truth.py`                          | Pack ad8b9438 revision 1969                 | 19, 3,118 object-frames    | Samples 61 to 831     | Scorer only; another pack           |
| `lidar-ground-truth-eval perframe`        | Pack ad8b9438 revision 1969, the D2 recipe  | 19, 3,122 reference points | Samples 60 to 831     | No: another scorer and pack         |
| `lidar-track-scorecard`                   | None (label-free)                           | Not applicable             | About 12 s to the end | Yes                                 |

`score_truth.py` positions each labelled object at the mean of its mask's returns. It matches an
estimate within 1 m plus half the object's footprint diagonal, scores every labelled road user
(proposals included) from 6 s after sample 0, and solves each frame with continuity first. Its
window starts at sample 61, because sample 60 falls 5.98 s after sample 0.

The 24 September truth was not the pack's head. It was revision 204, extracted on 21 September
for the annotation-scored sweep: 292 of its 4,606 masks were reviewed, the rest accepted
proposals. The pack had reached revision 402 by the time D2 ran. Rescoring byte-identical copies
of the 24 September databases against revision 204 with the stored `score_truth.py` reproduces
its three truth files and five of its six gate files exactly, apart from the last digit of three
floating-point speed figures. The sixth gate file, "double", did not record its parameters. This
report rescored both builds at 2 m plus the full footprint diagonal instead.

Pack ad8b9438 is also kirk0. Its manifest names `kirk0.pcapng`, and its points, sample count and
first and last timestamps match pack 8e422582. All 23 of its objects are reviewed, 20 of them road
users. It is a fresh labelling, so object IDs do not carry across the packs.

The per-frame evaluator ran the D2 recipe from [per-frame evaluation](per-frame-evaluation.md#kirk0):
one `tuning` split of every road user, one episode from sample 60 to the end,
`-allow-tuning-split -include-proposed -reference-position point_mean`, the footprint gate with a
1 m slack, both arms as declared baselines at stage `online`. It refused pack 8e422582 (see
[Refusals and surprises](#refusals-and-surprises)), so it scored ad8b9438 only.

## Findings

### Against the 24 September truth

`score_truth.py`, pack 8e422582 revision 204, footprint gate, samples 61 to 831. The 24 September
columns are the stored values, reproduced from the 24 September databases.

| Metric                                 | OBB 24 Sep | OBB now | Medoid 24 Sep | Medoid now |
| -------------------------------------- | ---------: | ------: | ------------: | ---------: |
| Recall                                 |      0.534 |   0.533 |         0.527 |      0.526 |
| Identity switches                      |        146 |     128 |            92 |         92 |
| Fragmentations                         |         64 |      63 |            55 |         57 |
| Identity recall                        |      0.323 |   0.326 |         0.334 |      0.332 |
| IDF1                                   |      0.366 |   0.370 |         0.380 |      0.378 |
| MOTA                                   |      0.433 |   0.439 |         0.453 |      0.451 |
| Spurious false positives               |        113 |     114 |           101 |        102 |
| Objects mostly lost (of 30)            |         13 |      13 |            12 |         12 |
| Vehicle speed step, RMS (m/s²)         |       2.11 |    2.02 |          1.96 |       1.91 |
| Vehicle speed against truth, MAD (m/s) |      0.526 |   0.520 |         0.483 |      0.485 |

The plan's D2 table gives the medoid's speed step as 1.97. The stored value is 1.965.

With only the tracks that reached four estimates (49 OBB, 48 medoid), the picture is the same.
Identity switches are 118 against 89 (24 September: 137 against 89). Fragmentations are 66
against 58 (67 against 56). IDF1 is 0.371 against 0.379 (0.367 against 0.381).

Against the pack's head, revision 402, the absolute numbers fall because there are more objects
and object-frames. The gap moves the same way. Switches are 128 against 102 (24 September: 146
against 102). Fragmentations are 74 against 63 (75 against 61). IDF1 is 0.321 against 0.329
(0.318 against 0.330). Recall is 0.420 against 0.416.

Only three objects in revision 204 have reviewed masks, 291 object-frames. Scored on those alone,
both arms have three switches in both builds.

### Gate sensitivity

Same scorer and truth. Each cell is OBB centre / medoid. Both builds were scored here with
identical parameters.

| Gate                          | Switches 24 Sep | Switches now | Fragmentations now | IDF1 now      | Recall now    |
| ----------------------------- | --------------- | ------------ | ------------------ | ------------- | ------------- |
| 1 m + half footprint diagonal | 146 / 92        | 128 / 92     | 63 / 57            | 0.370 / 0.378 | 0.533 / 0.526 |
| 0.5 m + quarter diagonal      | 140 / 87        | 126 / 87     | 60 / 56            | 0.364 / 0.376 | 0.524 / 0.516 |
| 2 m + full diagonal           | 132 / 98        | 116 / 98     | 64 / 59            | 0.359 / 0.377 | 0.535 / 0.528 |
| Fixed 1 m                     | 86 / 77         | 70 / 77      | 71 / 58            | 0.325 / 0.326 | 0.416 / 0.424 |
| Fixed 2 m                     | 133 / 84        | 119 / 84     | 68 / 64            | 0.363 / 0.375 | 0.523 / 0.513 |
| Fixed 3 m                     | 145 / 90        | 129 / 90     | 66 / 64            | 0.369 / 0.381 | 0.533 / 0.525 |

On 24 September the direction held at every gate. Now it holds at five. At a fixed 1 m gate the
OBB centre has fewer switches, 70 against 77. That gate drops recall to about 0.42, and there the
OBB centre still fragments more and its IDF1 is lower by only 0.001.

### Where the switches are

A few long-lived objects carry most of the count. Per object, under the footprint gate:

| Pack, object                        | Object-frames | OBB 24 Sep | OBB now | Medoid 24 Sep | Medoid now |
| ----------------------------------- | ------------: | ---------: | ------: | ------------: | ---------: |
| 8e422582 r204, `obj_b6924926` (car) |           426 |         68 |      51 |            33 |         33 |
| 8e422582 r204, `obj_e765825b` (car) |           196 |         35 |      33 |             4 |          4 |
| 8e422582 r204, `obj_4ab835fa` (car) |           410 |         19 |      19 |            17 |         17 |
| 8e422582 r204, `obj_5d384ff5` (car) |           108 |          1 |       2 |            18 |         18 |
| ad8b9438, `obj_35c661db` (truck)    |           455 |         69 |      52 |            33 |         33 |
| ad8b9438, `obj_8529ec5b` (car)      |           109 |          1 |       2 |            18 |         18 |
| ad8b9438, `obj_788622b5` (car)      |           771 |          0 |       0 |            16 |         16 |

Almost the whole OBB improvement is on one vehicle: 68 to 51 switches in one pack, 69 to 52 in
the other. The two rows are the same vehicle; their centroids agree to a median 0.5 m over all 426
shared frames. Revision 204 labels it a car, and it is an unreviewed proposal there. A second
proposal, `obj_e765825b`, lies within 3 m of that truck in 100 of its 196 frames. The reviewed
pack labels both as one truck.

On the 24 September truth the OBB centre's excess is 18 switches on the truck and 29 on that
second proposal, with 12 more across four other cars and a bus. The medoid has its own excess: 16
on a car at the end of the capture, 6 on a slow car near the sensor and 1 elsewhere. The net gap
is 36. In the reviewed pack the medoid's excess falls on the same two cars: 16 at the end (18
against 2) and 16 on the slow car (16 against 0). The reviewed pack follows the slow car through
the whole capture; revision 204 labels it only up to 18 s.

### The reviewed pack, per-frame evaluator

The D2 recipe on ad8b9438, episode samples 60 to 831, 3,122 reference points. The 24 September
columns are the 24 September databases scored by today's evaluator. No per-frame result was
published on 24 September. Recall here is matches over reference points.

| Metric            | OBB 24 Sep build | OBB now | Medoid 24 Sep build | Medoid now |
| ----------------- | ---------------: | ------: | ------------------: | ---------: |
| Recall            |            0.403 |   0.403 |               0.398 |      0.398 |
| Identity switches |              117 |     101 |                  99 |         99 |
| Fragmentations    |               55 |      55 |                  46 |         46 |
| IDF1              |            0.331 |   0.331 |               0.352 |      0.351 |
| Identity recall   |            0.269 |   0.268 |               0.284 |      0.283 |
| HOTA              |            0.383 |   0.386 |               0.392 |      0.393 |
| AssA              |            0.575 |   0.585 |               0.593 |      0.594 |
| DetA              |            0.274 |   0.274 |               0.278 |      0.278 |
| MOTA              |            0.146 |   0.153 |               0.150 |      0.151 |
| MOTP (m)          |            0.779 |   0.774 |               0.930 |      0.930 |

How the comparison moves with the scoring choices. Each cell is OBB centre / medoid for the
current build, with the 24 September build in brackets:

| Scoring                                               | Switches             | Fragmentations    | IDF1                          | HOTA                          |
| ----------------------------------------------------- | -------------------- | ----------------- | ----------------------------- | ----------------------------- |
| Evaluator, D2 recipe                                  | 101 / 99 (117 / 99)  | 55 / 46 (55 / 46) | 0.331 / 0.351 (0.331 / 0.352) | 0.386 / 0.393 (0.383 / 0.392) |
| Evaluator, default policy: footprint centre, reviewed | 83 / 100 (116 / 100) | 54 / 46 (54 / 46) | 0.331 / 0.351 (0.332 / 0.352) | 0.388 / 0.383 (0.388 / 0.383) |
| Evaluator, D2 recipe with a fixed 2 m gate            | 91 / 93 (106 / 93)   | 62 / 53 (62 / 53) | 0.312 / 0.315 (0.311 / 0.315) | 0.340 / 0.353 (0.337 / 0.353) |
| `score_truth.py`, footprint gate                      | 101 / 99 (117 / 99)  | 55 / 46 (55 / 46) | 0.308 / 0.311 (0.305 / 0.311) | Not computed                  |
| `score_truth.py`, tracks with four or more estimates  | 93 / 96 (110 / 96)   | 58 / 47 (58 / 47) | 0.309 / 0.312 (0.306 / 0.311) | Not computed                  |

On this pack the switch gap is within a few switches. It changes sign with the reference position
and with the track filter. Fragmentation favours the medoid by 8 to 11 in every row. IDF1 favours
it by 0.003 to 0.020. MOTP is a distance to a visible-mask position. Its sign flips with the gate,
and it is not evidence about the physical centre.

### Label-free

`lidar-track-scorecard -scoring-start-seconds 6`, as on 24 September. The scorecard counts that
start from the first evidence frame, sample 59, so it scores from about 12 s to the end, 71.2 s.

| Measure                                        | OBB 24 Sep | OBB now | Medoid 24 Sep | Medoid now |
| ---------------------------------------------- | ---------: | ------: | ------------: | ---------: |
| Tracks in the window                           |         45 |      43 |            40 |         40 |
| Median lifetime (s)                            |        2.6 |     2.6 |           2.6 |        2.6 |
| Share of tracks under 1 s                      |      0.311 |   0.302 |         0.375 |      0.375 |
| Association density                            |      0.713 |   0.747 |         0.779 |      0.777 |
| Coast error RMS, 0.5 s, steady, from 5 m/s (m) |      0.769 |   0.762 |         0.822 |      0.814 |
| Cross-track part of that error (m)             |      0.314 |   0.263 |         0.391 |      0.382 |
| Contested terminations                         |    8 of 46 | 6 of 44 |       7 of 42 |    6 of 42 |

Medoid against OBB centre as a reference run (2 m match): MOTA 0.839 (24 September 0.836), HOTA
0.695 (0.685), 122 identity switches (124), 28 fragmentations (28). The two arms still diverge
materially, by as much as they did.

## Determinism

- **Two OBB replays.** `obb` and `obb_again` wrote byte-identical estimate rows and identical
  observation rows. Their scorecards are byte-identical. As a reference run, one scores MOTA 1,
  HOTA 1, no switches and no misses against the other. Both the per-frame evaluator and
  `score_truth.py` give identical figures for every metric. The 24 September repeat was also
  identical.
- **Within each replay.** The corpus tool replays each arm twice, once with evidence and once
  without, and compares the tracking baselines. They were equal for all three arms, 771 frames
  each.
- **Across arms.** The L4 observations are identical in the medoid and OBB arms. Only tracking
  differs.
- **Across warm-ups.** Medoid runs at 5, 5.5, 6 and 20 s wrote byte-identical estimates.

## Refusals and surprises

1. **The per-frame evaluator cannot score pack 8e422582.** With the manifest pinned to revision
   402 it refused: the head is a legacy sidecar, and only revisions 1 to 401 are archived under
   `annotation-revisions/`. The pin is optional, so a second manifest left it out. The evaluator
   then refused: "sample 1 point 631 is claimed by both obj_16d9094b and obj_b401a3c0". The sidecar
   validator refuses a return claimed by two objects. Every retained revision has such returns:
   14,048 at revision 204 and 17,622 at revision 402. This was not worked around. Which object
   owns a return is a labelling decision, and the pack is read-only here. The kirk0 D2 recipe in
   [per-frame evaluation](per-frame-evaluation.md#kirk0) therefore cannot run on this pack with
   the current build. `score_truth.py` does not check exclusivity, which is why it carries the
   like-for-like comparison.
2. **The warm-up does not gate evidence.** The per-frame evaluation guide says an evidence run
   writes no estimates during its warm-up. On kirk0 in this build, evidence starts at the first
   foreground frame whatever the warm-up: the `-warmup 20` recipe for kirk0 writes the same
   estimates as `-warmup 6`. The warm-up sets the recording start, the tracking-baseline boundary
   and the settled check.
3. **Scorecard windows.** `-scoring-start-seconds` counts from the first evidence frame, not from
   the capture start or the warm-up boundary. Both builds here share the window, so this
   comparison holds. The 24-site corpus scorecards of 24 September used
   `-scoring-start-seconds 70` after a 70 s warm-up, so each started 70 s after its first evidence
   frame. At the two sites checked that scored 170 s and 194 s of the 200 s replayed, not all of
   it.
4. **The 24 September truth was revision 204**, not the pack's head at the time (402). Rescoring
   against 402 moves every absolute figure, though not the direction.

## Interpretation

The current build helps the OBB centre and leaves the medoid almost unchanged. On the
like-for-like truth, OBB switches fall by 18, its fragmentations by one, and its IDF1 rises by
0.004. The medoid gains two fragmentations and loses 0.002 IDF1. Each arm's estimates are
identical across the builds up to 28.2 s, and the OBB change sits almost entirely on one long
truck. A plausible reading is that the OBB centre of a long, partly seen vehicle moves more from
frame to frame, so two tracks compete inside its gate more often, and that is where the padded
solver's wrong assignments fell. That is a hypothesis. This run does not isolate it.

D5's reason still holds on the like-for-like truth, but by less. The OBB centre still has more
switches at five of six gates, more fragmentations at all six and lower IDF1 at all six, at level
recall. The switch margin is a third smaller than on 24 September.

On the fully reviewed pack the switch count no longer separates the arms. Its sign depends on the
reference position and on whether short tracks count. That is because the count rests on a handful
of objects, and on how they were labelled. The truck that drives the OBB centre's excess was two
objects in the proposal-heavy truth and one in the reviewed pack. Fragmentation and IDF1 are
steadier, and both favour the medoid in every scoring here.

The label-free measures agree in kind. The OBB centre's association density and short-track share
improved. Its constant-velocity coast error stays lower, especially across track, which is the
self-consistency D2 predicted, not accuracy. The two arms still diverge by a MOTA of about 0.84,
so the choice is not cosmetic.

Nothing here argues for switching. The case against the OBB centre is weaker than it looked on
24 September.

## Limitations

- **One capture.** kirk0 is 83.5 s with 19 to 32 labelled road users. A few vehicles carry most
  of the identity counts. No significance test is possible on one capture, and none is claimed.
- **Tuning data.** kirk0 is a tuning capture. Both splits are version 1 `tuning` drafts, not
  frozen. Nothing here is held out, and no figure may be quoted as held out.
- **Reviewed masks are not physical references.** Every reference position here, point mean or
  footprint centre, is a property of the returns a person marked. Both sit towards the faces the
  sensor saw. Position error against them favours the medoid by construction, because the medoid
  is also a centroid of visible returns, so position figures are reported and not used.
- **The 24 September truth is mostly proposals.** 292 of 4,606 masks in revision 204 were
  reviewed. The fully reviewed pack gives a different picture of the switch count.
- **Label-free measures say nothing about correctness.** They measure self-consistency and the
  divergence between arms.
- **Not #600 alone.** The rerun measures the current build. Other tracker changes landed after
  24 September; the observations are identical, so the change is on the tracker side, but it is
  not attributed to one pull request.
- **The 24-site corpus was not re-run.** Its label-free findings are still pre-#600.
- **Two scorers meet on one pack only.** The per-frame evaluator could not score pack 8e422582.

## Recommendation

1. **Keep `medoid_v0` as the production position.** `obb_centre_v1` stays opt-in for
   comparisons. D5 stands. The OBB centre still tracks identity no better at equal recall, and
   fragments more in every scoring, though the margin is smaller than recorded.
2. **Retire the pre-#600 OBB figures.** Do not quote 146 switches, 64 fragmentations, IDF1 0.366,
   the 146-against-92 gap, or "the direction held at every gate". Quote this report: 128 against
   92 switches, 63 against 57 fragmentations, IDF1 0.370 against 0.378, recall 0.533 against
   0.526, all against revision 204 with the 1 m footprint gate. The medoid's own pre-#600 figures
   match these within two counts, but quote the current ones. Results should be compared only
   within one build. The 24-site label-free figures stay pre-#600 until the corpus-baseline
   refresh re-runs them.
3. **Follow-ups for the operator.** The kirk0 recipe in the per-frame evaluation guide needs a
   pack the validator accepts, or a decision about overlapping claims in 8e422582. The guide's
   statement about warm-up and evidence needs correcting. Updating the plan's D2 outcome with
   these figures is a separate change.

## Provenance

| Identity                    | Value                                                                                                                                              |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Build                       | `db5832133582a95463c89afccfc2c1782fc3f038` (main `3ee42366c` plus a profiling flag), stamped                                                       |
| Corpus tool binary SHA-256  | `3ef83764cc7a0f6c22e7b2b6bece875dc4f817a54852f55243ffef6fb286f564`                                                                                 |
| Evaluator binary SHA-256    | `d149462f9eee51a892bca43d4b3a4c66b9aedf5b1f4cb8917975cac96ec882f1`                                                                                 |
| Scorecard binary SHA-256    | `327073054941829ab663556cff45b9b093b2b336328e43d31345511d23d34f62`                                                                                 |
| Split-draft binary SHA-256  | `d0f3a77737fd87461cf73054150343d6f35586b86b7c91ab6a4be7a29389d3ef`                                                                                 |
| Source manifest             | `sha256:5e8b3b0704d3d2512345c91ba616681623e8304947b0a97b1e42dc88a9971dd2`                                                                          |
| Capture                     | `kirk0.pcapng`, 200,657,872 bytes, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`                                       |
| Observation source ID       | `source/v1/04880163fb989819ea8bc1a03c5da87e9ac4bfced497c4a55a9a14caf2462c39`                                                                       |
| Parameter hash              | `sha256:fd35b0b28fc1d941040e4af3e4de5117d616844f85c6df3456dd193521eb237a`                                                                          |
| 24 September run            | Source manifest `sha256:700b82cccb6f6652c14b337bf24b812d1e7ba332226f67c3e6937983bbb86da7`, parameter hash `sha256:01aecfff…`, build unstamped      |
| Pack 8e422582               | `pack_digest` `sha256:a2027d434c323ce9bf346b999b3cad4930625756d47b926b26a7483392b0736d`, dataset `ds_a2027d434c323ce9`                             |
| 8e422582 revisions          | Head 402, `annotations.json` `sha256:c9e410b1…`; 24 September truth from revision 204, archive `sha256:dd500767…`                                  |
| Pack ad8b9438               | `pack_digest` `sha256:95be6fb22498b44a49821b98043e830130c74a0c4968d9806a984aeefa3849d6`, dataset `ds_95be6fb22498b44a`                             |
| ad8b9438 revision           | 1969, `annotations.json` `sha256:b0a1137731a22c7f8ec51a038573970b8cb29f16803d469bf629463fc588d417`                                                 |
| ad8b9438 split manifest     | `sha256:0b604ea54ab3fa0f65f86be9afe90394875612d2c30b1611d1e70cb5c13bbfe5`, drafted with `lidar-annotation-split-draft -from-sample 60`, not frozen |
| Per-frame reference digests | D2 recipe `sha256:29eb69659a8b…`; default policy `sha256:b0172f03eba5…`                                                                            |
| `score_truth.py`            | `sha256:1cc322d220f0c13649b3aa295b5f2213330bc581a1bb44ba2f4274c4a63c15c9`                                                                          |
| 24 September truth file     | `kirk0-tuning/ground-truth.json`, `sha256:7a7be9eef36fadf1047659f00f8ace709f95b5cc101e238e127cd88bb1a46406`                                        |

Raw outputs, none of them in Git:

- `/Volumes/lidar/lidar/velocity-campaign/near-edge-followups-20261005/d2/` holds the source
  manifest, the evidence databases, scorecards, truth files, every score, the split manifests,
  the logs and a `README.txt` that says which directories are authoritative. Refused and check
  runs are kept beside them, never reused.
- Recordings are under `~/near-edge-followups/d2/<arm>/out` on the internal disk.
- The 24 September evidence is unchanged under
  `/Volumes/lidar/lidar/velocity-campaign/obb-centre-ab-20260924/`. Its databases were scored from
  byte-identical copies.
- The annotation packs were only read.
