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

| Requirement                                        | Why                                                                                                                       |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Read `EstimatedState` at `stage = final`           | The online estimate is revised afterwards; a report built on it quotes a superseded number                                |
| Carry covariance and `estimator_id` on every state | Uncertainty must propagate into every derived metric, and every metric must be reproducible against its estimator version |
| Heading and yaw rate are states with covariance    | Lateral acceleration, curvature and conflict geometry are undefined without an orientation that has an uncertainty        |
| Object extents are a track-level belief with sigma | Clearance and bumper-to-bumper gap are measured between surfaces, so they inherit extent uncertainty directly             |
| Never read raw bounding-box centres                | The medoid measurement carries a viewpoint-dependent bias of up to half a vehicle width, measured at 0.676 m mean         |

## 3. The observation budget

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
| Speed relative to posted limit            | **high**       | high                | Both need the limit as data; see Section 9.1                                                                                |
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

## 7. Behaviour feature matrix

Column key. **Scope**: `single` uses one track, `pair` requires two. **Map**:
roadway context required, per [lidar-l7-scene-plan](lidar-l7-scene-plan.md).
**RS**: fixed-roadside suitability. **Bench**: benchmark kind from Section 5.
**Form**: `C` continuous, `E` event-based, `X` exposure-normalised.

### 7.1 Speed behaviour

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

### 7.2 Longitudinal control

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

**Jerk suppression is mandatory.** From
[lidar-state-estimation-plan Section 13.2](lidar-state-estimation-plan.md):
four-point differencing at 0.2 s spacing with a 0.05 m measurement gives about
28 m/s³ of noise against a 1 to 5 m/s³ signal. Every jerk figure therefore
carries its effective window, and the API refuses to return jerk when the
passage is shorter than the window requires, or when the track is poorly
observed. A jerk number without a bandwidth describes the filter, not the
vehicle.

### 7.3 Following behaviour

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

### 7.4 Time to collision

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

### 7.5 DRAC and required braking

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

### 7.6 Crossing conflicts and post-encroachment time

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

### 7.7 Lane and path keeping

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

### 7.8 Lane changes, merges and cut-ins

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
| `induced_response`                    | See Section 7.12                                     | n/a   | pair   | no      | medium | n/a                     | E    |

**A cut-in that leaves a following vehicle with a 0.6 s THW and a 1.1 s TTC is a
measured interaction.** An "aggressive lane change" classifier is a guess. Build
the former.

**Completeness gate.** Compute manoeuvre metrics only when the whole manoeuvre
is inside the field of view, with a stated margin before and after. A partially
observed lane change yields a truncated duration and an understated
displacement, and must be marked incomplete rather than reported.

### 7.9 Cyclist and vulnerable-road-user overtaking

This is the sensor's strongest case and should be treated as a headline
capability rather than a subsection of lane changes. An overtaking driver's own
sensors see the clearance poorly; a cyclist's rear camera sees one side and no
absolute geometry. A roadside LiDAR sees both parties and the road.

| Feature                   | Definition                                  | Units | Scope | Map | RS   | Bench                             | Form |
| ------------------------- | ------------------------------------------- | ----- | ----- | --- | ---- | --------------------------------- | ---- |
| `passing_clearance_min`   | Minimum surface-to-surface lateral distance | m     | pair  | no  | high | `legal` + `external_distribution` | E    |
| `passing_speed`           | Motor vehicle speed at closest approach     | m/s   | pair  | no  | high | `local_distribution`              | E    |
| `cyclist_speed`           | Cyclist speed at closest approach           | m/s   | pair  | no  | high | n/a                               | E    |
| `relative_speed_pass`     | Difference at closest approach              | m/s   | pair  | no  | high | `local_distribution`              | E    |
| `duration_alongside`      | Time with longitudinal overlap              | s     | pair  | no  | high | n/a                               | E    |
| `longitudinal_separation` | Signed along-path offset over the manoeuvre | m     | pair  | no  | high | n/a                               | C    |
| `close_pass_rate`         | `close_pass_count / cyclist_overtake_count` | ratio | pair  | no  | high | `legal` + `X`                     | X    |

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

