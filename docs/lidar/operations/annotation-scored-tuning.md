# Annotation-scored tuning

What the first whole-capture annotation pack says about the pipeline's tuning,
and the kirk0 sweep run from it.

- **Status:** Measured; sweep of 11,861 configs completed 2026-09-22
- **Layers:** L3 Grid, L4 Perception, offline analysis
- **Related:** [Point annotation tool](point-annotation-tool.md), [Performance regression testing](performance-regression-testing.md), [Tuning guide](tuning-guide.md)
- **Scripts:** [data/explore/annotation-scored-tuning/](../../../data/explore/annotation-scored-tuning/)

## Why this is different from every sweep before it

Every sweep so far has ranked configs on proxies: how many clusters, how many
tracks, how much foreground, how often a box was empty. Those measure how much
the pipeline did, never whether it did the right thing. A config that finds
twice as much speckle scores as busy as one that finds the cars.

A labelled pack ends that. It names, frame by frame, which returns belong to a
car and which to a tyre mark on the road, so a run can be scored on what it
found rather than how much.

## The pack

`8e422582-…-20260922-043329`, the whole of kirk0: 832 frames, 83.1 seconds,
1,833,561 foreground points, revision 204.

| Labelled           | Objects | Object-frames |
| ------------------ | ------- | ------------- |
| Car                | 24      | 2,369         |
| Pedestrian         | 7       | 400           |
| Bus                | 1       | 51            |
| **Road users**     | **32**  | **2,820**     |
| Noise              | 27      | 1,698         |
| Ground             | 2       | 87            |
| **Not road users** | **29**  | **1,785**     |

