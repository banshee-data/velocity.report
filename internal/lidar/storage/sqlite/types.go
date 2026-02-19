package sqlite

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
)

// Type aliases to avoid import cycles.
//
// The storage layer needs to reference types from higher layers (L3-L6)
// for persistence. To avoid circular dependencies, we define local type
// aliases that point to the canonical definitions in their respective layers.

// TrackedObject represents a tracked object/vehicle from L5 (tracking layer).
type TrackedObject = l5tracks.TrackedObject

// TrackState represents the Kalman filter state of a tracked object.
type TrackState = l5tracks.TrackState

// TrackPoint represents a single point associated with a track.
type TrackPoint = l5tracks.TrackPoint

// WorldCluster represents a cluster of points in world coordinates from L4 (perception layer).
type WorldCluster = l4perception.WorldCluster

// WorldPoint represents a single point in world coordinates from L4 (perception layer).
type WorldPoint = l4perception.WorldPoint

// BackgroundParams contains parameters for background subtraction from L3 (grid layer).
type BackgroundParams = l3grid.BackgroundParams

// DBSCANParams contains parameters for DBSCAN clustering from L4 (perception layer).
type DBSCANParams = l4perception.DBSCANParams

// TrackerConfig contains configuration for the object tracker from L5 (tracking layer).
type TrackerConfig = l5tracks.TrackerConfig

// Constants re-exported from l5tracks for track lifecycle states.
const (
	TrackTentative = l5tracks.TrackTentative
	TrackConfirmed = l5tracks.TrackConfirmed
	TrackDeleted   = l5tracks.TrackDeleted
)

// Function aliases for cross-layer utilities.

// ComputeSpeedPercentiles calculates speed percentiles from a history of speed values.
var ComputeSpeedPercentiles = l6objects.ComputeSpeedPercentiles

// HungarianAssign performs Hungarian algorithm assignment.
var HungarianAssign = l5tracks.HungarianAssign
