# The geometry convergence candidate on held-out segments, October 2026

The [rectangle heading report](solid-body-rectangle-heading-2026-10.md) chose the current
candidate, the full arm with the rectangle fit weighed at 1.5 times its σ, on the first five-minute
segment of each corpus site, the same segment its σ scale was compared on. This run replays the
same four arms on each site's second five-minute segment, which no geometry screen has read, and
asks which of that report's findings hold on data the choice was not made on.

- **Status:** Measured, October 12, 2026, on all 22 sites: 14 in the unattended run and 8 the
  next day from the same build and files, after the run was paused for disk. Nothing shipped
  changes
- **Layers:** L4 rectangle fit, L5 solid body
- **Related:** [convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md),
  [criteria record](geometry-convergence-criteria.md),
  [rectangle heading report](solid-body-rectangle-heading-2026-10.md),
  [containment screen](solid-body-containment-corpus-2026-10.md)

## Summary

- **The heading result holds.** On the held-out segments the candidate's share of rows more than
  30° off the course is 0.032 at two to four seconds and 0.022 after four, against 0.025 and 0.035
  on the first segments; 12 of the 22 sites meet G3's 0.05 at both ages, against 9 on the first
  segments and 10 for the alignment candidate held out, and the worst site after four seconds is
  0.093 where the alignment candidate's is 0.183. A2's is 0.261 and 0.266. The median axis error
  falls from 5.2 to 5.9° under the alignment candidate to 1.6 to 1.7° at 21 and 22 of 22 sites.
- **Young tracks are the exception.** Between two and four seconds the candidate's G3 is above the
  alignment candidate's at 14 of 22 held-out sites (+0.010 at the median), where on the first
  segments it was below at 15; after four seconds it is below at 14. The young track's axis is
  where W1b's unbuilt low-speed label and ambiguous row would act.
- **The lateral margin over the alignment candidate holds at two thirds on the same windows, not
  per site.** On the windows both arms score, the candidate's p99 is 0.164 m against 0.172
  (14,620 windows); on the first segments it was 0.162 against 0.174. The per-site all-window p99
  advantage the first segments showed (better at 14 of 22) does not replicate: 11 worse, 11
  better, median change +0.001 m.
- **σ × 1.5 against σ × 1: better on the same windows, not on each arm's own.** The scale improves
  the axis at 18 to 22 sites, and on the windows both score its p99 is lower on both segments
  (0.185 against 0.192 m held out, 16,360 windows). Across each arm's own windows it is worse than
  σ × 1 held out on the all-window p99 at 13 of 21 sites (+0.011 m), because σ × 1 keeps 1,768
  body-centre windows that σ × 1.5 does not, with a lighter tail (p99 0.302 against the 838
  windows only σ × 1.5 keeps, at 0.331). σ × 1 is level with the alignment candidate on matched
  windows on both segments.
- **Containment and extents hold.** The share of body-centre rows holding their points rises from
  0.662 to 0.715 (18 of 22 sites better), and the share of rows whose reported length or width
  falls short of the frame's span falls from 0.023 and 0.040 to 0.011 and 0.009, as on the first
  segments.

## Method

- **Arms**, each from its checked-in parameter set (the replay names no experiments):

  | Arm             | Parameter set                                                                                          | What it is                                                  |
  | --------------- | ------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------- |
  | `a2`            | [solid-body-a2.json](../../../config/experiments/solid-body-a2.json)                                   | A2                                                          |
  | `a2-allo-cont`  | [alignment-candidate.json](../../../config/experiments/alignment-candidate.json)                       | The alignment candidate                                     |
  | `a2-new-cf-mid` | [geometry-convergence-candidate.json](../../../config/experiments/geometry-convergence-candidate.json) | The current candidate, σ × 1.5                              |
  | `a2-new-cf`     | [geometry-convergence-sigma1.json](../../../config/experiments/geometry-convergence-sigma1.json)       | The candidate with the fit at its own σ, built for this run |

