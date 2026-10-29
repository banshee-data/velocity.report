# Driving instability, approach stability, and TTC benchmarking (2026-10-29)

Status: research proposal; no safety thresholds adopted.

## Research reviewed

1. Concas et al., *Transportation Research Part F* (October 2026), “Assessing the Impact of Connected Vehicle Warnings on Driving Instability,” https://doi.org/10.1016/j.trf.2026.103812 . Reported sample: 8,029 driver-days, 43 Tampa participants. Briefing reports jerk volatility +23.9% on warning days without active displays versus warning-free days, +9.7% with displays, a 14.2 percentage-point moderation; acceleration volatility moderation 17.5 percentage points, with no measurable attenuation over roughly 12 months. Observational warning exposure is not randomized; no crash reduction or universal threshold follows. Verify definitions, denominators and statistical model against full text before reproducing claims publicly.
2. Taheri et al., *Accident Analysis & Prevention* (2026-10-21), “How Automated Vehicle Penetration, Vehicle Size, and Desired Headway Shape Urban Traffic Conflict Patterns,” https://doi.org/10.1016/j.aap.2026.108835 . Calibrated SUMO simulation in Charlottenburg, Berlin, testing desired headways 0.6/0.8/1.0 s and minimum TTC cutoffs 0.3/0.5/0.75 s. Briefing reports approximately 45% fewer TTC-defined conflicts at full automation under the 0.6 s setting, while some mixed-traffic scenarios yield a larger fraction of very-short-TTC conflicts (<0.5 s). Simulation, not measured crash outcomes; these cutoffs are sensitivity settings, not universal safety boundaries. Verify full-text configuration and denominators before public citation.

## Interpretation

Acceleration volatility is the dispersion of longitudinal acceleration around a defined baseline; jerk is the time derivative of acceleration, and jerk volatility is its dispersion. Neither is synonymous with a hard-braking event. Specify the baseline, spread estimator (e.g. SD, robust MAD, p90-p10), aggregation unit, smoothing and time window. Do not silently equate within-track, across-track and driver-day volatility.

A high-speed smooth approach can be hazardous despite low volatility. Moderate-speed stop-start behavior may indicate sightline, geometry, signal, pedestrian or congestion effects, not necessarily defective design. Slow predictable approaches are desirable only in context. Instability is a diagnostic, not a standalone danger score.

## Proposed Approach Stability profile

For each directed intersection approach and movement class (through, left, right), spatially align trajectories to stop line and crosswalk; preserve:

- Speed p50/p85/p98 and full distributions by distance-to-crosswalk.
- Longitudinal acceleration and jerk distribution, robust volatility, and extreme-event frequency with explicit definitions.
- Braking onset location, peak deceleration location/magnitude, late-braking frequency and spatial concentration; distinguish expected signal/turn deceleration from abrupt corrections.
- Trajectory consistency, lateral variation and turn geometry where observable.
- Traffic conflicts: event rate per exposure and **full minimum-TTC distribution**, stratified by relative speed; evaluate multiple TTC cutoffs as sensitivity analyses, not hard safety labels. Consider PET where trajectories support it.
- Vehicle count, observation hours, movement type, speed strata, signal phase if known, traffic density, pedestrian/cyclist activity if measured, tracking coverage, confidence intervals and missingness.

Example *hypothesis*, not empirical result: if 35% of 5,000 observed approaches show abrupt braking within 15 m of a crosswalk versus 8% at a matched comparator, investigate visibility, advance cues, geometry and conflict patterns; do not claim causality from this comparison alone.

## Cross-site and before/after protocol

