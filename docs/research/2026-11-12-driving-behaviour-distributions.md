# Driving behaviour and distribution logging: research synthesis (2026-11-12)

This brief connects the October 29, November 5, and November 12 driving-benchmark
briefings with the proposed velocity.report data contract for physically qualified
behaviour and conflict distributions. It records design guidance, not a new
metric implementation or a claim that a safety surrogate predicts crashes.

- **Status:** Research assessment and distribution-design proposal only; no production metric,
  benchmark, safety threshold, release gate, or priority changed.
- **References:** `Concas2026`, `Taheri2026`, `Wu2026`, `Nasri2026`,
  `Shangguan2026`, `Salah2026`; [canonical bibliography](../../data/maths/references.bib).
- **Canonical:** [behaviour analytics plan](../plans/lidar-behaviour-analytics-plan.md),
  [state-estimation plan](../plans/lidar-state-estimation-plan.md),
  [metrics registry](../platform/architecture/metrics-registry.md),
  [percentile semantics](../radar/architecture/percentile-aggregation-semantics.md),
  [backlog](../BACKLOG.md).
- **Earlier research:** [October 29 approach stability](2026-10-29-approach-stability-and-ttc.md),
  [November 5 causal surrogates](2026-11-05-causal-surrogate-validation.md),
  [October 22 DRAC/TTC/PET](2026-10-22/drac-and-conflict-metrics.md),
  [November 9 Salah full reading](../lidar/operations/brief/research-briefing-2026-11-09.md).

## Executive assessment

Treat the analysis as three distinct claims:

1. **Physical validity:** the estimated vehicle position, body geometry, speed and
   acceleration correspond to motion, not a shifting visible-surface medoid, lost points,
   track re-association, clock error, or derivative amplification.
2. **Behaviour and conflict validity:** a qualified trajectory supports a particular
   manoeuvre state, geometry-appropriate conflict measure, and correctly defined exposure.
3. **Safety validity:** that measure is associated with an independent safety outcome, or,
   more demanding, can stand in causally for the effect of an intervention on crashes.

Passing one claim does not imply the next. The immediately actionable engineering decision is
to **retain exact, versioned per-passage/per-encounter observations before computing
distributions**, and to distinguish time-weighted from vehicle- or encounter-weighted views.
A high-braking location is a diagnostic prompt, not proof of dangerous street design.

## Evidence reviewed

### Concas et al.: acceleration and jerk volatility (October 2026)

