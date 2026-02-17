// Package sqlite contains SQLite repository implementations for LiDAR
// domain types.
//
// All database read/write operations for tracks, observations, scenes,
// evaluations, and analysis runs belong here rather than in the domain
// layer packages (L3-L6). This keeps domain logic free of SQL noise and
// makes it easier to swap storage backends for testing.
//
// See docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md §2
// for the design rationale.
package sqlite