1. Choose comparable approach/movement classes; stratify or adjust for entry speed, signal state, congestion, turning, grade, geometry, weather/time and exposure. Publish both raw and adjusted comparisons; do not compare whole-site jerk variance alone.
2. Derive velocity from quality-filtered tracks with a documented time base. Estimate acceleration and jerk using consistent smoothing/differentiation, minimum track length and sample spacing. At 10 Hz LiDAR, jerk amplifies positional/tracking noise: quantify uncertainty using synthetic trajectories, repeated passes and where possible independent reference measurements. Report robustness to filter/window choices and track fragmentation.
3. Aggregate per passage before site statistics to avoid treating frames as independent observations. Bootstrap at passage/session level as appropriate; report confidence intervals, denominator and sample size.
4. Plot spatial braking heatmaps/profiles with distance relative to stop line/crosswalk, speed percentile envelopes and uncertainty; preserve the raw per-passage event record for reproducibility.
5. For TTC, retain continuous minimum TTC, relative speed, event type and exposure; run threshold sensitivity at 0.3/0.5/0.75 s only where meaningful and identify censoring/occlusion and false conflicts. Show both frequency and severity distribution; no one-number ranking.
6. Before/after intervention: hold placement, movement and sampling conditions as comparable as possible; document changes in traffic mix and observation conditions. Seek independent conflict/crash or engineering evidence before concluding safety improvements.

## Implementation proposal / acceptance criteria

- **Phase 0: definitions and validation.** Written metric contracts for acceleration, jerk, volatility, hard braking, late braking, TTC and exposure; noise/smoothing sensitivity report using existing 10 Hz LiDAR tracks.
- **Phase 1: offline experiment.** Run on existing sites, emit reproducible per-approach per-movement scorecards and distance-binned speed/braking plots, with uncertainty and coverage diagnostics. Compare volatility with extreme braking counts and rank stability under matched subsets.
- **Phase 2: conflicts.** Preserve full minimum-TTC distributions and relative speed; compare rankings across sensitivity thresholds, documenting cases where conflict count falls but severe tail worsens.
- **Phase 3: public report design.** Prototype an Approach Stability section alongside speed percentiles, headway/PET and stop compliance; label exploratory findings, confounders, sample size and uncertainty. Avoid unsupported “dangerous intersection” classifications.
- **Phase 4: intervention validation.** Repeat same-site before/after observations and evaluate whether late braking, approach consistency and conflicts improve under matched conditions.

## Questions to resolve

- Are LiDAR track lengths and accuracy adequate for jerk at all existing sites? What filter and track-quality gates are required?
- What exact spatial reference and map alignment will define crosswalk/stop line distances?
- How will signal phase, turning movements, pedestrian activity and congestion be observed or marked unknown?
- What minimum observation exposure and comparator-matching rules support publication?
- Should initial public release show braking location and speed profiles first, keeping jerk volatility explicitly experimental?

**Recommendation:** prioritize spatial braking profiles and speed distributions over a site-wide volatility index. Keep volatility and TTC severity as separate supporting measurements. This is a research/design PR, not a claim of validated safety thresholds.


## Proposed real-world experiment: fixed roadside LiDAR vs instrumented vehicle

**Purpose.** Establish independent, repeatable evidence for vehicle position, heading, speed, acceleration and turning accuracy before publishing spatial braking or jerk. Test whether apparent motion arises from viewpoint-dependent cluster centroids, imperfect physical body estimation, track association, or the constant-velocity motion model. A visually smooth track is not sufficient evidence of accuracy.

**Existing infrastructure.** Reuse the Gerardo server's scene registration; do not build a parallel registration pipeline. Reuse recorded LiDAR/VRLOG observations, L5 tracking, L8 trajectory contracts, replay tooling and the visualizer.

### Phase A — low-cost phone pilot

