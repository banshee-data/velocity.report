package lidar

import "github.com/banshee-data/velocity.report/internal/lidar/l6objects"

// Type aliases for feature extraction types migrated to l6objects.
type ClusterFeatures = l6objects.ClusterFeatures
type TrackFeatures = l6objects.TrackFeatures

// Function aliases re-exported from l6objects.
var (
ExtractClusterFeatures = l6objects.ExtractClusterFeatures
ExtractTrackFeatures   = l6objects.ExtractTrackFeatures
SortedFeatureNames     = l6objects.SortedFeatureNames
SortFeatureImportance  = l6objects.SortFeatureImportance
)
