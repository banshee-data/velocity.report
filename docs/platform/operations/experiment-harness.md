# Experiment harness

- **Status:** Implemented runner and deployment files; NAS deployment and soak pending
- **Plan:** [Unattended experiment harness](../../plans/platform-experiment-harness-plan.md)
- **Next steps:** [Bounded queue, compatibility and deployment acceptance](../../plans/platform-experiment-harness-next-steps.md)

`velocity experiments` runs the mechanical steps of approved experiments on
arrow-worker without an agent session. It leaves interpretation, manual work,
reports, approval and adoption with the experiment author and owner.

## Run and inspect

Use the [host profile and installation files](../../../deploy/harness/README.md).
Keep captures read-only and all state, logs, generated configurations, recordings
and databases outside the checkout. Run one replay at a time.

```bash
velocity experiments tick -root /var/lib/velocity-harness/repo -host /var/lib/velocity-harness/host.json -dry-run
velocity experiments run ground0001 -root /var/lib/velocity-harness/repo -host /var/lib/velocity-harness/host.json
velocity experiments status -host /var/lib/velocity-harness/host.json
```

`run` selects one slug; `tick` selects every eligible slug, ranked by downstream
machine time, age and slug. Selection never changes a spec's approval. A clean,
checked-in local `main` is required. A dirty checkout or a different local head
refuses execution; no reset or stash hides local edits. Each unit checks Git again
before starting. The host status records the observed commit and skip reasons.

With `fetch_on_tick: true`, a tick fetches `origin/main` and fast-forwards a clean
local `main`. It never creates a merge commit. A network failure is recorded and
execution continues from the checked-in local revision. A failed fast-forward is
also reported; the operator resolves divergent history. `-offline` disables fetch
and publication; `-no-publish` disables publication while allowing fetch. A dry run
selects without fetching, building, replaying or publishing.

The timer's service account needs its exact Go toolchain and dependencies cached.
Executors are built through `make build-experiment-tools` into
`<evidence_root>/builds/<commit>/bin/`; each reports `-version`. Cache reuse checks
binary hashes and the pinned source commit. The worker and harness share the
state-estimation tool's argument builder. The CLI is in the `velocity` binary;
replay/evaluation tools remain separate stamped binaries, as the plan's interim
executor contract permits.

### Evidence on a share that refuses `O_EXCL`

The arrow NAS (NFSv3) refuses `chmod` and exclusive creates, which breaks `git worktree add`
and the executors' write-once files. Three host settings keep evidence there:

- `build_root` (absolute, optional) holds `builds/<commit>` on local disk instead of under
  `evidence_root`. The build cache is rebuildable.
- `stage_attempts: true` runs each attempt in `<stage_dir>/<run>/work` and copies the
  finished attempt to the evidence root with synced, non-exclusive writes. A failed copy fails
  that attempt and keeps the staged files.
- The harness's own records use `experiments.CreateTemp`, a checked random name without
  `O_EXCL`.

The build version is the `VERSION` file when a checkout carries one, else the Makefile's
`VERSION`. Executors receive `PATH`, `HOME`, `TMPDIR`, `LANG`, `LC_ALL`, `LD_LIBRARY_PATH` and
the eligibility settings `VELOCITY_COHORT_MANIFEST`, `VELOCITY_ANNOTATION_ROOT` and
`VELOCITY_ELIGIBILITY_DB`. Without a cohort manifest that assigns a capture and a use ledger,
development replays are refused; the harness does not assign cohorts.

## Spec compatibility and dependencies

Unversioned specs use the original contract. `spec_version: 1.0` is strict;
`1.<later minor>` accepts additional descriptive fields while keeping the understood
execution fields. A different major version is skipped before decoding the rest of
its header. Unsupported kinds, unknown execution parameters, stale tuning
fingerprints and unavailable capabilities are visible skips for that experiment;
they do not stop unrelated eligible experiments. Repository `validate` remains
strict so malformed current specs are caught in CI.

A manual step may carry `closed`. Implementation closures are a commit reachable
from local `main` or a squash PR reference such as `"#1006"`; decisions are `D-NN`
entries in `docs/DECISIONS.md`; capture, annotation and frozen-split closures use
`sha256:<64 hex digits>`. Reports use the existing report path. The validator checks
these locally. An unclosed manual predecessor blocks its dependent machine units;
the harness never closes it itself. Independent units can still run.

`depends_on` requires an answered predecessor. `needs_runs: [slug]` requires a
successful, complete run note checked into the local repository. A failed or
incomplete note does not release a data dependency. Fetching after a remote merge
makes that note available to the next scheduling pass.

## Executors and parameters

Arms always name checked-in tuning files and their pinned fingerprints. Corpus
and index files are checked-in inputs, mapped by the host profile. The first
segment manifests select one capture from each of 23 screening sites, excluding
the held-out Embarcadero–Folsom site. Synthetic `case01` names do not identify an
archive capture: unattended corpus specs must name their sites explicitly.

| Kind                        | Supported parameters                                                     | Evidence and headline measures                                                                                                               |
| --------------------------- | ------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `state_estimation_baseline` | `ground_report`, `surface_ground`, `keep_evidence`, `warmup`, `duration` | Source hashes, deterministic summary, campaign metrics and spurious-cluster windows; frames, duration and confirmed tracks                   |
| `box_eval`                  | `lower_body`, `warmup`, `duration`                                       | Box summary and observations database; scored frames, confirmed tracks and wall time                                                         |
| `ground_truth_eval`         | `split_role` (`tuning` or `held_out`), `retention`                       | Frozen-reference paired per-frame score; HOTA, IDF1, MOTA, switches, fragmentations; optional pooled retention, fragmented and merged shares |
| `corpus_screen`             | `measures`                                                               | By default, the matching baseline's complete steady spurious-cluster window and lasting-track count                                          |

