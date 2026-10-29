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
