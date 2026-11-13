# Oracle brief: a weekend towards per-site following and DRAC logging

Prepare an executable weekend programme for November 14–15, 2026. The objective is a
reproducible per-site record of following gap, net time gap, and experimental DRAC, with
enough evidence to explain which measurements can be trusted and which are suppressed.
Use the questions below to choose the development work and at most three experiments.

- **Status:** Questions answered by GPT-6 Astra in the
  [oracle response and weekend work order](lidar-weekend-facets-headway-drac-oracle-response.md).
  Experiments, implementation, adoption and user decisions remain pending.
- **Scope:** Fixed roadside LiDAR, physical body estimation, facet diagnostics, interaction
  persistence, and site-level measurement coverage.
- **Starting discussion:** [Explore facet-based LiDAR tracking](codex://threads/01a0f41f-e8f6-7912-8e49-1d8814bf123a).
- **New research:** [PR #1017](https://github.com/banshee-data/velocity.report/pull/1017),
  merged at `e9ff0a16a614cc990bd5bfcbcbe3da7087537eef`.

## Request to the oracle

Act as a technical reviewer planning work that can finish this weekend. Challenge the
proposed priorities with the evidence below. Separate existing implementation, measured
results, research proposals, and your own hypotheses. Read current code before proposing
another implementation of an existing feature. Cite a primary source or derivation for
new algorithmic claims, and give each proposed change a falsifier.

For planning, assume one engineer has 16 focused hours across the weekend, an operator
can contribute two additional hours, and existing replay compute can run overnight.
These are planning assumptions, not confirmed availability. Give a reduced plan if only
eight engineer-hours are available, and identify any work a second engineer could do
independently. Include time for implementation, meaningful tests, report writing, and review.
Do not put a new field campaign or the full annotation-tool redesign on the critical path.

The desired Sunday demonstration is: select a site and explicit analysis version, reproduce
its interaction records and coverage accounting from persisted evidence, and inspect why
headway or DRAC is available, provisional, inapplicable, or suppressed. A truthful empty
distribution with complete diagnostics is useful, but report it separately from obtaining
qualified measurements on independent encounters. Specify both exits.

## Evidence

The linked sources below are frozen to the preparation snapshot. Their measurements are
historical results, not reruns of the current checkout.

| ID  | Finding                                                                                                                                                                                                                                                                                            | Consequence for the questions                                                                                                                                       |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| E1  | The original facet discussion became the [registration experiment plan][facet-plan]. It compares refreshed near-edge tracking, accumulated-point registration, fresh patches, and persistent constellations. Its full decision programme was estimated at 15–25 engineer-days, before integration. | Choose a small attribution or repeatability test this weekend; do not describe a complete facet tracker as a two-day task.                                          |
| E2  | [F0][f0] mixed face placement and extent shortfall. [geom0016][geom0016] restates the attribution gate to separate them and requires reviewed masks for its decision read.                                                                                                                         | The old gate's apparent support for constellations is not an adoption result. Missing extent cannot automatically be fixed by better face localisation.             |
| E3  | The [box-based following run][box-following] scored 10 of 11 kirk0 tuning reference gaps, with 0.18 m mean absolute error and 0.48 m maximum. It found 212 s of valid following at 18 of 23 sites, out of 1,593 s accounted.                                                                       | There is a provisional end-to-end path to extend. These selected gaps involved seen ends; they do not validate unseen bumpers or calibrated endpoint uncertainty.   |
| E4  | In that run, 767 s were class-suppressed. The [class counterfactual][class-following] raised valid time to 623 s by treating every `dynamic` track as a car, explicitly an invalid upper bound. Its proposed size-and-motion rule failed.                                                          | Measure the class bottleneck without treating a relaxation of class evidence as a geometry improvement. The October figures need refreshing on the chosen baseline. |
| E5  | The [second-segment G-EST-1 read][gest1] pooled 8,426 tracks over 45 segments. On the second segments, along-track standardised residual RMS was 2.419 at age at most 1 s and 0.846 afterwards.                                                                                                    | The measured excess is a track-start effect. The report keeps CA and IMM offline; it does not qualify speed, acceleration, or relative speed physically.            |
| E6  | The subsequent [velocity-prior shadow screen][prior] still failed invented-speed and noise clauses. It left 33 rows over 15 m/s at franklin-mcallister and increased duplicate rates on all six segments.                                                                                          | Initialisation has a diagnosed problem but no accepted remedy. Do not repeat the November 8 proposal as if the later options had passed.                            |
| E7  | [geom0019][profile] failed its four clauses. [geom0020][anchored-profile] corrected admission and origin movement, then still failed P1–P4: early unseen ends remained censored and faster cars grew too long.                                                                                     | An accumulated map must account for pose, heading, timing, and visibility. Adding persistence alone has not solved extent.                                          |
| E8  | The [likelihood/birth-gate screen][identity] reproduced kirk0's 90-to-6 switch reduction, but no arm passed K1–K3 on placements; no arm proceeded to K5.                                                                                                                                           | Strong tuning identity gains are not a promoted cross-site baseline. Count changes can also reflect changed fragmentation and the moving-track definition.          |
| E9  | [PR #1017's synthesis][distribution-research] proposes exact versioned passage/encounter records, exposure, censoring, and distinct weighting. It changes no runtime metric or release order.                                                                                                      | Translate its proposal into a minimal implementation boundary. Preserve the shipping following distribution.                                                        |
| E10 | [l8fol0004][following-limits] remains an approved specification for following references, leader scoring, and pinned limits. [The one-hour labelling plan][labelling] is proposed and explicitly says nothing in it is built.                                                                      | Audit actual evidence and tooling availability. Do not budget the proposed one-hour workflow as demonstrated throughput.                                            |

Existing code also narrows the work. [Fieldrun.Run][fieldrun] already reads persisted
estimates, solid bodies, or online boxes, calls `AnalyseFollowing`, and writes versioned
interactions. [InteractionStore][interaction-store] persists events, instants, and exposure
windows with idempotent replay and conflict refusal. [The scene API][scene-api] selects one
source/version and clips encounters to its window. That is not yet proof of durable site/run
attribution: its source discovery is based on overlapping capture times.

The inspected [metric table][metric-table] registers following quantities, not DRAC or TTC.
[Following projection][following-code] supplies bumper projections and both parties' along-path
speeds, but its uncertainty calculation explicitly lacks pose/extent cross-covariance.
The box reader's reported endpoint sigma is a lower bound, as E3's report explains.

The facet foundation is further along than the November 8 maths proposal alone suggests:
[segment fitting][segment-code] tests straightness in plan view, and
[FacetMeasurementFromProposal][facet-code] supplies measured yaw, explicit bounds-to-variance
conversion, unresolved directions, and an interval alternative. The code also warns about
prior reuse and yaw ambiguity. This is a measurement adapter, not a qualified runtime facet
tracker.

## Open questions

Discuss these in dependency order, beginning with the Sunday demonstration and metric definition.

### Q1. What can we finish and demonstrate on Sunday?

Starting from E3 and the existing persistence path, identify the shortest dependency chain
to per-site logging. Which parts are engineering work we can complete with synthetic
verification, and which require independent physical references? Give two acceptance lists:
reproducible provisional logging, and physically qualified metric availability. Name the
largest remaining obstacle for each, and propose an early stop if the chosen sites have too
little jointly observed following. Does the best weekend spend most time on L8/persistence,
class evidence, physical endpoints, or initialisation? Rank these by expected increase in
trustworthy encounter coverage per engineer-hour.

### Q2. What exactly are we measuring, and does DRAC require CA?

Keep fixed-line front-to-front passage headway distinct from the current net following time
gap. For a confidently following pair, define supported bumper gap `g`, follower along-path
speed `vF`, leader speed `vL`, and closing speed `c = vF − vL`. Review net time gap `g/vF`
and the candidate constant-relative-deceleration DRAC `c²/(2g)` on the closing domain.
State the collision-avoidance assumptions, units, and applicability limits. Distinguish
required deceleration from actual measured braking.

Can experimental DRAC therefore start from qualified CV relative velocities without an
acceleration state? What evidence would justify changing that answer? Specify metric-specific
eligibility for non-closing traffic, low follower speed, small or uncertain closing speed,
gap bounds crossing zero, overlapping bodies, missing endpoints, and crossing/merging pairs.
Decide when a true zero is meaningful and when the outcome is inapplicable or suppressed.
Do not inherit headway's opportunity speed floor or encounter ceiling without checking whether
it excludes the DRAC situations we want to observe.

### Q3. Which error currently dominates the metric, rather than the picture?

Design a paired error-attribution experiment covering leader choice and identity, visible
endpoint placement, unseen extent, speed/relative speed, heading, timestamp alignment, and
path geometry. Derive uncertainty propagation for `g/vF` and `c²/(2g)`, retaining covariance
terms. Explain what changes near zero gap or closing speed, where linearisation may fail.
Shared registration error and same-vehicle position/extent correlation must not be counted
as independent errors by default.

Which diagnostic substitutions of reviewed membership, endpoint bounds, leader decisions,
or speed references would isolate these causes? Label such substitutions as diagnostic oracle
inputs, separate from deployable arms, and keep target truth out of candidate initialisation.
Specify the minimum reference precision required for each conclusion. Which result would
actually justify facets, and which instead calls for class, continuity, timing, or extent work?

### Q4. What is the smallest facet experiment that could change our decision?

Reconcile E2 and E7 with the original facet hypothesis. Is the next useful test geom0016,
single-ring/patch repeatability, or registration of a directly relevant front/rear endpoint?
Choose one. Define a same-observation comparison against the current provisional box path and
the best appropriate simpler registration comparator. Control motion priors, membership,
support, initialisation, compute budget, and use of prior information.

Require observed geometry to constrain only observable directions: distinguish an actual
edge from a scan-ring intersection or occlusion boundary. Show how the chosen line, plane,
or compact patch affects a bumper bound, yaw, or relative speed. State how to handle the
segment yaw's pi ambiguity and correlated prior-derived proposals using the existing adapter.
Choose a small number of tuning episodes, a separate evaluation partition, a predeclared
benefit and no-regression rule, and a decision that remains useful if facets fail. Defer the
full constellation graph unless this test earns it.

### Q5. Is there any state-estimation change worth implementing this weekend?

Given E5–E8, distinguish a remaining physical-estimation defect from calibration of reported
uncertainty. Which falsifiable change is smaller and more useful: a guarded birth model,
reflection/duplicate diagnosis, better endpoint covariance, or a face/extent update? Identify
the exact failure the existing default-off variants did not fix. Keep the baseline frozen
unless a new candidate passes its own rule. Specify what evidence would reopen CA/IMM;
apparently smooth trails or lower residuals alone are insufficient. Include stopped queues,
turns, braking, occlusion/coasting, reacquisition, and fast spurious tracks in the relevant tests.

### Q6. Which sites and labels buy the most information in two operator-hours?

Use kirk0 for diagnosis and columbus-broadway for changing motion/visibility where supported.
Check the [following-opportunity ranking][opportunity] before choosing a field logging pilot:
the October report favoured Embarcadero Broadway/Bryant/Bay for vehicle pair-seconds, but its
selector used track centres and is not a count of qualified bumper-gap encounters.

Return a site matrix: existing masks, supported physical endpoints, leader decisions,
independent speed evidence, site pose/path geometry, known exclusions, and current split role.
Allocate operator minutes to the references that decide Q3/Q4, including at least a difficult
negative case, rather than requesting another general masking campaign. Masks alone do not
establish an unseen bumper or true body speed. Preserve family-specific evaluation history:
embarcadero-folsom has already been read for some geometry, so its old blanket held-out label
must not be reused. Identify genuinely unused evaluation episodes before scoring.

### Q7. What is the minimum durable per-site logging contract?

Trace the change from `fieldrun.Run` and `AnalyseFollowing` through interaction validation,
SQLite storage, registry/surface audit, and export/API consumers. Propose one coherent
extension, with the smallest set of schema/method versions and migrations actually needed.
Which source fields already retain gap, both speeds, uncertainty, support, and geometry,
and which values need adding so DRAC can be recomputed exactly? Where must path geometry
be persisted rather than referenced only by an opaque ID?

Specify the durable relationship among site, sensor/placement, run/capture, approach/movement,
coordinate transform, analysis stage, build SHA, parameter/config digest, and quality policy.
Prevent a replay or two simultaneously recorded sites from being associated merely because
their capture times overlap. Separate recording mode from analysis mode. Preserve exact
values before aggregation, name unit/definition versions, and avoid silently merging stages
or estimator versions. State which work can ship this weekend and what remains an offline
prototype. Do not invent a parallel metric registry.

### Q8. How do we count encounters and opportunity without counting fragments twice?

Define encounter identity, leader changes, brief gaps, reacquisition, censoring at a site or
window boundary, and repeated replay of the same physical evidence. Distinguish physical-pair
identity from tracker IDs without merging vehicles on proximity alone. Give a conservative
fallback when identity cannot be resolved.

Specify unique follower-opportunity time separately from overlapping pair-seconds, and expose
attempted, eligible, valid, suppressed, predicted-only, and unobserved time or counts by reason.
How are intervals integrated at irregular effective rates, and when may interpolation bridge
a gap? Which extrema are only observed-window minima/maxima? How should online provisional
events be superseded or coexist with final reprocessing under the existing write-once model?

### Q9. Which distributions should a site log first?

Preserve `following_distribution_v1` and its time weighting. Choose a separate experimental
DRAC series and, if justified, one maximum per eligible encounter with coverage and censoring.
Reconcile any encounter-weighted following proposal with the existing B10 deferral and
l8fol0004's prerequisite; do not silently reopen a decided dependency.

Specify denominators and minimum evidence for reporting a distribution, not just selected
conflict severity. Include fixed versioned bins, overflow, exact underlying values, empty
strata, and suppression. Explain how to obtain combined quantiles from underlying eligible
units, rather than averaging site/window percentiles. Keep canonical speed p50/p85/p98 over
per-passage maximum speeds. Distinguish physical uncertainty from session/day sampling
uncertainty. Which comparisons are honest this weekend with only one session per site?

### Q10. What executable tests prevent attractive but wrong DRAC logs?

Specify meaningful analytic fixtures and persistence/replay tests for equal-speed following,
a closing pair with a known gap and relative speed, a diverging pair, a stopped queue,
uncertain/overlapping bumpers, a turn, a crossing pair, a cut-in/leader switch, duplicate tracks,
an occlusion, irregular timestamps, and a window truncated around the true extremum.
Include same-version idempotence, changed-content conflict, version separation, two sites
with overlapping capture times, and recomputation of aggregates from exact records.

For the selected field episodes, require endpoint/gap/relative-speed error, wrong-leader rate,
uncertainty coverage, metric error, eligible encounter yield, exclusions, and runtime/memory
cost. Use paired episode or encounter comparisons and intervals, not independent-frame sample
sizes. Fix the evaluation rule before running; keep a failing arm's missing coverage in view.

### Q11. What should we deliberately defer, and what would reopen it?

Give explicit reopening evidence for full facet constellations, a 2D/3D accumulated body map,
CA/IMM, acceleration/jerk and braking maps, duration-aware behaviour states, and crossing PET.
Vehicle make/model and meshing have no assumed dependency on this weekend's metric logger.
Keep safety association and causal claims separate from physical measurement qualification.
State any existing decision that the proposed weekend order would change, with the reason,
rather than presenting it as already agreed.

### Q12. Turn your answers into a bounded work order

Return a Friday preflight, Saturday implementation/diagnosis, overnight replay, and Sunday
evaluation schedule. Each work item needs: owner role, engineer/operator/compute time,
dependencies, exact code or document surfaces, output, acceptance/failure condition, and
fallback. Mark critical-path work separately from useful parallel tasks.

Select at most three experiments and give each a hypothesis, checked-in arm configuration,
baseline and comparator, source/evidence manifest, partition, primary metric, frozen rule,
runtime estimate, and disposition on pass/fail/insufficient evidence. Parameters belong in
`config/experiments/`; implementation switches also need exact build/experiment identity.
Use the repository's existing harness and Makefile build/version stamping. Keep raw outputs
local; commit only implementation, hand-authored configuration, and edited reports. Include
`make check-no-results` and inspection of the staged diff before any later commit or push.

Conclude with two lists: what we can review or merge by Sunday, and what evidence still blocks
qualified field publication. If a dependency exceeds the budget, cut scope and retain a working
per-site diagnostic logger. Do not replace a failed experiment with an unbounded parameter sweep.

## Preparation provenance and limits

Prepared from the linked chat's four user turns and their final responses, live PR #1017
metadata/discussion (no comments returned), and repository sources through
`origin/main` at `e9ff0a16a614cc990bd5bfcbcbe3da7087537eef`. The local checkout was `main`
at `b8ea18d4f1018b331335d9e95b8c5e8249c2e106`, 16 commits behind that ref, with an existing
edit to `.vscode/settings.json`. Newer reports were read directly from the Git snapshot;
the inspected following, interaction-store, fieldrun, scene-API, and facet-adapter files
had no diff between local HEAD and that snapshot. No checkout, pull, source-code change,
database inspection, physical-label audit, replay, or experiment was performed for this brief.
The preparation did not resurvey the original research papers or all open PRs. The oracle
must refresh source drift, available local evidence, and the split ledger before execution.

[facet-plan]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/plans/lidar-facet-registration-experiment-plan.md
[f0]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/facet-f0-attribution-kirk0-report.md
[geom0016]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/data/experiments/geom/geom0016.md
[box-following]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/following-box-endpoints-2026-10.md
[class-following]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/following-class-release-corpus-2026-10.md
[gest1]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/gest1-second-segments-2026-11.md
[prior]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/velocity-prior-shadow-2026-11.md
[profile]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/extent-occupancy-profile-2026-11.md
[anchored-profile]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/extent-profile-anchored-2026-11.md
[identity]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/likelihood-birth-gate-placements-2026-11.md
[distribution-research]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/research/2026-11-12-driving-behaviour-distributions.md
[following-limits]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/data/experiments/l8fol/l8fol0004.md
[labelling]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/plans/lidar-one-hour-labelling-plan.md
[fieldrun]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/internal/report/headway/fieldrun/fieldrun.go
[interaction-store]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/internal/lidar/storage/sqlite/interaction_store.go
[scene-api]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/internal/api/server_scenes_headway.go
[metric-table]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/internal/lidar/l8behaviour/metrics.go
[following-code]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/internal/lidar/l8behaviour/following.go
[segment-code]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/internal/lidar/annotation/features_segment.go
[facet-code]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/internal/lidar/annotation/facet_measurement.go
[opportunity]: https://github.com/banshee-data/velocity.report/blob/e9ff0a16a614cc990bd5bfcbcbe3da7087537eef/docs/lidar/operations/following-opportunity-corpus-2026-10.md