- **Data.** For each site of the D2 first-segment corpus, its second five-minute capture from
  [tools/s2-archive/site-index.json](../../../tools/s2-archive/site-index.json), 200 s scored after
  a 70 s warm-up, as every corpus screen ran. `embarcadero-folsom` stays held out for a frozen
  held-out score; `ashbury-downey` has one 20-minute capture and no second segment, which leaves
  22 sites. Every comparison below reads the same 22 sites on both segments; the first-segment
  figures are pass 8 and earlier's evidence at `physical-align-corpus-20261009`, re-read without
  `ashbury-downey`, so they differ slightly from the 23-site figures in the rectangle heading
  report.
- **Equivalence.** The candidate replayed from its parameter set on 3rd-folsom's first segment
  gives a scorecard identical to the same arm named as experiments, and both reproduce pass 8's
  row exactly (7,914 solid bodies, 100 lapses, body-centre p99 0.180 m over 556 windows), so the
  arms here and the first-segment arms are the same estimators.
- **A summary rebuilt.** The harness wrote the solid-body section of its summary only when the
  `solid_body` experiment was named, so these parameter-set runs recorded their solid bodies but
  had no section; fixed at `190c23b03`. Each arm's section was rebuilt from its evidence with the
  harness's own function, which on 3rd-folsom reproduces the named arm's section exactly. The
  corpus scripts of the earlier screens (`g3-sites.py`, `heading-summary.py`, `summary2.py`,
  `paired.py`) then ran unchanged on both segments.

## Findings

**Heading (G3)**, the share of rows more than 30° off the course, median over the 22 sites:

| Arm                 | First segment, 2 to 4 s / after 4 s | Held out, 2 to 4 s / after 4 s | Sites at or under 0.05 at both, first / held out | Worst site after 4 s, held out |
| ------------------- | ----------------------------------- | ------------------------------ | ------------------------------------------------ | -----------------------------: |
| A2                  | 0.229 / 0.262 (`a2-cont`)           | 0.261 / 0.266                  | 0 / 0                                            |                          0.491 |
| Alignment candidate | 0.046 / 0.051                       | 0.024 / 0.047                  | 8 / 10                                           |                          0.183 |
| Candidate, σ × 1    | 0.041 / 0.045                       | 0.046 / 0.039                  | 9 / 9                                            |                          0.096 |
| Candidate, σ × 1.5  | 0.025 / 0.035                       | 0.032 / 0.022                  | 9 / 12                                           |                          0.093 |

Pass 1's A2 predates the axis table, so on the first segment A2's figure is `a2-cont`, which
leaves the heading alone, as in the rectangle heading report.

**Per-site changes**, median change and sites worse / better, first segment against held out:

