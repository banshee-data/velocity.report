# Class evidence for the following gate: what kirk0's reviewed vehicles end classed as

<!-- ignore-style-length -->

Following is a rigid-vehicle metric on both sides, and the provisional field run on kirk0
suppressed every evaluated instant as `class_not_supported`. This report measures, for the first
time, what class the tracks on kirk0's reviewed road users end with, when the class arrives, and
how that moves with one track per vehicle and under the L3 retune. It answers
[l8fol0002](../../../data/experiments/l8fol/l8fol0002.md).

- **Status:** Measurement complete, 2026-10-20. One unattended run (exp-20261020); no default
  changed
- **Layers:** L6 (classification), read where L8 following reads it
- **Related:** [experiment review, October 18](experiment-review-2026-10-18.md),
  [per-frame evaluation](per-frame-evaluation.md#class-evidence-for-the-following-gate),
  [duplicate tracks](l5-duplicate-tracks-2026-10.md),
  [L3 candidates on the corpus](l3-candidates-corpus-dwell-2026-10.md),
  [headway report oracle](headway-report-oracle.md)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) and the
  [CMR set](../../../config/experiments/l3-retune-cm2-mp8-floor.json); the overlap end and the
  solid body are replay experiments

## Answer

**The class gate, not the geometry, is what hides following.** Under the best arm, 5 of kirk0's
17 reviewed vehicles end on a track classed rigid (29 %), against the four in five the spec set
before the replays as the point at which the gate stops being the binding constraint. Under the
defaults it is 3 of 17 (18 %). At the frame level a reviewed vehicle's track is classed rigid at
a fifth to a quarter of its reviewed frames. Most vehicles' tracks end `dynamic` (the classifier's
"unclassified moving object", which the following adapter reads as unknown): 10 of the 13
reviewed cars under the defaults.

One track per vehicle (the overlap end on the body, keeping the older track) and the L3 retune
each add one or two vehicles; neither comes near the threshold. No reviewed pedestrian ends
classed as a vehicle under any arm, so the gate's false applicability is nil on this capture.

The track features say why. Of kirk0's 18 vehicle-sized tracks under the defaults, 13 end
`dynamic`; 9 of those 13 pass the car rule's size test, and 8 of the 9 fail both of its other tests:
they are slower than its speed floor (mean over 5 m/s or peak over 7.5 m/s) and lower than its
height floor (mean height over 1.2 m). kirk0 is a residential intersection with traffic at 1 to
5 m/s, and the sensor, about 2.3 m above the road, sees a car's mean cluster height as 0.8 to
1.1 m. The rule's either-fast-or-tall condition fails at a slow site for any vehicle whose
lower body the sensor does not see.

Following on the classifier plan's scorecard therefore moves ahead of the endpoint work, as the
spec's reading said it would. The [recommendations](#recommendations) name the next question.

## Question

The spec's reading, written 2026-10-18 before the replays, per arm on kirk0:

1. the share of reviewed vehicle objects whose track ends rigid;
2. the share of their reviewed frames at which the track is already rigid;
3. the age at first rigid class, median and p90;
4. false applicability: reviewed pedestrians and cyclists whose track ends car, truck or bus.

If, under the best arm, fewer than four in five reviewed vehicles end rigid, the class gate is
what hides following in a field run and the classifier plan's scorecard moves ahead of the
endpoint work; if four in five or more do, the endpoint source (l8fol0003) is the constraint.

## Method

