# Does the background converge by its own criterion? Settling trajectories on 23 sites

<!-- ignore-style-length -->

With the honest first check, every replay waits out the 30 s warm-up ceiling, because the spread
delta never reaches `settling_max_spread_delta` 0.001. This report logs every convergence check
of the first segment of 23 corpus sites up to 120 s and asks whether any absolute threshold would
let scenes stop waiting, and what stopping earlier costs on kirk0 and kirk1. It answers
[l3bg0022](../../../data/experiments/l3bg/l3bg0022.md).

- **Status:** Measured, 2026-11-03. One unattended run on arrow-worker (exp-20261103, lane A); no
  default changed
- **Layers:** L3 (settling: the warm-up ceiling and the convergence criterion)
- **Related:** [replay determinism and the honest first check](replay-determinism-2026-10.md),
  [replay speed and analysis mode](replay-speed-and-analysis-mode-2026-10.md),
  [l3bg0030](../../../data/experiments/l3bg/l3bg0030.md), the relative criterion this proposes

## Summary

- **0.001 is a threshold no scene meets.** None of the 23 sites converges under it before 30 s,
  and none by 120 s. The lowest spread delta any site reached before 30 s was 0.0024
  (california-leon-baker); site medians run from 0.0028 (ashbury-downey) to 0.050
  (pierce-haight).
- **No absolute threshold passes the rule.** At 0.005, 3 of 23 sites converge before the ceiling;
  at 0.01, 15. The rule needs 18. Even 0.02 gives 21, but 18 of them at the first check where the
  delta can be measured, about 7 s in, so the criterion would end warm-up as soon as it is allowed.
- **The spread delta does not settle; it measures traffic.** It does not fall with time: 12 of 23
  sites have a lower median after 30 s than before, and each site holds roughly its own level from
  the first measured check to 120 s. Busy sites (1st-mission, columbus-broadway, pierce-haight)
  sit at 0.02 to 0.05; quiet ones (ashbury-downey, california-leon-baker) at 0.003.
- **Only the spread delta decides.** Region stability reads 1.0 at every check, because regions
  are identified only when settling completes, and mean confidence is above 59 at the first check
  of every site, against a criterion of 10. Coverage holds at 0.8 or more except at
  marina-broderick, under it at 20 of its 24 checks before 30 s.
- **A delta relative to the mean spread separates better.** The ratio's median by site runs only
  from 0.047 to 0.084. At a ratio of 0.075, 19 of 23 sites converge before 30 s, none at the first
  check, at a median of 16.9 s. This was found after looking, so it is proposed as
  [l3bg0030](../../../data/experiments/l3bg/l3bg0030.md), not adopted.
- **On kirk, stopping earlier costs nothing; it gains.** Under 0.01 both kirk captures converge at
  their first measured check (6.9 s): kirk0 HOTA rises from the ceiling's 0.3936 to 0.4269 and
  identity switches fall from 92 to 82, and kirk1 covers all 49 labelled road users against 41.
  Under 0.005 kirk0 never converges (its delta sits at 0.005 to 0.006) and scores as the ceiling.
  The corpus clause fails first, so this decides nothing.

## Method

**Trajectories.** `lidar-state-estimation-baseline -settling-trajectory`, new in this branch: the
grid records every convergence check of its warm-up (elapsed time by the frames' clock, coverage,
spread delta, region stability, mean confidence, mean spread and the decision), and the tool
writes them per case and fails the case if the repeat run checked differently. Each of the 23
sites' first five-minute segment was replayed once, with the replay experiment `settling_honest`
and [`l3-settling-trajectory.json`](../../../config/experiments/l3-settling-trajectory.json), the
defaults with the warm-up ceiling raised to 120 s, at 120 s warm-up and 10 s scored, with
`-require-settled=false`. A check is the same computation whatever the ceiling, so the checks
before 30 s are the ones the defaults' ceiling would make; those after 30 s answer the question's
"given time". Checks run every ten frames, about once a second, from the 50-frame minimum
(5.9 s); the first measured one, with a previous check's spreads to compare against, comes at
6.9 s at every site. 115 checks per site, each repeat identical.

**Reading the rule** (fixed in the spec before any replay). A site converges before the ceiling
under a threshold T when some measured check before 30 s meets the other three criteria at their
defaults (coverage 0.8, region stability 0.95, mean confidence 10) with spread delta at most T.
The trajectory gives this for every T at once. The clause "no scene converges before the 50-frame
minimum" cannot fail as written, since no check is made before the minimum, so it was read as what
it guards against: a site fails it when its first converged check is the first measured one.

