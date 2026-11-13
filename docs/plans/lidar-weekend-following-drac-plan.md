# Weekend following and DRAC evidence plan

Produce reproducible per-site candidate, following, experimental DRAC and exclusion evidence,
then review a few multi-vehicle periods to decide the next geometry investment. The working
programme is November 14–15, 2026. Lombard at Broderick is the provisional intensive-review
site, subject to capture, lane and reference preflight.

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

| Historical reading                                                                            | Population and meaning                                                                                                                                                                               |
| --------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 92 vehicle pair-seconds and 116 vehicle track pairs                                           | [Opportunity screen](../lidar/operations/following-opportunity-corpus-2026-10.md#by-site), under `body-older`: track-centre corridor, speed and heading selector; vehicle size is a geometric screen |
| 0.2 valid / 94.7 accounted seconds                                                            | [Box following run](../lidar/operations/following-box-endpoints-2026-10.md#clause-2-paths-on-the-corpus): its box baseline, physical endpoint reader, pairing and applicability rules                |
| About 62 class-suppressed seconds; 42.1 valid seconds after treating every `dynamic` as a car | [Class counterfactual](../lidar/operations/following-class-release-corpus-2026-10.md#by-site): diagnostic upper bound, not a classifier candidate                                                    |

These readings differ in arm and eligibility. Dividing 0.2 by 92 would not give a qualification
rate. Their contrast makes Lombard useful for diagnosing exclusions; it does not establish
current measurement yield or guarantee suitable physical references.

## Work and acceptance

The following are outcome boundaries for later handoffs, not prescribed implementation designs.
Each implementation change must include its relevant tests and evidence. Substantial work can
be handed to Claude independently after the shared contract and dependencies are settled.

| Work                            | Problem and desired outcome                                                                                                                                                             | Dependencies                                                                      | Acceptance and evidence                                                                                                                                                                |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| W1: evidence preflight          | Identify accessible packets or persisted evidence, accepted placement/lanes, pack/reference revisions and evaluation history                                                            | Existing manifests, site/run/clip records, operator access                        | One explicit site/source/version binding, readable evidence, known missing fields and split roles; a recorded fallback if access fails                                                 |
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

If Lombard packets cannot be recovered within preflight, use its published scene for qualitative
reconnaissance and an accessible evidence dataset for the executable logger. Identify the two
demonstrations separately. Capture/label access failures must not trigger an unbounded new
capture campaign or consume held-out windows to find a better result.

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

Lombard segments already read remain screening/tuning for the corresponding measurement family.
Check each new segment's use ledger before scoring. Preserve embarcadero-folsom's existing
evaluation boundaries. Run at most the oracle's two selected experiments: current-baseline
coverage/logging and conditional geom0016 tuning attribution; freeze configurations and rules
before execution. Raw outputs stay local; edited reports retain provenance and limitations.

## Sunday demonstration

Show the selected site's binding and analysis version, a repeatable evidence log with both
speeds, reconciled candidate/eligible/excluded time, and three or four named review periods.
Explain a supported value and an ambiguous or suppressed case where evidence permits them;
otherwise show why no physical value qualifies. State separately which engineering checks
passed and what remains missing for physical publication. A geometry screen reports its actual
status, including insufficient evidence, rather than implying a completed facet comparison.
