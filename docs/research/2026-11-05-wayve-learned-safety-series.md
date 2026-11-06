# Wayve's learned-safety series: how it uses metrics, and what velocity.report takes from it (2026-11-05)

This note reads the three posts Wayve published on November 4, 2026 as one argument about how a
safety-critical measurement system should be validated, lists every metric and quantitative claim
they make, and maps each practice to the repository: what already holds, what is adopted as a
proposal, what becomes an experiment, and what is refused with its reason. The posts are about an
end-to-end driving model; the project measures traffic from a fixed roadside sensor, so the
transfer is of validation method and vocabulary, not of any number.

- **Status:** Research assessment only. No metric, threshold, benchmark kind or default changes.
  One experiment is proposed as a spec ([figure0003](../../data/experiments/figure/figure0003.md))
  and one backlog item follows; everything else is a proposal for the owner's decision.
- **References:** `Fabris2026`, `Basso2026`, `Arani2026` (all new);
  [canonical bibliography](../../data/maths/references.bib).
- **Canonical:** [behaviour analytics plan](../plans/lidar-behaviour-analytics-plan.md),
  [metrics registry](../platform/architecture/metrics-registry.md),
  [state-estimation plan](../plans/lidar-state-estimation-plan.md),
  [experiment review of November 5](../lidar/operations/experiment-review-2026-11-05.md) and
  [backlog](../BACKLOG.md).

## Answer

The series states almost no numbers, and the one it does state is a bound, not a point: pooling
evidence from earlier releases and customer fleets through a Bayesian power prior "reduced the 95%
upper confidence bound on the estimated event rate by a factor of three and reduced the required
validation data by more than 20-fold". The figure comes from an internal analysis with no data,
denominator or event definition published, so it is a claim about method, not a result anyone can
check. Everything else is argument: validate the behaviour of the whole system rather than its
components; grade behaviour with one learned critic rather than a web of hand-written metrics;
transfer evidence across releases, platforms and sites by statistical similarity; make the model's
own safety judgement observable and compare it with what the vehicle did; and never mistake a
fluent explanation for evidence that the action was safe.

Three of those five already describe this repository's method, and the November 5 review arrived at
the first independently: the `figure` area tests what a survey prints rather than what a tracker
scores. Two are refused. A learned critic contradicts Tenets 3 and 5 and the benchmark taxonomy,
under which every comparison names its authority; and pooling a prior period into a published
figure presumes the stationarity a before-and-after survey exists to test. The series' premise that
"similar models will exhibit consistent event rates" becomes, for us, a question the archived rows
can answer: do two segments of the same site give the same p85, within its interval? That is
[figure0003](../../data/experiments/figure/figure0003.md). The one practice adopted as a proposal
is the bound itself: the headway distribution's pooled band rate is the one public value that
declares no uncertainty, and Wayve's habit of printing an upper confidence bound on a rare rate is
the right shape for it.

## Evidence

All three posts were fetched and read in full on November 5, 2026 (about 2,600 words each,
excluding navigation). They are engineering blog posts, not papers: no data, methods section,
sample size or reproducible figure accompanies any claim. Quotations below are short and
attributed; the pages are copyright Wayve Technologies Ltd.

### Part 1: Fabris, "Building a Safe AI Driver" (November 4, 2026) <!-- spelling-ignore -->

Simone Fabris (Vice President, Product and Delivery) argues that an end-to-end driving model can
carry "real, auditable structure". One shared scene understanding feeds three learned tasks that
are separated "in their objectives, evaluation, and deployment responsibilities, not in separate
neural networks": a driving task ("how to move"), a risk-anticipation task ("what is safe to do
right now") and a self-awareness task that "recognizes when [the shared understanding] can no
longer be trusted" and triggers the fallback appropriate to the automation level (L2+, L3, L4).
<!-- spelling-ignore -->

The separation is justified by two named safety principles, functional independence (with
"freedom from interference" on the target system-on-chip) and "complementary, diverse redundancy",
the learned form of a doer-checker pair. The risk-anticipation output is made visible as the
Learned Safety Envelope, "a live visualization of risk around the vehicle", learned from data <!-- spelling-ignore -->
"rather than built from hand-labeled risks or a fixed set of thresholds", so that engineers "can <!-- spelling-ignore -->
inspect what the driving model considered safe and compare that judgment with the resulting
driving behavior". <!-- spelling-ignore -->