**The measure.** A new `-class` section of `lidar-ground-truth-eval perframe`
([per-frame evaluation](per-frame-evaluation.md#class-evidence-for-the-following-gate)). It reads the class the
following gate reads: each solid-body row's motion class and posterior, taken through
`l8behaviour.PassageFromL5` and `ClassApplicability(MetricFollowingSpatialGap, ...)` exactly as
the gate does, so "rigid" here is what the gate admits (car, truck and bus map to
`rigid_vehicle`). Per frame, the class is that of the track the per-frame MOT matching put on the
reviewed object at that frame. Per object, the end track is the one matched at the object's last
matched reviewed frame, read at its own latest row. Fragments and unchosen duplicates appear only
in the per-object track list and in "any track ends rigid". Every share is over all reviewed
objects, so an unmatched vehicle counts against it. Track age is from the track's first class row
to its first rigid row; object time is from the object's first reviewed frame to its first rigid
matched frame.

**Amended before the replays.** Three facts found while building the measure changed how it
runs, recorded in the spec before any replay (commit `1d73f1b4d`):

- The classifier's decision is persisted only on solid-body rows, which a replay writes only
  with the `solid_body` experiment. Every arm adds it. It shadows the filter and does not change
  the tracks; this run checked that: kirk0 under the defaults with and without it gives the same
  4,339 online estimates on 105 tracks, with identical position and velocity sums.
- Only kirk0 has a reviewed pack and split, so the measure runs on kirk0's tuning split alone,
  episode `whole-pack`. kirk1's labels are track-level and the evaluator cannot read them.
- Only `rigid_vehicle` is persisted, not car, truck or bus, so "first classed car, truck or bus"
  is read as first classed rigid.

**Arms.** Each replayed on the whole of kirk0 (6 s warm-up) with
`lidar-state-estimation-baseline`, then scored in pairs.

| Arm            | Tuning                                            | Replay experiments                                                     |
| -------------- | ------------------------------------------------- | ---------------------------------------------------------------------- |
| a0             | `config/tuning.defaults.json`                     | `solid_body`                                                           |
| body-older     | `config/tuning.defaults.json`                     | `solid_body`, `overlap_end_1`, `overlap_end_body`, `overlap_end_older` |
| cmr            | `config/experiments/l3-retune-cm2-mp8-floor.json` | `solid_body`                                                           |
| cmr-body-older | `config/experiments/l3-retune-cm2-mp8-floor.json` | `solid_body`, `overlap_end_1`, `overlap_end_body`, `overlap_end_older` |

**The reviewed objects.** kirk0's tuning split holds 17 reviewed vehicles (13 cars, 3 trucks,
1 bus) and 4 pedestrians; 16 of the vehicles are matched by some track under every arm.

## Findings

### The reading

| Arm            | Vehicles ending rigid | Any matched track ends rigid | Vehicle frames rigid (of all / of matched) | Track age to first rigid, median / p90 (reached, never) | Object time to first rigid, median / p90 (reached, never) | Pedestrians ending rigid |
| -------------- | --------------------- | ---------------------------- | ------------------------------------------ | ------------------------------------------------------- | --------------------------------------------------------- | ------------------------ |
| a0             | 3 of 17 (18 %)        | 5                            | 20.5 % / 24.5 %                            | 3.2 s / 5.6 s (7, 9)                                    | 6.0 s / 19.9 s (10, 6)                                    | 0 of 4                   |
| body-older     | 5 of 17 (29 %)        | 6                            | 19.0 % / 22.8 %                            | 3.8 s / 10.1 s (9, 7)                                   | 5.4 s / 19.8 s (10, 6)                                    | 0 of 4                   |
| cmr            | 4 of 17 (24 %)        | 9                            | 24.8 % / 31.6 %                            | 2.7 s / 5.5 s (7, 9)                                    | 6.0 s / 16.9 s (10, 6)                                    | 0 of 4                   |
| cmr-body-older | 5 of 17 (29 %)        | 7                            | 25.2 % / 32.0 %                            | 1.8 s / 4.7 s (10, 6)                                   | 5.4 s / 16.9 s (10, 6)                                    | 0 of 4                   |

The best arms reach 29 %; the rule's threshold is 80 %. The result does not depend on which
arm is best: no arm is within a factor of two of it.

"Any matched track ends rigid" is higher than "ends rigid" because a vehicle's class sometimes
arrives on a fragment or a duplicate that is not the track matched at its last reviewed frame.
Under CMR, which keeps more of each vehicle's returns, 9 vehicles have some track that ends rigid
but only 4 end on one. Where a class arrives, it arrives quickly (a median of 2 to 4 s of track
life); for six vehicles under every arm it never arrives on any matched frame.

### By label

Share of each label's reviewed objects whose end track is rigid, and what the others end as.

| Label          | a0                          | body-older                 | cmr                        | cmr-body-older             |
| -------------- | --------------------------- | -------------------------- | -------------------------- | -------------------------- |
| car (13)       | 2 (10 unknown, 1 unmatched) | 3 (9 unknown, 1 unmatched) | 3 (9 unknown, 1 unmatched) | 3 (9 unknown, 1 unmatched) |
| truck (3)      | 0 (3 unknown)               | 1 (2 unknown)              | 0 (3 unknown)              | 1 (2 unknown)              |
| bus (1)        | 1                           | 1                          | 1                          | 1                          |
| pedestrian (4) | 0 (4 unknown)               | 0 (4 unknown)              | 0 (4 unknown)              | 0 (4 unknown)              |

"Unknown" is the adapter's reading of the classifier's `dynamic`. kirk0's pack holds one object
labelled bus, which the spec had said it did not; it is the one vehicle every arm classes rigid.

### Why the tracks end `dynamic`

The classifier (`l6objects.ClassifyFeatures`) classes a track a car when it is large (mean length
over 3.0 m or mean width over 1.5 m) and either fast (mean speed over 5 m/s or peak over 7.5 m/s)
or tall (mean height over 1.2 m); truck classification is disabled, so trucks fall to the car
rule. Under the defaults on kirk0, 18 tracks are vehicle-sized (lasting 2 s or more, with a p90
cluster longer side of 3.0 m or more). Their track-level features, from the same replay
(the a0 tracks, identical row for row):

| Ends as | Tracks | Large | Fast | Tall | Pass size, fail both speed and height |
| ------- | ------ | ----- | ---- | ---- | ------------------------------------- |
| car     | 5      | 5     | 2    | 3    | 0                                     |
| dynamic | 13     | 9     | 1    | 1    | 8                                     |

The `dynamic` tracks' mean heights are 0.6 to 1.2 m and their mean speeds 1.1 to 5.0 m/s (peaks
2.6 to 7.6 m/s). These are the tracks' final aggregates; the classifier decides on the running
features at each frame, which is why one track whose final mean height reaches the floor still
ends `dynamic`. A car's roof is about 1.4 to 1.5 m above the road, so a mean cluster height
under 1.2 m means the cluster's lowest returns sit well above the road: the lower body the sensor
does not see ([l3bg0024](../../../data/experiments/l3bg/l3bg0024.md) asks where it goes). At a
site where traffic moves at 1 to 5 m/s, the speed branch cannot rescue those tracks either.

