# SRAC research and validation plan

Develop an offline steering-demand evaluator only after its mathematical contract is reviewed.
The immediate deliverable is the [Euler proposal](../research/srac-steering-demand-maths.md).
This plan describes later work; it schedules no campaign and adopts no production metric.

- **Status:** Proposed; implementation, experiment execution, and physical qualification pending.
- **Canonical:** [Metrics registry](../platform/architecture/metrics-registry.md).
- **Related:** [Maths proposal](../research/srac-steering-demand-maths.md),
  [behaviour analytics plan](lidar-behaviour-analytics-plan.md),
  [geometry data purpose](../geometry-priors-data-purpose.md).

## Work order and acceptance

| Stage                     | Deliverable                                                                        | Acceptance                                                                                                                                    | Dependency                                       |
| ------------------------- | ---------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ |
| 1. Mathematical review    | Resolve nominal policy, control objective, terminal support, and scene opportunity | Independent derivation of crossing, settled, curved, and blocked-corridor examples; explicit sign/unit conventions                            | Proposal review                                  |
| 2. Analytical worksheet   | Small deterministic reference tool, later implemented with tests                   | Reproduces all authored examples; distinguishes lower bounds from executable witnesses; suppresses degenerate inputs                          | Stage 1                                          |
| 3. Offline evaluator      | Joint actor geometry, bounded manoeuvre families, independent witness checker      | Every accepted witness meets dynamics, geometry, and terminal requirements within declared error bounds; unresolved search remains unresolved | Stage 2, input contract                          |
| 4. Observation audit      | Source-bound review of available tracks and scene support                          | Records what can be observed, inferred, or only assumed; complete exclusion accounting                                                        | Qualified physical references                    |
| 5. Review plots           | Scene view, delay curve, demand timeline, exclusion ledger                         | A reviewer can trace one supported and one suppressed case to source evidence                                                                 | Stages 3–4                                       |
| 6. Qualification decision | Held-out physical evaluation and written recommendation                            | Meets preregistered physical-error and interval-coverage criteria or remains review-only                                                      | Adequate independent references and frozen split |

Do not expand the existing weekend following/DRAC programme to absorb these stages. They have
separate prerequisites, particularly steering capability, future occupancy, and terminal support.
Keep ownership in L8; reuse physical track/scene contracts rather than modifying the tracker to
compute a criticality score. No database migration, public API, or UI is necessary for stages 1–3.

## Candidate tools and numerical design

The worksheet answers whether the formula and boundary conditions are understood. A finite
manoeuvre library answers whether one of its candidates works. A continuous optimiser may improve
a candidate, but does not certify global optimality merely by terminating. Keep each tool's
claim explicit. Compare with analytical reference cases before introducing a larger solver
dependency. A reference optimum bounds a different model only after proving a relaxation mapping:
matching inputs and units alone is insufficient. In particular, fixed-forward-motion lateral
examples do not automatically bound a bicycle model whose steering changes overlap time.

For N time intervals, K candidates, and M obstacles, a direct fixed-complexity footprint checker
costs O(KNM); streamed checking can use O(M) working memory besides the input and retained witness.
A branching search with B controls per step has O(B^N) sequences. General nonconvex optimisation
has no promised polynomial global convergence here. A spatial index can reduce typical checks,
but does not change the worst-case evidence requirement.

Require independent continuous-time or conservatively bounded swept collision checks. If the
clearance function has a proved Lipschitz bound V, sampled clearance of at least c+VΔt/2 at both
ends of each interval suffices for clearance c between samples; integration and footprint error
must also be subtracted from the margin. V must include rotational footprint motion, not just
centre speed. Without such a bound, reducing the timestep is sensitivity evidence, not a proof.

Termination records wall/iteration budget, best feasible witness, any valid lower bound, and
objective gap. Convergence means a justified gap and constraint tolerance, not successive iterates
looking similar. Inspect delay discontinuities, tangencies, narrow passages, and solution switches.
Use compensated or rescaled calculations where subtraction of nearly equal positions would
consume clearance precision. Crossing formulas near T=τ are ill-conditioned and need a domain
refusal based on input uncertainty, not a convenient numeric clamp.

