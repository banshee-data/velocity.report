# The geometry convergence candidate on held-out segments, October 2026

The [rectangle heading report](solid-body-rectangle-heading-2026-10.md) chose the current
candidate, the full arm with the rectangle fit weighed at 1.5 times its σ, on the first five-minute
segment of each corpus site, the same segment its σ scale was compared on. This run replays the
same four arms on each site's second five-minute segment, which no geometry screen has read, and
asks which of that report's findings hold on data the choice was not made on.

- **Status:** Measured, October 12, 2026. 14 of the 22 planned sites; the run was stopped when
  the machine ran short of disk. Nothing shipped changes
- **Layers:** L4 rectangle fit, L5 solid body
- **Related:** [convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md),
  [criteria record](geometry-convergence-criteria.md),
  [rectangle heading report](solid-body-rectangle-heading-2026-10.md),
  [containment screen](solid-body-containment-corpus-2026-10.md)

## Summary

- **The heading result holds.** On the held-out segments the candidate's share of rows more than
  30° off the course is 0.032 at two to four seconds and 0.022 after four, against 0.024 and
  0.026 on the first segments of the same 14 sites; seven sites meet G3's 0.05 at both ages
  against five, and the worst site is 0.081 after four seconds where the alignment candidate's
  is 0.183. A2's is 0.254 and 0.311. The median axis error falls from 5 to 6° under the
  alignment candidate to 1.5 to 1.7° at every one of the 14 sites at both ages.
- **The lateral margin over the alignment candidate shrinks to a third.** On the windows both
  arms score, the candidate's p99 is 0.166 m against 0.170 (9,156 windows); on the first
  segments of the same sites it was 0.173 against 0.185. The all-window p99 advantage the
  rectangle heading report counted (better at 10 of 14 sites here on the first segment) does not
  replicate: 7 worse, 7 better, median change +0.001 m.
- **σ × 1.5 against σ × 1: better on the same windows, not on each arm's own.** The scale
  improves the axis at 13 of 14 sites, and on the windows both score its p99 is lower on both
  segments (0.179 against 0.189 m held out, 10,589 windows). Across each arm's own windows it is
  worse than σ × 1 held out on the all-window p99 at 10 of 13 sites (+0.012 m) and the body-centre
  p99 at 9 of 14, where on the first segments the two were level: the 508 windows only σ × 1.5
  scores have a p99 of 0.343 m. The matched-window ordering, candidate below alignment below
  σ × 1, holds on both segments.
- **Containment and extents hold.** The share of body-centre rows holding their points rises
  from 0.644 to 0.710 (11 of 14 sites better), and the share of rows whose reported length or
  width falls short of the frame's span falls from 0.025 and 0.042 to 0.011 and 0.012, as on the
  first segments.

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
  held-out score; `ashbury-downey` has one 20-minute capture and no second segment. The run
  reached 14 of the remaining 22 sites, in corpus order: broadway-gough, california-leon-baker,
  columbus-broadway, columbus-north-point, embarcadero-bay, fulton-divisadero, haight-divisadero,
  laguna-eddy, lombard-broderick, lombard-laguna, marina-broderick, marina-webster-beach,
  pierce-haight and van-ness-sacramento. Every comparison below reads the same 14 sites on both
  segments; the first-segment figures are pass 8 and earlier's evidence at
  `physical-align-corpus-20261009`, re-read, so they differ slightly from the 23-site figures in
  the rectangle heading report.
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

**Heading (G3)**, the share of rows more than 30° off the course, median over the 14 sites:

