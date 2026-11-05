---
id: figure0003
title: "The interval on a survey's figures: how wide p85 and p98 are at a segment's track count, and whether a second segment of the same site falls inside it"
status: proposed
owner: david
created: 2026-11-05
asks:
  - docs/research/2026-11-05-wayve-learned-safety-series.md#proposed-experiments
  - docs/plans/lidar-behaviour-analytics-plan.md#13-open-research-questions
depends_on: [figure0001]
requires:
  capabilities: [pcap:s2]
reclaim: any
cases:
  corpus: d2-first-segment
  count: 23
steps:
  - id: implement
    kind: manual:implement
    estimate: { human: 3h }
    note: >-
      scripts/lidar_figure_interval.py, beside figure0001's lidar_figure_sensitivity.py and
      reading its moving tracks and per-track maximum speeds: per site and arm, a seeded bootstrap
      over tracks (2,000 resamples) giving the 5th to 95th percentile of the nearest-rank p85 and
      p98, and the exact order-statistic interval beside it; the same on the third segment where
      archived; the pooled two-segment figure and its interval; the half-width at subsamples of
      10, 20, 50, 100 and 200 tracks; fixture tests on a ten-track and a 200-track population with
      known quantiles, and on a site with under ten moving tracks
  - id: rescore
    kind: corpus_screen
    over: [cases]
    estimate: { each: 3m, worker: m1pro }
    note: >-
      no capture is read: l5id0012's archived rows (a0 and overlap-end, the first segment of 23
      sites and the third of 22) on the results share, read where they are, as figure0001 reads
      them
  - id: write
    kind: manual:write
    estimate: { human: 2h }
---

# figure0003: the interval on a survey's figures

**Question.** The product publishes a vehicle count and the p85 and p98 of vehicle maximum speeds
with no interval. figure0001 asks how far those figures move between pipeline arms; nothing asks
how far they would move if the same traffic were sampled again. At the number of moving tracks a
five-minute segment holds, how wide are p85 and p98? And does the third segment of the same site,
recorded minutes later, fall inside the first segment's interval, or do two periods of one site
differ by more than sampling explains? The second question is the one Wayve's evidence transfer
assumes away ("similar models will exhibit consistent event rates"); the
[Wayve series review](../../../docs/research/2026-11-05-wayve-learned-safety-series.md) refuses to
pool a prior period into a published figure on principle, and this experiment says whether the
data agree.

**Definitions, fixed before any row is read (2026-11-05).**

- **Moving track and a track's maximum speed:** figure0001's definitions, unchanged.
- **Percentile:** nearest rank over the moving tracks' maximum speeds, the rule of
  [D-18](../../../docs/DECISIONS.md#d-18--speed-percentile-aggregation-semantics).
- **Bootstrap interval:** resample the segment's moving tracks with replacement 2,000 times, seeded,
  recompute p85 and p98 on each resample, and take the 5th and 95th percentiles of each. A track is
  the unit, not a frame.
- **Order-statistic interval:** the exact 90 % interval on the population quantile from the
  binomial distribution of ranks at the segment's track count, read as the two order statistics
  that bracket it. It checks the bootstrap and needs no resampling.
- **Half-width:** half the difference between the interval's ends, in m/s.
- **Inside:** the third segment's nearest-rank p85 (or p98) lies between the ends of the first
  segment's bootstrap interval.
- **Pooled figure:** the nearest-rank percentile over the union of both segments' moving tracks,
  with its own bootstrap interval.
- **A site with under ten moving tracks** in either segment is reported but excluded from the
  shares and medians.

**Rule (2026-11-05).** A measurement; it decides no default. The readings are:

1. Per site and arm, the half-width of p85 and p98 at the segment's track count, by both methods;
   the median site and the widest; and the half-width against the subsample sizes, so the report
   can state the track count at which the half-width falls below 0.5 m/s, if any segment reaches
   it. This is the first measurement toward the behaviour plan's open question B6.
2. Over the 22 sites with two archived segments, the share whose third-segment p85 and p98 lie
   inside the first segment's interval. The report states the share and names the sites outside
   it. If more than a quarter of the sites fall outside on either percentile, a prior period is
   not a prior for the present one at these sites, and the review's refusal stands on data; if
   nearly all fall inside, the refusal stands on principle alone and the report says so.
3. Per site, the pooled figure and interval against each segment's own: how much the pooled
   interval narrows, and how far the pooled figure sits from either segment's. The report prints
   both so a reader sees what pooling would hide.

The count carries no sampling interval. Its uncertainty is systematic: under the defaults about one
track in four shares its vehicle (l5id0012), and figure0001 reads how far the count moves between
arms. Resampling tracks would manufacture an interval that says nothing about either, so none is
reported.

**What could mislead.** The two segments are the first and third five minutes of one recording, so
they share the traffic state of one afternoon; agreement between them is weaker evidence of
stationarity than disagreement is of its absence, and the report says which it found. A bootstrap
over a few dozen tracks is itself uncertain, which is why the order-statistic interval is printed
beside it. The maxima are the published box's speeds (figure0002 found the per-row speed held a
running mean, but the per-track maximum was unaffected); a bias common to every track moves both
segments together and is invisible here. Sites where the overlap end removes duplicates have fewer
tracks under that arm, so its intervals are wider for a reason the reading must not call noise.

**What it does not establish.** How far either figure is from the vehicles' speeds (E17); anything
about periods further apart than the minutes between the segments, which is what a before-and-after
survey compares; the interval on the count.
