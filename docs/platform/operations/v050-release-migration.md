# v0.5.0 Release migration

- **Status:** Complete; retained as migration reference

This document is the migration guide for the v0.5.0 release, which coordinates all breaking changes and backward-compatibility shim removals into a single version bump.

## Principle

One coordinated breaking-change release. All shims removed in one version
bump. No temporary dual-format shims retained after the cut.

## Shim removal outcomes

| Outcome             | Sections                        | Notes                                                                                                                                                               |
| ------------------- | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Removed in code     | §2-§4, §6, §7, §9-§13, §17, §18 | All non-SQL-migration shims removed; sweep fields, download endpoint, `PacketHeader`, Python/web/macOS fallback code, and VRLOG legacy speed-key fallback all clean |
| Complete / resolved | §1, §15                         | Speed contract reset landed in #352; branch-local percentile surfaces never merged; `avgSpeedMps`/`maxSpeedMps` verified                                            |
| Deferred / retained | §5, §8                          | Either owned by another plan or still an active implementation path rather than a removable shim today                                                              |
| Reclassified        | §16                             | `pointBuffer` is a rendering fallback, not a compat shim; tracked as renderer-retirement work                                                                       |

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
