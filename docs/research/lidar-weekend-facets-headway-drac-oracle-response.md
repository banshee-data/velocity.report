# Oracle response: per-site following and DRAC logging this weekend

Build a reproducible offline site logger first. Keep the current tracker as the control, add
an explicitly versioned experimental DRAC calculation, and spend the remaining research time
on the existing face-placement versus extent-shortfall attribution. The next facet investment
should target a supported physical endpoint and its uncertainty. A full constellation tracker
is not the dependency that this weekend needs to resolve.

- **Status:** GPT-6 Astra's answers and proposed work order; implementation and experiments
  have not been executed. Recommendations are not recorded user decisions or promotions.
- **Scope:** Answers Q1–Q12 of the [oracle brief](lidar-weekend-facets-headway-drac-oracle-brief.md),
  including the earlier facet discussion and PR #1017.
- **Budget:** 16 engineer-hours, two additional operator-hours, and overnight compute, as
  assumed by the brief; an eight-hour fallback appears under Q12.
- **Execution plan:** The owner's later Lombard review adjustment and v0.5.2 scheduling are
  consolidated in the [weekend plan](../plans/lidar-weekend-following-drac-plan.md), which
  governs site selection and the shared review/geometry allowance. The recommendations below
  preserve the original oracle reasoning.
- **Baseline:** Live GitHub `main` verified at `e9ff0a16a614cc990bd5bfcbcbe3da7087537eef`.
  Every historical measurement below retains the qualification in its source.

## Evidence

The brief's E1–E10 provide the historical evidence. Five findings from the code inspection
make the proposed implementation smaller and more precise:

1. [FollowingPoint](../../internal/lidar/l8behaviour/following.go) already contains projected
   leader and follower speeds, speed sigmas, and physical endpoints. The persisted
   [InteractionInstant](../../internal/lidar/l8behaviour/interaction.go) retains endpoints,
   support, gap/time-gap values, and reasons, but **does not retain those speeds**. An export
   cannot recover closing speed from existing interaction rows alone.
2. [FollowingAnalysis](../../internal/lidar/l8behaviour/encounter.go) already contains paths
   and every follower's pairing timeline. The three interaction tables omit the full paths
   and timelines. Free-flow and path-refused follower instants may therefore be absent from
   an interaction-only denominator.
3. [InteractionEventID](../../internal/lidar/l8behaviour/interaction.go) identifies a pair
   and analysis version, without an episode start. The analyser groups a pair's instants
   across the capture. Existing events must not be relabelled as contiguous physical episodes.
4. [Leader-truth scoring](../../internal/lidar/perframeeval/following_leader.go),
   [following acceptance](../../internal/lidar/perframeeval/following_acceptance.go), and the
   [criteria loader](../../internal/lidar/perframeeval/following_criteria.go) exist. So does
   [near-face attribution schema v2](../../internal/lidar/perframeeval/neareface.go).
   Their older experiment specifications contain implementation steps that are now partly
   complete. The checked-in following-criteria file is still an
   [unpinned draft](../../config/criteria/following-limits.draft.json). Its valid-following
   definition is recorded and validated by the scorer, but is explicitly not yet applied to
   scored instants. Wiring and agreement with runtime semantics need checking.
5. There are two site namespaces: integer `site.id` behind
   [Scene.SiteID](../../internal/db/scene.go), and LiDAR clip/site identity used by
   [AttachClipSites](../../internal/lidar/storage/sqlite/run_listing.go).
   [Runs](../../internal/lidar/storage/sqlite/run.go) already carry clip, sensor, build,
   parameter, and execution provenance. A bare `site_id` string is an inadequate new join.

The configured PCAP root in `local.mk`, `/Volumes/lidar/lidar/pcaps`, was unavailable during
this response. Local capture/label databases were not audited. Historical reports establish
that evidence existed; they do not prove it is accessible or unchanged today.

## Q1. The Sunday deliverable and priority order

**Deliver a CLI-generated, review-only site bundle from one explicitly selected estimate
version.** It contains existing following output, exact projected kinematics for experimental
DRAC, site/source provenance, paths, and a complete follower-opportunity ledger. Re-running
the same inputs reproduces the analytical content. A small tabular summary makes the bundle
reviewable; a new dashboard is optional follow-through.

There are two distinct exits:

| Exit                 | Acceptance                                                                                                                                                                                                                         | Main obstacle                                                                                            |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Engineering          | Analytic fixtures pass; site/source binding is explicit; repeat analysis is idempotent; every omitted value has a reason; aggregate bins and durations reproduce from exact rows; source/stage/version selection is unambiguous    | Capturing the evidence lost between the in-memory analysis and persistence, and validating the site join |
| Physical measurement | Independent pair identity, relevant endpoints, relative speed and timing support the declared error/coverage limits; qualified encounter yield and missing opportunity meet pinned criteria on an appropriate evaluation partition | References and calibrated uncertainty, especially the unseen endpoint and relative speed                 |

The first exit is a reasonable weekend target. The second cannot be promised from two slow
kirk0 pairs with historical gap-reference half-widths of 1.5–2.0 m and no verified independent
relative-speed reference. The [physical-reference pilot](../lidar/operations/physical-reference-pilot-kirk0-2026-10.md)
documents those limitations. Reporting a small error against such broad references does not
establish decimetre accuracy.

