---
area: l3bg
title: L3 background model
layers: L3
owner: david
---

# l3bg: L3 background model

Experiments on the background grid: what it absorbs, what it lets through as foreground, how it
settles and what it does with an object that stops. The questions come from the
[beam-wise review](../../../data/maths/proposals/20261011-beamwise-background-subtraction-review.md)
and the reports it led to. Specs are named `l3bg0001` onwards; see the [README](../README.md).

The [LiDAR-Processing-V2 review](../../../docs/lidar/operations/lidar-processing-v2-segmentation-review.md)
adds the area's first boundary-rule question, [l3bg0026](l3bg0026.md): a vote over the dilated
foreground edge of the grid, weighted by 3-D distance and able to flip a return in either
direction, scored on W3 retention against false foreground. It is output only, as the support
filter is, and leaves the window and the background model alone.
