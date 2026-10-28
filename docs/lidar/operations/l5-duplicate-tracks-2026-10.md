# One vehicle, two tracks: the identity options against duplicate tracks

<!-- ignore-style-length -->

The published box refinement ([report](published-box-refinement-2026-10.md)) traced every box
defect it could not fix to one cause: two confirmed tracks on one vehicle. This report counts
how often that happens and scores the tracker's default-off identity options against it, under a
rule committed before their replays.

- **Status:** Screen complete, 2026-10-17. One unattended run; experiment [l5id0001](../../../data/experiments/l5id/l5id0001.md) to [l5id0005](../../../data/experiments/l5id/l5id0005.md)
- **Layers:** L5 (association, reacquisition, identity)
- **Related:** [published box refinement](published-box-refinement-2026-10.md), [time-domain model](../architecture/time-domain-model.md), [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md), [gap analysis](../../../data/maths/paper-implementation-gap-analysis.md)
- **Parameter set:** the [defaults](../../../config/tuning.defaults.json) for every arm; the options are replay experiments, not tuning keys

## Summary

- **A quarter of vehicle tracks share their vehicle with another track.** Under the defaults, 242
  pairs of confirmed vehicle-sized tracks overlap by more than half the smaller box for five
  frames or more over the first segment of 23 sites and the two kirk captures: 15.5 pairs per
  hundred vehicle tracks, 26 % of them in a pair. The legacy box shows the same (10.7 per
  hundred). On kirk0, two thirds of a duplicate's overlapping frames are one track coasting on top
  of the live track that took its vehicle; about a quarter are a vehicle split into two clusters, each
  with a track. A duplicate counts a vehicle twice and is why truck 1 is drawn short.
- **The shipped identity options do not fix it without a cost (l5id0001).** The reacquisition
  guard, contested rejoins and tentative priority cut duplicates by a seventh to a fifth; contested
  rejoins with the likelihood cost halve them and take identity switches on kirk0 from 124 to 39,
  but lose association accuracy (HOTA 0.376 to 0.358) and vehicle frames. Tentative priority is
  the only option that improves every reviewed figure (HOTA 0.385, IDF1 0.390).
- **Ending a coasting track that lies on a live one removes the duplicates (l5id0002, l5id0003).**
  A new default-off tracker option, `OverlapEndFrames`, ends a coasting track whose drawn box
  shares more than half the smaller box with a live confirmed track's. On its first such frame it
  takes duplicates from 15.5 to 0.4 per hundred vehicle tracks, and on kirk0 identity switches
  from 124 to 29, false positives from 722 to 664, IDF1 from 0.350 to 0.391, with HOTA level and
  19 more matches. Held out, the duplicates fall as far.
- **But it ends real cars in queues.** On the fifth segment of 9 sites, read for the first time,
  every arm drew 3 to 7 % fewer observed vehicle frames than the defaults, past the 2 % allowed:
  18 % fewer on the busiest site, where the box model's floored boxes of queued cars overlap and
  the rule takes two cars for one. It stays off. The overlap test needs the body's observed
  returns, not its drawn box, to tell a duplicate from a neighbour.
- **On the body's returns, keeping the older track, it passes (l5id0004).** Ending a coasting
  track only when its predicted position lies within a live track's observed returns, and ending
  the younger of the two rather than the coasting one, takes duplicates from 15.5 to 1.3 per
  hundred vehicle tracks on the tuning set and from 15.1 to 1.2 on the second segment of 22 sites,
  never scored for identity, with observed vehicle frames 99.6 % and 101.7 % of the defaults'. On
  kirk0's reviewed pack IDF1 rises from 0.350 to 0.435, HOTA from 0.376 to 0.395 and association
  accuracy from 0.679 to 0.736, identity switches fall from 124 to 24 and false positives from 722
  to 664, with matches level.
