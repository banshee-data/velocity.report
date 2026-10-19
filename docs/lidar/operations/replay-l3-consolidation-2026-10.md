# One L3 for every replay: the defaults lost half the tracks, and now carry the best L3 measured, October 2026

Until #782 a scaled or realtime replay substituted three L3 values for the tuning file's while it
ran (closeness 2.0 against 3, safety margin 0.30 m against 0.15, neighbour confirmation 5 against
3), and every published scene ran them. #782 removed the substitution, so every replay now runs
its tuning file's L3, the one live input and the experiment tools run. This experiment asks
whether the defaults' L3 materially loses tracks or shortens them against the substituted one,
and if it does, which of the three keys the defaults should carry; l3bg0024 then chose the best
L3 measured for the defaults, against the CMR retune.

- **Status:** Measured, October 18–19, 2026 (experiments
  [l3bg0023](../../../data/experiments/l3bg/l3bg0023.md) and
  [l3bg0024](../../../data/experiments/l3bg/l3bg0024.md)). **Shipped:** the defaults carry the
  substituted L3 (closeness 2.0, safety margin 0.30 m, neighbour confirmation 5) for live input and
  every replay, with the noise costs below; the previous L3 is
  `config/experiments/l3-pre-consolidation.json`
- **Layers:** L3 foreground extraction; its effect on L4 clusters and L5 tracks
- **Related:** [replay speed and analysis mode](replay-speed-and-analysis-mode-2026-10.md) (perf0002),
  [replay determinism](replay-determinism-2026-10.md) (l3bg0020, l3bg0021),
  [region override attribution](l3-region-override-attribution-kirk-2026-10.md) (the CMR retune)

## Summary

- **The defaults lose about half the vehicle tracks the substituted L3 finds**, on the corpus and
  on the labels. On the first segment of 23 sites, 1,270 lasting vehicle-sized tracks against
  2,399 (−47 %), moving ones 939 against 1,850, with shorter lives (p50 4.1 s against 4.8, p90
  10.1 against 14.5) and shorter paths (p50 25 m against 34). On kirk0's reviewed labels the
  substituted L3 scores HOTA 0.542 and IDF1 0.656 against 0.376 and 0.350, with object-frame
  recall 0.936 against 0.587; on kirk1 it covers all 49 reference road users against 38. Most
  of the extra vehicle tracks are road users: the labels find them, and duplicate pairs fall
  (1.42 per hundred vehicle tracks against 2.28). The extra foreground also brings noise, which
  the rule did not measure: below.
- **Closeness carries the detection; the other two keys carry identity.** Closeness 2 alone
  finds 97 % of the substituted L3's vehicle tracks and nearly all its recall (0.924), but makes
  437 identity switches on kirk0 against 196, and IDF1 0.550 against 0.656. The safety margin and
  neighbour confirmation alone change almost nothing (1,236 vehicle tracks, HOTA 0.357). Together
  they are the substituted L3.
- **Under the rule written before the replays,** the defaults fail its first clause on every
  measure, the one-key candidate fails it on kirk's identity measures, and the candidate is the
  substituted L3 whole, three keys. On the second segment of 22 sites, read for L3 for the first
  time, it keeps its gain: 2,495 vehicle-sized tracks against 1,281 (+95 %, against +89 % on the
  first segment) and life p50 4.8 s against 4.0. The rule passes it.
- **It has costs the rule did not measure:** five times the spurious clusters on kirk0's labels
  (1.64 a frame against 0.31), three to ten times the stationary tracks lasting a minute on the
  corpus, twice the road-surface clusters, and a departed car's ghost lasting about half as long
  again. The CMR retune was built to take that noise back, so l3bg0024 scored it beside the
  substituted L3 on one build, under a rule written first: CMR has a third of the spurious
  clusters but nearly twice the identity switches (IDF1 0.593 against 0.656), and CMR with the
  substituted L3's margin and neighbour keys does no better (0.621). The substituted L3 is the only
  arm within 0.02 of the best on both identity measures, so it is the best L3 currently known,
  and the defaults now carry it. The repository's standing gate for an L3 retune (a labelled
  held-out window, label0001, and the Pi's frame time, perf0001) was set aside by decision: one
  L3 everywhere, at the best values measured. Both remain owed.

