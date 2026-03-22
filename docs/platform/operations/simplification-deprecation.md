# Simplification and Deprecation

Active plan: [platform-simplification-and-deprecation-plan.md](../../plans/platform-simplification-and-deprecation-plan.md)

## Goal

Reduce non-core operational surface area and clean up backward compatibility
debt across Make targets, `cmd/` applications, CLI flags, metrics/frontend
surfaces, and data model shims.

## Core vs Non-Core

**Core:** `cmd/radar` binary and its API serving path, database migration/query
paths, web app on `:8080`.

**Non-core/candidates:** Legacy deployment wrappers, one-off migration binaries,
local plotting helpers, CLI flags tied to transitional debug pathways.

## Deprecation Targets (Prioritised)

1. **`cmd/deploy` deprecation path** — remove after #210 image pipeline
   milestones.
2. **Deployment Make target cleanup** — `setup-radar`, `deploy-*`,
   `build-deploy*`.
3. **Data model and API compat-shim removal** — v0.5.0 breaking changes
   (see [v050-release-migration.md](v050-release-migration.md)).
4. **`cmd/transit-backfill` removal** — ✅ Complete. Replaced by
   `velocity-report transits rebuild`.
5. **LiDAR forwarding flag simplification.**
6. **Stats/plot/API-shortcut target consolidation** after #252 parity.

## Deploy Retirement Gate

Removal of `cmd/deploy` is gated on **all** of:

1. #210 image pipeline operational (bootable Pi image, service starts, API
   responds).
2. Packaging path confirmed (successful end-to-end deployment).
3. Migration period elapsed (at least one minor release with both paths).
4. No active deploy-tool users confirmed.

Once met, `cmd/deploy/`, `internal/deploy/`, 8+ Makefile targets, and
`scripts/setup-radar-host.sh` are removed.

## v0.5.0 Breaking Changes

1. **Track speed contract:** `peak_speed_mps` → `max_speed_mps`; percentiles
   reserved for grouped/report aggregates only.
2. **Deploy surface deprecated** — prints warnings; removal in v0.7.0+.
3. **`cmd/transit-backfill` soft-deprecated.**
4. **Sweep API:** Legacy request/result fields removed; `param_values` only.
5. **Report download:** Query-parameter endpoint removed; path-based only.
6. **Stats API:** Bare-array response removed; always `{ metrics, histogram }`.
7. **Sweep handler:** Malformed JSON now returns 400.

## Consolidation: Option B (Recommended) ✅

Consolidate on Svelte surface via
[web-frontend-consolidation](../../ui/web-frontend-consolidation.md).
Retire duplicated stats/debug HTML surfaces after parity is reached.

## Project Status

| Project | Scope                  | Status              |
| ------- | ---------------------- | ------------------- |
| A       | Deprecation signalling | ✅ Complete         |
| B       | Deploy retirement gate | Gated on #210       |
| C       | Frontend consolidation | Blocked on #252     |
| D       | CLI simplification     | Partially addressed |
| E       | Compat shim removal    | ✅ Complete         |
