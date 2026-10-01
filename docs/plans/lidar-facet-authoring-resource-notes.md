# Facet authoring: keep the first cost measurable

The native annotation increment is useful for collecting repeatability evidence. It does not
yet establish a tracking benefit. Keep reflectivity as a small diagnostic until geometry,
reference bounds and association make a credible comparison possible.

- **Status:** Local implementation and scalar timing; field and full-pipeline costs unmeasured
- **Scope:** Native pose sketches, facet subsets, raw intensity inspection and compact/straight-edge registration proposals
- **Related:** [Minimum facet specification](lidar-facet-annotation-minimum-spec.md), [authoring design](lidar-physical-authoring-comprehension-design.md), [registration experiment](lidar-facet-registration-experiment-plan.md)

## What the current increment costs

Raw intensity already exists as one byte per recorded return, alongside coordinates and
classification. L4 already computes cluster mean intensity; L5 carries the running mean. This
increment does not add a per-return intensity column, a reflectivity map or an intensity term
to tracker association. **Facet subset** in the histogram reads the existing bytes and excludes
uncertain members. Missing intensity remains unavailable rather than becoming measured zeros.

Facet evidence stores canonical return indices for each supported frame. In a Swift array the
index payload is four bytes per return, before array/protobuf metadata. Packed protobuf indices
use one to five bytes each. Four facets with 200 returns each therefore carry 3,200 bytes of index
payload per supported frame, before observations, source pins and history. That is a payload
calculation, not a measured process-memory budget. Retaining every frame is different from a
bounded online history; do not multiply an annotation document into every runtime track.

The writer archives the complete previous facet document on every save. If each successive save
adds another observation, retained snapshot bytes can grow quadratically with observation count;
the final document's size alone understates disk use. Begin with two or three informative frames
per facet and measure the whole revision directory before propagating through long recordings.
Retirement preserves evidence, and no automatic history pruning is implemented.

The geometry inspector allocates temporary selected-point and fitting data when the subset
changes. The native session caches one frame/subset/type result, so hovering returns or changing
the camera does not refit it. The cache retains one result and its index key, using Swift array
sharing until a mutation; it does not retain a fitted mesh or another point-cloud history.

## A small timing check

Run the standalone harness with an installed Swift compiler:

```sh
sh tools/visualiser-macos/scripts/facet-fit-benchmark.sh
sh tools/visualiser-macos/scripts/facet-fit-benchmark.sh --baseline
```

It compiles the production scalar fitter with optimisation. Minimal source types isolate the
fields that fitter reads. Synthetic tilted planes exercise selected-return sorting, covariance,
eigenvectors and support checks, with 20 warm-ups and 200 measured calls per size. One local arm64
run produced:

| Selected returns | Median fit, ms | p95 fit, ms |
| ---------------: | -------------: | ----------: |
|               50 |        0.00592 |     0.00663 |
|              200 |        0.01471 |     0.01550 |
|            1,000 |        0.07004 |     0.08242 |
|            5,000 |        0.40958 |     0.43488 |

The clock/input baseline was about 0.00004 ms at p95. These are single-run scalar measurements,
not a sensor simulation, M1 qualification, tracker latency gate or confidence interval. They
exclude decoding, matching, multiple facets, rendering, protobuf storage and concurrent scene
load. A standalone process's resident size also includes Swift startup and the harness, so it
cannot be labelled the algorithm's added memory.

## What to measure next

Use short reviewed episodes to inspect the same facet's raw-code distribution across range,
aspect, visibility and scan phase. Check the exact per-frame intensity-presence flag first.
The current histogram supports this inspection without changing pose or point membership.
Only then try a bounded offline association cue beside geometric matching, recording a
geometry-only control, wrong associations, abstentions, identity continuity and endpoint error.

Measure whole-frame p50/p95/p99 time, allocations, peak resident memory and actual encoded file
size at declared support/history limits. Keep a held-out decision set closed while choosing
normalisation and thresholds. Raw codes are not material reflectance; a dense relative map
would also depend on stable body pose, visibility and sensor calibration. It remains later
work. Smooth trails or a stable colour patch cannot establish better boxes or bumper gaps.

Straight-edge proposals now store a small horizontal normal/offset relation and recheck selected
support when registering or loading it. Optional frozen pins retain exact proposal revisions;
they do not copy a second point cloud. Their checks are outside the scalar timing above.
Runtime facet measurements, surface/full-3D registration and the two-to-three reset
ledger remain open. Manual annotation edits do not spend that future
runtime reset budget. The existing geometry/reference and following-metric work remains the
priority while this diagnostic earns evidence.
