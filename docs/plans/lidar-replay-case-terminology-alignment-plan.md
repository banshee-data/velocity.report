# LiDAR replay case terminology alignment

- **Status:** Planned for v0.5.1 or v0.5.2
- **Design Phase:** Nomenclature standardisation
- **Scope:** Rename "scene" → "replay case" across Go API, store layer, sweep interfaces, Web routes, and Svelte components.
- **Canonical:** [lidar-pipeline-reference.md](../lidar/architecture/lidar-pipeline-reference.md)

## Overview

The LiDAR evaluation/replay system was originally built using "scene" as the internal naming convention. "Scene" is now ambiguous across the codebase:

- **Physical scene:** The geometric environment a sensor observes (L3 grid, background persistence, settling evaluation); should remain "scene"
- **Replay case:** An evaluation environment tied to a PCAP file, sensor, and optimal parameters for track labelling and auto-tuning; should be called "replay case"

This plan consolidates the "replay case" terminology across the system while preserving "scene" in its geometric context.

## Current state

### Dashboard frontend (complete: v0.5.0)

- [x] `sweep_dashboard.html`, `sweep_dashboard.css`, `sweep_dashboard.js`: all local identifiers renamed
- [x] Test file sweep_dashboard.test.ts: 292 tests pass, API contract refs only

### Database schema (complete: v0.5.x)

- [x] Table `lidar_replay_cases` with `replay_case_id` primary key (migration 031)
- [x] Indexes: `idx_lidar_replay_cases_sensor`, `idx_lidar_replay_cases_pcap`, `idx_lidar_replay_cases_recommended_param_set`
- [x] Field: `recommended_param_set_id` links to `lidar_param_sets` table

### Documentation (complete: v0.5.0)

- [x] `docs/lidar/operations/scene-management-implementation.md` → `replay-case-management-implementation.md` (content updated, rename staged in git)
- [x] Update stale reference in `docs/plans/platform-hub-restructure-plan.md` (line 83)

### Store/API layer (outstanding: rename batch 1)

**Completed in current codebase:**

- Go struct: `ReplayCase` (already using correct name)
- Struct fields: `ReplayCaseID`, `PCAPFile`, etc. (already using correct names)
- Database layer: Reads/writes to `lidar_replay_cases` table ✅

**Pending:**

- File names: `scene_store.go` → `replay_case_store.go`, `scene_api.go` → `replay_case_api.go`
- Method names: `InsertScene()` → `InsertReplayCase()`, `GetScene()` → `GetReplayCase()`, etc.
- API routes: `/api/lidar/scenes` → `/api/lidar/replay-cases`
- Response shape: `scenes` array → `replay_cases` array
- Request types: `CreateSceneRequest` → `CreateReplayCaseRequest`

Breaking change: All consumers (Svelte, tests, integration) must update API URLs and response handling.

### Sweep layer (outstanding: rename batch 2)

- `internal/lidar/sweep/hint.go`: `SceneGetter` → `ReplayCaseGetter`, `HINTScene` → `HINTReplayCase`, `sceneStore`/`sceneGetter` fields
- `internal/lidar/sweep/auto.go`: `SceneStoreSaver` → `ReplayCaseStoreSaver`, setter methods
- `internal/lidar/sweep/hint_notifications.go`: Helper functions `scenePCAPStart` → `casePCAPStart`, `scenePCAPDuration` → `casePCAPDuration`
- Test files: `hint_test.go`, `hint_coverage_test.go`, `auto_test.go`, `auto_coverage_test.go`
- `cmd/radar/radar.go`: `sceneStore`, `hintSceneAdapter`, wiring logic

### Web/Svelte (outstanding: rename batch 3)

- `web/src/lib/api.ts`: Response var `scenes` → `replayCases`, param name `scene` → `replayCase`, preserve API contract URLs (handled by backend)
- `web/src/routes/lidar/replay-cases/+page.svelte`: Local vars `scenes` → `replayCases`, `selectedScene` → `selectedCase`, `loadScenes` → `loadReplayCases`, comments/labels
- `web/src/routes/lidar/tracks/+page.svelte`: Scene selector dropdown; label "Scene" → "Replay Case", local vars sync
- `web/src/routes/lidar/runs/+page.svelte`: Similar updates
- Status page HTML: Remove scene-related UI text

### Database layer (complete: already renamed in v0.5.x migrations)

Migration 031 has renamed:

- Table: `lidar_scenes` → `lidar_replay_cases` ✅
- Columns: `scene_id` → `replay_case_id` ✅
- Indexes: Updated to match ✅
- Field: `optimal_params_json` → `recommended_param_set_id` (links to `lidar_param_sets`) ✅

No further database work required: code changes follow renamed schema.

### Documentation (batch 4: defer to v0.5.2+)

~50+ markdown files reference "scene". Sweep will happen after code rename lands, focusing on evaluation/replay context:

- Replace "scene" with "replay case" where it refers to the PCAP-tied evaluation environment
- Keep "scene" intact in L3 grid, background persistence, settling, tracking (physical geometry context)
- Update plan docs, architecture docs, and troubleshooting guides

## Breaking changes

**API Contract Change (Batch 1):**

```
GET /api/lidar/scenes → GET /api/lidar/replay-cases
POST /api/lidar/scenes → POST /api/lidar/replay-cases
Response: { scenes: [...] } → { replay_cases: [...] }
```

All consumers (Svelte, testing, integration) must update. This is a deliberate breaking change for v0.5.1 or v0.5.2.

## Testing expectations

1. Store tests must verify both old and new method names work correctly
2. API tests must assert response shape (array property name, field names)
3. Sweep interface tests must mock renamed types
4. Svelte component tests must verify data flow through renamed variables
5. E2E tests on replay-cases page must verify selector population and filtering

## Exclusions (keep "Scene")

These files/uses of "scene" remain unchanged:

- L3 grid layer (`internal/lidar/l3grid/`): `SceneSignature()`, scene hash matching
- Background persistence (`internal/lidar/l3grid/background_persistence.go`)
- L4 perception, L5 tracking geometric operations
- Sweep metrics: "Scene-level foreground capture" (refers to full sensor view, not replay case)
- Database migrations (immutable history)
- Maths proposals and architecture docs (domain-specific geometric context)

## Rollout strategy

1. **Batch 1 (v0.5.1 or v0.5.2):** Store + API layer rename (API-breaking)
2. **Batch 2:** Sweep interfaces and wiring
3. **Batch 3:** Web/Svelte local updates
4. **Batch 4:** Documentation sweep (lower priority, can extend into v0.5.3+)

All batches go to main together to maintain API consistency.

## References

- Complete audit: `/memories/session/replay-case-terminology-audit.md`
- Taxonomy:
  - Category A (keep): L3 grid, background, settling, tracking geometry (18 files, ~100 refs)
  - Category B (rename): Go store/API/sweep (40+ files, ~1,200 refs)
  - Category C (rename): Web/Svelte (6 files, ~160 refs)
  - Category D (immutable): Migrations, maths, historical docs (50+ files, ~900 refs)