**kirk.** Each arm (`ceiling`: defaults and `settling_honest`; `delta-0.005` and `delta-0.01`: the
parameter sets [`l3-settling-delta-0005.json`](../../../config/experiments/l3-settling-delta-0005.json)
and [`l3-settling-delta-001.json`](../../../config/experiments/l3-settling-delta-001.json) with
`settling_honest`) replayed kirk0 whole and kirk1's 270 s trim at 6 s warm-up with an evidence
database per case and the trajectory written. kirk0 was scored by `lidar-ground-truth-eval
perframe` against the reviewed pack ad8b9438 (version 2), split `kirk0-tuning`, ignoring the
parked car `obj_788622b5`, each arm against `ceiling`. kirk1 has no per-frame ground truth, so, as
in l3bg0021, it was scored on its 49 labelled road users: covered when any track overlaps 30 % of a
reference's span.

## Results

### The trajectories, site by site

The spread delta is the mean absolute change in a cell's range spread between two checks, over the
observed cells. "Mean spread" is the mean range spread of the observed cells at the check nearest
30 s.

| Site                  | Spread delta at the first measured check | Median, 7–30 s | Lowest, 7–30 s | Median, 30–120 s | Lowest, 30–120 s | Mean spread at 30 s, m | Coverage at 30 s |
| --------------------- | ---------------------------------------: | -------------: | -------------: | ---------------: | ---------------: | ---------------------: | ---------------: |
| 1st-mission           |                                   0.0321 |         0.0299 |         0.0260 |           0.0283 |           0.0216 |                  0.393 |             0.98 |
| 3rd-folsom            |                                   0.0196 |         0.0146 |         0.0121 |           0.0149 |           0.0108 |                  0.156 |             0.98 |
| ashbury-downey        |                                   0.0032 |         0.0028 |         0.0025 |           0.0027 |           0.0023 |                  0.051 |             0.99 |
| broadway-franklin     |                                   0.0108 |         0.0078 |         0.0065 |           0.0076 |           0.0030 |                  0.142 |             0.98 |
| bush-powell           |                                   0.0064 |         0.0082 |         0.0059 |           0.0091 |           0.0067 |                  0.101 |             0.99 |
| california-leon-baker |                                   0.0031 |         0.0031 |         0.0024 |           0.0029 |           0.0018 |                  0.054 |             0.97 |
| columbus-broadway     |                                   0.0301 |         0.0261 |         0.0206 |           0.0263 |           0.0147 |                  0.217 |             0.97 |
| columbus-north-point  |                                   0.0379 |         0.0174 |         0.0115 |           0.0092 |           0.0066 |                  0.140 |             0.98 |
| embarcadero-bay       |                                   0.0110 |         0.0109 |         0.0094 |           0.0105 |           0.0094 |                  0.172 |             0.96 |
| embarcadero-broadway  |                                   0.0169 |         0.0164 |         0.0140 |           0.0166 |           0.0098 |                  0.210 |             0.93 |
| embarcadero-bryant    |                                   0.0153 |         0.0160 |         0.0138 |           0.0199 |           0.0161 |                  0.246 |             0.90 |
| franklin-mcallister   |                                   0.0147 |         0.0129 |         0.0112 |           0.0141 |           0.0116 |                  0.178 |             0.99 |
| fulton-divisadero     |                                   0.0151 |         0.0123 |         0.0082 |           0.0075 |           0.0058 |                  0.118 |             0.99 |
| haight-divisadero     |                                   0.0030 |         0.0112 |         0.0027 |           0.0049 |           0.0026 |                  0.114 |             0.98 |
| howard-6th            |                                   0.0098 |         0.0131 |         0.0098 |           0.0080 |           0.0057 |                  0.140 |             0.98 |
| hyde-ofarrell         |                                   0.0115 |         0.0082 |         0.0068 |           0.0105 |           0.0067 |                  0.105 |             0.99 |
| laguna-eddy           |                                   0.0063 |         0.0065 |         0.0055 |           0.0063 |           0.0049 |                  0.092 |             0.99 |
| lombard-broderick     |                                   0.0325 |         0.0145 |         0.0079 |           0.0171 |           0.0035 |                  0.111 |             0.98 |
| lombard-laguna        |                                   0.0136 |         0.0120 |         0.0090 |           0.0108 |           0.0072 |                  0.150 |             0.96 |
| marina-broderick      |                                   0.0073 |         0.0080 |         0.0070 |           0.0098 |           0.0083 |                  0.121 |             0.80 |
| marina-webster-beach  |                                   0.0089 |         0.0075 |         0.0068 |           0.0078 |           0.0057 |                  0.126 |             0.81 |
| pierce-haight         |                                   0.0121 |         0.0498 |         0.0068 |           0.0063 |           0.0045 |                  0.423 |             0.99 |
| van-ness-sacramento   |                                   0.0110 |         0.0137 |         0.0110 |           0.0241 |           0.0158 |                  0.219 |             0.99 |