## Method

**Arms.** Four tuning files, one replay each per case, run by `lidar-state-estimation-baseline`:

| Arm                | Parameter set                                        | L3 keys against the defaults              |
| ------------------ | ---------------------------------------------------- | ----------------------------------------- |
| `defaults`         | `config/tuning.defaults.json`                        | none                                      |
| `replay-l3`        | `config/experiments/replay-l3-overrides.json`        | closeness 2.0, margin 0.3 m, neighbours 5 |
| `closeness-2`      | `config/experiments/replay-l3-closeness2.json`       | closeness 2.0                             |
| `margin-neighbour` | `config/experiments/replay-l3-margin-neighbour.json` | margin 0.3 m, neighbours 5                |

**Corpus.** The first five-minute segment of the 23 tuning and screen sites (embarcadero-folsom
held out), replayed from a 70 s warm-up for 200 s with 256 sample points, as the L3 corpus screen
l3bg0012 did. The evidence holds each track's estimates on the frames it was observed and the
cluster each was associated with, but no class and no confirmation state, so the measures were
defined from those before the replays and committed with the spec: a **lasting** track spans 2 s
or more; a **vehicle-sized** track is a lasting track whose associated clusters' longer box side
has a 90th percentile of 3.0 m or more; a **moving** one also has a 90th percentile speed of
2 m/s; **life** p50 and p90 are over vehicle-sized tracks, pooled across sites; **length** p50 is
the path along a moving vehicle-sized track's estimates; a **duplicate pair** is two vehicle-sized
tracks with estimates in at least 5 common frames, centres within 1.5 m in each.

**Labels.** kirk0 whole and kirk1's 272 s trim (172 s scored), replayed from a 6 s warm-up with
1,024 sample points, as l3bg0018 did. kirk0 is scored by `lidar-ground-truth-eval perframe`
against the reviewed pack and its tuning split (each arm as B against the defaults as A) and by
the zero-code arms report's `score_arm.py` (object-frame recall by class against the sweep's
ground truth), with W3 retention; kirk1 by `score_arm.py`'s reference coverage (M1).

**Held out.** The second segment of 22 sites, never read for L3, replayed under the defaults and
the candidate the rule selects, with the same definitions.

## Findings

### Corpus, first segment, 23 sites

| Measure                                     | defaults | replay-l3 | closeness-2 | margin-neighbour |
| ------------------------------------------- | -------: | --------: | ----------: | ---------------: |
| Clusters a frame, site median               |     4.30 |      8.21 |        8.01 |             4.41 |
| Tracks                                      |    7,031 |    10,309 |      10,309 |            7,023 |
| Tracks lasting 2 s or more                  |    3,145 |     5,586 |       5,431 |            3,189 |
| Moving tracks (2 s, p90 ≥ 2 m/s)            |    1,331 |     2,465 |       2,468 |            1,303 |
| Vehicle-sized tracks (lasting)              |    1,270 |     2,399 |       2,325 |            1,236 |
| Moving vehicle-sized tracks                 |      939 |     1,850 |       1,833 |              915 |
| Vehicle track life p50 (s), pooled          |      4.1 |       4.8 |         4.7 |              4.1 |
| Vehicle track life p90 (s), pooled          |     10.1 |      14.5 |        13.6 |             10.7 |
| Moving vehicle track length p50 (m), pooled |     25.0 |      34.2 |        33.8 |             25.5 |
| Duplicate pairs                             |       29 |        34 |          39 |               24 |
| Duplicate pairs per 100 vehicle tracks      |     2.28 |      1.42 |        1.68 |             1.94 |

The difference is not concentrated: the defaults find fewer vehicle-sized tracks than the
substituted L3 at 22 of 23 sites (the same number at marina-webster-beach), more than 25 % fewer
at 19, and the most at lombard-broderick
(50 against 176) and fulton-divisadero (24 against 76). Only marina-webster-beach (40 and 40)
and marina-broderick (92 and 98) are close. Closeness 2 alone is within 25 % of the substituted
L3 at every site.

### Labels, kirk0 and kirk1

