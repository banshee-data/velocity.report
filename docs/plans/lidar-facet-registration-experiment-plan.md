# Facet registration: test the geometry before expanding the tracker

Test whether persistent fragments of a vehicle's surface improve physical tracking and following
measurements enough to justify their cost. Start with a bounded offline comparison alongside
0.5.2. A useful negative result is an acceptable exit.

- **Status:** Proposed experiment, revised after the mathematical review; no implementation or measured benefit
- **Target:** Decision experiment alongside v0.5.2 when independently staffed; conditional integration in v0.5.6; catalogue and mesh follow-ons at v1.x+
- **Layers:** L3 visibility, L4 geometric evidence, L5 estimation, L6 identification, L8 following evaluation
- **Canonical:** [Visibility-aware tracking maths](../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md)
- **Related:** [Facet registration maths review][review], [0.5.2 MVP](lidar-052-mvp-sprint-plan.md), [near-edge tracked state](lidar-near-edge-tracked-state-plan.md), [October near-edge campaign](../lidar/operations/near-edge-campaign-2026-10.md), [state estimation](lidar-state-estimation-plan.md), [physical references](lidar-physical-reference-review-plan.md), [behaviour analytics](lidar-behaviour-analytics-plan.md), [shape descriptors](lidar-shape-descriptors-plan.md), [single-site demo](lidar-single-site-shape-demo-sprint-plan.md), [vehicle identification](scene-vehicle-identification-plan.md)

[review]: ../../data/maths/proposals/20261008-facet-registration-maths-review.md

## 1. Decision and priority

The hypothesis is that a vehicle's persistent local surfaces, and the relationships between them,
provide more stable physical pose estimates than a changing visible cluster. A facet means a
bounded geometric primitive with uncertain support: initially a line fragment or a planar patch.
A car may need many small patches; a truck may supply fewer large ones. The detector must earn
that distinction from the data.

The first outcome is a decision about measurements for the existing road-conditioned tracker.
Use three-dimensional observations while estimating planar translation and body yaw under an
explicit road assumption. Grade, roll, pitch, and articulation are validity conditions; the pilot
does not establish a general six-degree-of-freedom tracker.

Keep the 0.5.2 critical path on its existing S0–S7 sequence. Physical references, corrected
near-edge tracking, uncertainty calibration, refined body persistence, and following analysis
remain necessary even if facets work. The experiment reuses those contracts and the one-site
pilot rather than commissioning another annotation system or estimator output format.

The latest near-edge plan records a remaining side-face entry tail and a pending T5/F7 comparison.
These are a reason to test the mechanism, not evidence that facets solve it. Complete F7, the A1
ablation, and the planned screening before selecting the baseline for this experiment. Pin that
baseline; compare against the refreshed simpler model, not an old medoid result.

The state plan's Section 9.3 already conditions progression to point-to-model residuals on retained
evidence and residual error attributable to edge localisation rather than dimension priors. An
offline diagnostic can investigate that condition now; adopting facets must satisfy it. If the
error comes from an unseen bumper or a wrong length prior, more patches on the same side will
not create the missing measurement.

The [mathematical review][review] makes that limit quantitative and extends it to the sparse
tail. A patch constrains its normal and nothing else, and patches on one plane share a null
direction whatever their number. Eight returns already place a face plane to 7 mm, while the
half-extent prior that turns a face into a centre carries 0.2 m. The errors that dominate far,
end-on, or occluded views are face identity, the unobserved extent, membership, and occlusion
boundaries read as edges; persistent patch identity reduces none of them. The experiment therefore
separates two decisions. Arm D is a bounded challenger for mid-pass lateral steadiness, where the
near-edge campaign's face-transition tail lives. Arm B, body-local memory of the shape learned
while the vehicle was dense, with a shared visibility component, is the candidate for the sparse
tail. Neither creates a measurement of an unseen bumper: the output contract for that case is an
interval, not a point.

## 2. Separate the claims

