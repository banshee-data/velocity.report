// Package l3grid owns Layer 3 (Grid) of the LiDAR data model.
//
// Responsibilities: background model learning, foreground extraction,
// region management, and background snapshot persistence.
// Key types: BgSnapshot, RegionSnapshot, RegionData.
//
// Dependency rule: L3 may depend on L1-L2, but never on L4+.
// No SQL/database code is allowed in this package.
//
// See docs/lidar/architecture/lidar-data-layer-model.md for the full
// layer model.
package l3grid
