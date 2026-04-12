# LiDAR architecture graph plan

- **Status:** Active
- **Layers:** All (L1–L10)
- **Parent:** [L8/L9/L10 refactor plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md)
- **Related:** [L7 Scene plan](lidar-l7-scene-plan.md)
- **Canonical:** [lidar-pipeline-reference.md](../lidar/architecture/lidar-pipeline-reference.md)

## Summary

Generate and maintain a DOT graph of the LiDAR package dependency structure under [internal/lidar/](../../internal/lidar). This was originally a final-verification item in the L8/L9/L10 refactor plan; it is extracted here because it is an independent deliverable that does not block the refactor landing.

## Scope

1. A DOT graph describing the directed dependency edges between `internal/lidar/*` packages.
2. A rendered SVG checked into the repository.
3. A generation script that can reproduce both artefacts from the current `go.mod` / import graph.
4. A CI guardrail or `make` target that verifies the checked-in graph matches the live import structure.

## Checklist

- [ ] DOT graph added under [docs/lidar/architecture/](../lidar/architecture) or equivalent location
- [ ] SVG rendered from DOT and checked in alongside
- [ ] generation script added (e.g. `scripts/generate-lidar-graph.sh` using `go list` + `dot`)
- [ ] `make lint-docs` or equivalent target verifies graph freshness
- [ ] CI runs the verification target

## Design notes

- Use `go list -json ./internal/lidar/...` to extract the import graph; no manual maintenance.
- Filter to `internal/lidar/*` imports only; exclude stdlib and external modules.
- Colour or group nodes by layer (L1–L10, cross-cutting, server, storage).
- Keep the script POSIX-portable where practical; Graphviz is the only external dependency.

## Non-Goals

- Graphing the full module dependency tree (only internal LiDAR packages).
- Documenting non-LiDAR packages.
