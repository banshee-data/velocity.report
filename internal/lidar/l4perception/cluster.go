package l4perception

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export clustering types from the parent package.
// These aliases enable gradual migration: callers can import from
// l4perception while the implementation remains in internal/lidar.

// SpatialIndex accelerates nearest-neighbour queries for DBSCAN.
type SpatialIndex = lidar.SpatialIndex

// DBSCANParams holds configuration for DBSCAN clustering.
type DBSCANParams = lidar.DBSCANParams

// Constructor and function re-exports.

// NewSpatialIndex creates a SpatialIndex with the given cell size.
var NewSpatialIndex = lidar.NewSpatialIndex

// DefaultDBSCANParams returns production-default DBSCAN parameters.
var DefaultDBSCANParams = lidar.DefaultDBSCANParams

// DBSCAN performs density-based spatial clustering on world points.
var DBSCAN = lidar.DBSCAN

// TransformToWorld converts polar points to world coordinates using the sensor pose.
var TransformToWorld = lidar.TransformToWorld

// TransformPointsToWorld converts Cartesian points to world coordinates using the sensor pose.
var TransformPointsToWorld = lidar.TransformPointsToWorld