| Measure                          | σ × 1.5 vs alignment, first | held out         | σ × 1 vs alignment, first | held out         | σ × 1.5 vs σ × 1, first | held out         |
| -------------------------------- | --------------------------- | ---------------- | ------------------------- | ---------------- | ----------------------- | ---------------- |
| G3, 2 to 4 s                     | −0.009 (7 / 15)             | +0.010 (14 / 8)  | −0.001 (10 / 12)          | +0.021 (15 / 7)  | −0.008 (7 / 15)         | −0.007 (3 / 18)  |
| G3, after 4 s                    | −0.002 (10 / 11)            | −0.016 (7 / 14)  | +0.003 (12 / 9)           | −0.010 (9 / 12)  | −0.009 (6 / 15)         | −0.004 (5 / 15)  |
| Median axis error, 2 to 4 s (°)  | −3.1 (0 / 22)               | −3.3 (1 / 21)    | −2.6 (1 / 21)             | −2.8 (2 / 20)    | −0.5 (0 / 22)           | −0.5 (0 / 22)    |
| Median axis error, after 4 s (°) | −4.4 (0 / 22)               | −4.2 (0 / 22)    | −3.4 (1 / 21)             | −3.7 (0 / 22)    | −0.5 (1 / 21)           | −0.6 (2 / 20)    |
| Body-centre p99 (m)              | −0.003 (8 / 14)             | −0.002 (11 / 11) | +0.003 (11 / 11)          | −0.001 (11 / 11) | −0.003 (9 / 13)         | +0.003 (12 / 10) |
| All-window p95 (m)               | −0.002 (9 / 13)             | +0.001 (13 / 9)  | −0.003 (10 / 12)          | −0.003 (10 / 12) | +0.001 (12 / 10)        | +0.007 (13 / 9)  |
| All-window p99 (m)               | −0.013 (8 / 14)             | +0.001 (11 / 11) | −0.013 (8 / 14)           | −0.016 (9 / 13)  | −0.002 (9 / 13)         | +0.011 (13 / 8)  |
| Lapses                           | +4.5 (16 / 5)               | +1 (12 / 10)     | +5 (16 / 6)               | +2.5 (13 / 8)    | +1 (12 / 9)             | −1 (8 / 12)      |
| Held share                       | +0.041 (2 / 20)             | +0.043 (4 / 18)  | +0.039 (2 / 20)           | +0.034 (3 / 19)  | −0.003 (15 / 7)         | +0.006 (6 / 16)  |

**On the same windows**, five-point body-centre windows matched across two arms by frame time and
position, pooled over the 22 sites:

