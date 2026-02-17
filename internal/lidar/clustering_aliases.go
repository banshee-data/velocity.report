package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Type aliases for clustering types that have been migrated to l4perception.
// These maintain backward compatibility for existing code that imports from internal/lidar.

// WorldPoint is an alias for l4perception.WorldPoint.
// Use l4perception.WorldPoint directly in new code.
type WorldPoint = l4perception.WorldPoint

// SpatialIndex is an alias for l4perception.SpatialIndex.
// Use l4perception.SpatialIndex directly in new code.
type SpatialIndex = l4perception.SpatialIndex

// DBSCANParams is an alias for l4perception.DBSCANParams.
// Use l4perception.DBSCANParams directly in new code.
type DBSCANParams = l4perception.DBSCANParams

// ClustererInterface is an alias for l4perception.ClustererInterface.
// Use l4perception.ClustererInterface directly in new code.
type ClustererInterface = l4perception.ClustererInterface

// ClusteringParams is an alias for l4perception.ClusteringParams.
// Use l4perception.ClusteringParams directly in new code.
type ClusteringParams = l4perception.ClusteringParams

// DBSCANClusterer is an alias for l4perception.DBSCANClusterer.
// Use l4perception.DBSCANClusterer directly in new code.
type DBSCANClusterer = l4perception.DBSCANClusterer

// PointPolar is an alias for l4perception.PointPolar.
// Use l4perception.PointPolar directly in new code.
type PointPolar = l4perception.PointPolar

// OrientedBoundingBox is an alias for l4perception.OrientedBoundingBox.
// Use l4perception.OrientedBoundingBox directly in new code.
type OrientedBoundingBox = l4perception.OrientedBoundingBox

// Function aliases for clustering functions that have been migrated to l4perception.

// DBSCAN is an alias for l4perception.DBSCAN.
// Use l4perception.DBSCAN directly in new code.
var DBSCAN = l4perception.DBSCAN

// NewSpatialIndex is an alias for l4perception.NewSpatialIndex.
// Use l4perception.NewSpatialIndex directly in new code.
var NewSpatialIndex = l4perception.NewSpatialIndex

// DefaultDBSCANParams is an alias for l4perception.DefaultDBSCANParams.
// Use l4perception.DefaultDBSCANParams directly in new code.
var DefaultDBSCANParams = l4perception.DefaultDBSCANParams

// TransformToWorld is an alias for l4perception.TransformToWorld.
// Use l4perception.TransformToWorld directly in new code.
var TransformToWorld = l4perception.TransformToWorld

// TransformPointsToWorld is an alias for l4perception.TransformPointsToWorld.
// Use l4perception.TransformPointsToWorld directly in new code.
var TransformPointsToWorld = l4perception.TransformPointsToWorld

// NewDBSCANClusterer is an alias for l4perception.NewDBSCANClusterer.
// Use l4perception.NewDBSCANClusterer directly in new code.
var NewDBSCANClusterer = l4perception.NewDBSCANClusterer

// NewDefaultDBSCANClusterer is an alias for l4perception.NewDefaultDBSCANClusterer.
// Use l4perception.NewDefaultDBSCANClusterer directly in new code.
var NewDefaultDBSCANClusterer = l4perception.NewDefaultDBSCANClusterer

// EstimateOBBFromCluster is an alias for l4perception.EstimateOBBFromCluster.
// Use l4perception.EstimateOBBFromCluster directly in new code.
var EstimateOBBFromCluster = l4perception.EstimateOBBFromCluster

// SmoothOBBHeading is an alias for l4perception.SmoothOBBHeading.
// Use l4perception.SmoothOBBHeading directly in new code.
var SmoothOBBHeading = l4perception.SmoothOBBHeading

// SphericalToCartesian is an alias for l4perception.SphericalToCartesian.
// Use l4perception.SphericalToCartesian directly in new code.
var SphericalToCartesian = l4perception.SphericalToCartesian

// ApplyPose is an alias for l4perception.ApplyPose.
// Use l4perception.ApplyPose directly in new code.
var ApplyPose = l4perception.ApplyPose

// Constants

// EstimatedPointsPerCell is an alias for l4perception.EstimatedPointsPerCell.
const EstimatedPointsPerCell = l4perception.EstimatedPointsPerCell

// IdentityTransform4x4 is an alias for l4perception.IdentityTransform4x4.
var IdentityTransform4x4 = l4perception.IdentityTransform4x4
