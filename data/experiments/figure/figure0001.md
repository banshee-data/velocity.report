---
id: figure0001
title: "How much the survey's figures move with the pipeline: vehicle count, p85 and p98 by site under every archived arm"
status: proposed
owner: david
created: 2026-11-05
asks:
  - docs/lidar/operations/experiment-review-2026-11-05.md#e-confidence-in-the-outputs
requires:
  capabilities: [pcap:s2]
reclaim: any
cases:
  corpus: d2-first-segment
  count: 23
steps:
  - id: implement
    kind: manual:implement
    estimate: { human: 2h }
    note: >-
      scripts/lidar_figure_sensitivity.py: from a -rows CSV or an evidence database, the moving
      tracks (l5id0012's definition), each track's maximum speed by the definition below, and per
      site the count, p85 and p98 of those maxima; two arms compared per site; fixture tests for
      the maximum-speed definition and for a site with under ten moving tracks
  - id: rescore
    kind: corpus_screen
    over: [cases]
    estimate: { each: 2m, worker: m1pro }
    note: >-
      no capture is read: l5id0012's archived rows (a0 and overlap-end, the first segment of 23
      sites and the third of 22) on the results share, and the pcl-20261102 evidence (a0, veil,
      radius-graph, body-older-tentative and the g1 to g4 gates on the first and fourth captures)
      on hydra's LiDAR volume, read where they are
  - id: write
    kind: manual:write
    estimate: { human: 2h }
---

# figure0001: how much the survey's figures move with the pipeline

**Question.** The product publishes a vehicle count and the p85 and p98 of vehicle maximum
speeds. Eight arms have been replayed on the corpus this autumn and compared on duplicates,
clutter, retention and identity, and no report has asked what any of them does to the figures a
survey prints. The rows and evidence are archived. Site by site, how far do the count, p85 and
p98 move between each arm and the defaults, and which stability work do the headline figures
depend on?

**Definitions, fixed before any row is read (2026-11-05).**

- **Moving track:** l5id0012's: lasting 2 s or more within the scored window, 5 m or more first to
  last, median speed 1 m/s or more.
- **A track's maximum speed:** the largest value of the rolling median of its row speeds over a 1 s
  window (at least three rows), so one frame's spike is not a maximum. A track with fewer than
  three rows has none and is excluded from the percentiles but counted.
- **Per site and arm:** the count of moving tracks; the p85 and p98 (nearest rank) of their
  maximum speeds; each as the arm's figure less `a0`'s, in vehicles and in m/s.
- **A site with under ten moving tracks** is reported but excluded from the medians.

**Rule (2026-11-05).** A measurement; the readings are:

1. For each arm, the median site's |Δcount| as a share of `a0`'s count, |Δp85| and |Δp98| in m/s,
   and the worst site of each. An arm whose worst site is within 2 % on the count and within
   0.2 m/s on p85 and p98 does not touch the headline figures; the stability work it represents
   is confidence work, not figure work, and the report says so.
2. An arm that moves the headline past those bounds at any site names the figure and the site;
   the review that follows ranks that stability work by the figure it moves.
3. The overlap end's figures are read on both sets (first and third segments) and compared: a
   change that holds on both is the option's; one that does not is a site effect.

No default changes. The bounds are reporting bounds, not acceptance: nothing here says a figure
is right, only how far it moves.

**What could mislead.** `a0` is not truth: an arm that lowers a count by removing duplicates moves
the figure toward the vehicles, and the reading cannot tell that from losing vehicles; l5id0013's
footprint read does. The percentiles over a five-minute segment at a quiet site rest on a few
tracks. The rows are the published box's rows; a track's speed is the filter's, so a young track's
spike is what the rolling median removes, and a slow truck's maximum is its real one.

**What it does not establish.** How far any figure is from the vehicles' speeds, which needs an
independent reference the repository does not hold (the review's E17); anything about counts at
range beyond what the pipeline sees.
