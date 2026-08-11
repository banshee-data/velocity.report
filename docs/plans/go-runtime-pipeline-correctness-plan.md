# Go runtime pipeline correctness and cleanup remediation plan

- **Status:** Active
- **Layers:** Go server, LiDAR replay/tracking pipeline, radar DB derivation, API capability reporting
- **Target:** v0.5.1-v0.5.7; runtime correctness gates land early, contract/API fixes follow in v0.5.3, and structural follow-through remains scheduled in the existing cleanup milestones.
- **Companion plans:** [lidar-clock-abstraction-and-time-domain-model-plan.md](lidar-clock-abstraction-and-time-domain-model-plan.md), [lidar-performance-measurement-harness-plan.md](lidar-performance-measurement-harness-plan.md), [lidar-architecture-foundations-fixit-plan.md](lidar-architecture-foundations-fixit-plan.md), [metrics-registry-and-observability-plan.md](metrics-registry-and-observability-plan.md), [unpopulated-data-structures-remediation-plan.md](unpopulated-data-structures-remediation-plan.md), [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md), [go-cmd-extraction-plan.md](go-cmd-extraction-plan.md), [lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)

## Motivation

The June 2026 cleanup review found a small set of serious runtime failures that are easy to hide behind otherwise successful runs: PCAP replay can start without creating an analysis run, analysis replay can preserve frame counts while dropping semantic processing, valid radar raw rows can stop transit derivation, LiDAR capabilities can remain permanently stuck at startup state, and VRLOG loading bypasses the repository's symlink-safe path validator.

These are not broad style or package cleanup items. They affect measurement provenance, replay evaluation, transit tables, API state, and local file boundaries. This plan is the remediation document for those findings and records how related cleanup plans should be combined or kept separate.

## Cleanup consolidation evaluation

| Backlog scope                                                                                        | Decision                                                                                 | Reason                                                                                                                                                                                                       |
| ---------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Access-control and localhost listener hardening in v0.5.1                                            | Keep as existing backlog work; reference as a boundary condition.                        | Listener defaults and access policy are release-hardening work. This plan only owns the local replay file-safety fix where pipeline handlers currently bypass shared validation.                             |
| Clock abstraction, replay/performance measurement, and LiDAR foundations in v0.5.2                   | Keep companion plans; absorb the replay correctness invariant here.                      | Clock injection, perf harnesses, and foundations are broader than this plan. This plan owns the invariant that persisted analysis output cannot be semantically throttled by wall time.                      |
| Metric registry and unpopulated data remediation in v0.5.3                                           | Keep companion plans; reference output/provenance dependencies.                          | Registry and unpopulated-column work define metric naming and data completeness. This plan ensures analysis runs and transit tables are trustworthy inputs to that work.                                     |
| Capabilities API redesign in v0.5.3                                                                  | Response-shape redesign delivered by #547; lifecycle correctness stays here.             | `/api/capabilities` now returns named `radar`/`lidar` maps and the web store/nav consume them. The remaining runtime bug is that production LiDAR still never advances beyond `starting` or reports `error`. |
| Go structural hygiene, SQL-boundary cleanup, silent error-drop cleanup, and cmd extraction in v0.5.6 | Keep structural cleanup plans; fold behaviour regressions here.                          | Moving SQL or cmd ownership before pinning behaviour risks preserving bugs. This plan adds regression tests and contracts; v0.5.6 moves the corrected code to better boundaries.                             |
| Background grid display, VRLOG seek index, and replay UX/stability in v0.5.7                         | Keep UX/stability plans; fold only VRLOG load safety and replay semantic integrity here. | Background rendering, seek indexing, and macOS replay UX are user-facing follow-through. This plan handles server-side correctness gates that those surfaces depend on.                                      |

## Consolidation Options

### Option A: minimal consolidation

Keep all existing cleanup docs unchanged and use this plan only for the five runtime findings.

**Pros:** Lowest churn; no risk of stealing ownership from established plans.

**Cons:** Leaves readers to infer how the runtime correctness work gates replay evaluation, metrics, and structural cleanup.

**Recommendation:** Not enough. It under-documents the sequencing risk.

### Option B: behaviour-first consolidation