[Concas et al.](https://doi.org/10.1016/j.trf.2026.103812), *Transportation Research
Part F*, analysed 8,029 driver-days from 43 Tampa participants in a connected-vehicle
warning deployment. The published abstract reports warning-day jerk-volatility increases of
23.9% when the warning display was inactive and 9.7% when active, compared with
warning-free days, a 14.2-percentage-point moderation. The reported acceleration-volatility
moderation was 17.5 percentage points.

The transferable idea is to examine **dispersion of acceleration and jerk**, separate
from an event count. Volatility means spread around an explicitly defined baseline; it
does not mean mean acceleration, peak deceleration, or the count of hard brakes. It cannot
be copied as a driver-day statistic onto a several-second roadside observation. The
deployment was observational, not a randomized demonstration of crash reduction, and
provides no universal braking or jerk boundary. An apparently stable 40 mph approach can
still be dangerous; stop-start traffic can have high volatility without a design defect.

### Taheri et al.: conflict count versus severity (October 21, 2026)

[Taheri et al.](https://doi.org/10.1016/j.aap.2026.108835), *Accident Analysis &
Prevention*, used calibrated SUMO microsimulation in Charlottenburg, Berlin. It examined
automated-vehicle penetration, vehicle-size mix and desired headways of 0.6, 0.8 and
1.0 seconds. At a 0.6-second desired headway, the full-automation scenario had
approximately 45% fewer TTC-defined conflicts. Some mixed-traffic conditions had a
larger **share** of selected events with minimum TTC below 0.5 seconds.

A larger share does not necessarily mean a larger **absolute count or rate** of severe
events. Retain both all-eligible encounter minima and threshold-specific counts per
declared opportunity. The tested 0.3/0.5/0.75-second TTC screens are simulation
sensitivity choices, not portable human-safety thresholds. Evaluate whether the
available effective sampling rate and uncertainty resolve such short intervals.

### Wu and Flannagan: when a surrogate supports a safety claim (October 10, 2026)

[Wu and Flannagan](https://doi.org/10.1016/j.aap.2026.108773), *Accident Analysis &
Prevention*, propose a causal-graphical validation framework for hard braking as a
surrogate for rear-end crashes in SHRP2 naturalistic driving. It addresses assumptions,
intervention-to-crash mediation, surrogate paradox and transportability. The indexed
abstract does **not** establish that one braking threshold is universally causally valid.

A redesign could reduce detected brakes while leaving crashes unchanged, or cause a
shift in conflict type. Keep independently observed crash outcomes and quasi-experimental
design separate from a physically accurate braking heatmap. The existing
[November 5 note](2026-11-05-causal-surrogate-validation.md) owns this deferred
validation work; this synthesis adds no second backlog item.

### Nasri et al.: observational DRAC/TTC/PET associations (June 2026)

[Nasri et al.](https://doi.org/10.1016/j.jsr.2026.04.008), *Journal of Safety
Research*, used 54.8 million one-second trajectory observations at 139 Tucson
signalized intersections with five years of crash data. DRAC was the strongest
tested surrogate predictor across the reported model families. This is
observational association, not causal mediation. **DRAC is the required deceleration
under a specified collision-avoidance model**, not an observed braking manoeuvre.
PET performance in that paper is limited by its sampling and study design; do not
transfer numeric effects or event thresholds to our effective LiDAR sample rate.

### Shangguan et al.: duration-aware behaviour states (October 30, 2026)

[Shangguan et al.](https://doi.org/10.1016/j.aap.2026.108844),
*Accident Analysis & Prevention* 240:108844, conducted a driving-simulator study
under manual, ADAS and automated operation at three traffic densities. A
hierarchical Dirichlet process hidden semi-Markov model (HDP-HSMM) identified eight
scenario-specific states: residual/mixed, hard braking, brief braking adjustment,
brake release, rightward evasive braking, stable cruising, leftward steering
correction and sustained braking. The authors coupled duration-dependent
Bayesian survival features to a graph-attention LSTM for transition prediction.

The indexed abstract reports 41 drivers and 19,284 evaluation windows, F1 0.868,
accuracy 0.816 and time-to-transition MAE 0.493 seconds. These are
**simulator/model results**, not thresholds, roadside-LiDAR performance guarantees,
or evidence that its eight classes are universal. The classifier used vehicle
controls, eye tracking and surrounding-vehicle interactions that we do not all
observe. Its component ablation results were mixed: eye features did not improve
every target.

**Separation of responsibilities:** a CV/CA/turn-rate interacting
multiple-model (IMM) tracker estimates physical motion; an optional HSMM or
duration-aware state model interprets qualified sequences as cruising, braking
and manoeuvres. Do not conflate an IMM's motion-model probabilities with an
HSMM's driver-behaviour labels. Start with interpretable rule/episode baselines,
then test duration-aware models on independently labeled held-out tracks.

### Salah, Li and Wang: conflict geometry and observability (November 3, 2026)

[Salah, Li and Wang](https://arxiv.org/abs/2611.04988) synthesize 56 sources
in a structured narrative preprint, accepted for the 2027 TRB meeting according
to its manuscript. There was no preregistered search protocol or formal quality
appraisal. Its useful classification is by **conflict geometry** and **when a
metric becomes observable**:

- Longitudinal TTC (or its modified form) requires a confidently following,
  closing pair and a supported bumper-to-bumper gap and relative speed.
- Crossing or turning paths need two-dimensional motion and qualified
  occupancy/footprint geometry rather than blindly applying 1D TTC.
- Observed PET is a retrospective time gap between two completed occupancies
  of a shared conflict region; incomplete crossings are censored or ineligible.
- Braking, gap, speed and steering are different measurements and cannot be
  collapsed into one universal risk scalar.

The review's risk-state organization (exposure, imminence, potential severity,
persistence) is useful as distinct explanatory dimensions, not an endorsed
velocity.report risk score. Its secondary TTC/PET threshold examples, including
1.15 seconds and up to 6 seconds, require the original primary studies for any
decision. The [November 9 review](../lidar/operations/brief/research-briefing-2026-11-09.md)
already covers these uncertainties and records `Salah2026`; do not register a
duplicate paper or alter existing metric gates.

## Proposed recording and distribution contract

### Level 0: observation and estimator provenance

Keep the authoritative replayable sensor/evidence lineage in the existing
LiDAR/VRLOG pipeline, with immutable identifiers or hashes, clock/timestamp
basis, the Gerardo registration version and coordinate transform, estimator
configuration/version, observation-support flags, object body-reference
definition, and uncertainty. Exact estimated values and support intervals
must remain recoverable for a replay or analytical export; this is **not**
a proposal to commit frame-level data into Git.

Keep direct observations distinct from predicted/coasted values. A CV
filter's near-zero estimated acceleration is not evidence of no braking.
Do not introduce a derivative metric unless the state estimator is
physically validated for that derivative and its effective bandwidth.

### Level 1: independent analysis units

Each metric declares **one independent or clustered observational unit**
and its membership rules:

- **Passage:** an eligible vehicle passing a defined directed approach. Keep
  identity, approach/movement, observed start/end, track completeness,
  speed/geometry summaries, and the existing per-track **raw maximum** speed.
  The canonical report p50/p85/p98 population is the collection of
  *per-passage maximum speeds*, **not** one speed at a newly introduced
  reference line and not pooled frames. A distance-profile speed measure
  is separate and explicitly named, never an alias for the report percentile.
- **Physical-state sample:** timestamp, effective interval, longitudinal
  acceleration (signed), lateral acceleration if supported, estimator-derived
  jerk, confidence and quality/suppression reason. Treat these as
  **correlated within a passage**. Record original value precision.
- **Behaviour episode:** one braking/turning/acceleration episode per
  definition and tracked passage, with onset/end, duration, peak and
  integrated deceleration if qualified, path-relative onset and peak
  locations, completeness/censoring and uncertainty. A single long
  brake must not be counted repeatedly because of many frames.
- **Interaction/encounter:** stable pair and interaction type; start/end,
  eligible observed duration, minimum qualified TTC if applicable,
  relative speed, prediction horizon, coverage and censoring. One
  encounter is not one event **per frame** and fragmented IDs must not
  double count it. For completed crossings, store observed PET and both
  occupancy intervals. Missing or non-applicable metrics have reasons,
  not fabricated zero/large values.
- **Session/site:** immutable grouping by run, sensor, date, approach,
  movement class, time/context stratum, instrument/config and analysis
  version. Report unique eligible opportunities, attempted observations,
  covered distance/time and distinct sessions/days.

Frame samples are useful for time-weighted distributions. Passage/event
records are necessary for vehicle-weighted or encounter-weighted
distributions. Neither weighting is universally correct: preserve both,
name them clearly, and never silently merge them.

### Level 2: versioned aggregate summaries

An aggregate is derived from exact eligible records with the following
logical fields (reuse the existing metrics registry and event/instant/window
surfaces rather than creating a parallel runtime API):

- `schema_version`, `metric_id`, `metric_definition_version`,
  `unit`, `population_filter`, `estimator_id`, `obs_model_id`,
  `coordinate_frame`, `reference_geometry_version`.
- `site_id`, `approach_id`, `movement`, `window_start/end`,
  `source_mode`, `survey_or_run_version`, and optional audited context
  (signal phase, congestion, weather, road-user class); unknown remains unknown.
- `sample_unit` = `passage`, `episode`, `encounter` or `supported_instant`;
  `weighting` = `one_per_unit` or `observed_duration`. If a
  duration-weighted value is constructed, store effective support duration.
- `attempted_count`, `eligible_count`, `excluded_count_by_reason`,
  `independent_session_count`, `valid_observation_seconds`,
  `eligible_encounter_count` as applicable. A rate declares its
  numerator, denominator and opportunity definition.
- `histogram_edges` (ordered, in physical units), half-open-bin convention,
  `counts_or_duration`, underflow and overflow, `min/max`, exact/estimated
  `quantiles`, uncertainty method, interval bounds, coverage and
  suppression rule.
- `processing_version`, replay/analysis hash and quality-policy version
  so old surveys can be recomputed and compared.

Empty, censored, not-applicable, low-confidence and unobserved are
different states. Zero is a physically observed numeric value, never a
stand-in for missing. A minimum TTC from an encounter truncated at the
sensor boundary is an **observed-window minimum**, not the full encounter
minimum.

### Which distribution belongs to which unit?

- **Speed:** one per eligible passage's raw maximum for canonical
  report p50/p85/p98. Optional along-path spatial profiles produce one
  declared summary per observed passage per distance bin; compare over
  shared supported path length.
- **Acceleration:** preserve signed qualified samples/time support and
  episode summaries. Publish robust spread (e.g., SD/MAD or
  interquantile range) only with exact baseline, sample/time weighting,
  effective interval and estimator version. Do not label per-frame
  volatility as independent vehicles.
- **Jerk:** experimental until independently validated. Only from the
  derivative of refined acceleration with declared bandwidth; never
  third differences of observed positions. Report filter/window
  sensitivity and response delay rather than rewarding smoother curves.
- **TTC:** minimum per eligible *encounter* with a declared prediction
  model and horizon, plus counts/rates for sensitivity bands. Distinguish
  all eligible encounters from the subset that crossed a threshold; a
  larger fraction severe **among selected conflicts** can coexist with
  a lower absolute severe-event rate.
- **PET:** one qualified retrospective value per completed crossing
  encounter. Incomplete and outside-FOV encounters do not become
  fake PET measurements.
- **Following time gap and spatial gap:** **reuse**
  [`following_distribution_v1`](../../internal/lidar/l8behaviour/distribution.go),
  which already records time-weighted observed instants, exposure,
  quality exclusions and band rates, with 0.25-second net-time-gap
  and 2-meter spatial-gap bins. Its possible one-vote-per-encounter
  companion remains the existing B10 open question; do not rewrite it.
- **Braking location:** one onset and peak location per qualifying
  episode, relative to a versioned named stop line/crosswalk. Aggregate
  counts per eligible passage with full spatial exposure and a separate
  severity distribution. Along-path signed distance is not simply
  Euclidean radius on a curved approach.

### Histogram binning and quantile policy

**Research does not provide a universal discretization width.** Use
unrounded exact values as the authoritative analytical record and give
every materialized histogram a fixed, versioned bin specification.
Candidate *visualization experiments* are 1 mph speed, 0.25-0.5 m/s^2
longitudinal acceleration, 0.1-0.25 seconds short-range TTC/PET, and
2-5 meters along-path braking distance. These are **illustrative
engineering choices**, not configured defaults or validated risk bands.
Defer jerk bins until the derivative's physical precision is known.

Test bin stability against propagated measurement/timing error and
effective observation interval; suppress distinctions finer than
supported precision. Short-TTC bins should be nonuniform only if
predeclared and consistently applied across sites, with overflow and
censoring explicit. A continuous encounter minimum should survive any
later threshold change.

Exact quantiles should be computed from eligible underlying units
when feasible. **Never average percentiles or reconstruct an aggregate
p98 by combining per-window p98 values.** For large-stream rollups,
mergeable KLL or relative-error REQ sketches may be evaluated *later*;
they approximate **rank**, not physical measurement error, do not repair
track bias and cannot replace rare-event records or uncertainty
intervals. See [Apache KLL](https://datasketches.apache.org/docs/KLL/KLLAccuracyAndSize.html)
and [Apache REQ](https://datasketches.apache.org/docs/REQ/ReqSketch.html).
Do not introduce a sketch dependency or new schema in this documentation PR.

### Exposure, uncertainty and comparison

For every aggregate, expose attempted versus eligible opportunities,
valid time/distance, occlusion and track-identity failures, class
uncertainty, reference uncertainty, and named rejection reasons.
A suppressed estimate is **not** a zero and does not enter a public
histogram. Rates must say whether the denominator is passages,
completed encounters, following-valid seconds, jointly observed
seconds, or another opportunity count.

Uncertainty has at least three distinguishable layers: physical
measurement error and its propagation; event/metric estimation
uncertainty; and sampling/site-comparison uncertainty. Do not replace
one with another. Correlated frames do not justify binomial confidence
intervals with frame count as `n`. For cross-site inference, aggregate
per eligible passage/encounter first and use session/day clustering
or an appropriate hierarchical bootstrap. Declare number of
independent sessions/days as well as passages.

Cross-site comparisons should stratify or adjust for approach and
movement, speed, grade, signal/stop control, congestion, visibility,
vehicle mix and time of day when available. Publish raw and matched
results separately. For before/after interventions, predeclare the
estimand and controls; matching on a post-treatment change in speed
may accidentally remove part of the intervention effect. Report a
descriptive physical change unless independent safety validation
supports a stronger claim.

## Spatial approach-stability presentation

A useful top-down site output would show observed trajectories
behind a path-relative band of **braking onset frequency**, **peak
deceleration location/severity**, and observation coverage. Overlay
reference stop lines and crosswalks from Gerardo registration. Pair
the map with speed distribution, event count per eligible passage,
movement-type strata, uncertainty and missingness. For encounters,
plot a *separate* conflict location layer with minimum-TTC
distribution and eligible encounter exposure.

Example hypothesis, **not an observed result**: repeatable braking
onsets concentrated within 15 m of a crosswalk could motivate an
investigation into visibility, geometry, congestion or signal timing.
It is not evidence by itself that the road design *caused* the
braking or that crashes would be prevented by a change.

## Prerequisites, prototype and verification

The existing measurement-first ordering remains authoritative:

1. **Measure baseline bias** on reviewed physical-reference tracks.
   Separate visible-surface medoid movement, body-center error,
   foreground-point loss and association breaks.
2. **Improve observations/body anchoring** (facets, heading, extent,
   physical reference point, and continuity). Compare PCL keypoint/
   correspondence ideas offline against the same held-out reference;
   do not assume keypoints or smoothing are better than facets.
3. **Validate independently:** phone GNSS for coarse alignment, then
   RTK GNSS/IMU for body-referenced accuracy, aspect dependence,
   speed, actual manoeuvre response and false lateral drift. Reuse
   Gerardo registration; freeze clock, coordinate transform and
   tuning on calibration passes; score held-out passes.
4. **Evaluate motion models offline** with matched observations:
   baseline CV, improved-body CV, improved-body CA, then optional
   CV/CA IMM and turning model. Physical accuracy, coverage,
   response delay and uncertainty calibration decide, not
   apparent smoothness.
5. **Qualify derivatives and interactions** using replay fixtures,
   downsampled/noisy synthetic references, real held-out manoeuvres,
   gaps/occlusions and explicit metric-level suppression. Only then
   evaluate behavioural states, braking maps and conflict
   distributions on field tracks.
6. **Prototype aggregate contract offline** using versioned exact
   event rows and existing L8/registry semantics; compare one-vote-
   per-encounter and time-weighted views without redefining the
   shipping following distribution.
7. **Evaluate safety association separately**, against independent
   outcomes and comparable sites/periods. Reserve causality claims
   for an intervention design that tests mediation and paradox.

For a prototype to be reviewable, require a reproducible manifest,
reference and estimator versions, numerator/denominator and
censoring definitions, explicit event-deduplication tests, geometry
and unit tests, a merge/recompute test from exact events, stable
cross-site weighting, interval calibration and failure-case figures.
An offline field experiment with unqualified trajectories can
evaluate **estimation error**, not publish braking or jerk safety
findings. No new v0.5.x gate or backlog reprioritization is proposed.

## Open questions

1. Which eligible passage/encounter and truncation definitions are
   already fully implemented, and which need an L8 contract extension?
2. At the actual irregular effective rate (often around 5 Hz), how
   narrow can TTC, braking onset and acceleration distributions be
   before error dominates?
3. What external RTK/IMU uncertainty and derivative response are
   adequate for field publication at the desired spatial resolution?
4. Which sites have observed signal phase, turn identity, road grade,
   pedestrian/cyclist opportunity and complete approach coverage?
5. For B10, is encounter-weighted following a useful additional
   question, or is the existing duration-weighted view sufficient?
6. Would a simple duration-aware rule/HSMM add out-of-sample
   diagnostic value beyond qualified braking episodes, and on
   which independently labeled behaviour states?
7. For intervention evaluation, what independent crash/conflict
   evidence and untreated comparators can establish predictive
   association or causally valid surrogate mediation?

**Disposition:** adopt this document as a research and distribution
contract proposal, cross-reference existing work, and keep current
release ordering and public metric names unchanged.
