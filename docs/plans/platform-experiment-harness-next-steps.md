# Unattended experiment harness: next steps

Prepare a bounded unattended queue for the weekend experiments, with explicit prerequisites,
deployment evidence and a recovery checklist. Each scope below describes a problem and its
acceptance criteria for implementation handoff.

- **Status:** Proposed
- **Scope:** Planning; implementation and worker qualification remain separate work
- **Canonical:** [Harness runbook](../platform/operations/experiment-harness.md)
- **Related:** [Harness plan](platform-experiment-harness-plan.md),
  [deployment](../../deploy/harness/README.md),
  [experiment queue](../../data/experiments/README.md)

## Current evidence and limits

The implementation merged in [PR #1014](https://github.com/banshee-data/velocity.report/pull/1014).
This review used commit `990f746b4251f6dfd0d1193a5af67939bfa04e7f`. Recheck candidate
specifications and actual worker capabilities before approving a campaign.

The runner already provides a thirty-minute systemd timer, optional Git fetch and fast-forward,
eligibility checks, serial execution, pinned builds, recovery, resource gates and optional run-note
publication. See [selection and recovery](../../internal/experiments/harness/run.go),
[host inputs](../../internal/experiments/harness/host.go),
[execution](../../internal/experiments/harness/execute.go) and
[publication](../../internal/experiments/harness/publish.go).
The runbook records NAS deployment and a week-long soak as pending. This review did not inspect
a running worker or execute a capture.

Automatic ticks consider every eligible approved specification; a single-slug run selects one.
There is no campaign allowlist. The supported executors are `state_estimation_baseline`,
`box_eval`, `ground_truth_eval` and `corpus_screen`. Unsupported kinds or parameters can exclude
the whole specification before its otherwise runnable units start. Do not assume the first
replay will run merely because a later unsupported evaluator is not yet needed.

## Work scopes

### 1. Bound the unattended queue

Provide an operator-controlled selection of experiment slugs for periodic ticks without changing
unrelated experiments' approval. Decide whether the selection belongs in Git or host configuration,
and record its revision with scheduling status. Define absent, empty, unknown and duplicate
selections, dependency handling and what happens to active pinned work when selection changes.
Fetching new specifications must not accidentally expand a deliberately bounded campaign.

**Acceptance:** only selected eligible experiments start; selection never approves an experiment,
closes a manual step or implicitly launches an excluded dependency. Excluded, blocked and invalid
entries have visible reasons. Restart, selection changes and newly fetched specifications are
covered. A dry run lists the complete intended queue before enabling the timer.

### 2. Close experiment compatibility and evidence gaps

The candidate inventory at the reviewed commit is:

| Candidate                                                 | Current blockers                                                                                                               | Intended next action                                                                        |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| [geom0016](../../data/experiments/geom/geom0016.md)       | Proposed; unclosed implementation; unsupported `near_face_eval`; reviewed tuning and held-out references need verification     | Scope evaluator support and verify implementation evidence; prepare the tuning screen first |
| [l5id0013](../../data/experiments/l5id/l5id0013.md)       | Approved; unclosed implementation; Kirk captures, labels and scoring bindings required; depends on l5id0006                    | Verify the predecessor and implementation closure, then explicit paired scoring inputs      |
| [l5id0016](../../data/experiments/l5id/l5id0016.md)       | Proposed; unclosed implementation; archived body-aware re-score is not a supported generic corpus measure; depends on l5id0012 | Verify archived inputs and scope the named re-score capability                              |
| [ground0002](../../data/experiments/ground/ground0002.md) | Proposed; unclosed implementation; unsupported benchmark kind and `physical_reference` parameter                               | Resolve execution and scoring compatibility before approval                                 |

For each chosen candidate, verify answered predecessors, any required successful run notes,
implementation closures reachable from main, host capabilities, actual source paths, reviewed
references and fingerprints. The example host advertises S2 captures; it does not establish Kirk
or label availability. Ground-truth scoring needs explicit pack, manifest, split and paired
baseline/candidate database bindings under the existing host contract.

Choose whether each gap needs an executor extension, a justified manual closure or deferral.
Do not remove required scoring or change a frozen rule just to admit a replay. If partial machine
execution is proposed, its dependency and completion semantics need explicit review and tests.
Each arm needs a checked-in parameter set, declared changed keys and a verified fingerprint.
Replay-only switches and diagnostic overlays also need recorded identities: a tuning hash alone
may not distinguish arms.

**Acceptance:** dry-run skip reasons match real blockers; unsupported inputs fail visibly;
completed steps retain verifiable source and output identities; missing measures never become
zero. Tuning evidence does not release a held-out read without its frozen references and gate.
Manual work and interpretive reports remain visible after machine units finish. No candidate in
this inventory is declared launch-ready by this plan.

### 3. Deploy and qualify the worker

Complete the deployment checklist's security review before installation, then use the
[deployment checklist](../../deploy/harness/README.md) on the actual worker: dedicated
clean main checkout, service identity, read-only captures, evidence and staging mounts, cached
toolchain and dependencies, host profile, service and timer. Confirm that evidence and scratch
storage satisfy the selected specifications; advertised capabilities alone do not prove inputs
exist. A dry run does not fetch, build or execute, so qualify those separately with a bounded
representative job as the service user.

Decide whether a fetch failure may use the last local revision, as currently documented, or
must stop new work. Define how the installed scheduler itself receives updates as distinct from
the per-commit executor builds. Offline operation disables fetch and publication; connected
unattended operation needs credentials and network access only for its configured services.

**Acceptance:** observe the installed timer firing and fetching an authorised change; record
the selected commit, configuration and host input identities. Exercise dirty or divergent Git,
missing dependencies or mounts, fetch failure and offline restart. Verify exclusion and manual
closures after a subsequent checked-in update. Record deployment evidence and complete the
week-long soak before claiming the existing H5 deployment milestone.

### 4. Verify recovery, publication and operator visibility

These are qualification checks for existing behaviour, plus any changes introduced by scopes
1 and 2. Keep the checks tied to the campaign so a green unit suite cannot substitute for worker
acceptance.

| Check                     | Required evidence                                                                                                                                                                     |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Single worker and restart | Concurrent ticks cannot overlap replays; stopping the service stops descendants; completed units keep their attempts and interrupted work recovers                                    |
| Resource and pause gates  | Pause, disk and daily budget gates stop new units; timeout and retry limits remain bounded                                                                                            |
| Changed or corrupt inputs | Source, tuning, specification and reference changes follow supersession/refusal rules; corrupted completed evidence is not reused                                                     |
| Publication               | Credentials stay on the host; unavailable publication is visible and retryable without replay; a representative note reaches its draft PR and respects checks and review before merge |
| Operator visibility       | Heartbeat and last-run status expose selected and skipped work, commit, failures, pending manual work and publication state; the operator knows where to inspect them                 |

Run publication checks only when publication is configured. Observe reconnection and resume as
well as normal operation. Define who inspects status after an unattended interval, how a stalled
worker is recognised and how the timer is disabled or work paused before recovery. Evidence must
distinguish a fixture, a real corpus replay, NAS installation, publication and the soak.

## Sequencing and handoff

1. Inventory the actual worker, installed scheduler version, mounts and candidate prerequisites.
2. Agree the bounded selection and fetch policies; complete scope 1 before enabling a broad tick.
3. Complete only the compatibility and evidence work for the chosen first experiment. A single-slug
   run remains an available bounded preflight; its normal prerequisites still apply.
4. Demonstrate a representative run, restart and publication where configured, then enable the
   selected campaign and observe the timer. Keep the week-long soak as a later acceptance gate.

The Broderick logger and annotation planning can proceed in parallel. Its references and review
outcomes are separate inputs, not automatic harness products. This plan changes no approval,
closure, selection or deployed worker.

Implementation handoffs should retain these outcomes, dependencies and checks without prescribing
implementation code. Update the runbook and candidate specifications alongside any eventual code
changes, with behaviour tests in the same commit. Run the relevant executor and scheduler tests,
repository experiment/configuration validation and affected build checks before deployment.

Raw recordings, row-level data, logs, databases and campaign state stay outside the checkout.
Commit the edited deployment or experiment report with methods, numerical findings, provenance,
limitations and explicit unfinished gates. The Sunday demonstration and week-long soak are
separate milestones; neither establishes physical measurement accuracy.