| Claim                                                                    | Evidence needed                                                                                                                                                                      | What a pass permits                                                           |
| ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| H1: geometric fragments are physical                                     | Repeatability under known motion, changing scan phase, range, and aspect; every extent end labelled physical, occluded, field of view, or dropout by a scored visibility labeller    | Build a temporal patch representation                                         |
| H2: persistent geometry improves pose                                    | Paired physical endpoint and pose errors against the best simpler comparator, with matched information and motion priors                                                             | Try a facet measurement in the tracked state                                  |
| H2s: body memory and visibility evidence improve pose in the sparse tail | Paired physical pose and endpoint errors in the far, end-on, and occluded strata, with abstention counted; arm B with the visibility component against arm A with the same component | Promote the sparse-tail candidate; publish interval output for unseen extents |
| H3: a constellation improves identity and following                      | Automatic membership and association, correct leader choice, calibrated bumper gap and net time gap, honest coverage                                                                 | Consider integration into the field-qualified pipeline                        |
| H4: the geometry distinguishes vehicle types                             | Held-out physical vehicles and absent-catalogue examples, with confidence and rejection calibrated                                                                                   | Add a supported identification depth                                          |
| H5: the geometry supports a map or mesh                                  | Held-out observed-surface accuracy, view coverage, and revision consistency                                                                                                          | Publish a measured partial surface, with completion provenance                |

H2 and H2s are decided separately; either can pass while H3 fails. H4 and H5 are later
experiments. A smooth trail, low registration residual, or attractive mesh cannot substitute for
any of these measurements.

## 3. Evidence and controls

The [minimum facet annotation specification](lidar-facet-annotation-minimum-spec.md) defines the
operator increment for F0/F1: one to four active features, canonical point subsets per frame,
separate review and body registration, and a ledger limiting discrete reanchors to three after
birth. Ordinary compatible measurements remain continuous. This is proposed tooling and an
experimental policy, not a delivered extractor or a measured tracking benefit.

Start with a planning allowance of 60 independent vehicle passages and 20 following encounters
across at least three captures. The passages should include cars, vans, and rigid trucks. The
encounters may reuse those passages; both members and every overlapping clip belong to the same
split. Use approximately 20 passages for development, 20 for tuning, and 20 for a fresh decision
set, with approximately ten encounters in tuning and ten in the decision set. Preserve existing
split assignments. Previously inspected held-out cases are screening evidence for this new idea;
reserve fresh episodes for its decision.

These counts are a feasibility allowance, not a statistically powered acceptance corpus. Before
opening decision data, use development variability and reference precision to estimate the
episodes needed for the predeclared effect. Add data or return `insufficient_evidence` when the
budget cannot support a decision. Twenty held-out passages cannot establish rare-failure or
population p99 claims. Field promotion continues to use the existing physical gates and their
required evidence.

Select decision windows by traffic opportunity or random sampling. Failure-driven selection is
useful for development only. Hold out a placement or capture session where possible; grouping by
vehicle episode takes precedence over reaching an exact quota. A claim of site transfer needs
fresh site evidence beyond the small pilot.

Cover changing side/end visibility, a long flat truck side, sparse distant returns, oblique
bodies, turns, braking, reversing, temporary occlusion, neighbouring vehicles, and a vehicle near
a wall. Include trailers, strong road grade, and clipped scans as explicit stress cases; either
handle them or declare and test suppression. Report empty strata.

Predeclare the sparse-tail strata before E1 and report them separately in every arm: range at or
beyond 50 m; fewer than 60 returns on the object; folded aspect within 20 degrees of end-on; and
any frame in which an extent end is labelled occluded. On the deployed ring table and a 3 m mount,
an end face drops below the near-edge model's eight-return floor at about 80 m and a side beyond
100 m, so the range bins are 20 to 50 m, 50 to 80 m, and beyond 80 m. Label every extent end of
every evidence frame as physical, occluded, field of view, or dropout by casting the reviewed body
against the L3 range baseline and the frame's nearer returns ([review][review], Section 5.4). The
label is reference material for E1 and an input to the visibility component in E2; its own
precision and recall against the facet audit are an E1 result.

Review three distinct things:

- Membership and identity: which returns belong to which physical vehicle across the episode.
- Physical references: independently established pose, dimensions, and bumper bounds, with
  component uncertainty and unknown geometry retained. Use the existing physical-reference
  contract. Keep the review blind to the candidate trajectory.
