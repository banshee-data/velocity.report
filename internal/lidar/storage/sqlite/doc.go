// Package sqlite contains SQLite repository implementations for LiDAR
// domain types.
//
// All database read/write operations for tracks, observations, scenes,
// evaluations, and analysis runs belong here rather than in the domain
// layer packages (L3-L6). This keeps domain logic free of SQL noise and
// makes it easier to swap storage backends for testing.
//
// # Track Snapshot Pattern
//
// Two tables store track measurement data with different lifecycles:
//
//   - lidar_tracks — L5 live tracking buffer, pruned after ~5 minutes.
//     PK: track_id. Serves the real-time TrackAPI endpoint.
//   - lidar_run_tracks — L8 immutable snapshots tied to analysis runs.
//     PK: (run_id, track_id). Serves run comparison, labelling, and sweeps.
//
// The 15 shared measurement columns (sensor_id through classification_model)
// are intentionally duplicated in both tables: live tracks are ephemeral,
// run-track snapshots are permanent. Go-layer DRY is enforced through:
//
//   - l5tracks.TrackMeasurement — embedded struct in both TrackedObject and RunTrack
//   - track_measurement_sql.go — shared SQL column list, scan helpers, and insert args
//   - lidar_all_tracks VIEW — UNION ALL for ad-hoc cross-table queries
//
// See docs/lidar/architecture/LIDAR_ARCHITECTURE.md §L5/L8 for the
// full layer context.
//
// See docs/lidar/architecture/lidar-layer-alignment-refactor-review.md §2
// for the design rationale.
package sqlite
