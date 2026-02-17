// Package l6objects owns Layer 6 (Objects) of the LiDAR data model.
//
// Responsibilities: track classification (vehicle, pedestrian, noise),
// quality assessment, and taxonomy management.
//
// Dependency rule: L6 may depend on L1-L5.
// No SQL/database code is allowed in this package.
//
// See docs/lidar/architecture/lidar-data-layer-model.md for the full
// layer model.
package l6objects
