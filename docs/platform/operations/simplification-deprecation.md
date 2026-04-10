# Simplification and Deprecation

Active plan: [platform-simplification-and-deprecation-plan.md](../../plans/platform-simplification-and-deprecation-plan.md)

This document tracks the programme to reduce non-core operational surface area by deprecating legacy tools, compatibility shims, and redundant interfaces across the codebase.

## Goal

Reduce non-core operational surface area and clean up backward compatibility
debt across Make targets, `cmd/` applications, CLI flags, metrics/frontend
surfaces, and data model shims.

## Core vs Non-Core

**Core:** `cmd/radar` binary and its API serving path, database migration/query
paths, web app on `:8080`.

**Non-core/candidates:** Legacy deployment wrappers, one-off migration binaries,
local plotting helpers, CLI flags tied to transitional debug pathways.

> **Deprecation targets and progress:** see the active plan above.

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
3. **Sweep API:** Legacy request/result fields removed; `param_values` only.
4. **Report download:** Query-parameter endpoint removed; path-based only.
5. **Stats API:** Bare-array response removed; always `{ metrics, histogram }`.
6. **Sweep handler:** Malformed JSON now returns 400.

## Consolidation

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

## Binary Size

The velocity-report Linux ARM64 binary must stay below 40 MB. See the
active plan for root cause, phases, and checklist.

- **Plan:** [binary-size-reduction-plan](../../plans/binary-size-reduction-plan.md)
- **Target:** < 40 MB production binary (currently ~211 MB, almost entirely
  stale embeds)
- **CI gate:** `scripts/check-binary-size.sh` (planned)

| Segment                | Size   | Notes                                  |
| ---------------------- | ------ | -------------------------------------- |
| Stale `static/` embeds | 172 MB | Root cause — build hygiene, not Svelte |
| Go code + all deps     | 38 MB  | Includes SQLite, gRPC, protobuf, gonum |
| `web/build/` (current) | 1.1 MB | The actual SvelteKit build             |

Phases: (1) eliminate stale embeds (~172 MB), (2) strip debug symbols (~8–12 MB),
(3) CI binary size gate (45 MB threshold), (4) optional further reductions
(echarts removal, lazy Leaflet, UPX).
