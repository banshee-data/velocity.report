# The L3 region map: where road users and the overrides-off flood fall, October 2026

The [region override attribution](l3-region-override-attribution-kirk-2026-10.md) found that the
volatile tercile's ×8 window both absorbs road users and holds back the flood of clusters the
overrides-off arm produces, and inferred why: a cell on a lane through which traffic passed while
the grid settled has a wide settling spread, so lanes land in the volatile tercile with the
vegetation. That run could not see the region map. This one records it in the replay evidence and
places kirk0's reviewed road users, and the overrides-off arm's extra clusters on kirk0 and kirk1,
on it.

- **Status:** Measured, October 13, 2026 (experiment
  [l3bg0007](../../../data/experiments/l3bg/l3bg0007.md)). Nothing shipped changes
- **Layers:** L3 background, region identification and overrides
- **Related:** [region override attribution](l3-region-override-attribution-kirk-2026-10.md),
  [beam-wise background subtraction review](../../../data/maths/proposals/20261011-beamwise-background-subtraction-review.md),
  [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md),
  [dwell policy](l3-dwell-policy-arrival-2026-10.md)

## Summary

- **The flood half of the reading holds.** Of the extra unlabelled cluster returns the
  overrides-off arm makes, 96% on kirk0 and 91% on kirk1 fall in cells of volatile regions, which
  are 40% and 43% of the seen cells; on kirk0 only 0.5% of the flood's cells are in stable
  regions. Switching the volatile window off is what floods.
- **The lane half does not.** Moving road users were almost absent while kirk0's grid settled:
  6,064 of their 829,732 returns (0.7%) fall in the 5.9 s before the regions were identified, and
  the 3,044 cells those returns touched ended up 99% in stable regions, with a lower settling
  spread than cells nobody crossed. Traffic during settling did not make kirk0's lanes volatile.
- **Lanes are volatile anyway, for what is behind them.** The cells moving road users cross after
  settling are in volatile regions at 47% against 37% for cells nobody crosses, and never in a
  stable region; by their own spread, before regions are merged, 42% are volatile and 15% stable.
  Their settling spread is higher without traffic in it (median variance 0.0038 against 0.0029),
  so it is the surface's own: a road user's returns land in a volatile cell 67% of the time in
  front of an above-ground surface and 23% in front of the road.
- **So recommendation 4 of the attribution report would not separate them.** A settling spread
  that excludes foreground returns would change the 3,044 cells the settling traffic touched,
  which are stable already. The flood and the lanes share the volatile tercile because both lie
  over variable surfaces, not because of traffic.

## Method

- **Recording the map.** The replay harness now writes the background grid and the region
  snapshot into its evidence database twice: when regions are identified (the map the overrides
  use for the rest of the replay) and at the end, both stamped with capture time, through the
  existing persistence path. `lidar-region-map` turns a snapshot into one row per cell: ring,
  azimuth bin, elevation and azimuth, region and its tercile, the cell's own tercile and settling
  variance, settled range and x, y, z. The snapshot gained each region's tercile and each cell's;
  restore does not read them, so the overrides behave as before. The replay summary, tracking
  baselines and manifest are byte-identical to a build without the change on kirk0.
- **Arms.** The shipped defaults (a0), and the same with the region overrides off (`off`:
  `config/tuning.defaults.json` with the replay option `no_region_overrides`), on kirk0 (83 s) and
  kirk1's first 270 s, `-warmup 6 -duration 0 -sample-points 256`.
- **Placing a return in a cell.** By its direction: the ring of nearest elevation and the azimuth
  bin of nearest azimuth, both read from the map's own cell geometry. As a check, 97.8% (kirk0)
  and 94.7% (kirk1) of the shipped arm's foreground returns lie more than 0.3 m in front of the
  settled range of the cell they are placed in.
- **Road users.** The reviewed masks of kirk0's pack ad8b9438 (version 1), road-user classes only
  (car, truck, bus, pedestrian; its ground and noise masks are excluded): 1,724,380 returns in
  32,408 cells. An object is moving when the 10th-to-90th-percentile spread of its mask centres
  exceeds 3 m; four objects, including the parked car, are static. Returns at or before the
  identification snapshot's capture time are "during settling".
- **The flood.** Per cell, the off arm's retained cluster returns less the shipped arm's, both
  with the reviewed road-user returns removed by exact float32 identity (kirk0); on kirk1, which
  has no point-level labels, every retained return.
- **Height.** A cell is ground when its settled return lies within 0.5 m of the road, 2.3 m below
  the sensor.

## Findings

The map (identified after 59 settling frames, 5.9 s):

| Capture | Seen cells | Regions | Volatile regions | Volatile share: own spread | Volatile share: region |
| ------- | ---------: | ------: | ---------------: | -------------------------: | ---------------------: |
| kirk0   |     69,951 |      50 |               16 |                      33.3% |                  40.4% |
| kirk1   |     69,899 |      50 |               18 |                      33.3% |                  43.2% |

Merging cells into regions moves about 5,000 of kirk0's cells into volatile regions that their
own spread would not put there.

Where kirk0's road users fall (shares of distinct cells, region tercile unless stated):