Quantitative content: none, beyond "a few frames" (how early the envelope reacts to a cyclist's
head seen through parked cars' windows) and "millions of miles" (over which rare events "may barely
appear"). The one effectiveness claim is that the envelope "mitigates the vast majority of on-road
safety-critical interventions", with no count, rate or baseline. The post names ISO 26262,
ISO 21448 (SOTIF), ISO/PAS 8800 and UL 4600, says the standards "do not fully prescribe how to
test an end-to-end driving model", and adds "large-scale behavioral testing, structured scenario
coverage, uncertainty testing, and continuous monitoring". <!-- spelling-ignore -->
Validation evidence is three kinds: closed-loop simulation ("controlled breadth"), targeted
on-road testing ("depth") and classical software validation. The explicit limit is stated: the
envelope "does not, by itself, prove that the complete driving system is safe".

### Part 2: Basso, "Validating the AI Driver" (November 4, 2026)

Brandon Basso (VP Engineering, Simulation, Evaluation and Validation) names the method
"Validation 2.0" and gives it three pillars.

1. **Closed-loop simulation** with GAIA, Wayve's generative simulator, which "initializes <!-- spelling-ignore -->
   interactive scenarios directly from unstructured real-world video, such as dashcam footage",
   and which active learning steers toward the scenarios where the simulation itself needs
   improving, "to reduce the sim-to-real gap as an end-to-end problem".
2. **A learned critic,** GRADER (Generalizable Reward for Autonomous Driving Evaluation and RL), <!-- spelling-ignore -->
   "an offline component independent of the AV stack" that "maintains high recall and generalizes <!-- spelling-ignore -->
   across ODDs, vehicle types, and geographies". It is offered against the alternative the post
   calls a "heuristic analytics stack that can be more complex and harder to maintain than the
   driving stack it judges, and it only improves when people extend it". Its generalisation is
   shown by example only: it marks "remaining stopped before turning right on a red light" high in
   Germany and low in the United States without being given either rule, and it recognises an
   expected stop for a worker directing traffic or a temporary signal in four countries, two of
   them from third-party dashcam data. No recall figure is given.
3. **Statistical evidence transfer:** aggregating "metrics across software releases, platforms,
   and testing modalities such as simulation, on-road, rig transfer, dashcam, and customer fleet
   data", weighting past data "based on statistical similarity" with a Bayesian power prior. This
   is where the series' one number lives: "In an internal analysis, leveraging customer fleet data
   reduced the 95% upper confidence bound on the estimated event rate by a factor of three and
   reduced the required validation data by more than 20-fold." The event, its denominator, the
   prior's weighting and the sample are not given. The caveat follows: "cumulative data is still
   needed for strict targets".

The argument that binds them is the section "The Modular Myth": "strong subsystem-level
performance and traceability does not guarantee system-level safety", because a mapping or
perception module can pass its tests and still "feed confidently incorrect data to the planner".
The remedy is "behavior-first" validation of "the complete system". The post's diagnosis of the <!-- spelling-ignore -->
alternative is worth keeping verbatim: standard practice "rests on large sets of formal
requirements, translated into large sets of tests, graded against test-specific metrics, with
results aggregated by heuristics that transfer poorly", and "the resulting spider-web of
requirements, tests, metrics, and acceptance criteria often exceeds the complexity of the driving
stack under test". The target of validation is named as "the absence of unreasonable risk (AUR)".
No standard is cited by number here, no scenario taxonomy or coverage figure is given, and shadow
mode, human baselines and open-loop evaluation are not discussed.

### Part 3: Arani and Dagan, "Language Should Ride Shotgun, Not Stall the Control Loop" (November 4, 2026)

Elahe Arani (Director of AI Research) and Erez Dagan (President) separate three claims that
"are often collapsed into one": "a model can produce an explanation, the explanation is faithful
to the computation that produced an action, and the action is safe". Each "requires different
evidence". The operative slogans are "Fluency is not causality", "Interpretability is not
validation" and "Measure the vehicle, not the narrative". The test they propose for a rationale is
"controlled changes to the cited scene evidence", because "agreement between a stated reason and a
trajectory is not enough". The properties they value in a component are the measurable ones:
"accuracy under relevant conditions, calibrated uncertainty, timing guarantees, closed-loop
robustness, and known failure envelopes".

The architectural content is an "operating contract" for the risk-anticipation capability: "a high
update rate, physically feasible outputs, bounded execution time, feedback from the vehicle and
environment, and safe behavior when deliberative services are unavailable", with "graceful <!-- spelling-ignore -->
degradation" as "the decisive property". Language's six legitimate uses are listed (curriculum,
representations, constrained intent, test-space specification, debugging, human interaction), and
the rule is stated narrowly: "acute stabilization and collision avoidance must not depend on an <!-- spelling-ignore -->
unbounded reasoning process". The post names LINGO-1 (2023) and LINGO-2 (2024) and gives no
metric, benchmark, dataset figure or threshold. Section 5 lists the pipeline a claim must pass:
"representative data, intervention learning, physical modeling, closed-loop simulation, vehicle <!-- spelling-ignore -->
integration, fault injection, verification and validation, and independent assurance".

