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
require registered specs, merge cases, and checked-in parameter sets before replaying. The
[LiDAR-Processing-V2 review](../../../docs/lidar/operations/lidar-processing-v2-segmentation-review.md)
adds curved-voxel connectivity (connected components over spherical bins, no density core) as a
third arm of that comparison once the path question is answered; it is not a spec of its own.

The [motion-supported joining candidate](l4clus0008.md) follows the failed threshold and fixed-radius alternatives, with explicit ownership, membership and merge gates.

[Track-conditioned joining on two placements drawn by lot](l4clus0011.md) keeps kirk0's identity
(M1) and costs no noise on three of four busy segments (M2), but lombard-broderick's first segment's
credited moving count rises 6.6 % as the candidate doubles vehicles it was meant to join (M3), and
its inferred merges exceed `a0`'s floor by 14 over the four segments, mostly one-frame contacts
(M4), so it is not named for label0003's window
([report](../../../docs/lidar/operations/fragment-join-membership-lot-2026-11.md)).
