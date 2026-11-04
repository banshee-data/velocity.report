---
area: lane
title: Lane segments
layers: L5, L8
owner: david
---

# lane: lane segments

Experiments on the hand-drawn lane map of a placement: directed segments, their successors and
merge transitions, how much of the observed traffic they hold, and what drawing one costs. The
map constrains fragment stitching in L5 and can stand in for the fitted local path in L8. Specs
are named `lane0001` onwards; see the [README](../README.md). The design is the
[lane stitching plan](../../../docs/plans/lidar-lane-stitching-plan.md).

[lane0002](lane0002.md) scores the travel lanes Gerardus proposes from the summed lane fields
against lanes the owner draws in the painter without seeing them, at kirk and columbus-broadway,
so the proposals' `high` grade is checked against an independent reference before an operator is
asked to approve it with a glance. The design is the
[lane snapshot and attribution plan](../../../docs/plans/lidar-lane-snapshot-and-attribution-plan.md#item-2-experiment-lane0002-gerarduss-proposals-against-drawn-lanes).