### 7.10 Yielding behaviour

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

### 7.11 Stop behaviour

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

### 7.12 Induced evasive response

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

### 7.13 Interaction geometry

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

## 8. Uncertainty propagation and suppression

Every metric here is a function of estimated quantities, so every metric has an
uncertainty. The rule from the estimation plan applies throughout: **a metric is
unavailable rather than falsely precise when observability is inadequate.**

Uncertainties are taken from the `final` state covariance per track, not from
constants. The illustrative numbers below use a filtered speed sigma of
0.15 m/s and a position sigma of 0.05 m, which are the estimation plan's Phase 2
targets; the real values come from the covariance at evaluation time.

### 8.1 Derived suppression rules

| Metric                | Propagation                                                            | Suppression rule                                                                                                                                                                                                                                                                                                                                                                                      |
| --------------------- | ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ttc`                 | `σ_TTC/TTC = sqrt((σ_gap/gap)² + (σ_Δv/Δv)²)`                          | **The dominant failure mode.** As closing speed approaches zero, TTC diverges and its uncertainty diverges faster. With `σ_Δv = √2 · σ_v ≈ 0.21 m/s`, a closing speed of 0.5 m/s gives a 42 % relative error. Suppress when `Δv < 3 σ_Δv`, about 0.64 m/s at the illustrative sigma. Report a confidence interval, never a bare value                                                                 |
| `thw`                 | `σ_THW/THW = sqrt((σ_gap/gap)² + (σ_v/v)²)`                            | Suppress when `speed_F` is below the stop epsilon; THW is undefined at rest, not infinite                                                                                                                                                                                                                                                                                                             |
| `gap`                 | `σ_gap² = σ_sL² + σ_sF² + (σ_LL/2)² + (σ_LF/2)²`                       | Inherits both vehicles' **length** uncertainty. Suppress when either length belief has fewer observations than the estimation plan's dimension-convergence minimum                                                                                                                                                                                                                                    |
| `passing_clearance`   | `σ_c² = σ_lat² + (σ_Wv/2)² + (σ_Wc/2)²`                                | **Dominated by extent, not position.** A cyclist is narrow and sparsely sampled: a per-track width sigma of 0.3 m contributes 0.15 m by itself, against a legal discrimination need of about 0.1 m. Therefore **cyclist and pedestrian widths must come from a class prior with a tight sigma, not from a per-track estimate**. This is the single most important uncertainty conclusion in this plan |
| `pet`                 | `σ_PET² = (σ_sA/v_A)² + (σ_sB/v_B)² + (σ_boundary/v)²`                 | Robust: at 0.05 m and 10 m/s the crossing-time sigma is 5 ms, and at pedestrian speeds about 50 ms. The conflict-region boundary definition, not the trajectory, is the larger term. Suppress only when either party's crossing is not fully in view                                                                                                                                                  |
| `accel_long`          | From the state covariance directly, once acceleration is a state       | Suppress below the estimation plan's minimum temporal support, about 1 s                                                                                                                                                                                                                                                                                                                              |
| `jerk_*`              | Amplified by `1/dt³` if differenced; from the smoothed state otherwise | Suppress when the passage is shorter than the declared window. Always emit the bandwidth                                                                                                                                                                                                                                                                                                              |
| `lateral_accel`       | `σ² = (ω σ_v)² + (v σ_ω)²`                                             | The `v σ_ω` term dominates because yaw rate is weakly observable at 5 Hz. Suppress when the yaw-rate variance exceeds a stated bound                                                                                                                                                                                                                                                                  |
| `stop_position`       | `σ² = σ_s² + σ_pose² + σ_stopline²`                                    | **Not computable today**: the pipeline runs in sensor coordinates with a nil pose, so `σ_pose` is undefined. Blocked on the site frame                                                                                                                                                                                                                                                                |
| `lateral_offset_lane` | `σ² = σ_lat² + σ_pose² + σ_lane_geometry²`                             | Same block. The map's own accuracy is a term, and a hand-drawn lane centreline can easily dominate the trajectory error                                                                                                                                                                                                                                                                               |
| Any pairwise metric   | Both tracks contribute                                                 | Suppress when either track is below the quality floor, or when either was coasting rather than observed at the relevant instant                                                                                                                                                                                                                                                                       |

### 8.2 Coasting is not observation

The estimation plan persists a `final` state for frames where the tracker was
coasting through an occlusion. Those states are legitimate trajectory estimates
and they are **not** observations. A pairwise metric evaluated at an instant
when either party was coasting must be marked as such, and an exposure
denominator must exclude coasted time. Otherwise a vehicle hidden behind a van
for six frames silently contributes six frames of fictitious following
exposure.

## 9. Data model

### 9.1 Roadway context does not exist yet, including the speed limit

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

### 9.2 Proposed types

Adapted to repository conventions rather than copied: track identifiers are
**strings** (`trk_<uuid>`, per `TrackedObject.TrackID`), not integers;
timestamps are `TSUnixNanos int64`; analytics uses `float64`; JSON tags are
snake case.

```go
package l8behaviour // new, under internal/lidar/