- Facet audit: a small set of physical surfaces or edges labelled around view transitions. Dense
  point correspondence is unnecessary because the beam normally strikes different material points.

Use measured vehicle dimensions and a controlled, surveyed pass if natural captures cannot
establish endpoint truth accurately enough. A side-only mask cannot certify an unseen bumper.
Shared sensor/calibration errors must remain in the reference error budget. References whose
bounds exceed the proposed improvement cannot decide that improvement.

Freeze capture, calibration, ring/firing metadata, build, parameters, source times, reference
revision, split, and estimator version. Verify per-return acquisition time and scan topology in
the chosen evidence profile. If topology is absent, restrict that arm to spatial patches and
record that single-strand repeatability was not tested. The timing requirement is sized: the
sweep inside one cluster is 7 ms at 10 m and 1.4 ms at 50 m, so a body-local map built at close
range must deskew, while the filter's per-cluster capture instant stands as it is and deskew
beyond 50 m buys less than the plane sigma ([review][review], Section 4.7).

## 4. Experiment in three steps

### E1: establish whether a strand or patch is physical evidence

Use a small sensor-aware synthetic scene with known rigid motion: a plane, an L-shaped corner,
a cuboid, and a curved vehicle-like shell. Vary scan phase and beam intersections independently
of object motion. Use measured sensor geometry, and perturb range/angular noise, missing returns,
incidence, occlusion, and per-return acquisition time. This is a diagnostic generator, not a
vehicle catalogue. Validate its sampling and noise against real captures before making transfer
claims.

Extract line fragments within a ring and plane patches across sufficient non-collinear support.
A single straight strand does not supply a unique surface normal. Preserve that ambiguity rather
than borrowing a confident normal from a box. Mask boundaries and last returns are not assumed
to be physical edges.

Measure repeatability on common visible support, normal/direction error, correspondence precision,
and false persistence under known pose. Evaluate on synthetic truth, controlled measurements, or
independent physical references, never using the candidate registration as its own truth. Bin
results by range, aspect, support, and primitive type. Perturb and re-match to expose weak motion
directions; verify that a plane remains unconstrained along its tangent despite extra points.

Four checks are must-pass before E2 is funded, each on synthetic truth with the inputs recorded in
the review's Section 9:

- Patches on one plane leave the data-only tangential eigenvalue at zero however many there are.
  An extractor that reports otherwise is counting fixed correspondences or noisy normals.
- Normals perturbed within one degree produce a spurious tangential eigenvalue of order
  `N sin^2(eps)`. The degeneracy test must reject it at a stated noise level, not at a raw
  condition number.
- Line fragments are fitted in plan view or against the ring's cone. A constant-elevation ring
  sags 0.13 to 0.22 m across a flat 4.5 m face at 5 m on the steep rings, and a three-dimensional
  straight-line fit or a planarity threshold must not reject a flat panel for it.
- The extent-end labeller's precision and recall against the facet audit, by range and aspect,
  with the bias an unlabelled occlusion boundary would introduce, half the hidden length, reported
  beside it.

### E2: isolate the registration benefit

Use reviewed memberships as a deliberately favourable diagnostic for every arm. Supply the same
initial seed and seed uncertainty, body-origin convention, motion prior, time correction,
available returns, and history horizon. Subsequent physical references are evaluation-only.