| Arm                 | First segment, 2 to 4 s / after 4 s | Held out, 2 to 4 s / after 4 s | Sites at or under 0.05 at both, first / held out | Worst site after 4 s, held out |
| ------------------- | ----------------------------------- | ------------------------------ | ------------------------------------------------ | -----------------------------: |
| A2                  | 0.229 / 0.264 (`a2-cont`)           | 0.254 / 0.311                  | 0 / 0                                            |                          0.491 |
| Alignment candidate | 0.055 / 0.051                       | 0.024 / 0.060                  | 4 / 5                                            |                          0.183 |
| Candidate, σ × 1    | 0.035 / 0.050                       | 0.042 / 0.039                  | 4 / 4                                            |                          0.092 |
| Candidate, σ × 1.5  | 0.024 / 0.026                       | 0.032 / 0.022                  | 5 / 7                                            |                          0.081 |

Pass 1's A2 predates the axis table, so on the first segment A2's figure is `a2-cont`, which
leaves the heading alone, as in the rectangle heading report.

**Per-site changes**, median change and sites worse / better, first segment against held out:

| Measure                          | σ × 1.5 vs alignment, first | held out        | σ × 1 vs alignment, first | held out        | σ × 1.5 vs σ × 1, first | held out        |
| -------------------------------- | --------------------------- | --------------- | ------------------------- | --------------- | ----------------------- | --------------- |
| G3, 2 to 4 s                     | −0.010 (3 / 11)             | +0.006 (8 / 6)  | −0.001 (6 / 8)            | +0.018 (9 / 5)  | −0.009 (4 / 10)         | −0.008 (1 / 13) |
| G3, after 4 s                    | −0.005 (5 / 8)              | −0.038 (3 / 10) | +0.003 (7 / 6)            | −0.020 (4 / 9)  | −0.012 (3 / 10)         | −0.004 (4 / 9)  |
| Median axis error, 2 to 4 s (°)  | −3.9 (0 / 14)               | −3.4 (0 / 14)   | −2.9 (1 / 13)             | −2.8 (1 / 13)   | −0.5 (0 / 14)           | −0.5 (0 / 14)   |
| Median axis error, after 4 s (°) | −4.7 (0 / 14)               | −4.7 (0 / 14)   | −3.6 (0 / 14)             | −4.2 (0 / 14)   | −0.6 (0 / 14)           | −0.7 (0 / 14)   |
| Body-centre p99 (m)              | −0.010 (3 / 11)             | −0.014 (6 / 8)  | −0.008 (6 / 8)            | −0.004 (6 / 8)  | −0.006 (6 / 8)          | +0.006 (9 / 5)  |
| All-window p95 (m)               | −0.001 (7 / 7)              | +0.001 (8 / 6)  | +0.001 (7 / 7)            | −0.003 (7 / 7)  | +0.003 (9 / 5)          | +0.009 (9 / 5)  |
| All-window p99 (m)               | −0.013 (4 / 10)             | +0.001 (7 / 7)  | −0.012 (6 / 8)            | −0.022 (5 / 9)  | −0.001 (6 / 8)          | +0.012 (10 / 3) |
| Lapses                           | +8.5 (11 / 3)               | −1 (7 / 7)      | +6 (9 / 5)                | +1 (7 / 7)      | +3.5 (10 / 4)           | −1.5 (4 / 9)    |
| Held share                       | +0.052 (2 / 12)             | +0.040 (3 / 11) | +0.051 (2 / 12)           | +0.030 (2 / 12) | −0.002 (10 / 4)         | +0.008 (3 / 11) |

**On the same windows**, five-point body-centre windows matched across two arms by frame time and
position, pooled over the 14 sites:

| Pair                 | First segment: matched, p95, p99        | Held out: matched, p95, p99             |
| -------------------- | --------------------------------------- | --------------------------------------- |
| σ × 1.5 vs alignment | 8,252: 0.077 → 0.067 m, 0.185 → 0.173 m | 9,156: 0.077 → 0.073 m, 0.170 → 0.166 m |
| σ × 1 vs alignment   | 8,221: 0.075 → 0.070, 0.182 → 0.186     | 9,230: 0.076 → 0.075, 0.164 → 0.167     |
| σ × 1.5 vs σ × 1     | 9,428: 0.072 → 0.070, 0.185 → 0.180     | 10,589: 0.078 → 0.074, 0.189 → 0.179    |