// BenchmarkKind records what authority, if any, a comparison carries.
type BenchmarkKind uint8

const (
    BenchmarkNone BenchmarkKind = iota
    BenchmarkLegal                // a jurisdictional rule
    BenchmarkResearchThreshold    // a published methodology's threshold
    BenchmarkExternalDistribution // a published population
    BenchmarkLocalDistribution    // our own stratified observations
    BenchmarkNoEstablished        // reported as a value only
)

// Benchmark identifies the specific authority behind a comparison, so that a
// number in a report can be traced to the statute or paper that justifies it.
type Benchmark struct {
    Kind         BenchmarkKind `json:"kind"`
    ID           string        `json:"id"`                     // e.g. "ssam.ttc.1.5s", "site.42.speed_limit"
    Jurisdiction string        `json:"jurisdiction,omitempty"` // legal only
    Citation     string        `json:"citation,omitempty"`     // research only
    EffectiveAt  int64         `json:"effective_at,omitempty"` // legal only
    Population   string        `json:"population,omitempty"`   // distribution only: the stratification used
}

// Measurement is one value with everything needed to judge it. Uncertainty and
// provenance are mandatory, not optional decoration.
type Measurement struct {
    Name        string  `json:"name"`
    Value       float64 `json:"value"`
    Unit        string  `json:"unit"`
    Sigma       float64 `json:"sigma"`
    Suppressed  bool    `json:"suppressed"`
    SuppressWhy string  `json:"suppress_why,omitempty"`

    Benchmark  *Benchmark `json:"benchmark,omitempty"`
    Percentile *float64   `json:"percentile,omitempty"` // against Benchmark.Population

    // Exposure denominator, for rate-form metrics. Zero means not applicable.
    OpportunitySeconds float64 `json:"opportunity_seconds,omitempty"`

    // Provenance back to the estimator that produced the trajectory.
    EstimateStage string `json:"estimate_stage"` // must be "final"
    EstimatorID   string `json:"estimator_id"`
    ParamHash     string `json:"param_hash"`
}

// PassageSummary is the per-track behavioural record: one road user, one
// traverse of one site. Deliberately not called a driver profile.
type PassageSummary struct {
    TrackID     string `json:"track_id"`
    SiteID      int64  `json:"site_id"`
    SensorID    string `json:"sensor_id"`
    ClassLabel  string `json:"class_label"`

    StartUnixNanos int64 `json:"start_unix_nanos"`
    EndUnixNanos   int64 `json:"end_unix_nanos"`

    ObservedPathMetres float64 `json:"observed_path_metres"`
    ObservedSeconds    float64 `json:"observed_seconds"`
    CoastedSeconds     float64 `json:"coasted_seconds"`

    Metrics []Measurement `json:"metrics"`
}

// InteractionType is classified before any metric is computed, because it
// determines which formulae are valid at all.
type InteractionType string

