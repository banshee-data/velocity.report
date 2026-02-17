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

### Docs-only branch tasks (current)

1. `arena.go` deprecation and layered model relocation design
   - Doc: `docs/lidar/architecture/arena-go-deprecation-and-layered-type-layout-design-20260217.md`
   - Outcome: remove mixed/dead container file and map active shared types to L2/L3/L4 ownership.
2. LiDAR logging stream split (`ops`/`debug`/`trace`) with routing rubric
   - Doc: `docs/lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md`
   - Outcome: isolate actionable logs from high-volume telemetry while preserving `Debugf` compatibility.

## P0 (Start Now)

### 1. Establish L1-L6 code boundaries and orchestration cleanup

- Source: `docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md`
- Why now: this is the main readability and logic issue in core runtime code; it unlocks safer future feature work.

### 2. Simplify HTTP API wiring and route registration

- Sources:
  - `docs/plans/frontend-consolidation.md` (Phase 0 and Phase 5)
  - `internal/lidar/monitor/webserver.go` route sprawl
- Why now: route complexity is already high and directly affects maintainability and feature velocity.

### 3. Close run workflow gaps in current LiDAR evaluation loop

- Sources:
  - `docs/lidar/future/track-labeling-auto-aware-tuning.md` (Phase 9)
  - `internal/lidar/monitor/run_track_api.go` (`/reprocess` 501)
  - `internal/lidar/monitor/scene_api.go` (`/evaluations` 501)
- Why now: these are active user-facing gaps in an otherwise implemented labeling/evaluation flow.

### 4. Documentation consistency sweep (status and checklist reconciliation)

- Why now: multiple docs are marked "implemented/complete" but still show large open checklists, which distorts planning.
- First files to reconcile:
  - `docs/plans/hint-sweep-mode.md`
  - `docs/lidar/visualiser/performance-investigation.md`
  - `docs/features/speed-limit-schedules.md`
  - `docs/lidar/future/track-labeling-auto-aware-tuning.md` (contains internal contradictions)

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

## Immediate Next Actions (2-Week Plan)

1. Execute P0-4 first: reconcile doc status/checklist mismatches so planning signals are trustworthy.
2. Start P0-1/P0-2 in parallel:
   - Layer contracts + pipeline extraction plan
   - Route table refactor scaffold
3. Ship P0-3 minimal closure:
   - Persisted scene evaluations
   - Real `reprocess` behavior or explicit removal of the endpoint

## Notes on Status Reliability

During review, the following status patterns were observed and should be treated cautiously until reconciled:

- Implemented/complete docs with large unchecked blocks
- Deferred docs with "next action: begin implementation" language
- Design docs that reference files/tables no longer present in code

This is why P0 includes a doc consistency pass before broad execution.