Four objects and 292 masks carry `reviewed`; the rest are `proposed`, which is
to say accepted from a proposal but not yet stepped through. The strict reading
of [what counts](point-annotation-tool.md#what-counts) is that only the
reviewed ones are reference truth. Every number below is reported against both,
and the two agree on direction throughout, which is the reason for trusting the
larger set for now.

The class assignment is the operator's, on every object: accepting a proposal
as "car" is a decision, not an algorithm's guess. It is the split between road
user and not that this analysis leans on, and that split is exactly what the
accept step records.

### What the labels say about the scene

| Class      | Median points/frame | Median range | Median footprint |
| ---------- | ------------------- | ------------ | ---------------- |
| Car        | 182                 | 16.5 m       | 2.9 × 3.2 m      |
| Bus        | 245                 | 32.0 m       | 5.0 × 3.7 m      |
| Pedestrian | 16                  | 19.5 m       | 0.38 × 0.29 m    |
| Noise      | 20                  | 10.3 m       | 0.61 × 0.85 m    |

The pedestrian row is the important one. **Not one of the 400 labelled
pedestrian-frames has fewer than five returns**, and only 35 have fewer than
eight. Pedestrians are not too sparse for a minimum cluster size of five. If
the pipeline is missing them, it is not L4 throwing them away for being small.

## How a run is scored

`lidar-bench -clusters-output` writes one line per cluster per frame: frame,
capture timestamp, centroid, bounding box, point count. A cluster matches a
labelled object in the same frame when its centroid falls within a gate of one
metre plus half the object's own horizontal diagonal — a bus tolerates more
centroid drift than a pedestrian, because a partial view of it moves the
centroid further.

| Measure    | Meaning                                                    |
| ---------- | ---------------------------------------------------------- |
| Recall     | Labelled road-user object-frames a cluster was found for   |
| Precision  | Clusters that landed on a road user                        |
| F1         | The two balanced, with no weight to argue about            |
| frag       | Clusters per matched object-frame; 1.0 is one cluster each |
| spurious/f | Clusters per frame landing on nothing labelled at all      |

Frame indices line up with the pack's sample ids, and the capture timestamps
agree to within 1.2 ms over the whole 83 seconds, so the two are the same
frames. Coordinates agree too: both are sensor-origin with no site transform.

## What the pipeline actually does today

Shipped defaults, `main` as of 2026-09-21, full profile:

|                      | Recall | Precision | F1    | Pedestrian recall | Clusters/frame |
| -------------------- | ------ | --------- | ----- | ----------------- | -------------- |
| Shipped defaults     | 0.560  | 0.806     | 0.661 | **0.205**         | 3.4            |
| Loosest corner tried | 0.902  | 0.190     | 0.313 | 0.713             | 31.7           |

Against the strictly-reviewed subset alone, the shipped defaults recall 0.416
and the loosest corner 0.986.

Two things follow.

**The pipeline finds one pedestrian-frame in five.** Given that no labelled
pedestrian is below the minimum cluster size, the returns are being lost before
clustering: L3 is folding them into the background. Loosening L3 recovers them
— to 71% at the extreme — so they are present in the data and reachable.

**It is not short of headroom, it is badly placed on the trade.** The loosest
corner finds nearly everything and invents 24 spurious clusters a frame. The
useful configs are between the two corners, and nothing before now could have
told us where.

### One artefact is most of the noise

`obj_3c67a8a9` is labelled noise, reviewed, and present in 778 of the 832
frames: 44% of every non-road object-frame in the pack. It appears at **frame
54, the frame warm-up ends**, sits at 10 m, holds about 24 returns, and its
centroid wanders 22.8 m over its life without ever going anywhere. It is
something the background model did not learn while it was warming up and never
learned afterwards.

It accounts for 5.5% of the clusters the shipped defaults produce — 0.19 of the
0.21 clusters a frame that land on labelled noise. Nearly all of the measured
noise is this one thing, and `post_settle_update_fraction`, which is 0 by
default, is the parameter that would let the model absorb it.

The other 0.74 clusters a frame land on nothing labelled at all. Some of those
are real: 5.5% of the pack's foreground carries no label yet. Precision as
measured is therefore a lower bound, and the gap will close as more of the pack
is labelled.

### The single most effective change found

Lowering `l3.ema_baseline_v1.background_update_fraction` from 0.02 to 0.005, on
its own, with nothing else touched:

|                                    | Recall | Precision | F1        | Pedestrian | Reviewed-only recall |
| ---------------------------------- | ------ | --------- | --------- | ---------- | -------------------- |
| Shipped                            | 0.560  | 0.806     | 0.661     | 0.205      | 0.416                |
| `background_update_fraction` 0.005 | 0.757  | 0.718     | **0.737** | 0.362      | 0.873                |

A slower background model stops absorbing the things it should be separating.
`closeness_multiplier` 3.0 → 2.5 is the next most effective (F1 0.697), and
lowering `neighbour_confirmation_count` makes things **worse**, not better,
which is the opposite of what the parameter's name suggests you would try.

## The sweep

Ran 2026-09-21 22:43 PDT to 2026-09-22 05:52, six runs at a time on the internal
SSD, kirk0 only: **11,861 configs, no failures**. The grid finished with time
left, and the hill-climb that followed it ran until it had no untried
neighbour of its best 25 left.

| Axis                               | Values                         |
| ---------------------------------- | ------------------------------ |
| `l3.closeness_multiplier`          | 1.5, 2.0, 2.5, 3.0             |
| `l3.safety_margin_metres`          | 0.05, 0.15                     |
| `l3.neighbour_confirmation_count`  | 2, 3, 4                        |
| `l3.noise_relative`                | 0.005, 0.01, 0.02              |
| `l3.background_update_fraction`    | 0.001, 0.005, 0.01, 0.02, 0.05 |
| `l3.post_settle_update_fraction`   | 0.0, 0.005                     |
| `l4.foreground_dbscan_eps`         | 0.3, 0.4, 0.55, 0.7, 0.8       |
| `l4.foreground_min_cluster_points` | 3, 5, 8                        |

10,800 configs, run in a shuffled order with a fixed seed so that a sweep cut
short by the clock is still an unbiased sample of the whole space. The shipped
defaults and one-axis-at-a-time probes off them run first, whatever happens
after. When the grid finishes, the driver hill-climbs from the best 25 along
finer ladders that reach past the grid's ends.

**No L5 parameter is in the sweep.** Clusters are L4 output and the dump is
written before tracking, so no tracker setting can move this score. Tuning L5
against annotations needs a track dump and a track-level score, which is
separate work.

`foreground_max_input_points` is held at 30,000 in every run including the
reference, not the shipped 8,000, so that a loosened foreground is never
silently truncated. Comparisons within the sweep are therefore sound; the
reference row is not quite the shipped binary.

### What it found

The search converged. Ranked on F1 alone the winner is
`closeness 3.0 / background_update 0.005 / post_settle 0.0025 / eps 0.25 /
min_points 3` at F1 0.819 — but so are the next nineteen, and **every one of
them fragments each object into about 3.6 clusters**. Precision counts every
piece of a shattered object as right, so a tiny clustering radius scores well
by breaking one car into four. That is the metric flattering itself, not a
better pipeline, and a tracker handed 3.6 clusters per vehicle would be worse
off than it is today.

Capping fragmentation at the shipped level gives the answer worth acting on.
It changes three parameters and leaves the other five alone:

|                                    | Shipped | Recommended |                   |
| ---------------------------------- | ------- | ----------- | ----------------- |
| `l3.closeness_multiplier`          | 3.0     | **2.5**     |                   |
| `l3.background_update_fraction`    | 0.02    | **0.01**    |                   |
| `l4.foreground_min_cluster_points` | 5       | **8**       |                   |
| Recall                             | 0.560   | **0.761**   | +20 points        |
| Pedestrian recall                  | 0.205   | **0.435**   | more than doubled |
| Reviewed-only recall               | 0.416   | **0.900**   |                   |
| Precision                          | 0.806   | 0.786       | −2 points         |
| Clusters per object                | 1.46    | **1.29**    | better            |
| Spurious clusters/frame            | 0.74    | **0.58**    | better            |

It is better on recall, on pedestrians, on fragmentation and on spurious
clusters at once, and pays two points of precision for it. Raising
`min_cluster_points` from 5 to 8 while loosening L3 is what keeps the speckle
out: the extra foreground buys real objects, and the higher floor discards what
it also lets through.

Two axes turned out not to matter. `safety_margin_metres` is flat across its
whole range — the top six configs differ only in it, by a thousandth of an F1
point. `noise_relative` wants to stay at its default 0.02; both 0.01 and 0.04
are worse, in opposite directions.

### Reading the results

```bash
python3 data/explore/annotation-scored-tuning/analyse.py \
  /Users/david/code/sensor_data/lidar/kirk0-tuning/results.jsonl
```

It prints the best by F1, the best recall available at each precision floor,
the Pareto front, and what each axis does on its own. The front is the useful
part: picking a config means choosing a point on it, and that choice depends on
what the tracker downstream does with a spurious cluster — which this score
cannot see.

## Caveats

- **Timing in the sweep is meaningless.** Six runs share eight cores, and the
  frame-budget gate is disabled because it fires on contention and fires
  hardest on the configs that produce the most foreground. Anything shortlisted
  has to be re-measured one run at a time before it goes near a Pi.
- **One capture, one scene.** kirk0 is 83 seconds of one junction. A config
  tuned to it is tuned to it.
- **The pack's run was tuned, not built, differently.** It produced 1,833,561
  foreground points where today's `main` produces 985,224 on the same capture,
  which looked like a regression. It is not. Building `lidar-bench` at the
  pack's own commit `744f995f` and running it against the defaults gives
  985,224 — the same number, to the point — and the two default configs differ
  on no shared key, only on four L4/L5 keys since removed. `internal/lidar/l3grid`
  is unchanged between the two. So the pack was recorded by a run whose L3 was
  loosened from the defaults; the sweep reaches its point count with
  `closeness 2.5 / noise_relative 0.01`, 0.4% off. The operator labelled a
  richer foreground than the defaults produce, which is the right way round for
  reference truth and part of why the defaults score so poorly against it.
- **Scoring lives in Python.** It belongs in Go behind a `velocity` subcommand,
  where it could gate CI the way the perf benchmark does. It is not there yet
  because the point of tonight was to find out whether the annotations could
  answer the question at all. They can.
- **Two of the swept axes are not uniform across the scene.** After L3 settles,
  `effectiveCellParams` replaces the global `noise_relative`,
  `neighbour_confirmation_count` and update fraction with per-region values for
  any cell that has an override, scaled by variance tercile and frozen at
  identification. A sweep that sets them in the config before the run, as this
  one does, reaches them — they are the defaults the overrides are derived
  against, and the measured effects are large — but the effect is region-shaped,
  so how it carries to another scene is not something one capture can say.
  `closeness_multiplier` has no override and is uniform.
- **Fragmentation is not in the score, only beside it.** F1 here treats every
  cluster on a real object as right, however many of them there are. Until a
  run is scored on whether it produced _one_ cluster per object, the ranking
  has to be read with the fragmentation column next to it.