### What the series measures, and what it does not

| Quantity the series names                                   | Given as                       | Status for this note                                                 |
| ----------------------------------------------------------- | ------------------------------ | -------------------------------------------------------------------- |
| 95 % upper confidence bound on an event rate                | "reduced by a factor of three" | Stated without data, event definition, denominator or prior          |
| Validation data required for a release                      | "reduced by more than 20-fold" | Stated without data; the target it was measured against is not given |
| Share of on-road safety-critical interventions mitigated    | "the vast majority"            | No count, rate or baseline                                           |
| GRADER recall                                               | "high recall"                  | No figure, dataset or label source                                   |
| Frames before the envelope reacts to a part-visible cyclist | "a few frames"                 | Illustration from one clip                                           |
| Rare-event exposure                                         | "millions of miles"            | Order of magnitude only                                              |

Absent entirely: miles per intervention or disengagement, collision rates, any human-driver
baseline figure, scenario counts or coverage, confidence levels other than the one 95 %, sample
sizes, TTC or hard-braking definitions, and any threshold. The series' own vocabulary for the
quantities it does not publish is "explicit release criteria", "residual risk" and "strict
targets". For the repository's purposes the series is a statement of method with one anecdotal
magnitude, and it is read as such: nothing in it can be re-derived, and no figure from it enters
any plan as a number.

## How Wayve uses metrics

Five patterns, each stated in the series' own terms before it is mapped.

1. **Behaviour before components.** The unit of evidence is "the complete system operating in
   closed loop", and component scores are "development signals" only. The failure the posts warn
   against is a subsystem that passes its tests and misleads the whole.
2. **One critic, not a metric web.** Hand-written, per-market metric stacks grow until they exceed
   the system under test and "only improve when people extend it"; a learned grader is proposed in
   their place, and its worth is argued from generalisation across geographies.
3. **Evidence is transferred, weighted by similarity.** A new release, platform or domain does not
   start its evidence from zero; prior evidence enters through a power prior, and the gain is
   reported as a tighter one-sided bound on a rate and as data not needed.
4. **Judgement is made observable and compared with behaviour.** The risk-anticipation output is
   visualised so that what the model "considered safe" can be set against what the vehicle did;
   the self-awareness task says when the shared understanding "can no longer be trusted".
5. **Explanation is not evidence.** A rationale's agreement with the action is "not enough"; the
   claim that an action was safe needs closed-loop, target-hardware, failure-handling evidence,
   and the narrative is tested by perturbing the evidence it cites.

Two further habits are smaller but telling: the one number is a bound at a stated coverage, not a
point estimate; and the rate's economics (how much data a target needs) is itself treated as a
reportable quantity.

## Where each practice lands in velocity.report

The sensor observes passages at one place, not a vehicle's own decisions, so the mapping is between
Wayve's system under test (a driving policy) and ours (a measurement pipeline whose published
outputs are a vehicle count, speed percentiles over vehicle maxima, and the provisional following
distribution). The traffic itself is the quantity measured, not the thing validated; that
distinction decides several verdicts below.