| Arm | Measurement/representation                                                                                                                                                                                                                                                                                                            | Purpose                                                                                                                                           |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| A   | Frozen current near-edge tracked baseline                                                                                                                                                                                                                                                                                             | Measures the value beyond work already funded for 0.5.2                                                                                           |
| B   | Body-local map capped at a declared number of returns, each with provenance and per-return anisotropic noise; robust point-to-plane residuals against normals from the map; the map frozen before each update and de-duplicated against the current scan; abstention below a declared overlap; no exported inverse-Hessian covariance | Controls for gains due merely to retaining history, and is the sparse-tail candidate; locally inspired by SOTracker, not an asserted reproduction |
| C   | Lines and finite planes fitted afresh each frame, with no persistent primitive identities, plus physical-edge features where the labeller supports them                                                                                                                                                                               | Tests whether better local geometry alone supplies the gain                                                                                       |
| D0  | Persistent body-local patch identities without relational terms                                                                                                                                                                                                                                                                       | Optional: separates the identity claim from the graph claim when D differs from C                                                                 |
| D   | Persistent body-local patches and a small graph of rigid geometric relationships                                                                                                                                                                                                                                                      | Tests the full constellation hypothesis against A, B, and C                                                                                       |
| V   | Shared visibility component: extent-end labels, expected support per face, and a free-space penalty from the L3 range baseline and the frame's nearer returns                                                                                                                                                                         | Applied to every arm in the sparse-tail pass; supplies arm A's truncation flag                                                                    |

Give B and D the same permitted history and information; compare C and D with the same extraction
settings. Keep the native A input contract, and disclose representation differences. Run a
matched-input comparison first, then report an equal-memory/compute comparison separately. Do
not improve only D's seed, deskew, loss, or ground calibration. Tune each arm on the same tuning
partition with a declared budget.

Run E2 in two passes. The matched-input pass compares A to D without V on the full population.
The sparse-tail pass adds V to every arm and decides H2s on the predeclared strata, with
abstentions counted in every denominator. Between E1 and E2, decompose the baseline's along-path
endpoint error per development episode into extent-prior error, edge localisation, and filter
lag, using the reviewed references. Fund E2's D arm only if the share a facet can move is large
enough to meet the H2 threshold, and record the decomposition whichever way it falls: edge
localisation is the smallest term in the review's error budget, and a decomposition dominated by
the extent prior routes the effort to extent memory and interval output instead.

Begin with a bounded number of patches and only local graph relationships, such as adjacency,
relative orientation, and separations of supported physical features. A visible patch centroid
can move across a flat surface; it is not automatically a persistent landmark. Retain physical
feature identity separately from the support visible in one scan. Avoid an all-pairs graph or
semantic part labels in the first experiment.

Use a robust surface likelihood, an explicit no-match outcome, a minimum-overlap requirement,
and uncertainty-aware association. Preserve alternative yaw/face hypotheses when comparable.
Record data-only observable directions separately from the motion prior. Correlated cached
points must not count as independent evidence, and a registration posterior containing the
motion prior must not enter that same prior again as a fresh measurement.

Use one bounded optimiser and a declared uncertainty approximation for the pilot, scored by
G-UNC-1 stratified by data-only rank. Report a prior-dominated direction as prior-dominated, never
as a Gaussian sigma: inverse-Hessian covariance from point-to-plane registration is optimistic in
exactly those directions. A general joint shape/pose factor graph is a later escalation if the
approximation cannot meet calibration. Label uncalibrated confidence as diagnostic until tested.

### E3: expose the automatic pipeline and following consequences

Replace reviewed memberships with normal L4 candidates and automatic association. Use the same
one-frame seed policy across arms, then assess automatic track birth separately. Future masks,
poses, or corrected track IDs are unavailable to every candidate. The seed-assisted result is
reported separately from autonomous detection and tracking.

Allow candidate membership to remain revisable within the experiment's bounded history. Test
fragmentation, two vehicles temporarily merged by L4, a wrong patch association, and recovery
after the offending evidence is removed. H3 needs a way to propose a split or to suppress an
unresolved merge; a shape cache must not silently make the mistake permanent. General multi-body
reassociation is beyond this pilot and is costed in the follow-on.

Test track-conditioned membership as a mechanism, not only a policy: gather returns inside the
predicted body footprint widened by its covariance, let the visibility component reject returns
the hypothesis cannot explain, and compare against the clusterer's partition in the sparse-tail
strata. Include the cache rollback case: one wrong association enters the body-local map, its
revision is removed, and the fit must return to within the plane sigma of its pre-contamination
state.

Feed each arm's versioned body trajectory through the existing following analyser. Hold the
reference path fixed first to isolate endpoint error, then repeat with each arm's normal fitted
path and leader choice to measure the product effect. Apply identical stages, eligibility,
speed floor, extent support, and opportunity accounting. Use causal inputs for online comparisons;
compare fixed-lag/final output only with matching information horizons and declared latency.

