# Annotation-scored tuning

What the first whole-capture annotation pack says about the pipeline's tuning,
and the kirk0 sweep run from it.

- **Status:** Measured; sweep run 2026-09-21, results to be read
- **Layers:** L3 Grid, L4 Perception, offline analysis
- **Related:** [Point annotation tool](point-annotation-tool.md), [Performance regression testing](performance-regression-testing.md), [Tuning guide](tuning-guide.md)
- **Scripts:** `/Users/david/code/sensor_data/lidar/kirk0-tuning/bin/`, and on branch `dd/lidar/bench-cluster-dump` under `data/explore/annotation-scored-tuning/`

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

Running from 2026-09-21 22:43 PDT, 8.4-hour deadline, six runs at a time on the
internal SSD, kirk0 only.

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

### Reading the results

```bash
python3 /Users/david/code/sensor_data/lidar/kirk0-tuning/bin/analyse.py \
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
- **The pack was made by a different build.** Its run is build `744f995f` at
  0.25× playback and produced 1,833,561 foreground points; today's `main` on
  the same capture produces 985,224. Both settle around frame 54, so it is not
  a warm-up difference. Whether that is a deliberate change or a regression is
  unresolved, and it is the first thing to check: it bounds how much of the
  missing recall tuning can recover.
- **Scoring lives in Python.** It belongs in Go behind a `velocity` subcommand,
  where it could gate CI the way the perf benchmark does. It is not there yet
  because the point of tonight was to find out whether the annotations could
  answer the question at all. They can.
- **The scripts are not committed here yet.** Adding any `.py` under `data/`
  wakes the `format-python` hook, which runs `black .` over the whole tree:
  it reformats eleven files in `scripts/` that are already committed, and it
  fails outright on `third_party/libpcap/testprogs/visopts.py`, which is
  Python 2 and cannot be parsed. `make lint` does not run black or ruff, so
  the drift never shows up in CI. Excluding `third_party/` from black and
  reconciling `scripts/` is a small job that unblocks this.
