---
area: l5id
title: L5 identity
layers: L5
owner: david
---

# l5id: L5 identity

Experiments on which track a cluster belongs to: association, reacquisition, and one track per
road user. Specs are named `l5id0001` onwards; see the [README](../README.md).

Fragment stitching across an occlusion, along drawn lane segments, is l5id0007 to l5id0010 with
[lane0001](../lane/lane0001.md); the design is the
[lane stitching plan](../../../docs/plans/lidar-lane-stitching-plan.md).

The [current-default reacquisition refresh](l5id0013.md) registers reproducible overlap/tentative arms and an explicit labelled held-out acceptance component.

The [S3 likelihood/containment comparison](l5id0011.md) has checked-in parameter sets and a [cloud report](../../../docs/lidar/operations/l5-likelihood-containment-2026-10.md).

The [robust cost's birth rule](l5id0021.md) (`robust_birth_gate`) keeps the robust likelihood
cost's identity gain on kirk0 and brings its brief tracks and clutter within l5id0019's bounds of
`a0`'s ([report](../../../docs/lidar/operations/robust-birth-gate-kirk0-2026-11.md)).

## The kirk0 anchor

l5id0015 (2026-11-05) bootstrapped the defaults on kirk0's tuning split (19 scored objects, the
pack reconstructed by `lidar-pack-rebuild`, main `9f13d2a`). Full comparison: HOTA 0.4284, IDF1
0.4728, 90 switches, 46 fragmentations. Half-widths of the 5th to 95th percentile interval over
2,000 draws of the objects: HOTA 0.063, IDF1 0.057, switches 119. A kirk0 clause bound tighter
than these cannot be told from the sample; state each kirk0 bound against them, and record the
`a0` figures a run reads beside these, finding the cause before reading a candidate when they
differ. [Report](../../../docs/lidar/operations/cloud-run-kirk0-labelled-2026-11.md).