Use this plan as the runtime correctness hub, fold in the serious behaviour fixes, and keep infrastructure/surface plans as companions.

**Pros:** Keeps the branch focused on remediation, removes the standalone operations-review artifact, and gives each existing plan a clear dependency boundary.

**Cons:** Requires this plan to stay explicit about handoffs so companion docs do not drift.

**Recommendation:** Adopt. This is the current plan structure.

### Option C: broad omnibus cleanup plan

Merge clock abstraction, performance harness, foundations fix-it, metrics registry, unpopulated data remediation, capabilities redesign, structural hygiene, cmd extraction, and replay UX into this plan.

**Pros:** One apparent source for all cleanup work.

**Cons:** Too large to execute cleanly, mixes runtime correctness with measurement infrastructure and frontend/macOS UX, and weakens the existing canonical docs already linked from the backlog.

**Recommendation:** Reject. It would obscure ownership and make the backlog harder to maintain.

## Current State

| Area                         | Current state                                                                                                                                                                                                   | Severity | Release view                                              |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | --------------------------------------------------------- |
| PCAP analysis default        | `handlePCAPStart` initializes analysis mode to true, then JSON/form parsing overwrites omitted values to false. Legacy sweep/client paths still send only `pcap_file`.                                          | Critical | v0.5.2 hotfix before relying on replay/HINT output        |
| Analysis replay throttling   | The tracking pipeline uses wall-clock `MaxFrameRate` throttling and publishes empty frames for throttled foreground frames. Blocking PCAP frame delivery only preserves cardinality, not semantic processing.   | Critical | v0.5.2, coordinated with clock abstraction                |
| VRLOG load path validation   | `handleVRLogLoad` uses string-prefix validation even though `internal/security.ValidatePathWithinDirectory` already handles symlink escape.                                                                     | High     | v0.5.2 local replay safety                                |
| Magnitude-only radar samples | Serial classification accepts magnitude-only raw rows, but transit derivation scans `ABS(speed)` into non-null `float64` after allowing rows with only magnitude.                                               | High     | v0.5.3 data-contract cleanup                              |
| LiDAR capability lifecycle   | #547 ships named capability maps and web gating. `SetLidarStarting` is wired in production, but `SetLidarReady` and `SetLidarError` are not. Radar remains a static built-in capability, not a hot-plug signal. | High     | v0.5.3 lifecycle follow-through after #547 response shape |

## Design Approach

Fix runtime correctness first, then fold ownership into existing cleanup streams.

1. Preserve production invariants with tests before refactoring package boundaries.
2. Keep PCAP analysis semantics explicit at API and client boundaries.
3. Treat persisted replay output as a data product: no wall-clock display throttle may silently change semantic content.
4. Make DB contracts match accepted ingest shapes.
5. Use existing shared safety helpers rather than parallel path checks.
6. Let v0.5.6 structural work move code after behavior is pinned down.

## Scope

### Phase 0: remediation scoping

**Summary:** Consolidate serious cleanup findings into this plan and remove standalone operations-review documentation from the branch.

**State:** Complete.

**Steps:**

1. Keep the remediation findings in this plan rather than a separate operations review.
2. Exclude minor style issues and cleanup already clearly scheduled in the backlog.
3. Identify which related cleanup docs remain companions and which behavior fixes this plan absorbs.

**Milestone:** v0.5.2 planning complete.

### Phase 1: replay provenance and semantic-output gate

**Summary:** Make PCAP analysis reliably create analysis runs and process foreground frames semantically.

**State:** Active.

**Steps:**

1. Change `analysis_mode` request parsing to preserve the intended default when the field is omitted. Use a pointer bool or explicit field-presence parsing.
2. Update simple PCAP clients and legacy sweep paths so their replay intent is explicit.
3. Add handler/client tests for omitted JSON `analysis_mode`, explicit false, and legacy simple-client replay.
4. Add an analysis replay mode signal to the tracking pipeline.
5. Disable wall-clock downstream throttling for persisted analysis output, or replace it with replay-time semantics that never converts foreground frames into empty semantic frames.
6. Add tests proving rapid foreground frames in analysis mode still reach clustering/tracking/recording paths.

