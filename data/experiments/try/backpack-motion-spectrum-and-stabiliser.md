# Experiment: backpack motion spectrum and quasi-static stabiliser

- **Status:** Proposed
- **Layers:** L2 Frames, L3 Grid, L5 Tracks
- **Related plan:** [lidar-route-capture-plan.md](../../../docs/plans/lidar-route-capture-plan.md)

## Goal

Two decisions: whether re-rendering corrected points into the reference polar grid (option A in
the plan) survives real wearer motion, and whether an IMU is needed at all. Both depend on the
motion spectrum a standing wearer actually produces, which has never been measured.

## Hypothesis

Part one: a standing wearer's motion is dominated by yaw steps of a few degrees and sub-degree
pitch and roll sway, so most frames fall within what azimuth shifting and ring re-rendering can
correct.

Part two: with LiDAR-only keyframe stabilisation and the stability gate, a twenty-minute backpack
capture retains at least 80% of frames and produces track counts and speed percentiles within
the run-to-run variation of tripod captures at the same site.

## Method

1. Rigid-transform report: run the per-frame estimator on the five Van Ness files
   (`s2_sf_4_202609021427*` to `*144716`) to characterise tripod nudges as the control.
2. Capture: wear the rig at one archived tripod site, same weekday and hour as the archive entry,
   following the plan's protocol (quiet start, survey walk, twenty-minute stand).
3. Spectrum: run the estimator over the stand; report amplitude and dominant frequency per axis,
   the count and size of steps, and settle time after each step.
4. Stabilise: run analysis mode with the stabiliser on, then again with it off, and once more with
   the stabiliser on but the gate off.
5. Compare against the archive's tripod run at the same site: frames retained, background
   settled-cell coverage at five and ten minutes, confirmed tracks per minute, p50 and p85 of
   vehicle maximum speeds with sample sizes, and fragmentation.

## Inputs and setup

- Control capture: the Van Ness episode named above, on the S2 archive volume.
- Backpack capture: new, recorded per the plan's protocol; file names carry the `backpack` mode.
- Config: `config/tuning.defaults.json` plus the stabiliser parameters, recorded with the run.
- Tooling: `velocity lidar pcap-split --dry-run --motion-json` for the timeline; the analysis-mode
  replay with `settle_before_recording`; `lidar-bench` for cost per frame.
- Environment: workstation replay; no live pipeline.

## Success criteria

- [ ] Part one: at least 80% of stand frames have pitch and roll within half a dense-band ring
      spacing of the reference; yaw steps are corrected exactly by azimuth shift.
- [ ] Part two: frames retained at or above 80%.
- [ ] Part two: confirmed tracks per minute within 20% of the tripod run at the same site.
- [ ] Part two: p85 of vehicle maximum speed within the tripod run's bootstrap confidence interval.
- [ ] No regression: the tripod control capture classifies as one static site with the stabiliser on.
- [ ] Stabiliser cost per frame recorded on the workstation and, once the Pi baseline lands, on the Pi.

## Risks and controls

| Risk                                                        | Control                                                                          |
| ----------------------------------------------------------- | -------------------------------------------------------------------------------- |
| Different traffic on the two days masks the comparison      | Same weekday and hour; report sample sizes and intervals, not point estimates    |
| The wearer's occlusion sector hides one approach            | Record the mask sector; compare only approaches visible to both runs             |
| Registration drifts where the corner lacks planar structure | Report residual over time; a drifting site is a finding, not an experiment fault |
| Settling consumes a quarter of the capture                  | Report coverage at five and ten minutes separately                               |

## Outputs

- Rigid-transform CSV per capture under the archive's analysis directory.
- Segment timeline JSON for the stabilised and unstabilised runs.
- Scorecard comparing the three replay variants against the tripod run.
- Decision note: option A or B for the grid, and whether the IMU item proceeds.

## Result

Fill after running.

## Next action

Promote to `data/explore/` with the scorecard, iterate on gate thresholds, or close if the
spectrum shows the wearer cannot be stabilised without inertial data.