| Wayve practice                                                      | Where it lands here                                                                                                                                                                                                                                                                                                                                                                                     | Verdict                                                                                                                                                                                                                                                                                                                           |
| ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Behaviour-first validation ("The Modular Myth")                     | The [November 5 review](../lidar/operations/experiment-review-2026-11-05.md#e-confidence-in-the-outputs) found eight arms compared on HOTA, IDF1, duplicates and retention and none on what a survey prints, and opened the [`figure` area](../../data/experiments/figure/README.md) with [figure0001](../../data/experiments/figure/figure0001.md) to read count, p85 and p98 per site under every arm | **Already holds** since November 5. Proposal P1 below asks that every rule which can change a default carry a figure clause beside its component clauses                                                                                                                                                                          |
| A learned critic in place of hand-written metrics                   | Tenet 3 (statistical claims carry their method), Tenet 5, the [benchmark taxonomy](../plans/lidar-behaviour-analytics-plan.md#5-benchmark-taxonomy) (every comparison names its authority) and the classifier plan's explainability constraint                                                                                                                                                          | **Not adopted.** A grader that infers the rule of the road from data is exactly what the `legal` benchmark's jurisdiction field exists to make explicit. The useful residue is the generalisation test: a rule written on kirk0 scored on a site it was not written on, which the review's held-out ledger (F21) already proposes |
| Evidence transfer across releases                                   | Replay determinism (l3bg0020 to l3bg0023) makes a build's evidence repeatable; the `a0` anchor drift (F22) shows evidence is not yet carried between builds; a scene republish replays every site for most of a day                                                                                                                                                                                     | **Open question Q1.** Which sites' figures must be re-replayed after a default change is figure0001's reading, once it reports. No pooling method is adopted                                                                                                                                                                      |
| Evidence transfer across periods or sites for a published figure    | A survey is a period ([vocabulary plan](../plans/platform-vocabulary-and-data-model-plan.md)); the `local_distribution` benchmark is this site's own observations with the stratification stated; before-and-after reports are a product goal                                                                                                                                                           | **Not adopted for published figures.** A prior period weighted into the present one's p85 presumes the stationarity the comparison exists to test, and would bias every before-and-after toward "no change". The premise is tested instead: [figure0003](../../data/experiments/figure/figure0003.md)                             |
| An upper confidence bound on a rare event rate                      | The headway distribution's pooled band rate declares no uncertainty, by design: "the per-encounter intervals do not combine into one" ([distribution.go](../../internal/lidar/l8behaviour/distribution.go)); the scene API serves it with `uncertainty none`; the plan's Section 9.3 already provides `interval` and `bounds` representations                                                           | **Adopted as a proposal** (P2 and the backlog item): resample encounters for an interval with coverage, method and sample count; for a band with few encounters, an upper bound only                                                                                                                                              |
| Data required to reach a target as a reportable quantity            | Open question B6 in the behaviour plan (do local distributions stabilise at achievable sample sizes per stratum?) has no measurement                                                                                                                                                                                                                                                                    | **Becomes a reading** of figure0003: interval half-width against the number of moving tracks, per site                                                                                                                                                                                                                            |
| Judgement made observable and compared with behaviour (LSE)         | Suppression reasons are first-class results ([Section 7.2](../plans/lidar-behaviour-analytics-plan.md#72-applicability-and-suppression)), the explainability record (7.6) names which states, geometry, thresholds and uncertainties produced each value, review-only metrics (the predicted gap) are shown apart from published ones, and the headway report prints reasons with counts                | **Already holds** in design and, for following, in the built report. The faithful explanation Wayve has to test for is ours by construction: the record is the computation's inputs, not a narrative about them                                                                                                                   |
| Self-awareness and graceful degradation                             | Support states (`observed`, `coasted`, `occluded_inferred`, `missed_unknown`), `model_degraded`, `insufficient_observation`, `estimate_not_final`; "a suppressed metric is preferable to false precision"                                                                                                                                                                                               | **Already holds.** The operating contract Wayve describes (what the output is when the capability cannot be trusted) is the metric contract's suppression vocabulary                                                                                                                                                              |
| Functional independence and freedom from interference               | Layer packages never import the composition root; L8 reads L5's `final` state; a value's suppression is decided apart from its magnitude                                                                                                                                                                                                                                                                | **Already holds** (architecture)                                                                                                                                                                                                                                                                                                  |
| Complementary, diverse redundancy                                   | The review's E17: the product's own OPS243 radar beside the LiDAR for one placement at a straight corpus site, the only path to a speed bound that is not the LiDAR checking itself                                                                                                                                                                                                                     | **Already proposed** (an owner decision in the review). Wayve's framing is the argument for it: a second physics measuring the same quantity, not a second reading of the same returns                                                                                                                                            |
| Explanation is not evidence; measure the vehicle, not the narrative | AGENTS.md requires reports to distinguish observed measurements from physical validation; the review labels every item "Measured:" or "Inferred:" and closes with "What this review does not establish"; figure0002 is the first speed read against a reference that is not the pipeline's                                                                                                              | **Already practised.** Proposal P3 makes the Measured/Inferred labelling the standard for every review and report an agent writes                                                                                                                                                                                                 |
| Closed-loop simulation initialised from real recordings             | The state-estimation plan's [synthetic generation](../plans/lidar-state-estimation-plan.md#163-synthetic-generation) (Section 16.3) is designed and not built; `gen-vrlog` writes a wholly synthetic scene; the settled background snapshots a VRLOG carries could seed a real scene                                                                                                                    | **Candidate, not a spec.** Known-speed vehicles injected into a real settled background would have caught figure0002's running-mean defect without a physical reference; it waits on the synthscene package (Q4)                                                                                                                  |
| Safety vocabulary and product-safety standards                      | The wording guard in [wording.go](../../internal/report/headway/wording.go) rejects `risk`, `score`, `unsafe`, `danger` and `driver` in anything a headway report prints, by Section 1's rule: observables, not verdicts                                                                                                                                                                                | **Not adopted.** Wayve's terms describe an ego vehicle's own judgement; ours describe other people's passages. ISO 26262, SOTIF, ISO/PAS 8800 and UL 4600 govern a product's safety case, not a roadside measurement; the nearest standards for us are speed-survey methods                                                       |
| "Superhuman", and comparison with a careful human driver            | Every comparison here is population-relative, against a named population (`local_distribution`, `external_distribution`), or against a cited statute or methodology                                                                                                                                                                                                                                     | **Not applicable.** No human baseline is claimed or needed                                                                                                                                                                                                                                                                        |

## Proposed experiments

### figure0003: the interval on a survey's figures, and whether a second segment of the same site falls inside it

Written as a spec: [figure0003](../../data/experiments/figure/figure0003.md), in the `figure`
area, depending on figure0001 for its moving-track and maximum-speed definitions and its archived
rows. It reads no capture. The question in one line: at the number of moving tracks a five-minute
segment holds, how wide are p85 and p98, and does the third segment of the same site fall inside
the first segment's interval?

Its three readings, each a measurement and none a default:

1. Per site and arm, a seeded bootstrap over tracks gives the 5th to 95th percentile of the
   nearest-rank p85 and p98 (the rule of
   [D-18](../DECISIONS.md#d-18--speed-percentile-aggregation-semantics)), beside the exact
   order-statistic interval, so the two methods check each other; and the
   half-width against the segment's track count, which is B6's missing measurement.
2. For the 22 sites with two archived segments, whether the third segment's p85 and p98 lie inside
   the first's interval, and the share of sites where they do not. A large share refutes, on the
   project's own data, the premise that a prior period is a prior for the present one.
3. The pooled two-segment figure and its interval against each segment's own: how much narrower
   pooling makes the interval, and how far it moves the figure from either segment.

The count carries no sampling interval: its uncertainty is systematic (one track in four shares
its vehicle under the defaults, per l5id0012), and the spec says so rather than resampling tracks
to manufacture one.

### Candidate, not yet a spec: known-speed vehicles in a real settled background

Wayve's simulation starts from real footage so that the scene is real and the event is controlled.
The project's equivalent is a VRLOG's settled background snapshots as the scene, with synthetic
vehicles of known position, speed and extent rendered through the parametric sensor model of
Section 16.3 and tracked by the production pipeline. The read is the published count, p85 and p98
against the injected truth. It validates the chain from returns to percentile, not the sensor's
physics, and so it does not replace E17's radar placement; but it is the one check that would have
found figure0002's defect (every observation row's speed held the running mean since birth) with
no physical reference at all. It needs the `synthscene` package first (an implement step of a day
or more), which is why it is logged as Q4 and not written as a spec.

### Not proposed

An experiment on a learned grader of tracks, and any pooling of one site's or period's figure into
another's published figure. Both fail the taxonomy before they could be run.

## Proposed language and metric updates

Each is a proposal; none is applied by this note.

- **P1. A figure clause in every rule that can change a default.** The run-experiments skill asks
  for a rule written before the replays; this adds to it one clause stating what the candidate
  does to the published count, p85 and p98 at the sites it ran, read with figure0001's script,
  beside its component clauses. The clause reports; it need not gate. It is Wayve's "measure the
  vehicle, not the narrative" applied to a tracker change: HOTA is the narrative, the survey's
  figure is the vehicle.
- **P2. No public rate without an interval.** The registry rule would read: a published value
  states its uncertainty as `sigma`, `interval` or `bounds`; `none` is permitted on review
  surfaces only. The pooled band rate is today's one exception, and the honest form for it is an
  interval from resampling encounters (the independent unit, as the
  [approach-stability note](2026-10-29-approach-stability-and-ttc.md) argues) with its coverage,
  method and sample count, and for a band with few encounters an upper bound alone. This is the
  backlog item below.
- **P3. Measured and Inferred, labelled.** The November 5 review wrote "Measured:" and
  "Inferred:" before each finding. Make it the output format of the experiment-review skill and
  the recommended form for every report's findings, so a reader can tell a rescored figure from a
  reading of the code. It is the repository's answer to "fluency is not causality": a report
  written by an agent is a narrative until its figures were run.
- **Terms borrowed, for internal use only:** "evidence transfer", as the name of Q1 (what
  validation evidence a new build or a new site inherits); "behaviour-level" and
  "component-level" for the two kinds of test the `figure` and `l5id` areas run.
- **Terms not borrowed:** "risk", "safety envelope", "red zone", "safe" and "unsafe", "margin",
  "safety-critical intervention", "residual risk", "absence of unreasonable risk", "superhuman".
  The first five are verdict words under the wording guard; "margin" would be a synonym for the
  registered gap, clearance and separation quantities, which the registry forbids; the last three
  are product-safety-case terms whose measurement analogue the 05x rubric already has as
  "qualified within a proven population and operating envelope".
- **No change to the registry's rows,** no new metric, and no rename.

## What this note does not establish

It does not establish that any Wayve claim is true, including the threefold bound and the 20-fold
data reduction, which are stated in the posts without data. It does not establish that the
project's figures are stable between segments or how wide their intervals are; figure0003 is the
measurement. It does not establish that intervals on band rates are computable from the current
encounter records without a design decision on the independent unit (Q3). It reads blog posts, not
papers: the LINGO and GAIA papers the posts rest on were not read, and nothing here evaluates
their methods.

## Open questions

- **Q1. Evidence transfer across builds.** After a default changes, which sites' published
  figures must be re-replayed, and which inherit the previous build's evidence? Answered in part
  by figure0001's reading of which arms move which figures at which sites; the republish's cost
  (most of a day) is the reason to ask.
- **Q2. Is a prior period a prior for the present one?** figure0003's second reading. If the third
  segment's p85 falls outside the first's interval at many sites, no pooled or borrowed figure is
  defensible, and the note's refusal stands on data rather than principle.
- **Q3. The independent unit of a band rate's interval.** Encounters are correlated within a
  session; the approach-stability note argues for resampling sessions with passages nested.
  Which unit the first implementation resamples, and what coverage it states, is a design decision
  for Euler's review before the backlog item is built.
- **Q4. What the synthscene package needs** to render known-speed vehicles into a VRLOG's settled
  background, and whether that read belongs in the `figure` area or in the estimation plan's
  Section 16.

## Sources checked

| Source                                                                                                                                                                                 | Date             | Status   |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------- | -------- |
| Fabris, _Building a Safe AI Driver_, [wayve.ai/thinking/building-a-safe-ai-driver](https://wayve.ai/thinking/building-a-safe-ai-driver/)                                               | November 4, 2026 | Read     |
| Basso, _Validating the AI Driver_, [wayve.ai/thinking/validating-the-ai-driver](https://wayve.ai/thinking/validating-the-ai-driver/)                                                   | November 4, 2026 | Read     |
| Arani and Dagan, _Language Should Ride Shotgun, Not Stall the Control Loop_, [wayve.ai/thinking/language-should-ride-shotgun](https://wayve.ai/thinking/language-should-ride-shotgun/) | November 4, 2026 | Read     |
| Wayve, _GAIA-4_, the closed-loop simulation post linked from all three posts                                                                                                           | August 3, 2026   | Not read |
| LINGO-1 (2023) and LINGO-2 (2024), named in Part 3                                                                                                                                     |                  | Not read |

## Provenance

- Posts fetched on November 5, 2026 with the session's web fetch and again with `curl` into an
  empty scratchpad directory, stripped to text and read whole; the three texts run to 2,616,
  2,582 and 2,637 words including site navigation.
- Repository at `b1522fa2c` (main, November 5, 2026), branch
  `claude/wayve-blog-metrics-review-973ee4`.
- Read for the mapping: the behaviour analytics plan (Sections 1, 5, 7, 9, 11 and 13), the metrics
  registry and its code mirror, the wording guard, the November 5 experiment review (Sections E
  and F, the engineering table and the decisions), the research notes of October 22 and 29 and
  the causal-surrogate note, the state-estimation plan's Section 16, the experiment README and the
  `figure`, `l5id` and `l8fol` specs, `distribution.go`'s pooled rate, and TENETS.md.
- No raw output was produced or read; nothing under a `results` directory was touched; no
  replay was run.