| Pair                 | First segment: matched, p95, p99         | Held out: matched, p95, p99              | Windows only the arm scores, held out, p99    |
| -------------------- | ---------------------------------------- | ---------------------------------------- | --------------------------------------------- |
| σ × 1.5 vs alignment | 14,858: 0.074 → 0.069 m, 0.174 → 0.162 m | 14,620: 0.076 → 0.072 m, 0.172 → 0.164 m | 2,578 at 0.346 m (alignment's 2,172 at 0.286) |
| σ × 1 vs alignment   | 14,788: 0.074 → 0.071, 0.169 → 0.167     | 14,709: 0.075 → 0.074, 0.168 → 0.167     | 3,419 at 0.332 (alignment's 2,083 at 0.321)   |
| σ × 1.5 vs σ × 1     | 16,469: 0.075 → 0.072, 0.176 → 0.174     | 16,360: 0.079 → 0.076, 0.192 → 0.185     | 838 at 0.331 (σ × 1's 1,768 at 0.302)         |

**Against A2**, held out: the candidate halves the lapses at all 22 sites (median 182.5 to 83),
lifts the held share from 0.49 to 0.72 at all 22, and lowers the all-window p99 by 0.081 m at 21;
the alignment candidate's figures are 182.5 to 87, 0.49 to 0.66 and 0.087 m at 19.

**Containment and extents**, corpus median over the 22 sites:

| Guard                                 | Alignment, first | Alignment, held out | σ × 1.5, first | σ × 1.5, held out | σ × 1, held out |
| ------------------------------------- | ---------------- | ------------------- | -------------- | ----------------- | --------------- |
| Body-centre rows holding their points | 0.667            | 0.662               | 0.704          | 0.715             | 0.707           |
| All rows holding their points         | 0.714            | 0.701               | 0.728          | 0.725             | 0.725           |
| Reported length short of the span     | 0.019            | 0.023               | 0.012          | 0.011             | 0.012           |
| Reported width short of the span      | 0.037            | 0.040               | 0.011          | 0.009             | 0.009           |

## Interpretation

1. **The rectangle heading generalises after four seconds, and the worst sites improve most.**
   Every heading figure for tracks older than four seconds comes back on segments nobody tuned on,
   and the worst site falls from the alignment candidate's 0.183 to 0.093. Between two and four
   seconds the candidate is above the alignment candidate at 14 sites held out, where the course
   heading the alignment candidate uses is already good on a straight pass; a young track's
   fitted axis is still settling, which is what W1b's low-speed label and ambiguous row are for.
2. **On the same windows the lateral ordering replicates; per site it does not.** The candidate
   leads the alignment candidate on matched windows on both segments, by 12 mm at p99 on the
   first and 8 mm held out, and σ × 1.5 leads σ × 1 by 2 and 7 mm. The per-site population tails
   moved with the segment: the windows each arm alone scores carry heavy, arm-specific tails (p99
   0.29 to 0.35 m), and which of them an arm keeps decides its own-window p99 more than the
   estimate on the shared windows does. The σ choice was partly read from those per-site tails;
   the matched-window figure is the one that replicated.
3. **The lateral case against the alignment candidate is a small win on the same windows and
   level per site**, with the candidate ahead on heading after four seconds, held share and
   extents at nearly every site.

## What this does not establish

- 22 sites, five minutes each, from the same two days of the same city. The held-out segments are
  the next five minutes at the same placement: held out from the choice, not from the site, the
  weather or the traffic mix. `embarcadero-folsom`, the declared held-out site, was not read.
- Label-free, as every corpus screen is: G3 compares the axis with the course, not with a
  reference, and the lateral figures are a residual about a line fitted to the estimate itself.
- Identity (switches and IDF1 against the switch band) was not scored: there are no labels on
  these segments.
- Per-site medians move by a few millimetres between segments; differences of that order are
  inside the spread the two segments show for the same arm.
- Since measured ([corpus pose sweep](l3-corpus-pose-sweep-2026-10.md)): three second-segment
  cases are scored across a knock to the sensor (haight-divisadero at 152 s, marina-webster-beach
  at 145 s, van-ness-sacramento from its start), which this report did not know.

## Recommendations

- **Keep the candidate as the W1b heading source**, and keep σ × 1.5: it leads σ × 1 on heading at
  nearly every site and on matched windows on both segments.
- **Read lateral on matched windows** in future screens, with the per-site tails beside them as a
  population check rather than the deciding figure.
- **Look at the 2 to 4 s axis** when the low-speed label and ambiguous row are built: it is the
  one heading figure that did not replicate, and the place they act.
- **Run held-out segments as routine** for any corpus choice: the parameter sets make it a
  one-file change to the runner, and a site costs about 5 minutes on an otherwise idle Mac and 10
  to 14 with the machine shared.

## Provenance

- Tree `27543717a` (branch `claude/experiment-planning-execution-db66fa`) for the replays,
  `lidar-state-estimation-baseline` and `lidar-track-scorecard` built with the Makefile's version
  stamp (`0.5.1-pre45`, git sha set) and `-tags=pcap`; summaries rebuilt at `190c23b03` (the first 14
  sites) and at `d519cc645` (the last 8), whose solid-body summary code is the same; `190c23b03`
  differs from `27543717a` only in the harness's summary condition and its test.
- Parameter sets and fingerprints: `solid-body-a2.json` `1fb29f8bea7f`,
  `alignment-candidate.json` `f55b8ad99081`, `geometry-convergence-candidate.json`
  `6d86dd759ee6`, `geometry-convergence-sigma1.json` `6953e997531d`.
- Flags: `-duration 200 -warmup 70 -sample-points 256 -pcap-root /Volumes/lidar/lidar/pcaps`,
  one case per run; each replay repeat-verified by the harness.
- Inputs: a second-segment corpus and site index written outside the repository from the D2
  first-segment corpus and `tools/s2-archive/site-index.json` (each site's `captures[1]`);
  first-segment comparisons from `physical-align-corpus-20261009` on the LiDAR volume.
- Evidence (observations databases, recordings, summaries and scorecards) for each site and arm
  is under `work/velocity-campaign/blend-20261011/heldout-geometry/` on the LiDAR volume; the
  analysis scripts are the alignment run's corpus scripts, unchanged.