### Thresholds

| Threshold | Sites converged before 30 s | Of them at the first measured check | Converged by 120 s | Median time, s |
| --------: | --------------------------: | ----------------------------------: | -----------------: | -------------: |
|     0.001 |                     0 of 23 |                                   0 |                  0 |              – |
|     0.002 |                     0 of 23 |                                   0 |                  1 |              – |
|     0.003 |                     3 of 23 |                                   0 |                  3 |            7.9 |
|     0.004 |                     3 of 23 |                                   3 |                  5 |            6.9 |
|     0.005 |                     3 of 23 |                                   3 |                  7 |            6.9 |
|    0.0075 |                    10 of 23 |                                   5 |                 15 |            7.9 |
|      0.01 |                    15 of 23 |                                   7 |                 17 |            7.9 |
|      0.02 |                    21 of 23 |                                  18 |                 22 |            6.9 |

The arms' thresholds are 0.001, 0.005 and 0.01; the others are read off the same trajectories to
show the shape. No threshold between 0.002 and 0.02 has 18 sites converge before 30 s without
most of them doing so at the first measured check.

### The relative form

The ratio of the spread delta to the mean spread, by site, is far narrower than the delta: its
median by site runs from 0.047 (california-leon-baker) to 0.084 (van-ness-sacramento), against
0.0028 to 0.050 for the delta. Read off the same trajectories:

| Ratio | Sites converged before 30 s | At the first measured check | Median time, s |
| ----: | --------------------------: | --------------------------: | -------------: |
|  0.05 |                     2 of 23 |                           0 |           25.4 |
| 0.075 |                    19 of 23 |                           0 |           16.9 |
|   0.1 |                    23 of 23 |                           5 |            7.9 |
|  0.15 |                    23 of 23 |                          17 |            6.9 |

### kirk0 and kirk1

| Arm           | kirk0 settles at | kirk0 HOTA |   IDF1 | ID switches | Fragmentations | False negatives | False positives | kirk1 settles at | kirk1 covered of 49 |
| ------------- | ---------------- | ---------: | -----: | ----------: | -------------: | --------------: | --------------: | ---------------- | ------------------: |
| `ceiling`     | 30 s (ceiling)   |     0.3936 | 0.4349 |          92 |             29 |            1091 |            3020 | 30 s (ceiling)   |                  41 |
| `delta-0.005` | 30 s (ceiling)   |     0.3936 | 0.4349 |          92 |             29 |            1091 |            3020 | 6.9 s            |                  49 |
| `delta-0.01`  | 6.9 s            |     0.4269 | 0.4678 |          82 |             48 |             530 |            4145 | 6.9 s            |                  49 |

All episodes of kirk0's tuning split. Each arm settled where its own trajectory said it would:
kirk0's first measured delta is 0.0052 and kirk1's 0.0036, so 0.005 converges kirk1 at once and
never kirk0. Under `ceiling` kirk0, a 59 s capture, has foreground for only its last 29 s, which
is why its false negatives double; the defaults, whose first check passes unmeasured at 5.9 s,
scored HOTA 0.4293 on kirk0 in [the November run](pcl-noise-and-splits-2026-11.md).

## The rule, applied

The rule (2026-10-18): a threshold is proposed as the default when at least 18 of the 23 scenes
converge before the ceiling under it, no scene converges before the 50-frame minimum, and kirk0's
and kirk1's HOTA under it are within 0.01 of the ceiling's.