Prioritise: **(1)** L8 logging and exclusion attribution, **(2)** targeted reference/leader/class
review, **(3)** endpoint uncertainty and geometry attribution, **(4)** tracker initialisation.
This ordering is an engineering judgement, not a measured productivity ratio. The logger
turns all subsequent work into comparable site measurements. Class evidence is the largest
historically recorded exclusion, but automatically relabelling `dynamic` is not a solution.

Stop expanding the field campaign after three cases: kirk0, columbus-broadway, and one
preselected Embarcadero pilot. If they yield fewer than five distinct reviewed closing pairs,
complete the logger and report insufficient physical evidence. Five is a **pilot triage
budget**, not a qualification sample size. Do not consume additional evaluation windows to
make the demo look successful.

## Q2. DRAC can use CV velocities

**Yes.** A motion estimator supplies the current state; DRAC applies a hypothetical future
motion model to that state. Constant-acceleration estimation is not required to evaluate the
constant-leader-speed, immediate-braking model already specified in
[behaviour analytics §8.5](../plans/lidar-behaviour-analytics-plan.md#85-drac-and-required-braking).

Let `g` be the positive bumper gap along a common directed path, `vF` the follower speed,
`vL` the leader speed, and `c = vF − vL > 0`. If the follower decelerates at magnitude `b`
while the leader maintains speed, the gap is `g(t) = g − c·t + b·t²/2`. It is smallest at
`t* = c/b`. Requiring that minimum to be non-negative gives:

- Net following time gap: `H = g/vF`, seconds, where follower speed supports division.
- Required deceleration: `D = c²/(2g)`, m/s², the limiting value with zero residual clearance.
- Time to equal speed under that limiting braking: `t* = 2g/c`.

This derivation fixes the factor of two and the meaning of the metric. It assumes a supported
same-path interaction, constant leader velocity, uniform immediate follower deceleration,
and no reaction delay. A desired residual clearance or reaction time defines a different
version/formula. Actual braking, tyre capability, and crash probability are separate quantities.
Fazekas et al. discuss the constant-leader-speed and reaction-time limitations in their
[primary study](https://onlinelibrary.wiley.com/doi/10.1155/2017/8376572). This response uses
the explicit derivation above, not thresholds transferred from that study.

| Situation                                                                              | Proposed experimental DRAC treatment                                                                                                                               |
| -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Supported same-path pair, gap lower bound positive, closing-speed lower bound positive | Calculate D; retain value, inputs, uncertainty and qualification status                                                                                            |
| Clearly non-closing, including exactly equal speeds                                    | No value under this closing-only definition; record `not_closing` applicability and its observed time. Do not erase these pairs from the parent opportunity ledger |
| Closing-speed uncertainty crosses zero                                                 | Record `closing_direction_unresolved`; retain an interval/bound if defensible, without placing a point value in the eligible histogram                             |
| Gap interval touches/crosses zero                                                      | Suppress finite D with `gap_not_resolved_positive`; never clamp the denominator to manufacture a large finite value                                                |
| Overlap, unresolved identity, or possibly the same vehicle twice                       | Geometry/identity review; no collision assertion and no DRAC                                                                                                       |
| Low follower speed or stopped queue                                                    | Spatial gap remains useful. Do not reuse the time-gap speed floor as a DRAC gate. A supported slow closing pair can have D; a stationary pair is non-closing       |
| Crossing, opposing, or unresolved merging motion                                       | Inapplicable to this longitudinal definition. A completed cut-in can start a following episode once path and leader are resolved                                   |
| Predicted/coasted party, unsupported bumper or timing                                  | Review evidence only; no qualified observed DRAC or observed exposure                                                                                              |

These outcome names are proposed semantic categories; implementation must reuse existing
registered reasons where equivalent and register new ones centrally. A zero must never stand
for missing. If a later all-following metric deliberately defines `max(c,0)²/(2g)`, its
non-closing zeros are model results under that **separate definition**, not the observed
absence of braking or risk. For this weekend use the closing-only definition above.

Do not require the existing headway ceiling, a short-headway band, or a minimum speed to select
DRAC candidates. Compute from geometrically credible following candidates, then apply its
own support rules. Otherwise a long-gap, rapidly closing pair can disappear before scoring.

## Q3. Attribute error before changing the tracker

There is no defensible current answer that one source dominates DRAC error: DRAC has not been
measured here. The reports identify extent bias, initialisation, class suppression, and identity
failure in different populations. Diagnose them on the **same declared eligible opportunities**.

For differentiable, well-separated positive inputs, first-order propagation gives:

`Var(H) ≈ Var(g)/vF² + g² Var(vF)/vF⁴ − 2g Cov(g,vF)/vF³`.

`Var(D) ≈ c⁴ Var(g)/(4g⁴) + c² Var(c)/g² − c³ Cov(g,c)/g³`.

Also `Var(c) = Var(vF) + Var(vL) − 2 Cov(vF,vL)` and
`Var(g) = Var(sL,rear) + Var(sF,front) − 2 Cov(sL,rear,sF,front)`.
These follow from gradients `(1/vF, −g/vF²)` and `(-c²/(2g²), c/g)`.
The two gradients were checked numerically by centred finite differences in preparing this answer.

A shared translation cancels in the same-frame gap. Heading, path, and synchronisation errors
usually do not cancel completely. A position/extent state can have strong covariance: a well
observed face may be precise even when centre and length separately are not. Project the joint
state through the endpoint Jacobian when available. Do not substitute zero covariance merely
because an adapter omitted it. With only standard deviations, a worst-correlation first-order
bound is possible, but it is not calibrated coverage; label that assumption.

Near a zero denominator or applicability boundary, use bounded propagation or draws from a
validated joint distribution, retaining draws that fail applicability as applicability mass.
Do not discard those draws and report a narrow conditional interval as unconditional confidence.
Unknown covariance or uncalibrated box extent remains an explicit qualification failure.

For hard bounds `g ∈ [gLo,gHi]`, `c ∈ [cLo,cHi]`, with `gLo > 0` and `cLo > 0`, a conservative
rectangular bound is `[cLo²/(2gHi), cHi²/(2gLo)]`. At `g=10 m`, `c=5 m/s`, D is 1.25 m/s².
Hard half-widths of 0.5 m and 0.5 m/s give `[0.964, 1.592] m/s²`. Those hard bounds are not
sigmas. If 0.5 and 0.5 instead denote independent sigmas, the local propagated sigma is
0.258 m/s². The different interpretations must survive serialisation.

Use diagnostic substitutions, keeping the automatic baseline intact:

| Substitution                                                       | What it can isolate                                    | What it cannot establish                                          |
| ------------------------------------------------------------------ | ------------------------------------------------------ | ----------------------------------------------------------------- |
| Reviewed leader/no-leader/ambiguous choice                         | Pairing and duplicate-induced false interactions       | Automatic leader performance after substitution                   |
| Reviewed class on the same bodies                                  | Class exclusion and remaining geometric coverage       | A deployable classifier or evidence that every `dynamic` is a car |
| Reviewed membership with unchanged estimator settings              | Fragmentation/contamination effects on geometry        | Body-origin truth or speed truth                                  |
| Independently supported endpoint bounds, same automatic velocities | Endpoint contribution to metric error                  | Relative-speed accuracy                                           |
| Independent relative-speed bounds, same endpoints                  | Velocity contribution                                  | Endpoint accuracy; unavailable references stay unavailable        |
| Both endpoints and relative speed                                  | Metric/model arithmetic relative to physical reference | An automatic production result                                    |

Inspect single substitutions and the joint geometry/speed substitution: their effects need
not add, because errors are correlated. Report paired absolute changes and changed eligibility,
not a pie chart whose percentages presume independent causes.

For a proposed detectable gain of 0.30 m in endpoint error, seek endpoint reference half-widths
at most 0.10 m; for a 0.60 m/s speed gain, at most 0.20 m/s. These one-third budgets are proposed
experiment-resolution choices, not new product gates. Metric-level reference intervals should
likewise be substantially narrower than the gain being tested. If the available pilot bounds
cannot resolve that gain, report insufficient precision. Do not tighten a reference by fiat.

Facet work earns priority only if supported face placement or registration remains material
after identity, membership and extent effects are exposed. A class-only gain directs work to
class evidence. A moving origin or unseen end directs work to body/extent uncertainty. A young
track or false fast cluster directs work to initialisation or reflection diagnosis.

## Q4. Run geom0016's attribution before building a new facet arm

Choose **geom0016**, using the implemented evaluator. It is the smallest test that can alter
the facet funding decision without building the machinery whose value is still in question.
Its control and face-state arms share input evidence and motion settings. The published box
remains the end-to-end following reference path; it is not silently substituted into an evaluator
that scores the shadow extent state.

Preserve the [existing rule](../../data/experiments/geom/geom0016.md): on its valid held-out
decision set, fewer than 100 end-residual instants or eight objects means insufficient evidence;
mean half-extent shortfall `S > P`, the mean absolute placement residual, means extent dominates;
otherwise `P < 0.10 m` means placement is immaterial; otherwise placement remains in play.
kirk0 is a tuning screen regardless of its result. With two operator-hours, plan for that
screen and evidence inventory, not an automatic promise of eight independent labelled objects.

There is a crucial mathematical limit: the labelled visible span is only a lower bound on
length, conditional on heading. Consequently the calculated shortfall is a lower bound on
extent error, and the remainder still contains unobserved extent, centre and heading error.
`placement_in_play` is permission to investigate registration; it is not proof that facets
explain the remainder. It is also not a physical following-accuracy gate.

The next bounded facet experiment, if this gate supports it, should be **direct endpoint
registration**. Track one independently identified physical end/edge and project its bound
onto the shared path, without requiring a complete body centre/length solution. This tests a
useful possibility exposed by the kirk0 box result: a seen bumper may be useful while the
opposite end remains unknown. A visible body panel is not automatically the bumper extremum;
any offset must itself be supported and uncertain.

For that follow-on use four matched comparators: published box; bounded accumulated-point
registration; fresh per-frame surface/edge fits; the same fits with persistent correspondence.
The last pair tests persistence. Identical memberships, retained returns, initial priors,
timing/deskew rules and compute caps apply. SOTracker is relevant precedent for accumulated
shape, registration and motion priors, not evidence that our roadside sensor will benefit
([Pang et al.](https://arxiv.org/abs/2103.06028)).

Predeclare six tuning episodes and six different evaluation episodes across at least two
placements as a diagnostic pilot, subject to references existing. Include end-on, changing
aspect, an occluded end, side-only support, an adjacent vehicle, and a turn. If six evaluation
episodes cannot be supplied, retain tuning-only status. A proposed continuation rule is at
least 20% reduction in paired median episode endpoint error versus the best simpler arm,
with no reduction in supported opportunities, no new wrong-object matches, and all hard
reference bounds covered. Report all per-episode changes. This small pilot cannot qualify
tail accuracy or uncertainty; a larger preregistered evaluation remains necessary.

A line or plane constrains its normal displacement, not arbitrary tangential motion. A side
plane often leaves longitudinal speed and the bumpers weak; an end face can constrain them.
Single-ring endpoints may be sampling/occlusion boundaries. Preserve that distinction and
abstain when it is unresolved. Reuse the facet adapter's modulo-pi yaw and unresolved-direction
contract. Resolve front/back with independent evidence; do not turn the nearest prior yaw
mode into new evidence. An anchor or map made with the tracked prior requires correlation-aware
treatment or an explicitly conditional diagnostic, not an independent second Kalman update.

## Q5. Improve the measurement contract before another state model

The weekend implementation should preserve and label endpoint/velocity uncertainty, including
its missing components. Do not change the production process noise, birth model, extent state,
or association cost. This is a deliberate use of the newer results: all tested initialisation
variants still failed some combination of false speed, identity or noise checks.

Record track age, time since the last direct observation, reference-point changes, face changes,
and available duplicate/reflection diagnostics beside the metric. Stratify the first second
and reacquisition. These identify whether DRAC spikes coincide with unsupported starts. A blanket
one-second deletion would hide the failure and could discard a real brief encounter, so retain
the candidate and exclusion history even where publication is suppressed.

The next estimator implementation should follow a labelled diagnosis of franklin-mcallister's
false fast tracks, not another broad shadow-refusal rule. Reopen CA/IMM only after a candidate
shows an acceleration-specific physical speed/response error on a stable observed reference,
persists beyond initialisation and face switches, and survives the existing coast, identity,
noise and held-out gates. NIS near one is useful consistency evidence, but cannot establish
physical speed accuracy. Conversely, under-dispersed residuals do not prove CV has no physical
lag. Both the covariance claim and the actual trajectory must be evaluated.

## Q6. Sites and two operator-hours

This matrix distinguishes documented evidence from evidence verified locally. No local capture
or label files were available through the configured PCAP root in this preparation.

| Site                 | Documented masks/physical evidence                                                          | Leader/speed evidence                                                         | Geometry and role                                                     | Weekend action                                                                           |
| -------------------- | ------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- | --------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| kirk0                | Reviewed masks; pilot has 22 poses, three vehicles, two pairs and 11 gaps; broad gap bounds | Two pilot following pairs; no independent relative-speed source verified here | Existing local paths/sensor context; repeatedly used tuning data      | Diagnose, reproduce attribution, inspect seen versus unseen ends; never call it held out |
| columbus-broadway    | Earlier annotation work exists; complete current reference coverage/digests unverified      | Reviewed leader coverage and independent speeds unverified                    | Changing motion/visibility; corpus screen/tuning use                  | Test difficult coverage, turns and young-track exclusions; no qualification promise      |
| embarcadero-bryant   | Capture and historical following output; physical-reference/mask availability unverified    | Historical valid following 47.3 s; no reviewed pair/speed inventory verified  | First segment already screened for following                          | First choice for a useful site logging pilot, selected before new scores                 |
| embarcadero-broadway | Same availability qualification                                                             | Historical valid following 37.9 s                                             | Previously screened first segment                                     | Fallback only for access/metadata failure, not because Bryant scores poorly              |
| embarcadero-folsom   | label0001/0003 target a lot-drawn window; completed frozen masks/spans unverified           | No new independent following/speed set verified                               | Some geometry already inspected; other families have their own ledger | Reserve; verify exact window and metric-family history before any decision read          |

The valid seconds come from the [October class-release report](../lidar/operations/following-class-release-corpus-2026-10.md).
They are a site-selection aid, not current throughput. Recover the run/clip registration and
path evidence for every selected site. A locally fitted path can support an explicitly local
gap, while grade error and unsupported registration may still suppress a physical claim.

Allocate the two operator-hours as follows: 15 minutes to confirm pack provenance, split role,
and identities; 30 minutes to review six short candidate pair clips including one adjacent or
crossing negative; 45 minutes on supported front/rear endpoint keyframes in the two most useful
clips; 15 minutes on class/leader disagreements; 15 minutes to inspect, freeze and document
the result. This is a capped pilot, not a promise to complete all six clips. Stop and record the
shortfall if review takes longer. Do not fit a complete unseen body merely to obtain a number.

Use non-overlapping reviewed episodes and preserve assisted-label provenance. Additional
same-sensor displacement measurements can diagnose speed, but are not independent external
validation. No RTK/IMU campaign is assumed this weekend.

## Q7. Minimum durable logging contract

**Implement an offline bundle first, reusing L8 types and the existing metric registry.**
Keep the existing versioned following interactions intact. Add one diagnostic export envelope
that stores the extra kinematics, paths, follower timelines and analysis context. This avoids
attempting a storage migration, old-record compatibility, new site API, live worker and new
metric in the same 16 hours. It still produces durable per-site logs outside the checkout.

| Record           | Required content and implementation boundary                                                                                                                                                                                                                       |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Analysis context | Explicit namespaced site identity; sensor and placement; run/clip and capture identity; capture-time window; recording source mode; processing mode; estimator/stage/observation model; build SHA; config bytes/digest; metric/quality/uncertainty policy versions |
| Geometry         | Serialised `LocalPathResult`/path vertices and arc lengths, frame/units, transform identity and payload or immutable referenced asset, validity window, grade/planar qualification; digest checked against the event's geometry ID                                 |
| Instant evidence | Both projected speeds, capture/measurement times, speed uncertainty, endpoints and provenance, gap, support, track age, available covariance blocks and explicit missing-correlation status; original estimate row identities                                      |
| Metric result    | Centrally registered review-only DRAC ID, method version fixing the formula, unit, applicability, support/quality outcome, value or absence, interval kind/assumptions and reasons                                                                                 |
| Opportunity      | Every follower decision and its interval, including free flow, no path, ambiguity, and record gaps; unique tracked-follower interval union distinct from pair exposure                                                                                             |
| Identity/episode | Source-qualified track pair, candidate contiguous episode boundaries, censoring, lineage evidence if available, and identity unresolved flag                                                                                                                       |
| Aggregate        | Exact selected source/version/population, eligible time/count, reason accounting, fixed-bin specification, and source row digests                                                                                                                                  |

Register the DRAC quantity in `l8behaviour/metrics.go` and structural/reason tokens in the
existing audit/vocabulary surfaces. Choose a canonical ID there before serialising; this answer
does not reserve an unreviewed spelling. Compute it from `FollowingPoint`'s bodies or shared
projection helpers, with metric-specific eligibility. Do not infer missing speeds from `gap/H`
or from finite differences of stored gaps. Both discard information and can reintroduce
geometry/identity changes as apparent motion.

Use the existing run → clip → LiDAR site relation where it is present. Bind the observation
source to that context using the recorded capture/manifest identity and sensor, verifying the
digest and time window. An explicit reviewed context manifest supplies missing joins for
offline evidence; unknown site metadata stays unknown. An integer scene site and a LiDAR site
token require an explicit mapping, not string equality. Time overlap may validate a binding;
it cannot establish it. Never silently choose among multiple sources or estimate versions.

The new bundle schema and DRAC method are version 1 of their own contracts. Existing
`interaction_record_v1` and `following_distribution_v1` retain their meanings. Write bundles
atomically under content identity: identical analytical content is a no-op; changed content
under the same declared identity is refused. Separate nondeterministic execution timestamps
from the analytical digest. Record exact code identity even when parameter hashes match.

Later database integration can add a context/path/kinematic snapshot store, or an explicitly
decoded interaction v2, through the repository migration and schema-sync workflow. It must
read old v1 rows and keep the new DRAC eligibility separate from `InteractionInstant.Valid`,
which currently means valid following time. The offline record is the schema rehearsal, not a
second metric implementation. A per-site API/live producer belongs in a follow-up once the
contract and sample outputs have been reviewed.

## Q8. Opportunity, episodes and identity

Retain the existing pair-over-capture event. Derive diagnostic episodes from the full follower
timeline: a chosen leader changes, the path becomes ambiguous/inapplicable, or a record/support
gap occurs, so the episode ends. A return to the same leader starts a new observed episode.
The gap remains missing time, with both boundaries marked censored. Use source, selected version,
pair and start time in the derived episode identity.

Do not stitch tracker fragments by proximity. Use reviewed or independently supported lineage
when available. Otherwise call the count **track-pair episodes**, flag uncertain identity, and
withhold claims about unique physical encounters. A deterministic replay is the same exposure;
different estimator versions are alternative readings of it, not extra samples. Overlapping
clips require a shared parent-capture/time mapping before their exposures can be combined.

For each follower, union its candidate intervals before counting unique tracked-follower
opportunity. Ambiguous leader candidates must not multiply that time. Keep joint pair-time as a
separate diagnostic. This only deduplicates within known track identity; it does not magically
deduplicate fragmented vehicles.

Use half-open sample-and-hold intervals, clipped at the next follower timestamp and analysis
window end. The final sample has no implied duration. Keep the current 150 ms missing-row
bound for the initial 10 Hz baseline; gaps above it count as record gaps. An effective rate
near 5 Hz caused by missed associations does not justify inventing continuous observation.
A genuinely different acquisition cadence needs a separately pinned support policy. No
interpolation across missing observations for the weekend's eligible exposure. Isolated valid
instants may retain point estimates while contributing zero duration.

Partition ledger time once into observed-applicable, observed-inapplicable, observed-unresolved,
predicted-only and record-gap categories. Within applicable time, partition qualified and
quality-suppressed. Keep all contributing reason flags, but use one precedence reason for
additive duration totals. Physical time before first/after last observation is boundary-censored,
not estimated from track duration. Broader sensor-unavailable time requires the capture ledger.

Online and final processing remain separate immutable analysis versions. An explicitly selected
final version can be the preferred view without deleting the online record. An episode maximum
is the **observed-window maximum**, with effective rate and missingness; without continuity and
bandwidth evidence it is not the continuous-time maximum.

## Q9. First distributions and honest comparisons

Keep shipping following histograms: 0.25 s net-time-gap bins and 2 m spatial-gap bins under
`following_distribution_v1`. Add a review-only, duration-weighted DRAC histogram over
applicable observed instants. Proposed exploratory edges are 0, 0.5, 1, 2, 4 and 8 m/s²,
with overflow at 8; all bins are half-open. These are coarse engineering bins, not risk
thresholds. Retain exact values so the bins can change under a new specification.

Retain an observed maximum per candidate episode for inspection, with coverage/censoring.
Do not publish an encounter-weighted physical distribution until identity and episode rules
are qualified. Do not reopen B10's encounter-weighted following view or l8fol0004's gate
implicitly. Public speed p50/p85/p98 remains over per-passage maxima.

Export diagnostic bins even for a single eligible instant, clearly giving count/time and
qualification status. Do not turn that into a site population inference. For a weekend
comparison table, a proposed triage minimum is 20 independently reviewed applicable encounters
per compared stratum; below it show individual records and exclusions, without a comparative
ranking or tail percentile. Twenty is not a statistical guarantee: a publishable sample size
must be chosen for the estimand, precision and session clustering. With no such independent
encounters verified here, expect the descriptive branch.

Merge compatible exact duration weights for time quantiles; merge eligible encounter records
for encounter quantiles. Never average percentiles. An empty histogram carries zero eligible
time/count and a reason, not an observed mean of zero. Show closing, non-closing, unresolved
and suppressed exposure separately so selection into the DRAC denominator is visible.

One session per site supports descriptive comparisons of those recordings only. Report
movement, speed range, grade/planar assumption, visibility, class mixture and observation
coverage beside them. Episode resampling cannot manufacture independent days. Physical error
intervals, within-session sampling variation and across-site generalisation are different
uncertainties. No street-safety ranking follows from this weekend's distributions.

## Q10. Tests and evaluation

| Test                                                | Required result                                                                                                      |
| --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `g=10 m`, `vF=15`, `vL=10 m/s`                      | H = 2/3 s, D = 1.25 m/s²; limiting braking reaches equal speed after 4 s; rejects the factor-of-two error            |
| Equal/diverging speeds                              | No closing-only D; explicit non-closing time remains in parent accounting                                            |
| Same closing speed, different common speed          | Same D at the same gap; H changes appropriately                                                                      |
| Stopped/slow pair                                   | Gap retained; no division by zero; supported slow closing is not rejected solely by headway's speed policy           |
| Gap bound straddles zero; unknown extent/covariance | Suppression or unresolved qualification, not clamping or an invented tight interval                                  |
| Shared translation and shared speed error           | Gap/closing-speed covariance cancellation reproduced when explicitly represented                                     |
| Side-only line or occluded end                      | Unobservable endpoint component remains unknown; no false bumper from a patch boundary                               |
| Crossing, turn, cut-in, duplicate and leader switch | Wrong interaction suppressed; supported same-path turn uses common path geometry; episodes split without double time |
| Occlusion/reacquisition and irregular timestamps    | No fabricated observed duration; long gaps counted; final sample has zero duration                                   |
| Window clipping around the extremum                 | Recomputed observed-window maximum and censoring, no out-of-window contribution                                      |
| Same source replay/version conflict                 | Identical analytical digest/no duplicate exposure; conflicting content refused; new version remains separate         |
| Two sites with overlapping times                    | Correct explicit source binding; missing binding refused                                                             |
| Aggregate reconstruction                            | Exact sum of durations/bin weights and reason partition; eligible + excluded accounting reconciles                   |
| Existing v1 fixture                                 | Existing following output and read compatibility preserved                                                           |

Test covariance propagation against numerical Jacobians and a few correlated analytic cases.
Use synthetic varying visibility with a stationary physical body to expose false relative
motion. DRAC never obtains qualification merely by passing synthetic tests.

Field reporting must include every episode, matched and unmatched, endpoint/gap error,
relative-speed error where independently scoreable, leader correctness, supported-opportunity
yield, interval/bound coverage, exclusions, and processing cost. Score uncertainty only when
its interpretation is actually supported. Use paired episode summaries and whole-episode
resampling, recognising shared vehicles/sessions. Six pilot episodes cannot establish p95/p99
performance. Preserve the existing geom0016 rule rather than tuning it to these results.

## Q11. Deferrals and reopening evidence

| Work                                               | Reopen when                                                                                                                                                         |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Full persistent facet constellation                | geom0016 supports placement work, and matched persistent features improve physical endpoints/coverage beyond fresh features and point registration                  |
| Accumulated 2D/3D body geometry                    | An anchor/timing/visibility-aware representation passes extent and unsupported-end checks without growth from pose drift; both geom0019/0020 failures are explained |
| CA/IMM                                             | Independent physical manoeuvre error remains after birth/measurement defects and the model passes coast, identity, calibration and held-out gates                   |
| Acceleration, jerk, braking-location distributions | Appropriate derivative bandwidth, response delay and independent manoeuvre references are established                                                               |
| HSMM/behaviour states                              | Qualified physical episodes and independent behaviour labels support improvement over an interpretable baseline                                                     |
| Crossing PET                                       | Shared-region geometry and complete observed occupancy intervals support a dedicated crossing protocol                                                              |
| Make/model and meshing                             | A separate product question and qualified geometry justify their cost                                                                                               |
| Safety association/causal conclusions              | Independent outcomes and a suitable observational/intervention design exist                                                                                         |

This recommendation keeps production DRAC deferred and adds a **review-only offline
prototype** now. It does not promote a failed tracker arm, amend geom0016, pin following limits,
or change B10. Bringing DRAC into a public API would be a subsequent scope/contract decision.
The user has asked for this plan; no assumption is made that every recommendation is adopted.

## Q12. Bounded work order

Every duration below is a planning allowance, not measured performance. Friday's hour counts
inside the 16-hour total. Operator work runs beside implementation after the evidence manifest
is frozen. CP means the Sunday logger depends on the item.

| Window               | Work, role and cost                                         | Dependency and surfaces                                                                                      | Output/acceptance                                                                                                  | Failure fallback                                                                                        |
| -------------------- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| Friday, CP           | Engineer, 1 h: freeze source and preflight evidence         | Makefile build stamp; corpus/run/clip manifests; frozen pack and split hashes                                | Build SHA/configs and three site bindings verified; evidence readable                                              | If mount/evidence unavailable, select an existing accessible evidence DB; otherwise synthetic exit only |
| Saturday, CP         | Engineer, 1 h: pin prototype contract and experiment spec   | Q2/Q7/Q8; existing registry and `config/experiments/README.md`                                               | Review-only semantics, exact fields, eligibility and bin policy recorded before scoring                            | Reduce to one site and exact rows; retain provenance/quality fields                                     |
| Saturday, CP         | Engineer, 2 h: DRAC evaluator and analytic tests            | `l8behaviour/following.go`, `metrics.go`, `surface.go`, vocabulary and tests                                 | Formula, applicability, covariance/bound semantics verified                                                        | No histogram until primitive/eligibility tests pass                                                     |
| Saturday, CP         | Engineer, 4 h: explicit-context offline export              | `internal/cmd/server/headway.go`, `internal/report/headway/fieldrun`, L8 paths/timelines; reuse SQLite reads | One atomic bundle with exact inputs, projection evidence, following and DRAC outputs; successful same-input replay | Cut optional charts and DB/API integration; do not cut source identity or missingness                   |
| Saturday evening, CP | Engineer, 1 h: repeat-run and site selection check          | Engineering slice above; existing replay harness                                                             | Start E-W1; only required selected captures replayed, twice where determinism is required                          | Abort on source/version ambiguity; preserve diagnostic failure report                                   |
| Overnight            | Existing worker, estimated 0.5–2 h for short selected cases | Capture availability, Makefile-built tools, enough disk                                                      | Complete replay evidence and manifests                                                                             | Wall-clock cap 4 h; retain completed cases and mark missing cases, no whole-corpus expansion            |
| Sunday               | Engineer, 2 h: E-W2 attribution screen                      | Existing near-face evaluator, two frozen configurations and reviewed masks                                   | Tuning P/S figures, observability strata, repeat verification and explicit gate status                             | Missing/weak references yield insufficient evidence; complete source/label inventory                    |
| Sunday, CP           | Engineer, 2 h: E-W1 interpretation and operator findings    | Exact logged rows, Q6 review outputs                                                                         | Per-site exclusion ledger and paired diagnostic cases; separate logging success from measurement qualification     | Small yields remain individual records, no population claims                                            |
| Sunday, CP           | Engineer, 3 h: checks, edited report and reviewable changes | Affected Go tests/build; doc checks; raw-output guard                                                        | Reproducible demo, evidence-linked report, honest residual blockers                                                | Defer optional schema/API work; leave failing slice unpromoted                                          |

The operator allocation is the two-hour schedule in Q6. With a second engineer, give them
the exporter/context work after the one-hour contract is fixed, while the first owns arithmetic,
tests and attribution. An optional endpoint experiment still waits on evidence; spare capacity
is not evidence of a successful gate.

### Experiment E-W1: current-baseline metric coverage and reproducible site logging

- **Hypothesis:** The current box-based path can produce replayable per-site following and
  experimental DRAC candidate records with complete source/quality accounting; its exclusions
  identify the next useful intervention.
- **Arms:** Current defaults with published boxes is the automatic control. Reviewed-class and
  reviewed-leader substitutions are explicitly diagnostic overlays on that identical evidence,
  where labels exist. They are not candidates for deployment and do not alter source DB rows.
- **Configuration:** Before execution, copy current defaults to the proposed
  `config/experiments/weekend-following-baseline.json`; name zero changed tuning keys in its
  README row. Give each diagnostic arm its own versioned overlay definition/digest. Keep
  replay flags out of ad hoc commands. Copy/fingerprint the actual baseline at execution;
  do not reuse a historical hash blindly.
- **Partition:** kirk0 and the preselected first segments at columbus-broadway and
  embarcadero-bryant, all screening/tuning. No held-out claim. Substitute Broadway only for
  inaccessible evidence before scores are read.
- **Manifest:** Exact code/build, tuning bytes/digest, capture/evidence source and sensor,
  site namespace/placement, label/overlay revision, geometry, analysis stage and policy versions.
- **Primary output:** Per-site observed eligible candidate seconds and reviewed pair count,
  with all losses/reasons and identity qualifications. These are diagnostic yields, not proof
  that DRAC is physically accurate.
- **Frozen engineering rule:** Zero unexplained source/version joins; repeatable analytical
  rows; no predicted time counted as observed; exact duration reconciliation; same-version
  duplicates/conflicts handled correctly. Any violation fails the engineering slice.
- **Decision:** Pass delivers the logger even if all physical values remain suppressed.
  Fewer than five reviewed closing pairs invokes Q1's small-pilot branch. Diagnostic gains
  direct the next experiment; they never promote a label-relaxed classifier.
- **Compute allowance:** Up to three selected captures, repeat baseline only as needed;
  reuse matching persisted evidence before replaying. Estimate 30–90 worker-minutes,
  including margin; verify after the first case.

### Experiment E-W2: the existing geom0016 tuning screen

- **Hypothesis:** The extent-state arm reduces the observed extent-shortfall term relative
  to the original shadow control; remaining placement determines whether a later endpoint
  registration experiment is worth preparing.
- **Arms:** Preserve geom0016's exact control and face-state implementation switches. Use
  proposed `config/experiments/weekend-facet-attribution-control.json` and
  `weekend-facet-attribution-state.json`, copied from defaults, with identical tuning where
  switches remain replay-only. Persist each switch list and build SHA in the experiment spec
  and manifest; equal config fingerprints are expected and insufficient to identify arms.
- **Partition:** kirk0 reviewed masks/frozen tuning split. A separate evaluation set is
  reserved, not consumed, until its family history, masks and geom0016 counts satisfy the
  existing decision rule. No new held-out set is asserted to be ready.
- **Primary output:** Per-object and pooled P, S and raw residuals, with visible-end versus
  unseen-end support, heading limitations, match failures and repeat checks.
- **Frozen rule:** Existing geom0016 rule, unchanged. Tuning alone produces a screen and
  leaves the decision insufficient. In a later valid decision read, extent dominance routes
  to endpoint/extent bounds; immaterial placement stops facet investment; placement in play
  permits the matched endpoint experiment described in Q4.
- **Falsifier:** No reduction of shortfall on the matched tuning opportunities falsifies the
  directional screen hypothesis. It does not justify a parameter sweep or override the
  held-out rule. A non-repeating arm has its figures withheld.
- **Compute allowance:** Approximately 15–30 worker-minutes for two arms with repeats and
  existing evaluators, subject to actual capture/tool throughput. Two engineer-hours assume
  existing masks and tools work; otherwise spend them documenting the evidence blocker.

These are the only two selected experiments. The configuration paths above are proposed work
items, not files created or fingerprinted by this answer. The matched endpoint registration
pilot is the next experiment if the gate earns it; fitting all four comparators into this
weekend would make the schedule implausible.

### Eight-hour fallback

Spend 1 h on preflight/contract, 2 h on evaluator/tests, 3 h on a one-site exact export, and
2 h on repeatability, interpretation and a short report. Omit E-W2 execution and all new
DB/API/UI integration. Reuse accessible persisted evidence; if unavailable, deliver the
synthetic end-to-end example and a specific input manifest for the field run. The two operator
hours can still prepare references for the next session. This is a useful partial engineering
exit, not completed physical per-site deployment.

### What is reviewable by Sunday

The DRAC primitive and applicability tests; review-only registry/export additions; explicit
site/version binding and exact evidence logs; a reproducible synthetic example; E-W1's field
coverage report if evidence was available; E-W2's tuning attribution or a precise insufficient-
evidence report. Each behavioural code change carries its affected tests in the same commit.
Run relevant Go suites and the stamped Makefile build, required repository quality gates,
documentation checks, `make check-no-results`, and inspect the staged diff before any later
commit or push. Runtime logs, row data, evidence databases and generated bundles stay local.

### What still blocks qualified field publication

Pinned and implemented metric-level limits; sufficiently precise independent endpoint and
relative-speed references; honest endpoint/cross-covariance treatment; supported physical
pair identity; adequate independent encounters/sessions; and an uncontaminated evaluation
partition. Live site production and a stable per-site API are subsequent integration work.
No successful exporter, smoother trail or synthetic fixture alone closes these gates.

## Open questions

Execution needs facts, not another broad research review: where the current evidence databases
and frozen packs are accessible; which exact site/clip/observation-source mapping they carry;
whether new independently reviewed pair/speed references exist; and which unused evaluation
episodes remain for this measurement family. Friday's preflight owns these checks. The plan
above already specifies what to do if any answer is missing.

## Provenance and validation of this response

This response read the brief and conversation, verified live main against its preparation
snapshot, inspected the cited source contracts and criteria, and read the published maths/
experiment records. It checked the configured capture root's availability and numerically
verified the local uncertainty gradients and worked example. It consulted the two linked
primary research sources for their stated modelling/comparator scope. It did not inspect
remote raw evidence, run LiDAR replays, measure a new field outcome, or implement the proposed
logger. The response and brief are documentation changes only.