**Milestone:** v0.5.2.

### Phase 2: local replay file-safety gate

**Summary:** Reuse the symlink-aware path validator for VRLOG loads.

**State:** Active.

**Steps:**

1. Replace `handleVRLogLoad` string-prefix validation with `security.ValidatePathWithinDirectory`.
2. Decide whether the loader should receive the original absolute path or a canonical resolved path, then document that invariant in the handler test.
3. Add tests for direct `vrlog_path` symlink escape and `run_id` lookup returning a symlink-escaped stored path.
4. Keep ordinary absolute paths under `vrlogSafeDir` working.

**Milestone:** v0.5.2.

### Phase 2b: exposure and local access boundary check

**Summary:** Keep this plan aligned with the v0.5.1 listener/access-control hardening without moving that release work.

**State:** Scheduled elsewhere.

**Steps:**

1. Verify the VRLOG and PCAP replay handler safety assumptions after the main and LiDAR listeners default to localhost.
2. Confirm that any future LAN exposure keeps replay file loading behind the same local safe-directory and symlink-validation rules.
3. Do not add command allowlisting work here; the v0.5.1 backlog item already owns the access-control model.

**Milestone:** v0.5.1 through existing backlog item #461.

### Phase 3: radar raw-data to transit contract

**Summary:** Align accepted raw radar payload shapes with transit worker expectations.

**State:** Scheduled.

**Steps:**

1. Decide whether magnitude-only rows are stored diagnostics only or analytic transit inputs.
2. If speed is required for transits, filter transit derivation on `speed IS NOT NULL` and keep magnitude-only rows out of transit clustering.
3. If magnitude-only rows are analytic inputs, scan speed as nullable and define clustering behavior without speed.
4. Add regression tests with magnitude-only rows in otherwise valid transit windows.
5. Document the ingest and transit contract in the radar operations docs or DB plan notes.

**Milestone:** v0.5.3.

### Phase 4: current capability lifecycle

**Summary:** Make the current `/api/capabilities` state truthful now that #547 has shipped the named-map response shape.

**State:** Scheduled.

**Current shipped baseline:** `/api/capabilities` returns non-null named maps:
`radar.default` is reported as enabled/`receiving`, LiDAR is omitted as `{}` when
disabled, and LiDAR is reported as `lidar.default.status = "starting"` when
`--enable-lidar` constructs the LiDAR server. The endpoint does not detect radar
disconnect/reconnect.

**Steps:**

1. Wire LiDAR startup success to `SetLidarReady(true)` once the server and sweep routes are usable.
2. Wire startup/listener failure paths to `SetLidarError`.
3. Add tests for enabled-ready, enabled-error, and disabled states.
4. Add hardware smoke guidance for release candidates: radar-only should return
   `lidar: {}` and hide LiDAR nav; `--enable-lidar` should add `lidar.default`
   and show LiDAR nav; radar disconnect/reconnect should not be treated as
   supported lifecycle validation until radar hot-plug state exists.

**Milestone:** v0.5.3.

### Phase 5: metrics and data-completeness handoff

**Summary:** Ensure the corrected runtime outputs are suitable inputs for metric registry and unpopulated-data remediation work.

**State:** Scheduled elsewhere.

**Steps:**

1. Treat analysis-run creation and `last_run_id` provenance as preconditions for run statistics and comparison surfaces.
2. Keep metric names and output contracts aligned with [metrics-registry-and-observability-plan.md](metrics-registry-and-observability-plan.md).
3. Keep `statistics_json`, track quality columns, and comparison outputs in [unpopulated-data-structures-remediation-plan.md](unpopulated-data-structures-remediation-plan.md); this plan only guarantees the replay/transit inputs are valid.

**Milestone:** v0.5.3 through existing backlog items.

### Phase 6: structural follow-through

**Summary:** Move the corrected ownership boundaries into the existing hygiene and cmd-extraction work.

**State:** Scheduled elsewhere.

**Steps:**

1. When [go-cmd-extraction-plan.md](go-cmd-extraction-plan.md) moves `capabilitiesProvider`, carry the lifecycle tests with it.
2. When [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md) continues SQL/query-boundary cleanup, keep the transit worker contract test as a non-regression gate.
3. When the clock-abstraction plan injects clocks, preserve the Phase 1 invariant that persisted analysis output is not semantically throttled.