## 6.35-second capability gate

Follow the proposal's [two-mode horizon contract](../research/srac-steering-demand-maths.md#a-635-second-horizon-on-a-20-hz-evaluation-grid).
Before any real-time work, qualify 127 future intervals plus the current state separately for
past-only prediction and recorded-future analysis. Extend E-S1 with between-node collision and
objective extrema, endpoint/terminal checks, irregular timestamps and certified integration error.
Extend E-S2 with joint-scene remaining-horizon coverage, fixed-lag availability, missing entrants,
scene exits and counterfactual actor-response assumptions. Extend E-S3 with horizon-wise enclosure
coverage and false-feasible uncertainty. Keep failed/censored opportunities in the report.

The initial deliverable is an offline witness/certificate viewer with separate information modes,
not a 20 Hz live solver. Benchmark K candidates, M actors and U scenarios with explicit caps,
refinement counts and deadline outcomes before proposing live scheduling. A solver timeout must
retain unresolved status; a valid witness retains its certified upper bound without claiming
optimality. No existing tracker covariance or 127-point interpolation supplies physical qualification.

## Scenario acceptance matrix

| Case                                                | Required result                                                             |
| --------------------------------------------------- | --------------------------------------------------------------------------- |
| Nominal clear straight path                         | Zero additional demand only after geometry, capability, and terminal checks |
| Safe curved path                                    | Zero additional demand with positive total acceleration                     |
| Stationary obstruction                              | Crossing lower bound and settled response distinguished                     |
| Braking lead vehicle                                | Piecewise relative dynamics including each stop time                        |
| Cut-in or crossing pedestrian                       | Time-dependent footprint overlap; no following-only shortcut                |
| Adjacent cyclist or occupied escape lane            | Reject an ego path conflicting with the adjacent actor                      |
| Road boundary or kerb                               | Full footprint containment; optional legal rules separately reported        |
| Braking-only escape                                 | Steering infeasibility does not imply collision inevitability               |
| Combined escape                                     | Joint witness; “required” only with both pure families excluded             |
| No candidate found                                  | Unresolved unless infeasibility is certified for the declared family        |
| Missing support, ambiguous body yaw, or weak extent | Reasoned suppression; no safe zero                                          |
| Horizon-edge escape                                 | Explicit terminal status; no durable-avoidance claim from truncation        |
| Zero speed, overlap at start, τ≥H                   | Inapplicable or initial-conflict status as appropriate                      |
| Irregular timestamps and retrospective smoothing    | Evidence cutoff retained; causal mode rejects future leakage                |

Test rigid translation/rotation invariance and dimensional scaling analytically. For fixed
information, horizon, and nested control sets, more waiting cannot improve the optimum. Across
new observations, changed predictions can lower demand. Reflection swaps left/right only when
geometry and directional capabilities are reflected too. Adding obstacles or tightening a bound
cannot improve the same optimum; changing the nominal objective or terminal set breaks that
comparison. Include independent references for assertions that later tests will encode.

Include these mathematical regression cases before using any solver output as a bound:

- A suboptimal witness with q₀=0 and cost 2 when the optimum is 1: its total-demand upper bound
  must use 2, not 1. Preserve any known optimality gap.
- A curve with nonzero longitudinal acceleration: distinguish the rate of q from inertial
  lateral jerk by the yaw-rate-times-longitudinal-acceleration term.
- Two bicycle trajectories with the same speed history and different headings: recompute
  longitudinal overlap and rotated footprints instead of freezing the nominal conflict time.
- A blocked cross-section in the prescribed-forward-motion reference family: label its
  infeasibility certificate with that family, and do not promote it to the bicycle family.

## Proposed experiments

These are preregistration outlines, not executable specifications. Before a run, create checked-in
parameter sets under `config/experiments/` with README rows naming changed keys and report paths.
If a new parameter has no supported schema yet, implement that contract first; do not conceal it
in command-line overrides. Preserve source/config/model fingerprints and a held-out use ledger.

