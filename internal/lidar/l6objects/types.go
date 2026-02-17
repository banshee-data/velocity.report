package l6objects

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// Re-export types from lower layers to avoid import cycle issues.
// These type aliases allow l6objects to use canonical types from l5tracks
// and l4perception without duplicating definitions.

// TrackedObject is the canonical tracking object type from l5tracks.
type TrackedObject = l5tracks.TrackedObject

// TrackState represents the lifecycle state of a track.
type TrackState = l5tracks.TrackState

// TrackPoint represents a single point in a track's history.
type TrackPoint = l5tracks.TrackPoint

// WorldPoint represents a point in Cartesian world coordinates (site frame).
type WorldPoint = l4perception.WorldPoint

// OrientedBoundingBox represents a 7-DOF (7 Degrees of Freedom) 3D bounding box.
type OrientedBoundingBox = l4perception.OrientedBoundingBox

// WorldCluster represents a cluster of world points.
type WorldCluster = l4perception.WorldCluster

// Constants re-exported from l5tracks.
const (
	TrackTentative = l5tracks.TrackTentative
	TrackConfirmed = l5tracks.TrackConfirmed
	TrackDeleted   = l5tracks.TrackDeleted
)
