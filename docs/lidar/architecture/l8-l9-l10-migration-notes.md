# L8/L9/L10 Layer Alignment — Migration Notes

- **Status:** Complete — retained as historical record

This document records the package renames and file splits performed to enforce clean layer boundaries between analytics (L8), endpoints (L9), and the HTTP server during the layer alignment refactor.

## Summary

The LIDAR codebase was restructured to enforce clean layer boundaries
between analytics (`l8analytics`), endpoints (`l9endpoints`), and the
HTTP server (`server`). This document records the changes and any
breaking-change rationale.

## Package Renames

| Old name       | New name                                          | Reason                                     |
| -------------- | ------------------------------------------------- | ------------------------------------------ |
| `monitor/`     | (deleted)                                         | Stale, orphaned files; nothing imported it |
| `webserver.go` | `server/server.go`                                | Reflects actual role                       |
| (split files)  | `state.go`, `routes.go`, `tuning.go`, `status.go` | Reduced 1 573-line file to focused units   |

## l10clients Exception

`l9endpoints/l10clients/` contains **only** embedded HTML/CSS/JS assets
for the legacy LIDAR dashboards. It has:

- No `.go` source files
- No exported Go types or functions

The `go:embed` directives in `l9endpoints/legacy_assets.go` reference
these paths. This subtree is transitional — it will be removed once the
Svelte frontend fully replaces the legacy dashboards. Until then it is
the canonical source for those assets.

## Breaking Changes

### Import path: `internal/lidar/monitor`

No external consumers existed. The package was imported by nothing and
contained only stale build artefacts from an earlier refactor. Deletion
is safe.

### webserver.go → server.go split

All functions remain on the same `*Server` receiver in the same Go
package. No import paths changed. The split is purely internal
organisation:

- **server.go** — struct, config, lifecycle (`NewServer`, `Start`, `Close`)
- **state.go** — state accessors and resetters
- **routes.go** — route type, registration, middleware
- **tuning.go** — `/api/lidar/params` handler
- **status.go** — status, health, grid, acceptance, and background handlers

### Items deferred to follow-on work

- Percentile extraction from storage → analytics (depends on speed-percentile plan)
- Full evaluation aggregation move to `l8analytics` (adapters is correct boundary for now)
- `ChartDataProvider` interface design (current `Prepare*()` delegation pattern works)
- Handler interface conversion and route-table interface wiring (structural hardening)
- Architecture DOT graph, SVG artefact, generation script, CI guardrails → see `docs/plans/lidar-architecture-graph-plan.md`