A named corpus screen may select `observed_clusters`, `frames_with_observations`
and `estimated_tracks` from the matching box replay's database. These count L4
observations, frames with observations and online track identities, respectively;
they do not establish confirmed vehicle counts or physical accuracy. The default
screen refuses an incomplete steady window and a box replay without a named
measure list. It never substitutes an unavailable measure with zero.

A `ground_truth_eval` host reference is keyed by `<slug>/<step>/<arm>` or
`<slug>/<step>` and contains `pack`, `manifest`, `split`, `a_db` and `b_db`. The
pack is reviewed truth, the manifest is its frozen split, and the two absolute
DB paths bind explicit, previously produced baseline and candidate estimate
versions. The evaluator's existing single-version and frozen-reference checks
remain authoritative. The harness does not invent a kirk replay or choose a
baseline from unrelated unit output. Configure these bindings before approving a
spec for unattended scoring. Use `retention: true` only with suitable retained
observation samples; the evaluator refuses inadequate evidence.

## Recovery and changed check-ins

The kernel lock covers the evidence root. Executor processes inherit it, so a
replay that survives a killed parent still prevents overlapping work. Under
systemd, `KillMode=control-group` also stops every descendant. Atomic, synced unit
records and per-attempt histories survive restarts. The ledger identifies the
unit by step, arm and case.

Each attempt has a fresh output directory. Failed staging databases are copied and
synced to that attempt's evidence directory before local scratch is removed. A
failed archive retains the source, and the next tick retries the archive before
starting another replay. A unit times out at three times its
estimate, and gets at most three attempts. A failed unit does not block unrelated
units. On restart, the original worker can recover an interrupted attempt under
the host lock when `reclaim` permits it; cross-worker reclaim also respects the
spec's policy and stale threshold. `reclaim: never` leaves recovery to the operator.
Completed units retain their original attempt and evidence. Their completion
receipts and summary digests are verified before reuse or publication; corrupt evidence refuses
reuse and remains available for investigation.

After correcting a failure, explicitly allow three more attempts without losing
successful units:

```bash
velocity experiments run host0001 -root /var/lib/velocity-harness/repo -host /var/lib/velocity-harness/host.json -retry-failed
```

The run stays at its pinned tool commit when a new check-in leaves the spec,
parameter files and data mappings unchanged. A spec, parameter file or corpus
manifest change supersedes remaining work and starts a fresh run. Source capture
hashes and frozen scoring inputs are checked before execution; changed bytes
refuse an attempt. Connectivity settings, budget changes and pause switches do
not reset the run's measurement identity.

Disk gates check both staging and evidence storage before every unit. The daily
machine-hour budget includes every retry's attempt history. A gate stops starting
work and records why; it does not sleep indefinitely. Create
`<evidence_root>/state/PAUSE`, or check in `config/harness.json` with `paused: true`,
to stop after the current unit. Remove the pause to resume on the next tick.
`status/heartbeat.json` describes the scheduling pass; `status/last-run.json`
contains current progress, publication status and any publication error.

## Notes and connected publication

A run note contains pinned provenance, arm fingerprints, named headline measures,
per-unit wall time and estimate, attempt counts, failures and summary digests.
It is capped at 64 KiB and lives initially at
`<evidence_root>/notes/<slug>/<run-id>.md`. It is a measurement record, never the
experiment's interpretive report. Raw output remains under the evidence root.

Configure `publish` in the host profile to use a repository-scoped GitHub App.
The worker mints a short-lived installation token from the local RSA key. It
creates a `harness/<slug>/<run-id>` branch at the pin and an initial draft PR as
the visible claim. At step boundaries it updates only the one note under
`data/experiments/runs/<slug>/<run-id>.md`. A branch containing any other change
refuses publication. Network errors are retained in status and retried on a later
tick without discarding local evidence.

Once machine work is complete, the PR becomes ready. The worker waits across
ticks for checks on the exact head SHA, at least one completed check, successful
commit statuses, and a review query showing no unresolved thread or requested
changes, with any required approval satisfied. More than 100 checks or review threads require operator handling. It
then squash-merges with a `Harness-Run` and `Signed-off-by` trailer. A lost merge
response is recovered from the existing merged PR before any branch is recreated.
Failed and blocked runs retain their draft PR so recovery can update the same note.
No report or adoption is merged by this mechanism.

Without publication, copy a finished note into
`data/experiments/runs/<slug>/<run-id>.md`, review it, run
`velocity experiments check-note <path>` and `make check-no-results
check-harness-notes`, then check it in normally. Checked-in notes are write-once.
The `harness-pr.yml` guard rejects edits to existing notes and any other path on
a `harness/` branch. Do not publish an in-progress note manually: later changes
would violate that contract.

The local recovery, scheduling, Git sync, App publication gates and note guard
have automated tests. The NAS installation, real App credentials, reproduced
report bands, a held-out scoring acceptance and the week-long deployment soak
still require their own recorded evidence. These are not established by runner
unit tests.