- **Promising, not adopted.** Every option stays off by default: an identity change needs a
  held-out per-frame identity comparison before it ships, and the labelled held-out window is
  label0001's. Two clauses of the earlier rules measured vehicle retention badly and were replaced,
  each time before the next experiment's figures were computed; the held-out segments were the
  decision every time.

## Question

A duplicate is two confirmed tracks holding one road user. On kirk0, truck 1's close pass is
taken by two tracks in alternate frames: one takes the cluster, the other coasts a frame, then
takes it back. Every such frame is flagged a probable merge, so the published box refuses its
spans and the truck is drawn short; and the vehicle is counted twice. The tracker carries
default-off options that decide who may take a cluster when more than one track could:

| Arm             | Replay experiments                    | What it does                                                                                                                         |
| --------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `a0`            | none                                  | The shipped joint assignment                                                                                                         |
| `guard`         | `reacquisition_guard`                 | A track that missed the last frame may not take a cluster that does not fit its believed body, nor an ambiguous one                  |
| `contested`     | `contested_rejoin`                    | The guard, and a track that missed the last frame may not take a cluster a live track explains about as well                         |
| `contested-lik` | `contested_rejoin`, `likelihood_cost` | The same with the Gaussian likelihood cost, which removes the discount a coasting track's inflated covariance gives its bid (gap S3) |
| `tentative`     | `tentative_priority`                  | Confirmed tracks are matched first, but only inside their 99 % ellipse; every track left then competes jointly                       |

Does any of them hold one track per vehicle without losing the vehicles?

## Method

**The duplicate count.** `lidar-box-eval` replays a capture once per arm and, on every scored
frame, takes the published boxes of the confirmed tracks (the box model of the defaults). A pair
of boxes each 3 m or longer that share more than half the smaller box's area is two tracks on one
body, since two vehicles cannot overlap; a pair that does so on five or more scored frames is a
duplicate pair. The figure is duplicate pairs per hundred vehicle-sized tracks (tracks drawn 3 m
or longer on five or more scored frames), pooled over the first segment of 23 corpus sites, kirk0
and kirk1's first 270 s (70 s of warm-up on the corpus, 20 s on kirk).

**The reviewed identity score.** Each arm replays kirk0 through `lidar-state-estimation-baseline`
(6 s of warm-up, the whole capture), and `lidar-ground-truth-eval perframe` scores it against a0
on the reviewed pack's tuning split (`kirk0-tuning`, footprint gate 1 m): matches, false
positives and negatives, identity switches, MOT16 fragmentations, HOTA with its detection and
association parts, and IDF1. The frozen held-out split is not read.

**Rule.** Committed before any candidate's replay ([l5id0001](../../../data/experiments/l5id/l5id0001.md)):
a candidate passes when its duplicate pairs per hundred vehicle tracks fall by a third; on kirk0's
tuning split its matches are at least 98 % of a0's, its HOTA and IDF1 no more than 0.005 lower,
its identity switches no more and its fragmentations no more than a0's × 1.1 + 1; and its
vehicle-sized frames on the corpus are at least 95 % of a0's. One that passes is scored on the
fifth segment of 11 sites, read for the first time, and is then promising, not adopted.

## Findings

### How often, and how

Under the defaults, 1,561 confirmed tracks are drawn vehicle-sized over the 25 captures, and 242
pairs of them overlap by more than half the smaller box on five or more frames: 15.5 duplicate
pairs per hundred vehicle tracks, with 407 tracks (26 %) in at least one pair. On kirk0 alone
there are 5 pairs among 20 vehicle tracks; on kirk1, 11 among 53.

On kirk0, 121 of the 188 overlapping frames of its duplicate pairs have one track holding a
cluster while the other coasts on top of it, at its prediction, for up to its 15-frame miss
budget; 51 have both holding a cluster (one vehicle split into two clusters, each with a track);
16 have both coasting. The commonest duplicate is a track that lost its vehicle's cluster to
another track and coasts on regardless, or takes it back the next frame.

