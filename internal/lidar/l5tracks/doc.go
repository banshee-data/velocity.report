// Package l5tracks owns Layer 5 (Tracks) of the LiDAR data model.
//
// Responsibilities: multi-object tracking via Kalman filtering,
// Hungarian assignment, track lifecycle (creation, confirmation,
// coasting, deletion), and quality metrics.
// Key types: TrackedObject, TrackObservation.
//
// Dependency rule: L5 may depend on L1-L4, but never on L6.
// No SQL/database code is allowed in this package.
//
// See docs/lidar/architecture/lidar-data-layer-model.md for the full
// layer model.
package l5tracks