For gap g and follower speed v, net time gap is g/v. As a sensitivity example, a 0.5 m gap error
at 10 m/s contributes 0.05 s of time-gap error if speed is exact. A 1 m/s speed error with a 10 m
gap at 10 m/s contributes about 0.1 s under first-order propagation. Measure endpoint, speed,
heading/path, pairing, and reference contributions before deciding where another week helps.
Propagate their joint uncertainty, including shared sensor errors. Better lateral alignment alone
does not establish better longitudinal gap.

## 5. Scorecard and decisions

Use paired comparisons on identical episodes. Report both common supported instants and the full
preselected opportunity population, including every abstention and failed track. A method cannot
improve its score by discarding its difficult cases. Resample whole independent episodes or
capture groups for intervals; do not treat frames or overlapping pairs as independent trials.

The following are proposed research decision thresholds. Confirm their practicality on development
data and freeze them before decision scoring. They are additional to the existing release gates.

| Decision                      | Proposed criterion                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Continue after E1             | Real-data geometric support agrees with the synthetic mechanism; retained patches add observable information or reduce measurement bias on target cases. If only sampling boundaries repeat, stop persistent facet identity work.                                                                                                                                                                                                                                                                                     |
| H2 worth integration          | At least 20% reduction in paired p95 absolute along-path physical-endpoint error against the best simpler arm; improvement interval excludes zero and exceeds reference resolution. Publish absolute metres as well as percentages.                                                                                                                                                                                                                                                                                   |
| Constellation justified       | D provides that material gain over the best of A/B/C, or a separately predeclared coverage benefit at equivalent physical accuracy. If C or B ties D within the decision resolution, choose the simpler representation.                                                                                                                                                                                                                                                                                               |
| H3 worth field promotion work | Default objective: at least 20% lower p95 physical gap error at matched supported opportunity, with no degradation beyond frozen margins in speed, net time gap, pairing, identity, or uncertainty. An alternative objective is at least 10 percentage points more valid opportunity while meeting the same absolute accuracy and calibration limits. Choose the objective before decision scoring.                                                                                                                   |
| Sparse tail (H2s)             | On the predeclared far, end-on, and occluded strata, B with V is not worse than A with V on paired physical pose error at matched supported opportunity, and lowers abstention or error by a margin frozen in F0. D with V must match B within the decision resolution to remain a tail candidate; otherwise the tail decision names B.                                                                                                                                                                               |
| Attribution gate              | E2's D arm is funded only if the per-episode decomposition of along-path endpoint error attributes enough to edge localisation for the H2 threshold to be reachable. A decomposition dominated by the extent prior routes the effort to extent memory and interval output, and that routing is itself a recorded outcome.                                                                                                                                                                                             |
| Guardrails                    | Freeze absolute error/non-inferiority margins from the following error budget in F0. Retain existing geometry/identity/manoeuvre criteria; report failures and denominator changes. A stratum with insufficient truth cannot pass.                                                                                                                                                                                                                                                                                    |
| Uncertainty                   | Evaluate empirical coverage and interval width together, by support/range/aspect and by data-only rank, using the existing G-UNC-1 protocol where its assumptions hold. Narrower wrong intervals fail; wider intervals alone do not win. A prior-dominated direction reported as a Gaussian sigma fails.                                                                                                                                                                                                              |
| Runtime                       | Measure added p50/p95/p99 frame time and bounded memory on M1 at a declared scene load. The October campaign measured the near-edge tracked arm at 3.5 to 8 ms p99 per update on the M1 Pro, 15 to 35 times the production tracker and already above the state plan's under-3-ms gate, which still applies to promotion. F0 states a per-pair cost model and budget; a facet measurement is made per assigned pair unless association is shown to need it. An offline result may pass H2 while failing deployability. |

Record longitudinal/lateral centre and endpoint error, yaw ambiguity, extent error, gaps and net
time gaps, false leader choices, ID switches, fragmentation, reacquisition, and valid-opportunity
share. p99 and maxima are diagnostics until the sample supports their interpretation. Include
observable rank, prior dominance, overlap, patch churn, wrong-surface merges, and cache rollback.