The count does not come from the box model's larger boxes. Counted from the legacy box (the
defaults with the box model off: the tracked position, the smoothed OBB heading, the frame's PCA
extents), which draws fewer tracks vehicle-sized (1,290) and on fewer frames, there are still 138
duplicate pairs, 10.7 per hundred vehicle tracks, with 249 tracks (19 %) in one.

### l5id0001: the identity options

Duplicate pairs over the 23 first segments and kirk:

| Arm             | Vehicle tracks | Vehicle frames | Duplicate pairs | Tracks in a pair | Pairs per 100 tracks | Pair-frames | Verdict on the corpus clauses |
| --------------- | -------------: | -------------: | --------------: | ---------------: | -------------------: | ----------: | ----------------------------- |
| `a0`            |          1,561 |         67,603 |             242 |              407 |                 15.5 |       5,415 |                               |
| `guard`         |          1,501 |         64,493 |             198 |              328 |                 13.2 |       3,631 | target missed                 |
| `contested`     |          1,563 |         63,097 |             195 |              331 |                 12.5 |       2,888 | target missed, frames lost    |
| `contested-lik` |          1,503 |         61,725 |             104 |              186 |                  6.9 |       1,477 | target met, frames lost       |
| `tentative`     |          1,422 |         67,300 |             190 |              311 |                 13.4 |       4,263 | target missed                 |

kirk0's tuning split, each arm against a0 (3,709 reviewed object-frames):

| Arm             | Matches | False positives | Identity switches | Fragmentations |  HOTA |  DetA |  AssA |  IDF1 | Reviewed clauses |
| --------------- | ------: | --------------: | ----------------: | -------------: | ----: | ----: | ----: | ----: | ---------------- |
| `a0`            |   1,394 |             722 |               124 |             48 | 0.376 | 0.234 | 0.679 | 0.350 |                  |
| `guard`         |   1,418 |             718 |                89 |             44 | 0.363 | 0.237 | 0.630 | 0.384 | HOTA             |
| `contested`     |   1,416 |             716 |                57 |             44 | 0.354 | 0.238 | 0.599 | 0.346 | HOTA             |
| `contested-lik` |   1,436 |             723 |                39 |             33 | 0.358 | 0.240 | 0.609 | 0.354 | HOTA             |
| `tentative`     |   1,440 |             718 |               117 |             36 | 0.385 | 0.241 | 0.689 | 0.390 | pass             |

No option passed. The rejoin rules do what they say: identity switches fall from 124 to 39 with
contested rejoins and the likelihood cost, and fragmentations from 48 to 33. But a track refused
its rejoin lives on as a coasting box and its object goes to a new identity, so association
accuracy, and with it HOTA, falls; and on the corpus the same arm halves the duplicate pairs while
drawing 8.7 % fewer vehicle frames, more than the duplicates it removed account for. Tentative
priority is the one option that improves every reviewed figure (HOTA 0.385, IDF1 0.390, 46 more
matches), and the one that changes the duplicates least that is not the guard.

None of the rejoin rules ends a duplicate that simply coasts on top of the live track, which is
most of them. That is l5id0002.

### l5id0002: ending a coasting track that lies on a live one

`OverlapEndFrames` (replay names `overlap_end_1`, `overlap_end_3`) ends a coasting track whose drawn
box shares more than half the smaller box with a confirmed track matched this frame, on the first
or third such frame of its coast, with a new expiry reason, `overlapped`. Two road users cannot
occupy the same ground, and an occluded one is behind its occluder, not inside it.

| Arm             | Vehicle tracks | Duplicate pairs | Pairs per 100 tracks | Pair-frames | Vehicle frames outside pairs | Of a0's |
| --------------- | -------------: | --------------: | -------------------: | ----------: | ---------------------------: | ------: |
| `a0`            |          1,561 |             242 |                 15.5 |       5,415 |                       62,188 |         |
| `oe1`           |          1,376 |               6 |                 0.44 |          93 |                       56,432 |  90.7 % |
| `oe3`           |          1,500 |             109 |                  7.3 |       1,628 |                       59,019 |  94.9 % |
| `oe1-tentative` |          1,314 |               5 |                 0.38 |          98 |                       57,896 |  93.1 % |