| Arm                | kirk0 HOTA |  IDF1 |  DetA |  AssA | ID switches | Fragmentations | Object-frame recall (car / bus / pedestrian) | W3 retention | kirk1 references covered |
| ------------------ | ---------: | ----: | ----: | ----: | ----------: | -------------: | -------------------------------------------- | -----------: | -----------------------: |
| `defaults`         |      0.376 | 0.350 | 0.234 | 0.679 |         124 |             48 | 0.587 (0.627 / 0.529 / 0.283)                |        0.494 |                 38 of 49 |
| `replay-l3`        |      0.542 | 0.656 | 0.428 | 0.753 |         196 |             64 | 0.936 (0.941 / 0.941 / 0.894)                |        0.869 |                 49 of 49 |
| `closeness-2`      |      0.504 | 0.550 | 0.437 | 0.642 |         437 |             58 | 0.924 (0.932 / 0.922 / 0.866)                |        0.871 |                 49 of 49 |
| `margin-neighbour` |      0.357 | 0.337 | 0.235 | 0.618 |         223 |             29 | 0.586 (0.631 / 0.549 / 0.233)                |        0.499 |                 36 of 49 |

The defaults reproduce the attribution report's shipped kirk0 row exactly (recall 0.5871,
3.43 clusters a frame, 59 tracks), so these runs and that report measure the same pipeline.

## The rule, applied

The rule was written before the replays (2026-10-18).

**Clause 1: are the defaults materially worse than the substituted L3?** They are, on every
measure the clause names: kirk0 HOTA 0.166 lower and IDF1 0.306 lower (the clause allows 0.02);
vehicle recall 31 points lower (it allows 3); pooled vehicle-sized tracks 47 % fewer and life p50
15 % shorter (it allows 10 %); and 19 sites more than 25 % apart in vehicle tracks (it allows
none). The defaults fail.

**Clause 2: which arm, with the fewest changed keys?** Each arm against the substituted L3:

| Arm                | kirk0 HOTA, IDF1 | Vehicle recall | Vehicle tracks | Life p50 | Sites > 25 % apart | Clause 1 |
| ------------------ | ---------------- | -------------- | -------------- | -------- | ------------------ | -------- |
| `closeness-2`      | −0.038, −0.106   | −1 point       | −3.1 %         | −2.1 %   | none               | fails    |
| `margin-neighbour` | −0.185, −0.319   | −31 points     | −48.5 %        | −14.6 %  | 21 and 8           | fails    |

Closeness 2 alone passes every corpus measure but fails kirk0's identity measures, by 0.038 HOTA
and 0.106 IDF1, so the candidate is `replay-l3`, three keys. Its duplicate-pair rate is lower than
the defaults' (1.42 against 2.28 per hundred vehicle tracks), so the clause's bound on duplicates
holds.

**Held out.** On the second segment of 22 sites the candidate must keep at least half its gain
over the defaults on each measure it was chosen for:

| Measure, 22 held-out sites             | defaults | replay-l3 | Gain    | First-segment gain | Holds               |
| -------------------------------------- | -------: | --------: | ------- | ------------------ | ------------------- |
| Vehicle-sized tracks                   |    1,281 |     2,495 | +94.8 % | +88.9 %            | yes                 |
| Vehicle track life p50 (s), pooled     |      4.0 |       4.8 | +0.8 s  | +0.7 s             | yes                 |
| Duplicate pairs per 100 vehicle tracks |     1.09 |      1.88 | +0.79   | −0.86              | within the +2 bound |

The defaults are more than 25 % below the candidate at 21 of the 22 sites. The rule passes the
substituted L3, and would move its three keys into the defaults.

## Costs the rule did not measure

The rule asked whether the defaults lose tracks. It did not ask what the extra foreground costs,
and the same evidence shows:

| Measure                                                      |    defaults |   replay-l3 | closeness-2 | margin-neighbour |
| ------------------------------------------------------------ | ----------: | ----------: | ----------: | ---------------: |
| kirk0 spurious clusters a frame (labels)                     |        0.31 |        1.64 |        1.66 |             0.34 |
| kirk0 hits on labelled noise a frame                         |        0.21 |        1.14 |        1.02 |             0.20 |
| Corpus stationary tracks over 20 s, first / held out         |     56 / 62 |   170 / 122 |     146 / . |           65 / . |
| Corpus stationary tracks over 60 s, first / held out         |       5 / 8 |     51 / 27 |      39 / . |            8 / . |
| Road-surface clusters a frame, site median, first / held out | 0.22 / 0.23 | 0.47 / 0.63 |    0.49 / . |         0.21 / . |