Return one of four outcomes: `go`, `use_simpler_arm`, `no_measured_gain`, or
`insufficient_evidence`. Do not tune on a failed decision set and call the next result held out.
Revisions need a new split or a declared later confirmation set. A research pass does not waive
G-GEO-1, G-UNC-1, G-SMO-1, the applicable evidence-fidelity gates, or the following-metric gate.

## 6. Work packages and effort

Estimates are focused engineer-days for someone familiar with the repository and numerical
estimation, including implementation review, targeted tests, and the decision report. They assume
replay, reference import/scoring, and the source evidence work. They are planning ranges, not
measured delivery rates. Operator effort and elapsed waiting for captures are separate.

| Package | Deliverable                                                                                                                                                                             | Engineer-days | Dependency/exit                                                |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------: | -------------------------------------------------------------- |
| F0      | Freeze source/baseline, audit scan metadata, allocate shared references, pin error budget, decision protocol, sparse-tail strata, and the per-pair cost model                           |           2–4 | Existing S0/reference tooling; stop or repair missing evidence |
| F1      | Ring/patch extractor, small sampling simulator, extent-end labeller and its score, E1 repeatability and degeneracy report                                                               |           4–6 | F0; first stop/go                                              |
| F2      | Common evaluator plus A–D seeded registration arms with B specified, the shared visibility component, bounded cache, weak-mode diagnostics, the attribution gate, and the two E2 passes |          6–10 | Useful F1 primitives; H2 and H2s comparisons                   |
| F3      | Automatic-membership stress test, following adapter, paired decision scorecard and recommendation                                                                                       |           5–9 | Physical references and F2; H2/H3 decision                     |
| F4      | Conditional tracked-state integration, versioned persistence/revisions, inspector diagnostics, uncertainty recalibration and gate reruns                                                |         10–15 | F3 gain and 0.5.2 contracts; scoped opt-in candidate           |
| F5      | Wider scenes, split/merge association, cache recovery, resource limits, and robustness qualification                                                                                    |         10–20 | F4; stated operating envelope                                  |

F0–F3 total **17–29 engineer-days**, about **3.5–6 focused engineering weeks**. The widening over
the first estimate pays for the extent-end labeller, the B specification, and the second E2 pass;
it is the review's cost, not slack. Allow **20–40 operator hours** initially for sparse-keyframe
review and adjudication; the labeller can propose edge labels and should reduce it. Measure the
first five episodes and reforecast rather than hiding annotation cost inside coding. If references
or scan-time metadata need additional plumbing, reserve **5–10 shared engineering days** and, where
needed, **1–2 controlled field days**. Those are shared 0.5.2 evidence costs and must not be counted
twice. More independent captures or broad confirmation statistics extend the calendar.

F0–F5 total **37–64 engineer-days**, about **7.5–13 focused engineering weeks**, conditional on
each decision. With one dedicated engineer, a workable corpus, and a reviewer available, allow
roughly **5–7 calendar weeks** for the first decision, then **4–7 more calendar weeks** for
F4–F5. These figures do not estimate the remaining whole 0.5.2 programme, catalogue construction,
full 6D tracking, articulation, a general factor graph, or Pi optimisation.

## 7. Schedule against the existing MVPs

| Existing milestone                                                             | Facet work alongside it                                                       | Release relationship                                                           |
| ------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| 0.5.2.0 / S0: immutable evidence and physical references                       | F0; share source topology, reference uncertainty, split manifests, and scorer | Shared dependency; prioritise this work first                                  |
| 0.5.2.1 / S1–S2: body anchors and near-edge tracked update                     | F1–F2 offline, against the completed F7/A1 baseline                           | Facets remain a challenger; no new MVP A dependency                            |
| 0.5.2.2 / S3–S4: calibrated uncertainty, continuity, refined body stages       | F2 diagnostics and prototype output adapter; F3 as inputs become available    | Freeze estimation conventions; candidate changes require recalibration         |
| 0.5.2.3–4 / S5–S7: following inspection, persistence, report and qualification | F3 uses the same analyser and references                                      | Provisional MVP A can finish independently; qualified exit B retains all gates |
| v0.5.3: passing clearance and PET                                              | Reuse references/error attribution if useful                                  | Do not make these products depend on a constellation graph                     |
| v0.5.6: perception/descriptor foundations                                      | F4–F5, only after a useful decision                                           | Natural default home for supported facet measurements and descriptors          |
| v0.6.2–3 mobile transfer, v0.6.7 Pi pass                                       | Revalidate sampling, ego-motion, timing, and target-hardware budget           | Static M1 evidence does not transfer automatically                             |
| v1.x+ vehicle identification and surface products                              | Separate H4/H5 pilots                                                         | No dependency on the first qualified headway report                            |

