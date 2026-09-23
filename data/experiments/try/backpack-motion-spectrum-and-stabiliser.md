# Experiment: backpack motion spectrum and quasi-static stabiliser

Measure which wearer motion can be stabilised, then compare LiDAR-only and inertial processing
with acquisition timing qualified independently. The timing experiment includes cargo-bike loads.

- **Status:** Proposed
- **Layers:** L2 Frames, L3 Grid, L5 Tracks
- **Related plan:** [lidar-route-capture-plan.md](../../../docs/plans/lidar-route-capture-plan.md)
- **Timing design:** [portable-capture-timing.md](../../../docs/lidar/architecture/portable-capture-timing.md)

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

### Timing qualification before an IMU comparison

Run the [six-step bench/field procedure][timing-procedure] before attributing a gain to inertial
deskew. This phase does not block the LiDAR-only motion survey.

| Test                                    | Saved evidence                                                                                                    | Proposed acceptance                                                                                          |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| Known PPS/test edges and serial seconds | Reference analyser traces at MCU and LiDAR inputs, exact connector/firmware, UTC validity and PPS/message pairing | Correct second association, including midnight; measured edge-capture error within its allocation            |
| Clock mapping and point reconstruction  | Raw counters, held-out residuals, sample identities, decoder comparison for blocks and return modes               | Pacing/arrival changes do not change native point time; missed samples and wraps are detected                |
| Motion and sensor delay                 | Encoder-referenced multi-axis motion, ODR/filter settings, temperature, separate extrinsics                       | Acquisition-delay model validates on held-out motion; no unexplained phase residual assigned to clock offset |
| Loss/reacquisition and resets           | Independent 1/10/60 s GNSS, PPS, and serial outages; resets and injected timestamp jumps                          | Quality state/uncertainty changes, automatic rejection beyond budget, no fit across resets                   |
| Power and field load                    | Battery-side W/Wh, peaks, packet/FIFO loss, twenty-minute stand, walking and rough/smooth bike runs               | Add-on tested against 1 W reserve; enough measured pack runtime and no unreported loss                       |

Use the design's motion/range-dependent timing gate: the 100 µs combined residual is a prototype
target, not measured performance or a pass for every regime. Report median, p95, p99.9, maximum,
reference uncertainty, and rejected coverage. Inject ±0.1/0.5/1/5 ms offsets and 20 ppm drift into
the same recordings to show that the quality gate detects harmful timing errors. Compare shared
PPS, measured arrival alignment, and LiDAR-only variants without changing their source capture.
Test GNSS-free synthetic epoch acceptance separately before calling that mode viable.

### Motion spectrum and stabiliser comparison

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
      spacing of the reference; report residual yaw error after quantised azimuth shifting.
- [ ] Part two: frames retained at or above 80%.
- [ ] Part two: confirmed tracks per minute within 20% of the tripod run at the same site.
- [ ] Part two: p85 of vehicle maximum speed within the tripod run's bootstrap confidence interval.
- [ ] No regression: the tripod control capture classifies as one static site with the stabiliser on.
- [ ] Stabiliser cost per frame recorded on the workstation and, once the Pi baseline lands, on the Pi.
- [ ] Before IMU claims: timing qualification passes for the stated range/motion/temperature envelope;
      uncertainty includes internal sensor delay as well as clock mapping.
- [ ] Timing failures reduce published coverage and are counted; they never become zero uncertainty.

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
- Timing report: raw edge traces, firmware/configuration hashes, epoch/clock mappings, residuals,
  delay and extrinsic calibration, loss/reacquisition behaviour, and battery-side power.
- Separate backpack and bike acceptance envelopes, with timing rejection counts and retained coverage.

## Result

Fill after running.

## Next action

Promote to `data/explore/` with the scorecard, iterate on gate thresholds, or close if the
spectrum shows the wearer cannot be stabilised without inertial data.

[timing-procedure]: ../../../docs/lidar/architecture/portable-capture-timing.md#reproducible-qualification