kirk0's tuning split against a0:

| Arm             | Matches | False positives | Identity switches | Fragmentations |  HOTA |  AssA |  IDF1 | Ended as overlapped |
| --------------- | ------: | --------------: | ----------------: | -------------: | ----: | ----: | ----: | ------------------: |
| `a0`            |   1,394 |             722 |               124 |             48 | 0.376 | 0.679 | 0.350 |                   0 |
| `oe1`           |   1,413 |             664 |                29 |             47 | 0.377 | 0.673 | 0.391 |                  72 |
| `oe3`           |   1,399 |             700 |                58 |             46 | 0.371 | 0.659 | 0.377 |                  31 |
| `oe1-tentative` |   1,441 |             676 |                23 |             31 | 0.385 | 0.685 | 0.370 |                  75 |

Every arm met the target and every reviewed clause: ending the duplicate on its first overlapping
frame takes identity switches from 124 to 29 and false positives from 722 to 664, with 19 more
matches and HOTA level. None passed the third clause, which counted every vehicle-sized frame
outside the overlapping pair-frames, coasting ones included: a duplicate coasts for up to 15
frames, most of them not overlapping by half, and those are the frames the option removes. The
clause could not tell a removed duplicate tail from a lost vehicle, so l5id0002 is answered as
failed and l5id0003 asks again with the frames in which a track holds a cluster.

### l5id0003: kept to the vehicles it observes