With **two engineers**, protect one for S0–S7 and assign the other to F0–F3, sharing the evidence
work and operator queue. Approximate research schedule: week 1 F0/F1; weeks 2–3 F1/F2; weeks 4–5
F2/F3; week 6 only for reference/review allowance or a declared evidence decision. S0–S7 completion
has its own schedule; parallel boxes on a calendar do not make references arrive sooner.

With **one engineer**, spend at most two days now on F0's evidence audit and error attribution,
then finish the provisional headway MVP and unblock its physical references. The first session
of that audit is written up as a runnable protocol on kirk0's reviewed pack
([F0 attribution on kirk0](../lidar/operations/facet-f0-attribution-kirk0.md)): four existing
arms, the near-face and identity scorers, and predeclared readings for the attribution gate, the
face-entry tail, and the sparse-tail onset. It ran on 2026-10-08
([report](../lidar/operations/facet-f0-attribution-kirk0-report.md)). The predeclared gate
reading kept arm D's case open, but the end error it attributes to localisation was mostly a
believed extent shorter than the labelled span; restate the gate to separate the two before it
funds arm D. Resume the remaining
experiment after MVP A while field qualification is being prepared. Splitting one person between
two critical paths adds delay to both. Keep a named date or milestone for the research decision;
do not leave an indefinitely running background experiment.

There is one reason to pull F4 into 0.5.2: the simpler tracked model fails the physical/metric gate,
F3 attributes the failure to visible-surface registration, and the facet arm fixes it on independent
evidence. Record the changed critical path and revised release estimate before integration. If
the blocker is missing references, unsupported length, poor class evidence, or absent persistence,
facets do not unblock it. If the simpler model passes, ship that qualified envelope and compare
facets as a later version.

## 8. Identification and mesh follow-ons

For H4, keep the measured primitive representation separate from catalogue labels. After H2/H3,
budget an additional **15–30 engineer-days** for a small body-style/model-family retrieval pilot,
assuming a reviewed shell subset and real exemplars already exist. Generate sensor-matched views
of catalogue shells, compare facet constellations against the existing descriptor-only approach,
and test unseen physical vehicles and absent catalogue entries. Make/model resolution is an
outcome to measure; a family-level or unknown answer is valid. Catalogue acquisition and broad
make/model validation need their own estimate. Keep identity priors out of the first tracking
experiment to prevent a guessed model from becoming its own geometric proof.

For H5, budget an additional **10–20 engineer-days** for a bounded partial-surface and mesh pilot
after reliable registration, assuming existing rendering and storage tooling. Keep vehicle maps in
body coordinates and static scene geometry in world coordinates. Rebuild or subtract contributions
when poses or membership change. Export measured, interpolated, and catalogue-completed surfaces
with different provenance, and score against views withheld from fusion. A watertight vehicle mesh
is optional; missing observations remain missing. General static mapping and detailed complete
vehicle reconstruction are separate scope.

Both estimates are exploratory extensions, not commitments to production identification or full
mesh mapping. The representation can support them later without making either a prerequisite for
measuring the distance between two vehicles.

Body-local shape data is a description of one vehicle's surface. Keep it in memory for the track's
lifetime only; persist nothing body-local in production; publish aggregate descriptors, never a
per-vehicle surface; and report H4 at family level. No PII by architecture is a tenet, and a
retained vehicle shape cache would be a new class of retained data that needs its own review
before any field build carries it.

## 9. Research basis and first deliverable

