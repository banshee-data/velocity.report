# Existing motion captures: following and DRAC operations

- **Status:** Operational sequence and evidence inventory; full-archive coverage and physical qualification remain unverified
- **Scope:** Existing fixed-roadside motion captures used for following distance and experimental DRAC
- **Related:** [Weekend plan](../../plans/lidar-weekend-following-drac-plan.md), [export contract](following-evidence-export.md), [native review workflow](../../ui/visualiser/following-audit-workflow.md)

## Purpose and evidence boundary

Use the existing captures systematically before assuming that more collection is needed.
Separate the amount of data processed from the amount manually reviewed and from the amount
physically qualified. Three or four reviewed periods are the first acceptance gate, not the
intended limit of the batch analysis.

This inventory reconciles the committed reports available on 17 November 2026, at main
`ac5ade9`. It is not a fresh disk inventory, a new experiment, or a status report from a live
local agent. An unpublished build or run may be further ahead. Each operator must reconcile
receipts and the use ledger before continuing, rather than rerunning another owner's work.

Moving-sensor route captures have a separate [SLAM operations plan](../../plans/lidar-route-slam-phase1-bridge-ego-plan.md).
Their exported rotations and timing checks are not additional fixed-site DRAC coverage.

## What has been done

| Layer                        | Recorded evidence                                                                                                                                                                                                                                                                                | What this does not establish                                                                |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| Historical archive inventory | 24 sites and 514 minutes in the [archive comparison](end-on-truck-window-spike-2026-10.md)                                                                                                                                                                                                       | The current complete archive, its availability, or current DRAC processing coverage         |
| General tracker corpus       | 312,315 scored frames across 24 independent placements in the [state-estimation baseline](state-estimation-phase01-corpus-baseline.md)                                                                                                                                                           | That every frame has been run through the current following analyser                        |
| Opportunity screening        | First segments at 23 sites; over 100 minutes of tracker output, 2,111 pair-seconds and 1,249 vehicle-only pair-seconds in the [screen](following-opportunity-corpus-2026-10.md)                                                                                                                  | Validated gaps, DRAC, distinct independent encounters, or a comparable current site ranking |
| DRAC baseline                | Arithmetic/logger, three development sources, repeatability/accounting and fresh source-identity rerun in the [weekend completion records](../../plans/lidar-weekend-following-drac-plan.md#next-actions-and-completion-records)                                                                 | Independent physical accuracy                                                               |
| Source-matched preparation   | Five recordings with paired observation databases and following exports in the [readiness report](../../ui/visualiser/mesh-and-following-readiness-2026-11.md)                                                                                                                                   | All archived minutes processed or all resulting interactions manually accepted              |
| Review tooling               | Server-managed preparation/evidence and native review in [#1057](https://github.com/banshee-data/velocity.report/pull/1057)/[#1058](https://github.com/banshee-data/velocity.report/pull/1058); failed-window retry repair in [#1061](https://github.com/banshee-data/velocity.report/pull/1061) | A completed real encounter audit or physical qualification merely because CI passed         |

The five preparations are a useful body of existing evidence, not just one or two short runs:

| Site               | Frames | Evidence rows | Minute files | Supported closing samples |
| ------------------ | -----: | ------------: | -----------: | ------------------------: |
| Ashbury–Downey     | 11,574 |        31,000 |           20 |                       610 |
| Kirkham–9th        |    631 |         5,624 |            2 |                         0 |
| Columbus–Broadway  | 19,629 |       459,139 |           33 |                     2,369 |
| Embarcadero–Bryant | 12,924 |       213,147 |           22 |                     1,973 |
| Lombard–Broderick  | 11,318 |       159,265 |           19 |                     3,417 |
| Total              | 56,076 |       868,175 |           96 |                     8,369 |

These counts come from the readiness report, with its producer identities and configuration
limits. A minute file is a slice with evidence, not proof of a full minute of usable coverage.
Rows and closing samples are not distinct encounters; consecutive samples are correlated.
Do not divide these frames by the older corpus frames to invent an archive-completion percentage.
Kirkham has three eligible non-closing samples, so zero closing results is not a failed export.

The recorded native checks imported matching samples at all five sites, but cancelled audit
forms rather than saving them. The later server-backed workflow has automated tests; a completed
three-to-four-period operator review with persisted conclusions is not established by those
reports. Physical gap, speed, and DRAC qualification remains outstanding.

## Cohort and use gate

Read the current capture-level cohort manifest and measurement-family use ledger before opening
or processing a source. A familiar street name or accessible published scene is not permission.
Match exact bytes, capture window, site assignment, intended use, and policy version.

- The weekend development baseline uses Kirkham–9th, Columbus–Broadway, and Embarcadero–Bryant.
- The five-site preparation report also assigns the pinned Ashbury–Downey full capture to development.
- Lombard–Broderick and Embarcadero–Broadway are validation cohorts. Do not use them for
  development diagnosis, parameter selection, or choosing the next algorithm.
- Existing Lombard preparation was admitted as `annotation_review`, with a durable use-ledger
  entry. That narrow admission does not authorise developer-diagnostic reads or retuning.
- Resolve other sites and new full-capture hashes through the manifest; do not infer their role
  from inclusion in an older screen. Keep the original cohort and use history intact.

Freeze the method, configuration, eligibility rules, review protocol, and acceptance criteria
before an authorised held-out evaluation. Record exposure and resulting decisions. If findings
are used to tune the method, do not later describe that same evidence as untouched validation.

## Expected end-to-end steps

### 1. Reconcile inventory and ownership

Create a local capture/window ledger from the catalogue, manifests, archive storage, and actual
receipts. Deduplicate by bytes and intervals, not filename. Record missing files and inaccessible
storage explicitly. Check running jobs and completed outputs before queuing replacements.

Each row needs the following information, with unknown values left unknown:

| Group      | Required fields                                                                                                                                |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Source     | Capture ID, SHA-256, byte size, site/placement, sensor, start/end, duration, storage location and availability                                 |
| Use        | Cohort, measurement family, permitted use, manifest/policy digest and use-ledger receipt                                                       |
| Processing | Producer commit/version, checked-in tuning file and fingerprint, warm-up/scoring bounds, retained-point policy, analyser/schema version        |
| Geometry   | Sensor coordinate frame, calibration identity, accepted placement identity, lane snapshot if used, physical-reference identity and uncertainty |
| Artefacts  | Recording/header hash, observation database identity, canonical export hash, review slices and receipt, completion/checkpoint status           |
| Progress   | Not admitted, queued, running, failed, prepared, reviewed, or qualified; owner, blocker, retry reason and next action                          |

Report denominators separately: known archive time, accessible time, admitted development time,
processed time under the selected version, eligible time, reviewed encounters, and qualified
encounters. Split by site and source window. Use the union of overlapping intervals so repeated
runs do not inflate capture coverage. Without a current inventory, report counts, not percentages.

### 2. Freeze and preflight one batch

Select a bounded batch from admitted development captures. Start with ready sources and include
sparse or difficult periods; do not select only dramatic DRAC peaks. Freeze the current method
and checked-in parameter set before launch. Build through the repository Makefile and verify the
producer stamp. Record warm-up separately from scored bounds and reviewable recording bounds.

Validate source bytes, clock continuity, storage capacity, calibration declaration and output
location. An identity sensor-frame transform is not a surveyed site placement. Display placement
and analyser calibration remain separate. Use the [preparation and provenance contract](../../ui/visualiser/mesh-and-following-readiness-2026-11.md)
and [export runbook](following-evidence-export.md), not copied command lines from an older run.

### 3. Reprocess only what is missing or incompatible

The readiness investigation found that an older replay database lacked persisted state estimates.
An old display recording cannot retroactively supply those estimator rows. Reprocess the original
source when necessary to produce a matching recording, observation database, and canonical export.
Reuse an existing complete preparation only when its identities, configuration, use admission,
and required fields match the batch contract.

Queue bounded jobs with resumable per-source receipts. Preserve original captures and prior
outputs. Keep raw streams, row-level metrics, databases, logs, and runtime state outside Git.
Do not mark a job complete until its final database checkpoint and export succeed; interrupted
or busy-checkpoint outputs remain incomplete. Measure throughput on the actual storage before
choosing concurrency: the readiness report documents severe shared-drive I/O pauses.

### 4. Check software consistency before pooling

For every completed preparation, verify source and recording bounds, producer/calibration binding,
export/schema identities, row accounting, exclusion reasons, and slice-to-canonical correspondence.
Re-export a fixed database to test repeatability; distinguish this from replay reproducibility.
Do not silently pool different analyser versions, retained-point policies, or eligibility rules.

Retain both projected speeds, gap definition and endpoints, path/candidate decisions, support,
uncertainty and exclusion reasons. A failed or empty interval is not an observation of zero risk.
Cross-window aggregates must not double-count straddling intervals or repeated slices.

### 5. Summarise the whole admitted batch

Produce a per-site/window summary of processed duration, valid/supportable following time,
closing time, exclusion shares, and distinct reviewable encounters. State the encounter grouping
rule and censoring at source boundaries; a run-local track ID alone is not a persistent actor ID.
Keep distributions and denominators, not only maxima or selected examples.

Rank review opportunities by identity continuity, visible endpoints, path support, duration, and
uncertainty, alongside metric values. Include negative and suppressed strata. Historical
opportunity scores can guide scheduling but must not be relabelled as current DRAC results.
This scalable batch pass uses existing captures; it does not require a new field campaign.

### 6. Complete the initial native review gate, then widen it

Use the [native workflow](../../ui/visualiser/following-audit-workflow.md) on three or four named,
source-bound development periods first. Open the correct recording and evidence, seek and step
exact samples, and inspect both actors, leader/path decisions, endpoint support, both projected
speeds, occlusion, lost returns, track breaks, and suppression. Include a negative or ambiguous
case and test a minute transition.

Record conclusions, save the audit through the supported workflow, leave the review, reopen it,
and verify the same source, sample, annotations, and interval bounds. Capture software failures
separately from measurement disagreements. Operator annotations do not change the analyser or
recompute DRAC. Do not fabricate physical judgements just to exercise persistence.

Once this loop works, expand a stratified review across the processed development batch. Track
review coverage by distinct encounters and conditions, not the number of clicks or sample rows.
Turn failures into a ranked fix list and rerun affected strata after changes, retaining prior
results and version identities. A build completing is not completion of this gate.

### 7. Qualify physical measurements and evaluate held-out data

Define the intended accuracy, conditions of use, and acceptance criteria before selecting a
reference comparison. Reconcile existing surveyed distances, accepted geometry, labelled returns,
and independent speed references first. Quantify gap and closing-speed error, their dependence,
and the resulting DRAC uncertainty. Agreement with the same tracker or the old Compare chord gap
is not independent validation of an along-path gap.

Collect new controlled data only for reference gaps the existing material cannot answer. This is
separate from expanding computational coverage. Freeze the resulting method before the permitted
held-out evaluation. Keep longitudinal DRAC limitations explicit for cut-ins and ambiguous paths;
a reliable number for one interaction does not establish lateral collision risk.

### 8. Publish the bounded outcome

Commit an authored report with coverage denominators, source/configuration identities, methods,
review findings, limitations, and the next decision. Link the local receipts without importing
raw outputs. Label software-verified, manually reviewed, and physically qualified results
separately. Do not publish an experimental DRAC population as a qualified safety score.

## Outstanding work and completion receipts

| Priority | Next action                                                                  | Completion evidence                                                                                     |
| -------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| 1        | Reconcile the current archive and existing jobs/preparations                 | Deduplicated source/window inventory with availability, cohorts, versions, owners and unknowns          |
| 2        | Freeze a permitted development batch and process missing compatible evidence | Per-source receipts, successful checkpoints, exact accounting and explicit failed/missing windows       |
| 3        | Complete the initial native encounter review                                 | Three or four named periods, including a negative case, with saved/reopened audits and failure notes    |
| 4        | Expand batch summaries and stratified manual review                          | Distinct-encounter and duration denominators, exclusions, review coverage and a prioritised defect list |
| 5        | Audit existing physical references and close identified gaps                 | Reference independence, uncertainty budget, predeclared criteria and comparison results                 |
| 6        | Run an authorised frozen-method held-out evaluation                          | Use-ledger admission, frozen protocol and a report that distinguishes evaluation from tuning            |

The conditional facet/extent attribution screen still needs its frozen references/masks and
supported evaluator. It is not a prerequisite for basic same-path DRAC, which uses gap and
closing speed rather than a new acceleration estimator. Harness/label acceptance, recovery/soak
checks, accepted lane geometry where needed, and export calibration self-containment retain the
[weekend plan's separate gates](../../plans/lidar-weekend-following-drac-plan.md#next-actions-and-completion-records).

Estimate remaining engineering time only after the inventory, first review, and throughput
sample identify the actual work. Separate machine runtime, operator review, code fixes, and any
new reference collection. The original weekend allocation is not a measured remaining-time
estimate, and no multi-week collection requirement follows from the current coverage counts.