l5id0003 replaced the third clause with observed vehicle frames: the vehicle-sized rows in which a
confirmed track holds a cluster, coasting rows excluded. On the tuning set every overlap-end arm
passed: observed frames 99.8 % of a0's for `oe1`, 98.1 % for `oe3` and 105.3 % for `oe1-tentative`,
against 38,775. On the fifth segment of 9 sites, read for the first time (Van Ness at Sacramento's
and Embarcadero at Broadway's are shorter than the warm-up), none did:

| Arm             | Vehicle tracks | Duplicate pairs | Pairs per 100 tracks | Observed vehicle frames | Of a0's | Verdict                  |
| --------------- | -------------: | --------------: | -------------------: | ----------------------: | ------: | ------------------------ |
| `a0`            |            860 |             130 |                 15.1 |                  15,138 |         |                          |
| `oe1`           |            780 |               4 |                 0.51 |                  14,081 |  93.0 % | target held, frames lost |
| `oe3`           |            839 |              73 |                  8.7 |                  14,704 |  97.1 % | target held, frames lost |
| `oe1-tentative` |            734 |               4 |                 0.54 |                  14,575 |  96.3 % | target held, frames lost |

The duplicates fall as far held out as on the tuning set, but the vehicles do not stay. Most of the
loss is one site: on Fulton at Divisadero, the busiest fifth segment (68 of the 130 duplicate
pairs), `oe1` draws 18 % fewer observed vehicle frames than a0. It is not double counting: grouping
each frame's observed vehicle boxes that overlap by half, a0 has 5,790 groups there and `oe1`
4,862, the same fall. In queued traffic the box model draws a briefly unseen car its floor and its
held extents, away from the sensor and into the car behind or ahead; the rule then takes the two
for one body and ends a real car, which is born again as a tentative track and is not counted
until it confirms. Elsewhere the observed frames move both ways, by up to a fifth on single sites
of either set, so the measure is noisy at site scale and the pooled figure is what the rule read.

On kirk0's reviewed poses, the overlap end does not restore the trucks' length either: under `oe1`
truck 1 still leaves drawn about 6.4 m long, and truck 2's leaving box is a new track's, 4.2 m
long against 9.4 m, because the rule ends the coasting track whatever its age, and the survivor is
sometimes the younger one with the shorter memory.

### l5id0004: on the body's returns, keeping the older track

`OverlapEndOnBody` (replay qualifier `overlap_end_body`) changes the overlap end's test: the
coasting track's predicted position must lie within a live confirmed track's observed returns,
their extent along the live track's heading grown by a quarter metre. `OverlapEndKeepOlder`
(`overlap_end_older`) ends the younger of the two: when that is the live track, the older takes
the cluster back the next frame.

Tuning set (first segment of 23 sites and the two kirk captures) and kirk0's tuning split:

| Arm          | Pairs per 100 tracks | Observed vehicle frames | Of a0's | Matches | False positives | Identity switches | Fragmentations |  HOTA |  AssA |  IDF1 |
| ------------ | -------------------: | ----------------------: | ------: | ------: | --------------: | ----------------: | -------------: | ----: | ----: | ----: |
| `a0`         |                 15.5 |                  38,775 |         |   1,394 |             722 |               124 |             48 | 0.376 | 0.679 | 0.350 |
| `body`       |                  1.3 |                  38,305 |  98.8 % |   1,413 |             683 |                28 |             45 | 0.374 | 0.662 | 0.392 |
| `body-older` |                  1.3 |                  38,608 |  99.6 % |   1,393 |             664 |                24 |             53 | 0.395 | 0.736 | 0.435 |

Both passed and were scored on the second segment of 22 sites (the D2 second-segment index), read
for the box model's rotation in geom0005 but never for identity:

| Arm          | Vehicle tracks | Duplicate pairs | Pairs per 100 tracks | Observed vehicle frames | Of a0's | Verdict |
| ------------ | -------------: | --------------: | -------------------: | ----------------------: | ------: | ------- |
| `a0`         |          1,328 |             201 |                 15.1 |                  33,982 |         |         |
| `body`       |          1,207 |              14 |                  1.2 |                  33,218 | 97.75 % | fails   |
| `body-older` |          1,118 |              13 |                  1.2 |                  34,556 | 101.7 % | passes  |

The on-body test alone failed by a quarter of a point. Keeping the older track passed: the
duplicates fall as far as on the tuning set, and the observed vehicle frames do not fall at all.
Per site the observed frames still swing, from 83 % (Laguna at Eddy, Embarcadero at Broadway) to
187 % (Lombard at Broderick), so the pooled figure is what the
rule read. Keeping the older track is also what lifts the reviewed figures past every other arm:
association accuracy 0.736 against 0.679, IDF1 0.435 against 0.350.

On kirk1, whose labelled reference holds road-user spans rather than per-frame masks (49 road
users), `body-older` recovers 3 one-to-one by temporal overlap against a0's 4 and covers the same
38; under the shipped L3 so few are recovered one-to-one that this neither supports nor
contradicts it. On the corpus it takes the share of vehicle rows whose box overlaps another
confirmed track's from 17.6 % to 5.0 % ([published box refinement](published-box-refinement-2026-10.md#the-box-with-one-track-per-vehicle)).

It does not fix the drawn box on kirk0's trucks. Truck 1 still leaves drawn about 6.4 m long and
truck 2's leaving box is still a new track's, and the reviewed axis p90 is 13.4° against 8.4°:
some surviving tracks restart their box unconverged. That is the box's next question, not this
rule's.

### l5id0005: the third segments

A replication under a rule committed before its replays: `body-older` against a0 on the third
segment of the same 22 sites, read for the box model's rotation in geom0005 but never for identity.

| Arm          | Vehicle tracks | Duplicate pairs | Pairs per 100 tracks | Observed vehicle frames | Of a0's |
| ------------ | -------------: | --------------: | -------------------: | ----------------------: | ------: |
| `a0`         |          1,244 |             230 |                 18.5 |                  34,223 |         |
| `body-older` |            991 |              14 |                  1.4 |                  34,380 | 100.5 % |

It replicated: the duplicates fall by 17.1 per hundred against the 7.1 required, and the observed
vehicle frames hold. Two sets of 22 captures neither tuned nor screened on now agree with the
tuning set; what is still missing before it ships is a per-frame identity comparison on a labelled
held-out window.

## Limitations

- **One reviewed capture.** The identity scores are kirk0's tuning split: 21 reviewed objects,
  and the split is all of kirk0. No held-out labelled window exists until label0001.
- **The duplicate count is a proxy.** It reads the published boxes, which the box model draws
  larger than the returns; two boxes that share half the smaller box are almost always one body,
  but in dense queues they can be two cars, as l5id0003 found.
- **Retention measures are coarse.** Vehicle frames, frames outside duplicate pairs and observed
  vehicle frames each count tracks, not vehicles, and move by up to a fifth per site between arms.
  The rule read pooled figures; they say a vehicle was lost, not which.
- **Kirk0 has slow traffic.** The identity figures come from a residential intersection; the
  queueing that defeated the overlap end is on the corpus, where there are no labels.

## Provenance

| Item        | Value                                                                                                                                                                                                               |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| l5id0001    | tools at `9c3847fed` (`lidar-box-eval`, `lidar-state-estimation-baseline`, `lidar-ground-truth-eval`); rule committed in the same tree, before any candidate replay                                                 |
| l5id0002    | tools at `791da30ba`; `OverlapEndFrames` and its replay names in that tree; rule committed in it, before its replays                                                                                                |
| l5id0003    | `lidar-box-eval` at `bc5c8999f`, the tracker unchanged from `791da30ba`; rule committed in it, before any observed-frame figure                                                                                     |
| l5id0004    | tools at `843ab61ae`, the tree rebased unchanged but for the shared geometry's home as `f9656b755`; rule committed before its replays                                                                               |
| Tuning      | `config/tuning.defaults.json` for every arm (parameter hash `sha256:c2a4fe43…`); the options are replay experiments                                                                                                 |
| Captures    | first segment of 23 corpus sites (70 s warm-up), kirk0 and kirk1's first 270 s (20 s); held out, the fifth segment of 9 sites (l5id0003) and the second segment of 22 sites (l5id0004) from the NAS capture archive |
| References  | reviewed pack `ds_67842cb1b22b70e7`, sidecar revision 2232, split `kirk0-tuning` of `kirk0-reference-tuning-r1.json` (manifest `sha256:5df49517…`), footprint gate 1 m                                              |
| Raw outputs | summaries, evidence databases and per-frame scores under the run's scratch directory and the LiDAR volume, archived on the results share; not in Git                                                                |

## Recommendations

1. **Make the overlap end on the body, keeping the older track, the default once label0001's window
   scores it.** It is the one option that held one track per vehicle on held-out data without
   losing vehicles, and it lifts every reviewed identity figure. Shipping it needs the per-frame
   identity comparison on a labelled held-out window the backlog asks for, and a decision on
   whether the three options become tuning keys.
2. **Keep every other identity option off.** None held one track per vehicle on held-out data
   without losing vehicles or association accuracy.
3. **Read tentative priority again with the overlap end.** It improved every reviewed figure on
   its own; with the box-overlap end it took fragmentations to 31.
4. **Give a surviving track's box its history.** When the overlap end keeps a track whose box has
   not converged, the drawn box restarts; the published box could inherit the ended track's
   heading and extent beliefs.

## PCL follow-up disposition

The [OQ-07 source review](pcl-hypothesis-verification-disposition.md) drops a separate
Greedy/Papazov arm. It distinguishes static shared-point support from this report's temporal
survivor policy and records counterexamples for split clusters, tied support, occlusion, and
nearby distinct bodies. This does not alter the trials or their results above. l5id0006's survivor
work, l5id0011's containment claims, and the labelled held-out identity gate remain their own
questions; no new PCL replay result is claimed.
