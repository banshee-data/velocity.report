# Prioritized Upcoming Work (From Full `docs/` Review) - 2026-02-17

## Scope

This backlog is based on:

1. The architecture/readability review in `docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md`
2. A review of all documentation under `docs/`

## Triage Method

Prioritization used four signals:

1. User impact on current deployments
2. Dependency unlocking (work that unblocks many other docs)
3. Delivery risk if delayed
4. Status confidence (active vs stale vs explicitly deferred)

## Snapshot of Outstanding Work

- Open checklist items across `docs/`: **614**
- By area:
  - `docs/lidar/*`: **296**
  - `docs/plans/*`: **230**
  - `docs/features/*`: **88**
- Highest checklist density:
  - `docs/plans/hint-sweep-mode.md` (57)
  - `docs/plans/frontend-consolidation.md` (53)
  - `docs/features/time-partitioned-data-tables.md` (49)
  - `docs/plans/industry-standard-ml-solver-expansion-plan.md` (44)

## Priority Queue

### Docs-only branch tasks (current) — ✅ Complete

1. ✅ `arena.go` deprecation and layered model relocation design
   - Doc: `docs/lidar/architecture/arena-go-deprecation-and-layered-type-layout-design-20260217.md`
   - Outcome: arena.go deleted, all active types migrated to layer packages.
2. LiDAR logging stream split (`ops`/`debug`/`trace`) with routing rubric
   - Doc: `docs/lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md`
   - Outcome: design approved, migration planned. Implementation is future work.

## P0 (Start Now) — ✅ Complete

### 1. ✅ Establish L1-L6 code boundaries and orchestration cleanup

- Source: `docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md`
- Status: **Complete.** All implementation code migrated to layer packages (l1packets→l6objects, pipeline, storage/sqlite, adapters). All shim files removed. Arena.go deprecated. All callers updated to import from layer packages directly.

### 2. ✅ Simplify HTTP API wiring and route registration

- Sources:
  - `docs/plans/frontend-consolidation.md` (Phase 0 and Phase 5)
  - `internal/lidar/monitor/webserver.go` route sprawl
- Status: **Complete.** `RegisterRoutes` converted to grouped `[]route` slices. Method-pattern dispatch and middleware wrappers remain as future work (P1 routing enhancements).

### 3. ✅ Close run workflow gaps in current LiDAR evaluation loop

- Sources:
  - `docs/lidar/future/track-labeling-auto-aware-tuning.md` (Phase 9)
  - `internal/lidar/monitor/run_track_api.go` (`/reprocess` 501)
  - `internal/lidar/monitor/scene_api.go` (`/evaluations` 501)
- Status: **Complete.** `lidar_evaluations` table (migration 000028), `EvaluationStore`, `handleCreateSceneEvaluation`, `handleListSceneEvaluations`, `handleReprocessRun` all implemented. 501 stubs removed.

### 4. ✅ Documentation consistency sweep (status and checklist reconciliation)

- Status: **Complete.** Reconciled status vs checklist in:
  - `docs/plans/hint-sweep-mode.md` (46/57 items checked)
  - `docs/lidar/visualiser/performance-investigation.md` (22/24 items checked)
  - `docs/features/speed-limit-schedules.md` (status corrected to "Not Implemented")
  - `docs/lidar/future/track-labeling-auto-aware-tuning.md` (Phase 9 items 9.1-9.4 checked)

## P1 (Next)

### 5. Sweep/HINT platform hardening (Phase B/C backlog)

- Source: `docs/plans/industry-standard-ml-solver-expansion-plan.md`
- Focus:
  - Transform pipeline
  - Objective registry/versioning
  - Explainability deltas and confidence penalties
- Why next: builds on current implementation and improves trust and scalability.

### 6. Settling optimization Phase 3 tooling

- Source: `docs/lidar/operations/settling-time-optimization.md`
- Focus: convergence/evaluation tool before adaptive mode.
- Why next: direct operational value with bounded scope.

### 7. Profile comparison system (Phase 9)

- Source: `docs/lidar/future/track-labeling-auto-aware-tuning.md`
- Focus: persist evaluations, expose scene evaluation APIs, add comparison UI.
- Why next: converts single-run evaluation into repeatable decision support.

### 8. Frontend consolidation Phases 1-3

- Source: `docs/plans/frontend-consolidation.md`
- Focus:
  - Migrate status and regions pages
  - Sweep dashboard migration to Svelte
- Why next: reduces dual-stack UI maintenance and supports route simplification goals.

## P2 (Later)

### 9. Distribution/packaging plan execution

- Source: `docs/plans/distribution-packaging-plan.md`
- Why later: high value, but less urgent than runtime maintainability and current workflow gaps.

### 10. Raspberry Pi image pipeline and flashing UX

- Source: `docs/plans/rpi-imager-fork-design.md`
- Why later: deployment UX improvement; depends on packaging and frontend lifecycle work.

### 11. Time-partitioned raw data tables

- Source: `docs/features/time-partitioned-data-tables.md`
- Why later: major architectural/storage change with high complexity and broad blast radius.

### 12. Visualiser QC program (features 1/2/3/5/7/8/10)

- Sources: `docs/lidar/visualiser/06-13*.md`
- Why later: large multi-feature program; should follow P0/P1 stabilization and doc cleanup.

### 13. CLI and Python-plan closure (documentation debt)

- Sources:
  - `docs/features/cli-comprehensive-guide.md`
  - `docs/plans/python-venv-consolidation-plan.md`
- Why later: useful cleanup; lower urgency than LiDAR runtime architecture and active feature gaps.

## P3 (Explicitly Deferred / Research)

### 14. AV dataset integration and motion-capture extensions

- Sources:
  - `docs/lidar/future/av-lidar-integration-plan.md`
  - `docs/lidar/future/motion-capture-architecture.md`
  - `docs/lidar/future/static-pose-alignment-plan.md`
- Why deferred: documents already label these as future/deferred AV research tracks, not core traffic-monitoring priorities.

## Immediate Next Actions

All P0 items are complete. Next priority is P1:

1. **P1-5**: Sweep/HINT platform hardening — transform pipeline, objective registry, explainability
2. **P1-6**: Settling optimization Phase 3 tooling — convergence/evaluation tool
3. **P1-7**: Profile comparison system — UI for cross-run evaluation
4. **P1-8**: Frontend consolidation — migrate status/regions to Svelte

## Notes on Status Reliability

During review, the following status patterns were observed and should be treated cautiously until reconciled:

- Implemented/complete docs with large unchecked blocks
- Deferred docs with "next action: begin implementation" language
- Design docs that reference files/tables no longer present in code

This is why P0 includes a doc consistency pass before broad execution.
