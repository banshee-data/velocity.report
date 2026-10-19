---
area: ground
title: Ground model and the plane the tracks live in
layers: L4, L5
owner: david
---

# ground: the ground model and the plane the tracks live in

Experiments on the road surface as the pipeline sees it: where it is, how the vertical clip
reads it, which plane the planar track state lives in, and where the published box stands on it.
Specs are named `ground0001` onwards; see the [README](../README.md).

## Why an area

The tracker's state is already planar: `[x, y, vx, vy]` with a 4×4 covariance, a measurement of
the box centre's x and y, and vertical position a belief outside the state
([state plan §5.4](../../../docs/plans/lidar-state-estimation-plan.md#54-what-is-in-the-state-and-what-is-not)).
So "constrain motion to the ground" is not a change to the state. It is four questions about the
ground the state is silent on, each with its own spec:

| Question                                                                       | Where the ground enters today                                                                                                                       | Spec                        |
| ------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------- |
| Where is the ground, and how far does the sensor's plane sit from it?          | A per-site global gradient measured on three sites (Marina 4.35 %, Columbus 0.88 %, Embarcadero 0.14 %); the other twenty unread                    | [ground0001](ground0001.md) |
| Does a vertical clip that follows the surface give steadier tracks?            | The L4 height band on absolute sensor Z, floor −2.8 m, 0.5 m below the road at the 2.3 m mount; the P11 regional surface clip exists in replay only | [ground0002](ground0002.md) |
| Is the sensor's horizontal plane the right plane for the planar state?         | Points reach L4 and L5 in the sensor frame, identity pose                                                                                           | [ground0003](ground0003.md) |
| Should the published box stand on the surface rather than the frame's cluster? | `LatestZ` and `OBBHeight` are the associated cluster's values in each frame                                                                         | [ground0004](ground0004.md) |

The order matters. ground0001 is a measurement and gates the rest. The geometry says the frame
question is second order at these grades (the planar speed is `cos(atan g)` of the speed along
the surface, 0.1 % at Marina) and the clip question is first order (the band sits 1.3 m off the
road 30 m along a 4.35 % grade). ground0002's control arm with one plane for the capture is what
tests whether the cell model, the mesh in waiting, earns its place on the clip; ground0001's
departure table says where it could. The planned piecewise-planar tiles
([ground plane extraction](../../../docs/lidar/architecture/ground-plane-extraction.md)) and the
static scene's road patches
([local fit plan](../../../docs/plans/lidar-static-scene-local-fit-plan.md)) would enter these
experiments as further arms of the same clip, frame and box, not as new specs: each measure here
is defined against "the surface's height at (x, y)", whatever fits it.

## What the literature says

Read in October 2026 from the papers on the LiDAR volume (`Schafer2026`, `Petrovskaya2009`,
`Kraemer2018`, `Weng2020b`, `Simon2010`, `Lim2021` in
[references.bib](../../maths/references.bib)). None ran the controlled comparison these specs
run; the planar assumption is taken, not tested.

- Fixed roadside nodes track in the plane. Schäfer, Alrifaee and Hashemi's node keeps
  `[x, y, vx, vy, θ, ω]` with a measurement of `[x, y, θ]`, estimates length, width and height
  outside the state, and removes ground by an elevation threshold because the node does not
  roll, pitch or yaw: this pipeline's own design, with no 2D-against-3D ablation.
- The earliest model-based tracker reduced 3D returns to a 2D virtual scan on an estimated
  ground and tracked the vehicle's pose in the plane (Petrovskaya and Thrun); Kraemer et al.
  track a box or polyline with velocities and yaw rate absolute over ground, and record large
  orientation errors where one side is visible, which is a visibility problem and not a planar
  one.
- The one measured gain from a 3D filter in hand, AB3DMOT's identity switches 7 to 0 and
  fragmentations 43 to 15, is against a Kalman filter in the image plane: the gain is depth,
  which a ground-plane state keeps, so it does not bear against planar tracking.
- Constraining a state to a surface is a perfect measurement or a projection (Simon). Here the
  constraint holds by construction, since z is a belief and not a state, and the ground model's
  only job in the filter is to say which plane x and y are in.
- Patchwork's per-zone planes for a moving sensor became, for a static one, planes fitted once
  from the settled background: `l3grid.RegionalGroundSurface`, 10 m cells with a global fallback.