With the three keys in the defaults, two of L3's characterisation tests had to be revisited, and
were: the closeness threshold at 100 m falls from 6.27 m, more than a vehicle length, to 4.38 m,
just under one, which changes a note in the gap analysis (HW1); and a parked car's departure
ghost, which the stationary-object review measured clearing about 3,100 frames after the car left
at closeness 3, now clears after about 4,600 (4,574 / 4,623 / 4,623 for the first, median and last
cell): a narrower closeness band takes longer to admit the road behind a car that has gone. That
is the stationary-ghost problem the CMR retune met with the same closeness 2, and the reason the
dwell policy was studied beside it (l3bg0008, l3bg0016).

So the substituted L3 trades noise for detection, as CMR does.

## l3bg0024: the best L3 currently known

The rule was written before the replays (2026-10-19): an arm is eligible if its kirk0 HOTA and
IDF1 are each within 0.02 of the highest of the three, its car recall within 3 points of the
highest and its kirk1 coverage within 2 of the most; the eligible arm with the fewest kirk0
spurious clusters is chosen, confirmed on the corpus unless it is the substituted L3, whose screen
l3bg0023 holds. All three ran on one build (`fcbd67a4c`), kirk0 whole and kirk1's trim, as above.

| Arm                                                                   | kirk0 HOTA |  IDF1 | ID switches | Car recall | Spurious clusters a frame | Hits on labelled noise | kirk1 covered | Eligible        |
| --------------------------------------------------------------------- | ---------: | ----: | ----------: | ---------: | ------------------------: | ---------------------: | ------------: | --------------- |
| `replay-l3` (closeness 2, margin 0.3, n 5)                            |      0.542 | 0.656 |         196 |      0.941 |                      1.64 |                   1.14 |      49 of 49 | yes             |
| `cmr` (closeness 2, post-settle 0.005, minimum cluster 8, floor −2.0) |      0.533 | 0.593 |         375 |      0.904 |                      0.52 |                   0.90 |      49 of 49 | no: IDF1 −0.063 |
| `cmr-mn` (CMR with margin 0.3, n 5)                                   |      0.536 | 0.621 |         374 |      0.916 |                      0.53 |                   0.99 |      49 of 49 | no: IDF1 −0.035 |

The substituted L3 is the only eligible arm and is chosen; its corpus screen is l3bg0023's. CMR's
noise controls take back two thirds of the spurious clusters, and CMR nearly doubles the identity
switches; which of its four keys costs the identity is not measured here, and an arm that kept
CMR's noise and the substituted L3's identity has not been found.

## What it costs on the Mac

The A/B gate (`scripts/perf-ab.sh`, five interleaved rounds of kirk0 on the development Mac,
the change against its parent) puts the cost of the new defaults at:

| Full profile              | Previous defaults | New defaults | Ratio |
| ------------------------- | ----------------: | -----------: | ----: |
| Wall clock (ms)           |             7,979 |        9,771 |  1.22 |
| Mean frame (ms)           |              6.42 |         8.47 |  1.34 |
| p95 frame (ms)            |              24.7 |         35.2 |  1.41 |
| Foreground points         |           985,224 |    1,760,445 |       |
| Clusters                  |             2,863 |        5,810 |       |
| Distinct confirmed tracks |                77 |          130 |       |

The l3-only profile moves by 4 %. The cost is the extra foreground the change exists to keep, so
the gate's 10 % threshold fails for the full profile by design, and the Mac baselines are
recaptured for the new fingerprint. The p95 frame is a third of the 98 ms budget on the Mac; the
Pi is slower, and unmeasured.

## Interpretation