| Arm               | Sites before 30 s (need 18) | Sites at the first check (need 0) | kirk0 HOTA against `ceiling` | kirk1 covered against `ceiling` | Verdict                                      |
| ----------------- | --------------------------: | --------------------------------: | ---------------------------- | ------------------------------- | -------------------------------------------- |
| `ceiling` (0.001) |                           0 |                                 0 | (the reference)              | (the reference)                 | Fails: no site converges                     |
| `delta-0.005`     |                           3 |                                 3 | +0.0000                      | 49 against 41                   | Fails: 3 of 18 sites, all at the first check |
| `delta-0.01`      |                          15 |                                 7 | +0.0333                      | 49 against 41                   | Fails: 15 of 18 sites, 7 at the first check  |

The kirk clause, read as a cost bound, holds for both thresholds: neither scores below the
ceiling. Read two-sided, `delta-0.01`'s gain of 0.033 would also fall outside it. Either way the
corpus clauses fail first. No absolute threshold meets the rule, so, as it directs, the report
describes the trajectories and proposes the relative form as a further spec,
[l3bg0030](../../../data/experiments/l3bg/l3bg0030.md). Nothing changes in the defaults, which settle on the current first check (l3bg0021), not the
honest one.

## What could mislead

- **A converged check is a quiet moment, not a settled background.** haight-divisadero converges
  under 0.005 at its first measured check (0.0030), though its median to 30 s is 0.011: traffic
  passing raises the delta again. Converging at the first measured check means the criterion
  waited for nothing.
- **The region criterion is vacuous during warm-up.** It reads 1.0 because no regions exist
  before settling completes, so a scene never fails it. The criterion as configured is the spread
  delta, coverage and nothing else.
- **The threshold may have been meant per frame.** The backlog records that
  `settling_max_spread_delta` is documented as a per-frame delta but evaluated between checks ten
  frames apart. A per-frame delta would be smaller, but not by a fixed factor, since changes in an
  averaged spread partly cancel; these trajectories measure the delta between checks only, as the
  criterion reads it, and cannot say whether 0.001 per frame would be met.
- **kirk1 HOTA was not measured.** It has no per-frame ground truth; its clause was read on
  covered road users, as l3bg0021 read it.

## What it does not establish

- That any scene's background is right when the criterion says it has converged. This measures
  when the grid's own statistics stop changing, not the background's accuracy.
- Whether a relative threshold costs anything on labelled captures: that is l3bg0030's question.
- Live behaviour. These are offline replays; a live start-up settles by the same code but under
  the current, not the honest, first check.
- Anything about wall time: arrow-worker ran another lane and the Gerardus server beside these
  replays, so no timing is read.

## Provenance

| Item           | Value                                                                                                                                                                                                                 |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Branch         | `exp/l3bg0022-convergence`, from main `84492f9e`                                                                                                                                                                      |
| Tools          | `lidar-state-estimation-baseline` and `lidar-ground-truth-eval` at `38ec89ca3` (the trajectory code; the later commits change only the spec and parameter sets), built with the Makefile's LDFLAGS and the `pcap` tag |
| Parameter sets | `config/tuning.defaults.json` SHA-256 `9ad7377f…`; `l3-settling-trajectory.json` `e2096d05…`; `l3-settling-delta-0005.json` `4daa8ef0…`; `l3-settling-delta-001.json` `014c1eea…`                                     |
| Replay option  | `settling_honest` in every arm and in the trajectories                                                                                                                                                                |
| Corpus         | first five-minute segment of 23 sites, the October 18 first-segment corpus and index (broadway-franklin appears there as broadway-gough, the same capture)                                                            |
| kirk           | kirk0 whole; kirk1's first 270 s (`kirk1-first270s.pcapng`); pack ad8b9438 version 2, split `kirk0-reference-tuning-r1`; kirk1 references `60a4774c`                                                                  |
| Host           | arrow-worker, 4 vCPU, 9 GB, shared with lane B and the Gerardus server                                                                                                                                                |
| Raw output     | `velocity-campaign/exp-20261103/laneA/l3bg0022/` on the results share                                                                                                                                                 |

## Recommendations

1. **Run [l3bg0030](../../../data/experiments/l3bg/l3bg0030.md)**: a convergence criterion on the
   spread delta relative to the mean spread, scored on kirk0 and held out on segments this run did
   not read.
2. **Drop or redefine the region-stability criterion** for warm-up: it cannot fail before regions
   exist.
3. **Leave the defaults' first check as it is** until a criterion exists that a scene can meet;
   0.001 cannot be met, so the honest check only ever waits out the ceiling.
