# Tenets review: questions for a refactor sequence

This first pass records places where the current code puts pressure on the project's tenets and
asks an oracle to decide which simplifications are worth making, in what order. It is an input to
existing plans, not an approved design, a new backlog, or a claim that every candidate is a defect.

## Scope and decision requested

The review sampled Go configuration, pipeline, storage, server and experiment execution, plus the
Svelte operator interface at commit `d7db542d7c2d77dce4390c31fd7cd611e80ece2d`. It did not
review every package, deployment path or client. In particular, it is not a privacy, security,
offline-operation or physical-validation audit; no absence of findings is evidence of compliance.
The observations below are source reads, not measurements of frequency, performance, or field
impact. The tenet links explain the tension; they do not establish that every design is forbidden.
The six commits through `ab01c6bb089ef573cc9b327cfadd1e1eb7dd7eda` were checked for changes
to the cited paths: the harness gained reference preflight, while the execution seam cited below
remains. The rest of the review is still a sample of the earlier commit.

The earlier [architecture review](../../plans/platform-architecture-review-plan.md) covers many
of the same seams at an older commit and records decisions and corrections. This note rechecks
selected examples and adds questions around newer evidence and the experiment harness. Reconcile
answers into the existing owning plans and [backlog](../../BACKLOG.md), consistent with
[D-32](../../DECISIONS.md#d-32--planning-limits-and-backlog-governance), rather than starting a
parallel refactor programme. [D-30](../../DECISIONS.md#d-30--one-binary-closed-roles-kinds-compiled-in)
and [D-31](../../DECISIONS.md#d-31--renames-are-cutovers-tombstones-for-people-aliases-for-the-app)
are constraints, not questions to reopen implicitly.

Ask the oracle to challenge the findings, then return a small, dependency-ordered series of PRs.
For each, name the owning existing plan, scope and explicit non-goals, prerequisite decisions,
observable invariant, tests or evidence, compatibility and rollback/cutover, and what old code or
state can be removed. Identify items that should be left alone, and the evidence that would change
that judgement. Do not infer a release slot from this note.

## Candidates and questions

### 1. Defaults depend on where a caller happens to run

**Evidence.** [`MustLoadDefaultConfig`](../../../internal/config/tuning.go#L569) tries five
working-directory-relative paths before the embedded default, discarding every load error along the
way. Layer constructors in [L3](../../../internal/lidar/l3grid/config.go#L67),
[L4](../../../internal/lidar/l4perception/cluster.go#L295),
[L5](../../../internal/lidar/l5tracks/tracking_config.go#L519) and
[L6](../../../internal/lidar/l6objects/classification.go#L129) call it. A valid file found
first can therefore win over the embedded bytes, while an invalid one can be skipped. The
[other loader](../../../internal/config/tuning.go#L534) explicitly distinguishes a missing file
from another error. The [root assets](../../../assets.go#L13) also embed web, docs and defaults
together. This is a current example of the defaults seam in the
[architecture review](../../plans/platform-architecture-review-plan.md#c6-boundaries-that-are-nominal).

**Tenets and consequence.** [Evidence over opinion](../../../TENETS.md#3-evidence-over-opinion),
[simplicity](../../../TENETS.md#5-simplicity-and-durability) and
[DRY](../../../TENETS.md#6-dry): one canonical file exists, but selection and failure behaviour
are implicit at multiple construction sites. Different effective bytes can undermine reproducibility;
this read does not establish that a recorded run was affected.

**Questions.** Should the composition root resolve and validate defaults once and inject the
appropriate typed sections, or should a smaller defaults provider remain available to tools and
tests? Which explicit operator override semantics must survive, and should a present but invalid
candidate fail rather than fall through? Can defaults be separated from web/doc assets without
creating another maintained copy? Which callers and fingerprints need migration?

**Evidence needed for a PR.** Exercise a binary outside the checkout, an explicit valid and invalid
file, and a conflicting working-directory file; compare effective config and recorded fingerprints.
Preserve offline operation and existing checked-in experiment parameter semantics.

### 2. The SQL boundary names storage but exposes the connection

**Evidence.** [`sqlite.SQLDB` and `SQLTx`](../../../internal/lidar/storage/sqlite/dbconn.go#L18)
are aliases for `database/sql` types. [`segments.LoadEstimates`](../../../internal/lidar/segments/read.go#L14)
opens a read snapshot and contains SQL and version-selection rules;
[`LoadRun`](../../../internal/lidar/segments/read.go#L92) contains another query. The
[pipeline config](../../../internal/lidar/pipeline/tracking_pipeline.go#L220) also accepts a
concrete SQL connection and a SQLite run manager. The import rule is real, but an import check by
itself cannot prevent query or transaction ownership from spreading outside storage.

**Tenets and consequence.** [Simplicity](../../../TENETS.md#5-simplicity-and-durability) and
[DRY](../../../TENETS.md#6-dry): schema changes and ownership are harder to follow across the
boundary. Some domain validation is deliberately near the query; its existence alone is not a
reason to move it or to introduce a generic repository framework.

**Questions.** Which seam gives the smallest useful boundary: move specific queries into the
existing SQLite stores, define consumer-owned narrow read/write interfaces, or retain direct SQL
for analytical tools? Who owns the single read snapshot across source, version and rows? Should a
new boundary check constrain capabilities or just document exceptions? How does this relate to the
[structural hygiene plan](../../plans/go-codebase-structural-hygiene-plan.md) and the earlier
architecture review's SQL work, rather than duplicating them?

**Evidence needed for a PR.** Preserve snapshot consistency, source/version refusal, NULL handling,
ordering, cancellation and query results on representative fixtures. Check the actual import and
query graph before and after; a passing import-only lint is not proof of isolation.

### 3. A frame callback combines orchestration, persistence and failure policy

**Evidence.** [`TrackingPipelineConfig`](../../../internal/lidar/pipeline/tracking_pipeline.go#L220)
holds mutable runtime dependencies, optional evidence sinks and a SQL connection.
[`NewFrameCallback`](../../../internal/lidar/pipeline/tracking_pipeline.go#L514) builds a long
closure which runs the foreground and subsequent stages, publishes and writes records. Its
[legacy track transaction](../../../internal/lidar/pipeline/tracking_pipeline.go#L1292) logs
write/commit failures; the separate [frame evidence sink](../../../internal/lidar/pipeline/tracking_pipeline.go#L192)
can retain failures for strict offline replay. The two paths deliberately have different contracts;
the comments explain that observations are immutable and estimates can be recomputed.

**Tenets and consequence.** [Evidence](../../../TENETS.md#3-evidence-over-opinion) and
[simplicity](../../../TENETS.md#5-simplicity-and-durability): following a frame's completeness and
failure outcome requires reading several optional paths. The code shows different reporting paths;
this review has not established whether a caller can observe every live persistence failure.

**Questions.** What is the smallest explicit frame result or error boundary that distinguishes
live best-effort operation from strict replay? Should a pipeline-owned frame record be handed to a
persistence adapter, or would that add allocations and coupling without solving the real problem?
Which stages can be extracted without changing frame order, snapshot semantics or timing? How
should transaction failure be surfaced to operators and evidence consumers without turning a
recoverable live failure into a sensor outage?

**Evidence needed for a PR.** Characterise empty, dropped and matched frames; inject preparation,
insert and commit failures; compare ordered observation/estimate records and provenance, and test
live and strict replay separately. Measure allocations and target-device throughput before changing
the hot path. Keep the existing observation-versus-estimate and transaction guarantees.

### 4. The serve composition root also owns operational workflows

**Evidence.** [`server.Main`](../../../internal/cmd/server/radar.go#L323) dispatches commands,
sets up logging and services, and installs recording and replay callbacks before waiting for
shutdown. For example, its [recording-stop callback](../../../internal/cmd/server/radar.go#L913)
detaches the recorder, ignores the `Close` error and returns a path. The LiDAR HTTP server exposes
[start/stop operations](../../../internal/lidar/server/datasource_handlers.go#L81) for the live
listener and replay. The existing [server decomposition plan](../../plans/cmd-server-decomposition-plan.md)
and [command extraction plan](../../plans/go-cmd-extraction-plan.md) describe overlapping work.

**Tenets and consequence.** [Simplicity](../../../TENETS.md#5-simplicity-and-durability) and
[evidence](../../../TENETS.md#3-evidence-over-opinion): lifecycle and recording outcomes are
spread across wiring and callbacks. A returned recording path after a close failure is a risk to
investigate, not proof that a corrupt recording has been accepted.

**Questions.** Should the first change extract lifecycle phases inside the same package, or move
recording/replay ownership behind a small service boundary before splitting `Main`? Which plan
owns the work and which overlapping instructions are stale? What should a failed recorder close
mean to the API, run state and operator? Is a supervisor warranted by actual failure behaviour, or
would explicit start/stop functions suffice under D-30's single-binary role model?

**Evidence needed for a PR.** Exercise start failure, stop/close failure, cancellation and restart,
including live-to-replay transitions and recording provenance. Preserve flag, CLI and API behaviour
unless an intentional cutover is documented; do not equate fewer lines with a better boundary.

### 5. Operator clients conflate unavailable evidence with empty evidence

**Evidence.** The Windows page's [`loadPacks`](../../../web/src/routes/lidar/windows/+page.svelte#L192)
turns any inventory request failure into an empty array. Its
[status selection](../../../web/src/routes/lidar/windows/+page.svelte#L150) then falls back to
segment status and the [inventory section](../../../web/src/routes/lidar/windows/+page.svelte#L750)
is absent. On the same page, a workspace failure is explicitly described as
[unknown rather than zero](../../../web/src/routes/lidar/windows/+page.svelte#L200).
Meanwhile [`api.ts`](../../../web/src/lib/api.ts#L139),
[`lidarControl.ts`](../../../web/src/lib/lidarControl.ts#L22), and the page's
[local helper](../../../web/src/routes/lidar/windows/+page.svelte#L183) format or parse errors
differently. Inventory also loads asynchronously, so loading and empty share the initial state.

**Tenets and consequence.** [Evidence](../../../TENETS.md#3-evidence-over-opinion) and
[DRY](../../../TENETS.md#6-dry): an unavailable inventory can look like no inventory or a less
advanced status. This is a source-level failure scenario, not a measured rate of operator error.

**Questions.** Should the page expose loading, unavailable and empty separately while allowing
ranking to remain usable? Is a shared transport/error helper across the clients worth its migration
cost, or should only the inventory state be fixed first? Which server messages and response types
must remain visible, and how should concurrent reloads avoid displaying stale status?

**Evidence needed for a PR.** Test delayed, empty, failed and successful inventory responses,
recovery and overlapping loads; confirm the selected status and inventory presentation do not
claim absence on failure. Preserve the usable ranking and existing actionable server errors.

### 6. Compatibility is necessary but its retirement needs a release fact

**Evidence.** [Routes](../../../internal/lidar/server/routes.go#L103) retain old replay-stop
names; [annotation routes](../../../internal/lidar/server/routes.go#L209) retain older paths.
These are marked through [`AliasRoute`](../../../internal/compatibility/compatibility.go#L80)
and listed in the [compatibility register](../operations/compatibility-register.md). The register
names older macOS DMGs as consumers and expresses several removal conditions in terms of the
oldest supported DMG, not a numeric version. D-31 explicitly allows such aliases while a
separately shipped app needs them. Their presence is therefore not itself a violation.

**Tenets and consequence.** [Simplicity](../../../TENETS.md#5-simplicity-and-durability),
[DRY](../../../TENETS.md#6-dry) and [cohesive releases](../../../TENETS.md#7-cohesive-releases):
each surviving contract has a cost, but removing it before the supported app moves would break a
real release boundary. The missing fact in this review is which DMGs are currently supported.

**Questions.** Where is the authoritative oldest-supported DMG recorded, and how does a release
make the register's conditional removal actionable? For each family, can the team prove both
current in-repo consumers and supported external consumers have moved? Should retirement be one
bounded change per contract family or tied to a client release? Which redirects or data migrations
remain necessary under D-31?

**Evidence needed for a PR.** Inventory supported app releases and scripts, check register entries
and matching handlers, and exercise old and new clients through the declared support window.
Remove only aliases whose conditions are met; retain a migration note or tombstone where required.

### 7. Experiment execution has two local coordination models

**Evidence.** The [experiment harness](../../../internal/experiments/harness/execute.go#L28)
validates supported steps and runs fixed tools, importing shared `jobs/toolargs` for part of its
argument contract. The [worker runner](../../../internal/lidar/jobs/runner/store.go#L29) keeps
attempts and job identity in a directory-backed store. The harness has its own ledger, pinned
inputs, retry and publication rules in its [runbook](../operations/experiment-harness.md).
They solve overlapping execution problems but have different responsibilities; sharing arguments
alone does not establish duplicate state or that either can be deleted.

**Tenets and consequence.** [Evidence](../../../TENETS.md#3-evidence-over-opinion),
[local-first](../../../TENETS.md#4-local-first-offline-capable) and
[simplicity](../../../TENETS.md#5-simplicity-and-durability): independently evolving validation,
identity and retry rules can make provenance harder to compare. The harness was intentionally
single-host and the worker supports a distinct contract; a consolidation could weaken both.

**Questions.** Which concepts are actually the same (kind, arguments, identity, attempt, retry,
evidence bundle), and which are intentionally different? Under D-30's one implementation and
one claim rule, should the harness submit to the worker/job contract, share only a typed execution
primitive, or stay separate as an experiment ledger that orchestrates jobs? What is the smallest
step that avoids a hub or distributed system before it is needed? How do the
[harness plan](../../plans/platform-experiment-harness-plan.md) and
[job queue plan](../../plans/lidar-job-queue-implementation-plan.md) reconcile?

**Evidence needed for a PR.** Map real consumers and persisted state before choosing a boundary;
compare pinned code/config/capture identities, eligibility and approval gates, cancellation,
crash recovery, retries and bundle verification on equivalent synthetic runs. Preserve offline
execution, raw-output locality and the distinction between a run note and an authored report.

## Ordering hypotheses for the oracle to test

1. Characterise failure and identity contracts first: defaults, frame persistence, recorder close
   and the inventory UI can each be tested without a repository-wide rewrite. These are candidates
   for independent small PRs, not a predetermined priority order.
2. Decide query/transaction ownership before moving pipeline persistence; otherwise a callback
   split may freeze the existing SQL leak into a new interface. Decide lifecycle ownership before
   relocating replay and recording callbacks. Either dependency may be disproved by a narrower
   patch.
3. Map actual harness and worker consumers before unifying execution. Retire aliases only after
   the supported-client condition is evidenced. Neither needs to block unrelated correctness work.
4. Use the existing plans and decision register as the final home for accepted decisions; order
   cross-cutting moves as reversible, reviewable slices with a test or comparison at each seam.

Ask the oracle to rank expected benefit and uncertainty, identify genuine dependencies versus
convenient grouping, and provide a stop rule for each candidate. It should say explicitly if the
best refactor is no refactor, and whether any higher-priority tenet constrains a proposed shortcut.