Closeness is the width of the band around each cell's background range inside which a return
counts as background, as a multiple of the cell's spread; at 3 the band absorbs the near side of
vehicles and the bottoms of pedestrians, which is why the defaults' W3 retention is 0.49 and the
substituted L3's 0.87, the same figure the CMR retune reached with the same lever. Narrowing it
alone admits more returns, and the tracker splits some of them into fragments that change
identity; the wider safety margin and the stricter neighbour confirmation take back the isolated
returns, so the combination keeps the detection and loses less identity. That the substituted L3
was a hand-tune from January, written for watching replays, and still beats the measured defaults
on the labels says the defaults' closeness was never scored on labels before this.

For the published scenes, this is the cost the removal of the substitution would have had: a
republish under the defaults would draw about half the vehicles and shorter tracks. For live input,
the defaults have run closeness 3 all along; on kirk's labels the substituted L3 finds more road
users there too, and more noise.

## What this does not establish

- **Held-out labels.** kirk0 and kirk1 are one site, and kirk0's split is the tuning split. The
  standing rule for an L3 retune is a labelled held-out window (label0001, l3bg0009), which does
  not exist yet; the corpus held-out read is label-free.
- **Cost on the deployed sensor.** The substituted L3 doubles clusters a frame (4.3 to 8.2, site
  median); the Pi's frame time under it is unmeasured (perf0001 asks the same question of the CMR
  retune).
- **That three keys is the right set.** The two halves bracket it; a finer sweep could find a
  closeness between 2 and 3 that keeps identity without the other two keys.
- **The vehicle-sized proxy.** Corpus tracks are classed by cluster box size, not by a label;
  lasting stands in for confirmed. The labels agree in direction, which is why both are in the
  rule.

## Recommendations

1. **Time the new defaults on the Pi (perf0001).** Clusters a frame roughly double against the
   previous defaults (4.3 to 8.2, site median on the corpus) and the Mac's p95 frame time rises
   from 25 to 35 ms; the deployed sensor is unmeasured.
2. **Look for an L3 with CMR's noise and the substituted L3's identity:** spurious clusters fell to
   a third under CMR's noise controls while identity switches nearly doubled, so the minimum
   cluster size and the floor are worth separating. The next L3 rule should measure spurious
   clusters, stationary tracks and road-surface clusters beside recall and identity.
3. **Score the new defaults on the labelled held-out window when label0001 exists**, beside CMR,
   in l3bg0009's Stage B; kirk is one site and its split is the tuning split.
4. **Revisit the dwell policy under the new defaults** (l3bg0008, l3bg0016): long stationary
   tracks are three to ten times as many, and the departure ghost lasts half as long again.
5. **Republish the scenes once, after perf0003**, under the defaults: they ran this L3 before, so
   their track counts should hold, and their headers will now name the L3 they ran.

## Provenance

- **Code:** branch `claude/l3-consolidation-20261018`; replays by
  `lidar-state-estimation-baseline` built at `2cd982769` (the spec commit before any replay;
  the binary's own build stamp reads unknown, so each unit's `provenance.txt` records the
  commit), scored by `lidar-ground-truth-eval` at the same commit and `score_arm.py` and
  `score_free.py` from the 2026-10-11 run.
- **Tuning:** the four files above, each recorded with its SHA-256 in every unit's provenance;
  the defaults' fingerprint is `c2a4fe43ac43`.
- **Captures:** the first-segment corpus definition from the D2 corpus run of 2026-10-05, the
  second-segment definition from the held-out geometry run of 2026-10-11, read from
  `/Volumes/lidar/lidar/pcaps`; kirk0 from the repository, kirk1's trim from the follow-up run.
- **Labels:** pack `ad8b9438…-v2`, split `kirk0-reference-tuning-r1`, sweep ground truth and
  references `dd98c68e`, `60a4774c`.
- **Host:** the development Mac (M1 Pro, 8 cores, 16 GB), October 18–19, 2026, sharing memory with
  other work: replays that take 2 to 3 minutes alone took 5 to 25, which changes no result (the
  replays are deterministic, l3bg0021).
- **Raw output** (evidence databases, screens, scores and logs) is outside the repository:
  evidence on the LiDAR volume under `work/velocity-campaign/consol-20261018/l3bg0023/`, archived
  to `/Volumes/banshee-results/velocity-campaign/consol-20261018/`.