- Fixed LiDAR at a permitted, safe roadside location; stationary mount and calibration retained throughout. Use a measured test vehicle (length, width, axle/body reference and phone mounting offset). Secure phone to vehicle; log raw GNSS fixes, reported accuracy, GNSS-derived speed if available, timestamps and IMU if available at their native rates. Retain raw records and device model/settings. A phone is a coarse comparator, **not centimeter-level ground truth**.
- Choose a legal, low-complexity loop with clear sightlines and minimal GNSS obstruction; document route, grade, weather, traffic, LiDAR position and sensor-to-map transform. Do not brake abruptly or perform unsafe maneuvers on public streets; controlled braking only where lawful and safe, preferably on a closed course.
- Collect repeated passes in both directions: steady speed at several legal speeds; normal acceleration; gentle planned deceleration; left/right turns; passes with near-side versus far-side visibility and partial occlusion. Aim initially for at least five clean repetitions per maneuver and direction, adjusting after observing usable track coverage.
- Record independent session markers (e.g., stationary start/end, clear pass-by landmarks) to help align streams. Estimate clock offset and drift **without fitting away spatial error**; report timing uncertainty. At 25 mph, 100 ms corresponds to about 1.12 m of travel.
- Convert GNSS geodetic coordinates into the registered local frame; account for antenna/phone-to-body-center lever arm, especially during turns. Keep LiDAR/GNSS coordinate transforms and clock fitting separate from tracker evaluation; fit them on calibration data, evaluate on held-out passes.
- Inspect time-aligned overlays and report track detection/association continuity, speed agreement, gross lateral/longitudinal bias, and missing intervals. Phone GNSS is insufficient to certify sub-meter facet geometry or jerk.

### Phase B — independent reference benchmark

- Repeat representative maneuvers with survey-grade RTK GNSS plus IMU (or comparable independently validated reference), logging solution status, covariance, correction age, dropouts, time base and antenna lever arm. Prefer hardware/common GNSS time or validated synchronization. Urban multipath can defeat RTK: exclude or separately report degraded intervals, rather than treating all fixes as ground truth.
- Establish a stable vehicle reference point (e.g., body center) from measured geometry and antenna mounting; record heading calibration and antenna/IMU alignment. Do not assume the GNSS antenna equals the body center.
- Use separate calibration and evaluation passes. Compare identical LiDAR observations through: (A) observed-cluster centroid baseline; (B) existing L5 CV tracking; (C) facet-aware body-center measurement with existing CV tracking; (D) optional PCL keypoint/correspondence-assisted geometry; and only then (E) alternative motion models such as CA or IMM with observation geometry held fixed.
- For each method compute along-track and cross-track position error, heading error when observable, velocity error, false lateral motion and acceleration on constant-speed straight passes, response delay/attenuation on genuine braking/turns, track fragmentation, missed detection, coverage and uncertainty calibration. Report medians, tail percentiles, per-maneuver plots and confidence intervals across passes; never select an estimator solely for smoothness.
- Specifically plot error against range, aspect angle, and visibility phase (front/side/rear, partial occlusion) to isolate the changing-visible-facet bias. Test whether a facet correction improves accuracy **without** sacrificing maneuver recovery.

### Data artifacts and review gates

- Reproducible run manifest: LiDAR recording hash, vehicle dimensions, sensor calibration and Gerardo registration version, GNSS/IMU logs, clock/transform fitting procedure, estimator commit/config hash, maneuver labels, quality masks and held-out pass IDs.
- Produce (1) top-down aligned tracks with error vectors; (2) time series of position/speed/acceleration and reference uncertainty; (3) error-vs-aspect plots; (4) comparison table of baselines/variants; (5) failure-case clips or LiDAR replay bookmarks; (6) a short decision memo identifying which error source dominates.
- **Gate 1:** phone pilot confirms acquisition, replay, clock/coordinate alignment and useful coarse comparisons; it does not certify fine-scale acceleration/jerk.
- **Gate 2:** RTK/IMU reference quality and lever-arm/clock uncertainty are independently documented, with adequate clean held-out passes.
- **Gate 3:** facet-aware geometry and motion-model variants beat or meaningfully characterize baselines on independent positional accuracy *and* preserve real maneuver dynamics; track uncertainty and failure cases are reported.
- **Gate 4:** only after Gate 3, derive spatial braking metrics and validate event timing/location; jerk volatility requires a separate noise-floor and bandwidth validation. No production or public safety inference before the relevant gates.

**Priority change:** this benchmark and physically stable body reference estimation precede the proposed spatial braking heatmap. Registration is already provided by Gerardo; the unresolved work is observation geometry, estimator dynamics and independent validation.
