# LiDAR road-user behaviour analytics plan

- **Status:** Draft (specification)
- **Layers:** L7 Scene, L8 Analytics, L9 Endpoints, storage
- **Target:** v0.7.x onward; every phase here is gated on smoothing landing in the estimation plan
- **Depends on:** [lidar-state-estimation-plan](lidar-state-estimation-plan.md) (owns Phases 0 to 5 and Phase 8; this plan owns Phases 6 and 7)
- **Companion plans:** [lidar-l7-scene-plan](lidar-l7-scene-plan.md), [lidar-test-corpus-plan](lidar-test-corpus-plan.md), [lidar-shape-descriptors-plan](lidar-shape-descriptors-plan.md), [lidar-static-pose-alignment-plan](lidar-static-pose-alignment-plan.md)

> **Scope split.** The estimation plan takes raw points to a trustworthy
> physical trajectory. This plan takes that trajectory and measures road-user
> behaviour from it. Phase numbering is shared across both documents. Nothing
> here may begin before gate G-SMO-1, because every metric below reads the
> `final` estimate and none of them may be computed from bounding-box centres.

## Executive summary

The central design decision is what **not** to build: no composite "safe
driver", "aggressive driver" or "risk" score. Such a score destroys the
information that makes the measurement useful and cannot be audited by the
people it describes. This plan instead specifies independently interpretable
observables, each with its own units, uncertainty, benchmark provenance and
suppression rule.

Three measured constraints shape everything that follows, and each one rules
out a category of metric that looks reasonable on paper.

| Constraint                         | Value                                                                   | Consequence                                                                              |
| ---------------------------------- | ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| A typical vehicle passage          | **9.6 s median, 42 observations, 51 m of observed path** at 6 to 10 m/s | The entire behavioural record for one road user is about ten seconds long                |
| Effective observation rate         | **5 Hz**, not the sensor's 10 Hz                                        | Jerk needs a ~1 s window, so a passage yields at most ten quasi-independent jerk samples |
| Observed path versus nominal range | 51 to 60 m median observed, against a ~100 m nominal useful range       | Every exposure denominator is roughly half what the sensor's range suggests              |

