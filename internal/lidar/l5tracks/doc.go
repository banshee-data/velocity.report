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
// Time-domain rule: every estimator-temporal quantity derives from capture
// time, never from the host's wall clock. See time_domain.go; a test scans
// this package's non-test sources to hold the rule.
//
// Continuity rule: existence is not observation. Every instant and history
// point records whether it was observed or coasted, and why; the default-off
// occlusion-continuity options live in continuity.go.
//
// See docs/lidar/architecture/LIDAR_ARCHITECTURE.md for the full
// layer model.
package l5tracks
