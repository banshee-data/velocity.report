// Package l4perception owns Layer 4 (Perception) of the LiDAR data model.
//
// Responsibilities: polar-to-world coordinate transformation, ground
// removal, voxel downsampling, and DBSCAN clustering.
// Key types: WorldPoint, WorldCluster.
//
// Dependency rule: L4 may depend on L1-L3, but never on L5+.
// No SQL/database code is allowed in this package.
//
// See docs/lidar/architecture/lidar-data-layer-model.md for the full
// layer model.
package l4perception
