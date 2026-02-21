# LiDAR Math Foundations Audit

Status: Active
Scope: `docs/maths/**`, velocity-coherence planning docs, and `internal/lidar/**` implementation.

## 1. Built vs Proposed

### Built (implemented in code)

1. L3 foreground/background extraction math is implemented in the production path:
   - `internal/lidar/pipeline/tracking_pipeline.go:240` calls `ProcessFramePolarWithMask`.
   - Core logic exists in `internal/lidar/l3grid/foreground.go`.
2. L4 clustering math is implemented:
   - world transform and DBSCAN in `internal/lidar/l4perception/cluster.go`.
3. L5 tracking math is implemented:
   - Kalman + gating + assignment in `internal/lidar/l5tracks/tracking.go`.

### Proposed (not implemented yet)

1. Velocity-coherent foreground extraction:
   - explicitly marked planning-only in `docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md:3`.
   - no velocity-coherent extractor files currently exist under `internal/lidar/`.
2. L4 ground plane tile fitting and vector scene map:
   - `docs/plans/lidar-architecture-vector-scene-map-plan.md:3` is `Status: Proposed`.
   - current runtime L4 ground removal is still height-band filtering (`internal/lidar/l4perception/ground.go:18`).
3. Unified L3/L4 settlement core:
   - still a proposal (`docs/maths/proposals/20260219-unify-l3-l4-settling.md:3`).

## 2. Velocity Coherence Separation

To keep foundations stable, split into two independent workstreams with one narrow interface.

### Workstream A: Vector-grid foundations (static scene model)

Owns:

- L3 cell settlement/reliability model
- region adaptation behavior
- future L4 ground-surface/vector-scene representations

Must not depend on:

- velocity-estimation logic
- long-tail tracking heuristics

Primary packages:

- `internal/lidar/l3grid/*`
- future L4 geometry packages (new, separate from extractor logic)

### Workstream B: Velocity-coherent foreground algorithm (motion model)

Owns:

- frame-to-frame correspondence
- point velocity confidence
- velocity-aware clustering/gating
- optional hybrid merge policy

Must not depend on:

- region-manager internals
- vector-scene-map structures

Primary packages:

- new extractor modules (for example `internal/lidar/extractors/*`)

### Interface contract between A and B

Use a single foreground extractor contract:

- input: frame points + timestamp
- output: foreground mask + extractor metrics

Pipeline keeps downstream behavior identical (transform, ground filter, clustering, tracking, persistence) and swaps only the foreground source.

## 3. Gaps Found

### [Resolved] Region-adaptive math parity on production mask path

Resolution:

- `ProcessFramePolarWithMask` now applies per-region overrides for noise, neighbor confirmation, and settle alpha (`internal/lidar/l3grid/foreground.go`).
- Added regression test: `internal/lidar/l3grid/foreground_test.go`.

Impact:

- Production behavior now aligns with adaptive-region maths and operations docs.

### [High] Velocity-coherent design docs and code state are inconsistent/fragmented

Evidence:

- planning doc says not implemented (`docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md:3`).
- architecture spec includes branch-complete tables that can be misread without full context (`docs/plans/lidar-architecture-dynamic-algorithm-selection-plan.md:25`).
- no extractor files exist in `internal/lidar/` despite referenced names.
- pipeline config has no `ExtractorMode`/hybrid fields (`internal/lidar/pipeline/tracking_pipeline.go:109`).

Impact:

- unclear implementation truth makes roadmap and dependency planning brittle.

### [Resolved] Broken internal links in velocity-coherence math/planning docs

Resolution:

- Updated velocity plan links to canonical maths proposal:
  - `docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md`
  - `docs/maths/proposals/20260220-velocity-coherent-foreground-extraction.md`

Impact:

- math spec and implementation plan links are now connected.

### [Medium] Ground-plane maths status reads implementation-aligned while runtime is still height-band filtering

Evidence:

- status line says architecture + implementation note (`docs/maths/ground-plane-maths.md:3`).
- implementation currently provides `HeightBandFilter` only (`internal/lidar/l4perception/ground.go:18`).

Impact:

- readers may assume tile-plane solver exists in runtime when it does not.

### [Medium] LiDAR doc path drift (partially resolved)

Evidence:

- fixed active-doc path references for velocity plan baseline and roadmap links in key docs.
- additional legacy docs still contain stale package/file references.

Impact:

- slows onboarding and causes incorrect assumptions during refactors.

### [Medium] Runtime config POST API is still schema-divergent (improved)

Evidence:

- explicit parity TODO and missing-key list remain in handler (`internal/lidar/monitor/webserver.go`).
- `max_tracks` POST support has now been wired.

Impact:

- parameter updates can drift from canonical tuning schema.

### [Low] Explicitly incomplete features in lidar code should be tracked as non-foundational

Evidence:

- snapshot-id export path returns not implemented (`internal/lidar/monitor/export_handlers.go:28`).
- track point-cloud export placeholders (`internal/lidar/adapters/track_export.go:62`, `internal/lidar/adapters/track_export.go:208`).
- recorder still uses JSON placeholder serialization and linear seek (`internal/lidar/visualiser/recorder/recorder.go:121`, `internal/lidar/visualiser/recorder/recorder.go:361`).

Impact:

- these are operational/tooling limitations, not core math blockers, but should be visible in planning.

## 4. Recommended Foundation Tasks

1. Keep a single source-of-truth status page for velocity-coherent work (implemented/planned/deferred), then align all references.
2. Continue stale package-path cleanup across older LiDAR docs.
3. Maintain explicit implemented-vs-planned labels in ground-plane maths and related architecture docs.
4. Resolve remaining config parity TODO in `/api/lidar/params` so tuning IO matches canonical schema.

## 5. Validation Run

LiDAR package tests were run after audit:

- `go test ./internal/lidar/...` passed.
