// Package l6objects owns Layer 6 (Objects) of the LiDAR data model.
//
// Responsibilities: track classification (vehicle, pedestrian, noise),
// quality assessment, and taxonomy management.
//
// Dependency rule: L6 may depend on L1-L5.
// No SQL/database code is allowed in this package.
//
// Type definitions: This package defines local copies of types from the parent
// lidar package to avoid import cycles. Some fields (e.g., speedHistory in
// TrackedObject) are intentionally unexported to maintain encapsulation while
// remaining accessible within the package for internal computations.
//
// See docs/lidar/architecture/lidar-data-layer-model.md for the full
// layer model.
package l6objects
