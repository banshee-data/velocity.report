# Experiment: survey walk geometry gain

- **Status:** Proposed
- **Layers:** L2 Frames, L7 Scene (submap hand-off), spatial priors tooling
- **Related plan:** [lidar-route-capture-plan.md](../../../docs/plans/lidar-route-capture-plan.md)

## Goal

Decide whether a short walk with the sensor tilted about 45° before the twenty-minute stand adds
enough street-level geometry to be worth the operator's time, and at what tilt angle. This informs
the capture protocol and what the spatial priors experiment receives from a site visit.

## Hypothesis

A tilted walk of two to five minutes around a junction produces a submap with at least twice the
façade surface coverage between 2 m and 8 m above ground, and finer kerb and road-surface sampling
within 15 m of the walk path, compared with the twenty-minute level stand alone, because the tilt
sweeps rings across surfaces that a level sensor only strikes at fixed elevations.

## Method

1. At one junction, record three walks of the same route: level, tilted 30°, and tilted 45°, each
   two to five minutes, followed by the standard level stand.
2. Split the capture; confirm each walk is detected as a `survey` segment by the ground-normal
   tilt test and the stand as static.
3. Export each walk with per-point timestamps and reconstruct it with the LiDAR-only odometry
   baseline the priors review names (KISS-ICP class), producing one submap per walk.
4. Accumulate the stand's stabilised background into a comparable point set.
5. Measure, at a fixed voxel size: occupied voxels by height band, façade completeness against the
   building footprints from the SF bootstrap sources, kerb edge sharpness, road surface point
   density within 15 m of the path, and reconstruction residual.
6. Register the stand's reference pose to each submap and record the transform residual.

## Inputs and setup

- Site: one archived tripod site with building frontage on at least two corners.
- Capture: per the plan's protocol; the walk repeated at three tilt settings.
- Reconstruction: the priors experiment environment on a workstation; not the sensing binary.
- Reference: SF building footprints and the 2023 USGS aerial cloud as named in the SF bootstrap plan.

## Success criteria

- [ ] Each walk is classified `survey` and the stand `static` without operator markers.
- [ ] The 45° walk at least doubles façade coverage in the 2 m to 8 m band versus the stand alone.
- [ ] Reconstruction residual stays within the target the priors review sets for the experiment.
- [ ] The stand registers to the walk submap with a residual small enough to seed the background.
- [ ] The walk adds no more than five minutes to a site visit.

## Risks and controls

| Risk                                                            | Control                                                                                              |
| --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Odometry fails on the tilted walk because of short forward rays | Try the 30° setting; report where the tool loses track                                               |
| Moving traffic contaminates the submap                          | Reconstruct from the walk only; the priors evidence model treats single-pass surfaces as unconfirmed |
| Aerial reference is thin on façades                             | Use footprints for completeness and the aerial cloud for ground and roofline only                    |
| Tilt is not repeatable between walks                            | Mast detents; record the measured angle from the ground fit for each walk                            |

## Outputs

- Three submaps and the accumulated stand point set, with reconstruction records.
- Coverage table by height band and tilt angle.
- Recommended tilt angle and walk duration for the capture protocol.

## Result

Fill after running.

## Next action

Adopt the recommended tilt in the capture protocol, hand the submap format to the priors
experiment, or drop the walk from the protocol if the gain is marginal.
