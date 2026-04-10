# Pre-v0.5.0 Schema Hardening

**Status:** Complete

## Summary

One-pass schema cleanup before the `v0.5.0` branch cut:

- Move free-form annotations from live-track-owned to replay-owned table
- Enable `PRAGMA foreign_keys = ON` globally
- Add real FKs where SQLite `rowid` references were implicit
- Nullable columns store `NULL`, not sentinel values
- Regression tests cover delete/nullability/FK paths

## Key Migrations

| Migration | Purpose                                                           |
| --------- | ----------------------------------------------------------------- |
| `000033`  | Replace `lidar_track_annotations` with `lidar_replay_annotations` |
| `000034`  | Stricter schema shape for FK-on operation                         |

## Related

- [Schema Simplification Migration 030 Plan](../../plans/schema-simplification-migration-030-plan.md)
- [Track Storage Consolidation](track-storage-consolidation.md)