const (
    InteractionFollowing InteractionType = "following"
    InteractionCrossing  InteractionType = "crossing"
    InteractionMerging   InteractionType = "merging"
    InteractionOvertake  InteractionType = "overtake"
    InteractionOpposing  InteractionType = "opposing"
)

// InteractionEvent is a pairwise encounter. Primary and Secondary name roles in
// the geometry, not fault.
type InteractionEvent struct {
    EventID           string          `json:"event_id"`
    PrimaryTrackID    string          `json:"primary_track_id"`
    SecondaryTrackID  string          `json:"secondary_track_id"`
    Type              InteractionType `json:"type"`

    StartUnixNanos int64 `json:"start_unix_nanos"`
    EndUnixNanos   int64 `json:"end_unix_nanos"`

    // Conflict geometry. ConflictFromMap distinguishes a named, persistent
    // region from one derived by intersecting two observed paths.
    ConflictX, ConflictY *float64 `json:"conflict_x,omitempty"`
    ConflictAngleRad     *float64 `json:"conflict_angle_rad,omitempty"`
    ConflictFromMap      bool     `json:"conflict_from_map"`

    // Metrics carry their own uncertainty and suppression state.
    Metrics []Measurement `json:"metrics"`

    // How well both parties were observed throughout.
    Confidence float64 `json:"confidence"`

    // True when either party was coasting during the measured interval.
    InvolvedCoasting bool `json:"involved_coasting"`
}

// ExposureWindow is an opportunity denominator, stored explicitly so a rate can
// be audited and recomputed.
type ExposureWindow struct {
    TrackID   string  `json:"track_id"`
    Kind      string  `json:"kind"` // "free_flow", "valid_following", "yield_opportunity", "overtake"
    Seconds   float64 `json:"seconds"`
    StartUnixNanos int64 `json:"start_unix_nanos"`
    EndUnixNanos   int64 `json:"end_unix_nanos"`
}
```

Three departures from the sketch in the brief, each deliberate.

**Uncertainty is not an optional float.** `Measurement` carries `Sigma`,
`Suppressed` and `SuppressWhy` because suppression is the common case for half
these metrics and an absent value must explain itself.

**Benchmark is a struct, not an ID string.** A legal benchmark needs a
jurisdiction and an effective date; a research threshold needs a citation; a
distribution needs its stratification. Flattening them to one string loses
exactly the provenance this plan exists to preserve.

**Interaction type is classified first.** It is a field on the event, and it
gates which metrics are computed, rather than every metric being attempted on
every pair.

### 9.3 Persistence

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

## 10. Evaluation datasets

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

## 11. Roadmap

Every phase is gated on **G-SMO-1** from the estimation plan. Nothing here can
start before a `final` estimate exists.

### Phase 6A: single-track kinematics

**Goal.** Passage-level speed, acceleration, braking, curvature, and stop
behaviour that needs no map.

**Inputs.** `final` estimates with covariance. No roadway context.

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
in `site_config_periods`, per Section 9.1.

**Acceptance.** Every `legal` benchmark carries its jurisdiction and effective
date. Lane-relative metrics propagate map uncertainty and suppress when the map
term dominates the trajectory term.

**Suppression conditions.** No active site pose; no geometry for the region
traversed; map accuracy unstated.

### Phase 7/8: vulnerable-road-user and conflict analytics

**Goal.** Cyclist passing clearance, yielding, PET against named conflict
regions, conflict angle, and the induced-response evidence surface.

**Inputs.** Phase 6B, Phase 7 geometry, class priors for VRU extents per
Section 8.1, and the estimation plan's Phase 8 abnormal-motion evidence.

**Acceptance.** Passing clearance sigma below 0.1 m for the class-prior width
model, verified against synthetic scenes with known geometry. Yield outcomes
distinguished on labelled real encounters. `InducedResponse` emits association
and magnitude, never causation.

**Suppression conditions.** VRU class uncertain; either party partially
observed at closest approach; clearance sigma above the legal discrimination
need.

## 12. Open research questions

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

## 13. References

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
