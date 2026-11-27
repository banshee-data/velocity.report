# LiDAR driving-behaviour benchmark validation plan

Translate the November driving-behaviour literature into reproducible measurement experiments, with a static qualification path in 05x and a portable transfer path in 06x. This is an evidence and implementation plan, not a new risk-score specification or a change to production thresholds.

- **Status:** Proposed
- **Related:** [Behaviour analytics](lidar-behaviour-analytics-plan.md), [state estimation](lidar-state-estimation-plan.md), [physical references](lidar-physical-reference-review-plan.md), [route capture](lidar-route-capture-plan.md), [experiment harness](platform-experiment-harness-plan.md)
- **Schedule:** [Backlog](../BACKLOG.md); D-36/D-37 continue to govern static qualification. Production PET, passing clearance and conflict extensions remain in 07x.

## 1. Research inputs and limits

The following are research inputs, not adopted settings. Record the consulted primary source,
version, exact metric definition, units, sampling rate, study population, exposure, uncertainty
and licence before reproducing a result.

| Source | Reported evidence and study context | Engineering consequence |
| --- | --- | --- |
| [Moshfeghi and Jang, Sensors (November 12, 2026)](https://doi.org/10.3390/s26227146) | Simulated tracking errors on drone-derived reference tracks from three Chinese signalised intersections (SinD); at 0.30 m mean position error, TTC precision was 0.09 for raw differentiation and 0.17 after causal tracking, versus PET precision 0.66 after tracking. Matched-noise learned cleaning reached 0.53 after tracking. | Score downstream errors and false events, not just tracking RMSE. Reference trajectories are not independent physical truth; do not adopt the paper's denoiser. |
| [Sanchez-Mateo et al., Scientific Data (November 23, 2026)](https://doi.org/10.1038/s41597-026-08900-x) | 35 drivers on one Madrid urban route; native 10 Hz GNSS, vehicle speed, steering and pedal data. The accepted paper reports residual timing uncertainty. | Benchmark recovery of speed, derivatives and manoeuvre timing from positions against vehicle signals; no claim of city-wide representativeness. |
| [Lee, Raifman and Griswold, Accident Analysis & Prevention (November 17, 2026)](https://doi.org/10.1016/j.aap.2026.108857) | San Francisco telematics quasi-experiment: reported relative changes of 16–22 % in speed-limit exceedance, 3–4 % in mean speed and 7–9 % in speed variability, across warning and citation phases. | Compare distributions over matched time windows and controls; distinguish policy phases, exposure and threshold definitions. Do not treat reported effects as acceptance criteria. |
| [Shaer, Fielbaum and Levinson, Journal of Transportation Safety & Security (January 22, 2026)](https://doi.org/10.1080/19439962.2025.2608002) | 258 hours across 31 Minnesota camera links, compared with 2016–2022 crashes. Short fixed-point passage headway (< 2 s), speed at least 10 km/h over the limit, and lane violations were investigated against TTC < 2 s cases. Dry, straight, non-congested links. | Analyse joint observable behaviour and conflict episodes with matched exposure; fixed-point passage headway is not our net time gap. Associations are not causal or transferable thresholds. |

The Madrid paper's data-records section identifies [Zenodo 18493475](https://doi.org/10.5281/zenodo.18493475),
but its data-availability section identifies [Zenodo 24137291](https://doi.org/10.5281/zenodo.24137291).
Resolve the dataset identity, licence, checksums and native timing before ingest; do not silently
select one or count repeated/interpolated values as independent samples. Only vehicle dynamics and
GNSS are needed, not demographics, questionnaires, physiological or gaze streams.

For reproduction only, the November briefing reported the Sensors study's TTC screens of 3 and
1.5 s and PET screens of 2 and 1 s, minimum closing speed 1 m/s and three consecutive frames.
Verify the exact primary-paper conditions and event construction before using these values in a
study-specific profile. Keep them separate from the canonical benchmark taxonomy in
[behaviour analytics](lidar-behaviour-analytics-plan.md#5-benchmark-taxonomy).

## 2. Static work packages (05x)

### B1. Reference adapters and frozen experimental protocol (0.5.3)

- [ ] Inspect the SinD trajectory schema, calibration, licence, sampling, footprint dimensions
  and identity truth; pin a version and source checksums. Use a reduced public trajectory subset
  only if its terms permit. Build a deterministic import into the existing evaluation boundary,
  not a new production trajectory format.
- [ ] Create hand-authored analytical fixtures with exact position, physical extents, time,
  velocity and expected interaction outcomes: constant speed, controlled braking and recovery,
  parallel non-conflicting traffic, following, cut-in, merge, crossing, stop, near-miss and
  no-interaction cases. Include cyclists and pedestrians where footprint truth is available.
- [ ] Record per-source reference uncertainty, coordinate frame, native rate, time alignment
  and missing values. Predeclare development and held-out splits by site, object and, for Madrid,
  participant; separate the reference used for scoring from the estimator's inputs.
- [ ] Freeze an experiment specification with the existing harness, estimator versions,
  seeds, tuning fingerprints, metrics and decision criteria before any held-out scoring.
  Include known-negative and ambiguous cases; unknown truth is not a negative.

Acceptance: deterministic repeat export, schema/unit/time tests, source and split digests,
documented licences, and a reference-admission ledger. Missing access or uncertain alignment
is reported as a blocker, not bypassed with assumed values. Keep source data and raw outputs
local under the existing evidence-storage policy.

### B2. Error-injection harness (0.5.6)

Implement an offline perturbation adapter at the versioned observation/trajectory boundary.
Do not modify the live tracker, the source reference or qualification cohorts. Retain a known
identity map and original support mask so track splits cannot hide missed or duplicate events.

| Error family | Proposed sweep (not a safety standard) |
| --- | --- |
| Position jitter, anisotropic or range-dependent | Mean error 0, 0.05, 0.10, 0.20, 0.30 and 0.50 m; specify distribution and realised error |
| Temporal drift, constant offset | Distinguish common-mode site error from independent per-object bias; include correlated noise |
| Heading, extent and endpoint errors | Sweep independently, including front/rear ambiguity and aspect-dependent body errors |
| Detection loss and fragmentation | Isolated missing frames, bursts, coasting, track splits, identity switches and wrong-leader cases |
| Timing and velocity | Clock offset/jitter, uneven sample intervals and velocity bias; distinguish estimated state from differentiated positions |

Use several deterministic seeds per level, with held-out error shapes and compound errors
selected before scoring. Calibrate plausible levels against measured LiDAR residuals and
capture metadata rather than claiming a camera-derived noise model describes LiDAR.
Run a zero-error arm first; perturbation at an output boundary tests metric sensitivity,
not the full sensor pipeline. An optional observation-boundary run tests propagation
through L5.

Acceptance: exact zero-error identity, seeded reproducibility, no invented observations,
unchanged reference digests, and correct accounting of missing, ambiguous and fragmented
tracks. Publish an authored summary of actual error by arm and host cost. No new default.

### B3. Kinematic derivative and manoeuvre validation (0.5.7)

Extend Phase 6A and the existing G-UNC-1/G-SMO-1 work instead of introducing an alternative
estimator. Compare raw differentiation, causal CV baseline, existing fixed-lag/final smoother
and supported body/facet estimates. Offline noncausal arms must not be presented as live output.
A learned denoiser is a separately approved later research arm, not part of the baseline.

- [ ] Import the verified Madrid GNSS/vehicle-signal subset. Use positions as the estimator
  input and keep independent vehicle speed and pedal/steering channels for evaluation.
  Project GNSS into a documented local metric frame, handle lever arm and heading, preserve
  source time, and bound the alignment uncertainty. Resample only with explicit support.
- [ ] Score speed error, longitudinal/lateral acceleration error where independently
  supported, peak attenuation, onset delay, event duration and manoeuvre preservation.
  Brake pedal onset is a control input, not the instant of physical deceleration; steering
  angle alone is not yaw rate. Compare fitted acceleration change-point and threshold onset
  on aligned windows, including hard braking, gentle braking and steady negatives.
- [ ] Establish derivative noise floors by window, sampling rate, range, aspect and support.
  Treat finite-difference jerk as experimental; evaluate against analytic truth and an
  independently supported acceleration reference where available. Report it unavailable
  if bandwidth or reference uncertainty is inadequate.
- [ ] Compare centre/centroid and body/facet trajectory arms on identical windows. Use
  independently reviewed endpoints, headings and trajectories from the physical-reference
  workflow; no claim of physical accuracy from agreement between two derived estimates.

Acceptance: predeclared signed bias, absolute-error quantiles, interval coverage, suppression,
peak preservation and timing-error results on held-out supported windows. Apply the existing
physical and derivative validity gates; pin metric-specific tolerances before the held-out run.
Never loosen G-SMO-1 to improve apparent conflict precision. No road-user jerk or spatial braking
publication without independent evidence; report insufficient evidence when appropriate.

### B4. Offline interaction metric propagation (0.5.8)

Reuse L7 shared-path/region relations and L8 equations, the version-selected trajectory,
existing following scorer and transit/encounter record contracts. For every fixture and
admitted external trajectory, compare clean/reference versus degraded estimates:

| Observable | Definition and check |
| --- | --- |
| Speed and speed choice | Passage speed and signed deviation from the effective posted limit; preserve full distributions and aggregate p50/p85/p98, not per-track percentiles |
| Following | Physical rear-to-front separation along common path; net time gap = gap / follower speed only with supported positive speed; retain fixed-point passage headway separately |
| TTC and DRAC | For supported closing shared-path pairs with positive gap, TTC = gap / closing speed and DRAC = closing speed squared / (2 × gap); use the canonical definitions and separate crossing geometry, overlap and non-closing cases |
| PET | Observed elapsed time between departure and subsequent arrival at a defined shared conflict region; actual overlap and unresolved occupancy are separate outcomes, not extrapolated PET |
| Lane/path keeping and cut-ins | Lateral offset and heading relative to accepted path; manoeuvre duration, lateral rate and follower clearance after lane transition, with lane membership and map uncertainty |
| Braking and induced response | Signed acceleration, peak and duration, onset and response latency with sustained evidence; temporal association alone does not establish causality |
| Road-user conflicts | Rear-end, crossing, merging/passing and uncertain geometry; actual observed evasive response recorded independently of predicted conflict |

Use the existing applicability and suppression rules. Retain uncertainty on relative
position/velocity and shared pose, including correlations; evaluate confidence-interval
coverage and event stability, not just a point estimate. A parallel pair with similar
velocity is a negative for a collision-course TTC, even at small separation.

Match events by reference road-user pair, geometry and time window, after consolidating
adjacent frames into episodes. Predeclare overlap/time tolerances and a one-to-one matching
rule; score false positives and missed events, duplicate rate, precision, recall, signed
severity bias, minimum-value error, false events per opportunity, valid coverage and reasons
for refusal. Bootstrap by independent episode/pair and site, not frames. Include uncertain
leader and lane changes, censoring at scene boundaries and support loss. Report clean
reference-screen performance separately from independent physical qualification.

Report distributions (including low-tail gap, net time gap, TTC and PET, upper-tail DRAC and
acceleration magnitudes) and an explicit denominator ledger: unique eligible passages,
pair/region encounters, eligible seconds, no-leader/free-flow intervals, refused/unknown
intervals and observation duration. Deduplicate opportunities and retain unsupported time
outside valid exposure. Screen values are named study profiles, not default danger boundaries.

Acceptance: deterministic event replay; zero-error analytical oracle agreement; no double
counting under splits, leader changes or replay; report per-family and per-stratum
precision/recall, severity error and support coverage for all arms. Score geometry and
uncertainty independently of estimator choices. Report which metric remains unobservable
at each error level, without adopting a universal pass/fail threshold. This is an
offline evidence deliverable; do not emit PET, general TTC, DRAC or conflict claims in
production or alter the existing 07x promotion gates.

### B5. Static field reference (0.5.9)

Use the existing D-29 RTK probe protocol for safe, ordinary scripted passes with independently
recorded timing and speed. If the admitted reference supports acceleration or braking onset,
score them as a secondary endpoint using B3's definitions; otherwise explicitly report missing
reference support. No staged close calls, unsafe manoeuvres or unpermitted capture.
Record the sensor pose, clocks, reference uncertainties, capture versions and all exclusions.
Retain the existing Pi performance and field publication gates. The pilot is not a citywide
benchmark or evidence of crash reduction.

## 3. Portable transfer (06x)

Static qualification does not transfer automatically to a moving sensor. Route processing
must consume the same observation, estimate-version and L8 metric contracts, with platform-
specific clock, deskew, pose and occlusion budgets. Preserve covariance between objects
sharing the same ego pose.

### B6. Portable perturbation and timing protocol (0.6.1)

Extend B2 offline with ego translation/rotation, velocity and timing errors, correlated
common pose error, vibration and deskew residuals, stops and route-regime transitions.
Use measured timing and PoseProvider quality from the portable capture plan. Verify that
common-frame transforms do not change intrinsic physical gaps and that uncertainty does
change reported applicability where warranted. Refuse clock resets, pose gaps and
unqualified regimes. No live tracker change.

### B7. Backpack and bike transfer (0.6.2–0.6.3)

For backpack, collect co-located static tripod and portable recordings with frozen matched
time windows and independently reviewed pairs/negative encounters; include stationary and
moving regimes separately. Repeat for bike with vibration, ego-motion, self-occlusion and
independently checked speed. Compare endpoint/gap, net time gap, leader identity, speed and
coverage with static estimates and the independent reference wherever present.

For bicycle and pedestrian encounters, prototype minimum physical separation and
encounter eligibility offline only, with region/path provenance and explicit support.
Do not treat the observed cohort as all road users or assume continuous opportunity
between passes. No experimental TTC/PET/passing-clearance promotion by virtue of the
headway transfer gate. Publish a regime-specific capability/limitations table, including
suppressed observations and uncertainty. Keep current static and portable promotion
criteria intact.

### B8. Intervention and cross-site benchmarks (0.6.4)

Build a reproducible offline comparison report from existing qualified static speed data
and, when separately qualified, directed route-segment measurements. Static studies need
not wait for bike headway qualification. Use one shared metric registry and versioned
report inputs; do not combine moving and static observations without checking selection
and measurement comparability.

- [ ] Predeclare intervention/control sites, warning and citation dates or other
  intervention periods, study windows, eligible hours, limits, road type and direction.
  Account for school terms, holidays, weather, traffic volume, season, enforcement changes
  and changes in capture geometry. Test matching balance and pre-trends; flag sparse
  controls or lack of overlap. Report descriptive before/after when a counterfactual is
  unsupported; use matched difference-in-differences or an event study when justified.
- [ ] Report speed ECDF/histogram, mean and variability, aggregate p50/p85/p98, speed-limit
  exceedance and named comparable exceedance bands as supplemental views, absolute and
  relative changes with uncertainty, counts, capture coverage and effective speed limits.
  Never equate exceeding the limit with exceeding it by 10 mph or 10 km/h.
- [ ] Produce a benchmark interchange table with study, site/metro/country, road type,
  road-user class, sensor and estimator version, conditions, exposure unit, metric
  definition, uncertainty, quantile and sample count. Compare matched road class, limit,
  time and traffic regime rather than rank cities using incomparable measurements.
- [ ] Prototype joint distributions of speed deviation, net time gap, lane/path offset,
  braking and conflict measures on admitted offline reference data, using supported
  eligible encounters and explicit unknowns. For local field reports include only
  separately qualified metrics. Treat any crash-linkage analysis as exploratory and
  validate against independent crash/exposure data with out-of-sample sites before a
  safety-effect claim. Do not create a composite driver-risk score or infer intent.

Acceptance: one reproducible static before/after analysis (with counterfactual only if
supported), a clearly labelled portable comparison when qualified, a provenance-bearing
benchmark table and explicit confidence intervals and exclusions. A failed match or
insufficient sample remains a reportable outcome. No claims of New York versus San
Francisco aggressiveness without comparable data.

## 4. Evidence, ownership and promotion

| Package | Target | Workstream | Deliverable |
| --- | --- | --- | --- |
| B1 | 0.5.3 | S-REF | Reference adapters, analytical fixtures, frozen protocol |
| B2 | 0.5.6 | S-REF | Deterministic perturbation harness |
| B3 | 0.5.7 | S-UNC | Kinematic derivative evidence (extend Phase 6A) |
| B4 | 0.5.8 | S-MET | Offline interaction sensitivity and denominator ledger |
| B5 | 0.5.9 | S-REF | Existing RTK campaign with conditional secondary evidence |
| B6 | 0.6.1 | S-MOBILE | Ego-pose and timing perturbation |
| B7 | 0.6.2–0.6.3 | S-MOBILE | Backpack and bike paired-reference transfer |
| B8 | 0.6.4 | S-MET | Intervention and matched benchmark pilot |

Each package records code SHA, source and split digests, criteria/configuration fingerprints,
sampling/coordinate contracts, seed and estimate stage, admitted and refused counts, raw
output/archive identifiers and an authored Markdown report. Use the existing experiment
harness and checked-in parameter sets. Keep raw results out of Git. Do not run held-out
qualification until criteria are pinned and the owner approves the experiment.

Start with small offline fixtures and reduced development runs; use the Mac/arrow-worker
evaluation path as appropriate before reserving long corpus replays. Run representative
performance checks before proposing any live change. The release backlog schedules evidence,
not automatic promotion, and the existing 05x physical-reference and 06x portable gates
remain authoritative. Production conflict extensions and causal safety claims retain their
own later gates.