### E-S1: reference-model and numerical sensitivity

- **Question:** Can the estimator preserve known demand bounds and reject between-sample contact?
- **Hypothesis:** Independent witness checking separates feasible candidates from attractive but
  invalid discretised solutions.
- **Baseline:** Closed-form crossing and settled double-integrator cases in the proposal.
- **Arms:** A bounded manoeuvre library and a smooth candidate family; each at three frozen time
  resolutions and with a separate swept-geometry check. These are different search families,
  so differences in optimum are not automatically numerical errors.
- **Inputs/split:** Authored synthetic geometry; tuning cases for development and separately
  frozen adversarial holdouts for tangencies, short contacts, and narrow corridors.
- **Measures:** Objective-bound gap, constraint violation, witness rejection rate, unresolved
  fraction, and runtime. Record every case, including failures.
- **Decision:** No accepted witness may violate the independent checker. Reference optima for
  the same mathematical problem must fall inside reported bounds; cross-model bounds require
  a proved relaxation mapping. Pin numeric tolerances from integration/geometry error budgets
  before scoring; this outline deliberately does not invent an accuracy target.
- **Limits:** Synthetic agreement establishes implementation consistency, not vehicle feasibility.

### E-S2: input sufficiency and baseline sensitivity

- **Question:** Which roadside opportunities support the declared model, and how much does the
  nominal continuation change the answer?
- **Hypothesis:** Scene support and nominal-policy choices materially affect eligibility and demand.
- **Baseline/arms:** Same frozen physical tracks under straight constant-velocity continuation
  and a supported curved continuation, compared only where both apply. Capability scenarios
  remain labelled assumptions. Do not infer a vehicle's limits from its observed maximum response.
- **Inputs/split:** Qualified trajectory/geometry records with acquisition and availability
  provenance; site/episode splits chosen before outcome inspection. No selection for large SRAC.
- **Measures:** Paired demand differences, rank changes, feasible-family changes, horizon effects,
  and candidate/eligible/excluded duration by reason. Cluster uncertainty by episode or site,
  not by treating correlated frames as independent samples.
- **Decision:** A yield report and explicit supported-domain recommendation, even if no physical
  opportunity qualifies. No production adoption from agreement between two assumed models.
- **Limits:** Sensitivity to assumptions does not identify which continuation the driver intended.

### E-S3: independent physical qualification

- **Question:** Are the trajectory, geometry, and capability inputs accurate enough for demand
  and feasibility statements in the selected domain?
- **Baseline/arms:** Independent surveyed/reference motion and measured vehicle controls versus
  LiDAR-derived inputs on the same frozen scenarios. Compare nominal and bounded treatments.
- **Inputs/split:** Instrumented, controlled reference manoeuvres where feasible; independent
  geometry and motion references with their own uncertainty. Human labels alone do not validate
  unseen bumpers, friction, steering state, or an unperformed counterfactual.
- **Measures:** Position/heading/velocity error, clearance error, demand sensitivity, false feasible
  declarations, interval coverage, and abstention rate, stratified by range, occlusion, and class.
- **Decision:** Pin domain-specific physical tolerances, sample-size requirements, and allowed
  false-feasible uncertainty before collecting/scoring holdouts. Insufficient evidence keeps the
  metric review-only. A correctly executed reference manoeuvre supplies a feasible witness, not
  proof that it was the minimum possible intervention.
- **Limits:** Physical qualification does not validate a crash-risk surrogate. That requires an
  independently designed outcome study and cannot be supplied by this implementation project.

## Documentation and release boundary

Each later behavioural change includes its own tests. Each experiment produces a readable report
under docs with arm definitions, edited per-case summaries, provenance, limitations, and decision;
raw outputs remain local. Link it from the relevant plan/index and a unique devlog fragment.

Production consideration requires a reviewed metric identity and scene-level opportunity contract,
physical qualification, uncertainty calibration appropriate to the claim, and reproducible
exclusion accounting. Preserve the existing restriction on publishing review-only results.
No release milestone, threshold, or priority changes through this plan alone.
