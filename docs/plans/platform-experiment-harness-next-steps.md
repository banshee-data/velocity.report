# Unattended experiment harness: next steps

- **Status:** Proposed
- **Scope:** Handoff for unattended operation and the weekend experiment queue
- **Related:** [Harness operations](../platform/operations/experiment-harness.md),
  [harness plan](platform-experiment-harness-plan.md),
  [experiment queue](../../data/experiments/README.md)

## Outcome

An operator can select a bounded set of approved experiments, leave the worker unattended,
and later explain what ran, what was skipped and why. The worker obtains checked-in changes
on its normal cadence, respects experiment prerequisites and preserves provenance and raw
evidence outside Git. Unavailable inputs or unsupported steps are visible, not silently
treated as successful.

The harness already provides a periodic tick, optional Git fetch, eligibility checks, serial
execution, recovery and optional run-note publication. Its operations guide records NAS
deployment and soak as pending. Implementation is not evidence that a worker is installed
or that a real campaign has passed.

## Work scopes

### 1. Bound the unattended queue

Define an operator-controlled way for a periodic tick to select only the intended experiment
slugs without changing the approval status of unrelated experiments. Decide where this
selection is maintained, how it interacts with in-progress pinned runs and how an empty or
invalid selection behaves. Preserve the existing dependency, capability, manual-step, disk
and budget gates.

**Done when:** a selection can include and exclude named eligible experiments; excluded
experiments do not start; invalid selections are visible; and restart, changed selection and
pinned-run behaviour are tested and documented. Selection must not itself approve an experiment.

### 2. Make the requested experiment steps executable

Review the selected experiment specs against current executors and close manual prerequisites
only when their stated evidence exists. The following are investigation findings, to be checked
against current main before implementation:

- `geom0016` is proposed and calls for a near-face evaluator not listed among supported
  harness executors. Its manual predecessor and reviewed references must be checked. Its tuning
  screen is not a held-out decision.
- `l5id0013` is approved but has an unclosed manual predecessor and needs Kirk captures,
  labels and appropriate scoring bindings.
- `l5id0016` is proposed and describes an archived-row identity re-score. A generic
  `corpus_screen` name does not establish support for that particular measure; it also has
  an unclosed manual predecessor.
- `ground0002` is proposed, has a manual predecessor and specifies benchmark and physical
  reference scoring beyond the documented executor contract.

For each chosen experiment, decide whether to extend the harness, complete an existing manual
step, or defer it. Preserve each spec's frozen rules and held-out boundaries. Do not mark
prerequisites closed merely to make a job runnable. Each arm needs its checked-in tuning
configuration and fingerprint under the repository experiment rules.

**Done when:** a dry run reports the intended units and honest skip reasons; supported steps
consume the specified inputs and retain provenance; tests cover missing or incompatible inputs;
and an experiment is never represented as complete when a required step was skipped.

### 3. Deploy and qualify unattended operation

Follow the existing deployment runbook on the actual worker: dedicated clean checkout, host
profile, read-only capture access, sufficient evidence and scratch storage, cached tools and
dependencies, and the timer and service. Configure publication credentials on the host only
if run-note publication is intended. Check the resulting status and skip reasons, then run a
bounded representative job and the planned soak.

Decide explicitly whether a failed fetch may run the last checked-in local revision. Current
documented behaviour permits it; a latest-only policy would need an explicit change and test.
Offline operation disables fetch and publication and is distinct from connected unattended
operation.

**Done when:** the installed timer is observed firing, the selected job is traced to its
source and configuration, interrupted work recovers as intended, failures and skips are
visible, and the deployment and soak results are recorded. Do not claim NAS readiness or
physical validation from local unit tests alone.

## Sequencing and weekend handoff

First confirm the worker and inputs, then establish queue selection and complete only the
prerequisites needed by the selected jobs. A manually invoked single-slug run remains an
available bounded starting point. The Broderick logging and review work can be planned in
parallel; its annotations and physical references are separate dependencies, not automatic
outputs of the harness.

Keep raw recordings, row-level data, logs, databases and campaign state outside the checkout.
Commit human-readable reports with methods, numerical findings, provenance and limitations
when experiments have actually run.
