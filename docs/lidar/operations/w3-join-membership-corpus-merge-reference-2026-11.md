# Track-conditioned joining on six more placements: spurious clusters, duplicates and a merge reference without labels

<!-- ignore-style-length -->

`fragment_join_track_membership` (#986) widens each motion footprint the fragment join tests
membership against by two standard deviations of the body's projected position. On kirk0 and two
placements it lowered spurious clusters at every case, and the open risk was the wrong join: two
road users made into one cluster. This run reads it on the first five-minute segment of six
placements it had not read, beside the join alone and the defaults, with a merge reference that
needs no labels. It answers [l4clus0011](../../../data/experiments/l4clus/l4clus0011.md). A
corpus read is a screen, not a decision.

- **Status:** Measured, 2026-11-09. One Claude Code cloud session; nothing adopted, no default
  changed
- **Spec:** [l4clus0011](../../../data/experiments/l4clus/l4clus0011.md), rule committed at
  `7842622d` (authored 16:40Z) before any replay; the merge reference's record and scorer at
  `21c1dd49`, also before any replay. Both are the rebased forms of the commits the replays ran
  on, `bde6df95` and `43f98014`, which were never pushed
- **Layers:** L4 joining, through the L3 track guard and L5 association; scored by
  `lidar-box-eval` and the label-free scripts
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`; the [join set](../../../config/experiments/motion-fragment-join.json),
  fingerprint `719d291ed197`. The membership option and the solid-body shadow are replay
  experiments
- **Related:** [truck membership on kirk0 (l4clus0010)](w3-truck-membership-extent-kirk0-2026-11.md),
  [motion fragment joining (l4clus0008)](w3-motion-fragment-joining-cloud-2026-11.md); the November
  benchmark (l5id0019) is on the open PR #994

## Summary

`membership` does not pass the screen. S1 holds, S2 and S3 fail.

- **The target holds at every site.** Spurious clusters a frame fell against `a0` at all six
  sites, by 3.0 % to 9.6 %, and by 7.3 % pooled (0.634 to 0.588), twice the join alone's 3.6 %.
  Brief tracks fell by 6, clutter by 7, and duplicate pairs from 85 to 69. No moving track was
  lost; six were shortened.
- **Noise fails on the letter at two sites.** At pierce-haight one new duplicate pair appeared
  (4 to 5), and neither of its tracks owned a join. At laguna-eddy the pairs held at 4 but vehicle
  tracks fell from 34 to 33, so the rate per hundred rose from 11.8 to 12.1. The join alone fails
  S2 at two sites as well.
- **The merge reference fails, and one count of it carries the finding.** `membership` made 370
  joins against `join`'s 219, and 127 of them took returns outside the owner's unwidened predicted
  box: those joins exist only because of the widening. Two-track joins (M2) rose from 4 to 14. All
  4 of `join`'s attach a young unclassed track to its neighbour; 5 of `membership`'s 14 join a
  second track that already holds a class: the same two cars in consecutive frames, two other cars,
  a bus and a pedestrian, and a car and a pedestrian. Oversize joins (M1) rose from 17 to 37, but every one of the 54 is within 0.5 m of
  the owner's own published box: a 7.4 to 10.9 m body under the car limit, not a merge. Solid-body
  length jumps (M3) fell from 2 to 0.
- **What follows.** The widened footprint buys a real drop in spurious clusters and pays for it
  with joins across confirmed tracks that the join alone does not make. Nothing is adopted; the
  option stays default-off. The narrow next test is the same option refusing a component that
  another confirmed track's predicted box claims.

## Question and rule

The question and the rule are in the [spec](../../../data/experiments/l4clus/l4clus0011.md),
committed before any replay. In short, against `a0` at each site unless stated:

- **S1, the target.** `membership`'s spurious clusters a frame lower than `a0`'s at 5 or more of
  the 6 sites.
- **S2, noise.** At every site, brief tracks up by no more than 3 and duplicate pairs per hundred
  vehicle tracks not up (l5id0019's R4 bounds).
- **S3, merges.** Pooled over the six sites, each of `membership`'s three merge-reference counts
  no higher than `join`'s.
- **Passes the screen** when S1, S2 and S3 all hold. R4's counts clause (moving count by more than
  5 %, p85 by more than 0.5 m/s) is reported, not gating.
- **Guards.** G0: on the first site, `membership` replayed without the solid-body shadow writes
  the same track observation rows. G1: each segment's UDP 2369 count equals
  `pcap-split --dry-run`'s for the same window.

## Method

- **Arms.** `a0`: the defaults. `join`: the join set. `membership`: the join set with
  `fragment_join_track_membership`. Every arm also ran `solid_body,solid_body_face_state` as a
  shadow, so the solid body's believed length could be read after each join.
- **Cases.** The first five minutes (0 to 300 s from the first packet) of the next six placements
  in the dataset's listing order after the two l5id0019 read: pierce-haight, fulton-divisadero,
  california-leon-baker, broadway-gough, laguna-eddy and lombard-broderick. embarcadero-folsom,
  the held-out site, was neither downloaded nor read.
- **Replays.** `lidar-box-eval -variants none -rows -require-settled=false -sample-points 1024`,
  40 s warm-up and 120 s scored, with an evidence database, as l5id0019 ran its segments; the two
  join arms also with `-join-records`. On each arm's evidence: `lidar_noise_screen.py`,
  `lidar_clutter_origin.py`, and against `a0`'s evidence `lidar_flip_cost.py --match footprint`
  and `lidar_figure_sensitivity.py`. Duplicates come from `lidar-box-eval`'s summary. These are
  l5id0019's tools and options.
- **The merge reference.** `lidar-box-eval -join-records` writes one record per cluster the join
  built from two or more DBSCAN components (`pipeline.FragmentJoinRecord`, new in `21c1dd49`):
  the joined cluster's oriented extent, its owner (the footprint holding every component), and
  each component's attribution to a confirmed track's predicted published box (projected to the
  frame by velocity, not widened, with the join's 0.15 m margin; a component goes to the track
  whose box holds the largest share of its returns when that share is at least a half). The hook
  observes the partition the join was given and returned; it changes no cluster.
  `scripts/lidar_join_merge_reference.py` counts **M1**, joins whose extent exceeds the owner
  class's plausible size (car 7.0 × 3.0 m; truck and vehicle 12.0 × 3.0 m; bus 19.0 × 3.2 m);
  **M2**, joins whose components are attributed to two or more confirmed tracks; and **M3**, joins
  after which the owner's solid-body length rises by more than 2.0 m between its last row before
  the frame and its first row within 0.2 s after.
- **How the segments were cut.** `pcap-split --start-seconds --duration-seconds` writes the whole
  capture (l5id0019's finding), so each window was cut by l5id0019's pcapng window filter. The
  first placement, pierce-haight, was fetched whole with `hf download` (3,138,797,280 bytes, its
  SHA-256 equal to the dataset's LFS oid), cut, and deleted. Then the shared disk fell to 1 GB
  free and once to zero, so the other five were streamed instead: an HTTP range request for the
  first 950 MB of each dataset file, which `tee` handed to the window filter and, through a FIFO,
  to `pcap-split --dry-run -port 2369`. Both stop at the window's end; no placement was on disk.
  The streamed sites' whole-file digests could not be checked; their bytes come from the dataset's
  `main` at revision `37ef1bef`.
- **Serial arms.** After the disk filled (19:02Z, during fulton-divisadero's first attempt, which
  was stopped and its output deleted unread), each site's arms ran one at a time so at most two
  evidence databases existed at once, behind a watchdog that paused, never killed, the replays
  under 400 MB free. Pierce-haight's two join arms had run side by side; no timing is read here.

## Guards

**G1 holds at all six sites.** Each segment's UDP 2369 count equals `pcap-split --dry-run`'s for
the same window, and no site was cut twice for a mismatch. Fulton-divisadero was cut twice
because its first attempt was stopped when the disk filled. Both streamed cuts gave the same
segment digest, so the streamed cut is deterministic.

| Site                  | UDP 2369 packets (cut = dry run) | Segment bytes | Segment SHA-256 |
| --------------------- | -------------------------------: | ------------: | --------------- |
| pierce-haight         |                          540,020 |   723,640,588 | `525df988907c…` |
| fulton-divisadero     |                          540,019 |   723,638,924 | `eda80c972e88…` |
| california-leon-baker |                          539,993 |   723,603,084 | `8c8d90480f71…` |
| broadway-gough        |                          539,994 |   723,607,836 | `5623f2e221f4…` |
| laguna-eddy           |                          540,002 |   723,613,172 | `3fad83b72d58…` |
| lombard-broderick     |                          540,001 |   723,612,320 | `a38d974e77fc…` |

**G0 holds, so M3 is readable.** The rule reads G0 on the first site.

- At pierce-haight, the `membership` run without the solid-body shadow wrote the same 5,731 box
  rows as the shadowed run, compared as multisets with the track column dropped, and the same
  partition into 60 tracks. Its join records' extents are identical.
- The observation-row hash recorded at pierce-haight differed. That first hash included the
  track ID, a random UUID per run, so it could not match. It was corrected before the next site.
  Pierce-haight's evidence databases had already been deleted, so its G0 is read on the box rows.
- To close that gap the check was also run at fulton-divisadero on the corrected hash. The
  shadowed and unshadowed runs wrote the same 13,635 track observation rows (SHA-256
  `6909ee6b20e4…`), the same 162-track partition, and the same 16,076 box rows.

## Results

All figures are over the 120 scored seconds of each segment (about 1,540 frames). "Dup pairs" are
`lidar-box-eval`'s duplicate pairs: two vehicle-sized published boxes that overlap by more than
half the smaller for five or more frames, also shown per hundred vehicle tracks.

### Pooled over the six sites

| Arm          | Spurious a frame | Brief    | Clutter  | Near-static | Moving (lost / short / added) | Dup pairs (per 100) | Pair-frames |
| ------------ | ---------------- | -------- | -------- | ----------: | ----------------------------- | ------------------- | ----------: |
| `a0`         | 0.634            | 133      | 171      |          38 | 437                           | 85 (18.2)           |       2,011 |
| `join`       | 0.611 (−3.6 %)   | 131 (−2) | 169 (−2) |          38 | 433 (0 / 4 / 0)               | 77 (16.7)           |       2,080 |
| `membership` | 0.588 (−7.3 %)   | 127 (−6) | 164 (−7) |          37 | 430 (0 / 6 / 0)               | 69 (15.0)           |       1,799 |

The pooled spurious rate weights each site by its scored frames, which are nearly equal.

### Each site against `a0`

| Site                  | Arm          | Spurious a frame | Brief Δ | Dup pairs (per 100) | Moving Δ    | p85 Δ, p98 Δ (m/s) | S1    | S2             |
| --------------------- | ------------ | ---------------- | ------: | ------------------- | ----------- | ------------------ | ----- | -------------- |
| pierce-haight         | `a0`         | 0.303            |         | 4 (20.0)            | 25          | 10.41, 14.30       |       |                |
|                       | `join`       | 0.296 (−2.4 %)   |      +0 | 4 (20.0)            | +0          | +0.10, +0.00       | falls | within         |
|                       | `membership` | 0.294 (−3.0 %)   |      +0 | 5 (23.8)            | +0          | +0.10, +0.00       | falls | **over: dups** |
| fulton-divisadero     | `a0`         | 0.657            |         | 22 (31.0)           | 85          | 9.58, 12.08        |       |                |
|                       | `join`       | 0.648 (−1.4 %)   |      +0 | 19 (27.5)           | −4 (−4.7 %) | +0.13, +0.00       | falls | within         |
|                       | `membership` | 0.627 (−4.6 %)   |      −1 | 20 (28.6)           | −5 (−5.9 %) | +0.13, +0.00       | falls | within         |
| california-leon-baker | `a0`         | 0.245            |         | 0 (0.0)             | 39          | 11.51, 13.83       |       |                |
|                       | `join`       | 0.234 (−4.5 %)   |      −4 | 1 (3.2)             | +1 (+2.6 %) | −0.19, +0.18       | falls | over: dups     |
|                       | `membership` | 0.226 (−7.9 %)   |      −4 | 0 (0.0)             | +0          | +0.00, +0.18       | falls | within         |
| broadway-gough        | `a0`         | 0.471            |         | 11 (15.7)           | 69          | 9.51, 10.53        |       |                |
|                       | `join`       | 0.456 (−3.2 %)   |      +0 | 11 (15.7)           | +0          | +0.00, +0.00       | falls | within         |
|                       | `membership` | 0.441 (−6.5 %)   |      +0 | 11 (15.7)           | +0          | +0.00, +0.00       | falls | within         |
| laguna-eddy           | `a0`         | 0.263            |         | 4 (11.8)            | 37          | 8.97, 11.25        |       |                |
|                       | `join`       | 0.256 (−2.7 %)   |      −1 | 4 (12.1)            | −1 (−2.7 %) | +0.10, +0.01       | falls | over: dups     |
|                       | `membership` | 0.254 (−3.5 %)   |      −1 | 4 (12.1)            | −1 (−2.7 %) | +0.10, +0.01       | falls | **over: dups** |
| lombard-broderick     | `a0`         | 1.866            |         | 44 (18.3)           | 182         | 11.65, 13.28       |       |                |
|                       | `join`       | 1.777 (−4.8 %)   |      +3 | 38 (15.9)           | +0          | +0.09, +0.00       | falls | within         |
|                       | `membership` | 1.688 (−9.6 %)   |      +0 | 29 (12.3)           | −1 (−0.5 %) | +0.09, −0.00       | falls | within         |

On `a0` rows the moving and p85, p98 columns give the counts themselves. Clutter rose in one
place only, under `join` at lombard-broderick (81 to 84); under `membership` it held or fell at
every site. Near-static tracks moved by at most one. The moving count comes from
`lidar_figure_sensitivity.py`; lost, shortened and added come from `lidar_flip_cost.py`'s
footprint match against `a0`. The count can fall while no track is lost, as at
fulton-divisadero. No arm lost a moving track at any site; `membership` shortened one at
fulton-divisadero and five at lombard-broderick.

### The two S2 failures

- **Pierce-haight, `membership`, one new pair.** Four of its five pairs are `a0`'s four, at the
  same frames and places. The fifth is two tracks overlapping around (14.7, 12.2) m for 12 frames
  (1.1 s), both classed pedestrian with published boxes of 4.1 and 4.2 m. Neither track owns any
  join. The pair comes from a changed partition downstream of a join, not from a join itself.
  Vehicle tracks rose from 20 to 21.
- **Laguna-eddy, both join arms, no new pair.** The pairs are 4 in every arm; vehicle tracks fell
  from 34 to 33, which moves the rate from 11.8 to 12.1 per hundred.

### The merge reference

| Site                  | Arm          | Joins (frames) | Owner classes   | M1 oversize | M2 two-track | M3 jump (readable) | Any | Returns outside the owner's box |
| --------------------- | ------------ | -------------- | --------------- | ----------: | -----------: | ------------------ | --: | ------------------------------: |
| pierce-haight         | `join`       | 7 (7)          | car 7           |           0 |            0 | 0 (7)              |   0 |                               0 |
|                       | `membership` | 8 (8)          | car 8           |           0 |            0 | 0 (8)              |   0 |                               1 |
| fulton-divisadero     | `join`       | 53 (52)        | car 53          |           5 |            3 | 0 (52)             |   7 |                               0 |
|                       | `membership` | 88 (84)        | car 88          |           9 |            5 | 0 (85)             |  13 |                              27 |
| california-leon-baker | `join`       | 10 (10)        | bus 8, car 2    |           0 |            0 | 2 (10)             |   2 |                               0 |
|                       | `membership` | 21 (21)        | bus 18, car 3   |           1 |            0 | 0 (21)             |   1 |                              11 |
| broadway-gough        | `join`       | 16 (16)        | car 16          |           0 |            0 | 0 (16)             |   0 |                               0 |
|                       | `membership` | 26 (26)        | car 26          |           0 |            0 | 0 (26)             |   0 |                              10 |
| laguna-eddy           | `join`       | 13 (13)        | car 13          |           0 |            0 | 0 (13)             |   0 |                               0 |
|                       | `membership` | 16 (16)        | car 16          |           0 |            0 | 0 (16)             |   0 |                               3 |
| lombard-broderick     | `join`       | 120 (113)      | bus 18, car 102 |          12 |            1 | 0 (117)            |  13 |                               0 |
|                       | `membership` | 211 (191)      | bus 23, car 188 |          27 |            9 | 0 (207)            |  36 |                              75 |
| **pooled**            | `join`       | 219 (211)      |                 |      **17** |        **4** | **2** (215)        |  22 |                               0 |
|                       | `membership` | 370 (346)      |                 |      **37** |       **14** | **0** (363)        |  50 |                             127 |

"Returns outside the owner's box" counts joins with any return outside the owner's unwidened
predicted box. `join` has none by construction; under `membership` those joins are the ones only
the widening admits. The published box's length never jumped by more than 2.0 m after a join in
either arm (0 of 215 and 0 of 363 readable). No solid-body row had a non-positive length.

**M1 reads class limits, not merges.** All 17 of `join`'s and all 37 of `membership`'s oversize
joins have an extent within 0.5 m of the owner's own published box, and all but one in each arm
have every attributed component on the owner. The owners are a few long bodies the classifier
calls car: one at fulton-divisadero with a 10.4 to 10.9 m box (5 joins under `join`, 9 under
`membership`) and two or three at lombard-broderick with 7.4 to 10.0 m boxes (12 and 27). The
other is `membership`'s one bus at california-leon-baker, 3.23 m wide against its own box's
2.85 m and the bus limit of 3.2 m. `membership` joins a long body's pieces in more frames; it
does not make it longer than its own box.

**M2 is where the arms differ in kind.** All 4 of `join`'s two-track joins attach a young
unclassed track (`dynamic`) to a car. That is the pattern of a fragment that founded its own track
before the join reached it. Of `membership`'s 14, on 10 owner tracks, 9 follow that pattern and 5
do not:

| Site              | Time (s) | Joined extent (m) | Owner           | Second track's component            | Outside the owner's box |
| ----------------- | -------: | ----------------- | --------------- | ----------------------------------- | ----------------------: |
| fulton-divisadero |     19.2 | 4.55 × 1.73       | car, 13 returns | car, 54 returns                     |                    0.04 |
| fulton-divisadero |     19.3 | 4.36 × 1.52       | car, 7 returns  | car, 54 returns (the same two cars) |                    0.00 |
| lombard-broderick |      8.6 | 3.34 × 1.41       | car, 30 returns | car, 305 returns                    |                    0.00 |
| lombard-broderick |     45.1 | 7.98 × 2.41       | bus, 79 returns | pedestrian, 25 returns              |                    0.15 |
| lombard-broderick |    150.6 | 6.72 × 2.43       | car, 42 returns | pedestrian, 8 returns (share 0.50)  |                    0.18 |

Times run from the first join record at the site. In three of the five the second track's
component holds most of the cluster's returns. Two of the 14 put most of their returns outside
the owner's box (0.65 and 0.53), both at lombard-broderick. Eight of the 14 have some.

## Against the rule

All six sites were read, so the screen is complete.

| Clause               | Reading                                                                                                                                  | Holds   |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| G0                   | Shadowed and unshadowed `membership` agree on rows and partition (pierce-haight on box rows, fulton-divisadero also on observation rows) | yes     |
| G1                   | Cut and dry-run UDP 2369 counts equal at all six sites                                                                                   | yes     |
| S1                   | Spurious clusters a frame below `a0`'s at 6 of 6 sites                                                                                   | **yes** |
| S2                   | Brief tracks within +3 at all six; duplicate pairs per hundred up at pierce-haight (20.0 to 23.8) and laguna-eddy (11.8 to 12.1)         | **no**  |
| S3                   | Pooled M1 37 against 17 and M2 14 against 4: higher; M3 0 against 2: not higher                                                          | **no**  |
| R4 counts (reported) | Moving count beyond 5 % at fulton-divisadero (−5.9 %); p85 within 0.5 m/s everywhere (largest 0.13)                                      | —       |

`membership` does not pass the screen. The rule gates on the letter. Two qualifications sit
beside the verdict, not over it. Laguna-eddy's S2 breach is a denominator change with no new
pair. M1's whole excess is long bodies within their own boxes. The verdict rests on S3's M2,
which these qualifications do not touch.

## Interpretation

- **The target generalises.** kirk0 and l5id0019's two placements showed the drop in spurious
  clusters; six more placements show it at every one. It is largest at the busiest site
  (lombard-broderick, 182 moving tracks, −9.6 %), though it does not follow traffic in order:
  fulton-divisadero, with 85, fell 4.6 %. Spurious clusters here are the clusters no track owns,
  5,872 pooled under `a0`, 5,657 under `join` and 5,444 under `membership`. Fewer duplicate pairs (69
  against 85) and fewer brief tracks fit the same reading: fragments that would found tracks or
  shadow a box are joined into their body.
- **The widening reaches across bodies.** 127 of `membership`'s 370 joins took returns the
  unwidened footprint would not. Some of that reach is intended: a long body's front or rear
  beyond its predicted box. Some is not. Five two-track joins join a component the tracker already
  holds as another classed road user. Two of them join a pedestrian-classed track into a vehicle's
  cluster, which is the merge the spec named as the risk. `join` alone made none of these in 219
  joins.
- **The merge count is small but not noise.** Five joins in 370 over 720 scored seconds is a low
  rate, and nothing here says each is wrong. A pedestrian-classed track beside a bus may be a
  fragment of the bus that the classifier misread. But the class of error is new under the widened
  footprint. A cross-track merge erases a road user from the count, and the count is what the
  product reports. A screen that admits it would be reading a target gain against an identity
  loss it cannot price.
- **M1, as specified, is the wrong instrument here.** Its class limits fire on long bodies the
  classifier calls car, which both arms join repeatedly. Every M1 flag sat within 0.5 m of the
  owner's own box. M1 neither found a merge nor cleared one; M2 did the work.
- **The solid body does not see a jump.** M3 is zero under `membership` and two under `join`,
  both on one bus at california-leon-baker. With no label of that bus's length, a jump there may
  be a length first made visible, which the spec named as M3's false alarm.

## What this does not establish

- **That any flagged join is a merge, or that an unflagged one is not.** No labels: M1 to M3 are
  geometric and identity signs. M1 reads the classifier's class, M2 the tracker's view of what is
  one body, and M3 a length the join itself may have made visible.
- **Anything beyond six segments of 120 scored seconds.** Each carries 25 to 182 moving tracks;
  the 5 % count bound is one to nine tracks. Six sites are fewer than a quarter of the 23 screen
  sites.
- **Anything held out.** These six are d2 first segments, read by l5id0012 for its flip cost;
  embarcadero-folsom is not read.
- **Physical truth.** No reference box, length or position off kirk0.
- **The streamed sites' whole-file digests.** Their segments' digests and packet counts are
  recorded; the files they came from are named by the dataset's revision only.
- **Pi cost or live behaviour.** No timing was read; two arms of the first site shared the host.

## Recommendations

1. **Keep `fragment_join_track_membership` default-off.** It failed the screen it was written
   against, and nothing was to be adopted from this run in any case.
2. **Test the option with a cross-track refusal next.** The narrow fix is to refuse a widened join
   when one of its components is claimed by another confirmed track's unwidened predicted box,
   under the merge reference's attribution rule (largest share, at least a half). That removes M2's
   classed joins by construction. The question is how much of the 7.3 % it keeps. Read it on these
   six segments, so the comparison is like for like, before any new site.
3. **Score M1 against the owner's own extent.** For the next spec, measure an oversize join
   against the owner's published box, or its longest settled length, not its class limit. As
   written, M1 spent all 54 flags on long bodies under the car limit. The classifier's 10 m cars
   are a separate L6 matter, noted here and not pursued.
4. **For future rules, bound duplicate pairs as a count alongside the rate.** A one-track change
   in the denominator broke S2 at laguna-eddy with no new pair. This is advice for specs yet to be
   written. It does not change this screen's reading.

## Provenance

- **Code.** `lidar-box-eval` and `pcap-split` built with the Makefile's LDFLAGS and the `pcap`
  tag at `43f98014` (VCS stamp `43f9801470158ec7b3a2e49e497a258f9e843c0e`, not modified), on the
  branch then based on main `704b8e46`. That commit was rebased, unchanged in content, to
  `21c1dd49`. Replays ran 2026-11-09, 18:38Z to 19:55Z.
- **Reproduced after the rebase.** Main then took #996, #997 and #1001. Their new behaviour is in
  default-off replay experiments that no arm here names, and the defaults file did not change.
  On 2026-11-10 the branch was rebased onto `091e21ed` and `lidar-box-eval` rebuilt. Laguna-eddy
  was cut again (the same segment digest) and its `a0` and `membership` arms replayed. Both match
  the original runs: box rows as multisets (8,129 and 8,103) and partitions (57 and 55 tracks),
  duplicate summaries field for field, and `membership`'s 16 join records. The other five sites
  were not replayed again.
- **Parameter sets.** `config/tuning.defaults.json`, SHA-256 `9ad7377fa5d3…`, fingerprint
  `ab0c4035780d`; `config/experiments/motion-fragment-join.json`, SHA-256 `2e604040caeb…`,
  fingerprint `719d291ed197`. Replay experiments: `solid_body,solid_body_face_state` on every
  arm, plus `fragment_join_track_membership` on `membership`.
- **Data.** `banshee-data/sf-street-speeds` at revision
  `37ef1befc1876ad860800beea1bbeaa6c9078da9`, `raw/lidar/`. Pierce-haight's whole file was
  checked against its LFS oid `9771bf57bfa1…` (3,138,797,280 bytes). The others are named by
  their oids at that revision: fulton-divisadero `ae6c6f0660c7…`, california-leon-baker
  `c21e469ffe6c…`, broadway-gough `8169d5749837…`, laguna-eddy `9d98d8873d40…`,
  lombard-broderick `1eb08d9e6ced…`. Segment digests are in the G1 table.
- **Merge reference.** `pipeline.FragmentJoinRecord` via `lidar-box-eval -join-records`, scored
  by `scripts/lidar_join_merge_reference.py` (tests in
  `scripts/test_lidar_join_merge_reference.py`), both committed at `21c1dd49`.
- **Run order.** Pierce-haight's `join` and `membership` ran side by side; every later arm ran
  alone. Each arm's evidence database was deleted after its summaries were written. The raw
  outputs (box rows, join records, summaries) were kept in the session's scratch space and are
  not committed.
- **Held out.** embarcadero-folsom was neither fetched nor read.
