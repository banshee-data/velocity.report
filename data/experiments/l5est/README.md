---
area: l5est
title: L5 state estimation
layers: L5
owner: david
---

# l5est: L5 state estimation

Experiments on the tracker's filter and what it draws: the motion model, the process and
measurement noise, and the trail a viewer sees from the state they produce. Which point on the body
the filter measures is [geom](../geom/README.md)'s question (geom0012), which track a cluster
belongs to is [l5id](../l5id/README.md)'s, and the calibration gate the measurement noise must
pass is G-UNC-1's ([predeclared criteria](../../../docs/lidar/operations/adaptive-uncertainty-criteria.md)).
Specs are named `l5est0001` onwards; see the [README](../README.md).

The state is planar, `[x, y, vx, vy]`, with a constant-velocity transition, a diagonal process
noise of `process_noise_pos` 0.05 and `process_noise_vel` 0.2 and a measurement noise of
`measurement_noise` 0.05, scaled by the frame interval
([state plan §5.4](../../../docs/plans/lidar-state-estimation-plan.md#54-what-is-in-the-state-and-what-is-not)).
The plan's principle governs every spec here: estimate the most probable physical path, never
smooth a trail to make it look plausible
([§0.2](../../../docs/plans/lidar-state-estimation-plan.md#02-estimate-the-most-probable-physical-path-do-not-cosmetically-smooth)).
A steadier trail passes only where it is also no further from the references.
