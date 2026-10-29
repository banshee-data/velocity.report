# Driving instability, approach stability, and TTC benchmarking (2026-10-29)

- **Status:** Research proposal; no safety thresholds adopted.

This note proposes approach-level measurements and supplementary reference experiments. The
[behaviour analytics plan](../plans/lidar-behaviour-analytics-plan.md),
[state-estimation plan](../plans/lidar-state-estimation-plan.md), and
[backlog](../BACKLOG.md) remain authoritative for metric contracts, acceptance, and scheduling.

## Research reviewed

1. Concas et al., _Transportation Research Part F_ (October 2026), [“Assessing the impact of connected vehicle warnings on driving instability”](https://doi.org/10.1016/j.trf.2026.103812). The [publisher abstract](https://www.sciencedirect.com/science/article/pii/S1369847826003074) reports 8,029 driver-days from 43 Tampa participants. Warning-day jerk volatility rises 23.9% with displays off and 9.7% with displays on, relative to warning-free days: a 14.2 percentage-point moderation; acceleration moderation is 17.5 percentage points. The event study finds no evidence of attenuation over twelve months, but notes a transient baseline increase after activation. Warning exposure is observational; neither crash reduction nor a universal threshold follows. Daily in-vehicle measures cannot directly calibrate short roadside passages.
2. Taheri et al., _Accident Analysis & Prevention_ (online October 21, 2026), [“How do automated vehicle Penetration, Vehicle-Size Composition, and desired headway shape urban traffic conflict Patterns? An analysis using a calibrated microsimulation model”](https://doi.org/10.1016/j.aap.2026.108835). The [indexed abstract](https://pubmed.ncbi.nlm.nih.gov/42906267/) describes SUMO simulations for Charlottenburg, Berlin: AV desired headways 0.6/0.8/1.0 s, with human-driven headway fixed at 1.0 s. At 0.6 s, full automation has approximately 45% fewer TTC-defined conflicts, while some mixed-traffic configurations have a larger fraction of selected events below 0.5 s TTC. Sensitivity cutoffs are 0.3/0.5/0.75 s. A larger conditional fraction does not establish a larger absolute severe-event count. These are simulation outcomes, not measured crash effects or universal safety boundaries.

The abstracts support these summaries; neither full methods section has been independently
reviewed here. Before adopting either study's measure, record the exact volatility/event definition,
denominator, sampling/filtering, model assumptions, uncertainty, and source section or table.
The paper summaries motivate hypotheses, not acceptance thresholds for velocity.report.

## Interpretation

Acceleration volatility is the dispersion of longitudinal acceleration around a defined baseline; jerk is the time derivative of acceleration, and jerk volatility is its dispersion. Neither is synonymous with a hard-braking event. Specify the baseline, spread estimator (e.g. SD, robust MAD, p90-p10), aggregation unit, smoothing and time window. Do not silently equate within-track, across-track and driver-day volatility.

A high-speed smooth approach can be hazardous despite low volatility. Moderate-speed stop-start behaviour may indicate sightline, geometry, signal, pedestrian or congestion effects, not necessarily defective design. Slow predictable approaches are desirable only in context. Instability is a diagnostic, not a standalone danger score.

## Proposed Approach Stability profile

For each directed intersection approach and movement class (through, left, right), spatially align trajectories to stop line and crosswalk; preserve:

- Report speed p50/p85/p98 over the population of per-passage maximum speeds, following [D-18](../DECISIONS.md#d-18--speed-percentile-aggregation-semantics). Distance-binned speed profiles are a separate research distribution: one declared speed summary per eligible passage per bin, explicit weighting and coverage, and distinct labels rather than reusing the report percentile names.
- Longitudinal acceleration and jerk distribution, robust volatility, and extreme-event frequency with explicit definitions.
- Braking onset location, peak deceleration location/magnitude, late-braking frequency and spatial concentration; distinguish expected signal/turn deceleration from abrupt corrections.
- Trajectory consistency, lateral variation and turn geometry where observable.
- Traffic conflicts: event rate per exposure and **full minimum-TTC distribution**, stratified by relative speed; evaluate multiple TTC cutoffs as sensitivity analyses, not hard safety labels. Consider PET where trajectories support it.
- Vehicle count, observation hours, movement type, speed strata, signal phase if known, traffic density, pedestrian/cyclist activity if measured, tracking coverage, confidence intervals and missingness.

Example _hypothesis_, not empirical result: if 35% of 5,000 observed approaches show abrupt braking within 15 m of a crosswalk versus 8% at a matched comparator, investigate visibility, advance cues, geometry and conflict patterns; do not claim causality from this comparison alone.

## Cross-site and before/after protocol

1. Choose comparable approach/movement classes; stratify or adjust for entry speed, signal state, congestion, turning, grade, geometry, weather/time and exposure. Publish both raw and adjusted comparisons; do not compare whole-site jerk variance alone.
2. Consume qualified final physical trajectories with a documented time base, observation support, gaps, and uncertainty. The sensor's nominal 10 Hz is not the effective track rate: the [behaviour plan's observation budget](../plans/lidar-behaviour-analytics-plan.md) reports roughly 5 Hz. Validate on actual irregular timestamps, fragmentation, and downsampled synthetic trajectories. Follow its Section 8.2.1: jerk comes from the refined acceleration state with declared bandwidth, never position differences; suppress inadequate support, short windows, and excessive uncertainty. A CV predictor's zero acceleration/jerk cannot validate real dynamics. Report sensitivity to filter/window choices and response delay.
3. Aggregate per passage before site statistics to avoid treating frames as independent observations. Preserve within-session dependence by resampling independent sessions/days, with passages nested where appropriate; report independent cluster counts as well as passages and intervals. Five repetitions in one session test acquisition, not population precision.
4. Plot spatial braking heatmaps/profiles against signed along-path distance to a named stop line/crosswalk. Freeze the reference geometry, transform, origin, direction, and uncertainty; a Euclidean distance is insufficient on a curved approach. Distinguish longitudinal acceleration from lateral turning acceleration and document grade/gravity correction for an IMU comparator. Use the separate distance-binned speed distribution defined above; preserve per-passage records locally for reproducibility.
5. For TTC, retain continuous encounter minima, relative speed, interaction class, and eligible exposure under the contract below. Run threshold sensitivity at 0.3/0.5/0.75 s only where temporal resolution and propagated uncertainty support those distinctions. At an effective 5 Hz, the 0.3 s cutoff spans only about 1.5 observation intervals; evaluate subsampling/phase error rather than assuming the bin is reliable. Show frequency and conditional severity distributions separately.
6. Before/after intervention: predeclare outcomes, observation periods, and comparable untreated approaches where available; examine pre-intervention trends, seasonality, traffic mix, and concurrent changes. A site selected for an unusually bad baseline is vulnerable to regression to the mean. For the total intervention effect, do not adjust away post-intervention speed or congestion changes caused by the intervention; show speed-matched mechanism comparisons separately. Without a credible control design, report a descriptive change and seek independent conflict/crash or engineering evidence before concluding safety improvements.

### Encounter, coverage, and publication contract

- Define one encounter's start/end, pair identity, merge/re-entry rule, prediction horizon, and minimum jointly observed duration before counting events. Frame threshold crossings and fragmented track IDs must not multiply a single encounter; retain unresolved identity as unknown.
- Apply longitudinal TTC only to confidently following, closing pairs using physical bumper gap and qualified closing speed. Non-closing, uncertain, or unobservable cases carry a reason, not zero TTC or a fabricated finite value. Crossing/turning encounters need validated projected footprints and two-dimensional motion, or PET when both users' occupancy of the shared conflict region is observed; see the [behaviour plan](../plans/lidar-behaviour-analytics-plan.md).
- State whether a minimum is complete or limited to the observed window. Report left/right censoring, occlusion, lost associations, and suppression by range, movement, and speed. A track already decelerating on entry has unknown braking onset; an unobserved upstream bin is missing, not a zero-braking passage. Compare sites over common supported approach lengths and publish excluded opportunities beside eligible denominators.
- Retain the complete eligible encounter population before applying conflict cutoffs. Report absolute short-TTC event counts/rates per declared opportunity alongside their share among selected conflicts. Changing the selected-conflict denominator can worsen the share while the absolute rate falls. A distribution of selected events alone is not the distribution of all encounters.
- Predeclare minimum effective exposure, reference precision, error/coverage limits, derivative bandwidth, event timing/location error limits, and interval precision on tuning data. Freeze them before held-out scoring and suppress unsupported cells. Reuse existing physical gates; these metric-specific criteria qualify publication and do not create another release gate. No numeric limits are adopted in this proposal.

## Implementation proposal / acceptance criteria

- **Phase 0: definitions and validation.** Written metric contracts for acceleration, jerk, volatility, hard braking, late braking, TTC and exposure; noise/smoothing sensitivity report using synthetic trajectories at measured effective rates and qualified independent references. Development against fixtures can proceed now; field-derived behavioural results require the existing physical and derivative gates.
- **Phase 1: offline experiment.** After those gates, run on existing sites and emit reproducible per-approach per-movement scorecards and distance-binned speed/braking plots, with uncertainty and coverage diagnostics. Compare volatility with extreme braking counts and rank stability under matched subsets. Until then, offline field work measures estimation error and observability, not braking or jerk distributions from unqualified tracks.
- **Phase 2: conflicts.** Preserve full minimum-TTC distributions and relative speed; compare rankings across sensitivity thresholds, documenting cases where conflict count falls but severe tail worsens.
- **Phase 3: public report design.** Prototype an Approach Stability section alongside speed percentiles, headway/PET and stop compliance; label exploratory findings, confounders, sample size and uncertainty. Avoid unsupported “dangerous intersection” classifications.
- **Phase 4: intervention validation.** Repeat same-site before/after observations and evaluate whether late braking, approach consistency and conflicts improve under matched conditions.

## Questions to resolve

- Are LiDAR track lengths and accuracy adequate for jerk at all existing sites? What filter and track-quality gates are required?
- What exact spatial reference and map alignment will define crosswalk/stop line distances?
- How will signal phase, turning movements, pedestrian activity and congestion be observed or marked unknown?
- What minimum observation exposure and comparator-matching rules support publication?
- Should initial public release show braking location and speed profiles first, keeping jerk volatility explicitly experimental?

**Recommendation:** prioritise spatial braking profiles and speed distributions over a site-wide volatility index. Keep volatility and TTC severity as separate supporting measurements. This is a research/design PR, not a claim of validated safety thresholds.

## Proposed real-world experiment: fixed roadside LiDAR vs instrumented vehicle

**Purpose.** Supplement the [existing external-reference protocol](../plans/lidar-physical-reference-review-plan.md#proposed-external-reference-rtk-probe-vehicle) with independent, repeatable evidence for vehicle position, heading, speed, acceleration and turning accuracy. Test whether apparent motion arises from viewpoint-dependent observations, imperfect physical body estimation, track association, or the constant-velocity motion model. A visually smooth track is not sufficient evidence of accuracy. These phases qualify this external benchmark; current 05x work continues under its existing reviewed-reference gates.

**Existing infrastructure.** Reuse the Gerardo server's scene registration; do not build a parallel registration pipeline. Reuse recorded LiDAR/VRLOG observations, L5 tracking, L8 trajectory contracts, replay tooling and the visualiser.

### Phase A — low-cost phone pilot

- Fixed LiDAR at a permitted, safe roadside location; stationary mount and calibration retained throughout. Use a measured test vehicle (length, width, axle/body reference and phone mounting offset). Secure phone to vehicle; log raw GNSS fixes, reported accuracy, GNSS-derived speed if available, timestamps and IMU if available at their native rates. Retain raw records and device model/settings. A phone is a coarse comparator, **not centimetre-level ground truth**.
- Choose a legal, low-complexity loop with clear sightlines and minimal GNSS obstruction; document route, grade, weather, traffic, LiDAR position and sensor-to-map transform. Do not brake abruptly or perform unsafe manoeuvres on public streets; controlled braking only where lawful and safe, preferably on a closed course.
- Collect repeated passes in both directions: steady speed at several legal speeds; normal acceleration; gentle planned deceleration; left/right turns; passes with near-side versus far-side visibility and partial occlusion. Aim initially for at least five clean repetitions per manoeuvre and direction, adjusting after observing usable track coverage.
- Record independent session markers (e.g., stationary start/end, clear pass-by landmarks) to help align streams. Estimate clock offset and drift **without fitting away spatial error**; report timing uncertainty. At 25 mph, 100 ms corresponds to about 1.12 m of travel.
- Convert GNSS geodetic coordinates into the registered local frame; account for antenna/phone-to-body-centre lever arm, especially during turns. Keep LiDAR/GNSS coordinate transforms and clock fitting separate from tracker evaluation; fit them on calibration data, evaluate on held-out passes.
- Inspect time-aligned overlays and report track detection/association continuity, speed agreement, gross lateral/longitudinal bias, and missing intervals. Phone GNSS is insufficient to certify sub-metre facet geometry or jerk.

### Phase B — independent reference benchmark

- Repeat representative manoeuvres with RTK GNSS plus IMU (or a comparable independently validated reference) whose measured position, velocity, heading, and timing uncertainties meet predeclared reference limits. Log solution status, covariance, correction age, dropouts, time base and antenna lever arm. Prefer hardware/common GNSS time or validated synchronisation. Urban multipath can defeat RTK: separately report degraded intervals and their exclusion reasons, rather than treating all fixes as ground truth.
- Establish a stable vehicle reference point (e.g., body centre) from measured geometry and antenna mounting; record heading calibration and antenna/IMU alignment. Do not assume the GNSS antenna equals the body centre.
- Use separate calibration and evaluation passes, with held-out sessions and sites for any generalisation claim. Freeze transform, timing, filter, noise, geometry parameters, and acceptance limits before evaluation; never align each estimator to reference separately. Compare identical LiDAR observations through: (A) the production observed-cluster medoid baseline; (B) existing L5 CV tracking; (C) facet-aware body-centre measurement with existing CV tracking; (D) optional PCL keypoint/correspondence-assisted geometry; and only then (E) alternative motion models such as CA or IMM with observation geometry held fixed. A centroid is a separate optional arm, not the production medoid. Keep attempted-pass detection/association failures in coverage, and publish reference-quality exclusions to expose selection bias.
- For each method compute along-track and cross-track position error, heading error when observable, velocity error, false lateral motion and acceleration on constant-speed straight passes, response delay/attenuation on genuine braking/turns, track fragmentation, missed detection, coverage and uncertainty calibration. Report medians, tail percentiles, per-manoeuvre plots and confidence intervals across passes; never select an estimator solely for smoothness.
- Specifically plot error against range, aspect angle, and visibility phase (front/side/rear, partial occlusion) to isolate the changing-visible-facet bias. Test whether a facet correction improves accuracy **without** sacrificing manoeuvre recovery.

### Data artefacts and benchmark review checkpoints

- Reproducible run manifest: LiDAR recording hash, vehicle dimensions, sensor calibration and Gerardo registration version, GNSS/IMU log hashes, clock/transform fitting procedure, estimator commit/config fingerprint, manoeuvre labels, quality masks, attempted-pass counts, and held-out pass/session IDs. Each arm must use a checked-in parameter set under `config/experiments/`, as required by the [repository instructions](../../AGENTS.md#run-experiments-from-checked-in-parameter-sets).
- Keep raw GNSS/IMU logs, recordings, event rows, and generated outputs local or in ignored storage outside the committed report. Commit the human-readable findings, provenance identifiers, uncertainty, and failure summaries in `docs/`; no raw results enter Git.
- Produce (1) top-down aligned tracks with error vectors; (2) time series of position/speed/acceleration and reference uncertainty; (3) error-vs-aspect plots; (4) comparison table of baselines/variants; (5) failure-case clips or LiDAR replay bookmarks; (6) a short decision memo identifying which error source dominates.
- **Checkpoint 1:** phone pilot confirms acquisition, replay, clock/coordinate alignment and useful coarse comparisons; it does not certify fine-scale acceleration/jerk.
- **Checkpoint 2:** RTK/IMU reference quality and lever-arm/clock uncertainty are independently documented, with adequate clean held-out passes. A reference that fails its predeclared uncertainty limits cannot certify the tracker.
- **Checkpoint 3:** score each candidate against predeclared position, coverage, and response-delay/attenuation limits on independent passes. Preserve genuine manoeuvre dynamics and report uncertainty and failure cases; if no candidate qualifies, retain the baseline and report the failed criteria. A small relative improvement alone does not establish adequate accuracy.
- **Checkpoint 4:** use a qualifying physical trajectory to validate braking-event timing/location against a reference with independently established derivative accuracy; position accuracy alone is insufficient. Jerk volatility requires the separate noise-floor and bandwidth validation in the behaviour plan. Existing G-GEO-1/G-UNC-1/G-SMO-1 and per-metric applicability rules govern production publication; these checkpoints add no RTK prerequisite to current 05x work.

**Publication dependency:** physically stable body reference estimation and derivative validity precede field braking or jerk publication. The phone/RTK benchmark supplies supplementary evidence under the existing reference protocol; it does not reorder releases or block fixture development. Registration is already provided by Gerardo; observation geometry, estimator dynamics and independent validation remain the measurement questions.

## Measurement-bias execution alignment (existing 05x priorities)

This proposal **does not advance CA/IMM, reorder release slots, or introduce a new blocking gate**. Follow [BACKLOG.md](../BACKLOG.md) and the existing [state-estimation plan](../plans/lidar-state-estimation-plan.md):

1. **v0.5.3, baseline/reference:** On frozen reviewed straight passes, compare observed medoid, current L5 CV state and physical body-centre estimate. Plot cross-/along-track discrepancies versus range, aspect and visibility; report false lateral movement and apparent acceleration, reference limitations and missing coverage.
2. **v0.5.4, observation integrity:** Complete existing foreground retention and L3/L4 fragmentation measurements/fixes before interpreting facet failures as motion-model failures.
3. **v0.5.5, physical geometry:** Use existing near-edge, heading/extent, containment and geometry-coherent tracking work to reduce viewpoint-dependent body-reference drift. Compare facet-aware and optional PCL-assisted alternatives on identical held-out episodes; select by physical accuracy and coverage, not smoothness.
4. **v0.5.6–0.5.7, continuity/finality:** Validate association, occlusion, calibrated uncertainty and final physical trajectories without conflating predicted/coasted and observed samples.
5. **After geometry evidence:** Evaluate CA then CV+CA IMM offline against geometry-corrected CV with identical observations; retain existing deferred production schedule unless measured results justify a separate prioritisation decision.

Use existing G-GEO-1/G-UNC-1/G-SMO-1 criteria and reviewed-reference splits rather than inventing thresholds. The phone pilot and later RTK/IMU passes are **supplementary external validation**, not a new dependency that blocks current 05x work. Gerardo registration already exists. Spatial braking and jerk remain research outputs until derivative-specific validation; no production metrics or thresholds are changed here.
