# Weekend following and DRAC evidence plan

Produce reproducible per-site candidate, following, experimental DRAC and exclusion evidence,
then review a few multi-vehicle periods to decide the next geometry investment. The working
programme is November 14–15, 2026. Lombard at Broderick is the provisional intensive-review
site, subject to capture, lane, reference and cohort preflight and the matched manual screen
against Embarcadero at Broadway's published scene. Both sites are now assigned to validation;
the cohort restrictions below take precedence over their earlier provisional development use.

- **Status:** Planned for v0.5.2 at the owner's request; implementation, experiment approval,
  frozen arm configurations and field qualification remain pending.
- **Scope:** A bounded offline logger, existing annotation/review capabilities, and a conditional
  geometry-attribution screen. This PR records the plan; receiving implementation agents choose
  the design and do the work in separate changes.
- **Canonical:** [LiDAR pipeline reference](../lidar/architecture/lidar-pipeline-reference.md).
- **Related:** [Behaviour analytics](lidar-behaviour-analytics-plan.md),
  [physical-reference workflow](lidar-physical-reference-review-plan.md),
  [oracle questions](../research/lidar-weekend-facets-headway-drac-oracle-brief.md),
  [oracle response](../research/lidar-weekend-facets-headway-drac-oracle-response.md),
  [distribution research](../research/2026-11-12-driving-behaviour-distributions.md),
  [facet experiment](lidar-facet-registration-experiment-plan.md),
  [v0.5.2 backlog](../BACKLOG.md#v052---unattended-experiment-harness--following-evidence-052).

## Outcome and boundary

On Sunday, select an explicit site, source and analysis version, reproduce its logs and
accounting, and inspect three or four named periods. Every value must explain its inputs,
support, uncertainty and eligibility. A difficult case with correctly recorded ambiguity is
part of the demonstration. Engineering success and qualified physical measurements are
separate exits; missing references cannot be replaced by apparently plausible annotations.

Keep exact projected speeds and endpoint evidence before aggregation. Basic same-path DRAC
uses supported gap and closing velocity; it does not require a new acceleration estimator.
The oracle response supplies the derivation and proposed applicability treatment. Production
thresholds, physical promotion, CA/IMM and full facet constellations retain their existing gates.

## Lombard review findings

The site index identifies `20260902T0938_80858-0d4_lombard-broderick.pcapng`, recorded September 2,
2026, at LiDAR site `808580d49`. Published scene assets were readable locally and cover about
1,202 seconds, with an accepted manually placed registration. That placement's internal check
is not independent physical validation. The remembered highway entry and turn were not verified.

The scene's vector export contains street and structural geometry, but no explicit travel-lane
features or lane snapshot files. Accepted lanes may exist in Gerardus or a device database;
preflight must inspect them, including snapshot/pose identity, directions and coverage. The
configured external PCAP root and default internal LiDAR data root were unavailable during
review. Replayable packets, current evidence databases and frozen reference packs are unverified.

That access statement records the first review, not the latest state: the volume was reconnected
later on November 13 and both configured data directories were present at the daily reconciliation.
The scene-refresh chat has begun new work. Mounted storage does not establish hash-matched
capture completeness, permissible development use or reviewed references; W1 must verify those.

| Historical reading                                                                            | Population and meaning                                                                                                                                                                               |
| --------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 92 vehicle pair-seconds and 116 vehicle track pairs                                           | [Opportunity screen](../lidar/operations/following-opportunity-corpus-2026-10.md#by-site), under `body-older`: track-centre corridor, speed and heading selector; vehicle size is a geometric screen |
| 0.2 valid / 94.7 accounted seconds                                                            | [Box following run](../lidar/operations/following-box-endpoints-2026-10.md#clause-2-paths-on-the-corpus): its box baseline, physical endpoint reader, pairing and applicability rules                |
| About 62 class-suppressed seconds; 42.1 valid seconds after treating every `dynamic` as a car | [Class counterfactual](../lidar/operations/following-class-release-corpus-2026-10.md#by-site): diagnostic upper bound, not a classifier candidate                                                    |

These readings differ in arm and eligibility. Dividing 0.2 by 92 would not give a qualification
rate. Their contrast makes Lombard useful for diagnosing exclusions; it does not establish
current measurement yield or guarantee suitable physical references.

## Matched published-scene screen

The proposed screen compares Lombard at Broderick and Embarcadero at Broadway before choosing
intensive-review periods. Its original W1/W3 diagnostic use is now on hold for the cohort
decision below: do not execute it as development reconnaissance merely because published files
are accessible. It is not a third experiment or a held-out qualification read. Both published
manifests and timelines were accessible during planning, with approximately 1,202 and 1,209 seconds respectively. The raw PCAP volume was
unavailable then and is now reconnected; published assets remain approximate screening material,
not a substitute for permitted, hash-verified raw evidence and independent references.

Freeze each scene's manifest/export identity, placement, available build/configuration metadata,
display settings and window list before viewing. Missing provenance stays explicit. Use the
same four 30-second half-open windows in each scene, at playback offsets 70–100, 130–160,
190–220 and 250–280 seconds from its published start: two minutes per site. These offsets do
not imply identical source acquisition windows, traffic state, signal phase or time of day.
Check the measurement-family use ledger first; replace a forbidden or unavailable window only
by the next allowed 30-second window at the same fixed one-minute spacing, record the reason,
and never replace a window because its traffic is sparse or difficult.

Use the same playback rate, approximate viewing scale and two complementary camera views.
Allow pauses and seeks to inspect evidence, while counting throughput over the fixed capture
duration rather than operator viewing time. Keep any extra illustrative clip separate from
the matched totals. Record these observations per window and pool only the compatible counts:

| Dimension             | Manual record and comparison                                                                                                                                                                                                                                            |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Traffic throughput    | Count apparent distinct vehicle passages using a documented visible reference line or bounded review region; separate vehicles already present at the boundary, uncertain identities and raw tracker IDs; report count per capture minute, not physical flow validation |
| Visible vehicle sides | For the same apparent bodies, record front, rear, left/right side or unresolved support, with example playback instants; distinguish visible returns from the rendered box's faces                                                                                      |
| Occlusion             | Record clear, partial and unresolved/hidden periods for each reviewed body, including FOV entry/exit separately; label duration estimates as approximate and retain unknowns                                                                                            |
| Track continuity      | Count apparent bodies carried continuously, split among IDs, duplicated, swapped or lost/reacquired; retain uncertain associations and the window-boundary censoring rather than joining by proximity                                                                   |
| Usable gaps           | Count apparent following periods with an identifiable pair, credible shared direction/path, and visible support for follower front and leader rear; record duration and why a period is unsuitable or unresolved; this is reviewability, not a qualified numeric gap    |

Use one window ledger and a few timestamped examples, with the same definitions at both sites.
Do not pool separate periods of one pair into independent physical encounters. Published scenes
may be downsampled, filtered, quantised or produced by different builds; record what their
metadata actually says. Counts, visibility, continuity and gap opportunities are approximate
observations of the exports. They do not establish current tracker performance, raw-return
completeness, true dimensions, relative-speed accuracy or DRAC validity. The historical
opportunity/class-release tables above are contextual evidence, not matched control arms.

Choose intensive periods for clear pair identity, relevant visible ends and a useful negative
case, not throughput alone. Keep Lombard if it provides that evidence; if Broadway is more
reviewable, record why and use it without claiming a general site ranking. If neither supports
numeric gaps, retain qualitative findings and proceed with an accessible logger fixture rather
than expanding the capture search. Reserve at most 45 of the existing 120 operator-minutes for
this matched screen; record incomplete review if the allowance expires, then use the remainder
for the selected periods and provenance. Engineer interpretation stays inside its existing two
hours, and new review tooling still displaces geometry time.

## Related investigations and dependencies

The skewed-box investigation in Codex chat `01a1d775-1d4a-7142-b12a-3c51ac9bde9c` identified
a web-player yaw-sign defect, corrected in merged [PR #1024](https://github.com/banshee-data/velocity.report/pull/1024).
The [renderer report](../lidar/operations/scene-player-yaw.md) distinguishes the corrected
display transform from unchanged stored heading and unvalidated physical yaw. The parallel
[PR #1027](https://github.com/banshee-data/velocity.report/pull/1027) contains overlapping work;
reconcile it against #1024 rather than applying the correction twice. Its investigation and
new asset-refresh work remain independently owned. Record the player module and asset/build
versions used for review; old-player results are not current-renderer evidence.

Euler's [SRAC maths](../research/srac-steering-demand-maths.md) and
[follow-up plan](lidar-srac-research-plan.md) landed in
[PR #1023](https://github.com/banshee-data/velocity.report/pull/1023).
SRAC remains future research and adds no logger field, live solver, experiment arm or acceptance
gate to this weekend. Its predicted and recorded-future 6.35 s modes require 127 future intervals
and 128 states including now; a verified feasible witness, terminal support and continuous-time
clearance evidence are separate requirements, not supplied by sampling alone. Reviewable traffic
episodes may later inform its synthetic cases, but ordinary non-collision recordings do not
validate hypothetical avoidance, driver intent or crash risk.

## November 13 reconciliation and task coverage

This inventory covers the Pacific calendar day November 13, 2026 (08:00 UTC through the next
08:00 UTC), through main `a4bdda9df` and the active-session snapshot taken during this update.
Later messages or PR changes require another refresh. The existing 16-hour logger/review budget
is unchanged. Parallel jobs and later research below are accounted for, not silently added to it.

### Cohort decision before further diagnostic use

Merged [PR #1029](https://github.com/banshee-data/velocity.report/pull/1029) introduced manifest
revision 2: Lombard–Broderick and Embarcadero–Broadway are validation sites, while Kirkham,
Columbus–Broadway and Embarcadero–Bryant are development sites. Embarcadero–Folsom is qualification.
The [eligibility rules](../lidar/operations/labelling-workspace.md#development-eligibility)
refuse validation inputs for tuning, diagnostics and development evaluation, independently of
an episode's earlier split label. Public availability or a historical tuning read does not
override the current cohort.

W1 must record the actual manifest/ledger decision. Keep the two-site screen as a pending
protocol: use it only in an eligible, explicitly defined validation/review workflow, or after
an explicit owner cohort decision with contamination history retained. Do not bypass a guard
by calling the activity production or by reading an export. Default logger development to
Kirkham, Columbus or Embarcadero–Bryant. Neither the new policy nor a fresh export makes previously
seen episodes independent. Do not alter cohort assignments in this planning PR.

### Source-to-work ledger

| Source reviewed                                                                                                                                                                                                                                                         | State at reconciliation                                                                                   | Weekend disposition, owner role and completion evidence                                                                                                                                                                                                                                                                                    |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| This planning chat and [PR #1022](https://github.com/banshee-data/velocity.report/pull/1022)                                                                                                                                                                            | W1–W4 planned; matched screen not executed                                                                | Logger implementer and operator retain the existing budget; Sunday output remains reproducible records, exclusions and a bounded review or explicit blocked status                                                                                                                                                                         |
| Voice chat “Discuss facet tracking experiments”, `01a1d6bc-73c2-7330-98dd-6d7d249fd449`                                                                                                                                                                                 | Requested identity/relative-speed diagnosis, point-loss follow-up, review tools and unattended candidates | W3 records both speeds, leader changes, fragmentation, observed ends and suspected lost returns; retention/cluster repairs require a separately frozen comparison after attribution                                                                                                                                                        |
| [#1021](https://github.com/banshee-data/velocity.report/pull/1021), [#1025](https://github.com/banshee-data/velocity.report/pull/1025), [#1026](https://github.com/banshee-data/velocity.report/pull/1026): harness selection, first-install fixes and deployment files | Merged; runbook records local/NAS kirk0 fixture equivalence, not an approved-campaign or soak result      | Harness owner verifies bounded selection, executor/reference compatibility, Git/timer behaviour, restart/resource gates and publication recovery under the [existing handoff](platform-experiment-harness-next-steps.md); report each acceptance separately; this is a parallel queue                                                      |
| Voice shortlist: `l5id0013`, `l5id0016`, `ground0002`; geom0016 already W4                                                                                                                                                                                              | Candidates, not verified launch-ready jobs                                                                | Harness owner verifies approvals, predecessors, manual closures, checked-in arm configs, archived inputs and supported evaluators; defer incompatible candidates with named blockers rather than weakening their scoring or broadening automatic ticks                                                                                     |
| [#1029](https://github.com/banshee-data/velocity.report/pull/1029): cohort revision 2                                                                                                                                                                                   | Merged assignment changes the proposed site use                                                           | Owner/eligibility reviewer resolves the diagnostic-versus-validation boundary first; record manifest digest and decisions. Its empty qualification capture also needs source recovery/hash verification before any later qualification use                                                                                                 |
| Chats “Fix published scene box yaw” and “Investigate skewed vehicle boxes”; [#1024](https://github.com/banshee-data/velocity.report/pull/1024), [#1027](https://github.com/banshee-data/velocity.report/pull/1027)                                                      | #1024 merged; overlapping #1027 open                                                                      | Rendering owner reconciles the duplicate PR and retains regression/investigation evidence. Verify Lombard +109.807 s and Broadway +36.819 s with the corrected player; then separate residual stored-yaw/turn/coast issues from display defects using permitted return-level evidence                                                      |
| Chat “Investigate skewed vehicle boxes (2)”, `01a1d81f-c4d8-7e53-9453-578437798051`                                                                                                                                                                                     | Fresh scene exports and a local preview requested; work active, asset PR/result not yet verified          | Publishing owner records stamped builds, same-database provenance, allowed capture identities, fresh manifests and interrupted-run recovery; review the separate asset PR and preview before any authorised merge. Verify dedicated API ports: an active-session update reports a replay interrupted by test traffic on shared port 8080   |
| Chat “Refine the SRAC Euler prompt”; merged [#1023](https://github.com/banshee-data/velocity.report/pull/1023)                                                                                                                                                          | Maths and horizon contract complete as documentation                                                      | Research owner carries solver/certificate viewer, synthetic examples and physical qualification into its separate follow-up plan; no SRAC implementation in W2                                                                                                                                                                             |
| Same chat; merged [#1030](https://github.com/banshee-data/velocity.report/pull/1030)                                                                                                                                                                                    | 20 Hz/48 ms source audit complete; readiness unproven                                                     | Performance owner follows the [20 Hz report](../research/lidar-20hz-readiness.md): instrument acquisition/assembly/service/age/drop funnel, audit time/count contracts and export stride, then obtain native 10/20 Hz workloads and workload-matched baselines; defer sensor/default changes and Pi qualification from this weekend's core |
| Chat “Review PRs and push fixes”; merged [#1018](https://github.com/banshee-data/velocity.report/pull/1018), [#1019](https://github.com/banshee-data/velocity.report/pull/1019), [Gerardus #85](https://github.com/banshee-data/gerardus/pull/85)                       | Contract/client fixes delivered; road-surface evidence remains fixture-only                               | Road-review owner retains real `ggp-mlk-1` build/access, supported geometry/mesh checks and ten-minute rendered/memory/latency acceptance under the [road review](../ui/visualiser/road-surface-review.md); separate acceptance queue, not evidence of vehicle lane maps or metric-qualified road geometry                                 |
| Friday briefing, “Schedule Friday Briefing” and cloud chat `01a09444-614c-7a13-b508-7247d427e01f`                                                                                                                                                                       | New noise/metric-validity research recommendations, not repository adoption                               | W2/W3 retain estimator/filter stage, jitter/drift/offset and relative-speed diagnostics. Literature owner verifies the primary study before a new experiment or threshold; derived-metric validation remains necessary even if position/speed errors improve; PET remains deferred                                                         |
| ChatGPT “Develop SRAC Metric Prompt”                                                                                                                                                                                                                                    | Initial broader prototype ideas superseded by the docs-only Euler request                                 | Covered by #1023; keep AV/human modes, nominal-versus-additional demand and predicted-versus-retrospective evidence distinctions without scheduling its proposed simulator this weekend                                                                                                                                                    |

### Practical ordering and cut rules from today's work

1. W1 checks mounted sources, current cohort policy, actual frozen references and available
   analysis versions. Settle permitted development inputs before selecting review or machine jobs.
2. Rendering/publishing owners finish their existing jobs separately. A player-only yaw correction
   needs no raw replay; the requested fresh pipeline exports are a different deliverable and may
   take hours. Do not restart completed recordings merely because export failed. Check build stamps,
   shared database paths, `local.mk` and scene-assets preflight, with private server ports and
   per-worker storage. Test processes must not share the production replay-control endpoint.
3. W2 develops on permitted development evidence while W3 performs only the allowed review work.
   If validation use remains unresolved, demonstrate the development-site logger and show the
   two-site protocol as blocked rather than claiming the manual comparison ran.
4. The harness owner selects only prerequisite-complete approved work, using explicit supported
   slug runs if automatic selection remains unbounded. A running timer or installation fixture
   does not complete H1/H3 acceptance or the week-long H5 soak. Do not enable an unrestricted
   campaign merely to meet Sunday.
5. W4 remains conditional on the existing geom0016 rule, repeatability, masks and evaluator support.
   Reviewed cases also record return-loss, class, heading and relative-speed hypotheses; they do
   not justify new tracker tuning within a validation cohort.
6. Keep the 20 Hz qualification, 6.35 s SRAC solver, real-road acceptance and stronger PET/safety
   studies as separately owned follow-ups. Record blockers and next evidence, not a completed
   weekend promise or an unbudgeted extra implementation lane.

### Review coverage and limits

Read the local Codex histories for the related chats named above, including the voice chat and
this chat; the older “Explore facet-based LiDAR tracking” chat was checked and its latest substantive
turn predates today. Read the relevant ChatGPT SRAC prompt and Friday briefing, plus the cloud
briefing's current-day turn. Voice history was paginated to its beginning. Active and archived
local/cloud Codex and ChatGPT inventories were checked; older pages were bounded once their
latest activity preceded this day. Unrelated account/business, household, network and other-repo
chats were excluded from repository task interpretation. Direct Claude transcripts were not
available through those inventories; today's repository PR descriptions and merged documents
provide their engineering record, without claiming every external session was read.

All eleven velocity.report PRs created or updated in the selected day and present in the query
were reconciled (#1018, #1019, #1021, #1022, #1023, #1024, #1025, #1026, #1027, #1029 and #1030); Gerardus #85
was the cross-repository dependency. The authoritative enumeration is the source ledger above:
#1020 is a stack link, not a separate PR in the returned set. #1017 is prior-day synthesis
already incorporated in the oracle response. Live jobs and PRs may change after the snapshot;
the source chat owners retain execution/merge responsibility. This update launches no jobs,
changes no cohort, deploys nothing and makes no physical-validation claim.

## Work and acceptance

The following are outcome boundaries for later handoffs, not prescribed implementation designs.
Each implementation change must include its relevant tests and evidence. Substantial work can
be handed to Claude independently after the shared contract and dependencies are settled.

| Work                            | Problem and desired outcome                                                                                                                                                             | Dependencies                                                                      | Acceptance and evidence                                                                                                                                                                |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| W1: evidence preflight          | Identify permitted cohort/use, accessible packets or persisted evidence, accepted placement/lanes, pack/reference revisions and evaluation history                                      | Existing manifests, site/run/clip records, operator access                        | One explicit site/source/version binding and permitted use, readable evidence, known missing fields and split roles; a recorded fallback if access or eligibility fails                |
| W2: per-site logger             | Existing interaction rows omit both speeds and complete follower paths/timelines; retain enough evidence to reproduce candidate, following and experimental DRAC outputs and exclusions | W1; frozen formula, applicability and provenance contract                         | Deterministic analytical content; exact values and reason accounting; no inferred site join from time overlap; explicit missing uncertainty; synthetic and storage/export checks below |
| W3: intensive review            | Distinguish bodies from fragmented tracks, credible leaders from adjacent/crossing traffic, and visible endpoints from unsupported shape                                                | W1 and W2's period identities; existing visualiser and annotation tools           | Three or four source-bound periods with identities, uncertainty, support, changes and disagreement recorded; physical references only where independently supported                    |
| W4: conditional geometry screen | Determine whether remaining end error is extent shortfall or material face placement before funding new facets                                                                          | Frozen masks/references and matching replay evidence; existing geom0016 evaluator | Repeatable control/face-state tuning screen and P/S attribution under the existing rule, or an explicit insufficient-evidence report; no adoption from tuning alone                    |

Review straight following, adjacent vehicles, a turn/merge if present, and fragmentation or
occlusion. Candidate selection serves diagnosis and stays tuning. Do not choose only successful
pairs. Annotation records must permit unknown leader/path membership, uncertain endpoints,
fragmented identity and assisted provenance. They do not by themselves validate physical speed
or an unseen bumper.

## Server and visualiser boundary

The server/analysis work owns reproducible equations, candidate/metric eligibility, exact speeds,
endpoints, versions, path evidence and exclusion accounting. An offline export is sufficient for
the first engineering exit; live production, a public API and broad schema changes can follow
review of the contract and outputs. Reuse the central metric registry and existing L8 semantics.

The visualiser already has seek/step, track selection/highlighting, track history, annotation
navigation, physical references and report Compare. Following-reference authoring uses the
existing drafting/import workflow. A new interaction timeline, pair highlighting, click-to-seek
entry points, gap overlay and diagnostic panel are stretch goals unless the first review shows
one is essential. Existing single-track highlighting is not proof of pair highlighting.

Accepted lane snapshots help interpret the scene; L8 local paths do not automatically consume
them. Record operator lane/path observations without claiming lane-constrained pairing is
implemented. Keep uncertain membership and geometry registration provenance visible.

## Budget and cut rules

| Work                                                      | Engineer allowance |
| --------------------------------------------------------- | -----------------: |
| Friday evidence and access preflight                      |                1 h |
| Shared contract and experiment rules                      |                1 h |
| DRAC arithmetic/applicability and tests                   |                2 h |
| Explicit-context offline logger                           |                4 h |
| Repeat-run check and overnight launch                     |                1 h |
| Review support and interpretation                         |                2 h |
| Conditional geometry screen or essential review unblocker |                2 h |
| Validation, edited report and review preparation          |                3 h |
| Total                                                     |               16 h |

Two additional operator-hours serve provenance, a few clips, supported endpoint keyframes,
leader/class disagreements and freezing. Overnight compute is bounded to selected short cases;
reuse compatible persisted evidence before replaying. Costs are allowances, not measured rates.

If review needs a tool change, it consumes the conditional two-hour allocation and displaces
geometry execution. If a full review workspace is needed, schedule it separately. The geometry
screen can use kirk0's existing supported tuning evidence; Lombard is not assumed to have new
physical truth ready. Preserve the oracle's eight-hour fallback: one site, primitive/tests,
exact export, repeatability and a short report.

If Lombard is unavailable or its diagnostic use is refused, use an eligible development site
for the executable logger. Any permitted published-scene validation/review is a separate
demonstration with its actual use purpose recorded. Capture/label access failures must not trigger
an unbounded new capture campaign or consume held-out windows to find a better result.

## Validation and evaluation boundaries

Test known-gap closing pairs, equal/diverging speeds, stopped queues, uncertain positive gap,
covariance assumptions, crossings/turns, leader changes, duplicate tracks, occlusion, irregular
timestamps and clipped extrema. Verify same-input repeatability, conflicting-content refusal,
version separation, overlapping capture times at two sites, aggregate reconstruction and exact
opportunity accounting. Retain existing following-distribution semantics.

For review navigation, verify source/pack/sample identity, seek landing, stale version refusal,
unsaved-edit guards and retained assisted provenance for any changed workflow. Physical
Compare's held-out restriction must remain enforced. Lane proposals, authored lanes and the
snapshot in use are distinct evidence states.

Record Lombard's earlier screening/tuning exposures under their original measurement family;
its current validation cohort still governs new use. Check each new segment's policy and use
ledger before scoring. Preserve embarcadero-folsom's existing
evaluation boundaries. Run at most the oracle's two selected experiments: current-baseline
coverage/logging and conditional geom0016 tuning attribution; freeze configurations and rules
before execution. Raw outputs stay local; edited reports retain provenance and limitations.

## Sunday demonstration

Show the two-site matched-screen ledger and its approximate observations only if that review
was permitted and executed; otherwise show the cohort/access blocker and the preserved protocol.
Include player/export provenance, skewed-box follow-up and any missing or incomplete windows.
Then show the eligible development site's binding and analysis version, a repeatable evidence log
with both speeds, reconciled candidate/eligible/excluded time, and three or four named review periods.
Explain a supported value and an ambiguous or suppressed case where evidence permits them;
otherwise show why no physical value qualifies. State separately which engineering checks
passed and what remains missing for physical publication. A geometry screen reports its actual
status, including insufficient evidence, rather than implying a completed facet comparison.