**Milestone:** v0.5.6 through existing backlog items.

### Phase 7: replay UX and stability handoff

**Summary:** Keep server-side replay correctness separate from frontend/macOS replay UX improvements.

**State:** Scheduled elsewhere.

**Steps:**

1. Leave background grid display repair, VRLOG timestamp indexing, and seek diagnostic logging in the existing v0.5.7 replay UX/stability backlog.
2. Use Phase 1 and Phase 2 tests as server-side gates before relying on visual replay surfaces for analysis validation.
3. Keep replay UX work free to optimize display and seek behavior without changing persisted analysis semantics.

**Milestone:** v0.5.7 through existing backlog items.

## Dependencies

- Phase 1 should land before side-by-side replay evaluation or HINT scoring is treated as trustworthy.
- Phase 1 and the clock-abstraction plan touch the same throttle boundary; behavior should be pinned with tests before clock injection broadens the surface.
- Phase 4 builds on #547's named-map capability API. It owns runtime truth for
  LiDAR ready/error transitions, not another response-shape redesign.
- Phase 5 does not need a new backlog item because existing v0.5.3 metric and data-completeness plans already own the broader output surfaces.
- Phase 6 does not need a new backlog item because existing v0.5.6 cleanup work already owns the package and cmd-boundary changes.
- Phase 7 does not need a new backlog item because existing v0.5.7 replay UX/stability work already owns visual replay follow-through.

## Risks

| Risk                                                                                  | Likelihood | Impact | Mitigation                                                                                                     |
| ------------------------------------------------------------------------------------- | ---------- | ------ | -------------------------------------------------------------------------------------------------------------- |
| Disabling replay-analysis throttling increases CPU during PCAP analysis               | Medium     | High   | Backpressure frame processing in analysis mode and measure with the v0.5.2 perf harness.                       |
| Changing `analysis_mode` defaults breaks callers that relied on omitted meaning false | Low        | Medium | Keep explicit false supported and make client intent explicit.                                                 |
| Transit contract choice changes historical transit counts                             | Medium     | Medium | Add migration notes and compare before/after windows with magnitude-only rows.                                 |
| Capability lifecycle wiring races startup ordering                                    | Medium     | Medium | Drive the provider from explicit readiness/failure callbacks rather than assuming goroutine start means ready. |
| Symlink-aware validation changes accepted VRLOG paths                                 | Low        | High   | Keep normal in-tree absolute paths working and reject only canonical escapes.                                  |

## Checklist

### Complete

- [x] Serious runtime findings captured directly in this remediation plan.
- [x] Existing backlog scope reviewed before adding new work.
- [x] Targeted package tests run where this worktree permits them.
- [x] Standalone operations-review scope folded into this remediation plan.

### Outstanding

- [ ] Phase 1: PCAP analysis default and semantic replay gate (`M`)
- [ ] Phase 2: VRLOG symlink-safe validation (`S`)
- [ ] Phase 3: magnitude-only radar transit contract (`S`)
- [ ] Phase 4: LiDAR capability lifecycle wiring (`S`)

### Deferred

- [ ] Phase 2b exposure/access boundary: covered by v0.5.1 backlog item #461.
- [ ] Phase 5 metrics/data-completeness handoff: covered by [metrics-registry-and-observability-plan.md](metrics-registry-and-observability-plan.md) and [unpopulated-data-structures-remediation-plan.md](unpopulated-data-structures-remediation-plan.md).
- [ ] Phase 6 structural movement: covered by [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md) and [go-cmd-extraction-plan.md](go-cmd-extraction-plan.md).
- [ ] Phase 7 replay UX/stability: covered by [lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md) and related v0.5.7 backlog items.

### Accepted Residuals

- [ ] No new backlog item for broad package cleanup. Existing v0.5.6 items already own that work.
- [ ] No broad merge of metric registry, unpopulated data, clock abstraction, performance harness, foundations, or replay UX docs into this plan. They remain separate because they own infrastructure and surfaces beyond runtime correctness.
