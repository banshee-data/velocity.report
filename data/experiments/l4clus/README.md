---
area: l4clus
title: L4 clustering
layers: L4
owner: david
---

# l4clus: L4 clustering

Experiments on how foreground points become clusters: DBSCAN's neighbourhood, the height band and
the size filters. Specs are named `l4clus0001` onwards; see the [README](../README.md).

## Next priority

Act on [l4clus0003's answer](../../../docs/lidar/operations/l4-min-cluster-size-kirk-2026-10.md):
the core threshold of 5 failed every clause on kirk0, so 8 stays in Stage B and l3bg0009 gains no
`cr` arm. Prioritise a fragment-joining experiment before cost-only, ground, or GPU work.

The [SpatialRust source review](../../../docs/lidar/operations/spatialrust-segmentation-review.md)
sets out the next steps: check for sparse XY radius-graph paths with an independent membership
oracle, then compare DBSCAN with exact Euclidean connectivity where those paths exist; absent
paths need motion-supported joining. Test vertical constraints independently. These follow-ups
require registered specs, merge cases, and checked-in parameter sets before replaying.