This is a diagnosis on one capture's tuning data, not a candidate: no rule was written for a
change to the classifier and none was tried.

## Limitations

- **One capture, tuning data.** kirk0's 17 reviewed vehicles on its tuning split, read many times
  before; nothing here is held out, and the figures are not a classification rate for the corpus.
- **A slow site.** kirk0's traffic is residential and slow, which is exactly where the car rule's
  speed branch fails; at an arterial site more tracks would pass on speed. The corpus has no class
  labels, so how general the miss is cannot be scored here; a label-free count of vehicle-sized
  tracks by class on the corpus is the cheap next read (see the recommendations).
- **Rigid, not car, truck or bus.** The persisted belief is the motion class, so the measure
  cannot say whether a truck was classed a truck; truck classification is disabled in any case.
- **The matching decides the end track.** A vehicle whose class arrived on a fragment that was not
  matched at its last frame counts as not rigid; "any matched track ends rigid" bounds that.
- **The experiment queue did not record the units.** Its `manual:implement` step closes only on a
  commit on main, so the queue refused the replay and score marks; the units ran as listed here.

## Provenance

| Item        | Value                                                                                                                                                                                                                                                      |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | measure at `020055f09`; spec amended and approved at `1d73f1b4d`, before any replay; harness and evaluator built at `1d73f1b4d` with the Makefile's LDFLAGS                                                                                                |
| Tuning      | `config/tuning.defaults.json` (sha256 `e6cf6c8a…`; a0 parameter hash `sha256:5d2ea17f…`, body-older `sha256:283b6e1d…`); `config/experiments/l3-retune-cm2-mp8-floor.json` (sha256 `6bc94773…`; cmr `sha256:5a3c446d…`, cmr-body-older `sha256:34493244…`) |
| Capture     | kirk0 (`internal/lidar/perf/pcap/kirk0.pcapng`), whole, 6 s warm-up, `-sample-points 1024`                                                                                                                                                                 |
| References  | reviewed pack `ds_67842cb1b22b70e7`, sidecar revision 2232, split `kirk0-tuning` of `kirk0-reference-tuning-r1.json` (manifest `sha256:5df49517…`, split revision 2), episode `whole-pack`, footprint gate                                                 |
| Diagnosis   | track features from a kirk0 replay with the defaults and `solid_body` at `d76ab354d` (the l8fol0003 branch's harness, which also writes `lidar_tracks`), whose 4,339 online estimates are identical to a0's                                                |
| Raw outputs | evidence databases, per-frame JSON and logs under the run's scratch directory (`exp-20261020/kirk/l8fol0002`, `results/l8fol0002`); not in Git                                                                                                             |

## Recommendations

1. **Put the classifier ahead of the endpoint for the following metric.** The spec's reading
   decides it: the gate admits under a third of kirk0's reviewed vehicles under every arm. The
   endpoint source (l8fol0003) and the following limits (l8fol0004) cannot produce a valid second
   until it admits more.
2. **Specify the car rule's next question before changing it.** The diagnosis points at the rule's
   either-fast-or-tall condition on a slow site with an unseen lower body. A candidate is a
   classifier experiment that admits a large, moving, vehicle-sized track without the speed or
   height test (or with the height read from the published box on the ground surface, which
   [ground0004](../../../data/experiments/ground/ground0004.md) proposes), scored by this measure on
   kirk0 with false applicability held at zero, and on kirk1's track-level labels as a second
   capture. It needs its rule written first and a held-out reading before any default changes;
   class labels on the published scenes and reports would move with it.
3. **Count the corpus's vehicle-sized tracks by class, label-free.** One query over the
   l8fol0001 and l8fol0003 evidence says how many vehicle-sized tracks end `dynamic` at each of
   the 23 sites, which tells how much of this is kirk0's slow traffic.
4. **Keep `solid_body` on any arm the class measure reads**, or persist the classifier's label
   with the estimates, so class can be scored without the shadow.