Passage figures are measured over 1,710 confirmed tracks in the production
database, stratified by speed band; see [Section 3](#3-the-observation-budget).

Two conclusions follow that contradict reasonable-sounding starting points, and
both are load-bearing.

**The Standard Deviation of Lateral Position cannot be measured here.** SDLP is
the primary outcome of a standardised on-the-road test: roughly an hour of
sustained highway driving with an explicit instruction to hold a steady lateral
position ([Verster and Roth, 2011](https://doi.org/10.2147/IJGM.S19639)). Its
published norms are defined against that protocol. A ten-second roadside
passage is three orders of magnitude short of it. We can compute a lateral
position standard deviation over the observed passage, and we must give it a
different name and never compare it to SDLP norms.

**The two-second following rule is not a research threshold.** It is driver
education. The 100-Car Naturalistic Driving Study records headway and
time-to-collision as event parameters ([Dingus et al., 2006, DOT HS 810
593](https://www.nhtsa.gov/sites/nhtsa.gov/files/100carmain.pdf)), but it
defines near-crashes by evasive manoeuvre and proximity criteria, not by a
headway cutoff. Time-headway bands of 2.0, 1.5 and 1.0 s are therefore useful
reporting bins, and must be labelled `no_established_threshold`, never
`research_threshold`.

The strongest use of this sensor is not driver profiling at all. It is
**interaction measurement**: passing clearance around cyclists, yielding at
crossings, post-encroachment time at conflict points. Those are geometric
quantities, measurable in a ten-second window, that mobile telemetry measures
poorly or not at all.

## 1. Core design principle

Model observables, not verdicts.

| Do                                                                        | Do not                                        |
| ------------------------------------------------------------------------- | --------------------------------------------- |
| Report minimum lateral passing clearance in metres, with its uncertainty  | Report a "close pass score"                   |
| Report time-headway exposure below a stated band, over stated opportunity | Report a "tailgating index"                   |
| Report the deceleration a follower would have needed                      | Label the driver unsafe                       |
| Report a percentile against a named comparison population                 | Imply a universal threshold where none exists |

A composite score may eventually be built **on top of** these observables, by
someone who states its weights. It may not replace them, and the raw values are
retained regardless.

### 1.1 Five distinct kinds of statement

Every metric in this plan is exactly one of these. Conflating them is the most
common failure in this field.

| Kind                              | What it asserts                                 | Example                                               | Authority                             |
| --------------------------------- | ----------------------------------------------- | ----------------------------------------------------- | ------------------------------------- |
| **Observable behaviour**          | What the trajectory shows                       | Minimum clearance was 0.82 m                          | The measurement, with its uncertainty |
| **Surrogate safety metric**       | Conflict severity under a published methodology | Minimum TTC was 1.2 s                                 | The methodology's authors, cited      |
| **Legal or normative compliance** | Conformance with a jurisdictional rule          | 34 km/h in a 30 km/h zone                             | The statute, which varies by place    |
| **Population-relative**           | Where this sits against comparable traffic      | 94th percentile approach speed for this site and hour | The comparison population, named      |
| **Longitudinal profiling**        | A persistent trait of a person                  | "This driver habitually tailgates"                    | **Not supported by this sensor**      |

The last row is the important one. A fixed roadside sensor observes a passage,
not a person. It has no identity, no trip history and, by
[TENETS.md](../../TENETS.md), no means of acquiring either. Every name in the
API and schema must reflect that: `passage`, `site interaction`, `observed
following exposure`. Never `driver aggressiveness` or `driver profile`.

## 2. Dependency on the estimation plan

Restating the contract from
[lidar-state-estimation-plan Section 13.1](lidar-state-estimation-plan.md), since
violating it silently is the easiest way for this work to produce confident
nonsense.

| Requirement                                                           | Why                                                                                                                                         |
| --------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Read `EstimatedState` at `stage = final`                              | The online estimate is revised afterwards; a report built on it quotes a superseded number                                                  |
| Carry covariance and `estimator_id` on every state                    | Uncertainty must propagate into every derived metric, and every metric must be reproducible against its estimator version                   |
| Heading and yaw rate are states with covariance                       | Lateral acceleration, curvature and conflict geometry are undefined without an orientation that has an uncertainty                          |
| Object extents are a track-level belief with sigma                    | Clearance and bumper-to-bumper gap are measured between surfaces, so they inherit extent uncertainty directly                               |
| Never read raw bounding-box centres                                   | The medoid measurement carries a viewpoint-dependent bias of up to half a vehicle width, measured at 0.676 m mean                           |
| Orientation is a belief with variance, held outside the dynamic state | Option A in the estimation plan keeps the motion filter linear; heading still arrives with an uncertainty, which is what these metrics need |
| Road-surface geometry, where used, is named on the estimate           | A metric computed under a planar fallback on a graded site is not the same measurement as one computed on a surface model                   |

**The contract, in one sentence.** State estimation produces an explainable,
uncertainty-bearing final physical trajectory. Behaviour analytics consumes that
trajectory and emits only metrics supported by class, geometry, observation
coverage and uncertainty.

### 2.1 Development may proceed in parallel; emission may not

The previous draft said nothing here could begin before gate G-SMO-1. That
conflated two different things and would have idled this work behind an
estimator milestone for no engineering reason.

| Activity                                                                                                                                                                                                      | Gate                                                              |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| Designing, implementing and testing behaviour metrics against **analytical synthetic trajectories**, fixture `EstimatedState` streams, external trajectory datasets, and deterministic reference trajectories | **None.** Start now                                               |
| Validating equations, suppression logic, uncertainty propagation and interaction classification                                                                                                               | **None.** These need trajectories, not _our_ trajectories         |
| Emitting any **production** behaviour result derived from live or replayed LiDAR tracks                                                                                                                       | **G-SMO-1**, plus the per-metric applicability rules in Section 7 |

The production gate, stated so it cannot be misread:

> No production behavioural result may be emitted from raw or current LiDAR
> tracks until the required trajectory-estimation gate has passed.

This is strictly stronger than the old wording where it matters, because it
covers the tempting middle case: computing a metric from today's medoid-based
tracks "just to see". A number produced that way carries a 0.676 m position
bias and must never reach a product surface.

The practical consequence is that a fixture `EstimatedState` stream, with
covariance and a declared estimator identity, is a first-class development
input. Phase 6A should build against it from day one.

## 3. The observation budget

**Canonical source.** Figures shared with the estimation plan, meaning the
43.6 % association rate, the 11.3 % excursion rate, the 0.316 m p99 lateral
residual and the count of tracks above 10 m/s, live in
[lidar-state-estimation-plan Section 1.5](lidar-state-estimation-plan.md) and are
cited from there rather than restated. When they are re-measured, that section
is the one to update. The passage budget below is specific to this plan and is
measured here.

Measured across 1,710 confirmed tracks with at least 10 observations, drawn from
the production database and stratified by peak speed.

| Peak speed       | Tracks | Passage duration p50 | p90    | Observations p50 | Observed path p50 |
| ---------------- | ------ | -------------------- | ------ | ---------------- | ----------------- |
| 1 to 3 m/s       | 494    | 12.5 s               | 89.8 s | 45               | 10.1 m            |
| 3 to 6 m/s       | 489    | 3.2 s                | 17.5 s | 14               | 12.5 m            |
| 6 to 10 m/s      | 487    | 9.6 s                | 22.6 s | 42               | 50.8 m            |
| 10 m/s and above | 240    | 13.8 s               | 44.6 s | 32               | 60.4 m            |

Read these as an upper bound on ambition. The 3 to 6 m/s row is the warning
sign: 3.2 s and 14 observations over 12.5 m, which is a fragmented or turning
track rather than a clean passage. Fragmentation, not metric design, is what
limits behaviour analytics at the low-speed end, which is another reason the
estimation plan's association work precedes this one.

### 3.1 What the budget permits

| Needs                                    | Available in ~10 s at 5 Hz?                                                      |
| ---------------------------------------- | -------------------------------------------------------------------------------- |
| Speed statistics                         | Yes, comfortably                                                                 |
| Longitudinal acceleration                | Yes, with about 1 s of support per estimate                                      |
| Peak deceleration in view                | Yes                                                                              |
| Jerk                                     | Marginal: about ten quasi-independent samples, each with a stated ~1 s bandwidth |
| Instantaneous headway and TTC at a point | Yes, when both road users are in view                                            |
| Headway _exposure_                       | Only over the observed encounter, which is seconds                               |
| Passing clearance at closest approach    | Yes, this is the sensor's best case                                              |
| PET at a conflict point                  | Yes, when both crossings are in view                                             |
| Lane-keeping tendency                    | No: see the SDLP conclusion above                                                |
| Trip-level speeding proportion           | No: there is no trip                                                             |
| Habitual anything                        | No                                                                               |

## 4. Fixed roadside versus mobile sensing

| Metric                                    | Fixed roadside | Mobile / in-vehicle | Why they differ                                                                                                             |
| ----------------------------------------- | -------------- | ------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Speed at a location                       | **high**       | medium              | The roadside sensor measures the location that matters; a probe vehicle measures wherever it happens to be                  |
| Speed relative to posted limit            | **high**       | high                | Both need the limit as data; see Section 10.1                                                                               |
| Approach speed to a crossing or curve     | **high**       | medium              | Requires the geometry to be fixed and known                                                                                 |
| Deceleration and braking in view          | **high**       | high                |                                                                                                                             |
| Stop-line compliance                      | **high**       | low                 | Needs the stop line surveyed once, which suits a fixed site                                                                 |
| Cyclist passing clearance                 | **high**       | low                 | Requires observing both parties from outside; an overtaking driver's own sensors see it poorly and a cyclist's see one side |
| Passing speed at closest approach         | **high**       | low                 | Same                                                                                                                        |
| Post-encroachment time                    | **high**       | low                 | Requires watching a shared conflict region over time                                                                        |
| Conflict geometry and minimum separation  | **high**       | medium              |                                                                                                                             |
| Yielding behaviour at crossings           | **high**       | low                 | Requires seeing the pedestrian and the vehicle simultaneously                                                               |
| Visible cut-ins and their consequences    | **high**       | high                | Roadside sees the whole geometry; in-vehicle sees the consequence                                                           |
| Local TTC during an observed encounter    | **high**       | high                |                                                                                                                             |
| Lane or path position with scene context  | **high**       | high                | Roadside needs a site frame; in-vehicle needs lane perception                                                               |
| Time-headway exposure                     | medium         | **high**            | Roadside measures seconds of it; a probe measures hours                                                                     |
| Following behaviour generally             | medium         | **high**            |                                                                                                                             |
| Acceleration distributions                | medium         | **high**            | Ten seconds is a thin sample of a driver's habits                                                                           |
| Lane-change dynamics                      | medium         | **high**            | Only when enough of the manoeuvre is inside the field of view                                                               |
| Weaving and lateral oscillation           | medium         | **high**            |                                                                                                                             |
| Jerk                                      | medium         | **high**            | Bandwidth-limited here; in-vehicle IMU is direct                                                                            |
| Free-flow speeding exposure               | medium         | **high**            | Roadside opportunity denominator is one short window                                                                        |
| Long-duration tailgating propensity       | **low**        | high                | Needs identity and duration                                                                                                 |
| Proportion of a trip spent speeding       | **low**        | high                | There is no trip                                                                                                            |
| Multi-mile lane-keeping consistency       | **low**        | high                |                                                                                                                             |
| Fatigue and distraction proxies           | **low**        | medium              |                                                                                                                             |
| Habitual acceleration and braking profile | **low**        | high                |                                                                                                                             |

The `medium` rows are valid **for the observed segment** and must never be
generalised. The API should make that impossible to get wrong by naming: a
field called `observed_following_exposure_seconds` cannot be mistaken for a
trait, whereas `tailgating_score` invites exactly that mistake.

## 5. Benchmark taxonomy

Every metric declares which of four kinds of benchmark, if any, it is compared
against. This is a required field, not documentation.

| Kind                       | Meaning                                         | Provenance                                                  | Example                                                      |
| -------------------------- | ----------------------------------------------- | ----------------------------------------------------------- | ------------------------------------------------------------ |
| `legal`                    | Conformance with a jurisdictional rule          | The statute, with jurisdiction and effective date           | Posted speed limit; statutory passing distance; stop line    |
| `research_threshold`       | Conflict severity under a published methodology | The paper or report, cited, with the use it was defined for | SSAM's TTC ≤ 1.5 s                                           |
| `external_distribution`    | Position against a published population         | The dataset, with its recording context                     | highD following distributions                                |
| `local_distribution`       | Position against comparable local traffic       | Our own observations, with the stratification used          | 94th percentile approach speed, this site, weekday afternoon |
| `no_established_threshold` | None defensible                                 | Explicitly stated                                           | Time-headway bands                                           |

**A metric with no defensible universal threshold is reported as a value and a
percentile, never converted into pass or fail.** This is the rule that keeps the
output honest, and it applies to more metrics than is comfortable.

### 5.1 Thresholds actually established, and what for

| Value                                         | Source                                                                                                                                                                               | What it was defined for                                          | Do not use it as                                                               |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| TTC ≤ **1.5 s**                               | FHWA SSAM ([FHWA-HRT-08-049](https://www.fhwa.dot.gov/publications/research/safety/08049/), [validation report](https://www.fhwa.dot.gov/publications/research/safety/08051/06.cfm)) | Screening simulated vehicle trajectories for candidate conflicts | A definition of unsafe driving                                                 |
| PET ≤ **5 s**                                 | SSAM default maximum PET, per the software user manual (FHWA-HRT-08-050) as reported in the secondary literature                                                                     | Conflict _candidacy_ screening, deliberately generous            | A severity threshold                                                           |
| PET ≈ **1 to 1.5 s**                          | Traffic-conflict literature, context-dependent                                                                                                                                       | Identifying serious temporal proximity                           | A universal cutoff; thresholds vary substantially by site type and road user   |
| DRAC > **3 to 3.35 m/s²**                     | Surrogate-safety literature; threshold selection is itself contested ([Zheng and Sayed, 2021](https://www.sciencedirect.com/science/article/abs/pii/S0001457521000828))              | Flagging a following situation as a conflict                     | A physical limit; maximum available deceleration is much higher on dry asphalt |
| Passing distance **1.5 m**                    | Statute in several EU jurisdictions including Germany and Spain                                                                                                                      | Legal minimum when overtaking a cyclist                          | A research finding                                                             |
| Passing distance **3 ft (0.914 m)**           | Statute in over half of US states                                                                                                                                                    | Same                                                             | Same                                                                           |
| Passing distance **1.0 / 1.5 m** speed-tiered | Western Australia: 1 m at ≤60 km/h, 1.5 m above 70 km/h                                                                                                                              | Same                                                             | Same                                                                           |

Note carefully that the SSAM PET default is the one value here we could not
confirm from a primary FHWA document; the techbrief and validation report state
the TTC default and not the PET default. Cite it as secondary until the user
manual is checked directly. That distinction is exactly the kind this plan
exists to preserve.

### 5.2 What SSAM's thresholds are for

SSAM screens _simulated_ trajectories from a microsimulation model for
candidate conflicts, then classifies them by manoeuvre type and computes
severity surrogates. Its thresholds are tuned for that screening job. Applying
them to _measured_ trajectories is defensible and common, but it is an
extension of their validated use, and the plan should say so wherever an SSAM
number appears in output.

## 6. Opportunity normalisation

Raw event counts are misleading, and the naturalistic-driving literature
demonstrates why with unusual clarity. In the SHRP2 analysis of speeding,
**99.8 % of drivers had at least one speeding episode** in their sampled trips,
averaging 2.75 episodes per trip across 623,202 episodes
([NHTSA, DOT HS 812 858](https://rosap.ntl.bts.gov/view/dot/44242/dot_44242_DS1.pdf);
peer-reviewed as [Journal of Safety Research, 2020](https://pubmed.ncbi.nlm.nih.gov/32563403/)).
A metric that flags "this driver exceeded the limit" separates nobody from
anybody.

SHRP2's own method is the one to copy: parse the record into **speeding
episodes** and **free-flow episodes**, where the latter approximate the
_opportunity_ to speed. Exposure over opportunity discriminates; maxima do not.

Opportunity normalisation is a first-class concept in the data model, not a
reporting convention.

```
speeding_exposure_10 = time(speed >= limit + 10 mph) / free_flow_opportunity_time
following_close_rate = time(THW < threshold)          / valid_following_time
close_pass_rate      = close_pass_count               / cyclist_overtake_count
yield_failure_rate   = non_yield_events               / valid_yield_opportunities
```

Three rules govern denominators.

1. **Opportunity is defined only over what the sensor observed.** Never
   extrapolate an unobserved portion of a journey. If a vehicle was in view for
   9.6 s, the denominator is at most 9.6 s.
2. **Opportunity excludes constrained time.** A vehicle held below the limit by
   the queue in front of it had no opportunity to speed, and counting that time
   as compliance is as wrong as counting it as a violation.
3. **A rate with a denominator below a stated minimum is suppressed, not
   reported as zero.** One overtake is not a close-pass rate.

## 7. Metric framework

The feature matrix in Section 8 lists what can be measured. This section states
the rules that decide, for any given road user in any given passage, whether a
listed metric may actually be emitted. Those rules are load-bearing: most of the
matrix is inapplicable to most passages, and a framework that computes
everything for everyone would be wrong far more often than right.

### 7.1 Road-user scope: three tiers

The engine operates on **road-user passages and interactions**, not on driver
identities. Metrics fall into three tiers, and the tier determines applicability
before any threshold is considered.

| Tier                                 | Applies to                                                       | Examples                                                                                                                                                                                           |
| ------------------------------------ | ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Universal trajectory primitives**  | Every tracked road user, every class, including `unknown`        | Position and path, speed, velocity vector, path length, observed duration, trajectory curvature, acceleration where observable, per-state uncertainty, trajectory confidence, observation coverage |
| **Class-specific behaviour metrics** | One motion class, because the concept only means something there | Following headway, braking behaviour, complete-stop behaviour, lane keeping, lane departure, approach speed, longitudinal jerk: all `rigid_vehicle`                                                |
| **Interaction metrics**              | Pairs, usually across classes                                    | Surface-to-surface separation, PET, conflict geometry, yielding, vehicle-cyclist passing clearance, vehicle-pedestrian interaction, crossing conflicts                                             |

**Do not compute every metric for every class.** Time headway for a pedestrian
following another pedestrian on a footway is arithmetically computable and
means nothing. Lane offset for a cyclist in a shared lane is ambiguous rather
than wrong. Both are suppressed, with a reason, rather than emitted.

The interaction tier is where the fixed roadside sensor is strongest, and it is
also the tier that most needs cross-class handling: the interesting pairs are
vehicle-cyclist and vehicle-pedestrian, not vehicle-vehicle.

### 7.2 Applicability and suppression

Every metric declares its requirements. When any requirement fails, the engine
emits a **machine-readable suppression reason**, never a number and never a
silent absence.

```text
following_headway:
    requires:
        follower motion class    = rigid_vehicle
        leader                   = any object with an estimated extent
        relation                 = common path, longitudinal, same direction
        estimation lifecycle     = established, both parties
        observation support      = observed, both parties, at the sampled instant
        extent belief            = converged, both parties

lane_offset:
    requires:
        lane geometry            = available for the traversed region
        site pose                = active
        motion class             = rigid_vehicle or two_wheeler
        trajectory uncertainty   = lateral sigma below the lane-discrimination bound

pet:
    requires:
        interaction type         = crossing, with confidence above the bound
        conflict region          = derived or mapped
        both crossings           = fully within the field of view
        timing support           = crossing-time sigma below the reported precision
```

Suppression reasons form a closed vocabulary, registered alongside the project's
other controlled vocabularies in
[label-vocabulary.md](../lidar/architecture/label-vocabulary.md) rather than
invented per metric:

| Reason                            | Meaning                                                                                   |
| --------------------------------- | ----------------------------------------------------------------------------------------- |
| `class_not_supported`             | The metric is not defined for this motion class                                           |
| `insufficient_observation`        | Too few observed frames, or the passage is shorter than the metric's minimum support      |
| `trajectory_uncertainty_too_high` | Propagated uncertainty exceeds the metric's usable bound                                  |
| `interaction_type_uncertain`      | Classification confidence below the bound; see 7.5                                        |
| `no_common_path`                  | The pair does not share the relation the metric assumes                                   |
| `road_geometry_unavailable`       | No road-surface model for the traversed region                                            |
| `lane_geometry_unavailable`       | No lane centreline or edges                                                               |
| `metric_not_observable`           | Structurally unobservable at this sample rate, for example jerk on a short passage        |
| `model_degraded`                  | The estimator reported `model_invalid` or `temporarily_degraded` for a contributing track |
| `extent_not_converged`            | A required dimension belief has not met its admissibility count                           |
| `planar_fallback_insufficient`    | Computed under a planar assumption on a graded site, where the grade error dominates      |

**A suppressed metric is preferable to false precision**, and a suppression
reason is a first-class result: it is stored, queryable and reportable. The rate
of each suppression reason per site is itself a useful diagnostic, and it is the
main thing Phase 6A should publish before anyone tunes anything.

### 7.3 Observation support

Not every missing frame is an occlusion, and conflating them corrupts exposure
denominators. Every sampled instant on a track carries a support state.

| State               | Meaning                                                     | Counts toward an exposure denominator?            |
| ------------------- | ----------------------------------------------------------- | ------------------------------------------------- |
| `observed`          | A detection was associated at this instant                  | **Yes**                                           |
| `coasted`           | The estimator propagated without a measurement              | **No**                                            |
| `occluded_inferred` | Missing, and another object's geometry explains the absence | No, but recorded as expected-missing              |
| `missed_unknown`    | Missing with no explanation                                 | No, and flagged: this is a detector defect signal |
| `cluster_merged`    | The detection is present but merged with another object     | No                                                |
| `cluster_split`     | The detection is fragmented across clusters                 | No                                                |
| `out_of_fov`        | Geometrically outside the sensor's coverage                 | No, and not counted as a failure                  |

The distinction between `occluded_inferred` and `missed_unknown` is the one that
earns its keep. Occlusion is a property of the scene and is expected; an
unexplained miss is a property of the pipeline and is a bug signal. The previous
draft treated both as occlusion, which would have hidden defect P4, the 56 %
frame miss rate, behind a plausible-sounding label.

The principle from the previous draft is preserved and strengthened:

> **Coasting is not observation.** A coasted state is a legitimate trajectory
> estimate and is not evidence that anything was seen.

### 7.4 Numeric measurements versus categorical outcomes

The previous draft forced everything through a single `float64` measurement
type. That does not fit outcomes such as "rolling stop" or "did not yield", and
encoding them as numbers would be exactly the opaque mapping this plan exists to
avoid.

Two result kinds, related but distinct:

| Kind                   | Carries                                                                                                                            | Example                            |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------- |
| `BehaviourMeasurement` | A numeric value, its unit, its uncertainty, its benchmark and its provenance                                                       | `min_speed = 0.42 m/s, sigma 0.18` |
| `BehaviourOutcome`     | A categorical label, the method and threshold version that produced it, and **references to the measurements it was derived from** | `stop_outcome = rolling_stop`      |

**A categorical outcome never replaces its evidence.** `rolling_stop` must
remain traceable to the minimum estimated speed, the duration below the speed
threshold, the position relative to the stop zone, the trajectory uncertainty,
and the road-surface or map confidence that positioned the stop line. The same
rule applies to hard braking, short following distance, close passing, lane
departure and yielding.

Categorical outcomes are where the temptation to build a verdict is strongest,
so the rule is absolute: the outcome is a convenience over the evidence, never a
substitute for it.

### 7.5 Interaction classification is uncertain, and must say so

Every pairwise formula assumes an interaction type. Applying the longitudinal
TTC formula to a pair that is actually crossing produces a confident number with
no meaning.

Interaction type is therefore classified **first**, from conflict angle,
relative motion and path geometry, and it is classified **with a posterior**, not
a label:

```text
InteractionClassification:
    posterior over {following, crossing, merging, overtake, opposing}
    evidence: conflict angle, relative velocity, path overlap, duration
    confidence
```

When the posterior is ambiguous, for example `P(following)` close to
`P(merging)`, metrics that depend on the distinction are suppressed with
`interaction_type_uncertain`. They are not computed under the argmax.

Metrics differ in how much they care. PET depends only on two crossing times and
survives a merging-versus-crossing ambiguity. Longitudinal TTC does not survive
a following-versus-merging ambiguity at all. Each metric declares the minimum
classification confidence it needs, and the evidence behind the classification is
retained so a surprising suppression can be explained.

### 7.6 The explainability record

Per principle 0.1 of the estimation plan, every emitted behaviour result
preserves enough provenance to answer: which trajectory states contributed;
which roadway or path geometry contributed; which thresholds were applied; which
uncertainties were propagated; which method and version produced it; and why it
was emitted rather than suppressed.

This is a data-model requirement, not a logging aspiration. Section 10.2 carries
the fields.

## 8. Behaviour feature matrix

### 8.0 What is actually in scope

The groups below are all researched and all retained, because the citation work
is the expensive part and re-doing it later would be waste. **Most of them are
not in the first increment.** This table is the scope boundary; treat a group
marked deferred as reference material, not as a backlog item.

Scope follows the product priority in 12.1, not the order the groups happen to
appear in. That ordering is deliberate: passing clearance is the top product
priority and it sits at 8.9, while acceleration and braking are the easiest to
build and near the bottom of the product list.

| Group                             | Scope                                                                  | Reason                                                                                                               |
| --------------------------------- | ---------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| 8.1 Speed behaviour               | **In**: instantaneous, mean, median, max, percentile                   | The universal primitives. Speed-limit compliance and speeding exposure are deferred with the site metadata they need |
| 8.2 Longitudinal control          | **In**: acceleration, peak accel and decel, braking onset and duration | Cheap once the acceleration state exists. Jerk stays experimental per 8.2.1                                          |
| 8.3 Following behaviour           | **In**: gap, THW, exposure over valid following time                   | The first pairwise metric, and the one that exercises the interaction framework                                      |
| 8.4 Time to collision             | **In**                                                                 | Needed by the conflict work above it in the product list                                                             |
| 8.6 Crossing conflicts and PET    | **In**                                                                 | Product priority 2, and the strongest surrogate a fixed sensor can measure. Works without a map, per 8.6             |
| 8.9 Cyclist and VRU overtaking    | **In**                                                                 | **Product priority 1**                                                                                               |
| 8.10 Yielding behaviour           | **In**                                                                 | Product priority 3                                                                                                   |
| 8.13 Interaction geometry         | **In**                                                                 | Shared substrate for 8.3, 8.4, 8.6, 8.9 and 8.10                                                                     |
| 8.5 DRAC                          | Deferred                                                               | Threshold selection is itself contested; TTC covers the same situations for now                                      |
| 8.7 Lane and path keeping         | Deferred                                                               | Needs lane geometry. The empirical-path subset lands in Phase 6C                                                     |
| 8.8 Lane changes, merges, cut-ins | Deferred                                                               | Needs lane geometry and a completeness gate that the field of view often fails                                       |
| 8.11 Stop behaviour               | Deferred                                                               | Needs stop-line geometry for anything beyond minimum speed and dwell                                                 |
| 8.12 Induced evasive response     | Deferred                                                               | Depends on the estimation plan's Phase 8 evidence surface                                                            |

Eight groups in, five deferred. Every deferred group is blocked on roadway
context, site metadata, or an upstream phase, rather than on effort.

Column key. **Scope**: `single` uses one track, `pair` requires two. **Map**:
roadway context required, per [lidar-l7-scene-plan](lidar-l7-scene-plan.md).
**RS**: fixed-roadside suitability. **Bench**: benchmark kind from Section 5.
**Form**: `C` continuous, `E` event-based, `X` exposure-normalised.

### 8.1 Speed behaviour

| Feature                                                | Definition                                                          | Units | Scope  | Map     | RS     | Bench                | Form |
| ------------------------------------------------------ | ------------------------------------------------------------------- | ----- | ------ | ------- | ------ | -------------------- | ---- |
| `speed`                                                | `‖v‖` from the final state                                          | m/s   | single | no      | high   | n/a                  | C    |
| `speed_mean`, `speed_median`, `speed_max`, `speed_p85` | Over the passage                                                    | m/s   | single | no      | high   | `local_distribution` | C    |
| `speed_at_reference`                                   | Speed where the path crosses a named reference line                 | m/s   | single | yes     | high   | `local_distribution` | C    |
| `speed_over_limit`                                     | `speed − posted_limit`                                              | m/s   | single | yes     | high   | `legal`              | C    |
| `speeding_exposure_{5,10,15}`                          | `time(speed ≥ limit + Δ) / free_flow_opportunity_time`              | ratio | single | yes     | medium | `legal` + `X`        | X    |
| `free_flow`                                            | True when no leader within a stated gap and no queue constraint     | bool  | pair   | no      | medium | n/a                  | C    |
| `speed_relative_to_stream`                             | `speed − median speed of contemporaneous same-direction traffic`    | m/s   | pair   | no      | high   | `local_distribution` | C    |
| `speed_variance`                                       | Variance over the passage                                           | m²/s² | single | no      | medium | `local_distribution` | C    |
| `approach_speed`                                       | Speed at a stated distance upstream of a conflict point or crossing | m/s   | single | yes     | high   | `local_distribution` | C    |
| `turn_speed`                                           | Minimum speed through a turn, with curvature                        | m/s   | single | partial | high   | `local_distribution` | C    |

**Prefer exposure over maxima.** `speed_max` over a 10 s passage is a single
sample of a noisy quantity and every vehicle has one. `speeding_exposure_10`
carries information. Where the free-flow denominator cannot be established,
report `speed_p85` against the local distribution instead of inventing an
opportunity.

`p85` here is the aggregate over a population of per-passage maxima, matching
the existing project convention recorded in [CLAUDE.md](../../CLAUDE.md).

### 8.2 Longitudinal control

| Feature                                      | Definition                                                          | Units | Scope  | Map | RS     | Bench                      | Form |
| -------------------------------------------- | ------------------------------------------------------------------- | ----- | ------ | --- | ------ | -------------------------- | ---- |
| `accel_long`                                 | Longitudinal acceleration state, vehicle frame                      | m/s²  | single | no  | high   | n/a                        | C    |
| `accel_peak`, `decel_peak`                   | Signed extrema over the passage                                     | m/s²  | single | no  | high   | `local_distribution`       | C    |
| `braking_onset`                              | First time `accel_long < −a_thresh` sustained for ≥ `t_min`         | s     | single | no  | high   | `no_established_threshold` | E    |
| `braking_duration`                           | Duration of the sustained braking interval                          | s     | single | no  | high   | n/a                        | E    |
| `accel_rms`                                  | RMS of `accel_long` over the passage                                | m/s²  | single | no  | medium | `local_distribution`       | C    |
| `jerk_long`                                  | `d(accel_long)/dt` from the smoothed state, with declared bandwidth | m/s³  | single | no  | medium | `no_established_threshold` | C    |
| `jerk_peak_pos`, `jerk_peak_neg`, `jerk_rms` | Extrema and RMS, each carrying the bandwidth                        | m/s³  | single | no  | medium | `local_distribution`       | C    |
| `high_jerk_events`                           | Count above a stated percentile of the local distribution           | count | single | no  | low    | `local_distribution`       | E    |

**No universal harsh-acceleration threshold is defensible here.** Published
"harsh braking" cutoffs come from telematics products with undisclosed
filtering, or from studies whose sampling rate and vehicle instrumentation
differ from ours. Report percentiles against a named local or external
distribution.

#### 8.2.1 Jerk is experimental until the bandwidth supports it

Jerk is retained because it is genuinely useful when it can be measured, and it
is marked **experimental** because at the current effective sample rate it
usually cannot. Two quantities must never be confused:

|                                     | Definition                                                           | Status                                                                                                                        |
| ----------------------------------- | -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Raw finite-difference jerk          | Third difference of estimated positions                              | **Never published.** At 0.2 s spacing and 0.05 m position sigma this is about 28 m/s^3 of noise against a 1 to 5 m/s^3 signal |
| Band-limited estimator-derived jerk | Derivative of the refined acceleration state, over a declared window | Publishable, with its bandwidth attached                                                                                      |

Rules, all testable:

1. Jerk comes from the refined acceleration state. Never from position
   differences, at any stage.
2. Every jerk figure carries its effective window or temporal resolution. A
   jerk number without a bandwidth describes the filter, not the road user.
3. Jerk carries an uncertainty, per 9.3, and it is one of the metrics where an
   asymmetric representation is likely to be the honest one.
4. Jerk is suppressed with `metric_not_observable` when the passage is shorter
   than the window, when observation support is inadequate, or when the
   estimated jerk uncertainty exceeds the magnitude being reported.
5. Acceptance is measured against **synthetic trajectories with analytically
   known jerk**, swept across sample rate and noise, publishing the boundary at
   which expected driving jerk becomes distinguishable from measurement noise.
   Until that boundary is published, jerk stays experimental.

If the system cannot distinguish ordinary driving jerk from measurement noise at
the available rate, it suppresses jerk. It does not publish it because it can be
calculated.

**Jerk suppression is mandatory.** From
[lidar-state-estimation-plan Section 13.2](lidar-state-estimation-plan.md):
four-point differencing at 0.2 s spacing with a 0.05 m measurement gives about
28 m/s³ of noise against a 1 to 5 m/s³ signal. Every jerk figure therefore
carries its effective window, and the API refuses to return jerk when the
passage is shorter than the window requires, or when the track is poorly
observed. A jerk number without a bandwidth describes the filter, not the
vehicle.

### 8.3 Following behaviour

For follower `F` and leader `L` in the same lane or path, travelling the same
direction, the gap is **bumper to bumper**, not centre to centre:

```
gap        = s_L − s_F − (L_L / 2) − (L_F / 2)
THW        = gap / speed_F                    (defined only for speed_F > 0)
closing    = speed_F − speed_L
TTC        = gap / closing                    (defined only for closing > 0)
```

where `s` is arc length along the shared path and `L` is the estimated vehicle
length. Using centre-to-centre distance overstates the gap by roughly one
vehicle length, which at typical urban following distances is a large relative
error.

| Feature                           | Definition                                                | Units | Scope | Map     | RS     | Bench                      | Form |
| --------------------------------- | --------------------------------------------------------- | ----- | ----- | ------- | ------ | -------------------------- | ---- |
| `distance_headway`                | `gap` as above                                            | m     | pair  | partial | medium | `external_distribution`    | C    |
| `time_headway`                    | `THW` as above                                            | s     | pair  | partial | medium | `no_established_threshold` | C    |
| `thw_min`, `thw_median`           | Over the observed following interval                      | s     | pair  | partial | medium | `local_distribution`       | C    |
| `thw_below_{2.0,1.5,1.0}_seconds` | Duration below each band                                  | s     | pair  | partial | medium | `no_established_threshold` | E    |
| `thw_below_X_rate`                | `time(THW < X) / valid_following_time`                    | ratio | pair  | partial | medium | `local_distribution`       | X    |
| `following_valid_time`            | Time both parties were in view, same path, `F` behind `L` | s     | pair  | partial | medium | n/a                        | X    |

**The denominator is the point.** `time(THW < 1.5 s) / valid_following_time` is
a statement about the observed encounter. `time(THW < 1.5 s) / passage_duration`
is meaningless, because most of a passage may involve no leader at all.

**Naming.** This is `observed_following_exposure` for one encounter at one site.
It is not a tailgating propensity, and the schema must not permit that reading.

### 8.4 Time to collision

TTC is the time until contact **if current relative motion continues
unchanged**. It is defined only when the parties are closing.

Short headway and low TTC are different situations, and conflating them is a
common error. Two vehicles 8 m apart at matched speed have a THW near 0.5 s and
an **undefined** TTC: there is no collision trajectory. Two vehicles 40 m apart
with the follower closing at 10 m/s have a comfortable THW and a TTC of 4 s.

| Feature                   | Definition                        | Units | Scope | Map     | RS   | Bench                | Form |
| ------------------------- | --------------------------------- | ----- | ----- | ------- | ---- | -------------------- | ---- |
| `ttc`                     | `gap / closing`, closing > 0 only | s     | pair  | partial | high | `research_threshold` | C    |
| `ttc_min`                 | Minimum over the encounter        | s     | pair  | partial | high | `research_threshold` | E    |
| `ttc_below_{1.5,1.0,0.5}` | Duration below each band          | s     | pair  | partial | high | `research_threshold` | E    |

The 1.5 s band is [SSAM's conflict screening
default](https://www.fhwa.dot.gov/publications/research/safety/08049/), reported
as such. The tighter bands are severity gradations in common use and are not
independently established; label them accordingly.

TTC is retained as a **continuous measurement**. Thresholds are reporting bins
applied afterwards, never a filter applied before storage.

### 8.5 DRAC and required braking

Deceleration Rate to Avoid a Crash: the constant deceleration the follower would
need, from now, to avoid contact if the leader continues unchanged.

For the simple same-path case, with closing speed `Δv = speed_F − speed_L > 0`:

```
DRAC = Δv² / (2 · gap)
```

The assumptions are explicit and restrictive: same path, leader velocity
constant, follower decelerating uniformly from now, no reaction delay, point
masses after the bumper-to-bumper correction. State them wherever the number
appears.

| Feature          | Definition                              | Units | Scope | Map     | RS     | Bench                      | Form |
| ---------------- | --------------------------------------- | ----- | ----- | ------- | ------ | -------------------------- | ---- |
| `drac`           | As above                                | m/s²  | pair  | partial | medium | `research_threshold`       | C    |
| `drac_max`       | Maximum over the encounter              | m/s²  | pair  | partial | medium | `research_threshold`       | E    |
| `drac_over_madr` | `drac / maximum_available_deceleration` | ratio | pair  | partial | low    | `no_established_threshold` | C    |

The literature flags conflicts around DRAC of 3 to 3.35 m/s², and threshold
selection is itself an open research question with competing methods
([Zheng and Sayed, 2021](https://www.sciencedirect.com/science/article/abs/pii/S0001457521000828)). Crucially,
3.35 m/s² is comfortably within a passenger car's capability on dry asphalt: it
resembles highway-design deceleration assumptions, not a physical limit. A DRAC
above it means the situation required firm braking, not that a crash was
imminent. `drac_over_madr` is the ratio that would carry that meaning, and it
needs a surface-friction assumption we do not have, so it stays `low` for
roadside use.

Keep DRAC continuous.

### 8.6 Crossing conflicts and post-encroachment time

PET is the elapsed time between one road user **vacating** a conflict region and
another **entering** it:

```
PET = t_enter(B) − t_exit(A)
```

Unlike TTC, PET requires no assumption about continued motion. It is a measured
fact about two crossing times, which makes it the most robust surrogate
available to a fixed sensor and the natural primary metric for intersections,
crosswalks, turns, cycle crossings and merges.

| Feature                      | Definition                                                | Units | Scope | Map     | RS   | Bench                | Form |
| ---------------------------- | --------------------------------------------------------- | ----- | ----- | ------- | ---- | -------------------- | ---- |
| `pet`                        | As above                                                  | s     | pair  | partial | high | `research_threshold` | E    |
| `pet_min`                    | Minimum over all shared conflict regions                  | s     | pair  | partial | high | `research_threshold` | E    |
| `conflict_point`             | Location of the path intersection                         | x, y  | pair  | no      | high | n/a                  | E    |
| `conflict_angle`             | Angle between the two path tangents at the conflict point | rad   | pair  | no      | high | n/a                  | E    |
| `relative_speed_at_conflict` | `‖v_A − v_B‖` at closest approach                         | m/s   | pair  | no      | high | `local_distribution` | E    |
| `min_separation`             | Minimum surface-to-surface distance over the encounter    | m     | pair  | no      | high | `local_distribution` | E    |

**The conflict region does not require a map.** This is a useful result: a
conflict point can be derived from the intersection of two _observed_ paths,
which makes PET available in Phase 6B without roadway context. A map improves it
by defining stable, named conflict regions that persist across encounters, which
is what makes PET aggregatable over a site. Both forms are worth building, and
they must be distinguished in output.

Report PET as a continuous measurement. The literature's roughly 1 to 1.5 s
serious-proximity band and SSAM's much broader 5 s candidacy screen are
different tools for different jobs, and both are `research_threshold`.

### 8.7 Lane and path keeping

Three distinct references, never conflated:

| Reference                      | Needs                               | What deviation from it means                                                                                                   |
| ------------------------------ | ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Actual lane centreline         | Site frame plus a map               | Lane position, in the sense the literature uses                                                                                |
| Empirical dominant path        | Weeks of local trajectories, no map | Deviation from where traffic actually goes, which differs systematically from the lane centre wherever drivers favour one side |
| The object's own smoothed path | Nothing                             | Within-passage oscillation only                                                                                                |

| Feature                       | Definition                                                | Units | Scope  | Map     | RS     | Bench                          | Form |
| ----------------------------- | --------------------------------------------------------- | ----- | ------ | ------- | ------ | ------------------------------ | ---- |
| `lateral_offset_lane`         | Signed distance from lane centreline                      | m     | single | yes     | high   | `legal` / `local_distribution` | C    |
| `lateral_offset_path`         | Signed distance from the empirical dominant path          | m     | single | no      | high   | `local_distribution`           | C    |
| `lateral_position_sd_passage` | SD of `lateral_offset_*` over the passage                 | m     | single | partial | medium | `local_distribution`           | C    |
| `lateral_velocity`            | Rate of change of lateral offset                          | m/s   | single | partial | medium | n/a                            | C    |
| `lateral_accel`               | `v · ω` from the state                                    | m/s²  | single | no      | medium | `local_distribution`           | C    |
| `lateral_jerk`                | Derivative of the above, bandwidth declared               | m/s³  | single | no      | low    | `no_established_threshold`     | C    |
| `heading_error_lane`          | Estimated heading minus lane tangent                      | rad   | single | yes     | high   | n/a                            | C    |
| `lane_departure`              | Any part of the estimated footprint crosses a lane edge   | bool  | single | yes     | high   | `legal`                        | E    |
| `path_oscillation`            | Zero crossings of `lateral_offset_path` per unit distance | 1/m   | single | no      | medium | `local_distribution`           | C    |

**`lateral_position_sd_passage` is not SDLP.** It shares an arithmetic
definition and nothing else. SDLP is measured over roughly an hour of
standardised highway driving with an instruction to hold a steady position
([Verster and Roth, 2011](https://doi.org/10.2147/IJGM.S19639); see also
[Verster and Roth, 2014](https://doi.org/10.1002/hup.2406) on excursions
out-of-lane as an alternative outcome). Its norms, including the criteria used
to equate drug effects to blood-alcohol levels, are defined against that
protocol and against sustained driving. Comparing a ten-second roadside sample
to them would be a category error. The name is deliberately different, and the
API must never expose this field as `sdlp`.

### 8.8 Lane changes, merges and cut-ins

Counting lane changes is close to useless. The consequence is the measurement.

| Feature                               | Definition                                           | Units | Scope  | Map     | RS     | Bench                   | Form |
| ------------------------------------- | ---------------------------------------------------- | ----- | ------ | ------- | ------ | ----------------------- | ---- |
| `lane_change_duration`                | Start to end of sustained lateral transit            | s     | single | yes     | medium | `external_distribution` | E    |
| `lateral_displacement`                | Net lateral movement over the manoeuvre              | m     | single | partial | medium | n/a                     | E    |
| `peak_lateral_velocity`               | Maximum during the manoeuvre                         | m/s   | single | partial | medium | `external_distribution` | E    |
| `peak_lateral_accel`                  | Maximum during the manoeuvre                         | m/s²  | single | no      | medium | `external_distribution` | E    |
| `gap_front_before`, `gap_rear_before` | Gaps in the target lane before entry                 | m     | pair   | yes     | medium | `external_distribution` | E    |
| `accepted_gap`                        | Total gap accepted between lead and lag vehicles     | m     | pair   | yes     | medium | `external_distribution` | E    |
| `thw_post_change`                     | Following vehicle's THW immediately after the cut-in | s     | pair   | partial | medium | `local_distribution`    | E    |
| `ttc_post_change`                     | Following vehicle's minimum TTC after the cut-in     | s     | pair   | partial | medium | `research_threshold`    | E    |
| `induced_response`                    | See Section 8.12                                     | n/a   | pair   | no      | medium | n/a                     | E    |

**A cut-in that leaves a following vehicle with a 0.6 s THW and a 1.1 s TTC is a
measured interaction.** An "aggressive lane change" classifier is a guess. Build
the former.

**Completeness gate.** Compute manoeuvre metrics only when the whole manoeuvre
is inside the field of view, with a stated margin before and after. A partially
observed lane change yields a truncated duration and an understated
displacement, and must be marked incomplete rather than reported.

### 8.9 Cyclist and vulnerable-road-user overtaking

This is the sensor's strongest case and should be treated as a headline
capability rather than a subsection of lane changes. An overtaking driver's own
sensors see the clearance poorly; a cyclist's rear camera sees one side and no
absolute geometry. A roadside LiDAR sees both parties and the road.

#### The primitive is separation, not lateral distance

The previous draft defined the quantity as a lateral distance. That is a
special case, and it silently assumes a straight road, parallel paths and a
well-defined lateral axis. It breaks on curved roads, on angled cyclist paths,
during lane changes, at intersections, and for any crossing interaction.

The universal primitive is:

```
min_separation = min over synchronised time t of
                 surface_to_surface_distance( geom_A(t), geom_B(t) )
```

where `geom` is the estimated oriented geometry, meaning pose plus dimensions
plus their uncertainties, and **synchronised** means both states are evaluated at
the same instant after timestamp alignment, not at each track's own nearest
sample. That primitive is defined for every interaction type and every class
pair, and it needs no road frame.

`passing_clearance_lateral` is then a **projection** of that separation onto the
path-normal direction, computed only when the interaction is confidently
classified as a conventional overtake and a path-aligned frame exists. It is a
derived convenience for comparison against statutory passing distances, which
are themselves written in lateral terms.

|                                     | `min_separation`              | `passing_clearance_lateral`                                 |
| ----------------------------------- | ----------------------------- | ----------------------------------------------------------- |
| Defined for                         | All interactions, all classes | Confident overtakes with a path frame                       |
| Needs a road or path frame          | No                            | Yes                                                         |
| Compares against statute            | No                            | Yes                                                         |
| Suppression reason when unavailable | `insufficient_observation`    | `interaction_type_uncertain` or `road_geometry_unavailable` |

Both are retained. Reporting only the projection would lose the measurement on
exactly the geometries where a close pass is most dangerous.

| Feature                   | Definition                                                 | Units | Scope | Map     | RS   | Bench                             | Form |
| ------------------------- | ---------------------------------------------------------- | ----- | ----- | ------- | ---- | --------------------------------- | ---- |
| `min_separation`          | Minimum synchronised surface-to-surface distance           | m     | pair  | no      | high | `local_distribution`              | E    |
| `passing_clearance_min`   | Path-normal projection of `min_separation`, overtakes only | m     | pair  | partial | high | `legal` + `external_distribution` | E    |
| `passing_speed`           | Motor vehicle speed at closest approach                    | m/s   | pair  | no      | high | `local_distribution`              | E    |
| `cyclist_speed`           | Cyclist speed at closest approach                          | m/s   | pair  | no      | high | n/a                               | E    |
| `relative_speed_pass`     | Difference at closest approach                             | m/s   | pair  | no      | high | `local_distribution`              | E    |
| `duration_alongside`      | Time with longitudinal overlap                             | s     | pair  | no      | high | n/a                               | E    |
| `longitudinal_separation` | Signed along-path offset over the manoeuvre                | m     | pair  | no      | high | n/a                               | C    |
| `close_pass_rate`         | `close_pass_count / cyclist_overtake_count`                | ratio | pair  | no      | high | `legal` + `X`                     | X    |

**Clearance and speed are reported separately and both retained.** A 1.2 m pass
at 20 km/h and a 1.2 m pass at 60 km/h are different events. If a combined risk
function is ever introduced, it is computed from these fields and never replaces
them.

Statutory thresholds are jurisdiction-specific and belong in site configuration,
not in code: 1.5 m in several EU jurisdictions, 3 ft in over half of US states,
1.0 or 1.5 m speed-tiered in Western Australia. Compliance rates measured
elsewhere are strikingly low, with observational work reporting roughly half of
drivers meeting a 1.5 m requirement in one German city and lower rates in rural
Austria, which means the _distribution_ is the interesting output and a
pass-fail count discards most of it.

The analogous vehicle-to-pedestrian clearance is meaningful wherever pedestrians
share carriageway space without a kerb separation, and meaningless where a kerb
fixes the geometry. Gate it on scene context rather than computing it
everywhere.

### 8.10 Yielding behaviour

For a vehicle approaching a pedestrian or cyclist crossing its path:

| Feature                         | Definition                                                         | Units | Scope | Map     | RS     | Bench                      | Form |
| ------------------------------- | ------------------------------------------------------------------ | ----- | ----- | ------- | ------ | -------------------------- | ---- |
| `distance_at_braking_onset`     | Distance to the conflict point when sustained deceleration begins  | m     | pair  | partial | high   | `local_distribution`       | E    |
| `time_to_conflict_at_onset`     | Time to conflict point at the same instant                         | s     | pair  | partial | high   | `local_distribution`       | E    |
| `yield_decel`                   | Mean and peak deceleration during the yield                        | m/s²  | pair  | no      | high   | `local_distribution`       | E    |
| `stop_distance_before_crossing` | Distance from the crossing edge at minimum speed                   | m     | pair  | yes     | high   | `legal`                    | E    |
| `min_speed_at_crossing`         | Minimum speed during the interaction                               | m/s   | pair  | partial | high   | n/a                        | E    |
| `yield_outcome`                 | `full_stop`, `rolling`, `slowed`, `no_change`                      | enum  | pair  | partial | high   | `no_established_threshold` | E    |
| `vru_evasive`                   | Whether the vulnerable road user changed speed or path in response | bool  | pair  | no      | medium | n/a                        | E    |

This set distinguishes an early smooth yield from a late hard yield from a
failure to yield from an interaction where the vulnerable road user gave way
instead. That distinction is the entire value of the metric.

**Do not assign legal fault from trajectory data.** Right of way depends on
signage, signal state, local statute and often on what each party could see.
None of that is in the point cloud. Report the geometry and the outcome; let a
human apply the law.

### 8.11 Stop behaviour

| Feature                  | Definition                                                                      | Units | Scope  | Map | RS   | Bench                | Form |
| ------------------------ | ------------------------------------------------------------------------------- | ----- | ------ | --- | ---- | -------------------- | ---- |
| `approach_speed_stop`    | Speed at a stated distance upstream                                             | m/s   | single | yes | high | `local_distribution` | E    |
| `braking_onset_distance` | Distance from the stop line at braking onset                                    | m     | single | yes | high | n/a                  | E    |
| `min_speed`              | Minimum speed over the approach                                                 | m/s   | single | no  | high | n/a                  | E    |
| `stop_position`          | Signed distance of the vehicle's front face from the stop line at minimum speed | m     | single | yes | high | `legal`              | E    |
| `stop_duration`          | Time below a stated speed epsilon                                               | s     | single | no  | high | `legal`              | E    |
| `crossing_speed`         | Speed at the stop line                                                          | m/s   | single | yes | high | n/a                  | E    |
| `stop_outcome`           | `full_stop`, `rolling_stop`, `no_stop`                                          | enum  | single | yes | high | `legal`              | E    |

**Physical cessation and legal compliance are separate.** `min_speed` below an
epsilon is a physical fact with a measurement uncertainty. Whether it satisfies
a jurisdiction's requirement depends on that jurisdiction's wording, and few
require a specific dwell duration. Record `stop_duration` as an observable and
apply any dwell requirement as a `legal` benchmark carrying its jurisdiction.

The speed epsilon is not free: at 5 Hz with a 0.05 m measurement, speed
uncertainty near zero is roughly 0.2 to 0.35 m/s, so an epsilon below about
0.3 m/s is measuring noise. State the epsilon with the result.

### 8.12 Induced evasive response

A neutral representation for cases where one road user's motion is temporally
associated with a defensive response by another.

| Field                                                          | Meaning                                         |
| -------------------------------------------------------------- | ----------------------------------------------- |
| `actor_track_id`, `responder_track_id`                         | Who did what, without implying fault            |
| `candidate_manoeuvre`                                          | The actor motion the response follows           |
| `response_latency`                                             | Time from candidate manoeuvre to response onset |
| `responder_decel`, `responder_lateral_accel`, `responder_jerk` | Response magnitude, each with uncertainty       |
| `min_ttc`, `min_pet`, `min_separation`                         | Proximity during the episode                    |
| `confidence`                                                   | How well-observed both tracks were              |

**Causal attribution is much harder than geometric measurement, and this is an
evidence surface rather than a classifier.** A vehicle braking after a cut-in
may be responding to the cut-in, to a signal change, to a pedestrian, or to
nothing observable. The structure records the temporal association and the
magnitudes; it does not assert a cause. The name `InducedResponse` is
deliberately weaker than "caused by".

This connects directly to the traffic-conflict technique's evasive-action
criterion, and to the estimation plan's Phase 8 abnormal-motion evidence
surface, which supplies the residual and model-validity signals that make a
sudden response distinguishable from a tracking artefact.

### 8.13 Interaction geometry

| Feature                               | Meaningful for                |
| ------------------------------------- | ----------------------------- |
| `closest_point_of_approach`           | All interaction types         |
| `min_separation` (surface to surface) | All                           |
| `projected_path_intersection`         | Crossing, merging, turning    |
| `conflict_point`, `conflict_angle`    | Crossing, merging, turning    |
| `closing_velocity`                    | Following, opposite-direction |
| `relative_velocity_vector`            | All                           |
| `THW`, `TTC` (longitudinal form)      | **Following only**            |
| `PET`                                 | Crossing, merging, turning    |
| `passing_clearance`                   | Overtaking                    |

**Do not apply the longitudinal TTC formula to arbitrary crossing
trajectories.** `gap / closing_speed` assumes a shared one-dimensional path. For
crossing paths the correct treatment is either a two-dimensional TTC over
projected extents or, better for this sensor, PET, which needs no motion
assumption at all. Choosing the wrong formula produces confident numbers that
mean nothing, which is worse than reporting none.

Interaction type is therefore classified **first**, from the conflict angle and
relative motion, and it determines which metrics are computed at all.

## 9. Uncertainty propagation and suppression

Every metric here is a function of estimated quantities, so every metric has an
uncertainty. The rule from the estimation plan applies throughout: **a metric is
unavailable rather than falsely precise when observability is inadequate.**

Uncertainties are taken from the `final` state covariance per track, not from
constants. The illustrative numbers below use a filtered speed sigma of
0.15 m/s and a position sigma of 0.05 m, which are the estimation plan's Phase 2
targets; the real values come from the covariance at evaluation time.

### 9.1 Derived suppression rules

| Metric                | Propagation                                                                         | Suppression rule                                                                                                                                                                                                                                                                                                                                                                                      |
| --------------------- | ----------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ttc`                 | `σ_TTC/TTC = sqrt((σ_gap/gap)² + (σ_Δv/Δv)²)`                                       | **The dominant failure mode.** As closing speed approaches zero, TTC diverges and its uncertainty diverges faster. With `σ_Δv = √2 · σ_v ≈ 0.21 m/s`, a closing speed of 0.5 m/s gives a 42 % relative error. Suppress when `Δv < 3 σ_Δv`, about 0.64 m/s at the illustrative sigma. Report a confidence interval, never a bare value                                                                 |
| `thw`                 | `σ_THW/THW = sqrt((σ_gap/gap)² + (σ_v/v)²)`                                         | Suppress when `speed_F` is below the stop epsilon; THW is undefined at rest, not infinite                                                                                                                                                                                                                                                                                                             |
| `gap`                 | `σ_gap² = σ_sL² + σ_sF² + (σ_LL/2)² + (σ_LF/2)²`                                    | Inherits both vehicles' **length** uncertainty. Suppress when either length belief has fewer observations than the estimation plan's dimension-convergence minimum                                                                                                                                                                                                                                    |
| `passing_clearance`   | `σ_c² = σ_lat² + (σ_Wv/2)² + (σ_Wc/2)²`                                             | **Dominated by extent, not position.** A cyclist is narrow and sparsely sampled: a per-track width sigma of 0.3 m contributes 0.15 m by itself, against a legal discrimination need of about 0.1 m. Therefore **cyclist and pedestrian widths must come from a class prior with a tight sigma, not from a per-track estimate**. This is the single most important uncertainty conclusion in this plan |
| `pet`                 | `σ_PET² = (σ_sA/v_A)² + (σ_sB/v_B)² + (σ_boundary/v)²`                              | Robust: at 0.05 m and 10 m/s the crossing-time sigma is 5 ms, and at pedestrian speeds about 50 ms. The conflict-region boundary definition, not the trajectory, is the larger term. Suppress only when either party's crossing is not fully in view                                                                                                                                                  |
| `accel_long`          | From the state covariance directly, once acceleration is a state                    | Suppress below the estimation plan's minimum temporal support, about 1 s                                                                                                                                                                                                                                                                                                                              |
| `jerk_*`              | Amplified by `1/dt^3` if differenced; from the refined acceleration state otherwise | **Experimental.** Suppress when the passage is shorter than the declared window. Always emit the bandwidth. See 8.2.1                                                                                                                                                                                                                                                                                 |
| `lateral_accel`       | `σ² = (ω σ_v)² + (v σ_ω)²`                                                          | The `v σ_ω` term dominates because yaw rate is weakly observable at 5 Hz. Suppress when the yaw-rate variance exceeds a stated bound                                                                                                                                                                                                                                                                  |
| `stop_position`       | `σ² = σ_s² + σ_pose² + σ_stopline²`                                                 | **Not computable today**: the pipeline runs in sensor coordinates with a nil pose, so `σ_pose` is undefined. Blocked on the site frame                                                                                                                                                                                                                                                                |
| `lateral_offset_lane` | `σ² = σ_lat² + σ_pose² + σ_lane_geometry²`                                          | Same block. The map's own accuracy is a term, and a hand-drawn lane centreline can easily dominate the trajectory error                                                                                                                                                                                                                                                                               |
| Any pairwise metric   | Both tracks contribute                                                              | Suppress when either track is below the quality floor, or when either was coasting rather than observed at the relevant instant                                                                                                                                                                                                                                                                       |

### 9.2 Coasting is not observation

The estimation plan persists a `final` state for frames where the tracker was
coasting through an occlusion. Those states are legitimate trajectory estimates
and they are **not** observations. A pairwise metric evaluated at an instant
when either party was coasting must be marked as such, and an exposure
denominator must exclude coasted time. Otherwise a vehicle hidden behind a van
for six frames silently contributes six frames of fictitious following
exposure.

### 9.3 A scalar sigma is not always enough

A single Gaussian sigma is adequate for primitives such as speed, separation and
position. It is wrong for several of the metrics that matter most here, and
being wrong in a way that understates the tail is the dangerous direction.

| Metric                                                  | Why a scalar sigma misleads                                                                                                                                                                                  |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `ttc`                                                   | `gap / closing` is a ratio whose denominator can approach zero. Its distribution is heavy-tailed and asymmetric; a symmetric interval around the point estimate is meaningless near the suppression boundary |
| `pet`                                                   | A difference of two threshold-crossing times, each of which is a first-passage quantity, not a Gaussian sample                                                                                               |
| `min_separation`, `min_speed`, and every other extremum | The minimum of a correlated series. Its distribution is skewed toward the observed value: an extremum estimated from noisy samples is biased low, and symmetric error bars hide that                         |
| `stop_duration`                                         | Time between two threshold crossings, bounded below at zero                                                                                                                                                  |
| Any threshold-crossing time                             | Same                                                                                                                                                                                                         |

The data model must therefore support more than one uncertainty representation,
chosen per metric rather than imposed globally:

| Representation                                                        | Use                                                                                   |
| --------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `sigma`                                                               | Symmetric, approximately Gaussian primitives                                          |
| `interval` with a stated coverage, for example 5th to 95th percentile | Asymmetric or heavy-tailed quantities                                                 |
| `bounds`, lower and upper                                             | Quantities with a hard physical bound, and lower-bound-only cases                     |
| `method`                                                              | How it was propagated: `analytic`, `linearised`, `sigma_point`, `monte_carlo`         |
| `samples`                                                             | Sample count, when the method is stochastic, so the interval's own precision is known |

**Do not require every metric to use the same representation**, and do not
default to sigma because it is convenient. A metric whose uncertainty is
genuinely asymmetric and is reported as a sigma is misreported.

Sigma-point or Monte Carlo propagation is the honest option for the extrema and
the ratios, and it is affordable: these are per-event computations over a
handful of states, not per-frame ones.

### 9.4 Pairwise metrics are not independent

Propagating two tracks' uncertainties as though they were independent is wrong,
and it is wrong in both directions depending on the error source.

| Uncertainty source                         | Independent between tracks?              | Effect on a relative measurement                                                                                                                     |
| ------------------------------------------ | ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| Per-track estimation noise                 | Yes, approximately                       | Adds in quadrature, as expected                                                                                                                      |
| Extent belief per track                    | Yes                                      | Adds; dominates clearance, per 9.1                                                                                                                   |
| Sensor pose and site calibration           | **No: common mode**                      | Largely **cancels** in a relative measurement, so treating it as independent overstates the uncertainty                                              |
| Timestamp alignment between the two states | **No: common mode**, but does not cancel | A shared timing error moves both objects along their own paths, so it survives as a separation error proportional to relative speed                  |
| Road-surface and map geometry              | **No: common mode**                      | Cancels for separation between two users on the same surface; does not cancel across a surface discontinuity, which is exactly the intersection case |

Two consequences worth stating plainly. Passing clearance benefits from
cancellation: both parties are localised against the same sensor pose, so a
pose error moves both together and largely drops out of their separation. PET
does not benefit in the same way, because it is a difference of times rather
than of positions, and a timestamp error enters directly.

Three uncertainty scopes are therefore tracked separately rather than summed
into one number:

```text
track_local      per-track estimation and extent uncertainty
shared_sensor    sensor pose, site calibration, timestamp alignment
shared_geometry  road-surface and map geometry
```

**The first increment does not need to implement the cancellation.** It needs to
avoid making independence irreversible: the three scopes are carried separately
in the data model from the start, and the first implementation may simply sum
them. Retrofitting a scope distinction after metrics have shipped with a single
opaque sigma is the expensive path.

## 10. Data model

### 10.1 Roadway context does not exist yet, including the speed limit

An inspection finding that gates most `legal` benchmarks: **there is no posted
speed limit anywhere in the schema.** The `site` table carries latitude,
longitude, bounding box, map SVG and map angle, but no limit. The only speed
limit in the codebase is a per-request report parameter,
`ReportConfig.SpeedLimit`, supplied to `POST /api/generate_report`
([internal/report/config.go](../../internal/report/config.go)).

That is adequate for printing a number on a PDF and inadequate for compliance
analytics, because a limit has an effective date range and a report must be
reproducible against the limit that applied at the time. The right home is
`site_config_periods`, which already implements exactly that pattern: an
effective start and end, an `is_active` flag, and triggers enforcing a single
active period per site. Adding `speed_limit_kph` and its unit and jurisdiction
there costs one migration and makes every `legal` speed benchmark reproducible.

The remaining roadway context, lane centrelines, stop lines, crossings and
conflict regions, belongs to [lidar-l7-scene-plan](lidar-l7-scene-plan.md) and
is the reason Phase 7 exists.

### 10.2 Proposed types

Adapted to repository conventions rather than copied: track identifiers are
**strings** (`trk_<uuid>`, per `TrackedObject.TrackID`), not integers;
timestamps are `TSUnixNanos int64`; analytics uses `float64`; JSON tags are
snake case.

```go
package l8behaviour // new, under internal/lidar/

// ---------------------------------------------------------------------------
// Uncertainty: not always a scalar
// ---------------------------------------------------------------------------

// UncertaintyKind selects the representation. See 9.3: forcing every metric
// into a symmetric sigma misreports the ratios and the extrema.
type UncertaintyKind uint8

const (
    UncertaintyNone UncertaintyKind = iota
    UncertaintySigma                 // symmetric, approximately Gaussian
    UncertaintyInterval              // asymmetric, with a stated coverage
    UncertaintyBounds                // hard lower and/or upper bound
)

// PropagationMethod records how the uncertainty was obtained, because an
// analytic bound and a 200-sample Monte Carlo interval deserve different trust.
type PropagationMethod string

const (
    PropagationAnalytic   PropagationMethod = "analytic"
    PropagationLinearised PropagationMethod = "linearised"
    PropagationSigmaPoint PropagationMethod = "sigma_point"
    PropagationMonteCarlo PropagationMethod = "monte_carlo"
)

type Uncertainty struct {
    Kind   UncertaintyKind   `json:"kind"`
    Sigma  float64           `json:"sigma,omitempty"`
    Lower  *float64          `json:"lower,omitempty"`
    Upper  *float64          `json:"upper,omitempty"`
    Coverage float64         `json:"coverage,omitempty"` // e.g. 0.90 for a 5th-95th interval
    Method PropagationMethod `json:"method"`
    Samples int              `json:"samples,omitempty"`

    // Contributions by scope, per 9.4. Carried separately from the start so
    // that common-mode cancellation can be introduced later without
    // reinterpreting stored values.
    TrackLocal     float64 `json:"track_local,omitempty"`
    SharedSensor   float64 `json:"shared_sensor,omitempty"`
    SharedGeometry float64 `json:"shared_geometry,omitempty"`
}

// ---------------------------------------------------------------------------
// Provenance
// ---------------------------------------------------------------------------

// Provenance answers "why should I believe this number?" for every result.
// Four version axes, because a result can change for four independent reasons.
type Provenance struct {
    EstimateStage string `json:"estimate_stage"` // must be "final" for production
    EstimatorID   string `json:"estimator_id"`
    ObsModelID    string `json:"obs_model_id"`
    MethodID      string `json:"method_id"`      // behaviour method and threshold set
    GeometryID    string `json:"geometry_id,omitempty"` // road/map geometry version
    ParamHash     string `json:"param_hash"`

    // What was actually used.
    ContributingTrackIDs []string `json:"contributing_track_ids"`
    FirstUnixNanos       int64    `json:"first_unix_nanos"`
    LastUnixNanos        int64    `json:"last_unix_nanos"`
    ObservedFrames       int      `json:"observed_frames"`
    CoastedFrames        int      `json:"coasted_frames"`
    PlanarFallback       bool     `json:"planar_fallback"`
}

// ---------------------------------------------------------------------------
// Results: numeric and categorical are different things
// ---------------------------------------------------------------------------

// SuppressionReason is a closed vocabulary; see 7.2.
type SuppressionReason string

// BehaviourMeasurement is a numeric result. Either Value is meaningful, or
// Suppressed is set and Reason explains why. Never both, never neither.
type BehaviourMeasurement struct {
    Name  string  `json:"name"`
    Value float64 `json:"value"`
    Unit  string  `json:"unit"`

    Uncertainty Uncertainty `json:"uncertainty"`

    Suppressed bool              `json:"suppressed"`
    Reason     SuppressionReason `json:"reason,omitempty"`

    Benchmark  *Benchmark `json:"benchmark,omitempty"`
    Percentile *float64   `json:"percentile,omitempty"`

    // Exposure denominator for rate-form metrics, counting only `observed`
    // support per 7.3.
    OpportunitySeconds float64 `json:"opportunity_seconds,omitempty"`

    Provenance Provenance `json:"provenance"`
}

// BehaviourOutcome is a categorical result. It never replaces its evidence:
// DerivedFrom names the measurements that produced it, and they are stored.
type BehaviourOutcome struct {
    Name  string `json:"name"`  // e.g. "stop_outcome"
    Value string `json:"value"` // e.g. "rolling_stop"

    // Confidence in the categorisation itself, distinct from the uncertainty
    // of the measurements behind it.
    Confidence float64 `json:"confidence"`

    // The measurements this outcome was derived from, by name, and the
    // threshold set that mapped them to a category.
    DerivedFrom  []string `json:"derived_from"`
    ThresholdSet string   `json:"threshold_set"`

    Suppressed bool              `json:"suppressed"`
    Reason     SuppressionReason `json:"reason,omitempty"`

    Provenance Provenance `json:"provenance"`
}

// ---------------------------------------------------------------------------
// Passages and interactions
// ---------------------------------------------------------------------------

// ObservationSupport is the per-instant support state; see 7.3.
type ObservationSupport string

const (
    SupportObserved         ObservationSupport = "observed"
    SupportCoasted          ObservationSupport = "coasted"
    SupportOccludedInferred ObservationSupport = "occluded_inferred"
    SupportMissedUnknown    ObservationSupport = "missed_unknown"
    SupportClusterMerged    ObservationSupport = "cluster_merged"
    SupportClusterSplit     ObservationSupport = "cluster_split"
    SupportOutOfFOV         ObservationSupport = "out_of_fov"
)

// PassageSummary is the per-track behavioural record: one road user, one
// traverse of one site. Deliberately not called a driver profile.
type PassageSummary struct {
    TrackID  string `json:"track_id"`
    SiteID   int64  `json:"site_id"`
    SensorID string `json:"sensor_id"`

    // Motion class drives applicability, per 7.1. Confidence is carried so a
    // low-confidence class can gate class-specific metrics.
    MotionClass     string  `json:"motion_class"`
    ClassLabel      string  `json:"class_label"`
    ClassConfidence float64 `json:"class_confidence"`

    StartUnixNanos int64 `json:"start_unix_nanos"`
    EndUnixNanos   int64 `json:"end_unix_nanos"`

    ObservedPathMetres float64 `json:"observed_path_metres"`
    SupportSeconds     map[ObservationSupport]float64 `json:"support_seconds"`

    Measurements []BehaviourMeasurement `json:"measurements"`
    Outcomes     []BehaviourOutcome     `json:"outcomes"`
}

// InteractionType is classified with a posterior, not asserted; see 7.5.
type InteractionType string

const (
    InteractionFollowing InteractionType = "following"
    InteractionCrossing  InteractionType = "crossing"
    InteractionMerging   InteractionType = "merging"
    InteractionOvertake  InteractionType = "overtake"
    InteractionOpposing  InteractionType = "opposing"
)

type InteractionClassification struct {
    Posterior  map[InteractionType]float64 `json:"posterior"`
    MAP        InteractionType             `json:"map"`
    Confidence float64                     `json:"confidence"`

    // Evidence retained so a surprising suppression can be explained.
    ConflictAngleRad *float64 `json:"conflict_angle_rad,omitempty"`
    PathOverlap      *float64 `json:"path_overlap,omitempty"`
    RelativeSpeedMps *float64 `json:"relative_speed_mps,omitempty"`
}

// InteractionEvent is a pairwise encounter. Primary and Secondary name roles in
// the geometry, not fault.
type InteractionEvent struct {
    EventID          string `json:"event_id"`
    PrimaryTrackID   string `json:"primary_track_id"`
    SecondaryTrackID string `json:"secondary_track_id"`

    Classification InteractionClassification `json:"classification"`

    StartUnixNanos int64 `json:"start_unix_nanos"`
    EndUnixNanos   int64 `json:"end_unix_nanos"`

    // Conflict geometry. ConflictFromMap distinguishes a named, persistent
    // region from one derived by intersecting two observed paths.
    ConflictX, ConflictY *float64 `json:"conflict_x,omitempty"`
    ConflictAngleRad     *float64 `json:"conflict_angle_rad,omitempty"`
    ConflictFromMap      bool     `json:"conflict_from_map"`
    SurfaceContinuous    bool     `json:"surface_continuous"` // false across a grade discontinuity: see 9.4

    Measurements []BehaviourMeasurement `json:"measurements"`
    Outcomes     []BehaviourOutcome     `json:"outcomes"`

    // Worst support state across both parties during the measured interval.
    WorstSupport ObservationSupport `json:"worst_support"`
}

// ExposureWindow is an opportunity denominator, stored explicitly so a rate can
// be audited and recomputed. Seconds counts `observed` support only.
type ExposureWindow struct {
    TrackID        string  `json:"track_id"`
    Kind           string  `json:"kind"` // "free_flow", "valid_following", "yield_opportunity", "overtake"
    Seconds        float64 `json:"seconds"`
    StartUnixNanos int64   `json:"start_unix_nanos"`
    EndUnixNanos   int64   `json:"end_unix_nanos"`
}
```

Five departures from the sketch in the brief, each deliberate.

**Track identifiers are strings.** `TrackedObject.TrackID` is `trk_<uuid>`. An
`int64` here would not compile against the existing tracker.

**Numeric and categorical results are different types.** Forcing "rolling stop"
through a `float64` would be exactly the opaque enum-to-number encoding this
plan exists to avoid. `BehaviourOutcome` keeps a reference to the measurements
it was derived from, so the evidence survives the categorisation.

**Uncertainty is a struct, not a float.** Ratios and extrema are not symmetric,
and the three uncertainty scopes are carried separately from the start so that
common-mode cancellation is a later refinement rather than a migration.

**Benchmark is a struct, not an ID string.** A legal benchmark needs a
jurisdiction and an effective date; a research threshold needs a citation; a
distribution needs its stratification. Flattening them loses exactly the
provenance this plan exists to preserve.

**Interaction type carries a posterior.** It gates which metrics are computed at
all, and an ambiguous classification suppresses rather than picks the argmax.

### 10.3 Persistence

| Table                      | Content                                                       | Notes                                                                                                             |
| -------------------------- | ------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `lidar_passage_summaries`  | One row per track per estimator version                       | Keyed `(track_id, estimator_id)`; metrics as a JSON column, following the schema's existing JSON-first convention |
| `lidar_interaction_events` | One row per pairwise encounter                                | Indexed on both track ids and on `type`                                                                           |
| `lidar_exposure_windows`   | Opportunity denominators                                      | Separate so a rate can be recomputed without re-running the pipeline                                              |
| `site_config_periods`      | **Add** `speed_limit_kph`, `speed_limit_unit`, `jurisdiction` | Existing effective-date pattern; see 9.1                                                                          |

Behaviour output is derived data and must be reproducible from the persisted
final estimates. It therefore carries `estimator_id` and `param_hash`, and a
change to either invalidates the derived rows rather than silently mixing
versions.

## 11. Evaluation datasets

What each source can and cannot validate. Claiming validation from a dataset
lacking the necessary signal is the failure mode to avoid.

| Source                                                                                           | Can validate                                                                                                                                     | Cannot validate                                                                                                                                                                                                        |
| ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| velocity.report **synthetic corpus** (estimation plan Section 16.3)                              | Exact numerical correctness of every equation: THW, TTC, PET, DRAC, clearance, against analytically known trajectories                           | Anything about real behaviour distributions                                                                                                                                                                            |
| velocity.report **soma static captures**                                                         | Real static-sensor observability: how often both parties of an interaction are actually in view, fragmentation rates, achievable passage lengths | Behavioural norms; one site, one afternoon                                                                                                                                                                             |
| velocity.report **local field data**                                                             | Local distributions, the `local_distribution` benchmark itself, and whether a metric is computable at production rates                           | External comparability                                                                                                                                                                                                 |
| **SHRP2 NDS** ([DOT HS 812 858](https://rosap.ntl.bts.gov/view/dot/44242/dot_44242_DS1.pdf))     | The free-flow-opportunity method, and naturalistic speeding distributions with a real opportunity denominator                                    | Roadside geometry, interactions, clearance                                                                                                                                                                             |
| **NHTSA 100-Car** ([DOT HS 810 593](https://www.nhtsa.gov/sites/nhtsa.gov/files/100carmain.pdf)) | Naturalistic following and the event taxonomy of crashes, near-crashes and incidents                                                             | A headway safety threshold, which it does not establish                                                                                                                                                                |
| **highD** ([arXiv:1810.05642](https://arxiv.org/abs/1810.05642))                                 | Freeway following, lane changes, gap acceptance, from an overhead view with good geometry                                                        | Urban, low-speed, VRU, intersection behaviour                                                                                                                                                                          |
| **inD** ([arXiv:1911.07602](https://arxiv.org/abs/1911.07602))                                   | Urban intersections including pedestrians and cyclists: the closest external analogue to our deployment                                          | German road design and rules                                                                                                                                                                                           |
| **rounD** ([ITSC 2020](https://doi.org/10.1109/ITSC45102.2020.9294728))                          | Roundabout interactions and gap acceptance                                                                                                       | Non-roundabout geometry                                                                                                                                                                                                |
| **openDD** ([arXiv:2007.08463](https://arxiv.org/html/2007.08463))                               | Large-scale roundabout trajectories with HD maps                                                                                                 | Same                                                                                                                                                                                                                   |
| **Waymo Open Motion**                                                                            | Geometry and interaction structure at scale                                                                                                      | **Not a normative human-driving baseline**: it is collected around an autonomous vehicle whose presence and behaviour influence the surrounding traffic, and it is sampled for interest rather than representativeness |

The drone datasets deserve a specific note. They are recorded from overhead with
near-uniform visibility, which means their occlusion and observability
characteristics are _better_ than ours. They are excellent for validating that
an equation is implemented correctly and for external distributions. They are
poor for predicting what fraction of interactions our sensor will actually
observe, which is what the soma captures are for.

## 12. Roadmap

### 12.1 Two orderings, and they differ

Engineering dependency order and product priority are not the same list, and
conflating them lets implementation convenience masquerade as importance.

| Engineering dependency order       | Product priority                         |
| ---------------------------------- | ---------------------------------------- |
| 1. Universal trajectory primitives | 1. **Vehicle-cyclist passing clearance** |
| 2. Single-track kinematics         | 2. PET and conflict analysis             |
| 3. Pairwise geometry               | 3. Pedestrian yielding                   |
| 4. Empirical-path behaviour        | 4. Intersection approach behaviour       |
| 5. Roadway context                 | 5. Stop compliance                       |
| 6. Complex interaction metrics     | 6. Following exposure                    |
|                                    | 7. Acceleration and braking              |
|                                    | 8. Jerk                                  |

The two lists are close to inverted at the top. Acceleration and braking are
technically the easiest thing here and near the bottom of the product list.
Passing clearance is the top product priority and depends on pairwise geometry,
class priors for vulnerable-road-user extents, and a converged extent belief.

Practical consequences for sequencing:

- **Phase 6A ships the primitives, not the product.** Its output is
  infrastructure plus a suppression-rate report, not a headline metric. Say so,
  so that its completion is not mistaken for delivering value to a user.
- **Pull passing-clearance dependencies forward.** The class-prior extent model
  from 9.1, and the `min_separation` primitive from 8.9, are Phase 6B work that
  should be scheduled early within it rather than last.
- **Jerk is last in both orderings**, which is the one place they agree.

### 12.2 Gating

Development and emission are gated differently, per 2.1.

|                                                                                                                            | Gate                                                                                                      |
| -------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| Building and testing any phase below against synthetic trajectories, fixture `EstimatedState` streams or external datasets | **None.** All phases may begin now                                                                        |
| Emitting a production result from live or replayed LiDAR tracks                                                            | **G-SMO-1**, plus the phase's own acceptance criteria and the per-metric applicability rules in Section 7 |

### Phase 6A: single-track kinematics

**Goal.** Passage-level speed, acceleration, braking, curvature, and stop
behaviour that needs no map.

**Inputs.** `final` estimates with covariance, or a fixture stream during development. No roadway context.

**Files.** New `internal/lidar/l8behaviour/`; `Measurement`, `PassageSummary`,
`ExposureWindow`; migration for `lidar_passage_summaries` and
`lidar_exposure_windows`.

**Tests.** Every metric against synthetic trajectories with analytically known
values. Suppression assertions: jerk refused below the window, THW refused at
rest, acceleration refused below temporal support.

**Offline dataset.** Synthetic corpus for correctness; soma captures for
observability and computability rates.

**Acceptance.** Each metric reproduces synthetic ground truth within a stated
tolerance and carries a sigma. Jerk emits its bandwidth or nothing. Passage
summaries computed for a full soma replay with a published rate of suppressed
metrics per class.

**Suppression conditions.** Passage shorter than the metric's minimum support;
coasted fraction above a stated bound; track quality below floor.

### Phase 6B: pairwise interactions

**Goal.** Gap, headway, TTC, DRAC, closest approach, and PET from
trajectory-derived conflict points.

**Inputs.** Phase 6A. Interaction classification. No map required, which is the
notable result: PET works from observed path intersections.

**Files.** `l8behaviour/interaction.go`; `InteractionEvent`; migration for
`lidar_interaction_events`.

**Tests.** Two-body synthetic scenarios with analytic answers for each
interaction type. Explicit negative tests that the longitudinal TTC formula is
**not** applied to crossing pairs.

**Offline dataset.** Synthetic for correctness; highD for following and lane
changes; inD for urban crossing interactions; soma for how often both parties
are simultaneously observable.

**Acceptance.** Interaction type classified before metric computation, with a
published confusion matrix on synthetic data. TTC suppressed below the closing
speed floor in at least the fraction the uncertainty model predicts. PET
uncertainty within the derived bound.

**Suppression conditions.** Either party coasting; either party's extent belief
unconverged; closing speed below `3 σ_Δv`.

### Phase 6C: empirical-path behaviour

**Goal.** Dominant-path extraction, deviation from it, oscillation, and the
local distributions that make `local_distribution` benchmarks possible.

**Inputs.** Weeks of Phase 6A output. Still no map.

**Files.** `l8behaviour/path.go`; local distribution store with stratification
by site, direction, hour, class and traffic state.

**Tests.** Dominant path recovered from synthetic traffic with known lane
geometry, and shown to differ from the true centreline when the synthetic
population is deliberately offset.

**Acceptance.** Percentile queries answerable with a stated minimum sample size
per stratum, below which a percentile is suppressed rather than reported.

**Suppression conditions.** Fewer than a stated number of passages in the
stratum.

### Phase 7: roadway-context metrics

**Goal.** Speed-limit compliance, lane-relative position, stop-line compliance,
lane changes with target-lane gaps, intersection movement.

**Inputs.** Site frame, per [lidar-static-pose-alignment-plan](lidar-static-pose-alignment-plan.md).
Roadway geometry, per [lidar-l7-scene-plan](lidar-l7-scene-plan.md). Speed limit
in `site_config_periods`, per Section 10.1.

**Acceptance.** Every `legal` benchmark carries its jurisdiction and effective
date. Lane-relative metrics propagate map uncertainty and suppress when the map
term dominates the trajectory term.

**Suppression conditions.** No active site pose; no geometry for the region
traversed; map accuracy unstated.

### Phase 7/8: vulnerable-road-user and conflict analytics

**Goal.** Cyclist passing clearance, yielding, PET against named conflict
regions, conflict angle, and the induced-response evidence surface.

**Inputs.** Phase 6B, Phase 7 geometry, class priors for VRU extents per
Section 9.1, and the estimation plan's Phase 8 abnormal-motion evidence.

**Acceptance.** Passing clearance sigma below 0.1 m for the class-prior width
model, verified against synthetic scenes with known geometry. Yield outcomes
distinguished on labelled real encounters. `InducedResponse` emits association
and magnitude, never causation.

**Suppression conditions.** VRU class uncertain; either party partially
observed at closest approach; clearance sigma above the legal discrimination
need.

## 13. Open research questions

| #   | Question                                                                                               | Why it is unresolved                                                                                                                                                        |
| --- | ------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| B1  | How often are **both** parties of an interaction simultaneously well-observed at a real roadside site? | This determines whether Phase 6B is a headline capability or a rare event. Measurable today from the soma captures; nothing else in this plan matters as much               |
| B2  | What is the achievable sigma on cyclist width from a class prior, and is 0.1 m reachable?              | Passing clearance, the strongest use case, depends entirely on it                                                                                                           |
| B3  | Is a trajectory-derived conflict point stable enough across encounters to aggregate PET without a map? | Decides whether PET is genuinely Phase 6B or effectively Phase 7                                                                                                            |
| B4  | What is the right free-flow definition for a short roadside window?                                    | SHRP2's free-flow episodes are defined over trips. A ten-second window needs its own defensible definition, and getting it wrong invalidates every speeding exposure metric |
| B5  | Can interaction type be classified reliably at 5 Hz from conflict angle and relative motion?           | Everything in 6B is gated on classifying first                                                                                                                              |
| B6  | Do local distributions stabilise at achievable sample sizes per stratum?                               | Stratifying by site, direction, hour, class and traffic state multiplies quickly; the minimum viable stratification is unknown                                              |
| B7  | What dwell duration, if any, does the deployment jurisdiction require for a legal stop?                | Determines whether `stop_duration` carries a `legal` benchmark at all                                                                                                       |
| B8  | Does the SSAM PET default of 5 s appear in the primary user manual?                                    | Confirmed only from secondary sources; see Section 5.1                                                                                                                      |
| B9  | How much does the estimation plan's association rate improvement move behaviour feasibility?           | At 43.6 % the effective rate is 5 Hz; at 90 % it approaches 10 Hz, which roughly halves the jerk window and doubles interaction observability                               |

## 14. Answers to the questions this revision raised

**1. Which metrics apply to all road users?**
The universal trajectory primitives: position and path, speed, velocity vector,
path length, observed duration, trajectory curvature, acceleration where
observable, per-state uncertainty, trajectory confidence and observation
coverage. Plus, on the interaction side, `min_separation` and conflict geometry,
which are defined for any pair of estimated geometries. See 7.1.

**2. Which are motor-vehicle-specific?**
Following headway and its exposure, braking and acceleration behaviour, complete
stop behaviour, lane keeping and lane departure, approach speed, and
longitudinal jerk. These are `rigid_vehicle` metrics; computing them for a
pedestrian is arithmetically possible and meaningless.

**3. Which are pairwise interaction metrics?**
Minimum synchronised surface-to-surface separation, PET, conflict point and
angle, relative speed at conflict, TTC and DRAC for confidently longitudinal
pairs, passing clearance for confident overtakes, yielding, and induced
response. Most of the strategically valuable ones are cross-class.

**4. How is metric applicability represented?**
As explicit declared requirements per metric, covering motion class, relation
between parties, estimation lifecycle, observation support, extent convergence,
geometry availability, uncertainty bounds and interaction-classification
confidence. Evaluated before the metric is computed, not after. See 7.2.

**5. How are unsupported metrics suppressed?**
With a machine-readable `SuppressionReason` from a closed vocabulary, stored and
queryable alongside the results. A suppressed metric is a first-class result,
not an absence, and the per-site rate of each reason is a diagnostic in its own
right.

**6. How are categorical outcomes separated from numeric measurements?**
Two types. `BehaviourMeasurement` carries a value, unit, uncertainty, benchmark
and provenance. `BehaviourOutcome` carries a label, a confidence, the threshold
set that produced it, and **references to the measurements it was derived from**,
which are themselves stored. "Rolling stop" always remains traceable to minimum
speed, duration below threshold, position relative to the stop zone, trajectory
uncertainty and geometry confidence.

**7. How is trajectory uncertainty propagated into behaviour results?**
Per metric, with a declared propagation method and a representation appropriate
to its distribution: sigma for symmetric primitives, intervals for ratios and
extrema, bounds where a hard limit exists. Contributions are carried in three
scopes, `track_local`, `shared_sensor` and `shared_geometry`, so that
common-mode cancellation can be introduced without reinterpreting stored values.
See 9.3 and 9.4.

**8. How are hilly and site geometries incorporated?**
Metrics are computed in the local road-surface frame where one is available, and
the result records which surface model was used or that a planar fallback
applied. On a graded site a planar-fallback result whose grade error dominates
is suppressed with `planar_fallback_insufficient`. Separation across a surface
discontinuity, meaning the intersection case, loses the common-mode cancellation
that separation on a shared surface enjoys, and `InteractionEvent` records
`SurfaceContinuous` for exactly that reason. The surface model itself is L7; see
[lidar-state-estimation-plan Section 14](lidar-state-estimation-plan.md).

**9. Which metrics require lane or road-map context?**
Lane-relative position, heading error relative to lane, lane departure,
stop-line distance and the full stop-outcome taxonomy, lane-change target-lane
gaps, and intersection movement classification. Speed-limit compliance needs
site metadata rather than geometry, and that metadata does not exist yet either;
see 10.1. Everything else in Sections 7 and 8 works in sensor or path
coordinates.

**10. Which metrics are strategically highest-value for a fixed roadside
sensor?**
Vehicle-cyclist passing clearance first, then PET and conflict analysis, then
pedestrian yielding, then intersection approach behaviour, then stop
compliance. These are the measurements a roadside sensor makes well and mobile
telemetry makes poorly or not at all. Following exposure, acceleration and
braking, and jerk are technically earlier and strategically later. See 12.1.

## 15. Changes introduced by this revision

| #   | Change                                                                                                                                      | Why                                                                                                                                                                                                            |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Added Section 2.1: development may proceed in parallel; **emission** is what G-SMO-1 gates                                                  | The previous wording idled this work behind an estimator milestone for no engineering reason, while leaving the genuinely dangerous case, computing metrics from today's biased tracks, less clearly forbidden |
| 2   | Added Section 7: the metric framework, ahead of the feature matrix                                                                          | The matrix said what could be measured and nothing said when it may be emitted                                                                                                                                 |
| 3   | Added the three-tier road-user scope: universal primitives, class-specific metrics, interaction metrics                                     | The plan was implicitly motor-vehicle-shaped                                                                                                                                                                   |
| 4   | Added declared per-metric applicability and a closed `SuppressionReason` vocabulary                                                         | A suppressed metric is now a first-class stored result rather than a silent absence                                                                                                                            |
| 5   | Added the observation-support taxonomy, separating `occluded_inferred` from `missed_unknown`                                                | Treating every miss as occlusion would hide defect P4 behind a plausible label                                                                                                                                 |
| 6   | Split `Measurement` into `BehaviourMeasurement` and `BehaviourOutcome`                                                                      | Forcing "rolling stop" through a `float64` is the opaque encoding this plan exists to avoid. Outcomes now reference the evidence they were derived from                                                        |
| 7   | Added interaction classification as a **posterior** with retained evidence, gating metrics on classification confidence                     | Applying longitudinal TTC to an ambiguous following-versus-merging pair produces a confident meaningless number                                                                                                |
| 8   | Generalised passing clearance to `min_separation`, with lateral clearance as a projection for confident overtakes                           | Lateral distance assumes a straight road and parallel paths; it breaks on curves, angled paths, lane changes and intersections                                                                                 |
| 9   | Replaced scalar `Sigma` with an `Uncertainty` struct supporting intervals, bounds and a propagation method                                  | TTC, PET and every extremum are asymmetric or heavy-tailed; a symmetric sigma misreports them                                                                                                                  |
| 10  | Added three uncertainty scopes, `track_local`, `shared_sensor`, `shared_geometry`                                                           | Pairwise metrics are not independent: sensor pose largely cancels in a separation and does not cancel in a PET                                                                                                 |
| 11  | Marked jerk **experimental**, with the raw-versus-band-limited distinction and a published-boundary acceptance test                         | Jerk was retained but not sufficiently fenced                                                                                                                                                                  |
| 12  | Added Section 12.1: engineering dependency order and product priority as separate orderings, with VRU interactions leading the product list | The two are close to inverted at the top, and implementation convenience was implying importance                                                                                                               |
| 13  | Added four version axes to `Provenance`: estimator, observation model, behaviour method, geometry                                           | A result can change for four independent reasons                                                                                                                                                               |
| 14  | Added Section 13A answering the ten questions this revision raised                                                                          |                                                                                                                                                                                                                |

Deliberately unchanged: the observation budget measurements, the fixed-roadside
versus mobile feasibility table, the benchmark taxonomy and its citation
hygiene, the opportunity-normalisation argument, the feature groups themselves,
the evaluation-dataset strategy and the open research questions. The revision
corrects framework and representation; it does not relitigate the research.

## 16. References

Primary and authoritative sources only. Where a value could not be confirmed
from a primary document, that is stated at the point of use.

**Surrogate safety and traffic conflicts**

- FHWA, _Surrogate Safety Assessment Model (SSAM)_, TechBrief FHWA-HRT-08-049.
  <https://www.fhwa.dot.gov/publications/research/safety/08049/>
- FHWA, _Surrogate Safety Assessment Model and Validation: Final Report_,
  FHWA-HRT-08-051.
  <https://www.fhwa.dot.gov/publications/research/safety/08051/06.cfm>
- FHWA, _SSAM: Software User Manual_, FHWA-HRT-08-050. **Not yet consulted
  directly**; the PET default of 5 s is cited from secondary literature pending
  confirmation.
- Zheng, L. and Sayed, T., _Comparison of threshold determination methods for
  the deceleration rate to avoid a crash (DRAC)-based crash estimation_,
  Accident Analysis & Prevention, 2021.
  <https://www.sciencedirect.com/science/article/abs/pii/S0001457521000828>

**Naturalistic driving**

- Dingus, T. A. et al., _The 100-Car Naturalistic Driving Study, Phase II:
  Results of the 100-Car Field Experiment_, NHTSA, DOT HS 810 593, April 2006.
  <https://www.nhtsa.gov/sites/nhtsa.gov/files/100carmain.pdf>
- NHTSA, _Analysis of SHRP2 Speeding Data_, DOT HS 812 858, March 2020.
  <https://rosap.ntl.bts.gov/view/dot/44242/dot_44242_DS1.pdf>
- _Using SHRP2 naturalistic driving data to examine driver speeding behavior_,
  Journal of Safety Research, 2020.
  <https://pubmed.ncbi.nlm.nih.gov/32563403/>

**Lane keeping**

- Verster, J. C. and Roth, T., _Standard operation procedures for conducting the
  on-the-road driving test, and measurement of the standard deviation of lateral
  position (SDLP)_, International Journal of General Medicine, 2011.
  <https://doi.org/10.2147/IJGM.S19639>
- Verster, J. C. and Roth, T., _Excursions out-of-lane versus standard deviation
  of lateral position as outcome measure of the on-the-road driving test_, Human
  Psychopharmacology, 2014. <https://doi.org/10.1002/hup.2406>

**Cyclist overtaking**

- _Naturalistic study of vehicle-bicycle lateral passing distance on high-speed
  rural two-lane roadways with paved shoulders_, Transportation Research Part F, 2024. <https://www.sciencedirect.com/science/article/abs/pii/S1369847824000512>
- _Driver compliance and safety effects of three-foot bicycle passing laws_,
  Transportation Research Interdisciplinary Perspectives, 2020.
  <https://www.sciencedirect.com/science/article/pii/S2590198220300841>
- _Modelling duration of car-bicycles overtaking manoeuvres on two-lane rural
  roads using naturalistic data_, Accident Analysis & Prevention, 2021.
  <https://www.sciencedirect.com/science/article/abs/pii/S0001457521003481>

**Trajectory datasets**

- Krajewski, R., Bock, J., Kloeker, L. and Eckstein, L., _The highD Dataset_,
  ITSC 2018. <https://arxiv.org/abs/1810.05642>
- Bock, J., Krajewski, R., Moers, T., Runde, S., Vater, L. and Eckstein, L.,
  _The inD Dataset_, 2019. <https://arxiv.org/abs/1911.07602>
- Krajewski, R., Moers, T., Bock, J., Vater, L. and Eckstein, L., _The rounD
  Dataset_, ITSC 2020. <https://doi.org/10.1109/ITSC45102.2020.9294728>
- _openDD: A Large-Scale Roundabout Drone Dataset_, ITSC 2020.
  <https://arxiv.org/html/2007.08463> and
  <https://doi.org/10.1109/ITSC45102.2020.9294301>

**Citation hygiene.** Every link above was returned by a literature search
during drafting and points at a real record; none has been read in full. Where a
DOI was not directly observed, the publisher URL is given instead of a
constructed DOI, deliberately. Before this plan justifies any production
threshold, each numeric value in Section 5.1 must be checked against the primary
document and the check recorded against that row. Section 5.1 is the list that
matters, and B8 is the first item on it.
