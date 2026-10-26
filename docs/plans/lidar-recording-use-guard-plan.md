# Recording-use guard for tuning and acceptance

Keep reserved recordings out of tuning automatically, before the pipeline reads their packets.
A recording remains useful after development; the guard preserves the distinction between a
repeatable development check and fresh acceptance evidence.

- **Status:** Proposed; implementation not started
- **Target:** v0.7.0, deployment and packaging tooling
- **Layers:** Offline analysis, experiment jobs, and L10 tools
- **Canonical:** [Per-frame evaluation](../lidar/operations/per-frame-evaluation.md#freezing-a-split)
- **Related:** [Test corpus](lidar-test-corpus-plan.md), [capture-bound labels](lidar-capture-bound-annotation-format-plan.md), [experiment queue](platform-experiment-queue-plan.md), [capture identity, D-27](../DECISIONS.md#d-27--capture-identity-and-the-capture-indexs-rules), [backlog](../BACKLOG.md#v060---deployment--packaging-060)

## 1. Problem and scope

Frozen splits already bind reviewed objects, capture digests, time spans, label revisions, and
cumulative tuning history. The corpus evaluator checks a split when supplied one; a run without
`-split-manifest` has no such protection. The optional argument therefore cannot protect a
reserved recording from another sweep, replay command, or queued job.

Reservation must also work before labelling. A complete reviewed annotation pack is required to
freeze reference truth, but reserving a new site must not wait for that review.

The deliverable is one local recording-use policy, one Go admission check, and its integration
into recording-backed development and acceptance tools. It covers input selection, derived
clips, acceptance context, and an auditable record of use. Live sensing and ordinary device
startup do not depend on this offline policy.

This plan does not implement the guard, certify any capture as untouched, change tracker
defaults, or alter the 0.5.2 physical acceptance thresholds. It does not prescribe a fixed
percentage of footage to label. City coverage, label sampling, and sample-size targets remain
the corpus plan's work. It adds no cameras, road-user identities, cloud service, or remote
transmission of observations.

## 2. Existing pieces to reuse

| Piece                                                                                                                                                  | Reuse and remaining work                                                                                             |
| ------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------- |
| [Frozen split](../../internal/lidar/annotation/split_frozen.go)                                                                                        | Keep reviewed reference pins, capture roles, and cumulative tuning history. Add no second physical-reference format. |
| [Corpus split check](../../cmd/tools/lidar-state-estimation-baseline/split.go)                                                                         | Reuse case/capture binding; require a recording-use decision even when no labels are involved.                       |
| [Job capture manifest](../../internal/lidar/jobs/manifest.go)                                                                                          | Identify input by SHA-256 and verified bytes, with paths as hints.                                                   |
| [Worker capture resolver](../../internal/lidar/jobs/runner/captures.go)                                                                                | Check the verified inputs at execution as well as at job submission.                                                 |
| [Sweep runner](../../internal/lidar/sweep/runner.go), [auto tuner](../../internal/lidar/sweep/auto.go), and [HINT](../../internal/lidar/sweep/hint.go) | Admit the recording-backed request before the first trial and preserve its policy decision across trials.            |
| [Capture-bound labels](../lidar/operations/label-sets.md)                                                                                              | Reuse reviewed labels across replays without assigning a new data-use history to each run.                           |

The split's default 30-second guard separates nearby packs of one source. It does not establish
independence between neighbouring sites or repeated visits. First delivery reserves whole
capture files and their placement groups, including every file in a multi-file case. Reviewed
time spans still identify the short clips scored inside those reservations.

## 3. Policy and identity

Store the authoritative policy in the configured local analysis workspace, separately from
annotation packs. Version its schema and revisions, give each revision a canonical digest, and
retain its history. The configured workspace selects the active revision; a job cannot choose
an older revision or supply its own more permissive policy.

The proposed metadata contract is:

| Record                 | Required information                                                                                                                                        |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Capture                | Whole-file SHA-256, sensor identity, capture-time bounds, and placement/group ID; filename is a display label.                                              |
| Placement group        | Stable group ID, member captures, and the reason repeated visits or nearby placements share a role.                                                         |
| Use assignment         | Development, reserved test, or history unknown; revision, reason, and supporting history references.                                                        |
| Derivation             | Child digest, verified parent digest(s), source packet/time bounds, and the conversion or cut that produced it.                                             |
| Acceptance declaration | Experiment and question IDs, policy revision, frozen split digest, candidate configuration/build identities, criteria identity, and allowed cases/episodes. |
| Use event              | Purpose, actual inputs and context, policy decision, job/attempt identity, and whether results were released or used to guide changes.                      |

Protect unlabelled raw captures through use assignments. Add the frozen split only when a run
needs reviewed truth. The new use assignment is not another annotation role: at the boundary,
development maps to existing tuning or screen use, reserved test maps to held-out acceptance,
and unknown is ineligible for both until its history is resolved. Reject contradictory policy
and frozen-split roles.

A renamed or copied file retains its digest and role. A cut or re-encoded file has a new digest
and inherits its verified parents' reservations and exposure. Missing or unverified ancestry is
unknown. Verify and consume the same immutable bytes; changed inputs invalidate admission.

Whole-placement protection is intentionally broader than a labelled clip. Period-level
reservation is deferred until the guard can include settling passes, warm-up, interpolation,
and tracks that cross a boundary. Filtering scored rows after replay is not exclusion: the
protected packets may already have changed the background or tracker state.

## 4. Purposes and decisions

Purpose is fixed by the server operation or approved job kind, not an arbitrary caller string.
A client cannot label a sweep as annotation or acceptance to get around its input restrictions.

| Purpose                                                   | Development                                | Reserved test                                                                     | History unknown                                                       |
| --------------------------------------------------------- | ------------------------------------------ | --------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Tuning, parameter sweeps, and candidate-selection screens | Allow and record exposure                  | Refuse                                                                            | Refuse                                                                |
| Visible regression checks                                 | Allow; claim repeatability on exposed data | Refuse                                                                            | Refuse                                                                |
| Declared final acceptance                                 | Refuse a fresh held-out claim              | Allow only within the pinned declaration and reference split                      | Refuse                                                                |
| Annotation or capture-health QA                           | Allow                                      | Allow without candidate comparisons, fitted recommendations, or acceptance claims | Allow only to inspect provenance/health and prepare the history audit |

All permitted operations record a use event. Annotation can use the independent object
proposer and raw-point fit under their existing review/assistance rules; it cannot return
candidate tuning metrics under that purpose.

A broad request for all development inputs may select the eligible subset and report the
excluded IDs and reasons before starting. An explicit request naming any forbidden or unknown
input fails as a whole. It must not silently shrink the evaluation population.

A missing, unreadable, invalid, or unrecognised policy refuses governed analysis. The same
applies to unknown captures, conflicting group membership, mismatched digests, and unsupported
policy versions. Metadata inspection and file hashing needed to establish identity are allowed;
packet decoding, adaptive warm-up, fitting, and metric generation wait for admission.

## 5. One admission path

Implement the shared policy loader and decision logic beside the existing Go annotation/split
contracts. Keep command, HTTP, and worker wrappers thin. They resolve inputs and purpose, call
the shared check, and pass its immutable receipt into the execution context.

| Boundary                                                       | Required integration                                                                                                                                              |
| -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CLI corpus, refinement, box, benchmark, and ground-truth tools | Resolve every input and dependency, load the workspace policy by default, and check before replay or scoring.                                                     |
| Generic, auto, and HINT sweeps                                 | Validate the complete case set before any trial or run record is created; retain the approved input set across iterations.                                        |
| HTTP recording-backed replay and tuning                        | Apply the operation's purpose on the server; direct file paths cannot bypass content-based checks.                                                                |
| Campaign expansion and job submission                          | Validate every arm/case and pin policy, purpose, ancestry, references, and acceptance declaration in job identity.                                                |
| Worker capture resolution and execution                        | Recheck the active policy and verified bytes before execution; reject a stale admission if current assignments forbid it.                                         |
| Python campaign/export drivers                                 | Call the guarded Go surface; a driver cannot reproduce a weaker allow/deny rule. Exporting a reserved candidate comparison is an exposure, not capture-health QA. |

Inventory the actual commands and callers before declaring integration complete, including
tools used by older scripts. Unsupported callers fail with a migration message, rather than
falling back to an unchecked replay.

Extend job identity and bundle provenance together when adding policy inputs: two runs under
different purposes, declarations, or policy revisions cannot share an identity or cached score.
Version any affected identity contract explicitly. Resuming or retrying a job rechecks current
admission; a historical receipt proves what ran, not permission to run again.

For jobs already running when a policy changes, cancel or invalidate affected work before more
results are released. Its receipt names the revision under which packets were read and the
later invalidation. A policy change cannot turn partial or forbidden work into acceptance.

## 6. History and fresh evidence

Use history distinguishes three events: replaying data, exposing candidate results, and using
those results to select or change the system. A replay alone does not establish tuning, and a
missing log does not establish freshness.

Assignment changes require an explicit reason and append to retained history. Unknown footage
may become development without claiming freshness. Marking it reserved requires an audit that
supports that claim, with unresolved history remaining unknown. No automatic import assigns
unused status from filenames, dates, absence of labels, or a new split lineage.

A deliberate release of reserved footage for development changes its assignment and records
the exposure before tuning starts. There is no general ignore-policy flag. Prior development
use remains in history after a rename, clip, policy revision, or new annotation revision.

Acceptance records the predeclared questions and bounded run set, including failures and
missing reference evidence. Further runs must belong to that declaration or be recorded as
additional exposure. Once its results guide changes, the cohort becomes visible regression
evidence for those choices; it cannot be relabelled fresh for a shared pipeline by naming a
different downstream metric.

This is a cooperative scientific-workflow guard, not an operating-system access boundary.
People can read raw files with other programs, and the software cannot infer every human
decision. Such uses must be declared. Evidence imported from unchecked tools or without an
admission receipt is ineligible for a fresh acceptance claim.

## 7. Operator workflow

1. Register the new placement and capture digests before running candidate screens.
2. Assign the placement group to development, reserved test, or unknown history.
3. Select short annotation clips and review labels; reservation already applies to the whole group.
4. Freeze reviewed truth when complete, retaining unsupported components as unknown.
5. Run tuning on development inputs; inspect the preview showing included and excluded recordings.
6. Pin the final candidates, criteria, reference split, and questions in an acceptance declaration.
7. Run that declared comparison on the reserved cohort and record its outcome.
8. Keep exposed cohorts for regression and reserve later captures before they inform development.

For existing S2, import known development use and unresolved history without certifying the
entire corpus. Embarcadero–Folsom retains its intended reservation while its actual exposure
history is checked; the import must represent any uncertainty and block fresh acceptance until
resolved. Already used recordings remain valuable for labelling and regression.

## 8. Delivery sequence

| Step                       | Outcome                                                                                   | Acceptance                                                                                                                                     |
| -------------------------- | ----------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| R1: identity and policy    | Schema, local active revision, group assignments, ancestry, and history import            | Unlabelled captures can be reserved; renames preserve roles; inconsistent or unknown inputs cannot enter tuning.                               |
| R2: shared admission       | Go decision function, typed purposes, receipt, and read-only preview                      | Every allow/refuse row in Section 4 is exercised; absent policy and contradictory splits refuse before packet decoding.                        |
| R3: execution integration  | CLI, sweep/API, campaign, and worker paths use the same check                             | Direct calls and queued jobs give the same decision; no optional split argument bypasses policy; retries and cached results preserve identity. |
| R4: acceptance and history | Pinned declaration, frozen references, local use events, and release-for-development flow | Final tests cannot tune candidates, stale jobs are rejected, and prior exposure survives later policy and label revisions.                     |
| R5: deployment and handoff | Install/configure the policy location, migrate callers, and document recovery             | A clean v0.7.0 analysis setup previews and runs allowed inputs; missing setup explains how to repair it; live capture still starts offline.    |

Ship increments in this order without advertising automatic exclusion until R3 covers every
inventoried recording-backed analysis path. Keep the existing split requirements during
migration. Unknown historical outputs remain readable as development evidence but gain no
automatic acceptance receipt.

## 9. Validation

The implementation must demonstrate these behaviours through real entry points:

- A permitted development capture runs, while an explicitly named reserved or unknown capture refuses.
- Copying or renaming reserved bytes does not change the decision.
- A verified cut inherits its parent's reservation; an orphan cut remains unknown.
- Every member of a multi-file case and all replay context are checked before the first packet is decoded.
- A protected warm-up or settling input refuses even when the scored interval is elsewhere.
- Missing policy, stale policy, changed bytes, and conflicting frozen roles refuse.
- HTTP, direct CLI, generic/auto/HINT sweeps, and queued workers cannot disagree about the same input.
- A caller cannot use annotation or QA purpose to request candidate comparisons.
- Declared acceptance runs only its pinned candidates/cases and scores the frozen reference revision.
- A job queued before a reservation change is rechecked; resumed, cached, and in-flight work cannot publish a fresh result under an invalidated admission.
- Making a cohort development does not erase prior use, and an audit cannot infer freshness from absent logs.
- The operation receipt records the policy and source digests and reports exclusions and refusals.
- Installing or losing an analysis policy does not change live sensor acquisition.

Use synthetic small captures and spy readers for refusal-before-decode checks, then one
registered local development case for the command/worker round trip. Record policy-check and
hashing time separately from pipeline frame time. Reuse verified immutable capture digests
within an admission; do not hash multi-gigabyte inputs per frame or add a live-path cost.

## 10. Risks and boundaries

| Risk                                                      | Likelihood / impact | Response                                                                                                              |
| --------------------------------------------------------- | ------------------- | --------------------------------------------------------------------------------------------------------------------- |
| A legacy driver bypasses policy                           | High / high         | Inventory callers and test each entry point; require receipts for acceptance.                                         |
| Historical use is incomplete                              | High / high         | Import unknown explicitly; audit before reserving and preserve all earlier exposure.                                  |
| Derivation loses source identity                          | Medium / high       | Verify ancestry and inherit reservations; unknown ancestry blocks tuning and final tests.                             |
| Adjacent clips share vehicles or adaptive state           | High / high         | Whole placement/capture groups first; time-window admission deferred.                                                 |
| Policy changes race a worker or cache                     | Medium / high       | Pin identity, recheck at execution/resume, and invalidate affected results.                                           |
| Configuration blocks an existing analysis host            | Medium / medium     | Ship a preview/import path and clear repair messages; keep live capture independent.                                  |
| Scope expands into a new labelling app or security system | Medium / medium     | Reuse existing UI/Go contracts and scope the claim to governed analysis tools.                                        |
| Audit records disclose observations                       | Low / high          | Keep policy and receipts local, record evidence IDs rather than raw rows, and preserve existing export/privacy rules. |

No new dependency, background service, or cloud access is required. Metadata is small relative
to captures; verification runs at admission, not inside the Raspberry Pi's perception loop.
The plan protects experimental evidence without making a file unreadable to its owner.

## 11. Completion

The 0.7.0 backlog item is complete when the policy can reserve unlabelled placement groups,
all inventoried recording-backed analysis entry points enforce it, reference scoring retains
its frozen split, and a local receipt proves the purpose and data used. The refusal and
migration cases above must pass.

This plan's approval does not establish any dataset's freshness or any physical metric's
accuracy. Those remain evidence recorded by the corpus and experiment acceptance processes.