[SOTracker](https://arxiv.org/abs/2103.06028) combines point registration, accumulated vehicle
shape, and motion priors. It motivates arm B and the supplied-seed experiment; it does not prove
that persistent facets outperform points on our sensor.

[Mannari et al., ET-PMHT for partially visible convex polytopes](https://ietresearch.onlinelibrary.wiley.com/doi/10.1049/rsn2.70061)
is close to the surface/visibility model, but its single or widely separated target setting does
not solve our close-vehicle association problem.

[Kumru and Özkan, 3D extended-object tracking with Gaussian processes](https://arxiv.org/abs/1909.11358)
provides a joint shape/kinematics alternative. Keep it as a later comparator if explicit patches
fail on curved bodies; implementing all three research systems would exceed the bounded pilot.

The [mathematical review][review] adds the older and closer literature, with references in
`data/maths/references.bib`: Petrovskaya and Thrun's per-vehicle Bayes filter with anchor points
and a ray-cast likelihood; Granström, Lundquist, and Orguner's rectangle measurement model for
laser scanners, of which the near-edge model is a special case; Wyffels and Campbell's negative
information for occluded extended objects; Held et al.'s tracker that degrades gracefully to a
handful of returns against an accumulated model; Kraemer, Stiller, and Bouzouraa's polylines with
free-space information; Zhang et al.'s L-shape corner; and the ICP covariance and degeneracy
results of Censi, Brossard et al., Zhang, Kaess, and Singh, and Tuna et al. None uses persistent
small-patch identities with a relational graph as the pose measurement. The published trackers
that handle sparse returns best use accumulated shape or free space, or both.

The first deliverable is a source-pinned experiment manifest, a reference queue shared with S0,
and a baseline error-budget report. It must say whether the present headway limit is longitudinal
endpoint geometry, speed, identity, visibility, or missing evidence. That answer determines whether
F1 is worth funding and gives a failed facet experiment something useful to leave behind.

## 10. Side effects register

The [review][review], Section 8, derives each entry; this table is the working list for F0.

| Side effect                 | Expected                                                                                                             | Handling                                                                                       |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Runtime                     | More work per candidate pair than the near-edge arm, which already measures 3.5 to 8 ms p99 per update on the M1 Pro | Per-pair cost model and budget in F0; measurement per assigned pair; the runtime scorecard row |
| Memory                      | A capped body-local map or patch set per live track, tens of kilobytes at a thousand returns                         | Declare the cap; evict on track end                                                            |
| Determinism and replay      | A cross-frame cache adds state; iteration counts and float order can differ between runs                             | Fixed iteration counts, ordered reductions, byte-equal repeat runs as in the campaign          |
| Identity stickiness         | A wrong association or merge enters the cache and keeps pulling the fit toward itself                                | Provenance per contribution; the E3 rollback case; no silent permanence                        |
| Double counting             | Cached returns re-scored against the current scan; a posterior holding the motion prior fed back as a measurement    | Frozen pre-update cache, de-duplication, one combined update                                   |
| Calibration                 | Inverse-Hessian covariance optimistic in weak directions; spurious eigenvalues from noisy normals                    | Prior-dominated directions reported as such; G-UNC-1 by data-only rank                         |
| Interaction with T1, T3, T5 | A second measurement model on the same filter without a declared merger                                              | One measurement model per arm; the facet arm replaces the face update rather than adding to it |
| Class prior                 | Half-extents remain the class prior until extents converge, whatever the patch fit does                              | Extent memory and interval output                                                              |
| Configuration fingerprint   | New tuning keys move the fingerprint and refuse the perf baselines                                                   | An options block, absent by default, as the solid body uses                                    |
| Persistence and VRLOG       | Patch identities, revisions, and body registrations need a schema to be inspected or replayed                        | Offline and separately versioned until F4                                                      |
| Operator load               | Facet review per informative frame                                                                                   | The labeller proposes edge labels; measure the first five episodes and reforecast              |
| Privacy                     | A persisted vehicle shape cache is a new class of retained data; H4 retrieval approaches identification              | The retention rule in Section 8; family-level H4; aggregate descriptors only                   |