| Cells                        |  Cells | Volatile | Stable | Own volatile | Own stable | Median settling variance |
| ---------------------------- | -----: | -------: | -----: | -----------: | ---------: | -----------------------: |
| All seen                     | 69,951 |    40.4% |  33.4% |        33.3% |      33.3% |                        — |
| Crossed by any road user     | 32,408 |    44.5% |   9.3% |        39.8% |      20.1% |                  0.00370 |
| Crossed by moving road users | 30,146 |    42.0% |  10.0% |        38.6% |      20.6% |                  0.00366 |
| … during settling            |  3,044 |     1.1% |  98.9% |         6.4% |      71.8% |                  0.00226 |
| … only after settling        | 27,102 |    46.6% |   0.0% |        42.2% |      14.9% |                  0.00383 |
| Never crossed                | 37,725 |    36.7% |      — |            — |          — |                  0.00292 |

Weighted by returns, 53.5% of all road-user returns and 45.2% of moving road users' returns land in
volatile-region cells.

Where the overrides-off flood falls:

| Capture | Excess returns |  Cells | Volatile regions (returns) | Volatile regions (cells) | Stable regions (cells) | Own-spread volatile (returns) | Returns on ground cells |
| ------- | -------------: | -----: | -------------------------: | -----------------------: | ---------------------: | ----------------------------: | ----------------------: |
| kirk0   |        467,698 | 13,964 |                      96.1% |                    80.4% |                   0.5% |                         65.4% |                   59.9% |
| kirk1   |      1,792,165 | 26,112 |                      91.0% |                    71.1% |                  14.7% |                         59.9% |                   38.8% |

By height on kirk0: 22% of seen cells are ground, and 56% of those are in volatile regions against
36% of the cells above the ground. Half the moving road users' returns land in ground cells, where
23% are volatile; the other half land in front of above-ground surfaces, where 67% are.

## Interpretation

The volatile window is doing one job with two effects. The cells it covers are the ones whose
background varies (vegetation, walls and kerbs at grazing incidence, the road surface far out),
and the off arm's flood is those surfaces' own variation, so a window wide enough to hold them back
also takes a road user passing in front of them. The attribution report's reading put the road
users there for a different reason, traffic during settling, and on kirk0 that reason is absent:
the start of the capture was nearly empty of moving traffic, and what it did hold (parked cars and
standing pedestrians) made the cells it covered steadier, not wider.

Two features of the region identification widen the overlap: merging pulls one in seven of the
lane cells that are stable by their own spread into variable or volatile regions, and the window is
relative, so at 20 m a ×8 window is several metres deep. Either is a more promising lever than the
settling spread's treatment of foreground.

## What this does not establish

- **A busy settling window.** kirk0's first six seconds were quiet. A deployment settles once at
  boot with whatever traffic is passing, and a busy start could still widen lane cells; this run
  shows that it is not why kirk0's lanes are volatile, not that it never happens.
- **Labels beyond kirk0.** kirk1 has no point-level masks, so its lane half is not measured; its
  flood half replicates.
- **Exact cell placement.** Returns are placed by nearest ring and bin; 2 to 5% of the shipped
  arm's foreground returns do not sit in front of their cell's settled range.
- **A tuning split.** kirk0's pack split is a tuning split, and one capture of 83 s.
- **What the next arm should be.** The levers named below are candidates, not measured.

## Recommendations

- **Correct the attribution report's mechanism**: the volatile tercile holds the flood (measured),
  and the lanes are there for their backgrounds' variation and region merging, not traffic during
  settling (measured on kirk0).
- **Do not pursue recommendation 4 as the fix for lanes.** Excluding foreground from the settling
  spread is still sound hygiene for a busy boot, but on kirk0 it would move nothing.
- **Try the two levers this run points at**, each as an arm: overrides by each cell's own tercile
  rather than its merged region's, and a volatile window bounded in metres rather than relative to
  range. Score both on kirk0 and kirk1 with W3 and the flood count. Since measured
  ([levers](l3-region-levers-kirk-2026-10.md)): the metre cap lets half the flood back, and the
  cell's own tercile recovers pedestrians at a small cost alone but trails CMR, and on top of it
  trades pedestrians for three times the unlabelled clusters.
- **Keep the region map in evidence**: it costs about 6.6 MB a case and makes every L3 arm's map
  inspectable.

## Provenance

- **Code:** region map recording and `lidar-region-map` at 95955281d (on
  `claude/experiment-overnight`); replays from the harness stamped at eac41ad9e.
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/overnight-20261013/l3bg0007/replay/{a0,off}/{kirk0,kirk1}/`;
  the per-cell maps, the analysis JSON and a plan-view chart kept locally, outside the repository.
- **Reference:** pack ad8b9438 version 1 (dataset ds_95be6fb22498b44a), reviewed masks, road-user
  classes.
- **Run:** four replays of 53 s, 2 min 42 s, 1 min 24 s and 3 min 54 s (estimated 75 s each); the
  map step five times, about 50 s each. The first four attempts are superseded: they read the
  pack's points per point (they are stored by column), counted ground and noise masks as road
  users, judged motion by total travel, and lacked the height split; each rerun is recorded in the
  experiment's state with its reason.
