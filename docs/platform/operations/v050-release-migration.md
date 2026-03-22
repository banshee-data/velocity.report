# v0.5.0 Release Migration

Active plans:

- [v050-backward-compatibility-shim-removal-plan.md](../../plans/v050-backward-compatibility-shim-removal-plan.md)
- [v050-tech-debt-removal-plan.md](../../plans/v050-tech-debt-removal-plan.md)

## Principle

One coordinated breaking-change release. All shims removed in one version
bump. No temporary dual-format shims retained after the cut.

## Shim Removal Summary

| Section | Shim                                   | Status                   |
| ------- | -------------------------------------- | ------------------------ |
| §1      | Track speed contract → `max_speed_mps` | ✅ Complete (#352)       |
| §2      | Sweep legacy request/result fields     | ✅ Removed               |
| §3      | Legacy download endpoint format        | ✅ Removed               |
| §4      | Lenient sweep JSON parsing             | ✅ Removed               |
| §5      | Deploy executor compat methods         | Deferred (Project B)     |
| §6      | `PacketHeader` deprecated struct       | ✅ Removed               |
| §7      | `AddPoints` removal comment            | ✅ Removed               |
| §8      | Type aliases in `lidar/aliases.go`     | Retained (architectural) |
| §9      | Python legacy stats format             | ✅ Removed               |
| §10     | Python config dict-conversion          | ✅ Removed               |
| §11     | Python PyLaTeX fallback stubs          | ✅ Removed               |
| §12     | Svelte `BackgroundCell` legacy fields  | ✅ Removed               |
| §13     | Svelte stats cache bare-array          | ✅ Removed               |
| §14     | Sweep dashboard legacy param aliases   | ✅ Removed               |
| §15     | macOS branch-local speed labels        | ✅ Resolved              |
| §16     | macOS `pointBuffer`                    | Reclassified (renderer)  |
| §17     | macOS legacy playback defaults         | ✅ Removed               |
| §18     | VRLOG speed-key fallback               | Deferred to v0.5.2       |

## Items Explicitly Retained

- Type aliases in `lidar/l3grid/types.go`, `l6objects/types.go`,
  `storage/sqlite/types.go` — avoid import cycles.
- gRPC `UnimplementedServer` embedding — required by protobuf-go.
- gRPC stream type aliases — auto-generated.
- SVG-to-PDF converter fallback chain — operational resilience.
- Font fallback logic in PDF generator.
- DB legacy detection in `db.go` — needed for pre-migration upgrades.
- Old migration files (000002–000019) — immutable history.

## Tech Debt Items (v0.5.0 Sprint)

| Item | Description                             | Status      |
| ---- | --------------------------------------- | ----------- |
| A1   | Sweep dashboard legacy param alias map  | ✅ Complete |
| A2   | Svelte sweep legacy normalisation tests | ✅ Complete |
| A3   | `--lidar-sensor` CLI flag removal       | ✅ Complete |
| A4   | `cmd/transit-backfill` removal          | ✅ Complete |
| A5   | `cmd/tools/scan_transits` removal       | ✅ Complete |
| A6   | CONFIG-RESTRUCTURE Phase 2 Step 13      | ✅ Complete |

## Externally Gated Deferrals

- **`cmd/deploy`** — gated on #210 image pipeline (v0.7.0+).
- **Python PDF elimination** — gated on Go charting migration.
- **VRLOG speed-key fallback** — deferred to v0.5.2 (migration window).

## Config Restructure Status

| Phase | Description                 | Status      |
| ----- | --------------------------- | ----------- |
| 1     | Structural realignment      | ✅ Complete |
| 2     | Essential variable exposure | ✅ Complete |
| 2B    | Experiment contract         | Proposed    |
| 3     | Remaining variable exposure | Proposed    |
