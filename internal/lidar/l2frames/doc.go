// Package l2frames owns Layer 2 (Frames) of the LiDAR data model.
//
// Responsibilities: assembling raw points into complete rotation frames,
// coordinate geometry (polar ↔ Cartesian), and frame-level export.
// Key types: Point, FrameID, Pose.
//
// Dependency rule: L2 may depend on L1, but never on L3+.
//
// See docs/lidar/architecture/lidar-data-layer-model.md for the full
// layer model.
package l2frames
