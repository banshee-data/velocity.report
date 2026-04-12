# v0.5.0 Release migration

- **Status:** Complete; retained as migration reference

Plan: [v050-backward-compatibility-shim-removal-plan.md](../../plans/v050-backward-compatibility-shim-removal-plan.md); Complete

This document is the migration guide for the v0.5.0 release, which coordinates all breaking changes and backward-compatibility shim removals into a single version bump.

## Principle

One coordinated breaking-change release. All shims removed in one version
bump. No temporary dual-format shims retained after the cut.

> **Shim removal status and tech-debt tracking:** see the active plan above.

## Items explicitly retained

- Type aliases in `lidar/l3grid/types.go`, `l6objects/types.go`,
  `storage/sqlite/types.go`: avoid import cycles.
- gRPC `UnimplementedServer` embedding: required by protobuf-go.
- gRPC stream type aliases: auto-generated.
- SVG-to-PDF converter fallback chain: operational resilience.
- Font fallback logic in PDF generator.
- DB legacy detection in `db.go`: needed for pre-migration upgrades.
- Old migration files (000002–000019): immutable history.

## Externally gated deferrals

- **`cmd/deploy`**: gated on #210 image pipeline (v0.7.0+).
- **Python PDF elimination**: gated on Go charting migration.
- **VRLOG speed-key fallback**: deferred to v0.5.2 (migration window).

## Config restructure status

| Phase | Description                 | Status      |
| ----- | --------------------------- | ----------- |
| 1     | Structural realignment      | ✅ Complete |
| 2     | Essential variable exposure | ✅ Complete |
| 2B    | Experiment contract         | Proposed    |
| 3     | Remaining variable exposure | Proposed    |