**Against A2**, held out: the candidate halves the lapses at all 14 sites (median 175 to 77),
lifts the held share from 0.49 to 0.71 at all 14, and lowers the all-window p99 by 0.079 m at 13
of 14; the alignment candidate's figures are 175 to 85, 0.49 to 0.64 and 0.087 m at 12.

**Containment and extents**, corpus median (range) over the 14 sites:

| Guard                                 | Alignment, first | Alignment, held out | σ × 1.5, first | σ × 1.5, held out | σ × 1, held out |
| ------------------------------------- | ---------------- | ------------------- | -------------- | ----------------- | --------------- |
| Body-centre rows holding their points | 0.629            | 0.644               | 0.688          | 0.710             | 0.701           |
| All rows holding their points         | 0.691            | 0.694               | 0.714          | 0.719             | 0.711           |
| Reported length short of the span     | 0.020            | 0.025               | 0.011          | 0.011             | 0.012           |
| Reported width short of the span      | 0.042            | 0.042               | 0.011          | 0.012             | 0.009           |

## Interpretation

1. **The rectangle heading generalises.** Every heading figure the first segments showed comes
   back on segments nobody tuned on, and the worst sites improve most: after four seconds the
   candidate's worst site is 0.081 where the alignment candidate's is 0.183. Its 2 to 4 s median
   is 0.008 above the alignment candidate's held out, worse at 8 sites, a reminder that a young
   track's axis is still settling; G3 at both ages is met at more sites by the candidate than by
   any other arm, on both segments.
2. **The σ scale changes which windows are scored as much as how well.** On the windows both
   arms score, σ × 1.5 is better than σ × 1 on both segments, by 5 mm at p99 on the first and 10
   mm held out. On each arm's own windows the first segments showed the two level and the held-out
   segments show σ × 1.5 behind at two thirds of the sites, because the windows it alone scores,
   500 or so, carry a heavy tail (p99 0.343 m). The per-site population figures the σ choice was
   partly read from are therefore segment-sensitive, and the matched-window figure is the one that
   replicated. Neither scale is worse than the alignment candidate on matched windows by more than
   4 mm.
3. **The lateral case against the alignment candidate is level, not a win.** The candidate is
   ahead on heading, held share and extents everywhere, and level on lateral; that is the
   rectangle heading report's own reading at σ × 1, and held out it is also the reading at σ ×
   1.5.

## What this does not establish

- 14 of 22 sites, five minutes each, all from the same two days of the same city. The held-out
  segments are the next five minutes at the same placement: held out from the choice, not from
  the site, the weather or the traffic mix. `embarcadero-folsom`, the declared held-out site, was
  not read.
- Label-free, as every corpus screen is: G3 compares the axis with the course, not with a
  reference, and the lateral figures are a residual about a line fitted to the estimate itself.
- Identity (switches and IDF1 against the switch band) was not scored: there are no labels on
  these segments.
- Per-site medians over 14 sites move by a few millimetres between segments; differences of that
  order are inside the spread the two segments show for the same arm.

## Recommendations

- **Keep the candidate as the W1b heading source**: the heading result is the robust one.
- **Keep σ × 1.5, and read lateral on matched windows.** It leads σ × 1 on heading everywhere
  and on matched windows on both segments; the per-site population tails moved with the segment.
  The windows only σ × 1.5 scores, with their 0.34 m p99, are worth a look: they are where its
  body-centre claim survives and σ × 1's does not.
- **Run held-out segments as routine** for any corpus choice: the parameter sets make it a
  one-file change to the runner, and this run cost about 10 minutes a site with the machine shared.

## Provenance

- Tree `27543717a` (branch `claude/experiment-planning-execution-db66fa`) for the replays,
  `lidar-state-estimation-baseline` and `lidar-track-scorecard` built with the Makefile's version
  stamp (`0.5.1-pre45`, git sha set) and `-tags=pcap`; summaries rebuilt at `190c23b03`, which
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
